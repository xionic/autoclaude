package detector

import (
	"strings"
	"testing"
	"time"
)

func TestDetectUsageLimit_OldFormat(t *testing.T) {
	// Use a timestamp in the future
	futureTime := time.Now().Add(1 * time.Hour).Unix()
	content := "Some output\nClaude AI usage limit reached|" + string(rune(futureTime)) + "\nMore output"

	// Actually, let's use a real string format
	content = "Some output\nClaude AI usage limit reached|1735200000\nMore output"

	info := DetectUsageLimit(content)

	if !info.Detected {
		t.Error("Expected limit to be detected")
	}
	if info.Format != "old" {
		t.Errorf("Expected format 'old', got %q", info.Format)
	}
	if info.ResetTime.Unix() != 1735200000 {
		t.Errorf("Expected reset time 1735200000, got %d", info.ResetTime.Unix())
	}
}

func TestDetectUsageLimit_NewFormat(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantHour int
	}{
		{
			name:     "morning reset",
			content:  "Usage limit reached ∙ resets 9am",
			wantHour: 9,
		},
		{
			name:     "afternoon reset",
			content:  "limit reached ∙ resets 3pm",
			wantHour: 15,
		},
		{
			name:     "noon reset",
			content:  "limit reached ∙ resets 12pm",
			wantHour: 12,
		},
		{
			name:     "midnight reset",
			content:  "limit reached ∙ resets 12am",
			wantHour: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := DetectUsageLimit(tt.content)

			if !info.Detected {
				t.Error("Expected limit to be detected")
			}
			if info.Format != "new" {
				t.Errorf("Expected format 'new', got %q", info.Format)
			}
			if info.ResetTime.Hour() != tt.wantHour {
				t.Errorf("Expected hour %d, got %d", tt.wantHour, info.ResetTime.Hour())
			}
		})
	}
}

func TestDetectUsageLimit_NoMatch(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "empty content",
			content: "",
		},
		{
			name:    "normal output",
			content: "Hello, I'm Claude. How can I help you today?",
		},
		{
			name:    "partial match",
			content: "limit reached but no time",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := DetectUsageLimit(tt.content)

			if info.Detected {
				t.Error("Expected no limit to be detected")
			}
		})
	}
}

func TestParseOldFormat(t *testing.T) {
	timestamp := int64(1735200000)
	result, err := parseOldFormat("1735200000")

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result.Unix() != timestamp {
		t.Errorf("Expected %d, got %d", timestamp, result.Unix())
	}
}

func TestParseOldFormat_Invalid(t *testing.T) {
	_, err := parseOldFormat("notanumber")

	if err == nil {
		t.Error("Expected error for invalid input")
	}
}

func TestParseNewFormat(t *testing.T) {
	tests := []struct {
		hour     string
		period   string
		wantHour int
	}{
		{"9", "am", 9},
		{"12", "am", 0},
		{"12", "pm", 12},
		{"3", "pm", 15},
		{"11", "pm", 23},
	}

	for _, tt := range tests {
		t.Run(tt.hour+tt.period, func(t *testing.T) {
			result, err := parseNewFormat(tt.hour, tt.period)

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if result.Hour() != tt.wantHour {
				t.Errorf("Expected hour %d, got %d", tt.wantHour, result.Hour())
			}
		})
	}
}

func TestParseNewFormat_Invalid(t *testing.T) {
	_, err := parseNewFormat("notanumber", "am")
	if err == nil {
		t.Error("Expected error for invalid hour input")
	}
}

func TestParseNewFormat_AllHours(t *testing.T) {
	// Test all valid hours in AM
	amHours := []struct {
		input    string
		expected int
	}{
		{"1", 1}, {"2", 2}, {"3", 3}, {"4", 4}, {"5", 5}, {"6", 6},
		{"7", 7}, {"8", 8}, {"9", 9}, {"10", 10}, {"11", 11}, {"12", 0},
	}

	for _, tt := range amHours {
		t.Run(tt.input+"am", func(t *testing.T) {
			result, err := parseNewFormat(tt.input, "am")
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if result.Hour() != tt.expected {
				t.Errorf("parseNewFormat(%q, \"am\") hour = %d, want %d", tt.input, result.Hour(), tt.expected)
			}
		})
	}

	// Test all valid hours in PM
	pmHours := []struct {
		input    string
		expected int
	}{
		{"1", 13}, {"2", 14}, {"3", 15}, {"4", 16}, {"5", 17}, {"6", 18},
		{"7", 19}, {"8", 20}, {"9", 21}, {"10", 22}, {"11", 23}, {"12", 12},
	}

	for _, tt := range pmHours {
		t.Run(tt.input+"pm", func(t *testing.T) {
			result, err := parseNewFormat(tt.input, "pm")
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if result.Hour() != tt.expected {
				t.Errorf("parseNewFormat(%q, \"pm\") hour = %d, want %d", tt.input, result.Hour(), tt.expected)
			}
		})
	}
}

func TestParseNewFormat_TomorrowRollover(t *testing.T) {
	// If current hour is past the reset hour, it should roll over to tomorrow
	result, err := parseNewFormat("1", "am")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	now := time.Now()
	if result.Before(now) {
		t.Error("Reset time should not be in the past")
	}

	// Verify the date is either today or tomorrow
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	tomorrow := today.Add(24 * time.Hour)
	resultDay := time.Date(result.Year(), result.Month(), result.Day(), 0, 0, 0, 0, result.Location())

	if !resultDay.Equal(today) && !resultDay.Equal(tomorrow) {
		t.Errorf("Reset time should be today or tomorrow, got %v", result)
	}
}

func TestDetectUsageLimit_NewFormatVariations(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{
			name:    "with unicode bullet",
			content: "Usage limit reached ∙ resets 3pm",
			want:    true,
		},
		{
			name:    "with regular dot",
			content: "limit reached . resets 3pm",
			want:    true,
		},
		{
			name:    "with hyphen",
			content: "limit reached - resets 3pm",
			want:    true,
		},
		{
			name:    "lowercase limit",
			content: "limit reached ∙ resets 9am",
			want:    true,
		},
		{
			name:    "uppercase LIMIT",
			content: "LIMIT reached ∙ resets 9am",
			want:    false, // regex is case sensitive
		},
		{
			name:    "with extra text before",
			content: "Error: You have hit the limit reached ∙ resets 9am",
			want:    true,
		},
		{
			name:    "with extra text after",
			content: "limit reached ∙ resets 9am (EST)",
			want:    true,
		},
		{
			name:    "multiline with limit in middle",
			content: "Some text\nlimit reached ∙ resets 5pm\nMore text",
			want:    true,
		},
		{
			name:    "two digit hour",
			content: "limit reached ∙ resets 10am",
			want:    true,
		},
		{
			name:    "missing resets keyword",
			content: "limit reached ∙ at 9am",
			want:    false,
		},
		{
			name:    "missing am/pm",
			content: "limit reached ∙ resets 9",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := DetectUsageLimit(tt.content)
			if info.Detected != tt.want {
				t.Errorf("DetectUsageLimit() detected = %v, want %v", info.Detected, tt.want)
			}
		})
	}
}

func TestDetectUsageLimit_OldFormatVariations(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		want      bool
		timestamp int64
	}{
		{
			name:      "standard old format",
			content:   "Claude AI usage limit reached|1735200000",
			want:      true,
			timestamp: 1735200000,
		},
		{
			name:      "with surrounding text",
			content:   "Error: Claude AI usage limit reached|1735200000\nPlease wait.",
			want:      true,
			timestamp: 1735200000,
		},
		{
			name:      "different timestamp",
			content:   "Claude AI usage limit reached|1700000000",
			want:      true,
			timestamp: 1700000000,
		},
		{
			name:      "missing pipe separator",
			content:   "Claude AI usage limit reached 1735200000",
			want:      false,
			timestamp: 0,
		},
		{
			name:      "partial message",
			content:   "usage limit reached|1735200000",
			want:      false,
			timestamp: 0,
		},
		{
			name:      "invalid timestamp (letters)",
			content:   "Claude AI usage limit reached|abc123",
			want:      false,
			timestamp: 0,
		},
		{
			name:      "empty timestamp",
			content:   "Claude AI usage limit reached|",
			want:      false,
			timestamp: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := DetectUsageLimit(tt.content)
			if info.Detected != tt.want {
				t.Errorf("DetectUsageLimit() detected = %v, want %v", info.Detected, tt.want)
			}
			if tt.want && info.ResetTime.Unix() != tt.timestamp {
				t.Errorf("DetectUsageLimit() timestamp = %d, want %d", info.ResetTime.Unix(), tt.timestamp)
			}
		})
	}
}

func TestDetectUsageLimit_PreferOldFormat(t *testing.T) {
	// When both formats are present, old format should be detected first
	content := "Claude AI usage limit reached|1735200000\nlimit reached ∙ resets 3pm"

	info := DetectUsageLimit(content)

	if !info.Detected {
		t.Fatal("Expected limit to be detected")
	}
	if info.Format != "old" {
		t.Errorf("Expected format 'old' (preferred), got %q", info.Format)
	}
}

func TestDetectUsageLimit_RawMessageCapture(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantMessage string
	}{
		{
			name:        "new format message",
			content:     "Error: limit reached ∙ resets 5pm",
			wantMessage: "limit reached ∙ resets 5pm",
		},
		{
			name:        "old format message",
			content:     "Error: Claude AI usage limit reached|1735200000",
			wantMessage: "Claude AI usage limit reached|1735200000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := DetectUsageLimit(tt.content)
			if !info.Detected {
				t.Fatal("Expected limit to be detected")
			}
			if info.RawMessage != tt.wantMessage {
				t.Errorf("RawMessage = %q, want %q", info.RawMessage, tt.wantMessage)
			}
		})
	}
}

func TestLimitInfo_ZeroValue(t *testing.T) {
	// Test that a non-detected limit has sensible zero values
	info := DetectUsageLimit("normal content with no limit")

	if info.Detected {
		t.Error("Expected Detected to be false")
	}
	if !info.ResetTime.IsZero() {
		t.Error("Expected ResetTime to be zero value")
	}
	if info.RawMessage != "" {
		t.Errorf("Expected RawMessage to be empty, got %q", info.RawMessage)
	}
	if info.Format != "" {
		t.Errorf("Expected Format to be empty, got %q", info.Format)
	}
}

func TestDetectUsageLimit_LargeContent(t *testing.T) {
	// Test with large content that has limit message at various positions
	largePrefix := strings.Repeat("This is some normal terminal output.\n", 1000)
	largeSuffix := strings.Repeat("More terminal output after the message.\n", 1000)

	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "limit at beginning",
			content: "limit reached ∙ resets 3pm\n" + largeSuffix,
		},
		{
			name:    "limit at end",
			content: largePrefix + "limit reached ∙ resets 3pm",
		},
		{
			name:    "limit in middle",
			content: largePrefix + "limit reached ∙ resets 3pm\n" + largeSuffix,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := DetectUsageLimit(tt.content)
			if !info.Detected {
				t.Error("Expected limit to be detected in large content")
			}
		})
	}
}

func TestDetectUsageLimit_SpecialCharacters(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{
			name:    "with ANSI escape codes",
			content: "\x1b[31mlimit reached ∙ resets 3pm\x1b[0m",
			want:    true,
		},
		{
			name:    "with tab characters",
			content: "\tlimit reached ∙ resets 3pm",
			want:    true,
		},
		{
			name:    "with carriage return",
			content: "limit reached ∙ resets 3pm\r",
			want:    true,
		},
		{
			name:    "with unicode spaces",
			content: "limit reached\u00A0∙\u00A0resets 3pm",
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := DetectUsageLimit(tt.content)
			if info.Detected != tt.want {
				t.Errorf("DetectUsageLimit() detected = %v, want %v", info.Detected, tt.want)
			}
		})
	}
}

// Benchmark tests

func BenchmarkDetectUsageLimit_NoMatch(b *testing.B) {
	content := strings.Repeat("Normal terminal output line\n", 100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DetectUsageLimit(content)
	}
}

func BenchmarkDetectUsageLimit_OldFormat(b *testing.B) {
	content := "Some output\nClaude AI usage limit reached|1735200000\nMore output"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DetectUsageLimit(content)
	}
}

func BenchmarkDetectUsageLimit_NewFormat(b *testing.B) {
	content := "Some output\nlimit reached ∙ resets 3pm\nMore output"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DetectUsageLimit(content)
	}
}

func BenchmarkDetectUsageLimit_LargeContent(b *testing.B) {
	content := strings.Repeat("Normal line\n", 10000) + "limit reached ∙ resets 3pm"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DetectUsageLimit(content)
	}
}

func BenchmarkParseOldFormat(b *testing.B) {
	for i := 0; i < b.N; i++ {
		parseOldFormat("1735200000")
	}
}

func BenchmarkParseNewFormat(b *testing.B) {
	for i := 0; i < b.N; i++ {
		parseNewFormat("3", "pm")
	}
}
