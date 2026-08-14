# Image Modal Scale

## Problem

The fullscreen image modal (`o`, `internal/ui/app.go`'s `openImageInTerminal`)
computes a target cell box (`displayCols`/`displayRows`, 4/5 of the terminal
size) and hands it to whichever encoder matches the detected graphics
protocol (`internal/ui/imgview`: `EncodeKitty`, `EncodeITerm2`, `EncodeSixel`).

That computation is identical across all three protocols — same box, same
cell-pixel-size query, same fallback default. But how a given cell box maps
to actual on-screen pixels is a terminal-side decision the app cannot see or
query:

- iTerm2's `preserveAspectRatio=1` letterboxes the image inside the box using
  iTerm2's own real font metrics — if `cellPxW`/`cellPxH` (from
  `TerminalCellPixelSize`, `TIOCGWINSZ`/`GetCurrentConsoleFontEx`) don't
  exactly match, the image visibly shrinks with blank bars.
- Kitty always stretches to fill the box exactly, so a mismatch there only
  distorts aspect ratio, not size.
- Sixel has no terminal-side scale-to-fit at all — it downscales in pixel
  space itself, so its rendered size never depends on this guess.

None of the three protocols let the app read back what it actually rendered
at, so this can't be auto-corrected — DPI, terminal zoom, and font metrics
vary per machine and are effectively a moving target. See the code comments
in `internal/ui/imgview/{kitty,iterm2,sixel}.go` for the full breakdown.

## Fix

A user-controlled multiplier, `Config.ImageScale` (`internal/config/session.go`),
applied to `displayCols`/`displayRows` before they reach the encoder
(`openImageInTerminal`, `internal/ui/app.go`). Clamped to
`[config.MinImageScale, config.MaxImageScale]` = `[0.2, 3.0]` via
`Config.GetImageScale()`.

Two ways to set it:

1. **Config file** — `"imageScale"` in `~/.cyber-tui.json` (e.g. `1.3`).
   Unset or `0` defaults to `1.0`. Applies from the next modal open.
2. **Live, while the modal is open** — `+`/`=` increases, `-` decreases, in
   steps of `imageScaleStep` (0.1). Each press re-runs
   `openImageInTerminal` for the currently displayed image
   (`App.imageModalURL`) and re-renders immediately — a cache hit
   (`App.imageCache`), so it only re-encodes, not re-fetches. This is
   session-only and does not write back to the config file; it resets to
   the config value on the next app start.

`displayCols`/`displayRows` are still clamped to the terminal's actual
width/height after scaling, so a large scale can't request a box bigger than
the screen.

## Non-fix

This does not change the underlying protocol behavior (iTerm2 still
letterboxes on a metrics mismatch, Kitty still stretches). It's a manual
correction knob, not a detection improvement — there is currently no way for
the app to query what a terminal actually rendered an image at, across any
of the three protocols.
