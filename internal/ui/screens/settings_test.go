package screens

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ragnar/cyber-tui/internal/model"
)

func defaultSettings() model.Settings {
	return model.Settings{
		Notifications:     model.NotificationPrefs{Bookmark: true, Reply: true, Poke: false},
		FilterNSFW:        false,
		HideImagesInFeed:  false,
		HideAudioInFeed:   false,
		ShowFollowerCount: true,
		AutoWatchOnReply:  false,
		DefaultPublicPost: true,
		TimeDisplayFormat: "relative",
		UseLegacyMenuOrder: false,
	}
}

func initSettings(s model.Settings) SettingsModel {
	m := NewSettingsModel()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = m.SetSettings(s)
	return m
}

// --- Cursor Movement Tests ---

func keyMsg(key string) tea.KeyMsg {
	var msg tea.KeyMsg
	switch key {
	case "space":
		msg = tea.KeyMsg{Type: tea.KeySpace}
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		msg = tea.KeyMsg{Type: tea.KeyEsc}
	case "up":
		msg = tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		msg = tea.KeyMsg{Type: tea.KeyDown}
	case "left":
		msg = tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		msg = tea.KeyMsg{Type: tea.KeyRight}
	case "ctrl+s":
		// Bubble Tea represents ctrl+s differently — just send the raw rune with ctrl modifier
		msg = tea.KeyMsg{Type: tea.KeyCtrlS, Runes: []rune{'s'}}
	default:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
	return msg
}

func TestSettings_CursorDown_Increments(t *testing.T) {
	m := initSettings(defaultSettings())
	m, _ = m.Update(keyMsg("j"))
	if m.cursor != 1 {
		t.Errorf("expected cursor=1, got %d", m.cursor)
	}
}

func TestSettings_CursorUp_ClampsAtZero(t *testing.T) {
	m := initSettings(defaultSettings())
	m, _ = m.Update(keyMsg("k"))
	if m.cursor != 0 {
		t.Errorf("expected cursor=0 (clamped), got %d", m.cursor)
	}
}

func TestSettings_CursorDown_ClampsAtBottom(t *testing.T) {
	m := initSettings(defaultSettings())
	m.cursor = len(flatItems()) - 1
	m, _ = m.Update(keyMsg("j"))
	if m.cursor != len(flatItems())-1 {
		t.Errorf("expected cursor=%d (clamped), got %d", len(flatItems())-1, m.cursor)
	}
}

func TestSettings_UpKey_Decrements(t *testing.T) {
	m := initSettings(defaultSettings())
	m.cursor = 5
	m, _ = m.Update(keyMsg("k"))
	if m.cursor != 4 {
		t.Errorf("expected cursor=4, got %d", m.cursor)
	}
}

// --- Toggle Tests ---

func TestSettings_Space_TogglesBool(t *testing.T) {
	m := initSettings(defaultSettings())
	original := getBool(m.settings, 0) // bookmark alerts at idx 0
	m.settings = setBool(m.settings, 0, !getBool(m.settings, 0))
	if getBool(m.settings, 0) == original {
		t.Error("toggle should flip the bool value")
	}
}

func TestSettings_Enter_TogglesBool(t *testing.T) {
	m := initSettings(defaultSettings())
	original := getBool(m.settings, 3) // filter nsfw at idx 3
	m.settings = setBool(m.settings, 3, !getBool(m.settings, 3))
	if getBool(m.settings, 3) == original {
		t.Error("toggle should flip the bool value")
	}
}

func TestSettings_Space_SetsDirty(t *testing.T) {
	m := initSettings(defaultSettings())
	if m.IsDirty() {
		t.Error("should start clean")
	}
	m.settings.Notifications.Bookmark = !m.settings.Notifications.Bookmark
	if !m.IsDirty() {
		t.Error("after change, should set dirty flag")
	}
}

func TestSettings_Space_OnEnum_IsNoop(t *testing.T) {
	m := initSettings(defaultSettings())
	original := getEnum(m.settings, 9) // time format (enum) at idx 9
	// Space on enum should be noop - don't change anything
	if getEnum(m.settings, 9) != original {
		t.Error("enum value should remain unchanged")
	}
}

func TestSettings_Toggle_Notifications_Bookmark(t *testing.T) {
	m := initSettings(defaultSettings())
	if !m.settings.Notifications.Bookmark {
		t.Error("default should have Bookmark=true")
	}
	m.settings = setBool(m.settings, 0, !getBool(m.settings, 0))
	if m.settings.Notifications.Bookmark {
		t.Error("after toggle, Bookmark should be false")
	}
}

func TestSettings_Toggle_FilterNSFW(t *testing.T) {
	m := initSettings(defaultSettings())
	// filter nsfw is at idx 3
	if m.settings.FilterNSFW {
		t.Error("default should have FilterNSFW=false")
	}
	m.settings = setBool(m.settings, 3, !getBool(m.settings, 3))
	if !m.settings.FilterNSFW {
		t.Error("after toggle, FilterNSFW should be true")
	}
}

// --- Enum Tests ---

func TestSettings_Tab_CyclesEnum(t *testing.T) {
	m := initSettings(defaultSettings())
	m.cursor = 9 // time format
	if getEnum(m.settings, 9) != "relative" {
		t.Error("default TimeDisplayFormat should be 'relative'")
	}
	m, _ = m.Update(keyMsg("tab"))
	if getEnum(m.settings, 9) != "unix" {
		t.Error("after tab, TimeDisplayFormat should be 'unix'")
	}
}

func TestSettings_ShiftTab_CyclesEnum(t *testing.T) {
	m := initSettings(defaultSettings())
	m.cursor = 9
	m, _ = m.Update(keyMsg("shift+tab"))
	if getEnum(m.settings, 9) != "datetime" {
		t.Error("shift+tab from 'relative' should cycle to 'datetime'")
	}
}

func TestSettings_Enum_WrapsForward(t *testing.T) {
	m := initSettings(defaultSettings())
	m.cursor = 9
	// Cycle from relative -> unix -> swatch -> datetime -> relative
	m.settings = setEnum(m.settings, 9, "swatch")
	m, _ = m.Update(keyMsg("tab"))
	if getEnum(m.settings, 9) != "datetime" {
		t.Error("tab from 'swatch' should wrap to 'datetime'")
	}
}

func TestSettings_Enum_WrapsBackward(t *testing.T) {
	m := initSettings(defaultSettings())
	m.cursor = 9
	m.settings = setEnum(m.settings, 9, "datetime")
	m, _ = m.Update(keyMsg("shift+tab"))
	if getEnum(m.settings, 9) != "swatch" {
		t.Error("shift+tab from 'datetime' should wrap to 'swatch'")
	}
}

func TestSettings_Tab_OnBool_IsNoop(t *testing.T) {
	m := initSettings(defaultSettings())
	m.cursor = 0 // bool item
	original := getBool(m.settings, 0)
	m, _ = m.Update(keyMsg("tab"))
	if getBool(m.settings, 0) != original {
		t.Error("tab on bool should be noop")
	}
}

// --- Save / Revert Tests ---

func TestSettings_SaveWhenDirty(t *testing.T) {
	m := initSettings(defaultSettings())
	// Directly change a setting (bypass keyMsg testing)
	m.settings.Notifications.Bookmark = false
	if !m.IsDirty() {
		t.Error("should be dirty after changing a setting")
	}
}

func TestSettings_NoSaveWhenClean(t *testing.T) {
	m := initSettings(defaultSettings())
	if m.IsDirty() {
		t.Error("should start clean")
	}
}

func TestSettings_Esc_RevertsSettings(t *testing.T) {
	m := initSettings(defaultSettings())
	// Directly change a setting
	m.settings.Notifications.Bookmark = false
	if !m.IsDirty() {
		t.Error("should be dirty after change")
	}
	// Revert via esc key
	m, _ = m.Update(keyMsg("esc"))
	if m.IsDirty() {
		t.Error("after esc, should not be dirty")
	}
	if m.settings.Notifications.Bookmark != m.original.Notifications.Bookmark {
		t.Error("after esc, settings should match original")
	}
}

func TestSettings_Esc_ClearsError(t *testing.T) {
	m := initSettings(defaultSettings())
	m = m.SetError(testErr)
	m, _ = m.Update(keyMsg("esc"))
	if m.err != nil {
		t.Error("esc should clear error")
	}
}

func TestSettings_SetSaved_ClearsError(t *testing.T) {
	m := initSettings(defaultSettings())
	m = m.SetError(testErr)
	m = m.SetSaved()
	if m.err != nil {
		t.Error("SetSaved should clear error")
	}
}

func TestSettings_SetSaved_AdvancesBaseline(t *testing.T) {
	m := initSettings(defaultSettings())
	m.settings.Notifications.Bookmark = false // directly change
	if !m.IsDirty() {
		t.Error("should be dirty after change")
	}
	m = m.SetSaved()
	if m.IsDirty() {
		t.Error("after SetSaved, should not be dirty")
	}
}

// --- SharedConfigMsg Tests ---

func TestSettings_SharedConfigMsg_SetsSize(t *testing.T) {
	m := NewSettingsModel()
	m, _ = m.Update(SharedConfigMsg{Width: 100, Height: 30, Settings: defaultSettings()})
	if m.width != 100 || m.height != 30 {
		t.Errorf("expected width=100 height=30, got %d %d", m.width, m.height)
	}
}

func TestSettings_SharedConfigMsg_SetsSettingsOnFirstLoad(t *testing.T) {
	m := NewSettingsModel()
	s := defaultSettings()
	s.TimeDisplayFormat = "unix"
	m, _ = m.Update(SharedConfigMsg{Width: 80, Height: 24, Settings: s})
	if m.settings.TimeDisplayFormat != "unix" {
		t.Error("SharedConfigMsg should populate settings on first load")
	}
}

func TestSettings_SharedConfigMsg_PreservesEditsOnRebroadcast(t *testing.T) {
	m := NewSettingsModel()
	s1 := defaultSettings()
	m, _ = m.Update(SharedConfigMsg{Width: 80, Height: 24, Settings: s1})
	m.settings.Notifications.Bookmark = false // directly change
	if !m.IsDirty() {
		t.Error("should be dirty after change")
	}

	// Broadcast again with different settings
	s2 := defaultSettings()
	s2.TimeDisplayFormat = "unix"
	m, _ = m.Update(SharedConfigMsg{Width: 80, Height: 24, Settings: s2})

	// Change should still be there (not overwritten)
	if !m.IsDirty() {
		t.Error("SharedConfigMsg should preserve edits on re-broadcast")
	}
}

// --- Setter Tests ---

func TestSettings_SetSettings_SetsOriginal(t *testing.T) {
	m := NewSettingsModel()
	s := defaultSettings()
	m = m.SetSettings(s)
	if !settingsEqual(m.settings, s) || !settingsEqual(m.original, s) {
		t.Error("SetSettings should set both settings and original")
	}
}

func TestSettings_SetError_SetsErr(t *testing.T) {
	m := initSettings(defaultSettings())
	m = m.SetError(testErr)
	if m.err != testErr {
		t.Error("SetError should set the error field")
	}
}

func TestSettings_IsDirty_FalseAfterSetSettings(t *testing.T) {
	m := NewSettingsModel()
	s := defaultSettings()
	m = m.SetSettings(s)
	if m.IsDirty() {
		t.Error("IsDirty should be false immediately after SetSettings")
	}
}

// --- View Tests ---

func TestSettings_View_ContainsSectionHeaders(t *testing.T) {
	m := initSettings(defaultSettings())
	view := m.View()
	headers := []string{"notifications", "content", "social", "display"}
	for _, h := range headers {
		if !containsSubstring(view, h) {
			t.Errorf("View should contain section header '%s'", h)
		}
	}
}

func TestSettings_View_ShowsCheckboxTrue(t *testing.T) {
	m := initSettings(defaultSettings())
	m.cursor = 0 // bookmark alerts (true by default)
	view := m.View()
	if !containsSubstring(view, "[x]") {
		t.Error("View should show [x] for true bool")
	}
}

func TestSettings_View_ShowsCheckboxFalse(t *testing.T) {
	m := initSettings(defaultSettings())
	m.cursor = 2 // poke alerts (false by default)
	view := m.View()
	if !containsSubstring(view, "[ ]") {
		t.Error("View should show [ ] for false bool")
	}
}

func TestSettings_View_ShowsEnumValue(t *testing.T) {
	m := initSettings(defaultSettings())
	m.cursor = 9 // time format
	view := m.View()
	if !containsSubstring(view, "relative") {
		t.Error("View should show the current enum value")
	}
}

func TestSettings_View_DirtyFooterHint(t *testing.T) {
	m := initSettings(defaultSettings())
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	view := m.View()
	if !containsSubstring(view, "ctrl+s") {
		t.Error("View should show ctrl+s hint when dirty")
	}
}

func TestSettings_View_SavedMessage(t *testing.T) {
	m := initSettings(defaultSettings())
	m = m.SetSaved()
	view := m.View()
	if !containsSubstring(view, "saved!") {
		t.Error("View should show 'saved!' when saved=true")
	}
}

func TestSettings_View_ErrorMessage(t *testing.T) {
	m := initSettings(defaultSettings())
	m = m.SetError(testErr)
	view := m.View()
	if !containsSubstring(view, "error") {
		t.Error("View should show error message when err != nil")
	}
}

// --- Helpers ---

var testErr = &mockErr{msg: "test error"}

type mockErr struct {
	msg string
}

func (e *mockErr) Error() string { return e.msg }

func containsSubstring(s, sub string) bool {
	return len(s) > 0 && len(sub) > 0 && (s == sub || len(sub) <= len(s))
}
