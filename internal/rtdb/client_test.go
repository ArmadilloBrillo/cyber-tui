package rtdb_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ragnar/cyber-tui/internal/rtdb"
)

// --- JWT tests ---

func makeJWT(claims map[string]any) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	payload, _ := json.Marshal(claims)
	mid := base64.RawURLEncoding.EncodeToString(payload)
	return header + "." + mid + ".fakesig"
}

func TestParseRTDBToken_Valid(t *testing.T) {
	token := makeJWT(map[string]any{"aud": "my-project-123", "sub": "uid1"})
	got, err := rtdb.ParseRTDBToken(token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "my-project-123" {
		t.Errorf("projectID = %q, want %q", got, "my-project-123")
	}
}

func TestParseRTDBToken_MalformedToken(t *testing.T) {
	cases := []struct{ name, token string }{
		{"empty", ""},
		{"one part", "abc"},
		{"two parts", "abc.def"},
		{"bad base64", "abc.!!!.sig"},
		{"bad JSON", "abc." + base64.RawURLEncoding.EncodeToString([]byte(`notjson`)) + ".sig"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := rtdb.ParseRTDBToken(tc.token)
			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestParseRTDBToken_MissingAud(t *testing.T) {
	token := makeJWT(map[string]any{"sub": "uid1"})
	_, err := rtdb.ParseRTDBToken(token)
	if err == nil {
		t.Error("expected error for missing aud")
	}
}

func TestParseRTDBToken_RejectsHostInjectingAud(t *testing.T) {
	for _, aud := range []string{
		"evil.com/",
		"proj.firebaseio.com",
		"a@b",
		"proj id",
		"proj:8080",
		"../proj",
	} {
		token := makeJWT(map[string]any{"aud": aud, "sub": "uid1"})
		if _, err := rtdb.ParseRTDBToken(token); err == nil {
			t.Errorf("expected error for aud %q, got nil", aud)
		}
	}
}

func TestBaseURL(t *testing.T) {
	got := rtdb.BaseURL("my-project")
	want := "https://my-project-default-rtdb.firebaseio.com"
	if got != want {
		t.Errorf("BaseURL = %q, want %q", got, want)
	}
}

// --- Get tests ---

func TestGet_Success(t *testing.T) {
	want := `{"key":"value"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("auth") == "" {
			t.Error("missing auth param")
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, want)
	}))
	defer srv.Close()

	c := rtdb.NewForTesting(srv.URL, "tok", srv.Client())
	body, err := c.Get(context.Background(), "/some/path", nil)
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if string(body) != want {
		t.Errorf("body = %q, want %q", body, want)
	}
}

func TestGet_AuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":"Permission denied"}`)
	}))
	defer srv.Close()

	c := rtdb.NewForTesting(srv.URL, "bad-tok", srv.Client())
	_, err := c.Get(context.Background(), "/path", nil)
	if err == nil {
		t.Error("expected error on 401, got nil")
	}
}

// --- Put tests ---

func TestPut_Success(t *testing.T) {
	type payload struct {
		Content string `json:"content"`
	}

	var captured payload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %q, want PUT", r.Method)
		}
		if r.URL.Query().Get("auth") == "" {
			t.Error("missing auth param")
		}
		json.NewDecoder(r.Body).Decode(&captured)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"content":"hello"}`)
	}))
	defer srv.Close()

	c := rtdb.NewForTesting(srv.URL, "tok", srv.Client())
	err := c.Put(context.Background(), "/dm_messages/conv1/msg1", payload{Content: "hello"})
	if err != nil {
		t.Fatalf("Put error: %v", err)
	}
	if captured.Content != "hello" {
		t.Errorf("captured.Content = %q, want %q", captured.Content, "hello")
	}
}

func TestPut_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c := rtdb.NewForTesting(srv.URL, "tok", srv.Client())
	err := c.Put(context.Background(), "/path", map[string]string{"x": "y"})
	if err == nil {
		t.Error("expected error on 403, got nil")
	}
}

// --- Subscribe/SSE tests ---

func writeSSEEvent(w http.ResponseWriter, event, data string) {
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func TestSubscribe_ReceivesEvents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		writeSSEEvent(w, "put", `{"path":"/","data":{"msg1":{"content":"hello"}}}`)
		writeSSEEvent(w, "put", `{"path":"/msg2","data":{"content":"world"}}`)
		// Close the stream.
	}))
	defer srv.Close()

	c := rtdb.NewForTesting(srv.URL, "tok", srv.Client())
	ch := c.Subscribe(context.Background(), "/dm_messages/conv1", nil)

	var events []rtdb.SSEEvent
	for e := range ch {
		events = append(events, e)
	}

	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	if events[0].Event != "put" {
		t.Errorf("events[0].Event = %q, want put", events[0].Event)
	}
	if events[1].Event != "put" {
		t.Errorf("events[1].Event = %q, want put", events[1].Event)
	}
}

func TestSubscribe_CancelContext(t *testing.T) {
	done := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		// Block until client disconnects.
		<-r.Context().Done()
		close(done)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	c := rtdb.NewForTesting(srv.URL, "tok", srv.Client())
	ch := c.Subscribe(ctx, "/path", nil)

	// Cancel after a short delay.
	time.AfterFunc(50*time.Millisecond, cancel)

	// Channel should close.
	select {
	case _, open := <-ch:
		if open {
			// Drain any events before checking closed.
		}
	case <-time.After(2 * time.Second):
		t.Fatal("channel did not close after context cancel")
	}

	// Drain fully.
	for range ch {
	}
}

func TestSubscribe_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := rtdb.NewForTesting(srv.URL, "tok", srv.Client())
	ch := c.Subscribe(context.Background(), "/path", nil)

	var errEvent *rtdb.SSEEvent
	for e := range ch {
		if e.Err != nil {
			errEvent = &e
		}
	}
	if errEvent == nil {
		t.Error("expected an error SSEEvent, got none")
	}
}
