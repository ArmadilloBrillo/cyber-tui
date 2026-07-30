package screens

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ragnar/cyber-tui/internal/api"
	"github.com/ragnar/cyber-tui/internal/model"
)

// flakySubscribeClient makes SubscribeDMs/SubscribeRoom fail a fixed number
// of times before delegating to the embedded MockClient, for exercising the
// retry-with-backoff reconnect path.
type flakySubscribeClient struct {
	*api.MockClient
	subscribeFailures int32 // remaining failures, decremented atomically
}

func (c *flakySubscribeClient) SubscribeDMs(ctx context.Context, convID string) (<-chan model.Message, context.CancelFunc, error) {
	if atomic.AddInt32(&c.subscribeFailures, -1) >= 0 {
		return nil, nil, errors.New("boom")
	}
	return c.MockClient.SubscribeDMs(ctx, convID)
}

func (c *flakySubscribeClient) SubscribeRoom(ctx context.Context, roomID string) (<-chan model.Message, context.CancelFunc, error) {
	if atomic.AddInt32(&c.subscribeFailures, -1) >= 0 {
		return nil, nil, errors.New("boom")
	}
	return c.MockClient.SubscribeRoom(ctx, roomID)
}

func (c *flakySubscribeClient) SubscribeRoomPresence(ctx context.Context, roomID string, staleAfterMs int, initial []model.RoomUser) (<-chan []model.RoomUser, context.CancelFunc, error) {
	if atomic.AddInt32(&c.subscribeFailures, -1) >= 0 {
		return nil, nil, errors.New("boom")
	}
	return c.MockClient.SubscribeRoomPresence(ctx, roomID, staleAfterMs, initial)
}

// failingRefreshClient always fails RefreshSession, for exercising give-up
// after exhausting reconnect attempts.
type failingRefreshClient struct {
	*api.MockClient
}

func (c *failingRefreshClient) RefreshSession() error {
	return errors.New("refresh failed")
}

// withShortBackoff overrides reconnectBackoffSchedule with near-zero delays
// for the duration of a test, restored via t.Cleanup.
func withShortBackoff(t *testing.T) {
	t.Helper()
	orig := reconnectBackoffSchedule
	reconnectBackoffSchedule = []time.Duration{
		time.Millisecond, time.Millisecond, time.Millisecond, time.Millisecond, time.Millisecond,
	}
	t.Cleanup(func() { reconnectBackoffSchedule = orig })
}

// --- CIRC ---

func TestChatroomsReconnect_StaleEventIgnored(t *testing.T) {
	m := NewChatroomsModel("neo", api.NewMockClient())
	m.activeRoomID = "zion"
	m.mode = chatroomModeDetail

	_, cmd := m.Update(roomStreamClosedMsg{roomID: "old-abandoned-room"})
	if cmd != nil {
		t.Error("expected no reconnect command for a stale stream-closed event")
	}
}

func TestChatroomsReconnect_NotTriggeredOutsideDetailMode(t *testing.T) {
	m := NewChatroomsModel("neo", api.NewMockClient())
	m.activeRoomID = "zion"
	m.mode = chatroomModeList

	_, cmd := m.Update(roomStreamClosedMsg{roomID: "zion"})
	if cmd != nil {
		t.Error("expected no reconnect command when not viewing a room's detail")
	}
}

func TestChatroomsReconnect_SucceedsForActiveRoom(t *testing.T) {
	room := model.Room{ID: "r1", Slug: "zion", Name: "Zion"}
	m := NewChatroomsModel("neo", api.NewMockClient())
	m.activeRoomID = "zion"
	m.activeRoom = &room
	m.mode = chatroomModeDetail

	_, cmd := m.Update(roomStreamClosedMsg{roomID: "zion"})
	if cmd == nil {
		t.Fatal("expected a reconnect command")
	}
	msg := cmd()
	reconnected, ok := msg.(roomReconnectedMsg)
	if !ok {
		t.Fatalf("expected roomReconnectedMsg, got %T", msg)
	}
	if reconnected.sub == nil || reconnected.sub.RoomID != "zion" {
		t.Fatalf("expected reconnected sub for room zion, got %+v", reconnected.sub)
	}

	m2, cmd2 := m.Update(reconnected)
	if m2.sub != reconnected.sub {
		t.Error("expected m.sub to be set to the reconnected subscription")
	}
	if cmd2 == nil {
		t.Error("expected a batch command (resume waiting + reconnected toast) after reconnect")
	}
}

func TestChatroomsReconnect_RetriesWithBackoffThenSucceeds(t *testing.T) {
	withShortBackoff(t)

	client := &flakySubscribeClient{MockClient: api.NewMockClient(), subscribeFailures: 2}
	room := model.Room{ID: "r1", Slug: "zion", Name: "Zion"}
	m := NewChatroomsModel("neo", client)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.activeRoomID = "zion"
	m.activeRoom = &room
	m.mode = chatroomModeDetail

	m, cmd := m.Update(roomStreamClosedMsg{roomID: "zion"})
	if !m.reconnecting {
		t.Fatal("expected reconnecting to be true after stream closed")
	}

	var succeeded bool
	for i := 0; i < 10 && cmd != nil && !succeeded; i++ {
		msg := cmd()
		m, cmd = m.Update(msg)
		if _, ok := msg.(roomReconnectedMsg); ok {
			succeeded = true
		}
	}

	if !succeeded {
		t.Fatal("expected reconnect to eventually succeed")
	}
	if m.reconnecting || m.reconnectFailed {
		t.Errorf("expected reconnect state cleared after success, got reconnecting=%v reconnectFailed=%v", m.reconnecting, m.reconnectFailed)
	}
	if m.sub == nil {
		t.Error("expected sub to be set after successful reconnect")
	}
}

func TestChatroomsReconnect_GivesUpAfterMaxAttempts(t *testing.T) {
	withShortBackoff(t)

	client := &failingRefreshClient{MockClient: api.NewMockClient()}
	room := model.Room{ID: "r1", Slug: "zion", Name: "Zion"}
	m := NewChatroomsModel("neo", client)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.activeRoomID = "zion"
	m.activeRoom = &room
	m.mode = chatroomModeDetail

	m, cmd := m.Update(roomStreamClosedMsg{roomID: "zion"})
	for i := 0; i < 20 && cmd != nil && !m.reconnectFailed; i++ {
		msg := cmd()
		m, cmd = m.Update(msg)
	}

	if !m.reconnectFailed {
		t.Fatal("expected reconnectFailed to be true after exhausting all attempts")
	}
	if m.reconnecting {
		t.Error("expected reconnecting to be false once attempts are exhausted")
	}
	if m.sub != nil {
		t.Error("expected sub to remain nil after giving up")
	}
	if view := m.View(); !strings.Contains(view, "live updates lost") {
		t.Errorf("expected give-up indicator in view, got: %q", view)
	}
}

func TestChatroomsReconnect_CancelSubscriptionStopsRetrySequence(t *testing.T) {
	client := &failingRefreshClient{MockClient: api.NewMockClient()}
	room := model.Room{ID: "r1", Slug: "zion", Name: "Zion"}
	m := NewChatroomsModel("neo", client)
	m.activeRoomID = "zion"
	m.activeRoom = &room
	m.mode = chatroomModeDetail

	m, cmd := m.Update(roomStreamClosedMsg{roomID: "zion"})
	if !m.reconnecting {
		t.Fatal("expected reconnecting to be true after stream closed")
	}
	failMsg := cmd()

	m = m.CancelSubscription()
	if m.reconnecting {
		t.Error("expected reconnecting to be false after CancelSubscription")
	}
	if m.reconnectCancel != nil {
		t.Error("expected reconnectCancel to be cleared after CancelSubscription")
	}

	// A late-arriving result for the cancelled sequence must be a no-op.
	m2, cmd2 := m.Update(failMsg)
	if cmd2 != nil {
		t.Error("expected no further command from a stale reconnect result after cancellation")
	}
	if m2.reconnecting || m2.reconnectFailed {
		t.Error("expected reconnect state to stay cleared after a stale result")
	}
}

func TestChatroomsReconnect_IgnoresResultForAbandonedRoom(t *testing.T) {
	m := NewChatroomsModel("neo", api.NewMockClient())
	m.activeRoomID = "sprawl" // user navigated to a different room in the meantime
	m.mode = chatroomModeDetail

	sub := &roomSubscription{RoomID: "zion", C: make(chan model.Message), cancel: func() {}}
	m2, cmd := m.Update(roomReconnectedMsg{sub: sub})
	if m2.sub != nil {
		t.Error("expected sub to stay nil when the reconnect result is for an abandoned room")
	}
	// cmd may be non-nil (it cancels the stale sub synchronously already), but must not
	// be the reconnected-toast batch.
	_ = cmd
}

func TestChatroomsPresenceReconnect_StaleEventIgnored(t *testing.T) {
	m := NewChatroomsModel("neo", api.NewMockClient())
	m.activeRoomID = "zion"
	m.mode = chatroomModeDetail

	_, cmd := m.Update(roomPresenceStreamClosedMsg{roomID: "old-abandoned-room"})
	if cmd != nil {
		t.Error("expected no reconnect command for a stale stream-closed event")
	}
}

// TestChatroomsPresenceReconnect_RefreshesSessionBeforeResubscribing guards
// the fix for the gap where presence reconnect used to resubscribe directly
// without refreshing the shared session token first (unlike the message
// stream's reconnect, which always has). failingRefreshClient's
// RefreshSession always errors, so seeing a roomPresenceReconnectFailedMsg
// here proves RefreshSession is actually called on this path.
func TestChatroomsPresenceReconnect_RefreshesSessionBeforeResubscribing(t *testing.T) {
	client := &failingRefreshClient{MockClient: api.NewMockClient()}
	room := model.Room{ID: "r1", Slug: "zion", Name: "Zion"}
	m := NewChatroomsModel("neo", client)
	m.activeRoomID = "zion"
	m.activeRoom = &room
	m.mode = chatroomModeDetail
	m.staleAfterMs = 180000

	_, cmd := m.Update(roomPresenceStreamClosedMsg{roomID: "zion"})
	if cmd == nil {
		t.Fatal("expected a reconnect command")
	}
	msg := cmd()
	failed, ok := msg.(roomPresenceReconnectFailedMsg)
	if !ok {
		t.Fatalf("expected roomPresenceReconnectFailedMsg (RefreshSession failing), got %T", msg)
	}
	if failed.roomID != "zion" || failed.attempt != 0 {
		t.Errorf("unexpected failed msg: %+v", failed)
	}
}

func TestChatroomsPresenceReconnect_SucceedsForActiveRoom(t *testing.T) {
	room := model.Room{ID: "r1", Slug: "zion", Name: "Zion"}
	m := NewChatroomsModel("neo", api.NewMockClient())
	m.activeRoomID = "zion"
	m.activeRoom = &room
	m.mode = chatroomModeDetail
	m.staleAfterMs = 180000
	m.roomUsers = []model.RoomUser{{UserID: "1", Username: "alice"}}

	_, cmd := m.Update(roomPresenceStreamClosedMsg{roomID: "zion"})
	if cmd == nil {
		t.Fatal("expected a reconnect command")
	}
	msg := cmd()
	reconnected, ok := msg.(roomPresenceReconnectedMsg)
	if !ok {
		t.Fatalf("expected roomPresenceReconnectedMsg, got %T", msg)
	}
	if reconnected.sub == nil || reconnected.sub.RoomID != "zion" {
		t.Fatalf("expected reconnected sub for room zion, got %+v", reconnected.sub)
	}

	m2, cmd2 := m.Update(reconnected)
	if m2.presenceSub != reconnected.sub {
		t.Error("expected presenceSub to be set to the reconnected subscription")
	}
	if cmd2 == nil {
		t.Error("expected a command to resume waiting on the reconnected subscription")
	}
}

func TestChatroomsPresenceReconnect_RetriesWithBackoffThenSucceeds(t *testing.T) {
	withShortBackoff(t)

	client := &flakySubscribeClient{MockClient: api.NewMockClient(), subscribeFailures: 2}
	room := model.Room{ID: "r1", Slug: "zion", Name: "Zion"}
	m := NewChatroomsModel("neo", client)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.activeRoomID = "zion"
	m.activeRoom = &room
	m.mode = chatroomModeDetail
	m.staleAfterMs = 180000

	m, cmd := m.Update(roomPresenceStreamClosedMsg{roomID: "zion"})
	if !m.presenceReconnecting {
		t.Fatal("expected presenceReconnecting to be true after stream closed")
	}

	var succeeded bool
	for i := 0; i < 10 && cmd != nil && !succeeded; i++ {
		msg := cmd()
		m, cmd = m.Update(msg)
		if _, ok := msg.(roomPresenceReconnectedMsg); ok {
			succeeded = true
		}
	}

	if !succeeded {
		t.Fatal("expected reconnect to eventually succeed")
	}
	if m.presenceReconnecting || m.presenceReconnectFailed {
		t.Errorf("expected reconnect state cleared after success, got presenceReconnecting=%v presenceReconnectFailed=%v", m.presenceReconnecting, m.presenceReconnectFailed)
	}
	if m.presenceSub == nil {
		t.Error("expected presenceSub to be set after successful reconnect")
	}
}

func TestChatroomsPresenceReconnect_GivesUpAfterMaxAttempts(t *testing.T) {
	withShortBackoff(t)

	client := &failingRefreshClient{MockClient: api.NewMockClient()}
	room := model.Room{ID: "r1", Slug: "zion", Name: "Zion"}
	m := NewChatroomsModel("neo", client)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.activeRoomID = "zion"
	m.activeRoom = &room
	m.mode = chatroomModeDetail
	m.staleAfterMs = 180000

	m, cmd := m.Update(roomPresenceStreamClosedMsg{roomID: "zion"})
	for i := 0; i < 20 && cmd != nil && !m.presenceReconnectFailed; i++ {
		msg := cmd()
		m, cmd = m.Update(msg)
	}

	if !m.presenceReconnectFailed {
		t.Fatal("expected presenceReconnectFailed to be true after exhausting all attempts")
	}
	if m.presenceReconnecting {
		t.Error("expected presenceReconnecting to be false once attempts are exhausted")
	}
	if m.presenceSub != nil {
		t.Error("expected presenceSub to remain nil after giving up")
	}
}

func TestChatroomsPresenceReconnect_CancelSubscriptionStopsRetrySequence(t *testing.T) {
	client := &failingRefreshClient{MockClient: api.NewMockClient()}
	room := model.Room{ID: "r1", Slug: "zion", Name: "Zion"}
	m := NewChatroomsModel("neo", client)
	m.activeRoomID = "zion"
	m.activeRoom = &room
	m.mode = chatroomModeDetail
	m.staleAfterMs = 180000

	m, cmd := m.Update(roomPresenceStreamClosedMsg{roomID: "zion"})
	if !m.presenceReconnecting {
		t.Fatal("expected presenceReconnecting to be true after stream closed")
	}
	failMsg := cmd()

	m = m.CancelSubscription()
	if m.presenceReconnecting {
		t.Error("expected presenceReconnecting to be false after CancelSubscription")
	}
	if m.presenceReconnectCancel != nil {
		t.Error("expected presenceReconnectCancel to be cleared after CancelSubscription")
	}

	// A late-arriving result for the cancelled sequence must be a no-op.
	m2, cmd2 := m.Update(failMsg)
	if cmd2 != nil {
		t.Error("expected no further command from a stale reconnect result after cancellation")
	}
	if m2.presenceReconnecting || m2.presenceReconnectFailed {
		t.Error("expected reconnect state to stay cleared after a stale result")
	}
}

func TestChatroomsPresenceReconnect_IgnoresResultForAbandonedRoom(t *testing.T) {
	m := NewChatroomsModel("neo", api.NewMockClient())
	m.activeRoomID = "sprawl" // user navigated to a different room in the meantime
	m.mode = chatroomModeDetail

	sub := &roomPresenceSubscription{RoomID: "zion", C: make(chan []model.RoomUser), cancel: func() {}}
	m2, cmd := m.Update(roomPresenceReconnectedMsg{sub: sub})
	if m2.presenceSub != nil {
		t.Error("expected presenceSub to stay nil when the reconnect result is for an abandoned room")
	}
	_ = cmd
}

// --- C-Mail ---

func TestCMailReconnect_StaleEventIgnored(t *testing.T) {
	m := NewCMailModel("neo", api.NewMockClient())
	m.activeConvID = "c1"
	m.mode = cmailModeDetail

	_, cmd := m.Update(dmStreamClosedMsg{convID: "old-abandoned-conv"})
	if cmd != nil {
		t.Error("expected no reconnect command for a stale stream-closed event")
	}
}

func TestCMailReconnect_SucceedsForActiveConversation(t *testing.T) {
	conv := model.Conversation{ID: "c1", Participants: []model.User{{Username: "neo"}, {Username: "trinity"}}}
	m := NewCMailModel("neo", api.NewMockClient())
	m.activeConvID = "c1"
	m.activeConv = &conv
	m.mode = cmailModeDetail

	_, cmd := m.Update(dmStreamClosedMsg{convID: "c1"})
	if cmd == nil {
		t.Fatal("expected a reconnect command")
	}
	msg := cmd()
	reconnected, ok := msg.(dmReconnectedMsg)
	if !ok {
		t.Fatalf("expected dmReconnectedMsg, got %T", msg)
	}
	if reconnected.sub == nil || reconnected.sub.ConvID != "c1" {
		t.Fatalf("expected reconnected sub for conv c1, got %+v", reconnected.sub)
	}

	m2, cmd2 := m.Update(reconnected)
	if m2.dmSub != reconnected.sub {
		t.Error("expected m.dmSub to be set to the reconnected subscription")
	}
	if cmd2 == nil {
		t.Error("expected a batch command (resume waiting + reconnected toast) after reconnect")
	}
}

func TestCMailReconnect_RetriesWithBackoffThenSucceeds(t *testing.T) {
	withShortBackoff(t)

	client := &flakySubscribeClient{MockClient: api.NewMockClient(), subscribeFailures: 2}
	conv := model.Conversation{ID: "c1", Participants: []model.User{{Username: "neo"}, {Username: "trinity"}}}
	m := NewCMailModel("neo", client)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.activeConvID = "c1"
	m.activeConv = &conv
	m.mode = cmailModeDetail

	m, cmd := m.Update(dmStreamClosedMsg{convID: "c1"})
	if !m.reconnecting {
		t.Fatal("expected reconnecting to be true after stream closed")
	}

	var succeeded bool
	for i := 0; i < 10 && cmd != nil && !succeeded; i++ {
		msg := cmd()
		m, cmd = m.Update(msg)
		if _, ok := msg.(dmReconnectedMsg); ok {
			succeeded = true
		}
	}

	if !succeeded {
		t.Fatal("expected reconnect to eventually succeed")
	}
	if m.reconnecting || m.reconnectFailed {
		t.Errorf("expected reconnect state cleared after success, got reconnecting=%v reconnectFailed=%v", m.reconnecting, m.reconnectFailed)
	}
	if m.dmSub == nil {
		t.Error("expected dmSub to be set after successful reconnect")
	}
}

func TestCMailReconnect_GivesUpAfterMaxAttempts(t *testing.T) {
	withShortBackoff(t)

	client := &failingRefreshClient{MockClient: api.NewMockClient()}
	conv := model.Conversation{ID: "c1", Participants: []model.User{{Username: "neo"}, {Username: "trinity"}}}
	m := NewCMailModel("neo", client)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.activeConvID = "c1"
	m.activeConv = &conv
	m.mode = cmailModeDetail

	m, cmd := m.Update(dmStreamClosedMsg{convID: "c1"})
	for i := 0; i < 20 && cmd != nil && !m.reconnectFailed; i++ {
		msg := cmd()
		m, cmd = m.Update(msg)
	}

	if !m.reconnectFailed {
		t.Fatal("expected reconnectFailed to be true after exhausting all attempts")
	}
	if m.reconnecting {
		t.Error("expected reconnecting to be false once attempts are exhausted")
	}
	if m.dmSub != nil {
		t.Error("expected dmSub to remain nil after giving up")
	}
	if view := m.View(); !strings.Contains(view, "live updates lost") {
		t.Errorf("expected give-up indicator in view, got: %q", view)
	}
}

func TestCMailReconnect_CancelSubscriptionStopsRetrySequence(t *testing.T) {
	client := &failingRefreshClient{MockClient: api.NewMockClient()}
	conv := model.Conversation{ID: "c1", Participants: []model.User{{Username: "neo"}, {Username: "trinity"}}}
	m := NewCMailModel("neo", client)
	m.activeConvID = "c1"
	m.activeConv = &conv
	m.mode = cmailModeDetail

	m, cmd := m.Update(dmStreamClosedMsg{convID: "c1"})
	if !m.reconnecting {
		t.Fatal("expected reconnecting to be true after stream closed")
	}
	failMsg := cmd()

	m = m.CancelSubscription()
	if m.reconnecting {
		t.Error("expected reconnecting to be false after CancelSubscription")
	}
	if m.reconnectCancel != nil {
		t.Error("expected reconnectCancel to be cleared after CancelSubscription")
	}

	// A late-arriving result for the cancelled sequence must be a no-op.
	m2, cmd2 := m.Update(failMsg)
	if cmd2 != nil {
		t.Error("expected no further command from a stale reconnect result after cancellation")
	}
	if m2.reconnecting || m2.reconnectFailed {
		t.Error("expected reconnect state to stay cleared after a stale result")
	}
}
