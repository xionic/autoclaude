package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/henryaj/autoclaude/internal/tmux"
)

// Colors - subtle cyan/gray palette
var (
	cyan      = lipgloss.Color("#00d7ff")
	dimCyan   = lipgloss.Color("#5f87af")
	darkGray  = lipgloss.Color("#3a3a3a")
	lightGray = lipgloss.Color("#6c6c6c")
)

// Styles
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(cyan)

	versionStyle = lipgloss.NewStyle().
			Foreground(dimCyan)

	headerStyle = lipgloss.NewStyle().
			PaddingLeft(1).
			MarginBottom(1)

	mainPaneStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(darkGray).
			Padding(1, 2)

	dimTextStyle = lipgloss.NewStyle().
			Foreground(lightGray).
			Italic(true)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ff5555"))
)

// Messages
type layoutUpdateMsg struct {
	layout *tmux.Layout
	err    error
}

type Model struct {
	version        string
	width          int
	height         int
	layout         *tmux.Layout
	selectedPaneID string
	err            error
}

func New(version string) Model {
	return Model{
		version: version,
		width:   80,
		height:  24,
	}
}

func (m Model) Init() tea.Cmd {
	return fetchLayout
}

func fetchLayout() tea.Msg {
	layout, err := tmux.ListPanes()
	return layoutUpdateMsg{layout: layout, err: err}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "left":
			m.moveSelection(tmux.DirLeft)
		case "right":
			m.moveSelection(tmux.DirRight)
		case "up":
			m.moveSelection(tmux.DirUp)
		case "down":
			m.moveSelection(tmux.DirDown)
		case "tab":
			m.cycleMode()
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case layoutUpdateMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.updateLayout(msg.layout)
		}
	}

	return m, nil
}

func (m *Model) updateLayout(layout *tmux.Layout) {
	// Preserve modes from old layout
	if m.layout != nil && layout != nil {
		for _, newPane := range layout.Panes {
			if oldPane := m.layout.PaneByID(newPane.ID); oldPane != nil {
				newPane.Mode = oldPane.Mode
			}
		}
	}

	m.layout = layout

	// Ensure we have a selected pane
	if layout != nil && len(layout.Panes) > 0 {
		// Keep current selection if still valid
		if m.selectedPaneID != "" && layout.PaneByID(m.selectedPaneID) != nil {
			return
		}
		// Otherwise select first pane
		m.selectedPaneID = layout.Panes[0].ID
	}
}

func (m *Model) moveSelection(dir tmux.Direction) {
	if m.layout == nil {
		return
	}

	current := m.layout.PaneByID(m.selectedPaneID)
	if current == nil {
		return
	}

	next := m.layout.PaneInDirection(current, dir)
	if next != nil {
		m.selectedPaneID = next.ID
	}
}

func (m *Model) cycleMode() {
	if m.layout == nil {
		return
	}

	pane := m.layout.PaneByID(m.selectedPaneID)
	if pane == nil {
		return
	}

	if pane.Mode == tmux.ModeOff {
		pane.Mode = tmux.ModeContinueOnRateLimit
	} else {
		pane.Mode = tmux.ModeOff
	}
}

func (m Model) View() string {
	// Header with title and version
	title := titleStyle.Render("autoclaude")
	version := versionStyle.Render(fmt.Sprintf("v%s", m.version))
	headerWidth := m.width - 4
	if headerWidth < 20 {
		headerWidth = 20
	}
	// Place title left, version right
	titleLen := lipgloss.Width(title)
	versionLen := lipgloss.Width(version)
	spacerLen := headerWidth - titleLen - versionLen
	if spacerLen < 1 {
		spacerLen = 1
	}
	spacer := lipgloss.NewStyle().Width(spacerLen).Render("")
	header := headerStyle.Render(title + spacer + version)

	// Calculate main pane dimensions
	mainWidth := m.width - 4
	if mainWidth < 10 {
		mainWidth = 10
	}
	mainHeight := m.height - 6 // Account for header + footer + margins
	if mainHeight < 3 {
		mainHeight = 3
	}

	// Render content
	var content string
	if m.err != nil {
		content = errorStyle.Render(fmt.Sprintf("Error: %v", m.err))
	} else if m.layout == nil || len(m.layout.Panes) == 0 {
		content = dimTextStyle.Render("No panes found")
	} else {
		// Render the ASCII layout
		layoutWidth := mainWidth - 4  // Account for padding
		layoutHeight := mainHeight - 2
		content = renderLayout(m.layout, m.selectedPaneID, layoutWidth, layoutHeight)
	}

	mainPane := mainPaneStyle.
		Width(mainWidth).
		Height(mainHeight).
		Render(content)

	// Footer with help
	footer := dimTextStyle.Render("  ←↑↓→ navigate • tab toggle mode • q quit")

	// Compose the full view
	return lipgloss.JoinVertical(lipgloss.Left, header, mainPane, footer)
}
