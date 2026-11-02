# 58 - cIRC color nicknames and /localtime

## Overview

Two cIRC additions from API v0.9.0.

| Feature | Where | How |
|-------|-----|-----|
| Color nicknames | Settings, display group | "color nicknames" toggle, saved to the server as `colorNicknames` |
| Local time | CIRC (inside a room) | `/localtime` or `/localtime HH:MM` |

---

## Color nicknames

`colorNicknames` is a server-side setting (default off), so it follows the
account between the TUI and the website. It is read by `GET /v1/settings` and
sent on every `PATCH /v1/settings`.

When on, other users' names in cIRC message lines are drawn with
`theme.NickStyle`:

- Hue matches the website: FNV-1a 32 of the lower-cased username, mod 360.
- Saturation is fixed at 0.7.
- Lightness depends on the theme's text colour. The TUI never paints a
  background, so the terminal background is unknown; light text implies a
  dark terminal and gets lightness 0.65, dark text gets 0.35.

Applies to normal lines, `/me` action lines and ASCII art headers. Your own
name keeps `MeHighlight`, deleted tombstones stay `Subtle`. C-Mail and the
online-users panel are unchanged, since the API scopes the setting to cIRC
message lines.

The per-message render cache (`chatBodyCacheEntry`) records the setting, so
toggling it re-renders cached lines.

---

## /localtime

The server posts "It's HH:MM o'clock where @you is". It can't see the user's
clock, so the time must be in the command.

- `/localtime HH:MM` is sent unchanged; the server validates the time.
- A bare `/localtime` is filled in client-side with the current time in the
  configured timezone (Settings > timezone), formatted 24h.
- cIRC only. C-Mail rejects it locally as an unknown command, matching the API.
