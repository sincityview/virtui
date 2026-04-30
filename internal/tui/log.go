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

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	a.logFile = f

	a.loadExistingLogs(path)
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

	if len(allLines) > 50 {
		allLines = allLines[len(allLines)-50:]
	}
	a.logs = allLines
}

func (a *App) addLog(msg string) {
	timestamp := time.Now().Format("15:04:05")
	line := timestamp + " | " + msg
	a.logs = append(a.logs, line)

	if len(a.logs) > 50 {
		a.logs = a.logs[len(a.logs)-50:]
	}

	if a.logFile != nil {
		_, _ = a.logFile.WriteString(line + "\n")
	}
}

// wrapLogLines — перенос длинных строк
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