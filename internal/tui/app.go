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
	client      *libvirt.Client
	domains     []libvirt.DomainInfo
	filter      string
	filterMode  bool
	logs        []string
	logFile     *os.File
	err         error
	status      string
	ready       bool
	cursor      int
	confirming  bool

	// Для адаптивности
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
		a.width = msg.Width
		a.height = msg.Height
		return a, nil

	case tea.KeyMsg:
		key := msg.String()

		if key == "Q" {
			if a.logFile != nil { a.logFile.Close() }
			if a.client != nil { a.client.Close() }
			return a, tea.Quit
		}

		if a.confirming {
			if key == "Y" {
				fDoms := a.filteredDomains()
				if len(fDoms) > 0 && a.cursor < len(fDoms) {
					name := fDoms[a.cursor].Name
					if err := a.client.Destroy(name); err != nil {
						a.addLog(fmt.Sprintf("✗ Destroy %s: %v", name, err))
					} else {
						a.addLog("✓ Destroyed " + name)
					}
				}
			} else {
				a.addLog("Destroy отменён")
			}
			a.confirming = false
			return a, a.refresh()
		}

		if a.filterMode {
			switch key {
			case "esc":
				a.filter = ""
				a.filterMode = false
			case "enter":
				a.filterMode = false
			case "backspace":
				if len(a.filter) > 0 {
					a.filter = a.filter[:len(a.filter)-1]
				}
			default:
				if len(key) == 1 && key[0] >= ' ' && key[0] <= '~' {
					a.filter += key
				}
			}
			a.cursor = 0
			return a, nil
		}

		switch key {
		case "f":
			a.filterMode = true
			a.cursor = 0
			return a, nil

		case "up", "k":
			if a.cursor > 0 { a.cursor-- }
		case "down", "j":
			if a.cursor < len(a.filteredDomains())-1 { a.cursor++ }

		case "s": a.doAction("Start", a.client.Start)
		case "p": a.doAction("Shutdown", a.client.Shutdown)
		case "r": a.doAction("Reboot", a.client.Reboot)
		case "c":
			a.connectToConsole()
			return a, a.refresh()
		case "D":
			fDoms := a.filteredDomains()
			if len(fDoms) == 0 || a.cursor >= len(fDoms) {
				return a, nil
			}
			a.confirming = true
			return a, nil
		}

	case refreshMsg:
		return a, a.refresh()

	case initMsg:
		a.domains = msg.domains
		a.ready = true
		a.status = ""
		if a.cursor >= len(a.filteredDomains()) {
			a.cursor = 0
		}
	}

	return a, nil
}

func (a *App) View() string {
	if a.err != nil {
		return errorStyle.Render(fmt.Sprintf("Критическая ошибка: %v\n\nShift+Q — выход", a.err))
	}
	if !a.ready {
		return "Подключаемся к libvirt..."
	}

	// Адаптивная ширина
	panelWidth := (a.width - 4) / 2 // минус отступы

	header := headerStyle.Width(a.width).Render(
		titleStyle.Render("virtui — libvirt TUI"),
	)

	filterLine := ""
	if a.filterMode {
		filterLine = "Фильтр: " + a.filter + " (esc — сброс)"
	} else if a.filter != "" {
		filterLine = "Фильтр: " + a.filter + " (f — изменить)"
	} else {
		filterLine = "Нажми f для фильтра"
	}

	fDoms := a.filteredDomains()
	var listItems []string
	for i, d := range fDoms {
		line := fmt.Sprintf("%-20s [%s]", d.Name, d.Status)
		if i == a.cursor {
			line = selectedStyle.Render("→ " + line)
		} else {
			line = "  " + line
		}
		listItems = append(listItems, line)
	}
	listContent := strings.Join(listItems, "\n")
	if len(listItems) == 0 {
		listContent = "Домены не найдены"
	}

	domainsPanel := panelStyle.Width(panelWidth).Height(12).Render("ДОМЕНЫ\n" + filterLine + "\n" + listContent)

	info := "Ничего не выбрано"
	if len(fDoms) > 0 && a.cursor < len(fDoms) {
		d := fDoms[a.cursor]
		cpuSec := float64(d.CPU) / 1_000_000_000
		info = fmt.Sprintf("Имя: %s\nUUID: %s\nСтатус: %s\nOS: %s\n\nVCPU: %d\nПамять: %d / %d MB\nCPU time: %.2f сек",
			d.Name, d.UUID, d.Status, d.OS, d.VCPUs,
			d.Memory/1024, d.MaxMemory/1024, cpuSec)
	}
	infoPanel := panelStyle.Width(panelWidth).Height(12).Render("ИНФОРМАЦИЯ О ДОМЕНЕ\n" + info)

	// Логи — фиксированные 10 строк
	logContent := wrapLogLines(a.logs, panelWidth*2-4)
	if len(a.logs) > 10 {
		logContent = strings.Join(a.logs[len(a.logs)-10:], "\n")
	}
	logsPanel := panelStyle.Width(a.width-4).Height(10).Render("ЛОГИ (последние 10)\n" + logContent)

	mainArea := lipgloss.JoinHorizontal(lipgloss.Top, domainsPanel, infoPanel)

	view := header + "\n\n" + mainArea + "\n\n" + logsPanel

	if a.status != "" {
		view += "\n" + a.status
	}
	if a.confirming {
		view += "\n\n" + errorStyle.Render("Уверены? DESTROY (Shift+Y — да, любая клавиша — отмена)")
	}

	footer := footerStyle.Width(a.width).Render("↑↓/jk выбор | s Start | p Shutdown | r Reboot | c Console | Shift+D Destroy | f фильтр | Shift+Q выход")
	view += "\n\n" + footer

	return view
}