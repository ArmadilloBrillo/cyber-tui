# 56 - Compose CPU and memory performance

## Overview

A user on a Raspberry Pi 2B reported heavy CPU use whenever the post or reply
editor was open. Profiling showed the editor itself was not the main cost. The
`App` struct is about 222 KB and was copied by value roughly 83 times for every
message the program handled (keystrokes, cursor-blink messages, animation
frames, polls, chat events). Most of the CPU went to the Go garbage collector
scanning those copies.

The fixes shipped in PR #193 (five `perf:` commits plus one `fix:` commit). PR
#192 (Miller layout removal) was merged right after and dropped the small
`MillerLayout` wrapper that #193 had added for the new `View` signature. This
document records what was measured, why it worked, the rules that keep it fast,
and the optimizations that were identified but not done.

## Measured results

All numbers are Go benchmarks on an x86 desktop (Ryzen 8845HS), not a
Raspberry Pi. Treat them as ratios. The benchmark files were throwaway and were
not committed; "How to re-measure" below describes their shape.

| Benchmark (feed composer open) | Before | After |
|---|---|---|
| No-op message through `Update` | 4.2 ms, 18.6 MB, 82 allocs | 0.006 ms, 48 B, 1 alloc |
| `App.View()` | 0.68 to 0.73 ms, 453 KB | 0.53 to 0.54 ms, 220 KB |
| Keystroke, `Update` + `View`, title field focused | 5.6 ms, 19.4 MB, 1320 allocs | not re-run at the end |
| Keystroke with follow-up messages, body focused | not measured at the start | 0.88 to 0.90 ms, 0.52 MB, 1525 allocs |

The body-focused benchmark was built midway through the work, so its true
starting point was never measured. Step by step:

| Change | No-op message | Keystroke | `View` |
|---|---|---|---|
| Original | 4.2 ms, 18.6 MB | 5.6 ms, 19.4 MB (title field) | 0.70 ms, 453 KB |
| Handler chain on `*App` | 0.62 ms, 2.5 MB | 1.5 ms, 3.3 MB | unchanged |
| Layout delegation and inline-image sync on `*App` | 0.22 ms, 0.92 MB | 1.2 ms, 1.65 MB | unchanged |
| Static cursor (body focused, with follow-ups) | unchanged | 2.4 ms, 3.08 MB, 1 extra message, then 1.4 ms, 1.68 MB, 0 extra | unchanged |
| Tab layout render chain on `*App` | unchanged | 1.41 ms, 1.68 MB, then 1.23 ms, 1.45 MB | 0.71 ms, 453 KB, then 0.57 ms, 220 KB |
| `ui.Root` pointer model and two helper receivers | 0.2 ms, 0.92 MB, then 0.006 ms, 48 B | 1.23 ms, 1.45 MB, then 0.89 ms, 0.52 MB | 0.57 ms, then 0.53 ms |

The keystroke rows come from two different benchmarks, so they do not chain
into a single number. The logo-animation pause was not measured.

## What drove the improvement

1. **The handler chain copied `App` at every step.** `updateInner` and the
   `handleX` methods took and returned `App` by value. `updateInner` alone was
   64% of allocations. They now take `*App` and mutate in place.
2. **Hidden copies in helpers.** `canInlineImages` and `ditheringEnabled` were
   value-receiver helpers, and each call heap-allocated a full copy. The layout
   delegation and inline-image sync paths also copied `App`.
3. **Bubble Tea copied `App` through its model interface.** `ui.Root` holds
   `*App` and drives an in-place `step`, so nothing is copied or re-boxed per
   message. The value `App.Update` and `App.View` remain as thin wrappers used by
   tests.
4. **A wasted message on every keystroke.** `View` re-focused the textarea,
   which cancelled the blink command that `Update` had just returned. Each
   cursor-moving keystroke therefore produced a `cursor.blinkCanceled` message,
   which is a second full `Update` and `View`. The cursor never actually blinked
   (the re-focus reset its blink state each render), so both editors now use
   `cursor.CursorStatic`. Measured: 1.0 extra messages per keystroke down to 0,
   about 1.7x faster and 1.8x less memory.
5. **The tab bar copied `App` about 24 times per `View`** (about 8% of profile
   samples). The tab layout render chain now takes `*App`.
6. **Logo animation.** The scramble no longer starts while a post or reply
   editor is open. Its idle timer is re-armed, so it resumes afterwards.

## Rules to keep it fast

- Pass `*App` in anything on the per-message path. Do not add value-receiver
  methods on `App` that run per message; each call can copy 222 KB.
- A `tea.Cmd` closure runs on another goroutine. Copy any mutable `App` field
  it needs into a local before returning the command, and never write to `App`
  from inside the closure. `saveProfileCmd` wrote `a.currentUser` from its
  goroutine after the pointer change and had to be fixed to work on a snapshot
  (`TestSaveProfileCmd_DoesNotMutateLiveApp`).
- Handlers now edit `App` in place, so a handler must not mutate state on a
  path that returns "not handled".
- When adding a handler, use the `(a *App)` receiver and the
  `(*App, tea.Cmd, bool)` signature of the existing `handleX` methods.

## Remaining ideas (not done)

Ranked by likely payoff. Percentages are shares of `App.View` time from a CPU
profile with the feed composer open and an empty body; times are that share of
about 0.56 ms and are estimates. About half of `View` is `ansi.StringWidth`,
which is lipgloss re-measuring lines that were already rendered.

| # | Idea | Where | Evidence | Risk |
|---|---|---|---|---|
| 1 | Replace the compose-panel box render and the outer full-screen `Height/MaxHeight` render | `compose.go` (`boxStyle.Render`), `layout_tabs.go` (`TabsLayout.View`) | About 14% of `View` each, roughly 0.08 to 0.1 ms | Medium. lipgloss also pads lines to the widest line, so the output must stay byte-identical; guard with a golden comparison test. |
| 2 | Cache the tab bar and status bar strings | `layout_tabs.go` (`renderTabBar`, `renderStatusBar`) | About 20% of `View` together | Medium. Needs invalidation on width, theme, active screen, unread counts and the notification. |
| 3 | Textarea: render only the visible rows and make the update path incremental | bubbles `textarea` (would need a fork, about 1470 lines) | `View` iterates the whole document but shows 3 to 8 rows. Component benchmark: `View` 0.14 ms at 10 lines, 0.74 ms at 99 lines; keystroke plus `View` 0.31 ms and 1.42 ms | Higher. Owning a fork; behaviour must stay identical (cursor, scroll, wrap, wide characters). Saves nothing for short posts. |
| 4 | Skip the feed poll's whole-feed re-render while an editor is open | `feed.go` (`SetPendingNew` calls `refreshContent` every 60 s) | From code reading, not measured | Low |
| 5 | Reduce the chat and DM background message chains | `cmail.go` (500 ms typing tick), `api/client.go` (5 s presence snapshots) | From code reading, not measured. Only applies if a room or conversation is left open | Low to medium |
| 6 | Tune Go's garbage collector on the Pi (`GOGC`, `GOMEMLIMIT`) | Startup or docs | GC dominated the profile before the copy fixes; not re-measured since | Trades memory for CPU, which may not suit a Pi 2B |

Also left alone: the last per-message allocation cost is `View` itself (about
220 KB of rendered strings per call), which is genuine work rather than copying.

## Decision rule

There is no Raspberry Pi measurement yet. Before doing any of the above, get a
CPU profile from the reporting user while typing on a build that includes the
fixes. Do the textarea work (idea 3) only if they write long posts or the profile
shows the textarea dominating. For typical short replies it saves almost
nothing.

## How to re-measure

The benchmarks lived in the `internal/ui` package. Their shape:

- Build an `App` from `loggedInApp()`, send a `tea.WindowSizeMsg` of 120x40,
  press `n` on the feed to open the composer, and press Tab twice to focus the
  body.
- Wrap it with `NewRoot` and run `Update` then `View` for a keystroke, run any
  command the update returned (with a short timeout, timer stopped while
  waiting), and feed each resulting message back through `Update` and `View`.
- Report `-benchmem` at a fixed iteration count (for example
  `-benchtime 500x`). The body grows with every keystroke, so results at
  different iteration counts are not comparable.
- For a no-op message benchmark, send a message type no handler claims. For
  `View`, call `Root.View` repeatedly.
- For profiles, use `-cpuprofile` on its own. Combining it with a per-allocation
  `-memprofilerate 1` distorts the CPU samples badly.
