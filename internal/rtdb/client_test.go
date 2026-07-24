package rtdb_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ragnar/cyber-tui/internal/rtdb"
)

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

// --- SetToken tests ---

func TestSetToken_UpdatesAuthParam(t *testing.T) {
	var lastAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastAuth = r.URL.Query().Get("auth")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"k":"v"}`)
	}))
	defer srv.Close()

	c := rtdb.NewForTesting(srv.URL, "old-token", srv.Client())

	if _, err := c.Get(context.Background(), "/path", nil); err != nil {
		t.Fatal(err)
	}
	if lastAuth != "old-token" {
		t.Errorf("initial auth = %q, want old-token", lastAuth)
	}

	c.SetToken("new-token")

	if _, err := c.Get(context.Background(), "/path", nil); err != nil {
		t.Fatal(err)
	}
	if lastAuth != "new-token" {
		t.Errorf("after SetToken auth = %q, want new-token", lastAuth)
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

// --- Terminal server events ---

func TestSubscribe_AuthRevokedClosesChannelWithErr(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		writeSSEEvent(w, "put", `{"path":"/msg1","data":{"content":"hi"}}`)
		writeSSEEvent(w, "auth_revoked", `null`)
		// Server would normally keep the connection open past this point in
		// practice; block to prove the client doesn't need a close to react.
		<-r.Context().Done()
	}))
	defer srv.Close()

	c := rtdb.NewForTesting(srv.URL, "tok", srv.Client())
	ch := c.Subscribe(context.Background(), "/dm_messages/conv1", nil)

	var events []rtdb.SSEEvent
	for e := range ch {
		events = append(events, e)
	}

	if len(events) != 2 {
		t.Fatalf("got %d events, want 2 (1 put + 1 terminal)", len(events))
	}
	if events[0].Err != nil {
		t.Errorf("events[0].Err = %v, want nil", events[0].Err)
	}
	last := events[len(events)-1]
	if last.Event != "auth_revoked" || last.Err == nil {
		t.Errorf("last event = %+v, want auth_revoked with Err set", last)
	}
}

func TestSubscribe_CancelEventClosesChannelWithErr(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		writeSSEEvent(w, "cancel", `null`)
		<-r.Context().Done()
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
	if errEvent == nil || errEvent.Event != "cancel" {
		t.Fatalf("expected a cancel SSEEvent with Err set, got %+v", errEvent)
	}
}

// --- Idle watchdog ---

func TestSubscribe_IdleTimeoutClosesChannel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		writeSSEEvent(w, "put", `{"path":"/msg1","data":{"content":"hi"}}`)
		// Go quiet without closing â€” simulates a connection the server
		// never cleanly tears down after the auth token expires.
		<-r.Context().Done()
	}))
	defer srv.Close()

	c := rtdb.NewForTesting(srv.URL, "tok", srv.Client())
	c.SetIdleTimeoutForTesting(50 * time.Millisecond)
	ch := c.Subscribe(context.Background(), "/path", nil)

	var errEvent *rtdb.SSEEvent
	done := make(chan struct{})
	go func() {
		defer close(done)
		for e := range ch {
			if e.Err != nil {
				ee := e
				errEvent = &ee
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("channel did not close after idle timeout")
	}
	if errEvent == nil {
		t.Error("expected an idle-timeout SSEEvent.Err, got none")
	}
}

func TestSubscribe_IdleTimeoutResetByActivity(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		for range 4 {
			writeSSEEvent(w, "put", `{"path":"/msg","data":{"content":"hi"}}`)
			if flusher != nil {
				flusher.Flush()
			}
			time.Sleep(30 * time.Millisecond)
		}
		// Clean close â€” no idle timeout should have fired given 30ms gaps
		// against a much larger idle window below.
	}))
	defer srv.Close()

	c := rtdb.NewForTesting(srv.URL, "tok", srv.Client())
	c.SetIdleTimeoutForTesting(200 * time.Millisecond)
	ch := c.Subscribe(context.Background(), "/path", nil)

	var events []rtdb.SSEEvent
	for e := range ch {
		events = append(events, e)
	}
	for _, e := range events {
		if e.Err != nil {
			t.Errorf("unexpected error event, idle timeout should have been reset by activity: %v", e.Err)
		}
	}
	if len(events) != 4 {
		t.Fatalf("got %d events, want 4", len(events))
	}
}

func TestSubscribe_IdleTimeoutResetByCommentLines(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		for range 4 {
			fmt.Fprint(w, ": keepalive\n\n")
			if flusher != nil {
				flusher.Flush()
			}
			time.Sleep(30 * time.Millisecond)
		}
		// Clean close â€” comment-only traffic should still count as activity.
	}))
	defer srv.Close()

	c := rtdb.NewForTesting(srv.URL, "tok", srv.Client())
	c.SetIdleTimeoutForTesting(200 * time.Millisecond)
	ch := c.Subscribe(context.Background(), "/path", nil)

	var events []rtdb.SSEEvent
	for e := range ch {
		events = append(events, e)
	}
	for _, e := range events {
		if e.Err != nil {
			t.Errorf("unexpected error event, idle timeout should have been reset by comment lines: %v", e.Err)
		}
	}
}

// --- Connect-phase timeout ---

func TestSubscribe_ConnectTimeout(t *testing.T) {
	accepted := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(accepted)
		<-r.Context().Done() // never write a header
	}))
	defer srv.Close()

	transport := &http.Transport{ResponseHeaderTimeout: 50 * time.Millisecond}
	hc := &http.Client{Timeout: 0, Transport: transport}
	c := rtdb.NewForTesting(srv.URL, "tok", hc)
	ch := c.Subscribe(context.Background(), "/path", nil)

	select {
	case <-accepted:
	case <-time.After(2 * time.Second):
		t.Fatal("server never received the request")
	}

	select {
	case e, ok := <-ch:
		if !ok {
			t.Fatal("channel closed with no error event")
		}
		if e.Err == nil {
			t.Error("expected a connect-timeout SSEEvent.Err, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Subscribe did not surface a connect timeout in time")
	}
}
