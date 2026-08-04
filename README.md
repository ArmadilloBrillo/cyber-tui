# cyber-tui

A terminal user interface client for [cyberspace.online](https://cyberspace.online) — a retro text-only social network.

```
  ██████╗██╗   ██╗██████╗ ███████╗██████╗ ███████╗██████╗  █████╗  ██████╗███████╗
 ██╔════╝╚██╗ ██╔╝██╔══██╗██╔════╝██╔══██╗██╔════╝██╔══██╗██╔══██╗██╔════╝██╔════╝
 ██║      ╚████╔╝ ██████╔╝█████╗  ██████╔╝███████╗██████╔╝███████║██║     █████╗
 ██║       ╚██╔╝  ██╔══██╗██╔══╝  ██╔══██╗╚════██║██╔═══╝ ██╔══██║██║     ██╔══╝
 ╚██████╗   ██║   ██████╔╝███████╗██║  ██║███████║██║     ██║  ██║╚██████╗███████╗
  ╚═════╝   ╚═╝   ╚═════╝ ╚══════╝╚═╝  ╚═╝╚══════╝╚═╝     ╚═╝  ╚═╝ ╚═════╝╚══════╝
```

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea).

---

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/ArmadilloBrillo/cyber-tui/dev/install.sh | sh
```

Downloads the latest release binary for your platform, verifies it against the published `SHA256SUMS`, and installs it to `/usr/local/bin` (falling back to `~/.local/bin` when that is not writable). Then run:

```bash
cyber-tui
```

Prebuilt binaries cover Linux (amd64/arm64), macOS (amd64/arm64), and Windows (amd64). Anywhere else — or against a release that predates a given platform — the script falls back to cloning this repo and building with your local Go toolchain, so the same command works everywhere.

### Update and uninstall

The same script handles updates and removal. Through a pipe, pass the command after `sh -s --`:

```bash
# update to the latest release, in place
curl -fsSL https://raw.githubusercontent.com/ArmadilloBrillo/cyber-tui/dev/install.sh | sh -s -- update

# uninstall
curl -fsSL https://raw.githubusercontent.com/ArmadilloBrillo/cyber-tui/dev/install.sh | sh -s -- remove
```

| Command | Aliases | Description |
|---|---|---|
| `install` | — | Install cyber-tui (the default when no command is given) |
| `update` | `upgrade` | Reinstall the latest release over the current one, in the directory it already lives in |
| `remove` | `uninstall` | Delete the installed binary |
| `help` | `-h`, `--help` | Show usage |

`remove` leaves `~/.cyber-tui.json` alone, so your saved session survives a reinstall — delete that file yourself to clear it.

Overrides:

| Variable | Description |
|---|---|
| `CYBER_TUI_VERSION` | Install a specific tag, e.g. `v0.7.2` (default: latest release) |
| `CYBER_TUI_INSTALL_DIR` | Install to a specific directory |
| `CYBER_TUI_REPO` | Install from a fork, e.g. `you/cyber-tui` |

```bash
curl -fsSL https://raw.githubusercontent.com/ArmadilloBrillo/cyber-tui/dev/install.sh \
  | CYBER_TUI_INSTALL_DIR="$HOME/bin" sh
```

Prefer not to pipe to a shell? Download [`install.sh`](install.sh), read it, then run it — or [build from source](#running).

---

## Features

- **Feed** — browse posts from people you follow; compose new posts with topics; open any post for replies; delete your own posts
- **Post detail** — scrollable pager with threaded replies; compose replies inline; delete your own content
- **Notifications** — reply, follow, poke, and bookmark alerts; mark individual or all as read; jump straight to the referenced post
- **C-Mail** — direct messages with live updates via Firebase RTDB (SSE stream)
- **Journal** — private notes visible only to you; create, edit, and delete notes; browse full revision history
- **Bookmarks** — save and browse bookmarked posts and replies; remove bookmarks inline
- **Topics** — browse all tags sorted by post count; drill into a topic feed
- **Guilds** — browse the guild directory; drill into guild threads; compose new threads; join or leave a guild; view the member list and navigate to member profiles
- **Profile** — view any user's Info, Posts, Replies, Following, and Followers tabs; edit your own bio, website, and location; follow or unfollow users
- **Settings** — notification preferences, content filters, and display options synced to your account
- **Three themes** — `cyber` (bright green-on-black, default), `c64` (Commodore 64), `vt320` (amber VT320)
- **Display density** — toggle between dense and relaxed list views
- **Timezone** — display timestamps in any UTC offset
- **Markdown rendering** — GFM formatting and @mention highlighting in post and reply content
- **Session persistence** — refresh token saved to `~/.cyber-tui.json`; login only required when the token expires

**Not yet fully wired:**

- **Chatrooms** — UI complete; REST integration deferred (server-side paths not finalised)

---

## Requirements

- [Go](https://go.dev) 1.25+ — only needed to build from source; the [install script](#install) fetches a prebuilt binary on Linux, macOS, and Windows

---

## Running

Clone the repository and run directly:

```bash
git clone https://github.com/ArmadilloBrillo/cyber-tui.git
cd cyber-tui
go run ./cmd/cyber-tui
```

Or build a binary first:

```bash
go build -o cyber-tui ./cmd/cyber-tui
./cyber-tui
```

To build with version metadata injected (requires `make`):

```bash
make build
./dist/cyber-tui --version
```

On first run you will be prompted to log in with your cyberspace.online email and password. Your session token is saved to `~/.cyber-tui.json` and subsequent launches auto-login — your password is never stored. Login is only required again when the token expires.

---

## Configuration

All settings live in `~/.cyber-tui.json`. The file is created automatically on first login and written with mode **`0600`** (owner read/write only).

You can add any of the following fields manually:

```json
{
  "apiBaseURL": "https://api.cyberspace.online",
  "useMock": false,
  "debug": false,
  "theme": "cyber",
  "timezone": "UTC",
  "density": "",
  "autoEmail": "you@example.com",
  "autoPassword": "your_password"
}
```

| Field | Default | Description |
|---|---|---|
| `apiBaseURL` | `https://api.cyberspace.online` | Override the API endpoint |
| `useMock` | `false` | Run against built-in mock data (no credentials needed) |
| `debug` | `false` | Print verbose HTTP and RTDB output |
| `theme` | `"cyber"` | Active theme: `"cyber"`, `"c64"`, or `"vt320"` |
| `timezone` | `"UTC"` | Display timezone as a UTC offset label, e.g. `"UTC+2:00"` |
| `density` | `""` | `""` = dense, `"relaxed"` = blank lines between list items |
| `wanderLust` | `true` | Wander mode — randomises your profile location every 12 hours |
| `autoEmail` | — | Pre-fill email on the login screen |
| `autoPassword` | — | Pre-fill password for automatic login on startup ⚠️ |

> ⚠️ **`autoPassword` is not recommended.** Your password is stored in plain text. The preferred flow is to log in once interactively — the app saves a session token and auto-logins on subsequent launches without storing your password. Only set `autoPassword` if you have a specific need (e.g. CI or a kiosk setup) and understand the risk.

---

## Development

```bash
# Run tests
go test ./...

# Static analysis
go vet ./...
staticcheck ./...

# Build
go build -o cyber-tui ./cmd/cyber-tui
```

See `docs/` for per-feature documentation.

---

## Stack

| Concern | Library |
|---|---|
| TUI framework | [Bubble Tea](https://github.com/charmbracelet/bubbletea) |
| Styling | [Lip Gloss](https://github.com/charmbracelet/lipgloss) |
| Components | [Bubbles](https://github.com/charmbracelet/bubbles) |
| SSH hosting | [Wish](https://github.com/charmbracelet/wish) |
| Markdown | [goldmark](https://github.com/yuin/goldmark) |
