package screens

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func typeText(m KeywordEditorModel, s string) KeywordEditorModel {
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)})
	return m
}

func TestKeywordEditor_Open_SeedsCopyAndResetsState(t *testing.T) {
	src := []string{"foo", "bar"}
	m := NewKeywordEditorModel().Open(src)
	if len(m.Keywords()) != 2 || m.Keywords()[0] != "foo" || m.Keywords()[1] != "bar" {
		t.Errorf("Keywords() = %v, want [foo bar]", m.Keywords())
	}
	if m.cursor != 0 {
		t.Errorf("cursor = %d, want 0", m.cursor)
	}
	// Mutating the returned slice must not affect src (defensive copy).
	m.keywords[0] = "changed"
	if src[0] != "foo" {
		t.Error("Open must copy the input slice, not alias it")
	}
}

func TestKeywordEditor_AddViaEnterOnAddRow(t *testing.T) {
	m := NewKeywordEditorModel().Open(nil)
	m = typeText(m, "cyberdeck")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if len(m.Keywords()) != 1 || m.Keywords()[0] != "cyberdeck" {
		t.Errorf("Keywords() = %v, want [cyberdeck]", m.Keywords())
	}
	if m.input.Value() != "" {
		t.Errorf("input should be cleared after adding, got %q", m.input.Value())
	}
}

func TestKeywordEditor_AddLowercasesInput(t *testing.T) {
	m := NewKeywordEditorModel().Open(nil)
	m = typeText(m, "CyberDeck")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if len(m.Keywords()) != 1 || m.Keywords()[0] != "cyberdeck" {
		t.Errorf("Keywords() = %v, want [cyberdeck] (lowercased)", m.Keywords())
	}
}

func TestKeywordEditor_AddIgnoresCaseInsensitiveDuplicate(t *testing.T) {
	m := NewKeywordEditorModel().Open([]string{"cyberdeck"})
	m = typeText(m, "CYBERDECK")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if len(m.Keywords()) != 1 {
		t.Errorf("expected duplicate to be ignored, got %v", m.Keywords())
	}
}

func TestKeywordEditor_AddIgnoresEmpty(t *testing.T) {
	m := NewKeywordEditorModel().Open(nil)
	m = typeText(m, "   ")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if len(m.Keywords()) != 0 {
		t.Errorf("expected blank input to add nothing, got %v", m.Keywords())
	}
}

func TestKeywordEditor_DeleteHighlightedKeyword(t *testing.T) {
	m := NewKeywordEditorModel().Open([]string{"foo", "bar", "baz"})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // cursor -> foo (index 0)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if len(m.Keywords()) != 2 || m.Keywords()[0] != "bar" || m.Keywords()[1] != "baz" {
		t.Errorf("Keywords() = %v, want [bar baz]", m.Keywords())
	}
}

func TestKeywordEditor_DeleteNoopOnAddRow(t *testing.T) {
	m := NewKeywordEditorModel().Open([]string{"foo"})
	// cursor stays at 0 (add-row); "d" must not delete anything.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if len(m.Keywords()) != 1 {
		t.Errorf("expected no deletion while on the add-row, got %v", m.Keywords())
	}
}

func TestKeywordEditor_LettersDAndXTypeIntoAddRow(t *testing.T) {
	m := NewKeywordEditorModel().Open(nil)
	m = typeText(m, "d")
	m = typeText(m, "x")
	if m.input.Value() != "dx" {
		t.Errorf("input.Value() = %q, want %q (d/x must type, not delete, on the add-row)", m.input.Value(), "dx")
	}
}

func TestKeywordEditor_LettersJAndKTypeIntoAddRow(t *testing.T) {
	m := NewKeywordEditorModel().Open([]string{"a"})
	m = typeText(m, "jk")
	if m.input.Value() != "jk" {
		t.Errorf("input.Value() = %q, want %q (j/k must type on the add-row)", m.input.Value(), "jk")
	}
	if m.cursor != 0 {
		t.Errorf("cursor = %d, want 0 (j must not navigate on the add-row)", m.cursor)
	}
}

func TestKeywordEditor_JAndKNavigateOnKeywordList(t *testing.T) {
	m := NewKeywordEditorModel().Open([]string{"a", "b"})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.cursor != 2 {
		t.Errorf("cursor = %d, want 2 after j", m.cursor)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if m.cursor != 1 {
		t.Errorf("cursor = %d, want 1 after k", m.cursor)
	}
}

func TestKeywordEditor_UpDownNavigatesAcrossAddRowAndKeywords(t *testing.T) {
	m := NewKeywordEditorModel().Open([]string{"a", "b"})
	if m.cursor != 0 {
		t.Fatalf("setup: expected cursor 0, got %d", m.cursor)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 1 {
		t.Errorf("cursor = %d, want 1", m.cursor)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 2 {
		t.Errorf("cursor = %d, want 2", m.cursor)
	}
	// Can't go past the last keyword.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 2 {
		t.Errorf("cursor = %d, want 2 (clamped)", m.cursor)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.cursor != 0 {
		t.Errorf("cursor = %d, want 0", m.cursor)
	}
	// Can't go above the add-row.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.cursor != 0 {
		t.Errorf("cursor = %d, want 0 (clamped)", m.cursor)
	}
}

func TestKeywordEditor_ScrollOffsetTracksCursorPastVisibleWindow(t *testing.T) {
	keywords := []string{"k0", "k1", "k2", "k3", "k4", "k5", "k6", "k7"}
	m := NewKeywordEditorModel().Open(keywords)
	for i := 0; i < len(keywords); i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	if m.cursor != len(keywords) {
		t.Fatalf("cursor = %d, want %d", m.cursor, len(keywords))
	}
	// The last keyword (index len-1) must be within [offset, offset+6).
	lastIdx := len(keywords) - 1
	if lastIdx < m.offset || lastIdx >= m.offset+keywordEditorVisibleRows {
		t.Errorf("offset = %d, last keyword index %d not within the visible window", m.offset, lastIdx)
	}
}

func TestKeywordEditor_ViewShowsScrollIndicatorsOnlyWhenScrollable(t *testing.T) {
	short := NewKeywordEditorModel().Open([]string{"a", "b"}).View()
	if strings.Contains(short, "more") {
		t.Errorf("short list should show no scroll indicator:\n%s", short)
	}

	kws := []string{"a", "b", "c", "d", "e", "f", "g", "h"}
	m := NewKeywordEditorModel().Open(kws)
	top := m.View()
	if !strings.Contains(top, "▼ 2 more") || strings.Contains(top, "▲") {
		t.Errorf("at top want only '▼ 2 more':\n%s", top)
	}
	for range kws {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	bottom := m.View()
	if !strings.Contains(bottom, "▲ 2 more") || strings.Contains(bottom, "▼") {
		t.Errorf("at bottom want only '▲ 2 more':\n%s", bottom)
	}
	if lipgloss.Height(top) != lipgloss.Height(bottom) {
		t.Errorf("box height changed while scrolling: %d vs %d", lipgloss.Height(top), lipgloss.Height(bottom))
	}
}
