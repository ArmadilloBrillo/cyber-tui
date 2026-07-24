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
	chatroomDetailHeaderRows = 1 // "Room Name  #slug" header
	chatroomInputRows        = 3 // bordered textinput: 1 content + 2 border rows
	chatroomDetailChrome     = chatroomDetailHeaderRows + chatroomInputRows
)

// roomCardHeight is the number of terminal lines each room card occupies:
// top border + 2 content rows + bottom border = 4.
const roomCardHeight = 4

// roomSubscription holds the live RTDB channel and its cancellation function.
type roomSubscription struct {
	RoomID string
	C      <-chan model.Message
	cancel context.CancelFunc
}

// CIRC SSE subscription message types — unexported, handled within ChatroomsModel.
type roomSubscribedMsg struct {
	roomID string
	sub    *roomSubscription
}
type roomReceivedMsg struct{ msg model.Message }
type roomStreamClosedMsg struct{ roomID string }
type roomReconnectedMsg struct{ sub *roomSubscription }
type roomReconnectFailedMsg struct {
	roomID  string
	attempt int
	err     error
}
type roomReconnectRetryDueMsg struct {
	roomID  string
	attempt int
}
type circMsgsLoadedMsg struct {
	roomID string
	msgs   []model.Message
}
type circOlderMsgsLoadedMsg struct {
	roomID string
	msgs   []model.Message
}
type circErrMsg struct{ err error }

// RoomReconnectedMsg is emitted after the live RTDB stream is successfully
// re-established following an idToken expiry. App uses it to show a toast.
type RoomReconnectedMsg struct{}

// waitForRoomMsg blocks on the subscription channel and returns the next message as a tea.Cmd.
func waitForRoomMsg(sub *roomSubscription) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-sub.C
		if !ok {
			return roomStreamClosedMsg{roomID: sub.RoomID}
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

	mode            chatroomMode
	selectedRoom    int // index into rooms
	activeRoomID    string
	pendingRoomSlug string // set by SetPendingRoomSlug; consumed by OpenPendingRoom once rooms (re)load
	sub          *roomSubscription
	currentUser  string
	client       api.Client
	err          error // last message-load/subscribe failure for the active room; cleared on success

	// Reconnect-retry state, active only between a stream closing and either
	// a successful reconnect or exhausting maxReconnectAttempts.
	reconnectAttempt int
	reconnecting     bool
	reconnectFailed  bool
	reconnectCtx     context.Context
	reconnectCancel  context.CancelFunc

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

// cancelRoomSub stops any active RTDB subscription and any in-flight
// reconnect retry sequence, and clears subscription state.
func (m ChatroomsModel) cancelRoomSub() ChatroomsModel {
	if m.sub != nil {
		m.sub.cancel()
		m.sub = nil
	}
	if m.reconnectCancel != nil {
		m.reconnectCancel()
		m.reconnectCancel = nil
	}
	m.reconnecting = false
	m.reconnectFailed = false
	m.reconnectAttempt = 0
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
		return roomSubscribedMsg{roomID: roomID, sub: &roomSubscription{RoomID: roomID, C: ch, cancel: cancel}}
	}
}

// reconnectRoomCmd makes one reconnect attempt for roomID after the live
// stream closed (idToken expiry, idle-timeout, or a network error) — refreshes
// the session token and reopens the RTDB subscription. On failure it reports
// which attempt number just failed so Update can decide whether to back off
// and retry or give up.
func (m ChatroomsModel) reconnectRoomCmd(ctx context.Context, roomID string, attempt int) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		if client == nil {
			return nil
		}
		ch, cancel, err := attemptReconnect(client, ctx, func(ctx context.Context) (<-chan model.Message, context.CancelFunc, error) {
			return client.SubscribeRoom(ctx, roomID)
		})
		if err != nil {
			return roomReconnectFailedMsg{roomID: roomID, attempt: attempt, err: err}
		}
		return roomReconnectedMsg{sub: &roomSubscription{RoomID: roomID, C: ch, cancel: cancel}}
	}
}

// scheduleRoomReconnectRetryCmd waits out the backoff for attempt, then emits
// a roomReconnectRetryDueMsg to trigger the next reconnect attempt.
func scheduleRoomReconnectRetryCmd(roomID string, attempt int) tea.Cmd {
	return tea.Tick(reconnectDelay(attempt), func(time.Time) tea.Msg {
		return roomReconnectRetryDueMsg{roomID: roomID, attempt: attempt}
	})
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

// GetFocusedURLs returns URLs found across all currently loaded messages in
// the open room, for the 'o' / ctrl+o open-link shortcut. Reachable via
// ctrl+o even while the compose input is focused, which it always is in
// detail mode (there's no separate browsing vs. composing sub-mode here).
func (m ChatroomsModel) GetFocusedURLs() []string {
	if m.mode != chatroomModeDetail {
		return nil
	}
	var urls []string
	for _, msg := range m.messages {
		urls = append(urls, extractURLs(msg.Body)...)
	}
	return dedupeURLs(urls)
}

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

// SetPendingRoomSlug records a room to auto-open once the room list next
// loads (used for chat_mention notification navigation via OpenRoomMsg).
func (m ChatroomsModel) SetPendingRoomSlug(slug string) ChatroomsModel {
	m.pendingRoomSlug = slug
	return m
}

// OpenPendingRoom auto-enters detail mode for the slug previously set via
// SetPendingRoomSlug, once the containing room list has (re)loaded. No-op if
// no slug is pending or none of the loaded rooms match. The pending slug is
// always cleared so a stale/unmatched slug can't reactivate on a later,
// unrelated room-list reload (e.g. the user manually revisits cIRC afterward).
func (m ChatroomsModel) OpenPendingRoom() (ChatroomsModel, tea.Cmd) {
	slug := m.pendingRoomSlug
	m.pendingRoomSlug = ""
	if slug == "" {
		return m, nil
	}
	for i, room := range m.rooms {
		if room.Slug == slug {
			return m.enterRoomDetail(i, room)
		}
	}
	return m, nil
}

// enterRoomDetail switches into detail mode for room (at list index idx),
// cancelling any existing subscription and kicking off history load + live
// subscribe. Shared by the list "enter" keybinding and OpenPendingRoom
// (chat_mention notification navigation).
func (m ChatroomsModel) enterRoomDetail(idx int, room model.Room) (ChatroomsModel, tea.Cmd) {
	m.selectedRoom = idx
	m = m.cancelRoomSub()
	m.activeRoomID = room.Slug
	m.activeRoom = &room
	m.messages = nil
	m.mode = chatroomModeDetail
	m.historyExhausted = false
	m.loadingHistory = false
	m.err = nil
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

// AppendMessage adds a live incoming message to the currently open room.
func (m ChatroomsModel) AppendMessage(msg model.Message) ChatroomsModel {
	m.messages = append(m.messages, msg)
	m.err = nil
	if m.ready {
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
	}
	return m
}

// AppendSystemMessage adds a local-only notice (e.g. a /help reply) to the
// currently open room. Never sent to the server. No-op if roomID doesn't
// match the active room.
func (m ChatroomsModel) AppendSystemMessage(roomID, text string) ChatroomsModel {
	if m.activeRoom == nil || m.activeRoom.Slug != roomID {
		return m
	}
	return m.AppendMessage(model.Message{
		From:      model.User{Username: "system"},
		Body:      text,
		CreatedAt: time.Now(),
		IsSystem:  true,
	})
}

// SetMessages replaces the message history for the active room.
func (m ChatroomsModel) SetMessages(roomID string, msgs []model.Message) ChatroomsModel {
	if m.activeRoom == nil || m.activeRoom.Slug != roomID {
		return m
	}
	m.messages = msgs
	m.err = nil
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
	m.err = nil
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
		if msg.roomID != m.activeRoomID {
			return m, nil // stale event from an already-abandoned subscription
		}
		m.sub = nil
		if m.mode != chatroomModeDetail || m.activeRoom == nil {
			return m, nil
		}
		if m.reconnectCancel != nil {
			m.reconnectCancel()
		}
		m.reconnectCtx, m.reconnectCancel = context.WithCancel(context.Background())
		m.reconnecting = true
		m.reconnectFailed = false
		m.reconnectAttempt = 0
		return m, m.reconnectRoomCmd(m.reconnectCtx, m.activeRoom.Slug, 0)

	case roomReconnectFailedMsg:
		if msg.roomID != m.activeRoomID || !m.reconnecting {
			return m, nil // stale or cancelled sequence
		}
		next := msg.attempt + 1
		if next >= maxReconnectAttempts {
			m.reconnecting = false
			m.reconnectFailed = true
			return m, nil
		}
		m.reconnectAttempt = next
		return m, scheduleRoomReconnectRetryCmd(msg.roomID, next)

	case roomReconnectRetryDueMsg:
		if msg.roomID != m.activeRoomID || !m.reconnecting {
			return m, nil // stale or cancelled sequence
		}
		return m, m.reconnectRoomCmd(m.reconnectCtx, msg.roomID, msg.attempt)

	case roomReconnectedMsg:
		if msg.sub.RoomID != m.activeRoomID {
			msg.sub.cancel()
			return m, nil
		}
		m.sub = msg.sub
		m.reconnecting = false
		m.reconnectFailed = false
		m.reconnectAttempt = 0
		return m, tea.Batch(waitForRoomMsg(m.sub), func() tea.Msg { return RoomReconnectedMsg{} })

	case circErrMsg:
		m.err = msg.err
		m.loadingHistory = false
		if m.ready && m.mode == chatroomModeDetail {
			m.viewport.SetContent(m.renderMessages())
		}
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
					return m.enterRoomDetail(m.selectedRoom, m.rooms[m.selectedRoom])
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
		if m.err != nil {
			return theme.Subtle.Render("couldn't load messages")
		}
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
		slug := ""
		if m.activeRoom != nil {
			name = m.activeRoom.Name
			slug = m.activeRoom.Slug
		}
		header := theme.Title.Render(name) + "  " + theme.Subtle.Render("#"+slug)
		if m.loadingHistory {
			header += theme.Subtle.Render("  (loading history…)")
		}
		switch {
		case m.reconnecting:
			header += theme.Highlight.Render(fmt.Sprintf("  (live updates lost, reconnecting… %d/%d)", m.reconnectAttempt+1, maxReconnectAttempts))
		case m.reconnectFailed:
			header += theme.Error.Render("  (live updates lost)")
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
