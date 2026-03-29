package screens

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
)

// dmLocalChrome accounts for the header row and bordered input box below the viewport.
const dmLocalChrome = 3

type DMsModel struct {
	conversations    []model.Conversation
	activeConv       *model.Conversation
	viewport         viewport.Model
	input            textinput.Model
	ready            bool
	err              error
	currentUser      string
}

type SendDMMsg struct {
	ConversationID string
	Body           string
}

func NewDMsModel(currentUser string) DMsModel {
	input := textinput.New()
	input.Placeholder = "compose cybermail..."
	input.Width = 60

	return DMsModel{input: input, currentUser: currentUser}
}

func (m DMsModel) SetConversations(convs []model.Conversation) DMsModel {
	m.conversations = convs
	return m
}

func (m DMsModel) SetActiveConversation(conv model.Conversation) DMsModel {
	m.activeConv = &conv
	if m.ready {
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
	}
	m.input.Focus()
	return m
}

// InputFocused reports whether the message input is currently active.
// Used by the app to decide whether arrow keys should navigate tabs or the input cursor.
func (m DMsModel) InputFocused() bool { return m.input.Focused() }

func (m DMsModel) Init() tea.Cmd { return textinput.Blink }

func (m DMsModel) Update(msg tea.Msg) (DMsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h := msg.Height - theme.ChromeHeight - dmLocalChrome
		if !m.ready {
			m.viewport = viewport.New(msg.Width-28, h)
			m.ready = true
		} else {
			m.viewport.Width = msg.Width - 28
			m.viewport.Height = h
		}
		if m.activeConv != nil {
			m.viewport.SetContent(m.renderMessages())
			m.viewport.GotoBottom()
		}
	case tea.KeyMsg:
		if msg.String() == "enter" && m.activeConv != nil {
			val := m.input.Value()
			if val == "" {
				break
			}
			m.input.Reset()
			return m, func() tea.Msg {
				return SendDMMsg{ConversationID: m.activeConv.ID, Body: val}
			}
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	var vpCmd tea.Cmd
	m.viewport, vpCmd = m.viewport.Update(msg)
	return m, tea.Batch(cmd, vpCmd)
}

func (m DMsModel) otherParticipant(conv model.Conversation) string {
	for _, u := range conv.Participants {
		if u.Username != m.currentUser {
			return u.Username
		}
	}
	return "unknown"
}

func (m DMsModel) renderConvList() string {
	out := theme.Title.Render("cybermail") + "\n\n"
	for _, c := range m.conversations {
		style := theme.Subtle
		if m.activeConv != nil && c.ID == m.activeConv.ID {
			style = theme.Highlight
		}
		other := m.otherParticipant(c)
		out += style.Render("@"+other) + "\n"
		if len(c.Messages) > 0 {
			last := c.Messages[len(c.Messages)-1]
			preview := last.Body
			if len(preview) > 20 {
				preview = preview[:20] + "…"
			}
			out += theme.Subtle.Render("  "+preview) + "\n"
		}
		out += "\n"
	}
	return out
}

func (m DMsModel) renderMessages() string {
	if m.activeConv == nil || len(m.activeConv.Messages) == 0 {
		return theme.Subtle.Render("no messages")
	}
	var out string
	for _, msg := range m.activeConv.Messages {
		ts := theme.Subtle.Render(msg.CreatedAt.Format("15:04"))
		author := theme.Highlight.Render("@" + msg.From.Username)
		body := theme.Base.Render(msg.Body)
		out += lipgloss.JoinHorizontal(lipgloss.Top, ts, "  ", author, "  ", body) + "\n"
	}
	return out
}

func (m DMsModel) View() string {
	if m.err != nil {
		return theme.Error.Render(fmt.Sprintf("dm error: %s", m.err))
	}

	convList := theme.Border.Width(24).Render(m.renderConvList())

	var chatArea string
	if m.activeConv == nil {
		chatArea = theme.Subtle.Render("\n  select a conversation")
	} else {
		other := m.otherParticipant(*m.activeConv)
		header := theme.Title.Render("@" + other)
		inputBox := theme.Border.Render(m.input.View())
		chatArea = lipgloss.JoinVertical(lipgloss.Left,
			header,
			m.viewport.View(),
			inputBox,
		)
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, convList, "  ", chatArea)
}
