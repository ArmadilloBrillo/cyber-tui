# 39 — Feed background poll ("load new entries")

## Purpose
Passively detect new posts on the server without disturbing the user's scroll
position. New posts are never spliced into the viewport automatically — a
banner and a tab badge tell the user entries are waiting, and the existing
pull-to-top gesture (`docs/13-feed-refresh.md`) brings them into view.

## Behaviour
- Every 15s (after login, running globally regardless of the active tab —
  same shape as the notifications unread-count poll), the app fetches the
  newest page of the feed and diffs it against the currently loaded posts.
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
App (after login)
  → schedules feedPollTickMsg every 15s (scheduleFeedPollCmd)
App.Update (feedPollTickMsg)
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
| `internal/ui/app.go` | `scheduleFeedPollCmd()`, `fetchFeedPeekCmd()` | 15s self-rescheduling ticker (mirrors `schedulePollCmd`/`pollUnreadTickMsg`) |
| `internal/ui/app.go` | `afterLoginCmd()` | Starts the ticker once, alongside the notifications poll |
| `internal/ui/layout_tabs.go` | `renderTabBar()` | `(N)` badge for `screenFeed` |
| `internal/ui/layout_miller.go` | `renderNav()` | `●N` badge for `screenFeed` |
