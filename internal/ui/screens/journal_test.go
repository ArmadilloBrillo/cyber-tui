package screens

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func jkeyRunes(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

// openDraftNote opens a new note and types body into it.
func openDraftNote(t *testing.T, body string) JournalModel {
	t.Helper()
	m := NewJournalModel(80)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = m.Update(jkeyRunes("n")) // openNewNote
	if !m.editMode {
		t.Fatal("setup: expected editMode true after 'n'")
	}
	m, _ = m.Update(jkeyRunes(body))
	if m.compose.Content() != body {
		t.Fatalf("setup: compose content = %q, want %q", m.compose.Content(), body)
	}
	return m
}

// TestJournal_ConfirmPublish_KeepsEditorOpenUntilOutcome: Ctrl+P → y no longer
// closes the editor synchronously; it stays open and populated (marked
// publishing) so a failed publish leaves the text recoverable.
func TestJournal_ConfirmPublish_KeepsEditorOpenUntilOutcome(t *testing.T) {
	m := openDraftNote(t, "a fragile draft")

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	if m.confirming != confirmPublish {
		t.Fatalf("Ctrl+P: confirming = %v, want confirmPublish", m.confirming)
	}

	m, cmd := m.Update(jkeyRunes("y"))
	if !m.editMode || !m.publishing {
		t.Fatalf("after confirm: editMode=%v publishing=%v, want both true", m.editMode, m.publishing)
	}
	if m.compose.Content() != "a fragile draft" {
		t.Errorf("compose content lost: %q", m.compose.Content())
	}
	if cmd == nil {
		t.Fatal("expected a SubmitPublishNoteMsg cmd")
	}
	if got, ok := cmd().(SubmitPublishNoteMsg); !ok || got.Content != "a fragile draft" {
		t.Fatalf("publish cmd produced %#v, want SubmitPublishNoteMsg{Content:\"a fragile draft\"}", cmd())
	}

	// Failure recovery: editor stays, publishing cleared.
	recovered := m.ClearPublishing()
	if !recovered.editMode || recovered.publishing || recovered.compose.Content() != "a fragile draft" {
		t.Errorf("ClearPublishing: editMode=%v publishing=%v content=%q", recovered.editMode, recovered.publishing, recovered.compose.Content())
	}

	// Success: editor closes.
	if done := m.CloseEditAfterPublish(); done.editMode {
		t.Error("expected editMode false after CloseEditAfterPublish")
	}
}

// TestJournal_WhilePublishing_SubmitKeysInert: no double-publish, no discard,
// while the publish is in flight.
func TestJournal_WhilePublishing_SubmitKeysInert(t *testing.T) {
	m := openDraftNote(t, "in flight")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m, _ = m.Update(jkeyRunes("y"))
	if !m.publishing {
		t.Fatal("setup: expected publishing true")
	}

	for _, k := range []tea.KeyMsg{
		{Type: tea.KeyCtrlP},
		{Type: tea.KeyCtrlS},
		{Type: tea.KeyEsc},
	} {
		var cmd tea.Cmd
		m, cmd = m.Update(k)
		if cmd != nil {
			t.Errorf("key %v produced a cmd while publishing, want none", k)
		}
		if !m.editMode {
			t.Fatalf("key %v closed the editor while publishing", k)
		}
	}
}
