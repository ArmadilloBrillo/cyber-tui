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

// TestPostComposePanel_OpenForEdit_PrefillsImageAndAudioAttachments guards
// the prefill PostDetail/Feed rely on to avoid clobbering an existing
// attachment on save: OpenForEdit must seed attachmentURL from the post's
// own image/gif attachment and pendingAudio from its audio attachment, but
// leave both untouched so a save that never calls SetAttachmentURL/
// SetPendingAudio omits the attachments field entirely (see EditPost's
// attachmentTouched). OtherAttachments stays empty here since both slots
// were filled — it only catches attachment types beyond these two.
func TestPostComposePanel_OpenForEdit_PrefillsImageAndAudioAttachments(t *testing.T) {
	m := NewPostComposePanel(80)
	audio := model.Attachment{Type: "audio", Src: "https://example.com/track.mp3"}
	m, _ = m.OpenForEdit(model.Post{
		Attachments: []model.Attachment{
			audio,
			{Type: "image", Src: "https://example.com/pic.png"},
		},
	})
	if got := m.AttachmentURL(); got != "https://example.com/pic.png" {
		t.Errorf("AttachmentURL() = %q, want the image attachment's src", got)
	}
	if got := m.PendingAudio(); got == nil || *got != audio {
		t.Errorf("PendingAudio() = %v, want %v", got, audio)
	}
	if got := m.OtherAttachments(); len(got) != 0 {
		t.Errorf("OtherAttachments() = %v, want empty — both attachments have dedicated slots", got)
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

// TestPostComposePanel_ImageAndAudioAttachmentsAreMutuallyExclusive guards
// CreatePost's single-attachment signature: setting an image clears a
// pending audio one and vice versa, so a submit never has to decide which of
// two set attachments to actually send.
func TestPostComposePanel_ImageAndAudioAttachmentsAreMutuallyExclusive(t *testing.T) {
	audio := &model.Attachment{Type: "audio", Src: "https://youtu.be/dQw4w9WgXcQ", Origin: "youtube", Artist: "a", Title: "t"}

	m := NewPostComposePanel(80)
	m = m.SetPendingAudio(audio)
	m = m.SetAttachmentURL("https://example.com/pic.png")
	if got := m.PendingAudio(); got != nil {
		t.Errorf("PendingAudio() = %v after SetAttachmentURL, want nil", got)
	}

	m2 := NewPostComposePanel(80)
	m2 = m2.SetAttachmentURL("https://example.com/pic.png")
	m2 = m2.SetPendingAudio(audio)
	if got := m2.AttachmentURL(); got != "" {
		t.Errorf("AttachmentURL() = %q after SetPendingAudio, want empty", got)
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
