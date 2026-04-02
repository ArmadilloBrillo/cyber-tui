package screens

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ragnar/cyber-tui/internal/api"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
)

// cmailLocalChrome accounts for the header row and bordered input box below the viewport.
const cmailLocalChrome = 3

// CMailFocus identifies which pane of the C-Mail screen has keyboard focus.
type CMailFocus int

const (
	FocusCMailLeft  CMailFocus = iota // conversation list pane
	FocusCMailRight                   // chat + input pane
)

// dmSubscription holds the live RTDB channel and its cancellation function.
type dmSubscription struct {
	C      <-chan model.Message
	cancel context.CancelFunc
}

// DM subscription message types — unexported, handled entirely within CMailModel.
type dmSubscribedMsg struct {
	convID string
	sub    *dmSubscription
}
type dmReceivedMsg struct{ msg model.Message }
type dmStreamClosedMsg struct{}
type cmailMsgsLoadedMsg struct {
	convID string
	msgs   []model.Message
}
type cmailErrMsg struct{ err error }

// waitForDM blocks on the subscription channel and returns the next message as a tea.Cmd.
func waitForDM(sub *dmSubscription) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-sub.C
		if !ok {
			return dmStreamClosedMsg{}
		}
		return dmReceivedMsg{msg: msg}
	}
}

// CMailModel is the screen model for C-Mail (private 1-on-1 conversations).
type CMailModel struct {
	conversations []model.Conversation
	activeConv    *model.Conversation
	viewport      viewport.Model
	input         textinput.Model
	ready         bool
	err           error
	currentUser   string

	focusPane    CMailFocus
	selectedConv int            // index into conversations
	sidebarWidth int            // inner content width, computed on WindowSizeMsg
	width        int            // terminal width, stored for View()
	loc          *time.Location // timezone for timestamp display; nil = UTC

	// DM subscription state — managed entirely within CMailModel.
	client       api.Client
	dmSub        *dmSubscription
	activeConvID string
}

// SendCMailMsg is emitted when the user sends a C-Mail message.
type SendCMailMsg struct {
	ConversationID string
	Body           string
}

// NewCMailModel creates a new CMailModel for the given authenticated user.
// Pass nil for client when using the mock or in tests.
func NewCMailModel(currentUser string, client api.Client) CMailModel {
	input := textinput.New()
	input.Placeholder = "compose c-mail..."

	return CMailModel{
		input:       input,
		currentUser: currentUser,
		client:      client,
		focusPane:   FocusCMailLeft,
	}
}

// cancelDMSub stops any active RTDB subscription and clears subscription state.
func (m CMailModel) cancelDMSub() CMailModel {
	if m.dmSub != nil {
		m.dmSub.cancel()
		m.dmSub = nil
	}
	m.activeConvID = ""
	return m
}

// CancelSubscription is called by App when navigating away from the C-Mail screen.
func (m CMailModel) CancelSubscription() CMailModel {
	return m.cancelDMSub()
}

// openDMSubscriptionCmd starts a live RTDB stream for convID.
func (m CMailModel) openDMSubscriptionCmd(convID string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		if client == nil {
			return nil
		}
		ch, cancel, err := client.SubscribeDMs(context.Background(), convID)
		if err != nil {
			return cmailErrMsg{err}
		}
		return dmSubscribedMsg{convID: convID, sub: &dmSubscription{C: ch, cancel: cancel}}
	}
}

// loadConvMessagesCmd fetches message history for a conversation.
func (m CMailModel) loadConvMessagesCmd(convID string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		if client == nil {
			return nil
		}
		msgs, err := client.GetMessages(convID, 50)
		if err != nil {
			return cmailErrMsg{err}
		}
		return cmailMsgsLoadedMsg{convID: convID, msgs: msgs}
	}
}

// SetError stores an error to display in the C-Mail view.
func (m CMailModel) SetError(err error) CMailModel {
	m.err = err
	return m
}

// SetConversations replaces the conversation list, clamping the selection cursor if needed.
func (m CMailModel) SetConversations(convs []model.Conversation) CMailModel {
	m.conversations = convs
	if len(convs) > 0 && m.selectedConv >= len(convs) {
		m.selectedConv = len(convs) - 1
	}
	return m
}

// SetActiveConversation opens a specific conversation (used by external callers).
func (m CMailModel) SetActiveConversation(conv model.Conversation) CMailModel {
	m.activeConv = &conv
	if m.ready {
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
	}
	m.input.Focus()
	m.focusPane = FocusCMailRight
	return m
}

// InputFocused reports whether the message input is currently active.
// Used by the app to prevent arrow keys from navigating tabs while typing.
func (m CMailModel) InputFocused() bool { return m.input.Focused() }

// FocusPane returns the currently focused pane.
func (m CMailModel) FocusPane() CMailFocus { return m.focusPane }

// SelectedConv returns the index of the highlighted conversation in the list.
func (m CMailModel) SelectedConv() int { return m.selectedConv }

// HasActiveConv reports whether a conversation is currently open in the chat pane.
func (m CMailModel) HasActiveConv() bool { return m.activeConv != nil }

// SidebarWidth returns the computed inner content width of the conversation list pane.
func (m CMailModel) SidebarWidth() int { return m.sidebarWidth }

func (m CMailModel) Init() tea.Cmd { return textinput.Blink }

func (m CMailModel) Update(msg tea.Msg) (CMailModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.sidebarWidth = clamp(msg.Width/4, 20, 32)
		h := msg.Height - theme.ChromeHeight - cmailLocalChrome
		// sidebarOuter = sidebarWidth + 4 (border 2 + padding 2); gap = 2
		vpWidth := msg.Width - m.sidebarWidth - 6
		if !m.ready {
			m.viewport = viewport.New(vpWidth, h)
			m.ready = true
		} else {
			m.viewport.Width = vpWidth
			m.viewport.Height = h
		}
		// input sits inside its own border (border 2 + padding 2 = 4 chars overhead)
		m.input.Width = vpWidth - 4
		if m.activeConv != nil {
			m.viewport.SetContent(m.renderMessages())
			m.viewport.GotoBottom()
		}

	// --- DM subscription messages ---

	case dmSubscribedMsg:
		// Stale guard: ignore if the user navigated away before subscription connected.
		if msg.convID != m.activeConvID {
			msg.sub.cancel()
			return m, nil
		}
		m.dmSub = msg.sub
		return m, waitForDM(m.dmSub)

	case cmailMsgsLoadedMsg:
		if m.activeConv != nil && msg.convID == m.activeConvID {
			return m.SetConversationMessages(msg.convID, msg.msgs), nil
		}
		return m, nil

	case dmReceivedMsg:
		m = m.AppendMessage(msg.msg)
		if m.dmSub != nil {
			return m, waitForDM(m.dmSub)
		}
		return m, nil

	case dmStreamClosedMsg:
		m.dmSub = nil
		return m, nil

	case cmailErrMsg:
		m.err = msg.err
		return m, nil

	// --- Key input ---

	case tea.KeyMsg:
		switch m.focusPane {
		case FocusCMailLeft:
			switch msg.String() {
			case "up", "k":
				if m.selectedConv > 0 {
					m.selectedConv--
				}
				return m, nil
			case "down", "j":
				if m.selectedConv < len(m.conversations)-1 {
					m.selectedConv++
				}
				return m, nil
			case "enter":
				if len(m.conversations) > 0 {
					conv := m.conversations[m.selectedConv]
					m = m.cancelDMSub()
					m.activeConvID = conv.ID
					m.activeConv = &conv
					if m.ready {
						m.viewport.SetContent(m.renderMessages())
						m.viewport.GotoBottom()
					}
					m.input.Focus()
					m.focusPane = FocusCMailRight
					return m, tea.Batch(
						m.loadConvMessagesCmd(conv.ID),
						m.openDMSubscriptionCmd(conv.ID),
					)
				}
				return m, nil
			case "tab":
				m.focusPane = FocusCMailRight
				m.input.Focus()
				return m, nil
			}

		case FocusCMailRight:
			switch msg.String() {
			case "enter":
				if m.input.Focused() && m.activeConv != nil {
					val := m.input.Value()
					if val != "" {
						m.input.Reset()
						convID := m.activeConv.ID
						return m, func() tea.Msg {
							return SendCMailMsg{ConversationID: convID, Body: val}
						}
					}
				}
			case "esc":
				if m.input.Focused() {
					m.input.Blur()
				} else {
					m.focusPane = FocusCMailLeft
				}
				return m, nil
			case "tab":
				m.input.Blur()
				m.focusPane = FocusCMailLeft
				return m, nil
			}
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	var vpCmd tea.Cmd
	m.viewport, vpCmd = m.viewport.Update(msg)
	return m, tea.Batch(cmd, vpCmd)
}

func (m CMailModel) otherParticipant(conv model.Conversation) string {
	for _, u := range conv.Participants {
		if u.Username != m.currentUser {
			return u.Username
		}
	}
	return "unknown"
}

func (m CMailModel) renderConvList() string {
	out := theme.Title.Render("c-mail") + "\n\n"
	maxPreview := m.sidebarWidth - 4
	if maxPreview < 4 {
		maxPreview = 4
	}
	for i, c := range m.conversations {
		nameStyle := theme.Subtle
		if i == m.selectedConv {
			nameStyle = theme.Highlight
		}
		other := m.otherParticipant(c)
		out += nameStyle.Render("@"+other) + "\n"
		if len(c.Messages) > 0 {
			last := c.Messages[len(c.Messages)-1]
			out += theme.Subtle.Render("  "+truncate(last.Body, maxPreview)) + "\n"
		}
		out += "\n"
	}
	return out
}

func (m CMailModel) renderMessages() string {
	if m.activeConv == nil || len(m.activeConv.Messages) == 0 {
		return theme.Subtle.Render("no messages")
	}
	var out string
	for _, msg := range m.activeConv.Messages {
		ts := theme.Subtle.Render(formatTime(msg.CreatedAt, m.location(), "15:04"))
		author := theme.Highlight.Render("@" + msg.From.Username)
		body := theme.Base.Render(msg.Body)
		out += lipgloss.JoinHorizontal(lipgloss.Top, ts, "  ", author, "  ", body) + "\n"
	}
	return out
}

func (m CMailModel) location() *time.Location {
	if m.loc == nil {
		return time.UTC
	}
	return m.loc
}

func (m CMailModel) SetLocation(loc *time.Location) CMailModel {
	if loc == nil {
		loc = time.UTC
	}
	m.loc = loc
	if m.ready {
		m.viewport.SetContent(m.renderMessages())
	}
	return m
}

// SetConversationMessages replaces the message history for the active conversation.
// No-op if convID doesn't match the currently open conversation.
func (m CMailModel) SetConversationMessages(convID string, msgs []model.Message) CMailModel {
	if m.activeConv == nil || m.activeConv.ID != convID {
		return m
	}
	conv := *m.activeConv
	conv.Messages = msgs
	m.activeConv = &conv
	if m.ready {
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
	}
	return m
}

// AppendMessage adds a live incoming message to the currently open conversation.
// No-op when no conversation is open.
func (m CMailModel) AppendMessage(msg model.Message) CMailModel {
	if m.activeConv == nil {
		return m
	}
	conv := *m.activeConv
	conv.Messages = append(conv.Messages, msg)
	m.activeConv = &conv
	if m.ready {
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
	}
	return m
}

func (m CMailModel) View() string {
	if m.err != nil {
		return theme.Error.Render(fmt.Sprintf("c-mail error: %s", m.err))
	}

	listBorder := theme.Border
	if m.focusPane == FocusCMailLeft {
		listBorder = theme.ActiveBorder
	}
	convList := listBorder.Width(m.sidebarWidth).Render(m.renderConvList())

	var chatArea string
	if m.activeConv == nil {
		chatArea = theme.Subtle.Render("\n  select a conversation")
	} else {
		other := m.otherParticipant(*m.activeConv)
		header := theme.Title.Render("@" + other)

		inputBorder := theme.Border
		if m.focusPane == FocusCMailRight {
			inputBorder = theme.ActiveBorder
		}
		inputBox := inputBorder.Render(m.input.View())

		chatArea = lipgloss.JoinVertical(lipgloss.Left,
			header,
			m.viewport.View(),
			inputBox,
		)
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, convList, "  ", chatArea)
}

// clamp returns v clamped to [lo, hi].
func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// truncate shortens s to at most max runes, appending "…" if cut.
func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 1 {
		return "…"
	}
	return string(runes[:max-1]) + "…"
}
