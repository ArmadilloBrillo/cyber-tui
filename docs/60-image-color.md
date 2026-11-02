# 60 - Image color

## Overview

API v0.9.0 added `imageColor` (default off): "renders images in colour
instead of the theme's foreground and background". On the website this is the
"Colour" choice (Monochrome / Colour) under Settings, Image Style. It does not
turn dithering off; it switches the same dithering shader into a colour mode.

| Where | How |
|-------|-----|
| Settings, under "dithering" | "color" row, cycles `monochrome` / `color`, saved with `ctrl+s` |

The row is shown under the same conditions as "sharpness": dithering on and an
image viewer other than `browser`. Dithering on/off stays a local setting.

---

## Behaviour

- Read from `GET /v1/settings` and sent on `PATCH /v1/settings`. The PATCH
  field has no `omitempty`, so switching back to monochrome sends an
  explicit `false`.
- Monochrome (`false`): unchanged. Pixelate, grayscale, Bayer + noise
  threshold, recolor through the theme's foreground and background.
- Color (`true`): same pixelation and threshold, but each RGB channel is
  ordered-dithered to 6 levels, `floor(c * 5 + threshold) / 5`, giving the
  216-colour web-safe palette. The theme colors are ignored.
- The inline image cache key includes the mode, so a change re-encodes images
  without separate invalidation.

## Source

Ported from the website's RasterImage WebGL shader (`u_colorMode` branch) in
the cyberspace.online web bundle, checked 2026-11-02. On the website a theme
can also force colour mode (its `myspace` theme does); cyber-tui has no such
theme, so that override is not ported.

## Files

- `internal/api/client.go`: `wireSettings.ImageColor`, `wirePatchSettings.ImageColor`
- `internal/ui/imgview/dither.go`: `DitherOptions.ColorMode`, `webSafe`
- `internal/ui/app.go`: `ditherOptions` reads `a.settings.ImageColor`; `ditherKeyFragment` includes the mode
- `internal/ui/screens/settings.go`: color row, `settingsEqual`
