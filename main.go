package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/henryaj/autoclaude/internal/tmux"
	"github.com/henryaj/autoclaude/internal/tui"
)

var version = "dev"

func main() {
	// Validate tmux environment
	if err := tmux.CheckTmuxEnv(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	p := tea.NewProgram(
		tui.New(version),
		tea.WithAltScreen(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
