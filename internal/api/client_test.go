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

func TestHTTPCreatePost_SendsAllFields(t *testing.T) {
	var gotBody map[string]any
	c := newClient(t, loginHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &gotBody)
		writeOK(t, w, map[string]any{
			"postId": "new-post-1",
			"slug":   "my-title",
			"title":  "My Title",
		})
	})))
	c.Login("u@example.com", "pass") //nolint:errcheck

	post, err := c.CreatePost("body text", "My Title", []string{"cyber"}, true, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if post.ID != "new-post-1" {
		t.Errorf("post.ID = %q, want new-post-1", post.ID)
	}
	if post.Title != "My Title" {
		t.Errorf("post.Title = %q, want My Title", post.Title)
	}
	if post.Slug != "my-title" {
		t.Errorf("post.Slug = %q, want my-title", post.Slug)
	}
	if !post.IsPublic {
		t.Error("expected post.IsPublic=true")
	}
	if !post.IsNSFW {
		t.Error("expected post.IsNSFW=true")
	}
	if gotBody["title"] != "My Title" {
		t.Errorf("request body title = %v, want My Title", gotBody["title"])
	}
	if gotBody["isPublic"] != true {
		t.Errorf("request body isPublic = %v, want true", gotBody["isPublic"])
	}
	if gotBody["isNSFW"] != true {
		t.Errorf("request body isNSFW = %v, want true", gotBody["isNSFW"])
	}
}

func TestHTTPGetFeed_ParsesExtendedPostFields(t *testing.T) {
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeOK(t, w, []map[string]any{
			{
				"postId":         "p1",
				"authorId":       "u1",
				"authorUsername": "neuromancer",
				"content":        "hello",
				"title":          "A Title",
				"slug":           "a-title",
				"guildId":        "g1",
				"guildSlug":      "night-owls",
				"isGuildThread":  true,
				"topics":         []string{},
				"isPublic":       false,
				"isNSFW":         true,
				"deleted":        false,
				"createdAt":      "2025-01-01T12:00:00.000Z",
			},
		})
	}))

	posts, _, err := c.GetFeed("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(posts) != 1 {
		t.Fatalf("len(posts) = %d, want 1", len(posts))
	}
	p := posts[0]
	if p.Title != "A Title" {
		t.Errorf("Title = %q, want A Title", p.Title)
	}
	if p.Slug != "a-title" {
		t.Errorf("Slug = %q, want a-title", p.Slug)
	}
	if p.GuildID != "g1" {
		t.Errorf("GuildID = %q, want g1", p.GuildID)
	}
	if p.GuildSlug != "night-owls" {
		t.Errorf("GuildSlug = %q, want night-owls", p.GuildSlug)
	}
	if !p.IsGuildThread {
		t.Error("expected IsGuildThread=true")
	}
	if !p.IsNSFW {
		t.Error("expected IsNSFW=true")
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
				"metadata":      map[string]any{"replyId": "r42", "guildSlug": "chooms", "isGuildThread": true},
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
	notifs, cursor, err := c.GetNotifications("", false)
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
	if notifs[0].GuildSlug != "chooms" {
		t.Errorf("guildSlug mismatch: got %q, want %q", notifs[0].GuildSlug, "chooms")
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
	c.GetNotifications("cursor-abc", false)
	if !strings.Contains(capturedURL, "cursor=cursor-abc") {
		t.Errorf("expected cursor in URL, got: %s", capturedURL)
	}
}

func TestHTTPGetNotifications_UnreadOnlyParam(t *testing.T) {
	var capturedURL string
	c := newClient(t, authHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL.String()
		writeOKWithCursor(t, w, []map[string]any{}, "")
	})))
	c.LoginWithRefreshToken("tok")
	c.GetNotifications("", true)
	if !strings.Contains(capturedURL, "read=false") {
		t.Errorf("expected read=false in URL, got: %s", capturedURL)
	}
}

func TestHTTPGetNotifications_AllDoesNotAddReadParam(t *testing.T) {
	var capturedURL string
	c := newClient(t, authHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL.String()
		writeOKWithCursor(t, w, []map[string]any{}, "")
	})))
	c.LoginWithRefreshToken("tok")
	c.GetNotifications("", false)
	if strings.Contains(capturedURL, "read=") {
		t.Errorf("expected no read param in URL, got: %s", capturedURL)
	}
}

func TestHTTPGetUnreadNotificationCount_ReturnsCount(t *testing.T) {
	c := newClient(t, authHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/notifications/unread-count" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		io.WriteString(w, `{"data":{"count":7}}`)
	})))
	c.LoginWithRefreshToken("tok")
	count, err := c.GetUnreadNotificationCount()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 7 {
		t.Errorf("expected count 7, got %d", count)
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

// --- Bookmarks ---

func TestHTTPGetBookmarks_ParsesList(t *testing.T) {
	c := newClient(t, loginHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/bookmarks" {
			http.NotFound(w, r)
			return
		}
		writeOKWithCursor(t, w, []map[string]any{
			{
				"bookmarkId": "bm1",
				"type":       "post",
				"postId":     "p1",
				"post": map[string]any{
					"postId":         "p1",
					"authorId":       "u1",
					"authorUsername": "neuromancer",
					"content":        "flatline is not death",
					"topics":         []string{"cyber"},
					"repliesCount":   0,
					"bookmarksCount": 1,
					"isPublic":       false,
					"isNSFW":         false,
					"deleted":        false,
					"createdAt":      "2026-01-01T10:00:00Z",
				},
				"createdAt": "2026-01-02T10:00:00Z",
			},
		}, "cursor-abc")
	})))
	c.Login("u@example.com", "pass")

	bookmarks, cursor, err := c.GetBookmarks("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bookmarks) != 1 {
		t.Fatalf("len(bookmarks) = %d, want 1", len(bookmarks))
	}
	if bookmarks[0].ID != "bm1" {
		t.Errorf("ID = %q, want bm1", bookmarks[0].ID)
	}
	if bookmarks[0].Type != "post" {
		t.Errorf("Type = %q, want post", bookmarks[0].Type)
	}
	if bookmarks[0].Post == nil {
		t.Fatal("Post is nil, want embedded post")
	}
	if bookmarks[0].Post.ID != "p1" {
		t.Errorf("Post.ID = %q, want p1", bookmarks[0].Post.ID)
	}
	if cursor != "cursor-abc" {
		t.Errorf("cursor = %q, want cursor-abc", cursor)
	}
}

func TestHTTPGetBookmarks_WithCursor(t *testing.T) {
	var gotCursor string
	c := newClient(t, loginHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCursor = r.URL.Query().Get("cursor")
		writeOKWithCursor(t, w, []map[string]any{}, "")
	})))
	c.Login("u@example.com", "pass")

	_, _, err := c.GetBookmarks("my-cursor-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotCursor != "my-cursor-123" {
		t.Errorf("cursor query param = %q, want my-cursor-123", gotCursor)
	}
}

func TestHTTPCreateBookmark_Post(t *testing.T) {
	var gotBody map[string]any
	c := newClient(t, loginHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/v1/bookmarks" {
			http.NotFound(w, r)
			return
		}
		json.NewDecoder(r.Body).Decode(&gotBody)
		writeOK(t, w, map[string]string{"bookmarkId": "bm-new-1"})
	})))
	c.Login("u@example.com", "pass")

	id, err := c.CreateBookmark("p1", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "bm-new-1" {
		t.Errorf("bookmarkId = %q, want bm-new-1", id)
	}
	if gotBody["type"] != "post" {
		t.Errorf("type = %v, want post", gotBody["type"])
	}
	if gotBody["postId"] != "p1" {
		t.Errorf("postId = %v, want p1", gotBody["postId"])
	}
	if gotBody["replyId"] != nil {
		t.Errorf("replyId should be omitted, got %v", gotBody["replyId"])
	}
}

func TestHTTPCreateBookmark_Reply(t *testing.T) {
	var gotBody map[string]any
	c := newClient(t, loginHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		writeOK(t, w, map[string]string{"bookmarkId": "bm-new-2"})
	})))
	c.Login("u@example.com", "pass")

	id, err := c.CreateBookmark("", "r5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "bm-new-2" {
		t.Errorf("bookmarkId = %q, want bm-new-2", id)
	}
	if gotBody["type"] != "reply" {
		t.Errorf("type = %v, want reply", gotBody["type"])
	}
	if gotBody["replyId"] != "r5" {
		t.Errorf("replyId = %v, want r5", gotBody["replyId"])
	}
}

func TestHTTPDeleteBookmark_Success(t *testing.T) {
	var gotPath string
	c := newClient(t, loginHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		writeOK(t, w, map[string]any{})
	})))
	c.Login("u@example.com", "pass")

	if err := c.DeleteBookmark("bm-123"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/v1/bookmarks/bm-123" {
		t.Errorf("path = %q, want /v1/bookmarks/bm-123", gotPath)
	}
}

func TestHTTPDeleteBookmark_NotFound(t *testing.T) {
	c := newClient(t, loginHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, 404, "NOT_FOUND", "bookmark not found")
	})))
	c.Login("u@example.com", "pass")

	err := c.DeleteBookmark("no-such-id")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	apiErr, ok := asAPIError(err)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.Code != "NOT_FOUND" {
		t.Errorf("Code = %q, want NOT_FOUND", apiErr.Code)
	}
}

// --- Follows ---

func TestHTTPGetFollowing_Success(t *testing.T) {
	c := newClient(t, loginHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/follows" {
			t.Errorf("path = %q, want /v1/follows", r.URL.Path)
		}
		if r.URL.Query().Get("type") != "following" {
			t.Errorf("type = %q, want following", r.URL.Query().Get("type"))
		}
		writeOKWithCursor(t, w, []map[string]string{
			{"followId": "f1", "followerId": "uid-me", "followedId": "uid-them", "createdAt": "2026-01-01T00:00:00Z"},
		}, "f1")
	})))
	c.Login("u@example.com", "pass")

	follows, cursor, err := c.GetFollowing("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(follows) != 1 {
		t.Fatalf("len(follows) = %d, want 1", len(follows))
	}
	if follows[0].ID != "f1" {
		t.Errorf("ID = %q, want f1", follows[0].ID)
	}
	if follows[0].FollowerID != "uid-me" {
		t.Errorf("FollowerID = %q, want uid-me", follows[0].FollowerID)
	}
	if follows[0].FollowedID != "uid-them" {
		t.Errorf("FollowedID = %q, want uid-them", follows[0].FollowedID)
	}
	if cursor != "f1" {
		t.Errorf("cursor = %q, want f1", cursor)
	}
}

func TestHTTPGetFollowing_Cursor(t *testing.T) {
	c := newClient(t, loginHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.URL.Query().Get("cursor")
		if got != "prev-cursor" {
			t.Errorf("cursor = %q, want prev-cursor", got)
		}
		writeOKWithCursor(t, w, []map[string]string{}, "")
	})))
	c.Login("u@example.com", "pass")

	_, _, err := c.GetFollowing("prev-cursor")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHTTPFollow_Success(t *testing.T) {
	c := newClient(t, loginHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/v1/follows" {
			t.Errorf("method/path = %s %s, want POST /v1/follows", r.Method, r.URL.Path)
		}
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["followedId"] != "uid-target" {
			t.Errorf("followedId = %q, want uid-target", body["followedId"])
		}
		writeOK(t, w, map[string]string{"followId": "new-follow-id"})
	})))
	c.Login("u@example.com", "pass")

	followID, err := c.Follow("uid-target")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if followID != "new-follow-id" {
		t.Errorf("followID = %q, want new-follow-id", followID)
	}
}

func TestHTTPFollow_Conflict(t *testing.T) {
	c := newClient(t, loginHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, 409, "CONFLICT", "already following")
	})))
	c.Login("u@example.com", "pass")

	_, err := c.Follow("uid-target")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	apiErr, ok := asAPIError(err)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.Code != "CONFLICT" {
		t.Errorf("Code = %q, want CONFLICT", apiErr.Code)
	}
}

func TestHTTPUnfollow_Success(t *testing.T) {
	var gotPath string
	c := newClient(t, loginHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		gotPath = r.URL.Path
		w.WriteHeader(200)
		w.Write([]byte(`{"data":{}}`))
	})))
	c.Login("u@example.com", "pass")

	err := c.Unfollow("follow-abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/v1/follows/follow-abc" {
		t.Errorf("path = %q, want /v1/follows/follow-abc", gotPath)
	}
}

func TestHTTPUnfollow_NotFound(t *testing.T) {
	c := newClient(t, loginHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, 404, "NOT_FOUND", "follow not found")
	})))
	c.Login("u@example.com", "pass")

	err := c.Unfollow("no-such-follow")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	apiErr, ok := asAPIError(err)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.Code != "NOT_FOUND" {
		t.Errorf("Code = %q, want NOT_FOUND", apiErr.Code)
	}
}

func TestHTTPGetProfile_CountFields(t *testing.T) {
	c := newClient(t, loginHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeOK(t, w, map[string]interface{}{
			"userId":         "uid-123",
			"username":       "ragnar",
			"followersCount": 35,
			"followingCount": 45,
			"postsCount":     6,
		})
	})))
	c.Login("u@example.com", "pass")

	user, err := c.GetProfile("ragnar")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.FollowersCount != 35 {
		t.Errorf("FollowersCount = %d, want 35", user.FollowersCount)
	}
	if user.FollowingCount != 45 {
		t.Errorf("FollowingCount = %d, want 45", user.FollowingCount)
	}
	if user.PostsCount != 6 {
		t.Errorf("PostsCount = %d, want 6", user.PostsCount)
	}
}

func TestHTTPGetOwnProfile_ParsesNewFields(t *testing.T) {
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeOK(t, w, map[string]interface{}{
			"userId":            "u42",
			"username":          "case",
			"websiteName":       "My Blog",
			"websiteImageUrl":   "https://example.com/img.png",
			"locationLatitude":  -33.9249,
			"locationLongitude": 18.4241,
		})
	}))

	user, err := c.GetOwnProfile()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.WebsiteName != "My Blog" {
		t.Errorf("WebsiteName = %q, want My Blog", user.WebsiteName)
	}
	if user.WebsiteImageUrl != "https://example.com/img.png" {
		t.Errorf("WebsiteImageUrl = %q, want https://example.com/img.png", user.WebsiteImageUrl)
	}
	if user.LocationLatitude != -33.9249 {
		t.Errorf("LocationLatitude = %v, want -33.9249", user.LocationLatitude)
	}
	if user.LocationLongitude != 18.4241 {
		t.Errorf("LocationLongitude = %v, want 18.4241", user.LocationLongitude)
	}
}

// --- Notes ---

func TestHTTPGetNotes_ParsesList(t *testing.T) {
	c := newClient(t, loginHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/notes" {
			http.NotFound(w, r)
			return
		}
		writeOKWithCursor(t, w, []map[string]any{
			{
				"noteId":         "note-1",
				"authorId":       "uid-abc",
				"content":        "flatline is not death",
				"revisionNumber": 1,
				"deleted":        false,
				"createdAt":      "2026-04-16T11:22:55.390Z",
			},
			{
				"noteId":         "note-2",
				"authorId":       "uid-abc",
				"content":        "another note here",
				"revisionNumber": 2,
				"deleted":        false,
				"createdAt":      "2026-04-16T10:00:00.000Z",
			},
		}, "cursor-xyz")
	})))
	c.Login("u@example.com", "pass")

	notes, cursor, err := c.GetNotes("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(notes) != 2 {
		t.Fatalf("len(notes) = %d, want 2", len(notes))
	}
	if notes[0].ID != "note-1" {
		t.Errorf("ID = %q, want note-1", notes[0].ID)
	}
	if notes[0].Content != "flatline is not death" {
		t.Errorf("Content = %q, want 'flatline is not death'", notes[0].Content)
	}
	if notes[0].RevisionNumber != 1 {
		t.Errorf("RevisionNumber = %d, want 1", notes[0].RevisionNumber)
	}
	if notes[1].ID != "note-2" {
		t.Errorf("ID = %q, want note-2", notes[1].ID)
	}
	if cursor != "cursor-xyz" {
		t.Errorf("cursor = %q, want cursor-xyz", cursor)
	}
}

func TestHTTPGetNotes_WithCursor(t *testing.T) {
	var gotCursor string
	c := newClient(t, loginHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCursor = r.URL.Query().Get("cursor")
		writeOKWithCursor(t, w, []map[string]any{}, "")
	})))
	c.Login("u@example.com", "pass")

	_, _, err := c.GetNotes("my-note-cursor")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotCursor != "my-note-cursor" {
		t.Errorf("cursor query param = %q, want my-note-cursor", gotCursor)
	}
}

func TestHTTPCreateNote(t *testing.T) {
	var gotBody map[string]any
	c := newClient(t, loginHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/v1/notes" {
			http.NotFound(w, r)
			return
		}
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(201)
		writeOK(t, w, map[string]string{"noteId": "note-new-1"})
	})))
	c.Login("u@example.com", "pass")

	note, err := c.CreateNote("my note content", []string{"journal"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if note.ID != "note-new-1" {
		t.Errorf("ID = %q, want note-new-1", note.ID)
	}
	if note.Content != "my note content" {
		t.Errorf("Content = %q, want 'my note content'", note.Content)
	}
	if gotBody["content"] != "my note content" {
		t.Errorf("request body content = %v, want 'my note content'", gotBody["content"])
	}
}

func TestHTTPUpdateNote(t *testing.T) {
	var gotBody map[string]any
	var gotPath string
	c := newClient(t, loginHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PATCH" {
			http.NotFound(w, r)
			return
		}
		gotPath = r.URL.Path
		json.NewDecoder(r.Body).Decode(&gotBody)
		writeOK(t, w, map[string]any{})
	})))
	c.Login("u@example.com", "pass")

	err := c.UpdateNote("note-abc", "updated content", []string{"idea"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/v1/notes/note-abc" {
		t.Errorf("PATCH path = %q, want /v1/notes/note-abc", gotPath)
	}
	if gotBody["content"] != "updated content" {
		t.Errorf("request body content = %v, want 'updated content'", gotBody["content"])
	}
}

func TestHTTPDeleteNote(t *testing.T) {
	var gotMethod, gotPath string
	c := newClient(t, loginHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		writeOK(t, w, map[string]any{})
	})))
	c.Login("u@example.com", "pass")

	err := c.DeleteNote("note-xyz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != "DELETE" {
		t.Errorf("method = %q, want DELETE", gotMethod)
	}
	if gotPath != "/v1/notes/note-xyz" {
		t.Errorf("path = %q, want /v1/notes/note-xyz", gotPath)
	}
}

// --- GetNote ---

func TestHTTPGetNote_ParsesSingleNote(t *testing.T) {
	var gotPath string
	c := newClient(t, loginHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		writeOK(t, w, map[string]any{
			"noteId":         "n1",
			"authorId":       "u1",
			"content":        "private thoughts",
			"topics":         []string{"idea"},
			"revisionNumber": 2,
			"deleted":        false,
			"createdAt":      "2025-06-01T10:00:00.000Z",
		})
	})))
	c.Login("u@example.com", "pass")

	note, err := c.GetNote("n1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/v1/notes/n1" {
		t.Errorf("GET path = %q, want /v1/notes/n1", gotPath)
	}
	if note.ID != "n1" {
		t.Errorf("ID = %q, want n1", note.ID)
	}
	if note.RevisionNumber != 2 {
		t.Errorf("RevisionNumber = %d, want 2", note.RevisionNumber)
	}
}

// --- GetNoteRevision ---

func TestHTTPGetNoteRevision_UsesQueryParam(t *testing.T) {
	var gotURL string
	c := newClient(t, loginHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.RequestURI()
		writeOK(t, w, map[string]any{
			"noteId":         "n1",
			"authorId":       "u1",
			"content":        "first draft",
			"revisionNumber": 1,
			"createdAt":      "2025-06-01T09:00:00.000Z",
		})
	})))
	c.Login("u@example.com", "pass")

	note, err := c.GetNoteRevision("n1", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotURL != "/v1/notes/n1?revision=1" {
		t.Errorf("URL = %q, want /v1/notes/n1?revision=1", gotURL)
	}
	if note.RevisionNumber != 1 {
		t.Errorf("RevisionNumber = %d, want 1", note.RevisionNumber)
	}
}

// --- GetNoteRevisions ---

func TestHTTPGetNoteRevisions_ParsesList(t *testing.T) {
	var gotPath string
	c := newClient(t, loginHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		writeOKWithCursor(t, w, []map[string]any{
			{"revisionNumber": 2, "content": "v2 content", "topics": []string{"idea"}, "createdAt": "2025-06-02T10:00:00.000Z"},
			{"revisionNumber": 1, "content": "v1 content", "topics": []string{}, "createdAt": "2025-06-01T09:00:00.000Z"},
		}, "")
	})))
	c.Login("u@example.com", "pass")

	revs, cursor, err := c.GetNoteRevisions("n1", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/v1/notes/n1/revisions" {
		t.Errorf("GET path = %q, want /v1/notes/n1/revisions", gotPath)
	}
	if len(revs) != 2 {
		t.Fatalf("len(revs) = %d, want 2", len(revs))
	}
	if revs[0].RevisionNumber != 2 {
		t.Errorf("revs[0].RevisionNumber = %d, want 2", revs[0].RevisionNumber)
	}
	if cursor != "" {
		t.Errorf("cursor = %q, want empty", cursor)
	}
}

func TestHTTPGetNoteRevisions_PassesCursor(t *testing.T) {
	var gotQuery string
	c := newClient(t, loginHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		writeOKWithCursor(t, w, []map[string]any{}, "")
	})))
	c.Login("u@example.com", "pass")

	_, _, err := c.GetNoteRevisions("n1", "cursor-abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(gotQuery, "cursor=cursor-abc") {
		t.Errorf("query = %q, want cursor=cursor-abc", gotQuery)
	}
}

// --- GetFollowers ---

func TestHTTPGetFollowers_UsesFollowersType(t *testing.T) {
	var gotQuery string
	c := newClient(t, loginHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		writeOKWithCursor(t, w, []map[string]any{
			{"followId": "fw1", "followerId": "u2", "followedId": "u1", "followerUsername": "molly", "followedUsername": "case", "createdAt": "2025-01-01T00:00:00Z"},
		}, "")
	})))
	c.Login("u@example.com", "pass")

	follows, _, err := c.GetFollowers("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(gotQuery, "type=followers") {
		t.Errorf("query = %q, want type=followers", gotQuery)
	}
	if len(follows) != 1 {
		t.Fatalf("len(follows) = %d, want 1", len(follows))
	}
	if follows[0].FollowerUsername != "molly" {
		t.Errorf("FollowerUsername = %q, want molly", follows[0].FollowerUsername)
	}
}

// --- GetUserFollows ---

func TestHTTPGetUserFollows_PassesUserIDAndType(t *testing.T) {
	var gotQuery string
	c := newClient(t, loginHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		writeOKWithCursor(t, w, []map[string]any{}, "")
	})))
	c.Login("u@example.com", "pass")

	_, _, err := c.GetUserFollows("user-99", "following", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(gotQuery, "userId=user-99") {
		t.Errorf("query = %q, want userId=user-99", gotQuery)
	}
	if !strings.Contains(gotQuery, "type=following") {
		t.Errorf("query = %q, want type=following", gotQuery)
	}
}

// --- GetUserPosts ---

func TestHTTPGetUserPosts_BuildsCorrectPath(t *testing.T) {
	var gotPath string
	c := newClient(t, loginHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		writeOKWithCursor(t, w, []map[string]any{
			{
				"postId": "p1", "authorId": "u1", "authorUsername": "neuromancer",
				"content": "hello", "topics": []string{}, "repliesCount": 0,
				"bookmarksCount": 0, "isPublic": true, "isNSFW": false,
				"deleted": false, "createdAt": "2025-01-01T12:00:00.000Z",
			},
		}, "")
	})))
	c.Login("u@example.com", "pass")

	posts, _, err := c.GetUserPosts("neuromancer", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/v1/users/neuromancer/posts" {
		t.Errorf("path = %q, want /v1/users/neuromancer/posts", gotPath)
	}
	if len(posts) != 1 {
		t.Fatalf("len(posts) = %d, want 1", len(posts))
	}
}

// --- GetUserReplies ---

func TestHTTPGetUserReplies_BuildsCorrectPath(t *testing.T) {
	var gotPath string
	c := newClient(t, loginHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		writeOKWithCursor(t, w, []map[string]any{
			{
				"replyId": "r1", "postId": "p1", "authorId": "u1",
				"authorUsername": "neuromancer", "content": "a reply",
				"parentReplyId": "", "createdAt": "2025-01-01T12:00:00.000Z",
			},
		}, "")
	})))
	c.Login("u@example.com", "pass")

	replies, _, err := c.GetUserReplies("neuromancer", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/v1/users/neuromancer/replies" {
		t.Errorf("path = %q, want /v1/users/neuromancer/replies", gotPath)
	}
	if len(replies) != 1 {
		t.Fatalf("len(replies) = %d, want 1", len(replies))
	}
	if replies[0].AuthorUsername != "neuromancer" {
		t.Errorf("AuthorUsername = %q, want neuromancer", replies[0].AuthorUsername)
	}
}
