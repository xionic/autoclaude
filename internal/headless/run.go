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

const (
	pollInterval = 3 * time.Second

	// probeMinIdle is how long a pane must have been continuously rate-limited
	// before probing begins. Stops us from interrupting a freshly-limited pane
	// where the user might still be composing a follow-up.
	probeMinIdle = 5 * time.Minute
)

// probeBackoff returns how long to wait before the next probe after `n` failed
// probes. Schedule: 5m, 10m, 15m, 30m, then 30m forever.
func probeBackoff(n int) time.Duration {
	schedule := []time.Duration{
		5 * time.Minute,
		10 * time.Minute,
		15 * time.Minute,
		30 * time.Minute,
	}
	if n >= len(schedule) {
		return schedule[len(schedule)-1]
	}
	return schedule[n]
}

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
			p.LimitedSince = prev.LimitedSince
			p.ProbeAt = prev.ProbeAt
			p.ProbeCount = prev.ProbeCount
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
		s.clearLimitState(p)
		return
	}

	status := detection.CheckRateLimit(content)
	wasLimited := p.IsRateLimited
	p.IsRateLimited = status.IsLimited
	p.RateLimitResets = status.ResetsAt
	p.RateLimitTime = status.ResetTime

	now := time.Now()

	if !wasLimited && status.IsLimited {
		p.ContinueSent = false
		p.MenuHandled = false
		p.LimitedSince = now
		p.ProbeAt = now.Add(probeBackoff(0))
		p.ProbeCount = 0
		mode := "wait-for-captured-reset"
		if status.ResetTime.IsZero() {
			mode = fmt.Sprintf("probe-from=+%s", probeBackoff(0))
		}
		s.logger.Printf("INFO pane=%s rate-limited menu=%v resets=%q %s",
			p.Location(), status.MenuShown, status.ResetsAt, mode)
	}

	if wasLimited && !status.IsLimited {
		s.logger.Printf("INFO pane=%s rate-limit cleared (duration=%s probes=%d)",
			p.Location(), now.Sub(p.LimitedSince).Round(time.Second), p.ProbeCount)
		s.clearLimitState(p)
	}

	if p.IsRateLimited && p.Mode == tmux.ModeContinueOnRateLimit {
		if status.MenuShown {
			if p.MenuHandled {
				// Menu re-rendered after we dismissed → previous probe (or
				// claude's own retry) was rejected. Count it as a failed
				// probe and reset menu state so we dismiss again below.
				p.ProbeCount++
				p.ProbeAt = now.Add(probeBackoff(p.ProbeCount))
				s.logger.Printf("INFO pane=%s probe-failed (count=%d next-probe=+%s)",
					p.Location(), p.ProbeCount, probeBackoff(p.ProbeCount))
				p.MenuHandled = false
			}
			s.dismissMenu(p, content)
			p.MenuHandled = true
			return
		}

		if !p.RateLimitTime.IsZero() {
			// Captured reset time exists — trust it and wait. Probing here
			// is wasted dispatch (claude already knows when the limit lifts).
			if !p.ContinueSent && now.After(p.RateLimitTime) {
				s.sendContinue(p, "reset-elapsed")
				p.ContinueSent = true
			}
			return
		}

		// No captured reset time — probe with backoff to discover when the
		// limit lifts. Gated on probeMinIdle so freshly-limited panes don't
		// get interrupted while the user might still be composing.
		if !p.ContinueSent && now.After(p.ProbeAt) && now.Sub(p.LimitedSince) >= probeMinIdle {
			s.sendContinue(p, fmt.Sprintf("probe-%d", p.ProbeCount))
			// Don't bump ProbeCount yet; bump only if next tick shows menu
			// re-render (failure). On success, IsRateLimited goes false and
			// state clears via the wasLimited→!IsLimited branch above.
			p.ProbeAt = now.Add(probeBackoff(p.ProbeCount + 1))
		}
	}

	if testPattern != "" && strings.Contains(content, testPattern) &&
		p.Mode == tmux.ModeContinueOnRateLimit && !p.ContinueSent {
		s.sendContinue(p, "test-pattern")
		p.ContinueSent = true
	}
}

func (s *state) clearLimitState(p *tmux.Pane) {
	p.IsRateLimited = false
	p.RateLimitResets = ""
	p.RateLimitTime = time.Time{}
	p.ContinueSent = false
	p.MenuHandled = false
	p.LimitedSince = time.Time{}
	p.ProbeAt = time.Time{}
	p.ProbeCount = 0
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
