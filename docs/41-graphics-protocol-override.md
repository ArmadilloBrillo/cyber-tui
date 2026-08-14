# Graphics Protocol Override

## Problem

Inline/fullscreen image rendering (`internal/ui/imgview`) picks a terminal graphics
protocol automatically: `DetectProtocol()` checks env vars (`KITTY_WINDOW_ID`,
`TERM_PROGRAM`), and if that comes up empty, `ProbeSixel()` sends a DA1 (Primary
Device Attributes) query and waits up to `da1ProbeTimeout` (500ms) for a reply
declaring Sixel support.

Reported on mintty (Git Bash on Windows): images rendered inline only
intermittently, across repeated app restarts, even after raising the probe
timeout. `cfg.Debug` diagnostic logging (temporary `log.Printf` calls in
`ProbeSixel`, see its doc comment) captured the real cause across several
launches — mintty never returned a valid DA1 reply. Two attempts timed out
with no response at all; two others captured `"\x1b[B"`, which is a
Down-arrow key escape sequence (likely mintty's mouse-wheel-to-arrow-key
scroll emulation landing in the probe's read), not a device-attributes
reply. Autodetection can't distinguish "no Sixel support" from "this
terminal doesn't answer DA1 queries the way we expect" — both produce
`ProtocolNone`.

## Fix

A manual override, since mintty genuinely does support Sixel rendering —
it just doesn't answer the detection probe reliably in this environment.
`Config.GraphicsProtocol` (`internal/config/session.go`) and
`imgview.ProtocolFromName` (`internal/ui/imgview/protocol.go`) let a user
bypass `DetectProtocol`/`ProbeSixel` entirely.

Set `"graphicsProtocol"` in `~/.cyber-tui.json`:

| Value | Effect |
|---|---|
| `"kitty"` | Force Kitty graphics protocol (Kitty terminal, Ghostty) |
| `"iterm2"` | Force iTerm2 inline-image protocol (iTerm2, WezTerm) |
| `"sixel"` | Force Sixel graphics (mintty/Git Bash, xterm, foot, mlterm, Konsole, contour, …) |
| `"none"` | Disable inline/fullscreen terminal rendering (always fall back to the OS browser) |
| unset / `""` | Default: autodetect via env vars + DA1 probe |

`cmd/cyber-tui/main.go` checks the override first; if `ProtocolFromName` recognizes
the value, detection and the DA1 probe are skipped entirely.

The same override is also editable live from the Settings screen (nested
under "image viewer", shown only when that's set to `terminal`), as an
`auto`/`kitty`/`iterm2`/`sixel` enum — `"none"` isn't offered there since it
disables the feature rather than picking a protocol, and stays config-file-only.
Choosing `auto` mid-session re-resolves via `imgview.DetectProtocol()` (env
vars) only; it does not re-run the Sixel DA1 probe, since that requires raw
terminal access before Bubble Tea takes over stdin (see `ProbeSixel`'s doc
comment) — a full restart is needed to re-probe Sixel.

## Diagnosing further protocol issues

`cfg.Debug: true` (already-existing `debug` config key) redirects the standard
`log` package to `cyber-tui-debug.log` in the working directory, capturing:

- `DetectProtocol`'s env var readings and the final chosen protocol (`main.go`)
- `ProbeSixel`'s outcome: not-a-terminal, raw-mode failure, DA1 write failure,
  the raw response bytes (or timeout) and the parsed Sixel verdict
  (`internal/ui/imgview/protocol.go`)

This mirrors the diagnostic pattern used for the WezTerm/ConPTY investigation in
`docs/plan-inline-images-improvements.md` §9 (added, used, then stripped back out
once resolved there) — kept here since detection issues on other terminals may
still need it.
