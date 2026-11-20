# Plan: settings local prefs struct

Branch: `chore/settings-prefs-struct` (stacked on `feature/settings-regroup`)

## Goal

Adding a local-only setting today means touching roughly ten places by hand:
a live field and an `original*` field on `SettingsModel`, `SetSaved` (12
positional args) or a dedicated `SetSavedX`, the `SharedConfigMsg` seeding
block, `IsDirty`, the ctrl+s snapshot, the esc revert, `SaveSettingsMsg`,
App's `SaveSettingsMsg` handler, `settingsSavedMsg`, and the `saveConfig`
closure. Group the 15 local prefs into one comparable struct so that a new
setting is one struct field, one row in `settingsGroups`, one line of seeding,
and one line each in App's apply and persist code.

## Non-goals

- App's own fields (`a.wanderLust`, `a.timezone`, ...) stay as they are. They
  are read across `app.go` and `layout.go`; collapsing them is a separate,
  much larger change.
- `SharedConfigMsg` stays flat. Many screens read its fields directly.
- The API-backed `model.Settings` handling (`settings`/`original`,
  `settingsEqual`, `RemoteChanged`) is unchanged.
- No behaviour change. Same rows, same defaults, same save/revert semantics.

## The type

In `internal/ui/screens/settings.go`:

```go
// LocalPrefs are the settings-screen preferences persisted to the config
// file rather than the API.
type LocalPrefs struct {
    WanderLust              bool
    FeedManualRefreshOnly   bool
    TypingIndicatorsEnabled bool
    DesktopNotifications    bool
    ShowGlobeTab            bool
    RevealContentWarnings   bool
    DebugLog                bool
    MaxThreadDepth          int
    Timezone                string
    ImageViewer             string
    GraphicsProtocol        string
    InlineImages            bool
    Dithering               bool
    HardBreakKey            string
    KeywordAlerts           []string
}

func (p LocalPrefs) equal(o LocalPrefs) bool {
    a, b := p, o
    a.KeywordAlerts, b.KeywordAlerts = nil, nil
    return a == b && slices.Equal(p.KeywordAlerts, o.KeywordAlerts)
}
```

`equal` blanks the one slice and compares the rest with `==`. If someone later
adds a second slice or map field, `a == b` stops compiling, which forces them
to extend `equal` rather than silently skipping the field.

Polarity stays as it is at runtime today: `TypingIndicatorsEnabled` and
`ShowGlobeTab` positive, `FeedManualRefreshOnly` as stored. The inversion to
`TypingIndicatorsDisabled` / `HideGlobeTab` stays in App's `saveConfig`
closure.

## Steps

### Step 1: screen internals

`SettingsModel` replaces its 30 pref fields (15 live + 15 `original*`) with
`local, originalLocal LocalPrefs`. `capturing`, `captureErr`, `prefsSeeded`,
`cursor`, sizes and `err` stay.

- `settingsGroups` closures read/write `m.local.X`.
- `SharedConfigMsg` seeding builds one `LocalPrefs` from the message (keeping
  the `Timezone ""` -> `"UTC"` and `ImageViewer ""` -> `"terminal"`
  normalisation) and assigns it to both `local` and `originalLocal`.
  `MaxThreadDepth` (0 shown as 3) and `GraphicsProtocol` ("" shown as
  "auto") are not normalised at seeding today; that handling lives in the
  row closures and stays there.
- `IsDirty`: `!settingsEqual(...) || !m.local.equal(m.originalLocal)`.
  `stringSlicesEqual` is deleted (replaced by `slices.Equal`), including its
  one test caller at `settings_test.go:265`, or step 1 does not compile.
- esc: `m.local = m.originalLocal`.
- `KeywordAlerts()`, `SetKeywordAlerts`, `handleCapture`, `View` read/write
  `m.local`.

Public API (`SetSaved`, the three `SetSavedX`, `SaveSettingsMsg`) is unchanged
in this step, so `app.go` is untouched. The ctrl+s handler builds the
existing flat `SaveSettingsMsg` from `m.local`.

`settings_test.go` field accesses (`m.wanderLust`, `m.originalTimezone`, ...)
are rewritten to `m.local.X` / `m.originalLocal.X`. No dedicated `equal`
test: `==` cannot skip a scalar field, and the existing keyword-alerts
dirty/revert test already covers the slice.

Test: `go test ./internal/ui/...`, `go vet ./...`.

### Step 2: messages and App

- `SaveSettingsMsg` becomes `{Settings model.Settings; Prefs LocalPrefs;
  RemoteChanged bool}`.
- `SetSaved(p LocalPrefs)` replaces `SetSaved(12 args)`, `SetSavedHardBreakKey`,
  `SetSavedRevealCW` and `SetSavedDebugLog`. It also advances `original`
  (as today) and clears `err`.
- `settingsSavedMsg` becomes `{seq int; settings model.Settings; prefs
  screens.LocalPrefs}`.
- App's `SaveSettingsMsg` handler drops its 16 local copies and passes
  `msg.Prefs` through.
- App's `settingsSavedMsg` handler assigns App fields from `msg.prefs`, calls
  `SetSaved(msg.prefs)` once, and its `saveConfig` closure captures `p :=
  msg.prefs` and writes `cfg.X` from `p`. The seq guard, `wasManual` feed poll
  restart, `graphicsProtocolName` change check and debug-log notify text are
  kept as they are.
- Tests updated to the new shapes:
  - `app_test.go`: the 6 `SaveSettingsMsg` literals (lines 2178, 7113, 7157,
    7168, 7299, 7538).
  - `settings_test.go`: every `SetSaved(...)` call; the flat
    `SaveSettingsMsg` field reads (lines 472, 711, 792, 863, 931, 1011, 1206,
    1267, 1309) become `save.Prefs.X`; and
    `TestSettings_HardBreak_SetSavedAdvancesBaseline` (around line 1210),
    which calls `SetSavedHardBreakKey`, becomes a `SetSaved(prefs)` test.

Test: `go test ./...`, `go vet ./...`, plus a manual run: change a local and a
remote setting, ctrl+s, restart, confirm both persisted; esc reverts.

### Step 3: docs

- `docs/23-wander-mode.md` lines 58-73 describe `wanderLust` /
  `originalWanderLust`, `SaveSettingsMsg.WanderLust`, `SetSaved(wanderLust)`
  and the `settingsSavedMsg` pass-through; point them at `LocalPrefs`.
- `docs/00-project-reference.md:359` says `SaveSettingsMsg` "carries updated
  Settings"; add "and LocalPrefs".
- `docs/00-battery-audit.md` and `docs/plan-debug-log.md` are historical
  records and stay as they are.

## Risks and edge cases

- **Seeding order.** `SetSettings` can run before the first `SharedConfigMsg`
  (see the `prefsSeeded` comment). Seeding stays gated by `prefsSeeded`, not by
  `original`, so this is unchanged.
- **Zero-value prefs saved back.** If seeding ever failed, a zero `LocalPrefs`
  would be written to disk (e.g. `ShowGlobeTab=false` hides Globe). Same risk
  as today; covered by the existing
  `TestSettings_SharedConfigMsg_SeedsPrefsAfterSetSettings`.
- **Slice aliasing.** `local` and `originalLocal` share the `KeywordAlerts`
  backing array after seeding/save. Safe because nothing appends to or mutates
  it in place: the keyword editor copies on open (`keywordeditor.go:40`) and
  `SetKeywordAlerts` replaces the slice.
- **nil vs empty keyword list.** `slices.Equal(nil, []string{})` is true, same
  as `stringSlicesEqual` today, so no spurious dirty state.
- **Stale save.** The `settingsSaveSeq` check keeps dropping out-of-order
  completions; only the message payload shape changes.

## Out of scope, noted

- `settings_test.go` is `package screens` (white-box), which the Go rules in
  `go.md` say should be `package screens_test`. Pre-existing; step 1 relies on
  white-box access to `m.local`, so this is left alone.
