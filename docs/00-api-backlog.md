# API Backlog — Outstanding Features & Known Issues

Tracks gaps between the cyberspace.online API (v0.3.2) and what is currently implemented in the TUI client.
Update this file whenever a feature is implemented or an issue is discovered/resolved.

---

## Known API Issues (Server-Side Bugs)

These bugs exist in the server — no client-side fix is possible. Report to the API maintainer.

| Endpoint | Method | Status | Description | Discovered |
|---|---|---|---|---|
| `/v1/notes/:id` | PATCH | **Open** | Returns 500 Internal Server Error for all note update requests, even with a minimal valid body (`{"content":"..."}`). CREATE works fine. Confirmed via curl with a fresh token — not a client bug. Client-side: note creation, editing, and revision history are all disabled via `noteWriteDisabled: true` in `JournalModel` until this is resolved. | 2026-04-16 |
| `/v1/follows` | GET | **Open** | Response does not include `followerUsername` or `followedUsername`. The profile Following/Followers tabs fall back to showing a truncated user ID; profile navigation from those tabs is disabled until the API returns usernames. | 2026-04-17 |

---

## Unimplemented API Features

Features present in the v0.3.2 API spec that are not yet implemented in this client.
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
| `/v1/notes/:id` | GET | Fetch a single note (optionally a specific revision) | **Done** — used by revision preview; disabled client-side while `noteWriteDisabled` is set |
| `/v1/notes/:id/revisions` | GET | List all revisions for a note | **Done** — journal `h` key; disabled client-side alongside write operations |
| `/v1/notes/:id` | PATCH | Update note (already wired client-side) | High | **Server-side 500 bug — blocked until fixed** |

---

## Partially Implemented Features

| Feature         | What's Done                           | What's Missing                                                       |
| --------------- | ------------------------------------- | -------------------------------------------------------------------- |
| Notes (Journal) | List, delete | Create, edit, revision history disabled client-side (`noteWriteDisabled: true`) pending PATCH server fix |
| Profile         | View and edit all fields              | —                                                                    |
| Settings        | Most fields editable                  | `keyboardBindings`, `keyboardPreset`, `mutedUsersByRoom` not exposed |
| Follows         | Follow, unfollow, list following      | Followers list not fetched                                           |

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
