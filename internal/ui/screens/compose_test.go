package screens

import (
	"testing"

	"github.com/ragnar/cyber-tui/internal/model"
)

func TestPostComposePanel_SlugValue_LowercasesOnRead(t *testing.T) {
	m := NewPostComposePanel(80)
	m.slugInput.SetValue("MyPost-42")
	if got := m.SlugValue(); got != "mypost-42" {
		t.Errorf("SlugValue() = %q, want %q (typed case preserved in the field, lowercased on read)", got, "mypost-42")
	}
	if got := m.slugInput.Value(); got != "MyPost-42" {
		t.Errorf("slugInput.Value() = %q, want unchanged %q", got, "MyPost-42")
	}
}

// TestPostComposePanel_OpenForEdit_TextareaHeightMatchesBodyLines guards
// against a regression where OpenForEdit skipped the unconditional height
// reset Open() does, so recalcBodyHeight's short-circuit (when the computed
// line count happens to already equal bodyLines) could leave the textarea's
// actual rendered height desynced from what PanelHeight() reports — the
// panel would then render taller than the viewport reserved space for it,
// clipping the bottom of the body, the topics row, and the toggles.
func TestPostComposePanel_OpenForEdit_TextareaHeightMatchesBodyLines(t *testing.T) {
	m := NewPostComposePanel(80)
	m, _ = m.OpenForEdit(model.Post{Content: "one line"})
	if m.textarea.Height() != m.bodyLines {
		t.Errorf("textarea.Height() = %d, want %d (m.bodyLines) — panel will render taller than PanelHeight() reports", m.textarea.Height(), m.bodyLines)
	}
	if got, want := m.PanelHeight(), m.bodyLines+6; got != want {
		t.Errorf("PanelHeight() = %d, want %d", got, want)
	}
}
