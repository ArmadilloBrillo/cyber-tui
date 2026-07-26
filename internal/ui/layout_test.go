package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
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
