# 51 — Compose to Journal + keep the editor open on a failed publish

## Purpose

Two gaps in the compose flows, closed together:

1. **No way to divert a new post into the Journal.** The reverse already
   existed (journal editor `Ctrl+P` = "publish note as a post"), and the server
   silently demotes a too-soon post into a note, but the new-post composer only
   offered "publish to feed" (`Ctrl+S`). You now press **`Ctrl+D`** in the
   new-post panel to store what you've written as a private note instead.

2. **A failed publish threw the work away.** Every compose submit closed the
   editor *synchronously, before the API call* — `PostComposePanel.Close()`
   wipes every field, and the journal editor's `Ctrl+P` path called
   `closeEdit()` before emitting `SubmitPublishNoteMsg`. When `CreatePost` then
   failed (rate limit → `ErrRateLimited` 429, 5xx, network), the only feedback
   was a 4-second banner with the raw `API error RATE_LIMITED (429): …` string
   and the typed text was gone. Now the editor stays open and populated until
   the API confirms success; a failure drops you straight back into your work.

The "posted too soon" case (`postConvertedToNoteMsg`) is **not** a failure —
the server already stored the text — so it still closes the panel, just
explicitly now that submit no longer auto-closes.

## Behaviour

### New-post panel (Feed, `n`)

| Key | Action |
|-----|--------|
| `Ctrl+S` | Publish to the feed (unchanged) |
| `Ctrl+D` | Save what's written as a private Journal note — **new** |
| `Esc` | Cancel, discard |

- `Ctrl+D` is inert while editing an existing post (`e`) and when the body is
  empty.
- A non-empty `title` is prepended to the note as a `# Heading` line so the
  journal list label (`markdown.FirstLine`) reads sensibly. `slug` /
  `public` / `nsfw` are dropped — notes have no such fields.
- On success: banner "saved to your Journal", panel closes, the note is
  prepended to the Journal list.

### In-flight state (both editors)

From the moment a publish / save-as-note is dispatched until the App reports
the outcome:

- The editor stays open with every field exactly as the user left it.
- `Ctrl+S` / `Ctrl+D` / `Esc` (feed panel) and `Ctrl+P` / `Ctrl+S` / `Esc`
  (journal editor) are ignored, so a slow request can't be double-fired.
- The status-bar hint shows `… posting` / `… publishing`.

### On failure (non-401)

- The editor comes back, still populated.
- Banner text: a 429 becomes *"you're posting too fast — wait a bit, then
  retry or press `ctrl+d` / `ctrl+s` to save it to your Journal"*; other errors
  fall through to `friendlyErr`.
- A 401 still returns `actionErrMsg` so `handleUnauthorized` redirects to login.

## Implementation

| Layer | Change |
|-------|--------|
| `internal/ui/screens/compose.go` | `PostComposePanel.submitting` + `MarkSubmitting` / `ClearSubmitting` / `IsSubmitting`; `Ctrl+D` case emitting new `ComposeSaveAsNoteMsg`; in-flight key guard |
| `internal/ui/screens/feed.go` | `ComposeSubmitMsg` (new-post branch) marks submitting instead of `closeCompose()`; new `ComposeSaveAsNoteMsg` handler (title→heading, emits `SaveNewPostAsNoteMsg`); `CloseComposeAfterSuccess` / `ClearComposeSubmitting` / `ComposeEditing` / `ComposePanelActive` / `ComposeSubmitting` |
| `internal/ui/screens/journal.go` | `JournalModel.publishing`; confirm-publish `y` sets `publishing` instead of `closeEdit()`; `handleEditKey` in-flight guard; `CloseEditAfterPublish` / `ClearPublishing` / `IsPublishing` |
| `internal/ui/screens/messages.go` | `SaveNewPostAsNoteMsg{Content, Topics}` |
| `internal/ui/app.go` | msgs `postSubmitFailedMsg` / `noteFromComposeSavedMsg` / `notePublishFailedMsg`; `createPostCmd` + `publishNoteCmd` return the failed-* msg on non-401 errors; new `saveNewPostAsNoteCmd` (→ `CreateNote`); `composeFailText(err, saveKey)` helper; `handlePostDetail` / `handleJournal` cases close-or-reopen the editor |
| `internal/ui/layout_tabs.go` | Feed compose hints gain `Ctrl+d to journal` (new post only) and a `… posting` state; journal compose hints gain `… publishing`. Miller layout inherits via its `focusList` delegation to `TabsLayout.screenHints`. |

`SaveNewPostAsNoteMsg` and the new-post flow's other messages are handled in
`handlePostDetail` with no active-screen guard, same as the pre-existing
`SubmitNewPostMsg` / `postCreatedMsg` / `postConvertedToNoteMsg`.

## Scope / known gaps

- **Post *edit* failures still close the panel.** Only the new-post `Ctrl+S`
  and journal `Ctrl+P` paths were changed; `SubmitPostEditMsg` (feed `e`,
  post-detail `e`) is out of scope.
- The "posted too soon → saved as a malformed server-side Note" path
  (`docs/00-api-backlog.md`) is unchanged and still not recoverable — the
  server already consumed the text. This feature only helps genuine failures.

## Verification

- `go test ./...`, `go vet ./...`, `staticcheck ./...` — no warnings.
- `internal/ui/screens/compose_test.go` — `Ctrl+D` emits `ComposeSaveAsNoteMsg`
  (non-empty, create mode only); submit keys inert while `submitting`.
- `internal/ui/screens/feed_test.go` — `ComposeSubmitMsg` leaves the panel open
  + submitting; `ClearComposeSubmitting` keeps it, `CloseComposeAfterSuccess`
  tears it down; `ComposeSaveAsNoteMsg` yields `SaveNewPostAsNoteMsg` with the
  title as a `#` heading.
- `internal/ui/screens/journal_test.go` — confirm-publish leaves `editMode`
  true + `publishing` true with content intact; `CloseEditAfterPublish` closes,
  `ClearPublishing` keeps it; submit keys inert while publishing.
- `internal/ui/app_test.go` — `createPostCmd` / `saveNewPostAsNoteCmd` /
  `publishNoteCmd` return the failed-* msg on `ErrRateLimited` and
  `actionErrMsg` on `ErrUnauthorized`; `saveNewPostAsNoteCmd` success →
  `noteFromComposeSavedMsg`; `TestCreatePostCmd_TooSoonConversion…` still green.
- Manual (mock client): Feed `n`, type a title + body, `Ctrl+D` → "saved to
  your Journal", check tab `3`. Force the mock's `CreatePost` to error →
  `Ctrl+S` leaves every field intact with the rate-limit banner. Journal `n`,
  type, `Ctrl+P`, `y` with the mock erroring → editor stays, `Ctrl+S` saves.
