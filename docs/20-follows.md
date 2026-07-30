# Feature 20 — Follows

Implements follow/unfollow on user profiles, follower/following count display, and follow state detection.

> Post count (`postsCount`) was displayed here originally but was removed — the API field was deprecated and no longer returned reliable data (it's absent from the current API docs). See `docs/00-api-backlog.md`.

---

## Scope

| Capability | Status |
|---|---|
| Show follower / following counts on profile screen | Done |
| Show post count on profile screen | **Removed** — `postsCount` was a deprecated, unreliable API field |
| Follow another user (`f` key on read-only profile) | Done |
| Unfollow a user (`f` key again when already following) | Done |
| Detect follow state on profile load (first-page scan) | Done |
| Optimistic count update after follow/unfollow | Done |
| Followers/following list with usernames | **Deferred** — API returns user IDs only; blocked until the API adds username fields to the follows response |

---

## API Endpoints Used

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/v1/follows?type=following&limit=50` | Detect whether logged-in user follows a given profile |
| `POST` | `/v1/follows` | Follow a user by their user ID |
| `DELETE` | `/v1/follows/:id` | Unfollow using the follow document ID |

The user profile endpoints (`GET /v1/users/me`, `GET /v1/users/:username`) were already in use; this feature also maps the `followersCount` and `followingCount` fields that were previously ignored (a `postsCount` field was mapped too but was later removed — see Files Changed).

---

## UI

### Profile screen

- Counts line displayed below the username in the format `N followers · N following`.
- When viewing another user's profile (`readOnly=true`):
  - Hint bar shows `f · follow` (or `f · unfollow` when already following).
  - Pressing `f` sends `FollowUserMsg` or `UnfollowUserMsg` to `App`.
  - On success the count updates optimistically and a brief `following.` / `unfollowed.` confirmation appears.
- The follow key is suppressed on the logged-in user's own profile.

---

## Follow State Detection

On every foreign profile load, `loadUserProfileCmd` concurrently fetches the first page (50 items) of `GET /v1/follows?type=following`. If the target user's ID appears in the list, `isFollowing=true` and the follow document ID is stored for later unfollow.

This covers the majority of users. Users who follow more than 50 accounts may not see the correct initial state for accounts beyond the first page — this is an acceptable trade-off for MVP. A dedicated API endpoint for checking follow state would fix it cleanly.

---

## Files Changed

| File | Change |
|---|---|
| `internal/model/types.go` | Added `FollowersCount`, `FollowingCount` to `User` (originally also `PostsCount`, removed later — deprecated API field); added `Follow` struct |
| `internal/api/interface.go` | Added `GetFollowing`, `Follow`, `Unfollow` to `Client` interface |
| `internal/api/client.go` | Mapped count fields in `wireUser`; added `wireFollow`, `wireFollowToModel`; implemented three follow methods |
| `internal/api/mock.go` | Stub implementations for `GetFollowing`, `Follow`, `Unfollow` |
| `internal/ui/screens/shared.go` | Added `FollowUserMsg`, `UnfollowUserMsg` |
| `internal/ui/screens/profile.go` | Added `isFollowing`, `followID`, `followFeedback` fields; `SetFollowState`, `SetFollowFeedback`, `IncrementFollowersCount`; counts display; `f` key binding |
| `internal/api/client_test.go` | Tests for `GetFollowing`, `Follow`, `Unfollow`, and profile count field mapping |
| `internal/ui/screens/profile_test.go` | Tests for `SetFollowState`, counts display, `f` key messages, own-profile suppression |
| `internal/ui/app.go` | Extended `userProfileLoadedMsg`; added `followResultMsg`, `unfollowResultMsg`; `followUserCmd`, `unfollowUserCmd`; updated `loadUserProfileCmd`; wired follow handlers in `handleProfile` |
