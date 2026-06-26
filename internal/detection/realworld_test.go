package detection

import "testing"

// These fixtures are the ACTUAL strings Claude Code paints when rate-limited,
// taken verbatim from the repo's own example/example2 captures. They do NOT
// contain the synthetic "Context … │ Usage ⚠ Limit reached" framing that the
// hand-written testdata fixtures use, so they guard against detection being
// tightened to a format real Claude Code never emits.

// example2: the status-bar state after the limit is hit. The ⚠ line stands on
// its own — no "Context"/"Usage" on that line.
const realStatusBar = "  [Opus 4.5 | Max] ████████░░ 82% | ⏱️  2h 41m\n" +
	"  gender-biased-advice-paper git:(master*)\n" +
	"  ⚠ Limit reached (resets 8m)\n" +
	"  ✓ Bash ×10 | ✓ Read ×4 | ✓ Edit ×4 | ✓ Write ×1\n" +
	"  ⏵⏵ bypass permissions on · cd /Users/x/workspace/gender-bias… (running)\n"

// example: the blocking picker. Three-option form, no "Enter to confirm · Esc
// to cancel" footer line.
const realPicker = "  ⎿  You've hit your limit · resets 8pm (Europe/London)\n" +
	"     Opening your options…\n\n" +
	"> /rate-limit-options\n" +
	"╭──────────────────────────────────────────────╮\n" +
	"│ What do you want to do?                        │\n" +
	"│                                                │\n" +
	"│ ❯ 1. Stop and wait for limit to reset          │\n" +
	"│   2. Switch to extra usage                     │\n" +
	"│   3. Upgrade your plan                         │\n" +
	"╰──────────────────────────────────────────────╯\n"

func TestCheckRateLimit_RealStatusBar(t *testing.T) {
	status := CheckRateLimit(realStatusBar)
	if !status.IsLimited {
		t.Fatal("real status-bar '⚠ Limit reached (resets 8m)' must be detected as rate-limited")
	}
	if status.ResetsAt != "8m" {
		t.Errorf("ResetsAt = %q, want \"8m\"", status.ResetsAt)
	}
	if status.ResetTime.IsZero() {
		t.Error("expected ResetTime set from '(resets 8m)'")
	}
}

func TestCheckRateLimit_RealPicker(t *testing.T) {
	status := CheckRateLimit(realPicker)
	if !status.IsLimited {
		t.Fatal("real blocking picker (no footer line) must be detected as rate-limited")
	}
	if !status.MenuShown {
		t.Error("expected MenuShown=true for the real picker")
	}
	if status.ResetsAt != "8pm" {
		t.Errorf("ResetsAt = %q, want \"8pm\" (from the · resets 8pm line)", status.ResetsAt)
	}
}

// TestCheckRateLimit_BlankPaddedCapture reproduces the exact shape of a
// `tmux capture-pane -p` dump: the status bar followed by blank padding rows up
// to the pane height. captureTail must trim the padding so the ⚠ line stays in
// the liveness window.
func TestCheckRateLimit_BlankPaddedCapture(t *testing.T) {
	padded := realStatusBar
	for i := 0; i < 23; i++ {
		padded += "\n"
	}
	status := CheckRateLimit(padded)
	if !status.IsLimited {
		t.Fatal("status bar followed by blank padding must still be detected as rate-limited")
	}
	if status.ResetsAt != "8m" {
		t.Errorf("ResetsAt = %q, want \"8m\"", status.ResetsAt)
	}
}

// realSessionLimit is the inline message shown when the *subscription session
// limit* (not the API rate limit) is hit. It appears as a tool-result bullet
// in the chat body, not in the status bar, and uses "session limit" rather
// than "limit reached".
const realSessionLimit = "  ⎿  You've hit your session limit · resets 6:20pm (Europe/London)\n" +
	"     /upgrade to increase your usage limit.\n\n" +
	"✻ Churned for 1m 5s\n\n" +
	"────────────────────────────────────────────────────────────────────────────────\n" +
	"❯ Got the notification, add the cron line now\n" +
	"────────────────────────────────────────────────────────────────────────────────\n" +
	"  ⏵⏵ auto mode on (shift+tab to cycle) · ← for agents\n" +
	"                                         ~93k uncached · /clear to start fresh\n" +
	"                                                                           /rc\n"

func TestCheckRateLimit_SessionLimit(t *testing.T) {
	status := CheckRateLimit(realSessionLimit)
	if !status.IsLimited {
		t.Fatal("'hit your session limit' inline message must be detected as rate-limited")
	}
	if status.ResetsAt != "6:20pm" {
		t.Errorf("ResetsAt = %q, want \"6:20pm\"", status.ResetsAt)
	}
	if status.ResetTime.IsZero() {
		t.Error("expected ResetTime parsed from TZ-aware '6:20pm (Europe/London)'")
	}
}

func TestIsClaudeCode_RealCaptures(t *testing.T) {
	if !IsClaudeCode(realStatusBar) {
		t.Error("real status bar should be recognised as Claude Code")
	}
	if !IsClaudeCode(realPicker) {
		t.Error("real picker should be recognised as Claude Code")
	}
	if !IsClaudeCode(realSessionLimit) {
		t.Error("real session-limit inline message should be recognised as Claude Code")
	}
}
