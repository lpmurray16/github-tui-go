package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lpmurray16/github-tui-go/internal/ui"
)

func main() {
	program := tea.NewProgram(ui.New(), tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "github-tui-go: %v\n", err)
		os.Exit(1)
	}
}
