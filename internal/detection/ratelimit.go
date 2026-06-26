package detection

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// RateLimitStatus represents the rate limit state of a pane
type RateLimitStatus struct {
	IsLimited bool
	ResetsAt  string    // Original string like "2pm", "10:30am", "3h 10m"
	ResetTime time.Time // Parsed reset time (local)
	TimeUntil time.Duration
	MenuShown bool // Blocking picker visible (need to select option 2)
}

// Reset-time patterns, ordered by specificity.
//   0: TZ-aware absolute, e.g. "You've hit your limit · resets 7:10pm (America/Sao_Paulo)"
//      Also matches "session limit" variant: "hit your session limit · resets 6:20pm (Europe/London)"
//   1: status-line relative, e.g. "Usage ⚠ Limit reached (resets in 3h 10m)" / "(resets in 47m)"
//   2: bare absolute, e.g. "You've hit your limit · resets 10pm"
//   3: bare absolute, "limit reached" prefix
//   4: minutes-remaining, e.g. "(resets 8m)"
var rateLimitPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(?:hit\s+your\s+(?:session\s+)?limit|limit\s+reached)[^\n]*?resets?\s+(\d{1,2}(?::\d{2})?\s*[ap]m)\s*\(([A-Za-z_/+\-0-9]+)\)`),
	regexp.MustCompile(`(?i)(?:hit\s+your\s+(?:session\s+)?limit|limit\s+reached)[^\n]*?resets?\s+in\s+(?:(\d+)\s*h\s*)?(\d+)\s*m\b`),
	regexp.MustCompile(`(?i)hit\s+your\s+(?:session\s+)?limit.*?resets?\s+(\d{1,2}(?::\d{2})?\s*[ap]m)`),
	regexp.MustCompile(`(?i)limit\s+reached.*?resets?\s+(\d{1,2}(?::\d{2})?\s*[ap]m)`),
	regexp.MustCompile(`(?i)(?:hit\s+your\s+(?:session\s+)?limit|limit\s+reached).*?resets?\s+(\d{1,3})m\b`),
}

// Picker shown by Claude Code v2.1.x after hitting the rate limit:
//
//	What do you want to do?
//	  ❯ 1. Upgrade your plan
//	    2. Stop and wait for limit to reset
//	  Enter to confirm · Esc to cancel
var (
	menuPattern       = regexp.MustCompile(`(?i)stop\s+and\s+wait\s+for\s+limit\s+to\s+reset`)
	menuFooterPattern = regexp.MustCompile(`(?i)enter\s+to\s+confirm\s*[·•]\s*esc\s+to\s+cancel`)
	// menuHeaderPattern is the prompt the picker prints above its options. Real
	// Claude Code renders this header but does NOT always render the
	// "Enter to confirm · Esc to cancel" footer (see the three-option form in
	// the example fixture), so the header is an alternate liveness signal.
	menuHeaderPattern = regexp.MustCompile(`(?i)what\s+do\s+you\s+want\s+to\s+do\?`)
	// Live limit indicator — the status-bar glyph Claude Code paints when it
	// currently believes itself rate-limited. The ⚠ warning sigil immediately
	// before "limit reached"/"rate limited" is what distinguishes the live
	// status bar from chat-history prose (which quotes the words but never the
	// ⚠ sigil). Earlier this also demanded "Context … Usage" on the same line,
	// but real Claude Code paints the ⚠ line on its own (e.g.
	// "⚠ Limit reached (resets 8m)"), so that extra framing matched the
	// hand-written test fixtures yet never matched a real pane — and detection
	// silently never fired.
	liveIndicator = regexp.MustCompile(`(?i)⚠\s*(?:limit\s+reached|rate\s+limited)`)

	// sessionLimitIndicator catches the inline message Claude Code emits when
	// the *subscription session limit* (rather than the API rate limit) is hit.
	// Unlike the ⚠ status-bar line, this appears as a tool-result bullet in the
	// chat content: "⎿  You've hit your session limit · resets 6:20pm …".
	// The phrase "hit your session limit" is unique enough to use as a liveness
	// signal — it doesn't appear in generic conversation history.
	sessionLimitIndicator = regexp.MustCompile(`(?i)hit\s+your\s+session\s+limit`)
)

// captureTail returns the last n lines of content for "is this rendered now"
// checks (status bar + picker live near the bottom of the viewport).
//
// Trailing blank lines are dropped first: `tmux capture-pane -p` pads its
// output to the full pane height, so a status bar that doesn't sit on the very
// bottom row arrives followed by blank padding. Without trimming, that padding
// can shove the live indicator out of the n-line window and detection silently
// misses a genuinely rate-limited pane.
func captureTail(content string, n int) string {
	lines := strings.Split(content, "\n")
	end := len(lines)
	for end > 0 && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	start := end - n
	if start < 0 {
		start = 0
	}
	return strings.Join(lines[start:end], "\n")
}

// menuActive reports whether the picker is currently rendered (not merely
// quoted in chat history). Requires the "Stop and wait" row plus picker framing
// — either the "Enter to confirm · Esc to cancel" footer or the
// "What do you want to do?" header — in the bottom of the capture. A lone
// quoted "stop and wait" line (with neither header nor footer) does not count.
func menuActive(content string) bool {
	tail := captureTail(content, 25)
	if !menuPattern.MatchString(tail) {
		return false
	}
	return menuFooterPattern.MatchString(tail) || menuHeaderPattern.MatchString(tail)
}

// liveLimited reports whether the pane is *currently* rate-limited — the ⚠
// indicator visible in the status bar, the picker actively rendered, or the
// inline "session limit" message that appears in the chat body.
func liveLimited(content string) bool {
	if menuActive(content) {
		return true
	}
	tail := captureTail(content, 25)
	return liveIndicator.MatchString(tail) || sessionLimitIndicator.MatchString(tail)
}

// CheckRateLimit inspects pane content and returns the rate-limit state.
// Rate-limit liveness is gated on the ⚠ status-bar indicator or an actively
// rendered picker — chat-history quotes of past limit messages do NOT trigger.
func CheckRateLimit(content string) RateLimitStatus {
	if !liveLimited(content) {
		return RateLimitStatus{}
	}
	menu := menuActive(content)

	var match []string
	patternIdx := -1
	for i, pattern := range rateLimitPatterns {
		match = pattern.FindStringSubmatch(content)
		if match != nil {
			patternIdx = i
			break
		}
	}

	if match == nil {
		return RateLimitStatus{IsLimited: true, MenuShown: menu}
	}

	now := time.Now()

	switch patternIdx {
	case 0:
		resetStr := match[1]
		tzName := match[2]
		loc, err := time.LoadLocation(tzName)
		if err != nil {
			loc = now.Location()
		}
		resetTime, perr := parseResetTimeInLocation(resetStr, loc)
		if perr != nil {
			return RateLimitStatus{IsLimited: true, ResetsAt: resetStr, MenuShown: menu}
		}
		resetTime = adjustForDayRollover(resetTime, now)
		return RateLimitStatus{
			IsLimited: true,
			ResetsAt:  resetStr,
			ResetTime: resetTime,
			TimeUntil: resetTime.Sub(now),
			MenuShown: menu,
		}

	case 1:
		var hours int
		if match[1] != "" {
			hours, _ = strconv.Atoi(match[1])
		}
		minutes, _ := strconv.Atoi(match[2])
		dur := time.Duration(hours)*time.Hour + time.Duration(minutes)*time.Minute
		resetTime := now.Add(dur)
		var resetStr string
		if hours > 0 {
			resetStr = strconv.Itoa(hours) + "h " + strconv.Itoa(minutes) + "m"
		} else {
			resetStr = strconv.Itoa(minutes) + "m"
		}
		return RateLimitStatus{
			IsLimited: true,
			ResetsAt:  resetStr,
			ResetTime: resetTime,
			TimeUntil: dur,
			MenuShown: menu,
		}

	case 4:
		resetStr := match[1]
		minutes, err := strconv.Atoi(resetStr)
		if err != nil {
			return RateLimitStatus{IsLimited: true, ResetsAt: resetStr + "m", MenuShown: menu}
		}
		resetTime := now.Add(time.Duration(minutes) * time.Minute)
		return RateLimitStatus{
			IsLimited: true,
			ResetsAt:  resetStr + "m",
			ResetTime: resetTime,
			TimeUntil: time.Duration(minutes) * time.Minute,
			MenuShown: menu,
		}

	default:
		resetStr := match[1]
		resetTime, err := parseResetTimeInLocation(resetStr, now.Location())
		if err != nil {
			return RateLimitStatus{IsLimited: true, ResetsAt: resetStr, MenuShown: menu}
		}
		resetTime = adjustForDayRollover(resetTime, now)
		return RateLimitStatus{
			IsLimited: true,
			ResetsAt:  resetStr,
			ResetTime: resetTime,
			TimeUntil: resetTime.Sub(now),
			MenuShown: menu,
		}
	}
}

// adjustForDayRollover bumps a reset time forward 24h when wall-clock time
// has already passed today by more than one hour. Within the last hour it
// stays as-is so HasReset() can trigger immediately.
func adjustForDayRollover(t, now time.Time) time.Time {
	if now.Sub(t) > time.Hour {
		return t.Add(24 * time.Hour)
	}
	return t
}

// parseResetTimeInLocation parses "2pm" / "10:30am" / "3 pm" in the given location.
// Returned time is in local time for direct comparison with time.Now().
func parseResetTimeInLocation(s string, loc *time.Location) (time.Time, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	now := time.Now().In(loc)

	formats := []string{"3:04pm", "3:04 pm", "3pm", "3 pm"}
	for _, format := range formats {
		t, err := time.ParseInLocation(format, s, loc)
		if err == nil {
			combined := time.Date(now.Year(), now.Month(), now.Day(),
				t.Hour(), t.Minute(), 0, 0, loc)
			return combined.In(time.Local), nil
		}
	}

	isPM := strings.Contains(s, "pm")
	clean := strings.ReplaceAll(strings.ReplaceAll(s, "am", ""), "pm", "")
	clean = strings.TrimSpace(clean)

	var hour, minute int
	if strings.Contains(clean, ":") {
		parts := strings.Split(clean, ":")
		hour, _ = strconv.Atoi(parts[0])
		minute, _ = strconv.Atoi(parts[1])
	} else {
		hour, _ = strconv.Atoi(clean)
	}
	if isPM && hour != 12 {
		hour += 12
	} else if !isPM && hour == 12 {
		hour = 0
	}

	combined := time.Date(now.Year(), now.Month(), now.Day(),
		hour, minute, 0, 0, loc)
	return combined.In(time.Local), nil
}

// HasReset reports whether the parsed reset time has elapsed.
func (r RateLimitStatus) HasReset() bool {
	if !r.IsLimited || r.ResetTime.IsZero() {
		return false
	}
	return time.Now().After(r.ResetTime)
}
