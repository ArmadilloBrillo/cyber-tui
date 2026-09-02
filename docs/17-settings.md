# 17 — Settings

## Overview

User preferences are fetched from the API on login via `GET /v1/settings` and stored in `App.settings`. Settings are broadcast to all screens via `SharedConfigMsg` so they can respond to user preferences (content filtering, display format, etc.) without building a UI screen yet.

This feature aligns the codebase with API v0.3.2, which supports 13 preference fields. Screens can read `msg.Settings` in their Update method. Future work includes a Settings UI screen and syncing changes back via `PATCH /v1/settings`.

---

## Settings Fields

| Field | Type | Purpose | Scope |
|-------|------|---------|-------|
| `Notifications.Bookmark` | bool | Alert on bookmarks | Device-roaming |
| `Notifications.Reply` | bool | Alert on replies | Device-roaming |
| `Notifications.Poke` | bool | Alert on pokes | Device-roaming |
| `FilterNSFW` | bool | Hide posts where `isNSFW == true` in Feed, Topics, Guilds (posts view), and Profile (Posts tab). Bookmarks and PostDetail are unaffected. | Device-roaming |
| `ShowFollowerCount` | bool | Public follower visibility | Device-roaming |
| `AutoWatchOnReply` | bool | Auto-subscribe to thread on reply | Device-roaming |
| `IconTheme` | string | Icon set (not yet modelled) | Device-roaming |
| `TimeDisplayFormat` | string | Time display: `"datetime"`, `"relative"`, `"unix"`, `"swatch"` | Device-roaming |
| `FollowedTopics` | []string | Topics subscribed to | Device-roaming |
| `MutedTopics` | []string | Muted topics — posts tagged with any are hidden. Managed from the Topics tab (`m`), not this screen. See `docs/54-muted-topics.md`. | Device-roaming |
| `ImagePixelSize` | string | Pixel multiplier or preset (e.g., "sharp", "2") | Device-roaming |
| `DefaultPublicPost` | bool | Posts default to public | Device-roaming |

**Note:** `KeyboardBindings` and `MutedUsersByRoom` are opaque JSON objects — not modelled yet.

---

## Data Flow

### On Login

1. `loginCmd` or `tokenLoginCmd` exchanges credentials for tokens and calls `GetOwnProfile()`.
2. `afterLoginCmd` is triggered on successful auth, which:
   - Sets `a.active = screenFeed`
   - Calls `a.broadcastConfig()`
   - Triggers a `tea.Batch` including `a.loadSettingsCmd()`
3. `loadSettingsCmd()` calls `a.client.GetSettings()` and emits `settingsLoadedMsg`.
4. `handleSettings` receives the message, stores in `a.settings`, and calls `a.broadcastConfig()` again.

### Broadcasting to Screens

`broadcastConfig()` sends `SharedConfigMsg{..., Settings: a.settings}` to all active screens. Each screen's `Update` method can inspect `msg.Settings` and respond (e.g., filter NSFW posts if `FilterNSFW` is true).

---

## Model

```go
type NotificationPrefs struct {
    Bookmark bool
    Reply    bool
    Poke     bool
}

type Settings struct {
    Notifications      NotificationPrefs
    FilterNSFW         bool
    ShowFollowerCount  bool
    AutoWatchOnReply   bool
    IconTheme          string
    FollowedTopics     []string
    MutedTopics        []string
    ImagePixelSize     string // preset or multiplier, e.g., "sharp" or "2"
    TimeDisplayFormat string // "datetime", "relative", "unix", or "swatch"
    DefaultPublicPost bool
}
```

---

## API Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/v1/settings` | Fetch user's preference settings |
| `PATCH` | `/v1/settings` | Update one or more settings |

Rate limit: 2/min, 10/day (PATCH only).

---

## Local vs API Settings

### Local-Only (Never sync to server)
- `Timezone` (UTC offset, stored in `~/.cyber-tui.json`)
- `Density` (display compactness)
- `Theme` (color palette)

These are device-specific and should never roam.

### API-Driven (Sync to server)
All fields in the `Settings` struct above. These can be modified from multiple devices and should stay consistent.

---

## What's Not Yet Implemented

1. **Settings UI screen** — no screen to edit preferences yet; deferred to future feature branch.
2. **PATCH on change** — no endpoint calls to sync changes back to the server.
3. **Opaque JSON fields** — `KeyboardBindings` and `MutedUsersByRoom` are not modelled.
4. **Preference application** — most settings not yet used (e.g., filtering NSFW when `FilterNSFW=true`). **Exception:** `TimeDisplayFormat` is now applied to all timestamp displays.

---

## Timezone Note

Timezone (UTC offset) is **not** part of API settings. It remains local-only in `~/.cyber-tui.json` and is controlled via the `z` key picker. The API field `timeDisplayFormat` controls the display format (`"datetime"`, `"relative"`, `"unix"`, or `"swatch"`), which *is* synced.

### Relative time live refresh

`"relative"` mode ("2m ago") is the only display format whose output depends on the current time rather than just the message's own timestamp — every other format's string is fixed once the message exists. In CIRC and C-Mail, message bodies (timestamp included) are memoized in `chatBodyCache`/`cmailBodyCacheEntry` (`internal/ui/screens/render.go`) to avoid re-running the expensive per-message render on every redraw, so nothing forced a relative string to advance once cached — it displayed whatever it said when first rendered until an unrelated field (resize, theme switch, edit) happened to invalidate it.

Fixed via a 20s app-level tick (`relativeTimeTickMsg`, `internal/ui/app.go`) that calls `RefreshRelativeTimestamps()` on whichever of CIRC/C-Mail is active and in `"relative"` mode. To avoid reintroducing the full-history re-render the cache exists to prevent, the refresh is scoped to only the messages currently visible in the viewport (`visibleMessageIDs()`, mirroring `maybeStartStyleAnim`'s existing scoping for the style-animation ticker) — a message scrolled off-screen keeps its last-rendered string until it scrolls back into view, bounding the per-tick cost to viewport height regardless of room/conversation history length.

**Known gap:** every other screen using `displayTime`'s `"relative"` mode (Notifications, Feed, Guilds, Journal, PostDetail, Search, Bookmarks) has the same missing-timer problem but not the caching half of it — each recomputes its content on every discrete render event rather than memoizing it, so there's no shared render entry point to hook a scoped refresh into the way CIRC/C-Mail's `chatBodyCache` allowed. Not yet fixed.

---

## Settings Screen

The Settings screen is the fourth tab (shortcut `4` or tab navigation with `←` / `→`). It is a full-screen, cursor-navigable list of editable preferences grouped under four headers: **notifications**, **content**, **social**, and **display**.

### Key Bindings

| Key | Action |
|-----|--------|
| `j` / `↓` | Move cursor down |
| `k` / `↑` | Move cursor up |
| `space` / `enter` | Toggle boolean setting |
| `←` / `→` | Cycle enum option (e.g., time format) |
| `ctrl+s` | Save all pending changes |
| `esc` | Revert unsaved changes |

### Save Strategy

Changes are accumulated in memory until `ctrl+s` is pressed. This respects the API rate limit (2 writes/min, 10/day). A dirty indicator appears in the status bar and footer hint area while unsaved changes exist.

### Editable Fields

**Notifications group:**
- Bookmark alerts (bool)
- Reply alerts (bool)
- Poke alerts (bool)

**Content group:**
- Filter NSFW (bool)

**Social group:**
- Show follower count (bool)
- Auto-watch on reply (bool)
- Default public post (bool)

**Display group:**
- Time format (enum: datetime / relative / unix / swatch)

### Deferred Fields

The following settings require complex pickers and are deferred to a future feature branch:
- `IconTheme` — icon set selection
- `FollowedTopics` — topic subscription management
- `ImagePixelSize` — image scaling or presets

`MutedTopics` (muted topics) shipped in Feature 54 — managed from the Topics
tab, not this screen; included in `PATCH /v1/settings`. See `docs/54-muted-topics.md`.

---

## Integration Checklist

- [x] `Settings` type added to `internal/model/types.go`
- [x] `GetSettings()` and `UpdateSettings()` added to `Client` interface
- [x] Mock and HTTP implementations
- [x] Fetch on login via `loadSettingsCmd()`
- [x] Broadcast to screens via `SharedConfigMsg`
- [x] Settings UI screen
- [x] PATCH on settings change
- [x] Use settings to filter/display content in screens (`TimeDisplayFormat` applied to all timestamp displays)
