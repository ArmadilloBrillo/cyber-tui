package screens_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ragnar/cyber-tui/internal/ui/screens"
)

func typeKey(m screens.FlagPrompt, s string) (screens.FlagPrompt, tea.Cmd) {
	return m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)})
}

func TestFlagPrompt_EnterThenYes_SubmitsTypedReason(t *testing.T) {
	m := screens.NewFlagPrompt()
	m, _ = m.Open(screens.FlagKindPost)
	if !m.Active() {
		t.Fatal("expected prompt to be active after Open")
	}

	for _, r := range "spam" {
		m, _ = typeKey(m, string(r))
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})

	if m.Active() {
		t.Error("expected prompt to close after confirming")
	}
	if cmd == nil {
		t.Fatal("expected a cmd on confirm")
	}
	msg, ok := cmd().(screens.FlagSubmitMsg)
	if !ok {
		t.Fatalf("expected FlagSubmitMsg, got %T", cmd())
	}
	if msg.Reason != "spam" {
		t.Errorf("Reason = %q, want %q", msg.Reason, "spam")
	}
}

func TestFlagPrompt_NoAtConfirm_ReturnsToEditing(t *testing.T) {
	m := screens.NewFlagPrompt()
	m, _ = m.Open(screens.FlagKindReply)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if !m.Active() {
		t.Error("expected prompt to remain active after 'n' at confirm step")
	}
	if cmd == nil {
		t.Fatal("expected a cmd to re-focus the reason input")
	}

	// Still editable: typing now should not be swallowed as a y/n answer.
	m, _ = typeKey(m, "x")
	if !m.Active() {
		t.Error("expected prompt to still be active while editing")
	}
}

func TestFlagPrompt_FlagKindMessage_ShowsMessageWording(t *testing.T) {
	m := screens.NewFlagPrompt()
	m, _ = m.Open(screens.FlagKindMessage)
	if !strings.Contains(m.View(80), "Report this message") {
		t.Errorf("View() = %q, want it to mention 'Report this message'", m.View(80))
	}
}

func TestFlagPrompt_EscAnyStage_CancelsAndCloses(t *testing.T) {
	m := screens.NewFlagPrompt()
	m, _ = m.Open(screens.FlagKindPost)
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if m.Active() {
		t.Error("expected prompt to close on esc")
	}
	if cmd == nil {
		t.Fatal("expected a cancel cmd on esc")
	}
	if _, ok := cmd().(screens.FlagCancelMsg); !ok {
		t.Fatalf("expected FlagCancelMsg, got %T", cmd())
	}
}
