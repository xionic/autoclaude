package tui

import (
	"strings"

	"github.com/henryaj/autoclaude/internal/tmux"
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

// renderLayout renders the tmux pane layout as ASCII art
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

	// Create a 2D grid for rendering
	grid := make([][]rune, height)
	for i := range grid {
		grid[i] = make([]rune, width)
		for j := range grid[i] {
			grid[i][j] = ' '
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
		sb.WriteString(string(row))
		if i < len(grid)-1 {
			sb.WriteRune('\n')
		}
	}

	return sb.String()
}

// drawPane draws a single pane on the grid
func drawPane(grid [][]rune, p *tmux.Pane, selected bool, scaleX, scaleY float64, gridW, gridH int) {
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

	// Choose box characters
	var tl, tr, bl, br, h, v string
	if selected {
		tl, tr, bl, br, h, v = doubleTopLeft, doubleTopRight, doubleBottomLeft, doubleBottomRight, doubleHorizontal, doubleVertical
	} else {
		tl, tr, bl, br, h, v = singleTopLeft, singleTopRight, singleBottomLeft, singleBottomRight, singleHorizontal, singleVertical
	}

	// Draw corners
	setCell(grid, x1, y1, []rune(tl)[0])
	setCell(grid, x2, y1, []rune(tr)[0])
	setCell(grid, x1, y2, []rune(bl)[0])
	setCell(grid, x2, y2, []rune(br)[0])

	// Draw horizontal lines
	hRune := []rune(h)[0]
	for x := x1 + 1; x < x2; x++ {
		setCell(grid, x, y1, hRune)
		setCell(grid, x, y2, hRune)
	}

	// Draw vertical lines
	vRune := []rune(v)[0]
	for y := y1 + 1; y < y2; y++ {
		setCell(grid, x1, y, vRune)
		setCell(grid, x2, y, vRune)
	}

	// Draw mode label centered in the pane
	label := p.Mode.String()
	labelX := x1 + (x2-x1-len(label))/2
	labelY := y1 + (y2-y1)/2

	if labelX > x1 && labelX+len(label) < x2 && labelY > y1 && labelY < y2 {
		for i, r := range label {
			setCell(grid, labelX+i, labelY, r)
		}
	}
}

// setCell safely sets a cell in the grid
func setCell(grid [][]rune, x, y int, r rune) {
	if y >= 0 && y < len(grid) && x >= 0 && x < len(grid[y]) {
		grid[y][x] = r
	}
}
