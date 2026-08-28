# Feature 28 — Extended Posts

## Overview

Extends the `Post` model and compose flow to match the full v0.4 API capability: optional post title, slug, public/NSFW flags, and guild thread fields.

## Model Changes

`model.Post` (`internal/model/types.go`) gained five new fields:

| Field | Type | Source |
|---|---|---|
| `Title` | `string` | Optional post title set by the author |
| `Slug` | `string` | Per-author URL slug (auto-generated if not set) |
| `GuildID` | `string` | Populated on guild-forum threads |
| `GuildSlug` | `string` | Human-readable guild slug for display |
| `IsGuildThread` | `bool` | True when the post belongs to a guild forum |

## API Layer Changes

`CreatePost` signature (`internal/api/interface.go`):
```go
CreatePost(content, title string, topics []string, isPublic, isNSFW bool) (model.Post, error)
```

- `title` — sent as `title` in the request body; omitted when empty (`omitempty`)
- `isPublic` / `isNSFW` — were wired to the request body before but always false; now threaded through from the UI
- Response parsing captures `slug` and `title` from `{ postId, slug, title }` response

`wirePost` and `wirePostToModel` (`internal/api/client.go`) updated to map all five new fields.

## Compose UI Changes

### New inputs in the feed compose overlay (press `n`)

The compose overlay now stacks: content → title → topics → toggle line.

| Input | Purpose |
|---|---|
| Compose textarea | Post body (unchanged) |
| Title box | Optional title (single-line textinput, blank = no title sent) |
| Topics box | Comma-separated topics (unchanged) |
| Toggle line | `[ ] public  [ ] nsfw` state display |

### Tab navigation

`tab` cycles: **compose** → **title** → **topics** → back to **compose**.

### Toggle keys (active while compose overlay is open)

| Key | Action |
|---|---|
| `alt+p` | Toggle `isPublic` (initialises from `defaultPublicPost` setting on each open) |
| `alt+s` | Toggle `isNSFW` (always starts as false) |

### Submission

`ctrl+s` or `alt+enter` (from any input in the overlay) submits with all fields. The title is trimmed of leading/trailing whitespace before submission.

## Rendering

### Feed cards and Post Detail (`render.go`, `postdetail.go`)

Layout when optional fields are present:
1. Header: `@author  [timestamp]  [♫]  [★]  ...  replies`
2. **Badges line** *(only when any badge applies)*: `[#guild-slug]  [nsfw]  [public]`
3. **Title line** *(only when title is set)*: title text in highlight colour
4. Body
5. Topics

In Post Detail the title uses `theme.Title` for more prominence.

### Profile Posts tab (`profile.go`)

`renderPostItem` uses the post title as the preview text when set, instead of the first line of content.

### Bookmarks (`bookmarks.go`)

Line 2 (content preview) shows the post title when set, otherwise the first line of content as before.

## Tests Added

| File | Test |
|---|---|
| `internal/api/client_test.go` | `TestHTTPCreatePost_SendsAllFields` — verifies request body and response parsing |
| `internal/api/client_test.go` | `TestHTTPGetFeed_ParsesExtendedPostFields` — verifies all 5 new fields are parsed from feed response |
| `internal/api/mock_test.go` | `TestMockCreatePost_TitleAndFlags` — verifies title/isPublic/isNSFW echoed back |
| `internal/ui/app_test.go` | `TestCreatePostCmd_TooSoonConversion_ReturnsPostConvertedToNoteMsg` / `TestCreatePostCmd_NormalSuccess_ReturnsPostCreatedMsg` |

## Undocumented server behavior: posting too soon converts to a journal entry

Confirmed live (2026-08-12, see `docs/00-api-backlog.md`): submitting a post
within the server's per-account cooldown of a previous one is not rejected
with `429`. Instead `POST /v1/posts` returns its normal `{ postId, slug }`
success shape, but the ID doesn't resolve — `GET /v1/posts/:id` 404s — because
the content was silently saved as a journal/note entry instead. The server
does generate a "System" notification about it (visible on the website, and
reflected in `GET /v1/notifications/unread-count`), but that notification is
never returned by `GET /v1/notifications` under any filter — see the
`00-api-backlog.md` entry on this. So the dangling `postId` is the only
signal any REST client, including this one, can actually act on.

`createPostCmd` (`internal/ui/app.go`) now follows every successful
`CreatePost` with a `GetPost(id)` check. A 404 there produces
`postConvertedToNoteMsg` instead of `postCreatedMsg`, which surfaces as a
warning banner ("posted too soon after your last entry — saved to your
Journal instead") rather than the compose overlay closing as if the post
went through normally.

## Update (feature 51): keep the composer open on a genuine failure

A `429`/`5xx`/network failure from `POST /v1/posts` (as opposed to the
too-soon success sub-case above) previously wiped the composer before the
banner appeared. `createPostCmd` now returns `postSubmitFailedMsg` (not
`actionErrMsg`) on non-401 errors, and Feed's new-post panel stays open and
populated until App reports the outcome — so the typed text survives a
rate-limited post. The panel also gains `Ctrl+D` to divert the draft into the
Journal instead of publishing. See `docs/51-compose-to-journal.md`.
