# Graphics protocols review — Kitty, Sixel, iTerm2

Scope: `internal/ui/imgview/*`, `internal/ui/screens/inlineimage.go`, and the
App/layout wiring in `internal/ui/app.go` + `internal/ui/layout.go` that
drives the fullscreen image modal and inline post/reply images.

## Overall shape

`internal/ui/imgview` (encode/fetch/scale, ~750 lines) is clean: small
single-purpose files, no cross-protocol duplication beyond what's genuinely
shared (`downscaleToBox`, `fitBox`), good doc comments, solid test coverage
(27 tests covering caps, chunking, cell-size fallback, upscale prevention).
No findings there.

Almost all the complexity — and everything below — lives in `app.go`/
`layout.go`'s inline-image state machine (~600 lines across ~15 fields on
`App`). That machine exists to work around real, live-confirmed terminal
bugs (Konsole Sixel stale pixels, iTerm2 dropped OSC writes, WezTerm/ConPTY
scroll corruption), each documented with its own reasoning in
`docs/plan-inline-images-improvements.md`. It is dense, but it is not
accidental complexity — most of it is there because a simpler version was
tried first and demonstrably broke on real hardware. The findings below are
about where that machinery has outgrown its own justification, not about
ripping it out.

## Findings

### 1. Stale doc comment / dead code path: `EncodeKitty`'s anonymous-placement mode

`imgview/kitty.go:17-22` documents `placementID == 0` as "used by the
fullscreen modal, where only one image is ever shown at a time," including
the blunt `a=d,d=A` self-heal delete prefix. That's no longer true: the
modal was moved onto a dedicated fixed id, `kittyModalPlacementID =
999000000` (`app.go:2755`, introduced to stop the blunt delete from also
wiping inline placements once both features could be on screen at once).
Grepping every production call site (`app.go:3058`, `app.go:3245`) confirms
neither ever passes `0` — only the unit tests do. The `placementID == 0`
branch and its `a=d,d=A` prefix are dead in production.

Low risk (it's inert, and the tests keep exercising the code so it can't
bitrot silently), but the doc comment actively misdescribes current
behavior — a future reader fixing a modal bug here will look in the wrong
place. Either delete the anonymous mode (making placementID always
required) or fix the comment to say it's unused-but-kept for [reason].

### 2. `debug`-gated diagnostic logging has outlived the investigation it was added for

14 `log.Printf("image: ...")` calls are scattered through `app.go` and
`layout.go` (`app.go:2951,2977,2994,3000,3011,3107,3115,3185,4296,4301`,
`layout.go:369,377`), every one tagged `ponytail: temporary diagnostic` and
citing specific rounds of `docs/plan-inline-images-improvements.md`
(Round 6, Round 14). That doc's own changelog shows sections 1-10 all
resolved, with section 10 ("seven rounds to a working fix") closing out the
last open bug (Sixel/Konsole). The investigation these logs were
instrumented for appears concluded, not ongoing.

They're harmless as shipped (gated behind `a.debug`, off by default), but
they're real standing cost: 14 call sites of narration to read past on
every future touch of this code, a `debugLastDrawnByScreen`/
`debugLastActiveScreen` package-level var pair in `layout.go:355-379` that
exists only to support one of them, and a live tripwire for reviewers
(is this active instrumentation or dead weight?) each time this file comes
up in review. Worth a pass to delete the ones whose bug is confirmed fixed,
and keep only whatever's still earning its keep for open work.

### 3. Timing constants: reasonable, but three near-identical magic numbers with no shared home

- `da1ProbeTimeout = 150ms` (protocol.go) — DA1 probe ceiling.
- `carouselCycleDebounce = 300ms` (app.go) — modal carousel keypress debounce.
- `inlineImageSwitchSettleDelay = 250ms` (app.go) — post-screen-switch draw hold-back.
- `inlineImageStaleGrace = 500ms` (app.go) — stale-row accumulator clear delay.

Each is individually well-justified in its doc comment (the two settle
delays explicitly note they're empirical mitigations for unmeasurable
terminal-side timing, "revisit if still reported after this change," which
is exactly the right way to flag this kind of thing). This isn't a bug —
just noting that if a future terminal turns out to need yet another one of
these, it's the fourth independent hand-tuned constant for the same
underlying problem class ("give the terminal time to finish processing a
large write before sending the next one"), and at that point it's worth
naming the pattern once (e.g. a single `terminalWriteSettleDelay` used with
different multipliers) rather than a fifth standalone constant with its own
paragraph of justification.

### 4. `App` has grown ~15 image-specific fields with fairly intricate cross-field invariants

`kittyPlacementIDs`/`kittyNextPlacementID`/`kittyVisibleKeys`/
`pendingKittyDeletes`, `inlineImageVisibleRects`/`inlineImageStaleRows`/
`inlineImageStaleSince`, `inlineImageLastSelKey`/`inlineImagePaintGen`,
`imageRepaintGen`, `screenSwitchedAt` — nine fields whose correctness
depends on being updated together, in the right order, only from
`syncInlineImages`/`syncKittyPlacements`. Each is well-commented in
isolation, but understanding *why* a given field exists requires reading
four other fields' comments too (e.g. `imageRepaintGen`'s doc comment
references `pendingKittyDeletes`' "never auto-expire" reasoning by name).

This isn't a bug and I'm not recommending a mid-flight refactor of working,
hard-won logic — but if this subsystem grows again (a fourth protocol, or
per-protocol quirks multiplying further), it's a reasonable trigger to pull
all of it (fields + `sync*`/`accumulate*` functions) into its own file or
small struct with a documented invariant list at the top, rather than
letting it keep growing as loose fields on `App`.

### 5. Everything else checked and found fine

- **Protocol detection** (`protocol.go`): env-var detection for
  Kitty/iTerm2/WezTerm plus an active DA1 probe for Sixel is the right
  design — no anti-pattern, the WezTerm-on-Windows special case is
  explained and narrowly scoped.
- **Shared encode helpers**: `downscaleToBox`/`fitBox`/`fitCols`/`fitRows`
  are genuinely shared across all three protocols with no copy-paste
  drift; `pxPerCol`'s single default is reused consistently.
- **Kitty chunking** (`chunkKittyPayload`): correct handling of the
  first-chunk-has-full-control / subsequent-chunks-minimal-control split
  per spec, degenerates cleanly to the single-chunk case.
- **Sixel/iTerm2 lack of placement-delete**: the two different repaint
  strategies (`forceRowsDirty` resend-in-place for iTerm2 vs
  `sixelFullRepaint` full erase for Sixel) are each backed by a specific
  "tried the other one, it corrupted the screen on real hardware" note,
  not a guess. The `imageDirtyMarker` collision-proofing (why a fixed or
  toggled marker isn't enough under Bubble Tea's render coalescing) is a
  real, non-obvious invariant and is correctly explained once and reused.
- **Kitty placement-id permanence** (`kittyPlacementIDs` never pruned):
  justified against a specific stale-cache-reuse bug; the tradeoff
  (session-lifetime memory growth of a few bytes per image ever seen) is
  explicitly accepted and bounded by the fact it's just small ints in a
  map.
- No SSRF/security issues: `canRenderImageInline`/`canInlineImages` both
  gate on `!a.ephemeral`, consistent with the existing SSH-session guard
  noted in `routeURL`.

## Suggested next step

Findings 1 and 2 are cheap, low-risk cleanups (delete dead code / stale
comment; prune concluded-investigation logging) worth doing opportunistically.
Findings 3 and 4 are "watch, don't act" — they describe a ceiling this code
is approaching, not a problem it already has.
