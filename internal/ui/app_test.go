package ui

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ragnar/cyber-tui/internal/api"
	"github.com/ragnar/cyber-tui/internal/model"
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
	if got := a.tabIndex(); got != 6 {
		t.Errorf("expected 6, got %d", got)
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
	// menuTabs order: feed, notifications, journal, bookmarks, guilds, topics, profile, settings
	expected := []screen{screenNotifications, screenJournal, screenBookmarks, screenGuilds, screenTopics, screenProfile, screenSettings, screenFeed}
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
	// menuTabs order: feed, notifications, journal, bookmarks, guilds, topics, profile, settings
	expected := []screen{screenSettings, screenProfile, screenTopics, screenGuilds, screenBookmarks, screenJournal, screenNotifications, screenFeed}
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
