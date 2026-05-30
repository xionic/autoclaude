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

const pollInterval = 3 * time.Second

var (
	accentCyan   = lipgloss.Color("#00ffff")
	accentPurple = lipgloss.Color("#bd93f9")
	mutedGray    = lipgloss.Color("#6272a4")
	borderColor  = lipgloss.Color("#44475a")
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(accentCyan)

	versionStyle = lipgloss.NewStyle().
			Foreground(accentPurple)

	headerStyle = lipgloss.NewStyle().
			PaddingLeft(1).
			MarginBottom(1)

	mainPaneStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(borderColor).
			Padding(1, 2)

	dimTextStyle = lipgloss.NewStyle().
			Foreground(mutedGray)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ff5555")).
			Bold(true)
)

type layoutUpdateMsg struct {
	layout *tmux.Layout
	err    error
}

type pollTickMsg time.Time

type initMsg struct {
	ownPaneID string
	layout    *tmux.Layout
	err       error
}

type Model struct {
	version          string
	width            int
	height           int
	layout           *tmux.Layout
	selectedPaneID   string
	ownPaneID        string
	err              error
	errTime          time.Time
	testPattern      string
	lastContinueSent time.Time
	lastContinuePane string
	showHelp         bool
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

	layout, err := tmux.ListPanes("")
	if err != nil {
		return initMsg{ownPaneID: ownPaneID, err: err}
	}

	return initMsg{ownPaneID: ownPaneID, layout: layout}
}

func tickCmd() tea.Cmd {
	return tea.Tick(pollInterval, func(t time.Time) tea.Msg {
		return pollTickMsg(t)
	})
}

func fetchLayoutCmd() tea.Cmd {
	return func() tea.Msg {
		layout, err := tmux.ListPanes("")
		return layoutUpdateMsg{layout: layout, err: err}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.showHelp {
			m.showHelp = false
			return m, nil
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "h", "?":
			m.showHelp = true
		case "up", "k":
			m.moveSelection(-1)
		case "down", "j":
			m.moveSelection(1)
		case "tab", " ":
			m.cycleMode()
		case "a":
			m.enableAll()
		case "n":
			m.disableAll()
		case "r":
			m.pollPanes()
			return m, fetchLayoutCmd()
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
		m.updateLayout(msg.layout)
		m.pollPanes()
		return m, tickCmd()

	case layoutUpdateMsg:
		if msg.err != nil {
			m.err = msg.err
			m.errTime = time.Now()
		} else {
			m.updateLayout(msg.layout)
		}

	case pollTickMsg:
		if m.err != nil && time.Since(m.errTime) > 10*time.Second {
			m.err = nil
		}
		m.pollPanes()
		return m, tea.Batch(fetchLayoutCmd(), tickCmd())
	}

	return m, nil
}

func (m *Model) pollPanes() {
	if m.layout == nil {
		return
	}

	for _, pane := range m.layout.Panes {
		if pane.ID == m.ownPaneID {
			pane.HasClaudeCode = false
			continue
		}

		content, err := tmux.CapturePane(pane.ID)
		if err != nil {
			continue
		}

		pane.HasClaudeCode = detection.IsClaudeCode(content)

		if pane.HasClaudeCode {
			status := detection.CheckRateLimit(content)

			wasLimited := pane.IsRateLimited
			pane.IsRateLimited = status.IsLimited
			pane.RateLimitResets = status.ResetsAt
			pane.RateLimitTime = status.ResetTime

			if !wasLimited && status.IsLimited {
				pane.ContinueSent = false
				pane.LastPeriodicContinue = time.Time{}
			}

			if pane.IsRateLimited && pane.Mode == tmux.ModeContinueOnRateLimit {
				now := time.Now()

				if !pane.RateLimitTime.IsZero() {
					if !pane.ContinueSent && now.After(pane.RateLimitTime) {
						m.sendContinue(pane.ID)
						pane.ContinueSent = true
					}
				} else {
					periodicInterval := 15 * time.Minute
					if pane.LastPeriodicContinue.IsZero() || now.Sub(pane.LastPeriodicContinue) >= periodicInterval {
						m.sendContinue(pane.ID)
						pane.LastPeriodicContinue = now
					}
				}
			}

			if m.testPattern != "" &&
				strings.Contains(content, m.testPattern) &&
				pane.Mode == tmux.ModeContinueOnRateLimit &&
				!pane.ContinueSent {
				m.sendContinue(pane.ID)
				pane.ContinueSent = true
			}
		} else {
			pane.IsRateLimited = false
			pane.RateLimitResets = ""
			pane.RateLimitTime = time.Time{}
			pane.ContinueSent = false
			pane.LastPeriodicContinue = time.Time{}
		}
	}
}

// sendContinue dismisses any blocking menu Claude Code shows on rate limit,
// then sends "continue" + Enter. The 500ms gap gives the menu time to tear down
// and the prompt to re-render before we type — at 100ms keys raced the redraw
// and landed nowhere.
func (m *Model) sendContinue(paneID string) {
	_ = tmux.SendKeys(paneID, "Escape")
	time.Sleep(500 * time.Millisecond)
	_ = tmux.SendKeys(paneID, "continue")
	_ = tmux.SendKeys(paneID, "Enter")

	m.lastContinueSent = time.Now()
	m.lastContinuePane = paneID
}

func (m *Model) updateLayout(layout *tmux.Layout) {
	if m.layout != nil && layout != nil {
		for _, newPane := range layout.Panes {
			if oldPane := m.layout.PaneByID(newPane.ID); oldPane != nil {
				newPane.Mode = oldPane.Mode
				newPane.HasClaudeCode = oldPane.HasClaudeCode
				newPane.IsRateLimited = oldPane.IsRateLimited
				newPane.RateLimitResets = oldPane.RateLimitResets
				newPane.RateLimitTime = oldPane.RateLimitTime
				newPane.ContinueSent = oldPane.ContinueSent
				newPane.LastPeriodicContinue = oldPane.LastPeriodicContinue
			}
		}
	}

	m.layout = layout

	if layout != nil && len(layout.Panes) > 0 {
		if m.selectedPaneID != "" && layout.PaneByID(m.selectedPaneID) != nil {
			return
		}
		m.selectedPaneID = layout.Panes[0].ID
	}
}

func (m *Model) moveSelection(delta int) {
	if m.layout == nil || len(m.layout.Panes) == 0 {
		return
	}

	idx := 0
	for i, p := range m.layout.Panes {
		if p.ID == m.selectedPaneID {
			idx = i
			break
		}
	}
	idx = (idx + delta + len(m.layout.Panes)) % len(m.layout.Panes)
	m.selectedPaneID = m.layout.Panes[idx].ID
}

func (m *Model) cycleMode() {
	if m.layout == nil {
		return
	}

	pane := m.layout.PaneByID(m.selectedPaneID)
	if pane == nil || pane.ID == m.ownPaneID {
		return
	}

	if pane.Mode == tmux.ModeOff {
		pane.Mode = tmux.ModeContinueOnRateLimit
		m.checkPaneRateLimit(pane)
	} else {
		pane.Mode = tmux.ModeOff
	}
}

func (m *Model) checkPaneRateLimit(pane *tmux.Pane) {
	if pane == nil || pane.ID == m.ownPaneID {
		return
	}

	content, err := tmux.CapturePane(pane.ID)
	if err != nil {
		return
	}

	status := detection.CheckRateLimit(content)
	pane.IsRateLimited = status.IsLimited
	pane.RateLimitResets = status.ResetsAt
	pane.RateLimitTime = status.ResetTime
	pane.ContinueSent = false
}

func (m *Model) enableAll() {
	if m.layout == nil {
		return
	}
	for _, pane := range m.layout.Panes {
		if pane.ID == m.ownPaneID {
			continue
		}
		pane.Mode = tmux.ModeContinueOnRateLimit
		if pane.HasClaudeCode {
			m.checkPaneRateLimit(pane)
		}
	}
}

func (m *Model) disableAll() {
	if m.layout == nil {
		return
	}
	for _, pane := range m.layout.Panes {
		pane.Mode = tmux.ModeOff
	}
}

func (m Model) View() string {
	if m.showHelp {
		return m.renderHelp()
	}

	title := titleStyle.Render("autoclaude")
	version := versionStyle.Render(fmt.Sprintf("v%s", m.version))
	headerWidth := m.width - 4
	if headerWidth < 20 {
		headerWidth = 20
	}
	titleLen := lipgloss.Width(title)
	versionLen := lipgloss.Width(version)
	spacerLen := headerWidth - titleLen - versionLen
	if spacerLen < 1 {
		spacerLen = 1
	}
	spacer := lipgloss.NewStyle().Width(spacerLen).Render("")
	header := headerStyle.Render(title + spacer + version)

	mainWidth := m.width - 4
	if mainWidth < 10 {
		mainWidth = 10
	}
	mainHeight := m.height - 7
	if mainHeight < 3 {
		mainHeight = 3
	}

	var content string
	if m.err != nil {
		content = errorStyle.Render(fmt.Sprintf("Error: %v", m.err))
	} else if m.layout == nil || len(m.layout.Panes) == 0 {
		content = dimTextStyle.Render("No panes found")
	} else {
		content = renderLayout(m.layout, m.selectedPaneID, m.ownPaneID, mainWidth-4, mainHeight-2)
	}

	mainPane := mainPaneStyle.
		Width(mainWidth).
		Height(mainHeight).
		Render(content)

	var statusText string

	if !m.lastContinueSent.IsZero() && time.Since(m.lastContinueSent) < 20*time.Second {
		statusText = lipgloss.NewStyle().Foreground(lipgloss.Color("#f1fa8c")).Bold(true).Render("↳ continue sent to " + m.lastContinuePane)
	} else if m.layout != nil {
		if pane := m.layout.PaneByID(m.selectedPaneID); pane != nil {
			if pane.HasClaudeCode {
				if pane.Mode == tmux.ModeContinueOnRateLimit {
					statusText = lipgloss.NewStyle().Foreground(lipgloss.Color("#50fa7b")).Render("● Auto-continue enabled")
					if pane.IsRateLimited {
						if pane.ContinueSent {
							statusText += lipgloss.NewStyle().Foreground(lipgloss.Color("#f1fa8c")).Bold(true).Render(" continue sent")
						} else if pane.RateLimitResets != "" {
							statusText += errorStyle.Render(" resets " + pane.RateLimitResets)
						} else {
							statusText += errorStyle.Render(" rate limited")
						}
					}
				} else {
					statusText = dimTextStyle.Render("○ Auto-continue disabled")
				}
			}
		}
	}

	helpText := dimTextStyle.Render("↑↓ nav • tab toggle • a on • n off • r refresh • h help • q quit")

	var footer string
	if statusText != "" {
		footer = "  " + statusText + "\n  " + helpText
	} else {
		footer = "  " + helpText
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, mainPane, footer)
}

func (m Model) renderHelp() string {
	helpStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accentCyan).
		Padding(1, 2).
		Width(m.width - 4)

	titleLine := titleStyle.Render("autoclaude") + " " + versionStyle.Render(fmt.Sprintf("v%s", m.version))

	helpContent := `
Watches every tmux pane on the server for Claude Code rate-limit
messages and sends "continue" once the limit resets. Auto-continue
is on by default for every pane.

` + lipgloss.NewStyle().Bold(true).Foreground(accentCyan).Render("KEYS") + `

  ↑↓ / j k   Navigate panes
  tab/space  Toggle auto-continue for selected pane
  a          Enable auto-continue for every pane
  n          Disable auto-continue for every pane
  r          Refresh pane list
  h / ?      Show this help
  q          Quit

` + lipgloss.NewStyle().Bold(true).Foreground(accentCyan).Render("STATUS COLORS") + `

  ` + lipgloss.NewStyle().Foreground(autoGreen).Render("auto") + `         Watching this pane, will send continue on reset
  ` + lipgloss.NewStyle().Foreground(claudeOrange).Render("off") + `          Claude Code detected but auto disabled
  ` + lipgloss.NewStyle().Foreground(rateLimitRed).Render("resets …") + `     Rate limited, waiting for reset time
  ` + lipgloss.NewStyle().Foreground(lipgloss.Color("#f1fa8c")).Render("continue sent") + ` Continue already dispatched for this limit

` + lipgloss.NewStyle().Bold(true).Foreground(accentCyan).Render("HOW IT WORKS") + `

  When a Claude Code pane shows a rate limit message like
  "limit reached ∙ resets Xpm" or "You've hit your limit",
  autoclaude waits for that time to pass, then sends:
  Escape → 500ms → "continue" → Enter

  Polling occurs every 3 seconds across all sessions.

` + dimTextStyle.Render("Made by Henry Stanley (henrystanley.com)") + `
` + dimTextStyle.Render("Built with Claude Code")

	footer := dimTextStyle.Render("Press any key to close")

	return lipgloss.JoinVertical(lipgloss.Left,
		"",
		helpStyle.Render(titleLine+helpContent),
		"  "+footer,
	)
}
