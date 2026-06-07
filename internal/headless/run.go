package headless

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/henryaj/autoclaude/internal/detection"
	"github.com/henryaj/autoclaude/internal/tmux"
)

const pollInterval = 3 * time.Second

// Run starts the headless polling loop. Blocks until SIGINT/SIGTERM.
func Run(testPattern string) error {
	if err := verifyTmuxServer(); err != nil {
		return err
	}

	logger := log.New(os.Stderr, "", log.LstdFlags|log.Lmicroseconds)
	logger.Println("INFO autoclaude headless start")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		s := <-sigCh
		logger.Printf("INFO signal=%s shutting down", s)
		cancel()
	}()

	state := newState(logger)
	tick := time.NewTicker(pollInterval)
	defer tick.Stop()

	state.poll(testPattern)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
			state.poll(testPattern)
		}
	}
}

func verifyTmuxServer() error {
	cmd := exec.Command("tmux", "list-sessions")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tmux server unreachable (TMUX_TMPDIR=%s): %w",
			os.Getenv("TMUX_TMPDIR"), err)
	}
	return nil
}

type state struct {
	logger *log.Logger
	panes  map[string]*tmux.Pane
}

func newState(logger *log.Logger) *state {
	return &state{logger: logger, panes: map[string]*tmux.Pane{}}
}

func (s *state) poll(testPattern string) {
	layout, err := tmux.ListPanes("")
	if err != nil {
		s.logger.Printf("ERROR list-panes: %v", err)
		return
	}

	seen := map[string]bool{}
	for _, p := range layout.Panes {
		seen[p.ID] = true
		if prev, ok := s.panes[p.ID]; ok {
			p.Mode = prev.Mode
			p.HasClaudeCode = prev.HasClaudeCode
			p.IsRateLimited = prev.IsRateLimited
			p.RateLimitResets = prev.RateLimitResets
			p.RateLimitTime = prev.RateLimitTime
			p.ContinueSent = prev.ContinueSent
			p.LastPeriodicContinue = prev.LastPeriodicContinue
			p.MenuHandled = prev.MenuHandled
		}
		s.panes[p.ID] = p
		s.processPane(p, testPattern)
	}
	for id := range s.panes {
		if !seen[id] {
			delete(s.panes, id)
		}
	}
}

func (s *state) processPane(p *tmux.Pane, testPattern string) {
	content, err := tmux.CapturePane(p.ID)
	if err != nil {
		return
	}

	p.HasClaudeCode = detection.IsClaudeCode(content)
	if !p.HasClaudeCode {
		if p.IsRateLimited {
			s.logger.Printf("INFO pane=%s claude=gone clearing-state", p.Location())
		}
		p.IsRateLimited = false
		p.RateLimitResets = ""
		p.RateLimitTime = time.Time{}
		p.ContinueSent = false
		p.LastPeriodicContinue = time.Time{}
		p.MenuHandled = false
		return
	}

	status := detection.CheckRateLimit(content)
	wasLimited := p.IsRateLimited
	p.IsRateLimited = status.IsLimited
	p.RateLimitResets = status.ResetsAt
	p.RateLimitTime = status.ResetTime

	if !wasLimited && status.IsLimited {
		p.ContinueSent = false
		p.LastPeriodicContinue = time.Time{}
		p.MenuHandled = false
		s.logger.Printf("INFO pane=%s rate-limited menu=%v resets=%q", p.Location(), status.MenuShown, status.ResetsAt)
	}

	if p.IsRateLimited && p.Mode == tmux.ModeContinueOnRateLimit {
		if status.MenuShown && !p.MenuHandled {
			s.dismissMenu(p, content)
			p.MenuHandled = true
			return
		}

		now := time.Now()
		if !p.RateLimitTime.IsZero() && !p.ContinueSent && now.After(p.RateLimitTime) {
			s.sendContinue(p, "reset-elapsed")
			p.ContinueSent = true
		}
	}

	if testPattern != "" && strings.Contains(content, testPattern) &&
		p.Mode == tmux.ModeContinueOnRateLimit && !p.ContinueSent {
		s.sendContinue(p, "test-pattern")
		p.ContinueSent = true
	}
}

func (s *state) dismissMenu(p *tmux.Pane, content string) {
	move, ok := detection.FindStopAndWaitMove(content)
	if !ok {
		s.logger.Printf("WARN pane=%s menu detected but 'Stop and wait' row not parseable; skipping", p.Location())
		return
	}
	key := "Down"
	if move < 0 {
		key = "Up"
		move = -move
	}
	for i := 0; i < move; i++ {
		if err := tmux.SendKeys(p.ID, key); err != nil {
			s.logger.Printf("ERROR pane=%s send-keys %s: %v", p.Location(), key, err)
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err := tmux.SendKeys(p.ID, "Enter"); err != nil {
		s.logger.Printf("ERROR pane=%s send-keys Enter: %v", p.Location(), err)
		return
	}
	s.logger.Printf("INFO pane=%s action=menu-pick stop+wait (moved=%d%s)", p.Location(), move, key[:1])
}

func (s *state) sendContinue(p *tmux.Pane, reason string) {
	if err := tmux.SendKeys(p.ID, "continue"); err != nil {
		s.logger.Printf("ERROR pane=%s send-keys continue: %v", p.Location(), err)
		return
	}
	if err := tmux.SendKeys(p.ID, "Enter"); err != nil {
		s.logger.Printf("ERROR pane=%s send-keys Enter: %v", p.Location(), err)
		return
	}
	s.logger.Printf("INFO pane=%s action=continue reason=%s", p.Location(), reason)
}
