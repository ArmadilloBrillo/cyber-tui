# 07 — Post Detail View

Branch: `feature/post-detail`

## What was built

A full-screen post detail view reachable by pressing Enter on any feed post.
Shows the complete untruncated post and its replies. Esc returns to the feed.
As a side-effect, long posts in the feed are truncated to 4 wrapped lines with
a `▼ N more lines` hint so the feed stays scannable.

## Files changed

| File | Change |
|---|---|
| `internal/api/interface.go` | Added `GetPostReplies(postID string) ([]model.Reply, error)` |
| `internal/api/client.go` | Implemented `GetPostReplies` (GET /v1/posts/:id/replies?limit=20); added `wireReply` wire type and `wireReplyToModel` helper |
| `internal/api/mock.go` | Stub `GetPostReplies` returning 2 fake replies |
| `internal/api/client_test.go` | 2 new tests: `TestHTTPGetPostReplies_ParsesReplies`, `TestHTTPGetPostReplies_UsesPostID` |
| `internal/api/mock_test.go` | 2 new tests: `TestMockGetPostReplies_ReturnsReplies`, `TestMockGetPostReplies_AnyPostID` |
| `internal/ui/screens/feed.go` | Enter key emits `ShowPostMsg`; body truncated to 4 lines with `▼ N more lines` hint |
| `internal/ui/screens/postdetail.go` | New screen: `PostDetailModel`, `BackToFeedMsg` |
| `internal/ui/app.go` | `screenPostDetail` constant; `postDetail` field; `ShowPostMsg`/`BackToFeedMsg`/`repliesLoadedMsg` handlers; `loadRepliesCmd`; WindowSizeMsg broadcast; arrow-key guard |

## Design decisions

### Feed truncation
`renderPost` splits the Lip Gloss-wrapped string on `\n` and takes the first
`feedMaxBodyLines` (4) lines when the post is longer. A `▼ N more lines` hint
rendered in the Subtle style is appended so users know content is hidden.
Truncation only applies when `innerWidth > 0` (i.e. after the terminal size is known).

### Detail view as contextual overlay
`screenPostDetail` is not added to `menuTabs`. It is entered via Enter on the
feed and exited via Esc — no number shortcut, no tab-bar entry. Arrow-key tab
navigation is guarded against `screenPostDetail` so left/right scroll the
viewport instead of switching tabs.

### Replies loading
`SetPost` marks `loading = true` and clears replies immediately, so entering the
detail view for a new post always shows `loading replies…` until the API
responds. `loadRepliesCmd` fetches `GET /v1/posts/:id/replies?limit=20`.
On success, `repliesLoadedMsg` calls `SetReplies` which sets `loading = false`
and re-renders the viewport.

### Error handling
`errMsg` routing in `app.go` forwards errors to the active screen. If the
replies fetch fails while `screenPostDetail` is active, `SetError` is called
and the view shows the error string.

### Nested post navigation (post-to-post links)
A post's content can link to another post on cyberspace.online (`routeURL` in
`app.go` recognizes `/{user}/{slug}` and `/{user}/blog/{slug}`). Opening such
a link while already viewing a post pushes the currently open
`PostDetailModel` onto `App.postDetailStack` before loading the linked post,
instead of overwriting `postDetailReturn` (which would otherwise point back
at `screenPostDetail` itself and leave Esc unable to do anything — see
`docs/00-api-backlog.md`/git history for the bug this fixed). Esc pops the
stack — restoring the previous post, its already-loaded replies, and scroll
position without refetching — until the stack is empty, then falls back to
`postDetailReturn` (the original tab: Feed, Bookmarks, Profile, etc.) as
before. Re-pressing the origin tab's own key while nested (the existing
"escape hatch" in `activateScreen`) closes out completely and clears the
stack, same as a fresh single-post visit always did.

## UX

| Key | Action |
|---|---|
| `Enter` (feed) | Open post detail view |
| `Esc` (detail) | Return to feed |
| `↑`/`↓`/`j`/`k`/`PgUp`/`PgDn` | Scroll within detail view |

| State | Content shown |
|---|---|
| Loading replies | Full post + `loading replies…` |
| Replies loaded | Full post + reply count header + reply boxes |
| No replies | Full post + `no replies yet` |
| Error | Error message string |
