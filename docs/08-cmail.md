# 08 — C-Mail

Private 1-on-1 conversations. Accessed via tab `3` or the `c-mail` tab in the navigation bar.

---

## Layout

C-Mail uses a two-mode sequential flow:

### List mode (default)

Full-width conversation list. Each conversation is a bordered card:

```
╔═══════════════════════════════════════════════════════════╗  ← ActiveBorder when selected
║ @molly                                     2h ago   (3)  ║
║ hey, did you see the news about the ice…               ║
╚═══════════════════════════════════════════════════════════╝
╔═══════════════════════════════════════════════════════════╗
║ @wintermute                                    3d ago    ║
║ i am the one who remembers                              ║
╚═══════════════════════════════════════════════════════════╝
```

Card header: `@username` (left) + timestamp + `(N)` unread badge (right, when unread > 0).
Preview: first line of `LastMessage`, truncated to fit card width.

The C-Mail tab itself also shows an aggregate unread badge, mirroring the Notifications tab: `c-mail (N)` in the Tabs layout, `c-mail ●N` in the Miller layout, where `N` is the sum of `UnreadCount` across all conversations (`CMailModel.TotalUnread()`). The badge clears immediately (optimistically) when a conversation is opened, and refreshes from the server every 60s alongside the notifications poll.

### Detail mode

Full-width message history viewport + fixed compose input at bottom:

```
@molly                                                   ← title header row
────────────────────────────────────────────────────────
14:10  @molly   hey, did you see the news?              ← left-aligned (other)
                                     yeah, not good  ← right-aligned (me)
                                     14:12  @you
────────────────────────────────────────────────────────
╔═══════════════════════════════════════════════════════╗  ← ActiveBorder
║ compose c-mail...                                     ║
╚═══════════════════════════════════════════════════════╝
```

- Other person's messages: left-aligned (`@username  timestamp` header, then body)
- My messages: right-aligned (`timestamp  @me` header, then body)

**Scroll-to-load history**: scrolling to the top of the loaded messages (`↑`) automatically fetches the next older page (`GetMessages(conversationID, 50, before)`, `before` = the oldest loaded message's timestamp) and prepends it, preserving scroll position. The header shows `(loading history…)` while a page is in flight. Stops once a fetch returns no messages. If a fetch fails, `loadingHistory` resets so a retry is possible on the next scroll-to-top, and the viewport shows "couldn't load messages" instead of a misleading "no messages" if nothing has loaded yet.

**Live-stream reconnect**: the Firebase `idToken` backing the RTDB subscription expires hourly. When the stream closes while a conversation is still open, the app calls `api.Client.RefreshSession()` and reopens the subscription automatically, showing a brief "reconnected to live chat" notification. A single reconnect attempt is made; if it fails, the conversation is left without live updates until the user leaves and re-enters.

**Slash commands**: like CIRC, the server expands `/me`, `/poke`/`/hug`/`/hi5`/`/slap`, `/dice`, `/8ball`, and `/fortune` server-side. `/help` posts no message; its reply is captured from the send response and appended as a local-only system notice (`model.Message.IsSystem`, rendered via `renderSystemNotice` — no bubble, no border, just a muted `*** `-prefixed block). It's never sent to or stored by the server.

`/me` and other emotes set an undocumented `isAction` field on the message, discovered via live testing against CIRC (parsed defensively for C-Mail too, but not yet confirmed live there — see `docs/33-circ.md`). `model.Message.IsAction` messages render as `* username body *` (`renderActionLine` in `render.go`) instead of the usual bordered bubble — same classic-IRC treatment as CIRC.

---

## Key Bindings

### List mode

| Key | Action |
|---|---|
| `↑` / `k` | Move cursor up the conversation list |
| `↓` / `j` | Move cursor down the conversation list |
| `Enter` | Open selected conversation → switch to detail mode, focus input |

### Detail mode

| Key | Action |
|---|---|
| `Enter` | Send message (when input non-empty) |
| `↑` | Scroll message history up |
| `↓` | Scroll message history down |
| `Esc` | Return to list mode; cancel RTDB subscription |
| `ctrl+o` | Open URLs/images from the loaded conversation. Plain `o` can't reach this here — the compose input is focused for the entire detail view, so `o` always types into the message instead; `ctrl+o` is exempted from the focused-input gate specifically for this. |
| all other | Forwarded to compose input (`j`/`k` type normally) |

---

## Screen Model

**File:** `internal/ui/screens/cmail.go`

**Type:** `CMailModel`
**Constructor:** `NewCMailModel(currentUser string, client api.Client) CMailModel`
**Messages emitted:** `SendCMailMsg{ConversationID, Body}`, `CMailConvSelectedMsg{ConversationID}`, `StartConversationMsg{Username}` (from other screens)
**App field:** `a.cmail`
**Screen constant:** `screenCMail`

### Exported accessors (for testing)

| Method | Returns |
|---|---|
| `IsShowingDetail() bool` | Whether detail mode is active |
| `HasActiveConv() bool` | Alias for `IsShowingDetail()` |
| `SelectedConv() int` | Cursor index in conversation list |
| `InputFocused() bool` | True in detail mode (compose input focused) |
| `TotalUnread() int` | Sum of `UnreadCount` across all conversations, for the tab-bar badge |
| `GetFocusedURLs() []string` | URLs across all loaded messages in the open conversation (`URLProvider`); nil outside detail mode |

---

## API Integration

C-Mail uses a hybrid architecture: REST for listing, history, and sending; Firebase RTDB SSE for real-time new message delivery only.

### REST Endpoints

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/v1/cmail` | List all conversations (unread first, then newest activity) |
| `GET` | `/v1/cmail/:conversationId` | Load message history (query `?limit=N&before=<timestamp>`; `before` pages further back) |
| `POST` | `/v1/cmail` | Start or return an existing conversation (body: `{"recipientUsername": "..."}`) |
| `POST` | `/v1/cmail/:conversationId` | Send a message (body: `{"content": "..."}`) |
| `POST` | `/v1/cmail/:conversationId/read` | Mark conversation read (resets unread count) |

**Notes:**
- `POST /v1/cmail` is idempotent — returns 200 for an existing conversation, 201 for a new one.
- Rate limits: 15 sends/min, 300/day, 150/hour; 5 start/min, 50/day, 30/hour; 60 mark-read/min.
- Blocked in either direction returns 403.

### RTDB SSE Subscription

Real-time new messages are delivered via Firebase RTDB Server-Sent Events:

```
Path:   /dm_messages/<conversationId>
Params: orderBy="timestamp"&limitToLast=50
Auth:   ?auth=<idToken>
```

The initial `put` event has `path: "/"` and carries the full historical snapshot — it is **skipped** (history is already loaded via REST `GET /v1/cmail/:id`). All subsequent `put` events have `path: "/<msgId>"` and carry a single new message.

The subscription is opened when a conversation is selected (Enter in list mode) and cancelled when:
- The user presses Esc (returns to list mode)
- A different conversation is opened
- The user navigates away from the C-Mail screen

### API Client Methods

| Method | Signature | Notes |
|---|---|---|
| `GetConversations` | `() ([]model.Conversation, error)` | Populates `UnreadCount`, `LastMessage`, and `LastMessageAt` from wire response |
| `GetMessages` | `(convID string, limit int, before int64) ([]model.Message, error)` | Returns oldest-first; pass `before=0` for the latest page, or a previous message's timestamp for older pages |
| `SendMessage` | `(convID, body string) (string, error)` | POST to REST endpoint; returns the reply text for reply-only commands (`/help`), empty otherwise |
| `StartConversation` | `(recipientUsername string) (model.Conversation, error)` | POST to REST; idempotent |
| `MarkCMailRead` | `(convID string) error` | POST to REST; called when a conversation is opened |
| `SubscribeDMs` | `(ctx context.Context, convID string) (<-chan model.Message, context.CancelFunc, error)` | RTDB SSE; skips initial snapshot |
| `RefreshSession` | `() error` | Proactively refreshes the idToken (shared across all screens); used to reconnect a live RTDB subscription after it closes |

### App-Level Wiring

- Conversations are pre-loaded on login via `afterLoginCmd`, and re-fetched every 60s on the same `pollUnreadTickMsg` ticker that refreshes the notifications unread count (`app.go`), so the tab badge stays current even while the user is on another tab.
- When the user selects a conversation (Enter in list mode), `CMailModel` zeroes that conversation's local `UnreadCount` immediately (optimistic, before the server round-trip) and `CMailConvSelectedMsg` is emitted; App calls `markCMailReadCmd(convID)` to persist the read state server-side.
- Pressing `c` on a highlighted post, reply, notification, or profile (read-only) emits `StartConversationMsg{Username}`. App calls `StartConversation(username)`, then switches to C-Mail and opens the returned conversation in detail mode. Self-DMs are silently dropped in the App handler.

### Conversation List Display

Each conversation card shows:
- `@<otherUser>` — username (Highlight style)
- Timestamp from `LastMessageAt` (Subtle style, right-aligned)
- `(N)` unread badge when `UnreadCount > 0`
- One-line preview from `LastMessage` (from the REST list response) or the most-recent loaded message
