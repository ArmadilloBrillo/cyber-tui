package urlutil

import (
	"net/url"
	"path"
	"strings"
)

// imageExts is the set of file extensions that indicate an image URL.
var imageExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true,
	".gif": true, ".webp": true, ".avif": true,
	".bmp": true, ".svg": true,
}

// IsImageURL reports whether the URL's path ends with a recognised image extension.
func IsImageURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	ext := strings.ToLower(path.Ext(u.Path))
	return imageExts[ext]
}

// IsGIFURL reports whether the URL's path ends with a .gif extension.
func IsGIFURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return strings.ToLower(path.Ext(u.Path)) == ".gif"
}
