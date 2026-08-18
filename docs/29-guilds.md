# Feature 29: Guilds

## Overview

Guilds are member communities on cyberspace.online, each with their own thread forum. The guilds screen lets users browse the guild directory, read guild threads, and view the member list.

A user can belong to their own guild (founder or member — this is the profile "badge" guild) plus up to five apprenticeships in other guilds. Apprentices show up in a guild's member list and get its thread notifications, but the profile badge only ever follows the founder/member guild. Joining, leaving, and promoting an apprenticeship to badge status are all done from the guild threads view using `J`, `L`, and `P`.

## Menu placement

Guilds appear in the menu bar between bookmarks and topics:

```
feed | notifications | journal | bookmarks | guilds | topics | profile | settings
```

## Screens

### Guild list (default view)

Displays all guilds that have at least one member, in the order the API returns them (member count, most populated first), except the logged-in user's own guilds are floated to the top: their badge guild first, then any guilds they're apprenticed to (ordered by member count), then the rest of the list unchanged. If the badge guild or an apprenticed guild isn't present on the first page returned by the API, it's fetched individually (`GET /v1/guilds/:slug`) and injected into the list so it still floats to the top instead of waiting for pagination to reach it. Each row shows:

- Guild icon (emoji; plain-text icon names from the API fall back to `◆`, or to `★`/`☆` for the user's badge guild / an apprenticed guild)
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
- `J` (shift+j) — join this guild (available when not a member or apprentice of it). Joins as a badge member if the user has no badge guild yet, otherwise as an apprentice.
- `L` (shift+l) — leave this guild (available for members and apprentices; blocked for founders)
- `P` (shift+p) — promote this apprenticeship to the profile badge (available only when the user is an apprentice here)
- `esc` — cancel compose or confirm prompt if open, otherwise return to the guild list

The status bar shows contextual hints: `J join` when the user has no role in this guild (once detail loads), `L leave` (+ `P promote` if an apprentice) otherwise, except for founders who get neither. Pressing `J`, `L`, or `P` opens a confirmation prompt at the bottom of the screen. Confirm with `y` or cancel with `n` / `esc`.

**API constraints:**
- The 5-apprenticeship cap and "already a member of this guild" checks are enforced server-side only (409) — the screen does not pre-check them, the same way the founder-leave 403 is left to the server. A capacity or duplicate-membership error surfaces via the standard error banner.
- Founders cannot leave via the API (403). The `L` key is hidden for founders.
- Promoting an apprenticeship 404s if the user isn't in that guild, or 403s if the user founded their current badge guild (must be handled on the web).

### Guild members (members view)

Shows all members and apprentices of the selected guild, oldest-joined first (mixed in one list). Each row shows:

- Role icon (`◆` founder, `•` member, `◇` apprentice)
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
| GET | `/v1/guilds/:slug` | Guild detail including `isMember` and `role` (`"founder"`, `"member"`, `"apprentice"`, or `""`) |
| GET | `/v1/guilds/:slug/posts?limit=20&cursor=<id>` | Paginated thread list for a guild |
| GET | `/v1/guilds/:slug/members?limit=20&cursor=<membershipId>` | Paginated member+apprentice list for a guild |
| GET | `/v1/users/:username/guilds` | All guilds a user belongs to (badge guild + apprenticeships), max 6, unpaginated |
| POST | `/v1/guilds/:slug/posts` | Create a guild thread (server enforces membership) |
| POST | `/v1/guilds/:slug/join` | Join this guild (member if no badge guild yet, else apprentice) |
| POST | `/v1/guilds/:slug/leave` | Leave this guild (founders get 403) |
| POST | `/v1/guilds/:slug/promote` | Make an apprenticeship the new badge guild |

## Implementation

- `internal/model/types.go` — `Guild` struct (`IsMember`, `Role`, `ApprenticeCount`); `GuildMember` struct; `GuildMembership` struct (per-user guilds list)
- `internal/api/interface.go` — `GetGuilds`, `GetGuild`, `GetGuildPosts`, `CreateGuildPost`, `GetGuildMembers`, `GetUserGuilds`, `JoinGuild`, `LeaveGuild`, `PromoteGuild`
- `internal/api/client.go` — wire types and HTTP implementations
- `internal/ui/screens/guilds.go` — `GuildsModel` (three-view screen + embedded `PostComposePanel`); `sortGuildsForDisplay` floats the user's badge guild then apprenticeships (by member count) to the top of the list; `SetOwnGuildSlug`/`SetOwnApprenticeSlugs` re-sort on membership changes
- `internal/ui/screens/profile.go` — apprenticeships row on the Info tab (`SetApprenticeships`)
- `internal/ui/app.go` — `screenGuilds` enum, `handleGuilds`, load/create/promote commands, menu wiring; `ownApprenticeSlugs` (from the logged-in user's own `GetUserGuilds`, distinct from whichever profile `ProfileModel` currently has apprenticeships loaded for) feeds the Guilds tab ordering via `SharedConfigMsg.OwnApprenticeSlugs`. On `guildsLoadedMsg`, any of the badge guild or apprentice slugs missing from the freshly loaded page is fetched individually via `loadOwnGuildIntoListCmd`/`ownGuildInjectMsg` (batched with `tea.Batch`, one fetch per missing slug) and injected into `GuildsModel` with `InjectGuild`/`HasGuild`.

## Known limitations / out of scope

- Public/NSFW toggles are shown in the compose panel but have no effect on guild posts
- The API may not surface title or topics in the thread list on the website
- The profile's apprenticeships row has no keybinding to jump into that guild from the profile screen
