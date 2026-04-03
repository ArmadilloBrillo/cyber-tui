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
	"github.com/ragnar/cyber-tui/internal/rtdb"
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

// authHandler handles both /v1/auth/login and /v1/auth/refresh with fixed tokens,
// delegating all other requests to next. Use this when LoginWithRefreshToken is called.
func authHandler(t *testing.T, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/auth/login":
			writeOK(t, w, map[string]string{
				"idToken":      "test-id-token",
				"refreshToken": "test-refresh-token",
				"rtdbToken":    "test-rtdb-token",
			})
		case "/v1/auth/refresh":
			writeOK(t, w, map[string]string{
				"idToken":   "test-id-token",
				"rtdbToken": "test-rtdb-token",
			})
		default:
			next.ServeHTTP(w, r)
		}
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

func TestHTTPLoginWithRefreshToken_Success(t *testing.T) {
	var capturedRefreshToken string
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/auth/refresh" {
			var body struct {
				RefreshToken string `json:"refreshToken"`
			}
			raw, _ := io.ReadAll(r.Body)
			json.Unmarshal(raw, &body)
			capturedRefreshToken = body.RefreshToken
			writeOK(t, w, map[string]string{
				"idToken":   "fresh-id-token",
				"rtdbToken": "fresh-rtdb-token",
			})
			return
		}
		writeErr(w, 404, "NOT_FOUND", "unexpected path")
	}))

	tokens, err := c.LoginWithRefreshToken("saved-refresh-token")
	if err != nil {
		t.Fatalf("LoginWithRefreshToken: %v", err)
	}
	if capturedRefreshToken != "saved-refresh-token" {
		t.Errorf("sent refreshToken = %q, want %q", capturedRefreshToken, "saved-refresh-token")
	}
	if tokens.IDToken != "fresh-id-token" {
		t.Errorf("IDToken = %q, want %q", tokens.IDToken, "fresh-id-token")
	}
	if tokens.RTDBToken != "fresh-rtdb-token" {
		t.Errorf("RTDBToken = %q, want %q", tokens.RTDBToken, "fresh-rtdb-token")
	}
}

func TestHTTPLoginWithRefreshToken_Failure(t *testing.T) {
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, 401, "UNAUTHORIZED", "refresh token expired")
	}))

	_, err := c.LoginWithRefreshToken("expired-token")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	apiErr, ok := asAPIError(err)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
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

// --- RTDB / DM tests ---

func newClientWithRTDB(t *testing.T, rtdbHandler http.Handler) (*api.HTTPClient, *httptest.Server) {
	t.Helper()
	rtdbSrv := httptest.NewServer(rtdbHandler)
	t.Cleanup(rtdbSrv.Close)

	c := api.NewHTTPClientForTesting("http://unused", nil)
	rc := rtdb.NewForTesting(rtdbSrv.URL, "test-tok", rtdbSrv.Client())
	c.SetRTDBClientForTesting(rc)
	c.SetCurrentUID("uid-me")
	return c, rtdbSrv
}

// TestHTTPGetMessages_ReturnsEmpty confirms the stub returns empty (server-side not ready).
func TestHTTPGetMessages_ReturnsEmpty(t *testing.T) {
	c, _ := newClientWithRTDB(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	msgs, err := c.GetMessages("conv1", 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("len(msgs) = %d, want 0 (stub)", len(msgs))
	}
}

// TestHTTPSendMessage_NoOp confirms the stub does nothing (server-side not ready).
func TestHTTPSendMessage_NoOp(t *testing.T) {
	c, _ := newClientWithRTDB(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("SendMessage stub should not make any HTTP request")
	}))
	if err := c.SendMessage("conv1", "hi"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHTTPGetConversations_ReturnsEmpty confirms the stub returns empty (server-side not ready).
func TestHTTPGetConversations_ReturnsEmpty(t *testing.T) {
	c, _ := newClientWithRTDB(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("GetConversations stub should not make any HTTP request")
	}))
	convs, err := c.GetConversations()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(convs) != 0 {
		t.Errorf("len(convs) = %d, want 0 (stub)", len(convs))
	}
}

// --- notifications & GetPost ---

func TestHTTPGetNotifications_ParsesNotifs(t *testing.T) {
	c := newClient(t, authHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/notifications" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		writeOKWithCursor(t, w, []map[string]any{
			{
				"id":            "n1",
				"type":          "reply",
				"read":          false,
				"createdAt":     "2026-01-01T12:00:00Z",
				"actorId":       "u1",
				"actorUsername": "molly_millions",
				"targetId":      "p1",
				"targetType":    "reply",
				"metadata":      map[string]any{"replyId": "r42"},
			},
			{
				"id":            "n2",
				"type":          "poke",
				"read":          true,
				"createdAt":     "2026-01-01T11:00:00Z",
				"actorId":       "u2",
				"actorUsername": "wintermute",
				"targetId":      "",
			},
		}, "next-cursor")
	})))
	c.LoginWithRefreshToken("tok")
	notifs, cursor, err := c.GetNotifications("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(notifs) != 2 {
		t.Fatalf("expected 2 notifs, got %d", len(notifs))
	}
	if notifs[0].ID != "n1" || notifs[0].Type != "reply" || notifs[0].Read != false {
		t.Errorf("notifs[0] mismatch: %+v", notifs[0])
	}
	if notifs[0].Actor.Username != "molly_millions" {
		t.Errorf("actor username mismatch: %s", notifs[0].Actor.Username)
	}
	if notifs[0].TargetID != "p1" {
		t.Errorf("targetID mismatch: %s", notifs[0].TargetID)
	}
	if notifs[0].ReplyID != "r42" {
		t.Errorf("replyID mismatch: got %q, want %q", notifs[0].ReplyID, "r42")
	}
	if notifs[1].ID != "n2" || notifs[1].Read != true {
		t.Errorf("notifs[1] mismatch: %+v", notifs[1])
	}
	if cursor != "next-cursor" {
		t.Errorf("cursor mismatch: %s", cursor)
	}
}

func TestHTTPGetNotifications_CursorInURL(t *testing.T) {
	var capturedURL string
	c := newClient(t, authHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL.String()
		writeOKWithCursor(t, w, []map[string]any{}, "")
	})))
	c.LoginWithRefreshToken("tok")
	c.GetNotifications("cursor-abc")
	if !strings.Contains(capturedURL, "cursor=cursor-abc") {
		t.Errorf("expected cursor in URL, got: %s", capturedURL)
	}
}

func TestHTTPMarkNotificationRead_Method(t *testing.T) {
	var capturedMethod, capturedPath string
	c := newClient(t, authHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		io.WriteString(w, `{"data":null}`)
	})))
	c.LoginWithRefreshToken("tok")
	if err := c.MarkNotificationRead("n1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedMethod != "PATCH" {
		t.Errorf("expected PATCH, got %s", capturedMethod)
	}
	if capturedPath != "/v1/notifications/n1" {
		t.Errorf("expected /v1/notifications/n1, got %s", capturedPath)
	}
}

func TestHTTPMarkAllNotificationsRead_Method(t *testing.T) {
	var capturedMethod, capturedPath string
	c := newClient(t, authHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		io.WriteString(w, `{"data":null}`)
	})))
	c.LoginWithRefreshToken("tok")
	if err := c.MarkAllNotificationsRead(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedMethod != "POST" {
		t.Errorf("expected POST, got %s", capturedMethod)
	}
	if capturedPath != "/v1/notifications/read-all" {
		t.Errorf("expected /v1/notifications/read-all, got %s", capturedPath)
	}
}

func TestHTTPGetPost_ParsesPost(t *testing.T) {
	c := newClient(t, authHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/posts/p99" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		writeOK(t, w, map[string]any{
			"postId":         "p99",
			"authorId":       "u1",
			"authorUsername": "neuromancer",
			"content":        "hello world",
			"topics":         []string{"tui"},
			"repliesCount":   3,
			"isPublic":       true,
			"isNSFW":         false,
			"deleted":        false,
			"createdAt":      "2026-01-01T10:00:00Z",
		})
	})))
	c.LoginWithRefreshToken("tok")
	post, err := c.GetPost("p99")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if post.ID != "p99" {
		t.Errorf("ID mismatch: %s", post.ID)
	}
	if post.AuthorUsername != "neuromancer" {
		t.Errorf("AuthorUsername mismatch: %s", post.AuthorUsername)
	}
	if post.Content != "hello world" {
		t.Errorf("Content mismatch: %s", post.Content)
	}
	if post.RepliesCount != 3 {
		t.Errorf("RepliesCount mismatch: %d", post.RepliesCount)
	}
}
