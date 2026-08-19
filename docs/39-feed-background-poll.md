# 39 — Feed background poll ("load new entries")

## Purpose
Passively detect new posts on the server without disturbing the user's scroll
position. New posts are never spliced into the viewport automatically — a
banner and a tab badge tell the user entries are waiting, and the existing
pull-to-top gesture (`docs/13-feed-refresh.md`) brings them into view.

## Behaviour
- Every 60s (after login, running globally regardless of the active tab —
  same cadence and shape as the notifications unread-count poll), the app
  fetches the newest page of the feed and diffs it against the currently
  loaded posts. (60s since the 2026-08-19 battery audit — see "Battery"
  below; was 15s before.)
- New (not-yet-seen) post IDs are staged on `FeedModel` as `pendingNew` —
  not merged into `posts`, so the viewport and scroll position are
  untouched.
- While entries are pending, a banner line (`"  ↑ load N new entries ↑"`)
  is prepended above the first post, and the Feed tab shows a `(N)` badge
  (`layout_tabs.go`) / `●N` badge (`layout_miller.go`), mirroring the
  notifications badge.
- Pressing `Up`/`k` at the top of the feed while entries are pending plays
  the same brief `"  fetching new posts..."` transition as a manual refresh
  (~200ms, via `tea.Tick` — no network round-trip, the posts are already
  fetched) before merging them in locally (prepend + clear pending + scroll
  to top).
- The poll is skipped while a manual refresh is in flight (`refreshing`) or
  before the feed has loaded once, and reschedules itself either way.

## Battery: manual-refresh-only toggle
This poll was the single largest network offender found in the
2026-08-19 battery audit (`docs/00-battery-audit.md`, item #5) — a full
20-post page fetched every 15s, unconditionally, for the whole session. A
hard "only poll while Feed is the active tab" gate was considered and
rejected: it would silently defeat the cross-tab `(N)` badge above, which
this doc's own "Behaviour" section describes as deliberately running
globally. The fix taken instead:
- The interval was lengthened from 15s to 60s (`feedPollInterval` in
  `app.go`) — matches the notifications poll's cadence, a straight 4x cut
  in radio wakes, zero change to the cross-tab badge behavior.
- A new **Settings → feed → auto-refresh (background poll)** toggle
  (`config.Config.FeedManualRefreshOnly`, default off = auto-poll on) lets a
  user disable the poll entirely. With it on, `scheduleFeedPollCmd` is never
  started at login and `feedPollTickMsg` self-terminates without
  rescheduling if the poll was already running when the user turns it on;
  flipping it back off mid-session restarts the chain immediately from the
  settings-save handler (nothing else would revive it otherwise). The
  manual `Up`-at-top refresh gesture works identically either way — this
  toggle only affects the automatic background check.

## Known limitation
The poll reuses the same `GetFeed("")` endpoint as the initial load, which
returns only the newest 20 posts per request. Walking multiple
cursor-paginated pages per poll to get an exact count was considered and
rejected — it would multiply the request count on every 15s tick (not just
the rare case) and add latency on slow connections for a number in a
banner. Instead: if the single fetched page comes back entirely new posts
(the previously-known top post wasn't found within it), the real count
could be higher than what one page can show, so the count is displayed as
a floor — `"20+"` — rather than asserting a specific, likely-wrong number.
The count self-corrects on the next 15s tick as more of the backlog surfaces.

## Message flow
```
App (after login, only if FeedManualRefreshOnly is off)
  → schedules feedPollTickMsg every 60s (scheduleFeedPollCmd)
App.Update (feedPollTickMsg)
  → skips (no reschedule) if FeedManualRefreshOnly is now on
  → skips if feed not loaded yet or a manual refresh is in flight
  → else: fetchFeedPeekCmd() (GetFeed("")) + reschedules itself
  → result arrives as feedPeekMsg{posts}
  → calls feed.SetPendingNew(posts) — dedupes against feed.posts, stages the rest

FeedModel (up at index 0, pendingNew non-empty)
  → sets refreshing (banner reuses the manual-refresh "fetching new posts..." line)
  → returns tea.Tick(feedMergeAnimDelay) → mergePendingTickMsg
  → MergePendingNew(): prepends staged posts, clears pendingNew, scrolls to top
```

## Key files
| File | Symbol | Role |
|---|---|---|
| `internal/ui/screens/feed.go` | `FeedModel.pendingNew []model.Post` | Staged new posts, not yet in the viewport |
| `internal/ui/screens/feed.go` | `FeedModel.pendingCapped bool` | True when the peek page was entirely new posts — count shown as "N+" |
| `internal/ui/screens/feed.go` | `SetPendingNew()` | Dedupes incoming posts against `posts`, stages the rest, sets `pendingCapped` |
| `internal/ui/screens/feed.go` | `PendingNewCount()` | Getter used by both tab-bar layouts for the badge |
| `internal/ui/screens/feed.go` | `MergePendingNew()` | Prepends staged posts into `posts`, clears pending, scrolls to top |
| `internal/ui/screens/feed.go` | `mergePendingTickMsg`, `feedMergeAnimDelay` | ~200ms delay so the local merge shows the same transition as a real refresh |
| `internal/ui/screens/feed.go` | `buildContent()` | Renders the "load N (or N+) new entries" banner when `pendingNew` is non-empty |
| `internal/ui/app.go` | `feedPollTickMsg`, `feedPeekMsg` | Poll tick and peek-result messages |
| `internal/ui/app.go` | `feedPollInterval`, `scheduleFeedPollCmd()`, `fetchFeedPeekCmd()` | 60s self-rescheduling ticker (mirrors `schedulePollCmd`/`pollUnreadTickMsg`) |
| `internal/ui/app.go` | `afterLoginCmd()` | Starts the ticker once, alongside the notifications poll — skipped if `FeedManualRefreshOnly` is on |
| `internal/ui/app.go` | `App.feedManualRefreshOnly` | Local config value gating the ticker — see "Battery" above |
| `internal/config/session.go` | `Config.FeedManualRefreshOnly` | Persisted toggle value |
| `internal/ui/screens/settings.go` | `"feed"` settings group | "auto-refresh (background poll)" toggle row |
| `internal/ui/layout_tabs.go` | `renderTabBar()` | `(N)` badge for `screenFeed` |
| `internal/ui/layout_miller.go` | `renderNav()` | `●N` badge for `screenFeed` |
