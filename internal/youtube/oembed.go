// Package youtube provides lightweight, unauthenticated helpers for working
// with YouTube video links — validating a URL and looking up its title/
// channel via the public oEmbed endpoint. No API key involved or needed.
package youtube

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ragnar/cyber-tui/internal/version"
)

// userAgent identifies cyber-tui to YouTube, mirroring
// internal/ui/imgview's own dedicated User-Agent for third-party fetches.
var userAgent = "cyber-tui/" + version.Version + " (+https://github.com/ArmadilloBrillo/cyber-tui)"

// httpClient is dedicated to oEmbed lookups — a small JSON response, so a
// much shorter timeout than imgview's 30s image fetches is plenty.
var httpClient = &http.Client{Timeout: 8 * time.Second}

// oembedEndpoint is the oEmbed base URL. A package var so tests can point it
// at an httptest.Server instead of the real youtube.com.
var oembedEndpoint = "https://www.youtube.com/oembed"

// ExtractVideoID reports whether rawURL is a recognizable YouTube video link
// (youtube.com/watch?v=, youtu.be/<id>, youtube.com/shorts/<id>,
// youtube.com/embed/<id>) and, if so, its video ID. Used for client-side
// validation before a fetch or submit — the server itself rejects a
// non-YouTube /song link with a 400; this just gives faster feedback.
func ExtractVideoID(rawURL string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "", false
	}
	host := strings.TrimPrefix(strings.TrimPrefix(strings.ToLower(u.Host), "www."), "m.")
	switch host {
	case "youtu.be":
		id := strings.Trim(u.Path, "/")
		return id, id != ""
	case "youtube.com":
		switch {
		case u.Path == "/watch":
			id := u.Query().Get("v")
			return id, id != ""
		case strings.HasPrefix(u.Path, "/shorts/"):
			id := strings.TrimPrefix(u.Path, "/shorts/")
			return id, id != ""
		case strings.HasPrefix(u.Path, "/embed/"):
			id := strings.TrimPrefix(u.Path, "/embed/")
			return id, id != ""
		}
	}
	return "", false
}

// oembedResponse is the subset of YouTube's public oEmbed response this
// package uses.
type oembedResponse struct {
	Title      string `json:"title"`
	AuthorName string `json:"author_name"`
}

// FetchMetadata asks YouTube's public oEmbed endpoint for rawURL's video
// title and channel name — no API key required. The channel name
// (author_name) is the best available stand-in for "artist": oEmbed has no
// separate artist or genre field, so genre is always left for the caller to
// fill in manually.
func FetchMetadata(ctx context.Context, rawURL string) (title, author string, err error) {
	if _, ok := ExtractVideoID(rawURL); !ok {
		return "", "", fmt.Errorf("youtube: not a recognized YouTube URL")
	}
	endpoint := oembedEndpoint + "?format=json&url=" + url.QueryEscape(rawURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", "", fmt.Errorf("youtube: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("youtube: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("youtube: oembed returned %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", "", fmt.Errorf("youtube: read body: %w", err)
	}
	var parsed oembedResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", "", fmt.Errorf("youtube: decode: %w", err)
	}
	if parsed.Title == "" {
		return "", "", fmt.Errorf("youtube: oembed response missing title")
	}
	return parsed.Title, parsed.AuthorName, nil
}
