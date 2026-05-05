// virtui/internal/tui/app.go
package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"virtui/internal/config"
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
	cloneMode  bool
	cloneName  string
	deleteMode bool
	perfData   map[string]*PerfBuffer
	config     *config.Config

	width  int
	height int
}

func NewApp() *App {
	cfg := config.Load()
	return &App{
		logs:     make([]string, 0, cfg.MaxLogLines),
		perfData: make(map[string]*PerfBuffer),
		config:   cfg,
	}
}

func (a *App) Init() tea.Cmd {
	a.initLogFile()
	return tea.Batch(a.connect(), a.autoRefresh(), tea.WindowSize())
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case connectMsg:
		if msg.err != nil {
			a.err = msg.err
			return a, nil
		}
		a.client = msg.client
		a.client.IPv4Only = a.config.IPv4Only
		return a, a.refresh()

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
		now := time.Now()
		for _, d := range msg.domains {
			pb, ok := a.perfData[d.Name]
			if !ok {
				pb = NewPerfBuffer()
				a.perfData[d.Name] = pb
			}
			pb.AddSample(d.CPU, d.Memory, now, d.VCPUs)
		}
		for name := range a.perfData {
			found := false
			for _, d := range msg.domains {
				if d.Name == name {
					found = true
					break
				}
			}
			if !found {
				delete(a.perfData, name)
			}
		}
		return a, nil

	case tea.KeyMsg:
		if msg.String() == "Q" || msg.String() == "ctrl+c" {
			a.Close()
			return a, tea.Quit
		}

		if a.cloneMode {
			switch msg.String() {
			case "enter":
				if a.cloneName != "" {
					return a, a.doClone(a.cloneName)
				}
			case "esc":
				a.cloneMode = false
				a.cloneName = ""
			case "backspace", "ctrl+h":
				if len(a.cloneName) > 0 {
					a.cloneName = a.cloneName[:len(a.cloneName)-1]
				}
			default:
				if len(msg.Runes) == 1 {
					r := msg.Runes[0]
					if r >= 32 && r <= 126 {
						a.cloneName += string(r)
					}
				}
			}
			return a, nil
		}

		if a.deleteMode {
			switch msg.String() {
			case "enter":
				return a, a.doDelete()
			case "esc":
				a.deleteMode = false
			case "y", "Y":
				return a, a.doDelete()
			case "n", "N":
				a.deleteMode = false
			}
			return a, nil
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
			if len(a.domains) > 0 && a.cursor < len(a.domains) {
				if a.domains[a.cursor].Status == "Shutoff" {
					a.deleteMode = true
				} else {
					a.confirming = true
				}
			}
		case "K":
			if len(a.domains) > 0 && a.cursor < len(a.domains) && a.domains[a.cursor].Status == "Shutoff" {
				a.cloneMode = true
				a.cloneName = a.domains[a.cursor].Name + "-clone"
			}
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
	wideMode := totalWidth >= 100

	logLines := LogPanelLines
	if a.height < 35 {
		logLines = 5
	}
	if a.height < 25 {
		logLines = 3
	}
	panelHeight := a.height - 15 - logLines
	if panelHeight < 5 {
		panelHeight = 5
	}
	compact := panelHeight <= 22

	var listLines []string
	runningStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B"))
	shutoffStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555"))
	pausedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFB86C"))
	otherStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6272A4"))

	for i, d := range a.domains {
		st := otherStyle
		switch d.Status {
		case "Running":
			st = runningStyle
		case "Shutoff":
			st = shutoffStyle
		case "Paused":
			st = pausedStyle
		}
		statusPart := st.Render(fmt.Sprintf("[%s]", d.Status))
		line := fmt.Sprintf("%-20s %s", d.Name, statusPart)

		if i == a.cursor {
			line = selectedStyle.Render("→ " + line)
		} else {
			line = "  " + line
		}
		listLines = append(listLines, line)
	}

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

		if pb, ok := a.perfData[d.Name]; ok {
			cpus := pb.CPUs()
			mems := pb.Memories()
			if len(cpus) > 0 {
				spark := renderSparkline(cpus, 12)
				infoStr += fmt.Sprintf("\n\nCPU: %s  %3.0f%%", spark, cpus[len(cpus)-1])
			}
			if len(mems) > 0 {
				spark := renderSparkline(mems, 12)
				memCurr := float64(d.Memory) / 1024
				memMax := float64(d.MaxMemory) / 1024
				infoStr += fmt.Sprintf("\nMem: %s  %.0f / %.0f MiB", spark, memCurr, memMax)
			}
		}
	}

	rightContent := infoStr
	if a.cloneMode {
		srcName := ""
		if a.cursor < len(a.domains) {
			srcName = a.domains[a.cursor].Name
		}
		rightContent = fmt.Sprintf("Clone: %s\n\nNew name:\n\n  %s█\n\nEnter — clone | Esc — cancel", srcName, a.cloneName)
	} else if a.deleteMode {
		srcName := ""
		if a.cursor < len(a.domains) {
			srcName = a.domains[a.cursor].Name
		}
		rightContent = fmt.Sprintf("Delete VM: %s\n\nThis will remove all disks.\n\nY — delete | N/Esc — cancel", srcName)
	}

	if compact && len(a.domains) > 0 {
		sideWidth := (totalWidth / 2) - 1
		leftPanel := panelStyle.Width(sideWidth).Height(a.height).Render(
			"Domains:\n\n" + strings.Join(listLines, "\n"),
		)
		rightPanel := panelStyle.Width(sideWidth).Height(a.height).Render("Info:\n\n" + rightContent)
		return lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)
	}

	var mainArea string
	if wideMode {
		sideWidth := (totalWidth / 2) - 1
		leftPanel := panelStyle.Width(sideWidth).Height(panelHeight).Render(
			"Domains:\n\n" + strings.Join(listLines, "\n"),
		)
		rightPanel := panelStyle.Width(sideWidth).Height(panelHeight).Render("Info:\n\n" + rightContent)
		mainArea = lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)
	} else {
		listHeight := panelHeight * 2 / 3
		infoHeight := panelHeight - listHeight
		leftPanel := panelStyle.Width(totalWidth).Height(listHeight).Render(
			"Domains:\n\n" + strings.Join(listLines, "\n"),
		)
		rightPanel := panelStyle.Width(totalWidth).Height(infoHeight).Render("Info:\n\n" + rightContent)
		mainArea = lipgloss.JoinVertical(lipgloss.Top, leftPanel, rightPanel)
	}

	header := headerStyle.Width(totalWidth).Render(" VIRTUI — Libvirt Connector")

	displayLogs := a.logs
	if len(displayLogs) > logLines {
		displayLogs = displayLogs[len(displayLogs)-logLines:]
	}
	logContent := wrapLogLines(displayLogs, totalWidth-PanelPadding*2)

	logsPanel := panelStyle.
		Width(totalWidth).
		Height(logLines).
		Render("Logs:\n\n" + logContent)

	footer := footerStyle.Width(totalWidth).Render(" jk: Nav | S: Start | P: Stop | R: Restart | E: Edit | C: Console | K: Clone | D: Destroy | Q: Quit")

	res := "\n" + header + "\n" + mainArea + "\n" + logsPanel + "\n" + footer
	if a.confirming {
		res += "\n" + errorStyle.Render(" !! DESTROY? (Y - да) !!")
	}
	return res
}

func (a *App) Close() {
	if a.client != nil {
		a.client.Close()
		a.client = nil
	}
	if a.logFile != nil {
		a.logFile.Close()
		a.logFile = nil
	}
}