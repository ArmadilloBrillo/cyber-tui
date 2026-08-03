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
- The channel is closed — with a terminal `SSEEvent.Err` set — when: the context is cancelled, the server closes the stream, the server sends a terminal `auth_revoked`/`cancel` event, or the stream goes **idle for 10 minutes** with no line received (any line, including a discarded `:`-prefixed keepalive comment, resets the idle timer). This idle watchdog exists because the server doesn't always cleanly close the TCP connection when the auth token expires — without it, a zombie stream could otherwise hang forever with no signal to reconnect.
- The connect phase (waiting for response headers) is separately bounded by a **30-second `ResponseHeaderTimeout`** on the streaming transport, so a single connect attempt can't hang indefinitely on a dead network.
- A `streamClient` with `Timeout: 0` (but the header timeout above) is used for streaming; a 15-second `httpClient` is used for Get/Put.
- `SetToken` only affects *future* connections — it cannot revive an already-open SSE stream, since the token is fixed in that stream's URL at connect time. See `docs/08-cmail.md` / `docs/33-circ.md` for how the screens layer handles reconnecting a live stream after a token refresh.

---

## RTDB Paths

| Operation | Path | Notes |
|---|---|---|
| Live stream | `/dm_messages/<conversationId>` (SSE) | New messages |
| Typing indicator | `/dm_presence/<conversationId>` (SSE) | Who's currently typing; same full-snapshot-per-event shape as CIRC's `/chat_presence/<roomId>` |
| Conversation list | `/user_conversations/<uid>` (SSE) | Account-wide list of conversation summaries (unread count, last message, other participant); same full-snapshot-per-event shape, keyed by `conversationId` instead of `userId` |

All other C-Mail operations (list conversations, load history, send message, mark read, announce/clear typing) are handled by the REST API at `/v1/cmail/*`. See `docs/08-cmail.md` for the full endpoint table.

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

**Typing wire format** — one entry in `/dm_presence/<conversationId>`, keyed by userId:
```json
{
  "path": "/uid-abc",
  "data": {
    "username":  "molly",
    "typing":    true,
    "timestamp": 1700000001000
  }
}
```
Unlike the message stream, the initial `path: "/"` snapshot **is** consumed (not skipped) — it's the only way to know who's typing when the conversation is first opened. An entry is shown only while `typing == true` and `timestamp` is newer than `staleAfterMs` (9000ms); since a flag going stale produces no RTDB event of its own, the client re-filters on both every event and a 5s ticker — identical caveat to CIRC's `chat_presence` handling.

**Conversation-list wire format** — one entry in `/user_conversations/<uid>`, keyed by `conversationId` (discovered live, not documented in `docs/00-latest-api-reference.md` beyond the path and SSE mechanics — see `docs/00-api-backlog.md`):
```json
{
  "path": "/c1a2b3",
  "data": {
    "otherUserId":   "uid-abc",
    "otherUsername": "molly",
    "lastMessage":   "hello",
    "lastMessageAt": 1700000001000,
    "unreadCount":   1
  }
}
```
Every field but `unreadCount` is absent on some entries (older/stale conversations); the client falls back to "unknown" for the participant, same as it does for a REST-loaded conversation with no participant info. Like `chat_presence`/`dm_presence`, the initial `path: "/"` snapshot **is** consumed (it's the only source of the conversation list's live state — no separate ticker is needed here, since nothing about a conversation summary goes stale on its own the way presence/typing does).

Unlike presence/typing — where every write rewrites a participant's whole entry — a conversation summary write is often partial: a new message typically only touches `lastMessage`/`lastMessageAt`/`unreadCount`, not `otherUserId`/`otherUsername`. So a `patch` at a single `/<conversationId>` path is *merged* onto the existing entry (Go's `json.Unmarshal` onto a non-zero struct only sets fields present in the JSON, leaving the rest untouched) rather than replacing it outright — only a `put` (root or single-path) is a full replace. Root-level `patch` (multiple conversations changing in one multi-location update) still merges per-key like `applyPresenceEvent`.

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

**Account-wide conversation-list subscription** (independent of the flow above — not tied to any single conversation being open):

```
Login succeeds
  → afterLoginCmd calls CMailModel.OpenUserConvsSubscription() directly — no REST
    seed; the subscription's own first event (a full snapshot, like chat_presence's)
    populates the list, so there's exactly one writer to m.conversations, not two
  → openUserConvsSubscriptionCmd starts RTDB SSE stream on /user_conversations/<uid>,
    seeded with m.conversations (nil on a fresh login; the last known-good list on
    a reconnect)

openUserConvsSubscriptionCmd resolves
  → userConvsSubscribedMsg{sub} → store m.userConvsSub, fire waitForUserConvs(sub)

waitForUserConvs blocks on sub.C
  → userConvsReceivedMsg{convs} → SetConversations(convs) → re-fire waitForUserConvs
  → userConvsStreamClosedMsg    → reconnect sequence (RefreshSession + resubscribe,
                                   1s/2s/4s/8s/15s backoff, 6 attempts, same schedule
                                   as the message stream) — gives up silently (no UI
                                   indicator) if all attempts fail

Session ends (handleUnauthorized)
  → CancelUserConvsSubscription() → sub.cancel() → goroutine exits → channel closes
```

Routed via the same background-delivery path as the message/typing streams (`screens.IsDMStreamMsg`, checked in `App.handleCMail` when C-Mail isn't the active screen) — this is what lets the tab-bar unread badge update live regardless of which tab is on screen.

---

## Initialisation

After login, `loginCmd` in `app.go` calls:

```go
if hc, ok := a.client.(*api.HTTPClient); ok {
    _ = hc.InitRTDB(tokens.IDToken, tokens.RTDBUrl)
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
9. From the conversation list (or another tab entirely), have the other client send a new message — the tab-bar unread badge and the conversation's card (unread count, preview, timestamp) should update within ~1s via the live `user_conversations/<uid>` stream, with no 60s poll delay and no manual refresh.

---

## Debug Mode

Set `CYBERSPACE_DEBUG=1` to print raw RTDB responses for unknown shapes:

```bash
CYBERSPACE_DEBUG=1 go run ./cmd/cyber-tui
```

**`CYBERSPACE_DEBUG_KEYS`** (general-purpose, not RTDB-specific): logs every raw `tea.KeyMsg` (the string bubbletea assigns it, its `KeyType`, and its runes) to `cyber-tui-keys.log` via a `tea.WithFilter` observer in `cmd/cyber-tui/main.go`. Useful for diagnosing terminal-specific keybinding quirks — e.g. confirming exactly what byte sequence a given terminal sends for a ctrl-combo — without instrumenting app logic:

```bash
CYBERSPACE_DEBUG_KEYS=1 go run ./cmd/cyber-tui
```
