# API Backlog — Outstanding Features & Known Issues

Tracks gaps between the cyberspace.online API (v0.8.2) and what is currently implemented in the TUI client.
Update this file whenever a feature is implemented or an issue is discovered/resolved.

---

## Known API Issues (Server-Side Bugs)

These bugs exist in the server — no client-side fix is possible. Report to the API maintainer.

| Endpoint | Method | Status | Description | Discovered |
|---|---|---|---|---|
| `/v1/follows` | GET | **Open** | Response does not include `followerUsername` or `followedUsername`. Confirmed still missing in v0.4 (re-tested 2026-05-29). The profile Following/Followers tabs fall back to showing a truncated user ID; profile navigation from those tabs is disabled until the API returns usernames. | 2026-04-17 |
| `/v1/notifications` | GET | **Open (by design?)** | Notifications can point to posts that have since been deleted, and the notification object exposes no "target deleted/unavailable" flag — opening one is the only way to discover the target is gone (`GET /v1/posts/:id` → 404). The client now handles this gracefully (friendly "This post has been deleted" banner, non-blocking). A `targetDeleted` field (or server-side filtering of dead-target notifications) would let the client mark/skip them up front. | 2026-06-03 |
| Rate limits (spec) | — | **Resolved** | The v0.4.1 inline-vs-table contradiction is gone in v0.5.0: the consolidated Rate Limits table now matches the inline per-endpoint limits (Entries 15/day, Replies 15/day, Notes 30/day, Bookmarks 75/day, Profile/Settings 15/day). Read limits were also raised (most list endpoints 30→45/min; profile/follows/topics/bookmarks/notes 20→30/min). Resolved 2026-06-04. | 2026-05-29 |
| `/v1/search` | GET | **Open** | `createdAt` is inconsistent across hit types, and doesn't match the RFC3339 string every other user/post/reply-returning endpoint uses. Confirmed live: a numeric epoch (assumed ms) on user hits, and a raw Firestore Timestamp object (`{"_seconds":N,"_nanoseconds":N}`) on post hits — apparently un-normalized before being sent to the client. Not documented in the API spec. Client-side workaround: `apiTimestamp` (`internal/api/client.go`) accepts string, number, or object for `wireUser`/`wirePost`/`wireReply`'s date fields, and degrades to an empty timestamp rather than failing the whole response for any other shape. | 2026-07-23 |
| `/v1/notifications?type=...` | GET | **Open** | The `type` query param (comma-separated notification-type filter) returns `500 INTERNAL_ERROR` for every value tested live — single types (`type=reply`, `type=chat_mention`, `type=dm_message`) and comma-separated combinations (`type=dm_message,chat_mention`) all fail server-side. Confirmed via `apifetch`. The client's `types []string` param plumbing (`GetNotifications`) is left in place unaffected for when the server bug is fixed; no UI currently sends a non-nil filter, so there's no live-facing regression today. | 2026-07-24 |
| `/v1/users/me`, `/v1/users/:username` | GET | **Resolved (client-side removal)** | `postsCount` was deprecated and no longer returned reliable data; the field is also absent from the current API docs snapshot (`followersCount`/`followingCount` still present). Removed the `posts` segment from the profile counts line and the `PostsCount` field from `model.User`/`wireUser`. | 2026-07-29 |
| `/docs.md` | GET | **Resolved** | `docs.md` now reports v0.8 live. `docs/00-latest-api-reference.md` re-fetched and diffed — new surface (flagging, cIRC message delete, message attachments/styles/mute commands, `EMAIL_NOT_VERIFIED`) added to Unimplemented API Features below. | 2026-07-31 |
| `/v1/notifications` vs `/v1/notifications/unread-count` | GET | **Open (by design, per docs)** | The two endpoints disagree on a fresh, unpaginated session: `unread-count` returned `{"count": 5}` while `?limit=20&read=false` returned only 3 items (2 `thread_reply`, 1 `new_post_following`) — a `new_follower` and a `poke` notification counted in the badge were absent from the list. Confirmed live via `apifetch` 2026-08-03. Matches the documented caveat at `docs/00-latest-api-reference.md:690` ("the count may be slightly higher... which applies additional filtering"), so this is expected server behavior rather than a bug, but it means the TUI's badge and its notification list can legitimately disagree — not a TUI regression. No client-side fix possible; investigated after a user report of "webui shows 5 unread, tui shows 3." | 2026-08-03 |
| `/docs.md` | GET | **Resolved** | `docs.md` now reports v0.8.1 live. `docs/00-latest-api-reference.md` re-fetched and diffed — only change is cIRC presence idle tracking (`lastActivity`/`idleAfterMs`) and reworked presence/typing rate limits (per-room/per-conversation caps replacing flat per-minute ones); added to Unimplemented API Features below. | 2026-08-03 |
| `/docs.md` | GET | **Resolved** | `docs.md` now reports v0.8.2 live. `docs/00-latest-api-reference.md` re-fetched and diffed — only change is a newly-documented rate limit (10/min, 60/hour per IP) on `POST /v1/auth/check-username`, which is already out of scope for this client (web-only registration flow). No code changes needed. | 2026-08-03 |

---

## Unimplemented API Features

Features present in the v0.4 API spec that are not yet implemented in this client.
Ordered roughly by implementation effort / priority.

### Auth

| Endpoint | Method | Description | Priority |
|---|---|---|---|
| `/v1/auth/register` | POST | ~~Removed in v0.5.1~~ — registration is web-only; endpoint no longer in the API spec | N/A |
| `/v1/auth/resend-verification` | POST | Resend email verification link | Out of scope — web-only flow |
| `/v1/auth/check-username` | POST | Check if a username is available (no auth required) | Out of scope — only relevant alongside registration |

### Posts

| Endpoint | Method | Description | Priority |
|---|---|---|---|
| `/v1/users/:username/posts` | GET | Paginated post history for a user | **Done** — profile Posts tab |
| `/v1/users/:username/replies` | GET | Paginated reply history for a user | **Done** — profile Replies tab |

### Posts (extended — v0.4)

| Endpoint / Area | Description | Priority |
|---|---|---|
| `model.Post` fields | `Title`, `Slug`, `GuildID`, `GuildSlug`, `IsGuildThread` | **Done** — feature 28 |
| `POST /v1/posts` signature | Extended: `CreatePost(content, title, topics, isPublic, isNSFW)` | **Done** — feature 28 |
| `GET /v1/users/:username/posts/:slug` | Slug-based post lookup not in Client interface. Useful for deep-linking; not needed for core navigation. | Low |
| `POST /v1/posts` — optional `slug` field (v0.7) | Custom slug (`a-z0-9-`, max 60 chars); server generates one if omitted. Compose panel (`PostComposePanel`) now includes a slug field with inline validation; empty slug is silently omitted from the wire. Same applies to `POST /v1/guilds/:slug/posts`. | **Done** — v0.7 alignment |

### Guilds (new in v0.4)

Guilds are member groups with their own forum of threads. A user can belong to one guild at a time. `Guild` model type added; read-only browsing implemented in feature 29.

| Endpoint | Method | Description | Status |
|---|---|---|---|
| `/v1/guilds` | GET | List guilds (paginated, most-populated first) | **Done** — feature 29 |
| `/v1/guilds/:slug` | GET | Get guild detail + caller's `isMember` / `role` | **Done** — fetched alongside thread list; used for membership hint bar |
| `/v1/guilds/:slug/members` | GET | List guild members (paginated, oldest-joined first) | **Done** — feature 29 |
| `/v1/guilds/:slug/posts` | GET | List guild threads (most recently active first) | **Done** — feature 29 |
| `/v1/guilds/:slug/posts` | POST | Create guild thread (title + topics supported) | **Done** — feature 29 |
| `/v1/guilds/:slug/join` | POST | Join a guild (one per user; 409 if already in one) | **Done** — `J` key in guild threads view |
| `/v1/guilds/:slug/leave` | POST | Leave a guild (founders blocked via API; 403) | **Done** — `l` key in guild threads view; founders see no action key |

Notes:
- Guild threads are ordinary posts with `guildId`, `guildSlug`, `isGuildThread: true`; replying uses `POST /v1/replies` as normal.
- `guild_new_thread` notifications are display-ready (`notifSummary`/`notifIcon` have explicit cases) and navigation is wired via `TargetID` → `ShowNotificationPostMsg`.
- Notification metadata for guild **replies/posts** uses `metadata.guildSlug` + `metadata.isGuildThread: true` (observed 2026-06-03), **not** `metadata.guildName`. The client decodes `guildSlug` and shows `in #<slug>` on `reply`/`thread_reply`/`new_post_*` (prefers slug over the rarer `guildName`). As of API **v0.5.0** the server documents the notification object and its `metadata` keys (incl. `guildSlug`, `guildName`, `isGuildThread`, `threadId`, `postSlug`, `authorUsername`), closing the earlier doc gap; the client's slug-preference behavior matches the documented schema.
- The `isMember` / `role` fields on `GET /v1/guilds/:slug` were broken in v0.4 but are **fixed in v0.4.1** (verified 2026-06-01 — see Resolved Issues). The `GetGuild()` client method could now be called to read accurate membership state, though the current `User.GuildSlug` approach also works.
- Join/leave are now official v0.4.1 API endpoints. Any authenticated user can also create a guild thread without being a member (explicitly stated in v0.4.1 spec).
- v0.4.1 adds `profilePictureUrl` to both the guild list response and the guild members list response. `Guild` and `GuildMember` model types and wire layer now carry this field (v0.7 alignment); rendering is deferred until imgview support lands in the guild list.

### Guilds (new in v0.4.1)

| Area | Description | Priority |
|---|---|---|
| `profilePictureUrl` on Guild / GuildMember | v0.4.1 adds this field to the guild list response and the member list response. Captured in model and wire layer; rendering deferred (no imgview in guild list yet). | **Done** — v0.7 alignment |
| Guild join (`POST /v1/guilds/:slug/join`) | Now an official API endpoint. One guild per user; 409 if already in one. | **Done** |
| Guild leave (`POST /v1/guilds/:slug/leave`) | Now an official API endpoint. Founders get 403 — must use web. | **Done** |

### C-Mail (new in v0.7)

All REST endpoints and the RTDB SSE subscription are fully implemented. See `docs/08-cmail.md` for details.

| Endpoint | Method | Description | Status |
|---|---|---|---|
| `POST /v1/cmail` | POST | Start or get a conversation by `recipientUsername` (idempotent) | **Done** — `StartConversation` |
| `GET /v1/cmail` | GET | List conversations (unread first, then newest activity) | **Done** — `GetConversations`; populates `UnreadCount`, `LastMessage` |
| `GET /v1/cmail/:conversationId` | GET | Load message history | **Done** — `GetMessages` |
| `POST /v1/cmail/:conversationId` | POST | Send a message | **Done** — `SendMessage` |
| `POST /v1/cmail/:conversationId/read` | POST | Mark conversation as read | **Done** — `MarkCMailRead`; called on conversation open |
| RTDB `dm_messages/<conversationId>` | SSE | Real-time new messages | **Done** — `SubscribeDMs`; skips initial snapshot |
| RTDB `user_conversations/<uid>` | SSE | Live conversation list / unread updates | **Done** — `SubscribeUserConversations`; replaces the old 60s REST poll (`docs/08-cmail.md`, `docs/09-rtdb-cmail.md`) |

### cIRC (new in v0.7)

cIRC REST API is now fully documented. A room is addressed by its `roomId` (slug, e.g. `general`). Real-time reading uses Firebase RTDB SSE.

| Endpoint | Method | Description | Status |
|---|---|---|---|
| `GET /v1/circ` | GET | List rooms available to you (sorted by `sortOrder`, then newest activity) | **Done** — feature 33 |
| `GET /v1/circ/:roomId` | GET | Load room message history (paginated, oldest-first, `before` cursor) | **Done** — feature 33 |
| `POST /v1/circ/:roomId` | POST | Send a message to a room (supports slash commands) | **Done** — feature 33 |
| `POST /v1/circ/:roomId/read` | POST | Mark room as read (drives "new messages" indicator) | **Done** — feature 33 |
| RTDB `chat_messages/<roomId>` | SSE | Subscribe to real-time new messages | **Done** — feature 33 |
| `GET /v1/circ/:roomId/users` | GET | List who's currently in a room | **Done** — cIRC presence |
| `POST`/`DELETE /v1/circ/:roomId/presence` | POST/DELETE | Announce/heartbeat and leave presence | **Done** — cIRC presence |
| RTDB `chat_presence/<roomId>` | SSE | Subscribe to real-time presence changes | **Done** — cIRC presence |

Notes:
- Each room message includes `isChatAdmin` flag — parsed into `model.Message.IsChatAdmin`. No longer shown as a `[admin]` badge on the message line; admin status now lives only in the online-users side panel (see `docs/33-circ.md`).
- Rate limits: 15 sends/min, 300/day, 150/hour; 60 mark-read/min; 60 list-users/min; presence heartbeat/leave 15/min per room (90/min overall, as of v0.8.1 — previously a flat 30/min).
- 403 if room isn't available to you.
- Online-users list: implemented (cIRC presence) — `GET .../users` for the initial list, `chat_presence` RTDB stream for live updates, `POST`/`DELETE .../presence` for announce/heartbeat/leave. Room-list cards also show `onlineCount` from `GET /v1/circ`.
- Slash command rendering: server expands `/me`, `/poke`, `/dice` etc. server-side; no client-side preview yet.
- **Fixed: a single user leaving a room used to wipe the entire presence sidebar.** Confirmed via the `cfg.Debug` raw-event logging: Firebase frames a single user's removal from `chat_presence/<roomId>` as a **`patch`** event at path `/` with just that one key set to `null` (a multi-location update — "touch only these keys"), not a full-snapshot replace. `applyPresenceEvent` (and the identical `applyTypingEvent` for C-Mail's typing indicator) treated *any* event at path `/` — `put` or `patch` — as a full wipe-and-replace, so one person leaving cleared every other online user out of the local state; everyone else then only reappeared as their own next individual heartbeat re-added them — the exact "whole sidebar disappears, then trickles back in" symptom reported. Fixed by distinguishing `put` (genuine full snapshot — replace) from `patch` (merge only the listed keys, `null` deletes that key) at path `/`; the (also unconfirmed at the time) `@mention`-correlation was coincidental — the real trigger is simply anyone leaving the room. See `TestHTTPSubscribeRoomPresence_RootPatchMergesInsteadOfReplacing`/`TestHTTPSubscribeDMTyping_RootPatchMergesInsteadOfReplacing` in `client_test.go`. The temporary `cyber-tui-debug.log` raw-event logging (`internal/api/client.go`'s `SubscribeRoomPresence`, wired in `main.go`) is left in place behind `cfg.Debug` since it proved useful and costs nothing when off.

### Search (new in v0.7)

| Endpoint | Method | Description | Status |
|---|---|---|---|
| `GET /v1/search?q=<query>&type=all` | GET | Full-text search across users, posts, and replies — grouped preview | **Done** — feature 34, `Search()` |
| `GET /v1/search?q=<query>&type=posts\|replies\|users` | GET | Paginated single-category search | **Done** — feature 34, `SearchPosts`/`SearchReplies`/`SearchUsers()` |

Notes:
- `type=all` returns up to 8 hits per group (users/posts/replies), no pagination, no total count. The client treats "exactly 8 hits" as the only available signal that a category may have more — see `docs/34-search.md`.
- `type=posts|replies|users` returns paginated results; the client sends `page` (0-based) and treats the response `cursor` (next page number, or null) as an opaque cursor string, same as every other paginated endpoint — no special-casing needed.
- Search hits reuse the existing `model.User`/`model.Post`/`model.Reply` types; no dedicated hit types were needed. The doc-mentioned extra reply-hit context (`parentPostAuthor`/`parentPostContent`) and user-hit guild fields were not captured — not needed for the current UI (reply hits navigate to the parent post directly; user hits already carry guild fields via the existing `User` type).
- Rate limit: 30/min. Missing `q` → 400 VALIDATION_ERROR.

### Commands (new in v0.7)

Both cIRC and C-Mail support IRC-style slash commands expanded server-side: `/me`, `/poke`, `/hug`, `/hi5`, `/slap` (with optional `[@user]`), `/dice <notation>`, `/8ball <question>`, `/fortune`, `/help`. Malformed commands return 400. `/help` posts nothing; its `{ data: { reply } }` is captured by `SendRoomMessage`/`SendMessage` (**Done**) and shown as a local system notice — see `docs/33-circ.md` / `docs/08-cmail.md`.

`/me` and the other emotes set an `isAction` field (with `content` stripped of the username). Previously observed only via live-testing; as of v0.8 this is officially documented under [Message fields](#message-fields) in `docs/00-latest-api-reference.md`, along with `isDice`, `isEightball`/`eightballAnswer`, `isFortune`/`fortuneText`. `model.Message.IsAction` (**Done**) renders these as classic IRC `* username body *` lines.

### Flagging / Reporting (new in v0.8)

`POST /v1/posts/:id/flag`, `POST /v1/replies/:id/flag`, `POST /v1/circ/:roomId/messages/:messageId/flag` — report content for review. Idempotent (200 + `alreadyFlagged` on repeat), optional `reason` (max 500 chars), can't flag your own content, no way to withdraw. Shared rate limit: 5/min, 20/hour, 50/day.

| Endpoint | Method | Description | Priority |
|---|---|---|---|
| `/v1/posts/:id/flag` | POST | Report a post | **Done** — `!` key in Feed and Post Detail; see `docs/35-flagging.md` |
| `/v1/replies/:id/flag` | POST | Report a reply | **Done** — `!` key in Post Detail; see `docs/35-flagging.md` |
| `/v1/circ/:roomId/messages/:messageId/flag` | POST | Report a cIRC message | **Done** — `!` while browsing messages (`up`/`down`); see `docs/36-circ-message-flagging.md` |

### cIRC message delete (new in v0.8)

| Endpoint | Method | Description | Priority |
|---|---|---|---|
| `/v1/circ/:roomId/messages/:messageId` | DELETE | Soft-delete own cIRC message (`content` → `[DELETED]`, attachments stripped); arrives to other clients as an RTDB `patch`, not a new message | **Done** — `d` while browsing messages (own messages only), with live propagation to other clients via the now-handled RTDB `patch` event; see `docs/36-circ-message-flagging.md` |

### Message attachments & styles (new in v0.8)

cIRC/C-Mail messages can now carry `imageUrl`, `gifUrl` (`/gif <url>`), `audioAttachment` (`/song ... — supporter-only`), `style` (chainable text styles via `/blink`, `/l33t`, `/comic`, `/cursive`, `/times`, `/rainbow`, `/flip`, `/quiet`, `/slow`, `/glitch`, `/spoiler`, `/wave`), and ASCII art (`/art`, cIRC-only, base64-encoded `content` when `style: "art"`). `/mute`/`/unmute`/`/muted`/`/unmuteall` manage a per-room, client-side-enforced mute list (also stored in `mutedUsersByRoom` under Settings — currently intentionally omitted from the TUI per the Settings row below).

| Area | Description | Priority |
|---|---|---|
| `gifUrl`, `audioAttachment`, `style`, chained styles | Render/decode in message view; `style: "art"` needs base64 decode | **Done** — wire/model fields across all four message shapes, attachment badges reusing `renderAttachments`, and a middle-fidelity style pipeline (ANSI attributes for blink/quiet/rainbow, Unicode substitution for l33t/cursive/flip, ASCII-safe jitter for glitch, `tea.Tick`-driven slow/wave/glitch animation, select-to-reveal spoiler in cIRC only — see `internal/ui/screens/chatstyle.go`) |
| `/mute` family + `mutedUsersByRoom` | Client-side message filtering by muted user | **Done** — cIRC only (C-Mail 400s per API spec); see `docs/37-circ-mute.md` |
| Empty `content` with attachment-only messages | Message rendering must not assume non-empty `content` | **Done** — covered by the same change; `messageDisplayBody` skips duplicate URL text and empty bodies render without assuming non-empty `content` |

### cIRC idle presence (new in v0.8.1)

`GET /v1/circ/:roomId/users` and the `chat_presence/<roomId>` RTDB stream now carry `lastActivity` (ms epoch, or `null`) per user. `POST /v1/circ/:roomId/presence` accepts an optional `{ "lastActivity": <ms epoch> }` body and its response gains `idleAfterMs`. A user is idle once `lastActivity` is older than `idleAfterMs`; the website shows a 💤 badge. Also reworked: C-Mail typing on/off is now rate-limited 40/min per conversation (120/min overall) rather than a flat 45/min, and cIRC presence heartbeat/leave is 15/min per room (90/min overall) rather than a flat 30/min.

| Area | Description | Priority |
|---|---|---|
| `lastActivity`/`idleAfterMs` idle tracking | Send `lastActivity` on every presence heartbeat (tracked from any keypress while a room is open), plus an extra out-of-cycle, cooldown-guarded heartbeat on every keypress that finds the panel currently showing our own entry as idle (`ChatroomsModel.selfShownIdle`) — corrects a stale server-recorded `lastActivity` immediately rather than waiting for the next scheduled beat. Going idle needs no push of its own; the server computes it passively from the aging last-reported timestamp. Decode `lastActivity` from `GET .../users` and the `chat_presence` RTDB stream (nil = always active). Render a 💤 badge for idle users in the online-users panel, computed at render time off `idleAfterMs` — idle users are never filtered out of the list, only flagged. See `docs/33-circ.md`. | **Done** — 2026-08-03; corrected 2026-08-03 (self-idle badge could get stuck showing idle while actively typing — see `docs/33-circ.md`'s "Waking from idle" bullet) |

### Auth (new in v0.8)

| Error | Description | Priority |
|---|---|---|
| `403 EMAIL_NOT_VERIFIED` | New error code — account access now gated on email verification instead of supporter/API-access-grant. Surfaces from the profile fetch immediately after a successful login (login itself doesn't gate on this); the login screen shows a distinct message with an `r` keybinding to call `POST /v1/auth/resend-verification`, and `friendlyErr` softens the same code for any mid-session authenticated call. See `docs/38-email-verification.md`. | **Done** — 2026-08-03 |

### Thread Watching (new in v0.5.1)

Watching a thread means you receive `thread_reply` notifications when anyone replies to it. Posting a reply auto-watches the thread (controlled by the `autoWatchOnReply` setting, default on).

| Endpoint | Method | Description | Priority |
|---|---|---|---|
| `GET /v1/posts/:id/watch` | GET | Check whether the current user is watching a thread | **Done** |
| `POST /v1/posts/:id/watch` | POST | Watch a thread (idempotent; rate limit: 10/min, 100/day) | **Done** |
| `DELETE /v1/posts/:id/watch` | DELETE | Unwatch a thread | **Done** |
| `GET /v1/watches` | GET | List watched threads — used at startup to populate `◉` icons | **Done** |

Notes:
- `w` key in feed and post detail (root post only) toggles watch with optimistic update. `◉` icon displayed in feed, post detail, guild threads, and topics.
- All pages of `GET /v1/watches` are fetched progressively at login; icon set updates after each page.
- A dedicated "Watched Threads" screen (similar to bookmarks) remains a future low-priority option.
- The `autoWatchOnReply` settings field (v0.5.1) is already surfaced in the Settings screen.

### Notifications (new in v0.4.1)

| Area | Description | Priority |
|---|---|---|
| `type` filter on `GET /v1/notifications` | `?type=reply,reply_mention` — comma-separated list of notification types to fetch. API param is wired client-side (`GetNotifications(..., types []string)`) but the server returns `500 INTERNAL_ERROR` for any value (confirmed 2026-07-24, see Known API Issues above) — a UI filter control cannot be built until the server bug is fixed. | Blocked — server bug, not UI-deferred |
| `chat_mention` / `dm_message` navigation and context | `chat_mention` now carries `metadata.roomSlug`/`roomName`/`messageContent` (confirmed live 2026-07-24) — captured as `Notification.RoomSlug`/`RoomName`/`MessageContent`, shown as an inline `#room` summary + message preview, and `enter` jumps straight to the room (`OpenRoomMsg`). `dm_message` reuses the existing `StartConversationMsg` conversation-open flow on `enter` (same as the `c` key) rather than adding new metadata fields, since no live `dm_message` example has been observed to confirm its metadata shape. | **Done** — see `docs/15-notifications.md` |
| `dm_message` content preview | No confirmed metadata field carries message content/conversation ID for `dm_message` (unlike `chat_mention`'s `messageContent`). If a future live sighting reveals one, add a `docs/15-notifications.md`-style inline preview matching `post_mention`/`reply_mention`/`chat_mention`. | Low — blocked on live confirmation |

### Replies

| Endpoint | Method | Description | Priority |
|---|---|---|---|
| `/v1/posts/:postId/replies` | GET | Cursor-paginated replies (oldest-first by reply ID) | Deferred — replies are rendered as a tree grouped by `parentReplyId`; paginated loads arrive interleaved across parent/child, requiring tree re-parenting and a full re-render on each page. Cost outweighs benefit at current network scale. |

### Follows

| Endpoint | Method | Description | Priority |
|---|---|---|---|
| `/v1/follows?type=followers` | GET | List the current user's followers | **Done** — profile Followers tab |
| `/v1/follows?userId=...` | GET | Look up followers/following for another user | **Done** — profile Following/Followers tabs |

### Notes

| Endpoint | Method | Description | Priority | Blocker |
|---|---|---|---|---|
| `/v1/notes/:id` | GET | Fetch a single note (optionally a specific revision) | **Done** — used by revision preview |
| `/v1/notes/:id/revisions` | GET | List all revisions for a note | **Done** — journal `h` key |
| `/v1/notes/:id` | PATCH | Update note | **Done** | Fixed in v0.4; re-tested 2026-05-29 |

---

## Partially Implemented Features

| Feature         | What's Done                           | What's Missing                                                       |
| --------------- | ------------------------------------- | -------------------------------------------------------------------- |
| Notes (Journal) | List, create, edit, delete, revision history | — |
| Profile         | View and edit all fields              | —                                                                    |
| Settings        | All TUI-relevant fields editable; `mutedUsersByRoom` read and enforced (feature 37) | `keyboardBindings`, `keyboardPreset` — web UI concepts with no TUI equivalent; intentionally omitted |
| Follows         | Follow, unfollow, list following and followers | — |
| Notifications   | All v0.4 types received and displayed with dedicated text and icons | — |

---

## Resolved Issues

| Endpoint | Description | Resolved |
|---|---|---|
| `GET /v1/guilds/:slug` — `isMember`/`role` | Server-side bug: always returned `false`/`null` in v0.4. Fixed in v0.4.1 — verified 2026-06-01: `isMember: true`, `role: "member"` returned correctly for authenticated member of `technica`. TUI `GetOwnProfile`→`guildSlug` workaround stays as belt-and-braces. | 2026-06-01 |
| v0.4.1 notification types | All 9 new types (`supporter_granted/removed`, `hacker_granted/removed`, `image_permission_granted/removed`, `attachment_permission_granted/removed`, `system_ban`) have dedicated text (`notifSummary`) and icons (`notifIcon`) in `notifications.go`. Implemented prior to v0.4.1 doc update. | — |
| `DELETE /v1/posts/:id` | Delete own post — wired in client, feed, and post detail; `d` key with y/n confirmation | 2026-04-16 |
| `DELETE /v1/replies/:id` | Delete own reply — wired in client and post detail; `d` key with y/n confirmation | 2026-04-16 |
| `PATCH /v1/users/me` (extended) | `websiteName`, `websiteImageUrl`, `locationLatitude`, `locationLongitude` added to model, wire layer, and profile edit form (`e` key) | 2026-04-16 |
| `GET /v1/users/:username/posts` | User post history — profile Posts sub-tab; `tab` to navigate; feature 24 | 2026-04-17 |
| `GET /v1/users/:username/replies` | User reply history — profile Replies sub-tab; feature 24 | 2026-04-17 |
| `GET /v1/follows?type=followers` | Own followers list — profile Followers sub-tab; feature 24 | 2026-04-17 |
| `GET /v1/follows?userId=…` | Any user's follows — profile Following/Followers sub-tabs; feature 24 | 2026-04-17 |
| `GET /v1/notes/:id` | Single note fetch — used for revision preview; feature 25 | 2026-04-17 |
| `GET /v1/notes/:id/revisions` | Note revision history — journal `h` key; feature 25 | 2026-04-17 |
| `PATCH /v1/notes/:id` | Server-side 500 bug resolved in API v0.4. Note editing and revision history fully operational. | 2026-05-29 |
| `POST /v1/posts` (extended) | `CreatePost` now accepts `title`, `isPublic`, `isNSFW`. `Post` model gained `Title`, `Slug`, `GuildID`, `GuildSlug`, `IsGuildThread`. Title rendered in feed/detail/profile/bookmarks. Feature 28. | 2026-05-29 |
| Login / refresh — `rtdbUrl` (v0.7) | Login and token-refresh responses now return `rtdbUrl` (e.g. `https://…europe-west1.firebasedatabase.app`). Previously the RTDB URL was derived from the JWT's project ID, producing the wrong `firebaseio.com` regional domain. `Tokens` model gains `RTDBUrl`; `InitRTDB()` now takes the URL from the API response; `applyRefresh()` also updates `rtdbClient.token` via `SetToken()`. | 2026-07-20 |
| Notification metadata — `postContent` / `replyContent` (v0.7) | `post_mention` and `reply_mention` notifications now carry `postContent` and `replyContent` inline, eliminating the need for a follow-up `GET /v1/posts/:id` round trip. `Notification` model gains `PostSlug`, `PostAuthorUsername`, `PostContent`, `ReplyContent`; content preview is rendered inline in the notification list row. | 2026-07-20 |
| `GET /v1/guilds/:slug/members` | Guild member list — paginated, oldest-joined first; `m` from guild posts view; `enter` navigates to profile. Feature 29. | 2026-05-30 |
| `GET /v1/guilds/:slug` (join/leave flow) | Guild detail (`isMember`, `role`) fetched alongside thread list. `J` to join, `l` to leave with y/n confirmation and membership hint bar. Feature 29. | 2026-06-01 |
| `POST /v1/guilds/:slug/join` | Join guild — `J` key in guild thread feed with confirmation prompt; success banner "✓ Joined #name". Feature 29. | 2026-06-01 |
| `POST /v1/guilds/:slug/leave` | Leave guild — `l` key in guild thread feed with confirmation prompt; success banner "✓ Left #name"; navigates back to guild list. Feature 29. | 2026-06-01 |
| Attachments (image/audio) | Attachment URLs surfaced via `GetFocusedURLs` and opened with `o`. Best-effort handling for a TUI — no further work needed. | 2026-05-30 |
