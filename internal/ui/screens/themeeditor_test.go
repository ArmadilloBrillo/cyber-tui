package screens

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
)

func testPalette() theme.Palette {
	return theme.Palette{
		Foreground: "#111111", Border: "#222222", Accent: "#333333", Highlight: "#444444",
		Error: "#555555", BarText: "#666666", Dimmed: "#777777", Self: "#888888", Meta: "#999999",
	}
}

func runeMsg(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestThemeEditorModel_DirtyTracking(t *testing.T) {
	m := NewThemeEditorModel(testPalette())
	if m.IsDirty() {
		t.Error("expected fresh editor to not be dirty")
	}

	m, _ = m.Update(keyMsg("enter")) // focus row 0
	m, _ = m.Update(runeMsg("9"))
	if !m.IsDirty() {
		t.Error("expected editor to be dirty after editing a field")
	}

	m = m.SetSaved(m.currentPalette())
	if m.IsDirty() {
		t.Error("expected editor to not be dirty after SetSaved")
	}
}

func TestThemeEditorModel_TypingOverwritesAndAdvances(t *testing.T) {
	m := NewThemeEditorModel(testPalette()) // row 0 buffer starts "111111"
	m, _ = m.Update(keyMsg("enter"))
	if m.charCursor != 0 {
		t.Fatalf("charCursor = %d, want 0 on entering edit mode", m.charCursor)
	}

	m, _ = m.Update(runeMsg("a"))
	if m.values[0] != "A11111" {
		t.Errorf("values[0] = %q, want %q", m.values[0], "A11111")
	}
	if m.charCursor != 1 {
		t.Errorf("charCursor = %d, want 1 after typing", m.charCursor)
	}

	m, _ = m.Update(runeMsg("b"))
	if m.values[0] != "AB1111" {
		t.Errorf("values[0] = %q, want %q", m.values[0], "AB1111")
	}
	if m.charCursor != 2 {
		t.Errorf("charCursor = %d, want 2 after typing", m.charCursor)
	}
}

func TestThemeEditorModel_UppercasesLetters(t *testing.T) {
	m := NewThemeEditorModel(testPalette())
	m, _ = m.Update(keyMsg("enter"))
	m, _ = m.Update(runeMsg("f"))
	if got := m.values[0][:1]; got != "F" {
		t.Errorf("typed 'f' stored as %q, want uppercase %q", got, "F")
	}
}

func TestThemeEditorModel_LastSlotDoesNotAdvancePastEnd(t *testing.T) {
	m := NewThemeEditorModel(testPalette())
	m, _ = m.Update(keyMsg("enter"))
	for range hexDigits - 1 {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	}
	if m.charCursor != hexDigits-1 {
		t.Fatalf("charCursor = %d, want %d (last slot)", m.charCursor, hexDigits-1)
	}

	m, _ = m.Update(runeMsg("9"))
	if m.charCursor != hexDigits-1 {
		t.Errorf("charCursor = %d after typing on last slot, want it to stay at %d", m.charCursor, hexDigits-1)
	}
	if m.values[0][hexDigits-1] != '9' {
		t.Errorf("last slot = %q, want overwritten with '9'", m.values[0][hexDigits-1:])
	}

	// One more right press beyond the last slot must not move the cursor further.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.charCursor != hexDigits-1 {
		t.Errorf("charCursor = %d after right at last slot, want clamped at %d", m.charCursor, hexDigits-1)
	}
}

func TestThemeEditorModel_CursorClampedAtStart(t *testing.T) {
	m := NewThemeEditorModel(testPalette())
	m, _ = m.Update(keyMsg("enter"))
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if m.charCursor != 0 {
		t.Errorf("charCursor = %d after left at start, want clamped at 0", m.charCursor)
	}
}

func TestThemeEditorModel_Backspace_ClearsAndStepsBack(t *testing.T) {
	m := NewThemeEditorModel(testPalette())
	m, _ = m.Update(keyMsg("enter"))
	m, _ = m.Update(runeMsg("a")) // charCursor now 1, slot 0 = 'A'
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if m.charCursor != 0 {
		t.Errorf("charCursor = %d after backspace, want 0", m.charCursor)
	}
	if m.values[0][0] != ' ' {
		t.Errorf("values[0][0] = %q, want cleared to space", string(m.values[0][0]))
	}
}

func TestThemeEditorModel_CtrlS_EmitsSaveThemeMsg_WhenValidAndDirty(t *testing.T) {
	m := NewThemeEditorModel(testPalette())
	m, _ = m.Update(keyMsg("enter"))
	m, _ = m.Update(runeMsg("9"))
	m, _ = m.Update(keyMsg("enter")) // commit field, back to nav mode

	_, cmd := m.Update(keyMsg("ctrl+s"))
	if cmd == nil {
		t.Fatal("expected a cmd from ctrl+s")
	}
	msg := cmd()
	if _, ok := msg.(SaveThemeMsg); !ok {
		t.Errorf("expected SaveThemeMsg, got %T", msg)
	}
}

func TestThemeEditorModel_CtrlS_NoOp_WhenInvalid(t *testing.T) {
	m := NewThemeEditorModel(testPalette())
	m, _ = m.Update(keyMsg("enter"))
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace}) // clears slot 0, leaving a blank — malformed hex
	m, _ = m.Update(keyMsg("enter"))                    // commit field, back to nav mode

	m, cmd := m.Update(keyMsg("ctrl+s"))
	if cmd != nil {
		t.Error("expected no cmd from ctrl+s with invalid input")
	}
	if m.err == "" {
		t.Error("expected err to be set")
	}
}

func TestThemeEditorModel_CtrlS_WorksWhileMidFieldEdit(t *testing.T) {
	m := NewThemeEditorModel(testPalette())
	m, _ = m.Update(keyMsg("enter")) // focus row 0, still editing — no commit
	m, _ = m.Update(runeMsg("9"))
	if !m.editing {
		t.Fatal("expected to still be mid-edit before ctrl+s")
	}

	m, cmd := m.Update(keyMsg("ctrl+s"))
	if cmd == nil {
		t.Fatal("expected ctrl+s to save even while a field is focused")
	}
	if _, ok := cmd().(SaveThemeMsg); !ok {
		t.Errorf("expected SaveThemeMsg, got %T", cmd())
	}
	if m.editing {
		t.Error("expected ctrl+s to close the focused field")
	}
}

func TestThemeEditorModel_CtrlS_SavesUnchangedPrefill_NotDirty(t *testing.T) {
	// Mirrors opening the editor via T (post-theme preview) or file import:
	// the prefill IS the palette to keep, so ctrl+s must work with zero edits.
	p := testPalette()
	m := NewThemeEditorModel(p)
	if m.IsDirty() {
		t.Fatal("expected a fresh editor to not be dirty")
	}

	_, cmd := m.Update(keyMsg("ctrl+s"))
	if cmd == nil {
		t.Fatal("expected ctrl+s to save an unmodified (but valid) prefill")
	}
	msg, ok := cmd().(SaveThemeMsg)
	if !ok {
		t.Fatalf("expected SaveThemeMsg, got %T", cmd())
	}
	if msg.Palette != p {
		t.Errorf("saved palette = %+v, want the unmodified prefill %+v", msg.Palette, p)
	}
}

func TestThemeEditorModel_CtrlS_MidFieldEdit_InvalidStillBlocks(t *testing.T) {
	m := NewThemeEditorModel(testPalette())
	m, _ = m.Update(keyMsg("enter"))
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace}) // clears slot 0 — malformed hex, still mid-edit

	m, cmd := m.Update(keyMsg("ctrl+s"))
	if cmd != nil {
		t.Error("expected no cmd from ctrl+s with invalid input, even mid-edit")
	}
	if m.err == "" {
		t.Error("expected err to be set")
	}
}

func TestThemeEditorModel_Esc_EmitsCloseThemeEditorMsg(t *testing.T) {
	m := NewThemeEditorModel(testPalette())
	_, cmd := m.Update(keyMsg("esc"))
	if cmd == nil {
		t.Fatal("expected a cmd from esc")
	}
	if _, ok := cmd().(CloseThemeEditorMsg); !ok {
		t.Errorf("expected CloseThemeEditorMsg, got %T", cmd())
	}
}

func TestThemeEditorModel_Editing_IgnoresNonAlnumRunes(t *testing.T) {
	m := NewThemeEditorModel(testPalette())
	m, _ = m.Update(keyMsg("enter"))
	m, _ = m.Update(runeMsg("!"))
	if m.values[0] != "111111" {
		t.Errorf("values[0] = %q, want unchanged %q after non-alnum keypress", m.values[0], "111111")
	}
	if m.charCursor != 0 {
		t.Errorf("charCursor = %d, want 0 (non-alnum keypress should not advance)", m.charCursor)
	}
}
