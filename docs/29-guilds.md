# Feature 29: Guilds

## Overview

Guilds are member communities on cyberspace.online, each with their own thread forum. The guilds screen lets users browse the guild directory, read guild threads, and view the member list.

A user can belong to one guild at a time. Joining and leaving guilds is done on the web; the TUI is read-only for guild membership.

## Menu placement

Guilds appear in the menu bar between bookmarks and topics:

```
feed | notifications | journal | bookmarks | guilds | topics | profile | settings
```

## Screens

### Guild list (default view)

Displays all guilds that have at least one member, sorted by member count (most populated first). Each row shows:

- Guild icon (emoji; plain-text icon names from the API fall back to `◆`)
- Guild name (highlighted when selected)
- Member count (right-aligned)
- Bio (second line, truncated to stay clear of the member count; omitted when empty)

When a guild is selected all text steps up to normal brightness for readability.

Navigation:
- `j` / `down` — move down; triggers next-page load when at the bottom of a non-exhausted list
- `k` / `up` — move up
- `enter` — open the selected guild's thread list
- `o` — open the guild's website link in a browser (if the guild has one)

### Guild threads (posts view)

Shows threads for the selected guild, most recently active first. Each thread renders identically to a post in the feed, including the `[#guild-slug]` badge.

Entering a guild fetches the thread list (`GET /v1/guilds/:slug/posts`). The server enforces membership on post creation.

Navigation:
- `j` / `down`, `k` / `up` — navigate threads; pagination loads at the bottom
- `enter` — open thread in PostDetail; ESC from PostDetail returns here
- `m` — open the member list for this guild
- `n` — open compose panel
- `esc` — cancel compose if open, otherwise return to the guild list

### Guild members (members view)

Shows all members of the selected guild, oldest-joined first. Each row shows:

- Role icon (`◆` for founder, `•` for member)
- Username (highlighted when selected)
- Role label and join date (right-aligned)

Fetched from `GET /v1/guilds/:slug/members` with cursor pagination. Banned and shadow-banned members are omitted by the server.

Navigation:
- `j` / `down`, `k` / `up` — navigate members; pagination loads at the bottom
- `enter` — open the member's profile; ESC from the profile returns here
- `esc` — return to the guild thread list

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
| GET | `/v1/guilds/:slug/members?limit=20&cursor=<membershipId>` | Paginated member list for a guild |
| POST | `/v1/guilds/:slug/posts` | Create a guild thread (server enforces membership) |

## Implementation

- `internal/model/types.go` — `Guild` struct (includes `IsMember`, `Role`); `GuildMember` struct
- `internal/api/interface.go` — `GetGuilds`, `GetGuild`, `GetGuildPosts`, `CreateGuildPost`, `GetGuildMembers`
- `internal/api/client.go` — wire types and HTTP implementations
- `internal/ui/screens/guilds.go` — `GuildsModel` (three-view screen + embedded `PostComposePanel`)
- `internal/ui/app.go` — `screenGuilds` enum, `handleGuilds`, load/create commands, menu wiring

## Known limitations / out of scope

- Join and leave guild (`POST /v1/guilds/:slug/join`, `/leave`) are not implemented; use the web interface
- Public/NSFW toggles are shown in the compose panel but have no effect on guild posts
- The API may not surface title or topics in the thread list on the website
