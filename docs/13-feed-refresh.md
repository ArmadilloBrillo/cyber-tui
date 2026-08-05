# 13 — Feed refresh (pull-to-top)

## Purpose
Allow users to fetch the latest posts without restarting the app. Pressing `Up` / `k` when already at the top of the feed (first post selected) triggers a refresh instead of doing nothing.

## Behaviour
- When `selectedIndex == 0` and the user presses `Up` / `k`, a refresh is initiated provided no load is already in flight (`!loading && !refreshing`).
- During the fetch, a `"  fetching new posts..."` line is prepended above all existing posts, pushing them down by one line. This gives immediate visual feedback without clearing the feed.
- When the response arrives the feed is replaced with the fresh result (`SetPosts`) and the viewport scrolls to the top.
- Pressing `Up` again while a refresh is already in flight is a no-op.

## Message flow
```
FeedModel (up at index 0)
  → emits RefreshFeedMsg{}
App.Update
  → calls loadFeedCmd() (same as initial load, no cursor)
  → result arrives as feedLoadedMsg
  → calls feed.SetPosts(posts, cursor)
    → clears refreshing flag, scrolls to top
```

## Key files
| File | Symbol | Role |
|---|---|---|
| `internal/ui/screens/feed.go` | `RefreshFeedMsg` | Message emitted when refresh is triggered |
| `internal/ui/screens/feed.go` | `FeedModel.refreshing bool` | Guards against double-fetch; drives the status prefix |
| `internal/ui/screens/feed.go` | `buildContent()` | Prepends the fetching line and shifts post offsets by 1 when `refreshing` |
| `internal/ui/screens/feed.go` | `SetPosts()` | Clears `refreshing` on completion |
| `internal/ui/app.go` | `case screens.RefreshFeedMsg` | Calls `loadFeedCmd()` |

## See also
[`docs/39-feed-background-poll.md`](./39-feed-background-poll.md) — a passive 15s background poll that detects new posts and stages them (tab badge + banner) without disturbing the viewport. Pressing `Up`/`k` at the top merges staged posts locally when any are pending, instead of the network round-trip described above.
