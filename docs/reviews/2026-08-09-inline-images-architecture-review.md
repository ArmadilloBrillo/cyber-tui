# Inline-Images Architecture Review — 2026-08-09

**Reviewed branch:** `feature/inline-images` (5 commits over `main`, merge-base `2adf4c9`) · **Perspective:** solution architect, first look, design/quality focus (not a security review — see `docs/reviews/2026-07-24-code-security-review.md` for that lens).

Scope: 33 files, +1901/-182 lines. Kitty, Sixel, and iTerm2/WezTerm inline image rendering for post/reply bodies in Feed and PostDetail, plus a preexisting fullscreen image-viewer modal that now shares the same encoders.

---

## Executive Summary

| Severity | Count |
|---|---|
| High | 1 |
| Medium | 3 |
| Low | 3 |
| Informational | 3 |

The core design choice — encode everything as Bubble Tea messages/commands and keep all mutable state inside the single-threaded `Update` loop — is the right one, and it's applied consistently: there are no mutexes anywhere in this feature and none are needed, because no goroutine ever touches shared state directly. That part is not a finding, it's a correctly-made architectural decision.

The standout problem is functional, not stylistic: **the feature never renders in `MillerLayout`, only in `TabsLayout`.** `injectInlineImages` is called from exactly one of the two layout files. This is a direct consequence of the second-biggest issue — the fullscreen-modal and inline-image compositing logic is hand-duplicated across `layout_tabs.go` and `layout_miller.go` rather than shared, so a change applied to one silently doesn't apply to the other. Everything else here is smaller: an intentionally-accepted unbounded cache, a sync routine that runs on every message rather than just the relevant ones, un-cancelled fetch goroutines, and a couple of hardcoded protocol assumptions. Two of the five commits on this branch are dedicated post-merge bug fixes for lifecycle issues that were only found on real hardware (Ghostty) — both read as genuine root-cause fixes, but they're a signal about the test suite's blind spot (see the last Informational finding).

---

## Architecture Overview

Everything routes through the standard Elm-architecture loop: `App.Update` (`internal/ui/app.go:488`) delegates to `updateInner`, then unconditionally calls `syncInlineImages` (`app.go:490`) before returning. `syncInlineImages` (`app.go:2514`) is a pure diff — compares the active screen's `VisibleInlineImages()` against `inlineImageCache`/`inlineImageFetching`, and for Kitty additionally diffs a stable per-slot placement-id set to compute create/delete transitions. Missing slots become `fetchInlineImageCmd` (`app.go:2572`), a `tea.Cmd` that Bubble Tea runs as a goroutine, timed out at 20s, returning an `inlineImageFetchedMsg` that's folded back into `App` state inside `Update`. No goroutine ever writes to `App` fields directly — everything comes back as a message. This is why, despite three separate caches/maps and several goroutines in flight at once, there's no data race anywhere in this feature.

View-time compositing is layout-specific rather than shared: `TabsLayout.View` (`layout_tabs.go:112`) calls `injectInlineImages` (`layout_tabs.go:132`) to splice absolute-cursor-positioned escape sequences into the rendered frame. `MillerLayout` has its own, separately-written modal-compositing and cleanup-flag logic (`layout_miller.go:147-166`) but no equivalent inline-image call at all.

The two post-merge fix commits are useful evidence of where this design is fragile in practice: `70925fd` fixed four Kitty lifecycle bugs found on Ghostty (blunt delete-all colliding with the fullscreen modal, single-shot deletes lost to Bubble Tea's throttled renderer, a sticky cleanup flag that permanently disabled inline rendering after the modal closed once, and stale cache entries pointing at already-deleted placement ids); `3ae054c` fixed a stale-placement bug on toggle-off and a goroutine leak in the startup Sixel probe. Both are described accurately in their commit messages (verified against `git show`), and both fixes are structural (stable ids, accumulating delete sets, cancelable reads) rather than papered over — but the fact that four of them surfaced only after merge, on specific real hardware, says something about pre-merge verification coverage (see Informational finding below).

---

## Findings

### HIGH — Inline images never render in `MillerLayout`

**Files:** `internal/ui/layout_tabs.go:112-149` (`injectInlineImages`, only call site) vs. `internal/ui/layout_miller.go:147-166` (parallel modal logic, no inline-image call anywhere)

**The problem:** `grep -rn "injectInlineImages\|activeInlineImageSlots" internal/ui/*.go` shows `activeInlineImageSlots` is read only in `app.go` and `layout_tabs.go`; `injectInlineImages` is defined and called only in `layout_tabs.go`. `MillerLayout.View` has its own independently-written block for the fullscreen-modal composite and its Kitty-cleanup-flag injection, but never splices inline images into the frame at all.

**Failure scenario:** A user who runs the app in Miller (three-pane) layout enables inline images in settings, opens Feed or PostDetail, and sees nothing different — no error, no fallback text, just the same plain-text body they'd see with the setting off. The setting silently does nothing for roughly half of the app's layout surface. No test catches this: `feed_test.go`/`postdetail_test.go`/`render_test.go` test `VisibleInlineImages()` and the pure splice functions, never `MillerLayout.View()`'s actual output.

**Recommendation:** Move `injectInlineImages` (and ideally the near-duplicated modal-compositing/cleanup-flag blocks) out of `TabsLayout` into a shared helper both layouts call, rather than adding a second copy to `MillerLayout`. The duplication is the root cause, not just this one missed call site — fixing only the missing call leaves the same class of drift ready to happen again the next time either layout changes. Add a `MillerLayout` regression test asserting the composited `View()` output contains the expected escape sequence for a visible inline slot, mirroring what's missing in the test list below.

---

### MEDIUM — `inlineImageCache` grows unboundedly for the life of the session

**File:** `internal/ui/app.go:184-196` (field + doc comment), `app.go:2421-2423` (cache key includes column width)

**The problem:** `inlineImageCache` is keyed by slot key × URL × column budget × protocol and is explicitly never evicted — the field comment at `app.go:186-187` names this outright as an accepted spike-scope cut. Every distinct image URL seen, times every distinct terminal width the session was ever resized to, times (for Kitty) every distinct slot position it was ever placed at, stays cached forever. For Kitty specifically, the cached value is base64'd *raw, uncompressed RGBA* pixel data (`kitty.go:28-63`, no PNG/zlib compression), which is the most memory-hungry payload shape available for this data — so this is the single largest unbounded allocation surface in the feature.

**Failure scenario:** A long-running session that scrolls through a lot of image-bearing posts, or gets resized repeatedly (e.g. a terminal window dragged to resize, or a tmux pane reflow), accumulates cache entries indefinitely with no cap and no LRU. This was a reasonable, explicitly-labeled cut for an experimental spike; it stops being reasonable once this feature ships as non-experimental.

**Recommendation:** Bound the cache (LRU by entry count or byte size) before removing the "experimental" label. Note the permanent `kittyPlacementIDs`/`kittyNextPlacementID` maps (`app.go:198-212`) are coupled to this decision by design (a re-issued id on eviction would need to also invalidate the corresponding cache entry) — any eviction scheme has to account for that coupling, not bound the two caches independently.

---

### MEDIUM — `syncInlineImages` runs on every single `Update`, not just image-relevant messages

**File:** `internal/ui/app.go:488-495` (call site), `app.go:2514-2557` (`syncInlineImages` body)

**The problem:** `Update` unconditionally calls `syncInlineImages` after every message — keypresses, ticks, resizes, unrelated screen navigation — not gated on any check for whether the message could plausibly have changed the visible image set. The function itself is cheap (`VisibleInlineImages()` plus a couple of map diffs), so this isn't a measured performance problem today. It's flagged here as a structural habit: "recompute a screen-derived diff on every message rather than only on messages that could change it" is a pattern that scales badly if copied elsewhere in the app, and it makes reasoning about *when* image state changes harder than it needs to be, since the answer is "always, but usually a no-op" rather than "on these specific message types."

**Recommendation:** Not urgent to change alone. Worth revisiting if a future profiling pass shows `Update` latency issues, or before this pattern gets reused for another per-frame derived-state sync.

---

### MEDIUM — Fetch goroutines aren't cancelled on view-change, toggle-off, or quit

**File:** `internal/ui/app.go:2572-2600` (`fetchInlineImageCmd`), `app.go:2612` (only place `inlineImageFetching` is cleared)

**The problem:** Each in-flight fetch is bounded by a 20s `context.WithTimeout`, so this isn't a leak, but there's no cancellation path: navigating away from the screen that requested the fetch, toggling inline images off mid-load, or quitting the app while fetches are in flight all let the goroutine run to completion regardless. `inlineImageFetching[key]` is only cleared when the result message arrives, so nothing double-fires — but nothing stops the wasted network/decode work either, and on quit the result is simply discarded into a closed message channel.

**Recommendation:** Low priority given the 20s cap bounds the damage, but worth a `context.Cancel` tied to view-change/quit if this pattern is extended to more expensive fetches later (e.g. larger images, slower endpoints).

---

### LOW — Fetch failures are silent, with no retry, backoff, or user-visible state

**File:** `internal/ui/app.go:2607-2614` (`handleInlineImageFetched`)

**The problem:** A failed fetch (`err != nil`) is swallowed into a blank cache entry with no retry and no error surfaced to the user. The only way a failed slot gets another attempt is incidentally — a resize changes the cache key's column-width component, which happens to bypass the "already attempted" check.

**Recommendation:** At minimum, avoid caching a hard failure as if it were a successful blank result, so a manual re-trigger (e.g. re-entering the screen) can retry without needing a resize as a workaround.

---

### LOW — Sixel encoding hardcodes 256 colors with no capability negotiation

**File:** `internal/ui/imgview/sixel.go:44`

**The problem:** `enc.Colors = 256` is fixed regardless of what the connecting terminal actually reports supporting. True for most Sixel-capable terminals in practice, but not guaranteed by spec, and there's no fallback path if a terminal reports a smaller register count.

**Recommendation:** Low priority given the practical terminal landscape; note as a known assumption rather than something requiring immediate work.

---

### LOW — `pxPerCol = 10` fallback is an explicitly-flagged untested guess that directly drives Sixel sizing

**File:** `internal/ui/imgview/scale.go:3-7`, `internal/ui/screens/inlineimage.go:34-37`

**The problem:** This is the one spot in the reviewed diff carrying a `ponytail:`-style comment naming it as an untested fudge factor. For Kitty and iTerm2, both of which have terminal-side scale-to-fit parameters, a wrong guess here is low-risk — it just nudges the scaling target. For Sixel's fallback path (used whenever `TerminalCellPixelSize`'s `TIOCGWINSZ` read fails, which its own comment notes is common under tmux/screen), this value directly drives the pixel-space downscale before encoding, with no terminal-side correction available afterward — so a wrong guess here produces a visibly wrong-sized image, not just a suboptimal one.

**Recommendation:** Worth an actual measurement pass (common terminal font metrics) before this ships broadly, specifically for the Sixel-fallback case; the Kitty/iTerm2 usage is fine as-is.

---

### INFORMATIONAL — `EncodeKitty`'s placement-id sentinel is an implicit dual-mode API

**File:** `internal/ui/imgview/kitty.go:28`

`func EncodeKitty(img image.Image, maxCols, maxRows, placementID int)` — passing `placementID == 0` selects "anonymous, blunt delete-all" mode; any nonzero value selects "named, never blunt-deletes" mode. This is a deliberate, documented choice (there's an extensive comment elsewhere justifying the specific nonzero sentinel `kittyModalPlacementID = 999000000` used for the fullscreen modal), and it's exactly what was needed to fix one of the four Ghostty bugs. Still, a sentinel-driven behavior switch on an `int` parameter is easy to misuse from a new call site without reading the doc comment. An explicit mode type would make the two behaviors harder to confuse by accident.

---

### INFORMATIONAL — `pendingKittyDeletes` entries are resent every frame indefinitely, by design

**File:** `internal/ui/app.go:222-241`

Once a placement drops out of view, its delete gets resent on every subsequent `View()` call for the rest of the session unless the slot becomes visible again (which cancels it). This is a deliberate, well-reasoned fix for a real Bubble-Tea-throttled-render race (a countdown-based resend budget was tried first and rejected — noted in the comment itself), and placement ids are never reused so resending a stale delete is a harmless no-op. Flagging only because it's a second structurally-unbounded-but-practically-tiny map alongside the cache findings above — worth knowing about together, not fixing separately.

---

### INFORMATIONAL — Test suite is entirely pure-function/synthetic; the two post-merge fixes were both hardware-discovered

**Files:** `internal/ui/app_test.go` (`TestSyncKittyPlacements_*`, `TestSyncInlineImages_*`, `TestAccumulateKittyDeletes_*`), `internal/ui/imgview/*_test.go`

Coverage is genuinely good at the unit level — encoder framing, row/col capping, id-stability transitions, and both post-merge regressions now have dedicated tests. But every test calls the pure diff functions directly against hand-built state; none exercise `fetchInlineImageCmd`'s actual goroutine racing against a subsequent `Update`, and none assert on the final composited `View()` string (which is exactly the kind of test that would have caught the `MillerLayout` gap above). That gap is consistent with two of five commits on this branch being dedicated fixes for bugs that were only found by hand on a specific real terminal (Ghostty) after merging. Not a criticism of the fixes themselves — both are root-cause, not band-aid — but a signal that terminal-lifecycle behavior in this feature is currently verified by hand more than by test.

**Recommendation:** A small number of golden-output tests asserting the exact composited escape sequence for a fixed `View()` scenario, across both layouts, would close most of this gap cheaply.

---

## Prioritized Recommendations

1. Fix the `MillerLayout` gap by de-duplicating the modal/inline compositing logic into a shared path both layouts call — treat the missing call as a symptom of the duplication, not a standalone one-line fix.
2. Add a `MillerLayout` (and ideally cross-layout) golden-output test that would have caught #1, plus one for `TabsLayout`.
3. Bound `inlineImageCache` before this feature leaves "experimental," given the raw-RGBA payload size for Kitty.
4. Treat fetch-failure caching and Sixel's `pxPerCol` fallback accuracy as pre-GA polish items — both are low effort, low risk if deferred.
5. Everything else (unconditional per-`Update` sync, un-cancelled fetch goroutines, the `EncodeKitty` sentinel API) is fine to leave as-is; they're documented, low-blast-radius tradeoffs rather than bugs.
