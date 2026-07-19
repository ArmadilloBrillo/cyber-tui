package imgview

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"time"

	_ "golang.org/x/image/webp"
)

// maxImageBytes caps how much of an image response body is read into memory,
// matching the 10 MiB response cap used by the API and RTDB clients.
const maxImageBytes = 10 << 20

// maxImagePixels bounds the declared width×height before decoding, so a small
// file claiming huge dimensions cannot drive allocation during decode.
const maxImagePixels = 50 << 20 // ~52 megapixels

// httpClient is dedicated to image fetching so this path never inherits
// global mutations to http.DefaultClient.
var httpClient = &http.Client{Timeout: 30 * time.Second}

// Fetch downloads the image at rawURL and decodes it. Supports JPEG, PNG, GIF, and WebP.
func Fetch(ctx context.Context, rawURL string) (image.Image, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("imgview: %w", err)
	}
	req.Header.Set("Accept", "image/webp,image/png,image/jpeg,image/gif,image/*,*/*;q=0.8")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("imgview: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("imgview: server returned %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
	if err != nil {
		return nil, fmt.Errorf("imgview: read body: %w", err)
	}
	if len(body) > maxImageBytes {
		return nil, fmt.Errorf("imgview: image exceeds %d MiB size limit", maxImageBytes>>20)
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
