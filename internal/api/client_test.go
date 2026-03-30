package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ragnar/cyber-tui/internal/api"
	"github.com/ragnar/cyber-tui/internal/model"
)

// --- helpers ---

type testEnvelope struct {
	Data   json.RawMessage `json:"data,omitempty"`
	Cursor string          `json:"cursor,omitempty"`
	Error  *testAPIError   `json:"error,omitempty"`
}

type testAPIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeOKWithCursor(t *testing.T, w http.ResponseWriter, data any, cursor string) {
	t.Helper()
	raw, _ := json.Marshal(data)
	env := testEnvelope{Data: raw, Cursor: cursor}
	b, _ := json.Marshal(env)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write(b)
}

func writeOK(t *testing.T, w http.ResponseWriter, data any) {
	t.Helper()
	raw, _ := json.Marshal(data)
	env := testEnvelope{Data: raw}
	b, _ := json.Marshal(env)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write(b)
}

func writeErr(w http.ResponseWriter, status int, code, msg string) {
	env := testEnvelope{Error: &testAPIError{Code: code, Message: msg}}
	b, _ := json.Marshal(env)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(b)
}

func newClient(t *testing.T, handler http.Handler) *api.HTTPClient {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return api.NewHTTPClientForTesting(srv.URL, srv.Client())
}

func asAPIError(err error) (*api.APIError, bool) {
	e, ok := err.(*api.APIError)
	return e, ok
}

// loginHandler returns a handler that responds to POST /v1/auth/login with a
// fixed token and delegates all other requests to next.
func loginHandler(t *testing.T, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/auth/login" {
			writeOK(t, w, map[string]string{
				"idToken":      "test-id-token",
				"refreshToken": "test-refresh-token",
				"rtdbToken":    "test-rtdb-token",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// --- tests ---

func TestHTTPLogin_Success(t *testing.T) {
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeOK(t, w, map[string]string{
			"idToken":      "id-abc",
			"refreshToken": "ref-abc",
			"rtdbToken":    "rtdb-abc",
		})
	}))

	tokens, err := c.Login("user@example.com", "secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tokens.IDToken != "id-abc" {
		t.Errorf("IDToken = %q, want id-abc", tokens.IDToken)
	}
	if tokens.RefreshToken != "ref-abc" {
		t.Errorf("RefreshToken = %q, want ref-abc", tokens.RefreshToken)
	}
	if tokens.RTDBToken != "rtdb-abc" {
		t.Errorf("RTDBToken = %q, want rtdb-abc", tokens.RTDBToken)
	}
}

func TestHTTPLogin_BadCredentials(t *testing.T) {
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, 401, "UNAUTHORIZED", "invalid credentials")
	}))

	_, err := c.Login("bad@example.com", "wrong")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	apiErr, ok := asAPIError(err)
	if !ok {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.Code != "UNAUTHORIZED" {
		t.Errorf("Code = %q, want UNAUTHORIZED", apiErr.Code)
	}
}

func TestHTTPGetFeed_ParsesPosts(t *testing.T) {
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeOK(t, w, []map[string]any{
			{
				"postId":         "p1",
				"authorId":       "u1",
				"authorUsername": "neuromancer",
				"content":        "hello matrix",
				"topics":         []string{"cyber"},
				"repliesCount":   2,
				"bookmarksCount": 5,
				"isPublic":       true,
				"isNSFW":         false,
				"deleted":        false,
				"createdAt":      "2025-01-01T12:00:00.000Z",
			},
			{
				"postId":         "p2",
				"authorId":       "u2",
				"authorUsername": "molly",
				"content":        "don't stare",
				"topics":         []string{},
				"repliesCount":   0,
				"bookmarksCount": 0,
				"isPublic":       true,
				"isNSFW":         false,
				"deleted":        false,
				"createdAt":      "2025-01-01T11:00:00Z",
			},
		})
	}))

	posts, _, err := c.GetFeed("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(posts) != 2 {
		t.Fatalf("len(posts) = %d, want 2", len(posts))
	}
	if posts[0].ID != "p1" {
		t.Errorf("posts[0].ID = %q, want p1", posts[0].ID)
	}
	if posts[0].AuthorUsername != "neuromancer" {
		t.Errorf("AuthorUsername = %q, want neuromancer", posts[0].AuthorUsername)
	}
	if posts[0].RepliesCount != 2 {
		t.Errorf("RepliesCount = %d, want 2", posts[0].RepliesCount)
	}
}

func TestHTTPGetFeed_SendsCursor(t *testing.T) {
	var gotURL string
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.String()
		writeOK(t, w, []any{})
	}))

	c.GetFeed("cursor-xyz") //nolint:errcheck,dogsled
	if !strings.Contains(gotURL, "cursor=cursor-xyz") {
		t.Errorf("cursor not in URL: %s", gotURL)
	}
}

func TestHTTPGetOwnProfile_ParsesUser(t *testing.T) {
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeOK(t, w, map[string]string{
			"userId":      "u42",
			"username":    "case",
			"displayName": "Henry Case",
			"email":       "case@matrix.net",
			"bio":         "console cowboy",
		})
	}))

	user, err := c.GetOwnProfile()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.ID != "u42" {
		t.Errorf("ID = %q, want u42", user.ID)
	}
	if user.Username != "case" {
		t.Errorf("Username = %q, want case", user.Username)
	}
	if user.Bio != "console cowboy" {
		t.Errorf("Bio = %q, want console cowboy", user.Bio)
	}
}

func TestHTTPRateLimit(t *testing.T) {
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
	}))

	_, _, err := c.GetFeed("")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	apiErr, ok := asAPIError(err)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.Code != "RATE_LIMITED" {
		t.Errorf("Code = %q, want RATE_LIMITED", apiErr.Code)
	}
}

func TestHTTPTokenRefresh_Success(t *testing.T) {
	firstFeedCall := true
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/auth/login":
			writeOK(t, w, map[string]string{
				"idToken": "old-token", "refreshToken": "ref", "rtdbToken": "rtdb",
			})
		case "/v1/auth/refresh":
			writeOK(t, w, map[string]string{
				"idToken": "new-token", "rtdbToken": "new-rtdb",
			})
		default:
			if firstFeedCall {
				firstFeedCall = false
				writeErr(w, 401, "UNAUTHORIZED", "token expired")
			} else {
				writeOK(t, w, []any{})
			}
		}
	}))

	c.Login("u@example.com", "pw") //nolint:errcheck
	_, _, err := c.GetFeed("")
	if err != nil {
		t.Fatalf("expected success after token refresh, got: %v", err)
	}
}

func TestHTTPTokenRefresh_Failure(t *testing.T) {
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/auth/login":
			writeOK(t, w, map[string]string{
				"idToken": "old-token", "refreshToken": "ref", "rtdbToken": "rtdb",
			})
		default:
			// Both the original request and the refresh attempt return 401
			writeErr(w, 401, "UNAUTHORIZED", "token expired")
		}
	}))

	c.Login("u@example.com", "pw") //nolint:errcheck
	_, _, err := c.GetFeed("")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	apiErr, ok := asAPIError(err)
	if !ok {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.Code != "UNAUTHORIZED" {
		t.Errorf("Code = %q, want UNAUTHORIZED", apiErr.Code)
	}
}

func TestHTTPUpdateProfile_OmitsNilFields(t *testing.T) {
	var capturedBody []byte
	c := newClient(t, loginHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		capturedBody = raw
		writeOK(t, w, map[string]any{})
	})))

	c.Login("u@example.com", "pw") //nolint:errcheck

	bio := "new bio"
	err := c.UpdateProfile(model.ProfileUpdate{Bio: &bio})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(capturedBody, &m); err != nil {
		t.Fatalf("unmarshal PATCH body: %v", err)
	}
	if _, ok := m["bio"]; !ok {
		t.Error("bio field missing from PATCH body")
	}
	for _, field := range []string{"displayName", "pinnedPostId", "websiteUrl", "locationName"} {
		if _, ok := m[field]; ok {
			t.Errorf("field %q should be omitted (nil pointer), but was present", field)
		}
	}
}

func TestHTTPGetFeed_ReturnsCursor(t *testing.T) {
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeOKWithCursor(t, w, []any{}, "next-page-token")
	}))

	_, cursor, err := c.GetFeed("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cursor != "next-page-token" {
		t.Errorf("cursor = %q, want next-page-token", cursor)
	}
}

func TestHTTPGetPostReplies_ParsesReplies(t *testing.T) {
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeOK(t, w, []map[string]any{
			{
				"replyId":        "r1",
				"postId":         "p1",
				"authorId":       "u1",
				"authorUsername": "molly_millions",
				"content":        "interesting perspective",
				"parentReplyId":  "",
				"createdAt":      "2025-01-01T12:00:00.000Z",
			},
			{
				"replyId":        "r2",
				"postId":         "p1",
				"authorId":       "u2",
				"authorUsername": "wintermute",
				"content":        "i arranged for this",
				"parentReplyId":  "",
				"createdAt":      "2025-01-01T12:05:00.000Z",
			},
		})
	}))

	replies, err := c.GetPostReplies("p1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(replies) != 2 {
		t.Fatalf("len(replies) = %d, want 2", len(replies))
	}
	if replies[0].ID != "r1" {
		t.Errorf("replies[0].ID = %q, want r1", replies[0].ID)
	}
	if replies[0].AuthorUsername != "molly_millions" {
		t.Errorf("AuthorUsername = %q, want molly_millions", replies[0].AuthorUsername)
	}
	if replies[0].PostID != "p1" {
		t.Errorf("PostID = %q, want p1", replies[0].PostID)
	}
}

func TestHTTPGetPostReplies_UsesPostID(t *testing.T) {
	var gotURL string
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.String()
		writeOK(t, w, []any{})
	}))

	c.GetPostReplies("my-post-id") //nolint:errcheck
	if !strings.Contains(gotURL, "my-post-id") {
		t.Errorf("postID not in URL: %s", gotURL)
	}
}

func TestHTTPGetFeed_EmptyCursorWhenLastPage(t *testing.T) {
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeOK(t, w, []any{})
	}))

	_, cursor, err := c.GetFeed("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cursor != "" {
		t.Errorf("cursor = %q, want empty string", cursor)
	}
}
