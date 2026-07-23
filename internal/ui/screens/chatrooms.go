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
	"github.com/ragnar/cyber-tui/internal/ui/theme"
)

// chatroomMode identifies whether the screen is in room-list or chat-detail mode.
type chatroomMode int

const (
	chatroomModeList   chatroomMode = iota // full-width room list
	chatroomModeDetail                     // header + full-width message history + input
)

// Rows consumed by the detail view's header and input box (outside the message viewport).
const (
	chatroomDetailHeaderRows = 1 // "Room Name  ·  circ" header
	chatroomInputRows        = 3 // bordered textinput: 1 content + 2 border rows
	chatroomDetailChrome     = chatroomDetailHeaderRows + chatroomInputRows
)

// roomCardHeight is the number of terminal lines each room card occupies:
// top border + 2 content rows + bottom border = 4.
const roomCardHeight = 4

// roomSubscription holds the live RTDB channel and its cancellation function.
type roomSubscription struct {
	C      <-chan model.Message
	cancel context.CancelFunc
}

// CIRC SSE subscription message types — unexported, handled within ChatroomsModel.
type roomSubscribedMsg struct {
	roomID string
	sub    *roomSubscription
}
type roomReceivedMsg struct{ msg model.Message }
type roomStreamClosedMsg struct{}
type circMsgsLoadedMsg struct {
	roomID string
	msgs   []model.Message
}
type circOlderMsgsLoadedMsg struct {
	roomID string
	msgs   []model.Message
}
type circErrMsg struct{ err error }

// waitForRoomMsg blocks on the subscription channel and returns the next message as a tea.Cmd.
func waitForRoomMsg(sub *roomSubscription) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-sub.C
		if !ok {
			return roomStreamClosedMsg{}
		}
		return roomReceivedMsg{msg: msg}
	}
}

// ChatroomsModel is the screen model for CIRC (public chatrooms).
// It operates in two modes: a full-width room list and a full-width
// message history with compose input.
type ChatroomsModel struct {
	rooms       []model.Room
	activeRoom  *model.Room
	messages    []model.Message
	listVP      viewport.Model // scrolls the room list in list mode
	viewport    viewport.Model // scrolls message history in detail mode
	input       textinput.Model
	width       int
	height      int
	ready       bool
	loc         *time.Location
	timeDisplayFormat string

	mode         chatroomMode
	selectedRoom int // index into rooms
	activeRoomID string
	sub          *roomSubscription
	currentUser  string
	client       api.Client

	// History pagination state for the active room, reset on open.
	historyExhausted bool // true once an older-page fetch returns zero messages
	loadingHistory   bool // guards re-firing while an older-page fetch is in flight
}

// SendRoomMessageMsg is emitted when the user sends a chatroom message.
type SendRoomMessageMsg struct {
	RoomID string
	Body   string
}

// RoomOpenedMsg is emitted when the user enters a chatroom. App uses it to call MarkRoomRead.
type RoomOpenedMsg struct {
	RoomID string
}

// NewChatroomsModel creates a new ChatroomsModel for the given authenticated user.
func NewChatroomsModel(currentUser string, client api.Client) ChatroomsModel {
	inp := textinput.New()
	inp.Placeholder = "type a message..."
	return ChatroomsModel{
		input:       inp,
		currentUser: currentUser,
		client:      client,
		mode:        chatroomModeList,
	}
}

// cancelRoomSub stops any active RTDB subscription and clears subscription state.
func (m ChatroomsModel) cancelRoomSub() ChatroomsModel {
	if m.sub != nil {
		m.sub.cancel()
		m.sub = nil
	}
	m.activeRoomID = ""
	return m
}

// CancelSubscription is called by App when navigating away from the CIRC screen.
func (m ChatroomsModel) CancelSubscription() ChatroomsModel {
	return m.cancelRoomSub()
}

func (m ChatroomsModel) openRoomSubscriptionCmd(roomID string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		if client == nil {
			return nil
		}
		ch, cancel, err := client.SubscribeRoom(context.Background(), roomID)
		if err != nil {
			return circErrMsg{err}
		}
		return roomSubscribedMsg{roomID: roomID, sub: &roomSubscription{C: ch, cancel: cancel}}
	}
}

func (m ChatroomsModel) loadRoomMessagesCmd(roomID string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		if client == nil {
			return nil
		}
		msgs, err := client.GetRoomMessages(roomID, 50, 0)
		if err != nil {
			return circErrMsg{err}
		}
		return circMsgsLoadedMsg{roomID: roomID, msgs: msgs}
	}
}

// loadOlderRoomMessagesCmd fetches the page of messages preceding before (ms epoch).
func (m ChatroomsModel) loadOlderRoomMessagesCmd(roomID string, before int64) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		if client == nil {
			return nil
		}
		msgs, err := client.GetRoomMessages(roomID, 50, before)
		if err != nil {
			return circErrMsg{err}
		}
		return circOlderMsgsLoadedMsg{roomID: roomID, msgs: msgs}
	}
}

// InputFocused returns true in detail mode to prevent tab-navigation key capture.
func (m ChatroomsModel) InputFocused() bool { return m.mode == chatroomModeDetail }

// IsShowingDetail reports whether the detail view is active.
func (m ChatroomsModel) IsShowingDetail() bool { return m.mode == chatroomModeDetail }

// SetRooms replaces the room list.
func (m ChatroomsModel) SetRooms(rooms []model.Room) ChatroomsModel {
	m.rooms = rooms
	if len(rooms) > 0 && m.selectedRoom >= len(rooms) {
		m.selectedRoom = len(rooms) - 1
	}
	if m.ready {
		m.listVP.SetContent(m.renderRoomCards())
	}
	return m
}

// AppendMessage adds a live incoming message to the currently open room.
func (m ChatroomsModel) AppendMessage(msg model.Message) ChatroomsModel {
	m.messages = append(m.messages, msg)
	if m.ready {
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
	}
	return m
}

// SetMessages replaces the message history for the active room.
func (m ChatroomsModel) SetMessages(roomID string, msgs []model.Message) ChatroomsModel {
	if m.activeRoom == nil || m.activeRoom.Slug != roomID {
		return m
	}
	m.messages = msgs
	if m.ready {
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
	}
	return m
}

// PrependMessages inserts an older page of history above the currently loaded
// messages, preserving the user's scroll position rather than jumping.
// No-op if roomID doesn't match the active room.
func (m ChatroomsModel) PrependMessages(roomID string, msgs []model.Message) ChatroomsModel {
	m.loadingHistory = false
	if m.activeRoom == nil || m.activeRoom.Slug != roomID {
		return m
	}
	if len(msgs) == 0 {
		m.historyExhausted = true
		return m
	}
	oldOffset := m.viewport.YOffset
	var oldLines int
	if m.ready {
		oldLines = lipgloss.Height(m.renderMessages())
	}
	m.messages = append(msgs, m.messages...)
	if m.ready {
		newContent := m.renderMessages()
		m.viewport.SetContent(newContent)
		m.viewport.SetYOffset(oldOffset + lipgloss.Height(newContent) - oldLines)
	}
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
		m.listVP.SetContent(m.renderRoomCards())
		if m.activeRoom != nil {
			m.viewport.SetContent(m.renderMessages())
		}
	}
	return m
}

func (m ChatroomsModel) Init() tea.Cmd { return textinput.Blink }

func (m ChatroomsModel) Update(msg tea.Msg) (ChatroomsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		listH := msg.Height - theme.ChromeHeight
		detailH := msg.Height - theme.ChromeHeight - chatroomDetailChrome
		if !m.ready {
			m.listVP = viewport.New(msg.Width, listH)
			m.viewport = viewport.New(msg.Width, detailH)
			m.listVP.SetContent(m.renderRoomCards())
			m.ready = true
		} else {
			m.listVP.Width = msg.Width
			m.listVP.Height = listH
			m.viewport.Width = msg.Width
			m.viewport.Height = detailH
			m.listVP.SetContent(m.renderRoomCards())
			if m.activeRoom != nil {
				m.viewport.SetContent(m.renderMessages())
			}
		}
		m.input.Width = msg.Width - 4

	case SharedConfigMsg:
		m.timeDisplayFormat = msg.Settings.TimeDisplayFormat
		m = m.SetLocation(msg.Loc)
		return m, nil

	// --- CIRC subscription lifecycle ---

	case roomSubscribedMsg:
		if msg.roomID != m.activeRoomID {
			msg.sub.cancel()
			return m, nil
		}
		m.sub = msg.sub
		return m, waitForRoomMsg(m.sub)

	case circMsgsLoadedMsg:
		return m.SetMessages(msg.roomID, msg.msgs), nil

	case circOlderMsgsLoadedMsg:
		return m.PrependMessages(msg.roomID, msg.msgs), nil

	case roomReceivedMsg:
		m = m.AppendMessage(msg.msg)
		if m.sub != nil {
			return m, waitForRoomMsg(m.sub)
		}
		return m, nil

	case roomStreamClosedMsg:
		m.sub = nil
		return m, nil

	case circErrMsg:
		return m, nil

	// --- Keyboard ---

	case tea.KeyMsg:
		switch m.mode {
		case chatroomModeList:
			switch msg.String() {
			case "up", "k":
				if m.selectedRoom > 0 {
					m.selectedRoom--
					if m.ready {
						m.listVP.SetContent(m.renderRoomCards())
						m = m.ensureRoomVisible()
					}
				}
				return m, nil
			case "down", "j":
				if m.selectedRoom < len(m.rooms)-1 {
					m.selectedRoom++
					if m.ready {
						m.listVP.SetContent(m.renderRoomCards())
						m = m.ensureRoomVisible()
					}
				}
				return m, nil
			case "enter":
				if len(m.rooms) > 0 {
					room := m.rooms[m.selectedRoom]
					m = m.cancelRoomSub()
					m.activeRoomID = room.Slug
					m.activeRoom = &room
					m.messages = nil
					m.mode = chatroomModeDetail
					m.historyExhausted = false
					m.loadingHistory = false
					m.input.Focus()
					if m.ready {
						m.viewport.SetContent(m.renderMessages())
						m.viewport.GotoBottom()
					}
					roomID := room.Slug
					return m, tea.Batch(
						m.loadRoomMessagesCmd(room.Slug),
						m.openRoomSubscriptionCmd(room.Slug),
						func() tea.Msg { return RoomOpenedMsg{RoomID: roomID} },
					)
				}
				return m, nil
			}

		case chatroomModeDetail:
			switch msg.String() {
			case "esc":
				m = m.cancelRoomSub()
				m.mode = chatroomModeList
				m.activeRoom = nil
				m.messages = nil
				m.input.Blur()
				if m.ready {
					m.listVP.SetContent(m.renderRoomCards())
				}
				return m, nil
			case "enter":
				if m.activeRoom != nil {
					val := m.input.Value()
					if val != "" {
						m.input.Reset()
						roomID := m.activeRoom.Slug
						return m, func() tea.Msg {
							return SendRoomMessageMsg{RoomID: roomID, Body: val}
						}
					}
				}
				return m, nil
			case "up":
				m.viewport.ScrollUp(1)
				if m.viewport.AtTop() && !m.loadingHistory && !m.historyExhausted &&
					m.activeRoom != nil && len(m.messages) > 0 {
					m.loadingHistory = true
					before := m.messages[0].CreatedAt.UnixMilli()
					return m, m.loadOlderRoomMessagesCmd(m.activeRoom.Slug, before)
				}
				return m, nil
			case "down":
				m.viewport.ScrollDown(1)
				return m, nil
			}
		}
	}

	// In detail mode pass remaining key input to the text input.
	if m.mode == chatroomModeDetail {
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

// ensureRoomVisible adjusts listVP.YOffset so the selected room card is in view.
func (m ChatroomsModel) ensureRoomVisible() ChatroomsModel {
	cardTop := m.selectedRoom * roomCardHeight
	cardBot := cardTop + roomCardHeight - 1
	if cardTop < m.listVP.YOffset {
		m.listVP.YOffset = cardTop
	} else if cardBot >= m.listVP.YOffset+m.listVP.Height {
		m.listVP.YOffset = cardBot - m.listVP.Height + 1
	}
	return m
}

// renderRoomCards builds the room list content for listVP.
func (m ChatroomsModel) renderRoomCards() string {
	if len(m.rooms) == 0 {
		return theme.Subtle.Render("no rooms available")
	}

	innerWidth := max(m.width-4, 1) // border 2 + padding 2

	var sb strings.Builder
	for i, r := range m.rooms {
		nameStr := theme.Highlight.Render(r.Name)

		var tsStr string
		if !r.LastMessageAt.IsZero() {
			tsStr = theme.Subtle.Render(displayTime(r.LastMessageAt, m.location(), m.timeDisplayFormat, true))
		}

		gap := innerWidth - lipgloss.Width(nameStr) - lipgloss.Width(tsStr)
		var headerLine string
		if gap > 0 {
			headerLine = nameStr + strings.Repeat(" ", gap) + tsStr
		} else {
			headerLine = nameStr
		}

		slugLine := theme.Subtle.Render(fmt.Sprintf("#%s", r.Slug))
		content := lipgloss.JoinVertical(lipgloss.Left, headerLine, slugLine)

		boxStyle := theme.Border
		if i == m.selectedRoom {
			boxStyle = theme.ActiveBorder
		}
		if m.width > 4 {
			boxStyle = boxStyle.Width(m.width - 2)
		}
		sb.WriteString(boxStyle.Render(content) + "\n")
	}
	return sb.String()
}

func (m ChatroomsModel) renderMessages() string {
	if len(m.messages) == 0 {
		return theme.Subtle.Render("no messages yet")
	}
	return renderCircMessages(m.messages, m.location(), m.timeDisplayFormat, m.viewport.Width)
}

func (m ChatroomsModel) View() string {
	switch m.mode {
	case chatroomModeDetail:
		if !m.ready {
			return ""
		}
		name := ""
		if m.activeRoom != nil {
			name = m.activeRoom.Name
		}
		header := theme.Title.Render(name + "  ·  circ")
		if m.loadingHistory {
			header += theme.Subtle.Render("  (loading history…)")
		}
		inputBox := theme.ActiveBorder.Render(m.input.View())
		return lipgloss.JoinVertical(lipgloss.Left, header, m.viewport.View(), inputBox)
	default: // chatroomModeList
		if !m.ready {
			return theme.Subtle.Render("loading rooms…")
		}
		return m.listVP.View()
	}
}
