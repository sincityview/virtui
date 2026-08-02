// virtui/cmd/tui/main.go
package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"virtui/internal/config"
	"virtui/internal/tui"
)

var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	uri := flag.String("uri", "", "libvirt connection URI (default: qemu:///system)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "virtui — TUI for libvirt\n\nUsage:\n  virtui [flags]\n\nFlags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *showVersion {
		fmt.Printf("virtui %s\n", version)
		os.Exit(0)
	}

	cfg := config.Load()
	if *uri != "" {
		cfg.URI = *uri
	}

	p := tea.NewProgram(tui.NewApp(cfg),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	model, err := p.Run()
	if app, ok := model.(*tui.App); ok {
		app.Close()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
		os.Exit(1)
	}
}
