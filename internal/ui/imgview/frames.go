package imgview

import (
	"image"
	"image/draw"
	"image/gif"
	"time"
)

// GIFFrames composites g's frames onto an accumulating canvas and returns one
// full-canvas image per frame alongside its display delay.
//
// ponytail: does not implement per-frame disposal methods (background-color
// restore / restore-to-previous) — Over-compositing is correct for GIFs that
// redraw the full frame or paint additively (the common case), but can show
// minor ghosting on GIFs relying on DisposalBackground/DisposalPrevious.
// Upgrade path: branch on g.Disposal[i].
func GIFFrames(g *gif.GIF) (frames []image.Image, delays []time.Duration) {
	canvas := image.NewRGBA(image.Rect(0, 0, g.Config.Width, g.Config.Height))
	frames = make([]image.Image, len(g.Image))
	delays = make([]time.Duration, len(g.Image))
	for i, frame := range g.Image {
		draw.Draw(canvas, frame.Bounds(), frame, frame.Bounds().Min, draw.Over)
		snap := image.NewRGBA(canvas.Rect)
		copy(snap.Pix, canvas.Pix)
		frames[i] = snap

		delayCs := g.Delay[i]
		if delayCs <= 1 {
			delayCs = 10 // browser convention: near-zero delays render unwatchably fast
		}
		delays[i] = time.Duration(delayCs) * 10 * time.Millisecond
	}
	return frames, delays
}
