# Feature 35: Flagging / reporting (posts and replies)

Report a post or reply for moderation review (`POST /v1/posts/:id/flag`, `POST /v1/replies/:id/flag`, API v0.8). cIRC message flagging is out of scope for this feature — it needs per-message navigation in cIRC first.

## Entry point

`!` on the selected post (Feed, Post Detail) or the selected reply (Post Detail). Blocked with no key effect on your own content — the API returns `403` for a self-report and the client never sends the request.

## Flow

`FlagPrompt` (`internal/ui/screens/flagprompt.go`) is a small two-step overlay shared by `FeedModel` and `PostDetailModel`:

1. **Reason** — a single-line `textinput.Model` (max 500 chars, optional). `enter` moves to the confirm step with whatever was typed (blank is fine); `esc` cancels the whole flow.
2. **Confirm** — shows the typed reason (or "(no reason)") and asks `[y]es` / `[n]o back`. `y` submits; `n` returns to editing the reason; `esc` cancels.

There is no confirmation step before the reason box appears — pressing `!` goes straight to typing, matching the no-confirm feel of bookmark/watch. The confirm step sits right before the irreversible, un-withdrawable send.

On submit, `FeedModel`/`PostDetailModel` emit `FlagPostMsg{PostID, Reason}` or `FlagReplyMsg{ReplyID, PostID, Reason}` (`messages.go`). `App` (`app.go`) routes these to `flagPostCmd`/`flagReplyCmd`, which call the API and show a transient global banner: "reported" or "already reported" (the API is idempotent — reporting the same content twice returns `alreadyFlagged: true` instead of an error).

## API / model

- `api.Client` gained `FlagPost(postID, reason string) (flagID string, alreadyFlagged bool, err error)` and `FlagReply(replyID, reason string) (...)` — both POST a `{reason}` body and parse `{flagId, alreadyFlagged}` from the response, mirroring `CreateBookmark`.
- No new model fields: the API doesn't return flag status on `GET`, so there's nothing to persist client-side. Each `!` press is a fire-and-forget call; the banner is the only feedback.

## Not covered here

- cIRC message flagging (`POST /v1/circ/:roomId/messages/:messageId/flag`) — deferred until cIRC has per-message selection/navigation.
