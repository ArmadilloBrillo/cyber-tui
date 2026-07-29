package screens

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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

	out := renderRoomUsersPanel(users, "ragnar")

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

	out := renderRoomUsersPanel(users, "ragnar")

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

// --- presence message handling ---

func chatroomsInRoom(client api.Client, roomID string) ChatroomsModel {
	room := model.Room{ID: "r1", Slug: roomID, Name: roomID}
	m := NewChatroomsModel("neo", client)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 160, Height: 24})
	m.activeRoomID = roomID
	m.activeRoom = &room
	m.mode = chatroomModeDetail
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
