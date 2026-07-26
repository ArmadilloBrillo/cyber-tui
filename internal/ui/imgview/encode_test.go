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
	seq, cols, rows := imgview.EncodeKitty(img, 40, 20)
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
	_, cols, rows := imgview.EncodeKitty(img, 40, 20)
	if rows > 20 {
		t.Errorf("EncodeKitty: rows=%d, want <= maxRows(20)", rows)
	}
	if cols < 1 || cols > 40 {
		t.Errorf("EncodeKitty: cols=%d, want in [1, maxCols(40)]", cols)
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
