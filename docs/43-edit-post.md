# Feature 43 — Edit Post / Reply

Implements editing your own posts and replies within the API's 5-minute, supporter-only edit window.

---

## Scope

| Capability | Status |
|---|---|
| Edit own post (content, title, topics, public/NSFW) — `e` key on Feed and Post Detail | Done |
| Edit own reply (content only — the only field the API accepts) — `e` key on Post Detail | Done |
| Gated to: own content, published <5 minutes ago, supporter account | Done |
| `e` hint dynamically shown/hidden in the status bar based on the *currently selected* post/reply | Done |
| `e` documented statically in the `?` help modal ("edit own, <5min") | Done |
| `(edited)` — not yet rendered in the post/reply view; `EditedAt` is captured in the model but no UI badge draws it yet | Deferred — see Files Changed |
| Attachment editing | Out of scope — no attachment-editing UI exists; the field is omitted from the request entirely, which per the API leaves existing attachments untouched |

---

## API Endpoints Used

| Method | Path | Purpose |
|---|---|---|
| `PATCH` | `/v1/posts/:id` | Edit content/title/topics/isPublic/isNSFW on an own post |
| `PATCH` | `/v1/replies/:id` | Edit content on an own reply |

Both require a supporter account and a publish time within 5 minutes; outside either condition the API returns `403`. The response carries no fields worth returning (`{ "data": { "postId": "..." } }` / `{ "replyId": "..." } }`), so the client applies the submitted fields to its local copy directly rather than round-tripping through the response — the same approach `CreatePost` already uses for fields its own response omits.

Per the docs, "the entry then carries an `editedAt` timestamp" — read as a persisted field on the entry, so it should come back on future `GET`s to any viewer, not just the editor. `wirePost`/`wireReply` now parse an optional `editedAt` field (tolerant of absence, via the existing `apiTimestamp` type) into `model.Post.EditedAt` / `model.Reply.EditedAt`. **This hasn't been confirmed live** — the API doc's example `GET` response JSON blocks weren't regenerated to literally list it. Worth an `apifetch` sanity check on an edited post; see `docs/00-api-backlog.md`.

---

## UI

### Gating (`CanEditSelected`)

`FeedModel.CanEditSelected()` and `PostDetailModel.CanEditSelected()` are the single source of truth, checked both by the `e` keypress and by the status bar hint:

```
AuthorUsername == currentUsername && currentUserIsSupporter && time.Since(CreatedAt) < 5*time.Minute
```

Computed fresh on every call — no background ticker. There's no existing precedent in this codebase for time-based UI elements that hide themselves after N seconds of wall-clock elapsed (`tea.Tick` is used exclusively for network polling/heartbeats), so this follows the same "compute inline" approach already used for the `AuthorUsername == currentUsername` ownership check elsewhere.

`currentUserIsSupporter` is propagated to both screens the same way `currentUsername` already is — from `model.User.IsSupporter` (parsed from `GetOwnProfile`), set at login and whenever the own profile reloads.

### Status bar / help modal

Unlike the existing `d` (delete) and `p` (view profile) hints — which are static labels in the status bar regardless of the selected item's ownership — the `e · edit` hint is genuinely dynamic: it only appears in the status bar when `CanEditSelected()` is true for whatever's currently selected, and disappears again once the selection changes or the 5-minute window elapses. The `?` help modal still uses the existing static-label convention (`e · edit own, <5min`), matching `d`'s `delete own`.

### Feed screen

- `e` on an eligible post opens the same `PostComposePanel` used for `n` (new post), but pre-filled via a new `OpenForEdit` and with the slug field removed (slug is immutable once published — sending it returns `400`).
- Ctrl+S submits `SubmitPostEditMsg`; Esc cancels. On success the post's `Content`, `Title`, `Topics`, `IsPublic`, `IsNSFW`, and `EditedAt` are updated in place (`FeedModel.ApplyPostEdit`) — `AuthorID`, `CreatedAt`, `RepliesCount`, etc. are untouched.

### Post Detail screen

- `e` on the post (nothing selected below it) opens a `PostComposePanel` the same way Feed does — Post Detail gained its own `editPanel` field since it has no "new post" flow to reuse.
- `e` on a selected reply opens the existing lightweight `ComposeModel` (same widget `r`/reply already uses) pre-filled via `OpenWithContent`, content-only. A new `editingReplyID` field disambiguates "editing this reply" from "composing a new reply" when the shared `ComposeSubmitMsg` fires.
- On success, `PostDetailModel.ApplyPostEdit` / `ApplyReplyEdit` update the local copy in place; reply edits rebuild the reply tree the same way `RemoveReply` already does.

### Error handling

A `403` is expected to be rare (the client-side gate should prevent most attempts) but possible as a race — the window can expire, or supporter status can lapse, between opening the editor and hitting Ctrl+S. It gets a friendly toast: "can't edit — outside the 5-minute window or not a supporter" (`editErrorMsg`, mirrors `pokeErrorMsg`/`flagErrorMsg`). Anything else falls through to the standard `actionErrMsg` toast.

---

## Files Changed

| File | Change |
|---|---|
| `internal/model/types.go` | Added `EditedAt time.Time` to `Post` and `Reply` |
| `internal/api/interface.go` | Added `EditPost`, `EditReply` to the `Client` interface |
| `internal/api/client.go` | `wirePost`/`wireReply` gain `EditedAt apiTimestamp`; `editPostRequest`/`editReplyRequest` wire types (no `omitempty` on Title — the API accepts `""` to clear it); `EditPost`/`EditReply` implementations |
| `internal/api/mock.go` | Stub `EditPost`/`EditReply` |
| `internal/ui/screens/feed.go` | `SubmitPostEditMsg`; `currentUserIsSupporter`, `editingPostID` fields; `CanEditSelected`, `ApplyPostEdit`; `e` key handler; `ComposeSubmitMsg` branches into edit vs. new-post |
| `internal/ui/screens/postdetail.go` | `SubmitReplyEditMsg`; `currentUserIsSupporter`, `editPanel`, `editingReplyID` fields; `CanEditSelected`, `ApplyPostEdit`, `ApplyReplyEdit`; `e` key handler (post vs. reply branch); `ComposeSubmitMsg` priority branch (edit-panel → reply-edit → new-reply); `ComposeActive`/`WindowSizeMsg` updated for the new panel |
| `internal/ui/screens/compose.go` | `PostComposePanel` gains an `editing` field and `OpenForEdit` — hides/skips the slug row and its Ctrl+S validation in edit mode |
| `internal/ui/app.go` | `postEditedMsg`, `replyEditedMsg`; `editPostCmd`, `editReplyCmd`, `editErrorMsg`; `SetCurrentUserIsSupporter` wired at login and profile-reload alongside the existing `SetCurrentUsername` calls; edit results applied to both Feed and Post Detail's local copies (guarded by post ID for Post Detail, since it may have an unrelated post open) |
| `internal/ui/layout_tabs.go` | `e · edit` status-bar hint, shown only when `CanEditSelected()`; static `e · edit own, <5min` row in the `?` help modal for Feed and Post Detail |
| `internal/api/client_test.go` | `TestHTTPEditPost_CallsCorrectEndpoint`, `TestHTTPEditPost_SendsEmptyTitleToClear`, `TestHTTPEditPost_Forbidden`, `TestHTTPEditReply_CallsCorrectEndpoint` |
| `internal/ui/screens/feed_test.go` | `CanEditSelected` gate tests (own/supporter/window), `e` key tests |
| `internal/ui/screens/postdetail_test.go` | Same, plus reply-edit coverage |
| `internal/ui/app_test.go` | `editErrorMsg` 403-softening + fallthrough |
