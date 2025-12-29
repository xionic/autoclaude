package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/henryaj/autoclaude/internal/tmux"
)

// Colors for pane rendering
var (
	claudeOrange    = lipgloss.Color("#e07c3e") // Claude Code orange
	selectedColor   = lipgloss.Color("#00d7ff") // Cyan for selected
	unselectedColor = lipgloss.Color("#4a4a4a") // Dark gray for unselected
)

// Box drawing characters
const (
	// Single line (unselected)
	singleTopLeft     = "┌"
	singleTopRight    = "┐"
	singleBottomLeft  = "└"
	singleBottomRight = "┘"
	singleHorizontal  = "─"
	singleVertical    = "│"

	// Double line (selected)
	doubleTopLeft     = "╔"
	doubleTopRight    = "╗"
	doubleBottomLeft  = "╚"
	doubleBottomRight = "╝"
	doubleHorizontal  = "═"
	doubleVertical    = "║"
)

// renderLayout renders the tmux pane layout as ASCII art with colors
func renderLayout(layout *tmux.Layout, selectedID string, width, height int) string {
	if layout == nil || len(layout.Panes) == 0 {
		return ""
	}

	// Find the bounds of all panes
	maxRight, maxBottom := 0, 0
	for _, p := range layout.Panes {
		right := p.Left + p.Width
		bottom := p.Top + p.Height
		if right > maxRight {
			maxRight = right
		}
		if bottom > maxBottom {
			maxBottom = bottom
		}
	}

	if maxRight == 0 || maxBottom == 0 {
		return ""
	}

	// Create a 2D grid for rendering (stores styled strings per cell)
	grid := make([][]string, height)
	for i := range grid {
		grid[i] = make([]string, width)
		for j := range grid[i] {
			grid[i][j] = " "
		}
	}

	// Scale factors
	scaleX := float64(width-1) / float64(maxRight)
	scaleY := float64(height-1) / float64(maxBottom)

	// Draw each pane
	for _, p := range layout.Panes {
		isSelected := p.ID == selectedID
		drawPane(grid, p, isSelected, scaleX, scaleY, width, height)
	}

	// Convert grid to string
	var sb strings.Builder
	for i, row := range grid {
		for _, cell := range row {
			sb.WriteString(cell)
		}
		if i < len(grid)-1 {
			sb.WriteRune('\n')
		}
	}

	return sb.String()
}

// drawPane draws a single pane on the grid with appropriate styling
func drawPane(grid [][]string, p *tmux.Pane, selected bool, scaleX, scaleY float64, gridW, gridH int) {
	// Calculate scaled coordinates
	x1 := int(float64(p.Left) * scaleX)
	y1 := int(float64(p.Top) * scaleY)
	x2 := int(float64(p.Left+p.Width) * scaleX)
	y2 := int(float64(p.Top+p.Height) * scaleY)

	// Ensure minimum size
	if x2 <= x1 {
		x2 = x1 + 1
	}
	if y2 <= y1 {
		y2 = y1 + 1
	}

	// Clamp to grid bounds
	if x1 < 0 {
		x1 = 0
	}
	if y1 < 0 {
		y1 = 0
	}
	if x2 >= gridW {
		x2 = gridW - 1
	}
	if y2 >= gridH {
		y2 = gridH - 1
	}

	// Determine colors based on state
	var borderColor, labelColor lipgloss.Color
	if p.HasClaudeCode {
		borderColor = claudeOrange
		labelColor = claudeOrange
	} else if selected {
		borderColor = selectedColor
		labelColor = selectedColor
	} else {
		borderColor = unselectedColor
		labelColor = unselectedColor
	}

	// Override border color if selected
	if selected {
		borderColor = selectedColor
	}

	borderStyle := lipgloss.NewStyle().Foreground(borderColor)
	labelStyle := lipgloss.NewStyle().Foreground(labelColor)

	// Choose box characters
	var tl, tr, bl, br, h, v string
	if selected {
		tl, tr, bl, br, h, v = doubleTopLeft, doubleTopRight, doubleBottomLeft, doubleBottomRight, doubleHorizontal, doubleVertical
	} else {
		tl, tr, bl, br, h, v = singleTopLeft, singleTopRight, singleBottomLeft, singleBottomRight, singleHorizontal, singleVertical
	}

	// Draw corners
	setCell(grid, x1, y1, borderStyle.Render(tl))
	setCell(grid, x2, y1, borderStyle.Render(tr))
	setCell(grid, x1, y2, borderStyle.Render(bl))
	setCell(grid, x2, y2, borderStyle.Render(br))

	// Draw horizontal lines
	styledH := borderStyle.Render(h)
	for x := x1 + 1; x < x2; x++ {
		setCell(grid, x, y1, styledH)
		setCell(grid, x, y2, styledH)
	}

	// Draw vertical lines
	styledV := borderStyle.Render(v)
	for y := y1 + 1; y < y2; y++ {
		setCell(grid, x1, y, styledV)
		setCell(grid, x2, y, styledV)
	}

	// Draw labels centered in the pane
	centerY := y1 + (y2-y1)/2
	paneWidth := x2 - x1 - 2 // Available width inside borders

	if p.HasClaudeCode {
		// CC panes: show mode on center line, "claude code" below
		modeLabel := p.Mode.String()
		drawCenteredText(grid, modeLabel, x1, x2, centerY, labelStyle)

		// Show "claude code" label below if there's room
		if centerY+1 < y2 {
			titleStyle := lipgloss.NewStyle().Foreground(labelColor).Italic(true)
			drawCenteredText(grid, "claude code", x1, x2, centerY+1, titleStyle)
		}
	} else {
		// Non-CC panes: just show title in italics
		if p.Title != "" {
			titleStyle := lipgloss.NewStyle().Foreground(labelColor).Italic(true)
			title := truncate(p.Title, paneWidth)
			drawCenteredText(grid, title, x1, x2, centerY, titleStyle)
		}
	}
}

// drawCenteredText draws text centered horizontally between x1 and x2
func drawCenteredText(grid [][]string, text string, x1, x2, y int, style lipgloss.Style) {
	textRunes := []rune(text)
	textLen := len(textRunes)
	labelX := x1 + (x2-x1-textLen)/2

	if labelX > x1 && labelX+textLen < x2 && y >= 0 && y < len(grid) {
		for i, r := range textRunes {
			setCell(grid, labelX+i, y, style.Render(string(r)))
		}
	}
}

// truncate shortens a string to fit within maxLen
func truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-1]) + "…"
}

// setCell safely sets a cell in the grid
func setCell(grid [][]string, x, y int, s string) {
	if y >= 0 && y < len(grid) && x >= 0 && x < len(grid[y]) {
		grid[y][x] = s
	}
}
