# Inline-Images Architecture Critique — Solution Architect First Look

**Reviewed branch:** `feature/inline-images` (5 commits over `main`, merge-base `2adf4c9`) · **Scope:** 33 files, +1901/-182 lines · **Lens:** independent read of the code, not a review of any other document in this repo.

This covers Kitty, Sixel, and iTerm2/WezTerm inline image rendering for post/reply bodies in Feed and PostDetail (`d8c09ca`, `c896ca1`), two post-merge hardware-discovered bug-fix commits (`70925fd`, `3ae054c`), and the pre-existing fullscreen image-viewer modal that these share encoders with.

---

## Executive summary

| Severity | Count |
|---|---|
| High | 1 |
| Medium | 4 |
| Low | 2 |
| Informational | 2 |

The core architectural choice — route every image-related state change through Bubble Tea messages and mutate `App` fields only inside the single-threaded `Update` loop — is correct and consistently applied. No goroutine in this feature writes to `App` state directly; results always come back as a typed message (`inlineImageFetchedMsg`, `imageFetchedMsg`) and get folded in by `handleInlineImageFetched`/the modal's handler. There are no mutexes in this feature and none are needed. That is not a finding, it is the right call, made once and followed everywhere.

The standout problem is that the feature was built as **two independently-written pipelines** — a fullscreen modal path (`openImageInTerminal`, `internal/ui/app.go:2634-2696`) and an inline-in-post-body path (`syncInlineImages`/`fetchInlineImageCmd`, `app.go:2504-2621`) — that share only the leaf `imgview` encoder functions, not orchestration, caching, or compositing. That duplication is what let the inline path never get wired into `MillerLayout` at all, and it compounds a pre-existing, independently-confirmed bug: `MillerLayout` cannot be selected by any user at runtime, on `main`, before this branch. This feature's ~35 lines of Miller-specific fullscreen-modal maintenance (`internal/ui/layout_miller.go:139-169`) and its dedicated test coverage (`layout_test.go`, `app_test.go`) are exercising code path no build of this app can ever reach.

Everything else is smaller and mostly self-inflicted scope cuts that are honestly commented in the code — an unbounded session cache, a fetch path with no cancellation, a couple of encoder inconsistencies. The two post-merge commits are genuine root-cause fixes for real hardware bugs, not band-aids; they are evidence of a test suite that verifies pure diff functions well and rendered terminal output not at all.

---

## Architecture overview

`App.Update` (`app.go:488-495`) delegates to `updateInner`, then unconditionally calls `syncInlineImages` on the result and batches its command with whatever `updateInner` returned. `syncInlineImages` (`app.go:2514-2557`) is a pure diff: when `canInlineImages()` is true (user toggle on, not an SSH-ephemeral session, a graphics protocol was detected, image viewer preference isn't "browser" — `app.go:2383-2388`) it reads the active screen's `VisibleInlineImages()`, and for anything not already in `inlineImageCache` or `inlineImageFetching`, appends a `fetchInlineImageCmd` (`app.go:2572-2600`) to a `tea.Batch`. That closure runs as a Bubble Tea-scheduled goroutine, fetches via `imgview.Fetch` under a 20s `context.WithTimeout`, encodes for whichever protocol was detected at startup, and returns `inlineImageFetchedMsg`. `handleInlineImageFetched` (`app.go:2607-2621`) is the only place that writes `inlineImageCache`.

View-time compositing is layout-owned rather than shared. `TabsLayout.View` (`layout_tabs.go:112-115`) calls `activeInlineImageSlots()` and, if anything is visible or a Kitty delete is pending, calls `injectInlineImages` (`layout_tabs.go:132-149`), which splices `\x1b[row;colH<encoded>` sequences directly into the already-rendered frame string — deliberately outside lipgloss's own compositing, because embedding a raw image escape sequence inside a string lipgloss reflows would corrupt it (the same reasoning is documented for the fullscreen modal at `layout_tabs.go:68-70`). `MillerLayout.View` (`layout_miller.go:130-171`) has its own, separately-written block for the fullscreen modal and Kitty-cleanup-flag injection — inherited mostly unchanged from before this branch — but contains no call to `activeInlineImageSlots` or any inline-splice function anywhere in the file.

Both pipelines independently re-implement the same three-way protocol switch (Kitty / iTerm2 / Sixel → `EncodeKitty` / `EncodeITerm2` / `EncodeSixel`) with different placement-id schemes: the modal always uses a single fixed sentinel `kittyModalPlacementID = 999000000` (`app.go:2444`) since only one image is ever shown at once, while inline rendering hands out a permanently-assigned per-slot id via `kittyPlacementIDs`/`kittyNextPlacementID` (`app.go:242-243`) since several images can be on screen simultaneously. Both are reasonable choices for their respective constraints — but they are two designs, arrived at and coded separately, not one design parameterized by "how many images can be visible at once."

---

## Findings

### HIGH — `MillerLayout` cannot be reached by any user, and this feature adds real logic to it anyway

**Files:** `internal/ui/app.go:431-433` (`layoutFromName`), call sites `app.go:426` and `app.go:1243`; `internal/ui/layout_miller.go` (whole file, effectively dead)

**The problem:** `func layoutFromName(_ string) Layout { return TabsLayout{} }` discards its argument and always returns `TabsLayout{}`. Both call sites — restoring a saved session's `Layout` field, and applying a settings-screen save's `layoutName` — route through this function, so `App.layout` can never become `MillerLayout{}` regardless of what's persisted in `~/.cyber-tui.json` or chosen in-app. Confirmed this predates the branch: `git show main:internal/ui/app.go` has the identical discard-argument body. There is also no settings-screen UI item to pick a layout at all — the settings list (`internal/ui/screens/settings.go:140-172`) only exposes "timezone", "image viewer", and "inline images (experimental)"; `SettingsModel.layoutName` is tracked and round-tripped (`settings.go:202,226-241`) but never surfaced as a choosable item. `"miller"` appears in exactly one other place in the whole codebase outside comments/docs: a status-bar label rendered by `MillerLayout` itself (`layout_miller.go:475`) — a label no one can ever see it render.

**Failure scenario:** None, today — no user path exists to reach `MillerLayout`, so its incompleteness has zero live impact. The actual failure is on the team: this branch adds a genuine bug-fix (`imgview.DeleteKittyPlacement(kittyModalPlacementID)` replacing a hardcoded blunt delete, `layout_miller.go:166`, matching the same fix applied to `layout_tabs.go`) to a file that cannot run, and ships without ever wiring `injectInlineImages` into it — an omission nobody manual-testing the app could have caught, because there was no way to select the layout to test in the first place. `layout_test.go:66,337,385` and `app_test.go:317` instantiate `MillerLayout{}` directly in tests, which is the only way this code executes at all; that gives a false signal of coverage for something no shipped build exercises.

**Recommendation:** This is a repo-level bug, not scoped to this feature, but it should block calling `MillerLayout` "supported" going forward. Either fix `layoutFromName` to actually branch on its argument and add the missing settings UI item, or delete `layout_miller.go` and its tests until there's a real plan to finish and expose it. Shipping partial, untestable-in-practice logic into a file that already can't be reached is worse than leaving it alone — it's maintenance cost with no corresponding coverage.

---

### MEDIUM — `inlineImageCache`/`kittyPlacementIDs`/`pendingKittyDeletes` grow for the life of the session, with no cap

**Files:** `app.go:184-197` (`inlineImageCache`), `app.go:198-243` (`kittyPlacementIDs`, `kittyNextPlacementID`, `pendingKittyDeletes`)

**The problem:** `inlineImageCache` is keyed by slot key × URL × column budget × protocol (`inlineImageCacheKey`, `app.go:2415-2423`) and is never evicted — every distinct image seen, at every distinct column width the terminal was ever resized to, stays cached for the session. The doc comment explicitly names this an accepted cut ("see the plan's accepted..."), but no such plan document exists anywhere in the repository (`find`/`grep` across `docs/` and `internal/` for anything inline-image-related turns up nothing — the only "plan" files present are `docs/plan-guild-join-leave.md`, unrelated). For Kitty, the cached value is a base64-encoded raw 32-bit RGBA payload with no compression (`f=32`, `imgview/kitty.go:34-47,58`) — the most memory-expensive encoding available here, since `EncodeITerm2` PNG-encodes first (`imgview/iterm2.go:21-25`). `kittyPlacementIDs` and `kittyNextPlacementID` are coupled to the same cut by design: an id is never reused within a session (`app.go:235`), and the doc comment explains this is necessary given the current no-eviction cache, not incidental.

**Failure scenario:** A long session that scrolls through many image-bearing posts, or gets resized repeatedly (a dragged terminal window, a tmux pane reflow changing the effective column count), accumulates cache entries indefinitely. Each resize multiplies the cache-key space rather than replacing entries in place.

**Recommendation:** Bound `inlineImageCache` (LRU by byte size, given the RGBA payload size varies a lot by image) before removing the "experimental" label from this feature. Any eviction scheme needs to account for the `kittyPlacementIDs` coupling explicitly — evicting a cache entry for a still-visible-but-off-slot Kitty placement without also invalidating its id would desync the two.

---

### MEDIUM — Kitty and iTerm2 never query real terminal cell size; only Sixel does, in the same package

**Files:** `imgview/kitty.go:48`, `imgview/iterm2.go:26` vs `imgview/sixel.go:23-31`; `imgview/cellsize_unix.go`, `imgview/cellsize_windows.go`

**The problem:** `TerminalCellPixelSize(fd int) (cellW, cellH int, ok bool)` reads real cell-pixel geometry via `TIOCGWINSZ` on Unix (`cellsize_unix.go:14-20`; always `ok=false` on Windows, `cellsize_windows.go:8-10`). `EncodeSixel` accepts `cellPxW, cellPxH` and only falls back to the hardcoded `pxPerCol=10, 2*pxPerCol=20` constant when the caller passes `<=0` (`sixel.go:24-26`) — and callers do query the real value first (`app.go:2588` inline, `app.go:2680` modal). `EncodeKitty` and `EncodeITerm2` don't take cell-size parameters at all; they call `fitBox(..., pxPerCol, 2*pxPerCol)` unconditionally (`kitty.go:48`, `iterm2.go:26`), meaning the assumed 10×20px cell is used for every Kitty/iTerm2 render regardless of the terminal's actual font metrics, even on the same machine where the ioctl is already being called for Sixel a few lines away in the caller.

**Failure scenario:** Kitty's `c=%d,r=%d` display-size hint (`kitty.go:59-60`) is computed from a guessed cell size, then the terminal itself scales to fit that hint — so a wrong guess just nudges what fraction of available space the image claims, not a crash or corruption. Low severity in isolation. It's flagged as a finding because it's an inconsistency with no protocol justification: Kitty's protocol has no aversion to precise sizing, the real value is one function call away, and Sixel in the same file already proves the pattern works. This reads like an oversight from writing the three encoders at different times rather than a deliberate simplification.

**Recommendation:** Thread `TerminalCellPixelSize`'s result into `EncodeKitty`/`EncodeITerm2` the same way `EncodeSixel` already receives it, keeping `pxPerCol` only as the genuine last-resort fallback for all three instead of the only path for two of them.

---

### MEDIUM — In-flight inline fetches have no cancellation path

**Files:** `app.go:2572-2600` (`fetchInlineImageCmd`), `app.go:2607-2621` (`handleInlineImageFetched`)

**The problem:** Each fetch is bounded by a 20s `context.WithTimeout` so it's not unbounded, but nothing cancels it early. Scrolling a post's image out of view, toggling inline images off mid-fetch, navigating to a different screen, or quitting the app all let the goroutine run to completion; `inlineImageFetching[key]` is cleared only when the result message arrives (`app.go:2610`). Contrast with the fullscreen modal path, which has a staleness guard: `imageFetchGen` is incremented on every new open (`app.go:2642-2643`) and presumably checked against the message's `gen` field on arrival, so a superseded fetch's result gets discarded rather than applied. The inline path has no equivalent generation check — a slow fetch for a slot that's since scrolled away still writes into `inlineImageCache` when it lands, which is harmless (nothing reads a cache entry for a URL that's not currently visible) but the wasted network+decode work isn't avoided either way.

**Failure scenario:** Rapidly scrolling past many image-bearing posts fires a fetch per distinct slot seen, all of which run to completion even for slots visible for a fraction of a second. Bounded in the worst case by the 20s timeout and this feature's own size/pixel caps, so not a resource exhaustion risk today, but it's the kind of gap that gets worse if this pattern is copied for a more expensive per-item fetch later.

**Recommendation:** Store a `context.CancelFunc` alongside each `inlineImageFetching` entry and call it when a key drops out of `activeInlineImageSlots()`, on toggle-off, and on quit.

---

### MEDIUM — `syncInlineImages` runs unconditionally on every `Update`, not gated to relevant messages

**File:** `app.go:488-495` (call site), `app.go:2514-2557` (body)

**The problem:** Every keypress, tick, resize, or unrelated screen navigation calls `syncInlineImages`, not just messages that could plausibly change the visible image set. The function itself short-circuits cheaply when `canInlineImages()` is false or the visible-slot diff is empty, so this isn't measured as a performance problem — it's flagged as a structural habit. "Recompute a screen-derived diff on every message, relying on the diff being cheap rather than gating on message type" is a pattern that degrades quietly if it's ever copied for something costlier, and it makes "when does image state change" harder to reason about than it needs to be (the honest answer today is "always, but usually a no-op").

**Recommendation:** Not urgent in isolation. Worth revisiting if this exact pattern gets reused for another per-frame derived-state sync, or if a future profiling pass shows `Update` latency pressure.

---

### LOW — Fetch failures are cached as silent blanks, with no retry signal

**File:** `app.go:2607-2621` (`handleInlineImageFetched`)

**The problem:** On `err != nil`, the function clears the in-flight marker and returns without touching `inlineImageCache` (`app.go:2617-2619` only runs on the success path) — so the slot stays permanently un-cached, and `syncInlineImages` retries it on the very next `Update`, since neither map remembers the failure. There is no failure memoization at all: a persistently-failing image URL is refetched on every message while its post stays visible, with no backoff.

**Failure scenario:** A post containing a permanently-broken image URL, left visible while the user is idle-scrolling elsewhere on the same screen, causes a fresh HTTP request (bounded by the 20s timeout) on every `Update` that includes that slot in the visible set — keystrokes, ticks, anything.

**Recommendation:** Cache a "failed" sentinel (distinct from "not yet attempted") with a short cooldown, so a dead link doesn't get hammered every frame it's visible.

---

### LOW — Sixel hardcodes 256 colors with no capability negotiation

**File:** `imgview/sixel.go:44`

**The problem:** `enc.Colors = 256` is fixed regardless of what the connecting terminal reports. True for essentially every Sixel-capable terminal in practice; not guaranteed by spec.

**Recommendation:** Low priority; document as a known assumption rather than treat as urgent.

---

### INFORMATIONAL — Two independently-written encode-dispatch blocks are the root cause behind the Miller gap

**Files:** `app.go:2572-2600` (inline) vs `app.go:2634-2696` (modal)

Both functions contain the identical `switch proto { case ProtocolITerm2: ...; case ProtocolSixel: ...; case ProtocolKitty: ... }` structure, written twice, with different surrounding concerns (placement-id source, GIF-frame looping in the modal only, caching scope). This isn't wrong — Go doesn't make sharing a five-line switch trivially worth abstracting on its own — but it's the same duplication pattern that let one caller (`TabsLayout`) get inline compositing wired up while the other reachable... in principle... layout did not. Flagging as informational because the High finding above already covers the concrete consequence; this is the structural reason a second consequence (silent drift between the two pipelines on any future protocol-handling change) remains open even after `MillerLayout` itself is fixed or removed.

---

### INFORMATIONAL — Test suite verifies pure diff/encode functions well; rendered `View()` output, never

**Files:** `internal/ui/app_test.go` (`TestSyncKittyPlacements_*`, `TestSyncInlineImages_*`, `TestAccumulateKittyDeletes_*`), `internal/ui/imgview/*_test.go`, `internal/ui/screens/feed_test.go`, `postdetail_test.go`

Coverage of the diff/encode logic in isolation is genuinely good — id-stability transitions, row/col capping, fetch size/pixel/frame-count guards, and both post-merge Ghostty regressions now have dedicated regression tests. But every test calls pure functions directly against hand-built state; none assert on `TabsLayout.View()`'s or `MillerLayout.View()`'s actual composited output string. That's precisely the test category that would have caught the Miller gap (an assertion that a visible slot's escape sequence appears in `MillerLayout.View()`'s output would have failed from the day `injectInlineImages` was written), and it's consistent with two of five commits on this branch being fixes for lifecycle bugs found by hand on real hardware (Ghostty) after merge rather than by any test.

**Recommendation:** A small number of golden-output tests — fixed terminal size, fixed image content, assert the exact composited escape sequence appears at the expected cursor position — for each layout, would close most of this gap cheaply.

---

## If no image support existed yet: how I would have built this

Given the constraint that terminal image protocols require splicing raw escape sequences around lipgloss's own rendering (a genuine, unavoidable constraint this codebase already discovered and solved correctly), here is what I'd do differently starting from zero:

**One shared pipeline, not two.** A single `imagePipeline` type owning the cache, in-flight tracking, and Kitty placement-id allocation, parameterized by "how many images can this call site show at once" (1 for the modal, N for inline) rather than being two hand-built implementations of "fetch, encode, cache, track a placement id." The three-way protocol switch gets written once. This is the single highest-leverage change: it removes the class of bug the Miller gap belongs to, not just that one instance of it.

**Compositing lives above the layout, not inside each one.** Instead of every `Layout.View()` implementation being individually responsible for remembering to call `injectInlineImages`, put one call in `App`'s top-level render path — after `a.layout.View(a)` returns, before the string reaches the terminal — so a layout physically cannot skip it. A `Layout` implementation would return its own modal/chrome compositing (which genuinely does vary per layout — border position, overlay centering) but never own the "did I remember to splice in the images" responsibility. This turns the Miller-gap class of bug into something the type system enforces rather than something code review has to catch.

**Bounded cache from day one.** An LRU keyed the same way (`slot × URL × cols × protocol`), capped by total encoded-payload bytes, is barely more code than an unbounded map — `container/list` plus a map, evict-oldest on insert past the cap. There's no real cost to writing this the first time instead of deferring it; the "spike" framing in the current comments buys very little given how small the actual diff would be.

**Real cell size for all three protocols from day one**, since the ioctl already has to be written for Sixel regardless — `pxPerCol` becomes the fallback for all three uniformly (used when `TerminalCellPixelSize` reports `ok=false`), not the only path for two of them and a fallback for the third.

**`context.CancelFunc` stored next to each in-flight marker**, cancelled on scroll-away/toggle-off/quit. This is one extra field in the tracking map and one extra call at three call sites — cheap enough that there's no reason to defer it to "later," unlike genuinely complex work like GIF disposal-method handling.

**Kitty payload PNG-compressed (`f=100`) instead of raw RGBA (`f=32`) from the start.** Given the cache stores the post-encode string, this is a direct multiplier on the single largest memory-allocation surface in the feature, for the same amount of encoder code (Go's `image/png` is already a dependency here via `iterm2.go`).

**`syncInlineImages` gated on message type**, not run unconditionally. A short type-switch on the incoming `tea.Msg` (scroll, resize, selection-change, screen-switch, the toggle setting itself) before doing any diff work — a few lines, and it establishes the right default habit for any future per-frame derived-state sync in this codebase rather than "always recompute, rely on it being cheap."

**Don't build a second `Layout` implementation before there's a way to select it.** If Miller-layout support isn't finished enough to expose in settings, I would not have written fullscreen-modal or inline-image logic into `layout_miller.go` in this branch at all — either finish wiring `layoutFromName` and the settings item in the same change, or leave the file untouched until that's done. Partial logic in unreachable code is a liability with no offsetting benefit; it looks tested (there are unit tests that instantiate it directly) while being unverified in any way that matters to a real user.

**A handful of golden-output `View()` tests per protocol, written alongside the feature, not after.** Fixed terminal size, fixed post content, assert the literal expected escape sequence and its cursor position. This is the one test category proven, by this branch's own history, to catch the bugs that actually shipped (the Miller gap, and arguably the two Ghostty lifecycle bugs — a golden test that pinned the exact sequence of delete/create escapes across a scripted sequence of `Update` calls would have caught the sticky-cleanup-flag and stale-cache-entry bugs without needing real hardware).

None of this changes the core Elm-loop-plus-messages architecture — that part was the right call the first time and I'd keep it unchanged.

---

## What is already right

- Single-threaded state mutation via Bubble Tea messages throughout; no mutexes in this feature, none needed, because no goroutine ever touches `App` fields directly.
- `imgview/fetch.go` has real, tested guards: `maxImageBytes = 10<<20` matching the API/RTDB clients' cap (`fetch.go:18-20`), a declared-dimension check via `image.DecodeConfig` before the full decode so a small file claiming huge dimensions can't drive allocation (`fetch.go:22-24,72-82`), and both `maxGIFFrames`/`maxGIFTotalPixels` guards for the animated path.
- The two post-merge commits (`70925fd`, `3ae054c`) are genuine root-cause fixes — stable placement ids, an accumulating (not countdown-based) delete-resend set with the countdown alternative explicitly tried and reverted with its rationale documented (`app.go:222-236`), a cancelable `ProbeSixel` read — not band-aids over the symptoms.
- The code comments in this branch are unusually good at explaining *why*, not just *what* — e.g. the `q=2` suppress-response flag's rationale (`kitty.go:53-56`, a real bug: unsuppressed Kitty ACKs were read as keystrokes and closed the modal instantly), the anonymous-vs-named placement-id sentinel choice (`kitty.go:12-22`), and the fall-through-not-return requirement in the Kitty cleanup branch (`layout_tabs.go:95-100`). This made the codebase substantially easier to review accurately.
- `EncodeKitty`/`EncodeSixel`/`EncodeITerm2` all correctly refuse to upscale beyond an image's natural pixel size, tested explicitly (`TestEncodeSixel_NeverUpscales`).
- Feed capping to the first eligible image (`render.go:106-111`) is a deliberate, documented tradeoff for its truncated-body card layout, not an unexplained inconsistency — PostDetail correctly renders every eligible image since it has no such truncation.

---

## Prioritized recommendations

1. Resolve the `MillerLayout` dead-code situation one way or the other before adding more logic to it — either finish `layoutFromName` and expose a settings item, or delete the file and its tests. This is the one finding with real ambiguity about ownership (pre-existing bug vs. this feature's scope), so it needs a decision, not just a patch.
2. Add golden-output `View()` tests per layout/protocol — the test category that would have caught both the Miller gap and, plausibly, the Ghostty lifecycle bugs without needing real hardware.
3. Bound `inlineImageCache` and compress the Kitty payload (`f=100`) before this feature loses its "experimental" label.
4. Extend the fullscreen modal's fetch-staleness guard (`imageFetchGen`) to the inline path, or add `context.CancelFunc`-based cancellation.
5. Thread real cell-pixel size into `EncodeKitty`/`EncodeITerm2` the same way `EncodeSixel` already receives it.
6. Cache-and-cooldown fetch failures instead of retrying a dead URL on every `Update` it stays visible for.
7. Everything else here (unconditional per-`Update` sync, the fixed Sixel color count) is fine to leave as-is — low blast radius, already understood tradeoffs.
