package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbletea"
	"virtui/internal/libvirt"
)

type refreshMsg struct{}
type initMsg struct{ domains []libvirt.DomainInfo }
type errMsg struct{ err error }
type statusMsg string

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
		return initMsg{domains: domains}
	}
}

func (a *App) autoRefresh() tea.Cmd {
	return tea.Every(5*time.Second, func(time.Time) tea.Msg {
		return refreshMsg{}
	})
}

func (a *App) doAction(action string, fn func(string) error) {
	fDoms := a.filteredDomains()
	if len(fDoms) == 0 || a.cursor >= len(fDoms) {
		return
	}
	name := fDoms[a.cursor].Name

	if err := fn(name); err != nil {
		a.addLog(fmt.Sprintf("✗ %s %s: %v", action, name, err))
		return
	}
	a.addLog(fmt.Sprintf("✓ %s %s", action, name))
}

func (a *App) filteredDomains() []libvirt.DomainInfo {
	if a.filter == "" {
		return a.domains
	}
	var filtered []libvirt.DomainInfo
	for _, d := range a.domains {
		if strings.Contains(strings.ToLower(d.Name), strings.ToLower(a.filter)) {
			filtered = append(filtered, d)
		}
	}
	return filtered
}

func (a *App) connectToConsole() {
	fDoms := a.filteredDomains()
	if len(fDoms) == 0 || a.cursor >= len(fDoms) {
		a.addLog("Нет выбранного домена для консоли")
		return
	}

	name := fDoms[a.cursor].Name
	a.addLog(fmt.Sprintf("Запуск консоли для %s...", name))

	cmd := exec.Command("virsh", "console", name)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		a.addLog(fmt.Sprintf("Ошибка консоли: %v", err))
	} else {
		a.addLog("Выход из консоли")
	}
}