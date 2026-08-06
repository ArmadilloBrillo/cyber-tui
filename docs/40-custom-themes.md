# 40 — Custom Themes

## Overview

Alongside the 4 built-in themes (cyber, c64, vt320, bland), users can build and save their own "custom" theme from inside the TUI. The custom theme is an 8-color palette, edited via a modal opened from the existing theme picker (`t`), and persisted to `~/.cyber-tui.json`.

This is out of scope but adjacent: users on cyberspace.online post custom themes as formatted text blocks (base theme + hex colors + some webui-only font/effect fields). A future feature will detect one of these blocks in a post (post detail screen) and apply it via a shortcut. `theme.SetCustomPalette`/`theme.CurrentPalette` are the intended entry point for that feature — see Design Notes below.

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
3. `e` on the "custom" row opens the editor (`App.themeEditorOpen`), prefilled from the saved palette if one exists, otherwise from `theme.CurrentPalette()` (whatever theme is currently active).
4. In the editor: `j`/`k` moves between the 8 color rows, `enter` focuses a row's 6-digit hex buffer. Each row shows a fixed, non-removable `#` followed by 6 editable slots. Typing an alphanumeric character overwrites the slot under the cursor (uppercased automatically) and advances to the next slot — except on the last slot, where the cursor stays put since there's nowhere further to go. `←`/`→` move the cursor within the 6 slots (clamped at both ends); `backspace` clears the current slot and steps back. `enter`/`esc` commits the field and returns to row navigation. Every edit emits `screens.PreviewPaletteMsg`, which `App.handleThemeEditor` applies via `theme.SetCustomPalette` + `refreshViewports()` for live preview.
5. `ctrl+s` (when dirty and all 8 fields are valid `#RRGGBB`) emits `screens.SaveThemeMsg`. `App.handleThemeEditor` updates `App.customPalette` and persists via `saveConfig` (sets both `cfg.Theme = "custom"` and `cfg.CustomPalette`).
6. `esc` (row-nav mode) emits `screens.CloseThemeEditorMsg`, which reverts to the theme that was active before the editor opened (`App.themeEditorOrig`) and closes without saving.

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
```

`Self` and `Meta` were originally one field (`Self`, backed by `ColorWhite`) until it turned out to drive two unrelated things: own-username highlighting *and* the status bar's secondary text/hint descriptions. Split so each row controls exactly one thing — `ColorMeta` is a separate color var from `ColorWhite`, initialized to the same literal in every built-in theme (so no visual change there), letting only custom themes differentiate them.

### Post-field mapping

Fields are named to line up with the theme blocks users post on cyberspace.online (see Overview), so a future "detect theme in a post" feature can map them directly:

| Post field | `Palette` field | Notes |
|---|---|---|
| `Foreground` | `Foreground` | |
| `Background` | `Background` | Reserved — the TUI has no fillable full-screen background, so this has no visible effect yet |
| `Dimmed` | `Dimmed` | |
| `Border` | `Border` | |
| `Code` | `Highlight` | The TUI already renders code block/span text in the same color as `@mention` highlights |
| `Code BG` | `CodeBackground` | Reserved — no code-block background rendering exists yet |
| *(no post field)* | `Accent`, `Error`, `BarText`, `Self`, `Meta` | TUI-only roles with no webui-post equivalent; a post-import would leave these at whatever the base theme already has |

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
| `e` | picker, "custom" row | Open theme editor |
| `enter` | picker | Apply selected theme |
| `esc` | picker | Cancel, revert |
| `j`/`k` | editor (row nav) | Move between color fields |
| `enter` | editor (row nav) | Focus the selected field's hex buffer |
| `alnum key` | editor (editing) | Overwrite the slot under the cursor (uppercased), advance to the next slot (stays put on the last slot) |
| `←`/`→` | editor (editing) | Move within the 6 hex-digit slots, clamped at both ends |
| `backspace` | editor (editing) | Clear the current slot, step back |
| `enter`/`esc` | editor (editing) | Commit the field, back to row nav |
| `ctrl+s` | editor (row nav) | Save custom palette |
| `esc` | editor (row nav) | Close without saving, revert theme |

---

## Save Strategy

Same as the Settings screen: edits accumulate in memory (with live preview) until `ctrl+s`. No autosave, no partial writes — an invalid hex value blocks save and shows an inline error instead of persisting a broken palette.

---

## Design Notes

- **Editor is a modal, not a tab.** It's a sub-action of the theme picker (already a modal with live-preview/overlay plumbing built), not an independent destination — adding a tab would mean a new `screen` const, mnemonic, and renumbering across two layouts for no benefit.
- **Prefill from the active theme**, not blank fields — matches the picker's existing "start from what's on screen" model and means all 8 fields are valid immediately, so `ctrl+s` isn't blocked on first use.
- **Future "detect theme in a post" feature**: call `theme.SetCustomPalette(p)` to preview a parsed palette and `theme.CurrentPalette()` to compare against what's currently active. No further hooks are needed in `theme` for that feature to build on.

### Future: resolving unmapped fields when importing a post's theme

A post always declares a `Base Theme:` name alongside its explicit colors (see Overview). The post's colors only ever cover `Foreground`, `Background`, `Dimmed`, `Border`, and `Code` (→ `Highlight`) — see the post-field mapping table above. That leaves `Accent`, `Error`, `Self`, `Meta`, and `CodeBackground` with nothing to import from the post itself. Resolution order for those, decided but not yet built:

1. **If the post's `Base Theme` matches one of our own built-in themes** (`cyber`, `c64`, `vt320`, `bland`, case-insensitive) — prefill from *that* theme's full palette. We have the actual author-intended values for the unmapped fields sitting in `theme.go`; there's no reason to discard them for a guess.
2. **If the name doesn't match a known built-in** (a webui-only theme, a typo, a future theme not yet ported to the TUI) — prefill from `theme.CurrentPalette()` (today's active theme) instead, since inventing values for an unrecognized theme would just be guessing. This mirrors the manual editor's own prefill behavior (`e` on the picker's "custom" row).
3. Either way, overlay the post's explicit fields on top of whichever base was chosen in step 1 or 2.

**Prerequisite refactor**: step 1 needs a way to read a built-in theme's palette *without* switching to it — `setCyber`/`setC64`/`setVT320`/`setBland` currently only mutate the live `ColorX` vars imperatively, with no way to hand back "what vt320's colors are" short of actually activating vt320 (an unwanted side effect mid-import). The fix is to pull each built-in's hex literals out into a named `Palette` value (e.g. `var vt320Palette = Palette{Foreground: "#FFB000", ...}`) and have `setVT320()` call `applyPalette(vt320Palette)` — a single source of truth instead of literals duplicated across `setX()` and a second lookup table, and it gives the post-import feature its lookup for free. `bland` fits trivially (its palette is all-empty strings); its extra Bold/Underline/border-weight tweaks stay separate imperative code, since those aren't part of `Palette`.

---

## Integration Checklist

- [x] `theme.Palette` type + `SetCustomPalette`/`CurrentPalette`/`ValidHex`
- [x] `Config.CustomPalette` field, round-tripped through `~/.cyber-tui.json`
- [x] Theme editor screen (`internal/ui/screens/themeeditor.go`)
- [x] Wired into theme picker (`e` to edit/create) in both layouts (tabs, miller)
- [x] Live preview while editing
- [x] Ephemeral (SSH) sessions: editing works, persistence no-ops
- [ ] Detect a theme block in a post and apply it (future feature, out of scope here)
