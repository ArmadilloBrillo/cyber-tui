# 09 — RTDB C-Mail

Private 1-on-1 messaging (C-Mail) backed by Firebase Realtime Database.

---

## Architecture

C-Mail uses Firebase RTDB, not the cyberspace.online REST API. The `rtdbToken` returned by `POST /v1/auth/login` is used to authenticate all RTDB requests.

```
Login → rtdbToken → ParseRTDBToken → project ID → RTDB base URL
                                                  ↓
                                         internal/rtdb.Client
                                          ├── Get (one-shot)
                                          ├── Put (write)
                                          └── Subscribe (SSE stream)
```

The RTDB base URL is derived automatically from the JWT payload — no additional configuration is required.

---

## Package: `internal/rtdb`

### `jwt.go`

```go
func ParseRTDBToken(token string) (projectID string, err error)
func BaseURL(projectID string) string
```

Decodes the Firebase JWT payload (base64, no signature verification) and extracts the `aud` claim (Firebase project ID). `BaseURL` appends `-default-rtdb.firebaseio.com`.

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

| Operation | Path |
|---|---|
| List conversations | `/user_conversations/<userId>` |
| Fetch message history | `/dm_messages/<conversationId>` |
| Send message | `/dm_messages/<conversationId>/<msgId>` |
| Live stream | `/dm_messages/<conversationId>` (SSE) |

### ⚠️ Conversation listing — discovery note

The path `/user_conversations/<userId>` is our best guess based on common Firebase DM patterns. The API reference does not document this path explicitly. If it returns `null` or an unexpected shape, `GetConversations` returns an empty slice (not an error) and logs the raw response when `CYBERSPACE_DEBUG=1` is set.

If the path is wrong, adjust `GetConversations` in `internal/api/client.go` once the correct path is confirmed with the API team.

---

## DM Wire Format

**Send (PUT):**
```json
{
  "senderId":       "uid-abc",
  "senderUsername": "molly",
  "content":        "hello",
  "timestamp":      { ".sv": "timestamp" },
  "read":           false
}
```

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
  → CMailModel emits SelectConvMsg{ConversationID}
  → App.Update catches SelectConvMsg
      ├── cancels any existing dmSub
      ├── fires loadConvMessagesCmd (history)
      └── fires openDMSubscriptionCmd (SSE stream)

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
4. Select a conversation with Enter — message history loads in the right pane.
5. Type a message and press Enter — it is sent via RTDB PUT.
6. On another client/browser, open the same conversation — observe the message arrives.
7. Send a reply from the other client — it should appear live in the TUI without refreshing.
8. Press `1` or another tab key — subscription is cleanly cancelled.

---

## Debug Mode

Set `CYBERSPACE_DEBUG=1` to print raw RTDB responses for unknown shapes:

```bash
CYBERSPACE_DEBUG=1 go run ./cmd/cyber-tui
```
