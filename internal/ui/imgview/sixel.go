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
// used by Kitty/iTerm2. Returns the DCS escape sequence and the computed
// display size in terminal columns and rows.
func EncodeSixel(img image.Image, maxCols, maxRows, cellPxW, cellPxH int) (encoded string, cols, rows int, err error) {
	if cellPxW <= 0 || cellPxH <= 0 {
		cellPxW, cellPxH = pxPerCol, 2*pxPerCol
	}
	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	cols, rows = fitBox(w, h, maxCols, maxRows, cellPxW, cellPxH)
	src := downscaleToBox(img, cols, rows, cellPxW, cellPxH)

	var buf bytes.Buffer
	enc := sixel.NewEncoder(&buf)
	enc.Colors = 256
	if encErr := enc.Encode(src); encErr != nil {
		return "", 0, 0, fmt.Errorf("imgview: sixel encode: %w", encErr)
	}
	return buf.String(), cols, rows, nil
}
