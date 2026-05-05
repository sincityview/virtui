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
type connectMsg struct {
	client *libvirt.Client
	err    error
}

func (a *App) connect() tea.Cmd {
	return func() tea.Msg {
		client, err := libvirt.NewClient()
		if err != nil {
			return connectMsg{err: err}
		}
		return connectMsg{client: client}
	}
}

func (a *App) refresh() tea.Cmd {
	client := a.client
	return func() tea.Msg {
		if client == nil {
			return nil
		}

		domains, err := client.ListDomains()
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

func (a *App) doDelete() tea.Cmd {
	if len(a.domains) == 0 || a.cursor >= len(a.domains) {
		return nil
	}
	name := a.domains[a.cursor].Name
	a.deleteMode = false

	return func() tea.Msg {
		if err := a.client.RemoveDomain(name); err != nil {
			return actionResultMsg(fmt.Sprintf("✗ Delete %s: %v", name, err))
		}
		return actionResultMsg(fmt.Sprintf("✓ Deleted %s", name))
	}
}

func (a *App) doClone(cloneName string) tea.Cmd {
	if len(a.domains) == 0 || a.cursor >= len(a.domains) {
		return nil
	}
	name := a.domains[a.cursor].Name
	a.cloneMode = false
	a.cloneName = ""

	return func() tea.Msg {
		if err := a.client.CloneDomain(name, cloneName); err != nil {
			return actionResultMsg(fmt.Sprintf("✗ Clone %s → %s: %v", name, cloneName, err))
		}
		return actionResultMsg(fmt.Sprintf("✓ Clone %s → %s", name, cloneName))
	}
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
