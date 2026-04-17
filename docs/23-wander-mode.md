# Feature 23 — Wander Mode

## Overview

Wander mode is a local easter egg that silently randomizes the user's profile location (latitude, longitude, and location name) twice per day. It runs entirely in the background with no visible feedback to the logged-in user; failures are discarded silently.

The feature is **opt-out** — it is enabled by default for all users and can be toggled from the Settings screen under the **wander** group.

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

## Toggling Wander Mode

Wander mode is accessible in the Settings screen under the **wander** group. Toggle it with Enter/Space, then press `ctrl+s` to save.

It can also be set directly in `~/.cyber-tui.json`:

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

- `settingsGroups` — new `"wander"` group with a single `"wander mode"` bool item (flat index 11).
- `SettingsModel.wanderLust` / `.originalWanderLust` — track the local config value separately from API settings.
- `SharedConfigMsg.WanderLust` — carries the current value in the broadcast from App.
- `SaveSettingsMsg.WanderLust` — piggy-backs the local config change onto the API save message.
- `SetSaved(wanderLust bool)` — resets `originalWanderLust` alongside the API settings baseline.

### `internal/ui/app.go`

- `wanderTickMsg{}` — internal tick message (1 h interval).
- `wanderDoneMsg{at time.Time}` — returned from `checkAndWanderCmd`; zero `at` means no update.
- `scheduleWanderCmd()` — `tea.Tick(1*time.Hour, ...)`.
- `checkAndWanderCmd()` — loads config, calls `config.ShouldWanderNow`, picks random coords, calls `client.UpdateProfile`, returns `wanderDoneMsg`.
- `afterLoginCmd()` — batches `scheduleWanderCmd()` and `checkAndWanderCmd()` alongside existing init commands.
- Handlers for `wanderTickMsg` and `wanderDoneMsg`.
- `App.wanderLust bool` — cached local config value; defaults to `true`; set in `WithSavedSession` and updated when settings are saved.
- `broadcastConfig()` — includes `WanderLust` in the `SharedConfigMsg`.
- `handleSettings` — on save, loads config, writes `WanderLust`, and passes the value through `settingsSavedMsg`.

---

## Rate Limiting

The profile PATCH endpoint allows **10 updates per day**. Wander mode uses at most 2 per day (every 12 hours). This leaves 8 slots for user-initiated profile edits.

---

## Notes

- Wander mode **overwrites** whatever location the user manually set. This is intentional.
- The location name `Wandering the world...` is set on every update regardless of the previous value.
- The hourly ticker is started on login and not cancelled on logout; however, `checkAndWanderCmd` loads the config fresh each time, so disabling the feature takes effect on the next tick without requiring a restart.
