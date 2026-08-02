package screens_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ragnar/cyber-tui/internal/api"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/screens"
)

// --- ChatroomsModel.InputFocused ---

func TestChatroomsInputFocused_DefaultFalse(t *testing.T) {
	m := screens.NewChatroomsModel("", nil)
	if m.InputFocused() {
		t.Error("input should not be focused on a freshly created ChatroomsModel")
	}
}

// --- CMailModel.InputFocused ---

func TestCMailInputFocused_DefaultFalse(t *testing.T) {
	m := screens.NewCMailModel("neuromancer", nil)
	if m.InputFocused() {
		t.Error("input should not be focused on a freshly created CMailModel")
	}
}

// --- helpers ---

func twoConvs() []model.Conversation {
	return []model.Conversation{
		{
			ID:           "c1",
			Participants: []model.User{{Username: "neuromancer"}, {Username: "molly"}},
			Messages: []model.Message{
				{ID: "m1", From: model.User{Username: "molly"}, Body: "hey there", CreatedAt: time.Now()},
			},
		},
		{
			ID:           "c2",
			Participants: []model.User{{Username: "neuromancer"}, {Username: "wintermute"}},
			Messages: []model.Message{
				{ID: "m2", From: model.User{Username: "wintermute"}, Body: "i am the one", CreatedAt: time.Now()},
			},
		},
	}
}

func sendKey(m screens.CMailModel, key string) (screens.CMailModel, tea.Cmd) {
	return m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
}

func sendSpecialKey(m screens.CMailModel, keyType tea.KeyType) (screens.CMailModel, tea.Cmd) {
	return m.Update(tea.KeyMsg{Type: keyType})
}

// --- List mode navigation ---

func TestCMailCursorDown(t *testing.T) {
	m := screens.NewCMailModel("neuromancer", nil)
	m = m.SetConversations(twoConvs())
	m, _ = sendKey(m, "j")
	if m.SelectedConv() != 1 {
		t.Errorf("expected selectedConv=1 after ↓, got %d", m.SelectedConv())
	}
}

func TestCMailCursorUp_ClampsAtZero(t *testing.T) {
	m := screens.NewCMailModel("neuromancer", nil)
	m = m.SetConversations(twoConvs())
	m, _ = sendKey(m, "k")
	if m.SelectedConv() != 0 {
		t.Errorf("expected selectedConv=0 (clamped), got %d", m.SelectedConv())
	}
}

func TestCMailCursorDown_ClampsAtBottom(t *testing.T) {
	m := screens.NewCMailModel("neuromancer", nil)
	m = m.SetConversations(twoConvs())
	m, _ = sendKey(m, "j")
	m, _ = sendKey(m, "j") // already at bottom
	if m.SelectedConv() != 1 {
		t.Errorf("expected selectedConv=1 (clamped), got %d", m.SelectedConv())
	}
}

// --- Default mode is list ---

func TestCMailIsShowingDetail_DefaultFalse(t *testing.T) {
	m := screens.NewCMailModel("neuromancer", nil)
	if m.IsShowingDetail() {
		t.Error("fresh CMailModel should be in list mode, not detail mode")
	}
}

// --- Enter in list mode opens detail mode ---

func TestCMailEnterOpensConversation(t *testing.T) {
	m := screens.NewCMailModel("neuromancer", nil)
	m = m.SetConversations(twoConvs())
	m, _ = sendSpecialKey(m, tea.KeyEnter)
	if !m.HasActiveConv() {
		t.Error("expected activeConv to be set after Enter")
	}
	if !m.IsShowingDetail() {
		t.Error("expected detail mode after Enter on a conversation")
	}
	if !m.InputFocused() {
		t.Error("expected input to be focused after Enter")
	}
}

// --- Esc in detail mode returns to list mode ---

func TestCMailEsc_InDetail_ReturnsToList(t *testing.T) {
	m := screens.NewCMailModel("neuromancer", nil)
	m = m.SetConversations(twoConvs())
	m, _ = sendSpecialKey(m, tea.KeyEnter) // enter detail mode
	if !m.IsShowingDetail() {
		t.Fatal("setup: expected detail mode after Enter")
	}
	m, _ = sendSpecialKey(m, tea.KeyEsc)
	if m.IsShowingDetail() {
		t.Error("expected list mode after Esc in detail mode")
	}
	if m.InputFocused() {
		t.Error("expected input to be blurred after returning to list mode")
	}
}

// --- Enter in detail mode with non-empty input emits SendCMailMsg ---

func TestCMailSend_EmitsMessage(t *testing.T) {
	m := screens.NewCMailModel("neuromancer", nil)
	m = m.SetConversations(twoConvs())
	m, _ = sendSpecialKey(m, tea.KeyEnter) // open conversation

	// type message via Update with rune messages
	for _, r := range "hello" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	_, cmd := sendSpecialKey(m, tea.KeyEnter)
	if cmd == nil {
		t.Fatal("expected a command to be returned after Enter with text")
	}
	msg := cmd()
	sendMsg, ok := msg.(screens.SendCMailMsg)
	if !ok {
		t.Fatalf("expected SendCMailMsg, got %T", msg)
	}
	if sendMsg.Body != "hello" {
		t.Errorf("expected body='hello', got %q", sendMsg.Body)
	}
	if sendMsg.ConversationID != "c1" {
		t.Errorf("expected conversationID='c1', got %q", sendMsg.ConversationID)
	}
}

// --- Enter with empty body does not emit a command ---

func TestCMailSend_EmptyBodyNoCmd(t *testing.T) {
	m := screens.NewCMailModel("neuromancer", nil)
	m = m.SetConversations(twoConvs())
	m, _ = sendSpecialKey(m, tea.KeyEnter) // open conversation
	// input is empty
	_, cmd := sendSpecialKey(m, tea.KeyEnter)
	if cmd != nil {
		// cmd may be a batch from input/viewport — check it doesn't produce SendCMailMsg
		msg := cmd()
		if _, ok := msg.(screens.SendCMailMsg); ok {
			t.Error("expected no SendCMailMsg for empty body, got one")
		}
	}
}

// --- ChatroomsModel mode transition tests ---

func sampleRooms() []model.Room {
	return []model.Room{
		{ID: "r1", Slug: "zion", Name: "Zion", LastMessageAt: time.Now().Add(-2 * time.Minute), SortOrder: 1},
		{ID: "r2", Slug: "sprawl", Name: "Sprawl", LastMessageAt: time.Now().Add(-10 * time.Minute), SortOrder: 2},
	}
}

func sendChatroomKey(m screens.ChatroomsModel, key string) (screens.ChatroomsModel, tea.Cmd) {
	return m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
}

func sendChatroomSpecialKey(m screens.ChatroomsModel, keyType tea.KeyType) (screens.ChatroomsModel, tea.Cmd) {
	return m.Update(tea.KeyMsg{Type: keyType})
}

func TestChatrooms_DefaultIsListMode(t *testing.T) {
	m := screens.NewChatroomsModel("neuromancer", nil)
	if m.IsShowingDetail() {
		t.Error("fresh ChatroomsModel should be in list mode")
	}
}

func TestChatrooms_InputNotFocusedByDefault(t *testing.T) {
	m := screens.NewChatroomsModel("neuromancer", nil)
	if m.InputFocused() {
		t.Error("input should not be focused before a room is selected")
	}
}

func TestChatrooms_EnterOpensDetailMode(t *testing.T) {
	m := screens.NewChatroomsModel("neuromancer", nil)
	m = m.SetRooms(sampleRooms())
	m, _ = sendChatroomSpecialKey(m, tea.KeyEnter)
	if !m.IsShowingDetail() {
		t.Error("expected detail mode after Enter on a room")
	}
	if !m.InputFocused() {
		t.Error("expected input to be focused after entering a room")
	}
}

func TestChatrooms_EscReturnsToListMode(t *testing.T) {
	m := screens.NewChatroomsModel("neuromancer", nil)
	m = m.SetRooms(sampleRooms())
	m, _ = sendChatroomSpecialKey(m, tea.KeyEnter)
	if !m.IsShowingDetail() {
		t.Fatal("setup: expected detail mode after Enter")
	}
	m, _ = sendChatroomSpecialKey(m, tea.KeyEsc)
	if m.IsShowingDetail() {
		t.Error("expected list mode after Esc in detail mode")
	}
	if m.InputFocused() {
		t.Error("expected input to be blurred after returning to list mode")
	}
}

func TestChatrooms_JKNavigateList(t *testing.T) {
	m := screens.NewChatroomsModel("neuromancer", nil)
	m = m.SetRooms(sampleRooms())
	m, _ = sendChatroomKey(m, "j")
	// After j, should be at index 1 (can't inspect directly, but Enter should open r2)
	m2, cmd := sendChatroomSpecialKey(m, tea.KeyEnter)
	if !m2.IsShowingDetail() {
		t.Fatal("expected detail mode after Enter")
	}
	// The batch cmd fires loadRoomMessages, openRoomSubscription, and RoomOpenedMsg
	if cmd == nil {
		t.Error("expected a command batch after Enter on a room")
	}
}

func TestChatrooms_OpenPendingRoom_EntersDetailMode(t *testing.T) {
	m := screens.NewChatroomsModel("neuromancer", nil)
	m = m.SetPendingRoomSlug("sprawl")
	m = m.SetRooms(sampleRooms())
	m, cmd := m.OpenPendingRoom()
	if !m.IsShowingDetail() {
		t.Error("expected detail mode after OpenPendingRoom matches a loaded room")
	}
	if !m.InputFocused() {
		t.Error("expected input to be focused after OpenPendingRoom")
	}
	if cmd == nil {
		t.Error("expected a command batch after OpenPendingRoom")
	}
}

func TestChatrooms_OpenPendingRoom_NoMatchIsNoop(t *testing.T) {
	m := screens.NewChatroomsModel("neuromancer", nil)
	m = m.SetPendingRoomSlug("nonexistent-room")
	m = m.SetRooms(sampleRooms())
	m, cmd := m.OpenPendingRoom()
	if m.IsShowingDetail() {
		t.Error("expected list mode when pending slug matches no loaded room")
	}
	if cmd != nil {
		t.Error("expected nil command when pending slug matches no loaded room")
	}
	// The stale slug must be cleared so a later, unrelated reload can't
	// suddenly jump into a room the user never asked to open.
	m = m.SetRooms(sampleRooms())
	m, cmd = m.OpenPendingRoom()
	if m.IsShowingDetail() || cmd != nil {
		t.Error("expected pending slug to have been cleared after first OpenPendingRoom call")
	}
}

func TestChatrooms_OpenPendingRoom_EmptySlugIsNoop(t *testing.T) {
	m := screens.NewChatroomsModel("neuromancer", nil)
	m = m.SetRooms(sampleRooms())
	m, cmd := m.OpenPendingRoom()
	if m.IsShowingDetail() {
		t.Error("expected list mode when no pending slug was set")
	}
	if cmd != nil {
		t.Error("expected nil command when no pending slug was set")
	}
}

func TestChatrooms_EnterKey_StillWorksAfterRefactor(t *testing.T) {
	m := screens.NewChatroomsModel("neuromancer", nil)
	m = m.SetRooms(sampleRooms())
	m, _ = sendChatroomSpecialKey(m, tea.KeyEnter)
	if !m.IsShowingDetail() {
		t.Error("expected detail mode after Enter on the first room (regression check)")
	}
	if !m.InputFocused() {
		t.Error("expected input focused after Enter (regression check)")
	}
}

func TestChatrooms_Send_EmitsMessage(t *testing.T) {
	m := screens.NewChatroomsModel("neuromancer", nil)
	m = m.SetRooms(sampleRooms())
	m, _ = sendChatroomSpecialKey(m, tea.KeyEnter) // open room

	for _, r := range "hello circ" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	_, cmd := sendChatroomSpecialKey(m, tea.KeyEnter)
	if cmd == nil {
		t.Fatal("expected a command after Enter with text")
	}
	msg := cmd()
	sendMsg, ok := msg.(screens.SendRoomMessageMsg)
	if !ok {
		t.Fatalf("expected SendRoomMessageMsg, got %T", msg)
	}
	if sendMsg.Body != "hello circ" {
		t.Errorf("expected body='hello circ', got %q", sendMsg.Body)
	}
	if sendMsg.RoomID != "zion" {
		t.Errorf("expected roomID='zion', got %q", sendMsg.RoomID)
	}
}

func TestChatrooms_AppendMessage(t *testing.T) {
	m := screens.NewChatroomsModel("neuromancer", nil)
	m = m.SetRooms(sampleRooms())
	m, _ = sendChatroomSpecialKey(m, tea.KeyEnter)

	msg := model.Message{ID: "x1", From: model.User{Username: "molly"}, Body: "test", CreatedAt: time.Now()}
	_ = m.AppendMessage(msg)
	// No panic = pass; state is internal but AppendMessage should not crash
}

// --- Open-link shortcut (o / ctrl+o) ---

func TestChatrooms_GetFocusedURLs_NilInListMode(t *testing.T) {
	m := screens.NewChatroomsModel("neuromancer", nil)
	m = m.SetRooms(sampleRooms())
	// still in list mode: no room open, nothing to expose
	if urls := m.GetFocusedURLs(); urls != nil {
		t.Errorf("expected nil URLs in list mode, got %v", urls)
	}
}

func TestChatrooms_GetFocusedURLs_AggregatesLoadedMessagesAndDedupes(t *testing.T) {
	m := screens.NewChatroomsModel("neuromancer", nil)
	m = m.SetRooms(sampleRooms())
	m, _ = sendChatroomSpecialKey(m, tea.KeyEnter) // opens "zion"
	m = m.SetMessages("zion", []model.Message{
		{ID: "m1", From: model.User{Username: "molly"}, Body: "check https://example.com/one", CreatedAt: time.Now()},
		{ID: "m2", From: model.User{Username: "bob"}, Body: "no links here", CreatedAt: time.Now()},
		{ID: "m3", From: model.User{Username: "molly"}, Body: "also https://example.com/one and https://example.com/two", CreatedAt: time.Now()},
	})

	urls := m.GetFocusedURLs()
	if len(urls) != 2 {
		t.Fatalf("expected 2 deduped URLs, got %d: %v", len(urls), urls)
	}
}

// --- /help reply as a local system message ---

func TestChatrooms_AppendSystemMessage_RendersLocally(t *testing.T) {
	m := screens.NewChatroomsModel("neuromancer", nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = m.SetRooms(sampleRooms())
	m, _ = sendChatroomSpecialKey(m, tea.KeyEnter)
	m = m.SetMessages("zion", []model.Message{
		{ID: "m1", From: model.User{Username: "molly"}, Body: "hi", CreatedAt: time.Now()},
	})

	m = m.AppendSystemMessage("zion", "Commands: /me, /dice, /help")

	view := m.View()
	if !strings.Contains(view, "Commands: /me, /dice, /help") {
		t.Errorf("expected the system reply in the view, got: %q", view)
	}
	if strings.Contains(view, "<system>") {
		t.Errorf("expected no username bracket for a system message, got: %q", view)
	}
}

func TestChatrooms_AppendSystemMessage_WrongRoomIsNoOp(t *testing.T) {
	m := screens.NewChatroomsModel("neuromancer", nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = m.SetRooms(sampleRooms())
	m, _ = sendChatroomSpecialKey(m, tea.KeyEnter) // opens "zion"

	m = m.AppendSystemMessage("sprawl", "should not appear")

	view := m.View()
	if strings.Contains(view, "should not appear") {
		t.Error("expected AppendSystemMessage to no-op for a non-active room")
	}
}

func TestCMail_AppendSystemMessage_RendersLocally(t *testing.T) {
	m := screens.NewCMailModel("neuromancer", nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = m.SetConversations(sampleConvWithMessage())
	m, _ = sendSpecialKey(m, tea.KeyEnter)

	m = m.AppendSystemMessage("c1", "Commands: /me, /dice, /help")

	view := m.View()
	if !strings.Contains(view, "Commands: /me, /dice, /help") {
		t.Errorf("expected the system reply in the view, got: %q", view)
	}
}

func TestCMail_AppendSystemMessage_WrongConvIsNoOp(t *testing.T) {
	m := screens.NewCMailModel("neuromancer", nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = m.SetConversations(sampleConvWithMessage())
	m, _ = sendSpecialKey(m, tea.KeyEnter) // opens "c1"

	m = m.AppendSystemMessage("c2", "should not appear")

	view := m.View()
	if strings.Contains(view, "should not appear") {
		t.Error("expected AppendSystemMessage to no-op for a non-active conversation")
	}
}

// --- History pagination (load-more on scroll-to-top) ---

func TestChatrooms_UpAtTop_TriggersHistoryLoadThenGuards(t *testing.T) {
	m := screens.NewChatroomsModel("neuromancer", nil)
	m = m.SetRooms(sampleRooms())
	m, _ = sendChatroomSpecialKey(m, tea.KeyEnter)
	m = m.SetMessages("zion", []model.Message{
		{ID: "m1", From: model.User{Username: "molly"}, Body: "first message", CreatedAt: time.Now()},
	})

	m, cmd := sendChatroomSpecialKey(m, tea.KeyUp)
	if cmd == nil {
		t.Fatal("expected a history-load command when scrolling up at the top")
	}

	// A second "up" press while the load is in flight must not refire.
	_, cmd = sendChatroomSpecialKey(m, tea.KeyUp)
	if cmd != nil {
		t.Error("expected no command while a history load is already in flight")
	}
}

// TestChatrooms_Up_MultiMessageRoomThatFits_NoSpuriousHistoryLoad guards a
// bug found in manual testing: a short room's content already fits within
// the viewport, so the viewport is trivially "at top" (YOffset 0) from the
// moment it renders — checking viewport.AtTop() right after entering
// browsing fired a history-load on essentially every first "up" press,
// regardless of which message got selected. Pagination should only fire
// once the selection actually reaches the oldest loaded message.
func TestChatrooms_Up_MultiMessageRoomThatFits_NoSpuriousHistoryLoad(t *testing.T) {
	m := screens.NewChatroomsModel("neuromancer", nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = m.SetRooms(sampleRooms())
	m, _ = sendChatroomSpecialKey(m, tea.KeyEnter)
	m = m.SetMessages("zion", []model.Message{
		{ID: "m1", From: model.User{Username: "molly"}, Body: "first", CreatedAt: time.Now()},
		{ID: "m2", From: model.User{Username: "wintermute"}, Body: "second", CreatedAt: time.Now()},
		{ID: "m3", From: model.User{Username: "molly"}, Body: "third", CreatedAt: time.Now()},
	})

	m, cmd := sendChatroomSpecialKey(m, tea.KeyUp) // enter browsing, select m3 (newest)
	if cmd != nil {
		t.Error("expected no history-load command when entering browsing on a multi-message room")
	}
	if m.SelectedMessageID() != "m3" {
		t.Fatalf("SelectedMessageID() = %q, want m3", m.SelectedMessageID())
	}
}

func TestChatrooms_UpAtTop_NoMessagesNoCmd(t *testing.T) {
	m := screens.NewChatroomsModel("neuromancer", nil)
	m = m.SetRooms(sampleRooms())
	m, _ = sendChatroomSpecialKey(m, tea.KeyEnter) // no messages loaded yet

	_, cmd := sendChatroomSpecialKey(m, tea.KeyUp)
	if cmd != nil {
		t.Error("expected no history-load command with no messages loaded")
	}
}

// --- ChatroomsModel: per-message selection ("browsing") + flag/report ---

func TestChatrooms_Up_EntersBrowsing_SelectsNewest(t *testing.T) {
	m := screens.NewChatroomsModel("neuromancer", nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = m.SetRooms(sampleRooms())
	m, _ = sendChatroomSpecialKey(m, tea.KeyEnter)
	m = m.SetMessages("zion", []model.Message{
		{ID: "m1", From: model.User{Username: "molly"}, Body: "first", CreatedAt: time.Now()},
		{ID: "m2", From: model.User{Username: "wintermute"}, Body: "second", CreatedAt: time.Now()},
	})

	m, _ = sendChatroomSpecialKey(m, tea.KeyUp)
	if m.SelectedMessageID() != "m2" {
		t.Errorf("SelectedMessageID() = %q, want m2 (newest)", m.SelectedMessageID())
	}
}

func TestChatrooms_Up_SkipsSystemMessages(t *testing.T) {
	m := screens.NewChatroomsModel("neuromancer", nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = m.SetRooms(sampleRooms())
	m, _ = sendChatroomSpecialKey(m, tea.KeyEnter)
	m = m.SetMessages("zion", []model.Message{
		{ID: "m1", From: model.User{Username: "molly"}, Body: "first", CreatedAt: time.Now()},
	})
	m = m.AppendSystemMessage("zion", "*** a local notice")

	m, _ = sendChatroomSpecialKey(m, tea.KeyUp)
	if m.SelectedMessageID() != "m1" {
		t.Errorf("SelectedMessageID() = %q, want m1 (system notice must never be selectable)", m.SelectedMessageID())
	}
}

func TestChatrooms_Esc_WhileBrowsing_ClearsSelectionWithoutLeavingRoom(t *testing.T) {
	m := screens.NewChatroomsModel("neuromancer", nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = m.SetRooms(sampleRooms())
	m, _ = sendChatroomSpecialKey(m, tea.KeyEnter)
	m = m.SetMessages("zion", []model.Message{
		{ID: "m1", From: model.User{Username: "molly"}, Body: "first", CreatedAt: time.Now()},
	})
	m, _ = sendChatroomSpecialKey(m, tea.KeyUp)
	if m.SelectedMessageID() == "" {
		t.Fatal("setup: expected a selection after up")
	}

	m, _ = sendChatroomSpecialKey(m, tea.KeyEsc)
	if m.SelectedMessageID() != "" {
		t.Error("expected esc to clear the selection")
	}
	if !m.IsShowingDetail() {
		t.Error("expected esc while browsing to stay in the room, not leave it")
	}
}

func TestChatrooms_Down_PastNewest_ExitsBrowsing(t *testing.T) {
	m := screens.NewChatroomsModel("neuromancer", nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = m.SetRooms(sampleRooms())
	m, _ = sendChatroomSpecialKey(m, tea.KeyEnter)
	m = m.SetMessages("zion", []model.Message{
		{ID: "m1", From: model.User{Username: "molly"}, Body: "first", CreatedAt: time.Now()},
	})
	m, _ = sendChatroomSpecialKey(m, tea.KeyUp) // select the only (newest) message

	m, _ = sendChatroomSpecialKey(m, tea.KeyDown)
	if m.SelectedMessageID() != "" {
		t.Error("expected down past the newest message to exit browsing")
	}
}

func TestChatrooms_WhileBrowsing_OtherKeysAreSwallowed_NotTypedIntoCompose(t *testing.T) {
	m := screens.NewChatroomsModel("neuromancer", nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = m.SetRooms(sampleRooms())
	m, _ = sendChatroomSpecialKey(m, tea.KeyEnter)
	m = m.SetMessages("zion", []model.Message{
		{ID: "m1", From: model.User{Username: "molly"}, Body: "first", CreatedAt: time.Now()},
	})
	m, _ = sendChatroomSpecialKey(m, tea.KeyUp)

	m, _ = sendChatroomKey(m, "q")
	m, _ = sendChatroomKey(m, "1")
	if m.SelectedMessageID() == "" {
		t.Error("expected selection to remain active after unrelated keys")
	}

	m, _ = sendChatroomSpecialKey(m, tea.KeyEsc)
	if !m.ComposeEmpty() {
		t.Error("expected compose box to remain empty — browsing must not type into it")
	}
}

func TestChatrooms_DraftPreserved_AcrossBrowsingRoundTrip(t *testing.T) {
	m := screens.NewChatroomsModel("neuromancer", nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = m.SetRooms(sampleRooms())
	m, _ = sendChatroomSpecialKey(m, tea.KeyEnter)
	m = m.SetMessages("zion", []model.Message{
		{ID: "m1", From: model.User{Username: "molly"}, Body: "first", CreatedAt: time.Now()},
	})
	for _, r := range "hello" {
		m, _ = sendChatroomKey(m, string(r))
	}
	if m.ComposeEmpty() {
		t.Fatal("setup: expected a draft before browsing")
	}

	m, _ = sendChatroomSpecialKey(m, tea.KeyUp)  // enter browsing
	m, _ = sendChatroomSpecialKey(m, tea.KeyEsc) // back to typing

	if m.ComposeEmpty() {
		t.Error("expected the draft to survive a trip into browsing and back")
	}
}

func TestChatrooms_Up_WhileBrowsingAtOldest_StillTriggersHistoryLoad(t *testing.T) {
	m := screens.NewChatroomsModel("neuromancer", nil)
	// A short viewport (2 message-history rows) so 3 one-line messages don't
	// all fit at once — otherwise the viewport is trivially "at top" the
	// instant any message renders, regardless of which one is selected, and
	// this test couldn't distinguish "browsing" from "reached the oldest".
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
	m = m.SetRooms(sampleRooms())
	m, _ = sendChatroomSpecialKey(m, tea.KeyEnter)
	m = m.SetMessages("zion", []model.Message{
		{ID: "m1", From: model.User{Username: "molly"}, Body: "first", CreatedAt: time.Now()},
		{ID: "m2", From: model.User{Username: "wintermute"}, Body: "second", CreatedAt: time.Now()},
		{ID: "m3", From: model.User{Username: "molly"}, Body: "third", CreatedAt: time.Now()},
	})

	m, _ = sendChatroomSpecialKey(m, tea.KeyUp) // enter browsing, select m3 (newest)
	if m.SelectedMessageID() != "m3" {
		t.Fatalf("setup: SelectedMessageID() = %q, want m3", m.SelectedMessageID())
	}
	m, cmd := sendChatroomSpecialKey(m, tea.KeyUp) // move to m2 (middle — not the oldest yet)
	if cmd != nil {
		t.Fatal("expected no history-load cmd yet — not at the oldest message")
	}
	if m.SelectedMessageID() != "m2" {
		t.Fatalf("SelectedMessageID() = %q, want m2", m.SelectedMessageID())
	}

	_, cmd = sendChatroomSpecialKey(m, tea.KeyUp) // reaches m1 (oldest) — pagination fires
	if cmd == nil {
		t.Error("expected a history-load command when browsing reaches the oldest loaded message")
	}
}

// TestChatrooms_DownThroughManyMessages_ReachesNewestAndExits guards a real
// bug found in manual testing: with more messages than fit in the viewport,
// 'down' got permanently stuck partway through and never reached the newest
// message or exited browsing. Root cause was in renderCircMessagesWithSelection
// (render.go): each message's height was measured with lipgloss.Height,
// which is strings.Count(s, "\n")+1 — correct once for a whole multi-line
// string, but wrong when applied per-message and summed, since it counts
// each message's own trailing "\n" as a phantom extra line. The summed
// (inflated) offsets desynced from the viewport's real line count, so
// millerPageNav kept computing a YOffset the viewport's own maxYOffset()
// clamped right back down — an invisible deadlock.
func TestChatrooms_DownThroughManyMessages_ReachesNewestAndExits(t *testing.T) {
	m := screens.NewChatroomsModel("neuromancer", nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 14}) // small: forces real scrolling
	m = m.SetRooms(sampleRooms())
	m, _ = sendChatroomSpecialKey(m, tea.KeyEnter)

	var msgs []model.Message
	for i := 1; i <= 10; i++ {
		msgs = append(msgs, model.Message{
			ID:        fmt.Sprintf("m%d", i),
			From:      model.User{Username: "molly"},
			Body:      fmt.Sprintf("message number %d", i),
			CreatedAt: time.Now().Add(time.Duration(i) * time.Minute),
		})
	}
	m = m.SetMessages("zion", msgs)

	// Walk all the way up to the oldest message first (m1), then test that
	// 'down' can walk all the way back down without getting stuck.
	for i := 0; i < 10; i++ {
		m, _ = sendChatroomSpecialKey(m, tea.KeyUp)
	}
	if m.SelectedMessageID() != "m1" {
		t.Fatalf("setup: SelectedMessageID() = %q after walking up, want m1 (oldest)", m.SelectedMessageID())
	}

	for i := 0; i < 9; i++ {
		m, _ = sendChatroomSpecialKey(m, tea.KeyDown)
		if m.SelectedMessageID() == "" {
			t.Fatalf("down got stuck or exited early after %d presses (expected still browsing)", i+1)
		}
	}
	if m.SelectedMessageID() != "m10" {
		t.Fatalf("SelectedMessageID() = %q after 9 downs from m1, want m10 (newest)", m.SelectedMessageID())
	}

	m, _ = sendChatroomSpecialKey(m, tea.KeyDown) // one more: past newest, exits browsing
	if m.SelectedMessageID() != "" {
		t.Errorf("expected browsing to exit after 'down' past the newest message, got selected=%q", m.SelectedMessageID())
	}
}

func TestChatrooms_FlagKey_OnOwnMessage_DoesNothing(t *testing.T) {
	m := screens.NewChatroomsModel("neuromancer", nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = m.SetRooms(sampleRooms())
	m, _ = sendChatroomSpecialKey(m, tea.KeyEnter)
	m = m.SetMessages("zion", []model.Message{
		{ID: "m1", From: model.User{Username: "neuromancer"}, Body: "mine", CreatedAt: time.Now()},
	})
	m, _ = sendChatroomSpecialKey(m, tea.KeyUp)

	_, cmd := sendChatroomKey(m, "!")
	if cmd != nil {
		t.Error("expected no cmd when flagging own message")
	}
}

func TestChatrooms_FlagKey_OnOtherMessage_FullFlowEmitsFlagMessageMsg(t *testing.T) {
	m := screens.NewChatroomsModel("neuromancer", nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = m.SetRooms(sampleRooms())
	m, _ = sendChatroomSpecialKey(m, tea.KeyEnter)
	m = m.SetMessages("zion", []model.Message{
		{ID: "m1", From: model.User{Username: "molly"}, Body: "not mine", CreatedAt: time.Now()},
	})
	m, _ = sendChatroomSpecialKey(m, tea.KeyUp)

	m, cmd := sendChatroomKey(m, "!")
	if cmd == nil {
		t.Fatal("expected a focus cmd from opening the flag prompt")
	}
	for _, r := range "spam" {
		m, _ = sendChatroomKey(m, string(r))
	}
	m, _ = sendChatroomSpecialKey(m, tea.KeyEnter)
	m, cmd = sendChatroomKey(m, "y")
	if cmd == nil {
		t.Fatal("expected a cmd after confirming")
	}
	// The real runtime feeds the cmd's message back through Update; do the same here.
	_, cmd = m.Update(cmd())
	if cmd == nil {
		t.Fatal("expected a cmd after routing FlagSubmitMsg through Update")
	}
	msg, ok := cmd().(screens.FlagMessageMsg)
	if !ok {
		t.Fatalf("expected FlagMessageMsg, got %T", cmd())
	}
	if msg.MessageID != "m1" {
		t.Errorf("MessageID = %q, want m1", msg.MessageID)
	}
	if msg.RoomID != "zion" {
		t.Errorf("RoomID = %q, want zion", msg.RoomID)
	}
	if msg.Reason != "spam" {
		t.Errorf("Reason = %q, want spam", msg.Reason)
	}
}

func TestChatrooms_DeleteKey_OnOwnMessage_OpensConfirm(t *testing.T) {
	m := screens.NewChatroomsModel("neuromancer", nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = m.SetRooms(sampleRooms())
	m, _ = sendChatroomSpecialKey(m, tea.KeyEnter)
	m = m.SetMessages("zion", []model.Message{
		{ID: "m1", From: model.User{Username: "neuromancer"}, Body: "mine", CreatedAt: time.Now()},
	})
	m, _ = sendChatroomSpecialKey(m, tea.KeyUp)

	m, _ = sendChatroomKey(m, "d")
	if m.ComposeEmpty() {
		t.Error("expected the delete-confirm overlay to be open after 'd' on your own message")
	}
}

func TestChatrooms_DeleteKey_OnOtherMessage_DoesNothing(t *testing.T) {
	m := screens.NewChatroomsModel("neuromancer", nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = m.SetRooms(sampleRooms())
	m, _ = sendChatroomSpecialKey(m, tea.KeyEnter)
	m = m.SetMessages("zion", []model.Message{
		{ID: "m1", From: model.User{Username: "molly"}, Body: "not mine", CreatedAt: time.Now()},
	})
	m, _ = sendChatroomSpecialKey(m, tea.KeyUp)

	m, cmd := sendChatroomKey(m, "d")
	if cmd != nil {
		t.Error("expected no cmd when deleting someone else's message")
	}
	if !m.ComposeEmpty() {
		t.Error("expected no delete-confirm overlay for someone else's message")
	}
}

func TestChatrooms_DeleteKey_OnAlreadyDeletedMessage_DoesNothing(t *testing.T) {
	m := screens.NewChatroomsModel("neuromancer", nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = m.SetRooms(sampleRooms())
	m, _ = sendChatroomSpecialKey(m, tea.KeyEnter)
	m = m.SetMessages("zion", []model.Message{
		{ID: "m1", From: model.User{Username: "neuromancer"}, Body: "[DELETED]", Deleted: true, CreatedAt: time.Now()},
	})
	m, _ = sendChatroomSpecialKey(m, tea.KeyUp)

	m, _ = sendChatroomKey(m, "d")
	if !m.ComposeEmpty() {
		t.Error("expected no delete-confirm overlay for an already-deleted message")
	}
}

func TestChatrooms_FlagKey_OnAlreadyDeletedMessage_DoesNothing(t *testing.T) {
	m := screens.NewChatroomsModel("neuromancer", nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = m.SetRooms(sampleRooms())
	m, _ = sendChatroomSpecialKey(m, tea.KeyEnter)
	m = m.SetMessages("zion", []model.Message{
		{ID: "m1", From: model.User{Username: "molly"}, Body: "[DELETED]", Deleted: true, CreatedAt: time.Now()},
	})
	m, _ = sendChatroomSpecialKey(m, tea.KeyUp)

	_, cmd := sendChatroomKey(m, "!")
	if cmd != nil {
		t.Error("expected no cmd when flagging an already-deleted message")
	}
}

func TestChatrooms_DeleteConfirm_Yes_EmitsDeleteRoomMessageMsg(t *testing.T) {
	m := screens.NewChatroomsModel("neuromancer", nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = m.SetRooms(sampleRooms())
	m, _ = sendChatroomSpecialKey(m, tea.KeyEnter)
	m = m.SetMessages("zion", []model.Message{
		{ID: "m1", From: model.User{Username: "neuromancer"}, Body: "mine", CreatedAt: time.Now()},
	})
	m, _ = sendChatroomSpecialKey(m, tea.KeyUp)
	m, _ = sendChatroomKey(m, "d")

	m, cmd := sendChatroomKey(m, "y")
	if cmd == nil {
		t.Fatal("expected a cmd after confirming delete")
	}
	msg, ok := cmd().(screens.DeleteRoomMessageMsg)
	if !ok {
		t.Fatalf("expected DeleteRoomMessageMsg, got %T", cmd())
	}
	if msg.MessageID != "m1" {
		t.Errorf("MessageID = %q, want m1", msg.MessageID)
	}
	if msg.RoomID != "zion" {
		t.Errorf("RoomID = %q, want zion", msg.RoomID)
	}
	if !m.ComposeEmpty() {
		t.Error("expected the delete-confirm overlay to close after confirming")
	}
}

func TestChatrooms_DeleteConfirm_No_Cancels(t *testing.T) {
	m := screens.NewChatroomsModel("neuromancer", nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = m.SetRooms(sampleRooms())
	m, _ = sendChatroomSpecialKey(m, tea.KeyEnter)
	m = m.SetMessages("zion", []model.Message{
		{ID: "m1", From: model.User{Username: "neuromancer"}, Body: "mine", CreatedAt: time.Now()},
	})
	m, _ = sendChatroomSpecialKey(m, tea.KeyUp)
	m, _ = sendChatroomKey(m, "d")

	m, cmd := sendChatroomKey(m, "n")
	if cmd != nil {
		t.Error("expected no cmd when declining the delete confirmation")
	}
	if !m.ComposeEmpty() {
		t.Error("expected the delete-confirm overlay to close after declining")
	}
	if m.SelectedMessageID() != "m1" {
		t.Errorf("expected the selection to remain on m1 after declining, got %q", m.SelectedMessageID())
	}
}

func TestChatrooms_ApplyMessageDeleted_RendersTombstoneInPlace(t *testing.T) {
	m := screens.NewChatroomsModel("neuromancer", nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = m.SetRooms(sampleRooms())
	m, _ = sendChatroomSpecialKey(m, tea.KeyEnter)
	m = m.SetMessages("zion", []model.Message{
		{ID: "m1", From: model.User{Username: "molly"}, Body: "top secret plans", CreatedAt: time.Now()},
		{ID: "m2", From: model.User{Username: "wintermute"}, Body: "unrelated message", CreatedAt: time.Now()},
	})

	m = m.ApplyMessageDeleted("m1")

	view := m.View()
	if !strings.Contains(view, "[DELETED]") {
		t.Errorf("expected a [DELETED] tombstone in the view, got: %q", view)
	}
	if strings.Contains(view, "top secret plans") {
		t.Errorf("expected the original body to be gone, got: %q", view)
	}
	if !strings.Contains(view, "unrelated message") {
		t.Errorf("expected the other message to be unaffected, got: %q", view)
	}
}

func TestChatrooms_PrependMessages_InsertsOlderMessagesAbove(t *testing.T) {
	m := screens.NewChatroomsModel("neuromancer", nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = m.SetRooms(sampleRooms())
	m, _ = sendChatroomSpecialKey(m, tea.KeyEnter)
	m = m.SetMessages("zion", []model.Message{
		{ID: "m1", From: model.User{Username: "molly"}, Body: "first message", CreatedAt: time.Now()},
	})

	m = m.PrependMessages("zion", []model.Message{
		{ID: "m0", From: model.User{Username: "molly"}, Body: "older message", CreatedAt: time.Now().Add(-time.Hour)},
	})

	view := m.View()
	oldIdx := strings.Index(view, "older message")
	newIdx := strings.Index(view, "first message")
	if oldIdx == -1 || newIdx == -1 {
		t.Fatalf("expected both messages in the rendered view, got: %q", view)
	}
	if oldIdx > newIdx {
		t.Error("expected the prepended (older) message to render above the existing message")
	}
}

func TestChatrooms_PrependMessages_EmptyMarksExhausted(t *testing.T) {
	m := screens.NewChatroomsModel("neuromancer", nil)
	m = m.SetRooms(sampleRooms())
	m, _ = sendChatroomSpecialKey(m, tea.KeyEnter)
	m = m.SetMessages("zion", []model.Message{
		{ID: "m1", From: model.User{Username: "molly"}, Body: "first message", CreatedAt: time.Now()},
	})

	m = m.PrependMessages("zion", nil)

	_, cmd := sendChatroomSpecialKey(m, tea.KeyUp)
	if cmd != nil {
		t.Error("expected no further history-load command once history is exhausted")
	}
}

func sampleConvWithMessage() []model.Conversation {
	return []model.Conversation{
		{
			ID:           "c1",
			Participants: []model.User{{Username: "neuromancer"}, {Username: "molly"}},
			Messages: []model.Message{
				{ID: "m1", From: model.User{Username: "molly"}, Body: "first message", CreatedAt: time.Now()},
			},
		},
	}
}

func TestCMail_GetFocusedURLs_NilInListMode(t *testing.T) {
	m := screens.NewCMailModel("neuromancer", nil)
	m = m.SetConversations(sampleConvWithMessage())
	if urls := m.GetFocusedURLs(); urls != nil {
		t.Errorf("expected nil URLs in list mode, got %v", urls)
	}
}

func TestCMail_GetFocusedURLs_AggregatesLoadedMessages(t *testing.T) {
	m := screens.NewCMailModel("neuromancer", nil)
	m = m.SetConversations([]model.Conversation{
		{
			ID:           "c1",
			Participants: []model.User{{Username: "neuromancer"}, {Username: "molly"}},
			Messages: []model.Message{
				{ID: "m1", From: model.User{Username: "molly"}, Body: "check https://example.com/one", CreatedAt: time.Now()},
				{ID: "m2", From: model.User{Username: "neuromancer"}, Body: "no links here", CreatedAt: time.Now()},
			},
		},
	})
	m, _ = sendSpecialKey(m, tea.KeyEnter) // opens "c1"

	urls := m.GetFocusedURLs()
	if len(urls) != 1 || urls[0] != "https://example.com/one" {
		t.Errorf("expected [https://example.com/one], got %v", urls)
	}
}

func TestCMail_UpAtTop_TriggersHistoryLoadThenGuards(t *testing.T) {
	m := screens.NewCMailModel("neuromancer", nil)
	m = m.SetConversations(sampleConvWithMessage())
	m, _ = sendSpecialKey(m, tea.KeyEnter)

	m, cmd := sendSpecialKey(m, tea.KeyUp)
	if cmd == nil {
		t.Fatal("expected a history-load command when scrolling up at the top")
	}

	_, cmd = sendSpecialKey(m, tea.KeyUp)
	if cmd != nil {
		t.Error("expected no command while a history load is already in flight")
	}
}

func TestCMail_PrependMessages_InsertsOlderMessagesAbove(t *testing.T) {
	m := screens.NewCMailModel("neuromancer", nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = m.SetConversations(sampleConvWithMessage())
	m, _ = sendSpecialKey(m, tea.KeyEnter)

	m = m.PrependMessages("c1", []model.Message{
		{ID: "m0", From: model.User{Username: "molly"}, Body: "older message", CreatedAt: time.Now().Add(-time.Hour)},
	})

	view := m.View()
	oldIdx := strings.Index(view, "older message")
	newIdx := strings.Index(view, "first message")
	if oldIdx == -1 || newIdx == -1 {
		t.Fatalf("expected both messages in the rendered view, got: %q", view)
	}
	if oldIdx > newIdx {
		t.Error("expected the prepended (older) message to render above the existing message")
	}
}

func TestCMail_PrependMessages_EmptyMarksExhausted(t *testing.T) {
	m := screens.NewCMailModel("neuromancer", nil)
	m = m.SetConversations(sampleConvWithMessage())
	m, _ = sendSpecialKey(m, tea.KeyEnter)

	m = m.PrependMessages("c1", nil)

	_, cmd := sendSpecialKey(m, tea.KeyUp)
	if cmd != nil {
		t.Error("expected no further history-load command once history is exhausted")
	}
}

// --- Load-failure handling ---

// alwaysFailClient wraps a working MockClient but fails every message-history
// fetch, for exercising the "couldn't load messages" error state.
type alwaysFailClient struct {
	*api.MockClient
}

func (c *alwaysFailClient) GetRoomMessages(roomID string, limit int, before int64) ([]model.Message, error) {
	return nil, errors.New("boom")
}

func (c *alwaysFailClient) GetMessages(convID string, limit int, before int64) ([]model.Message, error) {
	return nil, errors.New("boom")
}

// flakyOlderPageClient succeeds on the initial page (before == 0) but fails
// every older-page fetch, for exercising the loadingHistory stuck-true bug fix.
type flakyOlderPageClient struct {
	*api.MockClient
}

func (c *flakyOlderPageClient) GetRoomMessages(roomID string, limit int, before int64) ([]model.Message, error) {
	if before > 0 {
		return nil, errors.New("boom")
	}
	return c.MockClient.GetRoomMessages(roomID, limit, before)
}

func (c *flakyOlderPageClient) GetMessages(convID string, limit int, before int64) ([]model.Message, error) {
	if before > 0 {
		return nil, errors.New("boom")
	}
	return c.MockClient.GetMessages(convID, limit, before)
}

func TestChatrooms_LoadOlderFailure_ResetsLoadingHistory(t *testing.T) {
	client := &flakyOlderPageClient{MockClient: api.NewMockClient()}
	m := screens.NewChatroomsModel("neuromancer", client)
	m = m.SetRooms(sampleRooms())
	m, _ = sendChatroomSpecialKey(m, tea.KeyEnter)
	m = m.SetMessages("zion", []model.Message{
		{ID: "m1", From: model.User{Username: "molly"}, Body: "first message", CreatedAt: time.Now()},
	})

	m, cmd := sendChatroomSpecialKey(m, tea.KeyUp)
	if cmd == nil {
		t.Fatal("expected a history-load command when scrolling up at the top")
	}
	m, _ = m.Update(cmd()) // circErrMsg — must reset loadingHistory, not leave it stuck

	_, cmd = sendChatroomSpecialKey(m, tea.KeyUp)
	if cmd == nil {
		t.Error("expected loadingHistory to reset after a failed load, allowing a retry")
	}
}

func TestChatrooms_View_ShowsErrorWhenInitialLoadFails(t *testing.T) {
	client := &alwaysFailClient{MockClient: api.NewMockClient()}
	m := screens.NewChatroomsModel("neuromancer", client)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = m.SetRooms(sampleRooms())
	m, cmd := sendChatroomSpecialKey(m, tea.KeyEnter)
	if cmd == nil {
		t.Fatal("expected a command batch after Enter")
	}
	for _, resolved := range resolveMsgs(cmd) {
		m, _ = m.Update(resolved)
	}

	view := m.View()
	if !strings.Contains(view, "couldn't load messages") {
		t.Errorf("expected error state in view, got: %q", view)
	}
}

func TestCMail_LoadOlderFailure_ResetsLoadingHistory(t *testing.T) {
	client := &flakyOlderPageClient{MockClient: api.NewMockClient()}
	m := screens.NewCMailModel("neuromancer", client)
	m = m.SetConversations(sampleConvWithMessage())
	m, _ = sendSpecialKey(m, tea.KeyEnter)

	m, cmd := sendSpecialKey(m, tea.KeyUp)
	if cmd == nil {
		t.Fatal("expected a history-load command when scrolling up at the top")
	}
	m, _ = m.Update(cmd()) // cmailErrMsg — must reset loadingHistory, not leave it stuck

	_, cmd = sendSpecialKey(m, tea.KeyUp)
	if cmd == nil {
		t.Error("expected loadingHistory to reset after a failed load, allowing a retry")
	}
}

func TestCMail_View_ShowsErrorWhenInitialLoadFails(t *testing.T) {
	client := &alwaysFailClient{MockClient: api.NewMockClient()}
	m := screens.NewCMailModel("neuromancer", client)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	// A conversation with no pre-loaded messages, so the failing initial fetch
	// is what determines the detail view's content.
	m = m.SetConversations([]model.Conversation{
		{ID: "c1", Participants: []model.User{{Username: "neuromancer"}, {Username: "molly"}}},
	})
	m, cmd := sendSpecialKey(m, tea.KeyEnter)
	if cmd == nil {
		t.Fatal("expected a command batch after Enter")
	}
	for _, resolved := range resolveMsgs(cmd) {
		m, _ = m.Update(resolved)
	}

	view := m.View()
	if !strings.Contains(view, "couldn't load messages") {
		t.Errorf("expected error state in view, got: %q", view)
	}
}

// resolveMsgs executes cmd and flattens one level of tea.BatchMsg into the
// individual resulting messages, without recursively executing whatever
// commands those messages' own Update() calls return (some of those, like
// waitForRoomMsg, block on a live channel and must not run in a unit test).
func resolveMsgs(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if msg == nil {
		return nil
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			if c == nil {
				continue
			}
			if m := c(); m != nil {
				out = append(out, m)
			}
		}
		return out
	}
	return []tea.Msg{msg}
}
