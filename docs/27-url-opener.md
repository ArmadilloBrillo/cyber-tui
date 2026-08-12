# 27 · URL Opener

Press **`o`** on any screen to open URLs from the currently focused item.

---

## Behaviour

| Scenario | Result |
|----------|--------|
| Focused item has no URLs | Key is a no-op |
| Focused item has one URL | Opens immediately |
| Focused item has multiple URLs | Shows picker overlay (↑↓ select, enter open, esc cancel) |
| URL path is `/u/{username}`, or a bare `/{username}`, on cyberspace.online | Navigates to profile in TUI; ESC goes back |
| URL path is `/{username}/{slug}` or `/{username}/blog/{slug}` (a post permalink) | Opens the post in PostDetail; ESC returns to whichever screen the link was opened from |
| URL path is `/topics/{slug}` or `/guilds/{slug}` | Opens that topic's or guild's post list in the Topics/Guilds tab |
| URL path is `/topics`, `/guilds`, or `/chat` (bare) | Opens that tab's list view |
| URL path is `/chat/{roomId}` | Opens that Circ room directly, same as a `chat_mention` notification deep link |
| Any other URL | Opens in OS default browser (`xdg-open` / `open`) |

Relative paths (e.g. `/support`) are automatically prefixed with `https://cyberspace.online`.

---

## What counts as a URL

The extractor walks the GFM markdown AST of the focused item's content:

- `[text](url)` — inline links
- `![alt](url)` — images
- `<https://…>` — autolinks

Profile screens expose `WebsiteUrl` and `WebsiteImageUrl` directly (no markdown parsing needed).

---

## Supported screens

| Screen | Source |
|--------|--------|
| Feed | Selected post content |
| Post Detail | Post content (no reply selected) or selected reply content |
| Profile | `WebsiteUrl`, `WebsiteImageUrl`; disabled in edit mode |
| Bookmarks | Selected bookmark's post or reply content |
| Topics | Selected post content (post-list view only; disabled in topic-list view) |
| Journal | Selected note; selected revision in history view; disabled in edit mode |
| Notifications | Not supported (post content not cached in model) |

---

## Internal routing

URLs on `cyberspace.online` or `www.cyberspace.online` are checked for known path patterns before opening a browser:

| Path pattern | Action |
|---|---|
| `/topics/{slug}` | Open that topic's post list in the Topics tab |
| `/topics` | Open the Topics tab (list view) |
| `/guilds/{slug}` | Open that guild's thread list in the Guilds tab |
| `/guilds` | Open the Guilds tab (list view) |
| `/chat/{roomId}` | Open that Circ room directly (reuses the `chat_mention` deep-link path) |
| `/chat` | Open the Chatrooms tab (room list) |
| `/u/{username}` | Navigate to that user's profile in the TUI |
| `/{username}/{slug}` or `/{username}/blog/{slug}` | Open that post permalink in PostDetail |
| `/{username}` (bare, and not one of the reserved words below) | Navigate to that user's profile in the TUI |
| Anything else | Open in browser |

Checks run in the order above — a top-level path (`topics`, `guilds`, `chat`,
plus a handful of other real cyberspace.online pages with no TUI equivalent —
`search`, `notifications`, `bookmarks`, `settings`, `jukebox`, `globe`,
`fortune`, `wiki`, `webring`, `faq`, `netiquette`, `contact`, `impressum`,
`support`, `terms`, `privacy-policy`, `changelog`, `admin`) is never
misread as a username. This list was built by checking the live site's
`<title>` for each candidate path — cyberspace.online is a client-rendered
SPA that returns HTTP 200 for almost any path, so only the title reveals
whether a path is a real distinct page (`Jukebox • Cyberspace`) or the
generic username catch-all (`@jukebox • Cyberspace`).

---

## New packages

`internal/ui/urlutil/`
- `extract.go` — `ExtractURLs(markdown)`, `NormalizeURL(url)`
- `open.go` — `OpenURL(url)` — fire-and-forget OS browser launch

---

## Keyboard shortcuts

| Key | Context | Action |
|-----|---------|--------|
| `o` | Any screen (except login) | Open URL from focused item |
| `↑` / `k` | URL picker | Move selection up |
| `↓` / `j` | URL picker | Move selection down |
| `enter` | URL picker | Open selected URL |
| `esc` | URL picker | Close picker |
