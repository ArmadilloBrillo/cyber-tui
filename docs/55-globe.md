# 55 — Globe

## Overview

A rotating globe, rendered in Unicode quadrant block characters, plotting the
caller's own location plus a toggleable marker set for every guild they
belong to — their guild plus up to five apprenticeships. Reached via `g l` or
arrow-cycling (it has no numeric alias — it's the 12th tab).

The tab can be hidden entirely from Settings ("show globe tab", under the
"globe" group) — a local-only preference (`config.Config.HideGlobeTab`,
inverted as `App.showGlobeTab`), not synced to the API. When hidden, it's
excluded from `visibleTabs` (tab bar/nav sidebar + arrow-key cycling) and
`leaderRows`/the `g l` chord itself — same treatment Search's `hidden`
`navTab` entry already gets, just driven by a runtime setting instead of a
compile-time flag. See `docs/17-settings.md`.

There is no global user directory in the cyberspace.online API — only
per-username profile lookup and a guild-members list — so "everyone on
cyberspace" was never an option. Follow-network markers (following/followers)
were tried and removed: `GET /v1/follows` doesn't return usernames (see
Data flow), so the marker set could never populate. Guild members are the
only enumerable set the API actually supports resolving a location for.

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
| `space` | pause / resume rotation |

Rotation is automatic-only — there's no manual spin control. Every other
single-pane screen in this app already avoids claiming bare arrow keys/`h`/`l`
(they're swallowed by tab-cycling or list navigation depending on layout), so
Globe follows that same convention rather than carving out an exception.

Markers: `@` self, `#` guild member, each followed by the user's username as a
label — both are non-alphanumeric symbols so a label never reads as part of
the username itself (e.g. `oragnar` would be ambiguous; `#ragnar` isn't).
Self is drawn last so it always wins a cell (and any label overlap) shared
with a guild marker.

---

## Data flow

`model.User.LocationLatitude`/`LocationLongitude` already existed
(self-reported, optional, independent of the free-text `LocationName`) — no
geocoding needed. On entering the tab (`activateScreen`, `layout.go`):

1. `GlobeModel.SetSelf(a.currentUser)` — the self marker needs no fetch.
2. If not already loaded this session and the caller is in a guild
   (`a.currentUser.GuildSlug != ""`): `loadGlobeGuildMembersCmd` calls
   `GetUserGuilds(username)` for the caller's full guild list — their guild
   plus up to five apprenticeships, at most six, never paginated — then
   `GetGuildMembers(slug, "")` for each one. Members are merged across all
   of them into one deduplicated, self-excluded username list; one guild's
   member fetch failing doesn't block the others. A user with no badge guild
   can't have apprenticeships either (joining any guild before your first
   makes it your badge guild), so the `GuildSlug != ""` check alone is
   enough to skip the whole fetch for a guildless caller.
3. That username list is enqueued for a profile fetch (`GlobeModel.enqueue`),
   deduplicated against already-cached profiles and the queue itself.

**Why there's no follow-network marker set.** It existed briefly and was
removed: `GET /v1/follows` doesn't return `followerUsername`/`followedUsername`
— only IDs (server-side bug, `docs/00-api-backlog.md`, open since
2026-04-17, re-confirmed live 2026-09-14). Unlike the profile
Following/Followers tabs (which can fall back to showing a truncated user
ID), Globe had no usable fallback: plotting a marker requires a location,
which requires `GET /v1/users/:username`, and there's no by-ID equivalent.
Guild markers don't have this problem — `GET /v1/guilds/:slug/members`
does return `username`.

`GET /v1/users/:username` — the only way to learn a *listed* user's
location, since the guild-members list response doesn't include it — is
rate-limited to 30/min. A caller in several sizeable guilds can easily
exceed that, so `App` paces the queue at one fetch every 2.5 s
(`globeFetchInterval`/`globeFetchTickMsg`) rather than bursting requests.
A 429 requeues the username at the front of the queue and retries forever at
the same fixed interval (no backoff bookkeeping — ponytail: ok for a screen
nobody is forced to sit on). Any other failure is silently dropped. Markers
pop in progressively as profiles resolve; there is no loading spinner for
this — the landmask and self marker render immediately regardless.

The `GetUserGuilds` + per-guild `GetGuildMembers` burst in step 2 is a
separate, much smaller rate limit ("List guilds / members / a user's guilds",
30/min) and isn't paced — up to six calls once per tab entry is well within
budget, unlike the potentially much longer per-profile queue in step 3.

The fetch chain and the rotation-tick chain (`globeAngleTickMsg`, 150 ms)
both stop rescheduling themselves as soon as `a.active != screenGlobe`, so
leaving the tab doesn't burn rate-limit budget or CPU on a screen nobody is
looking at; both resume automatically on return.

---

## Rendering

`GlobeModel.View()` (`internal/ui/screens/globe.go`) renders in Unicode
quadrant block characters (`▘▝▀▖▌▞▛▗▚▐▜▄▙▟█`, U+2580–259F range), not one flat
character per pane cell: each terminal cell is subdivided into an 8-position
sub-pixel sampling grid (2-wide × 4-tall, `globeSubCols`/`globeSubRows`,
`brailleDotBit` numbering), collapsed down to a 2×2 quadrant pattern per cell
for the actual glyph (`quadrantOnBits`/`quadrantGlyphs`). Braille's 2×4
subdivision of a character cell (itself roughly 1-wide × 2-tall in most
monospace fonts) happens to produce sub-pixels that are approximately square,
so the aspect-correction constant (`globeSubCellAspect`) is ~1.0 here rather
than the ~0.5 a whole-character grid would need — that geometry is the only
thing "braille" about this anymore; no braille glyph is ever rendered.

For each of a cell's 8 sub-positions, `sphereProject` inverts a normalized
offset — through the current rotation angle — back to a lat/lon on the
unrotated globe; positions outside the unit circle contribute nothing.
`landAt` samples the 720×360 `globeLandmask` bitmap for land vs. ocean at each
"on" sub-position. `classifyGlobeCell(okBits, landBits)` then picks the glyph
and color:
- **Rim cells** (the disc clips some sub-pixels — the outer silhouette): the
  quadrant shape comes from which sub-positions are inside the disc, colored
  with a single flat land/ocean-majority color (a terminal can only color a
  whole character cell one color, so a cell straddling a coastline here picks
  whichever had more samples).
- **Cells fully inside the disc** (interior terrain, land or ocean or a
  genuine coastline mix): the quadrant shape comes from the land/ocean split
  itself, rendered two-tone — land as the glyph's foreground, ocean as its
  background. This degrades correctly at the extremes: solid ocean is a space
  with the ocean background showing through, solid land is a full block
  (`█`), and a real coastline is a genuine split glyph — a proportional fill
  a dot-based glyph couldn't represent, since braille dots don't blend into a
  color the way a block character's solid fill does.

Markers (`@`/`#`, each followed by the user's username as a text label)
are plotted using the inverse function, `markerScreenPos`, at the same
sub-pixel resolution as terrain (so a marker aligns with the coastline under
it) before collapsing to the character cell that sub-pixel belongs to — where
the glyph and label fully override whatever terrain would otherwise render
there, since a marker needs to read as an unambiguous symbol, not blend into
the terrain. `ok = false` means the point is on the far side of the globe
right now — a guild member on the other side of the world simply isn't
visible until the globe rotates around to them, exactly like a real globe.

`sphereProject`/`markerScreenPos`/`landAt` are pure functions operating in
normalized `[-1,1]` space, unaware of sub-pixel vs. whole-cell sampling, and
are unit-tested directly, including a round-trip check between the first two
(`globe_test.go`). `brailleDotBit`'s dot-numbering → bit mapping and
`quadrantOnBits`/`quadrantGlyphs`/`classifyGlobeCell`'s glyph selection are
each tested separately (`TestBrailleDotBit`, `TestQuadrantBlockBit`,
`TestQuadrantGlyph`, `TestClassifyGlobeCell`).

---

## Ponytail simplifications

- Guild-members lists use the API's default first page only per guild — no
  exhaustive auto-pagination. A guild larger than one page will show a
  subset of members until this is revisited.
- 429s on the per-profile fetch retry forever at a fixed interval; no
  exponential backoff.
- No `View()` golden-output tests — cosmetic rendering, not logic.
- Username labels have no collision avoidance: markers close together on
  screen just overwrite each other's labels in draw order (guild, then self
  last) — acceptable for a screen with at most a handful of markers visible
  at once, revisit if that stops being true.

---

## Tests

| File | Tests |
|------|-------|
| `internal/ui/screens/globe_test.go` | `TestBrailleDotBit`, `TestQuadrantBlockBit`, `TestQuadrantGlyph`, `TestClassifyGlobeCell`, `TestViewOrientationNorthAtTop`, `TestViewMarkerLabelsUsername`, `TestSphereProjectCenter`, `TestSphereProjectOutsideDisc`, `TestSphereProjectMarkerRoundTrip`, `TestMarkerScreenPosFarSide`, `TestLandAt` (synthetic fixture bitmap, incl. longitude wraparound), `TestGlobeZoomClamps`, `TestGlobeToggleKeys`, `TestGlobeSetProfileOnlyKeepsLocated`, `TestGlobeEnqueueDedup`, `TestGlobeNextPendingAndRequeue`, `TestGlobeAdvancePausable` |
| `internal/ui/app_test.go` | `TestNavigateTab_*` updated for the 12th tab (Globe wraps left from Feed, sits last in the cycle); `TestLoadGlobeGuildMembersCmd_IncludesApprenticeshipGuilds` (badge guild + apprenticeship merging, cross-guild dedup, self excluded); `TestHandleKeys_Leader_GlobeHidden_ChordDoesNothing` |
| `internal/ui/layout_test.go` | `TestVisibleTabs_ExcludesGlobeWhenHidden` |
| `internal/ui/screens/settings_test.go` | `TestSettings_ShowGlobeTabToggle`, `TestSettings_ShowGlobeTabDirty`, `TestSettings_ShowGlobeTabSaveMsg` |
