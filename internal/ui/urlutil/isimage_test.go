package urlutil_test

import (
	"testing"

	"github.com/ragnar/cyber-tui/internal/ui/urlutil"
)

func TestIsImageURL(t *testing.T) {
	t.Helper()
	cases := []struct {
		url  string
		want bool
	}{
		{"https://example.com/photo.jpg", true},
		{"https://example.com/photo.JPEG", true},
		{"https://example.com/logo.PNG", true},
		{"https://example.com/anim.gif", true},
		{"https://example.com/image.webp", true},
		{"https://example.com/image.avif", true},
		{"https://example.com/image.bmp", true},
		{"https://example.com/icon.svg", true},
		{"https://example.com/post/123", false},
		{"https://example.com/document.pdf", false},
		{"https://example.com/", false},
		{"not a url", false},
	}
	for _, c := range cases {
		got := urlutil.IsImageURL(c.url)
		if got != c.want {
			t.Errorf("IsImageURL(%q) = %v, want %v", c.url, got, c.want)
		}
	}
}

func TestIsGIFURL(t *testing.T) {
	t.Helper()
	cases := []struct {
		url  string
		want bool
	}{
		{"https://example.com/anim.gif", true},
		{"https://example.com/anim.GIF", true},
		{"https://example.com/photo.jpg", false},
		{"https://example.com/post/123", false},
		{"https://example.com/", false},
		{"not a url", false},
	}
	for _, c := range cases {
		got := urlutil.IsGIFURL(c.url)
		if got != c.want {
			t.Errorf("IsGIFURL(%q) = %v, want %v", c.url, got, c.want)
		}
	}
}
