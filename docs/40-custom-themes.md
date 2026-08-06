# 40 — Custom Themes

## Overview

Alongside the 4 built-in themes (cyber, c64, vt320, bland), users can build and save their own "custom" theme from inside the TUI. The custom theme is a palette edited via a modal opened from the existing theme picker (`t`), and persisted to `~/.cyber-tui.json`.

Users on cyberspace.online also post custom themes as formatted text blocks (base theme + hex colors + some webui-only font/effect fields). Post Detail's `T` key detects one of these blocks and opens the same theme editor prefilled from it, so trying it out and saving/canceling both reuse the manual editor's existing preview/save/revert contract — see Post Theme Import below.

---

## Data Flow

### Startup

`cmd/cyber-tui/main.go` loads config, then:
```go
if cfg.CustomPalette != nil {
    theme.SetCustomPalette(*cfg.CustomPalette)
}
theme.Set(cfg.Theme)
```
`SetCustomPalette` must be called first so `Set("custom")` has a palette to apply.

### Theme Picker → Editor

1. `t` opens the theme picker (`App.themePickerOpen`), listing `availableThemes = []string{"cyber", "c64", "vt320", "bland", "custom"}`.
2. `↑`/`↓` previews each theme live. On the "custom" row, this only applies if a palette has already been saved (`App.customPalette != nil`) — otherwise the current theme stays put.
3. `e` — available from *any* row, not just "custom" — opens the editor (`App.themeEditorOpen`) prefilled from whichever theme is highlighted: a built-in's literal colors (`theme.BuiltinPalette(name)`) for a built-in row, or the saved custom palette (falling back to `theme.CurrentPalette()` if none saved yet) for the "custom" row. Saving always writes to the "custom" slot regardless of which row it started from — this is how you'd base a new custom theme on `vt320` and tweak a couple of colors, for instance.
4. In the editor: `j`/`k` moves between the 9 color rows, `enter` focuses a row's 6-digit hex buffer. Each row shows a fixed, non-removable `#` followed by 6 editable slots. Typing an alphanumeric character overwrites the slot under the cursor (uppercased automatically) and advances to the next slot — except on the last slot, where the cursor stays put since there's nowhere further to go. `←`/`→` move the cursor within the 6 slots (clamped at both ends); `backspace` clears the current slot and steps back. `enter`/`esc` commits the field and returns to row navigation. Every edit emits `screens.PreviewPaletteMsg`, which `App.handleThemeEditor` applies via `theme.SetCustomPalette` + `refreshViewports()` for live preview.
5. `ctrl+s` (whenever all 9 fields are currently valid `#RRGGBB` — regardless of whether a field is focused or anything has actually changed since the prefill) emits `screens.SaveThemeMsg`. This is checked ahead of the editing/nav-mode split in `Update`, not gated on `IsDirty()`: a `T`/import prefill with zero edits must still be savable as-is. `App.handleThemeEditor` updates `App.customPalette` and persists via `saveConfig` (sets both `cfg.Theme = "custom"` and `cfg.CustomPalette`).
6. `esc` (row-nav mode) emits `screens.CloseThemeEditorMsg`, which reverts to the theme that was active before the editor opened (`App.themeEditorOrig`) and closes without saving. If that theme was `"custom"`, the revert also restores `App.themeEditorOrigPalette` — a snapshot of `theme.CurrentPalette()` taken *before* the preview started — into `theme.customPalette` first; without this, `theme.Set("custom")` alone would just re-apply whatever the abandoned edit left in that shared package-level variable instead of the actual prior saved colors. (This was a real bug in the first version of this feature, fixed alongside Post Theme Import below since that entry path needs the same correct revert.)

### Post Theme Import

1. `PostDetailModel.SetPost` calls `theme.ParsePost(post.Content)` once per post load (not per keystroke) and stores the result in `postTheme *theme.Palette` (nil if no theme block was detected). `HasThemeInPost()` exposes this for the `T` hint (shown in the footer and help modal only when a block was found) and both layouts' `screenHints`.
2. `T`, pressed with the post itself focused (not a reply) and a block detected, emits `screens.PreviewPostThemeMsg{Palette}`.
3. `App`'s handler snapshots `themeEditorOrig`/`themeEditorOrigPalette` (as above), then previews and opens the theme editor exactly like the picker's `e` key — but prefilled from the parsed post palette instead of the current theme. From here it's the same editor: review the swatches, tweak anything, `ctrl+s` to keep it (persists as the new custom theme and activates it) or `esc` to cancel (reverts fully to whatever was active before).
4. Detection is post-body only, not replies — matches how these blocks are actually shared (their own top-level post) and keeps parsing a one-time cost per post load.

See `theme.ParsePost`'s doc comment and the post-field mapping table below for how the block's fields resolve.

### Export / Import

Lets any theme be archived to a plain JSON file and restored as the custom theme — export can target a built-in or the saved custom theme; import only ever writes the "custom" slot.

1. `x` — available from *any* row. Resolves the same way `e`'s prefill does (built-in → `theme.BuiltinPalette(name)`; "custom" → the saved palette, guarded — nothing to export if that row has never been saved) and captures it once into `App.pathPromptExportPalette` before the prompt opens, so a later picker-cursor move can't change what ends up written. Opens `screens.PathPromptModel` (`App.pathPromptOpen`, `pathPromptPurpose = pathPromptExport`), prefilled with `~/cyber-tui-theme.json`.
2. `i` — available from *any* row, no guard. Opens the same prompt with the same default path, `pathPromptPurpose = pathPromptImport`, so a plain export-then-import round trip needs no retyping. Opening the prompt never touches the active theme — nothing changes visually until a file actually validates.
3. `enter` in the prompt emits `screens.PathPromptSubmitMsg{Path}`; `esc` emits `screens.PathPromptCancelMsg{}`. The prompt itself has no filesystem awareness — it's a bare `textinput.Model` plus a settable warning line; all I/O and validation happen in `App.handlePathPrompt`.
4. **Export**: if the (`~`-expanded) path already exists and this exact path string hasn't already been flagged (`App.pathPromptOverwritePending`), the prompt stays open with a warning ("file exists — enter again to overwrite") instead of writing — an identical resubmit proceeds. Otherwise `theme.ExportToFile(path, App.pathPromptExportPalette)` writes the captured palette as indented JSON, mode 0600. Result surfaces via the existing notify banner (`a.notify(notifyInfo/notifyError, ...)`).
5. **Import**: `theme.ImportFromFile(path)` reads and validates the file. On error (missing file, invalid JSON, or valid JSON that fails `Palette.Valid()`), notifies and the prompt closes — nothing about the saved custom palette changes. On success, it's handled exactly like `PreviewPostThemeMsg`: snapshot for correct revert, preview live, open the theme editor prefilled from the imported palette — reviewed and `ctrl+s`-confirmed, never applied blind.
6. **Cancel** (`esc` in the prompt): reverts the active theme to `App.themePickerOrig` — the picker's own live preview-on-navigate may have changed the active theme while browsing rows before `x`/`i` was pressed, so backing out of the prompt restores whatever was active before the picker was ever opened, same as the picker's own `esc`.

### Ephemeral (SSH) Sessions

`saveConfig` already no-ops when `App.ephemeral` is true — editing and live preview work normally in an SSH session, but nothing is written to the host operator's `~/.cyber-tui.json`.

---

## Model

```go
// internal/ui/theme
type Palette struct {
    Foreground string // primary body text, markdown emphasis
    Dimmed     string // secondary/subtle text, strikethrough, separators
    Border     string // inactive borders, tab bar, code gutters
    Accent     string // titles, links, active border/tab
    Highlight  string // @mentions and code text
    Error      string // error messages/banner
    BarText    string // text on colored bars: status bar, logo badge, notify banner
    Self       string // your own username highlight (MeHighlight only)
    Meta       string // status bar secondary info text & hint key-descriptions

    // Reserved — not rendered by the TUI today, carried through for a future
    // post-import feature. Not shown as editor rows.
    Background     string
    CodeBackground string
}

func ValidHex(s string) bool
func (p Palette) Valid() bool     // the 9 rendered fields required; Background/CodeBackground optional
func SetCustomPalette(p Palette)
func CurrentPalette() Palette
func BuiltinPalette(name string) (Palette, bool)  // a built-in's literal colors without switching to it; ok=false for "custom" or an unknown name
func ParsePost(content string) (Palette, bool)
func ExpandHome(path string) (string, error)  // "~/..." → the user's home dir; used by both of the below and by App's overwrite check
func ExportToFile(path string, p Palette) error
func ImportFromFile(path string) (Palette, error)  // fails closed: ErrInvalidThemeFile if the JSON parses but Valid() doesn't hold
```

`Self` and `Meta` were originally one field (`Self`, backed by `ColorWhite`) until it turned out to drive two unrelated things: own-username highlighting *and* the status bar's secondary text/hint descriptions. Split so each row controls exactly one thing — `ColorMeta` is a separate color var from `ColorWhite`, initialized to the same literal in every built-in theme (so no visual change there), letting only custom themes differentiate them.

`builtinPalettes map[string]Palette` (unexported) holds each of the 4 built-ins' full color set as data — the single source of truth `setCyber`/`setC64`/`setVT320`/`setBland` apply from (`applyPalette(builtinPalettes["cyber"])`, etc.), and what lets `ParsePost` (and the exported `BuiltinPalette`, used by the picker's `e`/`x`) read a named built-in's colors without switching the live theme.

### `ParsePost` — detecting a theme block in a post

Fields are named to line up with the theme blocks users post on cyberspace.online (see Overview), so parsing maps them directly:

| Post field | `Palette` field | Notes |
|---|---|---|
| `Foreground` | `Foreground` | |
| `Background` | `Background` | Reserved — the TUI has no fillable full-screen background, so this has no visible effect yet |
| `Dimmed` | `Dimmed` | |
| `Border` | `Border` | |
| `Code` | `Highlight` | The TUI already renders code block/span text in the same color as `@mention` highlights |
| `Code BG` | `CodeBackground` | Reserved — no code-block background rendering exists yet |
| *(no post field)* | `Accent`, `Error`, `BarText`, `Self`, `Meta` | TUI-only roles with no webui-post equivalent; resolved from the base theme (see below) |

`ParsePost(content string) (Palette, bool)`:
1. Looks for the marker line (`/* Cyberspace Custom Theme */`, case-insensitive substring match). Absent → `ok=false`; nothing else matters.
2. Scans up to 30 lines after the marker for `Key: value` lines (case-insensitive key match); everything not in the table above (`Main Font`, `Disable Text Glow`, etc. — webui-only, no `Palette` field) is simply never matched, no special-casing needed to ignore it. Users paste the exported block into a post using whatever markdown styling their client applies — the web UI wraps each line in backticks (e.g. `` `Foreground: #ff5d00` ``), some clients quote it with `> ` blockquote markers instead — so `stripLineDecoration` strips a leading blockquote marker (nested `>>` included) and a wrapping backtick pair from each line before matching. Without this, every field misses the regex (it requires the line to start with a letter) while the marker itself, matched by loose substring, still detects fine. This was a real bug: `T` would enable normally but the preview silently fell back to the current theme for every field, since none of them ever matched.
3. Resolves the base palette: if `Base Theme` names one of our own built-ins (`cyber`/`c64`/`vt320`/`bland`, case-insensitive) → `builtinPalettes[name]`, the actual author-intended values for the unmapped fields; otherwise → `CurrentPalette()` (today's active theme), since inventing values for an unrecognized name would just be guessing.
4. Overlays each recognized color field onto the base **only if `ValidHex` passes** — a malformed or missing individual field silently falls back to the base's value rather than invalidating the whole block (a typo in one line shouldn't discard an otherwise-good theme). `ok=true` as long as the marker was found, even in this degenerate case.

```go
// internal/config
type Config struct {
    // ...
    Theme         string          `json:"theme,omitempty"`         // now includes "custom"
    CustomPalette *theme.Palette  `json:"customPalette,omitempty"` // nil until saved
}
```

---

## Key Bindings

| Key | Context | Action |
|-----|---------|--------|
| `t` | global | Open theme picker |
| `↑`/`↓` or `j`/`k` | picker | Move cursor, live-preview |
| `e` | picker, any row | Open theme editor, prefilled from the highlighted row's theme |
| `enter` | picker | Apply selected theme |
| `esc` | picker | Cancel, revert |
| `j`/`k` | editor (row nav) | Move between color fields |
| `enter` | editor (row nav) | Focus the selected field's hex buffer |
| `alnum key` | editor (editing) | Overwrite the slot under the cursor (uppercased), advance to the next slot (stays put on the last slot) |
| `←`/`→` | editor (editing) | Move within the 6 hex-digit slots, clamped at both ends |
| `backspace` | editor (editing) | Clear the current slot, step back |
| `enter`/`esc` | editor (editing) | Commit the field, back to row nav |
| `ctrl+s` | editor (row nav or mid-edit) | Save the current palette — works regardless of whether a field is focused, and even with no edits at all (e.g. right after `T`/import, to accept it as-is); blocked only if a color is currently invalid |
| `esc` | editor (row nav) | Close without saving, revert theme |
| `T` | Post Detail, post focused (not a reply) | Preview a detected post theme — opens the editor prefilled from it, when `HasThemeInPost()` |
| `x` | picker, any row | Export the highlighted row's theme to a file (guarded only on the "custom" row having nothing saved yet) |
| `i` | picker, any row | Import a theme file — opens the editor prefilled from it for review |
| `enter` | path prompt | Submit the path (a second identical submit confirms an export overwrite) |
| `esc` | path prompt | Cancel — reverts the active theme to whatever it was before the picker opened |

---

## Save Strategy

Same as the Settings screen: edits accumulate in memory (with live preview) until `ctrl+s`. No autosave, no partial writes — an invalid hex value blocks save and shows an inline error instead of persisting a broken palette.

---

## Design Notes

- **Editor is a modal, not a tab.** It's a sub-action of the theme picker (already a modal with live-preview/overlay plumbing built), not an independent destination — adding a tab would mean a new `screen` const, mnemonic, and renumbering across two layouts for no benefit. Post Theme Import reuses the same modal rather than building a second one.
- **Prefill from the active theme**, not blank fields — matches the picker's existing "start from what's on screen" model and means all 9 fields are valid immediately, so `ctrl+s` isn't blocked on first use. Post Theme Import follows the same principle, just prefilling from the parsed post palette instead.
- **Post Theme Import reuses the theme editor wholesale** rather than a bespoke confirm/cancel dialog: `T` just opens `ThemeEditorModel` prefilled differently. This gives the user the ability to tweak a color they don't like before committing, for free, and means there's exactly one preview/save/revert implementation to maintain instead of two.
- **The revert-on-cancel bug**: the first version of the theme editor only stored `themeEditorOrig` (a theme *name*) for reverting on `esc`. When that name was `"custom"`, reverting just called `theme.Set("custom")`, which re-applies whatever `theme.customPalette` currently holds — but live-preview edits (from either the manual editor or a post-theme preview) mutate that same package-level variable, so an abandoned edit was left applied instead of the actual prior saved colors. Fixed by also snapshotting `themeEditorOrigPalette := theme.CurrentPalette()` at the moment preview starts, and restoring it via `theme.SetCustomPalette(...)` before `theme.Set("custom")` on cancel.

---

## Integration Checklist

- [x] `theme.Palette` type + `SetCustomPalette`/`CurrentPalette`/`ValidHex`
- [x] `Config.CustomPalette` field, round-tripped through `~/.cyber-tui.json`
- [x] Theme editor screen (`internal/ui/screens/themeeditor.go`)
- [x] Wired into theme picker (`e` to edit/create) in both layouts (tabs, miller)
- [x] Live preview while editing
- [x] Ephemeral (SSH) sessions: editing works, persistence no-ops
- [x] `builtinPalettes` data table + `theme.ParsePost` (detects and resolves a post's theme block)
- [x] Post Detail's `T` key + `HasThemeInPost()` hint, wired into both layouts
- [x] Revert-on-cancel bug fixed (`themeEditorOrigPalette` snapshot), covered by a regression test
- [x] Markdown-wrapped post fields bug fixed (`ParsePost` strips backticks and blockquote markers per line via `stripLineDecoration`), covered by regression tests using two real posted formats (backtick-wrapped, blockquote-wrapped)
- [x] `theme.ExportToFile`/`ImportFromFile`/`ExpandHome`
- [x] `screens.PathPromptModel` (`internal/ui/screens/pathprompt.go`)
- [x] Wired into theme picker (`x` export / `i` import on the "custom" row) in both layouts
- [x] Export overwrite confirmation (`pathPromptOverwritePending`)
- [x] Import reuses the theme editor for review before commit
