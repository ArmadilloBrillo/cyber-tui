package imgview_test

import (
	"image"
	"image/color"
	"testing"

	"github.com/ragnar/cyber-tui/internal/ui/imgview"
)

var (
	testFg = color.RGBA{R: 0, G: 255, B: 65, A: 255}
	testBg = color.RGBA{R: 13, G: 13, B: 13, A: 255}
)

// TestDither_OutputIsBlendBetweenFgAndBg checks the core duotone invariant:
// every output channel lies between the corresponding BgColor and FgColor
// channel values (inclusive) — the recolor step is always a mix(bg, fg, t)
// for some t in [0,1], never a color outside that range.
func TestDither_OutputIsBlendBetweenFgAndBg(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 16), G: uint8(y * 16), B: 128, A: 255})
		}
	}
	out := imgview.Dither(img, imgview.DitherOptions{PixelSize: 1, FgColor: testFg, BgColor: testBg})
	inRange := func(v, a, b uint8) bool {
		if a > b {
			a, b = b, a
		}
		return v >= a && v <= b
	}
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			c := out.RGBAAt(x, y)
			if !inRange(c.R, testFg.R, testBg.R) || !inRange(c.G, testFg.G, testBg.G) || !inRange(c.B, testFg.B, testBg.B) || c.A != 255 {
				t.Fatalf("pixel (%d,%d) = %+v, want a blend between FgColor %+v and BgColor %+v with A=255", x, y, c, testFg, testBg)
			}
		}
	}
}

// TestDither_PixelationSamplesTopLeftTexelOnly checks that pixelation picks
// the top-left texel of each PixelSize block and ignores the rest of the
// block's content, rather than averaging it.
func TestDither_PixelationSamplesTopLeftTexelOnly(t *testing.T) {
	const size = 16
	const blockSize = 4
	uniform := image.NewRGBA(image.Rect(0, 0, size, size))
	noisy := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			blockX := (x / blockSize) * blockSize
			blockY := (y / blockSize) * blockSize
			v := uint8(((blockX/blockSize)*37+(blockY/blockSize)*59)%200 + 20)
			topLeft := color.RGBA{R: v, G: v, B: v, A: 255}
			uniform.Set(x, y, topLeft)
			if x == blockX && y == blockY {
				noisy.Set(x, y, topLeft)
			} else {
				// Different value elsewhere in the block — pixelation must ignore it.
				noisy.Set(x, y, color.RGBA{R: 255 - v, G: 255 - v, B: 255 - v, A: 255})
			}
		}
	}
	opts := imgview.DitherOptions{PixelSize: blockSize, FgColor: testFg, BgColor: testBg}
	outUniform := imgview.Dither(uniform, opts)
	outNoisy := imgview.Dither(noisy, opts)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if a, b := outUniform.RGBAAt(x, y), outNoisy.RGBAAt(x, y); a != b {
				t.Fatalf("pixel (%d,%d): top-left-only=%+v, full-block-noisy=%+v, want equal", x, y, a, b)
			}
		}
	}
}

// TestDither_Deterministic guards against accidentally introducing
// time-/rand-seeded noise: the same input must always produce the same
// output.
func TestDither_Deterministic(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 12, 12))
	for y := 0; y < 12; y++ {
		for x := 0; x < 12; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 20), G: uint8(y * 20), B: 100, A: 255})
		}
	}
	opts := imgview.DitherOptions{PixelSize: 2, FgColor: testFg, BgColor: testBg}
	a := imgview.Dither(img, opts)
	b := imgview.Dither(img, opts)
	for y := 0; y < 12; y++ {
		for x := 0; x < 12; x++ {
			if av, bv := a.RGBAAt(x, y), b.RGBAAt(x, y); av != bv {
				t.Fatalf("pixel (%d,%d): run1=%+v run2=%+v, want identical (Dither must be deterministic)", x, y, av, bv)
			}
		}
	}
}

// TestDither_NonZeroOriginBoundsHandledConsistently checks that a source
// image whose bounds don't start at (0,0) — e.g. a sub-image — produces the
// same dither pattern as an equivalent (0,0)-origin image, since Dither
// indexes the pixelation/Bayer pattern by output-relative coordinates.
func TestDither_NonZeroOriginBoundsHandledConsistently(t *testing.T) {
	base := image.NewRGBA(image.Rect(0, 0, 16, 16))
	shifted := image.NewRGBA(image.Rect(5, 7, 21, 23))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			c := color.RGBA{R: 160, G: 160, B: 160, A: 255}
			base.Set(x, y, c)
			shifted.Set(5+x, 7+y, c)
		}
	}
	opts := imgview.DitherOptions{PixelSize: 1, FgColor: testFg, BgColor: testBg}
	outBase := imgview.Dither(base, opts)
	outShifted := imgview.Dither(shifted, opts)
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			if a, b := outBase.RGBAAt(x, y), outShifted.RGBAAt(x, y); a != b {
				t.Fatalf("pixel (%d,%d): base=%+v shifted=%+v, want equal", x, y, a, b)
			}
		}
	}
}

func TestPixelSizeForSharpness(t *testing.T) {
	cases := map[string]int{
		"rough":   4,
		"medium":  3,
		"sharp":   2,
		"crisp":   1,
		"":        3,
		"garbage": 3,
	}
	for level, want := range cases {
		if got := imgview.PixelSizeForSharpness(level); got != want {
			t.Errorf("PixelSizeForSharpness(%q) = %d, want %d", level, got, want)
		}
	}
}

func TestParseHexColor(t *testing.T) {
	c, ok := imgview.ParseHexColor("#00FF41")
	if !ok {
		t.Fatal("ParseHexColor(#00FF41): ok = false, want true")
	}
	want := color.RGBA{R: 0, G: 255, B: 65, A: 255}
	if c != want {
		t.Errorf("ParseHexColor(#00FF41) = %+v, want %+v", c, want)
	}

	invalid := []string{"", "#fff", "00FF41", "#00FF4", "#GGFF41", "#00FF411"}
	for _, s := range invalid {
		if _, ok := imgview.ParseHexColor(s); ok {
			t.Errorf("ParseHexColor(%q): ok = true, want false", s)
		}
	}
}
