// virtui/cmd/tui/main.go
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

	model, err := p.Run()
	if app, ok := model.(*tui.App); ok {
		app.Close()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Ошибка запуска TUI: %v\n", err)
		os.Exit(1)
	}
}