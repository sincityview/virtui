// virtui/internal/tui/log.go
package tui

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (a *App) initLogFile() {
	dir := filepath.Join(os.Getenv("HOME"), ".local", "virtui")
	_ = os.MkdirAll(dir, 0755)

	path := filepath.Join(dir, "virtui.log")

	a.loadExistingLogs(path)

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		a.err = err
		return
	}
	for _, line := range a.logs {
		_, _ = f.WriteString(line + "\n")
	}
	a.logFile = f

	if len(a.logs) == 0 {
		a.logs = append(a.logs, "")
	}
}

func (a *App) loadExistingLogs(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var allLines []string
	for scanner.Scan() {
		allLines = append(allLines, scanner.Text())
	}

	if len(allLines) > a.config.MaxLogLines {
		allLines = allLines[len(allLines)-a.config.MaxLogLines:]
	}
	a.logs = allLines
}

func (a *App) addLog(msg string) {
	timestamp := time.Now().Format("15:04:05")
	line := timestamp + " | " + msg
	a.logs = append(a.logs, line)

	if len(a.logs) > a.config.MaxLogLines {
		a.logs = a.logs[len(a.logs)-a.config.MaxLogLines:]
	}

	if a.logFile != nil {
		_, _ = a.logFile.WriteString(line + "\n")
	}
}

func wrapLogLines(lines []string, maxWidth int) string {
	var wrapped []string
	for _, line := range lines {
		if len(line) <= maxWidth {
			wrapped = append(wrapped, line)
			continue
		}
		for len(line) > maxWidth {
			wrapped = append(wrapped, line[:maxWidth])
			line = line[maxWidth:]
		}
		if len(line) > 0 {
			wrapped = append(wrapped, line)
		}
	}
	return strings.Join(wrapped, "\n")
}