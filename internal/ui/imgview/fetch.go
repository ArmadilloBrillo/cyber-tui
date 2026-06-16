package imgview

import (
	"context"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/http"

	_ "golang.org/x/image/webp"
)

// Fetch downloads the image at rawURL and decodes it. Supports JPEG, PNG, GIF, and WebP.
func Fetch(ctx context.Context, rawURL string) (image.Image, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("imgview: %w", err)
	}
	req.Header.Set("Accept", "image/webp,image/png,image/jpeg,image/gif,image/*,*/*;q=0.8")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("imgview: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("imgview: server returned %s", resp.Status)
	}
	img, _, err := image.Decode(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("imgview: decode: %w", err)
	}
	return img, nil
}
