package screens

import (
	"context"
	"fmt"
	"slices"
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

// chatMessageBufferMaxBytes bounds the live message history kept for the
// active room, evicting oldest-first once exceeded — mirrors the byte cap
// used for inlineImageCache (app.go). Prevents unbounded growth from an
// active room over a long-running session.
const chatMessageBufferMaxBytes = 2 << 20 // 2 MiB

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

// mentionCycle tracks an in-progress @-mention preview: repeated Tab presses
// change which candidate is shown as ghost text, without touching m.input at
// all — text and cursor stay exactly as typed. Typing a space commits the
// currently-previewed candidate as real text; any other key just clears the
// preview, leaving whatever was actually typed untouched (see the
// chatroomModeDetail key switch).
type mentionCycle struct {
	atPos    int // rune index of '@' in the input value
	queryEnd int // rune index where the typed query ends — equals
	// the cursor for as long as the preview is active; text/cursor never
	// move during a cycle, only via a space-commit or by clearing it
	index int // Tab-press count since the cycle started; wrapped against
	// matchMentionCandidates' live result at use time (mentionActiveCandidate)
	// rather than stored, so someone joining or leaving the room mid-cycle is
	// reflected on the very next Tab press instead of needing a fresh cycle
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
	idleAfterMs  int
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
type roomPresenceReceivedMsg struct {
	sub   *roomPresenceSubscription // identifies which subscription produced this, so a stale/orphaned one can't clobber the active list
	users []model.RoomUser
}
type roomPresenceStreamClosedMsg struct{ roomID string }
type roomPresenceReconnectedMsg struct{ sub *roomPresenceSubscription }
type roomPresenceReconnectFailedMsg struct {
	roomID  string
	attempt int
	err     error
}
type roomPresenceReconnectRetryDueMsg struct {
	roomID  string
	attempt int
}

// RoomReconnectedMsg is emitted after the live RTDB stream is successfully
// re-established following an idToken expiry. App uses it to show a toast.
type RoomReconnectedMsg struct{}

// IsRoomStreamMsg reports whether msg belongs to CIRC's message/presence
// subscription lifecycle (the unexported room*Msg/circ*Msg types above). App
// uses this to keep routing these messages to ChatroomsModel.Update even when
// Chatrooms isn't the active screen, so the self-rescheduling
// waitForRoomMsg/heartbeat/reconnect tea.Cmd chains for the room the user had
// open don't die just because they switched tabs.
func IsRoomStreamMsg(msg tea.Msg) bool {
	switch msg.(type) {
	case roomSubscribedMsg, roomReceivedMsg, roomStreamClosedMsg,
		roomReconnectFailedMsg, roomReconnectRetryDueMsg, roomReconnectedMsg,
		circMsgsLoadedMsg, circOlderMsgsLoadedMsg, circErrMsg,
		roomPresenceAnnouncedMsg, roomHeartbeatTickMsg, roomUsersLoadedMsg,
		roomPresenceSubscribedMsg, roomPresenceReceivedMsg, roomPresenceStreamClosedMsg,
		roomPresenceReconnectFailedMsg, roomPresenceReconnectRetryDueMsg, roomPresenceReconnectedMsg:
		return true
	default:
		return false
	}
}

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
		return roomPresenceReceivedMsg{sub: sub, users: users}
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

	mutedUsersByRoom map[string][]string // roomID -> muted usernames, from Settings

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
	idleAfterMs  int

	// Own idle tracking: lastActivityAt is reported to the server on every
	// heartbeat; lastHeartbeatSentAt is when we last actually told the
	// server anything, used to cooldown-guard the corrective heartbeat (see
	// selfShownIdle) against the presence-heartbeat rate limit.
	lastActivityAt      time.Time
	lastHeartbeatSentAt time.Time

	// Presence-stream reconnect-retry state, mirroring the message stream's
	// reconnect* fields above but tracked separately since the two streams
	// can each independently close and reconnect.
	presenceReconnectAttempt int
	presenceReconnecting     bool
	presenceReconnectFailed  bool
	presenceReconnectCtx     context.Context
	presenceReconnectCancel  context.CancelFunc

	// mentionCycle tracks an in-progress Tab-completion of an @-mention; nil
	// when not cycling. See mentionCycle's doc comment.
	mentionCycle *mentionCycle

	// Message selection ("browsing") + flag/report overlay state.
	// selectedMsgID == "" is the sentinel for normal typing (input focused,
	// up/down raw-scroll); non-empty means the input is blurred and up/down/!
	// act on the selected message instead. Selection is tracked by ID rather
	// than index since PrependMessages splices older history onto the front
	// of m.messages, which would silently invalidate a stored index.
	selectedMsgID       string
	msgOffsets          []int // start line of m.messages[i]'s rendered block; 1:1 with m.messages
	msgHeights          []int // rendered line-height of m.messages[i]'s block; 1:1 with m.messages
	msgImages           [][]postImageSlot // inline image slot(s) for m.messages[i], nil unless eligible; 1:1 with m.messages
	inlineImagesEnabled bool
	// imageRealRows caches each image slot's actual fetched/fitted row
	// count (App.recordInlineImageRealRows, keyed by circMsgImageKey), so
	// its reserved band can shrink from the fallback-max placeholder down
	// to its real size once known — see SetImageRealRows.
	imageRealRows map[string]int
	flagPrompt          FlagPrompt
	flagTargetMsgID     string // message ID being flagged, set right before flagPrompt.Open()
	confirmingDeleteMsg bool   // true while the y/n delete-confirm overlay for the selected message is showing

	// revealed tracks which spoiler- or l33t-styled messages (keyed by ID)
	// the user has toggled to their original text via the browsing-mode
	// reveal key (enter). Unset/false means still obscured (spoiler: masked;
	// l33t: substituted). Only cIRC has this today — see chatstyle.go's
	// maskSpoilerBody doc comment.
	revealed map[string]bool

	// styleAnimFrame/styleAnimRunning drive the slow/wave/glitch animated
	// styles: frame increments on every styleAnimTickMsg, and running guards
	// against stacking multiple concurrent tickers. Coarse-scoped — see the
	// plan's Trade-offs section: the ticker runs whenever any animated-style
	// message is loaded in the room, not only while one is visible on screen.
	styleAnimFrame   int
	styleAnimRunning bool

	// animPaused suppresses the refreshMessages() re-render that styleAnimTickMsg
	// would otherwise trigger, without stopping the ticker chain itself. Set by
	// app.go while the image modal is open: an animated line's re-render changes
	// that terminal row's bytes, forcing Bubble Tea to resend the whole row —
	// including the parts of it covered by the modal's box — which erases the
	// modal's graphics-protocol pixels there.
	animPaused bool

	// focused is true while the Chatrooms tab is the one on screen. The RTDB
	// subscription for an open room stays alive regardless (see
	// IsRoomStreamMsg), so this only gates whether incoming messages bump
	// unreadCount for the tab-bar badge.
	focused     bool
	unreadCount int
}

// SendRoomMessageMsg is emitted when the user sends a chatroom message.
type SendRoomMessageMsg struct {
	RoomID string
	Body   string
}

// baseSlashCommands are the slash commands the server recognizes for both
// CIRC and C-Mail. Checked client-side so a typo'd command shows a local
// error instead of being sent as a literal chat message. Command *syntax*
// validation stays server-side.
var baseSlashCommands = map[string]bool{
	"/me": true, "/poke": true, "/hug": true, "/hi5": true, "/slap": true,
	"/dice": true, "/8ball": true, "/fortune": true, "/help": true,
	"/gif": true, "/song": true,
}

// circOnlySlashCommands are additionally recognized in CIRC only — C-Mail's
// server rejects them (see docs/00-latest-api-reference.md).
var circOnlySlashCommands = map[string]bool{
	"/mute": true, "/unmute": true, "/muted": true, "/unmuteall": true, "/art": true,
}

// knownCircStyles are the text-style command names (see chatstyle.go) that
// can appear alone or chained with "+" (e.g. "/comic+rainbow"), so they're
// checked separately from the exact-match baseSlashCommands lookup. Shared
// by both CIRC and C-Mail — styles work identically in either.
var knownCircStyles = map[string]bool{
	styleBlink: true, styleL33t: true, styleComic: true, styleCursive: true,
	styleTimes: true, styleRainbow: true, styleFlip: true, styleQuiet: true,
	styleSlow: true, styleGlitch: true, styleSpoiler: true, styleWave: true,
}

// isKnownStyleCombo reports whether cmd is a "/"-prefixed, "+"-joined
// combination of known style names, e.g. "/comic+rainbow". "/spoiler" is a
// known style but cannot be chained with any other (server-enforced, see
// the "/help" command text), so a combo containing it alongside another
// style is rejected even though each individual part is valid.
func isKnownStyleCombo(cmd string) bool {
	parts := strings.Split(strings.TrimPrefix(cmd, "/"), "+")
	for _, p := range parts {
		if !knownCircStyles[p] {
			return false
		}
	}
	return len(parts) == 1 || !slices.Contains(parts, styleSpoiler)
}

// isKnownSlashCommand reports whether cmd (lowercased, "/"-prefixed) is a
// command the server accepts for the calling screen. extra carries any
// screen-specific commands on top of the common set (e.g. circOnlySlashCommands).
func isKnownSlashCommand(cmd string, extra map[string]bool) bool {
	return baseSlashCommands[cmd] || extra[cmd] || isKnownStyleCombo(cmd)
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
		flagPrompt:  NewFlagPrompt(),
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
	if m.presenceReconnectCancel != nil {
		m.presenceReconnectCancel()
		m.presenceReconnectCancel = nil
	}
	m.reconnecting = false
	m.reconnectFailed = false
	m.reconnectAttempt = 0
	m.presenceReconnecting = false
	m.presenceReconnectFailed = false
	m.presenceReconnectAttempt = 0
	m.activeRoomID = ""
	m.roomUsers = nil
	m.heartbeatMs = 0
	m.staleAfterMs = 0
	m.idleAfterMs = 0
	m.lastHeartbeatSentAt = time.Time{}
	return m
}

// CancelSubscription is called by App when the CIRC room the user had open is
// being torn down for real (leaving the room via ESC, or logout) — not on an
// ordinary tab switch, which now leaves the subscription running in the
// background (see SetFocused).
func (m ChatroomsModel) CancelSubscription() ChatroomsModel {
	return m.cancelRoomSub()
}

// SetFocused marks whether the Chatrooms tab is the one currently on screen.
// Becoming focused clears unreadCount for the tab-bar badge. It also clears
// styleAnimRunning: styleAnimTickMsg isn't in IsRoomStreamMsg, so a tick that
// fires while this tab is backgrounded is dropped before updateInner ever
// resets the flag, permanently blocking maybeStartStyleAnim from restarting
// the ticker. Regaining focus is the point where any prior ticker is known
// to be dead, so it's safe to clear the flag here.
func (m ChatroomsModel) SetFocused(focused bool) ChatroomsModel {
	m.focused = focused
	if focused {
		m.unreadCount = 0
		m.styleAnimRunning = false
	}
	return m
}

// UnreadCount returns the number of messages received in the open room since
// the tab was last focused, for the tab-bar badge.
func (m ChatroomsModel) UnreadCount() int { return m.unreadCount }

// HasLiveRoom reports whether a room is currently open in detail view — used
// by activateScreen to decide whether re-entering the tab should resume that
// room instead of dropping back to the room list.
func (m ChatroomsModel) HasLiveRoom() bool {
	return m.mode == chatroomModeDetail && m.activeRoom != nil
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
func (m ChatroomsModel) announcePresenceCmd(roomID string, lastActivity time.Time) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		if client == nil {
			return nil
		}
		heartbeatMs, staleAfterMs, idleAfterMs, err := client.AnnouncePresence(roomID, lastActivity)
		if err != nil {
			return nil // presence is supplementary; a failed announce just means no panel data
		}
		return roomPresenceAnnouncedMsg{roomID: roomID, heartbeatMs: heartbeatMs, staleAfterMs: staleAfterMs, idleAfterMs: idleAfterMs}
	}
}

// sendHeartbeatCmd re-announces presence for roomID as a best-effort
// background heartbeat; the result is discarded (failures fail silently,
// matching presence's supplementary status).
func (m ChatroomsModel) sendHeartbeatCmd(roomID string, lastActivity time.Time) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		if client != nil {
			_, _, _, _ = client.AnnouncePresence(roomID, lastActivity)
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
		ch, cancel, err := client.SubscribeRoomPresence(context.Background(), roomID, staleAfterMs, nil)
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

// reconnectRoomPresenceCmd makes one reconnect attempt for roomID's presence
// stream after it closed — refreshes the session token (shared with the
// message stream via attemptReconnect, so a token expiry the presence stream
// notices first doesn't retry against the same stale token) and reopens the
// presence subscription, seeded with the last known-good user list so the
// panel doesn't flash empty while the fresh snapshot is in flight.
func (m ChatroomsModel) reconnectRoomPresenceCmd(ctx context.Context, roomID string, staleAfterMs int, attempt int) tea.Cmd {
	client := m.client
	knownUsers := m.roomUsers
	return func() tea.Msg {
		if client == nil {
			return nil
		}
		ch, cancel, err := attemptReconnect(client, ctx, func(ctx context.Context) (<-chan []model.RoomUser, context.CancelFunc, error) {
			return client.SubscribeRoomPresence(ctx, roomID, staleAfterMs, knownUsers)
		})
		if err != nil {
			return roomPresenceReconnectFailedMsg{roomID: roomID, attempt: attempt, err: err}
		}
		return roomPresenceReconnectedMsg{sub: &roomPresenceSubscription{RoomID: roomID, C: ch, cancel: cancel}}
	}
}

// scheduleRoomPresenceReconnectRetryCmd waits out the backoff for attempt,
// then emits a roomPresenceReconnectRetryDueMsg to trigger the next attempt.
func scheduleRoomPresenceReconnectRetryCmd(roomID string, attempt int) tea.Cmd {
	return tea.Tick(reconnectDelay(attempt), func(time.Time) tea.Msg {
		return roomPresenceReconnectRetryDueMsg{roomID: roomID, attempt: attempt}
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

// SelectedMessageID returns the ID of the currently browsed/highlighted
// message, or "" if none is selected (normal typing state).
func (m ChatroomsModel) SelectedMessageID() string { return m.selectedMsgID }

// ComposeEmpty reports whether there's no text a bare left/right arrow would
// need to navigate — app.go uses this to decide whether bare left/right can
// escape to tab-cycling instead of moving the input cursor. False while the
// flag/report overlay is open, regardless of the compose box's own value:
// arrows must move within the reason field then, never escape to tabs.
func (m ChatroomsModel) ComposeEmpty() bool {
	return !m.flagPrompt.Active() && !m.confirmingDeleteMsg && m.input.Value() == ""
}

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

// GetFocusedURLs returns URLs found in the currently selected message while
// browsing, or across all currently loaded messages in the open room
// otherwise, for the 'o' / ctrl+o open-link shortcut. Reachable via ctrl+o
// even while the compose input is focused, which it always is in detail
// mode outside of browsing.
func (m ChatroomsModel) GetFocusedURLs() []string {
	if m.mode != chatroomModeDetail {
		return nil
	}
	if m.selectedMsgID != "" {
		if msg, ok := findMessageByID(m.messages, m.selectedMsgID); ok {
			return dedupeURLs(messageURLs(msg))
		}
		return nil
	}
	var urls []string
	for _, msg := range m.messages {
		urls = append(urls, messageURLs(msg)...)
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

// SetAnimPaused pauses (or resumes) the styleAnimTickMsg re-render — see the
// animPaused field doc comment. Called from app.go when the image modal
// opens/closes.
func (m ChatroomsModel) SetAnimPaused(paused bool) ChatroomsModel {
	m.animPaused = paused
	return m
}

// AnimPaused reports whether the styleAnimTickMsg re-render is currently
// paused (see the animPaused field doc comment).
func (m ChatroomsModel) AnimPaused() bool { return m.animPaused }

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
	m.selectedMsgID = ""
	m.input.Focus()
	m.lastActivityAt = time.Now()
	m.lastHeartbeatSentAt = time.Now()
	if m.ready {
		m = m.refreshMessages()
		m.viewport.GotoBottom()
	}
	roomID := room.Slug
	return m, tea.Batch(
		m.loadRoomMessagesCmd(room.Slug),
		m.openRoomSubscriptionCmd(room.Slug),
		m.announcePresenceCmd(room.Slug, m.lastActivityAt),
		func() tea.Msg { return RoomOpenedMsg{RoomID: roomID} },
	)
}

// AppendMessage adds a live incoming message to the currently open room.
// Only follows the view down to the new message if it was already at the
// bottom beforehand — never yanks a scrolled-up reader down to the newest
// message, regardless of who posted it or what it contains.
func (m ChatroomsModel) AppendMessage(msg model.Message) ChatroomsModel {
	m.messages = append(m.messages, msg)
	m = m.trimMessageBuffer()
	m.err = nil
	if m.ready {
		wasAtBottom := m.viewport.AtBottom()
		m = m.refreshMessages()
		if wasAtBottom {
			m.viewport.GotoBottom()
		}
	}
	return m
}

// estimatedMessageSize approximates a model.Message's memory footprint for
// chatMessageBufferMaxBytes accounting. Doesn't need to be exact, just
// proportional to actual size — messages carrying ASCII art (already
// base64-decoded into Body) or attachments can be far larger than a typical
// chat line, which a fixed message-count cap wouldn't account for.
func estimatedMessageSize(msg model.Message) int {
	const overhead = 128
	n := overhead + len(msg.ID) + len(msg.From.Username) + len(msg.Body) +
		len(msg.ImageUrl) + len(msg.GifUrl)
	for _, s := range msg.Style {
		n += len(s)
	}
	if msg.AudioAttachment != nil {
		n += 128
	}
	return n
}

// trimMessageBuffer evicts oldest messages from m.messages while the
// estimated total size exceeds chatMessageBufferMaxBytes, always keeping at
// least the most recent message. Clears m.selectedMsgID if the evicted range
// contained the current selection, and resets m.historyExhausted so a later
// scroll-to-top re-fetches the evicted range from the server instead of
// treating it as permanently gone — the pagination cursor is m.messages[0]
// (see maybeLoadOlderMessages), so this is always safe to re-trigger.
func (m ChatroomsModel) trimMessageBuffer() ChatroomsModel {
	total := 0
	for _, msg := range m.messages {
		total += estimatedMessageSize(msg)
	}
	i := 0
	for total > chatMessageBufferMaxBytes && i < len(m.messages)-1 {
		if m.messages[i].ID != "" && m.messages[i].ID == m.selectedMsgID {
			m.selectedMsgID = ""
		}
		total -= estimatedMessageSize(m.messages[i])
		i++
	}
	if i > 0 {
		m.messages = m.messages[i:]
		m.historyExhausted = false
	}
	return m
}

// ApplyMessageDeleted marks an existing message (by ID) as soft-deleted in
// place — the message stays in the list (per the API: "the message stays in
// the room so the conversation around it still reads"), just with its body
// replaced by a tombstone marker. No-op if the ID isn't currently loaded.
// Used both for the optimistic update after the caller's own delete succeeds
// and for a delete patch received from the live RTDB stream.
func (m ChatroomsModel) ApplyMessageDeleted(messageID string) ChatroomsModel {
	for i, msg := range m.messages {
		if msg.ID != messageID {
			continue
		}
		m.messages[i].Deleted = true
		m.messages[i].Body = "[DELETED]"
		if m.ready {
			m = m.refreshMessages()
		}
		return m
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
		m = m.refreshMessages()
		m.viewport.GotoBottom()
	}
	return m
}

// SetRoomUsers replaces the presence panel's content, sorted admins-first
// then alphabetical within each block. Recomputes the message viewport width
// since the panel's presence affects it (panelWidths collapses to zero width
// until the first snapshot arrives). The live presence stream fires this on
// every change and on its own 5s re-evaluation timer, so — same as
// SetImageRealRows — the reflow must never yank the view down, and must
// re-home to the bottom if it was already there, or every presence tick
// silently drifts the scroll position out from under the user.
func (m ChatroomsModel) SetRoomUsers(users []model.RoomUser) ChatroomsModel {
	m.roomUsers = sortRoomUsers(users)
	if m.ready {
		msgW, _ := m.panelWidths()
		if msgW != m.viewport.Width {
			wasAtBottom := m.viewport.AtBottom()
			m.viewport.Width = msgW
			m = m.refreshMessages()
			if wasAtBottom {
				m.viewport.GotoBottom()
			}
		}
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
		newLines := lipgloss.Height(m.renderMessages())
		m = m.refreshMessages()
		m.viewport.SetYOffset(oldOffset + newLines - oldLines)
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
			m = m.refreshMessages()
		}
	}
	return m
}

func (m ChatroomsModel) Init() tea.Cmd { return textinput.Blink }

// Update processes msg via updateInner, then (re)arms the style-animation
// ticker (see maybeStartStyleAnim) if a loaded message needs one, and records
// own-activity for idle tracking. Wrapping the whole switch this way — rather
// than threading a tea.Cmd return through refreshMessages and its many call
// sites, several of which are public mutators also called directly from
// app.go outside of Update (e.g. AppendSystemMessage/ApplyMessageDeleted) —
// keeps the coarse-scoped animation check (and the activity check below) to a
// single choke point instead of a cascading signature change.
//
// Any key handled while a room is open counts as activity: it resets
// lastActivityAt (reported on the next heartbeat). If the online-users panel
// would currently render our own entry as idle (selfShownIdle) — i.e. the
// server's last-recorded lastActivity for us is stale, regardless of how
// recently we've actually been typing — it also fires an immediate
// out-of-cycle heartbeat to correct it rather than waiting for the next
// scheduled beat, cooldown-guarded by lastHeartbeatSentAt so a fast typing
// burst can't spam the presence-heartbeat rate limit. An earlier version
// tracked "am I idle" as a separate local guess derived purely from
// lastActivityAt; that could disagree with what was actually shown (the
// guess stayed "active" as long as the user kept typing, even while the
// server's copy of lastActivity — only ever updated by a heartbeat — had
// gone stale), so the correction never fired and the badge got stuck. Tying
// the check directly to the rendered state removes that second, conflicting
// source of truth.
func (m ChatroomsModel) Update(msg tea.Msg) (ChatroomsModel, tea.Cmd) {
	m, cmd := m.updateInner(msg)
	var tickCmd tea.Cmd
	m, tickCmd = m.maybeStartStyleAnim()
	if tickCmd != nil {
		cmd = tea.Batch(cmd, tickCmd)
	}
	if _, ok := msg.(tea.KeyMsg); ok && m.mode == chatroomModeDetail && m.activeRoomID != "" {
		m.lastActivityAt = time.Now()
		if m.selfShownIdle() && time.Since(m.lastHeartbeatSentAt) > selfIdleCorrectionCooldown {
			m.lastHeartbeatSentAt = time.Now()
			cmd = tea.Batch(cmd, m.sendHeartbeatCmd(m.activeRoomID, m.lastActivityAt))
		}
	}
	// Clear the unread badge the moment the view catches up to the bottom
	// while focused — whatever caused it (Esc from browsing, paging past
	// the newest message, sticky-scroll re-homing itself, or a plain
	// scroll), not just re-focusing the tab. A single choke point instead
	// of hunting down every individual path that can land back at the
	// bottom.
	if m.focused && m.mode == chatroomModeDetail && m.unreadCount > 0 && m.viewport.AtBottom() {
		m.unreadCount = 0
	}
	return m, cmd
}

// selfIdleCorrectionCooldown bounds how often selfShownIdle's correction can
// fire, so a fast typing burst right after one correction (before the RTDB
// round-trip clears the panel's stale view of us) can't spam
// POST .../presence past its 15/min-per-room rate limit. Comfortably under
// that limit, comfortably faster than waiting for the next heartbeatMs tick.
const selfIdleCorrectionCooldown = 5 * time.Second

// selfShownIdle reports whether the online-users panel would currently
// render the caller's own entry as idle — the same formula
// renderRoomUsersPanel applies to every row, applied here to whichever entry
// in m.roomUsers matches m.currentUser. Grounding the correction check in
// this (what's actually displayed) rather than a separately-tracked idle
// guess is what keeps the two from disagreeing — see Update's doc comment.
func (m ChatroomsModel) selfShownIdle() bool {
	if m.idleAfterMs <= 0 || m.currentUser == "" {
		return false
	}
	for _, u := range m.roomUsers {
		if u.Username == m.currentUser {
			return u.LastActivity != nil && time.Since(*u.LastActivity) > time.Duration(m.idleAfterMs)*time.Millisecond
		}
	}
	return false
}

// maybeStartStyleAnim starts the slow/wave/glitch animation ticker if a
// currently *visible* message needs one and it isn't already running.
// Scoped to the viewport (via m.msgOffsets/m.msgHeights) rather than the
// full history — a coarse whole-room scan meant a single animated message
// anywhere in a long-lived room's history kept a 150ms full re-render
// running indefinitely, even after that message scrolled off-screen.
// Returns a nil tea.Cmd when no start is needed.
func (m ChatroomsModel) maybeStartStyleAnim() (ChatroomsModel, tea.Cmd) {
	if m.styleAnimRunning || m.mode != chatroomModeDetail {
		return m, nil
	}
	top, bottom := m.viewport.YOffset, m.viewport.YOffset+m.viewport.Height
	for i, msg := range m.messages {
		if i >= len(m.msgOffsets) || i >= len(m.msgHeights) {
			break
		}
		itemStart, itemEnd := m.msgOffsets[i], m.msgOffsets[i]+m.msgHeights[i]
		if itemEnd <= top || itemStart >= bottom {
			continue
		}
		if hasAnimatedStyle(msg.Style) {
			m.styleAnimRunning = true
			return m, styleAnimTickCmd()
		}
	}
	return m, nil
}

func (m ChatroomsModel) updateInner(msg tea.Msg) (ChatroomsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case styleAnimTickMsg:
		m.styleAnimFrame++
		m.styleAnimRunning = false
		if !m.animPaused {
			m = m.refreshMessages()
		}
		return m, nil

	case InsertIconMsg:
		if m.mode == chatroomModeDetail {
			m.input = insertAtCursor(m.input, msg.Icon)
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		listH := msg.Height - theme.ChromeHeight
		msgW, _ := m.panelWidths()
		if !m.ready {
			m.listVP = viewport.New(msg.Width, listH)
			m.viewport = viewport.New(msgW, m.viewportHeight())
			m.listVP.SetContent(m.renderRoomCards())
			m.ready = true
		} else {
			m.listVP.Width = msg.Width
			m.listVP.Height = listH
			m.viewport.Width = msgW
			m.viewport.Height = m.viewportHeight()
			m.listVP.SetContent(m.renderRoomCards())
			if m.activeRoom != nil {
				m = m.refreshMessages()
			}
		}
		// textinput.View() renders 3 columns wider than Width the instant
		// there's any typed content: it adds Prompt's width (never
		// subtracted from its own padding math) plus 1 more for the phantom
		// end-of-line cursor glyph — neither of which its *empty*
		// placeholder rendering adds. Compensating here keeps the box at a
		// constant intended width once you start typing (previously it
		// silently grew 3 columns wider than the header/divider above it,
		// pushing the right border off-screen on any terminal where those
		// 3 columns were the difference between fitting and not).
		m.input.Width = msg.Width - 4 - lipgloss.Width(m.input.Prompt) - 1

	case SharedConfigMsg:
		m.timeDisplayFormat = msg.Settings.TimeDisplayFormat
		m.mutedUsersByRoom = msg.Settings.MutedUsersByRoom
		m.inlineImagesEnabled = msg.InlineImagesEnabled
		m = m.SetLocation(msg.Loc)
		if m.mode == chatroomModeDetail && m.activeRoom != nil {
			m = m.refreshMessages()
			if m.selectedMsgID != "" {
				m = m.ensureSelectedMessageVisible()
			} else {
				m.viewport.GotoBottom()
			}
		}
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
		if msg.msg.Deleted {
			// A delete patch — from us in another session, or from another
			// user — carries only {ID, Deleted}; merge onto the existing
			// message rather than appending it as a new one.
			m = m.ApplyMessageDeleted(msg.msg.ID)
		} else {
			m = m.AppendMessage(msg.msg)
			// Also counts as unread while focused if the view isn't at the
			// bottom (scrolled up reading history) — AppendMessage only
			// followed the view down if it was already there, so this
			// reliably reflects "have I actually seen the newest message."
			if !m.focused || !m.viewport.AtBottom() {
				m.unreadCount++
			}
		}
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
			m = m.refreshMessages()
		}
		return m, nil

	// --- Presence lifecycle ---

	case roomPresenceAnnouncedMsg:
		if msg.roomID != m.activeRoomID {
			return m, nil // stale — left the room before this returned
		}
		m.heartbeatMs = msg.heartbeatMs
		m.staleAfterMs = msg.staleAfterMs
		m.idleAfterMs = msg.idleAfterMs
		return m, tea.Batch(
			scheduleHeartbeatCmd(msg.roomID, msg.heartbeatMs),
			m.loadRoomUsersCmd(msg.roomID),
			m.openRoomPresenceSubscriptionCmd(msg.roomID, msg.staleAfterMs),
		)

	case roomHeartbeatTickMsg:
		if msg.roomID != m.activeRoomID {
			return m, nil // left the room; let the tick chain die out
		}
		m.lastHeartbeatSentAt = time.Now()
		return m, tea.Batch(
			m.sendHeartbeatCmd(msg.roomID, m.lastActivityAt),
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
		// A quick leave/re-enter of the same room slug can leave a prior,
		// still-live subscription for this roomID orphaned; cancel it rather
		// than leaking its goroutine/SSE connection now that a fresh one has
		// taken over.
		if m.presenceSub != nil && m.presenceSub != msg.sub {
			m.presenceSub.cancel()
		}
		m.presenceSub = msg.sub
		return m, waitForRoomPresenceMsg(m.presenceSub)

	case roomPresenceReceivedMsg:
		if msg.sub != m.presenceSub {
			return m, nil // stale/orphaned subscription — don't let it clobber the active list
		}
		m = m.SetRoomUsers(msg.users)
		return m, waitForRoomPresenceMsg(m.presenceSub)

	case roomPresenceStreamClosedMsg:
		if msg.roomID != m.activeRoomID {
			return m, nil // stale event from an already-abandoned subscription
		}
		m.presenceSub = nil
		if m.mode != chatroomModeDetail || m.activeRoom == nil {
			return m, nil
		}
		if m.presenceReconnectCancel != nil {
			m.presenceReconnectCancel()
		}
		m.presenceReconnectCtx, m.presenceReconnectCancel = context.WithCancel(context.Background())
		m.presenceReconnecting = true
		m.presenceReconnectFailed = false
		m.presenceReconnectAttempt = 0
		return m, m.reconnectRoomPresenceCmd(m.presenceReconnectCtx, msg.roomID, m.staleAfterMs, 0)

	case roomPresenceReconnectFailedMsg:
		if msg.roomID != m.activeRoomID || !m.presenceReconnecting {
			return m, nil // stale or cancelled sequence
		}
		next := msg.attempt + 1
		if next >= maxReconnectAttempts {
			m.presenceReconnecting = false
			m.presenceReconnectFailed = true
			return m, nil
		}
		m.presenceReconnectAttempt = next
		return m, scheduleRoomPresenceReconnectRetryCmd(msg.roomID, next)

	case roomPresenceReconnectRetryDueMsg:
		if msg.roomID != m.activeRoomID || !m.presenceReconnecting {
			return m, nil // stale or cancelled sequence
		}
		return m, m.reconnectRoomPresenceCmd(m.presenceReconnectCtx, msg.roomID, m.staleAfterMs, msg.attempt)

	case roomPresenceReconnectedMsg:
		if msg.sub.RoomID != m.activeRoomID {
			msg.sub.cancel()
			return m, nil
		}
		if m.presenceSub != nil && m.presenceSub != msg.sub {
			m.presenceSub.cancel()
		}
		m.presenceSub = msg.sub
		m.presenceReconnecting = false
		m.presenceReconnectFailed = false
		m.presenceReconnectAttempt = 0
		return m, waitForRoomPresenceMsg(m.presenceSub)

	case FlagSubmitMsg:
		messageID := m.flagTargetMsgID
		m.flagTargetMsgID = ""
		roomID := ""
		if m.activeRoom != nil {
			roomID = m.activeRoom.Slug
		}
		if m.ready {
			m.viewport.Height = m.viewportHeight()
		}
		return m, func() tea.Msg {
			return FlagMessageMsg{RoomID: roomID, MessageID: messageID, Reason: msg.Reason}
		}

	case FlagCancelMsg:
		m.flagTargetMsgID = ""
		if m.ready {
			m.viewport.Height = m.viewportHeight()
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
			case "pgup":
				if m.selectedRoom > 0 {
					m.selectedRoom = max(0, m.selectedRoom-pageJumpItems)
					if m.ready {
						m.listVP.SetContent(m.renderRoomCards())
						m = m.ensureRoomVisible()
					}
				}
				return m, nil
			case "pgdown":
				if m.selectedRoom < len(m.rooms)-1 {
					m.selectedRoom = min(len(m.rooms)-1, m.selectedRoom+pageJumpItems)
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
			if m.flagPrompt.Active() {
				var cmd tea.Cmd
				m.flagPrompt, cmd = m.flagPrompt.Update(msg)
				return m, cmd
			}
			if m.selectedMsgID != "" {
				return m.updateBrowsingKey(msg)
			}
			// Any key other than Tab or Space ends an in-progress mention
			// preview: Tab cycles which candidate is shown, Space commits
			// the current one. Everything else just clears the preview,
			// leaving whatever was actually typed untouched — nothing is
			// ever auto-inserted without one of those two keys.
			if msg.String() != "tab" && msg.String() != " " {
				m.mentionCycle = nil
			}
			switch msg.String() {
			case "tab":
				if c := m.mentionCycle; c != nil && m.input.Position() == c.queryEnd {
					m.mentionCycle = &mentionCycle{atPos: c.atPos, queryEnd: c.queryEnd, index: c.index + 1}
					return m, nil
				}
				if atPos, cursor, _, _, ok := m.mentionActiveCandidate(); ok {
					// index 1, not 0: index 0 is already showing as the
					// implicit default preview before any Tab is pressed
					// (mentionActiveCandidate's no-cycle fallback) — start
					// at 1 so the first Tab itself is a visible change, not
					// a no-op. Wrapped against the live match count at use
					// time (mentionActiveCandidate), not here, so this is
					// still correct even if the list has exactly one match
					// by the time it's read back.
					m.mentionCycle = &mentionCycle{atPos: atPos, queryEnd: cursor, index: 1}
				}
				return m, nil
			case " ":
				// Commits whatever mentionActiveCandidate is currently
				// showing — the active Tab-cycle's pick, or its passive
				// default (the first match, shown before any Tab press) —
				// so Space always commits exactly what's ghost-previewed,
				// never disagreeing with what the user sees.
				if atPos, cursor, _, candidate, ok := m.mentionActiveCandidate(); ok {
					newValue, newCursor := spliceMention(m.input.Value(), atPos, cursor, candidate)
					runes := []rune(newValue)
					runes = append(runes[:newCursor], append([]rune{' '}, runes[newCursor:]...)...)
					m.input.SetValue(string(runes))
					m.input.SetCursor(newCursor + 1)
					m.mentionCycle = nil
					return m, nil
				}
				m.mentionCycle = nil
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
						roomID := m.activeRoom.Slug
						if strings.HasPrefix(val, "/") {
							cmd := strings.ToLower(strings.Fields(val)[0])
							if !isKnownSlashCommand(cmd, circOnlySlashCommands) {
								m.input.Reset()
								return m.AppendSystemMessage(roomID, "*** unknown command: "+cmd), nil
							}
						}
						m.input.Reset()
						return m, func() tea.Msg {
							return SendRoomMessageMsg{RoomID: roomID, Body: val}
						}
					}
				}
				return m, nil
			case "up":
				sel := selectableMessageIndices(m.messages, m.mutedUsers())
				if len(sel) == 0 {
					m.viewport.ScrollUp(1)
					return m.maybeLoadOlderMessages()
				}
				m.input.Blur()
				m.selectedMsgID = m.messages[sel[len(sel)-1]].ID
				m = m.refreshMessages()
				m = m.ensureSelectedMessageVisible()
				// Only fetch older history here if entering browsing landed
				// straight on the oldest message (a single-message room) —
				// otherwise pagination fires once curPos reaches 0 while
				// already browsing (see updateBrowsingKey's "up" case).
				// Checking AtTop() here instead would fire on every short
				// room whose content already fits the viewport, regardless
				// of which message was just selected.
				if len(sel) == 1 {
					return m.maybeLoadOlderMessages()
				}
				return m, nil
			case "down":
				m.viewport.ScrollDown(1)
				return m, nil
			case "pgup":
				sel := selectableMessageIndices(m.messages, m.mutedUsers())
				if len(sel) == 0 {
					m.viewport.ScrollUp(m.viewport.Height)
					return m.maybeLoadOlderMessages()
				}
				m.input.Blur()
				m.selectedMsgID = m.messages[sel[len(sel)-1]].ID
				m = m.refreshMessages()
				m = m.ensureSelectedMessageVisible()
				if len(sel) == 1 {
					return m.maybeLoadOlderMessages()
				}
				return m, nil
			case "pgdown":
				m.viewport.ScrollDown(m.viewport.Height)
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

// mutedUsers returns the set of usernames muted in the active room, for
// filtering them out of the rendered message list.
func (m ChatroomsModel) mutedUsers() map[string]bool {
	if m.activeRoomID == "" || len(m.mutedUsersByRoom[m.activeRoomID]) == 0 {
		return nil
	}
	muted := make(map[string]bool, len(m.mutedUsersByRoom[m.activeRoomID]))
	for _, u := range m.mutedUsersByRoom[m.activeRoomID] {
		muted[strings.ToLower(u)] = true
	}
	return muted
}

func (m ChatroomsModel) renderMessages() string {
	if len(m.messages) == 0 {
		if m.err != nil {
			return theme.Subtle.Render("couldn't load messages")
		}
		return theme.Subtle.Render("no messages yet")
	}
	return renderCircMessages(m.messages, m.location(), m.timeDisplayFormat, m.viewport.Width, m.currentUser, m.mutedUsers())
}

// refreshMessages rebuilds the viewport content and the per-message
// offset/height tables (m.msgOffsets/m.msgHeights) used to keep the selected
// message in view and to highlight it. Every mutation of m.messages or the
// viewport's width must call this instead of setting content directly.
func (m ChatroomsModel) refreshMessages() ChatroomsModel {
	if len(m.messages) == 0 {
		m.viewport.SetContent(m.renderMessages())
		m.msgOffsets, m.msgHeights = nil, nil
		return m
	}
	content, offsets, heights, imgSlots := renderCircMessagesWithSelection(
		m.messages, m.location(), m.timeDisplayFormat, m.viewport.Width, m.currentUser, m.selectedMsgID,
		m.revealed, m.styleAnimFrame, m.mutedUsers(), m.inlineImagesEnabled, m.imageRealRows)
	m.viewport.SetContent(content)
	m.msgOffsets, m.msgHeights = offsets, heights
	m.msgImages = imgSlots
	return m
}

// SetImageRealRows records key's actual fetched/fitted row count (from
// App.recordInlineImageRealRows) and, if it changed, re-renders so the
// message list's reserved band shrinks from the fallback-max placeholder
// down to the image's real size — see renderCircMessagesWithSelection's
// band-sizing. If the viewport was scrolled to the bottom before the
// reflow, it's re-homed to the (now shorter) bottom afterward — otherwise
// the room would settle without ever reaching the true bottom as each
// image's band shrinks out from under a fixed YOffset, one fetch at a
// time. Never yanks the view down if the user had scrolled up to read
// history, mirroring SetMessages' own GotoBottom convention.
func (m ChatroomsModel) SetImageRealRows(key string, rows int) ChatroomsModel {
	if m.imageRealRows[key] == rows {
		return m
	}
	if m.imageRealRows == nil {
		m.imageRealRows = make(map[string]int)
	}
	m.imageRealRows[key] = rows
	if m.mode == chatroomModeDetail && m.ready {
		wasAtBottom := m.viewport.AtBottom()
		m = m.refreshMessages()
		if wasAtBottom {
			m.viewport.GotoBottom()
		}
	}
	return m
}

// VisibleInlineImages returns the inline image slots currently fully within
// the viewport, top to bottom, across every visible message — see
// PostDetailModel.VisibleInlineImages for the full contract.
func (m ChatroomsModel) VisibleInlineImages() []InlineImageSlot {
	if m.mode != chatroomModeDetail || !m.inlineImagesEnabled {
		return nil
	}
	top, bottom := m.viewport.YOffset, m.viewport.YOffset+m.viewport.Height

	var slots []InlineImageSlot
	for i, msg := range m.messages {
		if i >= len(m.msgImages) || i >= len(m.msgOffsets) {
			continue
		}
		for _, img := range m.msgImages[i] {
			abs := m.msgOffsets[i] + img.Line
			key := circMsgImageKey(msg.ID)
			// imgRows is the actual reserved image-row allowance for this
			// message (chatImageBandRows minus its leading spacer, which
			// sits before abs, not after) — not the old fixed
			// inlineImageMaxRows, which over-required clearance once the
			// band shrank below that constant and hid the last message's
			// image in the room (nothing below it to push bottom out
			// further).
			imgRows := chatImageBandRows(m.imageRealRows, key) - 1
			if abs < top || abs+imgRows > bottom {
				continue
			}
			colIndent := circMessageTextIndent(msg)
			slots = append(slots, InlineImageSlot{
				URL: img.URL,
				// abs-top is relative to the message viewport's own top edge
				// (row 0 = the viewport's first visible line), but the
				// viewport isn't screen row 0 within this screen's own
				// content — View() stacks header+divider
				// (chatroomDetailHeaderRows) above it. Without adding that
				// back, an image near the top of the visible messages lands
				// on the header/divider instead of the message it belongs to.
				Row:       abs - top + chatroomDetailHeaderRows,
				ColIndent: colIndent,
				MaxCols:   max(m.viewport.Width-colIndent-2, 10),
				MaxRows:   inlineImageEncodeMaxRows,
				Key:       key,
			})
		}
	}
	return slots
}

// selectableMessageIndices returns the indices into msgs of messages that can
// be selected/flagged: a real (non-system) message with a stable ID. System
// notices (AppendSystemMessage) have no ID and are never selectable. Muted
// senders are also excluded, since their messages render as hidden/zero-height.
func selectableMessageIndices(msgs []model.Message, muted map[string]bool) []int {
	var sel []int
	for i, msg := range msgs {
		if !msg.IsSystem && msg.ID != "" && !muted[strings.ToLower(msg.From.Username)] {
			sel = append(sel, i)
		}
	}
	return sel
}

// selectablePos returns the position within sel whose message ID matches id,
// or -1 if id is empty or not found (e.g. it scrolled out of the loaded
// window, or — in the future — was deleted).
func selectablePos(msgs []model.Message, sel []int, id string) int {
	if id == "" {
		return -1
	}
	for pos, idx := range sel {
		if msgs[idx].ID == id {
			return pos
		}
	}
	return -1
}

// findMessageByID returns the message with the given ID and whether it was found.
func findMessageByID(msgs []model.Message, id string) (model.Message, bool) {
	for _, msg := range msgs {
		if msg.ID == id {
			return msg, true
		}
	}
	return model.Message{}, false
}

// selOffsets/selHeights project offsets/heights (1:1 with a message list)
// through sel (the selectable-only index list), for feeding into
// millerPageNav. Shared by ChatroomsModel and CMailModel browsing.
func selOffsets(offsets []int, sel []int) []int {
	out := make([]int, len(sel))
	for i, idx := range sel {
		out[i] = offsets[idx]
	}
	return out
}

func selHeights(heights []int, sel []int) []int {
	out := make([]int, len(sel))
	for i, idx := range sel {
		out[i] = heights[idx]
	}
	return out
}

// ensureMessageVisible scrolls vp the minimum amount so the message whose ID
// matches selectedID is fully visible, mirroring PostDetailModel's
// ensureSelectedVisible. Shared by ChatroomsModel and CMailModel browsing.
func ensureMessageVisible(vp viewport.Model, msgs []model.Message, offsets, heights []int, selectedID string) viewport.Model {
	for i, msg := range msgs {
		if msg.ID != selectedID {
			continue
		}
		itemStart := offsets[i]
		itemEnd := itemStart + heights[i] - 1
		if itemStart < vp.YOffset {
			vp.SetYOffset(itemStart)
		} else if itemEnd >= vp.YOffset+vp.Height {
			vp.SetYOffset(itemEnd - vp.Height + 1)
		}
		return vp
	}
	return vp
}

// ensureSelectedMessageVisible scrolls the viewport the minimum amount so the
// selected message is fully visible.
func (m ChatroomsModel) ensureSelectedMessageVisible() ChatroomsModel {
	if !m.ready {
		return m
	}
	m.viewport = ensureMessageVisible(m.viewport, m.messages, m.msgOffsets, m.msgHeights, m.selectedMsgID)
	return m
}

// maybeLoadOlderMessages fires the older-history fetch once scrolled to the
// very top of loaded messages. Shared by both places 'up' can reach the top:
// raw scroll from the sentinel, and paging further up while already browsing.
func (m ChatroomsModel) maybeLoadOlderMessages() (ChatroomsModel, tea.Cmd) {
	if m.viewport.AtTop() && !m.loadingHistory && !m.historyExhausted &&
		m.activeRoom != nil && len(m.messages) > 0 {
		m.loadingHistory = true
		before := m.messages[0].CreatedAt.UnixMilli()
		return m, m.loadOlderRoomMessagesCmd(m.activeRoom.Slug, before)
	}
	return m, nil
}

// updateBrowsingKey handles keys while a message is selected
// (m.selectedMsgID != ""): up/down move the selection, esc returns to
// typing, '!' reports the selected message, 'p' views the sender's
// profile, enter toggles a spoiler- or l33t-styled message's reveal
// state. Everything else is swallowed rather than typed, since the
// input is blurred for the duration of browsing — see the 'up' case in
// the typing-mode switch, the only place browsing is entered.
func (m ChatroomsModel) updateBrowsingKey(msg tea.KeyMsg) (ChatroomsModel, tea.Cmd) {
	if m.confirmingDeleteMsg {
		switch msg.String() {
		case "y":
			messageID := m.selectedMsgID
			roomID := ""
			if m.activeRoom != nil {
				roomID = m.activeRoom.Slug
			}
			m.confirmingDeleteMsg = false
			m.viewport.Height = m.viewportHeight()
			return m, func() tea.Msg {
				return DeleteRoomMessageMsg{RoomID: roomID, MessageID: messageID}
			}
		case "n", "esc":
			m.confirmingDeleteMsg = false
			m.viewport.Height = m.viewportHeight()
		}
		return m, nil
	}
	sel := selectableMessageIndices(m.messages, m.mutedUsers())
	curPos := selectablePos(m.messages, sel, m.selectedMsgID)
	if curPos < 0 {
		// The selected message no longer exists — fall back to typing.
		m.selectedMsgID = ""
		m.input.Focus()
		return m.refreshMessages(), nil
	}
	switch msg.String() {
	case "esc":
		m.selectedMsgID = ""
		m.input.Focus()
		m = m.refreshMessages()
		m.viewport.GotoBottom()
		return m, nil
	case "up":
		if curPos == 0 {
			return m.maybeLoadOlderMessages()
		}
		newPos, newOffset := millerPageNav(-1, m.viewport.Height, 0,
			selOffsets(m.msgOffsets, sel), selHeights(m.msgHeights, sel), curPos, m.viewport.YOffset)
		if newPos < 0 {
			newPos = 0
		}
		m.selectedMsgID = m.messages[sel[newPos]].ID
		m.viewport.SetYOffset(newOffset)
		m = m.refreshMessages()
		if newPos == 0 {
			return m.maybeLoadOlderMessages()
		}
		return m, nil
	case "down":
		if curPos >= len(sel)-1 {
			m.selectedMsgID = ""
			m.input.Focus()
			m = m.refreshMessages()
			m.viewport.GotoBottom()
			return m, nil
		}
		newPos, newOffset := millerPageNav(+1, m.viewport.Height, 0,
			selOffsets(m.msgOffsets, sel), selHeights(m.msgHeights, sel), curPos, m.viewport.YOffset)
		m.selectedMsgID = m.messages[sel[newPos]].ID
		m.viewport.SetYOffset(newOffset)
		return m.refreshMessages(), nil
	case "pgup":
		if curPos == 0 {
			return m.maybeLoadOlderMessages()
		}
		newPos, newOffset := curPos, m.viewport.YOffset
		for i := 0; i < m.viewport.Height && newPos > 0; i++ {
			newPos, newOffset = millerPageNav(-1, m.viewport.Height, 0,
				selOffsets(m.msgOffsets, sel), selHeights(m.msgHeights, sel), newPos, newOffset)
		}
		if newPos < 0 {
			newPos = 0
		}
		m.selectedMsgID = m.messages[sel[newPos]].ID
		m.viewport.SetYOffset(newOffset)
		m = m.refreshMessages()
		if newPos == 0 {
			return m.maybeLoadOlderMessages()
		}
		return m, nil
	case "pgdown":
		if curPos >= len(sel)-1 {
			m.selectedMsgID = ""
			m.input.Focus()
			m = m.refreshMessages()
			m.viewport.GotoBottom()
			return m, nil
		}
		newPos, newOffset := curPos, m.viewport.YOffset
		for i := 0; i < m.viewport.Height && newPos < len(sel)-1; i++ {
			newPos, newOffset = millerPageNav(+1, m.viewport.Height, 0,
				selOffsets(m.msgOffsets, sel), selHeights(m.msgHeights, sel), newPos, newOffset)
		}
		m.selectedMsgID = m.messages[sel[newPos]].ID
		m.viewport.SetYOffset(newOffset)
		return m.refreshMessages(), nil
	case "enter":
		targetMsg, ok := findMessageByID(m.messages, m.selectedMsgID)
		if !ok || (!slices.Contains(targetMsg.Style, styleSpoiler) && !slices.Contains(targetMsg.Style, styleL33t)) {
			return m, nil
		}
		if m.revealed == nil {
			m.revealed = make(map[string]bool)
		}
		m.revealed[m.selectedMsgID] = !m.revealed[m.selectedMsgID]
		return m.refreshMessages(), nil
	case "!":
		targetMsg, ok := findMessageByID(m.messages, m.selectedMsgID)
		if !ok || targetMsg.Deleted || targetMsg.From.Username == m.currentUser {
			return m, nil
		}
		m.flagTargetMsgID = m.selectedMsgID
		var cmd tea.Cmd
		m.flagPrompt, cmd = m.flagPrompt.Open(FlagKindMessage)
		m.viewport.Height = m.viewportHeight()
		return m, cmd
	case "d":
		targetMsg, ok := findMessageByID(m.messages, m.selectedMsgID)
		if !ok || targetMsg.Deleted || targetMsg.From.Username != m.currentUser {
			return m, nil
		}
		m.confirmingDeleteMsg = true
		m.viewport.Height = m.viewportHeight()
		return m, nil
	case "p":
		targetMsg, ok := findMessageByID(m.messages, m.selectedMsgID)
		if !ok {
			return m, nil
		}
		username := targetMsg.From.Username
		return m, func() tea.Msg { return ShowUserProfileMsg{Username: username} }
	}
	return m, nil
}

// viewportHeight returns the message-history viewport height in rows,
// shrinking to make room for the flag/report overlay when it's open.
func (m ChatroomsModel) viewportHeight() int {
	h := m.height - theme.ChromeHeight - chatroomDetailChrome
	if m.flagPrompt.Active() {
		h -= m.flagPrompt.Height()
	}
	if m.confirmingDeleteMsg {
		h -= confirmBoxHeight
	}
	if h < 1 {
		h = 1
	}
	return h
}

// Panel sizing: the users panel is pinned to a preferred width that
// comfortably fits an admin marker, the longest possible username (20 chars,
// the API's max), and an idle badge; below roomUsersPanelMinMsgWidth for the
// message viewport, the panel collapses entirely rather than shrinking below
// its preferred width — an in-between size could be narrower than the
// worst-case content, and lipgloss/cellbuf.Wrap hard-breaks a too-long
// unbroken word (a username has no spaces to break on) instead of truncating
// it.
const (
	roomUsersPanelPreferredWidth = 27 // admin marker(2) + username(20) + idle badge(3) + padding(2)
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

// mentionCandidatePool returns the users eligible for @-mention completion:
// everyone currently online (m.roomUsers, admins-first/alphabetical) plus
// anyone else who has a message in the currently loaded history — every
// page pulled in so far via scroll-to-top pagination, not just what's
// visible in the viewport right now — so a since-departed poster can still
// be completed. History-only users are appended after the online ones
// (alphabetical, deduped case-insensitively) so Tab-cycling still prefers
// present users first when names collide by prefix.
func (m ChatroomsModel) mentionCandidatePool() []model.RoomUser {
	seen := make(map[string]bool, len(m.roomUsers))
	for _, u := range m.roomUsers {
		seen[strings.ToLower(u.Username)] = true
	}
	var extra []model.RoomUser
	for _, msg := range m.messages {
		name := msg.From.Username
		if name == "" || msg.IsSystem {
			continue
		}
		key := strings.ToLower(name)
		if seen[key] {
			continue
		}
		seen[key] = true
		extra = append(extra, model.RoomUser{Username: name})
	}
	sort.Slice(extra, func(i, j int) bool {
		return strings.ToLower(extra[i].Username) < strings.ToLower(extra[j].Username)
	})
	return append(append([]model.RoomUser(nil), m.roomUsers...), extra...)
}

// matchMentionCandidates returns the users whose username starts with query
// (case-insensitive), preserving the input slice's order.
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

// mentionActiveCandidate resolves the mention token at the cursor (if any)
// and which candidate is currently being previewed — the active Tab-cycle's
// selection, or with no cycle yet, the first match (the same default
// mentionGhostText shows before any Tab press). Shared by the ghost preview
// renderer and Space's commit handler so they can never disagree about what
// "the current candidate" is — that mismatch was the bug where Space typed
// immediately after "@al" (no Tab press) inserted a literal space instead of
// committing the name that was visibly previewed.
func (m ChatroomsModel) mentionActiveCandidate() (atPos, cursor int, query, candidate string, ok bool) {
	cursor = m.input.Position()
	if cursor != len([]rune(m.input.Value())) {
		return 0, 0, "", "", false
	}
	q, ap, mok := mentionQueryAt(m.input.Value(), cursor)
	if !mok {
		return 0, 0, "", "", false
	}
	matches := matchMentionCandidates(m.mentionCandidatePool(), q)
	if len(matches) == 0 {
		return 0, 0, "", "", false
	}
	if c := m.mentionCycle; c != nil && c.atPos == ap && c.queryEnd == cursor {
		return ap, cursor, q, matches[c.index%len(matches)].Username, true
	}
	return ap, cursor, q, matches[0].Username, true
}

// mentionGhostText returns the plain (unstyled) uncommitted remainder of the
// mention preview for display right after the cursor — only when the
// mention token reaches the very end of the input. That's the only place a
// preview can render unambiguously: with trailing text after the token
// ("hey @al there"), there's no sensible place to draw it without
// overlapping what's already typed — Tab/Space still work there, just
// without this visual hint. Styling is applied by the caller (View()),
// which needs the first rune split off separately to overlay it on the
// cursor.
func (m ChatroomsModel) mentionGhostText() string {
	_, _, query, candidate, ok := m.mentionActiveCandidate()
	if !ok {
		return ""
	}
	top := []rune(candidate)
	q := []rune(query)
	if len(top) <= len(q) {
		return "" // already fully typed/matched — nothing left to preview
	}
	return string(top[len(q):])
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
// for another admin, or unstyled otherwise. idleAfterMs is the threshold (ms)
// past which a user with no recent LastActivity is shown idle (💤, prefixed
// right before the name — after the admin marker, if any); a nil
// LastActivity always means active (the client never reported one), and
// idleAfterMs <= 0 (not yet known from the server) means nobody is flagged.
func renderRoomUsersPanel(users []model.RoomUser, currentUser string, idleAfterMs int) string {
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
		idle := idleAfterMs > 0 && u.LastActivity != nil && time.Since(*u.LastActivity) > time.Duration(idleAfterMs)*time.Millisecond
		if idle {
			name = theme.Subtle.Render("💤 ") + name
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
		// textinput.View() always renders its own cursor — there's no way to
		// make it overlay ghost text through that API, and its own padding
		// sits right after the cursor regardless of Width, so appending
		// anything after View()'s output leaves a gap. Bypass View()
		// entirely when a ghost is showing: hand-build the line from
		// textinput's own exported style fields, with the cursor overlaying
		// the ghost's first rune (the same thing textinput.View() would do
		// with a blank space) instead of a separate cursor cell.
		ghost := m.mentionGhostText()
		var inputContent string
		if ghost != "" {
			ghostRunes := []rune(ghost)
			cur := m.input.Cursor
			cur.SetChar(string(ghostRunes[0]))
			// Without this, the blink-off phase renders the overlaid
			// character in the cursor's default TextStyle — i.e. normal
			// text color, indistinguishable from something actually typed.
			cur.TextStyle = theme.Subtle
			textView := m.input.TextStyle.Inline(true).Render(m.input.Value())
			promptView := m.input.PromptStyle.Render(m.input.Prompt)
			rest := theme.Subtle.Render(string(ghostRunes[1:]))
			valWidth := lipgloss.Width(m.input.Value())
			// +1 matches a quirk of textinput.View()'s own padding math (the
			// no-ghost path below): it computes padding as Width-valWidth,
			// without accounting for the phantom end-of-line cursor glyph's
			// own column — so a no-ghost render is always exactly one column
			// wider than Width alone would suggest. Matching that here keeps
			// the box from jittering by one column when a ghost appears or
			// disappears between frames.
			pad := max(0, m.input.Width-valWidth-lipgloss.Width(ghost)+1)
			inputContent = promptView + textView + cur.View() + rest + strings.Repeat(" ", pad)
		} else if m.input.Value() == "" {
			// textinput.View()'s empty-input placeholder path
			// (placeholderView(), internal) totals its render at exactly
			// Width, unlike the typed-content path above (Width+len(Prompt)+1
			// — the whole reason input.Width is pre-reduced by that amount).
			// Hand-building this state too, the same way the ghost branch
			// above does, guarantees the same total without needing to
			// separately verify a second internal quirk: matching
			// PromptWidth+1+(rest+pad) to the typed path's confirmed-correct
			// total just requires rest+pad to sum to input.Width exactly,
			// which the pad line below does by definition.
			promptView := m.input.PromptStyle.Render(m.input.Prompt)
			cur := m.input.Cursor
			placeholder := []rune(m.input.Placeholder)
			cur.TextStyle = m.input.PlaceholderStyle
			var rest string
			if len(placeholder) > 0 {
				cur.SetChar(string(placeholder[0]))
				rest = m.input.PlaceholderStyle.Inline(true).Render(string(placeholder[1:]))
			} else {
				cur.SetChar(" ")
			}
			pad := max(0, m.input.Width-lipgloss.Width(rest))
			inputContent = promptView + cur.View() + rest + strings.Repeat(" ", pad)
		} else {
			inputContent = m.input.View()
		}
		inputBox := theme.ActiveBorder.Render(inputContent)

		_, usersW := m.panelWidths()
		messageArea := m.viewport.View()
		if usersW > 0 {
			panel := lipgloss.NewStyle().Width(usersW).Height(m.viewport.Height).MaxHeight(m.viewport.Height).
				Render(renderRoomUsersPanel(m.roomUsers, m.currentUser, m.idleAfterMs))
			sep := theme.Subtle.Render(strings.TrimSuffix(strings.Repeat("│\n", m.viewport.Height), "\n"))
			messageArea = lipgloss.JoinHorizontal(lipgloss.Top, messageArea, sep, panel)
		}
		divider := theme.Subtle.Render(strings.Repeat("─", max(m.width, 0)))
		if m.flagPrompt.Active() {
			return lipgloss.JoinVertical(lipgloss.Left, header, divider, messageArea, m.flagPrompt.View(m.width), inputBox)
		}
		if m.confirmingDeleteMsg {
			prompt := theme.Error.Render("Delete this message?") + "  " +
				theme.Base.Render("[y]es") + "  " +
				theme.Subtle.Render("[n]o / esc")
			promptView := theme.ActiveBorder.Width(m.width - 2).Render(prompt)
			return lipgloss.JoinVertical(lipgloss.Left, header, divider, messageArea, promptView, inputBox)
		}
		return lipgloss.JoinVertical(lipgloss.Left, header, divider, messageArea, inputBox)
	default: // chatroomModeList
		if !m.ready {
			return theme.Subtle.Render("loading rooms…")
		}
		return m.listVP.View()
	}
}
