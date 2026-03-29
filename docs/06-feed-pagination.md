# 06 — Feed Pagination

Branch: `feature/http-client` (landed alongside HTTP client)

## What was built

Cursor-based infinite-scroll pagination for the feed. The first page loads on login;
subsequent pages load automatically when the user scrolls to the bottom of the viewport.

## Files changed

| File | Change |
|---|---|
| `internal/api/interface.go` | `GetFeed` now returns `([]model.Post, string, error)` — second value is next-page cursor |
| `internal/api/client.go` | Returns `env.Cursor` from `GetFeed`; removed the deferred-pagination NOTE |
| `internal/api/mock.go` | Matches new signature; always returns empty cursor (no pagination in mock) |
| `internal/api/client_test.go` | Updated call sites; 2 new tests for cursor return and empty-cursor last page |
| `internal/api/mock_test.go` | Updated call sites; 1 new test asserting mock returns empty cursor |
| `internal/ui/screens/feed.go` | New fields, `AppendPosts`, `LoadMoreFeedMsg`, footer indicator |
| `internal/ui/app.go` | `feedLoadedMsg` carries cursor; new `feedPageMsg` and `loadFeedPageCmd` |

## Design decisions

### Auto-load on scroll
`FeedModel.Update` checks `viewport.AtBottom()` after every update. When the viewport
hits the bottom and a next-page cursor is available, it sets `loading = true` and emits
`LoadMoreFeedMsg{Cursor}`. The App catches this and calls `loadFeedPageCmd`. The
`loading` flag prevents duplicate requests while a fetch is in flight.

### Append, don't replace
`AppendPosts` appends to the existing slice and re-renders without calling `GotoTop`,
preserving the user's scroll position. `SetPosts` (first page / tab refresh) replaces
and resets to top.

### End-of-feed detection
When the API returns an empty cursor, `exhausted` is set to `true`. The footer shows
`— end of feed —` and no further `LoadMoreFeedMsg` events are emitted.

### Error recovery
`SetError` clears `loading`, so if a page fetch fails and the user scrolls to the bottom
again, the next-page load is retried automatically.

### No caching
Every tab switch re-fetches page 1. Pagination state (cursor, accumulated posts) is
reset on each fresh load.

## UX

| State | Footer text |
|---|---|
| Posts loaded, more available | *(no footer — scroll to trigger)* |
| Fetching next page | `loading more…` |
| No more pages | `— end of feed —` |
| No posts at all | `no posts yet` |
