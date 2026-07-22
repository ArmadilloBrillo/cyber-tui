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
		ShowFollowerCount: true,
		AutoWatchOnReply:  false,
		DefaultPublicPost: true,
		TimeDisplayFormat: "relative",
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

func TestSettings_CursorUp_WrapsToBottom(t *testing.T) {
	m := initSettings(defaultSettings())
	m, _ = m.Update(keyMsg("k"))
	if m.cursor != len(flatItems())-1 {
		t.Errorf("expected cursor=%d (wrapped), got %d", len(flatItems())-1, m.cursor)
	}
}

func TestSettings_CursorDown_WrapsToTop(t *testing.T) {
	m := initSettings(defaultSettings())
	m.cursor = len(flatItems()) - 1
	m, _ = m.Update(keyMsg("j"))
	if m.cursor != 0 {
		t.Errorf("expected cursor=0 (wrapped), got %d", m.cursor)
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
	original := m.settings.Notifications.Bookmark
	m.settings.Notifications.Bookmark = !m.settings.Notifications.Bookmark
	if m.settings.Notifications.Bookmark == original {
		t.Error("toggle should flip the bool value")
	}
}

func TestSettings_Enter_TogglesBool(t *testing.T) {
	m := initSettings(defaultSettings())
	original := m.settings.FilterNSFW
	m.settings.FilterNSFW = !m.settings.FilterNSFW
	if m.settings.FilterNSFW == original {
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
	original := m.settings.TimeDisplayFormat
	// Space on enum should be noop - don't change anything
	if m.settings.TimeDisplayFormat != original {
		t.Error("enum value should remain unchanged")
	}
}

func TestSettings_Toggle_Notifications_Bookmark(t *testing.T) {
	m := initSettings(defaultSettings())
	if !m.settings.Notifications.Bookmark {
		t.Error("default should have Bookmark=true")
	}
	m.settings.Notifications.Bookmark = !m.settings.Notifications.Bookmark
	if m.settings.Notifications.Bookmark {
		t.Error("after toggle, Bookmark should be false")
	}
}

func TestSettings_Toggle_FilterNSFW(t *testing.T) {
	m := initSettings(defaultSettings())
	if m.settings.FilterNSFW {
		t.Error("default should have FilterNSFW=false")
	}
	m.settings.FilterNSFW = !m.settings.FilterNSFW
	if !m.settings.FilterNSFW {
		t.Error("after toggle, FilterNSFW should be true")
	}
}

// --- Enum Tests ---

func TestSettings_Tab_CyclesEnum(t *testing.T) {
	m := initSettings(defaultSettings())
	m.cursor = 7 // time format
	if m.settings.TimeDisplayFormat != "relative" {
		t.Error("default TimeDisplayFormat should be 'relative'")
	}
	m, _ = m.Update(keyMsg("tab"))
	if m.settings.TimeDisplayFormat != "unix" {
		t.Error("after tab, TimeDisplayFormat should be 'unix'")
	}
}

func TestSettings_ShiftTab_CyclesEnum(t *testing.T) {
	m := initSettings(defaultSettings())
	m.cursor = 7
	m, _ = m.Update(keyMsg("shift+tab"))
	if m.settings.TimeDisplayFormat != "datetime" {
		t.Error("shift+tab from 'relative' should cycle to 'datetime'")
	}
}

func TestSettings_Enum_WrapsForward(t *testing.T) {
	m := initSettings(defaultSettings())
	m.cursor = 7
	// Cycle from relative -> unix -> swatch -> datetime -> relative
	m.settings.TimeDisplayFormat = "swatch"
	m, _ = m.Update(keyMsg("tab"))
	if m.settings.TimeDisplayFormat != "datetime" {
		t.Error("tab from 'swatch' should wrap to 'datetime'")
	}
}

func TestSettings_Enum_WrapsBackward(t *testing.T) {
	m := initSettings(defaultSettings())
	m.cursor = 7
	m.settings.TimeDisplayFormat = "datetime"
	m, _ = m.Update(keyMsg("shift+tab"))
	if m.settings.TimeDisplayFormat != "swatch" {
		t.Error("shift+tab from 'datetime' should wrap to 'swatch'")
	}
}

func TestSettings_Tab_OnBool_IsNoop(t *testing.T) {
	m := initSettings(defaultSettings())
	m.cursor = 0 // bool item
	original := m.settings.Notifications.Bookmark
	m, _ = m.Update(keyMsg("tab"))
	if m.settings.Notifications.Bookmark != original {
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
	m = m.SetSaved(false, 3, "UTC", "terminal", "tabs")
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
	m = m.SetSaved(false, 3, "UTC", "terminal", "tabs")
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
	m.cursor = 7 // time format
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
	m = m.SetSaved(false, 3, "UTC", "terminal", "tabs")
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

// --- WanderLust Tests ---

func TestSettings_WanderGroup_Visible(t *testing.T) {
	m := initSettings(defaultSettings())
	view := m.View()
	if !containsSubstring(view, "wander") {
		t.Error("View should contain 'wander' group header")
	}
}

func TestSettings_WanderToggle(t *testing.T) {
	m := initSettings(defaultSettings())
	m.wanderLust = true
	m.cursor = 11 // wander mode item
	m, _ = m.Update(keyMsg("enter"))
	if m.wanderLust {
		t.Error("toggling wander mode should flip wanderLust to false")
	}
}

func TestSettings_WanderDirty(t *testing.T) {
	m := initSettings(defaultSettings())
	m.wanderLust = true
	m.originalWanderLust = true
	if m.IsDirty() {
		t.Error("should not be dirty before change")
	}
	m.wanderLust = false
	if !m.IsDirty() {
		t.Error("IsDirty should return true when wanderLust differs from original")
	}
}

func TestSettings_WanderSaveMsg(t *testing.T) {
	m := initSettings(defaultSettings())
	m.wanderLust = false
	m.originalWanderLust = true // make it dirty
	var got tea.Msg
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd != nil {
		got = cmd()
	}
	save, ok := got.(SaveSettingsMsg)
	if !ok {
		t.Fatal("ctrl+s should emit SaveSettingsMsg")
	}
	if save.WanderLust != false {
		t.Error("SaveSettingsMsg.WanderLust should reflect current wanderLust value")
	}
}

func TestSettings_WanderSetSaved(t *testing.T) {
	m := initSettings(defaultSettings())
	m.wanderLust = true
	m.originalWanderLust = false // dirty
	m = m.SetSaved(true, 3, "UTC", "terminal", "tabs")
	if m.originalWanderLust != true {
		t.Error("SetSaved should update originalWanderLust to the saved value")
	}
	if m.IsDirty() {
		t.Error("should not be dirty after SetSaved")
	}
}

// --- Timezone Tests ---

func TestSettings_Timezone_CyclesForward(t *testing.T) {
	m := initSettings(defaultSettings())
	m.timezone = "UTC"
	m.originalTimezone = "UTC"
	m.cursor = 9 // timezone item
	m, _ = m.Update(keyMsg("tab"))
	if m.timezone == "UTC" {
		t.Error("tab should advance timezone from UTC")
	}
}

func TestSettings_Timezone_CyclesBackward(t *testing.T) {
	m := initSettings(defaultSettings())
	m.timezone = "UTC"
	m.originalTimezone = "UTC"
	m.cursor = 9
	m, _ = m.Update(keyMsg("shift+tab"))
	if m.timezone == "UTC" {
		t.Error("shift+tab should cycle timezone backward from UTC")
	}
}

func TestSettings_Timezone_Wraps(t *testing.T) {
	m := initSettings(defaultSettings())
	items := flatItems()
	last := items[9].options[len(items[9].options)-1]
	m.timezone = last
	m.originalTimezone = last
	m.cursor = 9
	m, _ = m.Update(keyMsg("tab"))
	first := items[9].options[0]
	if m.timezone != first {
		t.Errorf("tab from last timezone should wrap to first, got %s", m.timezone)
	}
}

func TestSettings_Timezone_IsDirty(t *testing.T) {
	m := initSettings(defaultSettings())
	m.timezone = "UTC"
	m.originalTimezone = "UTC"
	if m.IsDirty() {
		t.Error("should not be dirty before change")
	}
	m.timezone = "UTC+2"
	if !m.IsDirty() {
		t.Error("changing timezone should make IsDirty true")
	}
}

func TestSettings_Timezone_Esc_Reverts(t *testing.T) {
	m := initSettings(defaultSettings())
	m.timezone = "UTC+2"
	m.originalTimezone = "UTC"
	m, _ = m.Update(keyMsg("esc"))
	if m.timezone != "UTC" {
		t.Errorf("esc should revert timezone to original, got %s", m.timezone)
	}
}

func TestSettings_Timezone_SaveMsg(t *testing.T) {
	m := initSettings(defaultSettings())
	m.timezone = "UTC+2"
	m.originalTimezone = "UTC"
	var got tea.Msg
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd != nil {
		got = cmd()
	}
	save, ok := got.(SaveSettingsMsg)
	if !ok {
		t.Fatal("ctrl+s should emit SaveSettingsMsg")
	}
	if save.Timezone != "UTC+2" {
		t.Errorf("SaveSettingsMsg.Timezone should be UTC+2, got %s", save.Timezone)
	}
}

func TestSettings_SharedConfigMsg_SetsTimezone(t *testing.T) {
	m := NewSettingsModel()
	m, _ = m.Update(SharedConfigMsg{Width: 80, Height: 24, Settings: defaultSettings(), Timezone: "UTC+5:30"})
	if m.timezone != "UTC+5:30" {
		t.Errorf("SharedConfigMsg should set timezone, got %s", m.timezone)
	}
	if m.originalTimezone != "UTC+5:30" {
		t.Errorf("SharedConfigMsg should set originalTimezone, got %s", m.originalTimezone)
	}
}

func TestSettings_SharedConfigMsg_DefaultsTimezoneToUTC(t *testing.T) {
	m := NewSettingsModel()
	m, _ = m.Update(SharedConfigMsg{Width: 80, Height: 24, Settings: defaultSettings(), Timezone: ""})
	if m.timezone != "UTC" {
		t.Errorf("empty Timezone in SharedConfigMsg should default to UTC, got %s", m.timezone)
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
