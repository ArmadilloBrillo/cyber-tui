package screens

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ragnar/cyber-tui/internal/ui/theme"
)

const flagReasonCharLimit = 500

// FlagKind distinguishes what a FlagPrompt is reporting, for its confirm text.
type FlagKind int

const (
	FlagKindPost FlagKind = iota
	FlagKindReply
	FlagKindMessage
)

// FlagSubmitMsg is emitted once the user confirms a report.
type FlagSubmitMsg struct {
	Reason string
}

// FlagCancelMsg is emitted when the user backs out of the flag flow entirely.
type FlagCancelMsg struct{}

// FlagPrompt is a two-step overlay for reporting a post or reply: type an
// optional reason, then confirm y/n before the report (idempotent but
// un-withdrawable) is sent. Shared by the feed and post-detail screens.
type FlagPrompt struct {
	active     bool
	confirming bool // true once the reason step is done and a y/n confirm is showing
	kind       FlagKind
	reason     textinput.Model
}

func NewFlagPrompt() FlagPrompt {
	ti := textinput.New()
	ti.Placeholder = "reason (optional)"
	ti.CharLimit = flagReasonCharLimit
	return FlagPrompt{reason: ti}
}

// Open starts the flag flow, focusing the reason input.
func (m FlagPrompt) Open(kind FlagKind) (FlagPrompt, tea.Cmd) {
	m.active = true
	m.confirming = false
	m.kind = kind
	m.reason.SetValue("")
	return m, m.reason.Focus()
}

func (m FlagPrompt) Active() bool { return m.active }

// Height returns the number of rows the prompt overlay consumes, for
// viewport sizing — same footprint as the delete-confirm box.
func (m FlagPrompt) Height() int { return confirmBoxHeight }

func (m FlagPrompt) close() FlagPrompt {
	m.active = false
	m.confirming = false
	m.reason.Blur()
	return m
}

// Update handles a key while the prompt is active. Callers should route
// tea.KeyMsg to this only when Active() is true, and treat all other
// message types as opaque (not consumed here).
func (m FlagPrompt) Update(msg tea.KeyMsg) (FlagPrompt, tea.Cmd) {
	if m.confirming {
		switch msg.String() {
		case "y":
			reason := strings.TrimSpace(m.reason.Value())
			m = m.close()
			return m, func() tea.Msg { return FlagSubmitMsg{Reason: reason} }
		case "n":
			m.confirming = false
			return m, m.reason.Focus()
		case "esc":
			m = m.close()
			return m, func() tea.Msg { return FlagCancelMsg{} }
		}
		return m, nil
	}

	switch msg.String() {
	case "esc":
		m = m.close()
		return m, func() tea.Msg { return FlagCancelMsg{} }
	case "enter":
		m.confirming = true
		m.reason.Blur()
		return m, nil
	}
	var cmd tea.Cmd
	m.reason, cmd = m.reason.Update(msg)
	return m, cmd
}

func (m FlagPrompt) label() string {
	switch m.kind {
	case FlagKindReply:
		return "reply"
	case FlagKindMessage:
		return "message"
	default:
		return "post"
	}
}

// View renders the overlay box at the given width (matching the viewport it
// sits below).
func (m FlagPrompt) View(width int) string {
	m.reason.TextStyle = theme.Base
	m.reason.PlaceholderStyle = theme.Subtle

	var content string
	if m.confirming {
		reasonDesc := theme.Subtle.Render("(no reason)")
		if v := strings.TrimSpace(m.reason.Value()); v != "" {
			reasonDesc = theme.Base.Render(`"` + v + `"`)
		}
		content = theme.Error.Render("Report this "+m.label()+"?") + " " + reasonDesc + "  " +
			theme.Base.Render("[y]es") + "  " +
			theme.Subtle.Render("[n]o back / esc cancel")
	} else {
		content = theme.Base.Render("Report this "+m.label()+" — reason: ") + m.reason.View() + "  " +
			theme.Subtle.Render("enter to continue / esc to cancel")
	}
	return theme.ActiveBorder.Width(width - 2).Render(content)
}
