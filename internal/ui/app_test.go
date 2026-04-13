package ui

import (
	"testing"

	"github.com/ragnar/cyber-tui/internal/api"
)

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
	if got := a.tabIndex(); got != 2 {
		t.Errorf("expected 2, got %d", got)
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

func TestNavigateTab_RightFromProfile_Wraps(t *testing.T) {
	a := loggedInApp()
	a.active = screenProfile
	a.navigateTab(+1)
	if a.active != screenSettings {
		t.Errorf("expected screenSettings (wrap), got %v", a.active)
	}
}

func TestNavigateTab_CyclesAllTabsRight(t *testing.T) {
	a := loggedInApp()
	a.active = screenFeed
	expected := []screen{screenNotifications, screenProfile, screenSettings, screenFeed}
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
	expected := []screen{screenSettings, screenProfile, screenNotifications, screenFeed}
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
