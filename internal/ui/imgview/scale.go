package imgview

import (
	"image"

	"golang.org/x/image/draw"
)

// downscaleToBox resamples img to fit a cols x rows display box in real
// pixels (cols*cellPxW x rows*cellPxH), preserving img unchanged if it's
// already that size. When allowUpscale is false (the default for
// inline-thumbnail callers), img is also left unchanged if it's already
// smaller than that box — never upscaled. Kitty/iTerm2 have a terminal-side
// scale-to-fit (their c=/r= and width=/height= parameters), so skipping this
// and sending the full source resolution still displays correctly, but
// wastes bandwidth transmitting far more pixel data than a small thumbnail
// slot will ever show — e.g. a 365x512 source shown in a 6-row inline band
// produced a 521KB single-line escape sequence before this existed. Sixel
// has no such terminal-side scaling at all, so it always needed this step;
// EncodeITerm2/EncodeKitty now use the same helper. allowUpscale is true
// only for the fullscreen modal (see EncodeKitty/EncodeITerm2/EncodeSixel),
// which lets a user explicitly zoom a small image in past its native
// resolution — accepting the resulting blur, unlike a thumbnail.
func downscaleToBox(img image.Image, cols, rows, cellPxW, cellPxH int, allowUpscale bool) image.Image {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	targetW, targetH := cols*cellPxW, rows*cellPxH
	if targetW == w && targetH == h {
		return img
	}
	if !allowUpscale && targetW >= w && targetH >= h {
		return img
	}
	dst := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, bounds, draw.Src, nil)
	return dst
}

// pxPerCol is the assumed terminal column width in pixels, used as a default
// when the real cell size isn't known. Conservative across common terminal
// fonts and DPI settings. Cell height is assumed to be 2×pxPerCol (a 2:1
// height:width cell aspect ratio).
const pxPerCol = 10

// EffectiveCellPx returns cellPxW/cellPxH unchanged, substituting the
// assumed default cell size (pxPerCol x 2*pxPerCol) when the real size is
// unavailable (either <= 0 — the shared fallback used by EncodeKitty,
// EncodeITerm2, EncodeSixel, and by app.go's own native-size math ahead of
// encoding).
func EffectiveCellPx(cellPxW, cellPxH int) (int, int) {
	if cellPxW <= 0 || cellPxH <= 0 {
		return pxPerCol, 2 * pxPerCol
	}
	return cellPxW, cellPxH
}

// NativeCellBox returns the terminal cell box (cols, rows) needed to display
// an image of imgWidth x imgHeight pixels at 1:1 native resolution, given a
// cellPxW x cellPxH terminal cell size (run through EffectiveCellPx first,
// so a <= 0 cell size falls back the same way encoding does). Each axis is
// ceiling-divided independently — this is the image's own natural size, not
// an aspect-fit into some other box. Always at least 1x1.
func NativeCellBox(imgWidth, imgHeight, cellPxW, cellPxH int) (cols, rows int) {
	cellPxW, cellPxH = EffectiveCellPx(cellPxW, cellPxH)
	cols = (imgWidth + cellPxW - 1) / cellPxW
	rows = (imgHeight + cellPxH - 1) / cellPxH
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	return cols, rows
}

// fitCols returns the number of terminal columns to display an image of
// imgWidth pixels at, capped at maxCols, given a terminal column width of
// cellW pixels. When allowUpscale is false, this never exceeds the image's
// own natural column count (never upscales); when true, it always returns
// maxCols (the caller's target), letting downscaleToBox resize up to fill
// it. The result is always at least 1.
func fitCols(imgWidth, maxCols, cellW int, allowUpscale bool) int {
	if allowUpscale {
		if maxCols < 1 {
			maxCols = 1
		}
		return maxCols
	}
	natural := (imgWidth + cellW - 1) / cellW
	if natural < 1 {
		natural = 1
	}
	if maxCols > 0 && natural > maxCols {
		return maxCols
	}
	return natural
}

// fitRows estimates the number of terminal rows the image will occupy when
// displayed at cols columns, given a terminal cell size of cellW x cellH
// pixels. The result is always at least 1.
func fitRows(imgHeight, imgWidth, cols, cellW, cellH int) int {
	if imgWidth <= 0 || cols <= 0 {
		return 1
	}
	// rows = imgHeight / cellH, where displayWidth in px = cols * cellW
	// and actual displayHeight = imgHeight * (cols*cellW) / imgWidth
	rows := imgHeight * cols * cellW / (imgWidth * cellH)
	if rows < 1 {
		return 1
	}
	return rows
}

// fitBox computes the terminal cols/rows to display an image of imgWidth x
// imgHeight pixels within a maxCols x maxRows box, preserving aspect ratio,
// given a terminal cell size of cellW x cellH pixels. allowUpscale controls
// whether the result may exceed the image's natural size (see fitCols) — the
// row-rebalance step below (recomputing cols from rows when the row bound
// binds) stays aspect-preserving regardless of upscale direction. If fitting
// to maxCols would make rows exceed maxRows, cols is recomputed from the row
// constraint instead so both bounds hold.
func fitBox(imgWidth, imgHeight, maxCols, maxRows, cellW, cellH int, allowUpscale bool) (cols, rows int) {
	cols = fitCols(imgWidth, maxCols, cellW, allowUpscale)
	rows = fitRows(imgHeight, imgWidth, cols, cellW, cellH)
	if maxRows > 0 && rows > maxRows && imgHeight > 0 {
		rows = maxRows
		cols = cellH * rows * imgWidth / (cellW * imgHeight)
		if cols < 1 {
			cols = 1
		}
		if maxCols > 0 && cols > maxCols {
			cols = maxCols
		}
	}
	return cols, rows
}
