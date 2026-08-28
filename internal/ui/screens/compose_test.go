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

// TestPostComposePanel_OpenForEdit_PrefillsAudioAttachment guards the
// prefill PostDetail/Feed rely on to avoid clobbering an existing attachment
// on save: OpenForEdit must seed pendingAudio from the post's audio
// attachment, but leave it untouched so a save that never calls
// SetPendingAudio omits the attachments field entirely (see EditPost's
// attachmentTouched). A legacy (pre-v0.8.7) image attachment falls through
// to OtherAttachments, since image attach is no longer a dedicated slot but
// still needs to round-trip on an edit that touches other attachments.
func TestPostComposePanel_OpenForEdit_PrefillsAudioAttachment(t *testing.T) {
	m := NewPostComposePanel(80)
	audio := model.Attachment{Type: "audio", Src: "https://example.com/track.mp3"}
	legacyImage := model.Attachment{Type: "image", Src: "https://example.com/pic.png"}
	m, _ = m.OpenForEdit(model.Post{
		Attachments: []model.Attachment{audio, legacyImage},
	})
	if got := m.PendingAudio(); got == nil || *got != audio {
		t.Errorf("PendingAudio() = %v, want %v", got, audio)
	}
	if got := m.OtherAttachments(); len(got) != 1 || got[0] != legacyImage {
		t.Errorf("OtherAttachments() = %v, want [%v] — legacy image attachment carried through", got, legacyImage)
	}
	if m.AttachmentTouched() {
		t.Error("AttachmentTouched() = true after OpenForEdit, want false — prefilling isn't a user edit")
	}
}

// TestPostComposePanel_Open_ResetsAttachment guards against a stale
// attachment (or touched flag) from a previous edit session leaking into the
// next new-post compose.
func TestPostComposePanel_Open_ResetsAttachment(t *testing.T) {
	m := NewPostComposePanel(80)
	m, _ = m.OpenForEdit(model.Post{
		Attachments: []model.Attachment{{Type: "audio", Src: "https://example.com/track.mp3"}},
	})
	m = m.SetPendingAudio(&model.Attachment{Type: "audio", Src: "https://example.com/other.mp3"})
	m, _ = m.Open(false)
	if got := m.PendingAudio(); got != nil {
		t.Errorf("PendingAudio() = %v after Open(), want nil", got)
	}
	if m.AttachmentTouched() {
		t.Error("AttachmentTouched() = true after Open(), want false")
	}
}

// TestPostComposePanel_PanelHeight_GrowsForPendingAudio guards the layout
// math: a pending audio attachment adds exactly one row, the same as an
// image attachment does, so App's viewport-height recalculation stays in
// sync with what View() actually renders.
func TestPostComposePanel_PanelHeight_GrowsForPendingAudio(t *testing.T) {
	m := NewPostComposePanel(80)
	base := m.PanelHeight()
	m = m.SetPendingAudio(&model.Attachment{Type: "audio", Src: "https://youtu.be/dQw4w9WgXcQ", Artist: "a", Title: "t"})
	if got := m.PanelHeight(); got != base+1 {
		t.Errorf("PanelHeight() = %d after SetPendingAudio, want %d", got, base+1)
	}
}
