# Plan: Guild Join / Leave

## Context

Users currently have no way to join or leave guilds from the TUI — the feature was deferred pending the `isMember`/`role` fields being fixed in the API (resolved in v0.4.1). The server endpoints exist (`POST /v1/guilds/:slug/join` and `POST /v1/guilds/:slug/leave`) and the client-side `GetGuild` method already populates `isMember`/`role`. This feature closes the backlog item and enables the full guild workflow inside the TUI.

**Scope:** guild posts view (`viewGuildPosts`) only — that's where the user has full guild context.  
**API constraints:**  
- A user can only be in **one guild at a time** (409 if joining while already a member elsewhere).  
- Founders **cannot leave** (API returns 403).

---

## Keyboard Shortcuts

| Key | Action | Condition |
|-----|--------|-----------|
| `J` (shift+j) | Initiate join | In guild posts view, not a member |
| `l` (lowercase L) | Initiate leave | In guild posts view, member but not founder |

**Rationale:** `j` is taken (navigate down), so `J` is the natural join key. `l` is unmapped in the guild posts view and is mnemonic for "leave". Neither `m` (members), `n` (new thread), nor `esc` conflicts.

**Alternative** (if user prefers): `+` to join, `-` to leave — semantic and keyboard-friendly.

---

## UI Patterns

### Membership hint bar
Add a **1-line hint bar** rendered below the viewport in `viewGuildPosts`. This communicates membership status and available shortcuts without requiring the user to guess. States:

- Detail loading: `  loading guild info…`  (subtle)
- Not a member: `  not a member  [J] join`  (subtle + highlight for key)
- Member: `  member  [l] leave`  (base + highlight for key)
- Founder: `  ◆ founder`  (highlight, no action available)

The hint bar height (1 line wrapped by a border = 3 lines) is subtracted from `viewportHeight()`.

### Confirmation prompt
Follows the journal/feed pattern exactly — a bordered prompt replaces the hint bar and intercepts all keypresses:

- **Join:** `Join #guild-name?  [y]es  [n]o / esc`  — `theme.Highlight`
- **Leave:** `Leave #guild-name?  [y]es  [n]o / esc`  — `theme.Error`

### Notification banners (app-level)
On success, a `notifyMsg` banner auto-dismisses after 4 seconds:

- Joined: `✓ Joined #guild-name`  (notifyInfo / green)
- Left: `✓ Left #guild-name`  (notifyInfo / green)
- API error (409, 403, network): `✕ <error text>`  (notifyError / red, via `actionErrMsg`)

---

## Implementation Steps

### 1 · API client (`internal/api/interface.go` + `internal/api/client.go`)

Add to interface:
```go
JoinGuild(slug string) error
LeaveGuild(slug string) error
```

Implement in client (each does a `POST /v1/guilds/:slug/join|leave` with an empty body, returns error on non-2xx).

### 2 · Screen model (`internal/ui/screens/guilds.go`)

**New fields on `GuildsModel`:**
```go
activeGuildDetail  model.Guild   // populated by GetGuild; has IsMember/Role
guildDetailLoaded  bool          // false until detail fetch completes
confirming         guildsConfirm // enum: confirmNone | confirmJoin | confirmLeave
```

**New confirm enum:**
```go
type guildsConfirm int
const (
    confirmNoneG  guildsConfirm = iota
    confirmJoinG
    confirmLeaveG
)
```

**New message types:**
```go
type JoinGuildMsg  struct{ Slug string }
type LeaveGuildMsg struct{ Slug string }
```

**New setter:**
```go
func (m GuildsModel) SetGuildDetail(g model.Guild) GuildsModel
```
Sets `activeGuildDetail`, `guildDetailLoaded = true`, refreshes content.

**Key handling in `Update`** (inserted before existing key cases, in the `viewGuildPosts` branch):

```
confirming != confirmNone → route to handleConfirmKey()

"J" in viewGuildPosts + !isMember + guildDetailLoaded → set confirming = confirmJoinG
"l" in viewGuildPosts + isMember + role != "founder" → set confirming = confirmLeaveG
```

`handleConfirmKey`:
- `"y"` → emit `JoinGuildMsg` or `LeaveGuildMsg`, clear confirming
- `"n"` / `"esc"` → clear confirming

**`viewportHeight` adjustment:**
```go
if m.view == viewGuildPosts {
    h -= 3  // hint bar (1 content line + border top/bottom)
}
```
(When confirming, the hint bar is replaced by the confirm prompt — same height.)

**`View()` addition:**
```go
if m.view == viewGuildPosts {
    bar := m.renderMembershipBar()
    return lipgloss.JoinVertical(lipgloss.Left, m.viewport.View(), bar)
}
```

**`renderMembershipBar()` / `confirmPrompt()`:** Render hint bar or confirm prompt depending on `m.confirming`.

**`esc` in `viewGuildPosts`:** If `confirming != confirmNone`, cancel confirm instead of navigating back (add guard at top of esc handler).

**Clear state on leaving posts view:** When `esc` navigates back to list, reset `activeGuildDetail`, `guildDetailLoaded = false`, `confirming = confirmNoneG`.

### 3 · App wiring (`internal/ui/app.go`)

**Fetch guild detail alongside posts:**  
In `LoadGuildPostsMsg` handler, return `tea.Batch(a.loadGuildPostsCmd(slug), a.loadGuildDetailCmd(slug))`.

**New internal message + command:**
```go
type guildDetailLoadedMsg struct{ guild model.Guild }

func (a *App) loadGuildDetailCmd(slug string) tea.Cmd {
    return func() tea.Msg {
        g, err := a.client.GetGuild(slug)
        if err != nil { return actionErrMsg{err} }
        return guildDetailLoadedMsg{guild: g}
    }
}
```

Handle `guildDetailLoadedMsg` → `a.guilds = a.guilds.SetGuildDetail(msg.guild)`.

**New messages + commands for join/leave:**
```go
type guildJoinedMsg struct{ slug, name string }
type guildLeftMsg   struct{ slug, name string }

func (a *App) joinGuildCmd(slug, name string) tea.Cmd { ... }
func (a *App) leaveGuildCmd(slug, name string) tea.Cmd { ... }
```

Handle `screens.JoinGuildMsg` → `joinGuildCmd(slug, detail.Name)`.  
Handle `screens.LeaveGuildMsg` → `leaveGuildCmd(slug, detail.Name)`.

On `guildJoinedMsg`: call `SetGuildDetail` with updated isMember=true, show `notifyMsg{"✓ Joined #name", notifyInfo}`, refresh guild list (so member count updates).  
On `guildLeftMsg`: call `SetGuildDetail` with isMember=false, show `notifyMsg{"✓ Left #name", notifyInfo}`, navigate back to guild list (user is no longer a member).

### 4 · Tests (`internal/ui/screens/guilds_test.go`)

- `SetGuildDetail` stores guild and sets guildDetailLoaded
- `J` key when not a member sets `confirming = confirmJoinG`
- `J` key when already a member does nothing
- `l` key when a member (non-founder) sets `confirming = confirmLeaveG`
- `l` key when founder does nothing
- `y` confirmation emits `JoinGuildMsg` / `LeaveGuildMsg`
- `n` / `esc` clears confirming without emitting
- `esc` while confirming cancels confirm, not navigation
- `viewportHeight` is 3 lines shorter in viewGuildPosts

### 5 · Docs

- `docs/29-guilds.md` — add Join/Leave section describing shortcuts, confirmation, API constraints (one guild per user, founders can't leave)
- `docs/00-api-backlog.md` — mark join/leave as implemented
- `docs/00-project-reference.md` — update guild keyboard shortcuts table

---

## Critical Files

| File | Change |
|------|--------|
| `internal/api/interface.go` | Add `JoinGuild`, `LeaveGuild` to interface |
| `internal/api/client.go` | Implement both methods |
| `internal/ui/screens/guilds.go` | Confirm state, key handling, hint bar, message types |
| `internal/ui/app.go` | Wire messages, guild detail fetch, join/leave commands |
| `internal/ui/screens/guilds_test.go` | Tests for all new behaviour |
| `docs/29-guilds.md` | Document feature |
| `docs/00-api-backlog.md` | Mark items done |
| `docs/00-project-reference.md` | Update shortcuts table |

---

## Verification

1. Run `go test ./...` — all tests pass
2. Run `go vet ./...` and `staticcheck ./...` — no warnings
3. Launch the app (`/run`): navigate to a guild posts view
4. Verify the membership hint bar appears (loading → not a member / member / founder)
5. Press `J` → confirm prompt appears with guild name → `y` → banner "Joined #..." appears → hint bar updates to "member  [l] leave"
6. Press `l` → confirm prompt appears → `y` → banner "Left #..." → navigates back to guild list
7. Press `n`/`esc` on confirm prompt → cancels, hint bar returns
8. As founder: `l` key does nothing
9. API 409 (already in a guild): error banner appears, state unchanged
