# Feature 24 — Profile Sub-tabs

Adds five sub-tabs to the profile screen for browsing a user's activity history and social connections.

---

## Overview

The profile screen now exposes a horizontal tab bar in view mode (below the username/counts header):

```
[ Info ] [ Posts ] [ Replies ] [ Following ] [ Followers ]
```

Each tab lazy-loads its data on first visit and supports cursor-based pagination.

---

## Tabs

| Tab | Content | API endpoint |
|---|---|---|
| **Info** | Bio, website, location, follow/edit hint | (existing profile data) |
| **Posts** | Paginated post history for the viewed user | `GET /v1/users/:username/posts` |
| **Replies** | Paginated reply history for the viewed user | `GET /v1/users/:username/replies` |
| **Following** | Users this person follows | `GET /v1/follows?userId=…&type=following` |
| **Followers** | Users who follow this person | `GET /v1/follows?userId=…&type=followers` |

---

## Keyboard Shortcuts

| Key | Action |
|---|---|
| `tab` | Next sub-tab |
| `shift+tab` | Previous sub-tab |
| `j` / `↓` | Navigate down within a list tab |
| `k` / `↑` | Navigate up within a list tab |
| `enter` | Open post/reply in PostDetail, or navigate to user profile (Following/Followers) |
| `e` | Edit profile (own profile, Info tab only) |
| `f` | Follow / unfollow (read-only profile) |
| `esc` | Back to previous screen |

**Note:** `tab` / `shift+tab` in **edit mode** still navigate between the edit form fields (unchanged behaviour).

---

## Data Loading

- **Lazy**: a tab's data is only fetched the first time the user switches to it.
- **Pagination**: scrolling near the end of a list emits a `LoadMore*Msg` which the App handles by fetching the next page.
- **Reset**: switching to a different user's profile (`ShowUserProfileMsg`) calls `ClearTabs()`, which discards all cached tab data and returns to the Info tab.

---

## New API Methods

| Method | Endpoint |
|---|---|
| `GetUserPosts(username, cursor)` | `GET /v1/users/:username/posts?limit=20` |
| `GetUserReplies(username, cursor)` | `GET /v1/users/:username/replies?limit=20` |
| `GetFollowers(cursor)` | `GET /v1/follows?type=followers&limit=20` |
| `GetUserFollows(userID, followType, cursor)` | `GET /v1/follows?userId=…&type=…&limit=20` |

---

## New Message Types

```go
// Lazy-load triggers (profile → app)
ShowUserPostsMsg     { Username string }
ShowUserRepliesMsg   { Username string }
ShowUserFollowingMsg { UserID string }
ShowUserFollowersMsg { UserID string }

// Pagination (profile → app)
LoadMoreUserPostsMsg     { Username, Cursor string }
LoadMoreUserRepliesMsg   { Username, Cursor string }
LoadMoreUserFollowingMsg { UserID, Cursor string }
LoadMoreUserFollowersMsg { UserID, Cursor string }

// Post navigation from profile tabs (profile → app)
ShowProfilePostMsg { Post model.Post }
```

---

## New ProfileModel Methods

| Method | Purpose |
|---|---|
| `ClearTabs()` | Reset all tab data (called on profile switch) |
| `SetUserPosts(posts, cursor)` | Store first page of posts |
| `AppendUserPosts(posts, cursor)` | Add next page of posts |
| `SetUserReplies(replies, cursor)` | Store first page of replies |
| `AppendUserReplies(replies, cursor)` | Add next page |
| `SetUserFollowing(follows, cursor)` | Store first page of following |
| `AppendUserFollowing(follows, cursor)` | Add next page |
| `SetUserFollowers(follows, cursor)` | Store first page of followers |
| `AppendUserFollowers(follows, cursor)` | Add next page |

---

## Model Changes

`model.Follow` gained two new fields used for display in the Following/Followers tabs:

```go
type Follow struct {
    ID               string
    FollowerID       string
    FollowedID       string
    FollowerUsername string  // NEW
    FollowedUsername string  // NEW
    CreatedAt        time.Time
}
```

---

## Known Limitations

- The `GetFollowing` method (without userId) was not removed from the interface; it's used in `loadUserProfileCmd` for follow-state detection.
- When navigating to a user from the Following/Followers tab, pressing ESC on the nested profile returns to the original `profileReturn` destination (e.g., Feed), not the intermediate profile. Deep-linking through profiles is a single-level stack by design.
