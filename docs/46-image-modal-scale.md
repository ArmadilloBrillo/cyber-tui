# Image Modal Scale

## Problem

The fullscreen image modal (`o`, `internal/ui/app.go`'s `openImageInTerminal`)
computes a target cell box and hands it to whichever encoder matches the
detected graphics protocol (`internal/ui/imgview`: `EncodeKitty`,
`EncodeITerm2`, `EncodeSixel`). How a given cell box maps to actual
on-screen pixels is a terminal-side decision the app cannot see or query:

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

## First attempt, and why it didn't work

The first version of this feature expressed `Config.ImageScale` as a
fraction of "4/5 of the terminal window" and applied it to
`displayCols`/`displayRows` before they reached the encoder. Live-tested on
mintty/Sixel, iTerm2, and Ghostty/Kitty, `+`/`-` appeared to do nothing.
`CYBERSPACE_DEBUG_KEYS=1` (`cmd/cyber-tui/main.go`) confirmed the keys were
detected correctly and the requested box genuinely grew/shrank each press —
but the *encoded* box stayed frozen:

```
image scale=1.00 displayBox=188x46 cellPx=16x36 nativePx=773x512 -> box=49x14
image scale=1.50 displayBox=236x58 cellPx=16x36 nativePx=773x512 -> box=49x14
```

Root cause: `773px/16 ≈ 49 cols`, `512px/36 ≈ 14 rows` — the image's native
pixel size in cells was already smaller than 4/5 of the terminal, even at
`scale=1.0`. `fitBox`/`downscaleToBox` never upscaled past native
resolution, so every `+` was silently absorbed by that cap, and `-` needed
many presses before crossing below the native-size threshold. Not a bug —
just `scale` being expressed relative to a box the image was nowhere near
filling in the first place.

## Fix

`Config.ImageScale` (`internal/config/session.go`) is now relative to the
image's own native (1:1 pixel) size in cells, not the terminal window —
`imgview.NativeCellBox(imgWidth, imgHeight, cellPxW, cellPxH)` computes that
native cell box, ceiling-dividing each axis independently. `1.0` = native
size (clamped to fit the terminal); this also allows upscaling past native
resolution — accepting some blur — up to `2.0` (`config.MaxImageScale`),
since a user pressing `+` is explicitly asking to zoom in, unlike an inline
thumbnail that should never blow up a small image. `EncodeKitty`,
`EncodeITerm2`, `EncodeSixel`, `fitBox`, and `downscaleToBox` all take an
`allowUpscale bool` — `true` only for the fullscreen modal; inline
thumbnails still pass `false` and keep the old never-upscale behavior.

Two ways to set the scale:

1. **Config file** — `"imageScale"` in `~/.cyber-tui.json` (e.g. `1.3`).
   Unset or `0` defaults to `1.0`. Applies from the next modal open.
2. **Live, while the modal is open** — `+`/`=` increases, `-` decreases.
   `App.adjustImageScale` computes the step in terminal cells against the
   image's own native size (`imageScaleStep`, 10%) and floors it at 1 cell
   (`max(1, round(nativeCols*0.1))`) — so every press changes the rendered
   size by at least one cell until the true min (`config.MinImageScale`,
   0.2x) or max (2x) bound, rather than a fixed float step getting rounded
   away to nothing for images whose native size is small relative to a flat
   10% step. Each press re-runs `openImageInTerminal` for the currently
   displayed image (`App.imageModalURL`) and re-renders immediately — a
   cache hit (`App.imageCache`), so it only re-encodes, not re-fetches. This
   is session-only and does not write back to the config file; it resets to
   the config value on the next app start.

The resulting box is still clamped to the terminal's actual width/height
after scaling, so a large scale can't request a box bigger than the screen.

## Miller layout: clamping to the terminal wasn't enough

Every modal (theme picker, help, URL picker, and the image modal) is
centered against the *full* terminal width by `compositeOverlays`/
`overlayCenter` (`internal/ui/layout.go`), which splices the modal directly
into the already-rendered frame — sidebar included for Miller layout
(`internal/ui/layout_miller.go`, a 22-column nav pane on the left,
`millerSidebarWidth`). Once upscaling let the image box approach the full
terminal width, its centered left edge could dip into that sidebar and
overwrite its text — reported live as the modal "pushing the sidebar all
the way left."

Clamping to raw terminal width wasn't sufficient: centering splits the
unused space evenly on both sides, so avoiding a left-side obstruction of
width `r` requires reserving `2*r` off the total (half the reservation lands
as unused margin on the right, which is fine). `Layout.ModalMaxWidth(termWidth
int) int` (`internal/ui/layout.go`) captures this per layout —
`TabsLayout` returns `termWidth` unchanged (no side chrome, the tab bar is a
top row); `MillerLayout` returns `termWidth - 2*millerSidebarWidth`, floored
at 1. `openImageInTerminal` clamps `displayCols` against
`a.layout.ModalMaxWidth(a.width)` instead of raw `a.width`.

Scoped to the image modal rather than reworking `compositeOverlays` itself —
the other modals have fixed, narrow content and have never been wide enough
to trigger this.

## Non-fix

This does not change the underlying protocol behavior (iTerm2 still
letterboxes on a metrics mismatch, Kitty still stretches). It's a manual
correction knob, not a detection improvement — there is currently no way for
the app to query what a terminal actually rendered an image at, across any
of the three protocols.
