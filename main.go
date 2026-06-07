package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/henryaj/autoclaude/internal/headless"
	"github.com/henryaj/autoclaude/internal/tmux"
	"github.com/henryaj/autoclaude/internal/tui"
)

var version = "dev"

func main() {
	testPattern := flag.String("test-pattern", "", "Test mode: trigger auto-continue when this string is found (for debugging)")
	headlessMode := flag.Bool("headless", false, "Run without TUI; poll tmux and log to stderr (for systemd-user service)")
	flag.Parse()

	if *headlessMode {
		if err := headless.Run(*testPattern); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if err := tmux.CheckTmuxEnv(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	p := tea.NewProgram(
		tui.New(version, *testPattern),
		tea.WithAltScreen(),
	)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		p.Quit()
	}()

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
