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

- **Feed** — browse posts from people you follow; compose new posts with topics; open any post for threaded replies; edit or delete your own posts and replies
- **Notifications** — reply, follow, poke, bookmark, and chat-mention alerts; mark individual or all as read; jump straight to the referenced post, guild thread, or chatroom
- **C-Mail** — direct messages with live updates via Firebase RTDB (SSE), typing indicators, sent-line recall, and IRC-style slash commands
- **CIRC** — public chatrooms with live presence (who's online, idle status), `@mention` completion, and the same slash-command set as C-Mail (`/me`, `/dice`, `/8ball`, `/fortune`, `/poke`, text-style commands, and more)
- **Journal** — private notes visible only to you; create, edit, and delete notes; browse full revision history
- **Bookmarks** — save and browse bookmarked posts and replies; remove bookmarks inline
- **Topics** — browse all tags sorted by post count, drill into a topic feed, and mute topics you don't want to see anywhere in the app
- **Guilds** — browse the guild directory; drill into guild threads; compose new threads; join, leave, or get promoted; view the member list and navigate to member profiles
- **Profile** — Info/Posts/Replies/Following/Followers sub-tabs for any user; edit your own bio, website, and location; follow or unfollow
- **Search** — full-text search across users, posts, and replies
- **Settings** — notification preferences, content filters, and display options synced to your account
- **Globe** — a rotating Unicode globe plotting your own location and your guilds' members; can be hidden from the tab bar
- **SSH hosting** — optionally serve the client over SSH ([Wish](https://github.com/charmbracelet/wish)) for unauthenticated demo/kiosk access
- **Inline images** — render post/reply image attachments directly in the terminal (Kitty, iTerm2, or Sixel graphics protocols), with a fullscreen zoom/scale modal
- **Five themes** — `cyber` (bright green-on-black, default), `c64` (Commodore 64), `vt320` (amber VT320), `bland` (uses your terminal's own palette), and `custom` (build your own in an in-TUI editor)
- **Display density** — toggle between dense and relaxed list views
- **Timezone** — display timestamps in any UTC offset
- **Markdown rendering** — GFM formatting and @mention highlighting in post, reply, and chat content
- **Desktop notifications** — optional OS toast (OSC 9) for new C-Mail and activity while the app is backgrounded
- **Session persistence** — refresh token saved to `~/.cyber-tui.json`; login only required when the token expires

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

You can add any of the following fields manually. Most are also editable live from the in-app Settings screen; a few (marked below) are config-file-only.

```json
{
  "apiBaseURL": "https://api.cyberspace.online",
  "useMock": false,
  "debug": false,
  "theme": "cyber",
  "timezone": "UTC",
  "density": "",
  "layout": "",
  "autoEmail": "you@example.com",
  "autoPassword": "your_password"
}
```

**Core**

| Field | Default | Description |
|---|---|---|
| `apiBaseURL` | `https://api.cyberspace.online` | Override the API endpoint |
| `allowInsecureApi` | `false` | Permit a plain `http://` `apiBaseURL` to a non-loopback host |
| `useMock` | `false` | Run against built-in mock data (no credentials needed) |
| `debug` | `false` | Print verbose HTTP and RTDB output |
| `autoEmail` | — | Pre-fill email on the login screen |
| `autoPassword` | — | Pre-fill password for automatic login on startup ⚠️ |

**Display**

| Field | Default | Description |
|---|---|---|
| `theme` | `"cyber"` | `"cyber"`, `"c64"`, `"vt320"`, `"bland"`, or `"custom"` |
| `customPalette` | `null` | Your saved palette for the `"custom"` theme (written by the in-TUI theme editor — not hand-edited) |
| `layout` | `""` | `""`/`"tabs"` = tab bar (default), `"miller"` = sidebar columns |
| `density` | `""` | `""` = dense, `"relaxed"` = blank lines between list items |
| `timezone` | `"UTC"` | Display timezone as a UTC offset label, e.g. `"UTC+2:00"` |
| `hideGlobeTab` | `false` | Hide the Globe tab from the tab bar and navigation |
| `maxThreadDepth` | `3` | How many levels of reply nesting are visually indented in post detail |

**Images** (config-file-only except `imageViewer`, which also has a Settings toggle)

| Field | Default | Description |
|---|---|---|
| `inlineImages` | `false` | Render each post's first image attachment inline in Feed/post detail (experimental) |
| `imageViewer` | `"terminal"` | `"terminal"` shows images in a fullscreen modal; `"browser"` always opens the OS browser |
| `graphicsProtocol` | `""` (autodetect) | Force `"kitty"`, `"iterm2"`, `"sixel"`, or `"none"` when autodetection is unreliable (e.g. mintty/Git Bash) |
| `imageScale` | `0` (= `1.0`) | Fullscreen modal size multiplier relative to the image's native size, `0.2`–`2.0` (also adjustable live with `+`/`-`) |
| `dithering` | `false` | Bayer-ordered dithering/duotone recoloring for terminal-rendered images |
| `ditherSharpness` | `"medium"` | `"rough"`, `"medium"`, `"sharp"`, or `"crisp"` — only applies when `dithering` is on |

**Notifications & behavior**

| Field | Default | Description |
|---|---|---|
| `desktopNotifications` | `false` | OS desktop toast (OSC 9) for new C-Mail/activity while backgrounded |
| `typingIndicatorsDisabled` | `false` | Turn off C-Mail's typing-indicator subsystem |
| `feedManualRefreshOnly` | `false` | Disable Feed's 60s background poll for new-post badges |
| `wanderLust` | `false` | Wander mode — randomises your profile location every 12 hours |

**SSH hosting**

| Field | Default | Description |
|---|---|---|
| `sshListenAddr` | — | Enable SSH server mode, e.g. `":2222"` |
| `sshHostKeyPath` | — | Path to the SSH host key file (default: `./ssh_host_key`) |
| `allowRemoteSsh` | `false` | Permit `sshListenAddr` to bind a non-loopback address — SSH mode performs no authentication, so this stays off by default |

> ⚠️ **`autoPassword` is not recommended.** Your password is stored in plain text. The preferred flow is to log in once interactively — the app saves a session token and auto-logins on subsequent launches without storing your password. Only set `autoPassword` if you have a specific need (e.g. CI or a kiosk setup) and understand the risk.
>
> ⚠️ **SSH server mode is experimental and unauthenticated.** Only bind it beyond loopback (`allowRemoteSsh`) if you understand that anyone who can reach the address gets a full session.

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
