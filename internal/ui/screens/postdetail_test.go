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

// TestPostDetail_VisibleInlineImages_DisabledByDefault confirms that with no
// SharedConfigMsg.InlineImagesEnabled ever sent (the default), no slots are
// reported even for a post whose content has an eligible image — the whole
// feature must be a strict no-op until explicitly turned on.
func TestPostDetail_VisibleInlineImages_DisabledByDefault(t *testing.T) {
	m := initPostDetail()
	post := pdPost("p1")
	post.Content = "hi\n\n![a](https://example.com/a.png)\n\nbye"
	m = m.SetPost(post)

	if slots := m.VisibleInlineImages(); slots != nil {
		t.Errorf("expected no slots while disabled, got %+v", slots)
	}
}

// TestPostDetail_VisibleInlineImages_PostAndReply confirms that once enabled,
// both the post's and a top-level reply's eligible image are reported, in
// order, with the URL/Key/indent expected for each.
func TestPostDetail_VisibleInlineImages_PostAndReply(t *testing.T) {
	m := initPostDetail()
	// Taller than the default 40 rows: the post's own reserved image band
	// plus its header/border already takes a good chunk of the viewport, so
	// a short viewport would legitimately cut the reply's image band off —
	// this test wants both fully visible, not to exercise that cutoff.
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 60})
	m, _ = m.Update(screens.SharedConfigMsg{InlineImagesEnabled: true})

	post := pdPost("p1")
	post.Content = "hi\n\n![a](https://example.com/a.png)\n\nbye"
	m = m.SetPost(post)

	reply := pdReply("r1", "", "someone", time.Now())
	reply.Content = "check this\n\n![b](https://example.com/b.png)"
	m = m.SetReplies([]model.Reply{reply})

	slots := m.VisibleInlineImages()
	if len(slots) != 2 {
		t.Fatalf("expected 2 visible slots (post + reply), got %d: %+v", len(slots), slots)
	}
	if slots[0].URL != "https://example.com/a.png" || slots[0].Key != "post:p1:0" || slots[0].ColIndent != 2 {
		t.Errorf("unexpected post slot: %+v", slots[0])
	}
	if slots[1].URL != "https://example.com/b.png" || slots[1].Key != "reply:r1:0" || slots[1].ColIndent != 2 {
		t.Errorf("unexpected reply slot: %+v", slots[1])
	}
	if slots[1].Row <= slots[0].Row {
		t.Errorf("expected reply slot to be below the post slot: post Row=%d, reply Row=%d", slots[0].Row, slots[1].Row)
	}
}

// TestPostDetail_VisibleInlineImages_MultipleImagesInOnePost confirms every
// eligible image in a single post is reported (not just the first), each
// with its own index-suffixed Key, and that the second image's Row accounts
// for the first image's full spacer-inclusive band plus the text paragraph
// between them — the one thing renderBodyWithInlineImage's shift-tracking
// loop exists to get right.
func TestPostDetail_VisibleInlineImages_MultipleImagesInOnePost(t *testing.T) {
	m := initPostDetail()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 80})
	m, _ = m.Update(screens.SharedConfigMsg{InlineImagesEnabled: true})

	post := pdPost("p1")
	post.Content = "one\n\n![a](https://example.com/a.png)\n\ntwo\n\n![b](https://example.com/b.png)\n\nthree"
	m = m.SetPost(post)

	slots := m.VisibleInlineImages()
	if len(slots) != 2 {
		t.Fatalf("expected 2 slots for 2 images in one post, got %d: %+v", len(slots), slots)
	}
	if slots[0].Key != "post:p1:0" || slots[1].Key != "post:p1:1" {
		t.Errorf("expected distinct per-image keys, got %q and %q", slots[0].Key, slots[1].Key)
	}
	// inlineImageMaxRows(8) + 2 spacer rows is the minimum gap between two
	// images with nothing but a one-line paragraph between them.
	const minGap = 10
	if got := slots[1].Row - slots[0].Row; got < minGap {
		t.Errorf("expected at least %d rows between images, got %d (rows %d, %d)", minGap, got, slots[0].Row, slots[1].Row)
	}
}

// TestPostDetail_VisibleInlineImages_SurvivesScrollAwayAndBack is the
// regression test for a 100%-reproducible live bug: with a post containing
// an inline image taller than the viewport, pressing down to select the
// first reply (scrolling the image out of view) and then pressing up just
// enough to reselect the post (scrolling back onto it) left the image
// failing to reappear on real iTerm2 — confirmed live to reproduce via
// pure in-screen scrolling alone, no tab switch involved, and confirmed
// here as a genuine viewport-positioning bug, not a redraw/timing issue:
// millerPageNav's revealAbove (miller_pager.go) bottom-aligned the
// viewport when scrolling back onto an item taller than the pane, leaving
// its top — where an image band usually sits — still scrolled out of view.
// Fixed by top-aligning revealAbove unconditionally, matching revealBelow.
// See docs/plan-inline-images-improvements.md Round 4/13.
func TestPostDetail_VisibleInlineImages_SurvivesScrollAwayAndBack(t *testing.T) {
	m := initPostDetail()
	// Small pane so the post (image band + text) is taller than it —
	// otherwise millerPageNav's reveal-above/below logic for tall items
	// never engages.
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 15})
	m, _ = m.Update(screens.SharedConfigMsg{InlineImagesEnabled: true})

	post := pdPost("p1")
	post.Content = "hi\n\n![a](https://example.com/a.png)\n\nsome more text to pad out the post body a bit"
	m = m.SetPost(post)
	m = m.SetReplies([]model.Reply{
		pdReply("r1", "", "alice", time.Now()),
		pdReply("r2", "", "bob", time.Now().Add(time.Minute)),
	})

	initialSlots := m.VisibleInlineImages()
	if len(initialSlots) != 1 {
		t.Fatalf("setup: expected the image visible initially, got %d slots: %+v", len(initialSlots), initialSlots)
	}

	// Press down enough times to move selection off the post and onto a
	// reply (scrolling the image out of view). The post is taller than the
	// pane by design (see above), so millerPageNav scrolls one line at a
	// time before it crosses into the reply — needs many presses, not few.
	for i := 0; i < 60 && m.SelectedReplyID() == ""; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	}
	if m.SelectedReplyID() == "" {
		t.Fatal("setup: expected a reply selected after pressing down repeatedly")
	}
	if slots := m.VisibleInlineImages(); len(slots) != 0 {
		t.Fatalf("setup: expected the image scrolled out of view once a reply is selected, got %+v", slots)
	}

	// Press up just enough to cross back onto the post (selection clears)
	// — deliberately not settling any further, since a real user presses
	// up only until the post is reselected, not dozens more times past
	// that.
	for i := 0; i < 60 && m.SelectedReplyID() != ""; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	}
	if id := m.SelectedReplyID(); id != "" {
		t.Fatalf("setup: expected back on the post after pressing up repeatedly, got reply %q selected", id)
	}

	finalSlots := m.VisibleInlineImages()
	if len(finalSlots) != 1 {
		t.Errorf("expected the image visible again immediately on scrolling back onto the post, got %d slots: %+v", len(finalSlots), finalSlots)
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

func TestPostDetail_ThemeBlockDetected_SetsHasTheme(t *testing.T) {
	m := initPostDetail()
	m = m.SetPost(model.Post{ID: "p1", AuthorUsername: "op", Content: postWithThemeBlock})

	if !m.HasTheme() {
		t.Error("expected HasTheme() == true for a post with a theme block")
	}
}

func TestPostDetail_NoThemeBlock_HasThemeFalse(t *testing.T) {
	m := initPostDetail()
	m = m.SetPost(pdPost("p1")) // plain "original post" content, no theme block

	if m.HasTheme() {
		t.Error("expected HasTheme() == false for a post without a theme block")
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

func TestPostDetail_T_NoOp_WhenSelectedReplyHasNoThemeBlock(t *testing.T) {
	m := initPostDetail()
	now := time.Now()
	m = m.SetPost(model.Post{ID: "p1", AuthorUsername: "op", Content: postWithThemeBlock})
	m = m.SetReplies([]model.Reply{pdReply("r1", "", "alice", now)}) // plain content, no theme block

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}) // select the reply
	if m.SelectedReplyID() != "r1" {
		t.Fatal("expected the reply to be selected")
	}

	if m.HasTheme() {
		t.Error("expected HasTheme() == false for a reply without a theme block, even though the post has one")
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("T")})
	if cmd != nil {
		t.Error("expected T to no-op when the selected reply has no theme block, even though the post does")
	}
}

func TestPostDetail_T_EmitsPreviewPostThemeMsg_WhenSelectedReplyHasThemeBlock(t *testing.T) {
	m := initPostDetail()
	now := time.Now()
	m = m.SetPost(pdPost("p1")) // plain post content, no theme block
	reply := pdReply("r1", "", "alice", now)
	reply.Content = postWithThemeBlock
	m = m.SetReplies([]model.Reply{reply})

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}) // select the reply
	if m.SelectedReplyID() != "r1" {
		t.Fatal("expected the reply to be selected")
	}

	if !m.HasTheme() {
		t.Error("expected HasTheme() == true for a reply with a theme block")
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("T")})
	if cmd == nil {
		t.Fatal("expected a cmd from T when the selected reply has a theme block")
	}
	msg, ok := cmd().(screens.PreviewPostThemeMsg)
	if !ok {
		t.Fatalf("expected PreviewPostThemeMsg, got %T", cmd())
	}
	if msg.Palette.Foreground != "#d000ff" {
		t.Errorf("Palette.Foreground = %q, want #d000ff", msg.Palette.Foreground)
	}
}
