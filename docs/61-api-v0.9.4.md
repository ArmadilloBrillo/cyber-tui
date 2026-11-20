# Plan: API v0.9.4 alignment

## Context

The live API docs moved from v0.9.2 (our snapshot, `docs/00-latest-api-reference.md`) to v0.9.4. The diff adds three things:

1. `repliesDisabled` on entries: set at create time, fixed afterwards (PATCH returns 400), reply attempts return 403, clients should hide the reply action.
2. Two new chat commands, `/spin` and `/tarot [question]`, valid in both cIRC and C-Mail.
3. A new `isTarot` / `tarot` message field. `content` is complete on its own.

Decision (user): tarot is command-only. The `tarot` object is not parsed, matching how `/8ball` and `/fortune` are handled today (no client code reads `isEightball`/`isFortune`).

Branch: `feature/api-v0.9.4` off `dev`. No unmerged local branches besides `dev`.

Per the stored workflow, the first commit saves this plan as `docs/61-api-v0.9.4.md`, then a review agent checks it for edge cases before any code.

---

## Epic: API v0.9.4 alignment

### Story 0: Refresh the API snapshot
As a maintainer, I want the reference docs on file to match the live API, so future work is planned against the current contract.

Acceptance criteria
- `docs/00-latest-api-reference.md` holds the v0.9.4 text.
- `docs/00-api-backlog.md` has a `/docs.md` row for v0.9.4 and an "Unimplemented" entry per story below, updated as each story lands.

Tasks
- 0.1 Replace the snapshot with the fetched v0.9.4 file.
- 0.2 Save the diff as `docs/api-diff-v0.9.2-to-v0.9.4.diff` (matches the existing `api-diff-v0.8.12-to-v0.9.0.diff` convention).
- 0.3 Add the backlog rows; update the header version line (currently says v0.9.1).

### Story 1: Read `repliesDisabled` from entries
As a reader, I want the client to know when an entry is closed to replies, so it can avoid offering an action that will fail.

Acceptance criteria
- `model.Post` carries a `RepliesDisabled` bool populated from every endpoint that returns entries (feed, post detail, topics, bookmarks, guild posts, search hits that go through `wirePost`).
- Absent field decodes as false.

Tasks
- 1.1 Add `RepliesDisabled` to `wirePost` (`internal/api/client.go:~180`) and `model.Post` (`internal/model/types.go:~85`); map it in the wirePost to Post conversion (`client.go:~1031`, next to `ContentWarning`).
- 1.2 Give one mock post `RepliesDisabled: true` in `internal/api/mock.go` (near the existing CW mock post at ~205).
- 1.3 Test: decode a JSON entry with and without the field.

### Story 2: Hide the reply action on closed entries
As a reader, I want no reply option on an entry whose author disabled replies, and a clear marker explaining why.

Acceptance criteria
- `r` in the feed (`internal/ui/screens/feed.go:670`) and in post detail (`internal/ui/screens/postdetail.go:839`, `OpenCompose` at ~490) does nothing on a closed entry, or shows a short "replies are disabled" notice. Applies to replying to the entry and to replying to any reply under it.
- Any key hint or help line that advertises `r reply` is suppressed for closed entries.
- A "replies off" marker shows in the badges row of both the feed card and the full post in post detail.
- If a 403 still comes back from `POST /v1/replies`, the user sees "replies are disabled on this post", not the raw wire error. This is a fallback only: the field is immutable, so a 403 means some endpoint did not return the field.

Tasks
- 2.1 Guard both `r` handlers on `post.RepliesDisabled`; show the notice immediately instead of opening compose.
- 2.2 Gate the `{"r", "reply"}` hints in `screenHints` (`internal/ui/layout_tabs.go:336` feed, `:345` post detail), mirroring the existing `CanEditSelected()` gating next to them. Add accessors `FeedModel.SelectedRepliesDisabled()` and `PostDetailModel.RepliesDisabled()`. The `?` overlay reads the same slice via `hintRows()` (`layout_tabs.go:502`), so one change covers both.
- 2.3 Add the marker in both badge rows: `renderPostBody` in `internal/ui/screens/render.go` (feed) and `renderFullPost` in `internal/ui/screens/postdetail.go:1030` (post detail). Add `repliesDisabled` to `feedBodyCacheEntry` and its cache-hit check (`internal/ui/screens/feed.go:~910-952`).
- 2.4 In `createReplyCmd` (`internal/ui/app.go:5508`), turn a 403 into a friendly `actionErrMsg` before returning. Do not change the shared `actionErrMsg` handler (`app.go:2903`).
- 2.5 Tests: `r` on a closed entry opens no compose in feed and post detail; hints omit `r reply` for closed entries; marker renders in both views; 403 path shows the friendly message.

Deferred: the reply compose closes before the request runs (`postdetail.go:672`), so a 403 loses the draft. Not fixed here because a 403 should not happen with correct data. If the live check finds an endpoint that omits `repliesDisabled`, add a submitting state to the reply compose, mirroring the new-post panel's `MarkSubmitting`/`ClearSubmitting`.

### Story 3: Create an entry with replies disabled
As an author, I want to publish an entry nobody can reply to.

Acceptance criteria
- The new-post compose panel has a `[ ] no replies` toggle, space to flip, reachable with tab/shift+tab like `public`/`nsfw`/`bork`.
- The toggle only appears when creating, never when editing (field is immutable; PATCH would 400).
- `CreatePost` sends `repliesDisabled: true` only when set (`omitempty`). `EditPost` never sends it.
- Layout still fits at narrow widths: `togglesW` in `compose.go:582` is updated, or the toggle moves to the CW row if the topics input gets too narrow. Decide during implementation by checking at 80 columns.
- Divert-to-Journal (`ctrl+d`) is unaffected (notes have no such field).

Tasks
- 3.1 Add `isRepliesDisabled` field, focus slot, toggle handling and view rendering in `internal/ui/screens/compose.go` (mirror `isNSFW`: lines ~337, 417, 447, 556, 722, 866-889); reset on new compose, hide when `m.editing`. Skip the new slot in `moveFocus` when `m.editing`, mirroring the `postFieldSlug` skip (`compose.go:617`). Update the `?` overlay text "tick public / nsfw / bork" (`internal/ui/layout_tabs.go:511`).
- 3.2 Carry it through the compose submit message to `createPostCmd` (`app.go:1445`, `5521`).
- 3.3 Extend `CreatePost` in `internal/api/interface.go:33`, `client.go:1374`, `mock.go:245`; add the field to `createPostRequest` (`client.go:~365`). Update every caller and test double the compiler flags, including `publishNoteCmd` (`app.go:6598`, passes false) and `createPostErrClient` in `app_test.go`.
- 3.4 Tests: request body includes the field only when true; toggle hidden in edit mode; focus cycle includes the new slot.

### Story 4: `/spin` and `/tarot` chat commands
As a chatter, I want to use `/spin` and `/tarot` in cIRC and C-Mail without the client rejecting them as unknown.

Acceptance criteria
- `/spin` and `/tarot` (with or without a question) are sent to the server in both cIRC and C-Mail.
- Tarot messages render from `content` like `/fortune`; the `tarot` object is ignored.
- The mock `/help` reply lists both commands.

Tasks
- 4.1 Add both to `baseSlashCommands` (`internal/ui/screens/chatrooms.go:335`).
- 4.2 Update `mockHelpReply` (`internal/api/mock.go:16`).
- 4.3 Check whether any local command-completion or help list elsewhere needs the same entries (grep for `"/fortune"`; currently only these two sites).
- 4.4 Test: both commands pass the known-command check in cIRC and C-Mail.

---

## Order and commits

Story 0, then 1 then 2 then 3 (each depends on the previous field), then 4 (independent; could go first). One commit per story, conventional prefixes (`docs:` for 0, `feat:` for 1-4). Tests and the grouped test summary table after each story; wait for approval before each commit.

## Verification

- `go test ./...`, `go vet ./...`, `staticcheck ./...` after each story.
- Run the TUI against the mock client: closed mock post shows the marker and blocks `r`; compose shows the toggle on new post only.
- Live check with `apifetch`: create a throwaway entry with `repliesDisabled: true`, confirm GET returns the field on every list endpoint the client reads it from (feed, single post, topics, bookmarks, guild posts, profile posts, search), confirm a reply returns 403, confirm PATCH with the field returns 400, then delete it. Send `/spin` and `/tarot` in a test C-Mail to confirm rendering.

## Out of scope

- Parsing or displaying the `tarot` object (card name, image, meaning).
- Showing the `/spin` target specially; it renders from `content`.
