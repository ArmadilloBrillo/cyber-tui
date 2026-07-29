package screens

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

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
	chatroomDetailHeaderRows = 2 // "Room Name  #slug  ·  N online" header + divider rule
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

// roomPresenceSubscription holds the live presence RTDB channel and its
// cancellation function. Each receive is a full, filtered snapshot of who's
// currently online, not a single incremental event (see SubscribeRoomPresence).
type roomPresenceSubscription struct {
	RoomID string
	C      <-chan []model.RoomUser
	cancel context.CancelFunc
}

// mentionCycle tracks an in-progress @-mention Tab-completion: repeated Tab
// presses replace the same span with the next candidate, classic
// shell-completion style. Cleared by any key other than Tab (see the
// chatroomModeDetail key switch), which locks in whatever text is currently
// there.
type mentionCycle struct {
	atPos   int              // rune index of '@' in the input value
	end     int              // rune index right after the currently-inserted name
	matches []model.RoomUser // snapshot taken at cycle start, stable through the session
	index   int
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

// Presence message types — unexported, handled entirely within ChatroomsModel.
// Mirror the message-stream lifecycle types above.
type roomPresenceAnnouncedMsg struct {
	roomID       string
	heartbeatMs  int
	staleAfterMs int
}
type roomHeartbeatTickMsg struct{ roomID string }
type roomUsersLoadedMsg struct {
	roomID string
	users  []model.RoomUser
}
type roomPresenceSubscribedMsg struct {
	roomID string
	sub    *roomPresenceSubscription
}
type roomPresenceReceivedMsg struct{ users []model.RoomUser }
type roomPresenceStreamClosedMsg struct{ roomID string }

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

// waitForRoomPresenceMsg blocks on the presence subscription channel and
// returns the next snapshot as a tea.Cmd.
func waitForRoomPresenceMsg(sub *roomPresenceSubscription) tea.Cmd {
	return func() tea.Msg {
		users, ok := <-sub.C
		if !ok {
			return roomPresenceStreamClosedMsg{roomID: sub.RoomID}
		}
		return roomPresenceReceivedMsg{users: users}
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

	// canGoBack is true when the active room was opened via a deep link
	// (e.g. a chat_mention notification) rather than by switching to this
	// tab normally. When true, ESC in detail mode leaves Chatrooms
	// entirely instead of dropping to the room list. Reset to false by
	// activateScreen whenever Chatrooms is entered through ordinary tab
	// navigation.
	canGoBack bool

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

	// Presence state for the active room: who's online (side panel content),
	// the live presence stream, and the heartbeat cadence read from the
	// initial POST .../presence response (never hard-coded).
	roomUsers    []model.RoomUser
	presenceSub  *roomPresenceSubscription
	heartbeatMs  int
	staleAfterMs int

	// mentionCycle tracks an in-progress Tab-completion of an @-mention; nil
	// when not cycling. See mentionCycle's doc comment.
	mentionCycle *mentionCycle
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

// cancelRoomSub stops any active RTDB subscription (message and presence)
// and any in-flight reconnect retry sequence, and clears subscription state.
// A stray roomHeartbeatTickMsg firing after this just no-ops (guarded by
// roomID == m.activeRoomID, which is now "").
func (m ChatroomsModel) cancelRoomSub() ChatroomsModel {
	if m.sub != nil {
		m.sub.cancel()
		m.sub = nil
	}
	if m.presenceSub != nil {
		m.presenceSub.cancel()
		m.presenceSub = nil
	}
	if m.reconnectCancel != nil {
		m.reconnectCancel()
		m.reconnectCancel = nil
	}
	m.reconnecting = false
	m.reconnectFailed = false
	m.reconnectAttempt = 0
	m.activeRoomID = ""
	m.roomUsers = nil
	m.heartbeatMs = 0
	m.staleAfterMs = 0
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

// announcePresenceCmd announces the caller's presence in roomID, kicking off
// the heartbeat/users-load/presence-subscribe sequence once it returns.
func (m ChatroomsModel) announcePresenceCmd(roomID string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		if client == nil {
			return nil
		}
		heartbeatMs, staleAfterMs, err := client.AnnouncePresence(roomID)
		if err != nil {
			return nil // presence is supplementary; a failed announce just means no panel data
		}
		return roomPresenceAnnouncedMsg{roomID: roomID, heartbeatMs: heartbeatMs, staleAfterMs: staleAfterMs}
	}
}

// sendHeartbeatCmd re-announces presence for roomID as a best-effort
// background heartbeat; the result is discarded (failures fail silently,
// matching presence's supplementary status).
func (m ChatroomsModel) sendHeartbeatCmd(roomID string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		if client != nil {
			_, _, _ = client.AnnouncePresence(roomID)
		}
		return nil
	}
}

// scheduleHeartbeatCmd self-reschedules a heartbeat tick every heartbeatMs,
// the same self-rescheduling tea.Tick shape used by app.go's schedulePollCmd.
func scheduleHeartbeatCmd(roomID string, heartbeatMs int) tea.Cmd {
	return tea.Tick(time.Duration(heartbeatMs)*time.Millisecond, func(time.Time) tea.Msg {
		return roomHeartbeatTickMsg{roomID: roomID}
	})
}

func (m ChatroomsModel) loadRoomUsersCmd(roomID string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		if client == nil {
			return nil
		}
		users, err := client.GetRoomUsers(roomID)
		if err != nil {
			return nil // supplementary; the presence stream's next snapshot will fill in
		}
		return roomUsersLoadedMsg{roomID: roomID, users: users}
	}
}

// leaveRoomPresenceCmd sends a best-effort explicit leave; the result is
// discarded (the room's user list corrects itself via staleAfterMs expiry
// even if this fails or is never sent).
func leaveRoomPresenceCmd(client api.Client, roomID string) tea.Cmd {
	return func() tea.Msg {
		if client != nil {
			_ = client.LeaveRoomPresence(roomID)
		}
		return nil
	}
}

func (m ChatroomsModel) openRoomPresenceSubscriptionCmd(roomID string, staleAfterMs int) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		if client == nil {
			return nil
		}
		ch, cancel, err := client.SubscribeRoomPresence(context.Background(), roomID, staleAfterMs)
		if err != nil {
			return nil
		}
		return roomPresenceSubscribedMsg{roomID: roomID, sub: &roomPresenceSubscription{RoomID: roomID, C: ch, cancel: cancel}}
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

// ActiveRoomSlug returns the slug of the room currently shown in detail view,
// or "" if no room is open.
func (m ChatroomsModel) ActiveRoomSlug() string {
	if m.mode != chatroomModeDetail || m.activeRoom == nil {
		return ""
	}
	return m.activeRoom.Slug
}

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

// SetCanGoBack marks whether the active room was reached via a deep link,
// which determines whether ESC in detail mode leaves Chatrooms entirely
// (see canGoBack field doc).
func (m ChatroomsModel) SetCanGoBack(v bool) ChatroomsModel {
	m.canGoBack = v
	return m
}

// ResetToList clears any deep-link flag and drops back to the room list, for
// ordinary tab navigation into Chatrooms that may still have a deep-linked
// room open (SetCanGoBack(false) alone left mode stuck on detail).
func (m ChatroomsModel) ResetToList() ChatroomsModel {
	m.canGoBack = false
	m.mode = chatroomModeList
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
		m.announcePresenceCmd(room.Slug),
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

// SetRoomUsers replaces the presence panel's content, sorted admins-first
// then alphabetical within each block. Recomputes the message viewport width
// since the panel's presence affects it (panelWidths collapses to zero width
// until the first snapshot arrives).
func (m ChatroomsModel) SetRoomUsers(users []model.RoomUser) ChatroomsModel {
	m.roomUsers = sortRoomUsers(users)
	if m.ready {
		msgW, _ := m.panelWidths()
		m.viewport.Width = msgW
		m.viewport.SetContent(m.renderMessages())
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
		msgW, _ := m.panelWidths()
		if !m.ready {
			m.listVP = viewport.New(msg.Width, listH)
			m.viewport = viewport.New(msgW, detailH)
			m.listVP.SetContent(m.renderRoomCards())
			m.ready = true
		} else {
			m.listVP.Width = msg.Width
			m.listVP.Height = listH
			m.viewport.Width = msgW
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

	// --- Presence lifecycle ---

	case roomPresenceAnnouncedMsg:
		if msg.roomID != m.activeRoomID {
			return m, nil // stale — left the room before this returned
		}
		m.heartbeatMs = msg.heartbeatMs
		m.staleAfterMs = msg.staleAfterMs
		return m, tea.Batch(
			scheduleHeartbeatCmd(msg.roomID, msg.heartbeatMs),
			m.loadRoomUsersCmd(msg.roomID),
			m.openRoomPresenceSubscriptionCmd(msg.roomID, msg.staleAfterMs),
		)

	case roomHeartbeatTickMsg:
		if msg.roomID != m.activeRoomID {
			return m, nil // left the room; let the tick chain die out
		}
		return m, tea.Batch(
			m.sendHeartbeatCmd(msg.roomID),
			scheduleHeartbeatCmd(msg.roomID, m.heartbeatMs),
		)

	case roomUsersLoadedMsg:
		if msg.roomID != m.activeRoomID {
			return m, nil
		}
		return m.SetRoomUsers(msg.users), nil

	case roomPresenceSubscribedMsg:
		if msg.roomID != m.activeRoomID {
			msg.sub.cancel()
			return m, nil
		}
		m.presenceSub = msg.sub
		return m, waitForRoomPresenceMsg(m.presenceSub)

	case roomPresenceReceivedMsg:
		m = m.SetRoomUsers(msg.users)
		if m.presenceSub != nil {
			return m, waitForRoomPresenceMsg(m.presenceSub)
		}
		return m, nil

	case roomPresenceStreamClosedMsg:
		if msg.roomID != m.activeRoomID {
			return m, nil // stale event from an already-abandoned subscription
		}
		m.presenceSub = nil
		if m.mode != chatroomModeDetail || m.activeRoom == nil {
			return m, nil
		}
		// ponytail: simplest reconnect for a supplementary stream — immediate
		// retry, no backoff/attempt cap, no UI indicator. Share the message
		// stream's exponential-backoff state machine (reconnect.go) if this
		// thrashes in practice.
		return m, m.openRoomPresenceSubscriptionCmd(msg.roomID, m.staleAfterMs)

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
			// Any key other than Tab ends an in-progress mention cycle and
			// locks in whatever text is currently there — matches
			// shell-completion convention.
			if msg.String() != "tab" {
				m.mentionCycle = nil
			}
			switch msg.String() {
			case "tab":
				if c := m.mentionCycle; c != nil && m.input.Position() == c.end {
					next := (c.index + 1) % len(c.matches)
					newValue, newEnd := spliceMention(m.input.Value(), c.atPos, c.end, c.matches[next].Username)
					m.input.SetValue(newValue)
					m.input.SetCursor(newEnd)
					m.mentionCycle = &mentionCycle{atPos: c.atPos, end: newEnd, matches: c.matches, index: next}
					return m, nil
				}
				if query, atPos, ok := mentionQueryAt(m.input.Value(), m.input.Position()); ok {
					if matches := matchMentionCandidates(m.roomUsers, query); len(matches) > 0 {
						newValue, newEnd := spliceMention(m.input.Value(), atPos, m.input.Position(), matches[0].Username)
						m.input.SetValue(newValue)
						m.input.SetCursor(newEnd)
						m.mentionCycle = &mentionCycle{atPos: atPos, end: newEnd, matches: matches, index: 0}
					}
				}
				return m, nil
			case "esc":
				var leaveRoomID string
				if m.activeRoom != nil {
					leaveRoomID = m.activeRoom.Slug
				}
				if m.canGoBack {
					m = m.cancelRoomSub()
					m.canGoBack = false
					return m, tea.Batch(
						leaveRoomPresenceCmd(m.client, leaveRoomID),
						func() tea.Msg { return LeaveChatroomsMsg{} },
					)
				}
				m = m.cancelRoomSub()
				m.mode = chatroomModeList
				m.activeRoom = nil
				m.messages = nil
				m.input.Blur()
				if m.ready {
					m.listVP.SetContent(m.renderRoomCards())
				}
				return m, leaveRoomPresenceCmd(m.client, leaveRoomID)
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

		slugLine := theme.Subtle.Render(fmt.Sprintf("#%s · %d online", r.Slug, r.OnlineCount))
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
	return renderCircMessages(m.messages, m.location(), m.timeDisplayFormat, m.viewport.Width, m.currentUser)
}

// Panel sizing: the users panel is pinned to a preferred width that
// comfortably fits an admin marker plus the longest possible username (20
// chars, the API's max); below roomUsersPanelMinMsgWidth for the message
// viewport, the panel collapses entirely rather than shrinking below its
// preferred width — an in-between size could be narrower than the worst-case
// content, and lipgloss/cellbuf.Wrap hard-breaks a too-long unbroken word
// (a username has no spaces to break on) instead of truncating it.
const (
	roomUsersPanelPreferredWidth = 24 // admin marker(2) + username(20) + padding(2)
	roomUsersPanelMinMsgWidth    = 40 // message viewport never shrinks below this
	roomUsersPanelSep            = 1  // vertical separator column
)

// panelWidths returns the message-viewport and users-panel widths for the
// screen's current width. The panel is either at its full preferred width or
// fully collapsed (full-width messages) — never an in-between size — until
// the first presence snapshot arrives, or on a narrow terminal.
func (m ChatroomsModel) panelWidths() (msgW, usersW int) {
	if len(m.roomUsers) == 0 {
		return m.width, 0
	}
	if m.width-roomUsersPanelPreferredWidth-roomUsersPanelSep < roomUsersPanelMinMsgWidth {
		return m.width, 0
	}
	return m.width - roomUsersPanelPreferredWidth - roomUsersPanelSep, roomUsersPanelPreferredWidth
}

// mentionQueryAt returns the in-progress @-mention text and the rune index
// of the '@' if cursor sits inside one — an '@' at the start of the input or
// preceded by whitespace, followed by a run of non-whitespace characters up
// to cursor. ok is false otherwise (including mid-word cases like
// "user@host", where the '@' isn't preceded by whitespace/start-of-input).
func mentionQueryAt(value string, cursor int) (query string, atPos int, ok bool) {
	runes := []rune(value)
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(runes) {
		cursor = len(runes)
	}
	i := cursor - 1
	for i >= 0 && runes[i] != '@' && !unicode.IsSpace(runes[i]) {
		i--
	}
	if i < 0 || runes[i] != '@' {
		return "", 0, false
	}
	if i > 0 && !unicode.IsSpace(runes[i-1]) {
		return "", 0, false
	}
	return string(runes[i+1 : cursor]), i, true
}

// matchMentionCandidates returns the online users whose username starts with
// query (case-insensitive), in the same admins-first/alphabetical order
// SetRoomUsers already sorts m.roomUsers into.
func matchMentionCandidates(users []model.RoomUser, query string) []model.RoomUser {
	q := strings.ToLower(query)
	var out []model.RoomUser
	for _, u := range users {
		if strings.HasPrefix(strings.ToLower(u.Username), q) {
			out = append(out, u)
		}
	}
	return out
}

// spliceMention replaces value's [atPos:cursor) rune span with "@username"
// and returns the cursor position right after the inserted name — no
// trailing space, so a repeated Tab press can keep replacing the same span
// to cycle to the next candidate.
func spliceMention(value string, atPos, cursor int, username string) (newValue string, newCursor int) {
	runes := []rune(value)
	if cursor > len(runes) {
		cursor = len(runes)
	}
	if atPos > cursor {
		atPos = cursor
	}
	replacement := []rune("@" + username)
	out := make([]rune, 0, len(runes)-(cursor-atPos)+len(replacement))
	out = append(out, runes[:atPos]...)
	out = append(out, replacement...)
	out = append(out, runes[cursor:]...)
	return string(out), atPos + len(replacement)
}

// mentionGhostText returns the dim, uncommitted remainder of the top
// mention candidate for display right after the cursor — only when the
// mention token reaches the very end of the input. That's the only place
// ghost text can render unambiguously: with trailing text after the token
// ("hey @al there"), there's no sensible place to draw the preview without
// overlapping what's already typed. Tab-cycling still works identically in
// that case — it just won't show this particular hint.
func (m ChatroomsModel) mentionGhostText() string {
	value := []rune(m.input.Value())
	cursor := m.input.Position()
	if cursor != len(value) {
		return ""
	}
	query, _, ok := mentionQueryAt(m.input.Value(), cursor)
	if !ok {
		return ""
	}
	matches := matchMentionCandidates(m.roomUsers, query)
	if len(matches) == 0 {
		return ""
	}
	top := []rune(matches[0].Username)
	q := []rune(query)
	if len(top) <= len(q) {
		return "" // already fully typed/matched — nothing left to preview
	}
	return theme.Subtle.Render(string(top[len(q):]))
}

// sortRoomUsers returns a copy of users ordered admins-first, then
// alphabetically (case-insensitive) within each block.
func sortRoomUsers(users []model.RoomUser) []model.RoomUser {
	out := append([]model.RoomUser(nil), users...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].IsChatAdmin != out[j].IsChatAdmin {
			return out[i].IsChatAdmin
		}
		return strings.ToLower(out[i].Username) < strings.ToLower(out[j].Username)
	})
	return out
}

// renderRoomUsersPanel renders the side panel's content — assumed already
// sorted by sortRoomUsers. The admin marker always stays theme.Highlight (an
// admin signal, independent of who's viewing); the username text itself uses
// theme.MeHighlight for the viewer's own name — same substitution
// renderCircMessages makes for the message list (render.go) — theme.Highlight
// for another admin, or unstyled otherwise.
func renderRoomUsersPanel(users []model.RoomUser, currentUser string) string {
	if len(users) == 0 {
		return theme.Subtle.Render("no one else is here")
	}
	rows := make([]string, len(users))
	for i, u := range users {
		name := u.Username
		switch {
		case currentUser != "" && u.Username == currentUser:
			name = theme.MeHighlight.Render(name)
		case u.IsChatAdmin:
			name = theme.Highlight.Render(name)
		}
		if u.IsChatAdmin {
			rows[i] = theme.Highlight.Render("★ ") + name
		} else {
			rows[i] = name
		}
	}
	return strings.Join(rows, "\n")
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
		header := theme.Title.Render(name) + "  " + theme.Subtle.Render("#"+slug) +
			theme.Subtle.Render(fmt.Sprintf("  ·  %d online", len(m.roomUsers)))
		if m.loadingHistory {
			header += theme.Subtle.Render("  (loading history…)")
		}
		switch {
		case m.reconnecting:
			header += theme.Highlight.Render(fmt.Sprintf("  (live updates lost, reconnecting… %d/%d)", m.reconnectAttempt+1, maxReconnectAttempts))
		case m.reconnectFailed:
			header += theme.Error.Render("  (live updates lost)")
		}
		inputBox := theme.ActiveBorder.Render(m.input.View() + m.mentionGhostText())

		_, usersW := m.panelWidths()
		messageArea := m.viewport.View()
		if usersW > 0 {
			panel := lipgloss.NewStyle().Width(usersW).Height(m.viewport.Height).MaxHeight(m.viewport.Height).
				Render(renderRoomUsersPanel(m.roomUsers, m.currentUser))
			sep := theme.Subtle.Render(strings.TrimSuffix(strings.Repeat("│\n", m.viewport.Height), "\n"))
			messageArea = lipgloss.JoinHorizontal(lipgloss.Top, messageArea, sep, panel)
		}
		divider := theme.Subtle.Render(strings.Repeat("─", max(m.width, 0)))
		return lipgloss.JoinVertical(lipgloss.Left, header, divider, messageArea, inputBox)
	default: // chatroomModeList
		if !m.ready {
			return theme.Subtle.Render("loading rooms…")
		}
		return m.listVP.View()
	}
}
