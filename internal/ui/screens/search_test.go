package screens_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/screens"
)

// readySearch returns a SearchModel with its viewport initialised (m.ready == true),
// required before Set*/navigation calls actually build the selectable row list.
func readySearch() screens.SearchModel {
	m := screens.NewSearchModel()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return m
}

func typeText(m screens.SearchModel, s string) screens.SearchModel {
	for _, r := range s {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return m
}

func pressKey(m screens.SearchModel, key string) (screens.SearchModel, tea.Msg) {
	var msg tea.KeyMsg
	switch key {
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		msg = tea.KeyMsg{Type: tea.KeyEsc}
	default:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
	m2, cmd := m.Update(msg)
	if cmd == nil {
		return m2, nil
	}
	return m2, cmd()
}

func samplePosts(n int) []model.Post {
	posts := make([]model.Post, n)
	for i := range n {
		posts[i] = model.Post{ID: "p" + string(rune('0'+i)), AuthorUsername: "author", Content: "post content"}
	}
	return posts
}

// --- query mode ---

func TestSearch_SubmitQuery_EmitsSubmitSearchMsg(t *testing.T) {
	m := readySearch()
	m = typeText(m, "neuromancer")
	_, msg := pressKey(m, "enter")
	sub, ok := msg.(screens.SubmitSearchMsg)
	if !ok {
		t.Fatalf("expected SubmitSearchMsg, got %T", msg)
	}
	if sub.Query != "neuromancer" {
		t.Errorf("expected query %q, got %q", "neuromancer", sub.Query)
	}
}

func TestSearch_SubmitQuery_EmptyIsNoOp(t *testing.T) {
	m := readySearch()
	_, msg := pressKey(m, "enter")
	if msg != nil {
		t.Errorf("expected no message for an empty query, got %T", msg)
	}
}

// --- preview mode: selection starts on the first hit, not a header ---

func TestSearch_Preview_InitialSelectionSkipsHeader_OpensFirstUser(t *testing.T) {
	m := readySearch()
	preview := model.SearchPreview{
		Users: []model.User{{Username: "neuromancer"}, {Username: "molly"}},
	}
	m = m.SetPreview(preview, "case")

	_, msg := pressKey(m, "enter")
	show, ok := msg.(screens.ShowUserProfileMsg)
	if !ok {
		t.Fatalf("expected ShowUserProfileMsg on the first selectable row, got %T", msg)
	}
	if show.Username != "neuromancer" {
		t.Errorf("expected first user hit %q, got %q", "neuromancer", show.Username)
	}
}

func TestSearch_Preview_PostHit_EmitsShowSearchPostMsg(t *testing.T) {
	m := readySearch()
	preview := model.SearchPreview{
		Posts: []model.Post{{ID: "p1", AuthorUsername: "wintermute", Content: "flatline"}},
	}
	m = m.SetPreview(preview, "flatline")

	_, msg := pressKey(m, "enter")
	show, ok := msg.(screens.ShowSearchPostMsg)
	if !ok {
		t.Fatalf("expected ShowSearchPostMsg, got %T", msg)
	}
	if show.Post.ID != "p1" {
		t.Errorf("expected post p1, got %s", show.Post.ID)
	}
}

func TestSearch_Preview_ReplyHit_EmitsShowSearchReplyMsg(t *testing.T) {
	m := readySearch()
	preview := model.SearchPreview{
		Replies: []model.Reply{{ID: "r1", PostID: "p1", AuthorUsername: "molly", Content: "street samurai"}},
	}
	m = m.SetPreview(preview, "samurai")

	_, msg := pressKey(m, "enter")
	show, ok := msg.(screens.ShowSearchReplyMsg)
	if !ok {
		t.Fatalf("expected ShowSearchReplyMsg, got %T", msg)
	}
	if show.PostID != "p1" || show.ReplyID != "r1" {
		t.Errorf("expected PostID=p1 ReplyID=r1, got PostID=%s ReplyID=%s", show.PostID, show.ReplyID)
	}
}

// --- "see all" drill-down: only offered when a category hits the API's 8-hit cap ---

func TestSearch_Preview_SeeAllRow_EmitsDrillSearchTypeMsg(t *testing.T) {
	m := readySearch()
	preview := model.SearchPreview{Posts: samplePosts(8)} // exactly the cap: header + 8 hits + "see all"
	m = m.SetPreview(preview, "cyberspace")

	// Selection starts at the first hit (row 1); step down 8 times to reach the "see all" row.
	for range 8 {
		m, _ = pressKey(m, "j")
	}
	_, msg := pressKey(m, "enter")
	drill, ok := msg.(screens.DrillSearchTypeMsg)
	if !ok {
		t.Fatalf("expected DrillSearchTypeMsg on the see-all row, got %T", msg)
	}
	if drill.Type != "posts" {
		t.Errorf("expected drill type %q, got %q", "posts", drill.Type)
	}
}

func TestSearch_Preview_BelowCap_NoSeeAllRow(t *testing.T) {
	m := readySearch()
	preview := model.SearchPreview{Posts: samplePosts(3)} // below the cap: no "see all" row
	m = m.SetPreview(preview, "cyberspace")

	// Step down past the last hit; there is no seeAll row to land on, so selection
	// should stay pinned to the last hit and Enter should open it, not drill.
	for range 5 {
		m, _ = pressKey(m, "j")
	}
	_, msg := pressKey(m, "enter")
	show, ok := msg.(screens.ShowSearchPostMsg)
	if !ok {
		t.Fatalf("expected ShowSearchPostMsg (no see-all row below the cap), got %T", msg)
	}
	if show.Post.ID != "p2" {
		t.Errorf("expected last post p2, got %s", show.Post.ID)
	}
}

// --- esc navigation between the three modes ---

func TestSearch_Esc_FromPreview_ReturnsToQuery(t *testing.T) {
	m := readySearch()
	m = m.SetPreview(model.SearchPreview{Users: []model.User{{Username: "case"}}}, "case")
	m, _ = pressKey(m, "esc")
	if !m.InputFocused() {
		t.Error("expected esc from preview to return to query mode with the input focused")
	}
	if m.LastQuery() != "case" {
		t.Errorf("expected previous query %q preserved, got %q", "case", m.LastQuery())
	}
}

func TestSearch_Esc_FromTypeList_ReturnsToPreview(t *testing.T) {
	m := readySearch()
	m = m.SetPreview(model.SearchPreview{Posts: samplePosts(8)}, "cyberspace")
	m = m.SetTypeResults("posts", samplePosts(8), nil, nil, "")
	if !m.IsInTypeList() {
		t.Fatal("expected SetTypeResults to enter type-list mode")
	}
	m, _ = pressKey(m, "esc")
	if m.IsInTypeList() || m.InputFocused() {
		t.Error("expected esc from type-list to return to preview mode")
	}
}

// --- pagination: reaching the bottom of a non-exhausted type list requests more ---

func TestSearch_TypeList_BottomRequestsMore(t *testing.T) {
	m := readySearch()
	m = m.SetTypeResults("posts", samplePosts(2), nil, nil, "cursor123") // non-empty cursor: not exhausted

	// Selection starts on the first hit; step to the last hit, then once more to
	// trigger the bottom-of-list pagination fetch.
	m, _ = pressKey(m, "j")
	_, msg := pressKey(m, "j")
	loadMore, ok := msg.(screens.LoadMoreSearchMsg)
	if !ok {
		t.Fatalf("expected LoadMoreSearchMsg at the bottom of a non-exhausted list, got %T", msg)
	}
	if loadMore.Type != "posts" || loadMore.Cursor != "cursor123" {
		t.Errorf("expected Type=posts Cursor=cursor123, got Type=%s Cursor=%s", loadMore.Type, loadMore.Cursor)
	}
}

func TestSearch_TypeList_ExhaustedBottomIsNoOp(t *testing.T) {
	m := readySearch()
	m = m.SetTypeResults("posts", samplePosts(2), nil, nil, "") // empty cursor: exhausted

	m, _ = pressKey(m, "j")
	_, msg := pressKey(m, "j")
	if msg != nil {
		t.Errorf("expected no pagination request once exhausted, got %T", msg)
	}
}

// --- esc while the query box is focused leaves Search immediately (there's
// no result list showing at that point, so there's nothing to peel back
// first) — and always blurs on the way out, so a later arrival via
// tab-cycling (which doesn't call FocusQuery) never inherits a stuck focused
// state that would otherwise swallow q//tab navigation again ---

func TestSearch_Esc_InQueryMode_LeavesDirectly(t *testing.T) {
	m := readySearch() // starts in query mode, focused
	if !m.InputFocused() {
		t.Fatal("setup: expected the query box focused by default")
	}
	m, msg := pressKey(m, "esc")
	if m.InputFocused() {
		t.Error("expected esc to blur the query input on the way out")
	}
	if _, ok := msg.(screens.LeaveSearchMsg); !ok {
		t.Fatalf("expected LeaveSearchMsg on a single esc from query mode, got %T", msg)
	}
}

func TestSearch_Esc_AfterSetError_StillLeavesDirectly(t *testing.T) {
	// Reproduces the reported stuck state: a search that fails (SetError)
	// does not change the view, so the query box is still focused with no
	// results to show. A single esc must still be able to get out.
	m := readySearch()
	m = m.SetError(errors.New("boom"))
	if !m.InputFocused() {
		t.Fatal("setup: SetError should not itself blur the query input")
	}
	_, msg := pressKey(m, "esc")
	if _, ok := msg.(screens.LeaveSearchMsg); !ok {
		t.Fatalf("expected LeaveSearchMsg even after a failed search, got %T", msg)
	}
}

func TestSearch_Esc_PeelsBackThroughLevelsBeforeLeaving(t *testing.T) {
	m := readySearch()
	m = m.SetPreview(model.SearchPreview{Posts: samplePosts(8)}, "cyberspace")
	m = m.SetTypeResults("posts", samplePosts(8), nil, nil, "")

	m, msg := pressKey(m, "esc") // typeList -> preview
	if _, ok := msg.(screens.LeaveSearchMsg); ok {
		t.Fatal("did not expect to leave from type-list on the first esc")
	}
	if m.IsInTypeList() {
		t.Fatal("expected the first esc to step back to preview")
	}

	m, msg = pressKey(m, "esc") // preview -> query (focused)
	if _, ok := msg.(screens.LeaveSearchMsg); ok {
		t.Fatal("did not expect to leave from preview on the second esc")
	}
	if !m.InputFocused() {
		t.Fatal("expected the second esc to focus the query box")
	}

	_, msg = pressKey(m, "esc") // query focused -> leave
	if _, ok := msg.(screens.LeaveSearchMsg); !ok {
		t.Fatalf("expected LeaveSearchMsg on the third esc, got %T", msg)
	}
}

func TestSearch_FocusQuery_RefocusesAfterBlur(t *testing.T) {
	m := readySearch()
	m, _ = pressKey(m, "esc")
	if m.InputFocused() {
		t.Fatal("setup: expected input blurred")
	}
	m = m.FocusQuery()
	if !m.InputFocused() {
		t.Error("expected FocusQuery to refocus the query input")
	}
}

// --- post/reply hits are sorted newest-first, regardless of the order the
// API returned them in (undocumented, and not something the client can rely on) ---

func TestSearch_Preview_PostsSortedByRecencyDescending(t *testing.T) {
	now := time.Now()
	oldest := model.Post{ID: "old", CreatedAt: now.Add(-2 * time.Hour)}
	middle := model.Post{ID: "mid", CreatedAt: now.Add(-1 * time.Hour)}
	newest := model.Post{ID: "new", CreatedAt: now}

	m := readySearch()
	// Deliberately scrambled input order.
	m = m.SetPreview(model.SearchPreview{Posts: []model.Post{oldest, newest, middle}}, "q")

	wantOrder := []string{"new", "mid", "old"}
	for _, want := range wantOrder {
		_, msg := pressKey(m, "enter")
		show, ok := msg.(screens.ShowSearchPostMsg)
		if !ok {
			t.Fatalf("expected ShowSearchPostMsg, got %T", msg)
		}
		if show.Post.ID != want {
			t.Errorf("expected post %q, got %q", want, show.Post.ID)
		}
		m, _ = pressKey(m, "j")
	}
}

func TestSearch_Preview_RepliesSortedByRecencyDescending(t *testing.T) {
	now := time.Now()
	oldest := model.Reply{ID: "old", PostID: "p1", CreatedAt: now.Add(-2 * time.Hour)}
	middle := model.Reply{ID: "mid", PostID: "p1", CreatedAt: now.Add(-1 * time.Hour)}
	newest := model.Reply{ID: "new", PostID: "p1", CreatedAt: now}

	m := readySearch()
	m = m.SetPreview(model.SearchPreview{Replies: []model.Reply{middle, oldest, newest}}, "q")

	wantOrder := []string{"new", "mid", "old"}
	for _, want := range wantOrder {
		_, msg := pressKey(m, "enter")
		show, ok := msg.(screens.ShowSearchReplyMsg)
		if !ok {
			t.Fatalf("expected ShowSearchReplyMsg, got %T", msg)
		}
		if show.ReplyID != want {
			t.Errorf("expected reply %q, got %q", want, show.ReplyID)
		}
		m, _ = pressKey(m, "j")
	}
}

func TestSearch_TypeList_AppendedPageReSortsWholeList(t *testing.T) {
	now := time.Now()
	page1 := []model.Post{
		{ID: "newest", CreatedAt: now},
		{ID: "oldest-of-page1", CreatedAt: now.Add(-3 * time.Hour)},
	}
	// The second page contains an item that belongs in the middle of the
	// already-loaded list, not at the end — a naive append-only strategy
	// would leave it mis-ordered.
	page2 := []model.Post{
		{ID: "middle", CreatedAt: now.Add(-1 * time.Hour)},
	}

	m := readySearch()
	m = m.SetTypeResults("posts", page1, nil, nil, "cursor1")
	m = m.AppendTypeResults("posts", page2, nil, nil, "")

	wantOrder := []string{"newest", "middle", "oldest-of-page1"}
	for _, want := range wantOrder {
		_, msg := pressKey(m, "enter")
		show, ok := msg.(screens.ShowSearchPostMsg)
		if !ok {
			t.Fatalf("expected ShowSearchPostMsg, got %T", msg)
		}
		if show.Post.ID != want {
			t.Errorf("expected post %q, got %q", want, show.Post.ID)
		}
		m, _ = pressKey(m, "j")
	}
}

// --- scrolling down must keep the whole selected (multi-line) card in view,
// not just its top line ---

func TestSearch_ScrollDown_KeepsWholeSelectedCardVisible(t *testing.T) {
	m := screens.NewSearchModel()
	// A short viewport relative to each multi-line post card, so several
	// downward moves are guaranteed to require scrolling.
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 11})

	var posts []model.Post
	for i := range 6 {
		posts = append(posts, model.Post{
			ID:      fmt.Sprintf("p%d", i),
			Content: "post body",
			// Topics render on the card's last line before the closing
			// border — a unique tag here marks whether the *bottom* of this
			// specific card is actually visible, not just its top.
			Topics: []string{fmt.Sprintf("marker%d", i)},
		})
	}
	m = m.SetTypeResults("posts", posts, nil, nil, "")

	for i := range posts {
		marker := fmt.Sprintf("#marker%d", i)
		if !strings.Contains(m.View(), marker) {
			t.Errorf("post %d selected, but its bottom line (%q) has scrolled out of view:\n%s", i, marker, m.View())
		}
		m, _ = pressKey(m, "j")
	}
}
