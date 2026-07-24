// Package rtdb provides a minimal Firebase Realtime Database client using
// the Firebase REST API and Server-Sent Events (SSE) for live streaming.
// It has no knowledge of Bubble Tea or application model types.
package rtdb

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// maxResponseBytes caps how much of a one-shot REST response body is read into
// memory, guarding against an oversized body from a compromised endpoint.
const maxResponseBytes = 10 << 20 // 10 MiB

// defaultIdleTimeout is how long a Subscribe stream may go without receiving
// any line — an event, data, or a raw ":"-prefixed keepalive comment — before
// it's treated as dead and torn down (the caller sees a terminal SSEEvent.Err
// and can reconnect). Guards against a connection the server never cleanly
// closes, e.g. on auth token expiry.
const defaultIdleTimeout = 10 * time.Minute

// connectTimeout bounds how long Subscribe waits to receive response headers
// before giving up, so a connect attempt can't hang forever on a dead network.
const connectTimeout = 30 * time.Second

// SSEEvent is a single Server-Sent Event received from the RTDB stream.
// Event is "put", "patch", "cancel", or "auth_revoked". The latter two are
// terminal — the server is ending the stream — and are surfaced with Err set
// rather than dispatched as ordinary data events. Err is also set (with an
// empty Event) when the stream goes idle past the configured timeout or hits
// a network error; in every Err case the channel is closed immediately after.
type SSEEvent struct {
	Event string
	Data  []byte
	Err   error
}

// Client is a low-level Firebase RTDB HTTP client.
type Client struct {
	baseURL      string
	mu           sync.RWMutex
	token        string
	idleTO       time.Duration // 0 = use defaultIdleTimeout
	httpClient   *http.Client  // short-timeout client for Get/Put
	streamClient *http.Client  // long-lived client for SSE streams
}

// New creates a Client for the given RTDB base URL and auth token.
func New(baseURL, token string) *Client {
	streamTransport := http.DefaultTransport.(*http.Transport).Clone()
	streamTransport.ResponseHeaderTimeout = connectTimeout
	return &Client{
		baseURL:      strings.TrimRight(baseURL, "/"),
		token:        token,
		httpClient:   &http.Client{Timeout: 15 * time.Second},
		streamClient: &http.Client{Timeout: 0, Transport: streamTransport},
	}
}

// NewForTesting creates a Client backed by a custom http.Client (e.g. httptest).
// The same client is used for both regular and streaming requests.
func NewForTesting(baseURL, token string, hc *http.Client) *Client {
	return &Client{
		baseURL:      strings.TrimRight(baseURL, "/"),
		token:        token,
		httpClient:   hc,
		streamClient: hc,
	}
}

// Get performs a one-shot GET to <baseURL><path>.json?auth=<token>&<params>.
// Returns the raw response body on HTTP 200.
func (c *Client) Get(ctx context.Context, path string, params url.Values) ([]byte, error) {
	u := c.buildURL(path, params)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("rtdb: build GET request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rtdb: GET %s: %w", path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("rtdb: read GET response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rtdb: GET %s returned %d: %s", path, resp.StatusCode, body)
	}

	return body, nil
}

// Put marshals val to JSON and writes it via PUT to <baseURL><path>.json?auth=<token>.
func (c *Client) Put(ctx context.Context, path string, val any) error {
	body, err := json.Marshal(val)
	if err != nil {
		return fmt.Errorf("rtdb: marshal PUT body: %w", err)
	}

	u := c.buildURL(path, nil)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("rtdb: build PUT request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("rtdb: PUT %s: %w", path, err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("rtdb: PUT %s returned %d", path, resp.StatusCode)
	}

	return nil
}

// Subscribe opens an SSE stream to <baseURL><path>.json?auth=<token>&<params>.
// Returns immediately; events are sent on the returned channel.
// The channel is closed when ctx is cancelled, the server closes the stream,
// the server sends a terminal event ("auth_revoked"/"cancel"), or the stream
// goes idle past the configured timeout. A terminal error is sent as
// SSEEvent.Err before the channel is closed in every case but caller
// cancellation.
func (c *Client) Subscribe(ctx context.Context, path string, params url.Values) <-chan SSEEvent {
	ch := make(chan SSEEvent, 8)

	go func() {
		defer close(ch)

		// A private child context lets the idle watchdog (in readSSE) abort
		// this one request — unblocking a Read() stuck on a zombie
		// connection — without reaching into the caller's context.
		streamCtx, streamCancel := context.WithCancel(ctx)
		defer streamCancel()

		u := c.buildURL(path, params)
		req, err := http.NewRequestWithContext(streamCtx, http.MethodGet, u, nil)
		if err != nil {
			ch <- SSEEvent{Err: fmt.Errorf("rtdb: build SSE request: %w", err)}
			return
		}
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("Cache-Control", "no-cache")

		resp, err := c.streamClient.Do(req)
		if err != nil {
			if streamCtx.Err() != nil {
				return // context cancelled — not an error
			}
			ch <- SSEEvent{Err: fmt.Errorf("rtdb: SSE connect %s: %w", path, err)}
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			ch <- SSEEvent{Err: fmt.Errorf("rtdb: SSE %s returned %d", path, resp.StatusCode)}
			return
		}

		c.readSSE(streamCtx, streamCancel, resp.Body, ch)
	}()

	return ch
}

// readSSE reads SSE events from r and sends them on ch until EOF, a read
// error, a terminal server event ("auth_revoked"/"cancel"), an idle timeout,
// or ctx cancellation. cancel aborts the in-flight request (unblocking a
// Read() the scanning goroutine below may be stuck in) when the idle
// watchdog fires.
func (c *Client) readSSE(ctx context.Context, cancel context.CancelFunc, r io.Reader, ch chan<- SSEEvent) {
	// Scanning happens on its own goroutine because bufio.Scanner.Scan can
	// block indefinitely inside Read with no way to select on it directly;
	// routing lines through a channel lets the loop below race that against
	// an idle timer and ctx cancellation.
	lines := make(chan string)
	scanErr := make(chan error, 1)
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			select {
			case lines <- scanner.Text():
			case <-ctx.Done():
				scanErr <- nil
				return
			}
		}
		scanErr <- scanner.Err()
	}()

	idleTO := c.idleTimeout()
	idle := time.NewTimer(idleTO)
	defer idle.Stop()

	var eventType string
	var dataLines []string

	for {
		select {
		case <-ctx.Done():
			return

		case <-idle.C:
			cancel()
			ch <- SSEEvent{Err: fmt.Errorf("rtdb: SSE stream idle for over %s", idleTO)}
			return

		case line, ok := <-lines:
			if !ok {
				// The scanning goroutine always sends exactly one value to
				// scanErr before closing lines, on every exit path.
				if err := <-scanErr; err != nil && ctx.Err() == nil {
					ch <- SSEEvent{Err: fmt.Errorf("rtdb: SSE read error: %w", err)}
				}
				return
			}

			// Any line — including a discarded ":" comment — is stream
			// activity and resets the idle watchdog.
			if !idle.Stop() {
				select {
				case <-idle.C:
				default:
				}
			}
			idle.Reset(idleTO)

			if line == "" {
				// Blank line — dispatch accumulated event.
				switch eventType {
				case "auth_revoked", "cancel":
					ch <- SSEEvent{
						Event: eventType,
						Data:  []byte(strings.Join(dataLines, "\n")),
						Err:   fmt.Errorf("rtdb: SSE stream terminated by server (%s)", eventType),
					}
					return
				default:
					if eventType != "" && len(dataLines) > 0 {
						ch <- SSEEvent{
							Event: eventType,
							Data:  []byte(strings.Join(dataLines, "\n")),
						}
					}
				}
				eventType = ""
				dataLines = dataLines[:0]
				continue
			}

			if rest, ok := strings.CutPrefix(line, "event:"); ok {
				eventType = strings.TrimSpace(rest)
			} else if rest, ok := strings.CutPrefix(line, "data:"); ok {
				dataLines = append(dataLines, strings.TrimSpace(rest))
			}
			// Ignore comments (":") and other fields.
		}
	}
}

// idleTimeout returns the configured idle-read watchdog duration, falling
// back to defaultIdleTimeout unless overridden via SetIdleTimeoutForTesting.
func (c *Client) idleTimeout() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.idleTO > 0 {
		return c.idleTO
	}
	return defaultIdleTimeout
}

// SetIdleTimeoutForTesting overrides the SSE idle-read watchdog duration.
// Test use only.
func (c *Client) SetIdleTimeoutForTesting(d time.Duration) {
	c.mu.Lock()
	c.idleTO = d
	c.mu.Unlock()
}

// SetToken replaces the auth token used for future connections — one-shot
// Get/Put calls pick it up immediately (buildURL reads it live), but it has
// no effect on an already-open Subscribe stream: the token is fixed in that
// stream's URL at connect time, and Firebase doesn't re-read a mutated
// in-memory value. Reviving a live stream after a refresh requires tearing
// it down and calling Subscribe again with the new token.
func (c *Client) SetToken(token string) {
	c.mu.Lock()
	c.token = token
	c.mu.Unlock()
}

// buildURL constructs the full Firebase REST URL including the .json suffix and auth token.
func (c *Client) buildURL(path string, params url.Values) string {
	if params == nil {
		params = url.Values{}
	}
	c.mu.RLock()
	tok := c.token
	c.mu.RUnlock()
	params.Set("auth", tok)

	path = strings.TrimRight(path, "/")
	return c.baseURL + path + ".json?" + params.Encode()
}
