package screens_test

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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
	if sendMsg.RoomID != "r1" {
		t.Errorf("expected roomID='r1', got %q", sendMsg.RoomID)
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

