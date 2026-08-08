package imgview

import (
	"encoding/base64"
	"fmt"
	"image"
)

// EncodeKitty encodes img for display via the Kitty terminal graphics protocol.
// maxCols/maxRows bound the terminal display size; the image is never
// upscaled beyond its natural pixel size. Returns the APC escape sequence and
// the computed display size in terminal columns and rows.
func EncodeKitty(img image.Image, maxCols, maxRows int) (encoded string, cols, rows int) {
	bounds := img.Bounds()
	w := bounds.Max.X - bounds.Min.X
	h := bounds.Max.Y - bounds.Min.Y

	// Build raw RGBA pixel data.
	raw := make([]byte, w*h*4)
	i := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			raw[i] = byte(r >> 8)
			raw[i+1] = byte(g >> 8)
			raw[i+2] = byte(b >> 8)
			raw[i+3] = byte(a >> 8)
			i += 4
		}
	}

	payload := base64.StdEncoding.EncodeToString(raw)
	cols, rows = fitBox(w, h, maxCols, maxRows, pxPerCol, 2*pxPerCol)
	// a=T: transmit and display. f=32: 32-bit RGBA. s/v: pixel dimensions.
	// c/r: display size in terminal columns/rows (Kitty scales to fit, preserving aspect ratio).
	// m=0: final chunk.
	//
	// Prefixed with a=d,d=A (delete all placements): a no-op if nothing is
	// currently displayed, but self-heals a leftover placement from a
	// previous image whose own close-cleanup frame never reached the
	// terminal (e.g. dropped behind a slow flush of a large prior image).
	encoded = fmt.Sprintf("\x1b_Ga=d,d=A\x1b\\\x1b_Ga=T,f=32,s=%d,v=%d,c=%d,r=%d,m=0;%s\x1b\\", w, h, cols, rows, payload)
	return
}
