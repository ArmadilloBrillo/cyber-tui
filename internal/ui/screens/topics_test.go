package screens_test

import (
	"strings"
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

// --- Muted topics ---

func TestTopics_MutedTopics_HidesMatchingPost(t *testing.T) {
	m := screens.NewTopicsModel()
	m = m.SetTopics(sampleTopics(), "")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = m.SetTopicPosts([]model.Post{
		{ID: "tp1", AuthorUsername: "alice", Content: "keep", Topics: []string{"art"}},
		{ID: "tp2", AuthorUsername: "bob", Content: "drop", Topics: []string{"crypto"}},
		{ID: "tp3", AuthorUsername: "carol", Content: "keep too"},
	}, "")
	m, _ = m.Update(mutedTopicsMsg("crypto"))

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

// Pressing 'm' on a topic row toggles it in/out of the muted list carried by
// the emitted SetMutedTopicsMsg.
func TestTopics_MuteKey_TogglesMutedList(t *testing.T) {
	m := screens.NewTopicsModel()
	m = m.SetTopics(sampleTopics(), "") // topicIndex 0 == "tech"

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	if cmd == nil {
		t.Fatal("expected a SetMutedTopicsMsg cmd on first 'm'")
	}
	msg, ok := cmd().(screens.SetMutedTopicsMsg)
	if !ok {
		t.Fatalf("expected SetMutedTopicsMsg, got %T", cmd())
	}
	if len(msg.Topics) != 1 || msg.Topics[0] != "tech" {
		t.Fatalf("first 'm' Topics = %v, want [tech]", msg.Topics)
	}

	// Feed that new list back in (as App would via broadcastConfig) and press 'm'
	// again — "tech" should now be removed.
	m, _ = m.Update(mutedTopicsMsg(msg.Topics...))
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	if cmd == nil {
		t.Fatal("expected a SetMutedTopicsMsg cmd on second 'm'")
	}
	msg2, ok := cmd().(screens.SetMutedTopicsMsg)
	if !ok {
		t.Fatalf("expected SetMutedTopicsMsg, got %T", cmd())
	}
	if len(msg2.Topics) != 0 {
		t.Errorf("second 'm' Topics = %v, want []", msg2.Topics)
	}
}

// --- Topic-list filter ('f') ---

// openedSlug presses enter on the topic list and returns the slug of the
// LoadTopicPostsMsg it emits, or "" if enter produced no cmd.
func openedSlug(t *testing.T, m screens.TopicsModel) string {
	t.Helper()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		return ""
	}
	msg, ok := cmd().(screens.LoadTopicPostsMsg)
	if !ok {
		t.Fatalf("expected LoadTopicPostsMsg, got %T", cmd())
	}
	return msg.Slug
}

// 'f' once → hide muted: the muted row is dropped, selection lands on the first
// still-visible topic.
func TestTopics_Filter_HideMuted_DropsMutedRows(t *testing.T) {
	m := screens.NewTopicsModel()
	m = m.SetTopics(sampleTopics(), "") // [tech, art]
	m, _ = m.Update(mutedTopicsMsg("tech"))

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")}) // all -> hide muted
	if got := openedSlug(t, m); got != "art" {
		t.Errorf("hide-muted: enter opened %q, want \"art\" (tech is muted)", got)
	}
}

// 'f' twice → only muted: every slug in the muted set is shown, including one
// whose page was never fetched (proves it's sourced from Settings.MutedTopics,
// not the loaded topic pages).
func TestTopics_Filter_OnlyMuted_ShowsEveryMutedSlug(t *testing.T) {
	m := screens.NewTopicsModel()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	m = m.SetTopics(sampleTopics(), "") // [tech, art]
	m, _ = m.Update(mutedTopicsMsg("tech", "ghost"))

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")}) // -> only muted

	view := m.View()
	if !strings.Contains(view, "tech") || !strings.Contains(view, "ghost") {
		t.Errorf("only-muted view missing a muted slug:\n%s", view)
	}
	if strings.Contains(view, "art") {
		t.Errorf("only-muted view should not list the unmuted \"art\":\n%s", view)
	}
}

// only-muted rows are sorted by slug, not left in map-iteration order.
func TestTopics_Filter_OnlyMuted_SortedOrder(t *testing.T) {
	m := screens.NewTopicsModel()
	m = m.SetTopics(sampleTopics(), "")
	m, _ = m.Update(mutedTopicsMsg("zeta", "alpha"))

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")}) // -> only muted

	if got := openedSlug(t, m); got != "alpha" {
		t.Errorf("only-muted index 0 opened %q, want \"alpha\" (sorted)", got)
	}
}

// 'f' three times returns to the unfiltered list.
func TestTopics_Filter_Cycle_ReturnsToAll(t *testing.T) {
	m := screens.NewTopicsModel()
	m = m.SetTopics(sampleTopics(), "") // [tech, art]

	// No topics muted: only-muted (2nd press) is empty, enter is a no-op.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	if got := openedSlug(t, m); got != "" {
		t.Errorf("only-muted with nothing muted: enter opened %q, want no cmd", got)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")}) // -> all
	if got := openedSlug(t, m); got != "tech" {
		t.Errorf("after full cycle: enter opened %q, want \"tech\"", got)
	}
}

// 'f' does nothing while viewing a topic's posts.
func TestTopics_Filter_KeyIgnoredInPostView(t *testing.T) {
	m := screens.NewTopicsModel()
	m = m.SetTopics(sampleTopics(), "")
	m, _ = m.Update(mutedTopicsMsg("tech"))
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // open "tech"
	m = m.SetTopicPosts(sampleTopicPosts(), "")     // flips view to viewTopicPosts

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")}) // must be ignored here
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})                       // back to the list

	if got := openedSlug(t, m); got != "tech" {
		t.Errorf("filter changed from a post-view 'f': enter opened %q, want \"tech\"", got)
	}
}

// Unmuting the highlighted row in only-muted view shrinks the list; the
// selection index must clamp instead of pointing past the end.
func TestTopics_Filter_UnmuteCurrentRow_ClampsIndex(t *testing.T) {
	m := screens.NewTopicsModel()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	m = m.SetTopics(sampleTopics(), "")
	m, _ = m.Update(mutedTopicsMsg("art", "tech")) // both muted

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")}) // only muted: [art, tech]
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}) // select last row ("tech")

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")}) // unmute "tech" -> list shrinks to [art]
	if cmd == nil {
		t.Fatal("expected a SetMutedTopicsMsg cmd from 'm'")
	}
	msg := cmd().(screens.SetMutedTopicsMsg)
	m, _ = m.Update(mutedTopicsMsg(msg.Topics...)) // authoritative update back

	if got := openedSlug(t, m); got != "art" {
		t.Errorf("after clamp: enter opened %q, want \"art\"", got)
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
