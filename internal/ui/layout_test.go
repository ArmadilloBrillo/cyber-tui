package ui

import (
	"fmt"
	"strings"
	"testing"

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

func (f fakeModalRenderer) renderThemePicker(a App) string { return "" }
func (f fakeModalRenderer) renderThemeEditor(a App) string { return "" }
func (f fakeModalRenderer) renderPathPrompt(a App) string  { return "" }
func (f fakeModalRenderer) renderHelpModal(a App) string   { return "" }
func (f fakeModalRenderer) renderURLPicker(a App) string   { return "" }
func (f fakeModalRenderer) renderImageModal(a App) string  { return "" }
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
		inlineImageCache:  map[string]string{inlineImageCacheKey(slot, imgview.ProtocolKitty): "\x1b_Gfake\x1b\\"},
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

// TestInjectInlineImages_PaintGenTogglesTrailingLineBytes confirms the
// inlineImagePaintGen dirty-marker mechanism actually does what
// syncInlineImages/injectInlineImages depend on it for: without it, a
// selection change that recolors a band row elsewhere (but doesn't change
// which images are visible or their cache contents) leaves this function's
// output byte-identical to last frame, so Bubble Tea's per-line diff skips
// resending it and the image never gets repainted — this was the reason a
// tea.ClearScreen (later replaced by this mechanism) was needed at all. The
// output must differ whenever inlineImagePaintGen's parity differs, and be
// identical when parity is unchanged, since that parity flip is the only
// signal that forces Bubble Tea to reissue the line.
func TestInjectInlineImages_PaintGenTogglesTrailingLineBytes(t *testing.T) {
	slot := screens.InlineImageSlot{Key: "post:p1:0", URL: "https://example.com/a.png", Row: 1, ColIndent: 2}
	cache := map[string]string{inlineImageCacheKey(slot, imgview.ProtocolITerm2): "\x1b]1337;fake\x07"}
	slots := []screens.InlineImageSlot{slot}

	a0 := App{width: 40, height: 10, graphicsProtocol: imgview.ProtocolITerm2, inlineImageCache: cache, inlineImagePaintGen: 0}
	a1 := App{width: 40, height: 10, graphicsProtocol: imgview.ProtocolITerm2, inlineImageCache: cache, inlineImagePaintGen: 1}
	a2 := App{width: 40, height: 10, graphicsProtocol: imgview.ProtocolITerm2, inlineImageCache: cache, inlineImagePaintGen: 2}

	out0 := injectInlineImages(a0, "base", slots, 5, 7)
	out1 := injectInlineImages(a1, "base", slots, 5, 7)
	out2 := injectInlineImages(a2, "base", slots, 5, 7)

	if out0 == out1 {
		t.Error("expected an odd paint generation to produce different output than an even one")
	}
	if out0 != out2 {
		t.Error("expected two even paint generations to produce identical output")
	}
}

// TestInjectInlineImages_BlanksPendingErasures confirms a pending erasure
// (a moved or removed image's stale rectangle — see
// syncInlineImageErasures) actually produces blank-fill escape sequences in
// injectInlineImages' output, and that iteration order is deterministic —
// two calls with equal-content but distinct map instances (Go map
// iteration is randomized) must produce byte-identical output, since a
// non-deterministic byte order would make Bubble Tea's line-diff think the
// line changed even when the erasure set itself didn't.
func TestInjectInlineImages_BlanksPendingErasures(t *testing.T) {
	pending := map[string]inlineImageRect{
		"post:p1:0":  {Row: 6, Col: 9, Cols: 20, Rows: 3},
		"reply:r1:0": {Row: 12, Col: 9, Cols: 20, Rows: 3},
	}
	a := App{width: 40, height: 10, graphicsProtocol: imgview.ProtocolITerm2, pendingInlineImageErasures: pending}

	out := injectInlineImages(a, "base", nil, 0, 0)
	erase := ansi.EraseCharacter(20)
	for i := 0; i < 3; i++ {
		if want := fmt.Sprintf("\x1b[%d;9H%s", 6+i, erase); !strings.Contains(out, want) {
			t.Errorf("expected blank-fill %q for post:p1:0's rect in output, got %q", want, out)
		}
		if want := fmt.Sprintf("\x1b[%d;9H%s", 12+i, erase); !strings.Contains(out, want) {
			t.Errorf("expected blank-fill %q for reply:r1:0's rect in output, got %q", want, out)
		}
	}

	// A fresh map instance with the same logical contents (different
	// insertion order, since Go maps don't preserve one) must still
	// produce byte-identical output.
	pending2 := map[string]inlineImageRect{
		"reply:r1:0": {Row: 12, Col: 9, Cols: 20, Rows: 3},
		"post:p1:0":  {Row: 6, Col: 9, Cols: 20, Rows: 3},
	}
	a2 := App{width: 40, height: 10, graphicsProtocol: imgview.ProtocolITerm2, pendingInlineImageErasures: pending2}
	if out2 := injectInlineImages(a2, "base", nil, 0, 0); out2 != out {
		t.Errorf("expected deterministic output regardless of map insertion order, got %q vs %q", out2, out)
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
	a.inlineImageCache = map[string]string{inlineImageCacheKey(slots[0], a.graphicsProtocol): "\x1b_Gfake\x1b\\"}

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
	a.inlineImageCache = map[string]string{inlineImageCacheKey(slots[0], a.graphicsProtocol): "\x1b_Gfake\x1b\\"}

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
