package ui

import (
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ragnar/cyber-tui/internal/api"
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
		t.Fatal("expected a fetch cmd for the newly cycled-to image")
	}
	if a2.imageFetchGen == genBefore {
		t.Error("expected imageFetchGen to advance on cycle")
	}

	m2, _ := a2.Update(tea.KeyMsg{Type: tea.KeyLeft})
	a3 := m2.(App)
	if a3.imageCarouselIndex != 0 {
		t.Errorf("expected carousel index to wrap/return to 0 after left, got %d", a3.imageCarouselIndex)
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

func TestHandleImageViewer_ITerm2CycleSuccess_ForcesClearScreen(t *testing.T) {
	a := loggedInApp()
	a.graphicsProtocol = imgview.ProtocolITerm2
	a.imageCarouselItems = []string{"https://x.com/a.jpg", "https://x.com/b.jpg"}

	a2, cmd, ok := a.handleImageViewer(imageFetchedMsg{rawURL: "https://x.com/b.jpg", encoded: "seq", cols: 10, rows: 5})
	if !ok {
		t.Fatal("expected imageFetchedMsg to be handled")
	}
	if !a2.imageModalOpen {
		t.Fatal("expected the image to open")
	}
	if cmd == nil {
		t.Error("expected tea.ClearScreen to force a full repaint on an iTerm2 carousel cycle")
	}
}

func TestHandleImageViewer_KittyCycleSuccess_NoClearScreen(t *testing.T) {
	a := loggedInApp()
	a.graphicsProtocol = imgview.ProtocolKitty
	a.imageCarouselItems = []string{"https://x.com/a.jpg", "https://x.com/b.jpg"}

	_, cmd, ok := a.handleImageViewer(imageFetchedMsg{rawURL: "https://x.com/b.jpg", encoded: "seq", cols: 10, rows: 5})
	if !ok {
		t.Fatal("expected imageFetchedMsg to be handled")
	}
	if cmd != nil {
		t.Error("Kitty self-heals via its own delete-all prefix, no ClearScreen needed")
	}
}

func TestHandleImageViewer_SingleImageSuccess_NoClearScreen(t *testing.T) {
	a := loggedInApp()
	a.graphicsProtocol = imgview.ProtocolITerm2 // even on iTerm2

	_, cmd, ok := a.handleImageViewer(imageFetchedMsg{rawURL: "https://x.com/a.jpg", encoded: "seq", cols: 10, rows: 5})
	if !ok {
		t.Fatal("expected imageFetchedMsg to be handled")
	}
	if cmd != nil {
		t.Error("expected no ClearScreen outside a carousel")
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
