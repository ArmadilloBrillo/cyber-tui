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

**Follow-up fix**: the stale-partial-pixels fix above (`5dd18aa`'s
`pendingInlineImageErasures`) turned out to still leave ghost pixels on real iTerm2,
reported after pulling `dev` to a Mac. Root cause: `injectInlineImages` blank-filled
stale rects with literal repeated space characters, and everything this function builds
is deliberately packed onto one physical line (see the doc comment on
`injectInlineImages`) so Bubble Tea's per-line diff treats it as one unit. But Bubble
Tea's renderer (`standard_renderer.go`, `ansi.Truncate`) truncates every line to the
terminal's printable-width budget before writing it, and literal spaces count against
that budget while CSI/OSC escapes don't. A single pending erasure's blank-fill
(`MaxCols x MaxRows`, several times the terminal's own width) blew that budget on
essentially every real scroll, truncating mid-erasure and silently dropping the
current-frame image redraws and the `inlineImagePaintGen` marker that followed it in
the same builder. Fixed by swapping the literal-space blank-fill for
`ansi.EraseCharacter` (`CSI n X`), which erases the same cells but is zero-width to the
truncation budget like the function's other CSI sequences already were.

**Second follow-up fix — the erasure mechanism itself replaced, not just patched**: once
the erasure escape actually reached the terminal, a deeper bug in `pendingInlineImageErasures`'
bookkeeping became visibly destructive — reported as post card borders missing their top
and bottom lines, reproducing on Feed itself while scrolling (no tab switch needed). Root
cause: a pending entry's only exit condition was `claimed()`, requiring some *currently
visible* image to land on that exact rect again — which almost never happens once an image
scrolls off-screen, moves, or the active tab switches to a screen with no images. The entry
became permanently orphaned, and its out-of-band absolute-cursor blank-fill kept re-firing
every subsequent frame, corrupting whatever unrelated content (a different post's border)
later rendered at that screen position.

A first-pass fix (bounding each entry to a fixed number of frames before dropping it
regardless of claim status) was considered and rejected as a magic-number retry-count
papering over the real defect, rather than fixing it.

The actual fix replaces the mechanism: instead of guessing "blank" is the correct
replacement for a stale row (out-of-band, invisible to Bubble Tea's own per-line diff
cache, and wrong the moment different real content later occupies that row),
`syncInlineImageErasures` now returns the affected absolute row numbers, and a new
`forceRowsDirty` helper (mirroring the Kitty-modal-cleanup line edit already in
`injectInlineImages`) appends an inert SGR-reset marker directly to those specific lines
of `base` — the same technique `inlineImagePaintGen` already uses for the
selection-touch case, just scoped to arbitrary rows instead of only the trailing line.
Bubble Tea's own diff then resends each row's real, always-correct content. This is safe
to recompute fresh every `Update()` with no carry-forward: even losing a transition to
Bubble Tea's renderer coalescing several `Update()` calls before a flush (confirmed real —
`Update()`→`View()` is 1:1 and synchronous, but physical writes happen on a decoupled 60fps
ticker that can drop an intermediate buffered frame) just leaves a row stale a little
longer, self-healing as soon as that row's content next changes — never the permanent
corruption of unrelated content the unclaimed out-of-band blank-fill risked. This also
uniformly fixes the cross-tab case with no special-casing: switching to a screen with zero
images diffs every previously-visible row as stale exactly once, forces it dirty for that
one transition frame, and — since staleness is computed fresh each call — never repeats.

**Third follow-up fix — the fullscreen image-carousel flash (`o` to open, left/right to
cycle a multi-image post)**: a different, unrelated bug — not a line-diff/erasure-tracking
issue, but a literal, intentional `tea.ClearScreen` in `handleImageViewer`'s success branch,
gated to iTerm2/Sixel and firing on every carousel cycle (`len(a.imageCarouselItems) > 1`).
The concern behind it was real: the modal box is a fixed-position, size-varying bordered box
re-centered per image (`compositeOverlays`, `xOff/yOff` derived from `imageModalCols/Rows`),
so a cycled-to smaller image's box leaves the previous, larger image's raster pixels
extending outside the new box's footprint — and iTerm2/Sixel have no Kitty-style
delete-by-placement primitive to reclaim exactly that footprint. But nuking the whole screen
to fix it produced the flash. Replaced with the same `forceRowsDirty` technique: the outgoing
box's dimensions are snapshotted (`imageModalPrevRows/Cols`, only when a modal was already
open — i.e. a genuine cycle, not a fresh open) before being overwritten, and
`compositeOverlays` renders what the *previous* box's row range was (via `l.renderImageModal`
with the previous dims substituted in — reusing the real layout-specific renderer rather than
a duplicated size formula, since `TabsLayout` adds a carousel-index hint line that
`MillerLayout` doesn't) and forces exactly those rows dirty before compositing the current
frame's box. Kitty is unaffected (already self-heals via `kittyModalPlacementID`) and keeps
skipping this entirely.

**Not yet touched**: a second `tea.ClearScreen` remains on modal *close* for Sixel
specifically (`app.go`, gated to `imgview.ProtocolSixel`, not iTerm2) — out of scope for the
iTerm2 carousel-cycle report above; worth revisiting if Sixel close-flash is ever reported.

---

## 9. WezTerm on Windows: scroll bug fixed; Kitty-protocol experiment ruled out, black image still open

First real-world testing on WezTerm (Windows), reported by the user as a scrolled-into-view
image causing the status bar to balloon by several lines and the image to render partially,
offset from its intended position.

**Root cause and fix (merged, PR #103)**: WezTerm has a confirmed bug in its iTerm2-protocol
implementation — with the default `doNotMoveCursor=0`, it scrolls its whole screen when an
image's footprint reaches the terminal's last line
([wezterm/wezterm#3266](https://github.com/wezterm/wezterm/issues/3266)). That unrequested
scroll desyncs every subsequent absolute-cursor-positioned draw `injectInlineImages` issues.
Fixed by sending `doNotMoveCursor=1` unconditionally in `EncodeITerm2` — a WezTerm extension
to OSC 1337 that real iTerm2 tolerates as an unrecognized key, so no `TERM_PROGRAM` branching
was needed. The app never relied on the terminal's own post-image cursor advance anyway.

**Second bug found once the first was fixed**: the inline image's reserved space rendered
solid black, and the fullscreen modal (`o`) did too, with a garbled fragment visible
elsewhere on screen. Likely present all along, just visually masked by the scroll
distortion. Suspected cause: the app is a native Windows console process, so WezTerm's
local-process panes on Windows are necessarily backed by ConPTY — a documented source of
interference with long or unrecognized escape sequences
([microsoft/terminal#15551](https://github.com/microsoft/terminal/issues/15551),
[#17314](https://github.com/microsoft/terminal/issues/17314)). `EncodeITerm2` sends the
whole image as one giant unchunked OSC 1337 sequence (several KB on one physical line) —
a shape ConPTY is known to mishandle, and OSC 1337 has no official chunking mechanism to
work around it.

**Experiment (branch `fix/wezterm-kitty-graphics`, reverted)**: `DetectProtocol` was
temporarily switched to map `TERM_PROGRAM=WezTerm` to `ProtocolKitty` instead of
`ProtocolITerm2` — this had never been an explicit decision in the first place (see
`protocol.go`'s original mapping, introduced in `71ff4e1` without discussion of the
Kitty-vs-iTerm2 tradeoff for WezTerm specifically). `EncodeKitty` also gained chunked
transmission (`chunkKittyPayload`, splitting large base64 payloads into 4096-byte pieces
per the protocol's official chunked-transmission mode) as part of the same experiment —
that code is harmless and spec-compliant on its own, independent of the WezTerm mapping
question, and may be worth keeping/re-proposing separately.

**Result: ruled out for Windows.** The user tested on WezTerm/Windows — the image still
rendered black, just without the stray garbled fragment seen on the iTerm2 path. Research
turned up [wezterm/wezterm#5757](https://github.com/wezterm/wezterm/issues/5757): **the
Windows build of WezTerm does not implement the Kitty graphics protocol at all** — only
iTerm2-protocol images work there (Linux supports both). So the black render was WezTerm
silently ignoring an entirely unrecognized escape sequence, not evidence of anything fixed.
That same issue also undermines the original ConPTY-corruption theory for the iTerm2 path:
it notes a third-party tool (`timg -pi`) using the same iTerm2/OSC 1337 protocol renders
correctly on WezTerm/Windows through the same ConPTY layer — so the transport itself isn't
the problem, which points at something specific to cyber-tui's own OSC 1337 output or how
it's delivered through Bubble Tea's renderer instead.

`DetectProtocol` has been reverted to map WezTerm back to `ProtocolITerm2` unconditionally.

**Byte-level diagnostic added**: temporary `CYBER_TUI_DEBUG_LOG`/`CYBER_TUI_DEBUG_IMG_DUMP`
hooks in `fetchInlineImageCmd` (`app.go`) and a startup line in `main.go` (both since
removed — this investigation is concluded), logging fetch/encode success or failure and
dumping the raw encoded sequence. First finding: fetch and encode both succeeded — a
365×512 WebP produced a 521,087-byte (~509KB) single-line OSC 1337 sequence. That ruled out
a silently-failing fetch (the other live hypothesis at the time).

**Downscaling fix (kept, independently useful)**: `EncodeITerm2`/`EncodeKitty` were PNG-
encoding the *full source resolution* regardless of how small the target display box was,
relying on the terminal's own scale-to-fit — unlike `EncodeSixel`, which already downscaled
in pixel space first (Sixel has no terminal-side scale-to-fit to rely on). Added a shared
`downscaleToBox` helper (`imgview/scale.go`) and wired it into all three encoders. Cut the
521KB payload to 25,991 bytes (~20x) for the same image. **The black render was unchanged**
— ruling out payload size as the cause, and (combined with the same image rendering fine on
real iTerm2/macOS at the original 521KB size) ruling out "OSC sequences over some size
threshold" as a blanket ConPTY limitation.

**Root cause found via isolated replay**: captured the 26KB sequence and replayed it
directly to a plain WezTerm pane with a standalone tool that writes one clean `CUP` escape,
then the image, and nothing else in the same write — **it rendered correctly**. The same
bytes that fail inside the running app succeed in isolation; the only difference is
delivery. `injectInlineImages` batches every visible slot's `CUP`+OSC pair, Kitty deletes,
and a trailing "park cursor at bottom" `CUP` into one line, which Bubble Tea's renderer
writes in a single syscall alongside the rest of the frame. That's the shape
[microsoft/terminal#17314](https://github.com/microsoft/terminal/issues/17314) ("OSC
sequences received out-of-order in 3rd-party terminals") describes: ConPTY can defer/reorder
an OSC relative to a CSI that follows it in the same write, so the image renders wherever a
later cursor-move pointed instead of its own — matching the fragment-in-the-wrong-place
symptom exactly, and explaining why every earlier attempt (protocol switch, downscaling)
left it unchanged: none of them touched the batching/ordering.

**Attempted fix, reverted — the batching isn't ours to fix**: reordered `injectInlineImages`
so no CSI followed any image's OSC (park-cursor and paint-gen marker moved before the image
loop). Rebuilt and retested: **still didn't render, still the same wrong-place fragment**.
Traced into Bubble Tea's vendored `standard_renderer.go` (`flush()`, v1.3.10): in alt-screen
mode, it unconditionally appends its own trailing `CursorPosition` CSI *after* all diffed
line content, in the same single `Write()` call — regardless of what order our own code
builds its content in. That's the actual last-CSI-after-OSC, and it's baked into the
library, not reachable from `injectInlineImages`. Reverted the reorder (no benefit, and it
introduced a real cosmetic regression — the terminal cursor visibly resting on the last
image instead of parked at the bottom row — on every platform, iTerm2/Sixel/Kitty alike,
since none of them set a cursor-suppression flag strong enough to survive it).

**Decision: stop here.** A real fix would mean bypassing Bubble Tea's renderer entirely for
image writes (writing to stdout in a separate syscall, outside the library's render
pipeline) — a genuine architectural change with real risk of new bugs (flicker, timing/race
issues) and no guarantee it resolves ConPTY's exact reordering rule even if built correctly.
Given this is fundamentally a Windows-only ConPTY subsystem bug, not something in cyber-tui's
control, the user chose not to pursue it further. **Inline and fullscreen images do not
reliably render on WezTerm/Windows — known, accepted limitation, not planned to be fixed.**
Both the scroll fix (PR #103) and the downscaling improvement are kept — genuinely correct
and useful independent of this outcome, and downscaling benefits every terminal/platform by
cutting transmitted payload size for small thumbnail slots.

---

## 10. Sixel/Konsole: stale pixels on scroll and carousel-cycle — seven rounds to a working fix

Reported on Konsole (Sixel protocol): inline images left stale/ghost pixels behind on
scroll, and the fullscreen carousel left stale pixels on cycle. Suspected at first as a
same-day regression from an unrelated rework (`d9aa58f` onward, earlier that day) that had
added a selection-recolor repaint fix for a bug found on **iTerm2** and applied it jointly to
`iTerm2 || Sixel` via a shared code path. Seven live-tested rounds, on real Konsole hardware
each time (the sandbox has no Sixel-capable terminal), converged on a working fix — the
findings below correct several wrong turns along the way, kept here so the next person
doesn't retread them.

**Round 1 — targeted `CSI 2K` line erase, reverted.** Tried prepending an Erase-in-Line
escape to each stale row before Bubble Tea's normal per-line resend. Live result: didn't
clear the stale pixels, and additionally corrupted unrelated on-screen text on the carousel's
first cycle (`CSI 2K` erases the *entire* line, including content outside the image's own
footprint that the resend afterward didn't always cover).

**Round 2 — `tea.ClearScreen` Cmd, reverted.** The one mechanism already proven to work
elsewhere in this codebase (the pre-existing Sixel modal-close handler, `app.go`, already
used it). Live result: pixels *did* clear, but with a bad flash, and — a real correctness bug
— wiped inline-image thumbnails behind the fullscreen modal until it closed, since
`compositeOverlays` never redrew anything behind an open modal and nothing else repainted it.

**Round 3 — revert Sixel to its pre-`d9aa58f` baseline (no erasure at all), reverted.** The
user recalled Sixel/Konsole working fine before that day's rework, and the code from just
before it really did have zero erasure/repaint machinery for Sixel — worth checking whether
"stop doing anything" was itself the fix. Live result: **also broken** — "the same issues
persist." This ruled out the regression theory: Sixel's stale-pixel problem predates the
iTerm2-motivated rework; it isn't collateral damage, and doing nothing isn't an option.

**Root cause, established across rounds 1–3**: no *in-place* resend or erase — full-line,
partial, or none at all — reliably clears Sixel raster pixels on Konsole. Only a genuine
full-screen erase does (confirmed both by the pre-existing modal-close handler and by round
2's live test). Every fix that had worked for iTerm2 that day (`forceRowsDirty`: resend real
content, no erase) had already been applied to Sixel via the shared code path and had already
failed — it just hadn't been noticed as Sixel-specific before this investigation.

**Round 4 — single-write full repaint.** `tea.ClearScreen` flashes because
`standardRenderer.clearScreen()` (`bubbletea@v1.3.10/standard_renderer.go`) does two
*separate* writes: it executes the erase immediately when the Cmd is processed, then just
marks the render cache empty — the actual content redraw only happens on the next framerate
tick (up to ~16ms later by default), a real visible blank gap, confirmed by reading the
library source. Fix: emit the same erase (`ansi.EraseEntireScreen` + `ansi.CursorHomePosition`)
as the first bytes of `View()`'s own return value instead, so the erase and the full redraw
travel in one write with nothing in between — `sixelFullRepaint` (`layout.go`). Since a real
erase means any row Bubble Tea's diff decides to *skip* (because its bytes happen to match
last frame) would come back genuinely blank rather than stale, every row is also forced dirty
via `forceRowsDirty`, not just the ones known to have moved. Triggered from state already
tracked safely against Bubble Tea's render-throttling (`inlineImageStaleRows` for scroll, an
`imageModalPrevRows/Cols` size mismatch for carousel-cycle) instead of a one-shot Cmd. Also
fixed the round-2 hidden-thumbnails bug in the same pass, by having `compositeOverlays` draw
inline images into the frame *before* compositing the modal on top, not skip that step
entirely while the modal is open.

**Round 5 — the fixed marker collision.** Live result: slow scroll clean, fast scroll still
stale; carousel worked but occasionally turned the *entire screen* black. `forceRowsDirty`
force-dirties a row by appending a marker — a fixed `"\x1b[0m"` was fine for a single
repainted-vs-normal comparison, but under fast input Bubble Tea can drop several intermediate
`View()` calls before a flush (confirmed by reading `flush()`'s ticker loop), so two
*actually delivered* frames can both be repaints. Two repaints with the same fixed marker
produce byte-identical output for any row whose real text didn't change between them —
Bubble Tea's diff then skips resending it, and since a real erase already wiped the screen,
a skipped row comes back genuinely blank rather than stale. The more rows involved (the
carousel case touches the whole screen), the more visible the failure — hence occasional
full blackouts specifically there. Fixed with a collision-proof marker: `App.sixelRepaintGen`,
a monotonic counter bumped at both trigger points, encoded as a zero-width 24-bit true-color
SGR set immediately followed by a hard reset (`sixelDirtyMarker`, `layout.go`) — no characters
in between, so nothing is ever actually colored, and ~16.7 million distinct values before two
real flushes could coincidentally collide (not realistic from keyboard input).
`forceRowsDirty` took a `marker` parameter so iTerm2's existing fixed-marker call sites stay
byte-for-byte unchanged.

**Round 6 — the same collision, one more spot.** Live result: fast scroll much better, "almost
okay," not fully clean. `injectInlineImages`' trailing line — the one carrying the actual
per-slot image draws — still used `inlineImagePaintGen`'s original fixed `%2` parity toggle
for Sixel (deliberately left alone in round 5, since it wasn't yet confirmed broken). Fast
scrolling in this app typically moves the arrow-key selection at the same time it scrolls the
viewport, so the selection-touch trigger and the position-change trigger fire on the same
Updates — the parity toggle's identical collision risk was still live there. Fixed by having
`touchesVisible` also bump `sixelRepaintGen` for Sixel, and using `sixelDirtyMarker` there
unconditionally instead of the toggle (still exactly a fixed marker for iTerm2).

**Round 7 — the carousel's remaining black screen.** Live result: fast scroll "barely any
stale pixels" — solved. Carousel: holding the cycle key raced the on-screen index counter far
ahead of the displayed image, and after the burst "caught up," the screen went black except
the tab bar until any keypress. `openImageInTerminal` (`app.go`) redoes the full decode+encode
work on *every* cycle — real CPU cost — even on an image-bytes cache hit; `cycleImageCarousel`
fired one of these per keypress with no debouncing. `imageFetchGen`'s newest-wins guard
already discarded all but the last *result*, but did nothing to stop the wasted work or the
resulting flood of Update/View cycles while a key was held — which matched the "counter races
ahead of the image" symptom exactly, though this round couldn't fully confirm it as the exact
mechanism behind the black screen without further hardware access. Fixed by debouncing:
`cycleImageCarousel` moves the index immediately (counter stays responsive) but the real fetch
now waits `carouselCycleDebounce` (120ms) via a `carouselCycleGen`-guarded `tea.Tick` —
mirrors the existing `notify()`/`gifFrameTickCmd` gen+tick pattern already used elsewhere in
this file. Live result: "carousel works much better." Considered resolved.

**Round 8 — the same collision, left open for iTerm2.** Reported on real iTerm2: an inline
image occasionally not drawn fully on feed scroll/refresh — no stale/ghost pixels (unlike the
Konsole/Sixel symptom), consistent with iTerm2's resend-only strategy: a *skipped* resend
silently leaves old-but-real content in place rather than a blank hole, so it reads as "didn't
finish drawing" rather than "left pixels behind." Root cause: rounds 5–6 above gave Sixel a
collision-proof marker (`sixelRepaintGen`/`sixelDirtyMarker`) but deliberately left iTerm2 on
its original fixed/low-entropy markers — a plain `"\x1b[0m"` for the stale-row resend
(`forceRowsDirty` in `injectInlineImages`) and an `inlineImagePaintGen % 2` parity toggle for
the trailing per-frame marker — on the reasoning that iTerm2's in-place resend (no erase) had
already been "proven sufficient on real iTerm2," which wasn't tested under the same
fast-scroll/fast-refresh stress that surfaced the bug for Sixel. Both call sites, plus a third
(the carousel-cycle prev-box cleanup in `compositeOverlays`, also iTerm2-only), have the exact
same collision: two consecutive *actually flushed* frames can share the same fixed marker and
get wrongly skipped by Bubble Tea's line diff.

**Fix**: generalized the mechanism instead of duplicating it — `sixelRepaintGen`/
`sixelDirtyMarker` renamed to protocol-neutral `imageRepaintGen`/`imageDirtyMarker` and shared
by both protocols; the `graphicsProtocol == ProtocolSixel` gates around the counter bumps in
`syncInlineImages` and the carousel-cycle handler were removed so iTerm2 bumps it under the
same triggers Sixel already did (a stale row, a selection touching a visible image, a
size-changed carousel cycle); all three iTerm2 call sites now use `imageDirtyMarker(gen)`
instead of a fixed marker or parity toggle.

**Current state**: Sixel and iTerm2 still use different repaint *mechanisms* — iTerm2 resends
real content with no erase (`forceRowsDirty`), Sixel does a real single-write full-screen erase
(`sixelFullRepaint`) — but both now share the same collision-proof dirty-marker/generation-
counter machinery (`imageDirtyMarker`, `imageRepaintGen`) rather than Sixel alone having it.
Kitty was never part of any of this — its placement-delete primitive means it never needed
erasure or resend tricks at all. No known open issues; a true marker collision remains
theoretically possible but requires ~16.7 million repaint triggers between two actual flushes,
not realistic from keyboard input.
