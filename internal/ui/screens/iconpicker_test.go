package screens

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestIconPickerModel_Update_UnrecognizedKey_DoesNotResetSelection guards a
// bug: refiltered() unconditionally resets selection/scroll to the top, so
// forwarding every key the switch doesn't recognize straight into
// refiltered() — even one the query textinput itself ignores and that
// leaves the query text unchanged — silently yanked the selection back to
// row 0. Seen in practice: a terminal that doesn't encode ctrl+n the way
// this app expects fell through to this path on every ctrl+n press.
func TestIconPickerModel_Update_UnrecognizedKey_DoesNotResetSelection(t *testing.T) {
	m := NewIconPickerModel()
	m = m.moveSelection(2)
	selected := m.selected
	if selected == 0 {
		t.Fatal("setup: expected moveSelection(2) to move off row 0")
	}

	// ctrl+w is bound to word-delete in textinput's default keymap, but on
	// an empty query it's a no-op — value before and after are both "".
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})

	if m.selected != selected {
		t.Errorf("selected = %d after a query-unchanged key, want unchanged %d", m.selected, selected)
	}
}
