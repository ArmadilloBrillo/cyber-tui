package imgview_test

import (
	"image"
	"image/color"
	"image/gif"
	"testing"
	"time"

	"github.com/ragnar/cyber-tui/internal/ui/imgview"
)

func TestGIFFrames_CompositesAndClampsDelays(t *testing.T) {
	red := color.Palette{color.RGBA{}, color.RGBA{R: 255, A: 255}}
	blue := color.Palette{color.RGBA{}, color.RGBA{B: 255, A: 255}}

	// Frame 0: fully red. Frame 1: only pixel (1,0) painted blue, rest left
	// transparent so accumulation must be visible if compositing is correct.
	// Frame 2: same as frame 1, to exercise the delay-clamping branch.
	f0 := image.NewPaletted(image.Rect(0, 0, 2, 2), red)
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			f0.SetColorIndex(x, y, 1)
		}
	}
	f1 := image.NewPaletted(image.Rect(1, 0, 2, 1), blue)
	f1.SetColorIndex(1, 0, 1)
	f2 := image.NewPaletted(image.Rect(1, 0, 2, 1), blue)
	f2.SetColorIndex(1, 0, 1)

	g := &gif.GIF{
		Image:  []*image.Paletted{f0, f1, f2},
		Delay:  []int{0, 1, 5},
		Config: image.Config{Width: 2, Height: 2},
	}

	frames, delays := imgview.GIFFrames(g)
	if len(frames) != 3 || len(delays) != 3 {
		t.Fatalf("got %d frames, %d delays; want 3, 3", len(frames), len(delays))
	}

	wantDelays := []time.Duration{100 * time.Millisecond, 100 * time.Millisecond, 50 * time.Millisecond}
	for i, want := range wantDelays {
		if delays[i] != want {
			t.Errorf("delays[%d] = %v, want %v", i, delays[i], want)
		}
	}

	// (0,0) was only ever painted red in frame 0 and never touched again —
	// it must still be red in frame 2, proving accumulation rather than
	// each frame starting from a blank canvas.
	r, g2, b, _ := frames[2].At(0, 0).RGBA()
	if r>>8 != 255 || g2>>8 != 0 || b>>8 != 0 {
		t.Errorf("frames[2].At(0,0) = (%d,%d,%d), want red carried over from frame 0", r>>8, g2>>8, b>>8)
	}
	// (1,0) was painted blue in frame 1 and again in frame 2.
	r, g2, b, _ = frames[2].At(1, 0).RGBA()
	if b>>8 != 255 || r>>8 != 0 {
		t.Errorf("frames[2].At(1,0) = (%d,%d,%d), want blue", r>>8, g2>>8, b>>8)
	}

	// Mutating frame 0's snapshot must not affect frame 1's — proves the
	// per-frame copy isn't aliased to the shared accumulation canvas.
	rgba0 := frames[0].(*image.RGBA)
	rgba0.Set(0, 0, color.RGBA{G: 255, A: 255})
	r, _, _, _ = frames[1].At(0, 0).RGBA()
	if r>>8 != 255 {
		t.Error("frames[1] was mutated by writing to frames[0]; snapshots are aliased")
	}
}
