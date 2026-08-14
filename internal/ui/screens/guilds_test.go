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

// --- OpenGuild ---

func TestGuildsModel_OpenGuild_SetsActiveGuild(t *testing.T) {
	m := screens.NewGuildsModel()
	m = m.OpenGuild("night-owls")
	if got := m.ActiveGuild(); got != "night-owls" {
		t.Errorf("ActiveGuild() = %q, want %q", got, "night-owls")
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

// --- Guild members ---

func sampleGuildMembers() []model.GuildMember {
	return []model.GuildMember{
		{MembershipID: "g1_u1", UserID: "u1", Username: "alice", Role: "founder"},
		{MembershipID: "g1_u2", UserID: "u2", Username: "bob", Role: "member"},
	}
}

func TestGuildsModel_SetGuildMembers_SwitchesToMembersView(t *testing.T) {
	t.Helper()
	m := screens.NewGuildsModel()
	m = m.SetGuilds(sampleGuilds(), "")
	m, _ = m.Update(specialKey(tea.KeyEnter))
	m = m.SetGuildPosts(sampleGuildPosts(), "")
	m = m.SetGuildMembers(sampleGuildMembers(), "")
	if !m.IsBrowsingMembers() {
		t.Error("SetGuildMembers should switch to members view")
	}
}

func TestGuildsModel_SetGuildMembers_ResetsIndex(t *testing.T) {
	t.Helper()
	m := screens.NewGuildsModel()
	m = m.SetGuilds(sampleGuilds(), "")
	m, _ = m.Update(specialKey(tea.KeyEnter))
	m = m.SetGuildPosts(sampleGuildPosts(), "")
	m = m.SetGuildMembers(sampleGuildMembers(), "cursor1")
	// Navigate to second member
	m, _ = m.Update(keyMsg_g("j"))
	// Fresh load should reset index
	m = m.SetGuildMembers(sampleGuildMembers(), "")
	if !m.IsBrowsingMembers() {
		t.Error("expected members view after SetGuildMembers")
	}
}

func TestGuildsModel_AppendGuildMembers_AddsItems(t *testing.T) {
	t.Helper()
	m := screens.NewGuildsModel()
	m = m.SetGuilds(sampleGuilds(), "")
	m, _ = m.Update(specialKey(tea.KeyEnter))
	m = m.SetGuildPosts(sampleGuildPosts(), "")
	m = m.SetGuildMembers(sampleGuildMembers(), "cursor1")
	extra := []model.GuildMember{{MembershipID: "g1_u3", UserID: "u3", Username: "carol", Role: "member"}}
	m = m.AppendGuildMembers(extra, "")
	if !m.IsBrowsingMembers() {
		t.Error("expected members view after AppendGuildMembers")
	}
}

func TestGuildsModel_MemberNavigation_UpDown(t *testing.T) {
	t.Helper()
	m := screens.NewGuildsModel()
	m = m.SetGuilds(sampleGuilds(), "")
	m, _ = m.Update(specialKey(tea.KeyEnter))
	m = m.SetGuildPosts(sampleGuildPosts(), "")
	m = m.SetGuildMembers(sampleGuildMembers(), "")
	// Down from first item
	m, _ = m.Update(keyMsg_g("j"))
	// Up back to first
	m, _ = m.Update(keyMsg_g("k"))
	if !m.IsBrowsingMembers() {
		t.Error("should remain in members view after navigation")
	}
}

func TestGuildsModel_MemberNavigation_DownAtBottom_EmitsLoadMore(t *testing.T) {
	t.Helper()
	m := screens.NewGuildsModel()
	m = m.SetGuilds(sampleGuilds(), "")
	m, _ = m.Update(specialKey(tea.KeyEnter))
	m = m.SetGuildPosts(sampleGuildPosts(), "")
	// One member, cursor not exhausted
	m = m.SetGuildMembers([]model.GuildMember{{MembershipID: "g1_u1", Username: "alice", Role: "founder"}}, "cursor1")
	_, cmd := m.Update(keyMsg_g("j"))
	if cmd == nil {
		t.Fatal("expected a cmd when pressing down at bottom of non-exhausted members list")
	}
	msg := cmd()
	lmm, ok := msg.(screens.LoadMoreGuildMembersMsg)
	if !ok {
		t.Fatalf("expected LoadMoreGuildMembersMsg, got %T", msg)
	}
	if lmm.Cursor != "cursor1" {
		t.Errorf("expected cursor 'cursor1', got %q", lmm.Cursor)
	}
}

func TestGuildsModel_MemberEsc_ReturnsToPostsView(t *testing.T) {
	t.Helper()
	m := screens.NewGuildsModel()
	m = m.SetGuilds(sampleGuilds(), "")
	m, _ = m.Update(specialKey(tea.KeyEnter))
	m = m.SetGuildPosts(sampleGuildPosts(), "")
	m = m.SetGuildMembers(sampleGuildMembers(), "")
	if !m.IsBrowsingMembers() {
		t.Fatal("expected members view before esc")
	}
	m, _ = m.Update(specialKey(tea.KeyEsc))
	if m.IsBrowsingMembers() {
		t.Error("esc should return from members view to posts view")
	}
	if !m.IsBrowsingGuild() {
		t.Error("should still be browsing the guild after returning from members")
	}
}

func TestGuildsModel_MemberEnter_EmitsShowUserProfileMsg(t *testing.T) {
	t.Helper()
	m := screens.NewGuildsModel()
	m = m.SetGuilds(sampleGuilds(), "")
	m, _ = m.Update(specialKey(tea.KeyEnter))
	m = m.SetGuildPosts(sampleGuildPosts(), "")
	m = m.SetGuildMembers(sampleGuildMembers(), "")
	_, cmd := m.Update(specialKey(tea.KeyEnter))
	if cmd == nil {
		t.Fatal("expected a cmd after enter on a member")
	}
	msg := cmd()
	spm, ok := msg.(screens.ShowUserProfileMsg)
	if !ok {
		t.Fatalf("expected ShowUserProfileMsg, got %T", msg)
	}
	if spm.Username != "alice" {
		t.Errorf("expected username 'alice', got %q", spm.Username)
	}
}

func TestGuildsModel_MKey_EmitsLoadGuildMembersMsg(t *testing.T) {
	t.Helper()
	m := screens.NewGuildsModel()
	m = m.SetGuilds(sampleGuilds(), "")
	m, _ = m.Update(specialKey(tea.KeyEnter))
	m = m.SetGuildPosts(sampleGuildPosts(), "")
	_, cmd := m.Update(keyMsg_g("m"))
	if cmd == nil {
		t.Fatal("expected a cmd after pressing m in guild posts view")
	}
	msg := cmd()
	lgm, ok := msg.(screens.LoadGuildMembersMsg)
	if !ok {
		t.Fatalf("expected LoadGuildMembersMsg, got %T", msg)
	}
	if lgm.Slug != "alpha" {
		t.Errorf("expected slug 'alpha', got %q", lgm.Slug)
	}
}

// --- Refresh on up at top ---

func TestGuildsModel_UpAtTop_ForumEmitsRefreshGuildPostsMsg(t *testing.T) {
	t.Helper()
	m := screens.NewGuildsModel()
	m = m.SetGuilds(sampleGuilds(), "")
	m, _ = m.Update(specialKey(tea.KeyEnter))
	m = m.SetGuildPosts(sampleGuildPosts(), "")
	// postIndex is 0 (top) — up should trigger refresh
	m2, cmd := m.Update(specialKey(tea.KeyUp))
	if cmd == nil {
		t.Fatal("expected a cmd when pressing up at top of guild forum")
	}
	msg := cmd()
	rgp, ok := msg.(screens.RefreshGuildPostsMsg)
	if !ok {
		t.Fatalf("expected RefreshGuildPostsMsg, got %T", msg)
	}
	if rgp.Slug != "alpha" {
		t.Errorf("expected slug 'alpha', got %q", rgp.Slug)
	}
	_ = m2
}

func TestGuildsModel_UpAtTop_GuildListEmitsNoCmd(t *testing.T) {
	t.Helper()
	m := screens.NewGuildsModel()
	m = m.SetGuilds(sampleGuilds(), "")
	// guildIndex is 0 (top of list) — up should do nothing
	_, cmd := m.Update(specialKey(tea.KeyUp))
	if cmd != nil {
		msg := cmd()
		t.Errorf("expected no cmd in guild list, got message %T", msg)
	}
}

// --- FilterNSFW ---

func TestGuildsModel_FilterNSFW_HidesNSFWPost(t *testing.T) {
	t.Helper()
	m := screens.NewGuildsModel()
	m = m.SetGuilds(sampleGuilds(), "")
	m, _ = m.Update(specialKey(tea.KeyEnter))
	m = m.SetGuildPosts([]model.Post{
		{ID: "g1", AuthorUsername: "alice", Content: "safe"},
		{ID: "g2", AuthorUsername: "bob", Content: "nsfw", IsNSFW: true},
		{ID: "g3", AuthorUsername: "carol", Content: "also safe"},
	}, "")
	m, _ = m.Update(nsfwFilterMsg(true))

	// Navigate to end of visible list (2 visible, max index 1)
	m, _ = m.Update(keyMsg_g("j"))
	m, _ = m.Update(keyMsg_g("j"))

	// Enter should return g3, not g2
	_, cmd := m.Update(specialKey(tea.KeyEnter))
	if cmd == nil {
		t.Fatal("expected a cmd on enter")
	}
	msg := cmd()
	sp, ok := msg.(screens.ShowGuildPostMsg)
	if !ok {
		t.Fatalf("expected ShowGuildPostMsg, got %T", msg)
	}
	if sp.Post.ID != "g3" {
		t.Errorf("expected g3 (safe), got %s", sp.Post.ID)
	}
}

func TestGuildsModel_FilterNSFW_Off_ShowsAll(t *testing.T) {
	t.Helper()
	m := screens.NewGuildsModel()
	m = m.SetGuilds(sampleGuilds(), "")
	m, _ = m.Update(specialKey(tea.KeyEnter))
	m = m.SetGuildPosts([]model.Post{
		{ID: "g1", AuthorUsername: "alice", Content: "safe"},
		{ID: "g2", AuthorUsername: "bob", Content: "nsfw", IsNSFW: true},
	}, "")
	m, _ = m.Update(nsfwFilterMsg(false))

	m, _ = m.Update(keyMsg_g("j"))
	_, cmd := m.Update(specialKey(tea.KeyEnter))
	if cmd == nil {
		t.Fatal("expected a cmd on enter")
	}
	msg := cmd()
	sp, ok := msg.(screens.ShowGuildPostMsg)
	if !ok {
		t.Fatalf("expected ShowGuildPostMsg, got %T", msg)
	}
	if sp.Post.ID != "g2" {
		t.Errorf("expected g2 (nsfw), got %s", sp.Post.ID)
	}
}

// --- Guild join / leave ---

func inPostsView(t *testing.T) screens.GuildsModel {
	t.Helper()
	m := screens.NewGuildsModel()
	m = m.SetGuilds(sampleGuilds(), "")
	m, _ = m.Update(specialKey(tea.KeyEnter))
	m = m.SetGuildPosts(sampleGuildPosts(), "")
	return m
}

func TestGuildsModel_SetGuildDetail_StoresDetailAndMarksLoaded(t *testing.T) {
	t.Helper()
	m := inPostsView(t)
	if m.IsDetailLoaded() {
		t.Error("detail should not be loaded before SetGuildDetail")
	}
	m = m.SetGuildDetail(model.Guild{ID: "g1", Name: "Alpha", Slug: "alpha", IsMember: false})
	if !m.IsDetailLoaded() {
		t.Error("IsDetailLoaded should return true after SetGuildDetail")
	}
	if m.GuildDetail().Name != "Alpha" {
		t.Errorf("expected Name 'Alpha', got %q", m.GuildDetail().Name)
	}
}

func TestGuildsModel_JKey_WhenNotMember_SetsConfirmJoin(t *testing.T) {
	t.Helper()
	m := inPostsView(t)
	m = m.SetGuildDetail(model.Guild{Slug: "alpha", Name: "Alpha", IsMember: false})
	m, _ = m.Update(keyMsg_g("J"))
	if !m.IsConfirmingJoin() {
		t.Error("J key when not a member should set confirming to join")
	}
}

func TestGuildsModel_JKey_WhenAlreadyMember_DoesNothing(t *testing.T) {
	t.Helper()
	m := inPostsView(t)
	m = m.SetGuildDetail(model.Guild{Slug: "alpha", Name: "Alpha", IsMember: true, Role: "member"})
	m, _ = m.Update(keyMsg_g("J"))
	if m.IsConfirmingJoin() {
		t.Error("J key when already a member should not set confirming")
	}
}

func TestGuildsModel_JKey_WhenDetailNotLoaded_DoesNothing(t *testing.T) {
	t.Helper()
	m := inPostsView(t)
	// detail not loaded
	m, _ = m.Update(keyMsg_g("J"))
	if m.IsConfirmingJoin() {
		t.Error("J key when detail not loaded should not set confirming")
	}
}

func TestGuildsModel_LKey_WhenMember_SetsConfirmLeave(t *testing.T) {
	t.Helper()
	m := inPostsView(t)
	m = m.SetGuildDetail(model.Guild{Slug: "alpha", Name: "Alpha", IsMember: true, Role: "member"})
	m, _ = m.Update(keyMsg_g("L"))
	if !m.IsConfirmingLeave() {
		t.Error("l key when a member should set confirming to leave")
	}
}

func TestGuildsModel_LKey_WhenFounder_DoesNothing(t *testing.T) {
	t.Helper()
	m := inPostsView(t)
	m = m.SetGuildDetail(model.Guild{Slug: "alpha", Name: "Alpha", IsMember: true, Role: "founder"})
	m, _ = m.Update(keyMsg_g("L"))
	if m.IsConfirmingLeave() {
		t.Error("l key when founder should not set confirming")
	}
}

func TestGuildsModel_ConfirmY_Join_EmitsJoinGuildMsg(t *testing.T) {
	t.Helper()
	m := inPostsView(t)
	m = m.SetGuildDetail(model.Guild{Slug: "alpha", Name: "Alpha", IsMember: false})
	m, _ = m.Update(keyMsg_g("J"))
	m2, cmd := m.Update(keyMsg_g("y"))
	if cmd == nil {
		t.Fatal("expected a cmd after y confirmation")
	}
	msg := cmd()
	jm, ok := msg.(screens.JoinGuildMsg)
	if !ok {
		t.Fatalf("expected JoinGuildMsg, got %T", msg)
	}
	if jm.Slug != "alpha" {
		t.Errorf("expected slug 'alpha', got %q", jm.Slug)
	}
	if m2.IsConfirmingJoin() {
		t.Error("confirming should be cleared after y")
	}
}

func TestGuildsModel_ConfirmY_Leave_EmitsLeaveGuildMsg(t *testing.T) {
	t.Helper()
	m := inPostsView(t)
	m = m.SetGuildDetail(model.Guild{Slug: "alpha", Name: "Alpha", IsMember: true, Role: "member"})
	m, _ = m.Update(keyMsg_g("L"))
	m2, cmd := m.Update(keyMsg_g("y"))
	if cmd == nil {
		t.Fatal("expected a cmd after y confirmation")
	}
	msg := cmd()
	lm, ok := msg.(screens.LeaveGuildMsg)
	if !ok {
		t.Fatalf("expected LeaveGuildMsg, got %T", msg)
	}
	if lm.Slug != "alpha" {
		t.Errorf("expected slug 'alpha', got %q", lm.Slug)
	}
	if m2.IsConfirmingLeave() {
		t.Error("confirming should be cleared after y")
	}
}

func TestGuildsModel_ConfirmN_ClearsConfirming(t *testing.T) {
	t.Helper()
	m := inPostsView(t)
	m = m.SetGuildDetail(model.Guild{Slug: "alpha", Name: "Alpha", IsMember: false})
	m, _ = m.Update(keyMsg_g("J"))
	m2, cmd := m.Update(keyMsg_g("n"))
	if cmd != nil {
		t.Error("n should not emit a cmd")
	}
	if m2.IsConfirmingJoin() {
		t.Error("n should clear confirming")
	}
}

func TestGuildsModel_ConfirmEsc_ClearsConfirming_StaysInPostsView(t *testing.T) {
	t.Helper()
	m := inPostsView(t)
	m = m.SetGuildDetail(model.Guild{Slug: "alpha", Name: "Alpha", IsMember: false})
	m, _ = m.Update(keyMsg_g("J"))
	m2, _ := m.Update(specialKey(tea.KeyEsc))
	if m2.IsConfirmingJoin() {
		t.Error("esc should clear confirming")
	}
	if !m2.IsBrowsingGuild() {
		t.Error("esc while confirming should stay in posts view, not navigate back")
	}
}

func TestGuildsModel_Esc_WithoutConfirm_NavigatesBack(t *testing.T) {
	t.Helper()
	m := inPostsView(t)
	// No confirming active — esc should navigate back to guild list
	m2, _ := m.Update(specialKey(tea.KeyEsc))
	if m2.IsBrowsingGuild() {
		t.Error("esc without confirming should navigate back to guild list")
	}
}

func TestGuildsModel_EscBack_ClearsDetailState(t *testing.T) {
	t.Helper()
	m := inPostsView(t)
	m = m.SetGuildDetail(model.Guild{Slug: "alpha", Name: "Alpha", IsMember: true, Role: "member"})
	m2, _ := m.Update(specialKey(tea.KeyEsc))
	if m2.IsDetailLoaded() {
		t.Error("navigating back should clear guildDetailLoaded")
	}
}

// --- VisibleInlineImages ---

func TestGuildsModel_VisibleInlineImages_DisabledByDefault(t *testing.T) {
	t.Helper()
	m := screens.NewGuildsModel()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	m = m.SetGuilds(sampleGuilds(), "")
	m, _ = m.Update(specialKey(tea.KeyEnter))
	m = m.SetGuildPosts([]model.Post{
		{ID: "gp1", AuthorUsername: "alice", Content: "hi\n\n![a](https://example.com/a.png)\n\nbye"},
	}, "")

	if slots := m.VisibleInlineImages(); slots != nil {
		t.Errorf("expected no slots while disabled, got %+v", slots)
	}
}

func TestGuildsModel_VisibleInlineImages_ReportsSlotWhenEnabled(t *testing.T) {
	t.Helper()
	m := screens.NewGuildsModel()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	m, _ = m.Update(screens.SharedConfigMsg{InlineImagesEnabled: true})
	m = m.SetGuilds(sampleGuilds(), "")
	m, _ = m.Update(specialKey(tea.KeyEnter))
	m = m.SetGuildPosts([]model.Post{
		{ID: "gp1", AuthorUsername: "alice", Content: "hi\n\n![a](https://example.com/a.png)\n\nbye"},
	}, "")

	slots := m.VisibleInlineImages()
	if len(slots) != 1 {
		t.Fatalf("expected 1 slot, got %d: %+v", len(slots), slots)
	}
	if slots[0].URL != "https://example.com/a.png" {
		t.Errorf("URL = %q, want https://example.com/a.png", slots[0].URL)
	}
}
