package imgview

import (
	"image"
	"image/color"
	"math"
	"strconv"
)

// DitherOptions configures Dither. PixelSize is the pixelation block size in
// source pixels (see PixelSizeForSharpness). FgColor/BgColor are the two
// colors the dithered result is recolored through.
type DitherOptions struct {
	PixelSize int
	FgColor   color.RGBA
	BgColor   color.RGBA
}

// ditherAmount blends a flat 0.5 threshold (0.0) against the full Bayer
// ordered-dither pattern (1.0). Fixed rather than user-configurable —
// PixelSize alone differentiates the exposed sharpness levels.
const ditherAmount = 0.75

// ditherLevels is 2^bitDepth with bitDepth fixed at 2, matching the source
// shader's default. Not exposed as a setting.
const ditherLevels = 4.0

// bayer8x8 is the classic 8x8 ordered-dither threshold matrix, normalized to
// 0..1, row-major (index = y*8+x). Ported from cyberspace.online webui's
// RasterImage shader.
var bayer8x8 = [64]float64{
	0 / 64.0, 32 / 64.0, 8 / 64.0, 40 / 64.0, 2 / 64.0, 34 / 64.0, 10 / 64.0, 42 / 64.0,
	48 / 64.0, 16 / 64.0, 56 / 64.0, 24 / 64.0, 50 / 64.0, 18 / 64.0, 58 / 64.0, 26 / 64.0,
	12 / 64.0, 44 / 64.0, 4 / 64.0, 36 / 64.0, 14 / 64.0, 46 / 64.0, 6 / 64.0, 38 / 64.0,
	60 / 64.0, 28 / 64.0, 52 / 64.0, 20 / 64.0, 62 / 64.0, 30 / 64.0, 54 / 64.0, 22 / 64.0,
	3 / 64.0, 35 / 64.0, 11 / 64.0, 43 / 64.0, 1 / 64.0, 33 / 64.0, 9 / 64.0, 41 / 64.0,
	51 / 64.0, 19 / 64.0, 59 / 64.0, 27 / 64.0, 49 / 64.0, 17 / 64.0, 57 / 64.0, 25 / 64.0,
	15 / 64.0, 47 / 64.0, 7 / 64.0, 39 / 64.0, 13 / 64.0, 45 / 64.0, 5 / 64.0, 37 / 64.0,
	63 / 64.0, 31 / 64.0, 55 / 64.0, 23 / 64.0, 61 / 64.0, 29 / 64.0, 53 / 64.0, 21 / 64.0,
}

// Dither applies cyberspace.online webui's RasterImage duotone-dithering
// effect: block-pixelate, convert to grayscale, threshold against an 8x8
// Bayer ordered-dither pattern blended with posterization, then recolor the
// result through opts.FgColor/opts.BgColor. Always returns a new image; img
// is never mutated.
func Dither(img image.Image, opts DitherOptions) *image.RGBA {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	out := image.NewRGBA(image.Rect(0, 0, w, h))

	pixelSize := opts.PixelSize
	if pixelSize < 1 {
		pixelSize = 1
	}

	fgR, fgG, fgB := float64(opts.FgColor.R), float64(opts.FgColor.G), float64(opts.FgColor.B)
	bgR, bgG, bgB := float64(opts.BgColor.R), float64(opts.BgColor.G), float64(opts.BgColor.B)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			blockX := (x / pixelSize) * pixelSize
			blockY := (y / pixelSize) * pixelSize
			// ponytail: grayscale is computed from RGBA()'s alpha-premultiplied
			// values rather than straight RGB, so partially transparent source
			// pixels read slightly darker than the shader (which reads straight
			// texture RGB). Only visible on non-opaque source pixels; upgrade to
			// per-channel unpremultiply if that ever matters for this feature.
			r, g, b, _ := img.At(bounds.Min.X+blockX, bounds.Min.Y+blockY).RGBA()
			gray := (0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)) / 65535.0

			bayerValue := bayer8x8[(y%8)*8+(x%8)]
			noise := pseudoNoise(x, y) * 0.1
			bayerValue = bayerValue*0.7 + noise*0.3
			threshold := 0.5*(1-ditherAmount) + bayerValue*ditherAmount

			dithered := 0.0
			if gray >= threshold {
				dithered = 1.0
			}
			quantized := math.Floor(gray*ditherLevels) / ditherLevels
			final := quantized*(1-ditherAmount) + dithered*ditherAmount

			out.SetRGBA(x, y, color.RGBA{
				R: uint8(math.Round(bgR + (fgR-bgR)*final)),
				G: uint8(math.Round(bgG + (fgG-bgG)*final)),
				B: uint8(math.Round(bgB + (fgB-bgB)*final)),
				A: 255,
			})
		}
	}
	return out
}

// pseudoNoise reproduces the source shader's cheap GLSL hash-based random(),
// called as random(vec2(x,y)*0.01) with dot product weights (12.9898,78.233)
// and scale 43758.5453, taking the fractional part of the result.
func pseudoNoise(x, y int) float64 {
	v := math.Sin(float64(x)*0.01*12.9898+float64(y)*0.01*78.233) * 43758.5453
	return v - math.Floor(v)
}

// PixelSizeForSharpness maps a dithering sharpness setting to a pixelation
// block size in source pixels. Unrecognized or empty values fail closed to
// "medium", mirroring ProtocolFromName's convention.
func PixelSizeForSharpness(level string) int {
	switch level {
	case "rough":
		return 4
	case "sharp":
		return 2
	case "crisp":
		return 1
	default:
		return 3
	}
}

// ParseHexColor parses a "#RRGGBB" hex color string.
func ParseHexColor(s string) (color.RGBA, bool) {
	if len(s) != 7 || s[0] != '#' {
		return color.RGBA{}, false
	}
	v, err := strconv.ParseUint(s[1:], 16, 32)
	if err != nil {
		return color.RGBA{}, false
	}
	return color.RGBA{
		R: uint8(v >> 16),
		G: uint8(v >> 8),
		B: uint8(v),
		A: 255,
	}, true
}
