// virtui/internal/tui/app.go
package tui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"virtui/internal/libvirt"
)

type App struct {
	client     *libvirt.Client
	domains    []libvirt.DomainInfo
	logs       []string
	logFile    *os.File
	err        error
	ready      bool
	cursor     int
	confirming bool

	width  int
	height int
}

func NewApp() *App {
	return &App{
		logs: make([]string, 0, 50),
	}
}

func (a *App) Init() tea.Cmd {
	a.initLogFile()
	return tea.Batch(a.refresh(), a.autoRefresh(), tea.WindowSize())
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
		return a, nil

	case actionResultMsg:
		a.addLog(string(msg))
		return a, a.refresh()

	case errMsg:
		a.err = msg.err
		return a, nil

	case refreshMsg:
		return a, tea.Batch(a.refresh(), a.autoRefresh())

	case initMsg:
		a.domains = msg.domains
		a.ready = true
		if a.cursor >= len(a.domains) && len(a.domains) > 0 {
			a.cursor = len(a.domains) - 1
		}
		return a, nil

	case tea.KeyMsg:
		if msg.String() == "Q" || msg.String() == "ctrl+c" {
			if a.client != nil { a.client.Close() }
			return a, tea.Quit
		}

		if a.confirming {
			if msg.String() == "Y" {
				a.confirming = false
				return a, a.doAction("Destroy", a.client.Destroy)
			}
			a.confirming = false
			return a, nil
		}

		switch msg.String() {
		case "up", "k":
			if a.cursor > 0 { a.cursor-- }
		case "down", "j":
			if a.cursor < len(a.domains)-1 { a.cursor++ }
		case "S":
			return a, a.doAction("Start", a.client.Start)
		case "P":
			return a, a.doAction("Shutdown", a.client.Shutdown)
		case "R":
			return a, a.doAction("Reboot", a.client.Reboot)
		case "C":
			return a, a.connectToConsole()
		case "E":
    		return a, a.editDomainXML()
		case "D":
			if len(a.domains) > 0 { a.confirming = true }
		}
	}
	return a, nil
}

func (a *App) View() string {
	if a.err != nil {
		return errorStyle.Render(fmt.Sprintf("Ошибка: %v\nQ - Выход", a.err))
	}
	if !a.ready {
		return " Подключение к libvirt..."
	}

	totalWidth := a.width - 2
	sideWidth := (totalWidth / 2) - 1
	panelHeight := a.height - 25

	var listLines []string
	for i, d := range a.domains {
		stStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B"))
		if d.Status != "Running" {
			stStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555"))
		}
		
		statusPart := stStyle.Render(fmt.Sprintf("[%s]", d.Status))
		line := fmt.Sprintf("%-20s %s", d.Name, statusPart)
		
		if i == a.cursor {
			line = selectedStyle.Render("→ " + line)
		} else {
			line = "  " + line
		}
		listLines = append(listLines, line)
	}

	leftPanel := panelStyle.Width(sideWidth).Height(panelHeight).Render(
		"Domains:\n\n" + strings.Join(listLines, "\n"),
	)

	var infoStr string
	if len(a.domains) > 0 && a.cursor < len(a.domains) {
		d := a.domains[a.cursor]
		memStr := fmt.Sprintf("%d MiB", d.Memory/1024)

		disksStr := "None"
		if len(d.Disks) > 0 {
			var ds []string
			for _, disk := range d.Disks {
				ds = append(ds, "  - "+disk)
			}
			disksStr = strings.Join(ds, "\n")
		}

		ipsStr := "None"
		if len(d.IPs) > 0 {
			var is []string
			for _, ip := range d.IPs {
				is = append(is, "  - "+ip)
			}
			ipsStr = strings.Join(is, "\n")
		}

		infoStr = fmt.Sprintf("Name: %s\nStatus: %s\nUUID: %s\n\nCPUs: %d\nMem: %s\n\nDisks:\n%s\n\nNetwork:\n%s",
			d.Name, d.Status, d.UUID, d.VCPUs, memStr, disksStr, ipsStr)
	}

	rightPanel := panelStyle.Width(sideWidth).Height(panelHeight).Render("Info:\n\n" + infoStr)

	header := headerStyle.Width(totalWidth).Render(" VIRTUI — Libvirt Manager")
	mainArea := lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)

	logContent := wrapLogLines(a.logs, totalWidth-4)
		if len(a.logs) > 10 {
			logContent = strings.Join(a.logs[len(a.logs)-10:], "\n")
		}

	logsPanel := panelStyle.
		Width(totalWidth).
		Height(10).
		Render("Logs:\n\n" + logContent)

	footer := footerStyle.Width(totalWidth).Render(" jk: Nav | S: Start | P: Stop | R: Restart | E: Edit | C: Console | D: Destroy | Q: Quit")

	res := "\n" + header + "\n" + mainArea + "\n" + logsPanel + "\n" + footer
	if a.confirming {
		res += "\n" + errorStyle.Render(" !! DESTROY? (Y - да) !!")
	}
	return res
}