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
	"time"
)

// SSEEvent is a single Server-Sent Event received from the RTDB stream.
type SSEEvent struct {
	Event string // "put", "patch", "cancel", "auth_revoked"
	Data  []byte // raw JSON payload
	Err   error  // non-nil only on terminal error before channel close
}

// Client is a low-level Firebase RTDB HTTP client.
type Client struct {
	baseURL      string
	token        string
	httpClient   *http.Client // short-timeout client for Get/Put
	streamClient *http.Client // zero-timeout client for SSE streams
}

// New creates a Client for the given RTDB base URL and auth token.
func New(baseURL, token string) *Client {
	return &Client{
		baseURL:      strings.TrimRight(baseURL, "/"),
		token:        token,
		httpClient:   &http.Client{Timeout: 15 * time.Second},
		streamClient: &http.Client{Timeout: 0},
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

	body, err := io.ReadAll(resp.Body)
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
// The channel is closed when ctx is cancelled or the server closes the stream.
// A terminal error is sent as SSEEvent.Err before the channel is closed.
func (c *Client) Subscribe(ctx context.Context, path string, params url.Values) <-chan SSEEvent {
	ch := make(chan SSEEvent, 8)

	go func() {
		defer close(ch)

		u := c.buildURL(path, params)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			ch <- SSEEvent{Err: fmt.Errorf("rtdb: build SSE request: %w", err)}
			return
		}
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("Cache-Control", "no-cache")

		resp, err := c.streamClient.Do(req)
		if err != nil {
			if ctx.Err() != nil {
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

		c.readSSE(ctx, resp.Body, ch)
	}()

	return ch
}

// readSSE reads SSE events from r and sends them on ch until EOF, error, or ctx cancel.
func (c *Client) readSSE(ctx context.Context, r io.Reader, ch chan<- SSEEvent) {
	scanner := bufio.NewScanner(r)

	var eventType string
	var dataLines []string

	for scanner.Scan() {
		if ctx.Err() != nil {
			return
		}

		line := scanner.Text()

		if line == "" {
			// Blank line — dispatch accumulated event.
			if eventType != "" && len(dataLines) > 0 {
				ch <- SSEEvent{
					Event: eventType,
					Data:  []byte(strings.Join(dataLines, "\n")),
				}
			}
			eventType = ""
			dataLines = dataLines[:0]
			continue
		}

		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
		// Ignore comments (":") and other fields.
	}

	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		ch <- SSEEvent{Err: fmt.Errorf("rtdb: SSE read error: %w", err)}
	}
}

// buildURL constructs the full Firebase REST URL including the .json suffix and auth token.
func (c *Client) buildURL(path string, params url.Values) string {
	if params == nil {
		params = url.Values{}
	}
	params.Set("auth", c.token)

	path = strings.TrimRight(path, "/")
	return c.baseURL + path + ".json?" + params.Encode()
}
