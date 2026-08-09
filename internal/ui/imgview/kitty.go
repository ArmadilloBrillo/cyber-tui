package imgview

import (
	"encoding/base64"
	"fmt"
	"image"
)

// EncodeKitty encodes img for display via the Kitty terminal graphics
// protocol. maxCols/maxRows bound the terminal display size; the image is
// never upscaled beyond its natural pixel size. Returns the APC escape
// sequence and the computed display size in terminal columns and rows.
//
// placementID selects one of two modes:
//   - 0: anonymous placement (used by the fullscreen modal, where only one
//     image is ever shown at a time). Prefixed with a blunt a=d,d=A
//     (delete-all) self-heal — a no-op if nothing is displayed, but clears a
//     leftover placement from a previous image whose own close-cleanup
//     frame never reached the terminal.
//   - non-zero: a named placement (used for inline rendering, where several
//     images are on screen at once). Uses that value as both the image id
//     and placement id — deliberately never blunt-deletes, since that would
//     erase every other inline image's placement too; see
//     DeleteKittyPlacement for this mode's targeted counterpart. Sending
//     a=T again with the same id/placement (e.g. after scrolling) replaces
//     the existing placement in place per the protocol spec — this is how
//     an inline image "moves" without a separate reposition command.
func EncodeKitty(img image.Image, maxCols, maxRows, placementID int) (encoded string, cols, rows int) {
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
	// m=0: final chunk. q=2: suppress the terminal's OK/error response — by
	// default (q unset) the terminal writes one back over the same stream
	// used for keyboard input, and Bubble Tea's input reader has no way to
	// tell it apart from a real keystroke. The fullscreen modal treats any
	// keypress while open as "close" (see app.go), so that phantom response
	// was closing the modal the instant it opened — every single time.
	if placementID == 0 {
		encoded = fmt.Sprintf("\x1b_Ga=d,d=A\x1b\\\x1b_Ga=T,f=32,s=%d,v=%d,c=%d,r=%d,m=0,q=2;%s\x1b\\", w, h, cols, rows, payload)
	} else {
		encoded = fmt.Sprintf("\x1b_Ga=T,f=32,s=%d,v=%d,c=%d,r=%d,m=0,q=2,i=%d,p=%d;%s\x1b\\", w, h, cols, rows, placementID, placementID, payload)
	}
	return
}

// DeleteKittyPlacement returns the escape sequence to delete exactly one
// named placement (image id + placement id, both set to id by EncodeKitty's
// non-zero-placementID mode) without disturbing any other placement — the
// targeted counterpart to EncodeKitty's anonymous-mode blunt a=d,d=A
// self-heal, which cannot be used here since inline rendering has several
// placements on screen simultaneously.
func DeleteKittyPlacement(id int) string {
	return fmt.Sprintf("\x1b_Ga=d,d=i,i=%d,p=%d\x1b\\", id, id)
}
