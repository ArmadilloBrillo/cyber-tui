package screens_test

import (
	"errors"
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

func TestChatrooms_UpAtTop_NoMessagesNoCmd(t *testing.T) {
	m := screens.NewChatroomsModel("neuromancer", nil)
	m = m.SetRooms(sampleRooms())
	m, _ = sendChatroomSpecialKey(m, tea.KeyEnter) // no messages loaded yet

	_, cmd := sendChatroomSpecialKey(m, tea.KeyUp)
	if cmd != nil {
		t.Error("expected no history-load command with no messages loaded")
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

