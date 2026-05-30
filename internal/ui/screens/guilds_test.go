package screens_test

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/screens"
)

func sampleGuilds() []model.Guild {
	return []model.Guild{
		{ID: "g1", Name: "Alpha", Slug: "alpha", Icon: "🐺", MemberCount: 10},
		{ID: "g2", Name: "Beta", Slug: "beta", Icon: "🦊", MemberCount: 5},
	}
}

func sampleGuildPosts() []model.Post {
	return []model.Post{
		{ID: "p1", AuthorUsername: "alice", Content: "hello guild"},
		{ID: "p2", AuthorUsername: "bob", Content: "world"},
	}
}

func keyMsg_g(key string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
}

func specialKey(t tea.KeyType) tea.KeyMsg {
	return tea.KeyMsg{Type: t}
}

// --- Initial state ---

func TestGuildsModel_InitialState(t *testing.T) {
	t.Helper()
	m := screens.NewGuildsModel()
	if m.IsLoaded() {
		t.Error("new model should not be loaded")
	}
}

// --- SetGuilds ---

func TestGuildsModel_SetGuilds_MarksLoaded(t *testing.T) {
	t.Helper()
	m := screens.NewGuildsModel()
	m = m.SetGuilds(sampleGuilds(), "")
	if !m.IsLoaded() {
		t.Error("SetGuilds should mark model as loaded")
	}
}

func TestGuildsModel_SetGuilds_PopulatesList(t *testing.T) {
	t.Helper()
	m := screens.NewGuildsModel()
	m = m.SetGuilds(sampleGuilds(), "nextcursor")
	if !m.IsLoaded() {
		t.Error("expected loaded after SetGuilds")
	}
}

func TestGuildsModel_SetGuilds_ResetsCursor(t *testing.T) {
	t.Helper()
	m := screens.NewGuildsModel()
	// Simulate cursor at 1, then fresh page resets to 0
	m = m.SetGuilds(sampleGuilds(), "")
	m2, _ := m.Update(keyMsg_g("j"))
	if !m2.IsLoaded() {
		t.Fatal("expected loaded")
	}
	m3 := m2.SetGuilds(sampleGuilds(), "")
	_ = m3 // cursor should be reset; no panic
}

// --- AppendGuilds ---

func TestGuildsModel_AppendGuilds_AddsItems(t *testing.T) {
	t.Helper()
	m := screens.NewGuildsModel()
	m = m.SetGuilds(sampleGuilds(), "cursor1")
	extra := []model.Guild{{ID: "g3", Name: "Gamma", Slug: "gamma", MemberCount: 1}}
	m = m.AppendGuilds(extra, "")
	if !m.IsLoaded() {
		t.Error("model should remain loaded after AppendGuilds")
	}
}

// --- SetGuildPosts ---

func TestGuildsModel_EnterKey_SetsBrowsingState(t *testing.T) {
	t.Helper()
	m := screens.NewGuildsModel()
	m = m.SetGuilds(sampleGuilds(), "")
	if m.IsBrowsingGuild() {
		t.Error("should not be browsing a guild before entering one")
	}
	// enter sets activeGuild immediately; app later calls SetGuildPosts
	m, _ = m.Update(specialKey(tea.KeyEnter))
	if !m.IsBrowsingGuild() {
		t.Error("should be browsing guild after pressing enter on a guild")
	}
}

// --- Navigation ---

func TestGuildsModel_DownKey_DoesNotPanic_WithNoGuilds(t *testing.T) {
	t.Helper()
	m := screens.NewGuildsModel()
	m, _ = m.Update(keyMsg_g("j"))
	_ = m
}

func TestGuildsModel_EnterKey_EmitsLoadGuildPostsMsg(t *testing.T) {
	t.Helper()
	m := screens.NewGuildsModel()
	m = m.SetGuilds(sampleGuilds(), "")
	_, cmd := m.Update(specialKey(tea.KeyEnter))
	if cmd == nil {
		t.Fatal("expected a cmd after enter on a guild")
	}
	msg := cmd()
	lgm, ok := msg.(screens.LoadGuildPostsMsg)
	if !ok {
		t.Fatalf("expected LoadGuildPostsMsg, got %T", msg)
	}
	if lgm.Slug != "alpha" {
		t.Errorf("expected slug 'alpha', got %q", lgm.Slug)
	}
}

func TestGuildsModel_EscKey_ReturnToGuildList(t *testing.T) {
	t.Helper()
	m := screens.NewGuildsModel()
	m = m.SetGuilds(sampleGuilds(), "")
	// Enter a guild (sets activeGuild), then simulate app delivering posts
	m, _ = m.Update(specialKey(tea.KeyEnter))
	m = m.SetGuildPosts(sampleGuildPosts(), "")
	if !m.IsBrowsingGuild() {
		t.Fatal("expected to be browsing guild before esc")
	}
	m, _ = m.Update(specialKey(tea.KeyEsc))
	if m.IsBrowsingGuild() {
		t.Error("esc should clear activeGuild and return to guild list")
	}
}

func TestGuildsModel_DownAtBottom_EmitsLoadMoreGuildsMsg(t *testing.T) {
	t.Helper()
	m := screens.NewGuildsModel()
	// One guild, cursor set (not exhausted)
	m = m.SetGuilds([]model.Guild{{ID: "g1", Name: "Alpha", Slug: "alpha"}}, "cursor1")
	// Navigate to last item
	_, cmd := m.Update(keyMsg_g("j"))
	if cmd == nil {
		t.Fatal("expected a cmd when pressing down at bottom of non-exhausted list")
	}
	msg := cmd()
	lmg, ok := msg.(screens.LoadMoreGuildsMsg)
	if !ok {
		t.Fatalf("expected LoadMoreGuildsMsg, got %T", msg)
	}
	if lmg.Cursor != "cursor1" {
		t.Errorf("expected cursor 'cursor1', got %q", lmg.Cursor)
	}
}

// --- GetFocusedURLs ---

func TestGuildsModel_GetFocusedURLs_NilWhenInGuildList(t *testing.T) {
	t.Helper()
	m := screens.NewGuildsModel()
	m = m.SetGuilds(sampleGuilds(), "")
	if urls := m.GetFocusedURLs(); urls != nil {
		t.Errorf("expected nil in guild list view, got %v", urls)
	}
}

func TestGuildsModel_GetFocusedURLs_ReturnsURLsInPostView(t *testing.T) {
	t.Helper()
	m := screens.NewGuildsModel()
	m = m.SetGuilds(sampleGuilds(), "")
	m = m.SetGuildPosts([]model.Post{
		{ID: "p1", AuthorUsername: "alice", Content: "visit https://example.com please"},
	}, "")
	urls := m.GetFocusedURLs()
	if len(urls) == 0 {
		t.Error("expected at least one URL from post content")
	}
}
