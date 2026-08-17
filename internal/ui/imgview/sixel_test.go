package imgview_test

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/ragnar/cyber-tui/internal/ui/imgview"
)

func TestEncodeSixel_ContainsDCS(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	seq, cols, rows, err := imgview.EncodeSixel(img, 40, 20, 0, 0, false, nil)
	if err != nil {
		t.Fatalf("EncodeSixel: %v", err)
	}
	if !strings.HasPrefix(seq, "\x1bP") {
		t.Errorf("EncodeSixel: missing DCS prefix, got %q", seq[:min(len(seq), 20)])
	}
	if !strings.HasSuffix(seq, "\x1b\\") {
		t.Errorf("EncodeSixel: missing ST suffix")
	}
	if cols < 1 {
		t.Errorf("EncodeSixel: cols=%d, want >= 1", cols)
	}
	if rows < 1 {
		t.Errorf("EncodeSixel: rows=%d, want >= 1", rows)
	}
}

func TestEncodeSixel_CapsRows(t *testing.T) {
	// Tall portrait image: without a row cap this would need 50 rows at 10 cols.
	img := image.NewRGBA(image.Rect(0, 0, 100, 1000))
	_, cols, rows, err := imgview.EncodeSixel(img, 40, 20, 0, 0, false, nil)
	if err != nil {
		t.Fatalf("EncodeSixel: %v", err)
	}
	if rows > 20 {
		t.Errorf("EncodeSixel: rows=%d, want <= maxRows(20)", rows)
	}
	if cols < 1 || cols > 40 {
		t.Errorf("EncodeSixel: cols=%d, want in [1, maxCols(40)]", cols)
	}
}

func TestEncodeSixel_NeverUpscales(t *testing.T) {
	// A tiny image with generous bounds should stay tiny, not get blown up,
	// when allowUpscale is false (the inline-thumbnail case).
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	_, cols, rows, err := imgview.EncodeSixel(img, 200, 100, 0, 0, false, nil)
	if err != nil {
		t.Fatalf("EncodeSixel: %v", err)
	}
	if cols > 1 {
		t.Errorf("EncodeSixel: cols=%d for a 2px-wide image, want 1 (no upscaling)", cols)
	}
	if rows > 1 {
		t.Errorf("EncodeSixel: rows=%d for a 2px-tall image, want 1 (no upscaling)", rows)
	}
}

// TestEncodeSixel_AllowUpscaleFillsBox confirms allowUpscale=true (the
// fullscreen modal's user-driven zoom) does the opposite of
// TestEncodeSixel_NeverUpscales: a tiny image is scaled up to fill the
// requested box instead of being left at its native 1x1-cell size.
func TestEncodeSixel_AllowUpscaleFillsBox(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	_, cols, rows, err := imgview.EncodeSixel(img, 20, 10, 0, 0, true, nil)
	if err != nil {
		t.Fatalf("EncodeSixel: %v", err)
	}
	if cols != 20 {
		t.Errorf("EncodeSixel: cols=%d, want exactly maxCols(20) with allowUpscale=true", cols)
	}
	if rows < 1 || rows > 10 {
		t.Errorf("EncodeSixel: rows=%d, want in [1, maxRows(10)]", rows)
	}
}

func TestEncodeSixel_UsesRealCellSize(t *testing.T) {
	// A wide image at a small assumed cell size (0,0 -> default 10x20) needs
	// more columns than the same image measured against a much larger real
	// cell size (as Konsole or similar can report) — confirms cellPxW/cellPxH
	// actually drives the sizing math instead of being ignored.
	img := image.NewRGBA(image.Rect(0, 0, 400, 400))
	_, defaultCols, defaultRows, err := imgview.EncodeSixel(img, 200, 200, 0, 0, false, nil)
	if err != nil {
		t.Fatalf("EncodeSixel (default cell size): %v", err)
	}
	_, largeCellCols, largeCellRows, err := imgview.EncodeSixel(img, 200, 200, 40, 80, false, nil)
	if err != nil {
		t.Fatalf("EncodeSixel (large cell size): %v", err)
	}
	if largeCellCols >= defaultCols {
		t.Errorf("EncodeSixel: cols with a 4x larger cell size = %d, want < default cols = %d", largeCellCols, defaultCols)
	}
	if largeCellRows >= defaultRows {
		t.Errorf("EncodeSixel: rows with a 4x larger cell size = %d, want < default rows = %d", largeCellRows, defaultRows)
	}
}

// TestEncodeSixel_AppliesDither confirms a non-nil DitherOptions changes the
// encoded output. Unlike Kitty/iTerm2, Sixel's DCS payload isn't a
// self-contained decodable image format in this codebase (no Sixel decoder
// is available), so this checks the byte-level effect rather than decoding
// pixels back out — see imgview.Dither's own tests for the pixel-level
// invariants.
func TestEncodeSixel_AppliesDither(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 30), G: uint8(y * 30), B: 128, A: 255})
		}
	}
	plain, _, _, err := imgview.EncodeSixel(img, 40, 20, 0, 0, false, nil)
	if err != nil {
		t.Fatalf("EncodeSixel (no dither): %v", err)
	}
	dithered, _, _, err := imgview.EncodeSixel(img, 40, 20, 0, 0, false, &imgview.DitherOptions{
		PixelSize: 1,
		FgColor:   color.RGBA{R: 0, G: 255, B: 65, A: 255},
		BgColor:   color.RGBA{R: 13, G: 13, B: 13, A: 255},
	})
	if err != nil {
		t.Fatalf("EncodeSixel (dithered): %v", err)
	}
	if dithered == plain {
		t.Fatal("EncodeSixel: dithered output identical to undithered output")
	}
}
