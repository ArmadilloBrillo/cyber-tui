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

The C-Mail tab itself also shows an aggregate unread badge, mirroring the Notifications tab: `c-mail (N)` in the Tabs layout, `c-mail ●N` in the Miller layout, where `N` is the sum of `UnreadCount` across all conversations (`CMailModel.TotalUnread()`). The badge clears immediately (optimistically) when a conversation is opened, and otherwise updates live via the account-wide `user_conversations/<uid>` RTDB subscription (see "RTDB SSE Subscriptions" below) — not a poll.

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
- **Compose input width**: `input.Width` is set to `terminalWidth - 4 - len(Prompt) - 1`, not just `terminalWidth - 4`, and the empty (placeholder) state is hand-built from `textinput`'s exported style fields rather than left to its own `placeholderView()` — see `docs/33-circ.md`'s matching bullet for the full explanation (a pair of `bubbles/textinput` quirks where its empty-placeholder render and typed-content render total different widths, which without this fix left the box either 3 columns too wide once typing started, or 3 columns too narrow while empty).

**Per-message browsing**: pressing `↑` from the bottom of the history selects the newest message and enters browsing mode (mirrors CIRC — see `docs/33-circ.md`) — the compose input blurs, the selected message is highlighted, and `↑`/`↓` move message-by-message (not line-by-line) even across messages that wrap to several lines. `Esc` clears the selection and returns to typing without leaving the conversation; `↓` past the newest message does the same. Unlike CIRC, there's no `!` (flag) or `d` (delete) action while browsing — the API has no CMail message flag/delete endpoint (only `POST /v1/circ/:roomId/messages/:messageId/flag` for CIRC rooms, and the posts/replies flag endpoints), and no `Enter` reveal action, since CMail doesn't support spoiler/l33t styling. The one action browsing enables is scoping `ctrl+o` (open link) to just the selected message instead of every loaded message — see "Key Bindings" below.

**Scroll-to-load history**: scrolling to the top of the loaded messages (`↑`) automatically fetches the next older page (`GetMessages(conversationID, 50, before)`, `before` = the oldest loaded message's timestamp) and prepends it, preserving scroll position. The header shows `(loading history…)` while a page is in flight. Stops once a fetch returns no messages. If a fetch fails, `loadingHistory` resets so a retry is possible on the next scroll-to-top, and the viewport shows "couldn't load messages" instead of a misleading "no messages" if nothing has loaded yet.

**Live-stream reconnect**: the Firebase `idToken` backing the RTDB subscription expires hourly. The stream is treated as dead — triggering reconnect — on any of: the server sending a terminal `auth_revoked`/`cancel` SSE event, a 10-minute idle-read timeout (no line received, including keepalive comments), a 30-second connect-phase timeout, or an outright network error/close (see `internal/rtdb/client.go`). When the stream closes while a conversation is still open, the app calls `api.Client.RefreshSession()` and reopens the subscription, retrying with exponential backoff (`1s, 2s, 4s, 8s, 15s` — 6 attempts total) if an attempt fails. Success shows a brief "reconnected to live chat" notification. While retrying, the conversation header shows `(live updates lost, reconnecting… N/6)`; if all attempts fail, it shows a persistent `(live updates lost)` until the user leaves and re-enters the conversation — this indicator is independent of the message list, so it's visible even with history already loaded.

**Conversation stays open across tab switches**: switching to another tab (Feed, Journal, etc.) no longer cancels the open conversation's RTDB subscriptions or drops it back to the conversation list — the message stream, typing stream, and reconnect-retry chains all keep running via `App`'s background routing (`screens.IsDMStreamMsg`, checked in `handleCMail` when C-Mail isn't the active screen). A message received while backgrounded still lands in `CMailModel`'s message state and bumps the currently-open conversation's `UnreadCount` (`CMailModel.bumpActiveConvUnread`) — the same field the aggregate tab badge (`TotalUnread()`, above) already sums, so the badge reflects it immediately instead of waiting for the next 60s poll; unlike CIRC, there's no separate single-room counter. Switching back to C-Mail resumes the same conversation as-is and zeroes that conversation's unread count (`CMailModel.SetFocused`). Only the single conversation the user had open is kept live — not every conversation they're a participant in. The subscription is torn down for real only on `Esc` (leaving the conversation) or a session-expiry logout, matching `CancelSubscription`'s narrower scope now — mirrors CIRC's identical background-persistence pattern (`docs/33-circ.md`).

**Slash commands**: like CIRC, the server expands `/me`, `/poke`/`/hug`/`/hi5`/`/slap`, `/dice`, `/8ball`, and `/fortune` server-side. `/help` posts no message; its reply is captured from the send response and appended as a local-only system notice (`model.Message.IsSystem`, rendered via `renderSystemNotice` — no bubble, no border, just a muted `*** `-prefixed block). It's never sent to or stored by the server.

Like CIRC (`docs/33-circ.md`), any `/`-prefixed input not recognized is rejected client-side on Enter — the input clears and a local `*** unknown command: /foo` system notice appears instead of the text being sent as a literal message. This uses the same `isKnownSlashCommand` helper CIRC uses (`chatrooms.go`), since the server accepts nearly the same command set for both: `/me`, the emotes, `/dice`, `/8ball`, `/fortune`, `/gif`, `/song`, `/help`, and chained text styles (`/comic+rainbow`, etc. — same `/spoiler`-can't-chain rule as CIRC). Only `/mute`/`/unmute`/`/muted`/`/unmuteall` and `/art` are CIRC-only and rejected in C-Mail.

`/me` and other emotes set an undocumented `isAction` field on the message, discovered via live testing against CIRC (parsed defensively for C-Mail too, but not yet confirmed live there — see `docs/33-circ.md`). `model.Message.IsAction` messages render as `* username body *` (`renderActionLine` in `render.go`) instead of the usual bordered bubble — same classic-IRC treatment as CIRC.

**Typing indicator**: while the compose input is non-empty, the client announces "typing" (`POST .../typing`) and re-announces every `heartbeatMs` (3s, from the response) until the input goes idle for 2.5s or the input is emptied (either clears immediately via `DELETE .../typing`) or a message is sent (the server auto-clears on send, so no explicit clear call is made). Simultaneously, opening a conversation subscribes to the other participant's live typing status (`dm_presence/<conversationId>` RTDB node); when fresh, ` is typing` plus an animated dot count is appended right after the header's `@other` title, reading as one sentence: `@other is typing...` (Subtle style). The dots cycle no-dots → `.` → `..` → `...` on a 500ms tick (`typingAnimTickMsg`/`scheduleTypingAnimCmd`), free-running for as long as the conversation is open. Shown in the detail header only — no list-mode badge.

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
| `↑` | Not browsing: scroll message history up one line, or enter browsing (selecting the newest message) once the top of the loaded history is reached. Browsing: move the selection to the previous (older) message. |
| `↓` | Not browsing: scroll message history down one line. Browsing: move the selection to the next (newer) message, or exit browsing (back to typing) once past the newest. |
| `Esc` | Browsing: clear the selection and return to typing, staying in the conversation. Not browsing: return to list mode; cancel RTDB subscription — or, if this conversation was opened via a deep link (`c` from another screen, or a `dm_message` notification), leave C-Mail entirely and return to that origin screen instead. |
| `ctrl+o` | Open URLs/images — from just the selected message while browsing, or from the whole loaded conversation otherwise. Plain `o` can't reach this here — the compose input is focused for the entire detail view (`InputFocused()` doesn't distinguish browsing from typing), so `o` always types into the message, or is swallowed while browsing, instead; `ctrl+o` is exempted from the focused-input gate specifically for this. |
| `ctrl+q` | Quit (same as global `q`) |
| `ctrl+t` | Open theme picker (same as global `t`) |
| `ctrl+←` / `ctrl+→` | Cycle tabs (same as global `←`/`→`; Tabs layout only) |
| `←` / `→` | Cycle tabs — but only when the compose input is empty (detail mode, Tabs layout). With text in the box (just typed, or a draft left over from switching tabs away and back — the conversation and its subscription stay open in the background, see "Conversation stays open across tab switches" below), plain `←`/`→` moves the cursor instead and `ctrl+←`/`ctrl+→` is the way out, same as it's always been. Otherwise, resuming a backgrounded conversation on tab-return would silently swallow the very first `←`/`→` press into an empty box the user never asked to type into. See `CMailModel.ComposeEmpty()`. |
| all other | Forwarded to compose input (`j`/`k` type normally) |

---

## Screen Model

**File:** `internal/ui/screens/cmail.go`

**Type:** `CMailModel`
**Constructor:** `NewCMailModel(currentUser, currentUserID string, client api.Client) CMailModel` — `currentUserID` is the account's RTDB uid, used to open the account-wide conversation-list subscription
**Messages emitted:** `SendCMailMsg{ConversationID, Body}`, `CMailConvSelectedMsg{ConversationID}`, `StartConversationMsg{Username}` (from other screens), `LeaveCMailMsg{}` (Esc on a deep-linked conversation)
**App field:** `a.cmail`
**Screen constant:** `screenCMail`

### Exported accessors (for testing)

| Method | Returns |
|---|---|
| `IsShowingDetail() bool` | Whether detail mode is active |
| `HasActiveConv() bool` | Alias for `IsShowingDetail()` |
| `HasLiveConv() bool` | Detail mode active *and* a conversation object is actually loaded — used by `activateScreen` to decide whether re-entering the tab resumes in place |
| `SelectedConv() int` | Cursor index in conversation list |
| `InputFocused() bool` | True in detail mode (compose input focused) |
| `ComposeEmpty() bool` | Whether the compose input has no typed text — lets plain `←`/`→` fall through to tab-cycling |
| `TotalUnread() int` | Sum of `UnreadCount` across all conversations, for the tab-bar badge |
| `SelectedMessageID() string` | The browsing-selected message's ID, or `""` while composing/typing |
| `GetFocusedURLs() []string` | URLs from just the selected message while browsing, or across all loaded messages in the open conversation otherwise (`URLProvider`); nil outside detail mode |

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
| `POST` | `/v1/cmail/:conversationId/typing` | Announce "typing"; returns `{heartbeatMs, staleAfterMs}` (3000/9000) to honor |
| `DELETE` | `/v1/cmail/:conversationId/typing` | Clear typing status immediately |

**Notes:**
- `POST /v1/cmail` is idempotent — returns 200 for an existing conversation, 201 for a new one.
- Rate limits: 15 sends/min, 300/day, 150/hour; 5 start/min, 50/day, 30/hour; 60 mark-read/min; 45 typing on/off per min.
- Blocked in either direction returns 403.

### RTDB SSE Subscriptions

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

**Conversation list — `user_conversations/<uid>`.** Separately, an account-wide subscription drives the conversation list and its unread badge live (`SubscribeUserConversations`):

```
Path: /user_conversations/<uid>
Auth: ?auth=<idToken>
```

Each entry (keyed by `conversationId`) carries `unreadCount` (always present) plus `otherUserId`/`otherUsername`/`lastMessage`/`lastMessageAt` (absent on some legacy/stale conversations — the client falls back to "unknown" for the participant, same as it does for the REST list). Unlike `dm_messages`, entries mutate in place, so each receive on the channel is the full converted+sorted list (unread first, then most recently active), not a single incremental event — same shape as `SubscribeRoomPresence`/`SubscribeDMTyping`.

Put-vs-patch handling has one wrinkle beyond `applyPresenceEvent`'s precedent (see `docs/33-circ.md`'s presence bug writeup): a `patch` at a single `/<conversationId>` path — the common case, since a new message typically only rewrites `lastMessage`/`lastMessageAt`/`unreadCount` — is *merged* onto the existing entry rather than replacing it outright, so `otherUserId`/`otherUsername` (not repeated on every message write) survive. Only a `put` (root or single-path) fully replaces an entry; presence/typing entries don't need this distinction because every write there already rewrites the whole entry.

Opened once after the first conversation list loads (REST, at login) and stays open for the whole session — independent of which (if any) conversation is open, and unaffected by tab switches. Reconnects with the same `1s, 2s, 4s, 8s, 15s` backoff as the message stream (see "Live-stream reconnect" above) but fails silently after exhausting attempts — no UI indicator, since there's no natural place to show one outside an open conversation; the list simply stops updating live until the next login. Closed only on session end (`handleUnauthorized` → `CancelUserConvsSubscription`).

### API Client Methods

| Method | Signature | Notes |
|---|---|---|
| `GetConversations` | `() ([]model.Conversation, error)` | Populates `UnreadCount`, `LastMessage`, and `LastMessageAt` from wire response. Not currently called by the TUI — the live `SubscribeUserConversations` subscription's own first event replaces it as the list's source; kept as a real, tested REST binding for `GET /v1/cmail` |
| `GetMessages` | `(convID string, limit int, before int64) ([]model.Message, error)` | Returns oldest-first; pass `before=0` for the latest page, or a previous message's timestamp for older pages |
| `SendMessage` | `(convID, body string) (string, error)` | POST to REST endpoint; returns the reply text for reply-only commands (`/help`), empty otherwise |
| `StartConversation` | `(recipientUsername string) (model.Conversation, error)` | POST to REST; idempotent |
| `MarkCMailRead` | `(convID string) error` | POST to REST; called when a conversation is opened |
| `SubscribeDMs` | `(ctx context.Context, convID string) (<-chan model.Message, context.CancelFunc, error)` | RTDB SSE; skips initial snapshot |
| `AnnounceTyping` | `(convID string) (heartbeatMs, staleAfterMs int, err error)` | POST to REST; read the cadence from the response, never hard-code it |
| `ClearTyping` | `(convID string) error` | DELETE to REST; best-effort, discarded on failure |
| `SubscribeDMTyping` | `(ctx context.Context, convID string, staleAfterMs int) (<-chan []model.TypingUser, context.CancelFunc, error)` | RTDB SSE on `dm_presence/<convID>`; full filtered snapshot per receive, like `SubscribeRoomPresence` |
| `SubscribeUserConversations` | `(ctx context.Context, uid string, initial []model.Conversation) (<-chan []model.Conversation, context.CancelFunc, error)` | RTDB SSE on `user_conversations/<uid>`; full converted+sorted list per receive; `initial` seeds a reconnect so the list doesn't go blank |
| `RefreshSession` | `() error` | Proactively refreshes the idToken (shared across all screens); used to reconnect a live RTDB subscription after it closes |

### App-Level Wiring

- The conversation list has no REST seed. `afterLoginCmd` opens the `user_conversations/<uid>` RTDB subscription directly (`CMailModel.OpenUserConvsSubscription()`) right after login; its own first event is a full snapshot (see "Conversation list — `user_conversations/<uid>`" above) that populates the list, same as `chat_presence`'s initial event does for CIRC presence. Starting a new conversation and switching into the C-Mail tab (`activateScreen`, `layout.go`) both used to re-fetch the list via REST (`GetConversations`) as well — removed, since that REST call and the subscription's own internal merge state were two independent writers to `CMailModel.conversations` with no reconciliation between them, so whichever one updated last would silently win and the other's data would be reverted by the next unrelated live event. The subscription alone is now the only thing that ever updates the list after login. `pollUnreadTickMsg` (`app.go`) no longer touches C-Mail either — it now drives only the notifications unread-count badge.
- When the user selects a conversation (Enter in list mode), `CMailModel` zeroes that conversation's local `UnreadCount` immediately (optimistic, before the server round-trip) and `CMailConvSelectedMsg` is emitted; App calls `markCMailReadCmd(convID)` to persist the read state server-side.
- Pressing `c` on a highlighted post, reply, notification, or profile (read-only) — or opening a `dm_message` notification — emits `StartConversationMsg{Username}`. App records the screen this was sent from in `App.cmailReturn` and marks `CMailModel.canGoBack = true`, then calls `StartConversation(username)` and switches to C-Mail, opening the returned conversation in detail mode. Self-DMs are silently dropped in the App handler.
- **Deep-link back-navigation**: when `canGoBack` is true, `Esc` in detail mode emits `LeaveCMailMsg` instead of dropping to the conversation list; App sets `active = cmailReturn`, returning to the screen the conversation was opened from (e.g. back to the post you pressed `c` on, or back to Notifications). Reaching C-Mail through the ordinary tab/leader-key navigation calls `CMailModel.ResetToList()`, which clears `canGoBack` *and* drops back to list mode — so switching to the C-Mail tab manually while a deep-linked conversation is still open (instead of pressing `Esc`) also lands on the conversation list, not stuck in that conversation. Mirrors the `canGoBack`/`profileReturn` pattern already used by read-only profiles (`docs/16-view-profile.md`).

### Conversation List Display

Each conversation card shows:
- `@<otherUser>` — username (Highlight style)
- Timestamp from `LastMessageAt` (Subtle style, right-aligned)
- `(N)` unread badge when `UnreadCount > 0`
- One-line preview from `LastMessage` (from the REST list response) or the most-recent loaded message
