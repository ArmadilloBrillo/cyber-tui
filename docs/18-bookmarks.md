# 18 — Bookmarks

## Overview

Bookmarks let users save posts (and replies) for later. This feature adds a top-level **Bookmarks** screen (tab 5), a `b` key binding in Feed and Post Detail, and a delete action within the Bookmarks screen.

## API Endpoints Used

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/v1/bookmarks?limit=20&cursor=<id>` | Paginated bookmark list |
| `POST` | `/v1/bookmarks` | Create bookmark |
| `DELETE` | `/v1/bookmarks/:id` | Remove bookmark |

Rate limits: GET 20/min · POST 5/min 50/day.

## Changes

### Model (`internal/model/types.go`)

Added `Bookmark` struct:

```go
type Bookmark struct {
    ID        string
    Type      string // "post" or "reply"
    PostID    string
    ReplyID   string
    Post      *Post  // embedded when Type == "post"
    Reply     *Reply // embedded when Type == "reply"
    CreatedAt time.Time
}
```

### API Interface (`internal/api/interface.go`)

Three new methods:

```go
GetBookmarks(cursor string) ([]model.Bookmark, string, error)
CreateBookmark(postID, replyID string) (string, error)
DeleteBookmark(id string) error
```

### HTTP Client (`internal/api/client.go`)

- `GetBookmarks` — `GET /v1/bookmarks?limit=20&cursor=…`; decodes embedded post/reply via `wireBookmark`
- `CreateBookmark` — `POST /v1/bookmarks` with `{ "postId", "type": "post" }` or `{ "replyId", "type": "reply" }`
- `DeleteBookmark` — `DELETE /v1/bookmarks/:id`

### Screen (`internal/ui/screens/bookmarks.go`)

`BookmarksModel` follows the same viewport + pagination pattern as `NotificationsModel`.

Key bindings:

| Key | Action |
|-----|--------|
| `j` / `↓` | Navigate down; loads next page at bottom |
| `k` / `↑` | Navigate up; refreshes at top |
| `enter` | Open bookmarked post in Post Detail |
| `d` | Delete selected bookmark (optimistic) |

Methods: `SetBookmarks`, `AppendBookmarks`, `MarkDeleted`, `SetStatusMsg`, `SetError`.

### Shared Messages (`internal/ui/screens/shared.go`)

- `BookmarkedMsg` — confirmation sent after a successful `CreateBookmark`

### Screen Messages (`internal/ui/screens/bookmarks.go`)

- `BookmarkPostMsg { PostID }` — emitted from Feed/PostDetail on `b`
- `LoadMoreBookmarksMsg { Cursor }` — emitted when scrolling to bottom
- `RefreshBookmarksMsg` — emitted when scrolling past top
- `DeleteBookmarkMsg { BookmarkID }` — emitted on `d`

### App (`internal/ui/app.go`)

- `screenBookmarks` added to the screen enum
- `bookmarks BookmarksModel` field on `App`
- Tab bar: `{"bookmarks", screenBookmarks}` — accessible via `5` and left/right arrow cycling
- `handleBookmarks` handler wires all bookmark messages
- `loadBookmarksCmd`, `loadBookmarksPageCmd`, `createBookmarkCmd`, `deleteBookmarkCmd` Cmd functions
- `BookmarkPostMsg` from Feed/PostDetail triggers `createBookmarkCmd` and shows transient "bookmarked" status

### Feed / Post Detail (`internal/ui/screens/feed.go`, `postdetail.go`)

`b` key emits `BookmarkPostMsg{PostID: selectedPost.ID}`.

## UX Flow

1. Press `b` on a post in Feed or Post Detail → "bookmarked" status appears in Bookmarks screen header
2. Press `5` (or `→` to cycle) to open Bookmarks screen
3. Navigate with `j`/`k`; press `enter` to re-open the post
4. Press `d` to remove the selected bookmark (immediate optimistic delete, confirmed by API)
5. Scroll to bottom auto-loads the next page; scroll past top refreshes

## Tests

- `internal/api/client_test.go` — `TestHTTPGetBookmarks_*`, `TestHTTPCreateBookmark_*`, `TestHTTPDeleteBookmark_*`
- `internal/api/mock_test.go` — `TestMockGetBookmarks_*`, `TestMockCreateBookmark_*`, `TestMockDeleteBookmark_*`
- `internal/ui/app_test.go` — updated tab-cycle tests for 5-tab layout
