package screens

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ragnar/cyber-tui/internal/api"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
)

// --- sortRoomUsers ---

func TestSortRoomUsers_AdminsFirstThenAlphabetical(t *testing.T) {
	in := []model.RoomUser{
		{UserID: "1", Username: "zeb"},
		{UserID: "2", Username: "Molly", IsChatAdmin: true},
		{UserID: "3", Username: "alice"},
		{UserID: "4", Username: "bob", IsChatAdmin: true},
	}
	out := sortRoomUsers(in)

	want := []string{"bob", "Molly", "alice", "zeb"}
	if len(out) != len(want) {
		t.Fatalf("len(out) = %d, want %d", len(out), len(want))
	}
	for i, name := range want {
		if out[i].Username != name {
			t.Errorf("out[%d].Username = %q, want %q (full order: %+v)", i, out[i].Username, name, out)
		}
	}
	if !out[0].IsChatAdmin || !out[1].IsChatAdmin {
		t.Errorf("expected the first two entries to be admins, got %+v", out[:2])
	}
}

func TestSortRoomUsers_DoesNotMutateInput(t *testing.T) {
	in := []model.RoomUser{{UserID: "1", Username: "zeb"}, {UserID: "2", Username: "alice"}}
	_ = sortRoomUsers(in)
	if in[0].Username != "zeb" {
		t.Errorf("input slice was mutated: %+v", in)
	}
}

// --- renderRoomUsersPanel ---

// TestRenderRoomUsersPanel_OwnNameUsesMeHighlight confirms the viewer's own
// entry renders with theme.MeHighlight, same as their own username does in
// the message list (render.go), while a non-admin stranger stays unstyled.
func TestRenderRoomUsersPanel_OwnNameUsesMeHighlight(t *testing.T) {
	withTrueColor(t)
	users := []model.RoomUser{
		{Username: "ragnar"},
		{Username: "bob"},
	}

	out := renderRoomUsersPanel(users, "ragnar", 0)

	if !strings.Contains(out, theme.MeHighlight.Render("ragnar")) {
		t.Errorf("expected own name styled with MeHighlight, got: %q", out)
	}
	if strings.Contains(out, theme.Highlight.Render("bob")) {
		t.Errorf("did not expect a non-admin stranger to be styled at all, got: %q", out)
	}
}

// TestRenderRoomUsersPanel_AdminMarkerStaysHighlightRegardlessOfViewer
// confirms the ★ marker always renders in theme.Highlight (the admin
// signal), independent of whether the entry belongs to the viewer — and that
// the viewer's own admin entry gets MeHighlight on the name, not the
// admin/Highlight color, so the two signals never collapse into one.
func TestRenderRoomUsersPanel_AdminMarkerStaysHighlightRegardlessOfViewer(t *testing.T) {
	withTrueColor(t)
	users := []model.RoomUser{
		{Username: "bob", IsChatAdmin: true},    // admin, not the viewer
		{Username: "ragnar", IsChatAdmin: true}, // admin, and the viewer
	}

	out := renderRoomUsersPanel(users, "ragnar", 0)

	marker := theme.Highlight.Render("★ ")
	if strings.Count(out, marker) != 2 {
		t.Fatalf("expected the ★ marker styled with Highlight on both admin rows, got: %q", out)
	}
	if !strings.Contains(out, marker+theme.Highlight.Render("bob")) {
		t.Errorf("expected the non-viewer admin's name styled with Highlight, got: %q", out)
	}
	if !strings.Contains(out, marker+theme.MeHighlight.Render("ragnar")) {
		t.Errorf("expected the viewer's own admin name styled with MeHighlight (not Highlight), got: %q", out)
	}
}

// TestRenderRoomUsersPanel_IdleBadge confirms a user whose LastActivity is
// older than idleAfterMs gets the 💤 prefix, and one still within the window
// does not.
func TestRenderRoomUsersPanel_IdleBadge(t *testing.T) {
	withTrueColor(t)
	longIdle := time.Now().Add(-20 * time.Minute)
	recentActivity := time.Now().Add(-1 * time.Minute)
	users := []model.RoomUser{
		{Username: "sleepy", LastActivity: &longIdle},
		{Username: "awake", LastActivity: &recentActivity},
	}

	out := renderRoomUsersPanel(users, "", 600000) // idleAfterMs = 10min

	lines := strings.Split(out, "\n")
	badgeIdx := strings.Index(lines[0], "💤")
	nameIdx := strings.Index(lines[0], "sleepy")
	if badgeIdx == -1 {
		t.Fatalf("expected sleepy's row to carry the idle badge, got: %q", lines[0])
	}
	if badgeIdx > nameIdx {
		t.Errorf("expected the idle badge before the username, got badge at %d, name at %d: %q", badgeIdx, nameIdx, lines[0])
	}
	if strings.Contains(lines[1], "💤") {
		t.Errorf("expected awake's row to have no idle badge, got: %q", lines[1])
	}
}

// TestRenderRoomUsersPanel_NilLastActivityNeverIdle confirms a nil
// LastActivity (client never reported one) always renders active, regardless
// of idleAfterMs, and doesn't panic on the nil-dereference path.
func TestRenderRoomUsersPanel_NilLastActivityNeverIdle(t *testing.T) {
	withTrueColor(t)
	users := []model.RoomUser{{Username: "mystery"}}

	out := renderRoomUsersPanel(users, "", 1) // smallest possible idleAfterMs

	if strings.Contains(out, "💤") {
		t.Errorf("expected no idle badge for a nil LastActivity, got: %q", out)
	}
}

// --- panelWidths ---

func TestPanelWidths_CollapsesWhenNoUsersYet(t *testing.T) {
	m := ChatroomsModel{width: 200}
	msgW, usersW := m.panelWidths()
	if usersW != 0 || msgW != 200 {
		t.Errorf("panelWidths() = (%d, %d), want (200, 0) with no roomUsers loaded yet", msgW, usersW)
	}
}

func TestPanelWidths_CollapsesOnNarrowTerminal(t *testing.T) {
	m := ChatroomsModel{width: 50, roomUsers: []model.RoomUser{{Username: "alice"}}}
	msgW, usersW := m.panelWidths()
	if usersW != 0 || msgW != 50 {
		t.Errorf("panelWidths() = (%d, %d), want (50, 0) below the collapse threshold", msgW, usersW)
	}
}

func TestPanelWidths_ShowsPanelOnWideTerminal(t *testing.T) {
	m := ChatroomsModel{width: 160, roomUsers: []model.RoomUser{{Username: "alice"}}}
	msgW, usersW := m.panelWidths()
	if usersW != roomUsersPanelPreferredWidth {
		t.Errorf("usersW = %d, want exactly roomUsersPanelPreferredWidth (%d)", usersW, roomUsersPanelPreferredWidth)
	}
	if msgW+usersW+roomUsersPanelSep != 160 {
		t.Errorf("msgW(%d) + usersW(%d) + sep(%d) = %d, want 160", msgW, usersW, roomUsersPanelSep, msgW+usersW+roomUsersPanelSep)
	}
	if msgW < roomUsersPanelMinMsgWidth {
		t.Errorf("msgW = %d, must stay >= roomUsersPanelMinMsgWidth (%d)", msgW, roomUsersPanelMinMsgWidth)
	}
}

// TestPanelWidths_NeverBetweenZeroAndPreferred guards against a regression to
// percentage-of-width scaling, which could put the panel narrower than the
// worst-case content (an admin marker + a 20-char max-length username) and
// cause a hard mid-word wrap (lipgloss/cellbuf.Wrap breaks unbroken words
// that exceed the render width rather than truncating them).
func TestPanelWidths_NeverBetweenZeroAndPreferred(t *testing.T) {
	for width := 0; width <= 200; width++ {
		m := ChatroomsModel{width: width, roomUsers: []model.RoomUser{{Username: "alice"}}}
		_, usersW := m.panelWidths()
		if usersW != 0 && usersW != roomUsersPanelPreferredWidth {
			t.Fatalf("width=%d: usersW = %d, want 0 or exactly %d", width, usersW, roomUsersPanelPreferredWidth)
		}
	}
}

// TestRoomUsersPanelPreferredWidth_AccountsForIdleBadge guards the width bump
// that came with the idle badge: admin marker(2) + username(20) + idle
// badge(3) + padding(2) = 27.
func TestRoomUsersPanelPreferredWidth_AccountsForIdleBadge(t *testing.T) {
	if roomUsersPanelPreferredWidth != 27 {
		t.Errorf("roomUsersPanelPreferredWidth = %d, want 27", roomUsersPanelPreferredWidth)
	}
}

// --- presence message handling ---

func chatroomsInRoom(client api.Client, roomID string) ChatroomsModel {
	room := model.Room{ID: "r1", Slug: roomID, Name: roomID}
	m := NewChatroomsModel("neo", client)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 160, Height: 24})
	m.activeRoomID = roomID
	m.activeRoom = &room
	m.mode = chatroomModeDetail
	m.input.Focus() // enterRoomDetail always focuses the input in real usage;
	// without this, textinput.Update() no-ops (it early-returns when
	// unfocused), silently breaking any test that types through the normal
	// forwarding path rather than driving ChatroomsModel's own key handling.
	return m
}

func TestPresenceAnnounced_StoresCadenceAndSchedulesFollowUps(t *testing.T) {
	m := chatroomsInRoom(api.NewMockClient(), "zion")

	m, cmd := m.Update(roomPresenceAnnouncedMsg{roomID: "zion", heartbeatMs: 30000, staleAfterMs: 180000})
	if m.heartbeatMs != 30000 || m.staleAfterMs != 180000 {
		t.Errorf("heartbeatMs/staleAfterMs = %d/%d, want 30000/180000", m.heartbeatMs, m.staleAfterMs)
	}
	if cmd == nil {
		t.Fatal("expected a batch command (heartbeat tick + load users + subscribe)")
	}
}

func TestPresenceAnnounced_StaleRoomIDIgnored(t *testing.T) {
	m := chatroomsInRoom(api.NewMockClient(), "zion")

	m2, cmd := m.Update(roomPresenceAnnouncedMsg{roomID: "old-abandoned-room", heartbeatMs: 30000, staleAfterMs: 180000})
	if cmd != nil {
		t.Error("expected no command for a stale announce response")
	}
	if m2.heartbeatMs != 0 {
		t.Errorf("expected heartbeatMs to remain unset, got %d", m2.heartbeatMs)
	}
}

func TestHeartbeatTick_StaleRoomIDIgnored(t *testing.T) {
	m := chatroomsInRoom(api.NewMockClient(), "zion")
	m.heartbeatMs = 30000

	_, cmd := m.Update(roomHeartbeatTickMsg{roomID: "old-abandoned-room"})
	if cmd != nil {
		t.Error("expected no command for a heartbeat tick from a room the user already left")
	}
}

func TestHeartbeatTick_ActiveRoomReschedules(t *testing.T) {
	m := chatroomsInRoom(api.NewMockClient(), "zion")
	m.heartbeatMs = 30000

	_, cmd := m.Update(roomHeartbeatTickMsg{roomID: "zion"})
	if cmd == nil {
		t.Fatal("expected a batch command (send heartbeat + reschedule tick)")
	}
}

// --- own idle tracking (extra out-of-cycle heartbeat on a self idle/active flip) ---

// presenceSpyClient counts AnnouncePresence calls so tests can verify an
// extra out-of-cycle heartbeat actually fires (a returned tea.Cmd is lazy —
// nothing runs until it's invoked).
type presenceSpyClient struct {
	*api.MockClient
	calls int
}

func (c *presenceSpyClient) AnnouncePresence(roomID string, lastActivity time.Time) (int, int, int, error) {
	c.calls++
	return c.MockClient.AnnouncePresence(roomID, lastActivity)
}

// drainCmd recursively executes cmd (and, if it's a tea.Batch, every command
// inside it) so a test can observe side effects of commands that would
// otherwise only run once the Bubble Tea runtime gets around to them.
func drainCmd(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			drainCmd(c)
		}
	}
}

// TestSelfShownIdle_MatchesPanelRendering exercises selfShownIdle's cases
// directly: no self entry in the presence list, a fresh self entry, a stale
// one, and idleAfterMs not yet known (<= 0, the pre-first-announce default).
func TestSelfShownIdle_MatchesPanelRendering(t *testing.T) {
	stale := time.Now().Add(-20 * time.Minute)
	fresh := time.Now()

	m := ChatroomsModel{currentUser: "neo", idleAfterMs: 600000}
	if m.selfShownIdle() {
		t.Error("expected false with no self entry in roomUsers")
	}

	m.roomUsers = []model.RoomUser{{Username: "neo", LastActivity: &fresh}}
	if m.selfShownIdle() {
		t.Error("expected false for a fresh self entry")
	}

	m.roomUsers = []model.RoomUser{{Username: "neo", LastActivity: &stale}}
	if !m.selfShownIdle() {
		t.Error("expected true for a stale self entry past idleAfterMs")
	}

	m.idleAfterMs = 0
	if m.selfShownIdle() {
		t.Error("expected false when idleAfterMs isn't known yet (<= 0)")
	}
}

// TestChatroomsModel_KeyPressCorrectsStaleSelfIdleBadge confirms a keypress
// while the panel currently shows our own entry as idle (a stale
// server-recorded lastActivity, regardless of how recently we've actually
// been typing) fires an immediate corrective heartbeat rather than waiting
// for the next scheduled tick — this is the fix for the badge getting stuck
// showing idle while the user keeps typing.
func TestChatroomsModel_KeyPressCorrectsStaleSelfIdleBadge(t *testing.T) {
	spy := &presenceSpyClient{MockClient: api.NewMockClient()}
	m := chatroomsInRoom(spy, "zion")
	m.idleAfterMs = 600000
	stale := time.Now().Add(-20 * time.Minute)
	m.roomUsers = []model.RoomUser{{Username: "neo", LastActivity: &stale}}
	// lastHeartbeatSentAt left at its zero value — well outside the cooldown.

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})

	if time.Since(m.lastActivityAt) > time.Second {
		t.Errorf("expected lastActivityAt to be refreshed to now, got %v", m.lastActivityAt)
	}
	drainCmd(cmd)
	if spy.calls != 1 {
		t.Errorf("expected exactly one corrective heartbeat call, got %d", spy.calls)
	}
}

// TestChatroomsModel_KeyPressWhileActiveDoesNotExtraBeat confirms a keypress
// while the panel already shows us as active (no self entry, or a fresh one)
// doesn't fire a spurious extra heartbeat.
func TestChatroomsModel_KeyPressWhileActiveDoesNotExtraBeat(t *testing.T) {
	spy := &presenceSpyClient{MockClient: api.NewMockClient()}
	m := chatroomsInRoom(spy, "zion")
	m.idleAfterMs = 600000

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	drainCmd(cmd)
	if spy.calls != 0 {
		t.Errorf("expected no extra heartbeat when the panel doesn't show us idle, got %d calls", spy.calls)
	}
}

// TestChatroomsModel_KeyPressRespectsCorrectionCooldown guards the failure
// mode a fast typing burst right after a correction could otherwise hit: the
// panel's stale view of self won't clear until the corrective heartbeat
// round-trips through the server and RTDB, so without a cooldown, every key
// typed in that gap would fire another heartbeat — risking the 15/min-per-
// room presence rate limit.
func TestChatroomsModel_KeyPressRespectsCorrectionCooldown(t *testing.T) {
	spy := &presenceSpyClient{MockClient: api.NewMockClient()}
	m := chatroomsInRoom(spy, "zion")
	m.idleAfterMs = 600000
	stale := time.Now().Add(-20 * time.Minute)
	m.roomUsers = []model.RoomUser{{Username: "neo", LastActivity: &stale}}
	m.lastHeartbeatSentAt = time.Now() // a correction (or heartbeat) just fired

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	drainCmd(cmd)
	if spy.calls != 0 {
		t.Errorf("expected the cooldown to block a second correction so soon, got %d calls", spy.calls)
	}
}

// --- background room / unread badge (staying "in" a room while another tab is active) ---

func TestRoomReceived_BumpsUnreadWhileUnfocused(t *testing.T) {
	m := chatroomsInRoom(api.NewMockClient(), "zion")
	m = m.SetFocused(false)

	m, _ = m.Update(roomReceivedMsg{msg: model.Message{Body: "hey"}})
	m, _ = m.Update(roomReceivedMsg{msg: model.Message{Body: "hey again"}})

	if m.UnreadCount() != 2 {
		t.Errorf("UnreadCount() = %d, want 2", m.UnreadCount())
	}
}

func TestRoomReceived_DoesNotBumpUnreadWhileFocused(t *testing.T) {
	m := chatroomsInRoom(api.NewMockClient(), "zion")
	m = m.SetFocused(true)

	m, _ = m.Update(roomReceivedMsg{msg: model.Message{Body: "hey"}})

	if m.UnreadCount() != 0 {
		t.Errorf("UnreadCount() = %d, want 0 (actively viewing the room)", m.UnreadCount())
	}
}

func TestSetFocused_ClearsUnreadOnReturn(t *testing.T) {
	m := chatroomsInRoom(api.NewMockClient(), "zion")
	m = m.SetFocused(false)
	m, _ = m.Update(roomReceivedMsg{msg: model.Message{Body: "hey"}})
	if m.UnreadCount() != 1 {
		t.Fatalf("setup: expected unreadCount 1, got %d", m.UnreadCount())
	}

	m = m.SetFocused(true)
	if m.UnreadCount() != 0 {
		t.Errorf("UnreadCount() = %d, want 0 after refocusing the tab", m.UnreadCount())
	}
}

func TestHasLiveRoom(t *testing.T) {
	m := NewChatroomsModel("neo", api.NewMockClient())
	if m.HasLiveRoom() {
		t.Error("expected no live room before any room is opened")
	}

	m = chatroomsInRoom(api.NewMockClient(), "zion")
	if !m.HasLiveRoom() {
		t.Error("expected HasLiveRoom() once a room is open in detail mode")
	}
}

func TestIsRoomStreamMsg(t *testing.T) {
	streamMsgs := []tea.Msg{
		roomReceivedMsg{},
		roomStreamClosedMsg{},
		roomPresenceReceivedMsg{},
		roomHeartbeatTickMsg{},
	}
	for _, msg := range streamMsgs {
		if !IsRoomStreamMsg(msg) {
			t.Errorf("IsRoomStreamMsg(%T) = false, want true", msg)
		}
	}
	if IsRoomStreamMsg(tea.KeyMsg{Type: tea.KeyEnter}) {
		t.Error("IsRoomStreamMsg(tea.KeyMsg) = true, want false — key input must not be routed to a backgrounded room")
	}
}

func TestRoomReceived_KeepsStreamingWhileUnfocused(t *testing.T) {
	m := chatroomsInRoom(api.NewMockClient(), "zion")
	m.sub = &roomSubscription{RoomID: "zion", cancel: func() {}}
	m = m.SetFocused(false)

	_, cmd := m.Update(roomReceivedMsg{msg: model.Message{Body: "hey"}})
	if cmd == nil {
		t.Error("expected waitForRoomMsg to be re-issued so the stream keeps running in the background")
	}
}

func TestRoomUsersLoaded_SortsAndStores(t *testing.T) {
	m := chatroomsInRoom(api.NewMockClient(), "zion")

	m, _ = m.Update(roomUsersLoadedMsg{roomID: "zion", users: []model.RoomUser{
		{UserID: "1", Username: "zeb"},
		{UserID: "2", Username: "alice", IsChatAdmin: true},
	}})
	if len(m.roomUsers) != 2 {
		t.Fatalf("len(roomUsers) = %d, want 2", len(m.roomUsers))
	}
	if m.roomUsers[0].Username != "alice" {
		t.Errorf("roomUsers[0].Username = %q, want alice (admin sorts first)", m.roomUsers[0].Username)
	}
}

func TestRoomUsersLoaded_StaleRoomIDIgnored(t *testing.T) {
	m := chatroomsInRoom(api.NewMockClient(), "zion")

	m2, _ := m.Update(roomUsersLoadedMsg{roomID: "old-abandoned-room", users: []model.RoomUser{{Username: "alice"}}})
	if len(m2.roomUsers) != 0 {
		t.Errorf("expected roomUsers to remain empty for a stale load, got %+v", m2.roomUsers)
	}
}

func TestPresenceSubscribed_StaleRoomIDCancelsAndIgnores(t *testing.T) {
	m := chatroomsInRoom(api.NewMockClient(), "zion")

	cancelled := false
	sub := &roomPresenceSubscription{RoomID: "old-abandoned-room", cancel: func() { cancelled = true }}

	m2, cmd := m.Update(roomPresenceSubscribedMsg{roomID: "old-abandoned-room", sub: sub})
	if !cancelled {
		t.Error("expected the stale subscription to be cancelled")
	}
	if cmd != nil {
		t.Error("expected no command for a stale subscribe response")
	}
	if m2.presenceSub != nil {
		t.Error("expected presenceSub to remain nil for a stale subscribe response")
	}
}

func TestPresenceStreamClosed_ResubscribesWhileRoomStillOpen(t *testing.T) {
	m := chatroomsInRoom(api.NewMockClient(), "zion")
	m.staleAfterMs = 180000

	m2, cmd := m.Update(roomPresenceStreamClosedMsg{roomID: "zion"})
	if m2.presenceSub != nil {
		t.Error("expected presenceSub to be cleared")
	}
	if cmd == nil {
		t.Error("expected an immediate resubscribe command")
	}
}

func TestPresenceStreamClosed_StaleRoomIDIgnored(t *testing.T) {
	m := chatroomsInRoom(api.NewMockClient(), "zion")

	_, cmd := m.Update(roomPresenceStreamClosedMsg{roomID: "old-abandoned-room"})
	if cmd != nil {
		t.Error("expected no command for a stale stream-closed event")
	}
}

// TestPresenceReceived_StaleSubscriptionIgnored guards the fix for a race
// where a quick leave/re-enter of the same room can leave an orphaned prior
// presence subscription running; without checking which subscription a
// roomPresenceReceivedMsg came from, its snapshot could still clobber the
// current (correct) list.
func TestPresenceReceived_StaleSubscriptionIgnored(t *testing.T) {
	m := chatroomsInRoom(api.NewMockClient(), "zion")
	oldSub := &roomPresenceSubscription{RoomID: "zion", C: make(chan []model.RoomUser)}
	newSub := &roomPresenceSubscription{RoomID: "zion", C: make(chan []model.RoomUser)}
	m.presenceSub = newSub
	m.roomUsers = []model.RoomUser{{UserID: "1", Username: "bob"}}

	m2, cmd := m.Update(roomPresenceReceivedMsg{sub: oldSub, users: []model.RoomUser{{Username: "alice"}}})
	if len(m2.roomUsers) != 1 || m2.roomUsers[0].Username != "bob" {
		t.Errorf("expected roomUsers to be untouched by the orphaned subscription's snapshot, got %+v", m2.roomUsers)
	}
	if cmd != nil {
		t.Error("expected no re-arm command for a stale subscription's snapshot")
	}
}

func TestPresenceReceived_ActiveSubscriptionApplies(t *testing.T) {
	m := chatroomsInRoom(api.NewMockClient(), "zion")
	sub := &roomPresenceSubscription{RoomID: "zion", C: make(chan []model.RoomUser)}
	m.presenceSub = sub

	m2, cmd := m.Update(roomPresenceReceivedMsg{sub: sub, users: []model.RoomUser{{Username: "alice"}}})
	if len(m2.roomUsers) != 1 {
		t.Errorf("expected roomUsers to be updated from the active subscription, got %+v", m2.roomUsers)
	}
	if cmd == nil {
		t.Error("expected a re-arm command to keep waiting on the active subscription")
	}
}

// TestPresenceSubscribed_ReplacingLiveSubscriptionCancelsThePrevious guards
// against leaking an orphaned subscription's goroutine/SSE connection when a
// second roomPresenceSubscribedMsg for the same room arrives while one is
// already active (the quick leave/re-enter race).
func TestPresenceSubscribed_ReplacingLiveSubscriptionCancelsThePrevious(t *testing.T) {
	m := chatroomsInRoom(api.NewMockClient(), "zion")
	cancelled := false
	oldSub := &roomPresenceSubscription{RoomID: "zion", cancel: func() { cancelled = true }}
	m.presenceSub = oldSub

	newSub := &roomPresenceSubscription{RoomID: "zion", cancel: func() {}}
	m2, _ := m.Update(roomPresenceSubscribedMsg{roomID: "zion", sub: newSub})
	if !cancelled {
		t.Error("expected the previously active subscription to be cancelled when replaced")
	}
	if m2.presenceSub != newSub {
		t.Error("expected presenceSub to be updated to the new subscription")
	}
}

// --- detail view header ---

// TestDetailView_HeaderHasDividerBeforeMessages guards against the divider
// row (added so the header doesn't float with no visual bottom edge, unlike
// every other piece of chrome in this view) being dropped in a future edit.
func TestDetailView_HeaderHasDividerBeforeMessages(t *testing.T) {
	m := chatroomsInRoom(api.NewMockClient(), "zion")
	m = m.SetMessages("zion", []model.Message{{From: model.User{Username: "alice"}, Body: "hi"}})

	lines := strings.Split(m.View(), "\n")
	if len(lines) < 3 {
		t.Fatalf("expected at least a header, divider, and message line, got %d lines: %q", len(lines), lines)
	}
	if !strings.Contains(lines[0], "zion") {
		t.Fatalf("expected line 0 to be the room header, got: %q", lines[0])
	}
	if !strings.Contains(lines[1], "─") {
		t.Errorf("expected line 1 to be the divider rule, got: %q", lines[1])
	}
	if strings.Contains(lines[0], "─") {
		t.Errorf("did not expect the divider character on the header line itself, got: %q", lines[0])
	}
}

// inputBoxLines extracts the detail view's bordered input box (top/content/
// bottom) from a full View() render, located by its rounded-border corners
// — the only box in this view that uses them (the users panel only has a
// plain "│" separator column). Each line is trimmed of trailing spaces:
// lipgloss.JoinVertical pads every line in the block to the width of the
// widest one (here, the message-area+panel row), so the raw line can carry
// extra trailing padding that isn't part of the input box itself — right
// TrimRight is safe because a box line's own rightmost character is always
// its closing border glyph ("╮"/"│"/"╯"), never a space.
func inputBoxLines(t *testing.T, view string) (top, content, bottom string) {
	t.Helper()
	lines := strings.Split(view, "\n")
	topIdx, botIdx := -1, -1
	for i, l := range lines {
		if strings.Contains(l, "╭") {
			topIdx = i
		}
		if strings.Contains(l, "╰") {
			botIdx = i
		}
	}
	if topIdx == -1 || botIdx != topIdx+2 {
		t.Fatalf("could not locate the 3-line bordered input box in the view: %q", lines)
	}
	return strings.TrimRight(lines[topIdx], " "), strings.TrimRight(lines[topIdx+1], " "), strings.TrimRight(lines[botIdx], " ")
}

// TestInputBox_WidthConstantBetweenEmptyAndTyped is a regression test for a
// bug where textinput.View()'s *empty* placeholder rendering and its normal
// (typed-content) rendering total different widths — the placeholder path
// sums to exactly Width, but typed content adds Prompt's width plus one more
// for the phantom end-of-line cursor glyph, neither subtracted from its own
// padding math. Without compensating for that gap when setting input.Width,
// the box silently rendered 3 columns wider than the header/divider above it
// the instant any character was typed, pushing its right border off-screen
// on any terminal where those 3 columns were the difference between fitting
// and not (reproduced via tmux against the built binary, not just this
// test). Both states must render at exactly m.width.
func TestInputBox_WidthConstantBetweenEmptyAndTyped(t *testing.T) {
	m := chatroomsInRoom(api.NewMockClient(), "zion")

	top, content, bottom := inputBoxLines(t, m.View())
	for name, l := range map[string]string{"top (empty)": top, "content (empty)": content, "bottom (empty)": bottom} {
		if w := lipgloss.Width(l); w != m.width {
			t.Errorf("%s line width = %d, want %d (m.width)", name, w, m.width)
		}
	}

	m = setInput(m, "hello world", 11)
	top, content, bottom = inputBoxLines(t, m.View())
	for name, l := range map[string]string{"top (typed)": top, "content (typed)": content, "bottom (typed)": bottom} {
		if w := lipgloss.Width(l); w != m.width {
			t.Errorf("%s line width = %d, want %d (m.width) — box must not widen once typing starts", name, w, m.width)
		}
	}
}

// --- mentionQueryAt ---

func TestMentionQueryAt(t *testing.T) {
	cases := []struct {
		name      string
		value     string
		cursor    int
		wantQuery string
		wantAtPos int
		wantOK    bool
	}{
		{"bare @ at start", "@", 1, "", 0, true},
		{"partial mid-sentence", "hey @al", 7, "al", 4, true},
		{"partial at start", "@al", 3, "al", 0, true},
		{"no @ at all", "hello", 5, "", 0, false},
		{"email-like, no leading boundary", "user@host", 9, "", 0, false},
		{"cleared after a completed mention plus space", "hey @alice ", 11, "", 0, false},
		{"cursor mid-word still in query", "@alice", 3, "al", 0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			query, atPos, ok := mentionQueryAt(c.value, c.cursor)
			if ok != c.wantOK {
				t.Fatalf("ok = %v, want %v", ok, c.wantOK)
			}
			if !ok {
				return
			}
			if query != c.wantQuery {
				t.Errorf("query = %q, want %q", query, c.wantQuery)
			}
			if atPos != c.wantAtPos {
				t.Errorf("atPos = %d, want %d", atPos, c.wantAtPos)
			}
		})
	}
}

// --- matchMentionCandidates ---

func TestMatchMentionCandidates(t *testing.T) {
	users := []model.RoomUser{
		{Username: "bob", IsChatAdmin: true},
		{Username: "alice"},
		{Username: "Albert"},
	}
	// matchMentionCandidates doesn't sort — it filters in whatever order
	// it's given, matching SetRoomUsers' already-sorted m.roomUsers.
	got := matchMentionCandidates(users, "al")
	if len(got) != 2 || got[0].Username != "alice" || got[1].Username != "Albert" {
		t.Fatalf("got %+v, want [alice, Albert] (case-insensitive prefix match)", got)
	}

	if got := matchMentionCandidates(users, ""); len(got) != 3 {
		t.Errorf("empty query should match everyone, got %d", len(got))
	}

	if got := matchMentionCandidates(users, "zzz"); len(got) != 0 {
		t.Errorf("expected no matches for a non-existent prefix, got %+v", got)
	}
}

// --- spliceMention ---

func TestSpliceMention(t *testing.T) {
	newValue, newCursor := spliceMention("hey @al there", 4, 7, "alice")
	if newValue != "hey @alice there" {
		t.Errorf("newValue = %q, want %q", newValue, "hey @alice there")
	}
	if newCursor != 10 {
		t.Errorf("newCursor = %d, want 10 (right after the inserted name, no trailing space)", newCursor)
	}
}

// --- Tab-driven mention cycling (Update-level) ---

func setInput(m ChatroomsModel, value string, cursor int) ChatroomsModel {
	m.input.SetValue(value)
	m.input.SetCursor(cursor)
	return m
}

func TestMentionTab_FirstPressPreviewsWithoutTouchingText(t *testing.T) {
	m := chatroomsInRoom(api.NewMockClient(), "zion")
	m.roomUsers = []model.RoomUser{{Username: "alice"}, {Username: "albert"}}
	m = setInput(m, "hey @al", 7)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	if m.input.Value() != "hey @al" {
		t.Errorf("input = %q, want unchanged %q — Tab must not touch real text", m.input.Value(), "hey @al")
	}
	if m.input.Position() != 7 {
		t.Errorf("cursor = %d, want unchanged 7 — Tab must not move the cursor", m.input.Position())
	}
	// alice (index 0) is already the implicit default preview before any Tab
	// — the first Tab must skip straight to the next match (albert) so it's
	// a visible change, not a no-op.
	if m.mentionCycle == nil || m.mentionGhostText() != "bert" {
		t.Fatal("expected the first Tab to preview albert")
	}
}

func TestMentionTab_SecondPressCyclesPreviewToNextMatch(t *testing.T) {
	m := chatroomsInRoom(api.NewMockClient(), "zion")
	m.roomUsers = []model.RoomUser{{Username: "alice"}, {Username: "albert"}}
	m = setInput(m, "hey @al", 7)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // -> albert
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // -> wraps back to alice

	if m.input.Value() != "hey @al" {
		t.Errorf("input = %q, want still unchanged %q", m.input.Value(), "hey @al")
	}
	if got := m.mentionGhostText(); got != "ice" {
		t.Errorf("ghost = %q, want %q (second Tab should wrap back to the first match, alice)", got, "ice")
	}
}

func TestMentionTab_ThirdPressWrapsPreviewAround(t *testing.T) {
	m := chatroomsInRoom(api.NewMockClient(), "zion")
	m.roomUsers = []model.RoomUser{{Username: "alice"}, {Username: "albert"}}
	m = setInput(m, "hey @al", 7)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // -> albert
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // -> alice
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // -> albert again

	if got := m.mentionGhostText(); got != "bert" {
		t.Errorf("ghost = %q, want %q (third Tab should cycle back to albert)", got, "bert")
	}
}

// TestMentionTab_ReflectsPersonLeavingMidCycle guards against reintroducing
// the snapshot-at-cycle-start design: matches used to be captured once when
// cycling began and reused for the rest of the session, so someone who left
// the room mid-cycle stayed offered indefinitely. Candidates are now
// resolved fresh on every Tab press instead.
func TestMentionTab_ReflectsPersonLeavingMidCycle(t *testing.T) {
	m := chatroomsInRoom(api.NewMockClient(), "zion")
	m.roomUsers = []model.RoomUser{{Username: "alice"}, {Username: "albert"}}
	m = setInput(m, "hey @al", 7)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // -> albert
	if got := m.mentionGhostText(); got != "bert" {
		t.Fatalf("setup: ghost = %q, want %q (albert)", got, "bert")
	}

	m.roomUsers = []model.RoomUser{{Username: "alice"}} // albert left
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	if got := m.mentionGhostText(); got != "ice" {
		t.Errorf("ghost = %q, want %q (alice) — must not keep offering albert after they left", got, "ice")
	}
}

// TestMentionTab_ReflectsPersonJoiningMidCycle is the join-side counterpart:
// someone who joins mid-cycle must become reachable by continuing to Tab,
// not require the user to retype the query to start a fresh cycle.
func TestMentionTab_ReflectsPersonJoiningMidCycle(t *testing.T) {
	m := chatroomsInRoom(api.NewMockClient(), "zion")
	m.roomUsers = []model.RoomUser{{Username: "alice"}, {Username: "albert"}}
	m = setInput(m, "hey @al", 7)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // -> albert
	if got := m.mentionGhostText(); got != "bert" {
		t.Fatalf("setup: ghost = %q, want %q (albert)", got, "bert")
	}

	m.roomUsers = []model.RoomUser{{Username: "alice"}, {Username: "albert"}, {Username: "alan"}} // alan joined
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	if got := m.mentionGhostText(); got != "an" {
		t.Errorf("ghost = %q, want %q (alan) — a newly-joined match must become reachable without a fresh cycle", got, "an")
	}
}

func TestMentionSpace_CommitsCurrentPreviewThenInsertsSpace(t *testing.T) {
	m := chatroomsInRoom(api.NewMockClient(), "zion")
	m.roomUsers = []model.RoomUser{{Username: "alice"}, {Username: "albert"}}
	m = setInput(m, "hey @al", 7)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})                       // preview albert
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})                       // preview alice
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")}) // commit alice

	if m.input.Value() != "hey @alice " {
		t.Errorf("input = %q, want %q", m.input.Value(), "hey @alice ")
	}
	if m.input.Position() != len([]rune("hey @alice ")) {
		t.Errorf("cursor = %d, want end of the committed text", m.input.Position())
	}
	if m.mentionCycle != nil {
		t.Error("expected the preview to be cleared after committing")
	}
}

// TestMentionSpace_CommitsPassiveDefaultPreviewWithoutTab is a regression
// test for a bug where Space typed immediately after "@al" — with the first
// match already ghost-previewed by default, but no Tab press yet — didn't
// commit that preview at all: it fell through to plain space-insertion,
// producing the literal typed text plus a space (e.g. "hey @al ") instead of
// committing the name that was visibly showing (e.g. "hey @alice "). Space
// must commit whatever mentionGhostText is currently displaying, whether or
// not an explicit Tab-cycle is active.
func TestMentionSpace_CommitsPassiveDefaultPreviewWithoutTab(t *testing.T) {
	m := chatroomsInRoom(api.NewMockClient(), "zion")
	m.roomUsers = []model.RoomUser{{Username: "alice"}, {Username: "albert"}}
	m = setInput(m, "hey @al", 7)

	if got, want := m.mentionGhostText(), "ice"; got != want {
		t.Fatalf("setup: mentionGhostText() = %q, want %q (alice previewed by default)", got, want)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")}) // no Tab press first

	if m.input.Value() != "hey @alice " {
		t.Errorf("input = %q, want %q — Space must commit the passively-previewed match", m.input.Value(), "hey @alice ")
	}
}

func TestMentionSpace_NoOpWithoutActivePreview(t *testing.T) {
	m := chatroomsInRoom(api.NewMockClient(), "zion")
	m = setInput(m, "hey", 3)

	// Real space keystrokes from bubbletea's decoder carry Runes: []rune(" ")
	// alongside Type: KeySpace — textinput's insert logic reads Runes, so a
	// KeySpace msg without it (unlike the real thing) would insert nothing.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")})

	if m.input.Value() != "hey " {
		t.Errorf("input = %q, want normal space insertion %q", m.input.Value(), "hey ")
	}
}

func TestMentionTab_TypingBetweenPressesClearsPreviewWithoutCommitting(t *testing.T) {
	m := chatroomsInRoom(api.NewMockClient(), "zion")
	m.roomUsers = []model.RoomUser{{Username: "alice"}, {Username: "albert"}}
	m = setInput(m, "hey @al", 7)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // preview alice, text still "hey @al"
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	if m.mentionCycle != nil {
		t.Fatal("expected typing to clear the in-progress preview")
	}
	if m.input.Value() != "hey @ali" {
		t.Errorf("input = %q, want %q — typing must never auto-insert the previewed candidate", m.input.Value(), "hey @ali")
	}
}

func TestMentionTab_NoOpWithoutAtSign(t *testing.T) {
	m := chatroomsInRoom(api.NewMockClient(), "zion")
	m.roomUsers = []model.RoomUser{{Username: "alice"}}
	m = setInput(m, "hello", 5)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	if m.input.Value() != "hello" {
		t.Errorf("input = %q, want unchanged %q", m.input.Value(), "hello")
	}
	if m.mentionCycle != nil {
		t.Error("expected no mentionCycle without an @ in progress")
	}
}

func TestMentionTab_NoOpWithNoMatches(t *testing.T) {
	m := chatroomsInRoom(api.NewMockClient(), "zion")
	m.roomUsers = []model.RoomUser{{Username: "alice"}}
	m = setInput(m, "hey @zzz", 8)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	if m.input.Value() != "hey @zzz" {
		t.Errorf("input = %q, want unchanged %q", m.input.Value(), "hey @zzz")
	}
	if m.mentionCycle != nil {
		t.Error("expected no mentionCycle when nothing matches")
	}
}

// --- mentionGhostText ---

func TestMentionGhostText(t *testing.T) {
	m := chatroomsInRoom(api.NewMockClient(), "zion")
	m.roomUsers = []model.RoomUser{{Username: "alice"}, {Username: "albert"}, {Username: "bob"}}

	cases := []struct {
		name   string
		value  string
		cursor int
		want   string
	}{
		{"bare @ previews the first online username", "@", 1, "alice"},
		{"partial query previews just the remainder", "hey @al", 7, "ice"},
		{"cursor not at the end is unambiguous-placement limitation", "hey @al there", 7, ""},
		{"no matches", "hey @zzz", 8, ""},
		{"fully typed name has nothing left to preview", "hey @alice", 10, ""},
		{"no mention in progress", "hello", 5, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := setInput(m, c.value, c.cursor)
			got := m.mentionGhostText()
			if got != c.want {
				t.Errorf("mentionGhostText() = %q, want %q", got, c.want)
			}
		})
	}
}

// TestMentionGhostText_ReflectsActiveCyclePreview confirms the ghost text
// tracks whichever candidate Tab-cycling currently has selected, not always
// the first match — otherwise cycling wouldn't visibly do anything.
func TestMentionGhostText_ReflectsActiveCyclePreview(t *testing.T) {
	m := chatroomsInRoom(api.NewMockClient(), "zion")
	m.roomUsers = []model.RoomUser{{Username: "alice"}, {Username: "albert"}}
	m = setInput(m, "hey @al", 7)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // -> albert (skips the already-shown alice)
	if got, want := m.mentionGhostText(), "bert"; got != want {
		t.Errorf("after first Tab, mentionGhostText() = %q, want %q", got, want)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // -> alice
	if got, want := m.mentionGhostText(), "ice"; got != want {
		t.Errorf("after second Tab, mentionGhostText() = %q, want %q", got, want)
	}
}

// TestMentionGhostText_RendersAdjacentToCursorAtConstantWidth is a
// regression test for two related bugs:
//  1. The ghost text landing far past the visible cursor instead of right
//     next to it — caused by appending it after textinput.View()'s own
//     padding, which always fills out to whatever Width it's given.
//  2. A one-cell gap between the typed text and the ghost from the cursor's
//     own blank glyph — fixed by overlaying the cursor on the ghost's first
//     rune instead (mirrors this file's withTrueColor doc comment: tests
//     here run under the default no-color profile, so cursor.Model.View()
//     and theme.Subtle.Render both produce plain unstyled text, meaning
//     the rebuilt line reads as an unbroken "hey @al" + "ice" with nothing
//     in between).
//
// This reproduces View()'s actual construction rather than measuring
// through the full rendered View(), whose lines get padded to the width of
// the widest line in the block — the message-area+panel row — by
// lipgloss.JoinVertical, which would contaminate a width measurement taken
// that way.
func TestMentionGhostText_RendersAdjacentToCursorAtConstantWidth(t *testing.T) {
	m := chatroomsInRoom(api.NewMockClient(), "zion")
	m.roomUsers = []model.RoomUser{{Username: "alice"}}
	m = setInput(m, "hey @al", 7)

	baseline := lipgloss.Width(m.input.View()) // no ghost

	ghost := m.mentionGhostText()
	if ghost == "" {
		t.Fatal("expected a ghost text to be present for this setup")
	}
	ghostRunes := []rune(ghost)
	cur := m.input.Cursor
	cur.SetChar(string(ghostRunes[0]))
	cur.TextStyle = theme.Subtle
	textView := m.input.TextStyle.Inline(true).Render(m.input.Value())
	promptView := m.input.PromptStyle.Render(m.input.Prompt)
	rest := theme.Subtle.Render(string(ghostRunes[1:]))
	valWidth := lipgloss.Width(m.input.Value())
	pad := max(0, m.input.Width-valWidth-lipgloss.Width(ghost)+1) // see View()'s matching comment
	content := promptView + textView + cur.View() + rest + strings.Repeat(" ", pad)

	if w := lipgloss.Width(content); w != baseline {
		t.Errorf("rebuilt content width = %d, want %d (same as with no ghost at all) — must not widen the box", w, baseline)
	}
	if !strings.Contains(content, "hey @alice") {
		t.Errorf("expected the typed text and ghost to read as an unbroken %q with the cursor overlaying its first character, got: %q", "hey @alice", content)
	}
}

// TestChatrooms_Esc_WhileBrowsing_ResetsViewportToBottom guards a real bug
// found in manual testing: esc while browsing cleared the selection and
// refocused the input, but — unlike the "down past the newest message"
// exit path — left the viewport wherever browsing had scrolled it, instead
// of also jumping back to the live tail.
func TestChatrooms_Esc_WhileBrowsing_ResetsViewportToBottom(t *testing.T) {
	m := chatroomsInRoom(api.NewMockClient(), "zion")

	var msgs []model.Message
	for i := 1; i <= 30; i++ {
		msgs = append(msgs, model.Message{
			ID:        fmt.Sprintf("m%d", i),
			From:      model.User{Username: "molly"},
			Body:      fmt.Sprintf("message %d", i),
			CreatedAt: time.Now().Add(time.Duration(i) * time.Minute),
		})
	}
	m = m.SetMessages("zion", msgs)
	if !m.viewport.AtBottom() {
		t.Fatal("setup: expected the viewport to start at the bottom after SetMessages")
	}

	for i := 0; i < len(msgs); i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	}
	if m.selectedMsgID != "m1" {
		t.Fatalf("setup: selectedMsgID = %q after paging all the way up, want m1", m.selectedMsgID)
	}
	if m.viewport.AtBottom() {
		t.Fatal("setup: expected paging up through browsing to leave the bottom")
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if m.selectedMsgID != "" {
		t.Errorf("selectedMsgID = %q, want cleared after esc", m.selectedMsgID)
	}
	if !m.viewport.AtBottom() {
		t.Errorf("expected esc to reset the viewport to the bottom, YOffset=%d maxYOffset≈%d", m.viewport.YOffset, m.viewport.Height)
	}
}

// TestChatrooms_SettingsRefresh_WhileTyping_ResetsViewportToBottom guards a
// real bug: an incoming SharedConfigMsg (e.g. the settings refresh fired
// after the user's own /mute or /unmute) re-filters and re-renders the
// message list via refreshMessages(), which changes the rendered line count
// but never touched viewport.YOffset — leaving the view pinned to a stale
// raw line number that now maps to different content instead of following
// the reload, the same class of bug the esc-while-browsing case above guards.
func TestChatrooms_SettingsRefresh_WhileTyping_ResetsViewportToBottom(t *testing.T) {
	m := chatroomsInRoom(api.NewMockClient(), "zion")

	var msgs []model.Message
	for i := 1; i <= 30; i++ {
		msgs = append(msgs, model.Message{
			ID:        fmt.Sprintf("m%d", i),
			From:      model.User{Username: "molly"},
			Body:      fmt.Sprintf("message %d", i),
			CreatedAt: time.Now().Add(time.Duration(i) * time.Minute),
		})
	}
	m = m.SetMessages("zion", msgs)
	m.viewport.SetYOffset(0)
	if m.viewport.AtBottom() {
		t.Fatal("setup: expected the viewport to start away from the bottom")
	}

	m, _ = m.Update(SharedConfigMsg{Settings: model.Settings{MutedUsersByRoom: map[string][]string{"zion": {"molly"}}}})

	if !m.viewport.AtBottom() {
		t.Errorf("expected a settings refresh to reset the viewport to the bottom while typing, YOffset=%d", m.viewport.YOffset)
	}
}

// TestChatrooms_SettingsRefresh_WhileBrowsing_KeepsSelectionVisible mirrors
// the above for the browsing-a-selected-message case: the selected message
// should stay on screen (not necessarily at the bottom) after the reload.
func TestChatrooms_SettingsRefresh_WhileBrowsing_KeepsSelectionVisible(t *testing.T) {
	m := chatroomsInRoom(api.NewMockClient(), "zion")

	var msgs []model.Message
	for i := 1; i <= 30; i++ {
		msgs = append(msgs, model.Message{
			ID:        fmt.Sprintf("m%d", i),
			From:      model.User{Username: "molly"},
			Body:      fmt.Sprintf("message %d", i),
			CreatedAt: time.Now().Add(time.Duration(i) * time.Minute),
		})
	}
	m = m.SetMessages("zion", msgs)
	for i := 0; i < len(msgs); i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	}
	if m.selectedMsgID != "m1" {
		t.Fatalf("setup: selectedMsgID = %q after paging all the way up, want m1", m.selectedMsgID)
	}

	m, _ = m.Update(SharedConfigMsg{Settings: model.Settings{MutedUsersByRoom: map[string][]string{"zion": {"someoneElse"}}}})

	idx := -1
	for i, msg := range m.messages {
		if msg.ID == m.selectedMsgID {
			idx = i
		}
	}
	if idx == -1 {
		t.Fatalf("selected message %q not found after refresh", m.selectedMsgID)
	}
	itemStart := m.msgOffsets[idx]
	itemEnd := itemStart + m.msgHeights[idx] - 1
	if itemStart < m.viewport.YOffset || itemEnd >= m.viewport.YOffset+m.viewport.Height {
		t.Errorf("selected message (lines %d-%d) not visible in viewport window [%d, %d)",
			itemStart, itemEnd, m.viewport.YOffset, m.viewport.YOffset+m.viewport.Height)
	}
}

// TestMentionGhostText_CursorOverlayUsesGhostColor guards against the
// overlaid character rendering in normal text color during the cursor's
// text-style blink phase, which would make it indistinguishable from
// something the user actually typed.
func TestMentionGhostText_CursorOverlayUsesGhostColor(t *testing.T) {
	withTrueColor(t)
	m := chatroomsInRoom(api.NewMockClient(), "zion")
	m.roomUsers = []model.RoomUser{{Username: "alice"}}
	m = setInput(m, "hey @al", 7)
	m.input.Cursor.Blink = true // the phase cursor.Model.View() renders via TextStyle

	view := m.View()
	wantChar := theme.Subtle.Inline(true).Render("i")
	if !strings.Contains(view, wantChar) {
		t.Errorf("expected the cursor-overlaid character styled in ghost color (%q) somewhere in the view, got:\n%s", wantChar, view)
	}
}

// --- style animation ticker (coarse-scoped, see maybeStartStyleAnim) ---

func TestUpdate_StartsStyleAnimTickerWhenAnimatedMessageArrives(t *testing.T) {
	m := chatroomsInRoom(api.NewMockClient(), "zion")

	m, cmd := m.Update(roomReceivedMsg{msg: model.Message{ID: "m1", Body: "hi", Style: []string{"wave"}}})

	if !m.styleAnimRunning {
		t.Error("expected styleAnimRunning = true after an animated-style message arrived")
	}
	if cmd == nil {
		t.Error("expected a non-nil tea.Cmd to start the animation ticker")
	}
}

func TestUpdate_NoStyleAnimTickerForPlainMessage(t *testing.T) {
	m := chatroomsInRoom(api.NewMockClient(), "zion")

	m, _ = m.Update(roomReceivedMsg{msg: model.Message{ID: "m1", Body: "hi"}})

	if m.styleAnimRunning {
		t.Error("expected styleAnimRunning = false for a message with no animated style")
	}
}

func TestUpdate_StyleAnimTick_AdvancesFrameAndRearms(t *testing.T) {
	m := chatroomsInRoom(api.NewMockClient(), "zion")
	m, _ = m.Update(roomReceivedMsg{msg: model.Message{ID: "m1", Body: "hi", Style: []string{"glitch"}}})
	if !m.styleAnimRunning {
		t.Fatal("setup: expected styleAnimRunning = true")
	}

	m, cmd := m.Update(styleAnimTickMsg{})

	if m.styleAnimFrame != 1 {
		t.Errorf("styleAnimFrame = %d, want 1", m.styleAnimFrame)
	}
	if !m.styleAnimRunning {
		t.Error("expected styleAnimRunning to stay true (rearmed) while the glitch message is still loaded")
	}
	if cmd == nil {
		t.Error("expected a non-nil tea.Cmd to rearm the ticker")
	}
}

func TestSetFocused_ResumesStyleAnimAfterBackgroundedTickIsDropped(t *testing.T) {
	m := chatroomsInRoom(api.NewMockClient(), "zion")
	m, _ = m.Update(roomReceivedMsg{msg: model.Message{ID: "m1", Body: "hi", Style: []string{"wave"}}})
	if !m.styleAnimRunning {
		t.Fatal("setup: expected styleAnimRunning = true")
	}

	// Simulate switching away: the in-flight styleAnimTickMsg is dropped by
	// App's message routing (it's not an IsRoomStreamMsg), so updateInner
	// never runs to clear styleAnimRunning. Switching back must still be
	// able to restart the ticker.
	m = m.SetFocused(false)
	m = m.SetFocused(true)

	m, cmd := m.maybeStartStyleAnim()
	if !m.styleAnimRunning {
		t.Error("expected styleAnimRunning = true after resuming")
	}
	if cmd == nil {
		t.Error("expected a non-nil tea.Cmd restarting the ticker after refocusing")
	}
}

// --- spoiler reveal (see updateBrowsingKey's "enter" case) ---

func TestBrowsing_Enter_TogglesSpoilerReveal(t *testing.T) {
	m := chatroomsInRoom(api.NewMockClient(), "zion")
	m = m.SetMessages("zion", []model.Message{
		{ID: "m1", From: model.User{Username: "molly"}, Body: "the ending is a twist", Style: []string{"spoiler"}, CreatedAt: time.Now()},
	})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.selectedMsgID != "m1" {
		t.Fatalf("setup: selectedMsgID = %q, want m1", m.selectedMsgID)
	}
	renderedWithReveal := func(m ChatroomsModel) string {
		content, _, _ := renderCircMessagesWithSelection(m.messages, m.location(), m.timeDisplayFormat,
			m.viewport.Width, m.currentUser, m.selectedMsgID, m.revealed, m.styleAnimFrame, nil)
		return content
	}
	if strings.Contains(renderedWithReveal(m), "the ending is a twist") {
		t.Fatal("setup: expected spoiler body to start masked")
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.revealed["m1"] {
		t.Error("expected revealed[m1] = true after enter")
	}
	if !strings.Contains(renderedWithReveal(m), "the ending is a twist") {
		t.Errorf("expected spoiler body revealed after enter, got: %q", renderedWithReveal(m))
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.revealed["m1"] {
		t.Error("expected revealed[m1] = false after a second enter (toggle back off)")
	}
	if strings.Contains(renderedWithReveal(m), "the ending is a twist") {
		t.Error("expected spoiler body masked again after toggling off")
	}
}

func TestBrowsing_Enter_NoOpForNonSpoilerMessage(t *testing.T) {
	m := chatroomsInRoom(api.NewMockClient(), "zion")
	m = m.SetMessages("zion", []model.Message{
		{ID: "m1", From: model.User{Username: "molly"}, Body: "hi", CreatedAt: time.Now()},
	})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.revealed["m1"] {
		t.Error("expected enter on a non-spoiler message to be a no-op")
	}
}

// TestBrowsing_Enter_TogglesL33tReveal confirms l33t-styled messages use the
// same reveal toggle as spoiler: substituted text by default, enter reveals
// the original unsubstituted text, a second enter re-obscures it.
func TestBrowsing_Enter_TogglesL33tReveal(t *testing.T) {
	m := chatroomsInRoom(api.NewMockClient(), "zion")
	m = m.SetMessages("zion", []model.Message{
		{ID: "m1", From: model.User{Username: "molly"}, Body: "elite", Style: []string{"l33t"}, CreatedAt: time.Now()},
	})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.selectedMsgID != "m1" {
		t.Fatalf("setup: selectedMsgID = %q, want m1", m.selectedMsgID)
	}

	renderedWithReveal := func(m ChatroomsModel) string {
		content, _, _ := renderCircMessagesWithSelection(m.messages, m.location(), m.timeDisplayFormat,
			m.viewport.Width, m.currentUser, m.selectedMsgID, m.revealed, m.styleAnimFrame, nil)
		return content
	}
	if !strings.Contains(renderedWithReveal(m), "3l173") {
		t.Fatalf("setup: expected l33t-substituted text before reveal, got: %q", renderedWithReveal(m))
	}
	if strings.Contains(renderedWithReveal(m), "elite") {
		t.Fatal("setup: did not expect original text visible before reveal")
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.revealed["m1"] {
		t.Error("expected revealed[m1] = true after enter")
	}
	if !strings.Contains(renderedWithReveal(m), "elite") {
		t.Errorf("expected original text revealed after enter, got: %q", renderedWithReveal(m))
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.revealed["m1"] {
		t.Error("expected revealed[m1] = false after a second enter (toggle back off)")
	}
	if !strings.Contains(renderedWithReveal(m), "3l173") {
		t.Error("expected l33t-substituted text again after toggling off")
	}
}
