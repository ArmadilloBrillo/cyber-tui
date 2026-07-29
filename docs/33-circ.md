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
- Press `Esc` to return to the room list; the RTDB subscription is cancelled — or, if this room was opened via a `chat_mention` notification, leave Chatrooms entirely and return to Notifications instead (see "Deep-link back-navigation" below).
- **Scroll-to-load history**: scrolling to the top of the loaded messages (`↑`) automatically fetches the next older page (`GetRoomMessages(roomID, 50, before)`, `before` = the oldest loaded message's timestamp) and prepends it, preserving scroll position so the previously-visible messages don't jump. The header shows `(loading history…)` while a page is in flight. Stops once a fetch returns no messages (start of history reached). If a fetch fails, the room isn't left stuck — a retry is possible on the next scroll-to-top.
- **Failed loads are distinguishable from empty**: if the initial (or a later) message fetch fails, the viewport shows "couldn't load messages" instead of the misleading "no messages yet".
- **Live-stream reconnect**: the Firebase `idToken` backing the RTDB subscription expires hourly. The stream is treated as dead — triggering reconnect — on any of: the server sending a terminal `auth_revoked`/`cancel` SSE event, a 10-minute idle-read timeout (no line received, including keepalive comments), a 30-second connect-phase timeout, or an outright network error/close (see `internal/rtdb/client.go`). When the stream closes while a room is still open, the app refreshes the session and reopens the subscription, retrying with exponential backoff (`1s, 2s, 4s, 8s, 15s` — 6 attempts total) if an attempt fails. Success shows a brief "reconnected to live chat" notification. While retrying, the room header shows `(live updates lost, reconnecting… N/6)`; if all attempts fail, it shows a persistent `(live updates lost)` until the user leaves and re-enters the room — this indicator is independent of the message list, so it's visible even with history already loaded.
- **Online users panel**: a persistent side panel in detail mode lists who's currently in the room — admins (marked `★`, always in `theme.Highlight`) first, then everyone else, both blocks alphabetical (case-insensitive). The viewer's own name renders in `theme.MeHighlight` (bold white) wherever it appears in the panel, the same substitution `renderCircMessages` makes for the message list. The header row shows a live count (`N online`, from the local presence list, not the room-list's `onlineCount`). Panel width is binary, not scaled: either its full preferred width (24 cols — an admin marker plus the API's 20-char max username, so a max-length name never causes a resize) whenever the terminal is at least 65 total columns, or fully collapsed (messages go full-width) below that or before the first presence snapshot arrives — never an in-between size, which could be narrower than the worst-case content and cause a long username to hard-wrap mid-word (`lipgloss`/`cellbuf.Wrap` breaks unbroken words that exceed the render width rather than truncating them). See `ChatroomsModel.panelWidths` in `chatrooms.go`. Admin status used to show as a `[admin]` tag on each message line; that's been removed now that the panel is the single place admin status is shown (`renderCircMessages` in `render.go`).
- **Presence lifecycle**: entering a room announces presence (`POST .../presence`), starts a self-rescheduling heartbeat at the cadence the response specifies (`heartbeatMs` — read from the response, never hard-coded), loads the initial user list (`GET .../users`), and opens a live presence stream (`chat_presence/<roomId>` RTDB). Leaving via `Esc` sends an explicit leave (`DELETE .../presence`); leaving via tab-switch or `/` search relies on the server's own staleness expiry instead (see Known limitations).
- **Own name & mentions stand out**: the logged-in user's own username (as a message author) and any mention of it inside another user's message body — bare or `@`-prefixed, case-insensitive, word-bounded — render in bold white (`theme.MeHighlight`) instead of the usual yellow (`theme.Highlight`). Cyan was the first instinct (it's the "this is you" color in C-Mail) but is already used in CIRC for the room title, active border, and tab mnemonics, so white was picked to stay distinct. Handled by `highlightMentions` in `render.go`.
- **Jump-to-room from a notification**: pressing `Enter` on a `chat_mention` notification emits `OpenRoomMsg{RoomSlug, NotifID}`. App records the originating screen in `App.chatroomsReturn`, activates Chatrooms (reloading the room list), and stashes the slug via `SetPendingRoomSlug`; once the room list (re)loads, `OpenPendingRoom()` auto-enters detail mode for the matching room and `ChatroomsModel.canGoBack` is set to `true`.
- **Deep-link back-navigation**: when `canGoBack` is true, `Esc` in detail mode emits `LeaveChatroomsMsg` instead of dropping to the room list; App sets `active = chatroomsReturn`, returning to Notifications (the only deep-link origin today). Reaching Chatrooms through the ordinary tab/leader-key navigation calls `ChatroomsModel.ResetToList()`, which clears `canGoBack` *and* drops back to list mode — so switching to the CIRC tab manually while a deep-linked room is still open (instead of pressing `Esc`) also lands on the room list, not stuck in that room. Mirrors the `canGoBack`/`profileReturn` pattern already used by read-only profiles (`docs/16-view-profile.md`) and now also by C-Mail (`docs/08-cmail.md`).

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
| `Esc` | Return to room list — or, if deep-linked from a `chat_mention` notification, leave Chatrooms and return to Notifications |
| `ctrl+o` | Open URLs/images from the loaded room history (detail mode). Plain `o` — the shortcut used everywhere else in the TUI — can't reach this here: the compose input is focused for the entire detail view (not a transient sub-mode like Feed's reply box), so `o` always gets typed into the message instead. `ctrl+o` is exempted from the focused-input gate specifically for this. |

## API integration

| Endpoint | Method | Purpose |
|---|---|---|
| `GET /v1/circ` | REST | List available rooms (includes `onlineCount` per room) |
| `GET /v1/circ/:roomId` | REST | Load message history |
| `POST /v1/circ/:roomId` | REST | Send a message |
| `POST /v1/circ/:roomId/read` | REST | Mark room as read |
| RTDB `chat_messages/<roomId>` | SSE | Live message stream |
| `GET /v1/circ/:roomId/users` | REST | Initial who's-online list on room entry |
| `POST /v1/circ/:roomId/presence` | REST | Announce presence / heartbeat; returns `heartbeatMs`/`staleAfterMs` |
| `DELETE /v1/circ/:roomId/presence` | REST | Explicit leave (sent on `Esc`; see Known limitations) |
| RTDB `chat_presence/<roomId>` | SSE | Live presence stream — full filtered snapshot on every change and on a 5s re-evaluation timer |

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
| List who's in a room | 60/min |
| Presence heartbeat / leave | 30/min |

## Known limitations

- **No client-side slash-command validation/autocomplete**: the server validates and expands commands; the client just sends whatever was typed and shows whatever comes back (or the `400` error if malformed).
- **`isAction` styling relies on an undocumented field**: confirmed live for CIRC; C-Mail parses the same field defensively but it's unconfirmed there (no `/me` messages found in sampled conversation history to check against). If the backend ever changes this field's name/shape without notice, action messages would silently fall back to normal rendering rather than erroring.
- **Exhaustion heuristic is coarse**: an older-page fetch that returns zero messages marks history as exhausted for the session; a page that returns fewer than the requested limit but more than zero is not treated as exhausted, so the very last page may trigger one extra (empty) round-trip.
- **No unread/"new messages" indicator**: unlike C-Mail, room list cards and the tab bar show no unread badge. `GET /v1/circ`'s documented response fields don't include one, even though `POST /v1/circ/:roomId/read` is described as driving a "new messages indicator" server-side — would need confirming with the API before this can be built.
- **Reconnect gives up after ~30s of backoff**: 6 total attempts (`1s, 2s, 4s, 8s, 15s` backoff) are made when the live stream closes; if all fail (e.g. a longer network outage), the room stays without live updates — shown via a persistent `(live updates lost)` header indicator — until the user leaves and re-enters.
- **Presence stream reconnect has no backoff or UI indicator**: unlike the message stream, a dropped `chat_presence` connection retries immediately with no attempt cap and nothing shown to the user if it keeps failing — it's supplementary data, not core chat functionality. Upgrade path: share the message stream's backoff state machine (`reconnect.go`) if this proves unreliable in practice.
- **Explicit leave-presence only fires on `Esc`**: tab-switching away from Chatrooms or opening `/` search (routed through `ChatroomsModel.CancelSubscription`, called from `app.go` and `layout.go`'s `activateScreen`) does not send `DELETE .../presence` — threading a leave command through those call sites would mean changing `CancelSubscription`'s signature and touching files outside the screens package for a UX difference the API is explicitly designed to tolerate (you stay listed until `staleAfterMs` elapses — 3 minutes by default). The room's own list corrects itself either way.
- **RTDB presence merge doesn't handle deeply-nested patch paths**: a `chat_presence` event at `/<userId>` replaces that user's whole entry; a hypothetical deeper patch (e.g. just `/<userId>/lastSeen`) is ignored rather than merged. In practice each presence write replaces a user's whole entry, so this hasn't been observed to matter.

## Files

| File | Role |
|---|---|
| `internal/ui/screens/chatrooms.go` | Screen model (two-mode UX, message + presence SSE subscriptions, heartbeat scheduling); `panelWidths`, `sortRoomUsers`, `renderRoomUsersPanel`; `GetFocusedURLs` (`URLProvider`) |
| `internal/ui/screens/chatrooms_test.go` | Tests for `sortRoomUsers`, `panelWidths`, and presence message stale-guards |
| `internal/ui/screens/render.go` | `renderCircMessages` (IRC-style format, no admin tag); `renderActionLine`, `renderSystemNotice`, `highlightMentions` |
| `internal/api/interface.go` | `GetRooms`, `GetRoomMessages`, `SendRoomMessage` (returns reply text for `/help`), `MarkRoomRead`, `SubscribeRoom`, `GetRoomUsers`, `AnnouncePresence`, `LeaveRoomPresence`, `SubscribeRoomPresence` |
| `internal/api/client.go` | HTTP + RTDB SSE implementations; `applyPresenceEvent`/`filterFreshPresence` (presence RTDB state merge) |
| `internal/api/mock.go` | Mock implementations for development/testing; canned `/help` reply, canned presence data |
| `internal/model/types.go` | `model.Room` struct (+ `OnlineCount`); `model.RoomUser`; `model.Message.IsChatAdmin`, `IsSystem`, `IsAction` |
| `internal/ui/app.go` | `handleChatrooms`, `sendRoomMessageCmd`, `markRoomReadCmd` |
| `internal/ui/layout.go` | `menuTabs` (circ at index 3, key `4`) |
| `internal/ui/layout_tabs.go` | Key `4` binding, cancel wiring, hints, breadcrumbs |
