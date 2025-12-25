package watcher

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/henryaj/autoclaude/internal/detector"
)

// TestLastNLines tests the lastNLines helper function.
func TestLastNLines(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		n        int
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			n:        5,
			expected: "(empty)",
		},
		{
			name:     "only whitespace",
			input:    "   \n   \n   ",
			n:        5,
			expected: "(empty)",
		},
		{
			name:     "single line",
			input:    "Hello, World!",
			n:        5,
			expected: "    Hello, World!",
		},
		{
			name:     "multiple lines less than n",
			input:    "Line 1\nLine 2\nLine 3",
			n:        5,
			expected: "    Line 1\n    Line 2\n    Line 3",
		},
		{
			name:     "multiple lines more than n",
			input:    "Line 1\nLine 2\nLine 3\nLine 4\nLine 5\nLine 6\nLine 7",
			n:        3,
			expected: "    Line 5\n    Line 6\n    Line 7",
		},
		{
			name:     "lines with empty lines at end",
			input:    "Line 1\nLine 2\n\n\n",
			n:        5,
			expected: "    Line 1\n    Line 2",
		},
		{
			name:     "lines with empty lines in middle",
			input:    "Line 1\n\nLine 2\n\nLine 3",
			n:        3,
			expected: "    Line 1\n    Line 2\n    Line 3",
		},
		{
			name:     "long line gets truncated",
			input:    strings.Repeat("a", 100),
			n:        1,
			expected: "    " + strings.Repeat("a", 77) + "...",
		},
		{
			name:     "exactly 80 chars not truncated",
			input:    strings.Repeat("a", 80),
			n:        1,
			expected: "    " + strings.Repeat("a", 80),
		},
		{
			name:     "79 chars not truncated",
			input:    strings.Repeat("a", 79),
			n:        1,
			expected: "    " + strings.Repeat("a", 79),
		},
		{
			name:     "n equals zero",
			input:    "Line 1\nLine 2",
			n:        0,
			expected: "(empty)",
		},
		{
			name:     "lines with leading/trailing whitespace",
			input:    "  Line 1  \n  Line 2  ",
			n:        5,
			expected: "    Line 1\n    Line 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := lastNLines(tt.input, tt.n)
			if result != tt.expected {
				t.Errorf("lastNLines(%q, %d) = %q, want %q", tt.input, tt.n, result, tt.expected)
			}
		})
	}
}

// TestNewWatcher_NotInTmux tests watcher creation outside tmux.
func TestNewWatcher_NotInTmux(t *testing.T) {
	// Save and clear environment
	oldTmux := os.Getenv("TMUX")
	oldPane := os.Getenv("TMUX_PANE")
	os.Unsetenv("TMUX")
	os.Unsetenv("TMUX_PANE")
	defer func() {
		if oldTmux != "" {
			os.Setenv("TMUX", oldTmux)
		}
		if oldPane != "" {
			os.Setenv("TMUX_PANE", oldPane)
		}
	}()

	_, err := New(false, false)
	if err == nil {
		t.Error("New() should fail when not in tmux session")
	}
}

// TestNewWatcher_NoPaneEnv tests watcher creation without TMUX_PANE.
func TestNewWatcher_NoPaneEnv(t *testing.T) {
	// Save and modify environment
	oldTmux := os.Getenv("TMUX")
	oldPane := os.Getenv("TMUX_PANE")
	os.Setenv("TMUX", "/tmp/tmux-501/default,12345,0")
	os.Unsetenv("TMUX_PANE")
	defer func() {
		if oldTmux != "" {
			os.Setenv("TMUX", oldTmux)
		} else {
			os.Unsetenv("TMUX")
		}
		if oldPane != "" {
			os.Setenv("TMUX_PANE", oldPane)
		}
	}()

	_, err := New(false, false)
	if err == nil {
		t.Error("New() should fail when TMUX_PANE not set")
	}
}

// TestWatcherConstants verifies package constants.
func TestWatcherConstants(t *testing.T) {
	if pollInterval != 5*time.Second {
		t.Errorf("pollInterval = %v, want 5s", pollInterval)
	}
	if minDelay != 5*time.Second {
		t.Errorf("minDelay = %v, want 5s", minDelay)
	}
	if maxDelay != 10*time.Second {
		t.Errorf("maxDelay = %v, want 10s", maxDelay)
	}
	if resumeMsg != "continue" {
		t.Errorf("resumeMsg = %q, want %q", resumeMsg, "continue")
	}
	if testModeDelay != 10*time.Second {
		t.Errorf("testModeDelay = %v, want 10s", testModeDelay)
	}
}

// TestWatcherStruct verifies watcher struct fields are set correctly.
func TestWatcherStruct(t *testing.T) {
	w := &Watcher{
		window:   "@1",
		verbose:  true,
		testMode: true,
	}

	if w.window != "@1" {
		t.Errorf("window = %q, want @1", w.window)
	}
	if !w.verbose {
		t.Error("verbose should be true")
	}
	if !w.testMode {
		t.Error("testMode should be true")
	}
}

// captureOutput captures stdout during test execution.
func captureOutput(f func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	f()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

// TestWatcherLog tests the log method output format.
func TestWatcherLog(t *testing.T) {
	w := &Watcher{
		window:  "@1",
		verbose: false,
	}

	output := captureOutput(func() {
		w.log("Test message %d", 42)
	})

	if !strings.Contains(output, "[INFO]") {
		t.Error("log output should contain [INFO]")
	}
	if !strings.Contains(output, "Test message 42") {
		t.Error("log output should contain the formatted message")
	}
	// Verify timestamp format (YYYY-MM-DD HH:MM:SS)
	if !strings.Contains(output, "[20") {
		t.Error("log output should contain timestamp")
	}
}

// TestWatcherDebug tests the debug method output.
func TestWatcherDebug(t *testing.T) {
	tests := []struct {
		name       string
		verbose    bool
		wantOutput bool
	}{
		{
			name:       "verbose mode enabled",
			verbose:    true,
			wantOutput: true,
		},
		{
			name:       "verbose mode disabled",
			verbose:    false,
			wantOutput: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &Watcher{
				window:  "@1",
				verbose: tt.verbose,
			}

			output := captureOutput(func() {
				w.debug("Debug message %d", 123)
			})

			hasOutput := len(output) > 0
			if hasOutput != tt.wantOutput {
				t.Errorf("debug() output = %v, want output = %v", hasOutput, tt.wantOutput)
			}

			if tt.wantOutput {
				if !strings.Contains(output, "[DEBUG]") {
					t.Error("debug output should contain [DEBUG]")
				}
				if !strings.Contains(output, "Debug message 123") {
					t.Error("debug output should contain the formatted message")
				}
			}
		})
	}
}

// TestRunContextCancellation tests that Run respects context cancellation.
func TestRunContextCancellation(t *testing.T) {
	// Skip if not in tmux
	if os.Getenv("TMUX") == "" {
		t.Skip("Not running in tmux session")
	}

	w := &Watcher{
		window:   "@test",
		verbose:  false,
		testMode: false,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := w.Run(ctx)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("Run() returned error: %v", err)
	}

	// Should exit quickly after context is cancelled
	if elapsed > 500*time.Millisecond {
		t.Errorf("Run() took %v, expected to exit quickly after context cancellation", elapsed)
	}
}

// TestHandleLimitContextCancellation tests handleLimit respects cancellation.
func TestHandleLimitContextCancellation(t *testing.T) {
	w := &Watcher{
		window:   "@test",
		verbose:  false,
		testMode: false,
	}

	// Create a limit info with reset time in the future
	limitInfo := &detector.LimitInfo{
		Detected:   true,
		ResetTime:  time.Now().Add(1 * time.Hour),
		RawMessage: "test limit",
		Format:     "new",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	captureOutput(func() {
		w.handleLimit(ctx, "%0", limitInfo)
	})
	elapsed := time.Since(start)

	// Should exit quickly when context is cancelled, not wait for the hour
	if elapsed > 200*time.Millisecond {
		t.Errorf("handleLimit() took %v, expected to exit quickly after context cancellation", elapsed)
	}
}

// TestHandleLimitPastResetTime tests handleLimit with reset time in past.
func TestHandleLimitPastResetTime(t *testing.T) {
	w := &Watcher{
		window:   "@test",
		verbose:  false,
		testMode: false,
	}

	// Create a limit info with reset time in the past
	limitInfo := &detector.LimitInfo{
		Detected:   true,
		ResetTime:  time.Now().Add(-1 * time.Hour),
		RawMessage: "test limit",
		Format:     "new",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	output := captureOutput(func() {
		w.handleLimit(ctx, "nonexistent_pane", limitInfo)
	})

	// Should attempt to send keys (and fail since pane doesn't exist)
	if !strings.Contains(output, "Limit lifted") {
		t.Error("handleLimit() should proceed when reset time is in the past")
	}
}

// TestLimitInfoIntegration tests that detector.LimitInfo works with watcher.
func TestLimitInfoIntegration(t *testing.T) {
	// Verify the detector package returns compatible types
	content := "Usage limit reached ∙ resets 3pm"
	info := detector.DetectUsageLimit(content)

	if !info.Detected {
		t.Fatal("Expected limit to be detected")
	}

	// Verify the reset time is valid and can be used for time.Until
	if info.ResetTime.IsZero() {
		t.Error("ResetTime should not be zero")
	}

	// Verify format is set
	if info.Format != "new" {
		t.Errorf("Format = %q, want %q", info.Format, "new")
	}
}

// Benchmark tests

func BenchmarkLastNLines_Short(b *testing.B) {
	input := "Line 1\nLine 2\nLine 3"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lastNLines(input, 5)
	}
}

func BenchmarkLastNLines_Long(b *testing.B) {
	var lines []string
	for i := 0; i < 1000; i++ {
		lines = append(lines, "This is a moderately long line of text for benchmarking purposes")
	}
	input := strings.Join(lines, "\n")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lastNLines(input, 5)
	}
}

func BenchmarkLog(b *testing.B) {
	w := &Watcher{window: "@1", verbose: false}
	// Redirect stdout to discard
	old := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = old }()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.log("Benchmark message %d", i)
	}
}

func BenchmarkDebugDisabled(b *testing.B) {
	w := &Watcher{window: "@1", verbose: false}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.debug("Benchmark message %d", i)
	}
}

func BenchmarkDebugEnabled(b *testing.B) {
	w := &Watcher{window: "@1", verbose: true}
	// Redirect stdout to discard
	old := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = old }()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.debug("Benchmark message %d", i)
	}
}
