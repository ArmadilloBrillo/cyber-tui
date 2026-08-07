package screens_test

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/screens"
)

func nsfwFilterMsg(on bool) screens.SharedConfigMsg {
	return screens.SharedConfigMsg{
		Settings: model.Settings{FilterNSFW: on},
	}
}

// --- FilterNSFW ---

func TestFeed_FilterNSFW_HidesNSFWPost(t *testing.T) {
	m := screens.NewFeedModel()
	m = m.SetPosts([]model.Post{
		{ID: "p1", AuthorUsername: "alice", Content: "safe post"},
		{ID: "p2", AuthorUsername: "bob", Content: "nsfw post", IsNSFW: true},
		{ID: "p3", AuthorUsername: "carol", Content: "another safe post"},
	}, "")
	m, _ = m.Update(nsfwFilterMsg(true))

	// Down from index 0 should stop at 1 (only 2 visible posts)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})

	// Press Enter — should get p3 (second visible post), not p2
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a cmd on enter")
	}
	msg := cmd()
	sp, ok := msg.(screens.ShowPostMsg)
	if !ok {
		t.Fatalf("expected ShowPostMsg, got %T", msg)
	}
	if sp.Post.ID != "p3" {
		t.Errorf("expected p3 (safe), got %s", sp.Post.ID)
	}
}

// With FilterNSFW on, selectedIndex is a visible-list index. SetPosts restores the
// selection by ID; the restored index must stay in visible-list space so the scroll
// math (which indexes the visible-length offsets slice) does not panic, and so the
// URL opener reads the highlighted post rather than a filtered-out one.
func TestFeed_FilterNSFW_RefreshKeepsVisibleSelection(t *testing.T) {
	m := screens.NewFeedModel()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24}) // mark ready so scroll math runs
	m, _ = m.Update(nsfwFilterMsg(true))

	posts := []model.Post{
		{ID: "a", AuthorUsername: "alice", Content: "first safe https://example.com/a"},
		{ID: "x", AuthorUsername: "bob", Content: "nsfw https://example.com/x", IsNSFW: true},
		{ID: "b", AuthorUsername: "carol", Content: "second safe https://example.com/b"},
	}
	m = m.SetPosts(posts, "")

	// Move to the second visible post (b); the NSFW post x is filtered out.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})

	// Re-set the same posts (feed refresh / tab switch). Pre-fix this restored the
	// selection from the unfiltered slice (index 2) and panicked in the scroll math.
	m = m.SetPosts(posts, "")

	urls := m.GetFocusedURLs()
	if len(urls) != 1 || urls[0] != "https://example.com/b" {
		t.Fatalf("GetFocusedURLs = %v, want [https://example.com/b]", urls)
	}
}

func TestFeed_FilterNSFW_Off_ShowsAll(t *testing.T) {
	m := screens.NewFeedModel()
	m = m.SetPosts([]model.Post{
		{ID: "p1", AuthorUsername: "alice", Content: "safe post"},
		{ID: "p2", AuthorUsername: "bob", Content: "nsfw post", IsNSFW: true},
	}, "")
	m, _ = m.Update(nsfwFilterMsg(false))

	// Can navigate to the NSFW post
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a cmd on enter")
	}
	msg := cmd()
	sp, ok := msg.(screens.ShowPostMsg)
	if !ok {
		t.Fatalf("expected ShowPostMsg, got %T", msg)
	}
	if sp.Post.ID != "p2" {
		t.Errorf("expected p2 (nsfw), got %s", sp.Post.ID)
	}
}

// --- Flag / report ---

func keyRune(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func TestFeed_FlagKey_OnOwnPost_DoesNothing(t *testing.T) {
	m := screens.NewFeedModel()
	m = m.SetPosts([]model.Post{
		{ID: "p1", AuthorUsername: "alice", Content: "mine"},
	}, "")
	m = m.SetCurrentUsername("alice")

	_, cmd := m.Update(keyRune("!"))
	if cmd != nil {
		t.Fatal("expected no cmd when flagging own post")
	}
}

func TestFeed_FlagKey_OnOtherPost_FullFlowEmitsFlagPostMsg(t *testing.T) {
	m := screens.NewFeedModel()
	m = m.SetPosts([]model.Post{
		{ID: "p1", AuthorUsername: "bob", Content: "not mine"},
	}, "")
	m = m.SetCurrentUsername("alice")

	m, cmd := m.Update(keyRune("!"))
	if cmd == nil {
		t.Fatal("expected Open() to return a focus cmd")
	}

	for _, r := range "spam" {
		m, _ = m.Update(keyRune(string(r)))
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m, cmd = m.Update(keyRune("y"))
	if cmd == nil {
		t.Fatal("expected a cmd after confirming")
	}
	// The real runtime feeds the cmd's message back through Update; do the same here.
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
	if msg.Reason != "spam" {
		t.Errorf("Reason = %q, want spam", msg.Reason)
	}
}

func TestFeed_FlagKey_Cancel_EmitsNoFlagMsg(t *testing.T) {
	m := screens.NewFeedModel()
	m = m.SetPosts([]model.Post{
		{ID: "p1", AuthorUsername: "bob", Content: "not mine"},
	}, "")
	m = m.SetCurrentUsername("alice")

	m, _ = m.Update(keyRune("!"))
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		if _, ok := cmd().(screens.FlagPostMsg); ok {
			t.Fatal("esc should not emit FlagPostMsg")
		}
	}
}

func TestFeed_ComposeActive_TrueWhileFlagPromptOpen(t *testing.T) {
	m := screens.NewFeedModel()
	m = m.SetPosts([]model.Post{
		{ID: "p1", AuthorUsername: "bob", Content: "not mine"},
	}, "")
	m = m.SetCurrentUsername("alice")

	if m.ComposeActive() {
		t.Fatal("setup: expected ComposeActive false before opening the flag prompt")
	}
	m, _ = m.Update(keyRune("!"))
	if !m.ComposeActive() {
		t.Error("expected ComposeActive to report true while the flag/report overlay is open")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.ComposeActive() {
		t.Error("expected ComposeActive to go false again once the flag prompt is cancelled")
	}
}

func TestFeed_ComposeActive_TrueWhileConfirmingDelete(t *testing.T) {
	m := screens.NewFeedModel()
	m = m.SetPosts([]model.Post{
		{ID: "p1", AuthorUsername: "alice", Content: "mine"},
	}, "")
	m = m.SetCurrentUsername("alice")

	m, _ = m.Update(keyRune("d"))
	if !m.ComposeActive() {
		t.Error("expected ComposeActive to report true while the delete-confirm overlay is open")
	}
}

// --- Background poll: pending new entries ---

func TestFeed_SetPendingNew_FiltersAlreadyPresentPosts(t *testing.T) {
	m := screens.NewFeedModel()
	m = m.SetPosts([]model.Post{
		{ID: "p1", AuthorUsername: "alice", Content: "existing"},
	}, "")

	m = m.SetPendingNew([]model.Post{
		{ID: "p1", AuthorUsername: "alice", Content: "existing"}, // already present, filtered
		{ID: "p2", AuthorUsername: "bob", Content: "new"},
		{ID: "p3", AuthorUsername: "carol", Content: "also new"},
	})

	if got := m.PendingNewCount(); got != 2 {
		t.Fatalf("PendingNewCount() = %d, want 2", got)
	}
}

func TestFeed_MergePendingNew_PrependsAndClears(t *testing.T) {
	m := screens.NewFeedModel()
	m = m.SetPosts([]model.Post{
		{ID: "p1", AuthorUsername: "alice", Content: "existing"},
	}, "")
	m = m.SetPendingNew([]model.Post{
		{ID: "p2", AuthorUsername: "bob", Content: "new"},
	})

	m = m.MergePendingNew()

	if got := m.PendingNewCount(); got != 0 {
		t.Fatalf("PendingNewCount() after merge = %d, want 0", got)
	}

	// Enter on the (now first, selected) post should be the merged post.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a cmd on enter")
	}
	sp, ok := cmd().(screens.ShowPostMsg)
	if !ok {
		t.Fatalf("expected ShowPostMsg, got %T", cmd())
	}
	if sp.Post.ID != "p2" {
		t.Errorf("expected merged post p2 to be selected, got %s", sp.Post.ID)
	}
}

func TestFeed_MergePendingNew_NoOpWhenNothingPending(t *testing.T) {
	m := screens.NewFeedModel()
	m = m.SetPosts([]model.Post{
		{ID: "p1", AuthorUsername: "alice", Content: "existing"},
	}, "")

	m2 := m.MergePendingNew()
	if m2.PendingNewCount() != 0 {
		t.Fatalf("PendingNewCount() = %d, want 0", m2.PendingNewCount())
	}
}

// If a peek page comes back entirely new (no overlap with known posts), the
// real new-post count could exceed what one 20-post page can show — the
// banner should read "20+" rather than assert a specific, likely-wrong number.
func TestFeed_SetPendingNew_FullPageAllNew_ShowsCappedLabel(t *testing.T) {
	m := screens.NewFeedModel()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24}) // mark ready so View() renders content
	m = m.SetPosts([]model.Post{
		{ID: "known", AuthorUsername: "alice", Content: "existing"},
	}, "")

	full := make([]model.Post, 20)
	for i := range full {
		full[i] = model.Post{ID: "new" + strconv.Itoa(i), AuthorUsername: "bob", Content: "new"}
	}
	m = m.SetPendingNew(full)

	if got := m.PendingNewCount(); got != 20 {
		t.Fatalf("PendingNewCount() = %d, want 20", got)
	}
	if !strings.Contains(m.PendingNewLabel(), "20+") {
		t.Errorf("PendingNewLabel() = %q, want it to contain \"20+\"", m.PendingNewLabel())
	}
}

// A partial-overlap peek page (some posts already known) is not capped — the
// count is exact and the banner should show the plain number, not "N+".
func TestFeed_SetPendingNew_PartialOverlap_ShowsExactLabel(t *testing.T) {
	m := screens.NewFeedModel()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = m.SetPosts([]model.Post{
		{ID: "known", AuthorUsername: "alice", Content: "existing"},
	}, "")

	m = m.SetPendingNew([]model.Post{
		{ID: "n1", AuthorUsername: "bob", Content: "new"},
		{ID: "n2", AuthorUsername: "carol", Content: "new"},
		{ID: "known", AuthorUsername: "alice", Content: "existing"}, // overlap found within the page
	})

	if got := m.PendingNewCount(); got != 2 {
		t.Fatalf("PendingNewCount() = %d, want 2", got)
	}
	label := m.PendingNewLabel()
	if !strings.Contains(label, "load 2 new entries") {
		t.Errorf("PendingNewLabel() = %q, want it to contain \"load 2 new entries\"", label)
	}
	if strings.Contains(label, "2+") {
		t.Errorf("PendingNewLabel() = %q, should not show a capped label when count is exact", label)
	}
}

// Pressing up at the top of the feed with entries pending should play the
// same brief "fetching new posts..." transition as a real refresh (via a
// short tea.Tick) before merging locally, rather than firing a
// RefreshFeedMsg network round-trip or merging instantly with no feedback.
func TestFeed_UpAtTop_WithPendingNew_AnimatesThenMergesLocally(t *testing.T) {
	m := screens.NewFeedModel()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = m.SetPosts([]model.Post{
		{ID: "p1", AuthorUsername: "alice", Content: "existing"},
	}, "")
	m = m.SetPendingNew([]model.Post{
		{ID: "p2", AuthorUsername: "bob", Content: "new"},
	})

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyUp})

	// Not merged yet — the tick hasn't fired.
	if m.PendingNewCount() != 1 {
		t.Fatalf("PendingNewCount() = %d, want 1 (not yet merged)", m.PendingNewCount())
	}
	if !strings.Contains(m.View(), "fetching new posts...") {
		t.Errorf("View() = %q, want the transitional banner while the merge tick is pending", m.View())
	}
	if cmd == nil {
		t.Fatal("expected a cmd (the merge-delay tea.Tick)")
	}
	tickMsg := cmd() // tea.Tick's cmd reads its timer channel once — call it exactly once
	if _, ok := tickMsg.(screens.RefreshFeedMsg); ok {
		t.Fatal("expected the local merge path, not a RefreshFeedMsg network round-trip")
	}

	// Feed the tick's message back through Update to complete the merge.
	m, _ = m.Update(tickMsg)

	if m.PendingNewCount() != 0 {
		t.Errorf("PendingNewCount() = %d, want 0 after the merge tick fires", m.PendingNewCount())
	}
}

// --- ParseTopics ---

func TestParseTopics_LowercasesTrimsCapsAndIgnoresEmpty(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"MUSIC", []string{"music"}},
		{"Music, Linux", []string{"music", "linux"}},
		{"a,,b,", []string{"a", "b"}},
		{"a,b,c,d", []string{"a", "b", "c"}}, // capped at 3
	}
	for _, c := range cases {
		if got := screens.ParseTopics(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("ParseTopics(%q) = %#v, want %#v", c.in, got, c.want)
		}
	}
}
