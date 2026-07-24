package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ragnar/cyber-tui/internal/api"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/screens"
)

func keyMsg(key string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
}

func newTestApp() App {
	return NewApp(api.NewMockClient())
}

// Simulate being logged in on the feed screen.
func loggedInApp() App {
	a := newTestApp()
	a.active = screenFeed
	a.focus = focusMenu
	return a
}

// --- tabIndex ---

func TestTabIndex_Feed(t *testing.T) {
	a := loggedInApp()
	a.active = screenFeed
	if got := a.tabIndex(); got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

func TestTabIndex_Notifs(t *testing.T) {
	a := loggedInApp()
	a.active = screenNotifications
	if got := a.tabIndex(); got != 1 {
		t.Errorf("expected 1, got %d", got)
	}
}

func TestTabIndex_Profile(t *testing.T) {
	a := loggedInApp()
	a.active = screenProfile
	if got := a.tabIndex(); got != 8 {
		t.Errorf("expected 8, got %d", got)
	}
}

// --- navigateTab ---

func TestNavigateTab_RightFromFeed(t *testing.T) {
	a := loggedInApp()
	a.active = screenFeed
	a.navigateTab(+1)
	if a.active != screenNotifications {
		t.Errorf("expected screenNotifications, got %v", a.active)
	}
}

func TestNavigateTab_LeftFromFeed_Wraps(t *testing.T) {
	a := loggedInApp()
	a.active = screenFeed
	a.navigateTab(-1)
	if a.active != screenSettings {
		t.Errorf("expected screenSettings (wrap), got %v", a.active)
	}
}

func TestNavigateTab_RightFromBookmarks_GoesToGuilds(t *testing.T) {
	a := loggedInApp()
	a.active = screenBookmarks
	a.navigateTab(+1)
	if a.active != screenGuilds {
		t.Errorf("expected screenGuilds, got %v", a.active)
	}
}

func TestNavigateTab_RightFromGuilds_GoesToTopics(t *testing.T) {
	a := loggedInApp()
	a.active = screenGuilds
	a.navigateTab(+1)
	if a.active != screenTopics {
		t.Errorf("expected screenTopics, got %v", a.active)
	}
}

func TestNavigateTab_CyclesAllTabsRight(t *testing.T) {
	a := loggedInApp()
	a.active = screenFeed
	// menuTabs order: feed, notifications, c-mail, circ, journal, bookmarks, guilds, topics, profile, search, settings
	expected := []screen{screenNotifications, screenCMail, screenChatrooms, screenJournal, screenBookmarks, screenGuilds, screenTopics, screenProfile, screenSearch, screenSettings, screenFeed}
	for i, want := range expected {
		a.navigateTab(+1)
		if a.active != want {
			t.Errorf("step %d: expected %v, got %v", i+1, want, a.active)
		}
	}
}

func TestNavigateTab_CyclesAllTabsLeft(t *testing.T) {
	a := loggedInApp()
	a.active = screenFeed
	// menuTabs order: feed, notifications, c-mail, circ, journal, bookmarks, guilds, topics, profile, search, settings
	expected := []screen{screenSettings, screenSearch, screenProfile, screenTopics, screenGuilds, screenBookmarks, screenJournal, screenChatrooms, screenCMail, screenNotifications, screenFeed}
	for i, want := range expected {
		a.navigateTab(-1)
		if a.active != want {
			t.Errorf("step %d: expected %v, got %v", i+1, want, a.active)
		}
	}
}

// --- number shortcuts set correct screen ---

func TestNumberShortcut_1_SetsFeed(t *testing.T) {
	a := loggedInApp()
	a.active = screenProfile
	// simulate the key handler logic directly
	a.active = screenFeed
	if a.active != screenFeed {
		t.Errorf("expected screenFeed")
	}
}

// --- activeScreenHasFocusedInput ---

func TestActiveScreenHasFocusedInput_FeedReturnsFalse(t *testing.T) {
	a := loggedInApp()
	a.active = screenFeed
	if a.activeScreenHasFocusedInput() {
		t.Error("feed screen should never report a focused input")
	}
}

func TestActiveScreenHasFocusedInput_ProfileReturnsFalse(t *testing.T) {
	a := loggedInApp()
	a.active = screenProfile
	if a.activeScreenHasFocusedInput() {
		t.Error("profile screen should not report a focused input by default")
	}
}

func TestActiveScreenHasFocusedInput_ChatroomsDefault(t *testing.T) {
	a := loggedInApp()
	a.active = screenChatrooms
	// Input is not focused until a room is selected — default should be false
	// This test guards against regressions where input starts focused unexpectedly
	if a.chatrooms.InputFocused() {
		t.Error("chatrooms input should not be focused before a room is selected")
	}
}

// --- URL opener ---

func TestHandleKeys_O_LoginScreen_NoOp(t *testing.T) {
	a := newTestApp() // active == screenLogin
	a2, _, consumed := a.handleKeys(keyMsg("o"))
	if consumed {
		t.Error("'o' on login screen should not be consumed")
	}
	if a2.urlPickerOpen {
		t.Error("picker should not open on login screen")
	}
}

func TestHandleKeys_O_NoURLs_NoPicker(t *testing.T) {
	a := loggedInApp() // feed with no posts
	a2, cmd, _ := a.handleKeys(keyMsg("o"))
	if a2.urlPickerOpen {
		t.Error("picker should not open when focused item has no URLs")
	}
	if cmd != nil {
		t.Error("expected nil cmd when no URLs available")
	}
}

func TestHandleKeys_O_SingleURL_NoPicker(t *testing.T) {
	a := loggedInApp()
	a.feed = a.feed.SetPosts([]model.Post{
		{ID: "p1", Content: "[link](https://example.com)"},
	}, "")
	a2, _, _ := a.handleKeys(keyMsg("o"))
	if a2.urlPickerOpen {
		t.Error("picker should not open for a single URL")
	}
}

func TestHandleKeys_O_MultipleURLs_OpensPicker(t *testing.T) {
	a := loggedInApp()
	a.feed = a.feed.SetPosts([]model.Post{
		{ID: "p1", Content: "[a](https://one.com) [b](https://two.com)"},
	}, "")
	a2, _, _ := a.handleKeys(keyMsg("o"))
	if !a2.urlPickerOpen {
		t.Error("picker should open when focused item has multiple URLs")
	}
	if len(a2.urlPickerItems) != 2 {
		t.Errorf("expected 2 picker items, got %d", len(a2.urlPickerItems))
	}
	if a2.urlPickerCursor != 0 {
		t.Errorf("picker cursor should start at 0, got %d", a2.urlPickerCursor)
	}
}

// setupChatroomsDetailWithURL opens a room in detail mode with one message
// containing a URL, so InputFocused() is true — the state that makes plain
// 'o' unreachable and ctrl+o necessary.
func setupChatroomsDetailWithURL(a App) App {
	a.active = screenChatrooms
	a.chatrooms = a.chatrooms.SetRooms([]model.Room{{ID: "r1", Slug: "zion", Name: "Zion"}})
	cm, _ := a.chatrooms.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	cm, _ = cm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	cm = cm.SetMessages("zion", []model.Message{
		{ID: "m1", From: model.User{Username: "molly"}, Body: "check https://example.com", CreatedAt: time.Now()},
	})
	a.chatrooms = cm
	return a
}

func TestHandleKeys_CtrlO_ReachesOpenLink_WhileChatroomsInputFocused(t *testing.T) {
	a := setupChatroomsDetailWithURL(loggedInApp())
	if !a.chatrooms.InputFocused() {
		t.Fatal("setup: expected chatrooms input focused in detail mode")
	}
	_, _, consumed := a.handleKeys(tea.KeyMsg{Type: tea.KeyCtrlO})
	if !consumed {
		t.Error("expected ctrl+o to be consumed even while chatrooms input is focused")
	}
}

func TestHandleKeys_O_NotConsumed_WhileChatroomsInputFocused(t *testing.T) {
	a := setupChatroomsDetailWithURL(loggedInApp())
	if !a.chatrooms.InputFocused() {
		t.Fatal("setup: expected chatrooms input focused in detail mode")
	}
	_, _, consumed := a.handleKeys(keyMsg("o"))
	if consumed {
		t.Error("expected plain 'o' to NOT be consumed while chatrooms input is focused — it must still type into the compose box")
	}
}

// --- Search shortcut ('/') ---

func TestHandleKeys_Slash_OpensSearch(t *testing.T) {
	a := loggedInApp()
	a2, _, consumed := a.handleKeys(keyMsg("/"))
	if !consumed {
		t.Fatal("expected '/' to be consumed on a screen with no focused input")
	}
	if a2.active != screenSearch {
		t.Errorf("expected screenSearch, got %v", a2.active)
	}
	if !a2.search.InputFocused() {
		t.Error("expected the search query box to be focused")
	}
}

func TestHandleKeys_Slash_LoginScreen_NoOp(t *testing.T) {
	a := newTestApp() // active == screenLogin
	a2, _, consumed := a.handleKeys(keyMsg("/"))
	if consumed {
		t.Error("'/' on login screen should not be consumed")
	}
	if a2.active == screenSearch {
		t.Error("should not navigate to search from the login screen")
	}
}

func TestHandleKeys_Slash_NotConsumed_WhileChatroomsInputFocused(t *testing.T) {
	a := setupChatroomsDetailWithURL(loggedInApp())
	if !a.chatrooms.InputFocused() {
		t.Fatal("setup: expected chatrooms input focused in detail mode")
	}
	_, _, consumed := a.handleKeys(keyMsg("/"))
	if consumed {
		t.Error("expected '/' to NOT be consumed while chatrooms input is focused — it must still type into the compose box (needed for /dice, /me, etc.)")
	}
}

// --- Search reply deep-link ---

// resolveMsgs runs cmd and flattens a tea.BatchMsg into its individual resolved messages.
func resolveMsgs(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			if c != nil {
				out = append(out, resolveMsgs(c)...)
			}
		}
		return out
	}
	return []tea.Msg{msg}
}

func TestShowSearchReplyMsg_NavigatesToPostDetailAndScrollsToReply(t *testing.T) {
	a := loggedInApp()
	a.active = screenSearch

	m, cmd := a.Update(screens.ShowSearchReplyMsg{PostID: "p1", ReplyID: "r1"})
	a2 := m.(App)

	if a2.active != screenPostDetail {
		t.Fatalf("expected screenPostDetail, got %v", a2.active)
	}
	if a2.postDetailReturn != screenSearch {
		t.Errorf("expected postDetailReturn = screenSearch, got %v", a2.postDetailReturn)
	}
	if a2.pendingReplyID != "r1" {
		t.Errorf("expected pendingReplyID = r1, got %q", a2.pendingReplyID)
	}

	msgs := resolveMsgs(cmd)
	if len(msgs) == 0 {
		t.Fatal("expected resolved messages from the post+replies fetch batch")
	}
	for _, msg := range msgs {
		var model tea.Model
		model, _ = a2.Update(msg)
		a2 = model.(App)
	}
	if a2.pendingReplyID != "" {
		t.Errorf("expected pendingReplyID cleared after replies loaded, got %q", a2.pendingReplyID)
	}
}

// TestHandleKeys_EscBlursSearchQuery_ThenQuitWorks reproduces the reported
// bug: after opening Search, esc must be able to blur the query box so 'q'
// (and tab navigation) work again — this held even when a search failed,
// since SetError alone never changed SearchModel's view/focus state.
func TestHandleKeys_EscBlursSearchQuery_ThenQuitWorks(t *testing.T) {
	a := loggedInApp()
	a2, _, consumed := a.handleKeys(keyMsg("/"))
	if !consumed || !a2.search.InputFocused() {
		t.Fatal("setup: expected '/' to open Search with the query box focused")
	}

	// esc isn't globally intercepted (activeScreenHasFocusedInput blocks
	// everything except ctrl+c/ctrl+o while focused) — it reaches the screen
	// via the normal delegateUpdate fallthrough, exactly like a real keypress would.
	a2.search, _ = a2.search.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if a2.search.InputFocused() {
		t.Fatal("expected esc to blur the query input")
	}

	_, cmd, consumed := a2.handleKeys(keyMsg("q"))
	if !consumed {
		t.Error("expected 'q' to quit once the query input is blurred")
	}
	if cmd == nil {
		t.Error("expected a tea.Quit cmd")
	}
}

// TestHandleKeys_Slash_SetsSearchReturn confirms '/' records the origin
// screen (like profileReturn/postDetailReturn) rather than always assuming Feed.
func TestHandleKeys_Slash_SetsSearchReturn(t *testing.T) {
	a := loggedInApp()
	a.active = screenGuilds
	a2, _, consumed := a.handleKeys(keyMsg("/"))
	if !consumed {
		t.Fatal("expected '/' to be consumed")
	}
	if a2.searchReturn != screenGuilds {
		t.Errorf("expected searchReturn = screenGuilds, got %v", a2.searchReturn)
	}
}

// TestSearch_EscToLeave_ReturnsToOriginScreen confirms a single esc from
// query mode returns to the screen '/' was pressed from, the same
// return-to-origin pattern already used by 'p' -> profile -> esc.
func TestSearch_EscToLeave_ReturnsToOriginScreen(t *testing.T) {
	a := loggedInApp()
	a.active = screenGuilds
	a2, _, _ := a.handleKeys(keyMsg("/"))
	if a2.searchReturn != screenGuilds {
		t.Fatalf("setup: expected searchReturn = screenGuilds, got %v", a2.searchReturn)
	}

	// esc reaches the screen via the normal delegateUpdate fallthrough, same
	// as a real keypress would — resolve the resulting cmd and feed it back
	// through the full App dispatch chain, exactly as Bubble Tea would.
	_, cmd := a2.search.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected esc to return a LeaveSearchMsg cmd")
	}
	msg := cmd()
	if _, ok := msg.(screens.LeaveSearchMsg); !ok {
		t.Fatalf("expected LeaveSearchMsg, got %T", msg)
	}

	m, _ := a2.Update(msg)
	a3 := m.(App)
	if a3.active != screenGuilds {
		t.Errorf("expected esc to return to screenGuilds (origin), got %v", a3.active)
	}
}

// TestNavigateTab_IntoSearch_SetsSearchReturn reproduces a reported bug:
// tab-cycling into Search (rather than pressing '/') left searchReturn at
// its zero value (screenLogin), so Esc from Search would drop an
// authenticated user on the login screen. navigateTabBy must record the
// origin screen itself, since it's a second entry point into Search that
// bypasses the '/' handler's searchReturn assignment.
func TestNavigateTab_IntoSearch_SetsSearchReturn(t *testing.T) {
	a := loggedInApp()
	a.active = screenProfile
	a.navigateTab(+1)
	if a.active != screenSearch {
		t.Fatalf("setup: expected screenSearch, got %v", a.active)
	}
	if a.searchReturn != screenProfile {
		t.Fatalf("expected searchReturn = screenProfile, got %v", a.searchReturn)
	}

	m, _ := a.Update(screens.LeaveSearchMsg{})
	a2 := m.(App)
	if a2.active != screenProfile {
		t.Errorf("expected esc to return to screenProfile (origin), got %v", a2.active)
	}
}

// TestSearch_UserHitToProfile_EscReturnsToSearchOrigin reproduces the
// reported bug: searching from Feed, opening a user hit (-> Profile), then
// pressing esc landed on Guilds instead of back on Search. Root cause:
// handleGuilds's ShowUserProfileMsg case was missing the active-screen guard
// every other handler has, so — since handleGuilds runs before handleSearch
// in App.Update's dispatch chain — it unconditionally claimed the message
// and overwrote profileReturn with screenGuilds regardless of what was
// actually active.
func TestSearch_UserHitToProfile_EscReturnsToSearchOrigin(t *testing.T) {
	a := loggedInApp()
	a.active = screenFeed
	a2, _, _ := a.handleKeys(keyMsg("/")) // searchReturn = screenFeed
	a2.active = screenSearch

	// Search's ShowUserProfileMsg case sets profileReturn and fires
	// loadUserProfileCmd; resolve that and feed the real userProfileLoadedMsg
	// back through Update, exactly as Bubble Tea would.
	m, cmd := a2.Update(screens.ShowUserProfileMsg{Username: "neuromancer"})
	a3 := m.(App)
	if a3.profileReturn != screenSearch {
		t.Fatalf("expected profileReturn = screenSearch, got %v", a3.profileReturn)
	}
	if cmd == nil {
		t.Fatal("expected a profile-load cmd")
	}
	m2, _ := a3.Update(cmd())
	a4 := m2.(App)
	if a4.active != screenProfile {
		t.Fatalf("expected navigation to screenProfile, got %v", a4.active)
	}

	// esc from the read-only profile must return to Search (its own origin),
	// not to Guilds — the reported bug, caused by handleGuilds's
	// ShowUserProfileMsg case missing the active-screen guard every other
	// handler has, so it unconditionally intercepted the message first.
	m3, _ := a4.Update(screens.BackFromProfileMsg{})
	a5 := m3.(App)
	if a5.active != screenSearch {
		t.Errorf("expected esc from profile to return to screenSearch (Search's own origin), got %v", a5.active)
	}

	// esc from Search itself should then return to Feed, the original origin.
	_, cmd2 := a5.search.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m4, _ := a5.Update(cmd2())
	a6 := m4.(App)
	if a6.active != screenFeed {
		t.Errorf("expected esc from search to return to screenFeed (original origin), got %v", a6.active)
	}
}

func TestShowSearchPostMsg_NavigatesToPostDetail(t *testing.T) {
	a := loggedInApp()
	a.active = screenSearch

	m, _ := a.Update(screens.ShowSearchPostMsg{Post: model.Post{ID: "p1", AuthorUsername: "case"}})
	a2 := m.(App)

	if a2.active != screenPostDetail {
		t.Fatalf("expected screenPostDetail, got %v", a2.active)
	}
	if a2.postDetailReturn != screenSearch {
		t.Errorf("expected postDetailReturn = screenSearch, got %v", a2.postDetailReturn)
	}
}

// --- Status bar hints: '?' is unreachable in chat detail mode, ctrl+o isn't ---

func hasHint(hints []hint, key string) bool {
	for _, h := range hints {
		if h.key == key {
			return true
		}
	}
	return false
}

func TestScreenHints_ChatroomsDetail_NoHelpButHasCtrlO(t *testing.T) {
	a := setupChatroomsDetailWithURL(loggedInApp())
	hints := TabsLayout{}.screenHints(a)
	if hasHint(hints, "?") {
		t.Error("expected no '?' hint in chatrooms detail mode — it's unreachable while the compose input is focused")
	}
	if !hasHint(hints, "ctrl+o") {
		t.Error("expected a ctrl+o hint in chatrooms detail mode")
	}
}

func TestScreenHints_ChatroomsList_StillHasHelp(t *testing.T) {
	a := loggedInApp()
	a.active = screenChatrooms
	a.chatrooms = a.chatrooms.SetRooms([]model.Room{{ID: "r1", Slug: "zion", Name: "Zion"}})
	hints := TabsLayout{}.screenHints(a)
	if !hasHint(hints, "?") {
		t.Error("expected '?' hint in chatrooms list mode — no input is focused there")
	}
}

func TestScreenHints_CMailDetail_NoHelpButHasCtrlO(t *testing.T) {
	a := loggedInApp()
	a.cmail = a.cmail.SetConversations([]model.Conversation{
		{ID: "c1", Participants: []model.User{{Username: a.currentUser.Username}, {Username: "molly"}}},
	})
	cm, _ := a.cmail.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	cm, _ = cm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	a.cmail = cm
	a.active = screenCMail

	hints := TabsLayout{}.screenHints(a)
	if hasHint(hints, "?") {
		t.Error("expected no '?' hint in c-mail detail mode — it's unreachable while the compose input is focused")
	}
	if !hasHint(hints, "ctrl+o") {
		t.Error("expected a ctrl+o hint in c-mail detail mode")
	}
}

func TestScreenHints_CMailList_StillHasHelp(t *testing.T) {
	a := loggedInApp()
	a.active = screenCMail
	hints := TabsLayout{}.screenHints(a)
	if !hasHint(hints, "?") {
		t.Error("expected '?' hint in c-mail list mode — no input is focused there")
	}
}

func TestURLPickerKey_Esc_Closes(t *testing.T) {
	a := loggedInApp()
	a.urlPickerOpen = true
	a.urlPickerItems = []string{"https://one.com", "https://two.com"}
	a2, cmd, _ := a.handleKeys(keyMsg("esc"))
	if a2.urlPickerOpen {
		t.Error("picker should be closed after esc")
	}
	if a2.urlPickerItems != nil {
		t.Error("picker items should be nil after esc")
	}
	if cmd != nil {
		t.Error("expected nil cmd on esc")
	}
}

func TestURLPickerKey_Navigation(t *testing.T) {
	a := loggedInApp()
	a.urlPickerOpen = true
	a.urlPickerItems = []string{"https://one.com", "https://two.com", "https://three.com"}
	a.urlPickerCursor = 0

	a2, _, _ := a.handleKeys(keyMsg("j"))
	if a2.urlPickerCursor != 1 {
		t.Errorf("cursor should be 1 after j, got %d", a2.urlPickerCursor)
	}
	a3, _, _ := a2.handleKeys(keyMsg("k"))
	if a3.urlPickerCursor != 0 {
		t.Errorf("cursor should be 0 after k, got %d", a3.urlPickerCursor)
	}
}

func TestURLPickerKey_Enter_ClosesAndEmitsCmd(t *testing.T) {
	a := loggedInApp()
	a.urlPickerOpen = true
	a.urlPickerItems = []string{"https://one.com", "https://two.com"}
	a.urlPickerCursor = 1

	a2, cmd, _ := a.handleKeys(keyMsg("enter"))
	if a2.urlPickerOpen {
		t.Error("picker should be closed after enter")
	}
	if a2.urlPickerItems != nil {
		t.Error("picker items should be cleared after enter")
	}
	if cmd == nil {
		t.Error("expected a cmd after enter")
	}
}

func TestRouteURL_ProfilePath_SetsReturn(t *testing.T) {
	a := loggedInApp()
	a.active = screenFeed
	a2, cmd := a.routeURL("https://cyberspace.online/u/alice")
	if cmd == nil {
		t.Error("expected a cmd for profile navigation")
	}
	if a2.profileReturn != screenFeed {
		t.Errorf("profileReturn should be screenFeed, got %v", a2.profileReturn)
	}
}

func TestRouteURL_ExternalDomain_EmitsCmd(t *testing.T) {
	a := loggedInApp()
	_, cmd := a.routeURL("https://google.com")
	if cmd == nil {
		t.Error("expected a cmd for external URL")
	}
}

func TestRouteURL_RelativeURL_ExternalOpen(t *testing.T) {
	a := loggedInApp()
	_, cmd := a.routeURL("https://cyberspace.online/support")
	if cmd == nil {
		t.Error("expected a cmd for non-profile cyberspace path")
	}
}

func TestGetFocusedURLs_LoginScreen(t *testing.T) {
	a := newTestApp() // active == screenLogin, not in switch
	if got := a.getFocusedURLs(); got != nil {
		t.Errorf("expected nil on login screen, got %v", got)
	}
}

// --- ErrUnauthorized → login redirect ---

func TestHandleUnauthorized_ErrMsgRoutesToLogin(t *testing.T) {
	a := loggedInApp()
	a.ephemeral = true // saveConfig is a no-op, so the real config is untouched
	a.tokens = model.Tokens{IDToken: "x", RefreshToken: "y"}
	a.currentUser = model.User{Email: "me@example.com"}

	m, _ := a.Update(errMsg{api.ErrUnauthorized})
	got, ok := m.(App)
	if !ok {
		t.Fatalf("Update returned %T, want App", m)
	}
	if got.active != screenLogin {
		t.Errorf("active = %v, want screenLogin", got.active)
	}
	if got.tokens != (model.Tokens{}) {
		t.Errorf("tokens not cleared: %+v", got.tokens)
	}
}

func TestHandleUnauthorized_ActionErrAlsoRoutes(t *testing.T) {
	a := loggedInApp()
	a.ephemeral = true
	m, _ := a.Update(actionErrMsg{api.ErrUnauthorized})
	if got := m.(App); got.active != screenLogin {
		t.Errorf("active = %v, want screenLogin after actionErrMsg", got.active)
	}
}

func TestHandleUnauthorized_OtherErrorDoesNotRedirect(t *testing.T) {
	a := loggedInApp()
	a.ephemeral = true
	m, _ := a.Update(errMsg{errors.New("network down")})
	if got := m.(App); got.active != screenFeed {
		t.Errorf("active = %v, want screenFeed (non-401 must not redirect)", got.active)
	}
}

// --- errors never block: notifPostLoadErrMsg & handleErr → banner ---

// A notification pointing to a deleted post must surface as a friendly,
// non-blocking warning banner rather than blanking the notifications list.
func TestNotifPostLoadErr_DeletedPostShowsBanner(t *testing.T) {
	a := loggedInApp()
	a.active = screenNotifications
	deleted := &api.APIError{Code: "NOT_FOUND", Status: 404, Message: "Post not found"}

	m, _ := a.Update(notifPostLoadErrMsg{err: deleted})
	got := m.(App)
	if got.notifyText != "This post has been deleted" {
		t.Errorf("notifyText = %q, want deleted-post banner", got.notifyText)
	}
	if got.notifyLevel != notifyWarn {
		t.Errorf("notifyLevel = %v, want notifyWarn", got.notifyLevel)
	}
	if got.active != screenNotifications {
		t.Errorf("active = %v, want screenNotifications (must not navigate away)", got.active)
	}
}

// Any other post-open failure still surfaces in the banner (with its raw text),
// never blocking the screen.
func TestNotifPostLoadErr_OtherErrorShowsBanner(t *testing.T) {
	a := loggedInApp()
	a.active = screenNotifications

	m, _ := a.Update(notifPostLoadErrMsg{err: errors.New("network down")})
	got := m.(App)
	if got.notifyText != "network down" {
		t.Errorf("notifyText = %q, want raw error text", got.notifyText)
	}
	if got.notifyLevel != notifyError {
		t.Errorf("notifyLevel = %v, want notifyError", got.notifyLevel)
	}
}

// A dead session on the post-open path must still redirect to login rather than
// being swallowed by the banner.
func TestNotifPostLoadErr_UnauthorizedRedirectsToLogin(t *testing.T) {
	a := loggedInApp()
	a.active = screenNotifications
	a.ephemeral = true

	m, _ := a.Update(notifPostLoadErrMsg{err: api.ErrUnauthorized})
	if got := m.(App); got.active != screenLogin {
		t.Errorf("active = %v, want screenLogin", got.active)
	}
}

// handleErr must fire the global banner (non-blocking) for load failures while
// keeping the user on their current screen.
func TestHandleErr_FiresBannerWithoutBlocking(t *testing.T) {
	a := loggedInApp()
	a.active = screenFeed

	m, _ := a.Update(errMsg{errors.New("network down")})
	got := m.(App)
	if got.notifyText != "network down" {
		t.Errorf("notifyText = %q, want banner with error text", got.notifyText)
	}
	if got.active != screenFeed {
		t.Errorf("active = %v, want screenFeed (error must not navigate)", got.active)
	}
}

// friendlyErr softens the raw API 404 wording for the banner.
func TestFriendlyErr_404IsSoftened(t *testing.T) {
	got := friendlyErr(&api.APIError{Code: "NOT_FOUND", Status: 404, Message: "Post not found"})
	if got != "Not found — it may have been deleted." {
		t.Errorf("friendlyErr(404) = %q, want softened message", got)
	}
	if got := friendlyErr(errors.New("boom")); got != "boom" {
		t.Errorf("friendlyErr(non-api) = %q, want raw text", got)
	}
}

// --- routeURL: ephemeral (SSH) sessions must not drive host side effects ---

func TestRouteURL_EphemeralBlocksExternalOpen(t *testing.T) {
	a := loggedInApp()
	a.ephemeral = true
	got, _ := a.routeURL("https://example.com/page")
	if got.notifyText != "Opening links is disabled in SSH sessions" {
		t.Errorf("notifyText = %q, want SSH-disabled banner", got.notifyText)
	}
}

func TestRouteURL_EphemeralBlocksUnparsableURLFallback(t *testing.T) {
	a := loggedInApp()
	a.ephemeral = true
	got, _ := a.routeURL(":not-a-url")
	if got.notifyText != "Opening links is disabled in SSH sessions" {
		t.Errorf("notifyText = %q, want SSH-disabled banner", got.notifyText)
	}
}

func TestRouteURL_EphemeralAllowsInternalProfileNav(t *testing.T) {
	a := loggedInApp()
	a.ephemeral = true
	got, cmd := a.routeURL("https://cyberspace.online/u/somebody")
	if got.notifyText != "" {
		t.Errorf("notifyText = %q, want empty (internal nav must not be blocked)", got.notifyText)
	}
	if cmd == nil {
		t.Error("cmd = nil, want profile load command")
	}
}

// --- Live-stream reconnect toasts ---

func TestHandleChatrooms_RoomReconnected_ShowsToast(t *testing.T) {
	a := loggedInApp()
	m, _ := a.Update(screens.RoomReconnectedMsg{})
	got := m.(App)
	if got.notifyText != "reconnected to live chat" {
		t.Errorf("notifyText = %q, want reconnect banner", got.notifyText)
	}
	if got.notifyLevel != notifyInfo {
		t.Errorf("notifyLevel = %v, want notifyInfo", got.notifyLevel)
	}
}

func TestHandleCMail_ConvReconnected_ShowsToast(t *testing.T) {
	a := loggedInApp()
	m, _ := a.Update(screens.CMailReconnectedMsg{})
	got := m.(App)
	if got.notifyText != "reconnected to live chat" {
		t.Errorf("notifyText = %q, want reconnect banner", got.notifyText)
	}
	if got.notifyLevel != notifyInfo {
		t.Errorf("notifyLevel = %v, want notifyInfo", got.notifyLevel)
	}
}

// --- /help reply routed to a local system message ---

func TestHandleChatrooms_CommandReply_AppendsSystemMessage(t *testing.T) {
	a := loggedInApp()
	a.chatrooms = a.chatrooms.SetRooms([]model.Room{{ID: "r1", Slug: "zion", Name: "Zion"}})
	cm, _ := a.chatrooms.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	cm, _ = cm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	a.chatrooms = cm

	m, _ := a.Update(roomCommandReplyMsg{roomID: "zion", reply: "Commands: /me, /dice, /help"})
	got := m.(App)
	if view := got.chatrooms.View(); !strings.Contains(view, "Commands: /me, /dice, /help") {
		t.Errorf("expected the /help reply in the chatrooms view, got: %q", view)
	}
}

func TestHandleCMail_CommandReply_AppendsSystemMessage(t *testing.T) {
	a := loggedInApp()
	a.cmail = a.cmail.SetConversations([]model.Conversation{
		{ID: "c1", Participants: []model.User{{Username: a.currentUser.Username}, {Username: "molly"}}},
	})
	cm, _ := a.cmail.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	cm, _ = cm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	a.cmail = cm

	m, _ := a.Update(cmailCommandReplyMsg{convID: "c1", reply: "Commands: /me, /dice, /help"})
	got := m.(App)
	if view := got.cmail.View(); !strings.Contains(view, "Commands: /me, /dice, /help") {
		t.Errorf("expected the /help reply in the c-mail view, got: %q", view)
	}
}
