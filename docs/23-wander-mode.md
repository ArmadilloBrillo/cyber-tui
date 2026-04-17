# Feature 23 — Wander Mode

## Overview

Wander mode is a local easter egg that silently randomizes the user's profile location (latitude, longitude, and location name) twice per day. It runs entirely in the background with no visible feedback to the logged-in user; failures are discarded silently.

The feature is **opt-out** — it is enabled by default for all users and can be disabled from the Settings screen under the **Local** group.

---

## Behavior

- After login, an immediate check fires (`checkAndWanderCmd`). If wander mode is enabled and at least 12 hours have elapsed since the last update, the profile is updated.
- A recurring hourly ticker (`scheduleWanderCmd`, 1 h interval) performs the same check on every firing.
- On each qualifying update:
  - Latitude is chosen uniformly from `[-90, 90]`, rounded to 4 decimal places.
  - Longitude is chosen uniformly from `[-180, 180]`, rounded to 4 decimal places.
  - Location name is set to `Wandering the world...`.
- Any API error, network failure, or config load error causes the attempt to be silently skipped. The user is never notified.
- On a successful update, `lastWandered` is written back to `~/.cyber-tui.json`.

---

## Disabling Wander Mode

Wander mode does not appear in the Settings screen — it is intentionally hidden as an easter egg. To disable it, set `"wanderLust": false` in `~/.cyber-tui.json` manually:

---

## Config File Fields

Two fields are added to `~/.cyber-tui.json`:

| Field | Type | Default | Purpose |
|---|---|---|---|
| `wanderLust` | bool | `true` | `true` = on; `false` = off |
| `lastWandered` | string | absent (= never) | ISO timestamp of last successful update |

When `wanderLust` is absent from the JSON file, `Load()` inserts a default of `true`. This ensures the feature is on by default for all users including those with existing config files.

---

## Implementation

### `internal/config/session.go`

- `Config.WanderLust bool` — opt-out flag; defaults to `true` when absent from JSON.
- `Config.LastWandered time.Time` — rolling timestamp.
- `IsWanderEnabled() bool` — returns `WanderLust`.
- `ShouldWanderNow(cfg Config) bool` — combines enabled check with 12-hour interval check.
- `WanderInterval` — constant: `12 * time.Hour`.

### `internal/ui/screens/settings.go` / `shared.go`

No changes — wander mode is not exposed in the Settings UI.

### `internal/ui/app.go`

- `wanderTickMsg{}` — internal tick message (1 h interval).
- `wanderDoneMsg{at time.Time}` — returned from `checkAndWanderCmd`; zero `at` means no update.
- `scheduleWanderCmd()` — `tea.Tick(1*time.Hour, ...)`.
- `checkAndWanderCmd()` — loads config, calls `config.ShouldWanderNow`, picks random coords, calls `client.UpdateProfile`, returns `wanderDoneMsg`.
- `afterLoginCmd()` — batches `scheduleWanderCmd()` and `checkAndWanderCmd()` alongside existing init commands.
- Handlers for `wanderTickMsg` and `wanderDoneMsg`.

---

## Rate Limiting

The profile PATCH endpoint allows **10 updates per day**. Wander mode uses at most 2 per day (every 12 hours). This leaves 8 slots for user-initiated profile edits.

---

## Notes

- Wander mode **overwrites** whatever location the user manually set. This is intentional.
- The location name `Wandering the world...` is set on every update regardless of the previous value.
- The hourly ticker is started on login and not cancelled on logout; however, `checkAndWanderCmd` loads the config fresh each time, so disabling the feature takes effect on the next tick without requiring a restart.
