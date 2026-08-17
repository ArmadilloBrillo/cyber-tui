package imgview_test

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"math/rand"
	"strings"
	"testing"

	"github.com/ragnar/cyber-tui/internal/ui/imgview"
)

func TestEncodeKitty_ContainsAPC(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	seq, cols, rows, err := imgview.EncodeKitty(img, 40, 20, 0, 0, 0, false, nil)
	if err != nil {
		t.Fatalf("EncodeKitty: %v", err)
	}
	if !strings.HasPrefix(seq, "\x1b_G") {
		t.Errorf("EncodeKitty: missing APC prefix, got %q", seq[:min(len(seq), 20)])
	}
	if !strings.HasSuffix(seq, "\x1b\\") {
		t.Errorf("EncodeKitty: missing ST suffix")
	}
	if cols < 1 {
		t.Errorf("EncodeKitty: cols=%d, want >= 1", cols)
	}
	if rows < 1 {
		t.Errorf("EncodeKitty: rows=%d, want >= 1", rows)
	}
}

func TestEncodeKitty_CapsRows(t *testing.T) {
	// Tall portrait image: without a row cap this would need 50 rows at 10 cols.
	img := image.NewRGBA(image.Rect(0, 0, 100, 1000))
	_, cols, rows, err := imgview.EncodeKitty(img, 40, 20, 0, 0, 0, false, nil)
	if err != nil {
		t.Fatalf("EncodeKitty: %v", err)
	}
	if rows > 20 {
		t.Errorf("EncodeKitty: rows=%d, want <= maxRows(20)", rows)
	}
	if cols < 1 || cols > 40 {
		t.Errorf("EncodeKitty: cols=%d, want in [1, maxCols(40)]", cols)
	}
}

func TestEncodeKitty_PrependsDeleteAllPlacements(t *testing.T) {
	// placementID==0 (anonymous mode, used by the fullscreen modal) always
	// leads with a delete-all-placements command so opening a new image
	// self-heals a leftover placement from a previous image whose own
	// close-cleanup frame never reached the terminal (e.g. dropped behind a
	// slow flush of a large prior image).
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	seq, _, _, err := imgview.EncodeKitty(img, 40, 20, 0, 0, 0, false, nil)
	if err != nil {
		t.Fatalf("EncodeKitty: %v", err)
	}
	if !strings.HasPrefix(seq, "\x1b_Ga=d,d=A\x1b\\") {
		t.Errorf("EncodeKitty: expected delete-all-placements prefix, got %q", seq[:min(len(seq), 30)])
	}

	// Back-to-back calls (as would happen opening several images in a row)
	// must each still lead with the delete-all command.
	seq2, _, _, err := imgview.EncodeKitty(img, 40, 20, 0, 0, 0, false, nil)
	if err != nil {
		t.Fatalf("EncodeKitty: %v", err)
	}
	if !strings.HasPrefix(seq2, "\x1b_Ga=d,d=A\x1b\\") {
		t.Errorf("EncodeKitty: expected delete-all-placements prefix on repeated call, got %q", seq2[:min(len(seq2), 30)])
	}
}

// TestEncodeKitty_NamedPlacement_NeverBluntDeletes confirms that a non-zero
// placementID (inline rendering, where several images are on screen at
// once) never emits the blunt a=d,d=A delete-all — that would erase every
// other inline image's placement, not just this one — and instead embeds
// the id as both the image id and placement id so DeleteKittyPlacement can
// target it precisely later.
func TestEncodeKitty_NamedPlacement_NeverBluntDeletes(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	seq, _, _, err := imgview.EncodeKitty(img, 40, 20, 0, 0, 7, false, nil)
	if err != nil {
		t.Fatalf("EncodeKitty: %v", err)
	}
	if strings.Contains(seq, "d=A") {
		t.Errorf("EncodeKitty with a named placement must never blunt-delete-all, got %q", seq)
	}
	if !strings.Contains(seq, "i=7") || !strings.Contains(seq, "p=7") {
		t.Errorf("expected i=7 and p=7 in the encoded sequence, got %q", seq)
	}
}

// TestEncodeKitty_SuppressesTerminalResponse confirms every EncodeKitty mode
// sets q=2, suppressing the terminal's OK/error acknowledgment. Without it,
// the terminal writes that acknowledgment back over the same stream used for
// keyboard input, which Bubble Tea's input reader can't distinguish from a
// real keystroke — and the fullscreen modal treats any keypress while open
// as "close", so an unsuppressed response closed the modal the instant it
// opened.
func TestEncodeKitty_SuppressesTerminalResponse(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))

	seqAnon, _, _, err := imgview.EncodeKitty(img, 40, 20, 0, 0, 0, false, nil)
	if err != nil {
		t.Fatalf("EncodeKitty: %v", err)
	}
	if !strings.Contains(seqAnon, "q=2") {
		t.Errorf("anonymous-mode EncodeKitty must set q=2, got %q", seqAnon)
	}

	seqNamed, _, _, err := imgview.EncodeKitty(img, 40, 20, 0, 0, 7, false, nil)
	if err != nil {
		t.Fatalf("EncodeKitty: %v", err)
	}
	if !strings.Contains(seqNamed, "q=2") {
		t.Errorf("named-placement EncodeKitty must set q=2, got %q", seqNamed)
	}
}

// TestEncodeKitty_UsesPNGFormat is a regression test for switching from raw
// f=32 RGBA (uncompressed, the largest payload shape available) to f=100
// PNG: confirms the format flag says PNG, no s=/v= pixel-dimension keys are
// sent (not needed for PNG — the terminal reads them from the PNG header
// itself), and the base64 payload actually decodes to valid PNG data rather
// than raw pixels.
func TestEncodeKitty_UsesPNGFormat(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(1, 1, color.RGBA{B: 255, A: 255})
	seq, _, _, err := imgview.EncodeKitty(img, 40, 20, 0, 0, 0, false, nil)
	if err != nil {
		t.Fatalf("EncodeKitty: %v", err)
	}
	if !strings.Contains(seq, "f=100") {
		t.Errorf("expected f=100 (PNG), got %q", seq)
	}
	if strings.Contains(seq, "s=") || strings.Contains(seq, "v=") {
		t.Errorf("expected no s=/v= pixel-dimension keys for PNG payloads, got %q", seq)
	}
	// Extract the base64 payload (after the last ';', before the trailing ST)
	// and confirm it decodes to a PNG (magic bytes 0x89 'P' 'N' 'G').
	semi := strings.LastIndex(seq, ";")
	payload := strings.TrimSuffix(seq[semi+1:], "\x1b\\")
	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		t.Fatalf("payload did not decode as base64: %v", err)
	}
	if len(raw) < 8 || !bytes.HasPrefix(raw, []byte("\x89PNG\r\n\x1a\n")) {
		t.Errorf("expected the payload to be a PNG (magic bytes), got %d bytes starting %x", len(raw), raw[:min(len(raw), 8)])
	}
}

// TestEncodeKitty_AppliesDither confirms a non-nil DitherOptions actually
// reaches the encoded image: the decoded PNG payload differs from the
// undithered encoding, and every pixel is limited to the two supplied
// colors (see imgview.Dither's own tests for the blend invariant details).
func TestEncodeKitty_AppliesDither(t *testing.T) {
	img := noisyImage(8, 8)
	plain, _, _, err := imgview.EncodeKitty(img, 40, 20, 0, 0, 0, false, nil)
	if err != nil {
		t.Fatalf("EncodeKitty (no dither): %v", err)
	}
	fg := color.RGBA{R: 0, G: 255, B: 65, A: 255}
	bg := color.RGBA{R: 13, G: 13, B: 13, A: 255}
	dithered, _, _, err := imgview.EncodeKitty(img, 40, 20, 0, 0, 0, false, &imgview.DitherOptions{PixelSize: 1, FgColor: fg, BgColor: bg})
	if err != nil {
		t.Fatalf("EncodeKitty (dithered): %v", err)
	}
	if dithered == plain {
		t.Fatal("EncodeKitty: dithered output identical to undithered output")
	}
	decoded := decodeKittyPNGPayload(t, dithered)
	bounds := decoded.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := decoded.At(x, y).RGBA()
			c := color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
			if !channelBetween(c.R, fg.R, bg.R) || !channelBetween(c.G, fg.G, bg.G) || !channelBetween(c.B, fg.B, bg.B) {
				t.Fatalf("pixel (%d,%d) = %+v, want a blend between fg %+v and bg %+v", x, y, c, fg, bg)
			}
		}
	}
}

func channelBetween(v, a, b uint8) bool {
	if a > b {
		a, b = b, a
	}
	return v >= a && v <= b
}

// decodeKittyPNGPayload extracts and decodes the PNG payload from a Kitty
// APC escape sequence produced by EncodeKitty.
func decodeKittyPNGPayload(t *testing.T, seq string) image.Image {
	t.Helper()
	semi := strings.LastIndex(seq, ";")
	payload := strings.TrimSuffix(seq[semi+1:], "\x1b\\")
	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		t.Fatalf("payload did not decode as base64: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("payload did not decode as PNG: %v", err)
	}
	return img
}

// noisyImage returns a deterministic pseudo-random RGBA image — PNG
// compresses a uniform test image far below the Kitty protocol's 4096-byte
// chunk size, so chunking tests need incompressible pixel data to actually
// force multiple chunks.
func noisyImage(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	rng := rand.New(rand.NewSource(1))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{
				R: byte(rng.Intn(256)), G: byte(rng.Intn(256)),
				B: byte(rng.Intn(256)), A: 255,
			})
		}
	}
	return img
}

// TestEncodeKitty_ChunksLargePayloads is a regression test for WezTerm on
// Windows failing to render images sent as one giant unchunked APC sequence
// (suspected ConPTY interference) — EncodeKitty must split a payload over
// the protocol's 4096-byte chunk size into multiple APC sequences, each
// wrapped in its own ESC_G...ESC\, m=1 on every chunk but the last (m=0),
// and continuation chunks must carry no control keys besides m/q.
func TestEncodeKitty_ChunksLargePayloads(t *testing.T) {
	img := noisyImage(200, 200)
	seq, _, _, err := imgview.EncodeKitty(img, 40, 20, 0, 0, 7, false, nil)
	if err != nil {
		t.Fatalf("EncodeKitty: %v", err)
	}
	chunks := strings.Split(strings.TrimSuffix(seq, "\x1b\\"), "\x1b\\\x1b_G")
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks for a large payload, got %d (seq len %d)", len(chunks), len(seq))
	}
	for i, c := range chunks {
		last := i == len(chunks)-1
		wantM := "m=1"
		if last {
			wantM = "m=0"
		}
		if !strings.Contains(c, wantM) {
			t.Errorf("chunk %d: expected %s, got control data %q", i, wantM, c[:min(len(c), 40)])
		}
		if i > 0 {
			for _, key := range []string{"a=", "f=", "c=", "r=", "i=", "p="} {
				if strings.Contains(c, key) {
					t.Errorf("chunk %d: continuation chunk must not repeat control key %q, got %q", i, key, c[:min(len(c), 40)])
				}
			}
		}
	}
}

// TestEncodeITerm2_DownscalesLargeSourceToTargetBox and its Kitty
// counterpart below are regression tests for a real bug: EncodeITerm2/
// EncodeKitty used to PNG-encode the full source resolution regardless of
// how small the target display box was, relying on the terminal's own
// scale-to-fit to shrink it visually — wasteful for a small inline
// thumbnail slot (observed: a 365x512 source produced a 521KB payload for
// a 6-row inline band). EncodeSixel already downscaled in pixel space
// first (no terminal-side scale-to-fit to rely on); this confirms
// EncodeITerm2/EncodeKitty now do the same via the shared downscaleToBox
// helper — a large noisy (incompressible) source encoded for a small
// target box should be dramatically smaller than the same source encoded
// for a target box close to its own size.
func TestEncodeITerm2_DownscalesLargeSourceToTargetBox(t *testing.T) {
	img := noisyImage(400, 400)
	small, _, _, err := imgview.EncodeITerm2(img, 4, 2, 10, 20, false, nil)
	if err != nil {
		t.Fatalf("EncodeITerm2 (small box): %v", err)
	}
	large, _, _, err := imgview.EncodeITerm2(img, 200, 200, 10, 20, false, nil)
	if err != nil {
		t.Fatalf("EncodeITerm2 (large box): %v", err)
	}
	if len(small) >= len(large)/4 {
		t.Errorf("EncodeITerm2: small-box payload (%d bytes) not meaningfully smaller than large-box payload (%d bytes) — downscaling doesn't appear to be engaging", len(small), len(large))
	}
}

func TestEncodeKitty_DownscalesLargeSourceToTargetBox(t *testing.T) {
	img := noisyImage(400, 400)
	small, _, _, err := imgview.EncodeKitty(img, 4, 2, 10, 20, 0, false, nil)
	if err != nil {
		t.Fatalf("EncodeKitty (small box): %v", err)
	}
	large, _, _, err := imgview.EncodeKitty(img, 200, 200, 10, 20, 0, false, nil)
	if err != nil {
		t.Fatalf("EncodeKitty (large box): %v", err)
	}
	if len(small) >= len(large)/4 {
		t.Errorf("EncodeKitty: small-box payload (%d bytes) not meaningfully smaller than large-box payload (%d bytes) — downscaling doesn't appear to be engaging", len(small), len(large))
	}
}

func TestDeleteKittyPlacement_TargetsExactIDPair(t *testing.T) {
	seq := imgview.DeleteKittyPlacement(7)
	if !strings.HasPrefix(seq, "\x1b_G") || !strings.HasSuffix(seq, "\x1b\\") {
		t.Errorf("expected a well-formed APC sequence, got %q", seq)
	}
	if !strings.Contains(seq, "d=i") || !strings.Contains(seq, "i=7") || !strings.Contains(seq, "p=7") {
		t.Errorf("expected a targeted delete (d=i,i=7,p=7), got %q", seq)
	}
	if strings.Contains(seq, "d=A") {
		t.Errorf("expected a targeted delete, not delete-all, got %q", seq)
	}
}

func TestEncodeITerm2_ContainsOSC(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{G: 255, A: 255})
	seq, cols, rows, err := imgview.EncodeITerm2(img, 80, 40, 0, 0, false, nil)
	if err != nil {
		t.Fatalf("EncodeITerm2: %v", err)
	}
	if !strings.HasPrefix(seq, "\x1b]1337;") {
		t.Errorf("EncodeITerm2: missing OSC 1337 prefix, got %q", seq[:min(len(seq), 20)])
	}
	if !strings.HasSuffix(seq, "\x07") {
		t.Errorf("EncodeITerm2: missing BEL terminator")
	}
	if cols < 1 {
		t.Errorf("EncodeITerm2: cols=%d, want >= 1", cols)
	}
	if rows < 1 {
		t.Errorf("EncodeITerm2: rows=%d, want >= 1", rows)
	}
}

// TestEncodeITerm2_AppliesDither mirrors TestEncodeKitty_AppliesDither.
func TestEncodeITerm2_AppliesDither(t *testing.T) {
	img := noisyImage(8, 8)
	plain, _, _, err := imgview.EncodeITerm2(img, 80, 40, 0, 0, false, nil)
	if err != nil {
		t.Fatalf("EncodeITerm2 (no dither): %v", err)
	}
	fg := color.RGBA{R: 0, G: 255, B: 65, A: 255}
	bg := color.RGBA{R: 13, G: 13, B: 13, A: 255}
	dithered, _, _, err := imgview.EncodeITerm2(img, 80, 40, 0, 0, false, &imgview.DitherOptions{PixelSize: 1, FgColor: fg, BgColor: bg})
	if err != nil {
		t.Fatalf("EncodeITerm2 (dithered): %v", err)
	}
	if dithered == plain {
		t.Fatal("EncodeITerm2: dithered output identical to undithered output")
	}
	colon := strings.LastIndex(dithered, ":")
	payload := strings.TrimSuffix(dithered[colon+1:], "\x07")
	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		t.Fatalf("payload did not decode as base64: %v", err)
	}
	decoded, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("payload did not decode as PNG: %v", err)
	}
	bounds := decoded.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := decoded.At(x, y).RGBA()
			c := color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8)}
			if !channelBetween(c.R, fg.R, bg.R) || !channelBetween(c.G, fg.G, bg.G) || !channelBetween(c.B, fg.B, bg.B) {
				t.Fatalf("pixel (%d,%d) = %+v, want a blend between fg %+v and bg %+v", x, y, c, fg, bg)
			}
		}
	}
}

// TestEncodeITerm2_SetsDoNotMoveCursor is a regression test: without
// doNotMoveCursor=1, WezTerm scrolls its whole screen once an inline image's
// footprint reaches the terminal's last line (wezterm/wezterm#3266), which
// desyncs every absolute-cursor-positioned draw injectInlineImages issues
// afterward. The app never relies on the terminal's own post-image cursor
// advance, so this is safe to always send.
func TestEncodeITerm2_SetsDoNotMoveCursor(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	seq, _, _, err := imgview.EncodeITerm2(img, 80, 40, 0, 0, false, nil)
	if err != nil {
		t.Fatalf("EncodeITerm2: %v", err)
	}
	if !strings.Contains(seq, "doNotMoveCursor=1") {
		t.Errorf("EncodeITerm2: expected doNotMoveCursor=1, got %q", seq)
	}
}

// TestEncodeITerm2_UsesRealCellSize mirrors TestEncodeSixel_UsesRealCellSize:
// a 4x larger real cell size should need fewer cols/rows than the default
// assumed cell size, confirming cellPxW/cellPxH actually drives the sizing
// math (and, unlike Kitty, directly determines how much blank space iTerm2's
// preserveAspectRatio letterboxing leaves — see EncodeITerm2's doc comment).
func TestEncodeITerm2_UsesRealCellSize(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 400, 400))
	_, defaultCols, defaultRows, err := imgview.EncodeITerm2(img, 200, 200, 0, 0, false, nil)
	if err != nil {
		t.Fatalf("EncodeITerm2 (default cell size): %v", err)
	}
	_, largeCellCols, largeCellRows, err := imgview.EncodeITerm2(img, 200, 200, 40, 80, false, nil)
	if err != nil {
		t.Fatalf("EncodeITerm2 (large cell size): %v", err)
	}
	if largeCellCols >= defaultCols {
		t.Errorf("EncodeITerm2: cols with a 4x larger cell size = %d, want < default cols = %d", largeCellCols, defaultCols)
	}
	if largeCellRows >= defaultRows {
		t.Errorf("EncodeITerm2: rows with a 4x larger cell size = %d, want < default rows = %d", largeCellRows, defaultRows)
	}
}

// TestEncodeKitty_UsesRealCellSize mirrors TestEncodeSixel_UsesRealCellSize.
func TestEncodeKitty_UsesRealCellSize(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 400, 400))
	_, defaultCols, defaultRows, err := imgview.EncodeKitty(img, 200, 200, 0, 0, 0, false, nil)
	if err != nil {
		t.Fatalf("EncodeKitty (default cell size): %v", err)
	}
	_, largeCellCols, largeCellRows, err := imgview.EncodeKitty(img, 200, 200, 40, 80, 0, false, nil)
	if err != nil {
		t.Fatalf("EncodeKitty (large cell size): %v", err)
	}
	if largeCellCols >= defaultCols {
		t.Errorf("EncodeKitty: cols with a 4x larger cell size = %d, want < default cols = %d", largeCellCols, defaultCols)
	}
	if largeCellRows >= defaultRows {
		t.Errorf("EncodeKitty: rows with a 4x larger cell size = %d, want < default rows = %d", largeCellRows, defaultRows)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
