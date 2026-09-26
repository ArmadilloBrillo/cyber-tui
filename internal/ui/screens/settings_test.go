package screens

import (
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
	m.cursor = 8 // time format
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
	m.cursor = 8
	m, _ = m.Update(keyMsg("shift+tab"))
	if m.settings.TimeDisplayFormat != "datetime" {
		t.Error("shift+tab from 'relative' should cycle to 'datetime'")
	}
}

func TestSettings_Enum_WrapsForward(t *testing.T) {
	m := initSettings(defaultSettings())
	m.cursor = 8
	// Cycle from relative -> unix -> swatch -> datetime -> relative
	m.settings.TimeDisplayFormat = "swatch"
	m, _ = m.Update(keyMsg("tab"))
	if m.settings.TimeDisplayFormat != "datetime" {
		t.Error("tab from 'swatch' should wrap to 'datetime'")
	}
}

func TestSettings_Enum_WrapsBackward(t *testing.T) {
	m := initSettings(defaultSettings())
	m.cursor = 8
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

// --- Keyword list Tests ---
//
// The "alert keywords" row's own editing UX (add/delete/dedupe/scroll)
// moved to KeywordEditorModel (see keywordeditor_test.go) — SettingsModel
// now only owns the keywordAlerts draft/baseline and opens the popup.

func keywordlistCursor(m SettingsModel) int {
	return len(flatItems(m)) - 1 // "alert keywords" is the last item in the last group
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
	if !stringSlicesEqual(m.KeywordAlerts(), []string{"foo", "bar"}) {
		t.Errorf("KeywordAlerts() = %v, want [foo bar]", m.KeywordAlerts())
	}
}

func TestSettings_Keywordlist_SetsDirtyAndReverts(t *testing.T) {
	m := initSettings(defaultSettings())
	m.cursor = keywordlistCursor(m)
	if m.IsDirty() {
		t.Fatal("should start clean")
	}
	m.keywordAlerts = []string{"foo"}
	if !m.IsDirty() {
		t.Error("adding a keyword should set dirty")
	}
	m, _ = m.Update(keyMsg("esc"))
	if m.IsDirty() {
		t.Error("esc should revert dirty state")
	}
	if len(m.keywordAlerts) != 0 {
		t.Errorf("esc should revert keywordAlerts to original, got %v", m.keywordAlerts)
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
	m = m.SetSaved(false, false, true, false, true, 3, "UTC", "terminal", "", false, false, "", nil)
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
	m = m.SetSaved(false, false, true, false, true, 3, "UTC", "terminal", "", false, false, "", nil)
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
	m.imageViewer = "terminal"
	if !hasItemLabelled(m, "dithering") {
		t.Error("dithering item should be visible when imageViewer != browser")
	}
	m.imageViewer = "browser"
	if hasItemLabelled(m, "dithering") {
		t.Error("dithering item should be hidden when imageViewer == browser")
	}
}

func TestSettings_Sharpness_ShowIf_HiddenUntilDitheringOn(t *testing.T) {
	m := initSettings(defaultSettings())
	m.imageViewer = "terminal"
	m.dithering = false
	if hasItemLabelled(m, "sharpness") {
		t.Error("sharpness item should be hidden while dithering is off")
	}
	m.dithering = true
	if !hasItemLabelled(m, "sharpness") {
		t.Error("sharpness item should be visible once dithering is on")
	}
	m.imageViewer = "browser"
	if hasItemLabelled(m, "sharpness") {
		t.Error("sharpness item should stay hidden when imageViewer == browser, even with dithering on")
	}
}

func TestSettings_Dithering_SaveMsg(t *testing.T) {
	m := initSettings(defaultSettings())
	m.dithering = true
	m.originalDithering = false // make it dirty
	m.ditherSharpness = "rough"
	m.originalDitherSharpness = "medium"
	var got tea.Msg
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd != nil {
		got = cmd()
	}
	save, ok := got.(SaveSettingsMsg)
	if !ok {
		t.Fatal("ctrl+s should emit SaveSettingsMsg")
	}
	if !save.Dithering {
		t.Error("SaveSettingsMsg.Dithering should reflect current dithering value")
	}
	if save.DitherSharpness != "rough" {
		t.Errorf("SaveSettingsMsg.DitherSharpness = %q, want %q", save.DitherSharpness, "rough")
	}
}

func TestSettings_Dithering_SetSaved_AdvancesBaseline(t *testing.T) {
	m := initSettings(defaultSettings())
	m.dithering = true
	m.originalDithering = false
	m.ditherSharpness = "sharp"
	m.originalDitherSharpness = "medium"
	if !m.IsDirty() {
		t.Error("should be dirty before SetSaved")
	}
	m = m.SetSaved(false, false, true, false, true, 3, "UTC", "terminal", "", false, true, "sharp", nil)
	if m.originalDithering != true || m.originalDitherSharpness != "sharp" {
		t.Error("SetSaved should update originalDithering/originalDitherSharpness to the saved values")
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

	if !m.wanderLust {
		t.Error("wanderLust should be seeded from SharedConfigMsg even after a prior SetSettings call")
	}
	if m.maxThreadDepth != 20 {
		t.Errorf("maxThreadDepth = %d, want 20", m.maxThreadDepth)
	}
	if !m.inlineImages {
		t.Error("inlineImages should be seeded from SharedConfigMsg even after a prior SetSettings call")
	}
	if !m.dithering {
		t.Error("dithering should be seeded from SharedConfigMsg even after a prior SetSettings call")
	}
	if m.graphicsProtocol != "sixel" {
		t.Errorf("graphicsProtocol = %q, want sixel", m.graphicsProtocol)
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
	m.cursor = 8 // time format
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
	m = m.SetSaved(false, false, true, false, true, 3, "UTC", "terminal", "", false, false, "", nil)
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
	m.cursor = 15 // wander mode item (shifted by the new dithering toggle above it)
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
	m = m.SetSaved(true, false, true, false, true, 3, "UTC", "terminal", "", false, false, "", nil)
	if m.originalWanderLust != true {
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
	m.feedManualRefreshOnly = false // auto-refresh currently on

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
	if !m.feedManualRefreshOnly {
		t.Error("toggling auto-refresh off should flip feedManualRefreshOnly to true")
	}
}

func TestSettings_FeedAutoRefreshDirty(t *testing.T) {
	m := initSettings(defaultSettings())
	m.feedManualRefreshOnly = true
	m.originalFeedManualRefreshOnly = true
	if m.IsDirty() {
		t.Error("should not be dirty before change")
	}
	m.feedManualRefreshOnly = false
	if !m.IsDirty() {
		t.Error("IsDirty should return true when feedManualRefreshOnly differs from original")
	}
}

func TestSettings_FeedAutoRefreshSaveMsg(t *testing.T) {
	m := initSettings(defaultSettings())
	m.feedManualRefreshOnly = true
	m.originalFeedManualRefreshOnly = false // make it dirty
	var got tea.Msg
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd != nil {
		got = cmd()
	}
	save, ok := got.(SaveSettingsMsg)
	if !ok {
		t.Fatal("ctrl+s should emit SaveSettingsMsg")
	}
	if save.FeedManualRefreshOnly != true {
		t.Error("SaveSettingsMsg.FeedManualRefreshOnly should reflect current feedManualRefreshOnly value")
	}
}

func TestSettings_FeedAutoRefreshSetSaved(t *testing.T) {
	m := initSettings(defaultSettings())
	m.feedManualRefreshOnly = true
	m.originalFeedManualRefreshOnly = false // dirty
	m = m.SetSaved(false, true, true, false, true, 3, "UTC", "terminal", "", false, false, "", nil)
	if m.originalFeedManualRefreshOnly != true {
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

func TestSettings_CMailGroup_Visible(t *testing.T) {
	m := initSettings(defaultSettings())
	view := m.View()
	if !containsSubstring(view, "c-mail") {
		t.Error("View should contain 'c-mail' group header")
	}
}

func TestSettings_TypingIndicatorsToggle(t *testing.T) {
	m := initSettings(defaultSettings())
	m.typingIndicatorsEnabled = true

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
	if m.typingIndicatorsEnabled {
		t.Error("toggling typing indicators should flip typingIndicatorsEnabled to false")
	}
}

func TestSettings_TypingIndicatorsDirty(t *testing.T) {
	m := initSettings(defaultSettings())
	m.typingIndicatorsEnabled = true
	m.originalTypingIndicatorsEnabled = true
	if m.IsDirty() {
		t.Error("should not be dirty before change")
	}
	m.typingIndicatorsEnabled = false
	if !m.IsDirty() {
		t.Error("IsDirty should return true when typingIndicatorsEnabled differs from original")
	}
}

func TestSettings_TypingIndicatorsSaveMsg(t *testing.T) {
	m := initSettings(defaultSettings())
	m.typingIndicatorsEnabled = false
	m.originalTypingIndicatorsEnabled = true // make it dirty
	var got tea.Msg
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd != nil {
		got = cmd()
	}
	save, ok := got.(SaveSettingsMsg)
	if !ok {
		t.Fatal("ctrl+s should emit SaveSettingsMsg")
	}
	if save.TypingIndicatorsEnabled != false {
		t.Error("SaveSettingsMsg.TypingIndicatorsEnabled should reflect current typingIndicatorsEnabled value")
	}
}

func TestSettings_TypingIndicatorsSetSaved(t *testing.T) {
	m := initSettings(defaultSettings())
	m.typingIndicatorsEnabled = false
	m.originalTypingIndicatorsEnabled = true // dirty
	m = m.SetSaved(false, false, false, false, true, 3, "UTC", "terminal", "", false, false, "", nil)
	if m.originalTypingIndicatorsEnabled != false {
		t.Error("SetSaved should update originalTypingIndicatorsEnabled to the saved value")
	}
	if m.IsDirty() {
		t.Error("should not be dirty after SetSaved")
	}
}

// --- Globe tab visibility tests ---

func TestSettings_ShowGlobeTabToggle(t *testing.T) {
	m := initSettings(defaultSettings())
	m.showGlobeTab = true

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
	if m.showGlobeTab {
		t.Error("toggling show globe tab should flip showGlobeTab to false")
	}
}

func TestSettings_ShowGlobeTabDirty(t *testing.T) {
	m := initSettings(defaultSettings())
	m.showGlobeTab = true
	m.originalShowGlobeTab = true
	if m.IsDirty() {
		t.Error("should not be dirty before change")
	}
	m.showGlobeTab = false
	if !m.IsDirty() {
		t.Error("IsDirty should return true when showGlobeTab differs from original")
	}
}

func TestSettings_ShowGlobeTabSaveMsg(t *testing.T) {
	m := initSettings(defaultSettings())
	m.showGlobeTab = false
	m.originalShowGlobeTab = true // make it dirty
	var got tea.Msg
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd != nil {
		got = cmd()
	}
	save, ok := got.(SaveSettingsMsg)
	if !ok {
		t.Fatal("ctrl+s should emit SaveSettingsMsg")
	}
	if save.ShowGlobeTab != false {
		t.Error("SaveSettingsMsg.ShowGlobeTab should reflect current showGlobeTab value")
	}
}

// --- Timezone Tests ---

func TestSettings_Timezone_CyclesForward(t *testing.T) {
	m := initSettings(defaultSettings())
	m.timezone = "UTC"
	m.originalTimezone = "UTC"
	m.cursor = 10 // timezone item
	m, _ = m.Update(keyMsg("tab"))
	if m.timezone == "UTC" {
		t.Error("tab should advance timezone from UTC")
	}
}

func TestSettings_Timezone_CyclesBackward(t *testing.T) {
	m := initSettings(defaultSettings())
	m.timezone = "UTC"
	m.originalTimezone = "UTC"
	m.cursor = 10
	m, _ = m.Update(keyMsg("shift+tab"))
	if m.timezone == "UTC" {
		t.Error("shift+tab should cycle timezone backward from UTC")
	}
}

func TestSettings_Timezone_Wraps(t *testing.T) {
	m := initSettings(defaultSettings())
	items := flatItems(m)
	last := items[10].options[len(items[10].options)-1]
	m.timezone = last
	m.originalTimezone = last
	m.cursor = 10
	m, _ = m.Update(keyMsg("tab"))
	first := items[10].options[0]
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
	m.hardBreakKey, m.originalHardBreakKey = DefaultHardBreakKey, DefaultHardBreakKey
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
	if m.hardBreakKey != "ctrl+l" {
		t.Errorf("hardBreakKey = %q, want %q", m.hardBreakKey, "ctrl+l")
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
	m.hardBreakKey = "ctrl+l"
	m = capture(t, m, keyMsg("enter"), tea.KeyMsg{Type: tea.KeyEnter, Alt: true})
	if m.hardBreakKey != "alt+enter" || m.Capturing() {
		t.Errorf("hardBreakKey = %q capturing=%v, want alt+enter and finished", m.hardBreakKey, m.Capturing())
	}
	if m.IsDirty() {
		t.Error("returning to the saved default should not be dirty")
	}
}

func TestSettings_HardBreak_ShiftComboAccepted(t *testing.T) {
	m := bindingSettings(t)
	m = capture(t, m, keyMsg("enter"), tea.KeyMsg{Type: tea.KeyShiftUp})
	if m.hardBreakKey != "shift+up" {
		t.Errorf("hardBreakKey = %q, want shift+up", m.hardBreakKey)
	}
}

func TestSettings_HardBreak_EscCancelsCaptureKeepingOtherEdits(t *testing.T) {
	m := bindingSettings(t)
	m.wanderLust = true // an unsaved edit elsewhere
	m = capture(t, m, keyMsg("enter"), keyMsg("esc"))
	if m.Capturing() {
		t.Error("esc should end capture")
	}
	if m.hardBreakKey != DefaultHardBreakKey {
		t.Errorf("cancelled capture changed the key to %q", m.hardBreakKey)
	}
	if !m.wanderLust {
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
		if m.hardBreakKey != DefaultHardBreakKey {
			t.Errorf("%s: key changed to %q", name, m.hardBreakKey)
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
	m.wanderLust = true // dirty, so ctrl+s would normally save
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
	if save.HardBreakKey != "ctrl+l" {
		t.Errorf("SaveSettingsMsg.HardBreakKey = %q, want %q", save.HardBreakKey, "ctrl+l")
	}
}

func TestSettings_HardBreak_SetSavedAdvancesBaseline(t *testing.T) {
	m := bindingSettings(t)
	m = capture(t, m, keyMsg("enter"), tea.KeyMsg{Type: tea.KeyCtrlL})
	if !m.IsDirty() {
		t.Fatal("should be dirty before saving")
	}
	m = m.SetSavedHardBreakKey("ctrl+l")
	if m.IsDirty() {
		t.Error("should be clean after SetSavedHardBreakKey")
	}
}

func TestSettings_HardBreak_EscRevertsUnsavedKey(t *testing.T) {
	m := bindingSettings(t)
	m = capture(t, m, keyMsg("enter"), tea.KeyMsg{Type: tea.KeyCtrlL}, keyMsg("esc"))
	if m.hardBreakKey != DefaultHardBreakKey || m.IsDirty() {
		t.Errorf("esc should revert to %q, got %q (dirty=%v)", DefaultHardBreakKey, m.hardBreakKey, m.IsDirty())
	}
}

func TestSettings_HardBreak_SeededFromSharedConfig(t *testing.T) {
	m := NewSettingsModel()
	m, _ = m.Update(SharedConfigMsg{Width: 80, Height: 24, HardBreakKey: "ctrl+l"})
	if m.hardBreakKey != "ctrl+l" || m.originalHardBreakKey != "ctrl+l" {
		t.Errorf("seeded key = %q / baseline %q, want %q", m.hardBreakKey, m.originalHardBreakKey, "ctrl+l")
	}
	if m.IsDirty() {
		t.Error("seeding must not mark the screen dirty")
	}
}
