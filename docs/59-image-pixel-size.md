# 59 - Image pixel size

## Overview

API v0.9.0 documents `imagePixelSize` as one of `rough`, `medium`, `sharp`,
`crisp`. The TUI already had a local setting with the same four values
(`ditherSharpness` in `~/.cyber-tui.json`) controlling the dithering
pixelation block size. That local key is replaced by the server field, so the
value follows the account between the TUI and the website.

| Where | How |
|-------|-----|
| Settings, under "dithering" | "sharpness" row, cycles `rough` / `medium` / `sharp` / `crisp`, saved with `ctrl+s` |

The row is only shown when dithering is on and the image viewer is not
`browser`. Dithering on/off stays a local setting.

---

## Behaviour

- Read from `GET /v1/settings`, sent on `PATCH /v1/settings` with
  `omitempty`, so an account with no stored value never gets `""` written
  back.
- Empty or unrecognised values display and render as `medium`
  (`imgview.PixelSizeForSharpness` fails closed to `medium`).
- Block sizes in source pixels: `rough` 4, `medium` 3, `sharp` 2, `crisp` 1.
  Whether these match the website's sizes is unknown; the API documents the
  names only.
- The inline image cache key includes the pixel size, so a change re-encodes
  images without separate invalidation.

## Migration

None. Old config files keep the `ditherSharpness` key; the decoder ignores
it and it is dropped on the next config save. A previously chosen local
value does not carry over; the account's server value is used instead.

## Files

- `internal/api/client.go`: `wirePatchSettings.ImagePixelSize`
- `internal/ui/screens/settings.go`: sharpness row, `settingsEqual`
- `internal/ui/app.go`: `ditherOptions` reads `a.settings.ImagePixelSize`
