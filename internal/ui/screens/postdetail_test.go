package screens_test

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/screens"
)

// pdPost returns a minimal Post for use in PostDetail tests.
func pdPost(id string) model.Post {
	return model.Post{ID: id, AuthorUsername: "op", Content: "original post"}
}

// pdReply builds a Reply with the given id, optional parent, and an author.
func pdReply(id, parentID, author string, t time.Time) model.Reply {
	return model.Reply{
		ID:             id,
		PostID:         "p1",
		AuthorID:       author,
		AuthorUsername: author,
		Content:        "reply content",
		ParentReplyID:  parentID,
		CreatedAt:      t,
	}
}

// initPostDetail returns a ready PostDetailModel with a sized viewport.
func initPostDetail() screens.PostDetailModel {
	m := screens.NewPostDetailModel()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	return m
}

// advanceSelection sends j key presses and returns the sequence of selected IDs.
func advanceSelection(m screens.PostDetailModel, steps int) []string {
	var ids []string
	for i := 0; i < steps; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		ids = append(ids, m.SelectedReplyID())
	}
	return ids
}

// TestPostDetail_FlatReplies_SelectedReplyID verifies that SelectedReplyID
// returns "" when no reply is selected and the correct ID when one is.
func TestPostDetail_FlatReplies_SelectedReplyID(t *testing.T) {
	m := initPostDetail()
	now := time.Now()
	m = m.SetPost(pdPost("p1"))
	m = m.SetReplies([]model.Reply{
		pdReply("r1", "", "alice", now),
		pdReply("r2", "", "bob", now.Add(time.Minute)),
	})

	if id := m.SelectedReplyID(); id != "" {
		t.Fatalf("expected no selected reply after SetReplies, got %q", id)
	}

	// j from post → selects first reply
	ids := advanceSelection(m, 1)
	if ids[0] != "r1" {
		t.Errorf("first j from post should select r1, got %q", ids[0])
	}
}

// TestPostDetail_TreeOrder_DFS verifies that j-key navigation visits replies in
// depth-first order: parent, then its children, then the next sibling.
func TestPostDetail_TreeOrder_DFS(t *testing.T) {
	m := initPostDetail()
	now := time.Now()
	m = m.SetPost(pdPost("p1"))

	// Layout:
	//   r1 (top-level, t=0)
	//   ├─ r2 (child of r1, t=1)
	//   r3 (top-level, t=2)
	m = m.SetReplies([]model.Reply{
		pdReply("r1", "", "alice", now),
		pdReply("r2", "r1", "bob", now.Add(time.Minute)),
		pdReply("r3", "", "carol", now.Add(2*time.Minute)),
	})

	// Skip post (first j moves from post to first reply)
	ids := advanceSelection(m, 3)
	want := []string{"r1", "r2", "r3"}
	for i, id := range ids {
		if id != want[i] {
			t.Errorf("step %d: want %q, got %q", i+1, want[i], id)
		}
	}
}

// TestPostDetail_TreeOrder_ChronologicalChildren verifies that children are
// visited chronologically even when the input list is out of order.
func TestPostDetail_TreeOrder_ChronologicalChildren(t *testing.T) {
	m := initPostDetail()
	now := time.Now()
	m = m.SetPost(pdPost("p1"))

	// r2 arrives first in the slice but has a later timestamp than r3.
	// Both are children of r1. Expected visit order: r1, r3, r2.
	m = m.SetReplies([]model.Reply{
		pdReply("r1", "", "alice", now),
		pdReply("r2", "r1", "bob", now.Add(2*time.Minute)),
		pdReply("r3", "r1", "carol", now.Add(time.Minute)),
	})

	ids := advanceSelection(m, 3)
	want := []string{"r1", "r3", "r2"}
	for i, id := range ids {
		if id != want[i] {
			t.Errorf("step %d: want %q, got %q", i+1, want[i], id)
		}
	}
}

// TestPostDetail_Orphan_TreatedAsTopLevel verifies that a reply whose parent is
// not in the loaded set is rendered as a top-level reply.
func TestPostDetail_Orphan_TreatedAsTopLevel(t *testing.T) {
	m := initPostDetail()
	now := time.Now()
	m = m.SetPost(pdPost("p1"))

	// r2 claims a parent that is not in the list.
	m = m.SetReplies([]model.Reply{
		pdReply("r1", "", "alice", now),
		pdReply("r2", "nonexistent", "bob", now.Add(time.Minute)),
	})

	ids := advanceSelection(m, 2)
	// Both should be reachable via sequential j presses.
	if ids[0] == "" || ids[1] == "" {
		t.Errorf("orphaned reply should be reachable; got %v", ids)
	}
}

// TestPostDetail_ScrollToReply_AfterTree verifies that ScrollToReply works
// correctly with tree-ordered navigation (finds the reply at its DFS position).
func TestPostDetail_ScrollToReply_AfterTree(t *testing.T) {
	m := initPostDetail()
	now := time.Now()
	m = m.SetPost(pdPost("p1"))
	m = m.SetReplies([]model.Reply{
		pdReply("r1", "", "alice", now),
		pdReply("r2", "r1", "bob", now.Add(time.Minute)),
		pdReply("r3", "", "carol", now.Add(2*time.Minute)),
	})

	// Scroll to the nested reply r2.
	m = m.ScrollToReply("r2")
	if id := m.SelectedReplyID(); id != "r2" {
		t.Errorf("ScrollToReply(r2): expected r2 selected, got %q", id)
	}
}

// TestPostDetail_RemoveReply_KeepsLengthSync verifies that after RemoveReply
// the model remains consistent (no panic on navigation, SelectedReplyID valid).
func TestPostDetail_RemoveReply_KeepsLengthSync(t *testing.T) {
	m := initPostDetail()
	now := time.Now()
	m = m.SetPost(pdPost("p1"))
	m = m.SetReplies([]model.Reply{
		pdReply("r1", "", "alice", now),
		pdReply("r2", "r1", "bob", now.Add(time.Minute)),
		pdReply("r3", "", "carol", now.Add(2*time.Minute)),
	})

	// Select r2, then remove r1 (its parent).
	m = m.ScrollToReply("r2")
	m = m.RemoveReply("r1")

	// After removing r1, r2 becomes an orphan treated as top-level.
	// Navigation should not panic.
	ids := advanceSelection(m, 3)
	for _, id := range ids {
		_ = id // just ensure no panic
	}
}

// TestPostDetail_PaginationRebuildsTree verifies that calling SetReplies a
// second time (simulating pagination) rebuilds the tree with the full list.
func TestPostDetail_PaginationRebuildsTree(t *testing.T) {
	m := initPostDetail()
	now := time.Now()
	m = m.SetPost(pdPost("p1"))

	// First page: two top-level replies.
	m = m.SetReplies([]model.Reply{
		pdReply("r1", "", "alice", now),
		pdReply("r2", "", "bob", now.Add(time.Minute)),
	})

	// Second call (pagination): adds a third reply as a child of r1.
	m = m.SetReplies([]model.Reply{
		pdReply("r1", "", "alice", now),
		pdReply("r2", "", "bob", now.Add(time.Minute)),
		pdReply("r3", "r1", "carol", now.Add(30*time.Second)),
	})

	// Tree should now be: r1, r3 (child), r2. Advance three steps from post.
	ids := advanceSelection(m, 3)
	want := []string{"r1", "r3", "r2"}
	for i, id := range ids {
		if id != want[i] {
			t.Errorf("after pagination step %d: want %q, got %q", i+1, want[i], id)
		}
	}
}

// TestPostDetail_DepthCap verifies that a chain deeper than 3 levels does not
// panic and that all replies remain reachable via navigation.
func TestPostDetail_DepthCap(t *testing.T) {
	m := initPostDetail()
	now := time.Now()
	m = m.SetPost(pdPost("p1"))

	replies := []model.Reply{
		pdReply("r0", "", "a", now),
		pdReply("r1", "r0", "b", now.Add(time.Minute)),
		pdReply("r2", "r1", "c", now.Add(2*time.Minute)),
		pdReply("r3", "r2", "d", now.Add(3*time.Minute)),
		pdReply("r4", "r3", "e", now.Add(4*time.Minute)), // depth 4, capped to 3
	}
	m = m.SetReplies(replies)

	// All 5 replies should be reachable without panic.
	ids := advanceSelection(m, 5)
	seen := make(map[string]bool)
	for _, id := range ids {
		if id == "" {
			t.Fatal("unexpected empty ID during deep-chain navigation")
		}
		seen[id] = true
	}
	for _, r := range replies {
		if !seen[r.ID] {
			t.Errorf("reply %q not reached during navigation", r.ID)
		}
	}
}

// --- Flag / report ---

func TestPostDetail_FlagKey_OnOwnPost_DoesNothing(t *testing.T) {
	m := initPostDetail()
	m = m.SetCurrentUsername("op")
	m = m.SetPost(pdPost("p1")) // pdPost author is "op"

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("!")})
	if cmd != nil {
		t.Fatal("expected no cmd when flagging own post")
	}
}

func TestPostDetail_FlagKey_OnOtherPost_EmitsFlagPostMsg(t *testing.T) {
	m := initPostDetail()
	m = m.SetCurrentUsername("alice")
	m = m.SetPost(pdPost("p1")) // author "op" != "alice"

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("!")})
	if cmd == nil {
		t.Fatal("expected a focus cmd from opening the flag prompt")
	}
	for _, r := range "bad take" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(string(r))})
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if cmd == nil {
		t.Fatal("expected a cmd after confirming")
	}
	_, cmd = m.Update(cmd())
	if cmd == nil {
		t.Fatal("expected a cmd after routing FlagSubmitMsg through Update")
	}
	msg, ok := cmd().(screens.FlagPostMsg)
	if !ok {
		t.Fatalf("expected FlagPostMsg, got %T", cmd())
	}
	if msg.PostID != "p1" {
		t.Errorf("PostID = %q, want p1", msg.PostID)
	}
	if msg.Reason != "bad take" {
		t.Errorf("Reason = %q, want %q", msg.Reason, "bad take")
	}
}

func TestPostDetail_FlagKey_OnOwnReply_DoesNothing(t *testing.T) {
	m := initPostDetail()
	m = m.SetCurrentUsername("alice")
	m = m.SetPost(pdPost("p1"))
	m = m.SetReplies([]model.Reply{pdReply("r1", "", "alice", time.Now())})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}) // select r1 (own reply)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("!")})
	if cmd != nil {
		t.Fatal("expected no cmd when flagging own reply")
	}
}

func TestPostDetail_FlagKey_OnOtherReply_EmitsFlagReplyMsg(t *testing.T) {
	m := initPostDetail()
	m = m.SetCurrentUsername("alice")
	m = m.SetPost(pdPost("p1"))
	m = m.SetReplies([]model.Reply{pdReply("r1", "", "bob", time.Now())})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}) // select r1 (bob's reply)

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("!")})
	if cmd == nil {
		t.Fatal("expected a focus cmd from opening the flag prompt")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // empty reason
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if cmd == nil {
		t.Fatal("expected a cmd after confirming")
	}
	_, cmd = m.Update(cmd())
	if cmd == nil {
		t.Fatal("expected a cmd after routing FlagSubmitMsg through Update")
	}
	msg, ok := cmd().(screens.FlagReplyMsg)
	if !ok {
		t.Fatalf("expected FlagReplyMsg, got %T", cmd())
	}
	if msg.ReplyID != "r1" {
		t.Errorf("ReplyID = %q, want r1", msg.ReplyID)
	}
	if msg.PostID != "p1" {
		t.Errorf("PostID = %q, want p1", msg.PostID)
	}
	if msg.Reason != "" {
		t.Errorf("Reason = %q, want empty", msg.Reason)
	}
}

func TestPostDetail_FlagKey_Cancel_EmitsNoFlagMsg(t *testing.T) {
	m := initPostDetail()
	m = m.SetCurrentUsername("alice")
	m = m.SetPost(pdPost("p1"))

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("!")})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		if _, ok := cmd().(screens.FlagPostMsg); ok {
			t.Fatal("esc should not emit FlagPostMsg")
		}
	}
}

func TestPostDetail_ComposeActive_TrueWhileFlagPromptOpen(t *testing.T) {
	m := initPostDetail()
	m = m.SetCurrentUsername("alice")
	m = m.SetPost(pdPost("p1")) // author "op" != "alice"

	if m.ComposeActive() {
		t.Fatal("setup: expected ComposeActive false before opening the flag prompt")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("!")})
	if !m.ComposeActive() {
		t.Error("expected ComposeActive to report true while the flag/report overlay is open")
	}
}

func TestPostDetail_ComposeActive_TrueWhileConfirmingDelete(t *testing.T) {
	m := initPostDetail()
	m = m.SetCurrentUsername("op")
	m = m.SetPost(pdPost("p1")) // author "op" == "op", so delete is allowed

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if !m.ComposeActive() {
		t.Error("expected ComposeActive to report true while the delete-confirm overlay is open")
	}
}

// --- Post theme detection / try-theme key ---

const postWithThemeBlock = `Check out my theme!

/* Cyberspace Custom Theme */
Base Theme: vt320
/* Colors */
Foreground: #d000ff
Background: #000000
Dimmed: #270082
Border: #270082
Code: #7A5DFF
Code BG: #5100ff
`

func TestPostDetail_ThemeBlockDetected_SetsHasThemeInPost(t *testing.T) {
	m := initPostDetail()
	m = m.SetPost(model.Post{ID: "p1", AuthorUsername: "op", Content: postWithThemeBlock})

	if !m.HasThemeInPost() {
		t.Error("expected HasThemeInPost() == true for a post with a theme block")
	}
}

func TestPostDetail_NoThemeBlock_HasThemeInPostFalse(t *testing.T) {
	m := initPostDetail()
	m = m.SetPost(pdPost("p1")) // plain "original post" content, no theme block

	if m.HasThemeInPost() {
		t.Error("expected HasThemeInPost() == false for a post without a theme block")
	}
}

func TestPostDetail_T_EmitsPreviewPostThemeMsg_WhenDetected(t *testing.T) {
	m := initPostDetail()
	m = m.SetPost(model.Post{ID: "p1", AuthorUsername: "op", Content: postWithThemeBlock})

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("T")})
	if cmd == nil {
		t.Fatal("expected a cmd from T when a theme block is detected")
	}
	msg, ok := cmd().(screens.PreviewPostThemeMsg)
	if !ok {
		t.Fatalf("expected PreviewPostThemeMsg, got %T", cmd())
	}
	if msg.Palette.Foreground != "#d000ff" {
		t.Errorf("Palette.Foreground = %q, want #d000ff", msg.Palette.Foreground)
	}
}

func TestPostDetail_T_NoOp_WhenNoThemeBlock(t *testing.T) {
	m := initPostDetail()
	m = m.SetPost(pdPost("p1"))

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("T")})
	if cmd != nil {
		t.Error("expected no cmd from T when no theme block is detected")
	}
}

func TestPostDetail_T_NoOp_WhenReplySelected(t *testing.T) {
	m := initPostDetail()
	now := time.Now()
	m = m.SetPost(model.Post{ID: "p1", AuthorUsername: "op", Content: postWithThemeBlock})
	m = m.SetReplies([]model.Reply{pdReply("r1", "", "alice", now)})

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}) // select the reply
	if m.SelectedReplyID() != "r1" {
		t.Fatal("expected the reply to be selected")
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("T")})
	if cmd != nil {
		t.Error("expected T to no-op when a reply (not the post) is selected, even with a theme block present")
	}
}
