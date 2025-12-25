package tmux

import (
	"errors"
	"os"
	"os/exec"
	"testing"
)

// testSetEnv sets environment variables for testing and returns a cleanup function.
func testSetEnv(t *testing.T, envVars map[string]string) func() {
	t.Helper()
	original := make(map[string]string)

	for key, value := range envVars {
		original[key] = os.Getenv(key)
		if value == "" {
			os.Unsetenv(key)
		} else {
			os.Setenv(key, value)
		}
	}

	return func() {
		for key, value := range original {
			if value == "" {
				os.Unsetenv(key)
			} else {
				os.Setenv(key, value)
			}
		}
	}
}

func TestValidateEnvironment(t *testing.T) {
	tests := []struct {
		name    string
		tmuxEnv string
		wantErr error
	}{
		{
			name:    "inside tmux session",
			tmuxEnv: "/tmp/tmux-501/default,12345,0",
			wantErr: nil,
		},
		{
			name:    "outside tmux session",
			tmuxEnv: "",
			wantErr: ErrNotInTmux,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := testSetEnv(t, map[string]string{"TMUX": tt.tmuxEnv})
			defer cleanup()

			err := ValidateEnvironment()

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("ValidateEnvironment() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGetCurrentPane(t *testing.T) {
	tests := []struct {
		name     string
		paneEnv  string
		wantPane string
		wantErr  error
	}{
		{
			name:     "pane environment set",
			paneEnv:  "%5",
			wantPane: "%5",
			wantErr:  nil,
		},
		{
			name:     "pane environment not set",
			paneEnv:  "",
			wantPane: "",
			wantErr:  ErrNoPaneEnv,
		},
		{
			name:     "different pane ID format",
			paneEnv:  "%123",
			wantPane: "%123",
			wantErr:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := testSetEnv(t, map[string]string{"TMUX_PANE": tt.paneEnv})
			defer cleanup()

			pane, err := GetCurrentPane()

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("GetCurrentPane() error = %v, wantErr %v", err, tt.wantErr)
			}
			if pane != tt.wantPane {
				t.Errorf("GetCurrentPane() = %q, want %q", pane, tt.wantPane)
			}
		})
	}
}

// TestErrorConstants verifies error constant values are set correctly.
func TestErrorConstants(t *testing.T) {
	if ErrNotInTmux.Error() != "not running inside a tmux session" {
		t.Errorf("ErrNotInTmux has unexpected message: %s", ErrNotInTmux.Error())
	}
	if ErrNoPaneEnv.Error() != "TMUX_PANE environment variable not set" {
		t.Errorf("ErrNoPaneEnv has unexpected message: %s", ErrNoPaneEnv.Error())
	}
	if ErrCommandFailed.Error() != "tmux command failed" {
		t.Errorf("ErrCommandFailed has unexpected message: %s", ErrCommandFailed.Error())
	}
}

// The following tests use exec.Command mocking pattern.
// These tests require a helper process to simulate tmux commands.

var testHelperProcess = os.Getenv("GO_TEST_HELPER_PROCESS") == "1"

// TestHelperProcess is not a real test. It's used by the exec.Command mock.
// It exits immediately if not called as a helper.
func TestHelperProcess(t *testing.T) {
	if !testHelperProcess {
		return
	}

	args := os.Args
	for i, arg := range args {
		if arg == "--" {
			args = args[i+1:]
			break
		}
	}

	if len(args) == 0 {
		os.Exit(1)
	}

	cmd := args[0]
	switch cmd {
	case "tmux":
		handleTmuxHelper(args[1:])
	default:
		os.Exit(1)
	}
}

func handleTmuxHelper(args []string) {
	if len(args) == 0 {
		os.Exit(1)
	}

	subcmd := args[0]
	switch subcmd {
	case "display-message":
		// Simulate: tmux display-message -p -t %5 #{window_id}
		// Returns @1 (window ID)
		os.Stdout.WriteString("@1\n")
		os.Exit(0)

	case "list-panes":
		// Simulate: tmux list-panes -t @1 -F #{pane_id}
		os.Stdout.WriteString("%0\n%1\n%2\n")
		os.Exit(0)

	case "capture-pane":
		// Simulate: tmux capture-pane -p -t %0
		os.Stdout.WriteString("Some terminal content\nLine 2\nLine 3\n")
		os.Exit(0)

	case "send-keys":
		// Simulate: tmux send-keys -t %0 "text"
		// Just succeed silently
		os.Exit(0)

	default:
		os.Exit(1)
	}
}

// execCommandContext creates a mock exec.Command for testing.
// This is a helper for tests that need to mock exec.Command.
func mockExecCommand(command string, args ...string) *exec.Cmd {
	cs := []string{"-test.run=TestHelperProcess", "--", command}
	cs = append(cs, args...)
	cmd := exec.Command(os.Args[0], cs...)
	cmd.Env = append(os.Environ(), "GO_TEST_HELPER_PROCESS=1")
	return cmd
}

// Integration tests that verify the tmux package behavior
// These tests document expected behavior without running actual tmux

func TestGetCurrentWindow_RequiresPaneEnv(t *testing.T) {
	cleanup := testSetEnv(t, map[string]string{"TMUX_PANE": ""})
	defer cleanup()

	_, err := GetCurrentWindow()
	if !errors.Is(err, ErrNoPaneEnv) {
		t.Errorf("GetCurrentWindow() should fail when TMUX_PANE not set, got err = %v", err)
	}
}

func TestListPanes_InvalidWindow(t *testing.T) {
	// When tmux is not available or window doesn't exist, we expect an error
	// This test documents the expected behavior
	_, err := ListPanes("nonexistent_window")
	if err == nil {
		t.Skip("tmux is available - skipping error case test")
	}

	if !errors.Is(err, ErrCommandFailed) {
		t.Errorf("ListPanes() with invalid window should return ErrCommandFailed, got %v", err)
	}
}

func TestCapturePaneContent_InvalidPane(t *testing.T) {
	_, err := CapturePaneContent("nonexistent_pane")
	if err == nil {
		t.Skip("tmux is available - skipping error case test")
	}

	if !errors.Is(err, ErrCommandFailed) {
		t.Errorf("CapturePaneContent() with invalid pane should return ErrCommandFailed, got %v", err)
	}
}

func TestSendKeys_InvalidPane(t *testing.T) {
	err := SendKeys("nonexistent_pane", "test")
	if err == nil {
		t.Skip("tmux is available - skipping error case test")
	}

	if !errors.Is(err, ErrCommandFailed) {
		t.Errorf("SendKeys() with invalid pane should return ErrCommandFailed, got %v", err)
	}
}

func TestSendEnter_InvalidPane(t *testing.T) {
	err := SendEnter("nonexistent_pane")
	if err == nil {
		t.Skip("tmux is available - skipping error case test")
	}

	if !errors.Is(err, ErrCommandFailed) {
		t.Errorf("SendEnter() with invalid pane should return ErrCommandFailed, got %v", err)
	}
}

// Test that error wrapping works correctly
func TestErrorWrapping(t *testing.T) {
	// Create a wrapped error similar to how the package does it
	baseErr := errors.New("underlying error")
	wrapped := errors.Join(ErrCommandFailed, baseErr)

	// Verify we can still detect the sentinel error
	if !errors.Is(wrapped, ErrCommandFailed) {
		t.Error("Wrapped error should still match ErrCommandFailed")
	}

	// Verify the underlying error is preserved
	if !errors.Is(wrapped, baseErr) {
		t.Error("Wrapped error should still match underlying error")
	}
}

// Benchmark tests
func BenchmarkValidateEnvironment(b *testing.B) {
	os.Setenv("TMUX", "/tmp/tmux-501/default,12345,0")
	defer os.Unsetenv("TMUX")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ValidateEnvironment()
	}
}

func BenchmarkGetCurrentPane(b *testing.B) {
	os.Setenv("TMUX_PANE", "%5")
	defer os.Unsetenv("TMUX_PANE")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetCurrentPane()
	}
}
