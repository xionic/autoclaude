package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/henryaj/autoclaude/internal/tmux"
	"github.com/henryaj/autoclaude/internal/tui"
)

var version = "dev"

func main() {
	testPattern := flag.String("test-pattern", "", "Test mode: trigger auto-continue when this string is found (for debugging)")
	flag.Parse()

	// Validate tmux environment
	if err := tmux.CheckTmuxEnv(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	p := tea.NewProgram(
		tui.New(version, *testPattern),
		tea.WithAltScreen(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
