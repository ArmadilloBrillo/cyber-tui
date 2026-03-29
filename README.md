# cyber-tui

A terminal user interface client for [cyberspace.online](https://cyberspace.online) — a retro text-only social network.

```
  ██████╗██╗   ██╗██████╗ ███████╗██████╗ ███████╗██████╗  █████╗  ██████╗███████╗
 ██╔════╝╚██╗ ██╔╝██╔══██╗██╔════╝██╔══██╗██╔════╝██╔══██╗██╔══██╗██╔════╝██╔════╝
 ██║      ╚████╔╝ ██████╔╝█████╗  ██████╔╝███████╗██████╔╝███████║██║     █████╗
 ██║       ╚██╔╝  ██╔══██╗██╔══╝  ██╔══██╗╚════██║██╔═══╝ ██╔══██║██║     ██╔══╝
 ╚██████╗   ██║   ██████╔╝███████╗██║  ██║███████║██║      ██║  ██║╚██████╗███████╗
  ╚═════╝   ╚═╝   ╚═════╝ ╚══════╝╚═╝  ╚═╝╚══════╝╚═╝      ╚═╝  ╚═╝ ╚═════╝╚══════╝
```

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea). Runs locally or hosted over SSH so others can connect with just `ssh yourserver.com`.

---

## Features

- **Feed** — browse posts from the people you follow
- **Rooms** — IRC-style chatrooms
- **CyberMail** — direct messages
- **Profile** — view and edit your bio
- Green-on-black retro aesthetic
- SSH hosting via [Wish](https://github.com/charmbracelet/wish)

> **Note:** cyberspace.online API access is not yet public. The app currently runs against mock data. Real API integration will follow once access is available.

---

## Requirements

- [Go](https://go.dev) 1.21+

---

## Running locally

```bash
# Clone
git clone git@github.com:ArmadilloBrillo/cyber-tui.git
cd cyber-tui

# Run with mock data (no credentials needed)
go run ./cmd/cyber-tui
```

Once API access is available, create a `.env` file (see `.env.example`) and export the variables before running.

---

## SSH server mode

To host the TUI so others can connect via `ssh`:

```bash
export SSH_LISTEN_ADDR=:2222
export SSH_HOST_KEY_PATH=./ssh_host_key
go run ./cmd/cyber-tui
```

Users connect with:

```bash
ssh yourserver.com -p 2222
```

---

## Keyboard shortcuts

| Key | Action |
|---|---|
| `←` / `→` | Navigate between tabs |
| `1` | Feed |
| `2` | Rooms |
| `3` | CyberMail |
| `4` | Profile |
| `q` / `ctrl+c` | Quit |

---

## Development

```bash
# Run tests
go test ./...

# Vet
go vet ./...

# Build binary
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
