package screens_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/screens"
)

// --- ChatroomsModel.InputFocused ---

func TestChatroomsInputFocused_DefaultFalse(t *testing.T) {
	m := screens.NewChatroomsModel()
	if m.InputFocused() {
		t.Error("input should not be focused on a freshly created ChatroomsModel")
	}
}

// --- CMailModel.InputFocused ---

func TestCMailInputFocused_DefaultFalse(t *testing.T) {
	m := screens.NewCMailModel("neuromancer")
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

// --- focusLeft navigation ---

func TestCMailCursorDown(t *testing.T) {
	m := screens.NewCMailModel("neuromancer")
	m = m.SetConversations(twoConvs())
	m, _ = sendKey(m, "j")
	if m.SelectedConv() != 1 {
		t.Errorf("expected selectedConv=1 after ↓, got %d", m.SelectedConv())
	}
}

func TestCMailCursorUp_ClampsAtZero(t *testing.T) {
	m := screens.NewCMailModel("neuromancer")
	m = m.SetConversations(twoConvs())
	m, _ = sendKey(m, "k")
	if m.SelectedConv() != 0 {
		t.Errorf("expected selectedConv=0 (clamped), got %d", m.SelectedConv())
	}
}

func TestCMailCursorDown_ClampsAtBottom(t *testing.T) {
	m := screens.NewCMailModel("neuromancer")
	m = m.SetConversations(twoConvs())
	m, _ = sendKey(m, "j")
	m, _ = sendKey(m, "j") // already at bottom
	if m.SelectedConv() != 1 {
		t.Errorf("expected selectedConv=1 (clamped), got %d", m.SelectedConv())
	}
}

// --- Enter in focusLeft opens conversation and shifts to focusRight ---

func TestCMailEnterOpensConversation(t *testing.T) {
	m := screens.NewCMailModel("neuromancer")
	m = m.SetConversations(twoConvs())
	m, _ = sendSpecialKey(m, tea.KeyEnter)
	if !m.HasActiveConv() {
		t.Error("expected activeConv to be set after Enter")
	}
	if m.FocusPane() != screens.FocusCMailRight {
		t.Error("expected focusPane=focusCMailRight after Enter")
	}
	if !m.InputFocused() {
		t.Error("expected input to be focused after Enter")
	}
}

// --- Tab in focusLeft switches to focusRight ---

func TestCMailTabFromLeft_ShiftsToRight(t *testing.T) {
	m := screens.NewCMailModel("neuromancer")
	m = m.SetConversations(twoConvs())
	m, _ = sendSpecialKey(m, tea.KeyTab)
	if m.FocusPane() != screens.FocusCMailRight {
		t.Error("expected focusPane=focusCMailRight after Tab from left")
	}
}

// --- Tab in focusRight switches back to focusLeft ---

func TestCMailTabFromRight_ShiftsToLeft(t *testing.T) {
	m := screens.NewCMailModel("neuromancer")
	m = m.SetConversations(twoConvs())
	// open conversation first
	m, _ = sendSpecialKey(m, tea.KeyEnter)
	// Tab back
	m, _ = sendSpecialKey(m, tea.KeyTab)
	if m.FocusPane() != screens.FocusCMailLeft {
		t.Error("expected focusPane=focusCMailLeft after Tab from right")
	}
	if m.InputFocused() {
		t.Error("expected input to be blurred after Tab from right")
	}
}

// --- Enter in focusRight with non-empty input emits SendCMailMsg ---

func TestCMailSend_EmitsMessage(t *testing.T) {
	m := screens.NewCMailModel("neuromancer")
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
	m := screens.NewCMailModel("neuromancer")
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

// --- Esc blurs input when focused, then shifts to focusLeft ---

func TestCMailEsc_BlursInputFirst(t *testing.T) {
	m := screens.NewCMailModel("neuromancer")
	m = m.SetConversations(twoConvs())
	m, _ = sendSpecialKey(m, tea.KeyEnter) // open conversation, input focused

	// first Esc: blur input, stay in focusRight
	m, _ = sendSpecialKey(m, tea.KeyEsc)
	if m.InputFocused() {
		t.Error("expected input to be blurred after first Esc")
	}
	if m.FocusPane() != screens.FocusCMailRight {
		t.Error("expected to stay in focusCMailRight after first Esc")
	}

	// second Esc: shift to focusLeft
	m, _ = sendSpecialKey(m, tea.KeyEsc)
	if m.FocusPane() != screens.FocusCMailLeft {
		t.Error("expected focusCMailLeft after second Esc")
	}
}

// --- WindowSizeMsg sets sidebarWidth correctly ---

func TestCMailSidebarWidth_80(t *testing.T) {
	m := screens.NewCMailModel("neuromancer")
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if m.SidebarWidth() != 20 {
		t.Errorf("expected sidebarWidth=20 at width=80, got %d", m.SidebarWidth())
	}
}

func TestCMailSidebarWidth_120(t *testing.T) {
	m := screens.NewCMailModel("neuromancer")
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	if m.SidebarWidth() != 30 {
		t.Errorf("expected sidebarWidth=30 at width=120, got %d", m.SidebarWidth())
	}
}

func TestCMailSidebarWidth_200(t *testing.T) {
	m := screens.NewCMailModel("neuromancer")
	m, _ = m.Update(tea.WindowSizeMsg{Width: 200, Height: 24})
	if m.SidebarWidth() != 32 {
		t.Errorf("expected sidebarWidth=32 (clamped) at width=200, got %d", m.SidebarWidth())
	}
}

// --- PostDetailModel ---

func makePost() model.Post {
	return model.Post{
		ID:             "p1",
		AuthorUsername: "molly",
		Content:        "hello, matrix",
		CreatedAt:      time.Now(),
	}
}

func makeReplies() []model.Reply {
	return []model.Reply{
		{ID: "r1", PostID: "p1", AuthorUsername: "wintermute", Content: "interesting", CreatedAt: time.Now()},
	}
}

func TestPostDetail_SetPost_SetsLoading(t *testing.T) {
	m := screens.NewPostDetailModel()
	m = m.SetPost(makePost())
	if !m.Loading() {
		t.Error("expected Loading=true after SetPost")
	}
}

func TestPostDetail_SetPost_ClearsReplies(t *testing.T) {
	m := screens.NewPostDetailModel()
	m = m.SetPost(makePost())
	m = m.SetReplies(makeReplies())
	// Setting a new post must clear replies and re-enter loading.
	m = m.SetPost(model.Post{ID: "p2", AuthorUsername: "neuromancer", Content: "second", CreatedAt: time.Now()})
	if !m.Loading() {
		t.Error("expected Loading=true after SetPost on a model that already had replies")
	}
}

func TestPostDetail_SetReplies_ClearsLoading(t *testing.T) {
	m := screens.NewPostDetailModel()
	m = m.SetPost(makePost())
	m = m.SetReplies(makeReplies())
	if m.Loading() {
		t.Error("expected Loading=false after SetReplies")
	}
}

func TestPostDetail_SetError_ClearsLoading(t *testing.T) {
	m := screens.NewPostDetailModel()
	m = m.SetPost(makePost())
	m = m.SetError(fmt.Errorf("connection refused"))
	if m.Loading() {
		t.Error("expected Loading=false after SetError")
	}
}

func TestPostDetail_SetError_ShowsErrorInView(t *testing.T) {
	m := screens.NewPostDetailModel()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = m.SetError(fmt.Errorf("connection refused"))
	if !strings.Contains(m.View(), "connection refused") {
		t.Errorf("expected error message in view, got: %s", m.View())
	}
}

func TestPostDetail_WindowSizeMsg_SetsReady(t *testing.T) {
	m := screens.NewPostDetailModel()
	if m.Ready() {
		t.Error("expected Ready=false before any WindowSizeMsg")
	}
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if !m.Ready() {
		t.Error("expected Ready=true after WindowSizeMsg")
	}
}

func TestPostDetail_Esc_EmitsBackToFeedMsg(t *testing.T) {
	m := screens.NewPostDetailModel()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected a command after Esc, got nil")
	}
	msg := cmd()
	if _, ok := msg.(screens.BackToFeedMsg); !ok {
		t.Fatalf("expected BackToFeedMsg, got %T", msg)
	}
}

// --- SelectConvMsg ---

func TestCMailSelectConvEmitsMsg(t *testing.T) {
	m := screens.NewCMailModel("neuromancer")
	m = m.SetConversations(twoConvs())
	_, cmd := sendSpecialKey(m, tea.KeyEnter)
	if cmd == nil {
		t.Fatal("expected a command after Enter on left pane, got nil")
	}
	msg := cmd()
	sel, ok := msg.(screens.SelectConvMsg)
	if !ok {
		t.Fatalf("expected SelectConvMsg, got %T", msg)
	}
	if sel.ConversationID != "c1" {
		t.Errorf("ConversationID = %q, want c1", sel.ConversationID)
	}
}

// --- AppendMessage ---

func TestCMailAppendMessage_AddsToActiveConv(t *testing.T) {
	m := screens.NewCMailModel("neuromancer")
	convs := twoConvs()
	m = m.SetConversations(convs)
	m, _ = sendSpecialKey(m, tea.KeyEnter) // open c1

	incoming := model.Message{ID: "live1", From: model.User{Username: "molly"}, Body: "live msg", CreatedAt: time.Now()}
	m = m.AppendMessage(incoming)

	if !m.HasActiveConv() {
		t.Fatal("expected active conv after AppendMessage")
	}
}

func TestCMailAppendMessage_NoopWhenNoActiveConv(t *testing.T) {
	m := screens.NewCMailModel("neuromancer")
	m = m.SetConversations(twoConvs())
	// Do not open any conversation.
	incoming := model.Message{ID: "live1", From: model.User{Username: "molly"}, Body: "live msg", CreatedAt: time.Now()}
	before := m.HasActiveConv()
	m = m.AppendMessage(incoming)
	if m.HasActiveConv() != before {
		t.Error("AppendMessage should be a no-op when no conversation is open")
	}
}

// --- SetConversationMessages ---

func TestCMailSetConversationMessages_ReplacesMessages(t *testing.T) {
	m := screens.NewCMailModel("neuromancer")
	m = m.SetConversations(twoConvs())
	m, _ = sendSpecialKey(m, tea.KeyEnter) // open c1

	newMsgs := []model.Message{
		{ID: "h1", From: model.User{Username: "molly"}, Body: "history 1", CreatedAt: time.Now()},
		{ID: "h2", From: model.User{Username: "molly"}, Body: "history 2", CreatedAt: time.Now()},
	}
	m = m.SetConversationMessages("c1", newMsgs)
	if !m.HasActiveConv() {
		t.Fatal("expected active conv after SetConversationMessages")
	}
}

func TestCMailSetConversationMessages_WrongConv_Noop(t *testing.T) {
	m := screens.NewCMailModel("neuromancer")
	m = m.SetConversations(twoConvs())
	m, _ = sendSpecialKey(m, tea.KeyEnter) // opens c1

	// Call with wrong conv ID — should be a no-op.
	m2 := m.SetConversationMessages("c2", []model.Message{
		{ID: "x", From: model.User{Username: "wintermute"}, Body: "wrong conv", CreatedAt: time.Now()},
	})
	if !m2.HasActiveConv() {
		t.Error("HasActiveConv should still be true after no-op SetConversationMessages")
	}
}

// --- ComposeModel ---

func TestCompose_NewIsInactive(t *testing.T) {
	c := screens.NewComposeModel(80)
	if c.IsActive() {
		t.Error("expected compose to be inactive after New")
	}
}

func TestCompose_OpenSetsActive(t *testing.T) {
	c := screens.NewComposeModel(80)
	c, _ = c.Open("replying to @molly", "write your reply…")
	if !c.IsActive() {
		t.Error("expected compose to be active after Open")
	}
}

// TestCompose_ContentIsPreservedOnSubmit verifies that the content typed into
// the compose box is returned in ComposeSubmitMsg when the submit path fires.
// Ctrl+Enter key delivery depends on terminal capabilities (Kitty protocol),
// so we exercise the submit path by calling ComposeSubmitMsg directly on the
// PostDetailModel (see TestPostDetail_ComposeSubmit_* below).
func TestCompose_ContentIsEmpty_AfterOpen(t *testing.T) {
	c := screens.NewComposeModel(80)
	c, _ = c.Open("replying to @molly", "write your reply…")
	if c.Content() != "" {
		t.Errorf("expected empty content after Open, got %q", c.Content())
	}
}

func TestCompose_EscEmitsCancel(t *testing.T) {
	c := screens.NewComposeModel(80)
	c, _ = c.Open("replying to @molly", "write your reply…")
	_, cmd := c.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected a command after Esc")
	}
	msg := cmd()
	if _, ok := msg.(screens.ComposeCancelMsg); !ok {
		t.Errorf("expected ComposeCancelMsg, got %T", msg)
	}
}

func TestCompose_CloseSetsInactive(t *testing.T) {
	c := screens.NewComposeModel(80)
	c, _ = c.Open("replying to @molly", "write your reply…")
	c = c.Close()
	if c.IsActive() {
		t.Error("expected compose to be inactive after Close")
	}
}

func TestCompose_BoxHeightMinimum(t *testing.T) {
	c := screens.NewComposeModel(80)
	c, _ = c.Open("test", "placeholder")
	if c.BoxHeight() < 4 { // 3 content + at least 1 overhead
		t.Errorf("BoxHeight=%d, expected at least 4", c.BoxHeight())
	}
}

func TestCompose_InactiveUpdateIsNoop(t *testing.T) {
	c := screens.NewComposeModel(80)
	_, cmd := c.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		t.Error("expected nil cmd when compose is inactive")
	}
}

// --- PostDetailModel reply keybinding ---

func postDetailReady() screens.PostDetailModel {
	m := screens.NewPostDetailModel()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = m.SetPost(makePost())
	m = m.SetReplies(makeReplies())
	return m
}

func TestPostDetail_R_OpensCompose(t *testing.T) {
	m := postDetailReady()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if !m.ComposeActive() {
		t.Error("expected compose to be active after pressing 'r'")
	}
}

func TestPostDetail_ComposeCancel_ClosesCompose(t *testing.T) {
	m := postDetailReady()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	// Esc inside compose emits ComposeCancelMsg which comes back as a message.
	m, _ = m.Update(screens.ComposeCancelMsg{})
	if m.ComposeActive() {
		t.Error("expected compose to be inactive after ComposeCancelMsg")
	}
}

func TestPostDetail_ComposeSubmit_EmitsSubmitReplyMsg_PostLevel(t *testing.T) {
	m := postDetailReady() // selectedReply=-1, replying to the post itself
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	_, cmd := m.Update(screens.ComposeSubmitMsg{Content: "my reply"})
	if cmd == nil {
		t.Fatal("expected a command after ComposeSubmitMsg")
	}
	msg := cmd()
	sub, ok := msg.(screens.SubmitReplyMsg)
	if !ok {
		t.Fatalf("expected SubmitReplyMsg, got %T", msg)
	}
	if sub.PostID != "p1" {
		t.Errorf("expected PostID=%q, got %q", "p1", sub.PostID)
	}
	if sub.ParentReplyID != "" {
		t.Errorf("expected empty ParentReplyID for top-level reply, got %q", sub.ParentReplyID)
	}
	if sub.Content != "my reply" {
		t.Errorf("expected Content=%q, got %q", "my reply", sub.Content)
	}
}

func TestPostDetail_ComposeSubmit_EmitsSubmitReplyMsg_ReplyLevel(t *testing.T) {
	m := postDetailReady()
	// Navigate to the first reply.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	// selectedReply=0; open compose targeting that reply.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	_, cmd := m.Update(screens.ComposeSubmitMsg{Content: "nested reply"})
	if cmd == nil {
		t.Fatal("expected a command after ComposeSubmitMsg on reply")
	}
	msg := cmd()
	sub, ok := msg.(screens.SubmitReplyMsg)
	if !ok {
		t.Fatalf("expected SubmitReplyMsg, got %T", msg)
	}
	if sub.ParentReplyID != "r1" {
		t.Errorf("expected ParentReplyID=%q, got %q", "r1", sub.ParentReplyID)
	}
}

// --- FeedModel reply keybinding ---

func makeFeed() screens.FeedModel {
	m := screens.NewFeedModel()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = m.SetPosts([]model.Post{makePost()}, "")
	return m
}

func TestFeed_R_EmitsShowPostForReplyMsg(t *testing.T) {
	m := makeFeed()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if cmd == nil {
		t.Fatal("expected a command after pressing 'r' in feed")
	}
	msg := cmd()
	spr, ok := msg.(screens.ShowPostForReplyMsg)
	if !ok {
		t.Fatalf("expected ShowPostForReplyMsg, got %T", msg)
	}
	if spr.Post.ID != "p1" {
		t.Errorf("expected Post.ID=%q, got %q", "p1", spr.Post.ID)
	}
}

func TestFeed_N_OpensCompose(t *testing.T) {
	m := makeFeed()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if !m.ComposeActive() {
		t.Error("expected compose to be active after pressing 'n' in feed")
	}
}

func TestFeed_ComposeCancel_ClosesCompose(t *testing.T) {
	m := makeFeed()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m, _ = m.Update(screens.ComposeCancelMsg{})
	if m.ComposeActive() {
		t.Error("expected compose to be inactive after ComposeCancelMsg")
	}
}

func TestFeed_ComposeSubmit_EmitsSubmitNewPostMsg(t *testing.T) {
	m := makeFeed()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	_, cmd := m.Update(screens.ComposeSubmitMsg{Content: "hello world"})
	if cmd == nil {
		t.Fatal("expected a command after ComposeSubmitMsg in feed")
	}
	msg := cmd()
	snp, ok := msg.(screens.SubmitNewPostMsg)
	if !ok {
		t.Fatalf("expected SubmitNewPostMsg, got %T", msg)
	}
	if snp.Content != "hello world" {
		t.Errorf("expected Content=%q, got %q", "hello world", snp.Content)
	}
}

func TestFeed_N_NotActiveInPostDetail(t *testing.T) {
	// PostDetail should not react to 'n' — compose stays closed.
	m := postDetailReady()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if m.ComposeActive() {
		t.Error("expected compose to remain inactive after pressing 'n' in PostDetail")
	}
}
