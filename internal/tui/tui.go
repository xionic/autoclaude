package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
)

type Model struct {
	version string
	width   int
	height  int
}

func New(version string) Model {
	return Model{
		version: version,
		width:   80,
		height:  24,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}

	return m, nil
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
	mainHeight := m.height - 4 // Account for header + margins + border
	if mainHeight < 1 {
		mainHeight = 1
	}

	// Empty main pane with placeholder
	placeholder := dimTextStyle.Render("Ready")
	content := lipgloss.Place(
		mainWidth-4, // Account for padding
		mainHeight-2,
		lipgloss.Center,
		lipgloss.Center,
		placeholder,
	)

	mainPane := mainPaneStyle.
		Width(mainWidth).
		Height(mainHeight).
		Render(content)

	// Compose the full view
	return lipgloss.JoinVertical(lipgloss.Left, header, mainPane)
}
