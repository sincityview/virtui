package tui

import (
	"fmt"
	"os/exec"
	"time"
	"os"
	"sort"

	"github.com/charmbracelet/bubbletea"
	"virtui/internal/libvirt"
)

type refreshMsg struct{}
type initMsg struct{ domains []libvirt.DomainInfo }
type errMsg struct{ err error }
type actionResultMsg string

func (a *App) refresh() tea.Cmd {
	return func() tea.Msg {
		if a.client == nil {
			client, err := libvirt.NewClient()
			if err != nil {
				return errMsg{err}
			}
			a.client = client
		}

		domains, err := a.client.ListDomains()
		if err != nil {
			return errMsg{err}
		}

		sort.Slice(domains, func(i, j int) bool {
			return domains[i].Name < domains[j].Name
		})

		return initMsg{domains: domains}
	}
}

func (a *App) autoRefresh() tea.Cmd {
	return tea.Every(5*time.Second, func(time.Time) tea.Msg {
		return refreshMsg{}
	})
}

func (a *App) doAction(action string, fn func(string) error) tea.Cmd {
	if len(a.domains) == 0 || a.cursor >= len(a.domains) {
		return nil
	}
	name := a.domains[a.cursor].Name

	return func() tea.Msg {
		if err := fn(name); err != nil {
			return actionResultMsg(fmt.Sprintf("✗ %s %s: %v", action, name, err))
		}
		return actionResultMsg(fmt.Sprintf("✓ %s %s", action, name))
	}
}

func (a *App) connectToConsole() tea.Cmd {
	if len(a.domains) == 0 || a.cursor >= len(a.domains) {
		return nil
	}

	name := a.domains[a.cursor].Name
	c := exec.Command("virsh", "console", name)

	return tea.ExecProcess(c, func(err error) tea.Msg {
		if err != nil {
			return actionResultMsg(fmt.Sprintf("Ошибка консоли %s: %v", name, err))
		}
		return refreshMsg{}
	})
}

func (a *App) editDomainXML() tea.Cmd {
	if len(a.domains) == 0 || a.cursor >= len(a.domains) {
		return nil
	}
	name := a.domains[a.cursor].Name

	xml, err := a.client.GetXML(name)
	if err != nil {
		return func() tea.Msg { return actionResultMsg("✗ Ошибка XML: " + err.Error()) }
	}

	tmpFile, err := os.CreateTemp("", "virtui-*.xml")
	if err != nil {
		return func() tea.Msg { return actionResultMsg("✗ Ошибка файла: " + err.Error()) }
	}
	tmpFile.WriteString(xml)
	tmpPath := tmpFile.Name()
	tmpFile.Close()

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi" 
	}

	c := exec.Command(editor, tmpPath)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		defer os.Remove(tmpPath)
		if err != nil {
			return actionResultMsg("✗ Редактор: " + err.Error())
		}

		newXml, _ := os.ReadFile(tmpPath)
		if string(newXml) == xml {
			return actionResultMsg("ℹ Изменений нет")
		}

		if err := a.client.DefineXML(string(newXml)); err != nil {
			return actionResultMsg("✗ Ошибка сохранения: " + err.Error())
		}
		return actionResultMsg("✓ XML обновлен для " + name)
	})
}
