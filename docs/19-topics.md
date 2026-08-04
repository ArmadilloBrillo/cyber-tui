# 19 — Topics

> **STATUS: DRAFT / IN PROGRESS** — Implementation not yet started.
> Blocked on: confirming live API response shapes for `GET /v1/topics` and `GET /v1/topics/:slug/posts` before writing wire types.
> Resume by: running the curl commands below, then implementing per the plan below.

---

## Curl Commands to Verify API Responses

```bash
# threatcrush-disable-next-line secret-generic-credential  docs placeholder, not a credential
REFRESH_TOKEN="your_refresh_token_here"

ID_TOKEN=$(curl -s -X POST https://api.cyberspace.online/v1/auth/refresh \
  -H "Content-Type: application/json" \
  -d "{\"refreshToken\": \"$REFRESH_TOKEN\"}" | jq -r '.data.idToken')

echo "=== GET /v1/topics ==="
curl -s -X GET https://api.cyberspace.online/v1/topics \
  -H "Authorization: Bearer $ID_TOKEN" | jq .

echo -e "\n=== GET /v1/topics/<slug>/posts?limit=5 ==="
curl -s -X GET "https://api.cyberspace.online/v1/topics/go/posts?limit=5" \
  -H "Authorization: Bearer $ID_TOKEN" | jq .
```

Once confirmed, update the `wireTopic` struct in step 3 below and remove this section.

---

## Overview

A **Topics** tab (tab 5, shortcut `5`; settings moves to `6`) after Bookmarks. Two-level navigation within a single screen:

```
viewTopicList  ──enter──>  viewTopicPosts  ──enter──>  screenPostDetail
                <──esc──                   <──esc (BackToFeedMsg)──
```

---

## API Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/v1/topics` | All topics, sorted by post count |
| `GET` | `/v1/topics/:slug/posts?limit=20&cursor=<postId>` | Paginated posts for a topic |

Rate limits: GET /v1/topics 20/min · GET posts 30/min.

---

## Key Bindings

### Topic list view

| Key | Action |
|-----|--------|
| `j` / `↓` | Move cursor down |
| `k` / `↑` | Move up; at top → refresh topic list |
| `enter` | Open topic → switch to post list view |

### Topic posts view

| Key | Action |
|-----|--------|
| `j` / `↓` | Move cursor down; at last item → load next page |
| `k` / `↑` | Move up; at top → refresh post list |
| `enter` | Open post in Post Detail |
| `esc` | Return to topic list (internal — no App message) |

---

## Architecture

Single `screenTopics` constant with internal `topicsView` state machine. The viewport is shared and `refreshContent()` renders differently based on `m.view`.

### Model

```go
type topicsView int

const (
    viewTopicList  topicsView = iota
    viewTopicPosts
)

type TopicsModel struct {
    view        topicsView

    // topic list sub-state
    topics      []model.Topic
    topicIndex  int

    // topic posts sub-state
    activeTopic string // slug of the selected topic
    posts       []model.Post
    postIndex   int
    nextCursor  string
    exhausted   bool
    loading     bool

    // shared
    viewport    viewport.Model
    itemOffsets []int
    width       int
    height      int
    ready       bool
    err         error
    loc         *time.Location
    relaxed     bool
}
```

### model.Topic (to add to internal/model/types.go)

```go
type Topic struct {
    Slug      string
    PostCount int
}
```

> **Note:** Verify actual field names against live API before finalising.

---

## Messages (topics.go → App)

```go
RefreshTopicsMsg{}
LoadTopicPostsMsg{Slug string}
LoadMoreTopicPostsMsg{Slug, Cursor string}
RefreshTopicPostsMsg{Slug string}
ShowTopicPostMsg{Post model.Post}
```

`ShowTopicPostMsg` is intentionally distinct from `ShowPostMsg` (which is owned by `handleFeed` and would overwrite `postDetailReturn` to `screenFeed`).

---

## App Integration (internal/ui/app.go)

12 sites to update:

| Site | Change |
|------|--------|
| `screen` const | Add `screenTopics` after `screenBookmarks` |
| `App` struct | Add `topics screens.TopicsModel` |
| `NewApp` | `topics: screens.NewTopicsModel()` |
| `applyWindowSize` | `a.topics, _ = a.topics.Update(m)` |
| `broadcastConfig` | `a.topics, _ = a.topics.Update(msg)` |
| `refreshViewports` | `a.topics, _ = a.topics.Update(msg)` |
| `delegateUpdate` switch | `case screenTopics: a.topics, cmd = a.topics.Update(msg)` |
| `renderActiveScreen` switch | `case screenTopics: return a.topics.View()` |
| `navigateTab` switch | `case screenTopics: return a.loadTopicsCmd()` |
| `handleErr` switch | `case screenTopics: a.topics = a.topics.SetError(m.err)` |
| `menuTabs` slice | Insert `{"topics", screenTopics}` after bookmarks |
| `handleTopics` function | New — see below |

### handleTopics message dispatch

| Message | Action |
|---------|--------|
| `topicsLoadedMsg` | `a.topics.SetTopics(msg.topics)` |
| `topicPostsLoadedMsg` | `a.topics.SetTopicPosts(msg.posts, msg.cursor)` |
| `topicPostsPageMsg` | `a.topics.AppendTopicPosts(msg.posts, msg.cursor)` |
| `screens.RefreshTopicsMsg` | `a.loadTopicsCmd()` |
| `screens.LoadTopicPostsMsg` | `a.loadTopicPostsCmd(msg.Slug, "")` |
| `screens.RefreshTopicPostsMsg` | `a.loadTopicPostsCmd(msg.Slug, "")` |
| `screens.LoadMoreTopicPostsMsg` | `a.loadTopicPostsPageCmd(msg.Slug, msg.Cursor)` |
| `screens.ShowTopicPostMsg` | Set `a.postDetailReturn = screenTopics`, switch to PostDetail, fire `loadRepliesCmd` |

### Internal app message types

```go
type topicsLoadedMsg struct{ topics []model.Topic }
type topicPostsLoadedMsg struct{ posts []model.Post; cursor string }
type topicPostsPageMsg struct{ posts []model.Post; cursor string }
```

---

## Tab Shortcut Renumbering

| Key | Before | After |
|-----|--------|-------|
| `1` | feed | feed |
| `2` | notifications | notifications |
| `3` | bookmarks | bookmarks |
| `4` | profile | profile |
| `5` | settings | **topics** |
| `6` | — | settings |

Update `case "5"` / `case "6"` blocks in `handleKeys` and the help modal label from `"1-5"` → `"1-6"`.

---

## Status Bar Hints

| View | Hint |
|------|------|
| `viewTopicList` | `enter · open   k · refresh` |
| `viewTopicPosts` | `enter · view post   esc · back   k · refresh` |

---

## Implementation Template

Follow `internal/ui/screens/bookmarks.go` throughout: viewport pattern, `buildContent`/`refreshContent`/`ensureSelectedVisible`, pagination state machine, exported setters (`SetTopics`, `SetTopicPosts`, `AppendTopicPosts`, `SetError`, `SetStatusMsg`).

---

## Tests to Write

| File | Tests |
|------|-------|
| `internal/api/client_test.go` | `TestHTTPGetTopics_*`, `TestHTTPGetTopicPosts_*` (pagination, empty cursor) |
| `internal/api/mock_test.go` | `TestMockGetTopics_*`, `TestMockGetTopicPosts_*` |
| `internal/ui/screens/topics_test.go` | View switching, k-at-top refresh, enter emits correct msgs, esc returns to list, pagination trigger |
| `internal/ui/app_test.go` | Tab cycle updated for 6 tabs |

---

## Definition of Done

- [ ] All unit tests pass
- [ ] `go vet` clean
- [ ] API response shapes confirmed and wire types updated
- [ ] This doc updated to remove DRAFT status and curl section
- [ ] User approves merge to `dev`
