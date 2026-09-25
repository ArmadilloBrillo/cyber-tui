package screens

import (
	"testing"

	"github.com/charmbracelet/bubbletea"
)

func TestValidateHardBreakKey_Valid(t *testing.T) {
	for _, k := range []string{"alt+enter", "ctrl+l", "shift+up", "alt+x", "ctrl+enter"} {
		if err := ValidateHardBreakKey(k); err != nil {
			t.Errorf("ValidateHardBreakKey(%q) unexpected error: %v", k, err)
		}
	}
}

func TestValidateHardBreakKey_Invalid(t *testing.T) {
	for _, k := range []string{
		"", " ", "enter", "tab", "x", "c", "up",
		"shift+tab", "esc", "ctrl+c", "ctrl+s", "ctrl+j", "ctrl+b", "alt+d",
	} {
		if err := ValidateHardBreakKey(k); err == nil {
			t.Errorf("ValidateHardBreakKey(%q) = nil, want error", k)
		}
	}
}

func TestResolveHardBreakKey(t *testing.T) {
	for in, want := range map[string]string{
		"":             DefaultHardBreakKey,
		"ctrl+l":       "ctrl+l",
		"ctrl+s":       DefaultHardBreakKey,
		"ctrl+l enter": DefaultHardBreakKey,
		"x":            DefaultHardBreakKey,
	} {
		if got := resolveHardBreakKey(in); got != want {
			t.Errorf("resolveHardBreakKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHardBreakKeyFromMsg(t *testing.T) {
	got, err := HardBreakKeyFromMsg(tea.KeyMsg{Type: tea.KeyEnter, Alt: true})
	if err != nil || got != "alt+enter" {
		t.Errorf("alt+enter: got %q, %v", got, err)
	}
	if _, err := HardBreakKeyFromMsg(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ab")}); err == nil {
		t.Error("multi-rune input must be rejected")
	}
	if _, err := HardBreakKeyFromMsg(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a"), Paste: true}); err == nil {
		t.Error("pasted input must be rejected")
	}
}

func pasteOf(s string) string {
	return string(convertPaste(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s), Paste: true}).Runes)
}

func TestConvertPaste(t *testing.T) {
	for name, tc := range map[string]struct{ in, want string }{
		"single newline":       {"a\nb", "a  \nb"},
		"blank line kept":      {"a\n\nb", "a\n\nb"},
		"blank run collapses":  {"a\n\n\n\nb", "a\n\nb"},
		"crlf":                 {"a\r\nb\r\n\r\nc", "a  \nb\n\nc"},
		"lone cr":              {"a\rb", "a  \nb"},
		"existing hard break":  {"a  \nb", "a  \nb"},
		"trailing newline":     {"a\nb\n", "a  \nb\n"},
		"no newline":           {"plain text", "plain text"},
		"whitespace only line": {"a\n  \nb", "a\n\nb"},
		"fence untouched":      {"x\n```\ncode  \nmore\n```\ny", "x\n```\ncode  \nmore\n```\ny"},
		"list":                 {"- a\n- b", "- a  \n- b"},
	} {
		if got := pasteOf(tc.in); got != tc.want {
			t.Errorf("%s: convertPaste(%q) = %q, want %q", name, tc.in, got, tc.want)
		}
	}
}

func TestConvertPaste_LeavesTypingAlone(t *testing.T) {
	in := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a\nb")}
	if got := convertPaste(in); string(got.Runes) != "a\nb" {
		t.Errorf("non-paste input changed to %q", string(got.Runes))
	}
}
