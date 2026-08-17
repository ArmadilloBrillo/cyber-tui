package imgview

import (
	"bytes"
	"fmt"
	"image"

	"github.com/mattn/go-sixel"
)

// EncodeSixel encodes img for display via the DECSIXEL graphics protocol,
// supported by xterm, foot, mlterm, contour, mintty, yaft, and others.
// maxCols/maxRows bound the terminal display size; the image is never
// upscaled beyond its natural pixel size. Unlike Kitty/iTerm2, Sixel has no
// terminal-side scale-to-fit parameter, so the image is downscaled in pixel
// space here before encoding, using the terminal's real cell pixel size
// (cellPxW x cellPxH, from TerminalCellPixelSize) so the reported cols/rows
// match what the terminal will actually display. If cellPxW or cellPxH is
// <= 0 (real size unavailable), falls back to the assumed default cell size
// used by Kitty/iTerm2. allowUpscale, when true, lets the image be scaled up
// past its natural pixel size to fill maxCols/maxRows (used only by the
// fullscreen modal's user-driven zoom — see imgview.NativeCellBox); false
// (inline thumbnails) never upscales. Returns the DCS escape sequence and
// the computed display size in terminal columns and rows. dither, when
// non-nil, applies the RasterImage-style duotone dither effect (see Dither)
// to the image before encoding.
func EncodeSixel(img image.Image, maxCols, maxRows, cellPxW, cellPxH int, allowUpscale bool, dither *DitherOptions) (encoded string, cols, rows int, err error) {
	cellPxW, cellPxH = EffectiveCellPx(cellPxW, cellPxH)
	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	cols, rows = fitBox(w, h, maxCols, maxRows, cellPxW, cellPxH, allowUpscale)
	src := downscaleToBox(img, cols, rows, cellPxW, cellPxH, allowUpscale)
	if dither != nil {
		src = Dither(src, *dither)
	}

	var buf bytes.Buffer
	enc := sixel.NewEncoder(&buf)
	enc.Colors = 256
	if encErr := enc.Encode(src); encErr != nil {
		return "", 0, 0, fmt.Errorf("imgview: sixel encode: %w", encErr)
	}
	return buf.String(), cols, rows, nil
}
