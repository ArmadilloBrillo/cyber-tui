# Plan: Inline-Images Improvements

## Context

`feature/inline-images` shipped experimental Kitty/Sixel/iTerm2 inline image rendering
for Feed and PostDetail. Two independent solution-architect reviews
(`docs/reviews/2026-08-09-inline-images-architecture-review.md`,
`docs/reviews/2026-08-09-inline-images-greenfield-critique.md`) audited the branch and
converged on the same core findings; a third review
(`docs/reviews/2026-08-09-inline-images-chafa-review.md`) evaluated and rejected
replacing the hand-rolled encoders with `chafa`. This plan consolidates all three into
a single prioritized punch list for taking the feature from "experimental" to shippable.

Items are ordered by real-bug-first, smallest-diff-that-fixes-it — deferred items are
deferred because both reviews independently judged them low-urgency, not because they
were missed.

---

## 1. ~~Decide MillerLayout's fate~~ — Resolved: finished, not deleted

**Files:** `internal/ui/app.go` (`layoutFromName`), `internal/ui/layout_miller.go`,
`internal/ui/screens/settings.go`.

Decided in follow-up discussion: the two-code-sets concern behind option (a) checked out
as real but narrow (the overlay-compositing block specifically, not navigation/routing,
which was already centralized) — so it was cheaper to fix the actual duplication than to
delete the layout. `layoutFromName` now branches on its argument, Settings has a "layout"
picker item (`tabs`/`miller`, cycles live via the existing save path), and Feed gained
its own inline-image support for Miller's compact detail pane (post + reply images),
which it never had before. See `docs/reviews/2026-08-09-inline-images-greenfield-critique.md`
for the original analysis and commit `ebb4a64` for the implementation.

---

## 2. ~~Fix (or remove) the inline-image compositing gap~~ — Resolved: hoisted

**Files:** `internal/ui/layout.go` (`compositeOverlays`, `modalRenderer`), replacing the
duplicated tail of `internal/ui/layout_tabs.go`'s and `internal/ui/layout_miller.go`'s
`View()` methods.

Implemented the shared-pipeline fix both reviews recommended: `compositeOverlays` is now
the single place that composites the five simple modals, the fullscreen image modal, the
Kitty placement-cleanup delete, and inline-image injection, called as the last line of
both layouts' `View()`. Neither layout can independently forget a step or return early
partway through anymore — the exact shape of the original MillerLayout bug. Golden-output
tests (`internal/ui/layout_test.go`) assert on the composited escape sequences for both
layouts, closing the test-coverage gap both reviews flagged. See commit `ebb4a64`.

A related bug surfaced and was fixed separately while verifying this work: Miller's
`HasFocusedInput` for Circ/CMail didn't account for Miller's own focus state, trapping
nav keys inside a backgrounded room — commit `6371452`.

---

## 3. ~~Bound `inlineImageCache`~~ — Resolved

`inlineImageCache` is now bounded by `inlineImageCacheMaxBytes` (16 MiB, a starting cap
rather than a measured one), evicting the oldest-inserted entry first via
`cacheInlineImage` (`container/list` + map, as scoped). `kittyPlacementIDs` was left
untouched and permanent, as designed — eviction only removes the cache entry, and since
`inlineImageCacheKey` doesn't embed the placement id, a slot whose entry gets evicted
just re-fetches and re-encodes using its already-stable id on the next sync. No coupling
to handle, since ids themselves are never evicted.

---

## 4. ~~Compress the Kitty payload~~ — Resolved

`EncodeKitty` now emits PNG (`f=100`, no `s=`/`v=` needed — the terminal reads pixel
dimensions from the PNG header) instead of raw 32-bit RGBA (`f=32`), matching
`EncodeITerm2`'s existing pattern exactly, including propagating a `png.Encode` error
through a new `error` return (both real call sites and `encode_test.go` updated to
match).

---

## 5. ~~Stop silently caching fetch failures as retryable blanks~~ — Resolved

`handleInlineImageFetched` now records a failure timestamp (`inlineImageFailedAt`) on
error; `syncInlineImages` skips refetching a key within `inlineImageFailureCooldown`
(60s) of its last failure, and a subsequent success clears the record so a transient
blip doesn't leave a stale cooldown once the URL recovers.

---

## 6. Deferred — no action needed now

Both reviews independently judged these low-urgency or fine-as-is; revisit only if
circumstances change (noted trigger in parentheses):

- Uncancelled inline-fetch goroutines (`app.go:2572-2600`) — bounded by a 20s timeout
  today; extend the fullscreen modal's existing `imageFetchGen` staleness-guard pattern
  to the inline path if this gets reused for more expensive fetches later.
- ~~Kitty/iTerm2 not threading real terminal cell size~~ — Resolved in `d9aa58f`:
  `EncodeITerm2`/`EncodeKitty` now accept `cellPxW, cellPxH` and fall back to the
  assumed default only when the real size is unavailable, mirroring `EncodeSixel`'s
  existing shape. Fixed as part of first-time real iTerm2 testing (see section 8).
- `syncInlineImages` running unconditionally on every `Update` (`app.go:488-495`) — cheap
  no-op today; gate on message type if a future profiling pass shows `Update` latency
  pressure, or before this exact pattern gets reused elsewhere.
- Sixel hardcoding 256 colors (`imgview/sixel.go:44`) — true for essentially every
  Sixel-capable terminal in practice; leave as a documented assumption.

---

## 7. Chafa: no action

Evaluated in `docs/reviews/2026-08-09-inline-images-chafa-review.md` and rejected as a
replacement for `internal/ui/imgview` — every integration path (external binary via
`exec.Command`, cgo bindings, or the one community purego-based Go binding) gives up
something the current pure-Go implementation has for free: zero runtime dependencies,
full 5-target cross-compile coverage, or both. The one genuine capability gap chafa
would close — a rendering fallback for terminals with no detected graphics protocol —
is real but small enough to build natively if it's ever prioritized as its own feature,
without reopening this tradeoff.

---

## 8. First real iTerm2 testing: bugs found, and the workarounds their fixes depend on

The feature had never been manually tested against real iTerm2 until this round —
CI/unit tests can't catch terminal-rendering bugs, only Kitty behavior had been
eyeballed. Found and fixed, across `d9aa58f`, `33ea53b`, `ae42929`, `f788a79`, `5dd18aa`:

- Inline images disappearing on a selection-only move (no scroll) and staying gone.
- A regression where the fix above cleared the whole screen on *every* selection move.
- The fullscreen image modal (`o` key) leaving blank space below the image on iTerm2 —
  fixed alongside the cell-size threading noted in section 6.
- A flash on every selection change that touched a visible image (the clear-based fix
  above was correct but visually jarring).
- Stale partial image pixels left behind after a small scroll, when the image's new
  position overlapped its old one.

Two of the final fixes are genuine workarounds, not clean solutions — worth revisiting
if the underlying constraint ever changes:

- **`inlineImagePaintGen`** (`app.go`) forces Bubble Tea's per-line diff to reissue a
  line whose *meaningful* content didn't change, via an inert toggling SGR-reset marker.
  Needed because Bubble Tea has no public API to mark a single line dirty — it has an
  internal `repaintMsg` that does exactly this, but it's unexported (checked against
  v1.3.10, the latest release as of this writing). **Revisit if** a future Bubble Tea
  version exposes one; drop the marker trick for a direct call.
- **`syncInlineImageErasures` / `pendingInlineImageErasures`** (`app.go`) explicitly
  track each image's absolute on-screen rectangle and blank-fill stale ones when an
  image moves or disappears, mirroring the `pendingKittyDeletes` accumulate-until-revived
  pattern. Needed because iTerm2/Sixel have no placement concept — a raster paint is just
  overwritten text-grid cells with no compositing layer immune to unrelated writes,
  unlike Kitty. **Revisit if** iTerm2 or mainstream Sixel implementations ever adopt a
  Kitty-style placement/delete model, which would make the manual rectangle-diffing
  unnecessary for those protocols too.

One residual limitation was evaluated and accepted rather than fixed: a selection change
that touches a visible image still shows a brief single-line flash (Lip Gloss regenerates
a card's whole bordered box as one string on a border-color change, incidentally erasing
the image band before `inlineImagePaintGen` reissues it — two visible terminal paints,
not one atomic replace). Two fixes were considered and rejected:

- Hand-rolled DECSET 2026 (terminal synchronized-output) begin/end markers — the actual
  standard fix for this class of tearing, but Bubble Tea's renderer diffs and skips lines
  independently, so there's no way to guarantee a begin marker and its matching end marker
  land in the same flush; a mismatch would leave the terminal stuck buffering indefinitely
  (a frozen screen) — worse than the flash. **Revisit if** Bubble Tea ships native DECSET
  2026 support (not present in v1.3.10).
- Keeping the image band's border color constant regardless of selection, so those rows
  never need rewriting — the real root-cause fix, but requires replacing Lip Gloss's
  single-call box styling with manual per-line border construction in
  `feed.go`/`postdetail.go`. Judged too large/risky for the cosmetic gain; revisit if this
  flash becomes a bigger complaint or the border-rendering code gets touched for other
  reasons anyway.
