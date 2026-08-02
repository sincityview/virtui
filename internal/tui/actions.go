// virtui/internal/tui/actions.go
package tui

import (
	"fmt"
	"os/exec"
	"regexp"
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
		client, err := libvirt.NewClient(a.cfg)
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
	client := a.client

	return func() tea.Msg {
		if client == nil {
			return actionResultMsg("✗ client disconnected")
		}
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
	client := a.client

	c := exec.Command("virsh", "console", name)

	return tea.ExecProcess(c, func(err error) tea.Msg {
		if client == nil {
			return actionResultMsg("✗ client disconnected")
		}
		if err != nil {
			return actionResultMsg(fmt.Sprintf("Console error %s: %v", name, err))
		}
		return refreshMsg{}
	})
}

func (a *App) doDelete() tea.Cmd {
	if len(a.domains) == 0 || a.cursor >= len(a.domains) {
		return nil
	}
	name := a.domains[a.cursor].Name
	client := a.client
	a.deleteMode = false

	return func() tea.Msg {
		if client == nil {
			return actionResultMsg("✗ client disconnected")
		}
		if err := client.RemoveDomain(name); err != nil {
			return actionResultMsg(fmt.Sprintf("✗ Delete %s: %v", name, err))
		}
		return actionResultMsg(fmt.Sprintf("✓ Deleted %s", name))
	}
}

var validCloneName = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

func (a *App) doClone(cloneName string) tea.Cmd {
	if len(a.domains) == 0 || a.cursor >= len(a.domains) {
		return nil
	}

	if !validCloneName.MatchString(cloneName) {
		a.cloneMode = false
		a.cloneName = ""
		return func() tea.Msg {
			return actionResultMsg("✗ Invalid clone name: only alphanumeric, dots, dashes, underscores allowed")
		}
	}

	name := a.domains[a.cursor].Name
	client := a.client
	a.cloneMode = false
	a.cloneName = ""

	return func() tea.Msg {
		if client == nil {
			return actionResultMsg("✗ client disconnected")
		}
		if err := client.CloneDomain(name, cloneName); err != nil {
			return actionResultMsg(fmt.Sprintf("✗ Clone %s → %s: %v", name, cloneName, err))
		}
		return actionResultMsg(fmt.Sprintf("✓ Clone %s → %s", name, cloneName))
	}
}

func writeTempXML(content string) (path string, err error) {
	f, err := os.CreateTemp("", "virtui-*.xml")
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

func editorCommand() string {
	if e := os.Getenv("EDITOR"); e != "" {
		return e
	}
	return "vi"
}

func (a *App) editDomainXML() tea.Cmd {
	if len(a.domains) == 0 || a.cursor >= len(a.domains) {
		return nil
	}
	name := a.domains[a.cursor].Name
	client := a.client

	xml, err := client.GetXML(name)
	if err != nil {
		return func() tea.Msg { return actionResultMsg("✗ XML error: " + err.Error()) }
	}

	tmpPath, err := writeTempXML(xml)
	if err != nil {
		return func() tea.Msg { return actionResultMsg("✗ File error: " + err.Error()) }
	}

	c := exec.Command(editorCommand(), tmpPath)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		defer os.Remove(tmpPath)
		if client == nil {
			return actionResultMsg("✗ client disconnected")
		}
		if err != nil {
			return actionResultMsg("✗ Editor error: " + err.Error())
		}

		newXml, _ := os.ReadFile(tmpPath)
		if string(newXml) == xml {
			return actionResultMsg("ℹ No changes")
		}

		if err := client.DefineXML(string(newXml)); err != nil {
			return actionResultMsg("✗ Save error: " + err.Error())
		}
		return actionResultMsg("✓ XML updated for " + name)
	})
}
