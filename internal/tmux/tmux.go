package tmux

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

var (
	ErrNotInTmux = errors.New("not running inside tmux (TMUX environment variable not set)")
	ErrTimeout   = errors.New("tmux command timed out")
)

const commandTimeout = 5 * time.Second

// CheckTmuxEnv validates that we're running inside tmux
func CheckTmuxEnv() error {
	if os.Getenv("TMUX") == "" {
		return ErrNotInTmux
	}
	return nil
}

// CurrentPaneID returns the ID of the pane where this process is running
func CurrentPaneID() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "tmux", "display-message", "-p", "#{pane_id}")
	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", ErrTimeout
		}
		return "", fmt.Errorf("tmux display-message: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

// CurrentWindowID returns the ID of the window where this process is running
func CurrentWindowID() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "tmux", "display-message", "-p", "#{window_id}")
	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", ErrTimeout
		}
		return "", fmt.Errorf("tmux display-message: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

// ListPanes returns the layout of every pane across all tmux sessions/windows.
// The windowID arg is accepted for backwards compatibility but ignored — we always
// enumerate the entire server so autoclaude can watch panes wherever Claude Code runs.
func ListPanes(windowID string) (*Layout, error) {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	const sep = "\x1f"
	format := strings.Join([]string{
		"#{pane_id}",
		"#{session_name}",
		"#{window_index}",
		"#{window_name}",
		"#{pane_index}",
		"#{pane_left}",
		"#{pane_top}",
		"#{pane_width}",
		"#{pane_height}",
		"#{pane_title}",
	}, sep)

	cmd := exec.CommandContext(ctx, "tmux", "list-panes", "-a", "-F", format)
	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, ErrTimeout
		}
		return nil, fmt.Errorf("tmux list-panes: %w", err)
	}

	return parseListPanes(string(output), sep)
}

// parseListPanes parses the output of tmux list-panes
func parseListPanes(output, sep string) (*Layout, error) {
	layout := &Layout{
		Panes: make([]*Pane, 0),
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		pane, err := parsePaneLine(line, sep)
		if err != nil {
			return nil, fmt.Errorf("parse pane line %q: %w", line, err)
		}
		layout.Panes = append(layout.Panes, pane)
	}

	return layout, nil
}

// parsePaneLine parses a single line of tmux list-panes output
func parsePaneLine(line, sep string) (*Pane, error) {
	fields := strings.Split(line, sep)
	if len(fields) < 9 {
		return nil, fmt.Errorf("expected at least 9 fields, got %d", len(fields))
	}

	left, err := strconv.Atoi(fields[5])
	if err != nil {
		return nil, fmt.Errorf("parse left: %w", err)
	}
	top, err := strconv.Atoi(fields[6])
	if err != nil {
		return nil, fmt.Errorf("parse top: %w", err)
	}
	width, err := strconv.Atoi(fields[7])
	if err != nil {
		return nil, fmt.Errorf("parse width: %w", err)
	}
	height, err := strconv.Atoi(fields[8])
	if err != nil {
		return nil, fmt.Errorf("parse height: %w", err)
	}

	title := ""
	if len(fields) >= 10 {
		title = fields[9]
	}

	return &Pane{
		ID:          fields[0],
		Session:     fields[1],
		WindowIndex: fields[2],
		WindowName:  fields[3],
		PaneIndex:   fields[4],
		Left:        left,
		Top:         top,
		Width:       width,
		Height:      height,
		Title:       title,
		Mode:        ModeContinueOnRateLimit,
	}, nil
}

// CapturePane captures the content of a pane
func CapturePane(paneID string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "tmux", "capture-pane", "-t", paneID, "-p")
	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", ErrTimeout
		}
		return "", fmt.Errorf("tmux capture-pane: %w", err)
	}

	return string(output), nil
}

// SendKeys sends keystrokes to a pane
func SendKeys(paneID string, keys ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	args := []string{"send-keys", "-t", paneID}
	args = append(args, keys...)

	cmd := exec.CommandContext(ctx, "tmux", args...)
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return ErrTimeout
		}
		return fmt.Errorf("tmux send-keys: %w", err)
	}

	return nil
}
