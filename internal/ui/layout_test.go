package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/imgview"
	"github.com/ragnar/cyber-tui/internal/ui/screens"
)

// --- visibleTabs ---

func TestVisibleTabs_ExcludesHiddenSearch(t *testing.T) {
	for _, t2 := range visibleTabs() {
		if t2.s == screenSearch {
			t.Error("expected screenSearch to be excluded from visibleTabs")
		}
	}
	if got, want := len(visibleTabs()), len(menuTabs)-1; got != want {
		t.Errorf("expected %d visible tabs (menuTabs minus the one hidden entry), got %d", want, got)
	}
}

// --- renderTabBar / renderNav: Search must not appear on either bar ---

func TestRenderTabBar_DoesNotShowSearch(t *testing.T) {
	a := loggedInApp()
	a.width = 100
	out := ansi.Strip(TabsLayout{}.renderTabBar(a))
	if strings.Contains(out, "search") {
		t.Errorf("expected the tab bar to omit the hidden Search entry, got: %q", out)
	}
}

// --- unread badge: exact vs capped count (v0.8.5) ---

func TestRenderTabBar_UnreadBadge_ShowsExactCount(t *testing.T) {
	a := loggedInApp()
	a.width = 100
	a.polledUnreadCount = 3
	a.polledUnreadCountExact = true
	out := ansi.Strip(TabsLayout{}.renderTabBar(a))
	if !strings.Contains(out, "(3)") {
		t.Errorf("expected exact unread badge (3), got: %q", out)
	}
}

func TestRenderTabBar_UnreadBadge_ShowsCappedIndicator(t *testing.T) {
	a := loggedInApp()
	a.width = 100
	a.polledUnreadCount = 100
	a.polledUnreadCountExact = false
	out := ansi.Strip(TabsLayout{}.renderTabBar(a))
	if !strings.Contains(out, "(99+)") {
		t.Errorf("expected capped unread badge (99+), got: %q", out)
	}
}

func TestRenderNav_UnreadBadge_ShowsCappedIndicator(t *testing.T) {
	a := loggedInApp()
	a.width = 100
	a.polledUnreadCount = 100
	a.polledUnreadCountExact = false
	out := ansi.Strip(MillerLayout{}.renderNav(a))
	if !strings.Contains(out, "●99+") {
		t.Errorf("expected capped unread badge ●99+, got: %q", out)
	}
}

// --- renderFeedPendingBar: background-poll indicator lives in the separator
// row, not pushed into the feed viewport (see FeedModel.buildContent) ---

func TestRenderFeedPendingBar_ShowsLabelWhenPending(t *testing.T) {
	a := loggedInApp()
	a.feed = a.feed.SetPosts(nil, "")
	a.feed = a.feed.SetPendingNew([]model.Post{{ID: "p1"}, {ID: "p2"}})

	out := ansi.Strip(TabsLayout{}.renderFeedPendingBar(a))
	if !strings.Contains(out, "load 2 new entries") {
		t.Errorf("renderFeedPendingBar() = %q, want it to contain \"load 2 new entries\"", out)
	}
}

func TestRenderFeedPendingBar_BlankWhenNoneOrNotOnFeed(t *testing.T) {
	a := loggedInApp()
	a.feed = a.feed.SetPosts(nil, "")
	if out := (TabsLayout{}).renderFeedPendingBar(a); out != "" {
		t.Errorf("renderFeedPendingBar() = %q, want blank with nothing pending", out)
	}

	a.feed = a.feed.SetPendingNew([]model.Post{{ID: "p1"}})
	a.active = screenNotifications
	if out := (TabsLayout{}).renderFeedPendingBar(a); out != "" {
		t.Errorf("renderFeedPendingBar() = %q, want blank on a non-feed tab", out)
	}
}

func TestRenderNav_DoesNotShowSearch(t *testing.T) {
	a := loggedInApp()
	out := ansi.Strip(MillerLayout{}.renderNav(a))
	if strings.Contains(out, "search") {
		t.Errorf("expected the nav sidebar to omit the hidden Search entry, got: %q", out)
	}
}

// --- tabVisualState ---
//
// Unifies what used to be an ad hoc isActive expression duplicated (and
// drifting) between renderTabBar and renderNav: a tab is "selected" while
// a.active matches it directly, or PostDetail is open and was reached from
// it (postDetailReturn); it's additionally "in detail" — one level deep —
// for an open Circ room/C-Mail conversation/Guilds-Topics browse, or
// whenever PostDetail itself is what's open.
//
// detail is reported even while a tab isn't selected for Circ/Guilds/Topics,
// since their detail state is genuinely still live/persisted in the
// background (see tabVisualState's doc comment). C-Mail and PostDetail are
// selected-only: C-Mail's mode lingers stale between leaving and the next
// ResetToList() even though its subscription was already torn down, so
// showing it in the background would misrepresent a conversation as still
// open.

func TestTabVisualState_Unselected(t *testing.T) {
	a := loggedInApp()
	a.active = screenFeed
	selected, detail := tabVisualState(a, screenCMail)
	if selected || detail {
		t.Errorf("selected=%v detail=%v, want false,false for a tab that isn't active", selected, detail)
	}
}

func TestTabVisualState_SelectedListMode_NoDetail(t *testing.T) {
	a := loggedInApp()
	a.active = screenFeed
	selected, detail := tabVisualState(a, screenFeed)
	if !selected || detail {
		t.Errorf("selected=%v detail=%v, want true,false for the active tab in its top-level view", selected, detail)
	}
}

func TestTabVisualState_ChatroomsDetail(t *testing.T) {
	a := loggedInApp()
	a, _ = activateScreen(a, screenChatrooms)
	a.chatrooms = a.chatrooms.SetRooms([]model.Room{{ID: "r1", Slug: "zion", Name: "Zion"}})
	cm, _ := a.chatrooms.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	cm, _ = cm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	a.chatrooms = cm

	selected, detail := tabVisualState(a, screenChatrooms)
	if !selected || !detail {
		t.Errorf("selected=%v detail=%v, want true,true with a room open", selected, detail)
	}
}

func TestTabVisualState_CMailDetail(t *testing.T) {
	a := loggedInApp()
	a.cmail = a.cmail.SetConversations([]model.Conversation{
		{ID: "c1", Participants: []model.User{{Username: a.currentUser.Username}, {Username: "molly"}}},
	})
	cm, _ := a.cmail.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	cm, _ = cm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	a.cmail = cm
	a.active = screenCMail

	selected, detail := tabVisualState(a, screenCMail)
	if !selected || !detail {
		t.Errorf("selected=%v detail=%v, want true,true with a conversation open", selected, detail)
	}
}

func TestTabVisualState_GuildsBrowsingGuild(t *testing.T) {
	a := loggedInApp()
	a.active = screenGuilds
	a.guilds = a.guilds.SetGuilds([]model.Guild{{ID: "g1", Name: "Alpha", Slug: "alpha"}}, "")
	gm, _ := a.guilds.Update(tea.KeyMsg{Type: tea.KeyEnter})
	a.guilds = gm

	selected, detail := tabVisualState(a, screenGuilds)
	if !selected || !detail {
		t.Errorf("selected=%v detail=%v, want true,true while browsing a guild", selected, detail)
	}
}

func TestTabVisualState_TopicsBrowsingTopic(t *testing.T) {
	a := loggedInApp()
	a.active = screenTopics
	a.topics = a.topics.SetTopics([]model.Topic{{Slug: "tech"}}, "")
	tm, _ := a.topics.Update(tea.KeyMsg{Type: tea.KeyEnter})
	a.topics = tm

	selected, detail := tabVisualState(a, screenTopics)
	if !selected || !detail {
		t.Errorf("selected=%v detail=%v, want true,true while browsing a topic", selected, detail)
	}
}

// --- background persistence: Circ/Guilds/Topics report detail while unselected ---

func TestTabVisualState_ChatroomsDetail_PersistsWhenBackgrounded(t *testing.T) {
	a := loggedInApp()
	a, _ = activateScreen(a, screenChatrooms)
	a.chatrooms = a.chatrooms.SetRooms([]model.Room{{ID: "r1", Slug: "zion", Name: "Zion"}})
	cm, _ := a.chatrooms.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	cm, _ = cm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	a.chatrooms = cm

	a, _ = activateScreen(a, screenFeed) // background Chatrooms

	selected, detail := tabVisualState(a, screenChatrooms)
	if selected {
		t.Error("expected Chatrooms to not be selected after switching to Feed")
	}
	if !detail {
		t.Error("expected Chatrooms to still report detail=true — the room stays open in the background")
	}
}

func TestTabVisualState_GuildsBrowsing_PersistsWhenBackgrounded(t *testing.T) {
	a := loggedInApp()
	a.active = screenGuilds
	a.guilds = a.guilds.SetGuilds([]model.Guild{{ID: "g1", Name: "Alpha", Slug: "alpha"}}, "")
	gm, _ := a.guilds.Update(tea.KeyMsg{Type: tea.KeyEnter})
	a.guilds = gm

	a.active = screenFeed // background Guilds

	selected, detail := tabVisualState(a, screenGuilds)
	if selected {
		t.Error("expected Guilds to not be selected after switching to Feed")
	}
	if !detail {
		t.Error("expected Guilds to still report detail=true — browse state isn't reset on tab-away")
	}
}

func TestTabVisualState_TopicsBrowsing_PersistsWhenBackgrounded(t *testing.T) {
	a := loggedInApp()
	a.active = screenTopics
	a.topics = a.topics.SetTopics([]model.Topic{{Slug: "tech"}}, "")
	tm, _ := a.topics.Update(tea.KeyMsg{Type: tea.KeyEnter})
	a.topics = tm

	a.active = screenFeed // background Topics

	selected, detail := tabVisualState(a, screenTopics)
	if selected {
		t.Error("expected Topics to not be selected after switching to Feed")
	}
	if !detail {
		t.Error("expected Topics to still report detail=true — browse state isn't reset on tab-away")
	}
}

func TestTabVisualState_TopicsBrowsing_ClearsOnEsc(t *testing.T) {
	a := loggedInApp()
	a.active = screenTopics
	a.topics = a.topics.SetTopics([]model.Topic{{Slug: "tech"}}, "")
	tm, _ := a.topics.Update(tea.KeyMsg{Type: tea.KeyEnter})
	tm = tm.SetTopicPosts([]model.Post{{ID: "p1", AuthorUsername: "alice"}}, "") // Enter only fires the load cmd; this is what flips m.view to viewTopicPosts
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyEsc})
	a.topics = tm

	if selected, detail := tabVisualState(a, screenTopics); !selected || detail {
		t.Errorf("selected=%v detail=%v, want true,false after esc back to the topic list", selected, detail)
	}

	a.active = screenFeed // background Topics after esc
	if selected, detail := tabVisualState(a, screenTopics); selected || detail {
		t.Errorf("selected=%v detail=%v, want false,false — esc should clear detail even once backgrounded", selected, detail)
	}
}

func TestTabVisualState_CMailDetail_PersistsWhenBackgrounded(t *testing.T) {
	a := loggedInApp()
	a.cmail = a.cmail.SetConversations([]model.Conversation{
		{ID: "c1", Participants: []model.User{{Username: a.currentUser.Username}, {Username: "molly"}}},
	})
	cm, _ := a.cmail.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	cm, _ = cm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	a.cmail = cm
	a.active = screenCMail

	a.active = screenFeed // background C-Mail — its RTDB subscription stays live now

	selected, detail := tabVisualState(a, screenCMail)
	if selected {
		t.Error("expected C-Mail to not be selected after switching to Feed")
	}
	if !detail {
		t.Error("expected C-Mail to report detail=true while backgrounded — the conversation genuinely stays open now (mirrors Chatrooms)")
	}
}

func TestTabVisualState_PostDetail_CreditsOriginTab(t *testing.T) {
	for _, origin := range []screen{screenFeed, screenGuilds} {
		a := loggedInApp()
		a.active = screenPostDetail
		a.postDetailReturn = origin
		a.postDetail = a.postDetail.SetPost(model.Post{ID: "p1"})

		selected, detail := tabVisualState(a, origin)
		if !selected || !detail {
			t.Errorf("origin %v: selected=%v detail=%v, want true,true while PostDetail is open", origin, selected, detail)
		}

		// A different tab must not also claim the PostDetail state.
		other := screenNotifications
		if origin == screenNotifications {
			other = screenSettings
		}
		selected, detail = tabVisualState(a, other)
		if selected || detail {
			t.Errorf("origin %v, other tab %v: selected=%v detail=%v, want false,false", origin, other, selected, detail)
		}
	}
}

func TestTabVisualState_PostDetail_PersistsWhenBackgrounded(t *testing.T) {
	a := loggedInApp()
	a.active = screenPostDetail
	a.postDetailReturn = screenBookmarks
	a.postDetail = a.postDetail.SetPost(model.Post{ID: "p1"})

	a.active = screenFeed // background PostDetail

	selected, detail := tabVisualState(a, screenBookmarks)
	if selected {
		t.Error("expected Bookmarks to not be selected after switching to Feed")
	}
	if !detail {
		t.Error("expected Bookmarks to still report detail=true — the post stays open in the background")
	}
}

// --- renderTabBar / renderNav: detail marker reflects tabVisualState ---

func TestRenderTabBar_ShowsDetailMarker(t *testing.T) {
	a := loggedInApp()
	a.width = 100
	a, _ = activateScreen(a, screenChatrooms)
	a.chatrooms = a.chatrooms.SetRooms([]model.Room{{ID: "r1", Slug: "zion", Name: "Zion"}})
	cm, _ := a.chatrooms.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	cm, _ = cm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	a.chatrooms = cm

	out := ansi.Strip(TabsLayout{}.renderTabBar(a))
	if !strings.Contains(out, "›") {
		t.Errorf("expected a detail marker (›) in the tab bar with a room open, got: %q", out)
	}
}

func TestRenderTabBar_NoDetailMarkerInListMode(t *testing.T) {
	a := loggedInApp()
	a.width = 100
	a, _ = activateScreen(a, screenChatrooms)

	out := ansi.Strip(TabsLayout{}.renderTabBar(a))
	if strings.Contains(out, "›") {
		t.Errorf("expected no detail marker while Chatrooms is still in list mode, got: %q", out)
	}
}

func TestRenderNav_ShowsOpenMarkerForDetail(t *testing.T) {
	a := loggedInApp()
	a, _ = activateScreen(a, screenChatrooms)
	a.chatrooms = a.chatrooms.SetRooms([]model.Room{{ID: "r1", Slug: "zion", Name: "Zion"}})
	cm, _ := a.chatrooms.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	cm, _ = cm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	a.chatrooms = cm

	out := ansi.Strip(MillerLayout{}.renderNav(a))
	if !strings.Contains(out, "▷") {
		t.Errorf("expected the open (▷) marker in the nav sidebar with a room open, got: %q", out)
	}
}

func TestRenderTabBar_ShowsDetailMarkerWhenBackgrounded(t *testing.T) {
	a := loggedInApp()
	a.width = 100
	a, _ = activateScreen(a, screenChatrooms)
	a.chatrooms = a.chatrooms.SetRooms([]model.Room{{ID: "r1", Slug: "zion", Name: "Zion"}})
	cm, _ := a.chatrooms.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	cm, _ = cm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	a.chatrooms = cm
	a, _ = activateScreen(a, screenFeed) // background Chatrooms

	out := ansi.Strip(TabsLayout{}.renderTabBar(a))
	if !strings.Contains(out, "›") {
		t.Errorf("expected a detail marker for the backgrounded room, got: %q", out)
	}
}

func TestRenderTabBar_ShowsDetailMarkerForBackgroundedCMail(t *testing.T) {
	a := loggedInApp()
	a.width = 100
	a.cmail = a.cmail.SetConversations([]model.Conversation{
		{ID: "c1", Participants: []model.User{{Username: a.currentUser.Username}, {Username: "molly"}}},
	})
	cm, _ := a.cmail.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	cm, _ = cm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	a.cmail = cm
	a.active = screenFeed // background C-Mail — the conversation stays open now

	out := ansi.Strip(TabsLayout{}.renderTabBar(a))
	if !strings.Contains(out, "›") {
		t.Errorf("expected a detail marker for backgrounded C-Mail (its conversation genuinely stays open now), got: %q", out)
	}
}

func TestRenderNav_ShowsOpenMarkerWhenBackgrounded(t *testing.T) {
	a := loggedInApp()
	a, _ = activateScreen(a, screenChatrooms)
	a.chatrooms = a.chatrooms.SetRooms([]model.Room{{ID: "r1", Slug: "zion", Name: "Zion"}})
	cm, _ := a.chatrooms.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	cm, _ = cm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	a.chatrooms = cm
	a, _ = activateScreen(a, screenFeed) // background Chatrooms

	out := ansi.Strip(MillerLayout{}.renderNav(a))
	if !strings.Contains(out, "▷") {
		t.Errorf("expected the open (▷) marker in the nav sidebar for the backgrounded room, got: %q", out)
	}
}

// --- renderImageModal: carousel position hint ---
//
// Cycling arrows overlaid directly on the image were invisible in practice —
// Kitty placements are an independent compositing layer that can hide text
// drawn into their own cells regardless of z-index. The hint line renders
// through the normal bordered-box text path instead, so it can't be hidden
// by an image placement.

func TestRenderImageModal_Carousel_ShowsPositionHint(t *testing.T) {
	a := loggedInApp()
	a.imageModalCols = 20
	a.imageModalRows = 5
	a.imageCarouselItems = []string{"https://x.com/a.jpg", "https://x.com/b.jpg", "https://x.com/c.jpg"}
	a.imageCarouselIndex = 1
	out := ansi.Strip(TabsLayout{}.renderImageModal(a))
	if !strings.Contains(out, "2/3") {
		t.Errorf("expected a 2/3 position hint, got: %q", out)
	}
}

func TestRenderImageModal_SingleImage_NoHint(t *testing.T) {
	a := loggedInApp()
	a.imageModalCols = 20
	a.imageModalRows = 5
	out := ansi.Strip(TabsLayout{}.renderImageModal(a))
	if strings.Contains(out, "◂") || strings.Contains(out, "▸") {
		t.Errorf("expected no position hint for a plain single-image view, got: %q", out)
	}
}

// --- screenForNumber ---

func TestScreenForNumber_1Through9_MatchMenuTabsOrder(t *testing.T) {
	want := []screen{
		screenFeed, screenNotifications, screenCMail, screenChatrooms,
		screenJournal, screenBookmarks, screenGuilds, screenTopics, screenProfile,
	}
	for i, w := range want {
		key := string(rune('1' + i))
		got, ok := screenForNumber(key)
		if !ok {
			t.Errorf("key %q: expected ok=true", key)
		}
		if got != w {
			t.Errorf("key %q: expected %v, got %v", key, w, got)
		}
	}
}

func TestScreenForNumber_SearchAndSettingsHaveNoNumericAlias(t *testing.T) {
	// menuTabs' 10th and 11th entries (Search, Settings) are intentionally
	// unreachable by number — only "0" or beyond "9" would reach them, and
	// neither is a valid key here.
	if _, ok := screenForNumber("0"); ok {
		t.Error("\"0\" should not resolve to a screen")
	}
}

func TestScreenForNumber_InvalidKeys(t *testing.T) {
	for _, key := range []string{"", "a", "10", "g", "!"} {
		if _, ok := screenForNumber(key); ok {
			t.Errorf("key %q: expected ok=false", key)
		}
	}
}

// --- screenForMnemonic ---

func TestScreenForMnemonic_AllElevenTabsReachable(t *testing.T) {
	for _, tab := range menuTabs {
		got, ok := screenForMnemonic(string(tab.mnemonic))
		if !ok {
			t.Errorf("mnemonic %q: expected ok=true", tab.mnemonic)
		}
		if got != tab.s {
			t.Errorf("mnemonic %q: expected %v, got %v", tab.mnemonic, tab.s, got)
		}
	}
}

func TestScreenForMnemonic_UnmappedKeyCancels(t *testing.T) {
	if _, ok := screenForMnemonic("z"); ok {
		t.Error("\"z\" is not a mnemonic and should not resolve to a screen")
	}
}

func TestScreenForMnemonic_MnemonicsAreUniqueAndPresentInLabel(t *testing.T) {
	seen := make(map[rune]string)
	for _, tab := range menuTabs {
		if prev, ok := seen[tab.mnemonic]; ok {
			t.Errorf("mnemonic %q used by both %q and %q", tab.mnemonic, prev, tab.label)
		}
		seen[tab.mnemonic] = tab.label
		if _, ch, _ := splitMnemonic(tab.label, tab.mnemonic); ch == "" {
			t.Errorf("mnemonic %q for %q does not appear in its own label", tab.mnemonic, tab.label)
		}
	}
}

// --- splitMnemonic ---

func TestSplitMnemonic_Found(t *testing.T) {
	before, ch, after := splitMnemonic("c-mail", 'm')
	if before != "c-" || ch != "m" || after != "ail" {
		t.Errorf("got (%q, %q, %q)", before, ch, after)
	}
}

func TestSplitMnemonic_NotFound(t *testing.T) {
	before, ch, after := splitMnemonic("feed", 'z')
	if before != "feed" || ch != "" || after != "" {
		t.Errorf("got (%q, %q, %q), want (\"feed\", \"\", \"\")", before, ch, after)
	}
}

// --- layoutFromName ---

func TestLayoutFromName(t *testing.T) {
	if _, ok := layoutFromName("miller").(MillerLayout); !ok {
		t.Errorf("expected layoutFromName(%q) to return MillerLayout", "miller")
	}
	for _, name := range []string{"", "tabs", "unknown"} {
		if _, ok := layoutFromName(name).(TabsLayout); !ok {
			t.Errorf("expected layoutFromName(%q) to return TabsLayout", name)
		}
	}
}

// --- compositeOverlays ---

// fakeModalRenderer is a minimal modalRenderer test double so
// compositeOverlays' shared logic can be tested in isolation, without a full
// TabsLayout/MillerLayout render pipeline — see TestTabsLayoutView_ and
// TestMillerLayoutView_InjectsInlineImages below for the real-pipeline
// smoke tests that exercise each layout's own InlineImageSlots.
type fakeModalRenderer struct {
	slots                []screens.InlineImageSlot
	rowOrigin, colOrigin int
}

func (f fakeModalRenderer) renderThemePicker(a App) string     { return "" }
func (f fakeModalRenderer) renderThemeEditor(a App) string     { return "" }
func (f fakeModalRenderer) renderPathPrompt(a App) string      { return "" }
func (f fakeModalRenderer) renderHelpModal(a App) string       { return "" }
func (f fakeModalRenderer) renderURLPicker(a App) string       { return "" }
func (f fakeModalRenderer) renderIconPicker(a App) string      { return "" }
func (f fakeModalRenderer) renderAttachURLPrompt(a App) string { return "" }
func (f fakeModalRenderer) renderImageModal(a App) string      { return "" }
func (f fakeModalRenderer) InlineImageSlots(a App) ([]screens.InlineImageSlot, int, int, string) {
	return f.slots, f.rowOrigin, f.colOrigin, ""
}

// TestCompositeOverlays_KittyCleanupFallsThroughToInlineImages is a
// regression test for the bug MillerLayout originally shipped with: the
// Kitty placement-cleanup block must fall through to inline-image
// injection in the same frame, not return early — an early return there
// silently drops all inline-image rendering for the rest of the session
// (see app.go's imageNeedsCleanup doc comment). Since compositeOverlays is
// now the one place this logic lives, this test alone covers both layouts.
func TestCompositeOverlays_KittyCleanupFallsThroughToInlineImages(t *testing.T) {
	slot := screens.InlineImageSlot{Key: "post:p1:0", URL: "https://example.com/a.png", Row: 1, ColIndent: 2}
	a := App{
		width: 40, height: 10,
		graphicsProtocol:  imgview.ProtocolKitty,
		imageNeedsCleanup: true,
		imageModalRows:    3,
		inlineImageCache:  map[string]string{inlineImageCacheKey(slot, imgview.ProtocolKitty, nil): "\x1b_Gfake\x1b\\"},
	}
	l := fakeModalRenderer{slots: []screens.InlineImageSlot{slot}, rowOrigin: 5, colOrigin: 7}
	out := compositeOverlays(l, a, strings.Repeat("x\n", 9)+"x")

	if !strings.Contains(out, imgview.DeleteKittyPlacement(kittyModalPlacementID)) {
		t.Error("expected the Kitty modal placement delete in the composited output")
	}
	if !strings.Contains(out, "\x1b_Gfake\x1b\\") {
		t.Error("expected the inline image's cached escape sequence in the same composited output — cleanup must fall through, not return early")
	}
	if !strings.Contains(out, "\x1b[6;9H") { // rowOrigin(5)+slot.Row(1)=6, colOrigin(7)+slot.ColIndent(2)=9
		t.Errorf("expected the inline image positioned at rowOrigin+Row, colOrigin+ColIndent; got %q", out)
	}
}

// TestCompositeOverlays_ImageModalOpen_StillDrawsInlineImagesBehindIt
// confirms inline-image thumbnails still get drawn into the frame while the
// fullscreen modal is open, rather than being skipped entirely as before.
// Needed so sixelFullRepaint (a real full-screen erase, fired on a
// size-changed Sixel carousel cycle) has something to restore them from —
// before this fix, a Sixel cycle wiped the whole screen and nothing put the
// thumbnails back until the modal closed (confirmed live).
func TestCompositeOverlays_ImageModalOpen_StillDrawsInlineImagesBehindIt(t *testing.T) {
	slot := screens.InlineImageSlot{Key: "post:p1:0", URL: "https://example.com/a.png", Row: 1, ColIndent: 2}
	a := App{
		width: 40, height: 10,
		graphicsProtocol: imgview.ProtocolKitty,
		imageModalOpen:   true,
		inlineImageCache: map[string]string{inlineImageCacheKey(slot, imgview.ProtocolKitty, nil): "\x1b_Gfake\x1b\\"},
	}
	l := fakeModalRenderer{slots: []screens.InlineImageSlot{slot}, rowOrigin: 5, colOrigin: 7}
	out := compositeOverlays(l, a, strings.Repeat("x\n", 9)+"x")

	if !strings.Contains(out, "\x1b_Gfake\x1b\\") {
		t.Error("expected the inline image's cached escape sequence in the output even while the modal is open")
	}
	if !strings.Contains(out, "\x1b[6;9H") { // rowOrigin(5)+slot.Row(1)=6, colOrigin(7)+slot.ColIndent(2)=9
		t.Errorf("expected the inline image positioned at rowOrigin+Row, colOrigin+ColIndent; got %q", out)
	}
}

// TestCompositeOverlays_ImageModalCycle_ForcesPrevBoxRowsDirty confirms a
// carousel cycle to a differently-sized image (imageModalPrevRows/Cols
// differing from the current dims) forces the previous box's row range
// dirty via forceRowsDirty, instead of the tea.ClearScreen this replaced.
// Only checks rows outside the new (smaller) box's own footprint, since
// overlayCenter splices its own box content into overlapping rows.
func TestCompositeOverlays_ImageModalCycle_ForcesPrevBoxRowsDirty(t *testing.T) {
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = fmt.Sprintf("line%d", i+1)
	}
	base := strings.Join(lines, "\n")

	a := App{
		width:              40,
		height:             20,
		graphicsProtocol:   imgview.ProtocolITerm2,
		imageModalOpen:     true,
		imageCarouselItems: []string{"https://x.com/a.jpg", "https://x.com/b.jpg"},
		imageModalPrevRows: 10,
		imageModalPrevCols: 20,
		imageModalRows:     3,
		imageModalCols:     20,
	}

	rowRange := func(dims App) (yOff, h int) {
		modal := TabsLayout{}.renderImageModal(dims)
		h = len(strings.Split(modal, "\n"))
		yOff = (a.height - h) / 2
		if yOff < 0 {
			yOff = 0
		}
		return
	}
	prevA := a
	prevA.imageModalRows, prevA.imageModalCols = a.imageModalPrevRows, a.imageModalPrevCols
	prevYOff, prevH := rowRange(prevA)
	curYOff, curH := rowRange(a)

	out := compositeOverlays(TabsLayout{}, a, base)

	checked := 0
	for r := prevYOff + 1; r <= prevYOff+prevH; r++ {
		if r >= curYOff+1 && r <= curYOff+curH {
			continue // covered by the new box too; overlayCenter may splice over the marker there
		}
		checked++
		want := fmt.Sprintf("line%d%s", r, imageDirtyMarker(a.imageRepaintGen))
		if !strings.Contains(out, want) {
			t.Errorf("expected row %d (outside the new box) forced dirty (%q) in output", r, want)
		}
	}
	if checked == 0 {
		t.Fatal("test setup produced no non-overlapping rows to check — adjust prev/current dims")
	}
}

// TestCompositeOverlays_ImageModalCycle_SixelGetsFullRepaint confirms Sixel
// doesn't go through the iTerm2 forceRowsDirty prev-box block above — it
// gets sixelFullRepaint instead (a real erase, prepended once, plus every
// row forced dirty), the only mechanism live-confirmed on real Konsole
// hardware to actually clear stray Sixel pixels on a size-changed cycle.
func TestCompositeOverlays_ImageModalCycle_SixelGetsFullRepaint(t *testing.T) {
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = fmt.Sprintf("line%d", i+1)
	}
	base := strings.Join(lines, "\n")

	a := App{
		width:              40,
		height:             20,
		graphicsProtocol:   imgview.ProtocolSixel,
		imageModalOpen:     true,
		imageCarouselItems: []string{"https://x.com/a.jpg", "https://x.com/b.jpg"},
		imageModalPrevRows: 10,
		imageModalPrevCols: 20,
		imageModalRows:     3,
		imageModalCols:     20,
	}

	out := compositeOverlays(TabsLayout{}, a, base)
	if !strings.HasPrefix(out, "\x1b[2J\x1b[H") {
		t.Errorf("expected a full-screen erase prepended to the frame, got %q", out)
	}
	if !strings.Contains(out, "line1"+imageDirtyMarker(a.imageRepaintGen)) {
		t.Errorf("expected every row forced dirty (e.g. row 1), got %q", out)
	}
}

// TestInjectInlineImages_ITerm2RepaintGenAlwaysDiffersOnChange confirms the
// imageRepaintGen dirty-marker mechanism actually does what
// syncInlineImages/injectInlineImages depend on it for, for iTerm2: without
// it, a selection change that recolors a band row elsewhere (but doesn't
// change which images are visible or their cache contents) leaves this
// function's output byte-identical to last frame, so Bubble Tea's per-line
// diff skips resending it and the image never gets repainted — this was the
// reason a tea.ClearScreen (later replaced by this mechanism) was needed at
// all. Every distinct gen must produce different output, not just
// odd-vs-even ones — a fixed %2 toggle has the same collision class the
// round-5/6 fix closed for Sixel (two consecutive *actually flushed* frames
// landing on the same parity are byte-identical, so Bubble Tea wrongly skips
// resending this line), which iTerm2 was originally left exposed to until
// imageRepaintGen was extended to cover both protocols.
func TestInjectInlineImages_ITerm2RepaintGenAlwaysDiffersOnChange(t *testing.T) {
	slot := screens.InlineImageSlot{Key: "post:p1:0", URL: "https://example.com/a.png", Row: 1, ColIndent: 2}
	cache := map[string]string{inlineImageCacheKey(slot, imgview.ProtocolITerm2, nil): "\x1b]1337;fake\x07"}
	slots := []screens.InlineImageSlot{slot}

	a1 := App{width: 40, height: 10, graphicsProtocol: imgview.ProtocolITerm2, inlineImageCache: cache, imageRepaintGen: 1}
	a2 := App{width: 40, height: 10, graphicsProtocol: imgview.ProtocolITerm2, inlineImageCache: cache, imageRepaintGen: 2}
	a3 := App{width: 40, height: 10, graphicsProtocol: imgview.ProtocolITerm2, inlineImageCache: cache, imageRepaintGen: 3}

	out1 := injectInlineImages(a1, "base", slots, 5, 7)
	out2 := injectInlineImages(a2, "base", slots, 5, 7)
	out3 := injectInlineImages(a3, "base", slots, 5, 7)

	if out1 == out2 || out2 == out3 || out1 == out3 {
		t.Error("expected every distinct imageRepaintGen to produce different output, not just odd-vs-even")
	}
}

// TestInjectInlineImages_SixelRepaintGenAlwaysDiffersOnChange is the Sixel
// equivalent of TestInjectInlineImages_ITerm2RepaintGenAlwaysDiffersOnChange
// — both protocols share the same imageRepaintGen/imageDirtyMarker
// mechanism, this just confirms it holds with a Sixel-encoded cache entry
// and the Sixel protocol path through injectInlineImages too.
func TestInjectInlineImages_SixelRepaintGenAlwaysDiffersOnChange(t *testing.T) {
	slot := screens.InlineImageSlot{Key: "post:p1:0", URL: "https://example.com/a.png", Row: 1, ColIndent: 2}
	cache := map[string]string{inlineImageCacheKey(slot, imgview.ProtocolSixel, nil): "\x1bPq...fake\x1b\\"}
	slots := []screens.InlineImageSlot{slot}

	a1 := App{width: 40, height: 10, graphicsProtocol: imgview.ProtocolSixel, inlineImageCache: cache, imageRepaintGen: 1}
	a2 := App{width: 40, height: 10, graphicsProtocol: imgview.ProtocolSixel, inlineImageCache: cache, imageRepaintGen: 2}
	a3 := App{width: 40, height: 10, graphicsProtocol: imgview.ProtocolSixel, inlineImageCache: cache, imageRepaintGen: 3}

	out1 := injectInlineImages(a1, "base", slots, 5, 7)
	out2 := injectInlineImages(a2, "base", slots, 5, 7)
	out3 := injectInlineImages(a3, "base", slots, 5, 7)

	if out1 == out2 || out2 == out3 || out1 == out3 {
		t.Error("expected every distinct imageRepaintGen to produce different output, not just odd-vs-even")
	}
}

// TestForceRowsDirty confirms it appends the inert SGR-reset marker to
// exactly the requested absolute rows (1-indexed) of base, leaving every
// other line untouched, and is a no-op for a row index out of base's range.
func TestForceRowsDirty(t *testing.T) {
	lines := make([]string, 15)
	for i := range lines {
		lines[i] = fmt.Sprintf("line%d", i+1)
	}
	base := strings.Join(lines, "\n")

	out := forceRowsDirty(base, []int{6, 12, 999}, "\x1b[0m")
	outLines := strings.Split(out, "\n")
	if outLines[5] != "line6\x1b[0m" {
		t.Errorf("expected row 6 forced dirty, got %q", outLines[5])
	}
	if outLines[11] != "line12\x1b[0m" {
		t.Errorf("expected row 12 forced dirty, got %q", outLines[11])
	}
	if outLines[0] != "line1" {
		t.Errorf("expected row 1 untouched, got %q", outLines[0])
	}
	if len(outLines) != len(lines) {
		t.Errorf("expected out-of-range row 999 to be ignored, got %d lines want %d", len(outLines), len(lines))
	}
}

// TestSixelFullRepaint confirms it prepends a real full-screen erase and
// forces every row dirty, not just a subset — see its doc comment for why:
// a real erase requires every line to be resent afterward, or a line
// Bubble Tea's diff thinks is unchanged would come back genuinely blank
// rather than stale.
func TestSixelFullRepaint(t *testing.T) {
	lines := make([]string, 5)
	for i := range lines {
		lines[i] = fmt.Sprintf("line%d", i+1)
	}
	base := strings.Join(lines, "\n")

	out := sixelFullRepaint(base, len(lines), 42)
	if !strings.HasPrefix(out, "\x1b[2J\x1b[H") {
		t.Fatalf("expected the erase+home escapes prepended, got %q", out)
	}
	body := strings.TrimPrefix(out, "\x1b[2J\x1b[H")
	outLines := strings.Split(body, "\n")
	if len(outLines) != len(lines) {
		t.Fatalf("expected %d lines, got %d", len(lines), len(outLines))
	}
	marker := imageDirtyMarker(42)
	for i, line := range outLines {
		want := fmt.Sprintf("line%d%s", i+1, marker)
		if line != want {
			t.Errorf("row %d: expected %q, got %q", i+1, want, line)
		}
	}
}

// TestSixelFullRepaint_DifferentGensNeverCollide is the actual regression
// test for the bug this round fixes: two consecutive *actually flushed*
// repaint frames (which, under fast scroll/cycle, can legitimately both be
// sixelFullRepaint calls — see its doc comment) must never produce
// byte-identical output for a row whose real content didn't change,
// because Bubble Tea's per-line diff would then wrongly skip resending it
// after the real erase, leaving it genuinely blank. A fixed marker (the
// pre-fix behavior) fails this immediately; imageDirtyMarker(gen) must not.
func TestSixelFullRepaint_DifferentGensNeverCollide(t *testing.T) {
	base := "unchanged line"

	out1 := sixelFullRepaint(base, 1, 1)
	out2 := sixelFullRepaint(base, 1, 2)
	if out1 == out2 {
		t.Errorf("expected different gens to produce different output for an unchanged row, both got %q", out1)
	}
}

// TestInjectInlineImages_ForcesStaleRowsDirty confirms App.inlineImageStaleRows
// (a moved or removed image's stale rows — see syncInlineImageErasures)
// actually reaches base via forceRowsDirty in injectInlineImages' output,
// rather than the old out-of-band blank-fill.
func TestInjectInlineImages_ForcesStaleRowsDirty(t *testing.T) {
	lines := make([]string, 15)
	for i := range lines {
		lines[i] = fmt.Sprintf("line%d", i+1)
	}
	base := strings.Join(lines, "\n")
	a := App{width: 40, height: 10, graphicsProtocol: imgview.ProtocolITerm2, inlineImageStaleRows: []int{6, 12}}

	out := injectInlineImages(a, base, nil, 0, 0)
	marker := imageDirtyMarker(a.imageRepaintGen)
	if !strings.Contains(out, "line6"+marker) {
		t.Errorf("expected row 6 forced dirty in output, got %q", out)
	}
	if !strings.Contains(out, "line12"+marker) {
		t.Errorf("expected row 12 forced dirty in output, got %q", out)
	}
}

// TestInjectInlineImages_HoldsBackDrawsWithinSwitchSettleDelay is the
// regression test for the Round 7 mitigation: live debug-log evidence
// showed the app correctly recomputes and reissues the right image draw
// command on returning to a screen after a fast switch away and back, yet
// real iTerm2 still failed to render it — consistent with the terminal
// still processing the switch's own large, unrelated redraw. Draws are
// held back for inlineImageSwitchSettleDelay after a.screenSwitchedAt, but
// the stale-row resend itself (forceRowsDirty, unrelated to this slot)
// must still happen immediately regardless.
func TestInjectInlineImages_HoldsBackDrawsWithinSwitchSettleDelay(t *testing.T) {
	slot := screens.InlineImageSlot{Key: "post:p1:0", URL: "https://example.com/a.png", Row: 1, ColIndent: 2}
	cache := map[string]string{inlineImageCacheKey(slot, imgview.ProtocolITerm2, nil): "\x1b]1337;fake\x07"}
	slots := []screens.InlineImageSlot{slot}

	withinDelay := App{
		width: 40, height: 10, graphicsProtocol: imgview.ProtocolITerm2,
		inlineImageCache: cache, screenSwitchedAt: time.Now(),
	}
	out := injectInlineImages(withinDelay, "base", slots, 5, 7)
	if strings.Contains(out, "fake") {
		t.Errorf("expected the image draw withheld within the settle delay, got %q", out)
	}

	pastDelay := withinDelay
	pastDelay.screenSwitchedAt = time.Now().Add(-inlineImageSwitchSettleDelay - time.Second)
	out = injectInlineImages(pastDelay, "base", slots, 5, 7)
	if !strings.Contains(out, "fake") {
		t.Errorf("expected the image draw included once the settle delay elapsed, got %q", out)
	}
}

// TestTabsLayoutView_InjectsInlineImages is the golden-output test category
// both prior architecture reviews flagged as missing: an assertion on
// TabsLayout.View()'s actual composited output, not just the pure diff
// functions underneath it.
func TestTabsLayoutView_InjectsInlineImages(t *testing.T) {
	a := loggedInApp()
	a.width, a.height = 80, 40
	a.graphicsProtocol = imgview.ProtocolKitty
	a.feed, _ = a.feed.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	a.feed, _ = a.feed.Update(screens.SharedConfigMsg{InlineImagesEnabled: true})
	a.feed = a.feed.SetPosts([]model.Post{
		{ID: "p1", AuthorUsername: "alice", Content: "hi\n\n![a](https://example.com/a.png)\n\nbye"},
	}, "")
	slots := a.feed.VisibleInlineImages()
	if len(slots) != 1 {
		t.Fatalf("setup: expected 1 slot from the feed, got %d: %+v", len(slots), slots)
	}
	a.inlineImageCache = map[string]string{inlineImageCacheKey(slots[0], a.graphicsProtocol, nil): "\x1b_Gfake\x1b\\"}

	out := (TabsLayout{}).View(a)
	if !strings.Contains(out, "\x1b_Gfake\x1b\\") {
		t.Errorf("expected TabsLayout.View to composite the cached inline image, got: %q", out)
	}
}

// TestMillerLayoutView_InjectsInlineImages is the same golden-output check
// for MillerLayout's Feed detail pane specifically — the exact code path
// that silently rendered nothing before this branch (MillerLayout never
// called injectInlineImages at all, and Feed's detail pane has no other
// source of truth for its rendering width than MillerLayout.InlineImageSlots
// computing it fresh, so this exercises that whole chain end to end).
func TestMillerLayoutView_InjectsInlineImages(t *testing.T) {
	a := loggedInApp()
	a.width, a.height = 100, 40
	a.graphicsProtocol = imgview.ProtocolKitty
	a.feed, _ = a.feed.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	a.feed, _ = a.feed.Update(screens.SharedConfigMsg{InlineImagesEnabled: true})
	a.feed = a.feed.SetPosts([]model.Post{
		{ID: "p1", AuthorUsername: "alice", Content: "hi\n\n![a](https://example.com/a.png)\n\nbye"},
	}, "")
	a.feed, _ = a.feed.Update(screens.FeedDetailRepliesMsg{PostID: "p1", Replies: nil})

	slots, _, _, _ := MillerLayout{}.InlineImageSlots(a)
	if len(slots) != 1 {
		t.Fatalf("setup: expected 1 slot from Miller's Feed detail pane, got %d: %+v", len(slots), slots)
	}
	a.inlineImageCache = map[string]string{inlineImageCacheKey(slots[0], a.graphicsProtocol, nil): "\x1b_Gfake\x1b\\"}

	out := (MillerLayout{}).View(a)
	if !strings.Contains(out, "\x1b_Gfake\x1b\\") {
		t.Errorf("expected MillerLayout.View to composite the cached inline image in the Feed detail pane, got: %q", out)
	}
}

// --- MillerLayout.HasFocusedInput: backgrounded Circ/CMail room ---

// TestMillerHasFocusedInput_ChatroomsDoesNotTrapNavAtFocusMenu is a
// regression test: a Circ room stays in chatroomModeDetail (so
// ChatroomsModel.InputFocused() stays true) for as long as it's open, even
// after the user has pressed "left" to move Miller's own focus away to the
// spaces column. Before this fix, HasFocusedInput only checked
// InputFocused(), so every subsequent j/k/up/down was swallowed into the
// backgrounded room's Update instead of reaching HandleNav's focusMenu case
// (navigateTabBy) — the room was reachable but effectively un-leavable via
// vertical nav.
func TestMillerHasFocusedInput_ChatroomsDoesNotTrapNavAtFocusMenu(t *testing.T) {
	a := loggedInApp()
	a.active = screenChatrooms
	a.chatrooms = a.chatrooms.SetRooms([]model.Room{{Slug: "r1", Name: "r1"}})
	a.chatrooms, _ = a.chatrooms.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !a.chatrooms.InputFocused() {
		t.Fatal("setup: expected the room to be open (InputFocused true)")
	}

	a.focus = focusList
	if !(MillerLayout{}).HasFocusedInput(a) {
		t.Error("expected HasFocusedInput true while focus is still on the room")
	}

	a.focus = focusMenu
	if (MillerLayout{}).HasFocusedInput(a) {
		t.Error("expected HasFocusedInput false once focus has moved to the spaces column, even with the room still open")
	}
}

// TestMillerHasFocusedInput_CMailDoesNotTrapNavAtFocusMenu mirrors the Circ
// case above for CMail, which has the identical mode-based InputFocused
// pattern (cmailModeDetail).
func TestMillerHasFocusedInput_CMailDoesNotTrapNavAtFocusMenu(t *testing.T) {
	a := loggedInApp()
	a.active = screenCMail
	a.cmail = a.cmail.SetConversations([]model.Conversation{{ID: "c1"}})
	a.cmail, _ = a.cmail.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !a.cmail.InputFocused() {
		t.Fatal("setup: expected the conversation to be open (InputFocused true)")
	}

	a.focus = focusList
	if !(MillerLayout{}).HasFocusedInput(a) {
		t.Error("expected HasFocusedInput true while focus is still on the conversation")
	}

	a.focus = focusMenu
	if (MillerLayout{}).HasFocusedInput(a) {
		t.Error("expected HasFocusedInput false once focus has moved to the spaces column, even with the conversation still open")
	}
}

// --- ModalMaxWidth: keep the centered-on-full-width image modal off Miller's sidebar ---

func TestTabsLayout_ModalMaxWidth_IsFullWidth(t *testing.T) {
	if got := (TabsLayout{}).ModalMaxWidth(120); got != 120 {
		t.Errorf("ModalMaxWidth(120) = %d, want 120 (no side chrome)", got)
	}
}

func TestMillerLayout_ModalMaxWidth_ReservesTwiceTheSidebar(t *testing.T) {
	// millerSidebarWidth = 22; centering means avoiding a left-side
	// obstruction of width r requires reserving 2*r off the total.
	if got := (MillerLayout{}).ModalMaxWidth(120); got != 120-2*millerSidebarWidth {
		t.Errorf("ModalMaxWidth(120) = %d, want %d", got, 120-2*millerSidebarWidth)
	}
}

func TestMillerLayout_ModalMaxWidth_FloorsAtOne(t *testing.T) {
	if got := (MillerLayout{}).ModalMaxWidth(10); got != 1 {
		t.Errorf("ModalMaxWidth(10) = %d, want 1 (terminal narrower than 2x sidebar)", got)
	}
}
