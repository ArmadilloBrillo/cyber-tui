# 50 — Post/reply attachments: audio (song) everywhere

> **SUPERSEDED 2026-08-28 (API v0.8.7):** image/gif attachment support
> described below has been removed. `POST`/`PATCH /v1/posts` and
> `POST /v1/replies` now reject `type: "image"` in `attachments` with `400`
> — and reject an inline markdown image in `content` too, unless its URL
> points at `bunker.cyberspace.online` (uploaded via the website first).
> `ctrl+g` on Feed's new-post panel, Post Detail's edit panel, and the reply
> compose box now warns instead of attaching or inserting markdown, since no
> URL cyber-tui can produce will ever be accepted there. The "⚠
> EXPERIMENTAL: image dimensions are currently faked" section that used to
> follow this note is gone along with the feature it described. Everything
> about audio (`ctrl+j`) below is still current, plus new artist/title/genre
> limits added the same day — see "Attachment shape" below.

## Purpose

Two gaps closed together, both discovered via live `apifetch` testing against
the API (see `docs/00-api-backlog.md`'s "Attachments — shape & validation"
section for the full research):

1. **Replies never supported any attachment**, blocked by an outdated
   client-side assumption. Live-tested 2026-08-27: `POST /v1/replies` accepts
   an `attachments` array identically to posts — `docs.md` just never
   mentions it for replies. `ctrl+g` (image/gif URL) now works on the reply
   compose box the same way it already did on Feed's new-post panel and Post
   Detail's edit panel.
2. **The `ctrl+j` song-attach modal (`docs/49-song-attach.md`) was cIRC-only.**
   Live-tested 2026-08-27: posts and replies both accept a `type: "audio"`
   attachment (`{src, origin: "youtube", artist, title, genre}`) with no
   `width`/`height` required — unlike an image attachment, which the API
   requires both dimensions for (1–640px inclusive, checked independently).
   `ctrl+j` now also opens from Feed's new-post panel, Post Detail's edit
   panel, and Post Detail's reply compose box.

## Attachment shape

```typescript
export interface Attachment {
  type: 'audio'
  src: string
  origin: 'youtube'
  artist: string  // required, max 100 chars
  title: string   // required, max 150 chars
  genre: string   // required, max 50 chars, lowercase
}
```

`type: 'image'` is no longer accepted — see the superseded note at the top of
this doc.

Per docs.md v0.8.7, artist/title/genre are all required and length-capped.
`SongPromptModel` (`internal/ui/screens/songprompt.go`) enforces this
client-side: `textinput.CharLimit` caps each field at input time
(`songArtistCharLimit`/`songTitleCharLimit`/`songGenreCharLimit`), `GenreValue()`
lowercases on read (same convention as `SlugValue`/`ParseTopics`), and
`BuildAttachment()` requires all three non-empty before returning a usable
attachment — shared by both post/reply targets and the cIRC/C-Mail `/song`
command, so the modal now requires genre everywhere even though the `/song`
text syntax still shows it bracketed as optional.

`CreatePost`/`EditPost`/`CreateReply` each accept **at most one** native
attachment — the wire request supports an array, but every screen's compose
UI offers exactly one audio slot (`PostComposePanel.SetPendingAudio`,
`PostDetailModel.SetReplyAudioAttachment`).

## Reply attachments (`internal/ui/screens/postdetail.go`)

`PostDetailModel` has `replyAudioAttachment` and `replyContextBase` — the
compose box's context label ("replying to @user") before an attachment note
is appended. `setReplyComposeContext` rebuilds that label live via
`ComposeModel.SetContext` so the user sees "replying to @user · song
<artist> — <title>" before submitting.

`SubmitReplyMsg` carries an `Attachment *model.Attachment` field, populated
on submit and cleared on both submit and cancel so nothing leaks into the
next reply. `App.createReplyCmd` (`internal/ui/app.go`) passes the
already-built audio attachment straight through to `CreateReply`
(`internal/api/client.go`, `internal/api/interface.go`,
`internal/api/mock.go`).

Native image attachment on replies (added here, then removed the same day by
API v0.8.7) is described in the superseded note at the top of this doc.

## Song attach on posts/replies (`internal/ui/app.go`)

`ctrl+j`'s guard broadened from `a.active == screenChatrooms` to also match
Feed's panel (`a.feed.PanelActive()`) and Post Detail's edit panel / reply
compose (`EditPanelActive()`/`ReplyComposeActive()`), still gated on
supporter status either way — non-chat targets get a `notify(notifyWarn,
...)` banner instead of chat's local system-message rejection, since neither
Feed nor Post Detail has an equivalent inline notice area.

`submitSongPrompt` branches by target instead of always building a `/song
...` command string: Feed/edit-panel/reply-compose each get a
`model.Attachment` handed to their own setter (`SetPanelAudioAttachment`,
`SetEditPanelAudioAttachment`, `SetReplyAudioAttachment`); only the
chat targets still get the `/song ...` text via `SetComposeValueMsg`.

`SongPromptModel` gained `BuildAttachment() (model.Attachment, bool)` — the
same field validation `BuildCommand` already did, now shared so both callers
agree on what counts as valid (`internal/ui/screens/songprompt.go`).
`BuildCommand` is now implemented in terms of it rather than duplicating the
validation.

## Panel rendering (`internal/ui/screens/compose.go`)

`PostComposePanel` has a `pendingAudio *model.Attachment` field, rendered as
a `song   <artist> — <title>` row, adding one row to `PanelHeight()` when
set. `OpenForEdit`'s attachment-sorting loop splits two ways: audio →
`pendingAudio`, anything else (including a legacy `type: "image"`
attachment from before API v0.8.7) → `otherAttachments`, carried through
untouched so an edit that touches attachments doesn't silently drop it.

## Scope / known gaps

- **Supporter gating on posts/replies is untested.** Chat's `/song` 403s a
  non-supporter server-side; this feature's one live test used a supporter
  account. The client-side supporter gate is applied uniformly as a
  precaution — revisit if a non-supporter test ever confirms posts/replies
  don't actually require it.
- **Reply edit** (`PATCH /v1/replies/:id`) still only supports `content` per
  `docs.md` — no attachment editing on an existing reply, only at creation.
  `OpenCompose`'s edit-reply path explicitly resets any pending reply
  attachment state before opening.
- ~~Post Detail's edit panel doesn't send its pending audio attachment~~ —
  **fixed 2026-08-28.** The `ComposeSubmitMsg` handler for the edit panel
  (`postdetail.go`) never set `SubmitPostEditMsg.AudioAttachment`, unlike
  Feed's equivalent — a song attached via `SetEditPanelAudioAttachment` on
  Post Detail's edit panel was silently dropped on submit
  (`attachments: []` sent instead). Pre-existing, discovered while removing
  image-attach support in this same area; one-line fix, guarded by
  `TestPostDetail_EditPanelSubmit_CarriesAudioAttachment`.
- Image/gif attachment support (this doc's original subject) was removed
  2026-08-28 — see the superseded note at the top.

## Verification

- `go test ./...`, `go vet ./...`, `staticcheck ./...` — no warnings.
- `internal/ui/screens/songprompt_test.go`: `BuildAttachment`'s required-field
  and shape checks (now including genre), `GenreValue` lowercasing.
- `internal/ui/screens/compose_test.go`: `OpenForEdit` prefilling the audio
  slot and carrying a legacy image attachment into `OtherAttachments`,
  `PanelHeight` growth for audio.
- `internal/ui/screens/postdetail_test.go`: `SubmitReplyMsg` carrying/clearing
  the audio attachment across submit and cancel.
- `internal/ui/app_test.go`: `applyAttachURL` warns on Feed/Post Detail
  targets instead of attaching or inserting markdown; `ctrl+j` opens from
  Feed panel and reply compose; `submitSongPrompt` routes to the right
  target instead of `SetComposeValueMsg`; `createReplyCmd`/`editPostCmd`
  forward the audio attachment.
- Manual: run the TUI, `ctrl+g` on Feed/a reply — confirm a warning, not an
  attach. `ctrl+j` on Feed's new-post panel and a reply — paste a YouTube
  URL with artist/title/genre, confirm the `song` row appears, post, confirm
  the audio attachment shows up on the created post/reply via `apifetch`.
