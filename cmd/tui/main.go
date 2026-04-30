package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"virtui/internal/tui"
)

func main() {
	p := tea.NewProgram(tui.NewApp(),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Ошибка запуска TUI: %v\n", err)
		os.Exit(1)
	}
}