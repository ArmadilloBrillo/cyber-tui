package imgview

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"strings"
)

// EncodeKitty encodes img for display via the Kitty terminal graphics
// protocol. maxCols/maxRows bound the terminal display size; the image is
// never upscaled beyond its natural pixel size. Returns the APC escape
// sequence and the computed display size in terminal columns and rows.
//
// placementID selects one of two modes:
//   - 0: anonymous placement. Prefixed with a blunt a=d,d=A (delete-all)
//     self-heal — a no-op if nothing is displayed, but clears a leftover
//     placement from a previous image whose own close-cleanup frame never
//     reached the terminal. Not used by any caller in this codebase — the
//     fullscreen modal uses its own fixed non-zero id
//     (App.kittyModalPlacementID) instead, precisely so its cleanup never
//     blunt-deletes inline rendering's placements too — but kept as a
//     supported mode of the protocol itself, exercised directly by this
//     package's own tests.
//   - non-zero: a named placement (used for inline rendering, where several
//     images are on screen at once, and by the fullscreen modal's own fixed
//     id). Uses that value as both the image id and placement id —
//     deliberately never blunt-deletes, since that would erase every other
//     placement too; see DeleteKittyPlacement for this mode's targeted
//     counterpart. Sending a=T again with the same id/placement (e.g. after
//     scrolling) replaces the existing placement in place per the protocol
//     spec — this is how an inline image "moves" without a separate
//     reposition command.
//
// cellPxW/cellPxH is the terminal's real cell pixel size (from
// TerminalCellPixelSize); if <= 0 (real size unavailable), falls back to the
// assumed default cell size. Kitty stretches the image to exactly fill the
// computed c/r box regardless, so this only affects how closely the aspect
// ratio is preserved, not whether blank space appears. allowUpscale, when
// true, lets the image be scaled up past its natural pixel size to fill
// maxCols/maxRows (used only by the fullscreen modal's user-driven zoom —
// see imgview.NativeCellBox); false (inline thumbnails) never upscales.
// dither, when non-nil, applies the RasterImage-style duotone dither effect
// (see Dither) to the image before encoding.
func EncodeKitty(img image.Image, maxCols, maxRows, cellPxW, cellPxH, placementID int, allowUpscale bool, dither *DitherOptions) (encoded string, cols, rows int, err error) {
	cellPxW, cellPxH = EffectiveCellPx(cellPxW, cellPxH)
	bounds := img.Bounds()
	w := bounds.Max.X - bounds.Min.X
	h := bounds.Max.Y - bounds.Min.Y
	cols, rows = fitBox(w, h, maxCols, maxRows, cellPxW, cellPxH, allowUpscale)

	src := downscaleToBox(img, cols, rows, cellPxW, cellPxH, allowUpscale)
	if dither != nil {
		src = Dither(src, *dither)
	}
	var buf bytes.Buffer
	if encErr := png.Encode(&buf, src); encErr != nil {
		return "", 0, 0, fmt.Errorf("imgview: png encode: %w", encErr)
	}
	payload := base64.StdEncoding.EncodeToString(buf.Bytes())
	// a=T: transmit and display. f=100: PNG-encoded payload — no s=/v= needed,
	// the terminal reads pixel dimensions from the PNG data itself (unlike the
	// raw-RGBA f=32 this used to send, which required them explicitly). c/r:
	// display size in terminal columns/rows (Kitty scales to fit, preserving
	// aspect ratio). q=2: suppress the terminal's OK/error response — by
	// default (q unset) the terminal writes one back over the same stream
	// used for keyboard input, and Bubble Tea's input reader has no way to
	// tell it apart from a real keystroke. The fullscreen modal treats any
	// keypress while open as "close" (see app.go), so that phantom response
	// was closing the modal the instant it opened — every single time.
	var control string
	if placementID == 0 {
		control = fmt.Sprintf("a=T,f=100,c=%d,r=%d,q=2", cols, rows)
	} else {
		control = fmt.Sprintf("a=T,f=100,c=%d,r=%d,q=2,i=%d,p=%d", cols, rows, placementID, placementID)
	}
	chunked := chunkKittyPayload(control, payload)
	if placementID == 0 {
		encoded = "\x1b_Ga=d,d=A\x1b\\" + chunked
	} else {
		encoded = chunked
	}
	return
}

// kittyChunkSize is the Kitty graphics protocol's maximum payload size per
// APC sequence (all chunks but the last must be a multiple of 4). Terminals
// reached through Windows' ConPTY layer (e.g. WezTerm's local-process panes
// on Windows) have been observed failing to render a large image sent as one
// giant unchunked sequence — ConPTY is a known source of interference with
// long or unrecognized escape sequences. Chunking is also simply what the
// protocol spec expects for large payloads, independent of that.
const kittyChunkSize = 4096

// chunkKittyPayload splits payload into kittyChunkSize-byte APC sequences per
// the Kitty graphics protocol's chunked-transmission mode: the first chunk
// carries control (the full a=/f=/c=/r=/q=/i=/p= control data) plus m=1 if
// more chunks follow; every subsequent chunk carries only m (m=1, or m=0 on
// the last) and optionally q — no other control keys repeated, per spec. If
// payload fits in a single chunk, this degenerates to exactly the old
// single-sequence output (m=0, full control data, one APC block).
func chunkKittyPayload(control, payload string) string {
	if len(payload) <= kittyChunkSize {
		return fmt.Sprintf("\x1b_G%s,m=0;%s\x1b\\", control, payload)
	}
	var sb strings.Builder
	for i := 0; i < len(payload); i += kittyChunkSize {
		end := min(i+kittyChunkSize, len(payload))
		last := end == len(payload)
		m := 1
		if last {
			m = 0
		}
		if i == 0 {
			fmt.Fprintf(&sb, "\x1b_G%s,m=%d;%s\x1b\\", control, m, payload[i:end])
		} else {
			fmt.Fprintf(&sb, "\x1b_Gm=%d,q=2;%s\x1b\\", m, payload[i:end])
		}
	}
	return sb.String()
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
