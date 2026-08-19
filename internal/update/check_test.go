package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func serve(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { latestReleaseURL = "https://api.github.com/repos/ArmadilloBrillo/cyber-tui/releases/latest" })
	latestReleaseURL = srv.URL
	return srv
}

func TestLatest_ParsesRelease(t *testing.T) {
	serve(t, http.StatusOK, `{"tag_name":"v1.2.3","html_url":"https://example.com/releases/v1.2.3"}`)

	rel, err := Latest(context.Background())
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if rel.TagName != "v1.2.3" {
		t.Errorf("TagName = %q, want v1.2.3", rel.TagName)
	}
	if rel.HTMLURL != "https://example.com/releases/v1.2.3" {
		t.Errorf("HTMLURL = %q, want https://example.com/releases/v1.2.3", rel.HTMLURL)
	}
}

func TestLatest_NonOKStatus(t *testing.T) {
	serve(t, http.StatusNotFound, `{"message":"Not Found"}`)

	if _, err := Latest(context.Background()); err == nil {
		t.Fatal("Latest: expected error for 404, got nil")
	}
}

func TestLatest_MalformedJSON(t *testing.T) {
	serve(t, http.StatusOK, `not json`)

	if _, err := Latest(context.Background()); err == nil {
		t.Fatal("Latest: expected error for malformed JSON, got nil")
	}
}
