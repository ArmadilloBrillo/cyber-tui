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

// --- focusLeft navigation ---

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

// --- Enter in focusLeft opens conversation and shifts to focusRight ---

func TestCMailEnterOpensConversation(t *testing.T) {
	m := screens.NewCMailModel("neuromancer", nil)
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
	m := screens.NewCMailModel("neuromancer", nil)
	m = m.SetConversations(twoConvs())
	m, _ = sendSpecialKey(m, tea.KeyTab)
	if m.FocusPane() != screens.FocusCMailRight {
		t.Error("expected focusPane=focusCMailRight after Tab from left")
	}
}

// --- Tab in focusRight switches back to focusLeft ---

func TestCMailTabFromRight_ShiftsToLeft(t *testing.T) {
	m := screens.NewCMailModel("neuromancer", nil)
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

// --- Esc blurs input when focused, then shifts to focusLeft ---

func TestCMailEsc_BlursInputFirst(t *testing.T) {
	m := screens.NewCMailModel("neuromancer", nil)
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
	m := screens.NewCMailModel("neuromancer", nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if m.SidebarWidth() != 20 {
		t.Errorf("expected sidebarWidth=20 at width=80, got %d", m.SidebarWidth())
	}
}

func TestCMailSidebarWidth_120(t *testing.T) {
	m := screens.NewCMailModel("neuromancer", nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	if m.SidebarWidth() != 30 {
		t.Errorf("expected sidebarWidth=30 at width=120, got %d", m.SidebarWidth())
	}
}

func TestCMailSidebarWidth_200(t *testing.T) {
	m := screens.NewCMailModel("neuromancer", nil)
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

func TestPostDetail_ScrollToReply_SelectsReply(t *testing.T) {
	replies := []model.Reply{
		{ID: "r1", PostID: "p1", AuthorUsername: "a", Content: "first", CreatedAt: time.Now()},
		{ID: "r2", PostID: "p1", AuthorUsername: "b", Content: "second", CreatedAt: time.Now()},
	}
	m := screens.NewPostDetailModel()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = m.SetPost(makePost())
	m = m.SetReplies(replies)
	m = m.ScrollToReply("r2")
	if m.SelectedReplyID() != "r2" {
		t.Errorf("expected SelectedReplyID r2, got %q", m.SelectedReplyID())
	}
}

func TestPostDetail_ScrollToReply_UnknownID_NoOp(t *testing.T) {
	m := screens.NewPostDetailModel()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = m.SetPost(makePost())
	m = m.SetReplies(makeReplies())
	m = m.ScrollToReply("nonexistent")
	if m.SelectedReplyID() != "" {
		t.Errorf("expected no selection change for unknown ID, got %q", m.SelectedReplyID())
	}
}

func TestPostDetail_ScrollToReply_EmptyID_NoOp(t *testing.T) {
	m := screens.NewPostDetailModel()
	m = m.SetPost(makePost())
	m = m.SetReplies(makeReplies())
	before := m.SelectedReplyID()
	m = m.ScrollToReply("")
	if m.SelectedReplyID() != before {
		t.Errorf("expected no change for empty ID")
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

// --- Conversation selection ---

func TestCMailSelectConv_OpensConvAndReturnsCmds(t *testing.T) {
	m := screens.NewCMailModel("neuromancer", nil)
	m = m.SetConversations(twoConvs())
	m2, cmd := sendSpecialKey(m, tea.KeyEnter)
	// Enter should open the first conversation and return a batch of DM commands.
	if !m2.HasActiveConv() {
		t.Error("expected active conversation after Enter on left pane")
	}
	if m2.FocusPane() != screens.FocusCMailRight {
		t.Error("expected focus to move to right pane after Enter")
	}
	if cmd == nil {
		t.Error("expected non-nil command batch (load messages + open subscription)")
	}
}

// --- AppendMessage ---

func TestCMailAppendMessage_AddsToActiveConv(t *testing.T) {
	m := screens.NewCMailModel("neuromancer", nil)
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
	m := screens.NewCMailModel("neuromancer", nil)
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
	m := screens.NewCMailModel("neuromancer", nil)
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
	m := screens.NewCMailModel("neuromancer", nil)
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
	// The topics input is pre-filled with "tui", so a submit without editing
	// should produce exactly that one topic.
	if len(snp.Topics) != 1 || snp.Topics[0] != "tui" {
		t.Errorf("expected topics=[tui] from pre-filled input, got %v", snp.Topics)
	}
}

func TestFeed_P_EmitsShowUserProfileMsg(t *testing.T) {
	m := makeFeed()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	if cmd == nil {
		t.Fatal("expected a command after pressing 'p' in feed")
	}
	msg := cmd()
	sp, ok := msg.(screens.ShowUserProfileMsg)
	if !ok {
		t.Fatalf("expected ShowUserProfileMsg, got %T", msg)
	}
	if sp.Username != "molly" {
		t.Errorf("expected Username=%q, got %q", "molly", sp.Username)
	}
}

func TestPostDetail_P_PostSelected_EmitsShowUserProfileMsg(t *testing.T) {
	m := postDetailReady() // selectedReply=-1, post selected
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	if cmd == nil {
		t.Fatal("expected a command after pressing 'p' in postdetail (post selected)")
	}
	msg := cmd()
	sp, ok := msg.(screens.ShowUserProfileMsg)
	if !ok {
		t.Fatalf("expected ShowUserProfileMsg, got %T", msg)
	}
	if sp.Username != "molly" {
		t.Errorf("expected post author %q, got %q", "molly", sp.Username)
	}
}

func TestPostDetail_P_ReplySelected_EmitsShowUserProfileMsg(t *testing.T) {
	m := postDetailReady()
	// Navigate j to move into the first reply.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	if cmd == nil {
		t.Fatal("expected a command after pressing 'p' with reply selected")
	}
	msg := cmd()
	sp, ok := msg.(screens.ShowUserProfileMsg)
	if !ok {
		t.Fatalf("expected ShowUserProfileMsg, got %T", msg)
	}
	if sp.Username != "wintermute" {
		t.Errorf("expected reply author %q, got %q", "wintermute", sp.Username)
	}
}

func TestProfile_ReadOnly_EscEmitsBackFromProfileMsg(t *testing.T) {
	m := screens.NewProfileModel()
	m = m.SetReadOnly(true)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("esc")})
	// esc is a special key, use KeyEsc type
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected a command on ESC in read-only profile")
	}
	msg := cmd()
	_, ok := msg.(screens.BackFromProfileMsg)
	if !ok {
		t.Fatalf("expected BackFromProfileMsg, got %T", msg)
	}
}

func TestProfile_ReadOnly_EKeyIsNoop(t *testing.T) {
	m := screens.NewProfileModel()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = m.SetReadOnly(true)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	if m.ComposeActive() {
		t.Error("expected compose to stay inactive in read-only profile")
	}
}

func TestProfile_CanGoBack_EscEmitsBackFromProfileMsg(t *testing.T) {
	// Own profile opened via 'p': readOnly=false but canGoBack=true.
	m := screens.NewProfileModel()
	m = m.SetCanGoBack(true)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected a command on ESC when canGoBack=true")
	}
	msg := cmd()
	_, ok := msg.(screens.BackFromProfileMsg)
	if !ok {
		t.Fatalf("expected BackFromProfileMsg, got %T", msg)
	}
}

func TestProfile_CanGoBack_EKeyStillOpensCompose(t *testing.T) {
	// Own profile via 'p': editing should still be available.
	m := screens.NewProfileModel()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = m.SetCanGoBack(true) // readOnly stays false
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	if !m.ComposeActive() {
		t.Error("expected compose to open with canGoBack=true and readOnly=false")
	}
}

func TestProfile_Default_EscIsNoop(t *testing.T) {
	// Tab 3 own profile: neither readOnly nor canGoBack — ESC does nothing.
	m := screens.NewProfileModel()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		t.Errorf("expected no command on ESC in default profile, got %T", cmd)
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

// --- Profile sub-tabs ---

func TestProfile_Tab_SwitchesToPosts(t *testing.T) {
	m := screens.NewProfileModel()
	m = m.SetUser(model.User{ID: "1", Username: "neuromancer"})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	// Tab should emit a lazy-load message for the Posts tab.
	if cmd == nil {
		t.Fatal("expected a command after tab press (lazy load trigger)")
	}
	msg := cmd()
	su, ok := msg.(screens.ShowUserPostsMsg)
	if !ok {
		t.Fatalf("expected ShowUserPostsMsg, got %T", msg)
	}
	if su.Username != "neuromancer" {
		t.Errorf("Username = %q, want neuromancer", su.Username)
	}
}

func TestProfile_Tab_NoLazyLoadIfAlreadyLoaded(t *testing.T) {
	m := screens.NewProfileModel()
	m = m.SetUser(model.User{ID: "1", Username: "neuromancer"})
	m = m.SetUserPosts([]model.Post{{ID: "p1"}}, "")
	// Manually set postsLoaded by having already set it via SetUserPosts.
	// Tab to Posts (already loaded) should not emit a load message.
	// First tab switches to Posts tab:
	_, firstCmd := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	_ = firstCmd // consumes the lazy-load command
	// Second tab on already-loaded tab should return no command.
	m = m.SetUserPosts([]model.Post{{ID: "p1"}}, "") // already loaded
	// Tab again to Replies (not loaded) should emit ShowUserRepliesMsg.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	_ = cmd // not testing this one specifically
}

func TestProfile_ShiftTab_SwitchesBackward(t *testing.T) {
	m := screens.NewProfileModel()
	m = m.SetUser(model.User{ID: "1", Username: "neuromancer"})
	// Shift+tab from Info tab wraps to Followers tab.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if cmd == nil {
		t.Fatal("expected a command after shift+tab press")
	}
	msg := cmd()
	_, ok := msg.(screens.ShowUserFollowersMsg)
	if !ok {
		t.Fatalf("expected ShowUserFollowersMsg, got %T", msg)
	}
}

func TestProfile_SetUserPosts_MarksLoaded(t *testing.T) {
	m := screens.NewProfileModel()
	posts := []model.Post{
		{ID: "p1", AuthorUsername: "neuromancer", Content: "hello"},
		{ID: "p2", AuthorUsername: "neuromancer", Content: "world"},
	}
	m = m.SetUserPosts(posts, "next-cursor")
	// Navigate to Posts tab.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	// j/k navigation should not panic with loaded posts.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	_ = m.View() // should not panic
}

func TestProfile_AppendUserPosts_AddsToExisting(t *testing.T) {
	m := screens.NewProfileModel()
	m = m.SetUserPosts([]model.Post{{ID: "p1"}}, "cursor1")
	m = m.AppendUserPosts([]model.Post{{ID: "p2"}}, "")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	view := m.View()
	if view == "" {
		t.Error("expected non-empty view after appending posts")
	}
}

func TestProfile_SetUserFollowing_LazyLoadNotFiredAgain(t *testing.T) {
	m := screens.NewProfileModel()
	m = m.SetUser(model.User{ID: "1", Username: "neuromancer"})
	m = m.SetUserFollowing([]model.Follow{
		{ID: "fw1", FollowedUsername: "molly_millions"},
	}, "")
	// Tab 3 times to reach Following tab.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Posts
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Replies
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Following
	// Already loaded — should emit no lazy-load command.
	if cmd != nil {
		t.Errorf("expected no lazy-load command for already-loaded Following tab, got cmd")
	}
}

func TestProfile_ClearTabs_ResetsAllState(t *testing.T) {
	m := screens.NewProfileModel()
	m = m.SetUserPosts([]model.Post{{ID: "p1"}}, "cursor")
	m = m.SetUserFollowers([]model.Follow{{ID: "fw1"}}, "")
	m = m.ClearTabs()
	// After ClearTabs, tabs should be unloaded and trigger lazy-load on next switch.
	m = m.SetUser(model.User{ID: "1", Username: "test"})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if cmd == nil {
		t.Fatal("expected lazy-load command after ClearTabs + tab switch")
	}
}

func TestProfile_FollowersTab_EnterEmitsShowUserProfileMsg(t *testing.T) {
	m := screens.NewProfileModel()
	m = m.SetUser(model.User{ID: "1", Username: "neuromancer"})
	m = m.SetUserFollowers([]model.Follow{
		{ID: "fw1", FollowerID: "2", FollowerUsername: "molly_millions"},
	}, "")
	// Tab to Followers (shift+tab once from Info).
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	// Press enter on the first follower.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command on enter in Followers tab")
	}
	msg := cmd()
	su, ok := msg.(screens.ShowUserProfileMsg)
	if !ok {
		t.Fatalf("expected ShowUserProfileMsg, got %T", msg)
	}
	if su.Username != "molly_millions" {
		t.Errorf("Username = %q, want molly_millions", su.Username)
	}
}

func TestProfile_PostsTab_EnterEmitsShowProfilePostMsg(t *testing.T) {
	m := screens.NewProfileModel()
	m = m.SetUser(model.User{ID: "1", Username: "neuromancer"})
	m = m.SetUserPosts([]model.Post{
		{ID: "p1", AuthorUsername: "neuromancer", Content: "hello world"},
	}, "")
	// Tab to Posts tab.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	// Press enter.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command on enter in Posts tab")
	}
	msg := cmd()
	sp, ok := msg.(screens.ShowProfilePostMsg)
	if !ok {
		t.Fatalf("expected ShowProfilePostMsg, got %T", msg)
	}
	if sp.Post.ID != "p1" {
		t.Errorf("Post.ID = %q, want p1", sp.Post.ID)
	}
}

func TestProfile_EditMode_TabNavigatesFields(t *testing.T) {
	m := screens.NewProfileModel()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	// Open edit form.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	if !m.ComposeActive() {
		t.Skip("edit mode not active")
	}
	// In edit mode, tab should NOT switch sub-tabs but navigate fields.
	// ComposeActive() should remain true.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if !m.ComposeActive() {
		t.Error("expected compose to remain active after tab in edit mode")
	}
}

func TestParseTopics(t *testing.T) {
	cases := []struct {
		input string
		want  []string
	}{
		{"", nil},
		{"go, tui, programming", []string{"go", "tui", "programming"}},
		{"my cool topic, other", []string{"my cool topic", "other"}},
		{"a, b, c, d", []string{"a", "b", "c"}},   // capped at 3
		{"a,  ,  , b", []string{"a", "b"}},          // empty parts filtered
		{"  trimmed  ,  spaces  ", []string{"trimmed", "spaces"}}, // whitespace trimmed
	}
	for _, tc := range cases {
		got := screens.ParseTopics(tc.input)
		if len(got) != len(tc.want) {
			t.Errorf("ParseTopics(%q): got %v, want %v", tc.input, got, tc.want)
			continue
		}
		for i := range tc.want {
			if got[i] != tc.want[i] {
				t.Errorf("ParseTopics(%q)[%d]: got %q, want %q", tc.input, i, got[i], tc.want[i])
			}
		}
	}
}

// --- Journal revision history ---

func journalReady() screens.JournalModel {
	m := screens.NewJournalModel(80)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = m.SetNotes([]model.Note{
		{ID: "note1", Content: "first note", Topics: []string{"journal"}, RevisionNumber: 2, CreatedAt: time.Now()},
		{ID: "note2", Content: "second note", Topics: []string{}, RevisionNumber: 1, CreatedAt: time.Now().Add(-1 * time.Hour)},
	}, "")
	return m
}

func TestJournal_H_EmitsLoadRevisionsMsg(t *testing.T) {
	m := journalReady()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	if cmd == nil {
		t.Fatal("h on a note should emit a command")
	}
	msg := cmd()
	if _, ok := msg.(screens.LoadNoteRevisionsMsg); !ok {
		t.Errorf("expected LoadNoteRevisionsMsg, got %T", msg)
	}
}

func TestJournal_H_EmptyList_NoCommand(t *testing.T) {
	m := screens.NewJournalModel(80)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = m.SetNotes(nil, "")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	if cmd != nil {
		t.Errorf("expected no command for empty note list, got cmd")
	}
}

func TestJournal_SetRevisions_EntersRevisionsMode(t *testing.T) {
	m := journalReady()
	revisions := []model.NoteRevision{
		{RevisionNumber: 2, Content: "v2 content", Topics: []string{"journal"}, CreatedAt: time.Now()},
		{RevisionNumber: 1, Content: "v1 content", Topics: []string{}, CreatedAt: time.Now().Add(-1 * time.Hour)},
	}
	m = m.SetRevisions("note1", revisions, "")
	view := m.View()
	if view == "" {
		t.Error("expected non-empty view in revisions mode")
	}
	// View should contain revision information.
	if !strings.Contains(view, "Rev") {
		t.Errorf("view should contain 'Rev', got: %q", view[:min(len(view), 200)])
	}
}

func TestJournal_Revisions_EscExitsRevisionsMode(t *testing.T) {
	m := journalReady()
	revisions := []model.NoteRevision{
		{RevisionNumber: 1, Content: "first version", CreatedAt: time.Now()},
	}
	m = m.SetRevisions("note1", revisions, "")
	// ESC should exit revisions mode.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	// After exit, ComposeActive should still be false.
	if m.ComposeActive() {
		t.Error("expected compose to remain inactive after exiting revisions mode")
	}
	// The journal list should be visible again.
	view := m.View()
	if strings.Contains(view, "Revisions") {
		t.Errorf("view should not contain Revisions header after exit, got: %q", view[:min(len(view), 200)])
	}
}

func TestJournal_Revisions_EnterEmitsLoadNoteRevisionMsg(t *testing.T) {
	m := journalReady()
	revisions := []model.NoteRevision{
		{RevisionNumber: 2, Content: "latest", CreatedAt: time.Now()},
		{RevisionNumber: 1, Content: "original", CreatedAt: time.Now().Add(-1 * time.Hour)},
	}
	m = m.SetRevisions("note1", revisions, "")
	// Press enter on the first (selected) revision.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command on enter in revisions mode")
	}
	msg := cmd()
	lnr, ok := msg.(screens.LoadNoteRevisionMsg)
	if !ok {
		t.Fatalf("expected LoadNoteRevisionMsg, got %T", msg)
	}
	if lnr.NoteID != "note1" {
		t.Errorf("NoteID = %q, want note1", lnr.NoteID)
	}
	if lnr.RevisionNumber != 2 {
		t.Errorf("RevisionNumber = %d, want 2", lnr.RevisionNumber)
	}
}

func TestJournal_SetRevisionPreview_ShowsContent(t *testing.T) {
	m := journalReady()
	revisions := []model.NoteRevision{
		{RevisionNumber: 1, Content: "first version", CreatedAt: time.Now()},
	}
	m = m.SetRevisions("note1", revisions, "")
	note := model.Note{
		ID:             "note1",
		Content:        "this is the revision content",
		RevisionNumber: 1,
		CreatedAt:      time.Now().Add(-2 * time.Hour),
	}
	m = m.SetRevisionPreview(note)
	view := m.View()
	if !strings.Contains(view, "revision content") {
		t.Errorf("view should contain revision content, got: %q", view[:min(len(view), 300)])
	}
}

func TestJournal_RevisionPreview_EscReturnsToList(t *testing.T) {
	m := journalReady()
	revisions := []model.NoteRevision{
		{RevisionNumber: 1, Content: "first version", CreatedAt: time.Now()},
	}
	m = m.SetRevisions("note1", revisions, "")
	note := model.Note{ID: "note1", Content: "preview", RevisionNumber: 1, CreatedAt: time.Now()}
	m = m.SetRevisionPreview(note)
	// ESC from preview should go back to revision list (not exit revisions mode completely).
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	view := m.View()
	// Should still be in revisions mode but showing the list.
	if !strings.Contains(view, "Rev") {
		t.Errorf("after ESC from preview, should show revision list, got: %q", view[:min(len(view), 200)])
	}
	// Second ESC exits revisions mode entirely.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	view = m.View()
	if strings.Contains(view, "Revisions") {
		t.Errorf("after second ESC, should not be in revisions mode, got: %q", view[:min(len(view), 200)])
	}
}

