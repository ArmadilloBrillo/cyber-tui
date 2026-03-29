# 04 — API Alignment

## Overview

Aligns model types, API interface signatures, and the login UI with the real cyberspace.online API v0.2 spec (`docs/03-api-reference.md`). No real HTTP calls are made — the mock client is updated to match the new shapes so the app continues to run locally.

---

## What changed

### `internal/model/types.go`

| Type | Change |
|---|---|
| `User` | Added: `DisplayName`, `Email`, `WebsiteUrl`, `PinnedPostID`, `LocationName` |
| `Post` | `Body` → `Content`; `Author User` removed → flat `AuthorID`, `AuthorUsername`; added `RepliesCount`, `BookmarksCount`, `IsPublic`, `IsNSFW`, `Deleted` |
| `Tokens` | New — holds `IDToken`, `RefreshToken`, `RTDBToken` from login |
| `Reply` | New — `ID`, `PostID`, `AuthorID`, `AuthorUsername`, `Content`, `ParentReplyID`, `CreatedAt` |
| `ProfileUpdate` | New — pointer fields for `PATCH /v1/users/me` (`Bio`, `DisplayName`, `PinnedPostID`, `WebsiteUrl`, `LocationName`) |
| `Message`, `Conversation`, `Room` | Unchanged — RTDB territory |

### `internal/api/interface.go`

| Method | Change |
|---|---|
| `Login(username, password)` | → `Login(email, password string) (model.Tokens, error)` |
| `GetFeed(page int)` | → `GetFeed(cursor string) ([]model.Post, error)` |
| `CreatePost(body, ...)` | → `CreatePost(content string, ...)` |
| `UpdateProfile(bio string)` | → `UpdateProfile(update model.ProfileUpdate) error` |
| `GetOwnProfile()` | New — maps to `GET /v1/users/me` |
| Chat/DM methods | Unchanged — annotated as RTDB-pending |

### `internal/ui/screens/login.go`
- First input: `"username"` → `"email"`
- `SubmitLoginMsg.Username` → `SubmitLoginMsg.Email`
- `LoginMsg` simplified to empty struct (token storage moved to `App`)

### `internal/ui/app.go`
- `tokens model.Tokens` added to `App` struct
- `loginCmd`: takes `email` param; stores `Tokens`; calls `GetOwnProfile()` (not `GetProfile(username)`) so username comes from the profile response, not the login form
- `loadFeedCmd`: `GetFeed("")` (cursor-based)
- `loadProfileCmd`: `GetOwnProfile()`
- `saveProfileCmd`: wraps bio in `model.ProfileUpdate{Bio: &bio}`

### `internal/ui/screens/feed.go`
- `p.Author.Username` → `p.AuthorUsername`
- `p.Body` → `p.Content`

---

## Remaining gaps (future branches)

| Gap | Branch |
|---|---|
| Chat/DMs via Firebase RTDB (SSE streams) | `feature/rtdb-chat` |
| Replies (threaded) | `feature/replies` |
| Bookmarks, Follows, Notifications, Notes | Future feature branches |
| Topics screen | Future feature branch |
| Settings screen | Future feature branch |
| Real HTTPClient implementation | `feature/http-client` |
