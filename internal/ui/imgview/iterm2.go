package imgview

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
)

// EncodeITerm2 encodes img for display via the iTerm2 inline image protocol,
// which is also supported by WezTerm. maxCols/maxRows bound the terminal
// display size; the image is never upscaled beyond its natural pixel size.
// cellPxW/cellPxH is the terminal's real cell pixel size (from
// TerminalCellPixelSize); if <= 0 (real size unavailable), falls back to the
// assumed default cell size. Getting this right matters more for iTerm2 than
// Kitty: iTerm2's preserveAspectRatio=1 letterboxes the image inside the
// given width/height cell box using iTerm2's own real font metrics, so a
// wrong assumed cell aspect ratio leaves visible blank space rather than
// just mildly stretching the image (as Kitty's fill-exact-box behavior
// does). Returns the OSC escape sequence and the computed display size in
// terminal columns and rows.
func EncodeITerm2(img image.Image, maxCols, maxRows, cellPxW, cellPxH int) (encoded string, cols, rows int, err error) {
	if cellPxW <= 0 || cellPxH <= 0 {
		cellPxW, cellPxH = pxPerCol, 2*pxPerCol
	}
	bounds := img.Bounds()
	w := bounds.Max.X - bounds.Min.X
	h := bounds.Max.Y - bounds.Min.Y

	var buf bytes.Buffer
	if encErr := png.Encode(&buf, img); encErr != nil {
		return "", 0, 0, fmt.Errorf("imgview: png encode: %w", encErr)
	}
	payload := base64.StdEncoding.EncodeToString(buf.Bytes())
	cols, rows = fitBox(w, h, maxCols, maxRows, cellPxW, cellPxH)
	// width/height without suffix means terminal character cells in iTerm2.
	encoded = fmt.Sprintf(
		"\x1b]1337;File=inline=1;width=%d;height=%d;preserveAspectRatio=1:%s\x07",
		cols, rows, payload,
	)
	return
}
