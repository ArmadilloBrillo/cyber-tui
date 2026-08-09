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

## 1. Decide MillerLayout's fate (blocking, needs a decision)

**Files:** `internal/ui/app.go:431-433` (`layoutFromName`, discards its argument, always
returns `TabsLayout{}`), `internal/ui/layout_miller.go` (whole file).

`layoutFromName` predates this branch and makes `MillerLayout` unreachable by any user —
there's also no settings-screen item to pick a layout at all. This feature nonetheless
added real logic to `layout_miller.go` (a Kitty-cleanup fix matching the one applied to
`layout_tabs.go`), and never wired inline-image compositing into it either. This is a
product decision, not a code decision:

- **(a) Delete** `layout_miller.go` and its dedicated tests (`layout_test.go:66,337,385`,
  `app_test.go:317`) — it's dead code with no user-facing coverage, and every line added
  to it is maintenance cost with no corresponding benefit.
- **(b) Finish it** — fix `layoutFromName` to branch on its argument, add the missing
  settings-screen item (`internal/ui/screens/settings.go:140-172` currently has no
  layout picker even though `SettingsModel.layoutName` is tracked and round-tripped),
  and only then treat it as a real second layout worth keeping in sync with `TabsLayout`.

Resolve this before touching item 2 — it determines whether that item is "fix" or "n/a."

---

## 2. Fix (or remove) the inline-image compositing gap

**Files:** `internal/ui/layout_tabs.go:112-149` (`injectInlineImages`, only call site
today) vs `internal/ui/layout_miller.go:147-166`.

- **If Miller is kept** (1b): hoist `injectInlineImages` and the modal-compositing block
  to a single call site above both layouts — after `a.layout.View(a)` returns, before
  the string is returned to the terminal — so a `Layout` implementation cannot forget to
  call it. This is the shared-pipeline fix both reviews recommend over duplicating the
  call into `layout_miller.go`.
- **If Miller is deleted** (1a): this finding disappears with the file — no separate fix
  needed.

---

## 3. Bound `inlineImageCache` before removing the "experimental" label

**Files:** `internal/ui/app.go:184-197` (`inlineImageCache`, keyed by slot × URL × column
width × protocol, never evicted), `app.go:198-243` (`kittyPlacementIDs`,
`kittyNextPlacementID`, `pendingKittyDeletes` — coupled to the same cut by design).

Add an LRU bounded by encoded-payload byte size — `container/list` plus a map,
evict-oldest on insert past the cap. Any eviction scheme needs to invalidate the
matching `kittyPlacementIDs` entry when its cache entry is evicted, since a re-issued id
without a matching cache invalidation would desync the two.

---

## 4. Compress the Kitty payload

**File:** `internal/ui/imgview/kitty.go:28-63` (`EncodeKitty`, currently emits raw
32-bit RGBA via `f=32`, no compression).

Switch to PNG (`f=100`) the way `EncodeITerm2` already does (`imgview/iterm2.go:21-25`,
`image/png` is already a dependency). This directly shrinks the same cache memory
problem as item 3, for the same amount of encoder code, and is cheap enough to bundle
into the same change.

---

## 5. Stop silently caching fetch failures as retryable blanks

**File:** `internal/ui/app.go:2607-2621` (`handleInlineImageFetched`).

On `err != nil`, the in-flight marker is cleared but nothing is written to
`inlineImageCache`, so `syncInlineImages` retries the same dead URL on every subsequent
`Update` the slot stays visible for — no backoff. Cache a distinct "failed" sentinel
with a short cooldown so a permanently-broken image URL doesn't get hammered on every
keystroke/tick while visible.

---

## 6. Deferred — no action needed now

Both reviews independently judged these low-urgency or fine-as-is; revisit only if
circumstances change (noted trigger in parentheses):

- Uncancelled inline-fetch goroutines (`app.go:2572-2600`) — bounded by a 20s timeout
  today; extend the fullscreen modal's existing `imageFetchGen` staleness-guard pattern
  to the inline path if this gets reused for more expensive fetches later.
- Kitty/iTerm2 not threading real terminal cell size the way `EncodeSixel` already does
  (`imgview/kitty.go:48`, `imgview/iterm2.go:26` vs `imgview/sixel.go:23-31`) — low
  severity since both protocols scale-to-fit terminal-side; worth fixing opportunistically
  since the ioctl is already being called for Sixel in the same caller, but not urgent.
- `syncInlineImages` running unconditionally on every `Update` (`app.go:488-495`) — cheap
  no-op today; gate on message type if a future profiling pass shows `Update` latency
  pressure, or before this exact pattern gets reused elsewhere.
- Sixel hardcoding 256 colors (`imgview/sixel.go:44`) — true for essentially every
  Sixel-capable terminal in practice; leave as a documented assumption.
- Golden-output `View()` tests per layout/protocol — the test category that would have
  caught both the MillerLayout gap and the two Ghostty lifecycle bugs; worth adding
  alongside item 2 specifically (asserting the composited escape sequence appears in
  `View()`'s output), not as a standalone backlog item.

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
