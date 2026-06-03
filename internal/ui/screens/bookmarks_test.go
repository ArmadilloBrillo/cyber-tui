package screens_test

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/screens"
)

// With FilterNSFW on, NSFW bookmarked posts are hidden. Replies carry no NSFW
// flag and are always shown. Selection navigates only the visible items.
func TestBookmarks_FilterNSFW_HidesNSFWPostKeepsReply(t *testing.T) {
	m := screens.NewBookmarksModel()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = m.SetBookmarks([]model.Bookmark{
		{ID: "b1", Type: "post", PostID: "p1", Post: &model.Post{ID: "p1", Content: "safe https://example.com/a"}},
		{ID: "b2", Type: "post", PostID: "p2", Post: &model.Post{ID: "p2", Content: "nsfw https://example.com/x", IsNSFW: true}},
		{ID: "b3", Type: "reply", ReplyID: "r1", Reply: &model.Reply{ID: "r1", Content: "reply https://example.com/b"}},
	}, "")
	m, _ = m.Update(nsfwFilterMsg(true))

	// Visible list is [b1(post), b3(reply)]; the NSFW post b2 is hidden.
	if got := m.GetFocusedURLs(); len(got) != 1 || got[0] != "https://example.com/a" {
		t.Fatalf("focused URLs at top = %v, want [https://example.com/a]", got)
	}
	// Down once lands on the reply, not the hidden NSFW post.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if got := m.GetFocusedURLs(); len(got) != 1 || got[0] != "https://example.com/b" {
		t.Fatalf("focused URLs after down = %v, want [https://example.com/b]", got)
	}
	// A second down must not move past the last visible item.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if got := m.GetFocusedURLs(); len(got) != 1 || got[0] != "https://example.com/b" {
		t.Fatalf("focused URLs after second down = %v, want still [https://example.com/b]", got)
	}
}

func TestBookmarks_FilterNSFW_Off_ShowsNSFW(t *testing.T) {
	m := screens.NewBookmarksModel()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = m.SetBookmarks([]model.Bookmark{
		{ID: "b1", Type: "post", PostID: "p1", Post: &model.Post{ID: "p1", Content: "safe https://example.com/a"}},
		{ID: "b2", Type: "post", PostID: "p2", Post: &model.Post{ID: "p2", Content: "nsfw https://example.com/x", IsNSFW: true}},
	}, "")
	m, _ = m.Update(nsfwFilterMsg(false))

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if got := m.GetFocusedURLs(); len(got) != 1 || got[0] != "https://example.com/x" {
		t.Fatalf("focused URLs = %v, want [https://example.com/x]", got)
	}
}
