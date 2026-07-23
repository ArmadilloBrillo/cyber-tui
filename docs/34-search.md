# Feature 34: Search

Full-text search across users, posts, and replies (`GET /v1/search`, API v0.7).

## Entry point

- **Global `/` shortcut** — works from any screen with no focused text input (same gating as `t`/`v`/`?`/`o`). Jumps straight to the Search screen with the query box focused. Falls through to type a literal `/` when a compose/chat input is focused (needed for `/dice`, `/me`, etc. in CIRC/C-Mail), and while already inside Search's own query box.
- Also present in the `←`/`→` tab-cycle bar (no number key — same treatment as Settings).
- Search results are always **global in scope** (users + posts + replies together) regardless of which screen `/` was pressed from — the screen you launch it from has no bearing on what gets searched.

## Screens / modes

`SearchModel` (`internal/ui/screens/search.go`) is a single screen with three internal modes:

1. **Query** — a text input (`bubbles/textinput`, same component as Login). Enter submits (`SubmitSearchMsg`); empty query is a no-op.
2. **Preview** — the grouped `type=all` result: up to 8 hits each for Users / Posts / Replies (the API's cap, no total count returned). `j`/`k` move across a flattened row list (section headers are not selectable). `enter` on a hit opens it; `enter` on a "see all N …" row (shown only when a category landed on exactly 8 hits — the only truncation signal the API gives) drills into that category.
3. **Type list** — one category, fully paginated via `SearchPosts`/`SearchReplies`/`SearchUsers`. Reaching the bottom of a non-exhausted list fetches the next page automatically (same pattern as Topics/Guilds).

Post and reply hits are sorted newest-first client-side (`sortPostsByRecency`/`sortRepliesByRecency` in `search.go`) — the API's result ordering for search isn't documented, so this is enforced rather than assumed. Pagination re-sorts the *whole* accumulated list on each page, not just the newly fetched page, so a later page's items land in their correct chronological position rather than always being appended at the bottom. User hits aren't date-sorted (relevance/follower count matter more there).

`esc` peels back one level at a time, preserving cached results (no re-fetch):

```
type-list → preview → query (focused) → origin screen
```

The query box is where the chain ends: it's the outermost level, and there's no result list showing while it's up (query mode fully replaces the view), so there's nothing left to peel back — `esc` there leaves Search entirely and returns to whichever screen `/` was pressed from (`App.searchReturn`), the same return-to-origin pattern `p` → profile → `esc` already uses (`profileReturn`). `/` records the origin the first time it switches into Search from elsewhere; pressing `/` again while already on Search just refocuses the query box without touching `searchReturn`.

This is also the escape hatch when a search request fails: `SetError` doesn't change the view (the user stays in query mode with results still empty), so leaving via `esc` is the only way back to normal tab/quit navigation — a stuck, permanently-focused query box would otherwise swallow `q`, `/`, `t`, and arrow/number tab navigation (see `docs/00-api-backlog.md` for the discovery). `esc` always blurs the query box on the way out (whether or not it's leaving), so a later arrival via tab-cycling — which, unlike `/`, doesn't call `FocusQuery` — never inherits a stuck focused state.

## Opening a hit

- **User hit** → `ShowUserProfileMsg` (existing shared message), `profileReturn = screenSearch`.
- **Post hit** → `ShowSearchPostMsg`, navigates to Post Detail, `postDetailReturn = screenSearch`.
- **Reply hit** → `ShowSearchReplyMsg{PostID, ReplyID}` — fetches the parent post, then scrolls Post Detail to the reply (same mechanism as the Notifications reply deep-link).

## API / model

- `api.Client` gained `Search(query) (model.SearchPreview, error)` and `SearchPosts`/`SearchReplies`/`SearchUsers(query, cursor) (…, string, error)`.
- No new hit-specific wire types: search hits reuse `model.User`, `model.Post`, `model.Reply` directly — their shapes are a subset of the existing post/reply/user shapes. The only new model type is `model.SearchPreview` (the grouped envelope).
- Typed search pagination (`SearchPosts` etc.) is server-side `page`-number based, but the client treats the returned value as an opaque cursor string like every other paginated endpoint — no special-casing needed in the UI layer.
- `MockClient` implements all four methods with a simple case-insensitive substring match over its existing static users/posts/replies.

## Known limitations

- The `type=all` preview gives no total count per category — "exactly 8 hits" is the only (imperfect) signal that a category has more results to drill into.
- Search does not cover notifications, private notes, or chat messages — only what `GET /v1/search` itself covers (users, posts, replies).
- **Server-side `createdAt` drift, discovered via live testing:** `GET /v1/search` doesn't consistently return the documented RFC3339 string — a numeric epoch was observed for user hits, and a raw Firestore Timestamp object (`{"_seconds":N,"_nanoseconds":N}`) for post hits. Handled client-side by `apiTimestamp` in `internal/api/client.go`, which accepts string, number, or object, and degrades to an empty timestamp (never fails the whole response) for anything else. See `docs/00-api-backlog.md`.
