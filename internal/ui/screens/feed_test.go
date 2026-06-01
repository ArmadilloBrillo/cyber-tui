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
