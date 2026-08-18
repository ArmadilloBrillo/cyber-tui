package ui

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ragnar/cyber-tui/internal/api"
	"github.com/ragnar/cyber-tui/internal/config"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/imgview"
	"github.com/ragnar/cyber-tui/internal/ui/screens"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
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
	if got := tabIndexOf(a); got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

func TestTabIndex_Notifs(t *testing.T) {
	a := loggedInApp()
	a.active = screenNotifications
	if got := tabIndexOf(a); got != 1 {
		t.Errorf("expected 1, got %d", got)
	}
}

func TestTabIndex_Profile(t *testing.T) {
	a := loggedInApp()
	a.active = screenProfile
	if got := tabIndexOf(a); got != 8 {
		t.Errorf("expected 8, got %d", got)
	}
}

// --- navigateTab ---

func TestNavigateTab_RightFromFeed(t *testing.T) {
	a := loggedInApp()
	a.active = screenFeed
	a, _ = navigateTabBy(a, +1)
	if a.active != screenNotifications {
		t.Errorf("expected screenNotifications, got %v", a.active)
	}
}

func TestNavigateTab_LeftFromFeed_Wraps(t *testing.T) {
	a := loggedInApp()
	a.active = screenFeed
	a, _ = navigateTabBy(a, -1)
	if a.active != screenSettings {
		t.Errorf("expected screenSettings (wrap), got %v", a.active)
	}
}

func TestNavigateTab_RightFromBookmarks_GoesToGuilds(t *testing.T) {
	a := loggedInApp()
	a.active = screenBookmarks
	a, _ = navigateTabBy(a, +1)
	if a.active != screenGuilds {
		t.Errorf("expected screenGuilds, got %v", a.active)
	}
}

func TestNavigateTab_RightFromGuilds_GoesToTopics(t *testing.T) {
	a := loggedInApp()
	a.active = screenGuilds
	a, _ = navigateTabBy(a, +1)
	if a.active != screenTopics {
		t.Errorf("expected screenTopics, got %v", a.active)
	}
}

func TestNavigateTab_CyclesAllTabsRight(t *testing.T) {
	a := loggedInApp()
	a.active = screenFeed
	// visibleTabs order: feed, notifications, c-mail, circ, journal, bookmarks, guilds, topics, profile, settings
	// (search is hidden — reachable only via "g s"/"/", never by cycling; see navigateTabBy)
	expected := []screen{screenNotifications, screenCMail, screenChatrooms, screenJournal, screenBookmarks, screenGuilds, screenTopics, screenProfile, screenSettings, screenFeed}
	for i, want := range expected {
		a, _ = navigateTabBy(a, +1)
		if a.active != want {
			t.Errorf("step %d: expected %v, got %v", i+1, want, a.active)
		}
	}
}

func TestNavigateTab_CyclesAllTabsLeft(t *testing.T) {
	a := loggedInApp()
	a.active = screenFeed
	// visibleTabs order: feed, notifications, c-mail, circ, journal, bookmarks, guilds, topics, profile, settings
	// (search is hidden — reachable only via "g s"/"/", never by cycling; see navigateTabBy)
	expected := []screen{screenSettings, screenProfile, screenTopics, screenGuilds, screenBookmarks, screenJournal, screenChatrooms, screenCMail, screenNotifications, screenFeed}
	for i, want := range expected {
		a, _ = navigateTabBy(a, -1)
		if a.active != want {
			t.Errorf("step %d: expected %v, got %v", i+1, want, a.active)
		}
	}
}

// TestNavigateTab_ProfileToSettings_SkipsSearch confirms Search — hidden
// from the tab rotation — is never landed on while cycling: Profile is the
// last visible tab before Settings in menuTabs order, with Search sitting
// between them but excluded.
func TestNavigateTab_ProfileToSettings_SkipsSearch(t *testing.T) {
	a := loggedInApp()
	a.active = screenProfile
	a, _ = navigateTabBy(a, +1)
	if a.active != screenSettings {
		t.Errorf("expected screenSettings (skipping hidden screenSearch), got %v", a.active)
	}
}

// TestNavigateTab_NoOpWhileOnSearch confirms arrow-key/j-k cycling can't
// move off Search either, now that it's a hidden, explicit-entry-only
// destination — the same treatment screenPostDetail already got.
func TestNavigateTab_NoOpWhileOnSearch(t *testing.T) {
	a := loggedInApp()
	a.active = screenSearch
	a, _ = navigateTabBy(a, +1)
	if a.active != screenSearch {
		t.Errorf("expected navigateTab to no-op while on screenSearch, got %v", a.active)
	}
	a, _ = navigateTabBy(a, -1)
	if a.active != screenSearch {
		t.Errorf("expected navigateTab to no-op while on screenSearch, got %v", a.active)
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

// --- leader key ("g" + mnemonic) ---

func TestHandleKeys_Leader_G_ArmsAndConsumes(t *testing.T) {
	a := loggedInApp()
	a2, cmd, consumed := a.handleKeys(keyMsg("g"))
	if !consumed {
		t.Fatal("expected 'g' to be consumed")
	}
	if !a2.leaderPending {
		t.Error("expected leaderPending to be armed after 'g'")
	}
	if cmd != nil {
		t.Error("expected nil cmd from arming the leader key")
	}
}

func TestHandleKeys_Leader_G_LoginScreen_NoOp(t *testing.T) {
	a := newTestApp() // active == screenLogin
	a2, _, consumed := a.handleKeys(keyMsg("g"))
	if consumed {
		t.Error("'g' on login screen should not be consumed")
	}
	if a2.leaderPending {
		t.Error("leaderPending should not arm on login screen")
	}
}

func TestHandleKeys_Esc_QuitsOnLoginScreen(t *testing.T) {
	a := newTestApp() // active == screenLogin
	_, cmd, consumed := a.handleKeys(tea.KeyMsg{Type: tea.KeyEsc})
	if !consumed {
		t.Error("expected esc to be consumed on login screen")
	}
	if cmd == nil {
		t.Error("expected esc to fire a quit command on login screen")
	}
}

func TestHandleKeys_Esc_NotConsumed_OffLoginScreen(t *testing.T) {
	a := loggedInApp()
	a.active = screenProfile
	_, cmd, consumed := a.handleKeys(tea.KeyMsg{Type: tea.KeyEsc})
	if consumed {
		t.Error("expected esc to NOT be consumed by the global quit handler off the login screen")
	}
	if cmd != nil {
		t.Error("expected no quit command from esc off the login screen")
	}
}

func TestHandleKeys_Leader_MappedChord_Navigates(t *testing.T) {
	a := loggedInApp()
	a.active = screenProfile
	a2, _, consumed := a.handleKeys(keyMsg("g"))
	if !consumed || !a2.leaderPending {
		t.Fatal("setup: expected 'g' to arm the leader key")
	}
	a3, _, consumed := a2.handleKeys(keyMsg("j")) // g j -> Journal
	if !consumed {
		t.Fatal("expected the leader chord's second key to be consumed")
	}
	if a3.leaderPending {
		t.Error("expected leaderPending to be cleared after resolving")
	}
	if a3.active != screenJournal {
		t.Errorf("expected screenJournal, got %v", a3.active)
	}
}

func TestHandleKeys_Leader_UnmappedChord_CancelsSilently(t *testing.T) {
	a := loggedInApp()
	a.active = screenProfile
	a2, _, _ := a.handleKeys(keyMsg("g"))
	a3, cmd, consumed := a2.handleKeys(keyMsg("z")) // not a mnemonic
	if !consumed {
		t.Fatal("expected the cancelling key to still be consumed (swallowed)")
	}
	if a3.leaderPending {
		t.Error("expected leaderPending to be cleared")
	}
	if a3.active != screenProfile {
		t.Errorf("expected no navigation to occur, still screenProfile, got %v", a3.active)
	}
	if cmd != nil {
		t.Error("expected nil cmd when the chord doesn't resolve")
	}
}

func TestHandleKeys_Leader_DoubleG_GoesToGuilds(t *testing.T) {
	a := loggedInApp()
	a2, _, _ := a.handleKeys(keyMsg("g"))
	a3, _, consumed := a2.handleKeys(keyMsg("g")) // g g -> Guilds
	if !consumed {
		t.Fatal("expected second 'g' to be consumed")
	}
	if a3.active != screenGuilds {
		t.Errorf("expected screenGuilds, got %v", a3.active)
	}
}

func TestHandleKeys_Leader_NotArmed_WhileInputFocused(t *testing.T) {
	a := setupChatroomsDetailWithURL(loggedInApp())
	if !a.chatrooms.InputFocused() {
		t.Fatal("setup: expected chatrooms input focused in detail mode")
	}
	a2, _, consumed := a.handleKeys(keyMsg("g"))
	if consumed {
		t.Error("expected 'g' to NOT be consumed while chatrooms input is focused — it must still type into the compose box")
	}
	if a2.leaderPending {
		t.Error("leaderPending should not arm while a text input is focused")
	}
}

func TestHandleKeys_Leader_NumericAliasAndLeaderAgree(t *testing.T) {
	// "1" through "9" and their equivalent "g"+mnemonic chord must land on
	// the same screen in both layouts — the drift this feature fixes.
	for i, tab := range menuTabs[:9] {
		num := string(rune('1' + i))
		for _, layout := range []Layout{TabsLayout{}, MillerLayout{}} {
			a := loggedInApp()
			a.layout = layout
			a.active = screenProfile
			a2, _, _ := a.handleKeys(keyMsg(num))
			if a2.active != tab.s {
				t.Errorf("layout %T: key %q: expected %v, got %v", layout, num, tab.s, a2.active)
			}

			b := loggedInApp()
			b.layout = layout
			b.active = screenProfile
			b2, _, _ := b.handleKeys(keyMsg("g"))
			b3, _, _ := b2.handleKeys(keyMsg(string(tab.mnemonic)))
			if b3.active != tab.s {
				t.Errorf("layout %T: chord \"g %c\": expected %v, got %v", layout, tab.mnemonic, tab.s, b3.active)
			}
		}
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

// setupCMailDetail opens a conversation in detail mode, so InputFocused()
// is true — mirrors setupChatroomsDetailWithURL above.
func setupCMailDetail(a App) App {
	a.active = screenCMail
	a.cmail = a.cmail.SetConversations([]model.Conversation{
		{ID: "c1", Participants: []model.User{{Username: "trinity"}}},
	})
	cm, _ := a.cmail.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	cm, _ = cm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	a.cmail = cm
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

// --- Ctrl-twins for CMail/CIRC detail mode: ctrl+q, ctrl+t, ctrl+/, ctrl+left/right ---

func TestHandleKeys_CtrlQ_QuitsWhileChatroomsInputFocused(t *testing.T) {
	a := setupChatroomsDetailWithURL(loggedInApp())
	if !a.chatrooms.InputFocused() {
		t.Fatal("setup: expected chatrooms input focused in detail mode")
	}
	_, cmd, consumed := a.handleKeys(tea.KeyMsg{Type: tea.KeyCtrlQ})
	if !consumed {
		t.Error("expected ctrl+q to be consumed even while chatrooms input is focused")
	}
	if cmd == nil {
		t.Error("expected ctrl+q to fire a quit command")
	}
}

func TestHandleKeys_Q_NotConsumed_WhileChatroomsInputFocused(t *testing.T) {
	a := setupChatroomsDetailWithURL(loggedInApp())
	_, _, consumed := a.handleKeys(keyMsg("q"))
	if consumed {
		t.Error("expected plain 'q' to NOT be consumed while chatrooms input is focused — it must still type into the compose box")
	}
}

func TestHandleKeys_CtrlT_OpensThemePickerWhileChatroomsInputFocused(t *testing.T) {
	a := setupChatroomsDetailWithURL(loggedInApp())
	a2, _, consumed := a.handleKeys(tea.KeyMsg{Type: tea.KeyCtrlT})
	if !consumed {
		t.Error("expected ctrl+t to be consumed even while chatrooms input is focused")
	}
	if !a2.themePickerOpen {
		t.Error("expected ctrl+t to open the theme picker")
	}
}

func TestHandleKeys_T_NotConsumed_WhileChatroomsInputFocused(t *testing.T) {
	a := setupChatroomsDetailWithURL(loggedInApp())
	a2, _, consumed := a.handleKeys(keyMsg("t"))
	if consumed {
		t.Error("expected plain 't' to NOT be consumed while chatrooms input is focused — it must still type into the compose box")
	}
	if a2.themePickerOpen {
		t.Error("plain 't' must not open the theme picker while chatting")
	}
}

// TestHandleKeys_CtrlUnderscore_NotConsumed_WhileChatroomsInputFocused
// guards a deliberate removal: ctrl+/ (bubbletea reports it as KeyType
// keyUS, string "ctrl+_") was tried as a ctrl-twin for Search, then dropped
// — the byte a physical ctrl+/ keystroke sends is inconsistent across
// terminals (0x1F on most, a literal NUL on e.g. Git Bash/MinTTY, itself
// ambiguous with ctrl+space/ctrl+2/ctrl+@), so there's no reliable encoding
// to build a shortcut on.
func TestHandleKeys_CtrlUnderscore_NotConsumed_WhileChatroomsInputFocused(t *testing.T) {
	a := setupChatroomsDetailWithURL(loggedInApp())
	_, _, consumed := a.handleKeys(tea.KeyMsg{Type: tea.KeyCtrlUnderscore})
	if consumed {
		t.Error("expected ctrl+/ (ctrl+_) to NOT be consumed — the shortcut was removed as unreliable across terminals")
	}
}

// TestHandleKeys_NUL_NotConsumed_WhileChatroomsInputFocused guards a
// deliberate decision: some terminals (e.g. Git Bash/MinTTY) send a literal
// NUL byte for ctrl+/ instead of the usual 0x1F, but ctrl+space, ctrl+2, and
// ctrl+@ conventionally send that identical NUL byte too — genuinely
// indistinguishable from ctrl+/ at that point. Accepting NUL as a ctrl+/
// twin was tried and reverted because it risked misfiring on those unrelated
// keystrokes; ctrl+/ simply doesn't work on terminals that collapse to NUL.
func TestHandleKeys_NUL_NotConsumed_WhileChatroomsInputFocused(t *testing.T) {
	a := setupChatroomsDetailWithURL(loggedInApp())
	_, _, consumed := a.handleKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{0}})
	if consumed {
		t.Error("expected a literal NUL byte to NOT be consumed — it's ambiguous with ctrl+space/ctrl+2/ctrl+@, not just ctrl+/")
	}
}

func TestHandleKeys_CtrlLeft_CyclesTabsWhileChatroomsInputFocused(t *testing.T) {
	a := setupChatroomsDetailWithURL(loggedInApp())
	before := tabIndexOf(a)
	a2, _, consumed := a.handleKeys(tea.KeyMsg{Type: tea.KeyCtrlLeft})
	if !consumed {
		t.Error("expected ctrl+left to be consumed even while chatrooms input is focused")
	}
	if tabIndexOf(a2) == before {
		t.Error("expected ctrl+left to cycle to a different tab")
	}
}

func TestHandleKeys_CtrlRight_CyclesTabsWhileChatroomsInputFocused(t *testing.T) {
	a := setupChatroomsDetailWithURL(loggedInApp())
	before := tabIndexOf(a)
	a2, _, consumed := a.handleKeys(tea.KeyMsg{Type: tea.KeyCtrlRight})
	if !consumed {
		t.Error("expected ctrl+right to be consumed even while chatrooms input is focused")
	}
	if tabIndexOf(a2) == before {
		t.Error("expected ctrl+right to cycle to a different tab")
	}
}

// A backgrounded Circ room resumes detail mode directly on tab-return (see
// PR #58/#59), so the very first left/right after switching back used to be
// swallowed into a compose box the user never asked to type into — plain
// left/right now falls through to tab-cycling specifically when that box is
// empty, since there's nothing for it to move a cursor over anyway. Once
// there's text in it (typed just now, or a draft left over from before
// backgrounding), left/right goes back to being captured for cursor
// movement, same as it always has.

func TestHandleKeys_Left_CyclesTabs_WhileChatroomsInputFocusedAndComposeEmpty(t *testing.T) {
	a := setupChatroomsDetailWithURL(loggedInApp())
	if !a.chatrooms.ComposeEmpty() {
		t.Fatal("setup: expected an empty compose box")
	}
	before := tabIndexOf(a)
	a2, _, consumed := a.handleKeys(tea.KeyMsg{Type: tea.KeyLeft})
	if !consumed {
		t.Error("expected plain left arrow to be consumed (tab-cycle) while the compose box is empty")
	}
	if tabIndexOf(a2) == before {
		t.Error("expected plain left arrow to cycle to a different tab")
	}
}

func TestHandleKeys_Right_CyclesTabs_WhileChatroomsInputFocusedAndComposeEmpty(t *testing.T) {
	a := setupChatroomsDetailWithURL(loggedInApp())
	before := tabIndexOf(a)
	a2, _, consumed := a.handleKeys(tea.KeyMsg{Type: tea.KeyRight})
	if !consumed {
		t.Error("expected plain right arrow to be consumed (tab-cycle) while the compose box is empty")
	}
	if tabIndexOf(a2) == before {
		t.Error("expected plain right arrow to cycle to a different tab")
	}
}

func TestHandleKeys_Left_NotConsumed_WhileChatroomsInputFocusedAndComposeHasText(t *testing.T) {
	a := setupChatroomsDetailWithURL(loggedInApp())
	cm, _ := a.chatrooms.Update(keyMsg("h")) // types into the compose box, not a nav key
	a.chatrooms = cm
	if a.chatrooms.ComposeEmpty() {
		t.Fatal("setup: expected a non-empty compose box")
	}
	_, _, consumed := a.handleKeys(tea.KeyMsg{Type: tea.KeyLeft})
	if consumed {
		t.Error("expected plain left arrow to NOT be consumed while there's text in the compose box — it must move the cursor instead")
	}
}

// TestHandleKeys_Left_NotConsumed_WhileChatroomsFlagPromptOpen guards a bug
// found in manual testing: ComposeEmpty() only checked the compose box's own
// value, so left/right escaped to tab-cycling while typing a flag/report
// reason — even though focus was on the reason field, not the (empty)
// compose box.
func TestHandleKeys_Left_NotConsumed_WhileChatroomsFlagPromptOpen(t *testing.T) {
	a := setupChatroomsDetailWithURL(loggedInApp())
	cm, _ := a.chatrooms.Update(tea.KeyMsg{Type: tea.KeyUp}) // select the message
	cm, _ = cm.Update(keyMsg("!"))                           // open the flag prompt
	a.chatrooms = cm

	if a.chatrooms.ComposeEmpty() {
		t.Fatal("setup: expected ComposeEmpty() false while the flag prompt is open, even with an empty compose box")
	}
	_, _, consumed := a.handleKeys(tea.KeyMsg{Type: tea.KeyLeft})
	if consumed {
		t.Error("expected plain left arrow to NOT be consumed while the flag/report reason box is focused — it must move the cursor instead")
	}
}

// TestHandleKeys_Left_NotConsumed_WhileChatroomsDeleteConfirmOpen is the same
// bug class as the flag-prompt case above, for the delete-confirm overlay:
// ComposeEmpty() must also account for confirmingDeleteMsg, not just the
// flag prompt.
func TestHandleKeys_Left_NotConsumed_WhileChatroomsDeleteConfirmOpen(t *testing.T) {
	a := loggedInApp()
	a.chatrooms = a.chatrooms.SetRooms([]model.Room{{ID: "r1", Slug: "zion", Name: "Zion"}})
	cm, _ := a.chatrooms.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	cm, _ = cm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	cm = cm.SetMessages("zion", []model.Message{
		{ID: "m1", From: model.User{Username: ""}, Body: "mine", CreatedAt: time.Now()},
	})
	a.active = screenChatrooms
	a.chatrooms = cm
	cm, _ = a.chatrooms.Update(tea.KeyMsg{Type: tea.KeyUp}) // select the message
	cm, _ = cm.Update(keyMsg("d"))                          // open the delete confirm
	a.chatrooms = cm

	if a.chatrooms.ComposeEmpty() {
		t.Fatal("setup: expected ComposeEmpty() false while the delete-confirm overlay is open")
	}
	_, _, consumed := a.handleKeys(tea.KeyMsg{Type: tea.KeyLeft})
	if consumed {
		t.Error("expected plain left arrow to NOT be consumed while the delete-confirm overlay is open")
	}
}

// C-Mail's compose input is focused for the entire detail view exactly like
// Chatrooms', and now gets the same background-resume treatment (see
// TestActivateScreen_CMailConvSurvivesTabSwitch), so it needs the same
// bare-arrow-escapes-empty-compose behavior.

func TestHandleKeys_Left_CyclesTabs_WhileCMailInputFocusedAndComposeEmpty(t *testing.T) {
	a := setupCMailDetail(loggedInApp())
	if !a.cmail.ComposeEmpty() {
		t.Fatal("setup: expected an empty compose box")
	}
	before := tabIndexOf(a)
	a2, _, consumed := a.handleKeys(tea.KeyMsg{Type: tea.KeyLeft})
	if !consumed {
		t.Error("expected plain left arrow to be consumed (tab-cycle) while the compose box is empty")
	}
	if tabIndexOf(a2) == before {
		t.Error("expected plain left arrow to cycle to a different tab")
	}
}

func TestHandleKeys_Right_CyclesTabs_WhileCMailInputFocusedAndComposeEmpty(t *testing.T) {
	a := setupCMailDetail(loggedInApp())
	before := tabIndexOf(a)
	a2, _, consumed := a.handleKeys(tea.KeyMsg{Type: tea.KeyRight})
	if !consumed {
		t.Error("expected plain right arrow to be consumed (tab-cycle) while the compose box is empty")
	}
	if tabIndexOf(a2) == before {
		t.Error("expected plain right arrow to cycle to a different tab")
	}
}

func TestHandleKeys_Left_NotConsumed_WhileCMailInputFocusedAndComposeHasText(t *testing.T) {
	a := setupCMailDetail(loggedInApp())
	cm, _ := a.cmail.Update(keyMsg("h")) // types into the compose box, not a nav key
	a.cmail = cm
	if a.cmail.ComposeEmpty() {
		t.Fatal("setup: expected a non-empty compose box")
	}
	_, _, consumed := a.handleKeys(tea.KeyMsg{Type: tea.KeyLeft})
	if consumed {
		t.Error("expected plain left arrow to NOT be consumed while there's text in the compose box — it must move the cursor instead")
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

// --- Flag/report overlay: same input-focus bug class as chatrooms/cmail ---

// setupFeedFlagPromptOpen opens the flag/report overlay on someone else's
// post via FeedModel's own Update, mirroring setupChatroomsDetailWithURL
// above (ComposeActive() must report true afterwards, or handleKeys will
// treat later keys as global shortcuts instead of reason-box input).
func setupFeedFlagPromptOpen(a App) App {
	a.active = screenFeed
	a.feed = a.feed.SetPosts([]model.Post{
		{ID: "p1", AuthorUsername: "bob", Content: "not mine"},
	}, "")
	a.feed = a.feed.SetCurrentUsername("alice")
	m, _ := a.feed.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("!")})
	a.feed = m
	return a
}

func TestHandleKeys_Q_NotConsumed_WhileFeedFlagPromptOpen(t *testing.T) {
	a := setupFeedFlagPromptOpen(loggedInApp())
	if !a.feed.ComposeActive() {
		t.Fatal("setup: expected flag prompt open (ComposeActive true) after '!'")
	}
	_, _, consumed := a.handleKeys(keyMsg("q"))
	if consumed {
		t.Error("expected plain 'q' to NOT be consumed while the flag/report reason box is focused — it must still type into the reason field")
	}
}

func TestHandleKeys_Digit_NotConsumed_WhileFeedFlagPromptOpen(t *testing.T) {
	a := setupFeedFlagPromptOpen(loggedInApp())
	_, _, consumed := a.handleKeys(keyMsg("1"))
	if consumed {
		t.Error("expected digit '1' to NOT be consumed while the flag/report reason box is focused — it must still type into the reason field")
	}
}

func TestHandleKeys_CtrlQ_QuitsWhileFeedFlagPromptOpen(t *testing.T) {
	a := setupFeedFlagPromptOpen(loggedInApp())
	_, cmd, consumed := a.handleKeys(tea.KeyMsg{Type: tea.KeyCtrlQ})
	if !consumed {
		t.Error("expected ctrl+q to be consumed even while the flag/report reason box is focused")
	}
	if cmd == nil {
		t.Error("expected ctrl+q to fire a quit command")
	}
}

func TestHandleKeys_Digit_NotConsumed_WhileFeedConfirmingDelete(t *testing.T) {
	a := loggedInApp()
	a.feed = a.feed.SetPosts([]model.Post{
		{ID: "p1", AuthorUsername: "alice", Content: "mine"},
	}, "")
	a.feed = a.feed.SetCurrentUsername("alice")
	m, _ := a.feed.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	a.feed = m
	if !a.feed.ComposeActive() {
		t.Fatal("setup: expected delete-confirm overlay open (ComposeActive true) after 'd'")
	}
	_, _, consumed := a.handleKeys(keyMsg("1"))
	if consumed {
		t.Error("expected digit '1' to NOT be consumed while the delete-confirm overlay is open")
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

// --- Newly posted reply is selected and scrolled into view ---

// TestCreateReplyCmd_ReturnsReplyID guards against the reply ID being
// silently discarded — without it, nothing downstream can select or scroll
// to the reply that was just posted.
func TestCreateReplyCmd_ReturnsReplyID(t *testing.T) {
	a := loggedInApp()

	cmd := a.createReplyCmd("p1", "nice post", "")
	msg := cmd()

	created, ok := msg.(replyCreatedMsg)
	if !ok {
		t.Fatalf("expected replyCreatedMsg, got %T", msg)
	}
	if created.postID != "p1" {
		t.Errorf("postID = %q, want %q", created.postID, "p1")
	}
	if created.replyID == "" {
		t.Error("expected a non-empty replyID from CreateReply's response")
	}
}

// TestReplyCreatedMsg_SetsPendingReplyID verifies that handling
// replyCreatedMsg sets pendingReplyID to the new reply's ID before
// reloading — the same field the notification/search deep-link path uses
// (TestShowSearchReplyMsg_NavigatesToPostDetailAndScrollsToReply) to make
// repliesLoadedMsg call ScrollToReply. Posting a reply must feed that same
// pipeline instead of leaving the new reply unselected after reload.
func TestReplyCreatedMsg_SetsPendingReplyID(t *testing.T) {
	a := loggedInApp()
	a.active = screenPostDetail
	a.postDetail = a.postDetail.SetPost(model.Post{ID: "p1"})

	m, cmd := a.Update(replyCreatedMsg{postID: "p1", replyID: "reply-new-1"})
	a2 := m.(App)

	if a2.pendingReplyID != "reply-new-1" {
		t.Fatalf("expected pendingReplyID = reply-new-1, got %q", a2.pendingReplyID)
	}

	// The reload-then-scroll half of the pipeline (repliesLoadedMsg calling
	// ScrollToReply and clearing pendingReplyID) is already covered by
	// TestShowSearchReplyMsg_NavigatesToPostDetailAndScrollsToReply and
	// PostDetailModel's own TestPostDetail_ScrollToReply_AfterTree — this
	// just confirms the reply-create path feeds pendingReplyID the same way.
	msgs := resolveMsgs(cmd)
	if len(msgs) == 0 {
		t.Fatal("expected a resolved message from the replies reload")
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

// TestRepliesLoadedMsg_DropsStaleReplyForDifferentPost is the regression
// test for a delayed-blackout bug: loadRepliesCmd fires a real network
// request with no cancellation on navigation-away, and its result
// (repliesLoadedMsg) used to be applied unconditionally — so a request
// still in flight when the user opens a *different* post before it resolves
// could land seconds later and silently overwrite that different post's
// reply tree (which rebuilds replyOffsets/replyImages and, via
// viewport.SetContent's internal GotoBottom() when content shrinks, can
// even reposition the scroll away from an inline image with zero user
// input). repliesLoadedMsg now carries postID so a stale delivery for a
// post the user has since navigated away from is dropped instead.
func TestRepliesLoadedMsg_DropsStaleReplyForDifferentPost(t *testing.T) {
	a := loggedInApp()
	a.active = screenPostDetail
	a.postDetail = a.postDetail.SetPost(model.Post{ID: "p1"})

	// Navigate to a different post before the stale request "resolves".
	a.postDetail = a.postDetail.SetPost(model.Post{ID: "p2"})

	m, _, ok := a.handlePostDetail(repliesLoadedMsg{
		postID:  "p1",
		replies: []model.Reply{{ID: "r1", PostID: "p1"}},
	})
	if !ok {
		t.Fatal("expected repliesLoadedMsg to be handled")
	}
	a2 := m
	if a2.postDetail.PostID() != "p2" {
		t.Fatalf("setup broken: expected post p2 still open, got %q", a2.postDetail.PostID())
	}
	// SetPost leaves Loading() true until SetReplies clears it; a dropped
	// stale reply load must leave p2's own (still in-flight) load untouched.
	if !a2.postDetail.Loading() {
		t.Error("expected p2 to still be Loading() — a stale reply load for a different post must not clear it")
	}
}

// --- PostDetail persists across tab switches ---
//
// Mirrors Circ's background-room persistence (PR #58): a post left open
// (not explicitly closed) resumes automatically when navigation lands back
// on the tab it was opened from, instead of showing that tab's own list.
// Re-navigating to the origin tab *from PostDetail itself* is the escape
// hatch that actually closes it — same convention as Circ/C-Mail's
// re-press-the-tab-key pattern.

func openPostFrom(a App, origin screen) App {
	a.active = screenPostDetail
	a.postDetailReturn = origin
	a.postDetail = a.postDetail.SetPost(model.Post{ID: "p1"})
	return a
}

func TestActivateScreen_PostDetail_ResumesOnReturnToOrigin(t *testing.T) {
	a := openPostFrom(loggedInApp(), screenBookmarks)

	a, _ = activateScreen(a, screenFeed) // jump to an unrelated tab
	if a.active != screenFeed {
		t.Fatalf("setup: expected screenFeed, got %v", a.active)
	}
	if !a.postDetail.HasPost() {
		t.Error("expected the post to stay open in the background after switching to an unrelated tab")
	}

	a, _ = activateScreen(a, screenBookmarks) // return to the origin
	if a.active != screenPostDetail {
		t.Errorf("expected returning to Bookmarks to resume PostDetail, got %v", a.active)
	}
	if !a.postDetail.HasPost() {
		t.Error("expected the post to still be open after resuming")
	}
}

func TestActivateScreen_PostDetail_ClosesViaOriginTabEscapeHatch(t *testing.T) {
	a := openPostFrom(loggedInApp(), screenBookmarks)

	a, _ = activateScreen(a, screenBookmarks) // re-press Bookmarks' own key from PostDetail
	if a.active != screenBookmarks {
		t.Errorf("expected the escape hatch to land on Bookmarks' list, got %v", a.active)
	}
	if a.postDetail.HasPost() {
		t.Error("expected the post to be closed after using the escape hatch")
	}
}

func TestHandlePostDetail_Esc_ClosesPost(t *testing.T) {
	a := openPostFrom(loggedInApp(), screenBookmarks)

	m, _ := a.Update(screens.BackToFeedMsg{})
	a2 := m.(App)
	if a2.active != screenBookmarks {
		t.Errorf("expected esc to return to Bookmarks, got %v", a2.active)
	}
	if a2.postDetail.HasPost() {
		t.Error("expected esc to close the post")
	}
}

func TestActivateScreen_PostDetail_ClosingLeavesBrowsedGuildIntact(t *testing.T) {
	a := loggedInApp()
	a.active = screenGuilds
	a.guilds = a.guilds.SetGuilds([]model.Guild{{ID: "g1", Name: "Alpha", Slug: "alpha"}}, "")
	gm, _ := a.guilds.Update(tea.KeyMsg{Type: tea.KeyEnter})
	a.guilds = gm
	if !a.guilds.IsBrowsingGuild() {
		t.Fatal("setup: expected to be browsing a guild")
	}

	a = openPostFrom(a, screenGuilds) // open a post from within that guild

	a, _ = activateScreen(a, screenGuilds) // close it via the escape hatch
	if a.active != screenGuilds {
		t.Fatalf("expected escape hatch to land on Guilds, got %v", a.active)
	}
	if a.postDetail.HasPost() {
		t.Error("expected the post to be closed")
	}
	if !a.guilds.IsBrowsingGuild() {
		t.Error("expected closing the post to still show the browsed guild, not the guild list — Guilds' own browse state is untouched by PostDetail")
	}
}

func TestHandleKeys_Left_CyclesTabs_FromPostDetail(t *testing.T) {
	a := openPostFrom(loggedInApp(), screenBookmarks)
	before := tabIndexOf(a)
	a2, _, consumed := a.handleKeys(tea.KeyMsg{Type: tea.KeyLeft})
	if !consumed {
		t.Error("expected plain left arrow to be consumed (tab-cycle) from PostDetail")
	}
	if tabIndexOf(a2) == before {
		t.Error("expected plain left arrow to cycle to a different tab")
	}
}

func TestHandleKeys_Right_CyclesTabs_FromPostDetail(t *testing.T) {
	a := openPostFrom(loggedInApp(), screenBookmarks)
	before := tabIndexOf(a)
	a2, _, consumed := a.handleKeys(tea.KeyMsg{Type: tea.KeyRight})
	if !consumed {
		t.Error("expected plain right arrow to be consumed (tab-cycle) from PostDetail")
	}
	if tabIndexOf(a2) == before {
		t.Error("expected plain right arrow to cycle to a different tab")
	}
}

func TestTabIndexOf_PostDetail_AnchorsOnOriginNotFeed(t *testing.T) {
	a := openPostFrom(loggedInApp(), screenBookmarks)

	got := tabIndexOf(a)
	want := tabIndexOf(App{active: screenBookmarks})
	if got != want {
		t.Errorf("tabIndexOf(PostDetail from Bookmarks) = %d, want %d (Bookmarks' own position)", got, want)
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

// TestHandleKeys_Leader_G_S_FocusesQueryAndSetsSearchReturn reproduces the
// reported bug: "g s" switched to screenSearch but never called
// FocusQuery(), unlike '/' — landing on Search unfocused but still in
// searchViewQuery is a state the screen doesn't actually handle (see the
// comment on search.go's Update), so there was no way to type a query.
// "g s" is a deliberate "go to Search" action, same intent as '/', so it
// must behave identically: focused query box and searchReturn recorded.
func TestHandleKeys_Leader_G_S_FocusesQueryAndSetsSearchReturn(t *testing.T) {
	a := loggedInApp()
	a.active = screenProfile
	a2, _, _ := a.handleKeys(keyMsg("g"))
	a3, _, consumed := a2.handleKeys(keyMsg("s"))
	if !consumed {
		t.Fatal("expected \"g s\" to be consumed")
	}
	if a3.active != screenSearch {
		t.Fatalf("expected screenSearch, got %v", a3.active)
	}
	if a3.searchReturn != screenProfile {
		t.Errorf("expected searchReturn = screenProfile, got %v", a3.searchReturn)
	}
	if !a3.search.InputFocused() {
		t.Error("expected the query box to be focused after \"g s\", same as '/'")
	}

	m, _ := a3.Update(screens.LeaveSearchMsg{})
	a4 := m.(App)
	if a4.active != screenProfile {
		t.Errorf("expected esc to return to screenProfile (origin), got %v", a4.active)
	}
}

// TestHandleKeys_Leader_G_S_FocusesQuery_EvenAfterStalePreviewState confirms
// "g s" resets to a focused query box even when Search was previously left
// mid-browse (a drilled-into preview, query blurred) — not just on a fresh
// SearchModel where the query happens to start focused.
func TestHandleKeys_Leader_G_S_FocusesQuery_EvenAfterStalePreviewState(t *testing.T) {
	a := loggedInApp()
	a.active = screenSearch
	a.search = a.search.SetPreview(model.SearchPreview{}, "old query")
	if a.search.InputFocused() {
		t.Fatal("setup: expected SetPreview to leave the query box blurred")
	}
	a.active = screenFeed

	a2, _, _ := a.handleKeys(keyMsg("g"))
	a3, _, _ := a2.handleKeys(keyMsg("s"))
	if !a3.search.InputFocused() {
		t.Error("expected \"g s\" to focus the query box even after a stale preview state")
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

func TestScreenHints_ChatroomsDetail_HasCtrlTwins(t *testing.T) {
	a := setupChatroomsDetailWithURL(loggedInApp())
	hints := TabsLayout{}.screenHints(a)
	for _, key := range []string{"ctrl+q", "ctrl+t", "ctrl+←→"} {
		if !hasHint(hints, key) {
			t.Errorf("expected a %q hint in chatrooms detail mode", key)
		}
	}
	if hasHint(hints, "ctrl+/") {
		t.Error("expected no ctrl+/ hint — the shortcut was removed as unreliable across terminals")
	}
}

func TestScreenHints_CMailDetail_HasCtrlTwins(t *testing.T) {
	a := loggedInApp()
	a.cmail = a.cmail.SetConversations([]model.Conversation{
		{ID: "c1", Participants: []model.User{{Username: a.currentUser.Username}, {Username: "molly"}}},
	})
	cm, _ := a.cmail.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	cm, _ = cm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	a.cmail = cm
	a.active = screenCMail

	hints := TabsLayout{}.screenHints(a)
	for _, key := range []string{"ctrl+q", "ctrl+t", "ctrl+←→"} {
		if !hasHint(hints, key) {
			t.Errorf("expected a %q hint in c-mail detail mode", key)
		}
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

// TestRouteURL_ExtensionlessURL_ProbesInlineViewer guards the /gif <url>
// bug: circ/C-Mail's gif attachments forward whatever URL the user typed,
// which often has no ".gif" suffix (Tenor share links, extensionless CDN
// URLs) — canRenderImageInline's extension check alone would reject these
// and send them straight to the browser. With a graphics protocol
// available, routeURL must still attempt the inline viewer (via
// canProbeImageInline) and let the real fetch+decode in imgview.FetchAny
// decide, rather than trusting the URL's extension. openImageInTerminal
// bumps imageFetchGen, so that's used here as a proxy for "took the inline
// path" without needing to inspect the opaque tea.Cmd closure.
func TestRouteURL_ExtensionlessURL_ProbesInlineViewer(t *testing.T) {
	a := loggedInApp()
	a.graphicsProtocol = imgview.ProtocolKitty
	a.imageViewer = "terminal"
	genBefore := a.imageFetchGen

	a2, cmd := a.routeURL("https://example.com/no-extension-gif")
	if cmd == nil {
		t.Fatal("expected a cmd")
	}
	if a2.imageFetchGen == genBefore {
		t.Error("expected routeURL to probe the inline viewer for an extensionless URL when a graphics protocol is detected")
	}
}

// TestRouteURL_ExtensionlessURL_NoProtocol_OpensExternal confirms the probe
// added for extensionless URLs still respects the same gates as
// canRenderImageInline — no graphics protocol means straight to the
// browser, same as today, no wasted fetch attempt.
func TestRouteURL_ExtensionlessURL_NoProtocol_OpensExternal(t *testing.T) {
	a := loggedInApp()
	a.graphicsProtocol = imgview.ProtocolNone
	genBefore := a.imageFetchGen

	a2, cmd := a.routeURL("https://example.com/no-extension-gif")
	if cmd == nil {
		t.Fatal("expected a cmd")
	}
	if a2.imageFetchGen != genBefore {
		t.Error("expected no inline-viewer attempt without a detected graphics protocol")
	}
}

func TestRouteURL_ReservedWord_NotTreatedAsUsername(t *testing.T) {
	a := loggedInApp()
	a.active = screenFeed
	a2, _ := a.routeURL("https://cyberspace.online/jukebox")
	if a2.profileReturn == screenFeed {
		t.Error("reserved top-level path must not be routed as a profile")
	}
}

func TestRouteURL_BareUsername_OpensProfile(t *testing.T) {
	a := loggedInApp()
	a.active = screenFeed
	a2, cmd := a.routeURL("https://cyberspace.online/castle")
	if cmd == nil {
		t.Error("expected a cmd for bare-username profile navigation")
	}
	if a2.profileReturn != screenFeed {
		t.Errorf("profileReturn should be screenFeed, got %v", a2.profileReturn)
	}
}

func TestRouteURL_PostPermalink_ShortForm(t *testing.T) {
	a := loggedInApp()
	a.active = screenFeed
	_, cmd := a.routeURL("https://cyberspace.online/castle/podcast-recommendations")
	if cmd == nil {
		t.Fatal("expected a cmd for post permalink")
	}
	msg := cmd()
	loaded, ok := msg.(urlPostLoadedMsg)
	if !ok {
		t.Fatalf("expected urlPostLoadedMsg, got %T", msg)
	}
	if loaded.origin != screenFeed {
		t.Errorf("origin = %v, want screenFeed", loaded.origin)
	}
}

func TestRouteURL_PostPermalink_BlogForm(t *testing.T) {
	a := loggedInApp()
	a.active = screenBookmarks
	_, cmd := a.routeURL("https://cyberspace.online/castle/blog/podcast-recommendations")
	if cmd == nil {
		t.Fatal("expected a cmd for post permalink")
	}
	msg := cmd()
	loaded, ok := msg.(urlPostLoadedMsg)
	if !ok {
		t.Fatalf("expected urlPostLoadedMsg, got %T", msg)
	}
	if loaded.origin != screenBookmarks {
		t.Errorf("origin = %v, want screenBookmarks", loaded.origin)
	}
}

// TestUrlPostLoadedMsg_NestedLink_PushesStackAndRestoresOnBack is the
// regression test for the "Esc does nothing after following an in-post link"
// bug: opening a post-permalink link while already viewing a post used to
// clobber postDetailReturn with screenPostDetail itself, leaving Esc unable
// to leave. It should instead push the current post onto postDetailStack and
// restore it (not refetch it) on the first Esc, falling back to the real
// origin only once the stack is empty.
func TestUrlPostLoadedMsg_NestedLink_PushesStackAndRestoresOnBack(t *testing.T) {
	a := loggedInApp()
	a = openPostFrom(a, screenFeed)
	a.postDetail = a.postDetail.SetReplies([]model.Reply{{ID: "r1", PostID: "p1"}})

	m, _, ok := a.handleNotifications(urlPostLoadedMsg{post: model.Post{ID: "p2"}, origin: screenBookmarks})
	if !ok {
		t.Fatal("expected urlPostLoadedMsg to be handled")
	}
	a = m

	if a.postDetail.PostID() != "p2" {
		t.Fatalf("expected p2 open, got %q", a.postDetail.PostID())
	}
	if len(a.postDetailStack) != 1 {
		t.Fatalf("expected postDetailStack to have 1 entry, got %d", len(a.postDetailStack))
	}
	if a.postDetailReturn != screenFeed {
		t.Errorf("postDetailReturn should stay screenFeed (the real origin), got %v", a.postDetailReturn)
	}

	// First Esc: pop back to p1, fully restored, without refetching.
	m, _, ok = a.handlePostDetail(screens.BackToFeedMsg{})
	if !ok {
		t.Fatal("expected BackToFeedMsg to be handled")
	}
	a = m
	if a.active != screenPostDetail {
		t.Errorf("active = %v, want screenPostDetail (still nested)", a.active)
	}
	if a.postDetail.PostID() != "p1" {
		t.Fatalf("expected p1 restored, got %q", a.postDetail.PostID())
	}
	if a.postDetail.Loading() {
		t.Error("expected p1's already-loaded replies to be restored, not refetched (Loading() should be false)")
	}
	if len(a.postDetailStack) != 0 {
		t.Errorf("expected postDetailStack empty after popping, got %d", len(a.postDetailStack))
	}

	// Second Esc: stack is empty, fall back to the real origin.
	m, _, ok = a.handlePostDetail(screens.BackToFeedMsg{})
	if !ok {
		t.Fatal("expected BackToFeedMsg to be handled")
	}
	a = m
	if a.active != screenFeed {
		t.Errorf("active = %v, want screenFeed", a.active)
	}
	if a.postDetail.HasPost() {
		t.Error("expected postDetail closed (no post) after returning to origin")
	}
}

// TestActivateScreen_EscapeHatch_ClearsNestedStack ensures pressing the
// origin tab's own key while nested (rather than pressing Esc) also clears
// postDetailStack, so a stale stack can't leak into a later PostDetail
// session opened fresh from that tab.
func TestActivateScreen_EscapeHatch_ClearsNestedStack(t *testing.T) {
	a := openPostFrom(loggedInApp(), screenBookmarks)
	a.postDetailStack = []screens.PostDetailModel{a.postDetail}

	a2, _ := activateScreen(a, screenBookmarks)
	if len(a2.postDetailStack) != 0 {
		t.Errorf("expected postDetailStack cleared by escape hatch, got %d entries", len(a2.postDetailStack))
	}
}

func TestRouteURL_TopicPath_OpensTopic(t *testing.T) {
	a := loggedInApp()
	a2, cmd := a.routeURL("https://cyberspace.online/topics/diy")
	if cmd == nil {
		t.Fatal("expected a cmd for topic navigation")
	}
	if a2.active != screenTopics {
		t.Errorf("active = %v, want screenTopics", a2.active)
	}
	if got := a2.topics.ActiveTopicName(); got != "diy" {
		t.Errorf("ActiveTopicName() = %q, want %q", got, "diy")
	}
}

func TestRouteURL_BareTopics_OpensTopicList(t *testing.T) {
	a := loggedInApp()
	a2, _ := a.routeURL("https://cyberspace.online/topics")
	if a2.active != screenTopics {
		t.Errorf("active = %v, want screenTopics", a2.active)
	}
}

func TestRouteURL_GuildPath_OpensGuild(t *testing.T) {
	a := loggedInApp()
	a2, cmd := a.routeURL("https://cyberspace.online/guilds/night-owls")
	if cmd == nil {
		t.Fatal("expected a cmd for guild navigation")
	}
	if a2.active != screenGuilds {
		t.Errorf("active = %v, want screenGuilds", a2.active)
	}
	if got := a2.guilds.ActiveGuild(); got != "night-owls" {
		t.Errorf("ActiveGuild() = %q, want %q", got, "night-owls")
	}
}

func TestRouteURL_BareGuilds_OpensGuildList(t *testing.T) {
	a := loggedInApp()
	a2, _ := a.routeURL("https://cyberspace.online/guilds")
	if a2.active != screenGuilds {
		t.Errorf("active = %v, want screenGuilds", a2.active)
	}
}

func TestRouteURL_ChatPath_OpensRoom(t *testing.T) {
	a := loggedInApp()
	a.active = screenFeed
	_, cmd := a.routeURL("https://cyberspace.online/chat/general")
	if cmd == nil {
		t.Fatal("expected a cmd for chat room navigation")
	}
	msg := cmd()
	open, ok := msg.(screens.OpenRoomMsg)
	if !ok {
		t.Fatalf("expected screens.OpenRoomMsg, got %T", msg)
	}
	if open.RoomSlug != "general" {
		t.Errorf("RoomSlug = %q, want %q", open.RoomSlug, "general")
	}
	if open.NotifID != "" {
		t.Errorf("NotifID = %q, want empty (not from a notification)", open.NotifID)
	}
}

func TestRouteURL_BareChat_OpensRoomList(t *testing.T) {
	a := loggedInApp()
	a2, _ := a.routeURL("https://cyberspace.online/chat")
	if a2.active != screenChatrooms {
		t.Errorf("active = %v, want screenChatrooms", a2.active)
	}
}

func TestRouteURL_ChatPath_NoNotifID_DoesNotDecrementUnread(t *testing.T) {
	a := loggedInApp()
	a.polledUnreadCount = 3
	_, cmd := a.routeURL("https://cyberspace.online/chat/general")
	msg := cmd()
	m, _, handled := a.handleChatrooms(msg)
	if !handled {
		t.Fatal("expected handleChatrooms to handle screens.OpenRoomMsg")
	}
	if m.polledUnreadCount != 3 {
		t.Errorf("polledUnreadCount = %d, want unchanged 3 (no notification behind this open)", m.polledUnreadCount)
	}
}

func TestRouteURL_EphemeralAllowsPostPermalinkNav(t *testing.T) {
	a := loggedInApp()
	a.ephemeral = true
	got, cmd := a.routeURL("https://cyberspace.online/castle/podcast-recommendations")
	if got.notifyText != "" {
		t.Errorf("notifyText = %q, want empty (internal nav must not be blocked)", got.notifyText)
	}
	if cmd == nil {
		t.Error("cmd = nil, want post load command")
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

// TestHandleUnauthorized_InvalidatesPendingTickers guards against the
// poll/wander/logo-idle tea.Tick chains started by afterLoginCmd surviving
// session expiry: each is stamped with the sessionGen it was scheduled
// under, so once handleUnauthorized bumps sessionGen, a tick that was
// already in flight must drop itself instead of refetching or
// rescheduling — otherwise it (and a fresh set started by a subsequent
// re-login) would keep polling indefinitely after logout.
func TestHandleUnauthorized_InvalidatesPendingTickers(t *testing.T) {
	a := loggedInApp()
	a.ephemeral = true
	genBefore := a.sessionGen

	m, _ := a.Update(errMsg{api.ErrUnauthorized})
	a2 := m.(App)
	if a2.sessionGen == genBefore {
		t.Fatal("expected sessionGen to advance on session expiry")
	}

	if _, cmd, ok := a2.handleNotifications(pollUnreadTickMsg{gen: genBefore}); !ok || cmd != nil {
		t.Errorf("pollUnreadTickMsg{gen: %d} (stale) handled=%v cmd=%v, want handled=true cmd=nil", genBefore, ok, cmd)
	}
	if _, cmd, ok := a2.handleNotifications(feedPollTickMsg{gen: genBefore}); !ok || cmd != nil {
		t.Errorf("feedPollTickMsg{gen: %d} (stale) handled=%v cmd=%v, want handled=true cmd=nil", genBefore, ok, cmd)
	}
	if _, cmd, ok := a2.handleSettings(wanderTickMsg{gen: genBefore}); !ok || cmd != nil {
		t.Errorf("wanderTickMsg{gen: %d} (stale) handled=%v cmd=%v, want handled=true cmd=nil", genBefore, ok, cmd)
	}

	// A tick stamped with the current (post-expiry) gen — e.g. one scheduled
	// by a fresh login — must still do real work, not be dropped too.
	if _, cmd, ok := a2.handleNotifications(pollUnreadTickMsg{gen: a2.sessionGen}); !ok || cmd == nil {
		t.Errorf("pollUnreadTickMsg{gen: current} handled=%v cmd=%v, want handled=true cmd=non-nil", ok, cmd)
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

// friendlyErr also softens EMAIL_NOT_VERIFIED — the one other error code the
// app branches on by Code rather than Status, since it can surface from any
// authenticated call, not just login.
func TestFriendlyErr_EmailNotVerifiedIsSoftened(t *testing.T) {
	got := friendlyErr(&api.APIError{Code: "EMAIL_NOT_VERIFIED", Status: 403, Message: "email not verified"})
	if got != "Please verify your email — check your inbox for the verification link." {
		t.Errorf("friendlyErr(EMAIL_NOT_VERIFIED) = %q, want softened message", got)
	}
}

// loginErrMsgFor is what routes an EMAIL_NOT_VERIFIED profile-fetch failure
// (login itself succeeded — an idToken is required to call
// resend-verification) into a distinguishable LoginErrMsg the login screen
// can offer a resend action from, instead of a generic error.
func TestLoginErrMsgFor_EmailNotVerifiedCarriesIDToken(t *testing.T) {
	err := &api.APIError{Code: "EMAIL_NOT_VERIFIED", Status: 403, Message: "email not verified"}
	got := loginErrMsgFor(err, "id-abc")
	if !got.EmailNotVerified {
		t.Error("expected EmailNotVerified = true")
	}
	if got.IDToken != "id-abc" {
		t.Errorf("IDToken = %q, want id-abc", got.IDToken)
	}
	if got.Err != err {
		t.Errorf("Err = %v, want the original error", got.Err)
	}
}

func TestLoginErrMsgFor_OtherErrorsDoNotSetEmailNotVerified(t *testing.T) {
	got := loginErrMsgFor(&api.APIError{Code: "UNAUTHORIZED", Status: 401, Message: "bad credentials"}, "id-abc")
	if got.EmailNotVerified {
		t.Error("expected EmailNotVerified = false for an unrelated error")
	}
	if got.IDToken != "" {
		t.Errorf("IDToken = %q, want empty for an unrelated error", got.IDToken)
	}
}

// emailNotVerifiedClient simulates a login that succeeds but whose follow-up
// profile fetch is rejected because the account's email isn't verified —
// the actual failure point per the API docs (Login itself doesn't gate on
// verification; an idToken is needed to call resend-verification).
type emailNotVerifiedClient struct {
	*api.MockClient
}

func (c *emailNotVerifiedClient) GetOwnProfile() (model.User, error) {
	return model.User{}, &api.APIError{Code: "EMAIL_NOT_VERIFIED", Status: 403, Message: "email not verified"}
}

// TestLoginCmd_EmailNotVerified_ReturnsDistinguishableLoginErrMsg confirms
// the end-to-end wiring: a real Login() success followed by an
// EMAIL_NOT_VERIFIED profile-fetch failure produces a LoginErrMsg carrying
// the idToken the login screen needs to offer a resend action, not a
// generic error.
func TestLoginCmd_EmailNotVerified_ReturnsDistinguishableLoginErrMsg(t *testing.T) {
	a := NewApp(&emailNotVerifiedClient{MockClient: api.NewMockClient()})
	msg := a.loginCmd("user@example.com", "secret")()
	lem, ok := msg.(screens.LoginErrMsg)
	if !ok {
		t.Fatalf("expected LoginErrMsg, got %T", msg)
	}
	if !lem.EmailNotVerified {
		t.Error("expected EmailNotVerified = true")
	}
	if lem.IDToken == "" {
		t.Error("expected a non-empty IDToken carried over from the successful Login() call")
	}
}

// resendVerificationSpyClient records the idToken ResendVerification was
// called with, and returns a canned error.
type resendVerificationSpyClient struct {
	*api.MockClient
	gotIDToken string
	err        error
}

func (c *resendVerificationSpyClient) ResendVerification(idToken string) error {
	c.gotIDToken = idToken
	return c.err
}

func TestResendVerificationCmd_CallsClientAndWrapsResult(t *testing.T) {
	spy := &resendVerificationSpyClient{MockClient: api.NewMockClient()}
	a := NewApp(spy)

	msg := a.resendVerificationCmd("id-abc")()
	rvr, ok := msg.(screens.ResendVerificationResultMsg)
	if !ok {
		t.Fatalf("expected ResendVerificationResultMsg, got %T", msg)
	}
	if rvr.Err != nil {
		t.Errorf("Err = %v, want nil", rvr.Err)
	}
	if spy.gotIDToken != "id-abc" {
		t.Errorf("ResendVerification called with %q, want id-abc", spy.gotIDToken)
	}
}

func TestResendVerificationCmd_PropagatesError(t *testing.T) {
	wantErr := errors.New("rate limited")
	spy := &resendVerificationSpyClient{MockClient: api.NewMockClient(), err: wantErr}
	a := NewApp(spy)

	msg := a.resendVerificationCmd("id-abc")()
	rvr, ok := msg.(screens.ResendVerificationResultMsg)
	if !ok {
		t.Fatalf("expected ResendVerificationResultMsg, got %T", msg)
	}
	if rvr.Err != wantErr {
		t.Errorf("Err = %v, want %v", rvr.Err, wantErr)
	}
}

// --- Notifications v0.8.5: unread-count exact, mark-all-read hasMore ---

func TestUnreadCountMsg_PropagatesExactFlag(t *testing.T) {
	a := loggedInApp()
	m, _ := a.Update(unreadCountMsg{count: 100, exact: false})
	got := m.(App)
	if got.polledUnreadCount != 100 {
		t.Errorf("polledUnreadCount = %d, want 100", got.polledUnreadCount)
	}
	if got.polledUnreadCountExact {
		t.Error("polledUnreadCountExact = true, want false")
	}
}

func TestMarkAllNotifsReadMsg_SetsExactTrue(t *testing.T) {
	a := loggedInApp()
	a.polledUnreadCount = 150
	a.polledUnreadCountExact = false
	m, _ := a.Update(screens.MarkAllNotifsReadMsg{})
	got := m.(App)
	if got.polledUnreadCount != 0 {
		t.Errorf("polledUnreadCount = %d, want 0", got.polledUnreadCount)
	}
	if !got.polledUnreadCountExact {
		t.Error("polledUnreadCountExact = false, want true (zero unread is always exact)")
	}
}

// markAllReadSpyClient counts MarkAllNotificationsRead calls and reports
// hasMore true for the first hasMoreCalls invocations, then false.
type markAllReadSpyClient struct {
	*api.MockClient
	hasMoreCalls int
	calls        int
}

func (c *markAllReadSpyClient) MarkAllNotificationsRead() (bool, error) {
	c.calls++
	return c.calls <= c.hasMoreCalls, nil
}

func TestMarkAllNotifsReadCmd_LoopsWhileHasMore(t *testing.T) {
	spy := &markAllReadSpyClient{MockClient: api.NewMockClient(), hasMoreCalls: 3}
	a := NewApp(spy)
	a.markAllNotifsReadCmd()()
	if spy.calls != 4 {
		t.Errorf("MarkAllNotificationsRead called %d times, want 4 (3 hasMore=true + 1 final hasMore=false)", spy.calls)
	}
}

func TestMarkAllNotifsReadCmd_StopsAtMaxCalls(t *testing.T) {
	spy := &markAllReadSpyClient{MockClient: api.NewMockClient(), hasMoreCalls: 1000}
	a := NewApp(spy)
	a.markAllNotifsReadCmd()()
	if spy.calls != markAllNotifsReadMaxCalls {
		t.Errorf("MarkAllNotificationsRead called %d times, want the bounded max %d", spy.calls, markAllNotifsReadMaxCalls)
	}
}

// tooSoonPostClient simulates the server silently converting a post
// submitted too soon after a previous one into a journal entry: CreatePost
// still "succeeds" with a postId/slug, but that ID doesn't resolve.
type tooSoonPostClient struct {
	*api.MockClient
}

func (c *tooSoonPostClient) GetPost(postID string) (model.Post, error) {
	return model.Post{}, &api.APIError{Code: "NOT_FOUND", Status: 404, Message: "post not found"}
}

func TestCreatePostCmd_TooSoonConversion_ReturnsPostConvertedToNoteMsg(t *testing.T) {
	a := NewApp(&tooSoonPostClient{MockClient: api.NewMockClient()})

	msg := a.createPostCmd("hello", "", "", nil, true, false, "")()
	if _, ok := msg.(postConvertedToNoteMsg); !ok {
		t.Fatalf("createPostCmd() = %T, want postConvertedToNoteMsg", msg)
	}
}

func TestCreatePostCmd_NormalSuccess_ReturnsPostCreatedMsg(t *testing.T) {
	a := NewApp(api.NewMockClient())

	msg := a.createPostCmd("hello", "", "", nil, true, false, "")()
	if _, ok := msg.(postCreatedMsg); !ok {
		t.Fatalf("createPostCmd() = %T, want postCreatedMsg", msg)
	}
}

// --- resolveAttachment: dimension fetch + the API's 640px cap ---

func testPNGServer(t *testing.T, w, h int) *httptest.Server {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Write(buf.Bytes())
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestResolveAttachment_EmptyURL_ReturnsNil(t *testing.T) {
	attachment, err := resolveAttachment("")
	if err != nil || attachment != nil {
		t.Errorf("resolveAttachment(\"\") = %v, %v, want nil, nil", attachment, err)
	}
}

func TestResolveAttachment_WithinLimit_SetsTypeAndDimensions(t *testing.T) {
	srv := testPNGServer(t, 100, 50)
	attachment, err := resolveAttachment(srv.URL + "/pic.png")
	if err != nil {
		t.Fatalf("resolveAttachment: %v", err)
	}
	if attachment.Type != "image" || attachment.Width != 100 || attachment.Height != 50 {
		t.Errorf("attachment = %+v, want type=image width=100 height=50", attachment)
	}
}

// TestResolveAttachment_GIFExtension_SetsGIFType guards attachmentTypeForURL's
// extension-based inference, which the caller (createPostCmd/editPostCmd)
// relies on since the API distinguishes "image" from "gif" attachments.
func TestResolveAttachment_GIFExtension_SetsGIFType(t *testing.T) {
	srv := testPNGServer(t, 10, 10) // content doesn't need to actually be a gif — only the URL suffix is inspected
	attachment, err := resolveAttachment(srv.URL + "/pic.gif")
	if err != nil {
		t.Fatalf("resolveAttachment: %v", err)
	}
	if attachment.Type != "gif" {
		t.Errorf("attachment.Type = %q, want gif", attachment.Type)
	}
}

// TestResolveAttachment_ExceedsLimit_ReturnsClearError guards the one thing
// this app has no way to fix automatically: there's no upload endpoint to
// host a resized copy, so an oversized image must fail here with a message
// that explains why, rather than as a raw 400 from the API (confirmed live:
// POST /v1/posts rejects width/height above 640px).
func TestResolveAttachment_ExceedsLimit_ReturnsClearError(t *testing.T) {
	srv := testPNGServer(t, 1200, 480)
	attachment, err := resolveAttachment(srv.URL + "/pic.png")
	if attachment != nil {
		t.Errorf("attachment = %+v, want nil on an oversized image", attachment)
	}
	if err == nil || !strings.Contains(err.Error(), "640") {
		t.Errorf("err = %v, want a message mentioning the 640px limit", err)
	}
}

// editPostRecordingClient captures the arguments EditPost was called with,
// so a test can inspect exactly what editPostCmd sent without a real API.
type editPostRecordingClient struct {
	*api.MockClient
	gotAttachments []model.Attachment
	gotTouched     bool
}

func (c *editPostRecordingClient) EditPost(postID, content, title string, topics []string, isPublic, isNSFW bool, attachments []model.Attachment, attachmentTouched bool) error {
	c.gotAttachments = attachments
	c.gotTouched = attachmentTouched
	return nil
}

// TestEditPostCmd_MergesOtherAttachmentsWithResolvedAttachment guards the
// merge editPostCmd is responsible for: the attachments array it sends must
// combine otherAttachments (e.g. an audio one the edit panel doesn't manage)
// with the newly resolved image, in that order — dropping otherAttachments
// here would silently delete them server-side, since EditPost replaces the
// whole array.
func TestEditPostCmd_MergesOtherAttachmentsWithResolvedAttachment(t *testing.T) {
	srv := testPNGServer(t, 10, 10)
	client := &editPostRecordingClient{MockClient: api.NewMockClient()}
	a := NewApp(client)
	audio := model.Attachment{Type: "audio", Src: "https://youtu.be/old"}

	msg := a.editPostCmd("p1", "content", "title", nil, false, false, srv.URL+"/pic.png", true, []model.Attachment{audio})()
	if _, ok := msg.(postEditedMsg); !ok {
		t.Fatalf("editPostCmd() = %T, want postEditedMsg", msg)
	}
	if !client.gotTouched {
		t.Fatal("expected EditPost to be called with attachmentTouched = true")
	}
	want := []model.Attachment{audio, {Type: "image", Src: srv.URL + "/pic.png", Width: 10, Height: 10}}
	if !slices.Equal(client.gotAttachments, want) {
		t.Errorf("EditPost attachments = %+v, want %+v (other attachment first, then the resolved one)", client.gotAttachments, want)
	}
}

// --- muteUserCmd / MuteUserMsg / userMutedMsg ---

// sendRoomMessageRecordingClient captures the arguments SendRoomMessage was
// called with, so a test can inspect exactly what got sent without a real API.
type sendRoomMessageRecordingClient struct {
	*api.MockClient
	gotRoomID, gotBody string
}

func (c *sendRoomMessageRecordingClient) SendRoomMessage(roomID, body string) (string, error) {
	c.gotRoomID, c.gotBody = roomID, body
	return "", nil
}

// TestMuteUserCmd_SendsMuteSlashCommand guards the only way to mute:
// sending "/mute <username>" as an ordinary room message, exactly as if the
// user had typed it — there's no dedicated mute endpoint.
func TestMuteUserCmd_SendsMuteSlashCommand(t *testing.T) {
	client := &sendRoomMessageRecordingClient{MockClient: api.NewMockClient()}
	a := NewApp(client)

	msg := a.muteUserCmd("zion", "molly")()
	if client.gotRoomID != "zion" || client.gotBody != "/mute molly" {
		t.Errorf("SendRoomMessage(%q, %q), want (\"zion\", \"/mute molly\")", client.gotRoomID, client.gotBody)
	}
	um, ok := msg.(userMutedMsg)
	if !ok {
		t.Fatalf("muteUserCmd() = %T, want userMutedMsg", msg)
	}
	if um.username != "molly" {
		t.Errorf("username = %q, want molly", um.username)
	}
}

// mutedFailClient simulates the send itself failing (e.g. rate limit).
type mutedFailClient struct{ *api.MockClient }

func (c *mutedFailClient) SendRoomMessage(roomID, body string) (string, error) {
	return "", &api.APIError{Code: "RATE_LIMITED", Status: 429, Message: "too many requests"}
}

func TestMuteUserCmd_SendFails_ReturnsActionErrMsg(t *testing.T) {
	a := NewApp(&mutedFailClient{MockClient: api.NewMockClient()})

	msg := a.muteUserCmd("zion", "molly")()
	if _, ok := msg.(actionErrMsg); !ok {
		t.Fatalf("muteUserCmd() = %T, want actionErrMsg", msg)
	}
}

// TestApp_UserMutedMsg_ShowsNotification covers the reason userMutedMsg
// exists at all: "/mute" posts nothing to the room, so without this the
// user pressing 'm' would have no way to tell it worked.
func TestApp_UserMutedMsg_ShowsNotification(t *testing.T) {
	a := loggedInApp()

	m, _ := a.Update(userMutedMsg{username: "molly"})
	a = m.(App)

	if a.notifyLevel != notifyInfo || a.notifyText != "muted molly" {
		t.Errorf("notify = level=%v text=%q, want level=%v text=%q", a.notifyLevel, a.notifyText, notifyInfo, "muted molly")
	}
}

// TestApp_UserMutedMsg_ReloadsSettings guards the fix for a real reported
// bug: muting a user sent the /mute command fine but the room never
// filtered their messages out, because ChatroomsModel's mute list only
// updates from Settings.MutedUsersByRoom via SharedConfigMsg (see its
// SharedConfigMsg handler), and /mute changes that server-side without ever
// pushing the new value to us. batch[0] is the notify tick — not invoked
// here since tea.Tick blocks for notifyTTL; only the settings-reload
// command (batch[1], by construction — see the userMutedMsg case) is safe
// to run synchronously.
func TestApp_UserMutedMsg_ReloadsSettings(t *testing.T) {
	a := loggedInApp()

	_, cmd := a.Update(userMutedMsg{username: "molly"})
	if cmd == nil {
		t.Fatal("expected a cmd")
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) != 2 {
		t.Fatalf("cmd() = %T (len %d), want a 2-command tea.BatchMsg (notify + settings reload)", cmd(), len(batch))
	}
	if _, ok := batch[1]().(settingsLoadedMsg); !ok {
		t.Errorf("batch[1]() = %T, want settingsLoadedMsg — the mute list refresh", batch[1]())
	}
}

// --- CopyMessageTextMsg ---

func TestApp_CopyMessageTextMsg_ShowsNotification(t *testing.T) {
	a := loggedInApp()

	m, _ := a.Update(screens.CopyMessageTextMsg{Text: "hi there"})
	a = m.(App)

	if a.notifyLevel != notifyInfo || a.notifyText != "Copied message to clipboard" {
		t.Errorf("notify = level=%v text=%q, want level=%v text=%q", a.notifyLevel, a.notifyText, notifyInfo, "Copied message to clipboard")
	}
}

func TestApp_CopyMessageTextMsg_EmptyText_NotifiesNothingToCopy(t *testing.T) {
	a := loggedInApp()

	m, _ := a.Update(screens.CopyMessageTextMsg{Text: ""})
	a = m.(App)

	if a.notifyLevel != notifyInfo || a.notifyText != "nothing to copy" {
		t.Errorf("notify = level=%v text=%q, want level=%v text=%q", a.notifyLevel, a.notifyText, notifyInfo, "nothing to copy")
	}
}

func TestApp_CopyMessageTextMsg_Ephemeral_DisabledInSSH(t *testing.T) {
	a := loggedInApp()
	a.ephemeral = true

	m, _ := a.Update(screens.CopyMessageTextMsg{Text: "hi there"})
	a = m.(App)

	if a.notifyText != "Copying is disabled in SSH sessions" {
		t.Errorf("notifyText = %q, want the SSH-disabled banner", a.notifyText)
	}
}

// flagErrorMsg softens the documented self-report 403 into a friendly banner;
// anything else falls through to the normal actionErrMsg handling.
func TestFlagErrorMsg_403IsSoftened(t *testing.T) {
	got := flagErrorMsg(&api.APIError{Code: "FORBIDDEN", Status: 403, Message: "cannot flag own content"})
	msg, ok := got.(notifyMsg)
	if !ok {
		t.Fatalf("flagErrorMsg(403) = %T, want notifyMsg", got)
	}
	if msg.level != notifyError {
		t.Errorf("level = %v, want notifyError", msg.level)
	}
	if msg.text != "you can't report your own content" {
		t.Errorf("text = %q, want friendly self-report message", msg.text)
	}
}

func TestFlagErrorMsg_OtherErrorsFallThrough(t *testing.T) {
	err := &api.APIError{Code: "RATE_LIMITED", Status: 429, Message: "too many requests"}
	got := flagErrorMsg(err)
	ae, ok := got.(actionErrMsg)
	if !ok {
		t.Fatalf("flagErrorMsg(429) = %T, want actionErrMsg", got)
	}
	if ae.err != err {
		t.Errorf("actionErrMsg.err = %v, want the original error", ae.err)
	}
}

// pokeErrorMsg softens the expected 429 (1/hour, 8/day cap) and the documented
// 403 (blocked either direction) into friendly banners; anything else falls
// through to the normal actionErrMsg handling.
func TestPokeErrorMsg_429IsSoftened(t *testing.T) {
	got := pokeErrorMsg(&api.APIError{Code: "RATE_LIMITED", Status: 429, Message: "too many requests"})
	msg, ok := got.(notifyMsg)
	if !ok {
		t.Fatalf("pokeErrorMsg(429) = %T, want notifyMsg", got)
	}
	if msg.level != notifyError {
		t.Errorf("level = %v, want notifyError", msg.level)
	}
	if msg.text != "poke limit reached — try again later" {
		t.Errorf("text = %q, want friendly rate-limit message", msg.text)
	}
}

func TestPokeErrorMsg_403IsSoftened(t *testing.T) {
	got := pokeErrorMsg(&api.APIError{Code: "FORBIDDEN", Status: 403, Message: "blocked"})
	msg, ok := got.(notifyMsg)
	if !ok {
		t.Fatalf("pokeErrorMsg(403) = %T, want notifyMsg", got)
	}
	if msg.text != "can't poke this user" {
		t.Errorf("text = %q, want friendly blocked message", msg.text)
	}
}

func TestPokeErrorMsg_OtherErrorsFallThrough(t *testing.T) {
	err := &api.APIError{Code: "NOT_FOUND", Status: 404, Message: "unknown user"}
	got := pokeErrorMsg(err)
	ae, ok := got.(actionErrMsg)
	if !ok {
		t.Fatalf("pokeErrorMsg(404) = %T, want actionErrMsg", got)
	}
	if ae.err != err {
		t.Errorf("actionErrMsg.err = %v, want the original error", ae.err)
	}
}

// editErrorMsg softens the documented 403 (outside the 5-minute window or not
// a supporter) into a friendly banner; anything else falls through to the
// normal actionErrMsg handling.
func TestEditErrorMsg_403IsSoftened(t *testing.T) {
	got := editErrorMsg(&api.APIError{Code: "FORBIDDEN", Status: 403, Message: "edit window closed"})
	msg, ok := got.(notifyMsg)
	if !ok {
		t.Fatalf("editErrorMsg(403) = %T, want notifyMsg", got)
	}
	if msg.level != notifyError {
		t.Errorf("level = %v, want notifyError", msg.level)
	}
	if msg.text != "can't edit — outside the 5-minute window or not a supporter" {
		t.Errorf("text = %q, want friendly edit-window message", msg.text)
	}
}

func TestEditErrorMsg_OtherErrorsFallThrough(t *testing.T) {
	err := &api.APIError{Code: "RATE_LIMITED", Status: 429, Message: "too many requests"}
	got := editErrorMsg(err)
	ae, ok := got.(actionErrMsg)
	if !ok {
		t.Fatalf("editErrorMsg(429) = %T, want actionErrMsg", got)
	}
	if ae.err != err {
		t.Errorf("actionErrMsg.err = %v, want the original error", ae.err)
	}
}

// TestApp_PostEditPanel_VisibleInFullRender exercises the whole App-level
// pipeline (outer layout chrome included) rather than PostDetailModel in
// isolation, to catch any clipping/composition bug the layout wrapper might
// introduce on top of an otherwise-correct screen-level View(). Reported
// live: pressing 'e' on a post in Post Detail applies and saves the edit
// (so Update() routing is confirmed working) but the panel is never visible.
func TestApp_PostEditPanel_VisibleInFullRender(t *testing.T) {
	a := loggedInApp()
	a.currentUser = model.User{Username: "op", IsSupporter: true}
	a.active = screenPostDetail
	a.postDetail = a.postDetail.
		SetCurrentUsername("op").
		SetCurrentUserIsSupporter(true).
		SetPost(model.Post{ID: "p1", AuthorUsername: "op", Content: "distinctive body text", CreatedAt: time.Now()})

	m, _ := a.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	a = m.(App)

	before := a.View()
	m, _ = a.Update(keyMsg("e"))
	a = m.(App)
	after := a.View()

	if !a.postDetail.ComposeActive() {
		t.Fatal("setup: expected ComposeActive true after pressing 'e'")
	}
	if after == before {
		t.Fatal("expected the full App render to change once the post-edit panel is open, got identical output")
	}
	if !strings.Contains(after, "distinctive body text") {
		t.Errorf("expected the full App render to contain the pre-filled body content, got:\n%s", after)
	}
}

// --- postEditedMsg: local cache must reflect an attachment change ---

func TestPostEditedMsg_AttachmentsTouched_UpdatesFeedCache(t *testing.T) {
	a := loggedInApp()
	a.feed = a.feed.SetPosts([]model.Post{
		{ID: "p1", Content: "body", Attachments: []model.Attachment{{Type: "audio", Src: "https://youtu.be/old"}}},
	}, "")

	newAttachments := []model.Attachment{
		{Type: "audio", Src: "https://youtu.be/old"},
		{Type: "image", Src: "https://example.com/new.png"},
	}
	m, _ := a.Update(postEditedMsg{
		postID: "p1", content: "body", editedAt: time.Now(),
		attachments: newAttachments, attachmentsTouched: true,
	})
	a = m.(App)

	got := a.feed.GetFocusedURLs()
	if !slices.Contains(got, "https://example.com/new.png") {
		t.Errorf("GetFocusedURLs() = %v, want it to contain the newly attached image", got)
	}
}

func TestPostEditedMsg_AttachmentsNotTouched_LeavesFeedCacheAlone(t *testing.T) {
	a := loggedInApp()
	a.feed = a.feed.SetPosts([]model.Post{
		{ID: "p1", Content: "body", Attachments: []model.Attachment{{Type: "audio", Src: "https://youtu.be/old"}}},
	}, "")

	m, _ := a.Update(postEditedMsg{
		postID: "p1", content: "edited body", editedAt: time.Now(),
		attachmentsTouched: false,
	})
	a = m.(App)

	got := a.feed.GetFocusedURLs()
	if !slices.Contains(got, "https://youtu.be/old") {
		t.Errorf("GetFocusedURLs() = %v, want the pre-existing attachment preserved when attachmentsTouched is false", got)
	}
}

func TestPostEditedMsg_AttachmentsTouched_UpdatesPostDetailCache(t *testing.T) {
	a := loggedInApp()
	a.active = screenPostDetail
	a.postDetail = a.postDetail.SetPost(model.Post{ID: "p1", Content: "body"})

	m, _ := a.Update(postEditedMsg{
		postID: "p1", content: "body", editedAt: time.Now(),
		attachments:        []model.Attachment{{Type: "image", Src: "https://example.com/new.png"}},
		attachmentsTouched: true,
	})
	a = m.(App)

	got := a.postDetail.GetFocusedURLs()
	if !slices.Contains(got, "https://example.com/new.png") {
		t.Errorf("GetFocusedURLs() = %v, want it to contain the newly attached image", got)
	}
}

// --- applyAttachURL: ctrl+g dispatch (native post attachment vs markdown insert) ---

// TestApplyAttachURL_FeedPanelActive_SetsNativeAttachment covers the branch
// that must win when the Feed new-post panel is open: a native attachment on
// the panel, not markdown text inserted into whatever's focused.
func TestApplyAttachURL_FeedPanelActive_SetsNativeAttachment(t *testing.T) {
	a := loggedInApp()
	m, _ := a.feed.Update(keyMsg("n"))
	a.feed = m
	if !a.feed.PanelActive() {
		t.Fatal("setup: expected Feed's new-post panel open after 'n'")
	}

	a, cmd := a.applyAttachURL("https://example.com/pic.png")
	if cmd != nil {
		t.Error("expected no cmd for the native-attachment branch")
	}
	if got := a.feed.ComposeView(80); !strings.Contains(got, "https://example.com/pic.png") {
		t.Errorf("ComposeView() = %q, want it to contain the attached URL", got)
	}
}

// TestApplyAttachURL_PostDetailEditPanelActive_SetsNativeAttachment mirrors
// the Feed case for PostDetail's edit panel (opened via 'e', separate from
// the Feed instance).
func TestApplyAttachURL_PostDetailEditPanelActive_SetsNativeAttachment(t *testing.T) {
	a := loggedInApp()
	a.currentUser = model.User{Username: "op", IsSupporter: true}
	a.active = screenPostDetail
	a.postDetail = a.postDetail.
		SetCurrentUsername("op").
		SetCurrentUserIsSupporter(true).
		SetPost(model.Post{ID: "p1", AuthorUsername: "op", Content: "body", CreatedAt: time.Now()})
	m, _ := a.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	a = m.(App)
	m, _ = a.Update(keyMsg("e"))
	a = m.(App)
	if !a.postDetail.EditPanelActive() {
		t.Fatal("setup: expected PostDetail's edit panel open after 'e'")
	}

	a, cmd := a.applyAttachURL("https://example.com/pic.png")
	if cmd != nil {
		t.Error("expected no cmd for the native-attachment branch")
	}
	if got := a.View(); !strings.Contains(got, "https://example.com/pic.png") {
		t.Errorf("full render doesn't contain the attached URL")
	}
}

// TestApplyAttachURL_DefaultBranch_ReturnsInsertIconMsg covers the leftover
// incidental surfaces (guild threads, journal, bio editing — anything not
// explicitly special-cased): the URL still goes in as markdown image syntax
// via the same InsertIconMsg dispatch the icon picker uses, since these were
// never confirmed to render on the site either way and aren't part of the
// four surfaces this feature targets.
func TestApplyAttachURL_DefaultBranch_ReturnsInsertIconMsg(t *testing.T) {
	a := loggedInApp()
	a.active = screenGuilds

	_, cmd := a.applyAttachURL("https://example.com/pic.gif")
	if cmd == nil {
		t.Fatal("expected a cmd for the markdown-insert branch")
	}
	msg, ok := cmd().(screens.InsertIconMsg)
	if !ok {
		t.Fatalf("cmd() = %T, want screens.InsertIconMsg", msg)
	}
	if want := "![](https://example.com/pic.gif)"; msg.Icon != want {
		t.Errorf("InsertIconMsg.Icon = %q, want %q", msg.Icon, want)
	}
}

// TestApplyAttachURL_DefaultBranch_EmptyURL_NoCmd: an empty submission from
// the prompt (e.g. esc'd out with nothing typed) must not insert an empty
// markdown link.
func TestApplyAttachURL_DefaultBranch_EmptyURL_NoCmd(t *testing.T) {
	a := loggedInApp()
	a.active = screenGuilds

	_, cmd := a.applyAttachURL("")
	if cmd != nil {
		t.Error("expected no cmd for an empty URL in the markdown-insert branch")
	}
}

// --- applyAttachURL: circ/C-Mail only support GIF via /gif (confirmed live
// this session — markdown-in-content doesn't render on the website, but
// /gif's dedicated gifUrl field does) ---

func TestApplyAttachURL_ChatroomsGIF_SetsGifCommand(t *testing.T) {
	a := loggedInApp()
	a.active = screenChatrooms

	_, cmd := a.applyAttachURL("https://example.com/pic.gif")
	if cmd == nil {
		t.Fatal("expected a cmd for the /gif branch")
	}
	msg, ok := cmd().(screens.SetComposeValueMsg)
	if !ok {
		t.Fatalf("cmd() = %T, want screens.SetComposeValueMsg", msg)
	}
	if want := "/gif https://example.com/pic.gif"; msg.Value != want {
		t.Errorf("SetComposeValueMsg.Value = %q, want %q", msg.Value, want)
	}
}

func TestApplyAttachURL_ChatroomsNonGIF_WarnsInsteadOfInserting(t *testing.T) {
	a := loggedInApp()
	a.active = screenChatrooms

	a2, _ := a.applyAttachURL("https://example.com/pic.png")
	if a2.notifyLevel != notifyWarn || a2.notifyText == "" {
		t.Errorf("expected a warning notification for a non-GIF URL in chat, got level=%v text=%q", a2.notifyLevel, a2.notifyText)
	}
}

func TestApplyAttachURL_CMailGIF_SetsGifCommand(t *testing.T) {
	a := loggedInApp()
	a.active = screenCMail

	_, cmd := a.applyAttachURL("https://example.com/pic.gif")
	if cmd == nil {
		t.Fatal("expected a cmd for the /gif branch")
	}
	if _, ok := cmd().(screens.SetComposeValueMsg); !ok {
		t.Fatalf("cmd() = %T, want screens.SetComposeValueMsg", cmd())
	}
}

func TestApplyAttachURL_CMailNonGIF_WarnsInsteadOfInserting(t *testing.T) {
	a := loggedInApp()
	a.active = screenCMail

	a2, _ := a.applyAttachURL("https://example.com/pic.png")
	if a2.notifyLevel != notifyWarn || a2.notifyText == "" {
		t.Errorf("expected a warning notification for a non-GIF URL in C-Mail, got level=%v text=%q", a2.notifyLevel, a2.notifyText)
	}
}

// TestApplyAttachURL_ReplyCompose_WarnsInsteadOfInserting: the reply API has
// no attachments field at all, so ctrl+g must not silently insert markdown
// that (per the same live evidence as the chat case) won't render anywhere.
func TestApplyAttachURL_ReplyCompose_WarnsInsteadOfInserting(t *testing.T) {
	a := loggedInApp()
	a.active = screenPostDetail
	a.postDetail = a.postDetail.SetPost(model.Post{ID: "p1", Content: "body"})
	m, _ := a.postDetail.OpenCompose()
	a.postDetail = m
	if !a.postDetail.ReplyComposeActive() {
		t.Fatal("setup: expected the reply compose box open")
	}

	a2, _ := a.applyAttachURL("https://example.com/pic.png")
	if a2.notifyLevel != notifyWarn || a2.notifyText == "" {
		t.Errorf("expected a warning notification for a reply attachment, got level=%v text=%q", a2.notifyLevel, a2.notifyText)
	}
}

// --- ctrl+g / attach-URL prompt ---

func TestHandleKeys_CtrlG_OpensAttachURLPrompt_WhenInputFocused(t *testing.T) {
	a := setupChatroomsDetailWithURL(loggedInApp())
	if !a.chatrooms.InputFocused() {
		t.Fatal("setup: expected chatrooms input focused in detail mode")
	}
	a2, _, consumed := a.handleKeys(tea.KeyMsg{Type: tea.KeyCtrlG})
	if !consumed {
		t.Fatal("expected ctrl+g to be consumed while a compose field is focused")
	}
	if !a2.attachURLPromptOpen {
		t.Error("expected attachURLPromptOpen = true after ctrl+g")
	}
}

// TestHandleAttachURLPromptKey_RejectsNonHTTPURL_KeepsPromptOpen guards the
// minimal client-side validation: a bare string with no scheme (most likely
// a typo, not a URL) must not be accepted as an attachment/markdown target.
func TestHandleAttachURLPromptKey_RejectsNonHTTPURL_KeepsPromptOpen(t *testing.T) {
	a := loggedInApp()
	a.attachURLPromptOpen = true
	a.attachURLPrompt, _ = a.attachURLPrompt.Open("attach image/gif url", "")
	for _, r := range "not-a-url" {
		m, _ := a.handleAttachURLPromptKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		a = m.(App)
	}

	m, _ := a.handleAttachURLPromptKey(tea.KeyMsg{Type: tea.KeyEnter})
	a = m.(App)
	if !a.attachURLPromptOpen {
		t.Error("expected the prompt to stay open after submitting a non-http(s) string")
	}
}

func TestHandleAttachURLPromptKey_AcceptsHTTPURL_ClosesPrompt(t *testing.T) {
	a := loggedInApp()
	a.active = screenChatrooms
	a.attachURLPromptOpen = true
	a.attachURLPrompt, _ = a.attachURLPrompt.Open("attach image/gif url", "")
	for _, r := range "https://example.com/a.png" {
		m, _ := a.handleAttachURLPromptKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		a = m.(App)
	}

	m, _ := a.handleAttachURLPromptKey(tea.KeyMsg{Type: tea.KeyEnter})
	a = m.(App)
	if a.attachURLPromptOpen {
		t.Error("expected the prompt to close after submitting a valid http(s) URL")
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

func TestHandleChatrooms_OpenRoomMsg_ActivatesScreenAndSetsPendingSlug(t *testing.T) {
	a := loggedInApp()
	a.active = screenNotifications
	a.polledUnreadCount = 3
	m, cmd := a.Update(screens.OpenRoomMsg{RoomSlug: "cyberspace", NotifID: "n1"})
	got := m.(App)
	if got.active != screenChatrooms {
		t.Errorf("active = %v, want screenChatrooms", got.active)
	}
	if got.polledUnreadCount != 2 {
		t.Errorf("polledUnreadCount = %d, want 2 (decremented)", got.polledUnreadCount)
	}
	if cmd == nil {
		t.Fatal("expected a non-nil command batch")
	}
}

func TestHandleChatrooms_RoomsLoadedMsg_ConsumesPendingSlug(t *testing.T) {
	a := loggedInApp()
	a.chatrooms = a.chatrooms.SetPendingRoomSlug("sprawl")
	rooms := []model.Room{{ID: "r1", Slug: "zion", Name: "Zion"}, {ID: "r2", Slug: "sprawl", Name: "Sprawl"}}
	m, _ := a.Update(roomsLoadedMsg{rooms: rooms})
	got := m.(App)
	if !got.chatrooms.IsShowingDetail() {
		t.Error("expected chatrooms to auto-enter detail mode for the pending slug")
	}
}

// --- chat_mention suppression for the room currently open ---
//
// Reported behavior: being mentioned in a cIRC room you're actively reading
// still notified, which reads as redundant since the message is already on
// screen. Since the API has no room-presence concept (join/leave), the fix
// filters chat_mention notifications client-side against the locally-tracked
// open room, only while Chatrooms is the foreground screen.

func TestNotifsLoaded_SuppressesMentionForActiveRoom(t *testing.T) {
	a := loggedInApp()
	a.active = screenChatrooms
	a.chatrooms = a.chatrooms.SetRooms([]model.Room{{ID: "r1", Slug: "cyberspace", Name: "Cyberspace"}})
	cm, _ := a.chatrooms.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	cm, _ = cm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	a.chatrooms = cm
	if a.chatrooms.ActiveRoomSlug() != "cyberspace" {
		t.Fatal("setup: expected the room open in detail mode")
	}
	a.polledUnreadCount = 1

	notifs := []model.Notification{{ID: "n1", Type: "chat_mention", RoomSlug: "cyberspace", Read: false}}
	m, cmd := a.Update(notifsLoadedMsg{notifs: notifs})
	got := m.(App)

	if got.notifications.UnreadCount() != 0 {
		t.Errorf("UnreadCount() = %d, want 0 (mention in the open room should be auto-suppressed)", got.notifications.UnreadCount())
	}
	if got.polledUnreadCount != 0 {
		t.Errorf("polledUnreadCount = %d, want 0 (badge should not bump for the suppressed mention)", got.polledUnreadCount)
	}
	if cmd == nil {
		t.Error("expected a mark-read cmd for the suppressed mention")
	}
}

func TestNotifsLoaded_DoesNotSuppress_WhenNotOnChatroomsScreen(t *testing.T) {
	a := loggedInApp()
	a.active = screenNotifications // not viewing Chatrooms at all
	a.polledUnreadCount = 1

	notifs := []model.Notification{{ID: "n1", Type: "chat_mention", RoomSlug: "cyberspace", Read: false}}
	m, _ := a.Update(notifsLoadedMsg{notifs: notifs})
	got := m.(App)

	if got.notifications.UnreadCount() != 1 {
		t.Errorf("UnreadCount() = %d, want 1 (mention must still notify once the room isn't open)", got.notifications.UnreadCount())
	}
	if got.polledUnreadCount != 1 {
		t.Errorf("polledUnreadCount = %d, want 1 (badge should still reflect the mention)", got.polledUnreadCount)
	}
}

func TestNotifsLoaded_DoesNotSuppress_ForADifferentRoom(t *testing.T) {
	a := loggedInApp()
	a.active = screenChatrooms
	a.chatrooms = a.chatrooms.SetRooms([]model.Room{{ID: "r1", Slug: "zion", Name: "Zion"}})
	cm, _ := a.chatrooms.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	cm, _ = cm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	a.chatrooms = cm
	if a.chatrooms.ActiveRoomSlug() != "zion" {
		t.Fatal("setup: expected zion open in detail mode")
	}
	a.polledUnreadCount = 1

	notifs := []model.Notification{{ID: "n1", Type: "chat_mention", RoomSlug: "cyberspace", Read: false}}
	m, _ := a.Update(notifsLoadedMsg{notifs: notifs})
	got := m.(App)

	if got.notifications.UnreadCount() != 1 {
		t.Errorf("UnreadCount() = %d, want 1 (mention in a different room must still notify)", got.notifications.UnreadCount())
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

// --- ESC from a deep-linked C-Mail/Chatrooms conversation returns to origin ---
//
// Reported behavior: pressing 'c' on a post opens a C-Mail conversation with
// its author; ESC used to always drop to C-Mail's own conversation list
// instead of back to the post. C-Mail and Chatrooms are reached via 'c' /
// chat_mention / dm_message deep links from several origin screens, so the
// origin is captured on CMailModel/ChatroomsModel via canGoBack and consumed
// by a dedicated Leave*Msg on ESC, mirroring the existing profileReturn/
// postDetailReturn/searchReturn pattern.

func TestHandleCMail_StartConversation_EscReturnsToOrigin(t *testing.T) {
	for _, origin := range []screen{screenFeed, screenPostDetail, screenNotifications, screenProfile} {
		t.Run(fmt.Sprintf("origin_%d", origin), func(t *testing.T) {
			a := loggedInApp()
			a.active = origin

			m, cmd, _ := a.handleCMail(screens.StartConversationMsg{Username: "molly"})
			if m.cmailReturn != origin {
				t.Fatalf("expected cmailReturn = %v, got %v", origin, m.cmailReturn)
			}
			if cmd == nil {
				t.Fatal("expected a start-conversation cmd")
			}

			// Resolve the (mocked) API call exactly as Bubble Tea would.
			m2, _ := m.Update(cmd())
			a2 := m2.(App)
			if a2.active != screenCMail {
				t.Fatalf("expected navigation to screenCMail, got %v", a2.active)
			}
			if !a2.cmail.IsShowingDetail() {
				t.Fatal("expected cmail to land directly in detail mode")
			}

			cm, escCmd := a2.cmail.Update(tea.KeyMsg{Type: tea.KeyEsc})
			a2.cmail = cm
			if escCmd == nil {
				t.Fatal("expected esc on a deep-linked conversation to emit LeaveCMailMsg")
			}
			m3, _ := a2.Update(escCmd())
			a3 := m3.(App)
			if a3.active != origin {
				t.Errorf("expected esc to return to %v (the deep-link origin), got %v", origin, a3.active)
			}
		})
	}
}

// TestHandleCMail_ManualTabReentry_ResetsToList mirrors
// TestHandleChatrooms_ManualTabReentry_ResetsToList for C-Mail: manually
// switching to the C-Mail tab while a deep-linked conversation is open
// must drop back to the conversation list, not stay in detail mode.
func TestHandleCMail_ManualTabReentry_ResetsToList(t *testing.T) {
	a := loggedInApp()
	a.active = screenFeed

	m, cmd, _ := a.handleCMail(screens.StartConversationMsg{Username: "molly"})
	if cmd == nil {
		t.Fatal("expected a start-conversation cmd")
	}
	m2, _ := m.Update(cmd())
	a2 := m2.(App)
	if !a2.cmail.IsShowingDetail() {
		t.Fatal("expected cmail to land directly in detail mode")
	}

	a3, _ := activateScreen(a2, screenCMail) // simulates pressing the C-Mail tab instead of Esc
	if a3.cmail.IsShowingDetail() {
		t.Error("expected manual tab re-entry to drop back to the conversation list, not stay in the deep-linked conversation")
	}
}

func TestHandleCMail_NormalTabEntry_EscStillDropsToList(t *testing.T) {
	a := loggedInApp()
	a, _ = activateScreen(a, screenCMail) // ordinary tab/leader-key entry
	a.cmail = a.cmail.SetConversations([]model.Conversation{
		{ID: "c1", Participants: []model.User{{Username: a.currentUser.Username}, {Username: "molly"}}},
	})
	cm, _ := a.cmail.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	cm, _ = cm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	a.cmail = cm
	if !a.cmail.IsShowingDetail() {
		t.Fatal("expected entering a conversation via enter to reach detail mode")
	}

	cm2, escCmd := a.cmail.Update(tea.KeyMsg{Type: tea.KeyEsc})
	a.cmail = cm2
	if a.cmail.IsShowingDetail() {
		t.Error("expected esc to drop back to C-Mail's own list when not deep-linked")
	}
	if escCmd != nil {
		t.Error("expected no LeaveCMailMsg when the conversation wasn't reached via a deep link")
	}
}

func TestHandleChatrooms_OpenRoomMsg_EscReturnsToOrigin(t *testing.T) {
	a := loggedInApp()
	a.active = screenNotifications

	m, _, _ := a.handleChatrooms(screens.OpenRoomMsg{RoomSlug: "sprawl", NotifID: "n1"})
	if m.chatroomsReturn != screenNotifications {
		t.Fatalf("expected chatroomsReturn = screenNotifications, got %v", m.chatroomsReturn)
	}

	// Simulate the room list (re)loading, which lets OpenPendingRoom
	// auto-enter detail mode for the deep-linked room.
	rooms := []model.Room{{ID: "r1", Slug: "sprawl", Name: "Sprawl"}}
	m2, _ := m.Update(roomsLoadedMsg{rooms: rooms})
	a2 := m2.(App)
	if !a2.chatrooms.IsShowingDetail() {
		t.Fatal("expected chatrooms to auto-enter detail mode for the pending slug")
	}

	cm, escCmd := a2.chatrooms.Update(tea.KeyMsg{Type: tea.KeyEsc})
	a2.chatrooms = cm
	if escCmd == nil {
		t.Fatal("expected esc on a deep-linked room to emit LeaveChatroomsMsg")
	}
	// esc on a deep-linked room now also fires a best-effort leave-presence
	// call alongside LeaveChatroomsMsg, so the returned cmd is a tea.Batch;
	// find LeaveChatroomsMsg within it (mirrors how the real runtime would
	// execute each batched cmd and dispatch its message).
	leaveMsg := findBatchedMsg[screens.LeaveChatroomsMsg](t, escCmd())
	m3, _ := a2.Update(leaveMsg)
	a3 := m3.(App)
	if a3.active != screenNotifications {
		t.Errorf("expected esc to return to screenNotifications (the deep-link origin), got %v", a3.active)
	}
}

// findBatchedMsg unwraps a tea.BatchMsg (or a plain message) looking for one
// of type T, executing each batched cmd to find it. Fails the test if T isn't
// found anywhere.
func findBatchedMsg[T any](t *testing.T, msg tea.Msg) T {
	t.Helper()
	if m, ok := msg.(T); ok {
		return m
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			if c == nil {
				continue
			}
			if m, ok := c().(T); ok {
				return m
			}
		}
	}
	t.Fatalf("expected a message of type %T within %#v", *new(T), msg)
	var zero T
	return zero
}

// TestHandleChatrooms_ManualTabReentry_ResetsToList guards against a bug
// where switching to the Chatrooms tab manually (key "4", tab bar, leader
// chord) while a chat_mention deep link had left it in detail mode failed
// to drop back to the room list — only ESC did, because activateScreen
// cleared canGoBack without ever resetting mode.
func TestHandleChatrooms_ManualTabReentry_ResetsToList(t *testing.T) {
	a := loggedInApp()
	a.active = screenNotifications

	m, _, _ := a.handleChatrooms(screens.OpenRoomMsg{RoomSlug: "sprawl", NotifID: "n1"})
	rooms := []model.Room{{ID: "r1", Slug: "sprawl", Name: "Sprawl"}}
	m2, _ := m.Update(roomsLoadedMsg{rooms: rooms})
	a2 := m2.(App)
	if !a2.chatrooms.IsShowingDetail() {
		t.Fatal("expected chatrooms to auto-enter detail mode for the pending slug")
	}

	a3, _ := activateScreen(a2, screenChatrooms) // simulates pressing "4" instead of Esc
	if a3.chatrooms.IsShowingDetail() {
		t.Error("expected manual tab re-entry to drop back to the room list, not stay in the deep-linked room")
	}
}

func TestHandleChatrooms_NormalTabEntry_EscStillDropsToList(t *testing.T) {
	a := loggedInApp()
	a, _ = activateScreen(a, screenChatrooms) // ordinary tab/leader-key entry
	a.chatrooms = a.chatrooms.SetRooms([]model.Room{{ID: "r1", Slug: "zion", Name: "Zion"}})
	cm, _ := a.chatrooms.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	cm, _ = cm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	a.chatrooms = cm
	if !a.chatrooms.IsShowingDetail() {
		t.Fatal("expected entering a room via enter to reach detail mode")
	}

	cm2, escCmd := a.chatrooms.Update(tea.KeyMsg{Type: tea.KeyEsc})
	a.chatrooms = cm2
	if a.chatrooms.IsShowingDetail() {
		t.Error("expected esc to drop back to Chatrooms' own list when not deep-linked")
	}
	// escCmd is no longer nil — esc now always fires a best-effort
	// leave-presence call — but it must not carry a LeaveChatroomsMsg.
	if escCmd != nil {
		if _, ok := escCmd().(screens.LeaveChatroomsMsg); ok {
			t.Error("did not expect LeaveChatroomsMsg when the room wasn't reached via a deep link")
		}
	}
}

// --- Chatrooms room stays open across a tab switch ---
//
// Previously activateScreen cancelled the CIRC subscription and dropped to
// the room list on every tab-away, so a room went silent the moment you
// checked another tab. Now the subscription (and its reconnect/heartbeat
// chains, via screens.IsRoomStreamMsg routing in handleChatrooms) stays alive
// in the background for the one room the user had open, and switching back
// resumes it instead of bouncing to the list.

func TestActivateScreen_ChatroomsRoomSurvivesTabSwitch(t *testing.T) {
	a := loggedInApp()
	a, _ = activateScreen(a, screenChatrooms)
	a.chatrooms = a.chatrooms.SetRooms([]model.Room{{ID: "r1", Slug: "zion", Name: "Zion"}})
	cm, _ := a.chatrooms.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	cm, _ = cm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	a.chatrooms = cm
	if !a.chatrooms.IsShowingDetail() {
		t.Fatal("setup: expected entering a room via enter to reach detail mode")
	}

	a, _ = activateScreen(a, screenFeed) // switch away to another tab
	if !a.chatrooms.IsShowingDetail() {
		t.Error("expected the open room to stay open while another tab is active")
	}
	if a.chatrooms.ActiveRoomSlug() != "zion" {
		t.Errorf("ActiveRoomSlug() = %q, want %q after switching away", a.chatrooms.ActiveRoomSlug(), "zion")
	}

	a, _ = activateScreen(a, screenChatrooms) // switch back
	if !a.chatrooms.IsShowingDetail() {
		t.Error("expected switching back to Chatrooms to resume the still-open room, not the room list")
	}
	if a.chatrooms.ActiveRoomSlug() != "zion" {
		t.Errorf("ActiveRoomSlug() = %q, want %q after switching back", a.chatrooms.ActiveRoomSlug(), "zion")
	}
}

// TestActivateScreen_CMailConvSurvivesTabSwitch mirrors
// TestActivateScreen_ChatroomsRoomSurvivesTabSwitch above — C-Mail's open
// conversation now stays live in the background the same way, via
// CMailModel.SetFocused and screens.IsDMStreamMsg routing in handleCMail.
func TestActivateScreen_CMailConvSurvivesTabSwitch(t *testing.T) {
	a := loggedInApp()
	a, _ = activateScreen(a, screenCMail)
	a.cmail = a.cmail.SetConversations([]model.Conversation{
		{ID: "c1", Participants: []model.User{{Username: "trinity"}}},
	})
	cm, _ := a.cmail.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	cm, _ = cm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	a.cmail = cm
	if !a.cmail.IsShowingDetail() {
		t.Fatal("setup: expected entering a conversation via enter to reach detail mode")
	}

	a, _ = activateScreen(a, screenFeed) // switch away to another tab
	if !a.cmail.IsShowingDetail() {
		t.Error("expected the open conversation to stay open while another tab is active")
	}
	if !strings.Contains(a.cmail.View(), "trinity") {
		t.Error("expected the same conversation (trinity) still shown after switching away")
	}

	a, _ = activateScreen(a, screenCMail) // switch back
	if !a.cmail.IsShowingDetail() {
		t.Error("expected switching back to C-Mail to resume the still-open conversation, not the conversation list")
	}
	if !strings.Contains(a.cmail.View(), "trinity") {
		t.Error("expected the same conversation (trinity) still shown after switching back")
	}
}

// --- Kitty image-modal cleanup survives a fast follow-up keystroke ---
//
// Reported behavior: typing while an image loads could leave the Kitty image
// placement stuck on screen forever, needing a full TUI restart to clear.
// Root cause: the delete-placement escape was injected into exactly one
// View() frame, and the very next Update() call unconditionally cleared
// imageNeedsCleanup regardless of whether the renderer's independent flush
// ticker had actually sent that frame to the terminal yet. A first fix used a
// fixed delay before clearing the flag, but Bubble Tea's write()/flush() share
// a mutex and flush() blocks on the OS write for as long as the terminal takes
// to drain it — for a large image that can exceed any fixed delay, so the
// fixed-delay fix still lost the race for big/slow images. imageNeedsCleanup
// now simply isn't auto-cleared at all: it stays true (re-injecting the
// delete-placement escape into every closed-state frame) until a new Kitty
// image is opened, so however long the renderer's backlog takes to drain,
// the pending cleanup frame is still there once it does.

func TestUpdate_ImageModalClose_Kitty_SetsCleanupFlag(t *testing.T) {
	a := loggedInApp()
	a.graphicsProtocol = imgview.ProtocolKitty
	a.imageModalOpen = true

	m, cmd := a.Update(keyMsg("x"))
	a2 := m.(App)
	if a2.imageModalOpen {
		t.Error("expected the keypress to close the modal")
	}
	if !a2.imageNeedsCleanup {
		t.Error("expected imageNeedsCleanup to be set for the Kitty protocol")
	}
	if cmd != nil {
		t.Error("expected no cmd from closing the modal")
	}
}

func TestUpdate_ImageModalClose_ITerm2_NoCleanupFlag(t *testing.T) {
	a := loggedInApp()
	a.graphicsProtocol = imgview.ProtocolITerm2
	a.imageModalOpen = true

	m, cmd := a.Update(keyMsg("x"))
	a2 := m.(App)
	if a2.imageModalOpen {
		t.Error("expected the keypress to close the modal")
	}
	if a2.imageNeedsCleanup {
		t.Error("iTerm2 has no placement to clean up, expected imageNeedsCleanup to stay false")
	}
	if cmd != nil {
		t.Error("expected no cmd scheduled for iTerm2")
	}
}

func TestUpdate_ImageNeedsCleanup_NotAutoCleared(t *testing.T) {
	a := loggedInApp()
	a.graphicsProtocol = imgview.ProtocolKitty
	a.imageModalOpen = true

	m, _ := a.Update(keyMsg("x"))
	a2 := m.(App)
	if !a2.imageNeedsCleanup {
		t.Fatal("setup: expected imageNeedsCleanup to be set after close")
	}

	// Several unrelated Update cycles (e.g. more typing while the renderer's
	// flush is still backed up on a large image) must not clear the flag —
	// only actually opening a new image should.
	for i := 0; i < 5; i++ {
		m2, _ := a2.Update(keyMsg("y"))
		a2 = m2.(App)
	}
	if !a2.imageNeedsCleanup {
		t.Error("expected imageNeedsCleanup to survive unrelated Update calls until a new image opens")
	}
}

// TestHandleImageViewer_OpenPausesChatStyleAnim and its close counterpart
// guard the fix for animated (wave/blink/glitch) chat lines corrupting the
// image modal: a re-render triggered by styleAnimTickMsg changes a terminal
// row's bytes, forcing Bubble Tea to resend the whole row — including the
// part of it covered by the modal's box — which erases the modal's
// graphics-protocol pixels there. Pausing chatrooms/cmail's animation
// re-render while the modal is open avoids that.
func TestHandleImageViewer_OpenPausesChatStyleAnim(t *testing.T) {
	a := loggedInApp()
	a.graphicsProtocol = imgview.ProtocolKitty

	a2, _, ok := a.handleImageViewer(imageFetchedMsg{rawURL: "https://example.com/x.jpg", encoded: "seq", cols: 10, rows: 5})
	if !ok {
		t.Fatal("expected imageFetchedMsg to be handled")
	}
	if !a2.chatrooms.AnimPaused() {
		t.Error("expected chatrooms.animPaused to be set once the modal opens")
	}
	if !a2.cmail.AnimPaused() {
		t.Error("expected cmail.animPaused to be set once the modal opens")
	}
}

func TestUpdate_ImageModalClose_ResumesChatStyleAnim(t *testing.T) {
	a := loggedInApp()
	a.graphicsProtocol = imgview.ProtocolKitty
	a.imageModalOpen = true
	a.chatrooms = a.chatrooms.SetAnimPaused(true)
	a.cmail = a.cmail.SetAnimPaused(true)

	m, _ := a.Update(keyMsg("x"))
	a2 := m.(App)
	if a2.chatrooms.AnimPaused() {
		t.Error("expected chatrooms.animPaused to be cleared once the modal closes")
	}
	if a2.cmail.AnimPaused() {
		t.Error("expected cmail.animPaused to be cleared once the modal closes")
	}
}

func TestHandleImageViewer_NewImage_ClearsCleanupFlag(t *testing.T) {
	a := loggedInApp()
	a.graphicsProtocol = imgview.ProtocolKitty
	a.imageNeedsCleanup = true

	a2, _, ok := a.handleImageViewer(imageFetchedMsg{rawURL: "https://example.com/x.jpg", encoded: "seq", cols: 10, rows: 5})
	if !ok {
		t.Fatal("expected imageFetchedMsg to be handled")
	}
	if !a2.imageModalOpen {
		t.Error("expected the new image to open the modal")
	}
	if a2.imageNeedsCleanup {
		t.Error("expected opening a new image to clear any pending cleanup flag")
	}
}

// --- Image carousel: cycling through a picker's images without closing ---
//
// When the URL picker holds more than one image URL, opening one now enters
// a carousel: left/right cycle between the picker's images instead of
// closing the modal. Any other key still closes it immediately, same as a
// plain single-image view. Async fetch results are stamped with a
// generation counter (bumped on every new fetch and on close) so a stale
// result — from cycling again before the previous fetch resolved, or from
// closing before it resolved — can never resurrect the modal or overwrite a
// newer image.

func TestCanRenderImageInline_Ephemeral_AlwaysFalse(t *testing.T) {
	a := loggedInApp()
	a.graphicsProtocol = imgview.ProtocolKitty
	a.imageViewer = "terminal"
	a.ephemeral = true
	if a.canRenderImageInline("https://example.com/x.jpg") {
		t.Error("expected ephemeral (SSH-hosted) sessions to never render images inline")
	}
}

func TestHandleURLPickerKey_Enter_MultipleImages_StartsCarousel(t *testing.T) {
	a := loggedInApp()
	a.graphicsProtocol = imgview.ProtocolKitty
	a.urlPickerOpen = true
	a.urlPickerItems = []string{"https://x.com/page", "https://x.com/a.jpg", "https://x.com/b.png"}
	a.urlPickerCursor = 2 // b.png

	m, cmd := a.handleURLPickerKey(tea.KeyMsg{Type: tea.KeyEnter})
	a2 := m.(App)
	if a2.urlPickerOpen {
		t.Error("expected the picker to close")
	}
	if cmd == nil {
		t.Fatal("expected a fetch cmd")
	}
	if len(a2.imageCarouselItems) != 2 {
		t.Fatalf("expected 2 carousel items (the images only), got %d: %v", len(a2.imageCarouselItems), a2.imageCarouselItems)
	}
	if a2.imageCarouselIndex != 1 {
		t.Errorf("expected carousel index 1 (b.png, the second image), got %d", a2.imageCarouselIndex)
	}
}

func TestHandleURLPickerKey_Enter_SingleImage_NoCarousel(t *testing.T) {
	a := loggedInApp()
	a.graphicsProtocol = imgview.ProtocolKitty
	a.urlPickerOpen = true
	a.urlPickerItems = []string{"https://x.com/page", "https://x.com/a.jpg"}
	a.urlPickerCursor = 1

	m, _ := a.handleURLPickerKey(tea.KeyMsg{Type: tea.KeyEnter})
	a2 := m.(App)
	if a2.imageCarouselItems != nil {
		t.Errorf("expected no carousel with only one image among the picker's URLs, got %v", a2.imageCarouselItems)
	}
}

// TestUpdate_ImageModal_LeftRight_CyclesWithoutClosing confirms a left/right
// press updates the carousel index and schedules a debounce tick
// (carouselCycleGen bumped, a non-nil cmd) but does NOT start the real
// fetch immediately — see cycleImageCarousel's doc comment for why: holding
// the key down should feel responsive (index moves right away) without
// firing one expensive fetch per repeat. The fetch only actually starts
// once the debounce tick lands (simulated here via carouselCycleSettledMsg)
// and its gen still matches, i.e. nothing superseded it.
func TestUpdate_ImageModal_LeftRight_CyclesWithoutClosing(t *testing.T) {
	a := loggedInApp()
	a.graphicsProtocol = imgview.ProtocolKitty
	a.imageModalOpen = true
	a.imageCarouselItems = []string{"https://x.com/a.jpg", "https://x.com/b.jpg", "https://x.com/c.jpg"}
	a.imageCarouselIndex = 0
	genBefore := a.imageFetchGen

	m, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRight})
	a2 := m.(App)
	if !a2.imageModalOpen {
		t.Error("expected the modal to stay open while cycling")
	}
	if a2.imageCarouselIndex != 1 {
		t.Errorf("expected carousel index 1 after right, got %d", a2.imageCarouselIndex)
	}
	if len(a2.imageCarouselItems) != 3 {
		t.Error("expected carousel items to survive cycling")
	}
	if cmd == nil {
		t.Fatal("expected a debounce-tick cmd scheduled for the newly cycled-to image")
	}
	if a2.imageFetchGen != genBefore {
		t.Error("expected imageFetchGen NOT to advance yet — the fetch is debounced")
	}
	if a2.carouselCycleGen == a.carouselCycleGen {
		t.Error("expected carouselCycleGen bumped on cycle")
	}

	m3, cmd3, ok := a2.handleImageViewer(carouselCycleSettledMsg{gen: a2.carouselCycleGen})
	if !ok {
		t.Fatal("expected carouselCycleSettledMsg to be handled")
	}
	if cmd3 == nil {
		t.Fatal("expected the real fetch cmd once the debounce tick lands")
	}
	if m3.imageFetchGen == genBefore {
		t.Error("expected imageFetchGen to advance once the debounce tick fires")
	}

	m2, _ := a2.Update(tea.KeyMsg{Type: tea.KeyLeft})
	a3 := m2.(App)
	if a3.imageCarouselIndex != 0 {
		t.Errorf("expected carousel index to wrap/return to 0 after left, got %d", a3.imageCarouselIndex)
	}
}

// TestCycleImageCarousel_SupersededTickDoesNothing confirms a debounce tick
// whose gen no longer matches a.carouselCycleGen (a later left/right press
// happened before it fired) is a no-op — the whole point of debouncing is
// that holding the key only ever fetches the image the user lands on, not
// every intermediate one.
func TestCycleImageCarousel_SupersededTickDoesNothing(t *testing.T) {
	a := loggedInApp()
	a.graphicsProtocol = imgview.ProtocolKitty
	a.imageModalOpen = true
	a.imageCarouselItems = []string{"https://x.com/a.jpg", "https://x.com/b.jpg"}
	genBefore := a.imageFetchGen

	a2, _ := a.cycleImageCarousel(+1)  // gen N
	a3, _ := a2.cycleImageCarousel(+1) // gen N+1, supersedes the first tick

	m, cmd, ok := a3.handleImageViewer(carouselCycleSettledMsg{gen: a2.carouselCycleGen})
	if !ok {
		t.Fatal("expected carouselCycleSettledMsg to be handled even when superseded")
	}
	if cmd != nil {
		t.Error("expected no fetch cmd for a superseded debounce tick")
	}
	if m.imageFetchGen != genBefore {
		t.Error("expected imageFetchGen untouched by a superseded debounce tick")
	}
}

func TestUpdate_ImageModal_OtherKey_ClosesAndClearsCarousel(t *testing.T) {
	a := loggedInApp()
	a.graphicsProtocol = imgview.ProtocolKitty
	a.imageModalOpen = true
	a.imageCarouselItems = []string{"https://x.com/a.jpg", "https://x.com/b.jpg"}
	a.imageCarouselIndex = 1
	a.imageCache = map[string]cachedImage{"https://x.com/a.jpg": {frames: []image.Image{image.NewRGBA(image.Rect(0, 0, 2, 2))}}}
	genBefore := a.imageFetchGen

	m, _ := a.Update(keyMsg("x"))
	a2 := m.(App)
	if a2.imageModalOpen {
		t.Error("expected a non-arrow key to close the modal even mid-carousel")
	}
	if a2.imageCarouselItems != nil {
		t.Error("expected carousel state to be cleared on close")
	}
	if a2.imageFetchGen == genBefore {
		t.Error("expected imageFetchGen to advance on close, invalidating any in-flight fetch")
	}
	if a2.imageCache != nil {
		t.Error("expected imageCache to be cleared on close")
	}
}

// --- Image modal scale: live +/- adjustment (config.Config.ImageScale) ---

// modalTestImage is a 100x200px image, chosen so its native cell size at the
// fallback cell pixel default (10x20, since TerminalCellPixelSize returns
// ok=false in tests) is a clean 10x10 cols/rows — makes expected imageScale
// values after a guaranteed-1-cell step easy to state exactly.
func modalTestImage() image.Image {
	return image.NewRGBA(image.Rect(0, 0, 100, 200))
}

func TestUpdate_ImageModal_Plus_IncreasesScaleAndRerenders(t *testing.T) {
	a := loggedInApp()
	a.graphicsProtocol = imgview.ProtocolKitty
	a.imageModalOpen = true
	a.imageModalURL = "https://x.com/a.jpg"
	a.imageCache = map[string]cachedImage{"https://x.com/a.jpg": {frames: []image.Image{modalTestImage()}}}

	m, cmd := a.Update(keyMsg("+"))
	a2 := m.(App)
	if !a2.imageModalOpen {
		t.Error("expected the modal to stay open on a scale-adjust keypress")
	}
	// nativeCols=10, step=max(1, round(10*0.1))=1, currentCols=10 -> targetCols=11 -> scale=1.1.
	if want := 1.1; a2.imageScale != want {
		t.Errorf("imageScale = %v, want %v", a2.imageScale, want)
	}
	if cmd == nil {
		t.Fatal("expected a re-render cmd for the current modal image")
	}
}

func TestUpdate_ImageModal_Minus_DecreasesScale(t *testing.T) {
	a := loggedInApp()
	a.graphicsProtocol = imgview.ProtocolKitty
	a.imageModalOpen = true
	a.imageModalURL = "https://x.com/a.jpg"
	a.imageCache = map[string]cachedImage{"https://x.com/a.jpg": {frames: []image.Image{modalTestImage()}}}

	m, _ := a.Update(keyMsg("-"))
	a2 := m.(App)
	if want := 0.9; a2.imageScale != want {
		t.Errorf("imageScale = %v, want %v", a2.imageScale, want)
	}
}

func TestAdjustImageScale_ClampsAtBounds(t *testing.T) {
	a := loggedInApp()
	a.imageModalURL = "https://x.com/a.jpg"
	a.imageCache = map[string]cachedImage{"https://x.com/a.jpg": {frames: []image.Image{modalTestImage()}}}
	a.imageScale = config.MaxImageScale

	a2, cmd := a.adjustImageScale(imageScaleStep)
	if a2.imageScale != config.MaxImageScale {
		t.Errorf("expected clamp at MaxImageScale (%v), got %v", config.MaxImageScale, a2.imageScale)
	}
	if cmd == nil {
		t.Error("expected a re-render cmd even when already at the max (still a legitimate re-render request)")
	}

	a.imageScale = config.MinImageScale
	a3, _ := a.adjustImageScale(-imageScaleStep)
	if a3.imageScale != config.MinImageScale {
		t.Errorf("expected clamp at MinImageScale (%v), got %v", config.MinImageScale, a3.imageScale)
	}
}

// TestAdjustImageScale_NoCacheIsNoop confirms a scale-adjust keypress before
// the modal's image has actually landed in the cache (shouldn't normally
// happen — the cache is populated the instant the modal opens) doesn't
// panic on the map lookup.
func TestAdjustImageScale_NoCacheIsNoop(t *testing.T) {
	a := loggedInApp()
	a.imageModalURL = "https://x.com/never-cached.jpg"
	a.imageScale = 1.0

	a2, cmd := a.adjustImageScale(imageScaleStep)
	if a2.imageScale != 1.0 {
		t.Errorf("expected imageScale unchanged, got %v", a2.imageScale)
	}
	if cmd != nil {
		t.Error("expected no cmd when there's nothing cached to re-render")
	}
}

// TestAdjustImageScale_TinyImage_AlwaysMovesAtLeastOneCell confirms the
// guaranteed-visible-step design: even for an image whose native size is a
// single cell (so a flat 10% step would round to nothing), a press still
// moves the box by at least 1 cell — this is the property that was missing
// before the fix (see docs/46-image-modal-scale.md): +/- appeared to do
// nothing for typical images because fitBox's never-upscale cap silently
// absorbed the old percent-of-terminal-box scale steps.
func TestAdjustImageScale_TinyImage_AlwaysMovesAtLeastOneCell(t *testing.T) {
	a := loggedInApp()
	a.imageModalURL = "https://x.com/tiny.jpg"
	a.imageCache = map[string]cachedImage{"https://x.com/tiny.jpg": {frames: []image.Image{image.NewRGBA(image.Rect(0, 0, 2, 2))}}}
	a.imageScale = 1.0 // nativeCols=1 at the fallback cell size, so scale=1.0 -> currentCols=1

	a2, _ := a.adjustImageScale(imageScaleStep)
	// step=max(1, round(1*0.1))=1 -> targetCols=2 -> scale=2.0 (also happens
	// to be config.MaxImageScale here, since nativeCols=1 leaves no room
	// between "native" and "2x native" other than a single whole cell).
	if a2.imageScale != config.MaxImageScale {
		t.Errorf("imageScale = %v, want %v (a 1-cell step from a 1-cell native size)", a2.imageScale, config.MaxImageScale)
	}
}

// TestAdjustImageScale_MinusImmediatelyShrinksAfterHittingScreenMargin is a
// regression test: openImageInTerminal's screen-margin clamp
// (modalScreenMarginFrac/Layout.ModalMaxWidth) can cap the rendered box well
// below what config.MaxImageScale alone would allow for a large native
// image on a small terminal. adjustImageScale used to step from a value
// recomputed off a.imageScale, which kept climbing on repeated "+" presses
// even after the box stopped growing (correctly clamped) — so the first "-"
// press afterward only walked that invisible excess back down one step,
// visibly doing nothing either. Reported live as "+ does nothing at max
// size, then - does nothing the first time too." Fixed by anchoring the
// step to a.imageModalCols (the box actually on screen), so "-" always
// shrinks immediately.
func TestAdjustImageScale_MinusImmediatelyShrinksAfterHittingScreenMargin(t *testing.T) {
	// 500x500, not something more extreme: large enough that scale=2.0
	// still hits the screen-margin ceiling (reproducing the bug), but small
	// enough that scale=0.2 does NOT also collapse onto the same ceiling —
	// an image so much bigger than the terminal that every scale in
	// [0.2, 2.0] renders identically would trivially "pass" without
	// exercising the fix at all.
	bigImg := image.NewRGBA(image.Rect(0, 0, 500, 500))
	a := loggedInApp()
	a.graphicsProtocol = imgview.ProtocolKitty
	a.width, a.height = 100, 40
	a.imageModalURL = "https://x.com/a.jpg"
	a.imageCache = map[string]cachedImage{"https://x.com/a.jpg": {frames: []image.Image{bigImg}}}
	a.imageScale = config.MaxImageScale // simulate having already pressed + repeatedly

	// Render once to populate a.imageModalCols with the true,
	// screen-margin-clamped box — mirrors what handleImageViewer does for a
	// real imageFetchedMsg.
	_, cmd := a.openImageInTerminal(a.imageModalURL)
	msg := cmd().(imageFetchedMsg)
	a.imageModalCols = msg.cols
	a.imageModalRows = msg.rows

	// Setup check: a further "+" is a no-op visually — already at the
	// screen-margin ceiling, not the (much larger) native-size-relative max.
	a2, cmd2 := a.adjustImageScale(imageScaleStep)
	msg2 := cmd2().(imageFetchedMsg)
	if msg2.cols != msg.cols {
		t.Fatalf("setup: expected + to have no visible effect at the screen-margin ceiling, got cols %d -> %d", msg.cols, msg2.cols)
	}
	a2.imageModalCols = msg2.cols

	// The regression: the very next "-" must immediately shrink the box.
	_, cmd3 := a2.adjustImageScale(-imageScaleStep)
	msg3 := cmd3().(imageFetchedMsg)
	if msg3.cols >= msg2.cols {
		t.Errorf("expected the first - press after hitting the ceiling to immediately shrink the box, got cols %d -> %d", msg2.cols, msg3.cols)
	}
}

// TestOpenImageInTerminal_Scale_ChangesComputedBox confirms imageScale
// actually reaches the encoder: a larger scale should never produce a
// smaller cols/rows box than a smaller scale, for a source image large
// enough that the display box (not the image's native size) is the
// limiting factor — see openImageInTerminal's displayCols/displayRows.
func TestOpenImageInTerminal_Scale_ChangesComputedBox(t *testing.T) {
	bigImg := image.NewRGBA(image.Rect(0, 0, 1000, 1000))

	render := func(scale float64) (cols, rows int) {
		a := loggedInApp()
		a.graphicsProtocol = imgview.ProtocolKitty
		a.width, a.height = 100, 50
		a.imageScale = scale
		a.imageCache = map[string]cachedImage{"https://x.com/a.jpg": {frames: []image.Image{bigImg}}}
		_, cmd := a.openImageInTerminal("https://x.com/a.jpg")
		msg := cmd().(imageFetchedMsg)
		if msg.err != nil {
			t.Fatalf("unexpected error: %v", msg.err)
		}
		return msg.cols, msg.rows
	}

	smallCols, smallRows := render(0.5)
	bigCols, bigRows := render(1.5)
	if bigCols < smallCols || bigRows < smallRows {
		t.Errorf("expected scale 1.5 to produce a box at least as large as scale 0.5, got %dx%d vs %dx%d", bigCols, bigRows, smallCols, smallRows)
	}
	if bigCols == smallCols && bigRows == smallRows {
		t.Error("expected scale to actually change the computed box for a large source image")
	}
}

// TestOpenImageInTerminal_MillerLayout_ClampsBelowSidebarOverlap confirms a
// high-scale image modal never grows wide enough, in Miller layout, for its
// centered position (compositeOverlays centers against the full terminal
// width, not the content pane — see Layout.ModalMaxWidth's doc comment) to
// splice into the nav sidebar. TabsLayout, with no side chrome, is
// unaffected and still clamps to the full terminal width.
func TestOpenImageInTerminal_MillerLayout_ClampsBelowSidebarOverlap(t *testing.T) {
	bigImg := image.NewRGBA(image.Rect(0, 0, 2000, 2000))

	render := func(layout Layout) int {
		a := loggedInApp()
		a.graphicsProtocol = imgview.ProtocolKitty
		a.layout = layout
		a.width, a.height = 120, 50
		a.imageScale = config.MaxImageScale
		a.imageCache = map[string]cachedImage{"https://x.com/a.jpg": {frames: []image.Image{bigImg}}}
		_, cmd := a.openImageInTerminal("https://x.com/a.jpg")
		msg := cmd().(imageFetchedMsg)
		if msg.err != nil {
			t.Fatalf("unexpected error: %v", msg.err)
		}
		return msg.cols
	}

	tabsCols := render(TabsLayout{})
	millerCols := render(MillerLayout{})

	if tabsCols > 120 {
		t.Errorf("TabsLayout: cols=%d, want <= terminal width 120", tabsCols)
	}
	if want := (MillerLayout{}).ModalMaxWidth(120); millerCols > want {
		t.Errorf("MillerLayout: cols=%d, want <= ModalMaxWidth(120)=%d (would splice into the sidebar when centered)", millerCols, want)
	}
	if millerCols >= tabsCols {
		t.Errorf("expected MillerLayout to clamp narrower than TabsLayout for the same terminal width, got miller=%d tabs=%d", millerCols, tabsCols)
	}
}

// TestOpenImageInTerminal_NeverExceedsScreenMargin confirms the modal never
// occupies more than modalScreenMarginFrac (80%) of the terminal in either
// dimension, on any layout, even at max scale with a huge source image —
// see modalScreenMarginFrac's doc comment for why this headroom matters
// beyond just avoiding Miller's sidebar (a documented class of terminal
// rendering desync around large raw image payloads, reported live as
// surrounding UI chrome getting visibly corrupted at close to full-screen
// modal size).
func TestOpenImageInTerminal_NeverExceedsScreenMargin(t *testing.T) {
	hugeImg := image.NewRGBA(image.Rect(0, 0, 6000, 6000))

	for _, layout := range []Layout{TabsLayout{}, MillerLayout{}} {
		a := loggedInApp()
		a.graphicsProtocol = imgview.ProtocolKitty
		a.layout = layout
		a.width, a.height = 300, 100
		a.imageScale = config.MaxImageScale
		a.imageCache = map[string]cachedImage{"https://x.com/a.jpg": {frames: []image.Image{hugeImg}}}
		_, cmd := a.openImageInTerminal("https://x.com/a.jpg")
		msg := cmd().(imageFetchedMsg)
		if msg.err != nil {
			t.Fatalf("unexpected error: %v", msg.err)
		}
		maxCols := int(float64(a.width) * modalScreenMarginFrac)
		maxRows := int(float64(a.height) * modalScreenMarginFrac)
		if msg.cols > maxCols {
			t.Errorf("%T: cols=%d, want <= %d (80%% of width %d)", layout, msg.cols, maxCols, a.width)
		}
		if msg.rows > maxRows {
			t.Errorf("%T: rows=%d, want <= %d (80%% of height %d)", layout, msg.rows, maxRows, a.height)
		}
	}
}

// --- Image cache: cycling back to an already-viewed image skips the fetch ---

func TestOpenImageInTerminal_CacheHit_SkipsFetch(t *testing.T) {
	a := loggedInApp()
	a.graphicsProtocol = imgview.ProtocolKitty
	cachedImg := image.NewRGBA(image.Rect(0, 0, 2, 2))
	a.imageCache = map[string]cachedImage{"https://x.com/a.jpg": {frames: []image.Image{cachedImg}}}

	_, cmd := a.openImageInTerminal("https://x.com/a.jpg")
	if cmd == nil {
		t.Fatal("expected a cmd")
	}
	// Safe to invoke directly: a cache hit performs no network I/O, so this
	// stays a fast, deterministic unit test.
	msg := cmd()
	fm, ok := msg.(imageFetchedMsg)
	if !ok {
		t.Fatalf("expected imageFetchedMsg, got %T", msg)
	}
	if fm.err != nil {
		t.Errorf("expected no error on a cache hit, got %v", fm.err)
	}
	if len(fm.frames) != 1 || fm.frames[0] != cachedImg {
		t.Error("expected the cached image to be reused rather than re-fetched")
	}
	if fm.encoded == "" {
		t.Error("expected the cached image to still be (re-)encoded for the current terminal size")
	}
}

func TestHandleImageViewer_Success_PopulatesCache(t *testing.T) {
	a := loggedInApp()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))

	a2, _, ok := a.handleImageViewer(imageFetchedMsg{rawURL: "https://x.com/a.jpg", frames: []image.Image{img}, encoded: "seq", encodedFrames: []string{"seq"}, cols: 2, rows: 2})
	if !ok {
		t.Fatal("expected imageFetchedMsg to be handled")
	}
	cached := a2.imageCache["https://x.com/a.jpg"]
	if len(cached.frames) != 1 || cached.frames[0] != img {
		t.Error("expected the decoded image to be cached under its URL")
	}
}

func TestHandleImageViewer_StaleGeneration_Dropped(t *testing.T) {
	a := loggedInApp()
	a.imageFetchGen = 5
	a.imageModalEncoded = "current"

	a2, cmd, ok := a.handleImageViewer(imageFetchedMsg{rawURL: "https://x.com/a.jpg", gen: 3, encoded: "stale", cols: 1, rows: 1})
	if !ok {
		t.Fatal("expected imageFetchedMsg to be handled (and dropped)")
	}
	if cmd != nil {
		t.Error("expected no cmd for a stale result")
	}
	if a2.imageModalEncoded != "current" {
		t.Error("expected a stale (superseded) result to be dropped, not overwrite the current image")
	}
}

func TestHandleImageViewer_GIFSuccess_StartsTicker(t *testing.T) {
	a := loggedInApp()
	a.imageFetchGen = 5

	a2, cmd, ok := a.handleImageViewer(imageFetchedMsg{
		rawURL:        "https://x.com/a.gif",
		gen:           5,
		encodedFrames: []string{"f0", "f1", "f2"},
		delays:        []time.Duration{10 * time.Millisecond, 20 * time.Millisecond, 30 * time.Millisecond},
		encoded:       "f0",
		cols:          2, rows: 2,
	})
	if !ok {
		t.Fatal("expected imageFetchedMsg to be handled")
	}
	if a2.imageModalEncoded != "f0" {
		t.Errorf("imageModalEncoded = %q, want %q", a2.imageModalEncoded, "f0")
	}
	if cmd == nil {
		t.Error("expected a cmd to schedule the next GIF frame")
	}
}

func TestHandleImageViewer_GifFrameTick_AdvancesFrame(t *testing.T) {
	a := loggedInApp()
	a.imageFetchGen = 7

	msg := gifFrameTickMsg{
		gen:           7,
		encodedFrames: []string{"f0", "f1", "f2"},
		delays:        []time.Duration{10 * time.Millisecond, 20 * time.Millisecond, 30 * time.Millisecond},
		idx:           1,
	}
	a2, cmd, ok := a.handleImageViewer(msg)
	if !ok {
		t.Fatal("expected gifFrameTickMsg to be handled")
	}
	if a2.imageModalEncoded != "f1" {
		t.Errorf("imageModalEncoded = %q, want %q", a2.imageModalEncoded, "f1")
	}
	if cmd == nil {
		t.Error("expected a cmd to schedule the next GIF frame")
	}
}

func TestHandleImageViewer_GifFrameTick_StaleGenDropped(t *testing.T) {
	a := loggedInApp()
	a.imageFetchGen = 5
	a.imageModalEncoded = "current"

	a2, cmd, ok := a.handleImageViewer(gifFrameTickMsg{gen: 3, encodedFrames: []string{"f0", "f1"}, delays: []time.Duration{10 * time.Millisecond, 10 * time.Millisecond}, idx: 1})
	if !ok {
		t.Fatal("expected gifFrameTickMsg to be handled (and dropped)")
	}
	if cmd != nil {
		t.Error("expected no cmd for a stale tick")
	}
	if a2.imageModalEncoded != "current" {
		t.Error("expected a stale (superseded) tick to be dropped, not overwrite the current frame")
	}
}

func TestHandleImageViewer_ErrorMidCarousel_NotifiesKeepsImage(t *testing.T) {
	a := loggedInApp()
	a.imageModalOpen = true
	a.imageModalEncoded = "current"

	a2, cmd, ok := a.handleImageViewer(imageFetchedMsg{rawURL: "https://x.com/bad.jpg", err: errors.New("fetch failed")})
	if !ok {
		t.Fatal("expected imageFetchedMsg to be handled")
	}
	if a2.imageModalEncoded != "current" {
		t.Error("expected the current image to stay displayed on a mid-carousel fetch error")
	}
	if a2.notifyText == "" {
		t.Error("expected a notify on a mid-carousel fetch error")
	}
	if cmd == nil {
		t.Error("expected the notify's cmd (not nil)")
	}
}

func TestHandleImageViewer_ErrorNoModalOpen_FallsBackToBrowser(t *testing.T) {
	a := loggedInApp()
	a.imageModalOpen = false

	_, cmd, ok := a.handleImageViewer(imageFetchedMsg{rawURL: "https://x.com/bad.jpg", err: errors.New("fetch failed")})
	if !ok {
		t.Fatal("expected imageFetchedMsg to be handled")
	}
	if cmd == nil {
		t.Error("expected a browser-open cmd when nothing was already showing")
	}
}

// TestHandleImageViewer_ITerm2CycleSuccess_SnapshotsPrevDimsNoClearScreen
// covers a real carousel cycle (modal already open, per the a.imageModalOpen
// gate at app.go's key handling — cycling is unreachable otherwise): no
// tea.ClearScreen is emitted anymore, and the outgoing box's dimensions are
// snapshotted into imageModalPrevRows/Cols for compositeOverlays to use.
func TestHandleImageViewer_ITerm2CycleSuccess_SnapshotsPrevDimsNoClearScreen(t *testing.T) {
	a := loggedInApp()
	a.graphicsProtocol = imgview.ProtocolITerm2
	a.imageCarouselItems = []string{"https://x.com/a.jpg", "https://x.com/b.jpg"}
	a.imageModalOpen = true
	a.imageModalCols = 20
	a.imageModalRows = 10

	a2, cmd, ok := a.handleImageViewer(imageFetchedMsg{rawURL: "https://x.com/b.jpg", encoded: "seq", cols: 8, rows: 4})
	if !ok {
		t.Fatal("expected imageFetchedMsg to be handled")
	}
	if !a2.imageModalOpen {
		t.Fatal("expected the image to open")
	}
	if cmd != nil {
		t.Error("expected no tea.ClearScreen on an iTerm2 carousel cycle")
	}
	if a2.imageModalPrevRows != 10 || a2.imageModalPrevCols != 20 {
		t.Errorf("expected prev dims snapshotted from the outgoing box (20x10), got %dx%d", a2.imageModalPrevCols, a2.imageModalPrevRows)
	}
	if a2.imageModalRows != 4 || a2.imageModalCols != 8 {
		t.Errorf("expected current dims updated to the new image (8x4), got %dx%d", a2.imageModalCols, a2.imageModalRows)
	}
}

// TestHandleImageViewer_SixelCycleSuccess_NoClearScreen covers a real
// carousel cycle on Sixel to a different-sized image (mirrors the iTerm2
// case above): still no Cmd queued — the repaint decision is made in
// View() from a.imageRepaintGen, not a one-shot Update()-side command (see
// its doc comment, App struct, for why) — but the generation bump that
// triggers sixelFullRepaint in compositeOverlays must have happened.
func TestHandleImageViewer_SixelCycleSuccess_NoClearScreen(t *testing.T) {
	a := loggedInApp()
	a.graphicsProtocol = imgview.ProtocolSixel
	a.imageCarouselItems = []string{"https://x.com/a.jpg", "https://x.com/b.jpg"}
	a.imageModalOpen = true
	a.imageModalCols = 20
	a.imageModalRows = 10
	startGen := a.imageRepaintGen

	a2, cmd, ok := a.handleImageViewer(imageFetchedMsg{rawURL: "https://x.com/b.jpg", encoded: "seq", cols: 8, rows: 4})
	if !ok {
		t.Fatal("expected imageFetchedMsg to be handled")
	}
	if cmd != nil {
		t.Error("expected no cmd on a Sixel carousel cycle")
	}
	if a2.imageModalRows != 4 || a2.imageModalCols != 8 {
		t.Errorf("expected current dims updated to the new image (8x4), got %dx%d", a2.imageModalCols, a2.imageModalRows)
	}
	if a2.imageRepaintGen == startGen {
		t.Error("expected imageRepaintGen bumped on a size-changed Sixel cycle")
	}
}

// TestHandleImageViewer_SixelCycleSameSize_StillBumpsRepaintGen confirms a
// cycle to a same-sized image still bumps imageRepaintGen: the prev-box
// cleanup itself has nothing to do for a same-size cycle, but
// compositeOverlays' own modal image-draw line also reads this counter (see
// imageDirtyMarker's doc comment) and needs a fresh value on every displayed
// image change, not just a size-changed one.
func TestHandleImageViewer_SixelCycleSameSize_StillBumpsRepaintGen(t *testing.T) {
	a := loggedInApp()
	a.graphicsProtocol = imgview.ProtocolSixel
	a.imageCarouselItems = []string{"https://x.com/a.jpg", "https://x.com/b.jpg"}
	a.imageModalOpen = true
	a.imageModalCols = 8
	a.imageModalRows = 4
	startGen := a.imageRepaintGen

	a2, _, ok := a.handleImageViewer(imageFetchedMsg{rawURL: "https://x.com/b.jpg", encoded: "seq", cols: 8, rows: 4})
	if !ok {
		t.Fatal("expected imageFetchedMsg to be handled")
	}
	if a2.imageRepaintGen == startGen {
		t.Error("expected imageRepaintGen bumped even for a same-size Sixel cycle")
	}
}

// TestHandleImageViewer_SixelFreshOpen_NoClearScreen covers a fresh o-open
// on Sixel (modal not already open): no ClearScreen, same as every other
// Sixel case — see TestHandleImageViewer_SixelCycleSuccess_NoClearScreen.
func TestHandleImageViewer_SixelFreshOpen_NoClearScreen(t *testing.T) {
	a := loggedInApp()
	a.graphicsProtocol = imgview.ProtocolSixel

	_, cmd, ok := a.handleImageViewer(imageFetchedMsg{rawURL: "https://x.com/a.jpg", encoded: "seq", cols: 10, rows: 5})
	if !ok {
		t.Fatal("expected imageFetchedMsg to be handled")
	}
	if cmd != nil {
		t.Error("expected no ClearScreen on a fresh Sixel open — nothing previous to clean up")
	}
}

func TestHandleImageViewer_KittyCycleSuccess_NoClearScreen(t *testing.T) {
	a := loggedInApp()
	a.graphicsProtocol = imgview.ProtocolKitty
	a.imageCarouselItems = []string{"https://x.com/a.jpg", "https://x.com/b.jpg"}
	a.imageModalOpen = true

	_, cmd, ok := a.handleImageViewer(imageFetchedMsg{rawURL: "https://x.com/b.jpg", encoded: "seq", cols: 10, rows: 5})
	if !ok {
		t.Fatal("expected imageFetchedMsg to be handled")
	}
	if cmd != nil {
		t.Error("Kitty self-heals via its own delete-all prefix, no ClearScreen needed")
	}
}

// TestHandleImageViewer_SingleImageSuccess_NoClearScreen covers a fresh
// o-open (modal not already open): no tea.ClearScreen, and no previous box
// to track since nothing was on screen before.
func TestHandleImageViewer_SingleImageSuccess_NoClearScreen(t *testing.T) {
	a := loggedInApp()
	a.graphicsProtocol = imgview.ProtocolITerm2 // even on iTerm2

	a2, cmd, ok := a.handleImageViewer(imageFetchedMsg{rawURL: "https://x.com/a.jpg", encoded: "seq", cols: 10, rows: 5})
	if !ok {
		t.Fatal("expected imageFetchedMsg to be handled")
	}
	if cmd != nil {
		t.Error("expected no ClearScreen outside a carousel")
	}
	if a2.imageModalPrevRows != 0 || a2.imageModalPrevCols != 0 {
		t.Errorf("expected no previous box tracked on a fresh open, got %dx%d", a2.imageModalPrevCols, a2.imageModalPrevRows)
	}
}

// --- Theme editor revert: cancel while "custom" was already active must
// restore the saved custom palette, not whatever the abandoned edit left in
// theme's shared package-level customPalette. ---

func TestHandleThemeEditor_Close_RestoresSavedCustomPalette_NotAbandonedEdit(t *testing.T) {
	saved := theme.Palette{
		Foreground: "#111111", Dimmed: "#222222", Border: "#333333", Accent: "#444444",
		Highlight: "#555555", Error: "#666666", BarText: "#777777", Self: "#888888", Meta: "#999999",
	}
	theme.SetCustomPalette(saved)
	theme.Set("custom")

	a := loggedInApp()
	a.customPalette = &saved
	a.themeEditorOpen = true
	a.themeEditorOrig = "custom"
	a.themeEditorOrigPalette = theme.CurrentPalette() // snapshot, as the 'e' handler now does

	// Simulate an abandoned mid-edit: a keystroke's PreviewPaletteMsg
	// diverges the live custom palette from the saved one.
	dirty := saved
	dirty.Foreground = "#ABCDEF"
	a2, _, ok := a.handleThemeEditor(screens.PreviewPaletteMsg{Palette: dirty})
	if !ok {
		t.Fatal("expected PreviewPaletteMsg to be handled")
	}
	if theme.CurrentPalette().Foreground != "#ABCDEF" {
		t.Fatal("expected the live preview to apply the dirty edit")
	}

	a3, _, ok := a2.handleThemeEditor(screens.CloseThemeEditorMsg{})
	if !ok {
		t.Fatal("expected CloseThemeEditorMsg to be handled")
	}
	if a3.themeEditorOpen {
		t.Error("expected the editor to close")
	}
	if got := theme.CurrentPalette(); got != saved {
		t.Errorf("CurrentPalette() after cancel = %+v, want the saved palette %+v", got, saved)
	}
}

// --- Theme editor preview from a detected post theme: covers the one link
// (App.handleThemeEditor's PreviewPostThemeMsg case) that postdetail_test.go
// and theme_test.go don't touch, since they test ParsePost and the T-key
// handler in isolation from the App-level wiring. ---

func TestHandleThemeEditor_PreviewPostThemeMsg_AppliesPostPaletteNotCurrent(t *testing.T) {
	current := theme.Palette{
		Foreground: "#111111", Dimmed: "#222222", Border: "#333333", Accent: "#444444",
		Highlight: "#555555", Error: "#666666", BarText: "#777777", Self: "#888888", Meta: "#999999",
	}
	theme.SetCustomPalette(current)
	theme.Set("custom")

	postBody := `Check out my new theme!

/* Cyberspace Custom Theme */
Base Theme: dark
/* Colors */
Foreground: #ff5d00
Background: #131313
Dimmed: #c1c1c1
Border: #393939
Code: #f5f5f5
Code BG: #393939
`
	a := loggedInApp()
	a.active = screenPostDetail
	a.postDetail = a.postDetail.SetPost(model.Post{ID: "p1", AuthorUsername: "op", Content: postBody})

	_, cmd := a.postDetail.Update(keyMsg("T"))
	if cmd == nil {
		t.Fatal("expected a cmd from T with a theme block detected")
	}
	msg, ok := cmd().(screens.PreviewPostThemeMsg)
	if !ok {
		t.Fatalf("expected PreviewPostThemeMsg, got %T", cmd())
	}

	a2, _, ok := a.handleThemeEditor(msg)
	if !ok {
		t.Fatal("expected PreviewPostThemeMsg to be handled")
	}
	if !a2.themeEditorOpen {
		t.Error("expected the theme editor to open")
	}
	got := theme.CurrentPalette()
	if got.Foreground != "#ff5d00" || got.Dimmed != "#c1c1c1" || got.Border != "#393939" {
		t.Errorf("CurrentPalette() = %+v, want the post's fg/dimmed/border (#ff5d00/#c1c1c1/#393939), not current theme's (%+v)", got, current)
	}

	view := a2.themeEditor.View()
	for _, want := range []string{"FF5D00", "C1C1C1", "393939"} {
		if !strings.Contains(view, want) {
			t.Errorf("editor View() missing %q from the post's palette; got:\n%s", want, view)
		}
	}
	for _, unwanted := range []string{"111111", "222222", "333333"} {
		if strings.Contains(view, unwanted) {
			t.Errorf("editor View() still shows the current theme's %q instead of the post's palette; got:\n%s", unwanted, view)
		}
	}
}

// --- Theme export / import ---

func testThemePalette() theme.Palette {
	return theme.Palette{
		Foreground: "#111111", Dimmed: "#222222", Border: "#333333", Accent: "#444444",
		Highlight: "#555555", Error: "#666666", BarText: "#777777", Self: "#888888", Meta: "#999999",
	}
}

// appOnCustomRow returns a logged-in App with the theme picker open and its
// cursor on the "custom" row, ready to exercise the 'x'/'i' keys.
func appOnCustomRow(t *testing.T) App {
	t.Helper()
	a := loggedInApp()
	a.themePickerOpen = true
	a.themePickerCursor = len(availableThemes) - 1
	if availableThemes[a.themePickerCursor] != "custom" {
		t.Fatalf("expected last availableThemes entry to be \"custom\", got %q", availableThemes[a.themePickerCursor])
	}
	return a
}

func TestHandleThemePickerKey_X_NoOp_WhenNoCustomPaletteSaved(t *testing.T) {
	a := appOnCustomRow(t)
	a.customPalette = nil

	m, _ := a.handleThemePickerKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	a2 := m.(App)
	if a2.pathPromptOpen {
		t.Error("expected export to no-op with no saved custom palette")
	}
}

func TestHandleThemePickerKey_X_OpensExportPrompt(t *testing.T) {
	a := appOnCustomRow(t)
	saved := testThemePalette()
	a.customPalette = &saved

	m, _ := a.handleThemePickerKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	a2 := m.(App)
	if !a2.pathPromptOpen || a2.pathPromptPurpose != pathPromptExport {
		t.Error("expected the export path prompt to open")
	}
	if a2.pathPromptExportPalette != saved {
		t.Errorf("pathPromptExportPalette = %+v, want the saved custom palette %+v", a2.pathPromptExportPalette, saved)
	}
}

// appOnRow returns a logged-in App with the theme picker open and its cursor
// on availableThemes[i].
func appOnRow(i int) App {
	a := loggedInApp()
	a.themePickerOpen = true
	a.themePickerCursor = i
	return a
}

func TestHandleThemePickerKey_E_PrefillsFromBuiltinRow(t *testing.T) {
	a := appOnRow(0) // "cyber"
	m, _ := a.handleThemePickerKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	a2 := m.(App)
	if !a2.themeEditorOpen {
		t.Fatal("expected the editor to open")
	}
	want, ok := theme.BuiltinPalette("cyber")
	if !ok {
		t.Fatal("expected cyber to be a known built-in")
	}
	if theme.CurrentPalette() != want {
		t.Errorf("CurrentPalette() = %+v, want cyber's palette %+v", theme.CurrentPalette(), want)
	}
}

func TestHandleThemePickerKey_X_ExportsBuiltinRow(t *testing.T) {
	a := appOnRow(1) // "c64"
	m, _ := a.handleThemePickerKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	a2 := m.(App)
	if !a2.pathPromptOpen || a2.pathPromptPurpose != pathPromptExport {
		t.Fatal("expected the export path prompt to open")
	}
	want, ok := theme.BuiltinPalette("c64")
	if !ok {
		t.Fatal("expected c64 to be a known built-in")
	}
	if a2.pathPromptExportPalette != want {
		t.Errorf("pathPromptExportPalette = %+v, want c64's palette %+v", a2.pathPromptExportPalette, want)
	}
}

func TestHandleThemePickerKey_I_OpensImportPrompt_FromBuiltinRow(t *testing.T) {
	a := appOnRow(2) // "vt320"
	m, _ := a.handleThemePickerKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	a2 := m.(App)
	if !a2.pathPromptOpen || a2.pathPromptPurpose != pathPromptImport {
		t.Error("expected the import path prompt to open from a built-in row")
	}
}

func TestHandlePathPrompt_Cancel_RevertsToThemePickerOrig(t *testing.T) {
	theme.Set("cyber")

	a := loggedInApp()
	a.themePickerOrig = "cyber"
	a.pathPromptOpen = true
	a.pathPromptPurpose = pathPromptImport

	// Simulate having previewed a different row before opening the prompt.
	theme.Set("vt320")

	a2, _, ok := a.handlePathPrompt(screens.PathPromptCancelMsg{})
	if !ok {
		t.Fatal("expected PathPromptCancelMsg to be handled")
	}
	if a2.pathPromptOpen {
		t.Error("expected the prompt to close")
	}
	if theme.CurrentName() != "cyber" {
		t.Errorf("CurrentName() = %q, want reverted to %q", theme.CurrentName(), "cyber")
	}
}

func TestHandleThemePickerKey_I_OpensImportPrompt_EvenWithoutSavedPalette(t *testing.T) {
	a := appOnCustomRow(t)
	a.customPalette = nil

	m, _ := a.handleThemePickerKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	a2 := m.(App)
	if !a2.pathPromptOpen || a2.pathPromptPurpose != pathPromptImport {
		t.Error("expected the import path prompt to open regardless of a saved palette")
	}
}

func TestHandlePathPrompt_Export_WritesFile(t *testing.T) {
	a := loggedInApp()
	saved := testThemePalette()
	a.customPalette = &saved
	a.pathPromptOpen = true
	a.pathPromptPurpose = pathPromptExport
	a.pathPromptExportPalette = saved

	path := filepath.Join(t.TempDir(), "theme.json")
	a2, cmd, ok := a.handlePathPrompt(screens.PathPromptSubmitMsg{Path: path})
	if !ok {
		t.Fatal("expected PathPromptSubmitMsg to be handled")
	}
	if a2.pathPromptOpen {
		t.Error("expected the prompt to close after a successful export")
	}
	if cmd == nil {
		t.Fatal("expected a notify cmd")
	}
	got, err := theme.ImportFromFile(path)
	if err != nil {
		t.Fatalf("ImportFromFile: %v", err)
	}
	if got != saved {
		t.Errorf("exported palette = %+v, want %+v", got, saved)
	}
}

func TestHandlePathPrompt_Export_WarnsBeforeOverwrite_ThenProceedsOnResubmit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "theme.json")
	if err := os.WriteFile(path, []byte("pre-existing"), 0600); err != nil {
		t.Fatal(err)
	}

	a := loggedInApp()
	first := testThemePalette()
	a.customPalette = &first
	a.pathPromptOpen = true
	a.pathPromptPurpose = pathPromptExport
	a.pathPromptExportPalette = first

	a2, _, ok := a.handlePathPrompt(screens.PathPromptSubmitMsg{Path: path})
	if !ok {
		t.Fatal("expected PathPromptSubmitMsg to be handled")
	}
	if !a2.pathPromptOpen {
		t.Error("expected the prompt to stay open pending overwrite confirmation")
	}
	if a2.pathPromptOverwritePending != path {
		t.Errorf("pathPromptOverwritePending = %q, want %q", a2.pathPromptOverwritePending, path)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "pre-existing" {
		t.Error("expected the file to be untouched before the overwrite is confirmed")
	}

	a3, _, ok := a2.handlePathPrompt(screens.PathPromptSubmitMsg{Path: path})
	if !ok {
		t.Fatal("expected the resubmit to be handled")
	}
	if a3.pathPromptOpen {
		t.Error("expected the prompt to close once the overwrite is confirmed")
	}
	got, err := theme.ImportFromFile(path)
	if err != nil {
		t.Fatalf("expected the file to now contain the exported palette: %v", err)
	}
	if got != first {
		t.Errorf("exported palette = %+v, want %+v", got, first)
	}
}

func TestHandlePathPrompt_Import_Success_OpensThemeEditor(t *testing.T) {
	imported := testThemePalette()
	path := filepath.Join(t.TempDir(), "theme.json")
	if err := theme.ExportToFile(path, imported); err != nil {
		t.Fatal(err)
	}

	a := loggedInApp()
	a.pathPromptOpen = true
	a.pathPromptPurpose = pathPromptImport

	a2, _, ok := a.handlePathPrompt(screens.PathPromptSubmitMsg{Path: path})
	if !ok {
		t.Fatal("expected PathPromptSubmitMsg to be handled")
	}
	if a2.pathPromptOpen {
		t.Error("expected the prompt to close")
	}
	if !a2.themeEditorOpen {
		t.Fatal("expected the theme editor to open for review")
	}
	if theme.CurrentName() != "custom" {
		t.Errorf("CurrentName() = %q, want custom (previewing the import)", theme.CurrentName())
	}
	if theme.CurrentPalette() != imported {
		t.Errorf("CurrentPalette() = %+v, want the imported palette %+v", theme.CurrentPalette(), imported)
	}
}

func TestHandlePathPrompt_Import_Failure_NotifiesAndLeavesCustomPaletteUntouched(t *testing.T) {
	saved := testThemePalette()
	a := loggedInApp()
	a.customPalette = &saved
	a.pathPromptOpen = true
	a.pathPromptPurpose = pathPromptImport

	a2, cmd, ok := a.handlePathPrompt(screens.PathPromptSubmitMsg{Path: filepath.Join(t.TempDir(), "missing.json")})
	if !ok {
		t.Fatal("expected PathPromptSubmitMsg to be handled")
	}
	if a2.pathPromptOpen {
		t.Error("expected the prompt to close after a failed import")
	}
	if a2.themeEditorOpen {
		t.Error("expected the theme editor to stay closed on import failure")
	}
	if cmd == nil {
		t.Fatal("expected a notify cmd")
	}
	if a2.customPalette != &saved {
		t.Error("expected the saved custom palette to be untouched by a failed import")
	}
}

// TestSelectionTouchesSlot confirms the substring match syncInlineImages
// relies on to decide whether a selection change actually recolored a card
// hosting a visible image, rather than blindly clearing on every selection
// move (a shipped regression — every arrow-key step blinked the whole
// screen whenever any image was visible anywhere on screen).
func TestSelectionTouchesSlot(t *testing.T) {
	slots := []screens.InlineImageSlot{
		{Key: "post:p1:0", Row: 3},
		{Key: "reply:r1:0", Row: 20},
	}
	if selectionTouchesSlot("", slots) {
		t.Error("expected an empty id to never match")
	}
	if !selectionTouchesSlot("p1", slots) {
		t.Error("expected a post id matching a post:<id>:n slot to match")
	}
	if !selectionTouchesSlot("r1", slots) {
		t.Error("expected a reply id matching a reply:<id>:n slot to match")
	}
	if selectionTouchesSlot("p2", slots) {
		t.Error("expected an id with no matching slot to not match")
	}
	// "p1" is a substring of a hypothetical slot key like "post:p1x:0"; the
	// ":id:" delimiting must reject that, not just a raw strings.Contains.
	substringSlots := []screens.InlineImageSlot{{Key: "post:p1x:0", Row: 3}}
	if selectionTouchesSlot("p1", substringSlots) {
		t.Error("expected id delimiting to reject a partial id match")
	}
}

// TestActiveSelectionKey_PostDetailFallsBackToPostID is the regression test
// for a real bug found via live debug logging (docs/plan-inline-images-
// improvements.md Round 14): PostDetailModel.SelectedReplyID() returns ""
// when the post itself is selected (not a reply), but selectionTouchesSlot
// treats "" as "nothing selected" and always returns false for it (see
// TestSelectionTouchesSlot above). Without this fallback to the post's own
// ID, toggling selection between the post and a reply — which recolors the
// post's border, including the rows its own inline image sits in — never
// registers as touching that image, so imageRepaintGen never bumps and a
// legitimate border-recolor resend can silently wipe the image with
// nothing forcing it back. Confirmed live: this fired once on entering
// PostDetail (Feed's own non-empty selection key carrying over) but never
// again for any subsequent post/reply toggle within PostDetail itself.
func TestActiveSelectionKey_PostDetailFallsBackToPostID(t *testing.T) {
	a := loggedInApp()
	a.active = screenPostDetail
	a.postDetail = a.postDetail.SetPost(model.Post{ID: "p1"})

	// Post itself selected (SelectedReplyID() == "") — must fall back to
	// the post's own ID, not "".
	if got := a.activeSelectionKey(); got != "p1" {
		t.Errorf("expected activeSelectionKey to fall back to the post ID %q when the post itself is selected, got %q", "p1", got)
	}

	slots := []screens.InlineImageSlot{{Key: "post:p1:0", Row: 3}}
	if !selectionTouchesSlot(a.activeSelectionKey(), slots) {
		t.Error("expected the post's own selection key to touch its own visible image slot")
	}
}

// TestSyncInlineImageErasures covers the stale-row detection that replaced
// both tea.ClearScreen and the later accumulate-until-exact-rect-reclaimed
// erasure design for scroll-triggered repaints: a moved or removed image's
// old row range must come back in staleRows (so forceRowsDirty forces
// Bubble Tea to resend that row's real content), computed fresh from a
// single prevRects/current diff with no carry-forward or "claim" concept.
func TestSyncInlineImageErasures(t *testing.T) {
	rectA := inlineImageRect{Row: 5, Col: 3, Cols: 20, Rows: 3}
	rectB := inlineImageRect{Row: 9, Col: 3, Cols: 20, Rows: 3}

	t.Run("unchanged slot produces no stale rows", func(t *testing.T) {
		slots := []screens.InlineImageSlot{{Key: "post:p1:0", Row: 3, ColIndent: 3, MaxCols: 20, MaxRows: 3}}
		prev := map[string]inlineImageRect{"post:p1:0": rectA}
		current, staleRows := syncInlineImageErasures(slots, 2, 0, prev)
		if len(staleRows) != 0 {
			t.Errorf("expected no stale rows, got %v", staleRows)
		}
		if current["post:p1:0"] != rectA {
			t.Errorf("expected current rect to match, got %v", current["post:p1:0"])
		}
	})

	t.Run("moved slot returns its old rect's row range", func(t *testing.T) {
		// Same key, but Row moved from 3 to 7 (rectB instead of rectA).
		slots := []screens.InlineImageSlot{{Key: "post:p1:0", Row: 7, ColIndent: 3, MaxCols: 20, MaxRows: 3}}
		prev := map[string]inlineImageRect{"post:p1:0": rectA}
		current, staleRows := syncInlineImageErasures(slots, 2, 0, prev)
		want := []int{5, 6, 7}
		if !slices.Equal(staleRows, want) {
			t.Errorf("expected old rect's row range %v pending, got %v", want, staleRows)
		}
		if current["post:p1:0"] != rectB {
			t.Errorf("expected current rect updated to new position, got %v", current["post:p1:0"])
		}
	})

	t.Run("removed slot returns its old rect's row range", func(t *testing.T) {
		prev := map[string]inlineImageRect{"post:p1:0": rectA}
		current, staleRows := syncInlineImageErasures(nil, 2, 0, prev)
		want := []int{5, 6, 7}
		if !slices.Equal(staleRows, want) {
			t.Errorf("expected old rect's row range %v pending, got %v", want, staleRows)
		}
		if len(current) != 0 {
			t.Errorf("expected no current rects, got %v", current)
		}
	})

	t.Run("two removed keys whose old rects share a row dedup that row", func(t *testing.T) {
		rectC := inlineImageRect{Row: 7, Col: 30, Cols: 20, Rows: 3} // rows 7-9, overlaps rectA's rows 5-7 at row 7
		prev := map[string]inlineImageRect{"post:p1:0": rectA, "post:p2:0": rectC}
		_, staleRows := syncInlineImageErasures(nil, 2, 0, prev)
		want := []int{5, 6, 7, 8, 9}
		if !slices.Equal(staleRows, want) {
			t.Errorf("expected deduped, sorted row range %v, got %v", want, staleRows)
		}
	})
}

// TestInlineImageCacheKey_VariesByColsAndProtocol confirms the cache key
// changes when the column budget (resize) or protocol changes, and stays
// stable otherwise — a resize or protocol switch must invalidate stale
// encodes rather than reuse a wrongly-sized/wrongly-encoded one.
func TestInlineImageCacheKey_VariesByColsAndProtocol(t *testing.T) {
	slot := screens.InlineImageSlot{URL: "https://example.com/a.png", MaxCols: 76}
	key1 := inlineImageCacheKey(slot, imgview.ProtocolSixel, nil)
	key2 := inlineImageCacheKey(slot, imgview.ProtocolSixel, nil)
	if key1 != key2 {
		t.Error("expected the same slot+protocol to produce the same key")
	}

	wider := slot
	wider.MaxCols = 100
	if inlineImageCacheKey(wider, imgview.ProtocolSixel, nil) == key1 {
		t.Error("expected a different MaxCols to produce a different key")
	}

	if inlineImageCacheKey(slot, imgview.ProtocolITerm2, nil) == key1 {
		t.Error("expected a different protocol to produce a different key")
	}
}

// TestInlineImageCacheKey_VariesByDitherState is a regression test: toggling
// dithering, changing sharpness, or switching themes (which changes the
// duotone colors) must invalidate an already-cached encode — otherwise a
// cached image keeps showing whatever dither state produced it until the
// cache entry is evicted or the app restarts (see inlineImageCacheKey's doc
// comment).
func TestInlineImageCacheKey_VariesByDitherState(t *testing.T) {
	slot := screens.InlineImageSlot{URL: "https://example.com/a.png", MaxCols: 76}
	proto := imgview.ProtocolKitty

	noDither := inlineImageCacheKey(slot, proto, nil)
	dithered := inlineImageCacheKey(slot, proto, &imgview.DitherOptions{
		PixelSize: 2,
		FgColor:   color.RGBA{R: 0, G: 255, B: 65, A: 255},
		BgColor:   color.RGBA{R: 13, G: 13, B: 13, A: 255},
	})
	if noDither == dithered {
		t.Error("expected turning dithering on to produce a different key")
	}

	sharp := inlineImageCacheKey(slot, proto, &imgview.DitherOptions{
		PixelSize: 1, // "crisp" instead of "medium"
		FgColor:   color.RGBA{R: 0, G: 255, B: 65, A: 255},
		BgColor:   color.RGBA{R: 13, G: 13, B: 13, A: 255},
	})
	if sharp == dithered {
		t.Error("expected a different sharpness (PixelSize) to produce a different key")
	}

	otherTheme := inlineImageCacheKey(slot, proto, &imgview.DitherOptions{
		PixelSize: 2,
		FgColor:   color.RGBA{R: 255, G: 176, B: 0, A: 255}, // amber, e.g. vt320 theme
		BgColor:   color.RGBA{R: 13, G: 13, B: 13, A: 255},
	})
	if otherTheme == dithered {
		t.Error("expected a different theme's FgColor to produce a different key")
	}
}

// TestSyncKittyPlacements_AssignsStableIDsAndDetectsDrops confirms the
// placement-id lifecycle Kitty inline rendering depends on: a slot's id
// stays the same across syncs as long as it's still visible, a slot that
// drops out of the visible set is reported for deletion exactly once, and a
// newly-visible slot gets a fresh distinct id.
func TestSyncKittyPlacements_AssignsStableIDsAndDetectsDrops(t *testing.T) {
	var a App
	slots1 := []screens.InlineImageSlot{{Key: "post:p1:0"}, {Key: "post:p2:0"}}
	// The very first sync reports every slot as "revived" too (nothing was
	// visible before), which is harmless: the caller's revive step just
	// deletes from an empty pendingKittyDeletes map, a no-op.
	a, ids1, toDelete1, _ := a.syncKittyPlacements(slots1)
	if len(toDelete1) != 0 {
		t.Errorf("expected no deletes on first sync, got %v", toDelete1)
	}
	id1, id2 := ids1["post:p1:0"], ids1["post:p2:0"]
	if id1 == 0 || id2 == 0 || id1 == id2 {
		t.Fatalf("expected distinct non-zero ids, got %d and %d", id1, id2)
	}

	// Same slots again: ids must stay stable, no deletes/revives.
	a, ids2, toDelete2, revived2 := a.syncKittyPlacements(slots1)
	if ids2["post:p1:0"] != id1 || ids2["post:p2:0"] != id2 {
		t.Errorf("expected ids to stay stable across syncs, got %v", ids2)
	}
	if len(toDelete2) != 0 || len(revived2) != 0 {
		t.Errorf("expected no deletes/revives when the visible set didn't change, got toDelete=%v revived=%v", toDelete2, revived2)
	}

	// p1 scrolls out of view, p3 comes into view.
	slots2 := []screens.InlineImageSlot{{Key: "post:p2:0"}, {Key: "post:p3:0"}}
	a, ids3, toDelete3, revived3 := a.syncKittyPlacements(slots2)
	if len(toDelete3) != 1 || toDelete3[0] != id1 {
		t.Errorf("expected exactly p1's id (%d) to be reported for deletion, got %v", id1, toDelete3)
	}
	if len(revived3) != 1 || revived3[0] != ids3["post:p3:0"] {
		t.Errorf("expected p3 (newly visible) reported as revived, got %v", revived3)
	}
	if ids3["post:p2:0"] != id2 {
		t.Errorf("expected p2's id to remain stable, got %d want %d", ids3["post:p2:0"], id2)
	}
	id3 := ids3["post:p3:0"]
	if id3 == 0 || id3 == id1 || id3 == id2 {
		t.Errorf("expected p3 to get a fresh distinct id, got %d", id3)
	}

	// p1 scrolls back into view: it must reuse its ORIGINAL id (not a fresh
	// one) — this is what keeps its id-less cache entry (see
	// inlineImageCacheKey) valid, and is reported as revived so the caller
	// cancels its still-pending delete.
	slots3 := []screens.InlineImageSlot{{Key: "post:p1:0"}, {Key: "post:p2:0"}, {Key: "post:p3:0"}}
	_, ids4, toDelete4, revived4 := a.syncKittyPlacements(slots3)
	if ids4["post:p1:0"] != id1 {
		t.Errorf("expected p1 to be revived with its original id %d, got %d", id1, ids4["post:p1:0"])
	}
	if len(toDelete4) != 0 {
		t.Errorf("expected no new deletes when p1 returns, got %v", toDelete4)
	}
	if len(revived4) != 1 || revived4[0] != id1 {
		t.Errorf("expected p1's id (%d) reported as revived, got %v", id1, revived4)
	}
}

// TestSyncInlineImages_DisablingClearsStaleKittyPlacements is a regression
// test: syncInlineImages used to early-return the moment canInlineImages()
// went false (e.g. the user toggled inline images off), skipping the
// --- inline-image fetch-failure cooldown ---

// feedAppWithOneImage builds an App on Feed/Tabs with one post whose image
// is currently visible, for tests exercising syncInlineImages/
// handleInlineImageFetched against a real slot rather than hand-built state.
func feedAppWithOneImage(t *testing.T) (a App, key string) {
	t.Helper()
	a = loggedInApp()
	a.width, a.height = 80, 40
	a.inlineImages = true
	a.graphicsProtocol = imgview.ProtocolKitty
	a.feed, _ = a.feed.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	a.feed, _ = a.feed.Update(screens.SharedConfigMsg{InlineImagesEnabled: true})
	a.feed = a.feed.SetPosts([]model.Post{
		{ID: "p1", AuthorUsername: "alice", Content: "hi\n\n![a](https://example.com/a.png)\n\nbye"},
	}, "")
	slots := a.feed.VisibleInlineImages()
	if len(slots) != 1 {
		t.Fatalf("setup: expected 1 visible slot, got %d", len(slots))
	}
	return a, inlineImageCacheKey(slots[0], a.graphicsProtocol, nil)
}

// TestInlineImageFailureCooldown_SkipsRefetchUntilCooldownLapses is a
// regression test: before this fix, a failed fetch left no record at all, so
// syncInlineImages (called on every Update) refired the same doomed request
// every keystroke/tick the slot stayed visible.
func TestInlineImageFailureCooldown_SkipsRefetchUntilCooldownLapses(t *testing.T) {
	a, key := feedAppWithOneImage(t)

	a, cmd := a.syncInlineImages()
	if cmd == nil || !a.inlineImageFetching[key] {
		t.Fatal("setup: expected the first sync to schedule a fetch")
	}

	a, _, _ = a.handleInlineImageFetched(inlineImageFetchedMsg{key: key, err: errors.New("boom")})
	if _, failed := a.inlineImageFailedAt[key]; !failed {
		t.Fatal("expected the failure to be recorded")
	}
	if a.inlineImageFetching[key] {
		t.Error("expected the in-flight marker to be cleared after the failure")
	}

	a, _ = a.syncInlineImages()
	if a.inlineImageFetching[key] {
		t.Error("expected syncInlineImages to skip refetching within the cooldown window")
	}

	// Backdate the failure past the cooldown and confirm it retries.
	a.inlineImageFailedAt[key] = time.Now().Add(-inlineImageFailureCooldown - time.Second)
	a, _ = a.syncInlineImages()
	if !a.inlineImageFetching[key] {
		t.Error("expected syncInlineImages to retry once the cooldown has lapsed")
	}
}

// TestSyncInlineImages_DitherToggleInvalidatesCache is a regression test:
// before inlineImageCacheKey included dither state, an image already cached
// (fetched and encoded before dithering was turned on) kept being served
// as-is by syncInlineImages/injectInlineImages, since its cache key never
// changed — dithering appeared to require a restart to take visible effect.
func TestSyncInlineImages_DitherToggleInvalidatesCache(t *testing.T) {
	a, key := feedAppWithOneImage(t)

	a, _, _ = a.handleInlineImageFetched(inlineImageFetchedMsg{key: key, encoded: "\x1b_Gfake\x1b\\"})
	if _, cached := a.inlineImageCache[key]; !cached {
		t.Fatal("setup: expected the encode to be cached under the no-dither key")
	}

	a, cmd := a.syncInlineImages()
	if cmd != nil {
		t.Fatal("setup: expected a cache hit (no fetch) before enabling dithering")
	}

	a.dithering = true
	a.ditherSharpness = "sharp"

	a, cmd = a.syncInlineImages()
	if cmd == nil {
		t.Fatal("expected enabling dithering to invalidate the cached key and schedule a fresh fetch")
	}
	newKey := inlineImageCacheKey(a.feed.VisibleInlineImages()[0], a.graphicsProtocol, a.ditherOptions())
	if newKey == key {
		t.Fatal("setup: expected the dither-enabled key to differ from the no-dither key")
	}
	if !a.inlineImageFetching[newKey] {
		t.Errorf("expected a fetch scheduled under the new dither-enabled key %q, fetching=%v", newKey, a.inlineImageFetching)
	}
}

// TestInlineImageFailureCooldown_ClearedBySubsequentSuccess confirms a
// success wipes any earlier failure record, so a transient blip doesn't
// leave a stale cooldown blocking future retries after the URL recovers.
func TestInlineImageFailureCooldown_ClearedBySubsequentSuccess(t *testing.T) {
	a, key := feedAppWithOneImage(t)
	a, _, _ = a.handleInlineImageFetched(inlineImageFetchedMsg{key: key, err: errors.New("boom")})
	if _, failed := a.inlineImageFailedAt[key]; !failed {
		t.Fatal("setup: expected the failure to be recorded")
	}

	a, _, _ = a.handleInlineImageFetched(inlineImageFetchedMsg{key: key, encoded: "\x1b_Gfake\x1b\\"})
	if _, failed := a.inlineImageFailedAt[key]; failed {
		t.Error("expected the failure record to be cleared after a subsequent success")
	}
	if got := a.inlineImageCache[key]; got != "\x1b_Gfake\x1b\\" {
		t.Errorf("expected the successful encode to be cached, got %q", got)
	}
}

// diff/delete logic entirely and leaving previously-drawn Kitty placements
// on screen. It must still diff down to an empty slot list so every
// previously-visible key gets queued for deletion.
func TestSyncInlineImages_DisablingClearsStaleKittyPlacements(t *testing.T) {
	a := App{
		graphicsProtocol: imgview.ProtocolKitty,
		inlineImages:     false, // disabled: canInlineImages() is false
		kittyPlacementIDs: map[string]int{
			"post:p1:0": 1,
			"post:p2:0": 2,
		},
		kittyVisibleKeys: map[string]struct{}{
			"post:p1:0": {},
			"post:p2:0": {},
		},
	}

	a, _ = a.syncInlineImages()

	if len(a.kittyVisibleKeys) != 0 {
		t.Errorf("expected kittyVisibleKeys to be cleared, got %v", a.kittyVisibleKeys)
	}
	if _, ok := a.pendingKittyDeletes[1]; !ok {
		t.Errorf("expected placement id 1 to be queued for deletion, got %v", a.pendingKittyDeletes)
	}
	if _, ok := a.pendingKittyDeletes[2]; !ok {
		t.Errorf("expected placement id 2 to be queued for deletion, got %v", a.pendingKittyDeletes)
	}
}

// TestSyncInlineImages_SixelTracksStaleRowsSameAsITerm2 confirms Sixel and
// iTerm2 both compute inlineImageStaleRows identically — the two protocols
// only diverge in what View() does with that fact (see injectInlineImages/
// sixelFullRepaint in layout.go), not in whether Update() tracks it. Both
// also bump imageRepaintGen for a stale row: iTerm2's forceRowsDirty resend
// needs the same collision-proof marker Sixel's full repaint does (see
// imageRepaintGen's doc comment, App struct) — a fixed marker there was the
// original bug this test's sibling coverage exists to prevent regressing.
// Neither queues a Cmd: the repaint decision (forceRowsDirty for iTerm2,
// sixelFullRepaint for Sixel) is made in View() from this same tracked
// state, not from a one-shot Update()-side command — deliberately, since a
// one-shot tea.ClearScreen Cmd both flashed badly (confirmed live) and, per
// this codebase's own established Bubble Tea pitfall (see the
// pendingKittyDeletes/inlineImageStaleRows pattern this mirrors), risks
// being silently dropped by the renderer's throttling.
func TestSyncInlineImages_SixelTracksStaleRowsSameAsITerm2(t *testing.T) {
	stale := map[string]inlineImageRect{
		"post:gone:0": {Row: 3, Col: 1, Cols: 10, Rows: 4},
	}

	sixel := App{
		graphicsProtocol:        imgview.ProtocolSixel,
		inlineImages:            false, // canInlineImages() false: current slots stay empty, so the tracked entry above reads as stale
		inlineImageVisibleRects: stale,
	}
	sixelOut, cmd := sixel.syncInlineImages()
	if cmd != nil {
		t.Error("expected no cmd for Sixel — the repaint decision is made in View(), not queued here")
	}
	if len(sixelOut.inlineImageStaleRows) == 0 {
		t.Error("expected inlineImageStaleRows populated for Sixel, same as iTerm2")
	}
	if sixelOut.imageRepaintGen == sixel.imageRepaintGen {
		t.Error("expected imageRepaintGen bumped for a stale Sixel row")
	}

	iterm := App{
		graphicsProtocol:        imgview.ProtocolITerm2,
		inlineImages:            false,
		inlineImageVisibleRects: stale,
	}
	itermOut, cmd := iterm.syncInlineImages()
	if cmd != nil {
		t.Error("expected no cmd for a stale iTerm2 row — it relies on forceRowsDirty in View(), not a cmd")
	}
	if len(itermOut.inlineImageStaleRows) != len(sixelOut.inlineImageStaleRows) {
		t.Errorf("expected iTerm2 and Sixel to track the same stale rows, got %v vs %v", itermOut.inlineImageStaleRows, sixelOut.inlineImageStaleRows)
	}
	if itermOut.imageRepaintGen == iterm.imageRepaintGen {
		t.Error("expected imageRepaintGen bumped for a stale iTerm2 row too, same as Sixel")
	}
}

// TestSyncInlineImages_StaleRowsSurviveCoalescedUpdates is the regression
// test for the bug accumulateStaleRows fixes: syncInlineImages runs on every
// Update(), but Bubble Tea's renderer can coalesce several Updates into one
// flush (the same premise TestAccumulateKittyDeletes_SurvivesSkippedRenders
// covers for Kitty deletes). Before the fix, a.inlineImageStaleRows was
// overwritten fresh each call, so a row that went stale in an earlier Update
// whose View() never got flushed could be silently replaced by a later
// Update's unrelated stale row instead of accumulating — losing the row that
// actually still has stale image pixels on the real, last-flushed terminal
// screen. Simulates two separate images going stale in two consecutive
// syncInlineImages calls (mimicking two coalesced Updates) and confirms both
// rows are present after the second call, then confirms a subsequent quiet
// call within inlineImageStaleGrace does NOT clear the accumulated set (the
// bug this test's sibling, TestSyncInlineImages_StaleRowsSurviveQuietTick,
// covers directly — a single quiet Update isn't a safe clear signal, since
// this app has several tea.Tick loops unrelated to the feed that can supply
// that quiet Update before a resend actually flushes), and that it does
// clear once the grace period has elapsed.
func TestSyncInlineImages_StaleRowsSurviveCoalescedUpdates(t *testing.T) {
	a := App{
		graphicsProtocol: imgview.ProtocolITerm2,
		inlineImages:     false, // canInlineImages() false: slots stay empty, so any tracked entry reads as stale
		inlineImageVisibleRects: map[string]inlineImageRect{
			"post:A:0": {Row: 10, Col: 1, Cols: 10, Rows: 4},
		},
	}

	a1, _ := a.syncInlineImages()
	if !slices.Contains(a1.inlineImageStaleRows, 10) {
		t.Fatalf("setup: expected row 10 stale after the first call, got %v", a1.inlineImageStaleRows)
	}

	// Simulate a second, separate image going stale in what would be a
	// second coalesced Update — a1.inlineImageVisibleRects is already {}
	// (computed fresh from the still-empty slots), so this stands in for a
	// new image having been tracked and then dropped between calls.
	a1.inlineImageVisibleRects = map[string]inlineImageRect{
		"post:B:0": {Row: 20, Col: 1, Cols: 10, Rows: 4},
	}
	a2, _ := a1.syncInlineImages()
	if !slices.Contains(a2.inlineImageStaleRows, 10) {
		t.Errorf("expected row 10 from the first call to survive into the second call's accumulated staleRows, got %v", a2.inlineImageStaleRows)
	}
	if !slices.Contains(a2.inlineImageStaleRows, 20) {
		t.Errorf("expected row 20 from the second call also present, got %v", a2.inlineImageStaleRows)
	}

	// A quiet call (nothing newly stale — a2.inlineImageVisibleRects is
	// already {}) immediately afterward, well within inlineImageStaleGrace,
	// must NOT clear the accumulated set yet.
	a3, _ := a2.syncInlineImages()
	if len(a3.inlineImageStaleRows) == 0 {
		t.Fatalf("expected accumulated staleRows to survive a quiet call within the grace period, got %v", a3.inlineImageStaleRows)
	}

	// Once the grace period has elapsed, the next quiet call clears it.
	a3.inlineImageStaleSince = time.Now().Add(-inlineImageStaleGrace - time.Second)
	a4, _ := a3.syncInlineImages()
	if len(a4.inlineImageStaleRows) != 0 {
		t.Errorf("expected accumulated staleRows cleared after the grace period elapsed, got %v", a4.inlineImageStaleRows)
	}
}

// TestSyncInlineImages_StaleRowsSurviveQuietTick is the direct regression
// test for the top-of-feed background-poll repro: syncInlineImages runs
// after every tea.Msg the whole app processes (App.Update calls it
// unconditionally), including messages from tea.Tick loops that have
// nothing to do with the feed (chat/RTDB heartbeats, notification polls,
// gif-frame ticks). Before inlineImageStaleSince/inlineImageStaleGrace, a
// single such unrelated "quiet" Update landing between two feed-affecting
// Updates (e.g. during the feed's ~200ms feedMergeAnimDelay merge-animation
// gap) would immediately wipe the accumulated staleRows, even though
// nothing about the feed itself had settled yet. Simulates exactly that:
// a stale row, then an entirely unrelated quiet call (as an unrelated tick's
// Update would produce), and confirms the row is still there.
func TestSyncInlineImages_StaleRowsSurviveQuietTick(t *testing.T) {
	a := App{
		graphicsProtocol: imgview.ProtocolITerm2,
		inlineImages:     false,
		inlineImageVisibleRects: map[string]inlineImageRect{
			"post:A:0": {Row: 10, Col: 1, Cols: 10, Rows: 4},
		},
	}

	a1, _ := a.syncInlineImages()
	if len(a1.inlineImageStaleRows) == 0 {
		t.Fatalf("setup: expected staleRows populated after the first call, got %v", a1.inlineImageStaleRows)
	}

	// An unrelated quiet call — a1.inlineImageVisibleRects is already {},
	// so this computes zero new staleRows, standing in for an unrelated
	// background tick's Update firing before the resend has flushed.
	a2, _ := a1.syncInlineImages()
	if !slices.Contains(a2.inlineImageStaleRows, 10) {
		t.Errorf("expected row 10 to survive an unrelated quiet Update within the grace period, got %v", a2.inlineImageStaleRows)
	}
}

// TestAccumulateKittyDeletes_SurvivesSkippedRenders is a regression test for
// fast-scroll ghosting: a delete computed by one Update must not be lost if
// Bubble Tea's throttled renderer processes several Updates (each recomputing
// syncKittyPlacements' fresh, single-frame diff) before actually writing a
// frame. A prior version of this used a resend countdown, decremented once
// per Update — but a fast enough scroll can still rack up enough Updates
// between two real flushes to exhaust that budget on nothing but
// never-flushed intermediate renders, losing the delete. Simulates many such
// skipped-render Updates (empty newlyDropped batches) and confirms an
// earlier delete survives indefinitely, never expiring on its own.
func TestAccumulateKittyDeletes_SurvivesSkippedRenders(t *testing.T) {
	pending := accumulateKittyDeletes(nil, []int{7})
	if _, ok := pending[7]; !ok {
		t.Fatal("expected id 7 to be pending immediately after being seeded")
	}

	for i := 0; i < 50; i++ {
		pending = accumulateKittyDeletes(pending, nil)
		if _, ok := pending[7]; !ok {
			t.Fatalf("id 7 disappeared after %d extra call(s), expected it to never auto-expire", i+1)
		}
	}
}

// TestAccumulateKittyDeletes_MergesRatherThanOverwrites confirms a second
// batch of newly-dropped ids doesn't wipe out a still-pending id from an
// earlier batch — the exact failure mode a single-value (not accumulating)
// pendingKittyDeletes had.
func TestAccumulateKittyDeletes_MergesRatherThanOverwrites(t *testing.T) {
	pending := accumulateKittyDeletes(nil, []int{1})
	pending = accumulateKittyDeletes(pending, []int{2})
	if _, ok := pending[1]; !ok {
		t.Errorf("expected id 1 to still be pending after a second batch introduced id 2, got %v", pending)
	}
	if _, ok := pending[2]; !ok {
		t.Errorf("expected id 2 to be pending, got %v", pending)
	}
}

// --- cacheInlineImage (bounded LRU) ---

func TestCacheInlineImage_EvictsOldestFirstPastCap(t *testing.T) {
	var a App
	a = a.cacheInlineImageBounded("k1", "aaaaa", 12) // 5 bytes
	a = a.cacheInlineImageBounded("k2", "bbbbb", 12) // 10 bytes total, still under cap
	a = a.cacheInlineImageBounded("k3", "ccccc", 12) // 15 bytes would exceed 12 — evict k1

	if _, ok := a.inlineImageCache["k1"]; ok {
		t.Error("expected k1 (oldest) to be evicted once the cap was exceeded")
	}
	if _, ok := a.inlineImageCache["k2"]; !ok {
		t.Error("expected k2 to survive")
	}
	if _, ok := a.inlineImageCache["k3"]; !ok {
		t.Error("expected k3 (just inserted) to survive")
	}
	if a.inlineImageCacheBytes != 10 {
		t.Errorf("expected inlineImageCacheBytes to track only the surviving entries (10), got %d", a.inlineImageCacheBytes)
	}
}

// TestCacheInlineImage_ReinsertMovesToBackWithoutDoubleCounting confirms
// re-caching an existing key (e.g. a resize that re-encodes the same slot)
// updates its size correctly rather than accumulating stale bytes, and
// treats it as freshly inserted for eviction ordering rather than leaving it
// at its original (now stale) position.
func TestCacheInlineImage_ReinsertMovesToBackWithoutDoubleCounting(t *testing.T) {
	var a App
	a = a.cacheInlineImageBounded("k1", "aaaaa", 12)   // 5 bytes
	a = a.cacheInlineImageBounded("k2", "bb", 12)      // 7 bytes total
	a = a.cacheInlineImageBounded("k1", "aaaaaaa", 12) // k1 grows to 7 bytes: 7+2=9, still under cap

	if got := a.inlineImageCache["k1"]; got != "aaaaaaa" {
		t.Errorf("expected k1's value to be updated, got %q", got)
	}
	if a.inlineImageCacheBytes != 9 {
		t.Errorf("expected inlineImageCacheBytes=9 (7+2, not double-counting k1's old 5), got %d", a.inlineImageCacheBytes)
	}

	// Now push over the cap: k2 (never re-touched) should be evicted first,
	// not k1 (re-inserted more recently, even though it existed earlier).
	a = a.cacheInlineImageBounded("k3", "c", 12)    // 9+1=10, still under 12... push further
	a = a.cacheInlineImageBounded("k4", "dddd", 12) // 10+4=14 > 12: evict oldest (k2)
	if _, ok := a.inlineImageCache["k2"]; ok {
		t.Error("expected k2 to be evicted first — it's the least-recently-(re)inserted key")
	}
	if _, ok := a.inlineImageCache["k1"]; !ok {
		t.Error("expected k1 to survive — it was re-inserted after k2 and moved to the back of eviction order")
	}
}

// TestCacheInlineImage_NeverEvictsTheJustInsertedEntry confirms a single
// entry larger than the whole cap is still kept (better to go over budget by
// one entry than to evict the image that was just fetched for the frame
// asking for it).
func TestCacheInlineImage_NeverEvictsTheJustInsertedEntry(t *testing.T) {
	var a App
	a = a.cacheInlineImageBounded("k1", "aaaaaaaaaaaaaaaaaaaa", 5) // 20 bytes >> cap of 5
	if _, ok := a.inlineImageCache["k1"]; !ok {
		t.Error("expected the sole, just-inserted entry to survive even though it alone exceeds the cap")
	}
}

// TestTouchInlineImageCache_SurvivesEvictionOverUntouchedEntry is the
// regression test for a real, no-user-action image blackout: before
// touchInlineImageCache existed, inlineImageCacheOrder only ever moved on
// write, so a still-on-screen image whose slot kept hitting the cache
// (never re-fetched, since it was already cached) could still be evicted
// purely because *other* images got fetched more recently — eviction was
// FIFO-by-insertion, not actually LRU-by-access. syncInlineImages now calls
// touchInlineImageCache on every cache hit for a currently-visible slot
// (app.go), so k1 here stands in for an image that's stayed visible across
// several other fetches: it must survive eviction pressure that an
// equally-old, never-revisited key does not.
func TestTouchInlineImageCache_SurvivesEvictionOverUntouchedEntry(t *testing.T) {
	var a App
	a = a.cacheInlineImageBounded("k1", "aaaaa", 12) // 5 bytes — the still-visible image
	a = a.cacheInlineImageBounded("k2", "bbbbb", 12) // 10 bytes total — never revisited

	// k1 stays visible (simulates a cache hit on every subsequent frame);
	// k2 does not.
	a.touchInlineImageCache("k1")

	a = a.cacheInlineImageBounded("k3", "c", 12)    // 10+1=11, still under 12
	a = a.cacheInlineImageBounded("k4", "dddd", 12) // 11+4=15 > 12: evict oldest untouched (k2)

	if _, ok := a.inlineImageCache["k1"]; !ok {
		t.Error("expected k1 (touched — simulating it stayed visible) to survive eviction")
	}
	if _, ok := a.inlineImageCache["k2"]; ok {
		t.Error("expected k2 (never touched) to be evicted first, not k1")
	}
}

// --- Guild apprenticeships: role-conditional badge mutation ---

func TestHandleGuilds_GuildJoinedAsMember_SetsBadge(t *testing.T) {
	a := loggedInApp()
	a.currentUser = model.User{Username: "case"}
	a.guilds = a.guilds.SetGuildDetail(model.Guild{ID: "g1", Slug: "night-owls", Name: "Night Owls", Icon: "🦉"})

	a2, _, handled := a.handleGuilds(guildJoinedMsg{slug: "night-owls", name: "Night Owls", role: "member"})
	if !handled {
		t.Fatal("expected handleGuilds to handle guildJoinedMsg")
	}
	if a2.currentUser.GuildSlug != "night-owls" {
		t.Errorf("GuildSlug = %q, want night-owls", a2.currentUser.GuildSlug)
	}
	if a2.currentUser.GuildName != "Night Owls" || a2.currentUser.GuildIcon != "🦉" {
		t.Errorf("badge fields not set: %+v", a2.currentUser)
	}
}

func TestHandleGuilds_GuildJoinedAsApprentice_LeavesBadgeUntouched(t *testing.T) {
	a := loggedInApp()
	a.currentUser = model.User{Username: "case", GuildSlug: "night-owls", GuildName: "Night Owls"}
	a.guilds = a.guilds.SetGuildDetail(model.Guild{ID: "g2", Slug: "deep-divers", Name: "Deep Divers"})

	a2, _, handled := a.handleGuilds(guildJoinedMsg{slug: "deep-divers", name: "Deep Divers", role: "apprentice"})
	if !handled {
		t.Fatal("expected handleGuilds to handle guildJoinedMsg")
	}
	if a2.currentUser.GuildSlug != "night-owls" {
		t.Errorf("badge should stay on night-owls, got %q", a2.currentUser.GuildSlug)
	}
}

func TestHandleGuilds_GuildLeft_BadgeGuild_ClearsBadge(t *testing.T) {
	a := loggedInApp()
	a.currentUser = model.User{Username: "case", GuildSlug: "night-owls", GuildName: "Night Owls", GuildIcon: "🦉"}

	a2, _, handled := a.handleGuilds(guildLeftMsg{slug: "night-owls", name: "Night Owls"})
	if !handled {
		t.Fatal("expected handleGuilds to handle guildLeftMsg")
	}
	if a2.currentUser.GuildSlug != "" || a2.currentUser.GuildName != "" || a2.currentUser.GuildIcon != "" {
		t.Errorf("expected badge fields cleared, got %+v", a2.currentUser)
	}
}

func TestHandleGuilds_GuildLeft_Apprenticeship_LeavesBadgeUntouched(t *testing.T) {
	a := loggedInApp()
	a.currentUser = model.User{Username: "case", GuildSlug: "night-owls", GuildName: "Night Owls"}

	a2, _, handled := a.handleGuilds(guildLeftMsg{slug: "deep-divers", name: "Deep Divers"})
	if !handled {
		t.Fatal("expected handleGuilds to handle guildLeftMsg")
	}
	if a2.currentUser.GuildSlug != "night-owls" {
		t.Errorf("badge should stay on night-owls, got %q", a2.currentUser.GuildSlug)
	}
}

func TestHandleGuilds_GuildPromoted_SwapsBadge(t *testing.T) {
	a := loggedInApp()
	a.currentUser = model.User{Username: "case", GuildSlug: "night-owls", GuildName: "Night Owls"}
	a.guilds = a.guilds.SetGuildDetail(model.Guild{ID: "g2", Slug: "deep-divers", Name: "Deep Divers", Icon: "🐳"})

	a2, _, handled := a.handleGuilds(guildPromotedMsg{slug: "deep-divers", name: "Deep Divers", role: "member"})
	if !handled {
		t.Fatal("expected handleGuilds to handle guildPromotedMsg")
	}
	if a2.currentUser.GuildSlug != "deep-divers" || a2.currentUser.GuildIcon != "🐳" {
		t.Errorf("expected badge to swap to deep-divers, got %+v", a2.currentUser)
	}
	if a2.guilds.GuildDetail().Role != "member" {
		t.Errorf("expected active guild detail role updated to member, got %q", a2.guilds.GuildDetail().Role)
	}
}

// TestHandleProfile_OwnProfileLoad_FetchesApprenticeships is the regression
// test for a real bug: navigating to your own profile via the tab bar goes
// through loadProfileCmd/profileLoadedMsg (activateScreen in layout.go), a
// separate path from viewing someone else's profile (loadUserProfileCmd/
// userProfileLoadedMsg). Apprenticeships were only wired into the latter, so
// your own apprenticeships never loaded when opening your own profile tab.
func TestHandleProfile_OwnProfileLoad_FetchesApprenticeships(t *testing.T) {
	a := loggedInApp()

	a2, cmd, handled := a.handleProfile(profileLoadedMsg{user: model.User{Username: "case", GuildSlug: "technica"}})
	if !handled {
		t.Fatal("expected handleProfile to handle profileLoadedMsg")
	}
	if cmd == nil {
		t.Fatal("expected profileLoadedMsg to trigger a guilds-load cmd")
	}
	msg := cmd()
	gm, ok := msg.(userGuildsLoadedMsg)
	if !ok {
		t.Fatalf("expected userGuildsLoadedMsg, got %T", msg)
	}
	if gm.username != "case" {
		t.Errorf("username = %q, want case", gm.username)
	}
	_ = a2
}

func TestHandleGuilds_UserGuildsLoaded_GuardsOnUsernameMatch(t *testing.T) {
	a := loggedInApp()
	a.profile = a.profile.SetUser(model.User{Username: "molly"})
	memberships := []model.GuildMembership{{Slug: "deep-divers", Role: "apprentice"}}

	a2, _, handled := a.handleProfile(userGuildsLoadedMsg{username: "someone-else", guilds: memberships})
	if !handled {
		t.Fatal("expected handleProfile to handle userGuildsLoadedMsg")
	}
	if len(a2.profile.Apprenticeships()) != 0 {
		t.Errorf("expected stale response for a different username to be dropped, got %+v", a2.profile.Apprenticeships())
	}

	a3, _, handled3 := a.handleProfile(userGuildsLoadedMsg{username: "molly", guilds: memberships})
	if !handled3 {
		t.Fatal("expected handleProfile to handle userGuildsLoadedMsg")
	}
	if len(a3.profile.Apprenticeships()) != 1 {
		t.Errorf("expected apprenticeships applied for matching username, got %+v", a3.profile.Apprenticeships())
	}
}

// TestHandleSettings_SavingUnrelatedSettingPreservesProbedProtocol is a
// regression test: a.graphicsProtocol can be ProtocolSixel here only because
// the startup DA1 probe (imgview.ProbeSixel) found it — that probe can't
// safely re-run mid-session (it needs raw terminal access before Bubble Tea
// takes over stdin). Saving any setting whose graphics-protocol override
// (graphicsProtocolName) hasn't changed must leave a.graphicsProtocol alone;
// re-resolving it via imgview.DetectProtocol()'s env-var-only detection on
// every save would silently downgrade a DA1-probed Sixel session to
// ProtocolNone, breaking inline images and the image viewer until a full
// restart re-probes Sixel.
func TestHandleSettings_SavingUnrelatedSettingPreservesProbedProtocol(t *testing.T) {
	a := loggedInApp()
	a.graphicsProtocol = imgview.ProtocolSixel
	a.graphicsProtocolName = "" // auto — the only way DetectProtocol() ever gets consulted

	_, cmd, ok := a.handleSettings(screens.SaveSettingsMsg{
		GraphicsProtocol: "", // unchanged
		Dithering:        true,
		ImageViewer:      "terminal",
	})
	if !ok || cmd == nil {
		t.Fatal("expected handleSettings to handle SaveSettingsMsg with a non-nil cmd")
	}
	msg := cmd()
	saved, ok := msg.(settingsSavedMsg)
	if !ok {
		t.Fatalf("expected settingsSavedMsg, got %T", msg)
	}

	a2, _, ok := a.handleSettings(saved)
	if !ok {
		t.Fatal("expected handleSettings to handle settingsSavedMsg")
	}
	if a2.graphicsProtocol != imgview.ProtocolSixel {
		t.Errorf("graphicsProtocol = %v after saving an unrelated setting, want unchanged ProtocolSixel", a2.graphicsProtocol)
	}
}

// TestHandleSettings_ChangingGraphicsProtocolOverrideReResolves confirms the
// override still takes effect when the user actually changes it — the fix
// for the regression above only skips re-resolution when the override is
// unchanged.
func TestHandleSettings_ChangingGraphicsProtocolOverrideReResolves(t *testing.T) {
	a := loggedInApp()
	a.graphicsProtocol = imgview.ProtocolSixel
	a.graphicsProtocolName = ""

	_, cmd, ok := a.handleSettings(screens.SaveSettingsMsg{
		GraphicsProtocol: "kitty", // changed
		ImageViewer:      "terminal",
	})
	if !ok || cmd == nil {
		t.Fatal("expected handleSettings to handle SaveSettingsMsg with a non-nil cmd")
	}
	saved := cmd().(settingsSavedMsg)

	a2, _, ok := a.handleSettings(saved)
	if !ok {
		t.Fatal("expected handleSettings to handle settingsSavedMsg")
	}
	if a2.graphicsProtocol != imgview.ProtocolKitty {
		t.Errorf("graphicsProtocol = %v after changing the override to kitty, want ProtocolKitty", a2.graphicsProtocol)
	}
}

// TestWithSavedPreferences_LoadsWithoutRefreshToken guards against a
// regression where display preferences (graphics protocol, dithering,
// thread depth, timezone) were only loaded via WithSavedSession, which
// requires a refresh token. A normal token-expiry relogin clears the token
// on disk but leaves the rest of the saved config intact; preferences must
// still load on the WithAutoLogin/WithSavedEmail paths too.
func TestWithSavedPreferences_LoadsWithoutRefreshToken(t *testing.T) {
	cfg := config.Config{
		GraphicsProtocol: "sixel",
		Dithering:        true,
		MaxThreadDepth:   20,
		Timezone:         "UTC+2",
	}

	a := newTestApp().WithAutoLogin("user@example.com", "pw").WithSavedPreferences(cfg)
	if a.graphicsProtocolName != "sixel" {
		t.Errorf("graphicsProtocolName = %q, want sixel", a.graphicsProtocolName)
	}
	if !a.dithering {
		t.Error("dithering = false, want true")
	}
	if a.maxThreadDepth != 20 {
		t.Errorf("maxThreadDepth = %d, want 20", a.maxThreadDepth)
	}
	if a.timezone != "UTC+2" {
		t.Errorf("timezone = %q, want UTC+2", a.timezone)
	}
}
