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
- **Live-stream reconnect**: the Firebase `idToken` backing the RTDB subscription expires hourly. When the stream closes while a room is still open, the app refreshes the session and reopens the subscription automatically, showing a brief "reconnected to live chat" notification. A single reconnect attempt is made; if it fails, the room is left without live updates until the user leaves and re-enters (same as before this fix).
- **Admin badge**: messages sent by a chat admin (`isChatAdmin` from the API) show a `[admin]` tag next to the username.

### Message format

```
<alice>  hello everyone                                     14:32
<bob>    this is the CIRC chatroom                          14:35
<alice>  a message long enough that it needs to wrap
         onto a second line stays readable and the
         timestamp trails the last line              14:36
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
- **No slash command preview**: The API supports IRC-style commands (`/me`, `/poke`, `/dice`, etc.) expanded server-side, but the TUI has no client-side rendering for them yet.
- **Exhaustion heuristic is coarse**: an older-page fetch that returns zero messages marks history as exhausted for the session; a page that returns fewer than the requested limit but more than zero is not treated as exhausted, so the very last page may trigger one extra (empty) round-trip.
- **No unread/"new messages" indicator**: unlike C-Mail, room list cards and the tab bar show no unread badge. `GET /v1/circ`'s documented response fields don't include one, even though `POST /v1/circ/:roomId/read` is described as driving a "new messages indicator" server-side — would need confirming with the API before this can be built.
- **Reconnect has no retry/backoff**: a single reconnect attempt is made when the live stream closes; if that attempt itself fails, the room stays without live updates until the user leaves and re-enters.

## Files

| File | Role |
|---|---|
| `internal/ui/screens/chatrooms.go` | Screen model (two-mode UX, SSE subscription) |
| `internal/ui/screens/render.go` | `renderCircMessages` (IRC-style format) |
| `internal/api/interface.go` | `GetRooms`, `GetRoomMessages`, `SendRoomMessage`, `MarkRoomRead`, `SubscribeRoom` |
| `internal/api/client.go` | HTTP + RTDB SSE implementations |
| `internal/api/mock.go` | Mock implementations for development/testing |
| `internal/model/types.go` | `model.Room` struct; `model.Message.IsChatAdmin` |
| `internal/ui/app.go` | `handleChatrooms`, `sendRoomMessageCmd`, `markRoomReadCmd` |
| `internal/ui/layout.go` | `menuTabs` (circ at index 3, key `4`) |
| `internal/ui/layout_tabs.go` | Key `4` binding, cancel wiring, hints, breadcrumbs |
