package screens_test

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/screens"
)

func sampleTopics() []model.Topic {
	return []model.Topic{
		{Slug: "tech"},
		{Slug: "art"},
	}
}

func sampleTopicPosts() []model.Post {
	return []model.Post{
		{ID: "tp1", AuthorUsername: "alice", Content: "safe topic post"},
		{ID: "tp2", AuthorUsername: "bob", Content: "nsfw topic post", IsNSFW: true},
		{ID: "tp3", AuthorUsername: "carol", Content: "another safe topic post"},
	}
}

// --- IsBrowsingTopic / esc ---

func TestTopics_Esc_ClearsIsBrowsingTopic(t *testing.T) {
	m := screens.NewTopicsModel()
	m = m.SetTopics(sampleTopics(), "")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = m.SetTopicPosts(sampleTopicPosts(), "") // Enter only fires the load cmd; this is what flips m.view to viewTopicPosts
	if !m.IsBrowsingTopic() {
		t.Fatal("setup: expected IsBrowsingTopic() to be true after entering a topic")
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.IsBrowsingTopic() {
		t.Error("expected IsBrowsingTopic() to be false after esc back to the topic list")
	}
}

// --- FilterNSFW ---

func TestTopics_FilterNSFW_HidesNSFWPost(t *testing.T) {
	m := screens.NewTopicsModel()
	m = m.SetTopics(sampleTopics(), "")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = m.SetTopicPosts(sampleTopicPosts(), "")
	m, _ = m.Update(nsfwFilterMsg(true))

	// Navigate to end of visible list (2 visible posts, max index 1)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})

	// Enter should return tp3 (second visible), not tp2
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a cmd on enter")
	}
	msg := cmd()
	sp, ok := msg.(screens.ShowTopicPostMsg)
	if !ok {
		t.Fatalf("expected ShowTopicPostMsg, got %T", msg)
	}
	if sp.Post.ID != "tp3" {
		t.Errorf("expected tp3 (safe), got %s", sp.Post.ID)
	}
}

func TestTopics_FilterNSFW_Off_ShowsAll(t *testing.T) {
	m := screens.NewTopicsModel()
	m = m.SetTopics(sampleTopics(), "")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = m.SetTopicPosts(sampleTopicPosts(), "")
	m, _ = m.Update(nsfwFilterMsg(false))

	// Navigate to the NSFW post (index 1)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a cmd on enter")
	}
	msg := cmd()
	sp, ok := msg.(screens.ShowTopicPostMsg)
	if !ok {
		t.Fatalf("expected ShowTopicPostMsg, got %T", msg)
	}
	if sp.Post.ID != "tp2" {
		t.Errorf("expected tp2 (nsfw), got %s", sp.Post.ID)
	}
}
