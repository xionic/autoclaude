package detection

import (
	"regexp"
	"strconv"
	"strings"
)

// MenuOption describes one row of the rate-limit picker.
type MenuOption struct {
	Index    int    // 1-based index as Claude Code numbers it
	Text     string // Label after the digit
	Selected bool   // Whether ❯ marks this row currently
}

var (
	menuOptionRegex = regexp.MustCompile(`^\s*(❯)?\s*(\d+)\.\s*(.+?)\s*$`)
	stopAndWaitRegex = regexp.MustCompile(`(?i)stop\s+and\s+wait\s+for\s+limit\s+to\s+reset`)
)

// ParseMenuOptions extracts all visible picker rows from the captured content.
// Returns nil if no rows look like a menu.
func ParseMenuOptions(content string) []MenuOption {
	var opts []MenuOption
	for _, raw := range strings.Split(content, "\n") {
		line := StripANSI(raw)
		// Strip box-drawing borders that wrap each row
		line = strings.Trim(line, " │┃|")
		m := menuOptionRegex.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		idx, err := strconv.Atoi(m[2])
		if err != nil {
			continue
		}
		opts = append(opts, MenuOption{
			Index:    idx,
			Text:     m[3],
			Selected: m[1] == "❯",
		})
	}
	return opts
}

// FindStopAndWaitMove returns (downPresses, found). Negative means up.
// Caller sends abs() `Down`/`Up` keystrokes then Enter to pick the option.
// Returns (0, false) when the menu can't be parsed or "Stop and wait" missing.
func FindStopAndWaitMove(content string) (int, bool) {
	opts := ParseMenuOptions(content)
	if len(opts) == 0 {
		return 0, false
	}

	selectedIdx := -1
	targetIdx := -1
	for i, o := range opts {
		if o.Selected && selectedIdx == -1 {
			selectedIdx = i
		}
		if targetIdx == -1 && stopAndWaitRegex.MatchString(o.Text) {
			targetIdx = i
		}
	}
	if targetIdx == -1 {
		return 0, false
	}
	if selectedIdx == -1 {
		// Cursor not seen — assume first row (Claude Code default)
		selectedIdx = 0
	}
	return targetIdx - selectedIdx, true
}
