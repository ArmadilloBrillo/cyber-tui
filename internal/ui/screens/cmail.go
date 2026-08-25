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
	cmailDetailHeaderRows = 2 // "@otheruser" header + divider rule
	cmailInputRows        = 3 // bordered textinput: 1 content + 2 border rows
	cmailDetailChrome     = cmailDetailHeaderRows + cmailInputRows
)

// Typing-indicator cadence. dmTypingDefaultStaleAfterMs is used for the read
// subscription (showing the other participant typing) since we open it
// before ever announcing our own typing — our own re-announce heartbeat
// instead uses whatever AnnounceTyping's response returns, never hard-coded.
// dmTypingAnimInterval drives a single merged tea.Tick that does double duty:
// the dot-animation frame counter and the local idle-check (comparing
// against dmTypingIdleThreshold) both run off the same 500ms tick — they
// used to be two independent tea.Tick chains at the same cadence, coalesced
// into one (see the typingAnimTickMsg handler).
const (
	dmTypingDefaultStaleAfterMs = 9000
	dmTypingIdleThreshold       = 2500 * time.Millisecond
	dmTypingAnimInterval        = 500 * time.Millisecond // one dot per tick; 2s full "" → "." → ".." → "..." cycle
)

// dmSubscription holds the live RTDB channel and its cancellation function.
type dmSubscription struct {
	ConvID string
	C      <-chan model.Message
	cancel context.CancelFunc
}

// dmTypingSubscription holds the live typing-presence RTDB channel and its
// cancellation function. Each receive is a full, filtered snapshot of who's
// currently typing, not a single incremental event (see SubscribeDMTyping).
type dmTypingSubscription struct {
	ConvID string
	C      <-chan []model.TypingUser
	cancel context.CancelFunc
}

// userConvsSubscription holds the live RTDB channel and its cancellation
// function for /user_conversations/<uid> — the account-wide conversation
// list, independent of which (if any) conversation is currently open. Each
// receive is the full converted+sorted list (see SubscribeUserConversations).
type userConvsSubscription struct {
	C      <-chan []model.Conversation
	cancel context.CancelFunc
}

// DM subscription message types — unexported, handled entirely within CMailModel.
type dmSubscribedMsg struct {
	convID string
	sub    *dmSubscription
}
type dmReceivedMsg struct{ msg model.Message }
type dmStreamClosedMsg struct{ convID string }
type dmReconnectedMsg struct{ sub *dmSubscription }
type dmReconnectFailedMsg struct {
	convID  string
	attempt int
	err     error
}
type dmReconnectRetryDueMsg struct {
	convID  string
	attempt int
}
type cmailMsgsLoadedMsg struct {
	convID string
	msgs   []model.Message
}
type cmailOlderMsgsLoadedMsg struct {
	convID string
	msgs   []model.Message
}
type cmailErrMsg struct{ err error }

// Typing message types — unexported, handled entirely within CMailModel.
// Mirror the message-stream lifecycle types above.
type typingAnnouncedMsg struct {
	convID       string
	heartbeatMs  int
	staleAfterMs int
}
type typingHeartbeatTickMsg struct{ convID string }
type typingAnimTickMsg struct{ convID string }
type dmTypingSubscribedMsg struct {
	convID string
	sub    *dmTypingSubscription
}
type dmTypingReceivedMsg struct{ users []model.TypingUser }
type dmTypingStreamClosedMsg struct{ convID string }
type dmTypingReconnectedMsg struct{ sub *dmTypingSubscription }
type dmTypingReconnectFailedMsg struct {
	convID  string
	attempt int
	err     error
}
type dmTypingReconnectRetryDueMsg struct {
	convID  string
	attempt int
}

// Account-wide conversation-list subscription message types — unexported,
// handled entirely within CMailModel. Unlike the DM/typing types above,
// these aren't scoped to activeConvID: the subscription lives for as long as
// the user is logged in, regardless of which conversation (if any) is open.
type userConvsSubscribedMsg struct{ sub *userConvsSubscription }
type userConvsReceivedMsg struct{ convs []model.Conversation }
type userConvsStreamClosedMsg struct{}
type userConvsReconnectedMsg struct{ sub *userConvsSubscription }
type userConvsReconnectFailedMsg struct {
	attempt int
	err     error
}
type userConvsReconnectRetryDueMsg struct{ attempt int }

// CMailReconnectedMsg is emitted after the live RTDB stream is successfully
// re-established following an idToken expiry. App uses it to show a toast.
type CMailReconnectedMsg struct{}

// waitForDM blocks on the subscription channel and returns the next message as a tea.Cmd.
func waitForDM(sub *dmSubscription) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-sub.C
		if !ok {
			return dmStreamClosedMsg{convID: sub.ConvID}
		}
		return dmReceivedMsg{msg: msg}
	}
}

// waitForDMTyping blocks on the typing-presence subscription channel and
// returns the next snapshot as a tea.Cmd.
func waitForDMTyping(sub *dmTypingSubscription) tea.Cmd {
	return func() tea.Msg {
		users, ok := <-sub.C
		if !ok {
			return dmTypingStreamClosedMsg{convID: sub.ConvID}
		}
		return dmTypingReceivedMsg{users: users}
	}
}

// waitForUserConvs blocks on the account-wide conversation-list subscription
// channel and returns the next snapshot as a tea.Cmd.
func waitForUserConvs(sub *userConvsSubscription) tea.Cmd {
	return func() tea.Msg {
		convs, ok := <-sub.C
		if !ok {
			return userConvsStreamClosedMsg{}
		}
		return userConvsReceivedMsg{convs: convs}
	}
}

// IsDMStreamMsg reports whether msg belongs to C-Mail's message/typing/
// conversation-list subscription lifecycle. App uses this to keep routing
// these messages to CMailModel.Update even when C-Mail isn't the active
// screen, so the self-rescheduling waitForDM/heartbeat/idle-check/reconnect
// tea.Cmd chains for the conversation the user had open — and the always-on
// account-wide conversation-list stream — don't die just because they
// switched tabs.
func IsDMStreamMsg(msg tea.Msg) bool {
	switch msg.(type) {
	case dmSubscribedMsg, dmReceivedMsg, dmStreamClosedMsg,
		dmReconnectedMsg, dmReconnectFailedMsg, dmReconnectRetryDueMsg,
		cmailMsgsLoadedMsg, cmailOlderMsgsLoadedMsg, cmailErrMsg,
		typingAnnouncedMsg, typingHeartbeatTickMsg,
		typingAnimTickMsg, dmTypingSubscribedMsg, dmTypingReceivedMsg,
		dmTypingStreamClosedMsg, dmTypingReconnectedMsg, dmTypingReconnectFailedMsg,
		dmTypingReconnectRetryDueMsg,
		userConvsSubscribedMsg, userConvsReceivedMsg, userConvsStreamClosedMsg,
		userConvsReconnectedMsg, userConvsReconnectFailedMsg, userConvsReconnectRetryDueMsg:
		return true
	default:
		return false
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

	// Message browsing state, mirroring ChatroomsModel: selectedMsgID is ""
	// while composing/typing, and set to a message ID while browsing (up/down
	// move message-by-message rather than scrolling by raw line). msgOffsets/
	// msgHeights are 1:1 with activeConv.Messages, rebuilt on every
	// refreshMessages call.
	selectedMsgID string
	msgOffsets    []int
	msgHeights    []int
	// msgImages is parallel to msgOffsets/msgHeights — see ChatroomsModel's
	// field of the same name for the convention.
	msgImages           [][]postImageSlot
	inlineImagesEnabled bool
	// imageRealRows caches each image slot's actual fetched/fitted row
	// count — see ChatroomsModel's field of the same name.
	imageRealRows map[string]int
	// chatBodyCache memoizes renderChatMessagesWithSelection's per-message
	// output, keyed by message ID — see ChatroomsModel's chatBodyCache field
	// of the same name and cmailBodyCacheEntry's doc comment.
	chatBodyCache map[string]cmailBodyCacheEntry
	// otherProfiles caches a conversation partner's full profile (fetched via
	// GetProfile, keyed by username) once App has resolved it — see
	// SetOtherProfile/otherParticipantBadgeUser. Conversation data alone never
	// carries SupporterIcon/IsSupporter.
	otherProfiles map[string]model.User

	// canGoBack is true when the active conversation was opened via a
	// deep link (e.g. 'c' on a post, or a chat_mention/dm_message
	// notification) rather than by switching to this tab normally. When
	// true, ESC in detail mode leaves C-Mail entirely instead of dropping
	// to the conversation list. Reset to false by activateScreen whenever
	// C-Mail is entered through ordinary tab navigation.
	canGoBack bool

	// DM subscription state — managed entirely within CMailModel.
	client        api.Client
	currentUserID string
	dmSub         *dmSubscription
	activeConvID  string

	// Account-wide conversation-list subscription state, independent of
	// dmSub/activeConvID — lives for as long as the user is logged in. Its
	// own reconnect-retry state mirrors reconnectAttempt/reconnecting/
	// reconnectFailed/reconnectCtx/reconnectCancel below, kept separate so
	// the two reconnect sequences (per-conversation vs account-wide) never
	// interfere with each other.
	userConvsSub              *userConvsSubscription
	userConvsReconnectAttempt int
	userConvsReconnecting     bool
	userConvsReconnectCtx     context.Context
	userConvsReconnectCancel  context.CancelFunc

	// focused is true while the C-Mail tab is the one on screen. The RTDB
	// subscription for an open conversation stays alive regardless (see
	// IsDMStreamMsg), so this only gates whether an incoming message bumps
	// that conversation's UnreadCount for the tab-bar badge (TotalUnread()).
	focused bool

	// Reconnect-retry state, active only between a stream closing and either
	// a successful reconnect or exhausting maxReconnectAttempts.
	reconnectAttempt int
	reconnecting     bool
	reconnectFailed  bool
	reconnectCtx     context.Context
	reconnectCancel  context.CancelFunc

	// History pagination state for the active conversation, reset on open.
	historyExhausted bool // true once an older-page fetch returns zero messages
	loadingHistory   bool // guards re-firing while an older-page fetch is in flight

	// typingIndicatorsEnabled is the user's Settings preference (positive
	// polarity — see config.Config.TypingIndicatorsDisabled) gating the
	// entire typing-indicator subsystem below: the inbound RTDB subscription,
	// the outbound announce/clear calls, and the merged anim/idle-check tick.
	// Driven by SharedConfigMsg; see that case in Update for the live
	// on/off-mid-session transition.
	typingIndicatorsEnabled bool

	// Typing-indicator state: the read side (subscription to the other
	// participant's typing status) and the write side (our own announce).
	typingSub         *dmTypingSubscription
	typingUsers       []model.TypingUser // latest fresh snapshot; used to render "is typing..."
	announcingTyping  bool               // true from the first keystroke's POST until send/idle-clear
	lastKeystrokeAt   time.Time          // updated on every keystroke; idle-check compares against this
	typingHeartbeatMs int                // from AnnounceTyping's response; drives our own re-announce cadence
	typingAnimFrame   int                // cycles the indicator's animated dot count; free-runs while a conversation is open

	// Typing-stream reconnect-retry state, mirroring reconnectAttempt/
	// reconnecting/reconnectFailed/reconnectCtx/reconnectCancel above but for
	// the supplementary typing-presence read subscription.
	typingReconnectAttempt int
	typingReconnecting     bool
	typingReconnectFailed  bool
	typingReconnectCtx     context.Context
	typingReconnectCancel  context.CancelFunc

	// styleAnimFrame/styleAnimRunning drive the slow/wave/glitch animated
	// message styles — see maybeStartStyleAnim and chatrooms.go's identical fields.
	styleAnimFrame   int
	styleAnimRunning bool

	// animPaused suppresses the refreshMessages() re-render that styleAnimTickMsg
	// would otherwise trigger, without stopping the ticker chain itself. Set by
	// app.go while the image modal is open — see ChatroomsModel.animPaused for
	// the full rationale.
	animPaused bool
}

// SendCMailMsg is emitted when the user sends a C-Mail message.
type SendCMailMsg struct {
	ConversationID string
	Body           string
}

// CMailConvSelectedMsg is emitted when the user opens a conversation.
// App uses it to call MarkCMailRead on the server, and — when OtherUsername
// is non-empty — to fetch that participant's full profile (for their
// SupporterIcon; conversation data alone never carries it, see
// otherParticipantUser's doc comment) via GetProfile, caching the result
// with SetOtherProfile. Left empty by whoever emits this when the
// participant can't be resolved (e.g. "unknown") or is already cached.
type CMailConvSelectedMsg struct {
	ConversationID string
	OtherUsername  string
}

// NewCMailModel creates a new CMailModel for the given authenticated user.
// currentUserID is the account's RTDB uid, used to open the account-wide
// conversation-list subscription (see OpenUserConvsSubscription).
func NewCMailModel(currentUser, currentUserID string, client api.Client) CMailModel {
	inp := textinput.New()
	inp.Placeholder = "compose c-mail..."
	return CMailModel{
		input:         inp,
		currentUser:   currentUser,
		currentUserID: currentUserID,
		client:        client,
		mode:          cmailModeList,
		chatBodyCache: make(map[string]cmailBodyCacheEntry),
	}
}

// cancelDMSub stops any active RTDB subscription (message and typing) and
// any in-flight reconnect retry sequence, and clears subscription state.
func (m CMailModel) cancelDMSub() CMailModel {
	if m.dmSub != nil {
		m.dmSub.cancel()
		m.dmSub = nil
	}
	if m.typingSub != nil {
		m.typingSub.cancel()
		m.typingSub = nil
	}
	if m.reconnectCancel != nil {
		m.reconnectCancel()
		m.reconnectCancel = nil
	}
	if m.typingReconnectCancel != nil {
		m.typingReconnectCancel()
		m.typingReconnectCancel = nil
	}
	m.reconnecting = false
	m.reconnectFailed = false
	m.reconnectAttempt = 0
	m.typingReconnecting = false
	m.typingReconnectFailed = false
	m.typingReconnectAttempt = 0
	m.activeConvID = ""
	m.typingUsers = nil
	m.announcingTyping = false
	m.typingHeartbeatMs = 0
	m.typingAnimFrame = 0
	return m
}

// CancelSubscription is called by App when navigating away from the C-Mail screen.
func (m CMailModel) CancelSubscription() CMailModel {
	return m.cancelDMSub()
}

// OpenUserConvsSubscription starts the account-wide live conversation-list
// subscription. There's no REST seed: the subscription's own first event is
// a full snapshot (same as chat_presence's), so a separate GetConversations
// call first would just create a second, independent writer to
// m.conversations — see SetConversations' callers for why that's a bug, not
// a feature. m.conversations (nil on a fresh login) is still passed as
// initial so a *reconnect* can seed from the last known-good list instead of
// going blank. A no-op if already subscribed — App calls this once, right
// after login.
func (m CMailModel) OpenUserConvsSubscription() (CMailModel, tea.Cmd) {
	if m.userConvsSub != nil {
		return m, nil
	}
	return m, m.openUserConvsSubscriptionCmd(m.conversations)
}

// CancelUserConvsSubscription stops the account-wide conversation-list
// subscription and any in-flight reconnect sequence. Called on logout/
// session end — unlike cancelDMSub, this is not tied to leaving a single
// open conversation.
func (m CMailModel) CancelUserConvsSubscription() CMailModel {
	if m.userConvsSub != nil {
		m.userConvsSub.cancel()
		m.userConvsSub = nil
	}
	if m.userConvsReconnectCancel != nil {
		m.userConvsReconnectCancel()
		m.userConvsReconnectCancel = nil
	}
	m.userConvsReconnecting = false
	m.userConvsReconnectAttempt = 0
	return m
}

func (m CMailModel) openUserConvsSubscriptionCmd(initial []model.Conversation) tea.Cmd {
	client := m.client
	uid := m.currentUserID
	return func() tea.Msg {
		if client == nil || uid == "" {
			return nil
		}
		ch, cancel, err := client.SubscribeUserConversations(context.Background(), uid, initial)
		if err != nil {
			return nil
		}
		return userConvsSubscribedMsg{sub: &userConvsSubscription{C: ch, cancel: cancel}}
	}
}

// reconnectUserConvsCmd makes one reconnect attempt after the account-wide
// conversation-list stream closed — refreshes the session token and reopens
// the subscription, seeded with the last known-good list. Mirrors
// reconnectConvCmd's shape.
func (m CMailModel) reconnectUserConvsCmd(ctx context.Context, attempt int) tea.Cmd {
	client := m.client
	uid := m.currentUserID
	initial := m.conversations
	return func() tea.Msg {
		if client == nil || uid == "" {
			return nil
		}
		ch, cancel, err := attemptReconnect(client, ctx, func(ctx context.Context) (<-chan []model.Conversation, context.CancelFunc, error) {
			return client.SubscribeUserConversations(ctx, uid, initial)
		})
		if err != nil {
			return userConvsReconnectFailedMsg{attempt: attempt, err: err}
		}
		return userConvsReconnectedMsg{sub: &userConvsSubscription{C: ch, cancel: cancel}}
	}
}

// scheduleUserConvsReconnectRetryCmd waits out the backoff for attempt, then
// emits a userConvsReconnectRetryDueMsg to trigger the next reconnect attempt.
func scheduleUserConvsReconnectRetryCmd(attempt int) tea.Cmd {
	return tea.Tick(reconnectDelay(attempt), func(time.Time) tea.Msg {
		return userConvsReconnectRetryDueMsg{attempt: attempt}
	})
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
		return dmSubscribedMsg{convID: convID, sub: &dmSubscription{ConvID: convID, C: ch, cancel: cancel}}
	}
}

// announceTypingCmd announces that the caller is typing in convID.
func (m CMailModel) announceTypingCmd(convID string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		if client == nil {
			return nil
		}
		heartbeatMs, staleAfterMs, err := client.AnnounceTyping(convID)
		if err != nil {
			return nil // typing is supplementary; a failed announce just means no indicator shown to the peer
		}
		return typingAnnouncedMsg{convID: convID, heartbeatMs: heartbeatMs, staleAfterMs: staleAfterMs}
	}
}

// sendTypingHeartbeatCmd re-announces typing for convID as a best-effort
// background heartbeat; the result is discarded (failures fail silently,
// matching typing's supplementary status).
func (m CMailModel) sendTypingHeartbeatCmd(convID string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		if client != nil {
			_, _, _ = client.AnnounceTyping(convID)
		}
		return nil
	}
}

// scheduleTypingHeartbeatCmd self-reschedules a heartbeat tick every
// heartbeatMs, the same self-rescheduling tea.Tick shape as chatrooms.go's
// scheduleHeartbeatCmd.
func scheduleTypingHeartbeatCmd(convID string, heartbeatMs int) tea.Cmd {
	return tea.Tick(time.Duration(heartbeatMs)*time.Millisecond, func(time.Time) tea.Msg {
		return typingHeartbeatTickMsg{convID: convID}
	})
}

// scheduleTypingAnimCmd self-reschedules the indicator's merged tick every
// dmTypingAnimInterval: the dot-animation frame counter and the local
// idle-check (comparing lastKeystrokeAt against dmTypingIdleThreshold) both
// run off this one tea.Tick, coalesced from two independent same-cadence
// chains — see the typingAnimTickMsg handler. Free-runs for as long as a
// conversation is open, independent of whether anyone is actually typing —
// same always-on shape as this screen's textinput.Blink cursor animation.
func scheduleTypingAnimCmd(convID string) tea.Cmd {
	return tea.Tick(dmTypingAnimInterval, func(time.Time) tea.Msg {
		return typingAnimTickMsg{convID: convID}
	})
}

// clearTypingCmd sends a best-effort explicit clear; the result is discarded
// (the peer's view corrects itself via staleAfterMs expiry even if this
// fails or is never sent).
func clearTypingCmd(client api.Client, convID string) tea.Cmd {
	return func() tea.Msg {
		if client != nil {
			_ = client.ClearTyping(convID)
		}
		return nil
	}
}

// openTypingSubscriptionCmd opens the dm_presence read stream using the
// fixed, documented staleAfterMs (dmTypingDefaultStaleAfterMs) rather than a
// server-negotiated value — this subscribes before the caller necessarily
// calls AnnounceTyping themselves (they may just be reading).
func (m CMailModel) openTypingSubscriptionCmd(convID string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		if client == nil {
			return nil
		}
		ch, cancel, err := client.SubscribeDMTyping(context.Background(), convID, dmTypingDefaultStaleAfterMs)
		if err != nil {
			return nil
		}
		return dmTypingSubscribedMsg{convID: convID, sub: &dmTypingSubscription{ConvID: convID, C: ch, cancel: cancel}}
	}
}

// reconnectTypingCmd makes one reconnect attempt for the typing-presence read
// subscription after it closed, mirroring reconnectConvCmd: refreshes the
// session token first (unlike openTypingSubscriptionCmd's initial subscribe,
// which doesn't need to) and reports which attempt number just failed so
// Update can back off and retry or give up.
func (m CMailModel) reconnectTypingCmd(ctx context.Context, convID string, attempt int) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		if client == nil {
			return nil
		}
		ch, cancel, err := attemptReconnect(client, ctx, func(ctx context.Context) (<-chan []model.TypingUser, context.CancelFunc, error) {
			return client.SubscribeDMTyping(ctx, convID, dmTypingDefaultStaleAfterMs)
		})
		if err != nil {
			return dmTypingReconnectFailedMsg{convID: convID, attempt: attempt, err: err}
		}
		return dmTypingReconnectedMsg{sub: &dmTypingSubscription{ConvID: convID, C: ch, cancel: cancel}}
	}
}

// scheduleTypingReconnectRetryCmd waits out the backoff for attempt, then
// emits a dmTypingReconnectRetryDueMsg to trigger the next reconnect attempt.
func scheduleTypingReconnectRetryCmd(convID string, attempt int) tea.Cmd {
	return tea.Tick(reconnectDelay(attempt), func(time.Time) tea.Msg {
		return dmTypingReconnectRetryDueMsg{convID: convID, attempt: attempt}
	})
}

// reconnectConvCmd makes one reconnect attempt for convID after the live
// stream closed (idToken expiry, idle-timeout, or a network error) — refreshes
// the session token and reopens the RTDB subscription. On failure it reports
// which attempt number just failed so Update can decide whether to back off
// and retry or give up.
func (m CMailModel) reconnectConvCmd(ctx context.Context, convID string, attempt int) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		if client == nil {
			return nil
		}
		ch, cancel, err := attemptReconnect(client, ctx, func(ctx context.Context) (<-chan model.Message, context.CancelFunc, error) {
			return client.SubscribeDMs(ctx, convID)
		})
		if err != nil {
			return dmReconnectFailedMsg{convID: convID, attempt: attempt, err: err}
		}
		return dmReconnectedMsg{sub: &dmSubscription{ConvID: convID, C: ch, cancel: cancel}}
	}
}

// scheduleReconnectRetryCmd waits out the backoff for attempt, then emits a
// dmReconnectRetryDueMsg to trigger the next reconnect attempt.
func scheduleReconnectRetryCmd(convID string, attempt int) tea.Cmd {
	return tea.Tick(reconnectDelay(attempt), func(time.Time) tea.Msg {
		return dmReconnectRetryDueMsg{convID: convID, attempt: attempt}
	})
}

func (m CMailModel) loadConvMessagesCmd(convID string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		if client == nil {
			return nil
		}
		msgs, err := client.GetMessages(convID, 50, 0)
		if err != nil {
			return cmailErrMsg{err}
		}
		return cmailMsgsLoadedMsg{convID: convID, msgs: msgs}
	}
}

// loadOlderConvMessagesCmd fetches the page of messages preceding before (ms epoch).
func (m CMailModel) loadOlderConvMessagesCmd(convID string, before int64) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		if client == nil {
			return nil
		}
		msgs, err := client.GetMessages(convID, 50, before)
		if err != nil {
			return cmailErrMsg{err}
		}
		return cmailOlderMsgsLoadedMsg{convID: convID, msgs: msgs}
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

// SetCanGoBack marks whether the active conversation was reached via a deep
// link, which determines whether ESC in detail mode leaves C-Mail entirely
// (see canGoBack field doc).
func (m CMailModel) SetCanGoBack(v bool) CMailModel {
	m.canGoBack = v
	return m
}

// SetAnimPaused pauses (or resumes) the styleAnimTickMsg re-render — see the
// animPaused field doc comment. Called from app.go when the image modal
// opens/closes.
func (m CMailModel) SetAnimPaused(paused bool) CMailModel {
	m.animPaused = paused
	return m
}

// AnimPaused reports whether the styleAnimTickMsg re-render is currently
// paused (see the animPaused field doc comment).
func (m CMailModel) AnimPaused() bool { return m.animPaused }

// ResetToList clears any deep-link flag and drops back to the conversation
// list, for ordinary tab navigation into C-Mail that may still have a
// deep-linked conversation open (SetCanGoBack(false) alone left mode stuck
// on detail).
func (m CMailModel) ResetToList() CMailModel {
	m.canGoBack = false
	m.mode = cmailModeList
	return m
}

// SetActiveConversation opens a specific conversation (used by external callers).
// Cancels any existing DM subscription and sets activeConvID so that the
// dmSubscribedMsg / cmailMsgsLoadedMsg handlers accept the new conversation's results.
func (m CMailModel) SetActiveConversation(conv model.Conversation) CMailModel {
	m = m.cancelDMSub()
	for i := range m.conversations {
		if m.conversations[i].ID == conv.ID {
			m.conversations[i].UnreadCount = 0
			conv = m.conversations[i]
			break
		}
	}
	m.activeConvID = conv.ID
	m.activeConv = &conv
	m.mode = cmailModeDetail
	m.historyExhausted = false
	m.loadingHistory = false
	m.err = nil
	m.input.Focus()
	m.selectedMsgID = ""
	m.imageRealRows = nil
	if m.ready {
		m = m.refreshMessages()
		m.viewport.GotoBottom()
	}
	return m
}

// TotalUnread sums UnreadCount across all conversations for the tab-bar badge.
func (m CMailModel) TotalUnread() int {
	total := 0
	for _, c := range m.conversations {
		total += c.UnreadCount
	}
	return total
}

// SetFocused marks whether the C-Mail tab is the one currently on screen.
// Becoming focused clears the currently-open conversation's local unread
// count (mirroring ChatroomsModel.SetFocused), so TotalUnread() doesn't keep
// counting messages the user is now actively viewing. It also clears
// styleAnimRunning: styleAnimTickMsg isn't in IsDMStreamMsg, so a tick that
// fires while this tab is backgrounded is dropped before updateInner ever
// resets the flag, permanently blocking maybeStartStyleAnim from restarting
// the ticker. Regaining focus is the point where any prior ticker is known
// to be dead, so it's safe to clear the flag here.
func (m CMailModel) SetFocused(focused bool) CMailModel {
	m.focused = focused
	if focused {
		m = m.zeroActiveConvUnread()
		m.styleAnimRunning = false
	}
	return m
}

// zeroActiveConvUnread clears UnreadCount on the m.conversations entry
// matching the currently open conversation, if any.
func (m CMailModel) zeroActiveConvUnread() CMailModel {
	for i := range m.conversations {
		if m.conversations[i].ID == m.activeConvID {
			m.conversations[i].UnreadCount = 0
			break
		}
	}
	return m
}

// bumpActiveConvUnread increments UnreadCount on the m.conversations entry
// matching the currently open conversation, mirroring ChatroomsModel's
// `if !m.focused { m.unreadCount++ }` — except here the target is the
// existing per-conversation UnreadCount TotalUnread() already sums, not a
// separate counter, so the tab-bar badge reflects it immediately instead of
// waiting for the next 60s poll.
func (m CMailModel) bumpActiveConvUnread() CMailModel {
	for i := range m.conversations {
		if m.conversations[i].ID == m.activeConvID {
			m.conversations[i].UnreadCount++
			break
		}
	}
	return m
}

// HasLiveConv reports whether a conversation is currently open in detail
// mode with its subscription state intact — used by activateScreen to
// decide whether re-entering the C-Mail tab should resume in place instead
// of resetting to the conversation list.
func (m CMailModel) HasLiveConv() bool {
	return m.mode == cmailModeDetail && m.activeConv != nil
}

// ComposeEmpty reports whether the compose input has no typed text — used by
// App to let plain left/right fall through to tab-cycling instead of being
// captured as cursor movement (see handleKeys' focused-input gate in app.go).
func (m CMailModel) ComposeEmpty() bool { return m.input.Value() == "" }

// SelectedMessageID returns the currently browsing-selected message ID, or
// "" while composing/typing.
func (m CMailModel) SelectedMessageID() string { return m.selectedMsgID }

// ConvOpenCmds returns the batch command to load message history and open the
// live RTDB subscription for convID. Call after SetActiveConversation.
func (m CMailModel) ConvOpenCmds(convID string) tea.Cmd {
	cmds := []tea.Cmd{
		m.loadConvMessagesCmd(convID),
		m.openDMSubscriptionCmd(convID),
	}
	if m.typingIndicatorsEnabled {
		cmds = append(cmds, m.openTypingSubscriptionCmd(convID), scheduleTypingAnimCmd(convID))
	}
	return tea.Batch(cmds...)
}

// InputFocused returns true in detail mode to prevent tab-navigation key capture.
func (m CMailModel) InputFocused() bool { return m.mode == cmailModeDetail }

// IsShowingDetail reports whether the detail view (history + input) is active.
func (m CMailModel) IsShowingDetail() bool { return m.mode == cmailModeDetail }

// GetFocusedURLs returns URLs found in the currently selected message while
// browsing, or across all currently loaded messages in the open
// conversation otherwise, for the 'o' / ctrl+o open-link shortcut. Reachable
// via ctrl+o even while the compose input is focused, which it always is in
// detail mode outside of browsing.
func (m CMailModel) GetFocusedURLs() []string {
	if m.mode != cmailModeDetail || m.activeConv == nil {
		return nil
	}
	if m.selectedMsgID != "" {
		if msg, ok := findMessageByID(m.activeConv.Messages, m.selectedMsgID); ok {
			return dedupeURLs(messageURLs(msg))
		}
		return nil
	}
	var urls []string
	for _, msg := range m.activeConv.Messages {
		urls = append(urls, messageURLs(msg)...)
	}
	return dedupeURLs(urls)
}

// SelectedConv returns the cursor index in the conversation list.
func (m CMailModel) SelectedConv() int { return m.selectedConv }

// HasActiveConv reports whether the detail view is currently open.
func (m CMailModel) HasActiveConv() bool { return m.mode == cmailModeDetail }

func (m CMailModel) Init() tea.Cmd { return textinput.Blink }

// Update processes msg via updateInner, then (re)arms the style-animation
// ticker if a loaded message needs one — same wrap-the-switch shape as
// chatrooms.go's Update/maybeStartStyleAnim, chosen there to avoid a
// cascading tea.Cmd-return change through refreshMessages' many call sites.
func (m CMailModel) Update(msg tea.Msg) (CMailModel, tea.Cmd) {
	m, cmd := m.updateInner(msg)
	var tickCmd tea.Cmd
	m, tickCmd = m.maybeStartStyleAnim()
	if tickCmd != nil {
		cmd = tea.Batch(cmd, tickCmd)
	}
	// Clear the unread badge the moment the view catches up to the bottom
	// while focused — see ChatroomsModel.Update's equivalent for why this
	// single choke point beats hunting down every path that can land back
	// at the bottom.
	if m.focused && m.mode == cmailModeDetail && m.viewport.AtBottom() {
		m = m.zeroActiveConvUnread()
	}
	return m, cmd
}

// maybeStartStyleAnim starts the slow/wave/glitch animation ticker if the
// open conversation has a loaded message that needs one and it isn't already
// running. Coarse-scoped: checks every loaded message in the conversation,
// not just ones visible in the viewport — see the plan's Trade-offs section.
func (m CMailModel) maybeStartStyleAnim() (CMailModel, tea.Cmd) {
	if m.styleAnimRunning || m.mode != cmailModeDetail || m.activeConv == nil {
		return m, nil
	}
	for _, msg := range m.activeConv.Messages {
		if hasAnimatedStyle(msg.Style) {
			m.styleAnimRunning = true
			return m, styleAnimTickCmd()
		}
	}
	return m, nil
}

func (m CMailModel) updateInner(msg tea.Msg) (CMailModel, tea.Cmd) {
	switch msg := msg.(type) {
	case styleAnimTickMsg:
		m.styleAnimFrame++
		m.styleAnimRunning = false
		if !m.animPaused && m.mode == cmailModeDetail && m.activeConv != nil {
			m = m.refreshMessages()
		}
		return m, nil

	case InsertIconMsg:
		if m.mode == cmailModeDetail {
			m.input = insertAtCursor(m.input, msg.Icon)
		}
		return m, nil

	case SetComposeValueMsg:
		if m.mode == cmailModeDetail {
			m.input.SetValue(msg.Value)
			m.input.CursorEnd()
		}
		return m, nil

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
				m = m.refreshMessages()
			}
		}
		// See the matching comment in chatrooms.go: textinput.View() renders
		// 3 columns wider than Width the instant there's any typed content
		// (Prompt's width plus the phantom end-of-line cursor glyph, neither
		// subtracted from its own padding math, unlike the empty-placeholder
		// render). Compensating here keeps the box from silently growing
		// wider than the header above it and pushing its right border
		// off-screen as soon as the user starts typing.
		m.input.Width = msg.Width - 4 - lipgloss.Width(m.input.Prompt) - 1

	case SharedConfigMsg:
		m.timeDisplayFormat = msg.Settings.TimeDisplayFormat
		imagesChanged := msg.InlineImagesEnabled != m.inlineImagesEnabled
		m.inlineImagesEnabled = msg.InlineImagesEnabled
		m = m.SetLocation(msg.Loc)
		if imagesChanged && m.activeConv != nil {
			m = m.refreshMessages()
		}
		// typingChanged handles a live toggle while a conversation is already
		// open — the merged tick's own typingIndicatorsEnabled guard stops the
		// animation side within one tick on disable, but the RTDB subscription
		// is push-based (nothing else would close it) and needs an explicit
		// cancel here, or it — and the "@x is typing" render it feeds — would
		// keep running until the conversation closes.
		typingChanged := msg.TypingIndicatorsEnabled != m.typingIndicatorsEnabled
		m.typingIndicatorsEnabled = msg.TypingIndicatorsEnabled
		var cmds []tea.Cmd
		if typingChanged && m.activeConv != nil {
			if m.typingIndicatorsEnabled {
				cmds = append(cmds, m.openTypingSubscriptionCmd(m.activeConv.ID), scheduleTypingAnimCmd(m.activeConv.ID))
			} else {
				if m.typingSub != nil {
					m.typingSub.cancel()
					m.typingSub = nil
				}
				if m.announcingTyping {
					m.announcingTyping = false
					cmds = append(cmds, clearTypingCmd(m.client, m.activeConv.ID))
				}
				m.typingUsers = nil
			}
		}
		return m, tea.Batch(cmds...)

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

	case cmailOlderMsgsLoadedMsg:
		return m.PrependMessages(msg.convID, msg.msgs), nil

	case dmReceivedMsg:
		m = m.AppendMessage(msg.msg)
		// Also counts as unread while focused if the view isn't at the
		// bottom (scrolled up reading history) — see ChatroomsModel's
		// equivalent for why this is reliable here too.
		if !m.focused || !m.viewport.AtBottom() {
			m = m.bumpActiveConvUnread()
		}
		if m.dmSub != nil {
			return m, waitForDM(m.dmSub)
		}
		return m, nil

	case dmStreamClosedMsg:
		if msg.convID != m.activeConvID {
			return m, nil // stale event from an already-abandoned subscription
		}
		m.dmSub = nil
		if m.mode != cmailModeDetail || m.activeConv == nil {
			return m, nil
		}
		if m.reconnectCancel != nil {
			m.reconnectCancel()
		}
		m.reconnectCtx, m.reconnectCancel = context.WithCancel(context.Background())
		m.reconnecting = true
		m.reconnectFailed = false
		m.reconnectAttempt = 0
		return m, m.reconnectConvCmd(m.reconnectCtx, m.activeConv.ID, 0)

	case dmReconnectFailedMsg:
		if msg.convID != m.activeConvID || !m.reconnecting {
			return m, nil // stale or cancelled sequence
		}
		next := msg.attempt + 1
		if next >= maxReconnectAttempts {
			m.reconnecting = false
			m.reconnectFailed = true
			return m, nil
		}
		m.reconnectAttempt = next
		return m, scheduleReconnectRetryCmd(msg.convID, next)

	case dmReconnectRetryDueMsg:
		if msg.convID != m.activeConvID || !m.reconnecting {
			return m, nil // stale or cancelled sequence
		}
		return m, m.reconnectConvCmd(m.reconnectCtx, msg.convID, msg.attempt)

	case dmReconnectedMsg:
		if msg.sub.ConvID != m.activeConvID {
			msg.sub.cancel()
			return m, nil
		}
		m.dmSub = msg.sub
		m.reconnecting = false
		m.reconnectFailed = false
		m.reconnectAttempt = 0
		return m, tea.Batch(waitForDM(m.dmSub), func() tea.Msg { return CMailReconnectedMsg{} })

	case cmailErrMsg:
		m.err = msg.err
		m.loadingHistory = false
		if m.ready && m.mode == cmailModeDetail {
			m = m.refreshMessages()
		}
		return m, nil

	// --- Account-wide conversation-list subscription lifecycle ---

	case userConvsSubscribedMsg:
		m.userConvsSub = msg.sub
		return m, waitForUserConvs(m.userConvsSub)

	case userConvsReceivedMsg:
		m = m.SetConversations(msg.convs)
		if m.userConvsSub != nil {
			return m, waitForUserConvs(m.userConvsSub)
		}
		return m, nil

	case userConvsStreamClosedMsg:
		m.userConvsSub = nil
		if m.userConvsReconnectCancel != nil {
			m.userConvsReconnectCancel()
		}
		m.userConvsReconnectCtx, m.userConvsReconnectCancel = context.WithCancel(context.Background())
		m.userConvsReconnecting = true
		m.userConvsReconnectAttempt = 0
		return m, m.reconnectUserConvsCmd(m.userConvsReconnectCtx, 0)

	case userConvsReconnectFailedMsg:
		if !m.userConvsReconnecting {
			return m, nil // stale or cancelled sequence
		}
		next := msg.attempt + 1
		if next >= maxReconnectAttempts {
			m.userConvsReconnecting = false
			return m, nil // give up silently — the list just stops updating live until next login
		}
		m.userConvsReconnectAttempt = next
		return m, scheduleUserConvsReconnectRetryCmd(next)

	case userConvsReconnectRetryDueMsg:
		if !m.userConvsReconnecting {
			return m, nil // stale or cancelled sequence
		}
		return m, m.reconnectUserConvsCmd(m.userConvsReconnectCtx, msg.attempt)

	case userConvsReconnectedMsg:
		m.userConvsSub = msg.sub
		m.userConvsReconnecting = false
		m.userConvsReconnectAttempt = 0
		return m, waitForUserConvs(m.userConvsSub)

	// --- Typing indicator lifecycle ---

	case typingAnnouncedMsg:
		if msg.convID != m.activeConvID {
			return m, nil // stale — left the conversation before this returned
		}
		m.typingHeartbeatMs = msg.heartbeatMs
		return m, scheduleTypingHeartbeatCmd(msg.convID, msg.heartbeatMs)

	case typingHeartbeatTickMsg:
		if msg.convID != m.activeConvID || !m.announcingTyping {
			return m, nil // left the conversation or already stopped; let the tick chain die out
		}
		return m, tea.Batch(
			m.sendTypingHeartbeatCmd(msg.convID),
			scheduleTypingHeartbeatCmd(msg.convID, m.typingHeartbeatMs),
		)

	case typingAnimTickMsg:
		if msg.convID != m.activeConvID {
			return m, nil // conversation closed; let the tick chain die out
		}
		if !m.typingIndicatorsEnabled {
			return m, nil // user disabled typing indicators mid-session — chain dies here
		}
		m.typingAnimFrame++
		var cmds []tea.Cmd
		if m.announcingTyping && time.Since(m.lastKeystrokeAt) >= dmTypingIdleThreshold {
			m.announcingTyping = false
			cmds = append(cmds, clearTypingCmd(m.client, msg.convID))
		}
		cmds = append(cmds, scheduleTypingAnimCmd(msg.convID))
		return m, tea.Batch(cmds...)

	case dmTypingSubscribedMsg:
		if msg.convID != m.activeConvID {
			msg.sub.cancel()
			return m, nil
		}
		m.typingSub = msg.sub
		return m, waitForDMTyping(m.typingSub)

	case dmTypingReceivedMsg:
		m.typingUsers = msg.users
		if m.typingSub != nil {
			return m, waitForDMTyping(m.typingSub)
		}
		return m, nil

	case dmTypingStreamClosedMsg:
		if msg.convID != m.activeConvID {
			return m, nil // stale event from an already-abandoned subscription
		}
		m.typingSub = nil
		if m.mode != cmailModeDetail || m.activeConv == nil {
			return m, nil
		}
		if m.typingReconnectCancel != nil {
			m.typingReconnectCancel()
		}
		m.typingReconnectCtx, m.typingReconnectCancel = context.WithCancel(context.Background())
		m.typingReconnecting = true
		m.typingReconnectFailed = false
		m.typingReconnectAttempt = 0
		return m, m.reconnectTypingCmd(m.typingReconnectCtx, m.activeConv.ID, 0)

	case dmTypingReconnectFailedMsg:
		if msg.convID != m.activeConvID || !m.typingReconnecting {
			return m, nil // stale or cancelled sequence
		}
		next := msg.attempt + 1
		if next >= maxReconnectAttempts {
			m.typingReconnecting = false
			m.typingReconnectFailed = true
			return m, nil
		}
		m.typingReconnectAttempt = next
		return m, scheduleTypingReconnectRetryCmd(msg.convID, next)

	case dmTypingReconnectRetryDueMsg:
		if msg.convID != m.activeConvID || !m.typingReconnecting {
			return m, nil // stale or cancelled sequence
		}
		return m, m.reconnectTypingCmd(m.typingReconnectCtx, msg.convID, msg.attempt)

	case dmTypingReconnectedMsg:
		if msg.sub.ConvID != m.activeConvID {
			msg.sub.cancel()
			return m, nil
		}
		m.typingSub = msg.sub
		m.typingReconnecting = false
		m.typingReconnectFailed = false
		m.typingReconnectAttempt = 0
		return m, waitForDMTyping(m.typingSub)

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
			case "pgup":
				if m.selectedConv > 0 {
					m.selectedConv = max(0, m.selectedConv-pageJumpItems)
					if m.ready {
						m.listVP.SetContent(m.renderConvCards())
						m = m.ensureConvVisible()
					}
				}
				return m, nil
			case "pgdown":
				if m.selectedConv < len(m.conversations)-1 {
					m.selectedConv = min(len(m.conversations)-1, m.selectedConv+pageJumpItems)
					if m.ready {
						m.listVP.SetContent(m.renderConvCards())
						m = m.ensureConvVisible()
					}
				}
				return m, nil
			case "enter":
				if len(m.conversations) > 0 {
					m.conversations[m.selectedConv].UnreadCount = 0
					conv := m.conversations[m.selectedConv]
					m = m.cancelDMSub()
					m.activeConvID = conv.ID
					m.activeConv = &conv
					m.mode = cmailModeDetail
					m.historyExhausted = false
					m.loadingHistory = false
					m.err = nil
					m.input.Focus()
					m.selectedMsgID = ""
					if m.ready {
						m = m.refreshMessages()
						m.viewport.GotoBottom()
						m.listVP.SetContent(m.renderConvCards())
					}
					convID := conv.ID
					fetchOther := m.OtherProfileFetchTarget(conv)
					cmds := []tea.Cmd{
						m.loadConvMessagesCmd(conv.ID),
						m.openDMSubscriptionCmd(conv.ID),
						func() tea.Msg {
							return CMailConvSelectedMsg{ConversationID: convID, OtherUsername: fetchOther}
						},
					}
					if m.typingIndicatorsEnabled {
						cmds = append(cmds, m.openTypingSubscriptionCmd(conv.ID), scheduleTypingAnimCmd(conv.ID))
					}
					return m, tea.Batch(cmds...)
				}
				return m, nil
			}

		case cmailModeDetail:
			if m.selectedMsgID != "" {
				return m.updateCMailBrowsingKey(msg)
			}
			switch msg.String() {
			case "esc":
				var clearCmd tea.Cmd
				if m.announcingTyping && m.activeConv != nil {
					clearCmd = clearTypingCmd(m.client, m.activeConv.ID)
				}
				if m.canGoBack {
					m = m.cancelDMSub()
					m.canGoBack = false
					return m, tea.Batch(clearCmd, func() tea.Msg { return LeaveCMailMsg{} })
				}
				m = m.cancelDMSub()
				m.mode = cmailModeList
				m.activeConv = nil
				m.input.Blur()
				if m.ready {
					m.listVP.SetContent(m.renderConvCards())
				}
				return m, clearCmd
			case "enter":
				if m.activeConv != nil {
					val := m.input.Value()
					if val != "" {
						convID := m.activeConv.ID
						if strings.HasPrefix(val, "/") {
							cmd := strings.ToLower(strings.Fields(val)[0])
							if !isKnownSlashCommand(cmd, nil) {
								m.input.Reset()
								return m.AppendSystemMessage(convID, "*** unknown command: "+cmd), nil
							}
						}
						m.input.Reset()
						m.announcingTyping = false // server auto-clears typing on send; no DELETE needed
						return m, func() tea.Msg {
							return SendCMailMsg{ConversationID: convID, Body: val}
						}
					}
				}
				return m, nil
			case "up":
				var sel []int
				if m.activeConv != nil {
					sel = selectableMessageIndices(m.activeConv.Messages, nil)
				}
				if len(sel) == 0 {
					m.viewport.ScrollUp(1)
					return m.maybeLoadOlderConvMessages()
				}
				m.input.Blur()
				m.selectedMsgID = m.activeConv.Messages[sel[len(sel)-1]].ID
				m = m.refreshMessages()
				m = m.ensureSelectedMessageVisible()
				// Only fetch older history here if entering browsing landed
				// straight on the oldest message (a single-message
				// conversation) — otherwise pagination fires once curPos
				// reaches 0 while already browsing, mirroring cIRC's 'up'
				// entry point.
				if len(sel) == 1 {
					return m.maybeLoadOlderConvMessages()
				}
				return m, nil
			case "down":
				m.viewport.ScrollDown(1)
				return m, nil
			case "pgup":
				var sel []int
				if m.activeConv != nil {
					sel = selectableMessageIndices(m.activeConv.Messages, nil)
				}
				if len(sel) == 0 {
					m.viewport.ScrollUp(m.viewport.Height)
					return m.maybeLoadOlderConvMessages()
				}
				m.input.Blur()
				m.selectedMsgID = m.activeConv.Messages[sel[len(sel)-1]].ID
				m = m.refreshMessages()
				m = m.ensureSelectedMessageVisible()
				if len(sel) == 1 {
					return m.maybeLoadOlderConvMessages()
				}
				return m, nil
			case "pgdown":
				m.viewport.ScrollDown(m.viewport.Height)
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
		oldVal := m.input.Value()
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		var typingCmd tea.Cmd
		if newVal := m.input.Value(); newVal != oldVal && m.activeConv != nil {
			m, typingCmd = m.handleTypingInputChanged(newVal)
		}
		return m, tea.Batch(cmd, typingCmd)
	}

	// In list mode pass non-key messages (e.g. mouse scroll) to the list viewport.
	var vpCmd tea.Cmd
	m.listVP, vpCmd = m.listVP.Update(msg)
	return m, vpCmd
}

// updateCMailBrowsingKey handles keys while a message is selected
// (m.selectedMsgID != ""): up/down move the selection, esc returns to
// typing, 'p' views the sender's profile. Everything else is swallowed
// rather than typed, since the input is blurred for the duration of
// browsing — mirrors ChatroomsModel.updateBrowsingKey, trimmed to what
// CMail supports: no flag/delete (the API has no CMail message
// flag/delete endpoint) and no spoiler/l33t reveal (CMail's render
// pipeline doesn't support those styles).
func (m CMailModel) updateCMailBrowsingKey(msg tea.KeyMsg) (CMailModel, tea.Cmd) {
	if m.activeConv == nil {
		m.selectedMsgID = ""
		return m, nil
	}
	sel := selectableMessageIndices(m.activeConv.Messages, nil)
	curPos := selectablePos(m.activeConv.Messages, sel, m.selectedMsgID)
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
			return m.maybeLoadOlderConvMessages()
		}
		newPos, newOffset := millerPageNav(-1, m.viewport.Height, 0,
			selOffsets(m.msgOffsets, sel), selHeights(m.msgHeights, sel), curPos, m.viewport.YOffset)
		if newPos < 0 {
			newPos = 0
		}
		m.selectedMsgID = m.activeConv.Messages[sel[newPos]].ID
		m.viewport.SetYOffset(newOffset)
		m = m.refreshMessages()
		if newPos == 0 {
			return m.maybeLoadOlderConvMessages()
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
		m.selectedMsgID = m.activeConv.Messages[sel[newPos]].ID
		m.viewport.SetYOffset(newOffset)
		return m.refreshMessages(), nil
	case "pgup":
		if curPos == 0 {
			return m.maybeLoadOlderConvMessages()
		}
		newPos, newOffset := curPos, m.viewport.YOffset
		for i := 0; i < m.viewport.Height && newPos > 0; i++ {
			newPos, newOffset = millerPageNav(-1, m.viewport.Height, 0,
				selOffsets(m.msgOffsets, sel), selHeights(m.msgHeights, sel), newPos, newOffset)
		}
		if newPos < 0 {
			newPos = 0
		}
		m.selectedMsgID = m.activeConv.Messages[sel[newPos]].ID
		m.viewport.SetYOffset(newOffset)
		m = m.refreshMessages()
		if newPos == 0 {
			return m.maybeLoadOlderConvMessages()
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
		m.selectedMsgID = m.activeConv.Messages[sel[newPos]].ID
		m.viewport.SetYOffset(newOffset)
		return m.refreshMessages(), nil
	case "p":
		targetMsg, ok := findMessageByID(m.activeConv.Messages, m.selectedMsgID)
		if !ok {
			return m, nil
		}
		username := targetMsg.From.Username
		return m, func() tea.Msg { return ShowUserProfileMsg{Username: username} }
	case "y":
		targetMsg, ok := findMessageByID(m.activeConv.Messages, m.selectedMsgID)
		if !ok {
			return m, nil
		}
		text := messageCopyText(targetMsg)
		return m, func() tea.Msg { return CopyMessageTextMsg{Text: text} }
	}
	return m, nil
}

// handleTypingInputChanged reacts to a compose-input value change: announces
// typing (once) on the transition from empty/idle to non-empty, clears
// immediately when backspaced to empty, and otherwise just bumps
// lastKeystrokeAt so the already-running heartbeat chain and the merged
// anim/idle-check tick (started at conv-open, see scheduleTypingAnimCmd)
// observe continued activity without spawning duplicate chains. A no-op
// entirely when typingIndicatorsEnabled is off.
func (m CMailModel) handleTypingInputChanged(newVal string) (CMailModel, tea.Cmd) {
	if !m.typingIndicatorsEnabled {
		return m, nil
	}
	convID := m.activeConv.ID
	if newVal == "" {
		if m.announcingTyping {
			m.announcingTyping = false
			return m, clearTypingCmd(m.client, convID)
		}
		return m, nil
	}
	m.lastKeystrokeAt = time.Now()
	if m.announcingTyping {
		return m, nil // heartbeat/anim-idle-check chains already alive
	}
	m.announcingTyping = true
	return m, m.announceTypingCmd(convID)
}

// OtherParticipant returns conv's other participant's username.
func (m CMailModel) OtherParticipant(conv model.Conversation) string {
	return m.otherParticipantUser(conv).Username
}

// OtherProfileFetchTarget returns the username CMailConvSelectedMsg should
// set as OtherUsername to trigger a profile fetch for conv's other
// participant, or "" if there's nothing to fetch — unresolvable ("unknown",
// empty conversation stub) or already cached (HasOtherProfile). Both
// emission sites (opening an existing conversation, starting a new one)
// call this so the "should we fetch" decision lives in one place.
func (m CMailModel) OtherProfileFetchTarget(conv model.Conversation) string {
	other := m.OtherParticipant(conv)
	if other == "" || other == "unknown" || m.HasOtherProfile(other) {
		return ""
	}
	return other
}

// otherParticipantUser is OtherParticipant's full-user counterpart, used
// where the detail header needs more than just the name (e.g. badge icons).
// This is always the thin shape conversation data actually carries (no
// SupporterIcon/GuildIcon/IsSupporter) — see otherParticipantBadgeUser for
// the version that overlays a fetched full profile when available.
func (m CMailModel) otherParticipantUser(conv model.Conversation) model.User {
	for _, u := range conv.Participants {
		if u.Username != "" && u.Username != m.currentUser {
			return u
		}
	}
	return model.User{Username: "unknown"}
}

// otherParticipantBadgeUser is otherParticipantUser overlaid with a fetched
// full profile from m.otherProfiles, if one has been fetched (see
// SetOtherProfile) — its SupporterIcon/IsSupporter are what the header badge
// actually renders from, since conversation data alone never carries them.
// Falls back to the thin otherParticipantUser (no badge codes) until fetched.
func (m CMailModel) otherParticipantBadgeUser(conv model.Conversation) model.User {
	u := m.otherParticipantUser(conv)
	if full, ok := m.otherProfiles[u.Username]; ok {
		return full
	}
	return u
}

// HasOtherProfile reports whether username's full profile has already been
// fetched (see SetOtherProfile) — App checks this before firing another
// fetch for the same conversation partner.
func (m CMailModel) HasOtherProfile(username string) bool {
	_, ok := m.otherProfiles[username]
	return ok
}

// SetOtherProfile caches username's full profile (fetched via GetProfile so
// the C-Mail header badge can show their real SupporterIcon — conversation
// data itself never carries it, see otherParticipantUser). Never evicted:
// bounded by the number of distinct people DMed in a session, which is small.
func (m CMailModel) SetOtherProfile(username string, u model.User) CMailModel {
	if m.otherProfiles == nil {
		m.otherProfiles = make(map[string]model.User)
	}
	m.otherProfiles[username] = u
	return m
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
		other := m.OtherParticipant(c)

		// Left side: @username
		nameStr := theme.Highlight.Render("@" + other)

		// Right side: date  (N)
		var rightParts []string
		if !c.LastMessageAt.IsZero() && c.LastMessageAt.Unix() != 0 {
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
		if m.err != nil {
			return theme.Subtle.Render("couldn't load messages")
		}
		return theme.Subtle.Render("no messages")
	}
	return renderChatMessagesStyled(m.activeConv.Messages, m.currentUser, m.location(), m.timeDisplayFormat, m.viewport.Width, m.styleAnimFrame)
}

// refreshMessages re-renders the active conversation's messages into
// m.viewport, tracking each message's line offset/height for browsing
// navigation and highlighting m.selectedMsgID — mirroring
// ChatroomsModel.refreshMessages.
func (m CMailModel) refreshMessages() CMailModel {
	if m.activeConv == nil || len(m.activeConv.Messages) == 0 {
		m.viewport.SetContent(m.renderMessages())
		m.msgOffsets, m.msgHeights = nil, nil
		return m
	}
	content, offsets, heights, imgSlots := renderChatMessagesWithSelection(
		m.activeConv.Messages, m.currentUser, m.location(), m.timeDisplayFormat, m.viewport.Width, m.styleAnimFrame, m.selectedMsgID, m.inlineImagesEnabled, m.imageRealRows, m.chatBodyCache)
	m.viewport.SetContent(content)
	m.msgOffsets, m.msgHeights = offsets, heights
	m.msgImages = imgSlots
	return m
}

// SetImageRealRows records key's actual fetched/fitted row count and, if it
// changed, re-renders so the reserved band shrinks to the image's real
// size, re-homing the viewport to the bottom if it was already there
// before the reflow — see ChatroomsModel.SetImageRealRows.
func (m CMailModel) SetImageRealRows(key string, rows int) CMailModel {
	if m.imageRealRows[key] == rows {
		return m
	}
	if m.imageRealRows == nil {
		m.imageRealRows = make(map[string]int)
	}
	m.imageRealRows[key] = rows
	if m.activeConv != nil && m.ready {
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
func (m CMailModel) VisibleInlineImages() []InlineImageSlot {
	if m.activeConv == nil || !m.inlineImagesEnabled {
		return nil
	}
	top, bottom := m.viewport.YOffset, m.viewport.YOffset+m.viewport.Height

	var slots []InlineImageSlot

	badgeUser := m.otherParticipantBadgeUser(*m.activeConv)
	badges := userBadgeCodes(badgeUser)
	if len(badges) > 0 {
		headerW := lipgloss.Width(theme.Title.Render("@" + badgeUser.Username))
		for i, code := range badges {
			col := headerW + i*(badgeIconCols+1) + 1
			if slot, ok := badgeSlot(code, 0, col, fmt.Sprintf("cmail:%s:badge:%d", badgeUser.Username, i)); ok {
				slots = append(slots, slot)
			}
		}
	}

	for i, msg := range m.activeConv.Messages {
		if i >= len(m.msgImages) || i >= len(m.msgOffsets) {
			continue
		}
		for _, img := range m.msgImages[i] {
			abs := m.msgOffsets[i] + img.Line
			key := cmailMsgImageKey(msg.ID)
			// See ChatroomsModel.VisibleInlineImages' equivalent comment:
			// this must be the actual reserved image-row allowance for
			// this message, not the old fixed inlineImageMaxRows, which
			// over-required clearance and hid the last message's image.
			imgRows := chatImageBandRows(m.imageRealRows, key) - 1
			if abs < top || abs+imgRows > bottom {
				continue
			}
			slots = append(slots, InlineImageSlot{
				URL: img.URL,
				// See ChatroomsModel.VisibleInlineImages' equivalent comment:
				// abs-top is relative to the viewport's own top edge, but
				// View() stacks header+divider (cmailDetailHeaderRows) above
				// the viewport within this screen's own content.
				Row:       abs - top + cmailDetailHeaderRows,
				ColIndent: 2,
				MaxCols:   m.viewport.Width - 4,
				MaxRows:   inlineImageEncodeMaxRows,
				Key:       key,
			})
		}
	}
	return slots
}

// ensureSelectedMessageVisible scrolls the viewport the minimum amount so the
// selected message is fully visible.
func (m CMailModel) ensureSelectedMessageVisible() CMailModel {
	if !m.ready || m.activeConv == nil {
		return m
	}
	m.viewport = ensureMessageVisible(m.viewport, m.activeConv.Messages, m.msgOffsets, m.msgHeights, m.selectedMsgID)
	return m
}

// maybeLoadOlderConvMessages fires the older-history fetch once scrolled to
// the very top of loaded messages, mirroring ChatroomsModel.maybeLoadOlderMessages.
func (m CMailModel) maybeLoadOlderConvMessages() (CMailModel, tea.Cmd) {
	if m.viewport.AtTop() && !m.loadingHistory && !m.historyExhausted &&
		m.activeConv != nil && len(m.activeConv.Messages) > 0 {
		m.loadingHistory = true
		before := m.activeConv.Messages[0].CreatedAt.UnixMilli()
		return m, m.loadOlderConvMessagesCmd(m.activeConv.ID, before)
	}
	return m, nil
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
			m = m.refreshMessages()
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
	m.err = nil
	m.selectedMsgID = ""
	m = m.evictStaleBodyCache()
	if m.ready {
		m = m.refreshMessages()
		m.viewport.GotoBottom()
	}
	return m
}

// AppendMessage adds a live incoming message to the currently open conversation.
// No-op when no conversation is open. Only follows the view down to the new
// message if it was already at the bottom beforehand — never yanks a
// scrolled-up reader down to the newest message, regardless of who sent it
// or what it contains — mirroring ChatroomsModel.AppendMessage.
func (m CMailModel) AppendMessage(msg model.Message) CMailModel {
	if m.activeConv == nil {
		return m
	}
	conv := *m.activeConv
	conv.Messages = append(conv.Messages, msg)
	m.activeConv = &conv
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

// AppendSystemMessage adds a local-only notice (e.g. a /help reply) to the
// currently open conversation. Never sent to the server. No-op if convID
// doesn't match the active conversation.
func (m CMailModel) AppendSystemMessage(convID, text string) CMailModel {
	if m.activeConv == nil || m.activeConv.ID != convID {
		return m
	}
	return m.AppendMessage(model.Message{
		From:      model.User{Username: "system"},
		Body:      text,
		CreatedAt: time.Now(),
		IsSystem:  true,
	})
}

// PrependMessages inserts an older page of history above the currently loaded
// messages, preserving the user's scroll position rather than jumping.
// No-op if convID doesn't match the active conversation.
func (m CMailModel) PrependMessages(convID string, msgs []model.Message) CMailModel {
	m.loadingHistory = false
	if m.activeConv == nil || m.activeConv.ID != convID {
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
	conv := *m.activeConv
	conv.Messages = append(msgs, conv.Messages...)
	m.activeConv = &conv
	if m.ready {
		newLines := lipgloss.Height(m.renderMessages())
		m = m.refreshMessages()
		m.viewport.SetYOffset(oldOffset + newLines - oldLines)
	}
	return m
}

// trimMessageBuffer evicts oldest messages from the active conversation's
// history while the estimated total size exceeds chatMessageBufferMaxBytes,
// always keeping at least the most recent message. Mirrors
// ChatroomsModel.trimMessageBuffer — same const and estimatedMessageSize
// helper, just operating on m.activeConv.Messages. Clears m.selectedMsgID if
// the evicted range contained the current selection, and resets
// m.historyExhausted so a later scroll-to-top re-fetches the evicted range
// from the server instead of treating it as permanently gone — the
// pagination cursor is activeConv.Messages[0] (see
// maybeLoadOlderConvMessages), so this is always safe to re-trigger.
func (m CMailModel) trimMessageBuffer() CMailModel {
	if m.activeConv == nil {
		return m
	}
	msgs := m.activeConv.Messages
	total := 0
	for _, msg := range msgs {
		total += estimatedMessageSize(msg)
	}
	i := 0
	for total > chatMessageBufferMaxBytes && i < len(msgs)-1 {
		if msgs[i].ID != "" && msgs[i].ID == m.selectedMsgID {
			m.selectedMsgID = ""
		}
		total -= estimatedMessageSize(msgs[i])
		i++
	}
	if i > 0 {
		for j := 0; j < i; j++ {
			delete(m.chatBodyCache, msgs[j].ID)
		}
		conv := *m.activeConv
		conv.Messages = msgs[i:]
		m.activeConv = &conv
		m.historyExhausted = false
	}
	return m
}

// evictStaleBodyCache drops chatBodyCache entries for messages no longer
// present in the active conversation's history. Called from
// SetConversationMessages, whose wholesale replace of activeConv.Messages
// (conversation open/switch) is the point a message can permanently drop out
// of the loaded history — without this, chatBodyCache would keep every entry
// from every conversation ever opened this session. trimMessageBuffer
// handles the other eviction point (rolling history cap) directly, since it
// already knows exactly which messages it's dropping — mirrors
// ChatroomsModel.evictStaleBodyCache.
func (m CMailModel) evictStaleBodyCache() CMailModel {
	if m.activeConv == nil {
		return m
	}
	live := make(map[string]bool, len(m.activeConv.Messages))
	for _, msg := range m.activeConv.Messages {
		live[msg.ID] = true
	}
	for id := range m.chatBodyCache {
		if !live[id] {
			delete(m.chatBodyCache, id)
		}
	}
	return m
}

// typingIndicator returns " is typing" plus an animated 0-3 dot count if the
// other participant currently has a fresh entry in m.typingUsers, else "".
// The username itself is left out — it's appended right after the header's
// own "@other" title, so together they read as one sentence.
func (m CMailModel) typingIndicator() string {
	if m.activeConv == nil {
		return ""
	}
	other := m.OtherParticipant(*m.activeConv)
	for _, u := range m.typingUsers {
		if u.Username == other {
			dots := strings.Repeat(".", m.typingAnimFrame%4)
			return theme.Subtle.Render(" is typing" + dots)
		}
	}
	return ""
}

func (m CMailModel) View() string {
	switch m.mode {
	case cmailModeDetail:
		if !m.ready {
			return ""
		}
		other := ""
		var badgeUser model.User
		if m.activeConv != nil {
			badgeUser = m.otherParticipantBadgeUser(*m.activeConv)
			other = badgeUser.Username
		}
		header := theme.Title.Render("@" + other)
		if m.inlineImagesEnabled {
			if n := len(userBadgeCodes(badgeUser)); n > 0 {
				header += badgeGap(n)
			}
		}
		if m.loadingHistory {
			header += theme.Subtle.Render("  (loading history…)")
		}
		switch {
		case m.reconnecting:
			header += theme.Highlight.Render(fmt.Sprintf("  (live updates lost, reconnecting… %d/%d)", m.reconnectAttempt+1, maxReconnectAttempts))
		case m.reconnectFailed:
			header += theme.Error.Render("  (live updates lost)")
		}
		header += m.typingIndicator()
		// textinput.View()'s empty-input placeholder path (placeholderView(),
		// internal) totals its render at exactly Width, unlike the
		// typed-content path — Width+len(Prompt)+1 (the whole reason
		// input.Width is pre-reduced by that amount; see the matching
		// comment on input.Width above). Hand-building this state, mirroring
		// the same fix in chatrooms.go, guarantees the same total without
		// needing to separately verify a second internal quirk.
		var inputContent string
		if m.input.Value() == "" {
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
		divider := theme.Subtle.Render(strings.Repeat("─", max(m.width, 0)))
		return lipgloss.JoinVertical(lipgloss.Left, header, divider, m.viewport.View(), inputBox)
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
