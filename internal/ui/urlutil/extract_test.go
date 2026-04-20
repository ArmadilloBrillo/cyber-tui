package urlutil

import (
	"testing"
)

func TestExtractURLs_Empty(t *testing.T) {
	if got := ExtractURLs(""); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
	if got := ExtractURLs("   "); got != nil {
		t.Errorf("expected nil for whitespace, got %v", got)
	}
}

func TestExtractURLs_NoLinks(t *testing.T) {
	got := ExtractURLs("Hello world, no links here.")
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestExtractURLs_MarkdownLink(t *testing.T) {
	got := ExtractURLs("[click here](https://example.com)")
	if len(got) != 1 || got[0] != "https://example.com" {
		t.Errorf("expected [https://example.com], got %v", got)
	}
}

func TestExtractURLs_MarkdownImage(t *testing.T) {
	got := ExtractURLs("![alt text](https://img.example.com/photo.jpg)")
	if len(got) != 1 || got[0] != "https://img.example.com/photo.jpg" {
		t.Errorf("expected image URL, got %v", got)
	}
}

func TestExtractURLs_AutoLink(t *testing.T) {
	got := ExtractURLs("<https://example.com>")
	if len(got) != 1 || got[0] != "https://example.com" {
		t.Errorf("expected [https://example.com], got %v", got)
	}
}

func TestExtractURLs_Multiple(t *testing.T) {
	content := "[a](https://one.com) and ![img](https://two.com/img.jpg)"
	got := ExtractURLs(content)
	if len(got) != 2 {
		t.Fatalf("expected 2 URLs, got %v", got)
	}
}

func TestExtractURLs_Deduplication(t *testing.T) {
	content := "[a](https://example.com) [b](https://example.com)"
	got := ExtractURLs(content)
	if len(got) != 1 {
		t.Errorf("expected deduplication to 1 URL, got %v", got)
	}
}

func TestNormalizeURL_RelativePath(t *testing.T) {
	got := NormalizeURL("/support")
	want := "https://cyberspace.online/support"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNormalizeURL_AbsolutePassthrough(t *testing.T) {
	u := "https://example.com/page"
	if got := NormalizeURL(u); got != u {
		t.Errorf("got %q, want %q", got, u)
	}
}

func TestNormalizeURL_CyberspaceAbsolute(t *testing.T) {
	u := "https://cyberspace.online/about"
	if got := NormalizeURL(u); got != u {
		t.Errorf("got %q, want %q", got, u)
	}
}
