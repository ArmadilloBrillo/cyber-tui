package imgview_test

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/ragnar/cyber-tui/internal/ui/imgview"
)

func TestEncodeKitty_ContainsAPC(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	seq, cols, rows := imgview.EncodeKitty(img, 40, 20, 0)
	if !strings.HasPrefix(seq, "\x1b_G") {
		t.Errorf("EncodeKitty: missing APC prefix, got %q", seq[:min(len(seq), 20)])
	}
	if !strings.HasSuffix(seq, "\x1b\\") {
		t.Errorf("EncodeKitty: missing ST suffix")
	}
	if cols < 1 {
		t.Errorf("EncodeKitty: cols=%d, want >= 1", cols)
	}
	if rows < 1 {
		t.Errorf("EncodeKitty: rows=%d, want >= 1", rows)
	}
}

func TestEncodeKitty_CapsRows(t *testing.T) {
	// Tall portrait image: without a row cap this would need 50 rows at 10 cols.
	img := image.NewRGBA(image.Rect(0, 0, 100, 1000))
	_, cols, rows := imgview.EncodeKitty(img, 40, 20, 0)
	if rows > 20 {
		t.Errorf("EncodeKitty: rows=%d, want <= maxRows(20)", rows)
	}
	if cols < 1 || cols > 40 {
		t.Errorf("EncodeKitty: cols=%d, want in [1, maxCols(40)]", cols)
	}
}

func TestEncodeKitty_PrependsDeleteAllPlacements(t *testing.T) {
	// placementID==0 (anonymous mode, used by the fullscreen modal) always
	// leads with a delete-all-placements command so opening a new image
	// self-heals a leftover placement from a previous image whose own
	// close-cleanup frame never reached the terminal (e.g. dropped behind a
	// slow flush of a large prior image).
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	seq, _, _ := imgview.EncodeKitty(img, 40, 20, 0)
	if !strings.HasPrefix(seq, "\x1b_Ga=d,d=A\x1b\\") {
		t.Errorf("EncodeKitty: expected delete-all-placements prefix, got %q", seq[:min(len(seq), 30)])
	}

	// Back-to-back calls (as would happen opening several images in a row)
	// must each still lead with the delete-all command.
	seq2, _, _ := imgview.EncodeKitty(img, 40, 20, 0)
	if !strings.HasPrefix(seq2, "\x1b_Ga=d,d=A\x1b\\") {
		t.Errorf("EncodeKitty: expected delete-all-placements prefix on repeated call, got %q", seq2[:min(len(seq2), 30)])
	}
}

// TestEncodeKitty_NamedPlacement_NeverBluntDeletes confirms that a non-zero
// placementID (inline rendering, where several images are on screen at
// once) never emits the blunt a=d,d=A delete-all — that would erase every
// other inline image's placement, not just this one — and instead embeds
// the id as both the image id and placement id so DeleteKittyPlacement can
// target it precisely later.
func TestEncodeKitty_NamedPlacement_NeverBluntDeletes(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	seq, _, _ := imgview.EncodeKitty(img, 40, 20, 7)
	if strings.Contains(seq, "d=A") {
		t.Errorf("EncodeKitty with a named placement must never blunt-delete-all, got %q", seq)
	}
	if !strings.Contains(seq, "i=7") || !strings.Contains(seq, "p=7") {
		t.Errorf("expected i=7 and p=7 in the encoded sequence, got %q", seq)
	}
}

// TestEncodeKitty_SuppressesTerminalResponse confirms every EncodeKitty mode
// sets q=2, suppressing the terminal's OK/error acknowledgment. Without it,
// the terminal writes that acknowledgment back over the same stream used for
// keyboard input, which Bubble Tea's input reader can't distinguish from a
// real keystroke — and the fullscreen modal treats any keypress while open
// as "close", so an unsuppressed response closed the modal the instant it
// opened.
func TestEncodeKitty_SuppressesTerminalResponse(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))

	seqAnon, _, _ := imgview.EncodeKitty(img, 40, 20, 0)
	if !strings.Contains(seqAnon, "q=2") {
		t.Errorf("anonymous-mode EncodeKitty must set q=2, got %q", seqAnon)
	}

	seqNamed, _, _ := imgview.EncodeKitty(img, 40, 20, 7)
	if !strings.Contains(seqNamed, "q=2") {
		t.Errorf("named-placement EncodeKitty must set q=2, got %q", seqNamed)
	}
}

func TestDeleteKittyPlacement_TargetsExactIDPair(t *testing.T) {
	seq := imgview.DeleteKittyPlacement(7)
	if !strings.HasPrefix(seq, "\x1b_G") || !strings.HasSuffix(seq, "\x1b\\") {
		t.Errorf("expected a well-formed APC sequence, got %q", seq)
	}
	if !strings.Contains(seq, "d=i") || !strings.Contains(seq, "i=7") || !strings.Contains(seq, "p=7") {
		t.Errorf("expected a targeted delete (d=i,i=7,p=7), got %q", seq)
	}
	if strings.Contains(seq, "d=A") {
		t.Errorf("expected a targeted delete, not delete-all, got %q", seq)
	}
}

func TestEncodeITerm2_ContainsOSC(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{G: 255, A: 255})
	seq, cols, rows, err := imgview.EncodeITerm2(img, 80, 40)
	if err != nil {
		t.Fatalf("EncodeITerm2: %v", err)
	}
	if !strings.HasPrefix(seq, "\x1b]1337;") {
		t.Errorf("EncodeITerm2: missing OSC 1337 prefix, got %q", seq[:min(len(seq), 20)])
	}
	if !strings.HasSuffix(seq, "\x07") {
		t.Errorf("EncodeITerm2: missing BEL terminator")
	}
	if cols < 1 {
		t.Errorf("EncodeITerm2: cols=%d, want >= 1", cols)
	}
	if rows < 1 {
		t.Errorf("EncodeITerm2: rows=%d, want >= 1", rows)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
