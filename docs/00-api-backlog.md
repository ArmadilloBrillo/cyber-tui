# API Backlog — Outstanding Features & Known Issues

Tracks gaps between the cyberspace.online API (v0.4) and what is currently implemented in the TUI client.
Update this file whenever a feature is implemented or an issue is discovered/resolved.

---

## Known API Issues (Server-Side Bugs)

These bugs exist in the server — no client-side fix is possible. Report to the API maintainer.

| Endpoint | Method | Status | Description | Discovered |
|---|---|---|---|---|
| `/v1/guilds/:slug` | GET | **Open** | `isMember` field always returns `false`, even for authenticated members. `role` is always `null`. The `/v1/users/me` profile response correctly includes `guildSlug` and should be used instead to derive membership. The TUI works around this by calling `GetOwnProfile` and comparing `guildSlug`. | 2026-05-29 |
| `/v1/follows` | GET | **Open** | Response does not include `followerUsername` or `followedUsername`. Confirmed still missing in v0.4 (re-tested 2026-05-29). The profile Following/Followers tabs fall back to showing a truncated user ID; profile navigation from those tabs is disabled until the API returns usernames. | 2026-04-17 |
| Rate limits (spec) | — | **Open** | Inline per-endpoint docs and the consolidated Rate Limits table contradict each other. Entries: 15/day inline vs 10/day in table. Replies: 15/day vs 10/day. Notes: 30/day vs 20/day. Bookmarks: 75/day vs 50/day. Profile/Settings updates: 15/day vs 10/day. Unknown which is authoritative — report to API maintainer. | 2026-05-29 |

---

## Unimplemented API Features

Features present in the v0.4 API spec that are not yet implemented in this client.
Ordered roughly by implementation effort / priority.

### Auth

| Endpoint | Method | Description | Priority |
|---|---|---|---|
| `/v1/auth/register` | POST | User registration (email, password, username) | Low — existing users don't need this; useful for onboarding new users |
| `/v1/auth/resend-verification` | POST | Resend email verification link | Low |
| `/v1/auth/check-username` | POST | Check if a username is available (no auth required) | Low — only needed if registration is added |

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
| `/v1/guilds/:slug` | GET | Get guild detail + caller's `isMember` / `role` | Not called — server `isMember` bug; membership delegated to server on post |
| `/v1/guilds/:slug/members` | GET | List guild members (paginated, oldest-joined first) | Not implemented |
| `/v1/guilds/:slug/posts` | GET | List guild threads (most recently active first) | **Done** — feature 29 |
| `/v1/guilds/:slug/posts` | POST | Create guild thread (title + topics supported) | **Done** — feature 29 |
| `/v1/guilds/:slug/join` | POST | Join a guild (one per user; 409 if already in one) | Not implemented |
| `/v1/guilds/:slug/leave` | POST | Leave a guild (founders blocked — web only; 403 via API) | Not implemented |

Notes:
- Guild threads are ordinary posts with `guildId`, `guildSlug`, `isGuildThread: true`; replying uses `POST /v1/replies` as normal.
- `guild_new_thread` notifications are already display-ready (`notifSummary`/`notifIcon` have explicit cases) and navigation is wired via `TargetID` → `ShowNotificationPostMsg`.
- The `isMember` field on `GET /v1/guilds/:slug` always returns `false` (server bug); membership enforcement is left to the server — the TUI does not gate compose.
- Join/leave require use of the web interface.

### Replies

_(No unimplemented reply endpoints remaining at medium/high priority)_

### Attachments

| Type | Description | Priority |
|---|---|---|
| Image (`type: "image"`) | Attach an image URL (max 640×640) to posts/replies | Low — TUI cannot display images natively; could show a clickable URL |
| Audio (`type: "audio"`, YouTube) | Attach a YouTube link with artist/title/genre metadata | Low — display as metadata card in TUI |

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
| Settings        | Most fields editable                  | `keyboardBindings`, `keyboardPreset`, `mutedUsersByRoom` not exposed |
| Follows         | Follow, unfollow, list following      | Followers list not fetched                                           |
| Notifications   | All v0.4 types received and displayed with dedicated text and icons | — |

---

## Resolved Issues

| Endpoint | Description | Resolved |
|---|---|---|
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
