# 33 · CIRC — Public Chatrooms

## Overview

CIRC provides access to the cyberspace.online public chatrooms via tab **4** in the navigation bar. It follows the same two-mode UX pattern as C-Mail: a room-list view and a per-room chat view with live message streaming.

## UX

### List mode (default)

- Full-width scrollable list of room cards.
- Each card shows the room name and `#slug` subtitle, with the last-message timestamp right-aligned.
- Navigate with `j`/`k` or `↑`/`↓`; press `Enter` to open a room.

### Detail mode (after selecting a room)

- **Header** (1 row, always visible): `Room Name  #slug`
- **Message viewport**: scrollable history in IRC-style format.
  - Each message: `<username>  message body` with timestamp right-aligned; long bodies word-wrap and the timestamp trails the last wrapped line.
  - Messages are rendered oldest-to-newest; the viewport starts at the bottom.
- **Input box** (3 rows, at the bottom): type and press `Enter` to send.
- Press `Esc` to return to the room list; the RTDB subscription is cancelled.
- **Scroll-to-load history**: scrolling to the top of the loaded messages (`↑`) automatically fetches the next older page (`GetRoomMessages(roomID, 50, before)`, `before` = the oldest loaded message's timestamp) and prepends it, preserving scroll position so the previously-visible messages don't jump. The header shows `(loading history…)` while a page is in flight. Stops once a fetch returns no messages (start of history reached). If a fetch fails, the room isn't left stuck — a retry is possible on the next scroll-to-top.
- **Failed loads are distinguishable from empty**: if the initial (or a later) message fetch fails, the viewport shows "couldn't load messages" instead of the misleading "no messages yet".
- **Live-stream reconnect**: the Firebase `idToken` backing the RTDB subscription expires hourly. The stream is treated as dead — triggering reconnect — on any of: the server sending a terminal `auth_revoked`/`cancel` SSE event, a 10-minute idle-read timeout (no line received, including keepalive comments), a 30-second connect-phase timeout, or an outright network error/close (see `internal/rtdb/client.go`). When the stream closes while a room is still open, the app refreshes the session and reopens the subscription, retrying with exponential backoff (`1s, 2s, 4s, 8s, 15s` — 6 attempts total) if an attempt fails. Success shows a brief "reconnected to live chat" notification. While retrying, the room header shows `(live updates lost, reconnecting… N/6)`; if all attempts fail, it shows a persistent `(live updates lost)` until the user leaves and re-enters the room — this indicator is independent of the message list, so it's visible even with history already loaded.
- **Admin badge**: messages sent by a chat admin (`isChatAdmin` from the API) show a `[admin]` tag next to the username.

### Slash commands

The server expands IRC-style slash commands (`/me`, `/poke`/`/hug`/`/hi5`/`/slap`, `/dice`, `/8ball`, `/fortune`) before storing/broadcasting the message — whatever the user types is sent verbatim as `content`, and the expanded text comes back through the normal message pipeline. A malformed command (e.g. bad `/dice` notation) returns `400 VALIDATION_ERROR`, surfaced through the existing send-error toast.

`/help` is different: it posts no message — the server returns the command list only in the synchronous send response (`{ "data": { "reply": "…" } }`). The client captures this and appends it as a **local-only system notice** (`model.Message.IsSystem`) directly into the viewport — rendered without a username bracket, admin badge, or timestamp column, prefixed with `*** ` (`renderSystemNotice` in `render.go`). It's never sent to or stored by the server, so it only exists for the current session and disappears if the room is reopened.

**`isAction` (undocumented field, confirmed via live testing):** the API actually returns an `isAction: true` flag on `/me` and other emote-style command messages (and `content` is just the bare action text with no username baked in — e.g. sending `/me tests the plumbing` stores `content: "tests the plumbing"`, `isAction: true`). This isn't mentioned in `docs/00-latest-api-reference.md`, which was the assumption going in — the docs only show the command's *input* syntax, never the stored/broadcast shape. `model.Message.IsAction` carries this through both the REST and RTDB paths, and `renderActionLine` in `render.go` renders it in classic IRC form: `* username body *`, right-aligned timestamp, no bracket — reused by both CIRC and C-Mail. Confirmed live for CIRC only; added defensively to the C-Mail wire types too (harmless no-op if the field turns out not to be present there).

### Message format

```
<alice>  hello everyone                                     14:32
<bob>    this is the CIRC chatroom                          14:35
<alice>  a message long enough that it needs to wrap
         onto a second line stays readable and the
         timestamp trails the last line              14:36
* bob waves *                                               14:37
```

Long bodies word-wrap to fit the terminal width; continuation lines are indented to align under the body. The timestamp is right-aligned on the message's last (wrapped) line, with a minimum gap reserved so wrapped text never runs into or pushes it off-screen.

## Keyboard shortcuts

| Key | Action |
|---|---|
| `j` / `↓` | Next room (list mode) |
| `k` / `↑` | Previous room (list mode) |
| `Enter` | Open selected room |
| `↑` / `↓` | Scroll messages (detail mode) |
| `Enter` | Send message (detail mode) |
| `Esc` | Return to room list |
| `ctrl+o` | Open URLs/images from the loaded room history (detail mode). Plain `o` — the shortcut used everywhere else in the TUI — can't reach this here: the compose input is focused for the entire detail view (not a transient sub-mode like Feed's reply box), so `o` always gets typed into the message instead. `ctrl+o` is exempted from the focused-input gate specifically for this. |

## API integration

| Endpoint | Method | Purpose |
|---|---|---|
| `GET /v1/circ` | REST | List available rooms |
| `GET /v1/circ/:roomId` | REST | Load message history |
| `POST /v1/circ/:roomId` | REST | Send a message |
| `POST /v1/circ/:roomId/read` | REST | Mark room as read |
| RTDB `chat_messages/<roomId>` | SSE | Live message stream |

The real-time subscription mirrors the C-Mail pattern exactly:
- `SubscribeRoom(ctx, roomID)` in `api.Client` opens an RTDB SSE stream.
- The initial full-snapshot `put` is skipped; history is loaded via `GetRoomMessages`.
- Incremental `put` events are converted to `model.Message` and sent over a Go channel.
- `CancelSubscription()` is called when navigating away, closing the underlying HTTP stream.
- If the stream closes while the room is still open (idToken expiry, ~1hr), `api.Client.RefreshSession()` is called to get a fresh token, then `SubscribeRoom` is called again — it re-reads the current token at call time, so no other plumbing is needed. Success emits `RoomReconnectedMsg` (App shows a toast); an unmatched/stale close event (from a room the user already navigated away from) is a no-op.

## Rate limits (from API docs)

| Operation | Limit |
|---|---|
| Send message | 15/min, 300/day |
| Load history | 45/min |
| Mark read | 60/min |
| List rooms | 30/min |

## Known limitations

- **No online-users list**: The API does not expose an endpoint for listing users currently online in a room. The right panel from the original design was deferred.
- **No client-side slash-command validation/autocomplete**: the server validates and expands commands; the client just sends whatever was typed and shows whatever comes back (or the `400` error if malformed).
- **`isAction` styling relies on an undocumented field**: confirmed live for CIRC; C-Mail parses the same field defensively but it's unconfirmed there (no `/me` messages found in sampled conversation history to check against). If the backend ever changes this field's name/shape without notice, action messages would silently fall back to normal rendering rather than erroring.
- **Exhaustion heuristic is coarse**: an older-page fetch that returns zero messages marks history as exhausted for the session; a page that returns fewer than the requested limit but more than zero is not treated as exhausted, so the very last page may trigger one extra (empty) round-trip.
- **No unread/"new messages" indicator**: unlike C-Mail, room list cards and the tab bar show no unread badge. `GET /v1/circ`'s documented response fields don't include one, even though `POST /v1/circ/:roomId/read` is described as driving a "new messages indicator" server-side — would need confirming with the API before this can be built.
- **Reconnect gives up after ~30s of backoff**: 6 total attempts (`1s, 2s, 4s, 8s, 15s` backoff) are made when the live stream closes; if all fail (e.g. a longer network outage), the room stays without live updates — shown via a persistent `(live updates lost)` header indicator — until the user leaves and re-enters.

## Files

| File | Role |
|---|---|
| `internal/ui/screens/chatrooms.go` | Screen model (two-mode UX, SSE subscription); `GetFocusedURLs` (`URLProvider`) |
| `internal/ui/screens/render.go` | `renderCircMessages` (IRC-style format); `renderActionLine`, `renderSystemNotice` |
| `internal/api/interface.go` | `GetRooms`, `GetRoomMessages`, `SendRoomMessage` (returns reply text for `/help`), `MarkRoomRead`, `SubscribeRoom` |
| `internal/api/client.go` | HTTP + RTDB SSE implementations |
| `internal/api/mock.go` | Mock implementations for development/testing; canned `/help` reply |
| `internal/model/types.go` | `model.Room` struct; `model.Message.IsChatAdmin`, `IsSystem`, `IsAction` |
| `internal/ui/app.go` | `handleChatrooms`, `sendRoomMessageCmd`, `markRoomReadCmd` |
| `internal/ui/layout.go` | `menuTabs` (circ at index 3, key `4`) |
| `internal/ui/layout_tabs.go` | Key `4` binding, cancel wiring, hints, breadcrumbs |
