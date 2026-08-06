package screens

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestPathPromptModel_Open_PrefillsAndFocuses(t *testing.T) {
	m := NewPathPromptModel()
	m, _ = m.Open("export theme to", "~/theme.json")
	if m.input.Value() != "~/theme.json" {
		t.Errorf("input value = %q, want %q", m.input.Value(), "~/theme.json")
	}
	if !m.input.Focused() {
		t.Error("expected input to be focused after Open")
	}
	if m.title != "export theme to" {
		t.Errorf("title = %q, want %q", m.title, "export theme to")
	}
}

func TestPathPromptModel_Enter_EmitsSubmitWithCurrentValue(t *testing.T) {
	m := NewPathPromptModel()
	m, _ = m.Open("import theme from", "~/theme.json")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace}) // "~/theme.jso"
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})

	_, cmd := m.Update(keyMsg("enter"))
	if cmd == nil {
		t.Fatal("expected a cmd from enter")
	}
	msg, ok := cmd().(PathPromptSubmitMsg)
	if !ok {
		t.Fatalf("expected PathPromptSubmitMsg, got %T", cmd())
	}
	if msg.Path != "~/theme.json" {
		t.Errorf("Path = %q, want %q", msg.Path, "~/theme.json")
	}
}

func TestPathPromptModel_Esc_EmitsCancel(t *testing.T) {
	m := NewPathPromptModel()
	m, _ = m.Open("export theme to", "~/theme.json")

	_, cmd := m.Update(keyMsg("esc"))
	if cmd == nil {
		t.Fatal("expected a cmd from esc")
	}
	if _, ok := cmd().(PathPromptCancelMsg); !ok {
		t.Errorf("expected PathPromptCancelMsg, got %T", cmd())
	}
}

func TestPathPromptModel_Keystroke_ClearsWarning(t *testing.T) {
	m := NewPathPromptModel()
	m, _ = m.Open("export theme to", "~/theme.json")
	m = m.SetWarning("file exists — enter again to overwrite")

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if m.warning != "" {
		t.Errorf("expected warning to clear on keystroke, got %q", m.warning)
	}
}
