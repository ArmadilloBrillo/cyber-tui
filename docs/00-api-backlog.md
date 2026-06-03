# API Backlog — Outstanding Features & Known Issues

Tracks gaps between the cyberspace.online API (v0.4.1) and what is currently implemented in the TUI client.
Update this file whenever a feature is implemented or an issue is discovered/resolved.

---

## Known API Issues (Server-Side Bugs)

These bugs exist in the server — no client-side fix is possible. Report to the API maintainer.

| Endpoint | Method | Status | Description | Discovered |
|---|---|---|---|---|
| `/v1/follows` | GET | **Open** | Response does not include `followerUsername` or `followedUsername`. Confirmed still missing in v0.4 (re-tested 2026-05-29). The profile Following/Followers tabs fall back to showing a truncated user ID; profile navigation from those tabs is disabled until the API returns usernames. | 2026-04-17 |
| `/v1/notifications` | GET | **Open (by design?)** | Notifications can point to posts that have since been deleted, and the notification object exposes no "target deleted/unavailable" flag — opening one is the only way to discover the target is gone (`GET /v1/posts/:id` → 404). The client now handles this gracefully (friendly "This post has been deleted" banner, non-blocking). A `targetDeleted` field (or server-side filtering of dead-target notifications) would let the client mark/skip them up front. | 2026-06-03 |
| Rate limits (spec) | — | **Open** | Inline per-endpoint docs and the consolidated Rate Limits table still contradict each other in v0.4.1. Entries: 15/day inline vs 10/day in table. Replies: 15/day vs 10/day. Notes: 30/day vs 20/day. Bookmarks: 75/day vs 50/day. Profile updates: 15/day vs 10/day. Guild threads/join/leave are now consistent. Unknown which is authoritative — report to API maintainer. | 2026-05-29 |

---

## Unimplemented API Features

Features present in the v0.4 API spec that are not yet implemented in this client.
Ordered roughly by implementation effort / priority.

### Auth

| Endpoint | Method | Description | Priority |
|---|---|---|---|
| `/v1/auth/register` | POST | User registration (email, password, username) | Out of scope — registration and account management are web-only flows |
| `/v1/auth/resend-verification` | POST | Resend email verification link | Out of scope — same as above |
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
- `guild_new_thread` notifications are already display-ready (`notifSummary`/`notifIcon` have explicit cases) and navigation is wired via `TargetID` → `ShowNotificationPostMsg`.
- The `isMember` / `role` fields on `GET /v1/guilds/:slug` were broken in v0.4 but are **fixed in v0.4.1** (verified 2026-06-01 — see Resolved Issues). The `GetGuild()` client method could now be called to read accurate membership state, though the current `User.GuildSlug` approach also works.
- Join/leave are now official v0.4.1 API endpoints. Any authenticated user can also create a guild thread without being a member (explicitly stated in v0.4.1 spec).
- v0.4.1 adds `profilePictureUrl` to both the guild list response and the guild members list response. The `Guild` and `GuildMember` model types and wire layer do not yet carry this field.

### Guilds (new in v0.4.1)

| Area | Description | Priority |
|---|---|---|
| `profilePictureUrl` on Guild / GuildMember | v0.4.1 adds this field to the guild list response and the member list response. Not in model types or wire layer. Low value for a TUI but keeps model in sync. | Low |
| Guild join (`POST /v1/guilds/:slug/join`) | Now an official API endpoint. One guild per user; 409 if already in one. | **Done** |
| Guild leave (`POST /v1/guilds/:slug/leave`) | Now an official API endpoint. Founders get 403 — must use web. | **Done** |

### Notifications (new in v0.4.1)

| Area | Description | Priority |
|---|---|---|
| `type` filter on `GET /v1/notifications` | `?type=reply,reply_mention` — comma-separated list of notification types to fetch. Not currently used; could power a future "filter by type" UX. | Low |

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
| Settings        | All TUI-relevant fields editable      | `keyboardBindings`, `keyboardPreset`, `mutedUsersByRoom` — web UI concepts with no TUI equivalent; intentionally omitted |
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
| `GET /v1/guilds/:slug/members` | Guild member list — paginated, oldest-joined first; `m` from guild posts view; `enter` navigates to profile. Feature 29. | 2026-05-30 |
| `GET /v1/guilds/:slug` (join/leave flow) | Guild detail (`isMember`, `role`) fetched alongside thread list. `J` to join, `l` to leave with y/n confirmation and membership hint bar. Feature 29. | 2026-06-01 |
| `POST /v1/guilds/:slug/join` | Join guild — `J` key in guild thread feed with confirmation prompt; success banner "✓ Joined #name". Feature 29. | 2026-06-01 |
| `POST /v1/guilds/:slug/leave` | Leave guild — `l` key in guild thread feed with confirmation prompt; success banner "✓ Left #name"; navigates back to guild list. Feature 29. | 2026-06-01 |
| Attachments (image/audio) | Attachment URLs surfaced via `GetFocusedURLs` and opened with `o`. Best-effort handling for a TUI — no further work needed. | 2026-05-30 |
