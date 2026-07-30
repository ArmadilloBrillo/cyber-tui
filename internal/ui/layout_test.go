package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/ragnar/cyber-tui/internal/model"
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

func TestTabVisualState_CMailDetail_DoesNotPersistWhenBackgrounded(t *testing.T) {
	a := loggedInApp()
	a.cmail = a.cmail.SetConversations([]model.Conversation{
		{ID: "c1", Participants: []model.User{{Username: a.currentUser.Username}, {Username: "molly"}}},
	})
	cm, _ := a.cmail.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	cm, _ = cm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	a.cmail = cm
	a.active = screenCMail

	a.active = screenFeed // background C-Mail — mode is still cmailModeDetail here, stale

	selected, detail := tabVisualState(a, screenCMail)
	if selected {
		t.Error("expected C-Mail to not be selected after switching to Feed")
	}
	if detail {
		t.Error("expected C-Mail to report detail=false while backgrounded — its subscription was already torn down, so this would misrepresent a closed conversation as still open")
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

func TestRenderTabBar_NoDetailMarkerForBackgroundedCMail(t *testing.T) {
	a := loggedInApp()
	a.width = 100
	a.cmail = a.cmail.SetConversations([]model.Conversation{
		{ID: "c1", Participants: []model.User{{Username: a.currentUser.Username}, {Username: "molly"}}},
	})
	cm, _ := a.cmail.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	cm, _ = cm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	a.cmail = cm
	a.active = screenFeed // background C-Mail

	out := ansi.Strip(TabsLayout{}.renderTabBar(a))
	if strings.Contains(out, "›") {
		t.Errorf("expected no detail marker for backgrounded C-Mail (its conversation was already torn down), got: %q", out)
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
