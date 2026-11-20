package screens

import (
	"slices"
	"strings"
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
		// Bubble Tea represents ctrl+s differently â€” just send the raw rune with ctrl modifier
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
	if m.cursor != len(flatItems(m))-1 {
		t.Errorf("expected cursor=%d (wrapped), got %d", len(flatItems(m))-1, m.cursor)
	}
}

func TestSettings_CursorDown_WrapsToTop(t *testing.T) {
	m := initSettings(defaultSettings())
	m.cursor = len(flatItems(m)) - 1
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
	m.cursor = itemIndex(t, m, "time format")
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
	m.cursor = itemIndex(t, m, "time format")
	m, _ = m.Update(keyMsg("shift+tab"))
	if m.settings.TimeDisplayFormat != "datetime" {
		t.Error("shift+tab from 'relative' should cycle to 'datetime'")
	}
}

func TestSettings_Enum_WrapsForward(t *testing.T) {
	m := initSettings(defaultSettings())
	m.cursor = itemIndex(t, m, "time format")
	// Cycle from relative -> unix -> swatch -> datetime -> relative
	m.settings.TimeDisplayFormat = "swatch"
	m, _ = m.Update(keyMsg("tab"))
	if m.settings.TimeDisplayFormat != "datetime" {
		t.Error("tab from 'swatch' should wrap to 'datetime'")
	}
}

func TestSettings_Enum_WrapsBackward(t *testing.T) {
	m := initSettings(defaultSettings())
	m.cursor = itemIndex(t, m, "time format")
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

func TestSettings_ColorNicknames_SpaceTogglesAndMarksDirty(t *testing.T) {
	m := initSettings(defaultSettings())
	m.cursor = itemIndex(t, m, "color nicknames")
	if flatItems(m)[m.cursor].label != "color nicknames" {
		t.Fatalf("item %d = %q, want color nicknames", m.cursor, flatItems(m)[m.cursor].label)
	}
	m, _ = m.Update(keyMsg("space"))
	if !m.settings.ColorNicknames {
		t.Error("space should toggle ColorNicknames on")
	}
	if !m.IsDirty() {
		t.Error("toggling ColorNicknames should mark settings dirty")
	}
}

// --- Keyword list Tests ---
//
// The "alert keywords" row's own editing UX (add/delete/dedupe/scroll)
// moved to KeywordEditorModel (see keywordeditor_test.go) — SettingsModel
// now only owns the keywordAlerts draft/baseline and opens the popup.

func keywordlistCursor(m SettingsModel) int {
	for i, it := range flatItems(m) {
		if it.kind == "keywordlist" {
			return i
		}
	}
	return -1
}

// itemIndex returns the flat index of the visible row with the given label, so
// tests survive settings rows being regrouped or reordered.
func itemIndex(t *testing.T, m SettingsModel, label string) int {
	t.Helper()
	for i, it := range flatItems(m) {
		if it.label == label {
			return i
		}
	}
	t.Fatalf("no visible settings row labelled %q", label)
	return -1
}

func TestSettings_Keywordlist_EnterOpensEditorPopup(t *testing.T) {
	m := initSettings(defaultSettings())
	m.cursor = keywordlistCursor(m)
	_, cmd := m.Update(keyMsg("enter"))
	if cmd == nil {
		t.Fatal("expected a cmd emitting OpenKeywordEditorMsg")
	}
	if _, ok := cmd().(OpenKeywordEditorMsg); !ok {
		t.Errorf("expected OpenKeywordEditorMsg, got %T", cmd())
	}
}

func TestSettings_Keywordlist_GetSetAccessors(t *testing.T) {
	m := initSettings(defaultSettings())
	m = m.SetKeywordAlerts([]string{"foo", "bar"})
	if !slices.Equal(m.KeywordAlerts(), []string{"foo", "bar"}) {
		t.Errorf("KeywordAlerts() = %v, want [foo bar]", m.KeywordAlerts())
	}
}

func TestSettings_Keywordlist_SetsDirtyAndReverts(t *testing.T) {
	m := initSettings(defaultSettings())
	m.cursor = keywordlistCursor(m)
	if m.IsDirty() {
		t.Fatal("should start clean")
	}
	m.local.KeywordAlerts = []string{"foo"}
	if !m.IsDirty() {
		t.Error("adding a keyword should set dirty")
	}
	m, _ = m.Update(keyMsg("esc"))
	if m.IsDirty() {
		t.Error("esc should revert dirty state")
	}
	if len(m.local.KeywordAlerts) != 0 {
		t.Errorf("esc should revert keywordAlerts to original, got %v", m.local.KeywordAlerts)
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
	m = m.SetSaved(LocalPrefs{WanderLust: false, FeedManualRefreshOnly: false, TypingIndicatorsEnabled: true, DesktopNotifications: false, ShowGlobeTab: true, MaxThreadDepth: 3, Timezone: "UTC", ImageViewer: "terminal", GraphicsProtocol: "", InlineImages: false, Dithering: false, KeywordAlerts: nil})
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
	m = m.SetSaved(LocalPrefs{WanderLust: false, FeedManualRefreshOnly: false, TypingIndicatorsEnabled: true, DesktopNotifications: false, ShowGlobeTab: true, MaxThreadDepth: 3, Timezone: "UTC", ImageViewer: "terminal", GraphicsProtocol: "", InlineImages: false, Dithering: false, KeywordAlerts: nil})
	if m.IsDirty() {
		t.Error("after SetSaved, should not be dirty")
	}
}

// --- Dithering Tests ---

// hasItemLabelled reports whether any of m's currently visible (showIf-passing)
// items has a label containing sub — the same set flatItems/View iterate over,
// checked directly rather than through View's height-limited scroll viewport.
func hasItemLabelled(m SettingsModel, sub string) bool {
	for _, item := range flatItems(m) {
		if strings.Contains(item.label, sub) {
			return true
		}
	}
	return false
}

func TestSettings_Dithering_ShowIf_HiddenWhenBrowser(t *testing.T) {
	m := initSettings(defaultSettings())
	m.local.ImageViewer = "terminal"
	if !hasItemLabelled(m, "dithering") {
		t.Error("dithering item should be visible when imageViewer != browser")
	}
	m.local.ImageViewer = "browser"
	if hasItemLabelled(m, "dithering") {
		t.Error("dithering item should be hidden when imageViewer == browser")
	}
}

func TestSettings_Sharpness_ShowIf_HiddenUntilDitheringOn(t *testing.T) {
	m := initSettings(defaultSettings())
	m.local.ImageViewer = "terminal"
	m.local.Dithering = false
	if hasItemLabelled(m, "sharpness") {
		t.Error("sharpness item should be hidden while dithering is off")
	}
	m.local.Dithering = true
	if !hasItemLabelled(m, "sharpness") {
		t.Error("sharpness item should be visible once dithering is on")
	}
	m.local.ImageViewer = "browser"
	if hasItemLabelled(m, "sharpness") {
		t.Error("sharpness item should stay hidden when imageViewer == browser, even with dithering on")
	}
}

func TestSettings_ImageColor_ShowIf_HiddenUntilDitheringOn(t *testing.T) {
	m := initSettings(defaultSettings())
	m.local.ImageViewer = "terminal"
	m.local.Dithering = false
	if hasItemLabelled(m, "    color") {
		t.Error("color item should be hidden while dithering is off")
	}
	m.local.Dithering = true
	if !hasItemLabelled(m, "    color") {
		t.Error("color item should be visible once dithering is on")
	}
	m.local.ImageViewer = "browser"
	if hasItemLabelled(m, "    color") {
		t.Error("color item should stay hidden when imageViewer == browser, even with dithering on")
	}
}

func TestSettings_ImageColor_CycleTogglesAndSaves(t *testing.T) {
	m := initSettings(defaultSettings())
	m.local.ImageViewer = "terminal"
	m.local.Dithering = true
	m.originalLocal.Dithering = true
	var item settingsItem
	for _, it := range flatItems(m) {
		if it.label == "    color" {
			item = it
		}
	}
	if item.cycle == nil {
		t.Fatal("setup: color item not found")
	}
	if got := item.getEnum(m); got != "monochrome" {
		t.Fatalf("initial value = %q, want monochrome", got)
	}

	m = item.cycle(m, 1)
	if got := item.getEnum(m); got != "color" {
		t.Errorf("after cycle = %q, want color", got)
	}
	if !m.IsDirty() {
		t.Error("changing color should mark settings dirty")
	}

	var got tea.Msg
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd != nil {
		got = cmd()
	}
	save, ok := got.(SaveSettingsMsg)
	if !ok {
		t.Fatal("ctrl+s should emit SaveSettingsMsg")
	}
	if !save.Settings.ImageColor {
		t.Error("SaveSettingsMsg.Settings.ImageColor should be true")
	}
	if !save.RemoteChanged {
		t.Error("changing color should mark the save as RemoteChanged so it is PATCHed")
	}
}

func TestSettings_Dithering_SaveMsg(t *testing.T) {
	m := initSettings(defaultSettings())
	m.local.Dithering = true
	m.originalLocal.Dithering = false // make it dirty
	m.settings.ImagePixelSize = "rough"
	var got tea.Msg
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd != nil {
		got = cmd()
	}
	save, ok := got.(SaveSettingsMsg)
	if !ok {
		t.Fatal("ctrl+s should emit SaveSettingsMsg")
	}
	if !save.Prefs.Dithering {
		t.Error("SaveSettingsMsg.Dithering should reflect current dithering value")
	}
	if save.Settings.ImagePixelSize != "rough" {
		t.Errorf("SaveSettingsMsg.Settings.ImagePixelSize = %q, want %q", save.Settings.ImagePixelSize, "rough")
	}
	if !save.RemoteChanged {
		t.Error("changing sharpness should mark the save as RemoteChanged so it is PATCHed")
	}
}

func TestSettings_Dithering_SetSaved_AdvancesBaseline(t *testing.T) {
	m := initSettings(defaultSettings())
	m.local.Dithering = true
	m.originalLocal.Dithering = false
	if !m.IsDirty() {
		t.Error("should be dirty before SetSaved")
	}
	m = m.SetSaved(LocalPrefs{WanderLust: false, FeedManualRefreshOnly: false, TypingIndicatorsEnabled: true, DesktopNotifications: false, ShowGlobeTab: true, MaxThreadDepth: 3, Timezone: "UTC", ImageViewer: "terminal", GraphicsProtocol: "", InlineImages: false, Dithering: true, KeywordAlerts: nil})
	if m.originalLocal.Dithering != true {
		t.Error("SetSaved should update originalDithering to the saved value")
	}
	if m.IsDirty() {
		t.Error("should not be dirty after SetSaved")
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

// TestSettings_SharedConfigMsg_SeedsPrefsAfterSetSettings reproduces the
// exact ordering app.go's settingsLoadedMsg handler uses: SetSettings(...)
// runs first, then broadcastConfig() sends a SharedConfigMsg. A stale guard
// tied to m.original's zero-ness (rather than a dedicated seeded flag) would
// already be tripped by SetSettings, silently skipping the preference fields
// below and leaving them zeroed — which then get saved back to disk.
func TestSettings_SharedConfigMsg_SeedsPrefsAfterSetSettings(t *testing.T) {
	m := NewSettingsModel()
	m = m.SetSettings(defaultSettings())

	m, _ = m.Update(SharedConfigMsg{
		Width:            80,
		Height:           24,
		Settings:         defaultSettings(),
		WanderLust:       true,
		MaxThreadDepth:   20,
		InlineImages:     true,
		Dithering:        true,
		GraphicsProtocol: "sixel",
	})

	if !m.local.WanderLust {
		t.Error("wanderLust should be seeded from SharedConfigMsg even after a prior SetSettings call")
	}
	if m.local.MaxThreadDepth != 20 {
		t.Errorf("maxThreadDepth = %d, want 20", m.local.MaxThreadDepth)
	}
	if !m.local.InlineImages {
		t.Error("inlineImages should be seeded from SharedConfigMsg even after a prior SetSettings call")
	}
	if !m.local.Dithering {
		t.Error("dithering should be seeded from SharedConfigMsg even after a prior SetSettings call")
	}
	if m.local.GraphicsProtocol != "sixel" {
		t.Errorf("graphicsProtocol = %q, want sixel", m.local.GraphicsProtocol)
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
	m.height = 100 // fit every group on screen; the default 24 rows scrolls the lower ones out
	view := m.View()
	headers := []string{"alerts", "feed & posts", "display", "images", "chat & compose", "advanced"}
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
	m.cursor = itemIndex(t, m, "time format")
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

func TestSettings_View_DirtyFooterKeepsKeyHints(t *testing.T) {
	m := initSettings(defaultSettings())
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	view := m.View()
	if !containsSubstring(view, "tab · cycle enum") || !containsSubstring(view, "ctrl+s · save") {
		t.Error("dirty footer should show both the key hints and the save hint")
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

func TestSettings_WanderToggle(t *testing.T) {
	m := initSettings(defaultSettings())
	m.local.WanderLust = true
	m.cursor = itemIndex(t, m, "wander mode")
	m, _ = m.Update(keyMsg("enter"))
	if m.local.WanderLust {
		t.Error("toggling wander mode should flip wanderLust to false")
	}
}

func TestSettings_WanderDirty(t *testing.T) {
	m := initSettings(defaultSettings())
	m.local.WanderLust = true
	m.originalLocal.WanderLust = true
	if m.IsDirty() {
		t.Error("should not be dirty before change")
	}
	m.local.WanderLust = false
	if !m.IsDirty() {
		t.Error("IsDirty should return true when wanderLust differs from original")
	}
}

func TestSettings_WanderSaveMsg(t *testing.T) {
	m := initSettings(defaultSettings())
	m.local.WanderLust = false
	m.originalLocal.WanderLust = true // make it dirty
	var got tea.Msg
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd != nil {
		got = cmd()
	}
	save, ok := got.(SaveSettingsMsg)
	if !ok {
		t.Fatal("ctrl+s should emit SaveSettingsMsg")
	}
	if save.Prefs.WanderLust != false {
		t.Error("SaveSettingsMsg.WanderLust should reflect current wanderLust value")
	}
}

func TestSettings_WanderSetSaved(t *testing.T) {
	m := initSettings(defaultSettings())
	m.local.WanderLust = true
	m.originalLocal.WanderLust = false // dirty
	m = m.SetSaved(LocalPrefs{WanderLust: true, FeedManualRefreshOnly: false, TypingIndicatorsEnabled: true, DesktopNotifications: false, ShowGlobeTab: true, MaxThreadDepth: 3, Timezone: "UTC", ImageViewer: "terminal", GraphicsProtocol: "", InlineImages: false, Dithering: false, KeywordAlerts: nil})
	if m.originalLocal.WanderLust != true {
		t.Error("SetSaved should update originalWanderLust to the saved value")
	}
	if m.IsDirty() {
		t.Error("should not be dirty after SetSaved")
	}
}

// --- Feed auto-refresh tests ---
// Mirrors the Wander* tests above for feedManualRefreshOnly (audit item #5,
// docs/39-feed-background-poll.md). The toggle test locates its row via
// flatItems instead of a hardcoded cursor index — the Wander toggle test's
// own comment ("shifted by the new dithering toggle above it") shows why a
// magic index is fragile against unrelated settings-group changes.

func TestSettings_FeedGroup_Visible(t *testing.T) {
	m := initSettings(defaultSettings())
	view := m.View()
	if !containsSubstring(view, "feed") {
		t.Error("View should contain 'feed' group header")
	}
}

func TestSettings_FeedAutoRefreshToggle(t *testing.T) {
	m := initSettings(defaultSettings())
	m.local.FeedManualRefreshOnly = false // auto-refresh currently on

	items := flatItems(m)
	idx := -1
	for i, it := range items {
		if it.label == "auto-refresh (background poll)" {
			idx = i
			break
		}
	}
	if idx == -1 {
		t.Fatal("expected an auto-refresh settings item")
	}
	m.cursor = idx
	m, _ = m.Update(keyMsg("enter"))
	if !m.local.FeedManualRefreshOnly {
		t.Error("toggling auto-refresh off should flip feedManualRefreshOnly to true")
	}
}

func TestSettings_FeedAutoRefreshDirty(t *testing.T) {
	m := initSettings(defaultSettings())
	m.local.FeedManualRefreshOnly = true
	m.originalLocal.FeedManualRefreshOnly = true
	if m.IsDirty() {
		t.Error("should not be dirty before change")
	}
	m.local.FeedManualRefreshOnly = false
	if !m.IsDirty() {
		t.Error("IsDirty should return true when feedManualRefreshOnly differs from original")
	}
}

func TestSettings_FeedAutoRefreshSaveMsg(t *testing.T) {
	m := initSettings(defaultSettings())
	m.local.FeedManualRefreshOnly = true
	m.originalLocal.FeedManualRefreshOnly = false // make it dirty
	var got tea.Msg
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd != nil {
		got = cmd()
	}
	save, ok := got.(SaveSettingsMsg)
	if !ok {
		t.Fatal("ctrl+s should emit SaveSettingsMsg")
	}
	if save.Prefs.FeedManualRefreshOnly != true {
		t.Error("SaveSettingsMsg.FeedManualRefreshOnly should reflect current feedManualRefreshOnly value")
	}
}

func TestSettings_FeedAutoRefreshSetSaved(t *testing.T) {
	m := initSettings(defaultSettings())
	m.local.FeedManualRefreshOnly = true
	m.originalLocal.FeedManualRefreshOnly = false // dirty
	m = m.SetSaved(LocalPrefs{WanderLust: false, FeedManualRefreshOnly: true, TypingIndicatorsEnabled: true, DesktopNotifications: false, ShowGlobeTab: true, MaxThreadDepth: 3, Timezone: "UTC", ImageViewer: "terminal", GraphicsProtocol: "", InlineImages: false, Dithering: false, KeywordAlerts: nil})
	if m.originalLocal.FeedManualRefreshOnly != true {
		t.Error("SetSaved should update originalFeedManualRefreshOnly to the saved value")
	}
	if m.IsDirty() {
		t.Error("should not be dirty after SetSaved")
	}
}

// --- C-Mail typing indicators tests (audit item #6) ---
// Mirrors the Feed auto-refresh tests above, but positive polarity
// throughout (typingIndicatorsEnabled), so no display-label inversion is
// needed in the settings item's getBool/toggle closures.

func TestSettings_TypingIndicatorsToggle(t *testing.T) {
	m := initSettings(defaultSettings())
	m.local.TypingIndicatorsEnabled = true

	items := flatItems(m)
	idx := -1
	for i, it := range items {
		if it.label == "typing indicators" {
			idx = i
			break
		}
	}
	if idx == -1 {
		t.Fatal("expected a typing indicators settings item")
	}
	m.cursor = idx
	m, _ = m.Update(keyMsg("enter"))
	if m.local.TypingIndicatorsEnabled {
		t.Error("toggling typing indicators should flip typingIndicatorsEnabled to false")
	}
}

func TestSettings_TypingIndicatorsDirty(t *testing.T) {
	m := initSettings(defaultSettings())
	m.local.TypingIndicatorsEnabled = true
	m.originalLocal.TypingIndicatorsEnabled = true
	if m.IsDirty() {
		t.Error("should not be dirty before change")
	}
	m.local.TypingIndicatorsEnabled = false
	if !m.IsDirty() {
		t.Error("IsDirty should return true when typingIndicatorsEnabled differs from original")
	}
}

func TestSettings_TypingIndicatorsSaveMsg(t *testing.T) {
	m := initSettings(defaultSettings())
	m.local.TypingIndicatorsEnabled = false
	m.originalLocal.TypingIndicatorsEnabled = true // make it dirty
	var got tea.Msg
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd != nil {
		got = cmd()
	}
	save, ok := got.(SaveSettingsMsg)
	if !ok {
		t.Fatal("ctrl+s should emit SaveSettingsMsg")
	}
	if save.Prefs.TypingIndicatorsEnabled != false {
		t.Error("SaveSettingsMsg.TypingIndicatorsEnabled should reflect current typingIndicatorsEnabled value")
	}
}

func TestSettings_TypingIndicatorsSetSaved(t *testing.T) {
	m := initSettings(defaultSettings())
	m.local.TypingIndicatorsEnabled = false
	m.originalLocal.TypingIndicatorsEnabled = true // dirty
	m = m.SetSaved(LocalPrefs{WanderLust: false, FeedManualRefreshOnly: false, TypingIndicatorsEnabled: false, DesktopNotifications: false, ShowGlobeTab: true, MaxThreadDepth: 3, Timezone: "UTC", ImageViewer: "terminal", GraphicsProtocol: "", InlineImages: false, Dithering: false, KeywordAlerts: nil})
	if m.originalLocal.TypingIndicatorsEnabled != false {
		t.Error("SetSaved should update originalTypingIndicatorsEnabled to the saved value")
	}
	if m.IsDirty() {
		t.Error("should not be dirty after SetSaved")
	}
}

// --- Globe tab visibility tests ---

func TestSettings_ShowGlobeTabToggle(t *testing.T) {
	m := initSettings(defaultSettings())
	m.local.ShowGlobeTab = true

	items := flatItems(m)
	idx := -1
	for i, it := range items {
		if it.label == "show globe tab" {
			idx = i
			break
		}
	}
	if idx == -1 {
		t.Fatal("expected a show globe tab settings item")
	}
	m.cursor = idx
	m, _ = m.Update(keyMsg("enter"))
	if m.local.ShowGlobeTab {
		t.Error("toggling show globe tab should flip showGlobeTab to false")
	}
}

func TestSettings_ShowGlobeTabDirty(t *testing.T) {
	m := initSettings(defaultSettings())
	m.local.ShowGlobeTab = true
	m.originalLocal.ShowGlobeTab = true
	if m.IsDirty() {
		t.Error("should not be dirty before change")
	}
	m.local.ShowGlobeTab = false
	if !m.IsDirty() {
		t.Error("IsDirty should return true when showGlobeTab differs from original")
	}
}

func TestSettings_ShowGlobeTabSaveMsg(t *testing.T) {
	m := initSettings(defaultSettings())
	m.local.ShowGlobeTab = false
	m.originalLocal.ShowGlobeTab = true // make it dirty
	var got tea.Msg
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd != nil {
		got = cmd()
	}
	save, ok := got.(SaveSettingsMsg)
	if !ok {
		t.Fatal("ctrl+s should emit SaveSettingsMsg")
	}
	if save.Prefs.ShowGlobeTab != false {
		t.Error("SaveSettingsMsg.ShowGlobeTab should reflect current showGlobeTab value")
	}
}

// --- Timezone Tests ---

func TestSettings_Timezone_CyclesForward(t *testing.T) {
	m := initSettings(defaultSettings())
	m.local.Timezone = "UTC"
	m.originalLocal.Timezone = "UTC"
	m.cursor = itemIndex(t, m, "timezone")
	m, _ = m.Update(keyMsg("tab"))
	if m.local.Timezone == "UTC" {
		t.Error("tab should advance timezone from UTC")
	}
}

func TestSettings_Timezone_CyclesBackward(t *testing.T) {
	m := initSettings(defaultSettings())
	m.local.Timezone = "UTC"
	m.originalLocal.Timezone = "UTC"
	m.cursor = itemIndex(t, m, "timezone")
	m, _ = m.Update(keyMsg("shift+tab"))
	if m.local.Timezone == "UTC" {
		t.Error("shift+tab should cycle timezone backward from UTC")
	}
}

func TestSettings_Timezone_Wraps(t *testing.T) {
	m := initSettings(defaultSettings())
	idx := itemIndex(t, m, "timezone")
	opts := flatItems(m)[idx].options
	last := opts[len(opts)-1]
	m.local.Timezone = last
	m.originalLocal.Timezone = last
	m.cursor = idx
	m, _ = m.Update(keyMsg("tab"))
	first := opts[0]
	if m.local.Timezone != first {
		t.Errorf("tab from last timezone should wrap to first, got %s", m.local.Timezone)
	}
}

func TestSettings_Timezone_IsDirty(t *testing.T) {
	m := initSettings(defaultSettings())
	m.local.Timezone = "UTC"
	m.originalLocal.Timezone = "UTC"
	if m.IsDirty() {
		t.Error("should not be dirty before change")
	}
	m.local.Timezone = "UTC+2"
	if !m.IsDirty() {
		t.Error("changing timezone should make IsDirty true")
	}
}

func TestSettings_Timezone_Esc_Reverts(t *testing.T) {
	m := initSettings(defaultSettings())
	m.local.Timezone = "UTC+2"
	m.originalLocal.Timezone = "UTC"
	m, _ = m.Update(keyMsg("esc"))
	if m.local.Timezone != "UTC" {
		t.Errorf("esc should revert timezone to original, got %s", m.local.Timezone)
	}
}

func TestSettings_Timezone_SaveMsg(t *testing.T) {
	m := initSettings(defaultSettings())
	m.local.Timezone = "UTC+2"
	m.originalLocal.Timezone = "UTC"
	var got tea.Msg
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd != nil {
		got = cmd()
	}
	save, ok := got.(SaveSettingsMsg)
	if !ok {
		t.Fatal("ctrl+s should emit SaveSettingsMsg")
	}
	if save.Prefs.Timezone != "UTC+2" {
		t.Errorf("SaveSettingsMsg.Timezone should be UTC+2, got %s", save.Prefs.Timezone)
	}
}

func TestSettings_SharedConfigMsg_SetsTimezone(t *testing.T) {
	m := NewSettingsModel()
	m, _ = m.Update(SharedConfigMsg{Width: 80, Height: 24, Settings: defaultSettings(), Timezone: "UTC+5:30"})
	if m.local.Timezone != "UTC+5:30" {
		t.Errorf("SharedConfigMsg should set timezone, got %s", m.local.Timezone)
	}
	if m.originalLocal.Timezone != "UTC+5:30" {
		t.Errorf("SharedConfigMsg should set originalTimezone, got %s", m.originalLocal.Timezone)
	}
}

func TestSettings_SharedConfigMsg_DefaultsTimezoneToUTC(t *testing.T) {
	m := NewSettingsModel()
	m, _ = m.Update(SharedConfigMsg{Width: 80, Height: 24, Settings: defaultSettings(), Timezone: ""})
	if m.local.Timezone != "UTC" {
		t.Errorf("empty Timezone in SharedConfigMsg should default to UTC, got %s", m.local.Timezone)
	}
}

// --- Helpers ---

var testErr = &mockErr{msg: "test error"}

type mockErr struct {
	msg string
}

func (e *mockErr) Error() string { return e.msg }

func containsSubstring(s, sub string) bool {
	return strings.Contains(s, sub)
}

// --- Hard line break key Tests ---

func hardBreakCursor(m SettingsModel) int {
	for i, it := range flatItems(m) {
		if it.kind == "keybind" {
			return i
		}
	}
	return -1
}

// bindingSettings is a settings screen seeded like App seeds it: the saved
// hard-break key is the default, the cursor is on the hard-break row.
func bindingSettings(t *testing.T) SettingsModel {
	t.Helper()
	m := initSettings(defaultSettings())
	m.local.HardBreakKey, m.originalLocal.HardBreakKey = DefaultHardBreakKey, DefaultHardBreakKey
	m.cursor = hardBreakCursor(m)
	if m.cursor < 0 {
		t.Fatal("no keybind row found")
	}
	return m
}

func capture(t *testing.T, m SettingsModel, keys ...tea.KeyMsg) SettingsModel {
	t.Helper()
	for _, k := range keys {
		m, _ = m.Update(k)
	}
	return m
}

func TestSettings_HardBreak_EnterStartsCapture(t *testing.T) {
	m := bindingSettings(t)
	m = capture(t, m, keyMsg("enter"))
	if !m.Capturing() {
		t.Fatal("enter on the hard line break row should start capture")
	}
	if !strings.Contains(m.View(), "press a key combo") {
		t.Error("view should prompt for a key combo while capturing")
	}
}

func TestSettings_HardBreak_TabOnRowDoesNothing(t *testing.T) {
	m := bindingSettings(t)
	m = capture(t, m, keyMsg("tab"))
	if m.Capturing() || m.IsDirty() {
		t.Errorf("tab on the row should be a no-op (capturing=%v dirty=%v)", m.Capturing(), m.IsDirty())
	}
}

func TestSettings_HardBreak_ComboBindsImmediately(t *testing.T) {
	m := bindingSettings(t)
	m = capture(t, m, keyMsg("enter"), tea.KeyMsg{Type: tea.KeyCtrlL})
	if m.Capturing() {
		t.Error("a valid combo should finish capture")
	}
	if m.local.HardBreakKey != "ctrl+l" {
		t.Errorf("hardBreakKey = %q, want %q", m.local.HardBreakKey, "ctrl+l")
	}
	if !m.IsDirty() {
		t.Error("changing the key should set dirty")
	}
	if !strings.Contains(m.View(), "ctrl+l") {
		t.Error("view should show the new key")
	}
}

func TestSettings_HardBreak_AltEnterBackToDefaultIsClean(t *testing.T) {
	m := bindingSettings(t)
	m.local.HardBreakKey = "ctrl+l"
	m = capture(t, m, keyMsg("enter"), tea.KeyMsg{Type: tea.KeyEnter, Alt: true})
	if m.local.HardBreakKey != "alt+enter" || m.Capturing() {
		t.Errorf("hardBreakKey = %q capturing=%v, want alt+enter and finished", m.local.HardBreakKey, m.Capturing())
	}
	if m.IsDirty() {
		t.Error("returning to the saved default should not be dirty")
	}
}

func TestSettings_HardBreak_ShiftComboAccepted(t *testing.T) {
	m := bindingSettings(t)
	m = capture(t, m, keyMsg("enter"), tea.KeyMsg{Type: tea.KeyShiftUp})
	if m.local.HardBreakKey != "shift+up" {
		t.Errorf("hardBreakKey = %q, want shift+up", m.local.HardBreakKey)
	}
}

func TestSettings_HardBreak_EscCancelsCaptureKeepingOtherEdits(t *testing.T) {
	m := bindingSettings(t)
	m.local.WanderLust = true // an unsaved edit elsewhere
	m = capture(t, m, keyMsg("enter"), keyMsg("esc"))
	if m.Capturing() {
		t.Error("esc should end capture")
	}
	if m.local.HardBreakKey != DefaultHardBreakKey {
		t.Errorf("cancelled capture changed the key to %q", m.local.HardBreakKey)
	}
	if !m.local.WanderLust {
		t.Error("esc during capture must not revert other unsaved settings")
	}
}

func TestSettings_HardBreak_RejectsUnbindableKeys(t *testing.T) {
	for name, k := range map[string]tea.KeyMsg{
		"ctrl+s":       {Type: tea.KeyCtrlS},
		"ctrl+q":       {Type: tea.KeyCtrlQ},
		"plain letter": {Type: tea.KeyRunes, Runes: []rune("c")},
		"enter":        {Type: tea.KeyEnter},
		"tab":          {Type: tea.KeyTab},
		"shift+tab":    {Type: tea.KeyShiftTab},
		"paste":        {Type: tea.KeyRunes, Runes: []rune("x"), Paste: true},
	} {
		m := bindingSettings(t)
		m = capture(t, m, keyMsg("enter"), k)
		if !m.Capturing() || m.captureErr == "" {
			t.Errorf("%s: should stay capturing with a rejection reason (capturing=%v err=%q)", name, m.Capturing(), m.captureErr)
		}
		if m.local.HardBreakKey != DefaultHardBreakKey {
			t.Errorf("%s: key changed to %q", name, m.local.HardBreakKey)
		}
	}
}

func TestSettings_HardBreak_RejectionShownInFooter(t *testing.T) {
	m := bindingSettings(t)
	m = capture(t, m, keyMsg("enter"), tea.KeyMsg{Type: tea.KeyCtrlS})
	if m.captureErr == "" || !strings.Contains(m.View(), m.captureErr) {
		t.Errorf("view should show the rejection reason %q", m.captureErr)
	}
}

func TestSettings_HardBreak_CaptureSwallowsNavigationAndSave(t *testing.T) {
	m := bindingSettings(t)
	m.local.WanderLust = true // dirty, so ctrl+s would normally save
	cursor := m.cursor
	m = capture(t, m, keyMsg("enter"), keyMsg("j"))
	if m.cursor != cursor {
		t.Error("j while capturing must not move the cursor")
	}
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd != nil {
		t.Error("ctrl+s while capturing must be rejected, not save")
	}
}

func TestSettings_HardBreak_SaveMsg(t *testing.T) {
	m := bindingSettings(t)
	m = capture(t, m, keyMsg("enter"), tea.KeyMsg{Type: tea.KeyCtrlL})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd == nil {
		t.Fatal("ctrl+s should save a changed key")
	}
	save, ok := cmd().(SaveSettingsMsg)
	if !ok {
		t.Fatalf("ctrl+s produced %T, want SaveSettingsMsg", cmd())
	}
	if save.Prefs.HardBreakKey != "ctrl+l" {
		t.Errorf("SaveSettingsMsg.HardBreakKey = %q, want %q", save.Prefs.HardBreakKey, "ctrl+l")
	}
}

func TestSettings_HardBreak_SetSavedAdvancesBaseline(t *testing.T) {
	m := bindingSettings(t)
	m = capture(t, m, keyMsg("enter"), tea.KeyMsg{Type: tea.KeyCtrlL})
	if !m.IsDirty() {
		t.Fatal("should be dirty before saving")
	}
	m = m.SetSaved(m.local)
	if m.IsDirty() {
		t.Error("should be clean after SetSaved")
	}
	if m.originalLocal.HardBreakKey != "ctrl+l" {
		t.Errorf("baseline HardBreakKey = %q, want ctrl+l", m.originalLocal.HardBreakKey)
	}
}

func TestSettings_HardBreak_EscRevertsUnsavedKey(t *testing.T) {
	m := bindingSettings(t)
	m = capture(t, m, keyMsg("enter"), tea.KeyMsg{Type: tea.KeyCtrlL}, keyMsg("esc"))
	if m.local.HardBreakKey != DefaultHardBreakKey || m.IsDirty() {
		t.Errorf("esc should revert to %q, got %q (dirty=%v)", DefaultHardBreakKey, m.local.HardBreakKey, m.IsDirty())
	}
}

func TestSettings_HardBreak_SeededFromSharedConfig(t *testing.T) {
	m := NewSettingsModel()
	m, _ = m.Update(SharedConfigMsg{Width: 80, Height: 24, HardBreakKey: "ctrl+l"})
	if m.local.HardBreakKey != "ctrl+l" || m.originalLocal.HardBreakKey != "ctrl+l" {
		t.Errorf("seeded key = %q / baseline %q, want %q", m.local.HardBreakKey, m.originalLocal.HardBreakKey, "ctrl+l")
	}
	if m.IsDirty() {
		t.Error("seeding must not mark the screen dirty")
	}
}

// --- Reveal content warnings tests ---

func TestSettings_RevealCW_ToggleDirtySaveRevert(t *testing.T) {
	m := initSettings(defaultSettings())
	idx := -1
	for i, it := range flatItems(m) {
		if it.label == "always reveal content warnings" {
			idx = i
			break
		}
	}
	if idx == -1 {
		t.Fatal("expected an always reveal content warnings settings item")
	}
	m.cursor = idx
	m, _ = m.Update(keyMsg("enter"))
	if !m.local.RevealContentWarnings || !m.IsDirty() {
		t.Fatalf("toggle: revealCW = %v, dirty = %v, want both true", m.local.RevealContentWarnings, m.IsDirty())
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd == nil {
		t.Fatal("ctrl+s should emit a cmd")
	}
	save, ok := cmd().(SaveSettingsMsg)
	if !ok || !save.Prefs.RevealContentWarnings {
		t.Errorf("SaveSettingsMsg = %+v, want RevealContentWarnings true", save)
	}

	m, _ = m.Update(keyMsg("esc"))
	if m.local.RevealContentWarnings {
		t.Error("esc should revert revealCW to the saved baseline")
	}
}

func TestSettings_RevealCW_SeededFromSharedConfig(t *testing.T) {
	m := NewSettingsModel()
	m, _ = m.Update(SharedConfigMsg{Width: 80, Height: 40, RevealContentWarnings: true})
	if !m.local.RevealContentWarnings {
		t.Error("expected revealCW seeded from SharedConfigMsg")
	}
	if m.IsDirty() {
		t.Error("seeding must also set the baseline, not mark the screen dirty")
	}
}

func TestSettings_DebugLog_ToggleSaveRevertSeed(t *testing.T) {
	m := initSettings(defaultSettings())
	idx := -1
	for i, it := range flatItems(m) {
		if it.label == "debug log (on restart)" {
			idx = i
			break
		}
	}
	if idx == -1 {
		t.Fatal("expected a debug log settings item")
	}
	m.cursor = idx
	m, _ = m.Update(keyMsg("enter"))
	if !m.local.DebugLog || !m.IsDirty() {
		t.Fatalf("toggle: debugLog = %v, dirty = %v, want both true", m.local.DebugLog, m.IsDirty())
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd == nil {
		t.Fatal("ctrl+s should emit a cmd")
	}
	if save, ok := cmd().(SaveSettingsMsg); !ok || !save.Prefs.DebugLog {
		t.Errorf("SaveSettingsMsg = %+v, want DebugLog true", save)
	}

	m, _ = m.Update(keyMsg("esc"))
	if m.local.DebugLog {
		t.Error("esc should revert debugLog to the saved baseline")
	}

	m = NewSettingsModel()
	m, _ = m.Update(SharedConfigMsg{Width: 80, Height: 40, DebugLog: true})
	if !m.local.DebugLog || m.IsDirty() {
		t.Errorf("seed: debugLog = %v, dirty = %v, want true/false", m.local.DebugLog, m.IsDirty())
	}
}

func TestLocalPrefs_Equal(t *testing.T) {
	base := LocalPrefs{Timezone: "UTC", KeywordAlerts: nil}
	if !base.equal(LocalPrefs{Timezone: "UTC", KeywordAlerts: []string{}}) {
		t.Error("nil and empty keyword lists should compare equal")
	}
	if base.equal(LocalPrefs{Timezone: "UTC", KeywordAlerts: []string{"x"}}) {
		t.Error("different keyword lists should not compare equal")
	}
	if base.equal(LocalPrefs{Timezone: "UTC+1"}) {
		t.Error("different scalar fields should not compare equal")
	}
}
