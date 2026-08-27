package youtube

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExtractVideoID(t *testing.T) {
	tests := []struct {
		name   string
		rawURL string
		wantID string
		wantOK bool
	}{
		{"watch url", "https://www.youtube.com/watch?v=dQw4w9WgXcQ", "dQw4w9WgXcQ", true},
		{"watch url no www", "https://youtube.com/watch?v=dQw4w9WgXcQ", "dQw4w9WgXcQ", true},
		{"watch url with extra params", "https://www.youtube.com/watch?v=dQw4w9WgXcQ&t=30s", "dQw4w9WgXcQ", true},
		{"youtu.be short link", "https://youtu.be/dQw4w9WgXcQ", "dQw4w9WgXcQ", true},
		{"youtu.be trailing slash", "https://youtu.be/dQw4w9WgXcQ/", "dQw4w9WgXcQ", true},
		{"shorts", "https://www.youtube.com/shorts/dQw4w9WgXcQ", "dQw4w9WgXcQ", true},
		{"embed", "https://www.youtube.com/embed/dQw4w9WgXcQ", "dQw4w9WgXcQ", true},
		{"mobile host", "https://m.youtube.com/watch?v=dQw4w9WgXcQ", "dQw4w9WgXcQ", true},
		{"missing v param", "https://www.youtube.com/watch", "", false},
		{"non-youtube host", "https://vimeo.com/12345", "", false},
		{"not a url", "not a url at all", "", false},
		{"empty", "", "", false},
		{"youtube homepage", "https://www.youtube.com/", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, ok := ExtractVideoID(tt.rawURL)
			if ok != tt.wantOK || id != tt.wantID {
				t.Errorf("ExtractVideoID(%q) = (%q, %v), want (%q, %v)", tt.rawURL, id, ok, tt.wantID, tt.wantOK)
			}
		})
	}
}

func TestFetchMetadata_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"title":"Never Gonna Give You Up","author_name":"Rick Astley"}`))
	}))
	defer srv.Close()
	t.Cleanup(swapEndpoint(srv.URL))

	title, author, err := FetchMetadata(context.Background(), "https://youtu.be/dQw4w9WgXcQ")
	if err != nil {
		t.Fatalf("FetchMetadata returned error: %v", err)
	}
	if title != "Never Gonna Give You Up" || author != "Rick Astley" {
		t.Errorf("FetchMetadata = (%q, %q), want (%q, %q)", title, author, "Never Gonna Give You Up", "Rick Astley")
	}
}

func TestFetchMetadata_NonYouTubeURL(t *testing.T) {
	if _, _, err := FetchMetadata(context.Background(), "https://vimeo.com/12345"); err == nil {
		t.Error("expected an error for a non-YouTube URL, got nil")
	}
}

func TestFetchMetadata_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	t.Cleanup(swapEndpoint(srv.URL))

	if _, _, err := FetchMetadata(context.Background(), "https://youtu.be/dQw4w9WgXcQ"); err == nil {
		t.Error("expected an error for a non-200 oembed response, got nil")
	}
}

func TestFetchMetadata_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json`))
	}))
	defer srv.Close()
	t.Cleanup(swapEndpoint(srv.URL))

	if _, _, err := FetchMetadata(context.Background(), "https://youtu.be/dQw4w9WgXcQ"); err == nil {
		t.Error("expected an error for a malformed oembed response, got nil")
	}
}

func TestFetchMetadata_MissingTitle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"author_name":"Rick Astley"}`))
	}))
	defer srv.Close()
	t.Cleanup(swapEndpoint(srv.URL))

	if _, _, err := FetchMetadata(context.Background(), "https://youtu.be/dQw4w9WgXcQ"); err == nil {
		t.Error("expected an error when the oembed response has no title, got nil")
	}
}

// swapEndpoint points oembedEndpoint at url and returns a func that restores
// the original value — pass directly to t.Cleanup.
func swapEndpoint(url string) func() {
	orig := oembedEndpoint
	oembedEndpoint = url
	return func() { oembedEndpoint = orig }
}
