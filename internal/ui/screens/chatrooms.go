package screens

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
)

// localChrome accounts for the header row and bordered input box below the viewport.
const chatroomLocalChrome = 3

type ChatroomsModel struct {
	rooms              []model.Room
	activeRoom         *model.Room
	messages           []model.Message
	viewport           viewport.Model
	input              textinput.Model
	ready              bool
	err                error
	loc                *time.Location // timezone for timestamp display; nil = UTC
	timeDisplayFormat  string         // API setting: "datetime", "relative", "unix", "swatch"
}

type SendRoomMessageMsg struct {
	RoomID string
	Body   string
}

func NewChatroomsModel() ChatroomsModel {
	input := textinput.New()
	input.Placeholder = "type a message..."
	input.Width = 60

	return ChatroomsModel{input: input}
}

func (m ChatroomsModel) SetRooms(rooms []model.Room) ChatroomsModel {
	m.rooms = rooms
	return m
}

func (m ChatroomsModel) location() *time.Location {
	if m.loc == nil {
		return time.UTC
	}
	return m.loc
}

func (m ChatroomsModel) SetLocation(loc *time.Location) ChatroomsModel {
	if loc == nil {
		loc = time.UTC
	}
	m.loc = loc
	if m.ready {
		m.viewport.SetContent(m.renderMessages())
	}
	return m
}

func (m ChatroomsModel) SetActiveRoom(room model.Room, messages []model.Message) ChatroomsModel {
	m.activeRoom = &room
	m.messages = messages
	if m.ready {
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
	}
	m.input.Focus()
	return m
}

// InputFocused reports whether the message input is currently active.
// Used by the app to decide whether arrow keys should navigate tabs or the input cursor.
func (m ChatroomsModel) InputFocused() bool { return m.input.Focused() }

func (m ChatroomsModel) Init() tea.Cmd { return textinput.Blink }

func (m ChatroomsModel) Update(msg tea.Msg) (ChatroomsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case SharedConfigMsg:
		m.timeDisplayFormat = msg.Settings.TimeDisplayFormat
		m = m.SetLocation(msg.Loc)
		return m, nil

	case tea.WindowSizeMsg:
		h := msg.Height - theme.ChromeHeight - chatroomLocalChrome
		if !m.ready {
			m.viewport = viewport.New(msg.Width-24, h)
			m.ready = true
		} else {
			m.viewport.Width = msg.Width - 24
			m.viewport.Height = h
		}
		if m.activeRoom != nil {
			m.viewport.SetContent(m.renderMessages())
			m.viewport.GotoBottom()
		}
	case tea.KeyMsg:
		if msg.String() == "enter" && m.activeRoom != nil {
			val := m.input.Value()
			if val == "" {
				break
			}
			m.input.Reset()
			return m, func() tea.Msg {
				return SendRoomMessageMsg{RoomID: m.activeRoom.ID, Body: val}
			}
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	var vpCmd tea.Cmd
	m.viewport, vpCmd = m.viewport.Update(msg)
	return m, tea.Batch(cmd, vpCmd)
}

func (m ChatroomsModel) renderRoomList() string {
	out := theme.Title.Render("rooms") + "\n\n"
	for _, r := range m.rooms {
		style := theme.Subtle
		if m.activeRoom != nil && r.ID == m.activeRoom.ID {
			style = theme.Highlight
		}
		out += style.Render(r.Name) + "\n"
		out += theme.Subtle.Render(fmt.Sprintf("  %d online", r.Members)) + "\n\n"
	}
	return out
}

func (m ChatroomsModel) renderMessages() string {
	if len(m.messages) == 0 {
		return theme.Subtle.Render("no messages yet")
	}
	var out string
	for _, msg := range m.messages {
		ts := theme.Subtle.Render(displayTime(msg.CreatedAt, m.location(), m.timeDisplayFormat, true))
		author := theme.Highlight.Render("@" + msg.From.Username)
		body := theme.Base.Render(msg.Body)
		out += lipgloss.JoinHorizontal(lipgloss.Top, ts, "  ", author, "  ", body) + "\n"
	}
	return out
}

func (m ChatroomsModel) View() string {
	if m.err != nil {
		return theme.Error.Render(fmt.Sprintf("chatroom error: %s", m.err))
	}

	roomList := theme.Border.Width(20).Render(m.renderRoomList())

	var chatArea string
	if m.activeRoom == nil {
		chatArea = theme.Subtle.Render("\n  select a room with enter")
	} else {
		header := theme.Title.Render(m.activeRoom.Name) +
			"  " + theme.Subtle.Render(m.activeRoom.Description)
		inputBox := theme.Border.Render(m.input.View())
		chatArea = lipgloss.JoinVertical(lipgloss.Left,
			header,
			m.viewport.View(),
			inputBox,
		)
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, roomList, "  ", chatArea)
}
