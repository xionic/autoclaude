package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/henryaj/autoclaude/internal/detection"
	"github.com/henryaj/autoclaude/internal/tmux"
)

const pollInterval = 10 * time.Second

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

type pollTickMsg time.Time

type initMsg struct {
	ownPaneID   string
	ownWindowID string
	layout      *tmux.Layout
	err         error
}

type Model struct {
	version          string
	width            int
	height           int
	layout           *tmux.Layout
	selectedPaneID   string
	ownPaneID        string    // The pane running autoclaude (excluded from detection)
	ownWindowID      string    // The window to monitor (pinned at startup)
	err              error
	errTime          time.Time // When the error occurred (for auto-clear)
	testPattern      string    // Test mode: trigger on this string instead of rate limit
	lastContinueSent time.Time // When we last sent a continue command
	lastContinuePane string    // Which pane we sent it to
}

func New(version string, testPattern string) Model {
	return Model{
		version:     version,
		testPattern: testPattern,
		width:       80,
		height:      24,
	}
}

func (m Model) Init() tea.Cmd {
	return doInit
}

func doInit() tea.Msg {
	ownPaneID, err := tmux.CurrentPaneID()
	if err != nil {
		return initMsg{err: err}
	}

	ownWindowID, err := tmux.CurrentWindowID()
	if err != nil {
		return initMsg{ownPaneID: ownPaneID, err: err}
	}

	layout, err := tmux.ListPanes(ownWindowID)
	if err != nil {
		return initMsg{ownPaneID: ownPaneID, ownWindowID: ownWindowID, err: err}
	}

	return initMsg{ownPaneID: ownPaneID, ownWindowID: ownWindowID, layout: layout}
}

func tickCmd() tea.Cmd {
	return tea.Tick(pollInterval, func(t time.Time) tea.Msg {
		return pollTickMsg(t)
	})
}

func fetchLayoutCmd(windowID string) tea.Cmd {
	return func() tea.Msg {
		layout, err := tmux.ListPanes(windowID)
		return layoutUpdateMsg{layout: layout, err: err}
	}
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
		case "a":
			m.enableAll()
		case "n":
			m.disableAll()
		case "r":
			m.pollPanes()
			return m, fetchLayoutCmd(m.ownWindowID)
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case initMsg:
		if msg.err != nil {
			m.err = msg.err
			m.errTime = time.Now()
			return m, nil
		}
		m.ownPaneID = msg.ownPaneID
		m.ownWindowID = msg.ownWindowID
		m.updateLayout(msg.layout)
		m.pollPanes() // Poll immediately
		return m, tickCmd()

	case layoutUpdateMsg:
		if msg.err != nil {
			m.err = msg.err
			m.errTime = time.Now()
		} else {
			m.updateLayout(msg.layout)
		}

	case pollTickMsg:
		// Clear errors after 10 seconds
		if m.err != nil && time.Since(m.errTime) > 10*time.Second {
			m.err = nil
		}
		m.pollPanes()
		return m, tea.Batch(fetchLayoutCmd(m.ownWindowID), tickCmd())
	}

	return m, nil
}

func (m *Model) pollPanes() {
	if m.layout == nil {
		return
	}

	for _, pane := range m.layout.Panes {
		// Skip our own pane
		if pane.ID == m.ownPaneID {
			pane.HasClaudeCode = false
			continue
		}

		content, err := tmux.CapturePane(pane.ID)
		if err != nil {
			continue
		}

		pane.HasClaudeCode = detection.IsClaudeCode(content)

		// Check rate limit status for Claude Code panes
		if pane.HasClaudeCode {
			status := detection.CheckRateLimit(content)
			wasLimited := pane.IsRateLimited
			pane.WasRateLimited = wasLimited
			pane.IsRateLimited = status.IsLimited
			pane.RateLimitResets = status.ResetsAt

			// Auto-continue: if rate limit just reset and mode is auto
			if wasLimited && !status.IsLimited && pane.Mode == tmux.ModeContinueOnRateLimit {
				m.sendContinue(pane.ID)
			}

			// Test mode: trigger on test pattern
			if m.testPattern != "" && strings.Contains(content, m.testPattern) && pane.Mode == tmux.ModeContinueOnRateLimit {
				m.sendContinue(pane.ID)
			}
		} else {
			pane.IsRateLimited = false
			pane.RateLimitResets = ""
		}
	}
}

// sendContinue sends the continue command sequence to a pane
func (m *Model) sendContinue(paneID string) {
	// Send: Enter, "continue", Enter
	_ = tmux.SendKeys(paneID, "Enter")
	_ = tmux.SendKeys(paneID, "continue")
	_ = tmux.SendKeys(paneID, "Enter")

	// Track for UI feedback
	m.lastContinueSent = time.Now()
	m.lastContinuePane = paneID
}

func (m *Model) updateLayout(layout *tmux.Layout) {
	// Preserve state from old layout
	if m.layout != nil && layout != nil {
		for _, newPane := range layout.Panes {
			if oldPane := m.layout.PaneByID(newPane.ID); oldPane != nil {
				newPane.Mode = oldPane.Mode
				newPane.HasClaudeCode = oldPane.HasClaudeCode
				newPane.IsRateLimited = oldPane.IsRateLimited
				newPane.RateLimitResets = oldPane.RateLimitResets
				newPane.WasRateLimited = oldPane.WasRateLimited
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

	// Only allow mode changes on Claude Code panes
	if !pane.HasClaudeCode {
		return
	}

	if pane.Mode == tmux.ModeOff {
		pane.Mode = tmux.ModeContinueOnRateLimit
	} else {
		pane.Mode = tmux.ModeOff
	}
}

func (m *Model) enableAll() {
	if m.layout == nil {
		return
	}
	for _, pane := range m.layout.Panes {
		if pane.HasClaudeCode {
			pane.Mode = tmux.ModeContinueOnRateLimit
		}
	}
}

func (m *Model) disableAll() {
	if m.layout == nil {
		return
	}
	for _, pane := range m.layout.Panes {
		if pane.HasClaudeCode {
			pane.Mode = tmux.ModeOff
		}
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

	// Footer with selected pane status (left) and help (right)
	var statusText string

	// Show "continue sent" message for 20 seconds after sending
	if !m.lastContinueSent.IsZero() && time.Since(m.lastContinueSent) < 20*time.Second {
		statusText = lipgloss.NewStyle().Foreground(lipgloss.Color("#f1fa8c")).Bold(true).Render("↳ continue sent!")
	} else if m.layout != nil {
		if pane := m.layout.PaneByID(m.selectedPaneID); pane != nil {
			if pane.HasClaudeCode {
				if pane.IsRateLimited {
					statusText = errorStyle.Render("⏳ Rate limited")
					if pane.RateLimitResets != "" {
						statusText += dimTextStyle.Render(" resets " + pane.RateLimitResets)
					}
				} else if pane.Mode == tmux.ModeContinueOnRateLimit {
					statusText = lipgloss.NewStyle().Foreground(lipgloss.Color("#50fa7b")).Render("● Auto-continue enabled")
				} else {
					statusText = dimTextStyle.Render("○ Auto-continue disabled")
				}
			}
		}
	}

	helpText := dimTextStyle.Render("←↑↓→ nav • tab toggle • a on • n off • r refresh • q quit")

	// Calculate spacing to right-align help text
	statusLen := lipgloss.Width(statusText)
	helpLen := lipgloss.Width(helpText)
	footerWidth := m.width - 4
	footerSpacerLen := footerWidth - statusLen - helpLen
	if footerSpacerLen < 1 {
		footerSpacerLen = 1
	}
	footerSpacer := lipgloss.NewStyle().Width(footerSpacerLen).Render("")
	footer := "  " + statusText + footerSpacer + helpText

	// Compose the full view
	return lipgloss.JoinVertical(lipgloss.Left, header, mainPane, footer)
}
