package imgview

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
)

// EncodeITerm2 encodes img for display via the iTerm2 inline image protocol,
// which is also supported by WezTerm. maxCols is the maximum terminal column
// width; the image is never upscaled beyond its natural pixel size. Returns the
// OSC escape sequence and the computed display size in terminal columns and rows.
func EncodeITerm2(img image.Image, maxCols int) (encoded string, cols, rows int, err error) {
	bounds := img.Bounds()
	w := bounds.Max.X - bounds.Min.X
	h := bounds.Max.Y - bounds.Min.Y

	var buf bytes.Buffer
	if encErr := png.Encode(&buf, img); encErr != nil {
		return "", 0, 0, fmt.Errorf("imgview: png encode: %w", encErr)
	}
	payload := base64.StdEncoding.EncodeToString(buf.Bytes())
	cols = fitCols(w, maxCols)
	rows = fitRows(h, w, cols)
	// width/height without suffix means terminal character cells in iTerm2.
	encoded = fmt.Sprintf(
		"\x1b]1337;File=inline=1;width=%d;height=%d;preserveAspectRatio=1:%s\x07",
		cols, rows, payload,
	)
	return
}
