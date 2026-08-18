# Feature 10: Replies

## Overview

Users can reply to posts from both the feed and the post-detail view. Replies support threading: replying to a reply sets the `parentReplyId` field.

---

## Key Bindings

| Screen | Key | Action |
|--------|-----|--------|
| Feed | `r` | Open post detail for the selected post with the compose box pre-focused |
| Post detail | `r` | Open compose box (targets the post if post is selected, targets the selected reply if a reply is selected) |
| Compose (open) | `Enter` | Add a newline |
| Compose (open) | `Ctrl+Enter` | Submit the reply |
| Compose (open) | `Esc` | Cancel and close the compose box |

---

## Replying from the Feed

Pressing `r` in the feed navigates immediately to post detail with the compose box already open. This ensures the user sees the full post and existing replies for context before writing.

---

## Compose Component

All multi-line text entry uses the shared `ComposeModel` (`internal/ui/screens/compose.go`):

- Backed by `github.com/charmbracelet/bubbles/textarea`
- Grows from 3 to 8 content lines as the user types; viewport shrinks to compensate
- Character limit: 32,768 (matches API limit)
- Message types: `ComposeSubmitMsg{Content string}`, `ComposeCancelMsg{}`

**This is the intended editor pattern for all future text input** that requires more than one line: new posts, profile bio, c-mail compose, chatrooms. Single-line inputs (login fields) continue to use `textinput`.

---

## API

`POST /v1/replies` — rate limit: 3/min, 10/day.

```json
{
  "postId": "abc123",
  "content": "Your reply (markdown)",
  "parentReplyId": "def456"   // omit for top-level replies
}
```

Response: `{ "data": { "replyId": "..." } }` (201).

After a successful reply, the reply list for the current post is automatically reloaded, and the newly created reply is selected and scrolled fully into view — the same `App.pendingReplyID` → `repliesLoadedMsg` → `PostDetailModel.ScrollToReply` pipeline used to deep-link into a specific reply from a notification or search result (see Implementation Notes). If the new reply isn't in the loaded tree (e.g. thread nesting deeper than `effectiveMaxDepth`), `ScrollToReply` no-ops and selection falls back to whatever `SetReplies` defaults to.

---

## Implementation Notes

- `CreateReply` is defined in `api.Client` interface and implemented in `HTTPClient` and `MockClient`.
- `PostDetailModel` embeds `ComposeModel`. When compose is active, the viewport height shrinks by `compose.BoxHeight()` (= `contentLines + 3`) and navigation keys (`↑↓/jk`) are blocked.
- `ShowPostForReplyMsg` (from feed) and `SubmitReplyMsg` (from post detail) are the inter-screen message types handled by `App.Update` in `internal/ui/app.go`.
- `createReplyCmd` returns the new reply's ID (`CreateReply`'s response) on `replyCreatedMsg`; `handlePostDetail` sets `App.pendingReplyID` to it before reloading, reusing the same field/pipeline the notification and search deep-link paths (`ShowSearchReplyMsg`, etc.) already use to select and scroll to a specific reply.

---

## Ctrl+Enter Terminal Note

`Ctrl+Enter` is only available as a distinct key sequence in terminals that support the [Kitty keyboard protocol](https://sw.kovidgoyal.net/kitty/keyboard-protocol/), and only if the app negotiates it — this project's bubbletea version (v1.3.10) doesn't; that's a bubbletea v2-only feature, and neither `wish` nor `ssh` (the SSH libraries `internal/ssh/server.go` uses) implement it either. So today `Ctrl+Enter` falls back to a regular Enter keystroke everywhere, not just on unsupporting terminals. If this becomes an issue, an alternative binding (e.g. `Ctrl+S`) can be added to `ComposeModel.Update`.
