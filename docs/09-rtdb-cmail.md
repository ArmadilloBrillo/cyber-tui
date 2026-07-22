# 09 — RTDB C-Mail

Private 1-on-1 messaging (C-Mail) backed by Firebase Realtime Database.

---

## Architecture

C-Mail uses a hybrid of REST and Firebase RTDB. The REST API handles listing, history, and sending; RTDB SSE delivers new messages in real time.

```
Login → idToken  → REST /v1/cmail/*  (list, history, send, mark-read)
      → rtdbUrl  → internal/rtdb.Client
                       └── Subscribe (SSE stream for live messages only)
```

The RTDB base URL is returned directly in the login response (`rtdbUrl` field) — it is no longer derived from the JWT. The `idToken` is passed as `?auth=<idToken>` on all RTDB requests.

---

## Package: `internal/rtdb`

### `jwt.go`

```go
func ParseRTDBToken(token string) (projectID string, err error)
func BaseURL(projectID string) string
```

Decodes the Firebase JWT payload (base64, no signature verification) and extracts the `aud` claim (Firebase project ID). `BaseURL` appends `-default-rtdb.firebaseio.com`. These helpers are retained for compatibility but the RTDB URL is now taken from the `rtdbUrl` field in the login response (which uses a regional domain rather than the default `firebaseio.com` domain — see resolved issue in `docs/00-api-backlog.md`).

### `client.go`

```go
type Client struct { ... }

func New(baseURL, token string) *Client
func NewForTesting(baseURL, token string, hc *http.Client) *Client

func (c *Client) Get(ctx, path, params) ([]byte, error)
func (c *Client) Put(ctx, path string, val any) error
func (c *Client) Subscribe(ctx, path string, params) <-chan SSEEvent
```

**SSE mechanics:**
- `Subscribe` opens an HTTP connection with `Accept: text/event-stream`.
- A goroutine reads line-by-line, accumulates `event:` / `data:` lines, dispatches on blank line.
- The channel is closed when the context is cancelled or the server closes the stream.
- A `streamClient` with `Timeout: 0` is used for streaming; a 15-second `httpClient` is used for Get/Put.

---

## RTDB Paths

| Operation | Path | Notes |
|---|---|---|
| Live stream | `/dm_messages/<conversationId>` (SSE) | Only RTDB path still in use |

All other C-Mail operations (list conversations, load history, send message, mark read) are now handled by the REST API at `/v1/cmail/*`. See `docs/08-cmail.md` for the full endpoint table.

---

## DM Wire Format

**Receive (SSE `put` event):**
```json
{
  "path": "/msgId123",
  "data": {
    "senderId":       "uid-abc",
    "senderUsername": "molly",
    "content":        "hello",
    "timestamp":      1700000001000,
    "read":           false
  }
}
```

The initial SSE event has `path: "/"` and carries the full snapshot. This is skipped by the translator — history is loaded separately via `GetMessages`.

---

## Bubble Tea Subscription Lifecycle

```
User presses Enter on conversation list
  → CMailModel emits CMailConvSelectedMsg{ConversationID} + loadConvMessagesCmd + openDMSubscriptionCmd
  → App.Update catches CMailConvSelectedMsg → calls markCMailReadCmd(convID) via REST
  → loadConvMessagesCmd fetches history via REST GET /v1/cmail/:id
  → openDMSubscriptionCmd starts RTDB SSE stream

openDMSubscriptionCmd resolves
  → dmSubscribedMsg{convID, sub}
  → stale guard: if convID != activeConvID, cancel and discard
  → store a.dmSub, fire waitForDM(sub)

waitForDM blocks on sub.C
  → dmReceivedMsg{msg} → AppendMessage → re-fire waitForDM
  → dmStreamClosedMsg   → clear a.dmSub

Navigating away (keys 1/2/4, arrow tabs, or re-pressing 3)
  → cancelDMSubscription() → sub.cancel() → goroutine exits → channel closes
```

---

## Initialisation

After login, `loginCmd` in `app.go` calls:

```go
if hc, ok := a.client.(*api.HTTPClient); ok {
    _ = hc.InitRTDB(tokens.RTDBToken)
    hc.SetCurrentUID(user.ID)
}
```

`MockClient` does not need `InitRTDB` — it implements all DM methods with static fake data.

---

## Manual Smoke Test

```bash
go run ./cmd/cyber-tui
```

1. Log in (auto-login from `.env` works).
2. Press `3` to navigate to C-Mail.
3. Conversation list should populate (or show empty if no conversations yet).
4. Select a conversation with Enter — message history loads via REST; unread count clears.
5. Type a message and press Enter — it is sent via REST POST.
6. On another client/browser, open the same conversation — observe the message arrives.
7. Send a reply from the other client — it should appear live in the TUI without refreshing.
8. Press `1` or another tab key — subscription is cleanly cancelled.

---

## Debug Mode

Set `CYBERSPACE_DEBUG=1` to print raw RTDB responses for unknown shapes:

```bash
CYBERSPACE_DEBUG=1 go run ./cmd/cyber-tui
```
