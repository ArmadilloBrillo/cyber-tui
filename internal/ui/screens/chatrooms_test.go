package screens

import (
	"strings"
	"testing"

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
	if m.mentionCycle == nil || m.mentionCycle.matches[m.mentionCycle.index].Username != "albert" {
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
	if m.mentionCycle.matches[m.mentionCycle.index].Username != "alice" {
		t.Errorf("preview = %q, want %q (second Tab should wrap back to the first match)", m.mentionCycle.matches[m.mentionCycle.index].Username, "alice")
	}
}

func TestMentionTab_ThirdPressWrapsPreviewAround(t *testing.T) {
	m := chatroomsInRoom(api.NewMockClient(), "zion")
	m.roomUsers = []model.RoomUser{{Username: "alice"}, {Username: "albert"}}
	m = setInput(m, "hey @al", 7)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // -> albert
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // -> alice
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // -> albert again

	if m.mentionCycle.matches[m.mentionCycle.index].Username != "albert" {
		t.Errorf("preview = %q, want %q (third Tab should cycle back to albert)", m.mentionCycle.matches[m.mentionCycle.index].Username, "albert")
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
