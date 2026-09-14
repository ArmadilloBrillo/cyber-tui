# 55 — Globe

## Overview

A rotating braille-dot globe plotting the caller's own location plus two marker
sets they can toggle: their guild's members, and their combined follow
network (following + followers, one set — not two). Reached via `g l` or
arrow-cycling (it has no numeric alias — it's the 12th tab).

There is no global user directory in the cyberspace.online API — only
per-username profile lookup, a follows list, and a guild-members list — so
"everyone on cyberspace" was never an option. The three sets above are the
only ones the API lets a client enumerate.

Rendering technique adapted from
[gh-firehose](https://github.com/leereilly/gh-firehose): an equirectangular
land/ocean bitmap, sampled via inverse sphere projection per terminal cell,
rotated a little each tick. Pure stdlib `math` — no new dependency. The
bitmap itself (`globe_landmask.go`) was generated fresh from
[Natural Earth](https://www.naturalearthdata.com)'s public-domain 1:50m
land dataset (720×360, 0.5°/cell — sharp enough for the braille sub-pixel
sampling below to actually resolve coastline shape, not just the disc's
outer edge) rather than reusing gh-firehose's own copy, whose provenance
(credited to a third project, no stated license) was unclear.

---

## Controls

| Key | Action |
|---|---|
| `+` / `=` | zoom in |
| `-` / `_` | zoom out |
| `m` | toggle guild-member markers |
| `f` | toggle follow-network markers |
| `space` | pause / resume rotation |

Rotation is automatic-only — there's no manual spin control. Every other
single-pane screen in this app already avoids claiming bare arrow keys/`h`/`l`
(they're swallowed by tab-cycling or list navigation depending on layout), so
Globe follows that same convention rather than carving out an exception.

Markers: `@` self, `o` guild member, `*` follow network. Self is drawn last
so it always wins a cell shared with another marker.

---

## Data flow

`model.User.LocationLatitude`/`LocationLongitude` already existed
(self-reported, optional, independent of the free-text `LocationName`) — no
geocoding needed. On entering the tab (`activateScreen`, `layout.go`):

1. `GlobeModel.SetSelf(a.currentUser)` — the self marker needs no fetch.
2. If not already loaded this session: `GetGuildMembers(a.currentUser.GuildSlug, "")`
   (skipped if the caller isn't in a guild) and
   `GetFollowing("")` + `GetFollowers("")` (deduplicated into one list,
   excluding the caller) each resolve to a username list.
3. Each username list is enqueued for a profile fetch
   (`GlobeModel.enqueue`), deduplicated against already-cached profiles and
   the queue itself.

`GET /v1/users/:username` — the only way to learn a *listed* user's
location, since the follows/guild-members list responses don't include it —
is rate-limited to 30/min. A guild or follow list can easily exceed that, so
`App` paces the queue at one fetch every 2.5 s
(`globeFetchInterval`/`globeFetchTickMsg`) rather than bursting requests.
A 429 requeues the username at the front of the queue and retries forever at
the same fixed interval (no backoff bookkeeping — ponytail: ok for a screen
nobody is forced to sit on). Any other failure is silently dropped. Markers
pop in progressively as profiles resolve; there is no loading spinner for
this — the landmask and self marker render immediately regardless.

The fetch chain and the rotation-tick chain (`globeAngleTickMsg`, 150 ms)
both stop rescheduling themselves as soon as `a.active != screenGlobe`, so
leaving the tab doesn't burn rate-limit budget or CPU on a screen nobody is
looking at; both resume automatically on return.

---

## Rendering

`GlobeModel.View()` (`internal/ui/screens/globe.go`) renders in Unicode
braille dots (U+2800 base), not one character per pane cell: each terminal
cell is subdivided into a 2-wide × 4-tall sub-pixel grid
(`globeSubCols`/`globeSubRows`), giving 8x the addressable resolution of a
plain `#`/`.` block grid in the same pane — a visibly rounder circle edge and
finer coastline shape. Braille's 2×4 subdivision of a character cell (which
is itself roughly 1-wide × 2-tall in most monospace fonts) happens to produce
sub-pixels that are approximately square, so the aspect-correction constant
(`globeSubCellAspect`) is ~1.0 here rather than the ~0.5 a whole-character
grid would need.

For each of a cell's 8 sub-positions, `sphereProject` inverts a normalized
offset — through the current rotation angle — back to a lat/lon on the
unrotated globe; positions outside the unit circle contribute nothing.
`landAt` samples the 720×360 `globeLandmask` bitmap for land vs. ocean at each
"on" sub-position, and the cell's braille rune is `brailleDotBit`-packed from
every on sub-position. A terminal can only color a whole character cell one
color, so the cell's color is decided by majority vote between its land and
ocean sub-position counts — a cell straddling a coastline picks whichever
had more samples.

Markers (`@`/`o`/`*`) are plotted using the inverse function,
`markerScreenPos`, at the same sub-pixel resolution as terrain (so a marker
aligns with the coastline under it) before collapsing to the character cell
that sub-pixel belongs to — where they fully override whatever braille
pattern would otherwise render there, since a marker needs to read as an
unambiguous symbol, not dots. `ok = false` means the point is on the far side
of the globe right now — a guild member on the other side of the world simply
isn't visible until the globe rotates around to them, exactly like a real
globe.

`sphereProject`/`markerScreenPos`/`landAt` are pure functions operating in
normalized `[-1,1]` space, unaware of sub-pixel vs. whole-cell sampling, and
are unit-tested directly, including a round-trip check between the first two
(`globe_test.go`). `brailleDotBit`'s dot-numbering → bit mapping is tested
separately (`TestBrailleDotBit`).

---

## Ponytail simplifications

- Following + followers combined into one toggle, not two.
- Guild-members/follows lists use the API's default first page only — no
  exhaustive auto-pagination. A guild or follow network larger than one page
  will show a subset of members until this is revisited.
- 429s on the per-profile fetch retry forever at a fixed interval; no
  exponential backoff.
- No `View()` golden-output tests — cosmetic rendering, not logic.

---

## Tests

| File | Tests |
|------|-------|
| `internal/ui/screens/globe_test.go` | `TestBrailleDotBit`, `TestViewOrientationNorthAtTop`, `TestSphereProjectCenter`, `TestSphereProjectOutsideDisc`, `TestSphereProjectMarkerRoundTrip`, `TestMarkerScreenPosFarSide`, `TestLandAt` (synthetic fixture bitmap, incl. longitude wraparound), `TestGlobeZoomClamps`, `TestGlobeToggleKeys`, `TestGlobeSetProfileOnlyKeepsLocated`, `TestGlobeEnqueueDedup`, `TestGlobeNextPendingAndRequeue`, `TestGlobeAdvancePausable` |
| `internal/ui/app_test.go` | `TestNavigateTab_*` updated for the 12th tab (Globe wraps left from Feed, sits last in the cycle) |
