// Package update checks GitHub for a newer cyber-tui release.
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ragnar/cyber-tui/internal/version"
)

// userAgent identifies cyber-tui to the GitHub API, matching the convention
// used for image fetches (internal/ui/imgview/fetch.go) — Go's default UA is
// discouraged by GitHub's API etiquette.
var userAgent = "cyber-tui/" + version.Version + " (+https://github.com/ArmadilloBrillo/cyber-tui)"

// httpClient is dedicated to update checks so this path never inherits
// global mutations to http.DefaultClient.
var httpClient = &http.Client{Timeout: 8 * time.Second}

// latestReleaseURL is a var (not const) so tests can point it at an
// httptest.Server instead of the real GitHub API.
var latestReleaseURL = "https://api.github.com/repos/ArmadilloBrillo/cyber-tui/releases/latest"

// maxResponseBytes caps how much of the response body is read, defending
// against a misbehaving or malicious endpoint.
const maxResponseBytes = 1 << 20

// Release is the subset of GitHub's release JSON this package needs.
type Release struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

// Latest fetches the latest published release from GitHub.
func Latest(ctx context.Context) (Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, latestReleaseURL, nil)
	if err != nil {
		return Release{}, fmt.Errorf("update: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("update: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("update: unexpected status %d", resp.StatusCode)
	}

	var rel Release
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&rel); err != nil {
		return Release{}, fmt.Errorf("update: %w", err)
	}
	return rel, nil
}
