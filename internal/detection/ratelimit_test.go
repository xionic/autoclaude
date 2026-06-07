package detection

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func loadFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("failed to load fixture %s: %v", name, err)
	}
	return string(data)
}

func TestCheckRateLimit_NewFormat(t *testing.T) {
	content := loadFixture(t, "rate_limit_new_format.txt")
	status := CheckRateLimit(content)

	if !status.IsLimited {
		t.Error("expected IsLimited to be true")
	}
	if status.ResetsAt != "10pm" {
		t.Errorf("expected ResetsAt to be '10pm', got '%s'", status.ResetsAt)
	}
	if status.ResetTime.IsZero() {
		t.Error("expected ResetTime to be set")
	}
}

func TestCheckRateLimit_OldFormat(t *testing.T) {
	content := loadFixture(t, "rate_limit_old_format.txt")
	status := CheckRateLimit(content)

	if !status.IsLimited {
		t.Error("expected IsLimited to be true")
	}
	if status.ResetsAt != "2pm" {
		t.Errorf("expected ResetsAt to be '2pm', got '%s'", status.ResetsAt)
	}
	if status.ResetTime.IsZero() {
		t.Error("expected ResetTime to be set")
	}
}

func TestCheckRateLimit_NoMatch(t *testing.T) {
	content := loadFixture(t, "not_claude_code.txt")
	status := CheckRateLimit(content)

	if status.IsLimited {
		t.Error("expected IsLimited to be false")
	}
}

func TestCheckRateLimit_TimeFormats(t *testing.T) {
	// Every case must include the live ⚠ indicator — otherwise detection
	// (correctly) treats it as chat-history quotation and skips.
	live := "\nContext 25% │ Usage ⚠ Limit reached"
	cases := []struct {
		name     string
		content  string
		wantTime string
	}{
		{
			name:     "simple pm",
			content:  "You've hit your limit · resets 2pm" + live,
			wantTime: "2pm",
		},
		{
			name:     "simple am",
			content:  "You've hit your limit · resets 9am" + live,
			wantTime: "9am",
		},
		{
			name:     "with minutes",
			content:  "limit reached ∙ resets 10:30am" + live,
			wantTime: "10:30am",
		},
		{
			name:     "with space before am/pm",
			content:  "limit reached ∙ resets 3 pm" + live,
			wantTime: "3 pm",
		},
		{
			name:     "double digit hour TZ",
			content:  "You've hit your limit · resets 11pm (Europe/London)" + live,
			wantTime: "11pm",
		},
		{
			name:     "minutes remaining format",
			content:  "Context 5% │ Usage ⚠ Limit reached (resets 8m)",
			wantTime: "8m",
		},
		{
			name:     "minutes remaining double digit",
			content:  "Limit reached (resets 45m)" + live,
			wantTime: "45m",
		},
		{
			name:     "minutes remaining triple digit",
			content:  "Context 50% │ Usage ⚠ Limit reached (resets 120m)",
			wantTime: "120m",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status := CheckRateLimit(tc.content)
			if !status.IsLimited {
				t.Error("expected IsLimited to be true")
			}
			if status.ResetsAt != tc.wantTime {
				t.Errorf("expected ResetsAt to be '%s', got '%s'", tc.wantTime, status.ResetsAt)
			}
		})
	}
}

func TestCheckRateLimit_MinutesFormat(t *testing.T) {
	status := CheckRateLimit("Context 50% │ Usage ⚠ Limit reached (resets 30m)")

	if !status.IsLimited {
		t.Error("expected IsLimited to be true")
	}
	if status.ResetsAt != "30m" {
		t.Errorf("expected ResetsAt to be '30m', got '%s'", status.ResetsAt)
	}
	if status.ResetTime.IsZero() {
		t.Error("expected ResetTime to be set")
	}
	// TimeUntil should be approximately 30 minutes (within 1 second tolerance)
	expectedDuration := 30 * time.Minute
	if status.TimeUntil < expectedDuration-time.Second || status.TimeUntil > expectedDuration+time.Second {
		t.Errorf("expected TimeUntil to be ~30m, got %v", status.TimeUntil)
	}
}

func TestCheckRateLimit_FallbackNoTime_LiveIndicatorOnly(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{
			name:    "live status-bar line without parseable time",
			content: "[Opus] │ Context 5% │ Usage ⚠ Limit reached (resets in 2 hours)",
		},
		{
			name:    "live ⚠ Rate limited indicator",
			content: "Context 50% │ Usage ⚠ Rate limited",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status := CheckRateLimit(tc.content)
			if !status.IsLimited {
				t.Error("expected IsLimited to be true")
			}
			if status.ResetsAt != "" {
				t.Errorf("expected ResetsAt to be empty for fallback, got '%s'", status.ResetsAt)
			}
			if !status.ResetTime.IsZero() {
				t.Error("expected ResetTime to be zero for fallback")
			}
		})
	}
}

func TestCheckRateLimit_QuotedHistoryNotLimited(t *testing.T) {
	// Chat-history text that quotes a limit message but the pane is NOT
	// currently rate-limited (no live ⚠ indicator, no active picker).
	cases := []string{
		"User said: You've hit your limit, what now?",
		"Earlier output: Limit reached (resets 8pm)\n> follow-up prompt",
		`Quote: "  2. Stop and wait for limit to reset"  was the option I wanted.`,
	}
	for _, content := range cases {
		t.Run(content[:30], func(t *testing.T) {
			status := CheckRateLimit(content)
			if status.IsLimited {
				t.Errorf("quoted history should NOT trigger, got IsLimited=true (resets=%q)", status.ResetsAt)
			}
		})
	}
}

func TestCheckRateLimit_NoMatchCases(t *testing.T) {
	cases := []string{
		"Normal output without rate limit",
		"The limit of my patience",
		"Rate your experience",
	}

	for _, content := range cases {
		t.Run(content, func(t *testing.T) {
			status := CheckRateLimit(content)
			if status.IsLimited {
				t.Errorf("expected IsLimited to be false for: %q", content)
			}
		})
	}
}

func TestCheckRateLimit_StatusBarRelative(t *testing.T) {
	cases := []struct {
		name        string
		content     string
		wantResets  string
		wantApproxD time.Duration
	}{
		{
			name:        "hours and minutes",
			content:     "[Opus 4.7] │ Context 25% │ Usage ⚠ Limit reached (resets in 3h 10m)",
			wantResets:  "3h 10m",
			wantApproxD: 3*time.Hour + 10*time.Minute,
		},
		{
			name:        "minutes only",
			content:     "[Opus] │ Context 8% │ Usage ⚠ Limit reached (resets in 47m)",
			wantResets:  "47m",
			wantApproxD: 47 * time.Minute,
		},
		{
			name:        "alt form 'you've hit your limit'",
			content:     "You've hit your limit · resets in 2h 5m\nContext 25% │ Usage ⚠ Limit reached",
			wantResets:  "2h 5m",
			wantApproxD: 2*time.Hour + 5*time.Minute,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status := CheckRateLimit(tc.content)
			if !status.IsLimited {
				t.Fatal("expected IsLimited=true")
			}
			if status.ResetsAt != tc.wantResets {
				t.Errorf("ResetsAt = %q, want %q", status.ResetsAt, tc.wantResets)
			}
			diff := status.TimeUntil - tc.wantApproxD
			if diff < -time.Second || diff > time.Second {
				t.Errorf("TimeUntil = %v, want ≈%v (diff %v)", status.TimeUntil, tc.wantApproxD, diff)
			}
			if status.ResetTime.IsZero() {
				t.Error("expected ResetTime set")
			}
		})
	}
}

func TestCheckRateLimit_StatusBarBelowLimit_NoFalsePositive(t *testing.T) {
	// Normal status bar with usage <100% must NOT trigger.
	content := "[Opus 4.7] │ Context 8% │ Usage ██░░░░░░░░ 22% (resets in 4h 36m)"
	status := CheckRateLimit(content)
	if status.IsLimited {
		t.Errorf("normal usage status bar should not trigger, got IsLimited=true (resets=%q)", status.ResetsAt)
	}
}

func TestCheckRateLimit_TZAware(t *testing.T) {
	content := "❯ clear those stashes\n  ⎿  You've hit your limit · resets 7:10pm (America/Sao_Paulo)\nContext 25% │ Usage ⚠ Limit reached"
	status := CheckRateLimit(content)
	if !status.IsLimited {
		t.Fatal("expected IsLimited=true")
	}
	if status.ResetsAt != "7:10pm" {
		t.Errorf("ResetsAt = %q, want '7:10pm'", status.ResetsAt)
	}
	if status.ResetTime.IsZero() {
		t.Error("expected ResetTime set via LoadLocation")
	}
	// Sanity: hour-in-Sao-Paulo should equal 19 when projected back into that zone.
	loc, _ := time.LoadLocation("America/Sao_Paulo")
	if h := status.ResetTime.In(loc).Hour(); h != 19 {
		t.Errorf("ResetTime in America/Sao_Paulo = %dh, want 19h", h)
	}
}

func TestCheckRateLimit_MenuShown(t *testing.T) {
	content := `   What do you want to do?
   ❯ 1. Upgrade your plan
     2. Stop and wait for limit to reset

   Enter to confirm · Esc to cancel`
	status := CheckRateLimit(content)
	if !status.IsLimited {
		t.Fatal("expected IsLimited=true (active picker is fallback signal)")
	}
	if !status.MenuShown {
		t.Error("expected MenuShown=true")
	}
}

func TestHasReset(t *testing.T) {
	now := time.Now()

	cases := []struct {
		name   string
		status RateLimitStatus
		want   bool
	}{
		{
			name:   "not limited",
			status: RateLimitStatus{IsLimited: false},
			want:   false,
		},
		{
			name:   "limited but no reset time",
			status: RateLimitStatus{IsLimited: true},
			want:   false,
		},
		{
			name: "limited, reset time in future",
			status: RateLimitStatus{
				IsLimited: true,
				ResetTime: now.Add(1 * time.Hour),
			},
			want: false,
		},
		{
			name: "limited, reset time in past",
			status: RateLimitStatus{
				IsLimited: true,
				ResetTime: now.Add(-1 * time.Hour),
			},
			want: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.status.HasReset()
			if got != tc.want {
				t.Errorf("HasReset() = %v, want %v", got, tc.want)
			}
		})
	}
}
