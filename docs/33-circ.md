# 33 · CIRC — Public Chatrooms

## Overview

CIRC provides access to the cyberspace.online public chatrooms via tab **4** in the navigation bar. It follows the same two-mode UX pattern as C-Mail: a room-list view and a per-room chat view with live message streaming.

## UX

### List mode (default)

- Full-width scrollable list of room cards.
- Each card shows the room name and `#slug` subtitle, with the last-message timestamp right-aligned.
- Navigate with `j`/`k` or `↑`/`↓`; press `Enter` to open a room.

### Detail mode (after selecting a room)

- **Header** (1 row, always visible): `Room Name  ·  circ`
- **Message viewport**: scrollable history in IRC-style format.
  - Each message: `<username>  message body` with timestamp right-aligned.
  - Messages are rendered oldest-to-newest; the viewport starts at the bottom.
- **Input box** (3 rows, at the bottom): type and press `Enter` to send.
- Press `Esc` to return to the room list; the RTDB subscription is cancelled.

### Message format

```
<alice>  hello everyone                                     14:32
<bob>    this is the CIRC chatroom                         14:35
<alice>  line 1 of a multi-line message                    14:36
         line 2 indented to align with body
```

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
- **No pagination for history**: `GetRoomMessages` loads the latest 50 messages. The `before` cursor parameter is implemented but the UI does not yet trigger additional pages when scrolling to the top.

## Files

| File | Role |
|---|---|
| `internal/ui/screens/chatrooms.go` | Screen model (two-mode UX, SSE subscription) |
| `internal/ui/screens/render.go` | `renderCircMessages` (IRC-style format) |
| `internal/api/interface.go` | `GetRooms`, `GetRoomMessages`, `SendRoomMessage`, `MarkRoomRead`, `SubscribeRoom` |
| `internal/api/client.go` | HTTP + RTDB SSE implementations |
| `internal/api/mock.go` | Mock implementations for development/testing |
| `internal/model/types.go` | `model.Room` struct |
| `internal/ui/app.go` | `handleChatrooms`, `sendRoomMessageCmd`, `markRoomReadCmd` |
| `internal/ui/layout.go` | `menuTabs` (circ at index 3, key `4`) |
| `internal/ui/layout_tabs.go` | Key `4` binding, cancel wiring, hints, breadcrumbs |
