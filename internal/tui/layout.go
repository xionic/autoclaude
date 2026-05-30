package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/henryaj/autoclaude/internal/tmux"
)

var (
	claudeOrange    = lipgloss.Color("#ffb86c")
	autoGreen       = lipgloss.Color("#50fa7b")
	selectedColor   = lipgloss.Color("#00ffff")
	unselectedColor = lipgloss.Color("#6272a4")
	rateLimitRed    = lipgloss.Color("#ff5555")
	continueYellow  = lipgloss.Color("#f1fa8c")
)

func renderLayout(layout *tmux.Layout, selectedID, ownPaneID string, width, height int) string {
	if layout == nil || len(layout.Panes) == 0 {
		return lipgloss.NewStyle().Foreground(unselectedColor).Render("No panes found")
	}

	var sb strings.Builder
	for _, p := range layout.Panes {
		sb.WriteString(renderRow(p, p.ID == selectedID, p.ID == ownPaneID, width))
		sb.WriteRune('\n')
	}
	return sb.String()
}

func renderRow(p *tmux.Pane, selected, own bool, width int) string {
	marker := "  "
	if selected {
		marker = lipgloss.NewStyle().Foreground(selectedColor).Bold(true).Render("❯ ")
	}

	loc := p.Location()
	if own {
		loc += " (autoclaude)"
	}

	var statusLabel string
	var statusColor lipgloss.Color
	switch {
	case own:
		statusLabel = "self"
		statusColor = unselectedColor
	case !p.HasClaudeCode:
		statusLabel = "—"
		statusColor = unselectedColor
	case p.IsRateLimited && p.ContinueSent:
		statusLabel = "continue sent"
		statusColor = continueYellow
	case p.IsRateLimited && p.RateLimitResets != "":
		statusLabel = "resets " + p.RateLimitResets
		statusColor = rateLimitRed
	case p.IsRateLimited:
		statusLabel = "rate limited"
		statusColor = rateLimitRed
	case p.Mode == tmux.ModeContinueOnRateLimit:
		statusLabel = "auto"
		statusColor = autoGreen
	default:
		statusLabel = "off"
		statusColor = claudeOrange
	}

	locStyle := lipgloss.NewStyle().Foreground(unselectedColor)
	if p.HasClaudeCode {
		locStyle = lipgloss.NewStyle().Foreground(claudeOrange)
	}
	if selected {
		locStyle = locStyle.Bold(true)
	}

	statusStyle := lipgloss.NewStyle().Foreground(statusColor)

	title := strings.TrimSpace(p.Title)
	if title != "" {
		title = " " + lipgloss.NewStyle().Foreground(unselectedColor).Italic(true).Render(title)
	}

	left := marker + locStyle.Render(loc) + title
	right := statusStyle.Render(statusLabel)

	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	gap := width - leftW - rightW
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}
