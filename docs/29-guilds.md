# Feature 29: Guilds

## Overview

Guilds are member communities on cyberspace.online, each with their own thread forum. The guilds screen lets users browse the guild directory and read guild threads.

A user can belong to one guild at a time. Joining and leaving guilds is done on the web; the TUI is read-only for guild membership.

## Menu placement

Guilds appear in the menu bar between bookmarks and topics:

```
feed | notifications | journal | bookmarks | guilds | topics | profile | settings
```

## Screens

### Guild list (default view)

Displays all guilds that have at least one member, sorted by member count (most populated first). Each row shows:

- Guild icon (emoji, falls back to `◆` if absent)
- Guild name (highlighted when selected)
- Member count (right-aligned)

Navigation:
- `j` / `down` — move down; triggers next-page load when at the bottom of a non-exhausted list
- `k` / `up` — move up
- `enter` — open the selected guild's thread list

### Guild threads (posts view)

Shows threads for the selected guild, most recently active first. Each thread renders identically to a post in the feed, including the `[#guild-slug]` badge.

Entering a guild fetches the thread list (`GET /v1/guilds/:slug/posts`). The server enforces membership on post creation.

Navigation:
- `j` / `down`, `k` / `up` — navigate threads; pagination loads at the bottom
- `enter` — open thread in PostDetail; ESC from PostDetail returns here
- `n` — open compose panel
- `esc` — cancel compose if open, otherwise return to the guild list

### Compose panel

Uses `PostComposePanel` (same as feed) with title, body, and topics fields. Public/NSFW toggles are visible but not sent to the API — guild threads have no visibility or NSFW flag in the API.

- `Tab` / `Shift+Tab` — cycle fields
- `Ctrl+S` — submit
- `Esc` — cancel

After a successful post the thread list reloads.

## API endpoints used

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/v1/guilds?limit=20&cursor=<id>` | Paginated guild list |
| GET | `/v1/guilds/:slug/posts?limit=20&cursor=<id>` | Paginated thread list for a guild |
| POST | `/v1/guilds/:slug/posts` | Create a guild thread (server enforces membership) |

## Implementation

- `internal/model/types.go` — `Guild` struct (includes `IsMember`, `Role`)
- `internal/api/interface.go` — `GetGuilds`, `GetGuild`, `GetGuildPosts`, `CreateGuildPost`
- `internal/api/client.go` — wire types and HTTP implementations
- `internal/ui/screens/guilds.go` — `GuildsModel` (two-view screen + embedded `PostComposePanel`)
- `internal/ui/app.go` — `screenGuilds` enum, `handleGuilds`, load/create commands, menu wiring

## Known limitations / out of scope

- Join and leave guild (`POST /v1/guilds/:slug/join`, `/leave`) are not implemented; use the web interface
- Guild member list (`GET /v1/guilds/:slug/members`) is not implemented
- Public/NSFW toggles are shown in the compose panel but have no effect on guild posts
- The API may not surface title or topics in the thread list on the website
