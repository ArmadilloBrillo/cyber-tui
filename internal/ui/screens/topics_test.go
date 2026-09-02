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

// --- OpenTopic ---

func TestTopics_OpenTopic_SetsActiveTopicName(t *testing.T) {
	m := screens.NewTopicsModel()
	m = m.OpenTopic("diy")
	if got := m.ActiveTopicName(); got != "diy" {
		t.Errorf("ActiveTopicName() = %q, want %q", got, "diy")
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

// --- Blocked topics ---

func TestTopics_BlockedTopics_HidesMatchingPost(t *testing.T) {
	m := screens.NewTopicsModel()
	m = m.SetTopics(sampleTopics(), "")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = m.SetTopicPosts([]model.Post{
		{ID: "tp1", AuthorUsername: "alice", Content: "keep", Topics: []string{"art"}},
		{ID: "tp2", AuthorUsername: "bob", Content: "drop", Topics: []string{"crypto"}},
		{ID: "tp3", AuthorUsername: "carol", Content: "keep too"},
	}, "")
	m, _ = m.Update(blockedTopicsMsg("crypto"))

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a cmd on enter")
	}
	sp, ok := cmd().(screens.ShowTopicPostMsg)
	if !ok {
		t.Fatalf("expected ShowTopicPostMsg, got %T", cmd())
	}
	if sp.Post.ID != "tp3" {
		t.Errorf("expected tp3 (crypto post filtered), got %s", sp.Post.ID)
	}
}

// Pressing 'b' on a topic row toggles it in/out of the blocked list carried by
// the emitted SetBlockedTopicsMsg.
func TestTopics_BlockKey_TogglesBlockedList(t *testing.T) {
	m := screens.NewTopicsModel()
	m = m.SetTopics(sampleTopics(), "") // topicIndex 0 == "tech"

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	if cmd == nil {
		t.Fatal("expected a SetBlockedTopicsMsg cmd on first 'b'")
	}
	msg, ok := cmd().(screens.SetBlockedTopicsMsg)
	if !ok {
		t.Fatalf("expected SetBlockedTopicsMsg, got %T", cmd())
	}
	if len(msg.Topics) != 1 || msg.Topics[0] != "tech" {
		t.Fatalf("first 'b' Topics = %v, want [tech]", msg.Topics)
	}

	// Feed that new list back in (as App would via broadcastConfig) and press 'b'
	// again — "tech" should now be removed.
	m, _ = m.Update(blockedTopicsMsg(msg.Topics...))
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	if cmd == nil {
		t.Fatal("expected a SetBlockedTopicsMsg cmd on second 'b'")
	}
	msg2, ok := cmd().(screens.SetBlockedTopicsMsg)
	if !ok {
		t.Fatalf("expected SetBlockedTopicsMsg, got %T", cmd())
	}
	if len(msg2.Topics) != 0 {
		t.Errorf("second 'b' Topics = %v, want []", msg2.Topics)
	}
}

// --- VisibleInlineImages ---

func TestTopics_VisibleInlineImages_DisabledByDefault(t *testing.T) {
	m := screens.NewTopicsModel()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	m = m.SetTopics(sampleTopics(), "")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = m.SetTopicPosts([]model.Post{
		{ID: "tp1", AuthorUsername: "alice", Content: "hi\n\n![a](https://example.com/a.png)\n\nbye"},
	}, "")

	if slots := m.VisibleInlineImages(); slots != nil {
		t.Errorf("expected no slots while disabled, got %+v", slots)
	}
}

func TestTopics_VisibleInlineImages_ReportsSlotWhenEnabled(t *testing.T) {
	m := screens.NewTopicsModel()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	m, _ = m.Update(screens.SharedConfigMsg{InlineImagesEnabled: true})
	m = m.SetTopics(sampleTopics(), "")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = m.SetTopicPosts([]model.Post{
		{ID: "tp1", AuthorUsername: "alice", Content: "hi\n\n![a](https://example.com/a.png)\n\nbye"},
	}, "")

	slots := m.VisibleInlineImages()
	if len(slots) != 1 {
		t.Fatalf("expected 1 slot, got %d: %+v", len(slots), slots)
	}
	if slots[0].URL != "https://example.com/a.png" {
		t.Errorf("URL = %q, want https://example.com/a.png", slots[0].URL)
	}
}
