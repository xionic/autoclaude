package detection

import (
	"regexp"
	"strings"
)

// Claude Code UI patterns
var (
	// Box-drawing characters used in Claude Code's UI
	boxDrawingPattern = regexp.MustCompile(`[─│┌┐└┘├┤┬┴┼╭╮╯╰]`)

	// The input prompt pattern: > at start of line (with possible ANSI codes)
	promptPattern = regexp.MustCompile(`(?m)^(\x1b\[[0-9;]*m)*>\s`)

	// Alternative: look for the Claude Code status bar patterns
	statusBarPattern = regexp.MustCompile(`(?i)(claude|anthropic|sonnet|opus|haiku)`)
)

// IsClaudeCode detects if pane content appears to be running Claude Code
func IsClaudeCode(content string) bool {
	// Must have box-drawing characters (Claude Code uses them extensively)
	if !boxDrawingPattern.MatchString(content) {
		return false
	}

	// Must have the > prompt OR Claude-related text
	hasPrompt := promptPattern.MatchString(content)
	hasStatusBar := statusBarPattern.MatchString(content)

	return hasPrompt || hasStatusBar
}

// StripANSI removes ANSI escape codes from a string
func StripANSI(s string) string {
	ansiPattern := regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
	return ansiPattern.ReplaceAllString(s, "")
}

// GetVisibleLines returns non-empty lines from content
func GetVisibleLines(content string) []string {
	lines := strings.Split(content, "\n")
	visible := make([]string, 0, len(lines))
	for _, line := range lines {
		stripped := strings.TrimSpace(StripANSI(line))
		if stripped != "" {
			visible = append(visible, stripped)
		}
	}
	return visible
}
