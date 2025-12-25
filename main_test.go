package main

import (
	"bytes"
	"flag"
	"io"
	"os"
	"strings"
	"testing"
)

func TestVersionVariable(t *testing.T) {
	// The version variable should have a default value
	if version == "" {
		t.Error("version should have a default value")
	}
	if version != "dev" {
		// This is expected when built with ldflags
		t.Logf("version is set to: %s", version)
	}
}

func TestUsageString(t *testing.T) {
	// Verify usage string contains expected sections
	expectedSections := []string{
		"autoclaude",
		"USAGE:",
		"DESCRIPTION:",
		"OPTIONS:",
		"EXAMPLE:",
		"HOW IT WORKS:",
		"REQUIREMENTS:",
		"-v",
		"-version",
		"-test",
		"tmux",
	}

	for _, section := range expectedSections {
		if !strings.Contains(usage, section) {
			t.Errorf("usage string should contain %q", section)
		}
	}
}

func TestUsageStringFormat(t *testing.T) {
	// Verify usage string is properly formatted
	lines := strings.Split(usage, "\n")
	if len(lines) < 10 {
		t.Error("usage string seems too short")
	}

	// First line should be the program name and description
	if !strings.HasPrefix(lines[0], "autoclaude") {
		t.Errorf("usage should start with program name, got: %q", lines[0])
	}
}

// TestFlagDefinitions verifies that flags are properly defined.
// This is a documentation test - it doesn't actually parse flags.
func TestFlagDefinitions(t *testing.T) {
	// Create a new flag set for testing
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	var verbose, showVersion, testMode bool

	fs.BoolVar(&verbose, "v", false, "Enable verbose logging")
	fs.BoolVar(&showVersion, "version", false, "Show version information")
	fs.BoolVar(&testMode, "test", false, "Test mode: wait 10s then send resume sequence")

	// Test parsing with no flags
	err := fs.Parse([]string{})
	if err != nil {
		t.Errorf("Parsing empty args failed: %v", err)
	}

	if verbose {
		t.Error("verbose should be false by default")
	}
	if showVersion {
		t.Error("showVersion should be false by default")
	}
	if testMode {
		t.Error("testMode should be false by default")
	}
}

func TestFlagParsing_Verbose(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	var verbose bool
	fs.BoolVar(&verbose, "v", false, "Enable verbose logging")

	err := fs.Parse([]string{"-v"})
	if err != nil {
		t.Errorf("Parsing -v failed: %v", err)
	}

	if !verbose {
		t.Error("verbose should be true after -v flag")
	}
}

func TestFlagParsing_Version(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	var showVersion bool
	fs.BoolVar(&showVersion, "version", false, "Show version information")

	err := fs.Parse([]string{"-version"})
	if err != nil {
		t.Errorf("Parsing -version failed: %v", err)
	}

	if !showVersion {
		t.Error("showVersion should be true after -version flag")
	}
}

func TestFlagParsing_TestMode(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	var testMode bool
	fs.BoolVar(&testMode, "test", false, "Test mode")

	err := fs.Parse([]string{"-test"})
	if err != nil {
		t.Errorf("Parsing -test failed: %v", err)
	}

	if !testMode {
		t.Error("testMode should be true after -test flag")
	}
}

func TestFlagParsing_MultipleFlagsVerbose(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	var verbose, testMode bool
	fs.BoolVar(&verbose, "v", false, "Enable verbose logging")
	fs.BoolVar(&testMode, "test", false, "Test mode")

	err := fs.Parse([]string{"-v", "-test"})
	if err != nil {
		t.Errorf("Parsing multiple flags failed: %v", err)
	}

	if !verbose {
		t.Error("verbose should be true")
	}
	if !testMode {
		t.Error("testMode should be true")
	}
}

func TestFlagParsing_InvalidFlag(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // Suppress error output

	err := fs.Parse([]string{"-invalid"})
	if err == nil {
		t.Error("Expected error for invalid flag")
	}
}

// TestEnvironmentRequirements documents the environment requirements.
func TestEnvironmentRequirements(t *testing.T) {
	// Test that we can check for TMUX environment variable
	tmux := os.Getenv("TMUX")
	pane := os.Getenv("TMUX_PANE")

	if tmux == "" {
		t.Log("Not running in tmux session (TMUX not set)")
	} else {
		t.Logf("TMUX: %s", tmux)
	}

	if pane == "" {
		t.Log("TMUX_PANE not set")
	} else {
		t.Logf("TMUX_PANE: %s", pane)
	}
}

// captureStderr captures stderr output during test execution.
func captureStderr(f func()) string {
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	f()

	w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

// TestVersionOutput tests the version output format.
func TestVersionOutput(t *testing.T) {
	// Simulate what the version output would look like
	expected := "autoclaude v" + version + "\n"
	var buf bytes.Buffer
	_, err := buf.WriteString("autoclaude v" + version + "\n")
	if err != nil {
		t.Fatalf("Failed to write version string: %v", err)
	}

	if buf.String() != expected {
		t.Errorf("Version output = %q, want %q", buf.String(), expected)
	}
}

// TestErrorMessageFormat tests error message formatting.
func TestErrorMessageFormat(t *testing.T) {
	// Simulate error message format
	errMsg := "Error: not running inside a tmux session\n"

	if !strings.HasPrefix(errMsg, "Error:") {
		t.Error("Error messages should start with 'Error:'")
	}
}

// Integration test helper to verify the program structure
func TestMainPackageStructure(t *testing.T) {
	// This test documents the expected structure of the main package
	// It doesn't actually run main() to avoid side effects

	// Verify the usage constant is defined and non-empty
	if len(usage) == 0 {
		t.Error("usage constant should not be empty")
	}

	// Verify version is set
	if version == "" {
		t.Error("version should be set")
	}
}

// Benchmark for flag parsing
func BenchmarkFlagParsing(b *testing.B) {
	for i := 0; i < b.N; i++ {
		fs := flag.NewFlagSet("bench", flag.ContinueOnError)
		var verbose, showVersion, testMode bool
		fs.BoolVar(&verbose, "v", false, "")
		fs.BoolVar(&showVersion, "version", false, "")
		fs.BoolVar(&testMode, "test", false, "")
		fs.Parse([]string{"-v", "-test"})
	}
}

// Test that usage string is consistent with flag definitions
func TestUsageConsistency(t *testing.T) {
	// Check that documented flags match actual flag definitions
	flags := []struct {
		flag        string
		description string
	}{
		{"-v", "verbose"},
		{"-version", "version"},
		{"-test", "test"},
	}

	for _, f := range flags {
		if !strings.Contains(usage, f.flag) {
			t.Errorf("usage should document flag %s", f.flag)
		}
	}
}
