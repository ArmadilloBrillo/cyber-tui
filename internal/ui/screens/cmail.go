package screens

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ragnar/cyber-tui/internal/api"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/markdown"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
)

// cmailMode identifies whether the screen is in list or detail mode.
type cmailMode int

const (
	cmailModeList   cmailMode = iota // full-width conversation list
	cmailModeDetail                   // full-width history + input
)

// Rows consumed by the detail view's header and input box (outside the history viewport).
const (
	cmailDetailHeaderRows = 1 // "@otheruser · c-mail" header
	cmailInputRows        = 3 // bordered textinput: 1 content + 2 border rows
	cmailDetailChrome     = cmailDetailHeaderRows + cmailInputRows
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
// It operates in two modes: a full-width conversation list (list mode) and a
// full-width message history with compose input (detail mode).
type CMailModel struct {
	conversations []model.Conversation
	activeConv    *model.Conversation
	listVP        viewport.Model // scrolls the conversation list in list mode
	viewport      viewport.Model // scrolls message history in detail mode
	input         textinput.Model
	ready         bool
	err           error
	currentUser   string

	mode         cmailMode
	selectedConv int            // index into conversations
	width        int            // terminal width
	height       int            // terminal height
	loc          *time.Location // timezone for timestamp display; nil = UTC
	timeDisplayFormat string    // "datetime", "relative", "unix", or "swatch"

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

// CMailConvSelectedMsg is emitted when the user opens a conversation.
// App uses it to call MarkCMailRead on the server.
type CMailConvSelectedMsg struct {
	ConversationID string
}

// NewCMailModel creates a new CMailModel for the given authenticated user.
func NewCMailModel(currentUser string, client api.Client) CMailModel {
	inp := textinput.New()
	inp.Placeholder = "compose c-mail..."
	return CMailModel{
		input:       inp,
		currentUser: currentUser,
		client:      client,
		mode:        cmailModeList,
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

// SetError stores an error to display.
func (m CMailModel) SetError(err error) CMailModel {
	m.err = err
	return m
}

// SetConversations replaces the conversation list, clamping the cursor if needed.
func (m CMailModel) SetConversations(convs []model.Conversation) CMailModel {
	m.conversations = convs
	m.err = nil
	if len(convs) > 0 && m.selectedConv >= len(convs) {
		m.selectedConv = len(convs) - 1
	}
	if m.ready {
		m.listVP.SetContent(m.renderConvCards())
	}
	return m
}

// SetActiveConversation opens a specific conversation (used by external callers).
// Cancels any existing DM subscription and sets activeConvID so that the
// dmSubscribedMsg / cmailMsgsLoadedMsg handlers accept the new conversation's results.
func (m CMailModel) SetActiveConversation(conv model.Conversation) CMailModel {
	m = m.cancelDMSub()
	m.activeConvID = conv.ID
	m.activeConv = &conv
	m.mode = cmailModeDetail
	m.input.Focus()
	if m.ready {
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
	}
	return m
}

// ConvOpenCmds returns the batch command to load message history and open the
// live RTDB subscription for convID. Call after SetActiveConversation.
func (m CMailModel) ConvOpenCmds(convID string) tea.Cmd {
	return tea.Batch(m.loadConvMessagesCmd(convID), m.openDMSubscriptionCmd(convID))
}

// InputFocused returns true in detail mode to prevent tab-navigation key capture.
func (m CMailModel) InputFocused() bool { return m.mode == cmailModeDetail }

// IsShowingDetail reports whether the detail view (history + input) is active.
func (m CMailModel) IsShowingDetail() bool { return m.mode == cmailModeDetail }

// SelectedConv returns the cursor index in the conversation list.
func (m CMailModel) SelectedConv() int { return m.selectedConv }

// HasActiveConv reports whether the detail view is currently open.
func (m CMailModel) HasActiveConv() bool { return m.mode == cmailModeDetail }

func (m CMailModel) Init() tea.Cmd { return textinput.Blink }

func (m CMailModel) Update(msg tea.Msg) (CMailModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		listH := msg.Height - theme.ChromeHeight
		detailH := msg.Height - theme.ChromeHeight - cmailDetailChrome
		if !m.ready {
			m.listVP = viewport.New(msg.Width, listH)
			m.viewport = viewport.New(msg.Width, detailH)
			m.listVP.SetContent(m.renderConvCards())
			m.ready = true
		} else {
			m.listVP.Width = msg.Width
			m.listVP.Height = listH
			m.viewport.Width = msg.Width
			m.viewport.Height = detailH
			m.listVP.SetContent(m.renderConvCards())
			if m.activeConv != nil {
				m.viewport.SetContent(m.renderMessages())
			}
		}
		m.input.Width = msg.Width - 4

	case SharedConfigMsg:
		m.timeDisplayFormat = msg.Settings.TimeDisplayFormat
		m = m.SetLocation(msg.Loc)
		return m, nil

	// --- DM subscription lifecycle ---

	case dmSubscribedMsg:
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

	// --- Keyboard ---

	case tea.KeyMsg:
		switch m.mode {
		case cmailModeList:
			switch msg.String() {
			case "up", "k":
				if m.selectedConv > 0 {
					m.selectedConv--
					if m.ready {
						m.listVP.SetContent(m.renderConvCards())
						m = m.ensureConvVisible()
					}
				}
				return m, nil
			case "down", "j":
				if m.selectedConv < len(m.conversations)-1 {
					m.selectedConv++
					if m.ready {
						m.listVP.SetContent(m.renderConvCards())
						m = m.ensureConvVisible()
					}
				}
				return m, nil
			case "enter":
				if len(m.conversations) > 0 {
					conv := m.conversations[m.selectedConv]
					m = m.cancelDMSub()
					m.activeConvID = conv.ID
					m.activeConv = &conv
					m.mode = cmailModeDetail
					m.input.Focus()
					if m.ready {
						m.viewport.SetContent(m.renderMessages())
						m.viewport.GotoBottom()
					}
					convID := conv.ID
					return m, tea.Batch(
						m.loadConvMessagesCmd(conv.ID),
						m.openDMSubscriptionCmd(conv.ID),
						func() tea.Msg { return CMailConvSelectedMsg{ConversationID: convID} },
					)
				}
				return m, nil
			}

		case cmailModeDetail:
			switch msg.String() {
			case "esc":
				m = m.cancelDMSub()
				m.mode = cmailModeList
				m.activeConv = nil
				m.input.Blur()
				if m.ready {
					m.listVP.SetContent(m.renderConvCards())
				}
				return m, nil
			case "enter":
				if m.activeConv != nil {
					val := m.input.Value()
					if val != "" {
						m.input.Reset()
						convID := m.activeConv.ID
						return m, func() tea.Msg {
							return SendCMailMsg{ConversationID: convID, Body: val}
						}
					}
				}
				return m, nil
			case "up":
				m.viewport.ScrollUp(1)
				return m, nil
			case "down":
				m.viewport.ScrollDown(1)
				return m, nil
			}
		}
	}

	// In detail mode pass remaining key input to the text input.
	if m.mode == cmailModeDetail {
		if km, ok := msg.(tea.KeyMsg); ok {
			km, ok = filterAmbiguousKeyMsg(km)
			if !ok {
				return m, nil
			}
			msg = km
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	// In list mode pass non-key messages (e.g. mouse scroll) to the list viewport.
	var vpCmd tea.Cmd
	m.listVP, vpCmd = m.listVP.Update(msg)
	return m, vpCmd
}

func (m CMailModel) otherParticipant(conv model.Conversation) string {
	for _, u := range conv.Participants {
		if u.Username != m.currentUser {
			return u.Username
		}
	}
	return "unknown"
}

// cmailCardHeight is the number of terminal lines each conversation card occupies:
// top border + 2 content rows + bottom border = 4 (Padding(0,1) adds no vertical rows).
// The trailing "\n" in renderConvCards terminates the line; the next card follows immediately.
const cmailCardHeight = 4

// ensureConvVisible adjusts listVP.YOffset so the selected conversation card is in the visible area.
func (m CMailModel) ensureConvVisible() CMailModel {
	cardTop := m.selectedConv * cmailCardHeight
	cardBot := cardTop + cmailCardHeight - 1
	if cardTop < m.listVP.YOffset {
		m.listVP.YOffset = cardTop
	} else if cardBot >= m.listVP.YOffset+m.listVP.Height {
		m.listVP.YOffset = cardBot - m.listVP.Height + 1
	}
	return m
}

// renderConvCards builds the conversation list content for listVP.
// Each conversation is a full-width bordered card, active border on the selected row.
func (m CMailModel) renderConvCards() string {
	if len(m.conversations) == 0 {
		if m.err != nil {
			return theme.Subtle.Render("couldn't load conversations")
		}
		return theme.Subtle.Render("no conversations yet")
	}

	innerWidth := max(m.width-4, 1) // border 2 + padding 2

	var sb strings.Builder
	for i, c := range m.conversations {
		other := m.otherParticipant(c)

		// Left side: @username
		nameStr := theme.Highlight.Render("@" + other)

		// Right side: date  (N)
		var rightParts []string
		if !c.LastMessageAt.IsZero() {
			rightParts = append(rightParts, theme.Subtle.Render(displayTime(c.LastMessageAt, m.location(), m.timeDisplayFormat, true)))
		}
		if c.UnreadCount > 0 {
			rightParts = append(rightParts, theme.Highlight.Render(fmt.Sprintf("(%d)", c.UnreadCount)))
		}
		rightStr := strings.Join(rightParts, "  ")

		gap := innerWidth - lipgloss.Width(nameStr) - lipgloss.Width(rightStr)
		var headerLine string
		if gap > 0 {
			headerLine = nameStr + strings.Repeat(" ", gap) + rightStr
		} else {
			headerLine = nameStr
		}

		// Preview: first line of LastMessage, falling back to loaded messages.
		preview := c.LastMessage
		if preview == "" && len(c.Messages) > 0 {
			preview = c.Messages[len(c.Messages)-1].Body
		}
		previewLine := theme.Subtle.Render(truncate(preview, innerWidth))

		content := lipgloss.JoinVertical(lipgloss.Left, headerLine, previewLine)

		boxStyle := theme.Border
		if i == m.selectedConv {
			boxStyle = theme.ActiveBorder
		}
		if m.width > 4 {
			boxStyle = boxStyle.Width(m.width - 2)
		}
		sb.WriteString(boxStyle.Render(content) + "\n")
	}
	return sb.String()
}

func (m CMailModel) renderMessages() string {
	if m.activeConv == nil || len(m.activeConv.Messages) == 0 {
		return theme.Subtle.Render("no messages")
	}
	return renderChatMessages(m.activeConv.Messages, m.currentUser, m.location(), m.timeDisplayFormat, m.viewport.Width)
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
		m.listVP.SetContent(m.renderConvCards())
		if m.activeConv != nil {
			m.viewport.SetContent(m.renderMessages())
		}
	}
	return m
}

// SetConversationMessages replaces the message history for the active conversation.
// No-op if convID doesn't match.
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
	switch m.mode {
	case cmailModeDetail:
		if !m.ready {
			return ""
		}
		other := ""
		if m.activeConv != nil {
			other = m.otherParticipant(*m.activeConv)
		}
		header := theme.Title.Render("@" + other + "  ·  c-mail")
		inputBox := theme.ActiveBorder.Render(m.input.View())
		return lipgloss.JoinVertical(lipgloss.Left, header, m.viewport.View(), inputBox)
	default: // cmailModeList
		if !m.ready {
			return theme.Subtle.Render("loading c-mail…")
		}
		return m.listVP.View()
	}
}

// truncate shortens s to at most max terminal columns, appending "…" if cut.
func truncate(s string, max int) string {
	return markdown.TruncateToWidth(s, max)
}
