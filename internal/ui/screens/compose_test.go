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

// TestPostComposePanel_OpenForEdit_PrefillsImageAttachment guards the
// prefill PostDetail/Feed rely on to avoid clobbering an existing attachment
// on save: OpenForEdit must seed attachmentURL from the post's own
// image/gif attachment (skipping any audio one), but leave it untouched so
// a save that never calls SetAttachmentURL omits the attachments field
// entirely (see EditPost's attachmentTouched). The audio attachment must
// still be preserved (via OtherAttachments) rather than discarded, since a
// touched save resends the whole attachments array and would otherwise
// silently drop it.
func TestPostComposePanel_OpenForEdit_PrefillsImageAttachment(t *testing.T) {
	m := NewPostComposePanel(80)
	audio := model.Attachment{Type: "audio", Src: "https://example.com/track.mp3"}
	m, _ = m.OpenForEdit(model.Post{
		Attachments: []model.Attachment{
			audio,
			{Type: "image", Src: "https://example.com/pic.png"},
		},
	})
	if got := m.AttachmentURL(); got != "https://example.com/pic.png" {
		t.Errorf("AttachmentURL() = %q, want the image attachment's src (audio skipped)", got)
	}
	if got := m.OtherAttachments(); len(got) != 1 || got[0] != audio {
		t.Errorf("OtherAttachments() = %v, want [%v] (the audio attachment, preserved)", got, audio)
	}
	if m.AttachmentTouched() {
		t.Error("AttachmentTouched() = true after OpenForEdit, want false — prefilling isn't a user edit")
	}
}

// TestPostComposePanel_SetAttachmentURL_MarksTouchedEvenWhenClearing checks
// the touched flag flips on an explicit clear (empty string), not just a
// set — that's what tells EditPost to actually send `attachments: []`
// instead of silently leaving a stale attachment in place.
func TestPostComposePanel_SetAttachmentURL_MarksTouchedEvenWhenClearing(t *testing.T) {
	m := NewPostComposePanel(80)
	m, _ = m.OpenForEdit(model.Post{
		Attachments: []model.Attachment{{Type: "image", Src: "https://example.com/pic.png"}},
	})
	m = m.SetAttachmentURL("")
	if got := m.AttachmentURL(); got != "" {
		t.Errorf("AttachmentURL() = %q, want empty after clearing", got)
	}
	if !m.AttachmentTouched() {
		t.Error("AttachmentTouched() = false after an explicit clear, want true")
	}
}

// TestPostComposePanel_Open_ResetsAttachment guards against a stale
// attachment (or touched flag) from a previous edit session leaking into the
// next new-post compose.
func TestPostComposePanel_Open_ResetsAttachment(t *testing.T) {
	m := NewPostComposePanel(80)
	m, _ = m.OpenForEdit(model.Post{
		Attachments: []model.Attachment{{Type: "image", Src: "https://example.com/pic.png"}},
	})
	m = m.SetAttachmentURL("https://example.com/other.png")
	m, _ = m.Open(false)
	if got := m.AttachmentURL(); got != "" {
		t.Errorf("AttachmentURL() = %q after Open(), want empty", got)
	}
	if m.AttachmentTouched() {
		t.Error("AttachmentTouched() = true after Open(), want false")
	}
}
