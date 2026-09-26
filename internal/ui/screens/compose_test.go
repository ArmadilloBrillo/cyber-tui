package screens

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/ragnar/cyber-tui/internal/model"
)

// ctrlD is the "save this new post to the Journal" key.
var ctrlD = tea.KeyMsg{Type: tea.KeyCtrlD}

// TestPostComposePanel_CtrlD_EmitsSaveAsNoteMsg: Ctrl+D in a new-post compose
// with a non-empty body emits ComposeSaveAsNoteMsg carrying the body.
func TestPostComposePanel_CtrlD_EmitsSaveAsNoteMsg(t *testing.T) {
	m := NewPostComposePanel(80)
	m, _ = m.Open(false)
	m.textarea.SetValue("a half-finished thought")
	_, cmd := m.Update(ctrlD)
	if cmd == nil {
		t.Fatal("expected a cmd from Ctrl+D, got nil")
	}
	msg := cmd()
	got, ok := msg.(ComposeSaveAsNoteMsg)
	if !ok {
		t.Fatalf("Ctrl+D produced %T, want ComposeSaveAsNoteMsg", msg)
	}
	if got.Content != "a half-finished thought" {
		t.Errorf("Content = %q, want %q", got.Content, "a half-finished thought")
	}
}

// TestPostComposePanel_CtrlD_NoOpWhenEmpty: nothing to save, no message.
func TestPostComposePanel_CtrlD_NoOpWhenEmpty(t *testing.T) {
	m := NewPostComposePanel(80)
	m, _ = m.Open(false)
	m.textarea.SetValue("   \n  ")
	if _, cmd := m.Update(ctrlD); cmd != nil {
		t.Fatalf("expected no cmd for Ctrl+D on an empty body, got %T", cmd())
	}
}

// TestPostComposePanel_CtrlD_NoOpWhenEditing: "save as note" is not offered
// when correcting an already-published post.
func TestPostComposePanel_CtrlD_NoOpWhenEditing(t *testing.T) {
	m := NewPostComposePanel(80)
	m, _ = m.OpenForEdit(model.Post{Content: "already posted"})
	if _, cmd := m.Update(ctrlD); cmd != nil {
		t.Fatalf("expected no cmd for Ctrl+D while editing, got %T", cmd())
	}
}

// TestPostComposePanel_Submitting_SuppressesSubmitKeys: while a submit is in
// flight, Ctrl+S / Ctrl+D / Esc are inert so the user can't double-fire.
func TestPostComposePanel_Submitting_SuppressesSubmitKeys(t *testing.T) {
	m := NewPostComposePanel(80)
	m, _ = m.Open(false)
	m.textarea.SetValue("in flight")
	m = m.MarkSubmitting()
	for _, k := range []tea.KeyMsg{
		{Type: tea.KeyCtrlS},
		{Type: tea.KeyCtrlD},
		{Type: tea.KeyEsc},
	} {
		if _, cmd := m.Update(k); cmd != nil {
			t.Errorf("key %v produced a cmd while submitting, want none", k)
		}
	}
	m = m.ClearSubmitting()
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS}); cmd == nil {
		t.Error("Ctrl+S still inert after ClearSubmitting, want a submit cmd")
	}
}

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

func TestPostComposePanel_Bork_TabReachesToggleAndSpaceFlipsIt(t *testing.T) {
	m := NewPostComposePanel(80)
	m, _ = m.Open(false)
	m.focus = postFieldNSFW
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.focus != postFieldBork {
		t.Fatalf("focus after Tab from nsfw = %v, want postFieldBork", m.focus)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if !m.IsBork() {
		t.Error("IsBork() = false after Space on the bork toggle, want true")
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "[x] bork") {
		t.Errorf("expected a ticked bork box in the view, got: %q", view)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if m.IsBork() {
		t.Error("IsBork() = true after a second Space, want false")
	}
}

func TestPostComposePanel_Bork_ResetsOnOpenAndOpenForEdit(t *testing.T) {
	m := NewPostComposePanel(80)
	m.isBork = true
	m, _ = m.Open(false)
	if m.IsBork() {
		t.Error("IsBork() = true after Open(), want false")
	}
	m.isBork = true
	m, _ = m.OpenForEdit(model.Post{Content: "hi"})
	if m.IsBork() {
		t.Error("IsBork() = true after OpenForEdit(), want false")
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

var (
	altEnter = tea.KeyMsg{Type: tea.KeyEnter, Alt: true}
	plainEnt = tea.KeyMsg{Type: tea.KeyEnter}
	ctrlL    = tea.KeyMsg{Type: tea.KeyCtrlL}
)

func openReply(t *testing.T, spec string) ComposeModel {
	t.Helper()
	m := NewComposeModel(80)
	if spec != "" {
		m = m.SetHardBreakKey(spec)
	}
	m, _ = m.Open("reply", "write")
	return m
}

func TestCompose_HardBreak_DefaultAltEnterInsertsHardBreak(t *testing.T) {
	m := openReply(t, "")
	m, _ = m.Update(runesMsg("a"))
	m, _ = m.Update(altEnter)
	m, _ = m.Update(runesMsg("b"))
	if got := m.Content(); got != "a  \nb" {
		t.Errorf("Content = %q, want %q", got, "a  \nb")
	}
}

func TestCompose_HardBreak_EnterStillInsertsParagraphBreak(t *testing.T) {
	m := openReply(t, "")
	m, _ = m.Update(runesMsg("a"))
	m, _ = m.Update(plainEnt)
	m, _ = m.Update(runesMsg("b"))
	if got := m.Content(); got != "a\n\nb" {
		t.Errorf("Content = %q, want %q", got, "a\n\nb")
	}
}

func TestCompose_HardBreak_ConfiguredKeyReplacesDefault(t *testing.T) {
	m := openReply(t, "ctrl+l")
	m, _ = m.Update(runesMsg("a"))
	m, _ = m.Update(altEnter)
	m, _ = m.Update(ctrlL)
	m, _ = m.Update(runesMsg("b"))
	if got := m.Content(); got != "a  \nb" {
		t.Errorf("Content = %q, want %q", got, "a  \nb")
	}
}

func TestCompose_HardBreak_InvalidSpecFallsBackToDefault(t *testing.T) {
	m := openReply(t, "ctrl+s")
	m, _ = m.Update(runesMsg("a"))
	m, _ = m.Update(altEnter)
	if got := m.Content(); got != "a  \n" {
		t.Errorf("Content = %q, want %q", got, "a  \n")
	}
}

func TestCompose_Paste_ConvertsSingleNewlines(t *testing.T) {
	m := openReply(t, "")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("one\ntwo\n\nthree"), Paste: true})
	if got := m.Content(); got != "one  \ntwo\n\nthree" {
		t.Errorf("Content = %q, want %q", got, "one  \ntwo\n\nthree")
	}
}

func TestCompose_Typing_NotConvertedLikePaste(t *testing.T) {
	m := openReply(t, "")
	m, _ = m.Update(runesMsg("a\nb"))
	if got := m.Content(); got != "a\nb" {
		t.Errorf("Content = %q, want %q", got, "a\nb")
	}
}

func openBodyPanel(t *testing.T, spec string) PostComposePanel {
	t.Helper()
	m := NewPostComposePanel(80)
	if spec != "" {
		m = m.SetHardBreakKey(spec)
	}
	m, _ = m.Open(false)
	m.focus = postFieldBody
	m.textarea.Focus()
	return m
}

func TestPostComposePanel_HardBreak_BodyInsertsHardBreak(t *testing.T) {
	m := openBodyPanel(t, "")
	m, _ = m.Update(runesMsg("a"))
	m, _ = m.Update(altEnter)
	m, _ = m.Update(runesMsg("b"))
	if got := m.textarea.Value(); got != "a  \nb" {
		t.Errorf("body = %q, want %q", got, "a  \nb")
	}
}

func TestPostComposePanel_HardBreak_NotInsertedOutsideBody(t *testing.T) {
	m := openBodyPanel(t, "")
	m.focus = postFieldTitle
	m, _ = m.Update(altEnter)
	if got := m.textarea.Value(); got != "" {
		t.Errorf("body = %q, want empty", got)
	}
}

func TestPostComposePanel_Paste_ConvertsSingleNewlines(t *testing.T) {
	m := openBodyPanel(t, "")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a\nb"), Paste: true})
	if got := m.textarea.Value(); got != "a  \nb" {
		t.Errorf("body = %q, want %q", got, "a  \nb")
	}
}

// rowAfterTail returns the rendered row directly below the one containing
// "tail", or "" when there is none.
func rowAfterTail(view string) string {
	rows := strings.Split(ansi.Strip(view), "\n")
	for i, r := range rows {
		if strings.Contains(r, "tail") && i+1 < len(rows) {
			return rows[i+1]
		}
	}
	return ""
}

// The hard-break key must keep the new cursor row in view once the editor has
// reached its maximum height, like Enter does. The app renders between
// keypresses, so the tests do too.
func TestCompose_HardBreak_ScrollsWhenEditorAtMaxHeight(t *testing.T) {
	m := openReply(t, "")
	for i := 1; i <= composeMaxLines+4; i++ {
		m, _ = m.Update(runesMsg(fmt.Sprintf("line%d", i)))
		_ = m.View()
		m, _ = m.Update(altEnter)
		_ = m.View()
	}
	m, _ = m.Update(runesMsg("tail"))
	_ = m.View()
	m, _ = m.Update(altEnter)
	if row := rowAfterTail(m.View()); !strings.Contains(row, "┃") {
		t.Errorf("the new line after a hard break is out of view; row below tail = %q", row)
	}
}

func TestPostComposePanel_HardBreak_ScrollsWhenBodyAtMaxHeight(t *testing.T) {
	m := openBodyPanel(t, "")
	for i := 1; i <= composeMaxLines+4; i++ {
		m, _ = m.Update(runesMsg(fmt.Sprintf("line%d", i)))
		_ = m.View()
		m, _ = m.Update(altEnter)
		_ = m.View()
	}
	m, _ = m.Update(runesMsg("tail"))
	_ = m.View()
	m, _ = m.Update(altEnter)
	if row := rowAfterTail(m.View()); !strings.Contains(row, "┃") {
		t.Errorf("the new line after a hard break is out of view; row below tail = %q", row)
	}
}
