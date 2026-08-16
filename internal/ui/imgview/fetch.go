package imgview

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"time"

	_ "golang.org/x/image/webp"

	"github.com/ragnar/cyber-tui/internal/version"
)

// userAgent identifies cyber-tui to image hosts. Some (Wikimedia's edge,
// confirmed live) return 403 for Go's default "Go-http-client/1.1" — an
// identifiable UA with a contact URL is what their bot policy actually asks
// for, and fixes the block.
var userAgent = "cyber-tui/" + version.Version + " (+https://github.com/ArmadilloBrillo/cyber-tui)"

// maxImageBytes caps how much of an image response body is read into memory,
// matching the 10 MiB response cap used by the API and RTDB clients.
const maxImageBytes = 10 << 20

// maxImagePixels bounds the declared width×height before decoding, so a small
// file claiming huge dimensions cannot drive allocation during decode.
const maxImagePixels = 50 << 20 // ~52 megapixels

// maxGIFFrames bounds the number of frames a GIF may declare, guarding
// against a small file claiming an enormous frame count.
const maxGIFFrames = 512

// maxGIFTotalPixels bounds frames×width×height combined, since every frame
// is later composited into its own full-canvas RGBA image held in memory
// simultaneously (see GIFFrames) — maxImagePixels alone only bounds a single
// canvas, not the multiplied-out worst case across all frames.
const maxGIFTotalPixels = 64 << 20

// httpClient is dedicated to image fetching so this path never inherits
// global mutations to http.DefaultClient.
var httpClient = &http.Client{Timeout: 30 * time.Second}

// openImageResponse issues the GET request shared by fetchBody and
// Dimensions. The caller must close the returned response's body.
func openImageResponse(ctx context.Context, rawURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("imgview: %w", err)
	}
	req.Header.Set("Accept", "image/webp,image/png,image/jpeg,image/gif,image/*,*/*;q=0.8")
	req.Header.Set("User-Agent", userAgent)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("imgview: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("imgview: server returned %s", resp.Status)
	}
	return resp, nil
}

// fetchBody downloads rawURL, capping the read at maxImageBytes.
func fetchBody(ctx context.Context, rawURL string) ([]byte, error) {
	resp, err := openImageResponse(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
	if err != nil {
		return nil, fmt.Errorf("imgview: read body: %w", err)
	}
	if len(body) > maxImageBytes {
		return nil, fmt.Errorf("imgview: image exceeds %d MiB size limit", maxImageBytes>>20)
	}
	return body, nil
}

// Fetch downloads the image at rawURL and decodes it. Supports JPEG, PNG, GIF, and WebP.
// GIFs are decoded to their first frame only — use FetchGIF for all frames.
func Fetch(ctx context.Context, rawURL string) (image.Image, error) {
	body, err := fetchBody(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("imgview: decode: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > maxImagePixels {
		return nil, fmt.Errorf("imgview: image dimensions %dx%d exceed limit", cfg.Width, cfg.Height)
	}
	img, _, err := image.Decode(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("imgview: decode: %w", err)
	}
	return img, nil
}

// Dimensions fetches rawURL and returns its declared width/height. Unlike
// Fetch, it reads only as far as image.DecodeConfig needs (the format
// header, typically well under 1KiB) rather than downloading the whole
// body — used where a caller needs an image's size but not its content
// (e.g. cyberspace.online's post-attachments API, which requires
// width/height on the request and does not compute them itself).
func Dimensions(ctx context.Context, rawURL string) (width, height int, err error) {
	resp, err := openImageResponse(ctx, rawURL)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()
	cfg, _, err := image.DecodeConfig(io.LimitReader(resp.Body, maxImageBytes))
	if err != nil {
		return 0, 0, fmt.Errorf("imgview: decode: %w", err)
	}
	return cfg.Width, cfg.Height, nil
}

// FetchGIF downloads the GIF at rawURL and decodes all of its frames.
func FetchGIF(ctx context.Context, rawURL string) (*gif.GIF, error) {
	body, err := fetchBody(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	cfg, err := gif.DecodeConfig(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("imgview: decode: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > maxImagePixels {
		return nil, fmt.Errorf("imgview: image dimensions %dx%d exceed limit", cfg.Width, cfg.Height)
	}
	g, err := gif.DecodeAll(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("imgview: decode: %w", err)
	}
	if len(g.Image) > maxGIFFrames {
		return nil, fmt.Errorf("imgview: gif has %d frames, exceeds %d frame limit", len(g.Image), maxGIFFrames)
	}
	if total := len(g.Image) * cfg.Width * cfg.Height; total > maxGIFTotalPixels {
		return nil, fmt.Errorf("imgview: gif frames×dimensions %d exceed %d pixel budget", total, maxGIFTotalPixels)
	}
	return g, nil
}
