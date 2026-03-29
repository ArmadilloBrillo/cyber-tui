# 01 — Project Scaffold

## Overview

Initial scaffold for `cyber-tui`, a terminal user interface client for [cyberspace.online](https://cyberspace.online) — a retro text-only social network.

Built with Go + Bubble Tea. Can run as a local binary or as an SSH-hosted server (via Wish) so remote users can connect with `ssh <host>`.

---

## Tech Stack

| Concern | Library |
|---|---|
| TUI framework | [Bubble Tea](https://github.com/charmbracelet/bubbletea) — Elm-architecture model/update/view |
| Styling | [Lip Gloss](https://github.com/charmbracelet/lipgloss) — green-on-black cyber palette |
| UI components | [Bubbles](https://github.com/charmbracelet/bubbles) — viewport, textinput |
| SSH hosting | [Wish](https://github.com/charmbracelet/wish) — serve TUI over SSH |

---

## Directory Structure

```
cyber-tui/
├── cmd/cyber-tui/main.go       # entry point
├── internal/
│   ├── api/
│   │   ├── interface.go        # Client interface (all API calls)
│   │   ├── mock.go             # MockClient — fake data for development
│   │   └── client.go           # HTTPClient — stubs, filled in when API is available
│   ├── model/types.go          # shared data types: User, Post, Message, Room, Conversation
│   ├── ui/
│   │   ├── app.go              # root Bubble Tea model, screen router, commands
│   │   ├── theme/theme.go      # Lip Gloss styles and colour palette
│   │   └── screens/            # one file per screen
│   │       ├── login.go
│   │       ├── feed.go
│   │       ├── chatrooms.go
│   │       ├── dms.go
│   │       └── profile.go
│   └── ssh/server.go           # Wish SSH server
└── docs/                       # per-feature documentation (this folder)
```

---

## Screens

| Screen | Key | Description |
|---|---|---|
| Login | — | Username/password form, shown before auth |
| Feed | `1` | Scrollable post feed |
| Chatrooms | `2` | IRC-style room list + message viewport + input |
| CyberMail (DMs) | `3` | Conversation list + message thread + input |
| Profile | `4` | View and edit bio |

**Global keys:** `q` quit, `1`–`4` navigate between screens.

---

## API Layer

The `api.Client` interface defines every interaction with cyberspace.online. The UI only ever talks to this interface — it has no knowledge of whether it's hitting a real server or mock data.

- **`MockClient`** — returns static fake data. Active when `CYBERSPACE_API_TOKEN` is not set.
- **`HTTPClient`** — stubs for the real API. Will be filled in once API access is granted.

Switching from mock to real is a single environment variable change.

---

## Running Locally

```bash
# With mock data (no credentials needed)
go run ./cmd/cyber-tui

# With real API (once access is granted)
export CYBERSPACE_API_TOKEN=your_token
export CYBERSPACE_API_BASE_URL=https://cyberspace.online/api
go run ./cmd/cyber-tui
```

## Running as SSH Server

```bash
export SSH_LISTEN_ADDR=:2222
export SSH_HOST_KEY_PATH=./ssh_host_key
go run ./cmd/cyber-tui

# Users connect with:
# ssh yourserver.com -p 2222
```

---

## Environment Variables

See `.env.example` for all variables. Never commit a populated `.env` file.

| Variable | Default | Description |
|---|---|---|
| `CYBERSPACE_API_TOKEN` | — | API token. If unset, mock client is used |
| `CYBERSPACE_API_BASE_URL` | `https://cyberspace.online/api` | API base URL |
| `SSH_LISTEN_ADDR` | — | If set, starts SSH server on this address (e.g. `:2222`) |
| `SSH_HOST_KEY_PATH` | `./ssh_host_key` | Path to SSH host key file |

---

## Tests

```bash
go test ./...
go vet ./...
```

Tests cover the `MockClient` — all API methods, including failure modes (empty credentials, unknown users). Both `MockClient` and `HTTPClient` are verified to satisfy the `api.Client` interface at compile time.
