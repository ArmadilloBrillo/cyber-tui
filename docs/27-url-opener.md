# 27 · URL Opener

Press **`o`** on any screen to open URLs from the currently focused item.

---

## Behaviour

| Scenario | Result |
|----------|--------|
| Focused item has no URLs | Key is a no-op |
| Focused item has one URL | Opens immediately |
| Focused item has multiple URLs | Shows picker overlay (↑↓ select, enter open, esc cancel) |
| URL path is `/u/{username}` on cyberspace.online | Navigates to profile in TUI; ESC goes back |
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
| `/u/{username}` | Navigate to that user's profile in the TUI |
| Anything else | Open in browser |

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
