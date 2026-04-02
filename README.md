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

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea). Runs locally or hosted over SSH so others can connect with just `ssh yourserver.com`.

---

## Features

- **Feed** — browse posts from the people you follow; open any post for replies
- **Post detail** — read and write replies with a pager-style scrollable view
- **Profile** — view and edit your bio
- **Session persistence** — refresh token saved to `~/.cyber-tui.json`; login only required when the token expires
- Dense / relaxed display density toggle
- Green-on-black retro aesthetic
- SSH hosting via [Wish](https://github.com/charmbracelet/wish)

**Not yet working** (server-side paths not finalised):

- **Rooms** — UI exists, API not wired
- **CyberMail** — UI exists, sending and receiving not wired

---

## Requirements

- [Go](https://go.dev) 1.24+

---

## Running locally

```bash
git clone git@github.com:ArmadilloBrillo/cyber-tui.git
cd cyber-tui
go run ./cmd/cyber-tui
```

On first run you will be prompted to log in. Your session token is saved to `~/.cyber-tui.json` and subsequent launches auto-login — no password stored. Login is only required again when the token expires.

---

## Configuration

All settings live in `~/.cyber-tui.json`. The file is created automatically on first login and written with mode **`0600`** (owner read/write only — not readable by other users on the system).

You can add any of the following fields manually:

```json
{
  "apiBaseURL": "https://api.cyberspace.online",
  "useMock": false,
  "debug": false,
  "autoEmail": "you@example.com",
  "autoPassword": "your_password",
  "sshListenAddr": "",
  "sshHostKeyPath": "./ssh_host_key"
}
```

| Field | Default | Description |
|---|---|---|
| `apiBaseURL` | `https://api.cyberspace.online` | Override the API endpoint |
| `useMock` | `false` | Run against mock data (no credentials needed) |
| `debug` | `false` | Print verbose RTDB debug output |
| `autoEmail` | — | Pre-fill email for automatic login on startup |
| `autoPassword` | — | Pre-fill password for automatic login on startup ⚠️ |
| `sshListenAddr` | — | Enable SSH server mode (e.g. `:2222`) |
| `sshHostKeyPath` | `./ssh_host_key` | Path to the SSH host key file |

> ⚠️ **`autoPassword` is not recommended.** Your password is stored in plain text. The preferred flow is to log in once interactively — the app saves a session token and auto-logins on subsequent launches without storing your password. Only set `autoPassword` if you have a specific need (e.g. CI or a kiosk setup) and understand the risk.

---

## SSH server mode

Set `sshListenAddr` in `~/.cyber-tui.json` to host the TUI so others can connect via `ssh`:

```json
{
  "sshListenAddr": ":2222",
  "sshHostKeyPath": "./ssh_host_key"
}
```

```bash
go run ./cmd/cyber-tui
```

Users connect with:

```bash
ssh yourserver.com -p 2222
```

---

## Keyboard shortcuts

### Global

| Key | Action |
|---|---|
| `1` | Feed |
| `2` | Rooms |
| `3` | CyberMail |
| `4` | Profile |
| `←` / `→` | Cycle tabs |
| `v` | Toggle dense / relaxed display |
| `q` / `ctrl+c` | Quit |

### Feed

| Key | Action |
|---|---|
| `j` / `↓` | Next post |
| `k` / `↑` | Previous post |
| `enter` | Open post detail |
| `r` | Reply to selected post |
| `n` | New post |

### Post detail

| Key | Action |
|---|---|
| `j` / `↓` | Scroll down / next reply |
| `k` / `↑` | Scroll up / previous reply |
| `r` | Reply |
| `esc` | Back to feed |

### Compose box (reply / new post)

| Key | Action |
|---|---|
| `enter` | Paragraph break |
| `alt+enter` | Submit |
| `esc` | Cancel |

---

## Development

```bash
go test ./...
go vet ./...
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
