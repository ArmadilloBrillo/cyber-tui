package screens

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestLoginModel_EmailNotVerified_ShowsResendHint confirms an
// EMAIL_NOT_VERIFIED LoginErrMsg switches the status line to the
// verification message with a resend hint, instead of the generic error.
func TestLoginModel_EmailNotVerified_ShowsResendHint(t *testing.T) {
	m := NewLoginModel("")
	m, _ = m.Update(LoginErrMsg{Err: errors.New("boom"), EmailNotVerified: true, IDToken: "id-abc"})

	if !m.emailNotVerified {
		t.Fatal("expected emailNotVerified = true")
	}
	if m.idToken != "id-abc" {
		t.Errorf("idToken = %q, want id-abc", m.idToken)
	}
	view := m.View()
	if !strings.Contains(view, "isn't verified yet") {
		t.Errorf("expected the verification message in the view, got: %q", view)
	}
	if !strings.Contains(view, "press r to resend") {
		t.Errorf("expected the resend hint in the view, got: %q", view)
	}
}

// TestLoginModel_ROnlyTriggersResendWhenEmailNotVerified confirms the 'r'
// keybinding is scoped to the EMAIL_NOT_VERIFIED state — normally it's just
// a character typed into the focused input.
func TestLoginModel_ROnlyTriggersResendWhenEmailNotVerified(t *testing.T) {
	m := NewLoginModel("")
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if cmd != nil {
		msg := cmd()
		if _, ok := msg.(ResendVerificationMsg); ok {
			t.Fatal("expected 'r' to type into the input, not trigger resend, when not in the EMAIL_NOT_VERIFIED state")
		}
	}
	if m.inputs[0].Value() != "r" {
		t.Errorf("expected 'r' typed into the focused (email) input, got %q", m.inputs[0].Value())
	}
}

// TestLoginModel_RTriggersResendWhenEmailNotVerified confirms 'r' fires
// ResendVerificationMsg with the stored idToken once EMAIL_NOT_VERIFIED is
// set, and enters the "resending" state.
func TestLoginModel_RTriggersResendWhenEmailNotVerified(t *testing.T) {
	m := NewLoginModel("")
	m, _ = m.Update(LoginErrMsg{Err: errors.New("boom"), EmailNotVerified: true, IDToken: "id-abc"})

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if !m.resending {
		t.Error("expected resending = true")
	}
	if cmd == nil {
		t.Fatal("expected a command emitting ResendVerificationMsg")
	}
	msg, ok := cmd().(ResendVerificationMsg)
	if !ok {
		t.Fatalf("expected ResendVerificationMsg, got %T", msg)
	}
	if msg.IDToken != "id-abc" {
		t.Errorf("IDToken = %q, want id-abc", msg.IDToken)
	}
}

// TestLoginModel_ResendResult_UpdatesStatus confirms the result of a resend
// attempt (success or error) clears the "resending" flag and updates the
// status text shown to the user.
func TestLoginModel_ResendResult_UpdatesStatus(t *testing.T) {
	m := NewLoginModel("")
	m, _ = m.Update(LoginErrMsg{Err: errors.New("boom"), EmailNotVerified: true, IDToken: "id-abc"})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})

	m, _ = m.Update(ResendVerificationResultMsg{Err: nil})
	if m.resending {
		t.Error("expected resending = false after a result arrives")
	}
	if !strings.Contains(m.View(), "sent") {
		t.Errorf("expected a sent confirmation in the view, got: %q", m.View())
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	m, _ = m.Update(ResendVerificationResultMsg{Err: errors.New("rate limited")})
	if !strings.Contains(m.View(), "error: rate limited") {
		t.Errorf("expected the resend error in the view, got: %q", m.View())
	}
}

// TestLoginModel_Resubmit_ClearsEmailNotVerified confirms starting a fresh
// login attempt (e.g. after fixing credentials) resets the verification
// state rather than leaving a stale resend hint showing.
func TestLoginModel_Resubmit_ClearsEmailNotVerified(t *testing.T) {
	m := NewLoginModel("")
	m, _ = m.Update(LoginErrMsg{Err: errors.New("boom"), EmailNotVerified: true, IDToken: "id-abc"})
	m.focused = 1 // password field, so "enter" submits rather than advancing focus

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.emailNotVerified {
		t.Error("expected emailNotVerified to clear on a fresh submit")
	}
}
