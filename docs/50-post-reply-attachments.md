# 50 — Post/reply attachments: image on replies, audio (song) everywhere

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

## Attachment shape (confirmed live)

```typescript
export interface Attachment {
  type: 'audio' | 'image'
  src: string
  origin?: 'youtube'   // audio only
  artist?: string      // audio only
  title?: string       // audio only
  genre?: string       // audio only
  width?: number       // image only, required despite the "?"
  height?: number      // image only, required despite the "?"
}
```

`CreatePost`/`EditPost`/`CreateReply` each accept **at most one** native
attachment — the wire request supports an array, but every screen's compose
UI offers exactly one image/gif slot and one audio slot, and setting one
clears the other (`PostComposePanel.SetAttachmentURL`/`SetPendingAudio`,
`PostDetailModel.SetReplyAttachmentURL`/`SetReplyAudioAttachment`). Nobody
asked for "attach both an image and a song to the same post," so this stays
simple rather than widening every command's signature to a slice for a case
that's never been requested.

## Reply attachments (`internal/ui/screens/postdetail.go`)

`PostDetailModel` gains `replyAttachmentURL`/`replyAudioAttachment` (mutually
exclusive, set via `SetReplyAttachmentURL`/`SetReplyAudioAttachment`) and
`replyContextBase` — the compose box's context label ("replying to @user")
before an attachment note is appended. `setReplyComposeContext` rebuilds that
label live via `ComposeModel.SetContext` (new — the label was previously
fixed at `Open()` time) so the user sees "replying to @user · attach
<url>" or "· song <artist> — <title>" before submitting.

`SubmitReplyMsg` gained `AttachmentURL`/`Attachment` fields, populated on
submit and cleared on both submit and cancel so nothing leaks into the next
reply. `App.createReplyCmd` (`internal/ui/app.go`) resolves the image URL's
real dimensions via the existing `resolveAttachment` (same 640px client-side
guard as posts) or passes the already-built audio attachment straight
through — `CreateReply` itself gained an `attachment *model.Attachment`
param (`internal/api/client.go`, `internal/api/interface.go`,
`internal/api/mock.go`).

`applyAttachURL`'s reply-compose branch (`internal/ui/app.go`) now routes to
`SetReplyAttachmentURL` instead of warning "replies don't support image
attachments" — that message was accurate against `docs.md`'s prose but wrong
against the live API.

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

`PostComposePanel` gained a `pendingAudio *model.Attachment` field alongside
the existing `attachmentURL`, rendered as a `song   <artist> — <title>` row
directly below the existing `attach <url>` row (same truncation/width
handling), adding one row to `PanelHeight()` when set. `OpenForEdit`'s
attachment-sorting loop now splits three ways: image/gif → `attachmentURL`,
audio → `pendingAudio`, anything else → `otherAttachments` (unchanged
fallback, now effectively unreachable given the API's closed `type` union,
kept as a defensive catch-all).

## ⚠ EXPERIMENTAL: image dimensions are currently faked (2026-08-27)

`resolveAttachment` (`internal/ui/app.go`) no longer fetches an attached
image to determine its real size — it always declares `640x640`
(`maxAttachmentDim`), at the user's explicit request, to test whether the
API's declared-dimension check is meaningfully enforced anywhere beyond the
create/edit request itself (confirmed it's never checked against the real
file — see `docs/00-api-backlog.md`'s "Attachments — shape & validation").
This means:

- Any image attached through cyber-tui right now is declared as 640×640
  regardless of its real resolution — a false statement sent to the API and
  stored on the post/reply, visible to every viewer on every client.
- The client no longer rejects (or can detect) an oversized image before
  posting — the honest-dimension guard and its clear error message are gone
  along with the fetch.
- This is explicitly temporary, tracked here so it isn't mistaken for
  finished behavior. `resolveAttachment`'s doc comment in `app.go` has the
  exact revert instructions (restore the `imgview.Dimensions` fetch + the
  `maxAttachmentDim` comparison it replaced).

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
- The `attachmentTypeForURL`-assigned string `"gif"` (not in the API's
  documented `'audio' | 'image'` union) is a pre-existing, separate gap not
  touched by this feature — see `docs/00-api-backlog.md`.

## Verification

- `go test ./...`, `go vet ./...`, `staticcheck ./...` — no warnings.
- `internal/ui/screens/songprompt_test.go`: `BuildAttachment`'s shape and
  shared validation failure cases.
- `internal/ui/screens/compose_test.go`: `OpenForEdit` prefilling both slots,
  image/audio mutual exclusivity, `PanelHeight` growth.
- `internal/ui/screens/postdetail_test.go`: reply attachment mutual
  exclusivity, `SubmitReplyMsg` carrying/clearing the attachment across
  submit and cancel.
- `internal/ui/app_test.go`: `applyAttachURL` on reply compose no longer
  warns; `ctrl+j` opens from Feed panel and reply compose; `submitSongPrompt`
  routes to the right target instead of `SetComposeValueMsg`;
  `createReplyCmd`/`editPostCmd` resolve/forward the right attachment.
- Manual: run the TUI, `ctrl+g` on a reply — attach an image, post it, confirm
  it renders. `ctrl+j` on Feed's new-post panel and a reply — paste a
  YouTube URL, confirm the `song` row appears, post, confirm the audio
  attachment shows up on the created post/reply via `apifetch`.
