// virtui/cmd/tui/main.go
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"virtui/internal/config"
	"virtui/internal/tui"
)

func main() {
	cfg := config.Load()

	p := tea.NewProgram(tui.NewApp(cfg),
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