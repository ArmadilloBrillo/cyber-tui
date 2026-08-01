package screens_test

import (
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
	m, cmd = m.Update(cmd())
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
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
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
