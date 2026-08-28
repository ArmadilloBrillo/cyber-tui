# 49 — /song attach modal (cIRC)

## Purpose
cyberspace.online lets a cIRC message attach a YouTube track to a room's
jukebox via `/song <youtube url> | <artist> | <title> [| <genre>]`
(supporter-only; see `docs/00-latest-api-reference.md`). Typing that whole
pipe-delimited command by hand is tedious and error-prone. `ctrl+j`, while a
cIRC room's composer is focused, opens a modal with four fields — URL,
Artist, Title, Genre — where Artist and Title auto-fill (best-effort) from
the pasted URL, and every field stays editable before it's sent.

## Auto-fill: YouTube oEmbed (`internal/youtube`)
`internal/youtube` is a new, network-capable-but-network-free-until-called
package, independent of `internal/api` (this hits `youtube.com`, not
cyberspace.online):
- `ExtractVideoID(rawURL) (id string, ok bool)` — recognizes
  `youtube.com/watch?v=`, `youtu.be/<id>`, `youtube.com/shorts/<id>`, and
  `youtube.com/embed/<id>` (`www.`/`m.` host prefixes accepted). Used for
  instant client-side validation before a fetch or submit — the server
  itself 400s a non-YouTube `/song` link, this just avoids the round trip.
- `FetchMetadata(ctx, rawURL) (title, author string, err error)` — GETs
  YouTube's public oEmbed endpoint (`https://www.youtube.com/oembed?...`),
  no API key required. `author_name` (the channel) is the best available
  stand-in for "artist" — oEmbed has no separate artist or genre field, so
  genre is always left for the user to fill in by hand. Own `http.Client`
  (8s timeout) and User-Agent, mirroring `internal/ui/imgview/fetch.go`'s
  pattern for third-party fetches.

## The modal (`internal/ui/screens/songprompt.go`)
`SongPromptModel` is a 4-field form (url/artist/title/genre `textinput`s),
structurally closest to `PostComposePanel`'s tab-cycling `focus` enum, but
simpler — no textarea, no toggles. It never talks to the network itself;
`App` drives the async fetch, same split as the inline-image pipeline.

- **Tab / Shift+Tab** cycle the four fields, wrapping.
- **Enter on the URL field**: client-side validates via `ExtractVideoID`
  (invalid → inline warning, focus stays put); if valid, `App` starts the
  oEmbed fetch and focus moves to Artist immediately — fields populate in
  place when the fetch resolves (`SongPromptModel.ApplyMetadata`), or a
  "couldn't auto-fill — enter artist/title manually" note appears on error
  (`FetchFailed`) without blocking anything.
- **Enter on Artist/Title**: behaves like Tab (advance focus).
- **Enter on Genre (last field), or ctrl+s from any field**: attempt
  submit — requires a valid URL plus non-empty Artist, Title, **and Genre**
  (all three are required as of API v0.8.7, plus length/case limits — see
  `docs/50-post-reply-attachments.md`'s "Attachment shape" for the canonical
  reference; this modal enforces genre everywhere, even though the `/song`
  text syntax below still shows it bracketed as optional). On success the built
  `/song ...` string is handed to the cIRC composer via the existing
  `screens.SetComposeValueMsg` — the same mechanism `ctrl+g`'s GIF-attach
  flow already uses (`applyAttachURL`) — so the user sees the exact command
  in the input box and still presses Enter there to actually send it.
- **Esc**: cancel, discard everything.

## Wiring (`internal/ui/app.go`)
- `ctrl+j` is scoped to cIRC only: `a.active == screenChatrooms &&
  a.activeScreenHasFocusedInput()`. Non-supporters are rejected locally —
  `a.chatrooms.AppendSystemMessage(roomID, "*** song attachments require
  supporter status")` — instead of opening a modal whose submit would just
  hit the server's 403; this mirrors the existing local rejection of an
  unrecognized slash command.
- The async fetch follows the `inlineImageFetchedMsg` /
  `fetchInlineImageCmd` pattern exactly: `songMetadataFetchedMsg` +
  `fetchSongMetadataCmd`, intercepted early in `Update` via
  `handleSongMetadataFetched` (spliced in next to
  `handleInlineImageFetched`). A result arriving after the modal was
  cancelled is dropped rather than reopening or mutating a fresh prompt.
- `handleSongPromptKey` mirrors `handleIconPickerKey`/
  `handleAttachURLPromptKey`'s split: it intercepts esc/tab/shift-tab/
  ctrl+s/enter itself, forwarding everything else into
  `SongPromptModel.Update`.

## Layout
`renderSongPrompt(a App) string` (a one-line `a.songPrompt.View()`) was
added to the `modalRenderer` interface and both `TabsLayout`/`MillerLayout`
implementations, plus a `songPromptOpen` case in `compositeOverlays` —
same shape as every other simple modal (icon picker, attach-URL prompt).
The `ctrl+j` shortcut is listed in the help modal's global section in both
layouts, next to `ctrl+g`.

## Scope: cIRC only
`/song` is technically accepted server-side in C-Mail too
(`baseSlashCommands` in `chatrooms.go`), but this modal only triggers from
cIRC, matching what was asked for. Extending it to C-Mail later is a small
follow-up — the modal and fetch pipeline are screen-agnostic; only the
`ctrl+j` case's `a.active == screenChatrooms` check would need to also
allow `screenCMail`.

## Verification
- `go test ./...`, `go vet ./...`, `staticcheck ./...` — no warnings.
- `internal/youtube/oembed_test.go`: `ExtractVideoID` across `watch?v=`,
  `youtu.be/`, `/shorts/`, `/embed/`, `www.`/`m.` hosts, and non-YouTube/
  malformed input; `FetchMetadata` against an `httptest.Server` for the
  success, non-200, malformed-JSON, and missing-title paths.
- `internal/ui/screens/songprompt_test.go`: `Open` resets/focuses, Tab/
  Shift+Tab cycling and wrapping, typing routes to the focused field only,
  `BuildCommand` validation (full, genre lowercased, missing artist/title/
  genre, invalid/empty URL), `ApplyMetadata`/`FetchFailed`/`SetLoading`, and the
  keystroke-clears-status behavior.
- `internal/ui/app_test.go`: `ctrl+j` opens the modal for a supporter with
  cIRC's composer focused, rejects locally (system message, no modal) for a
  non-supporter, and is unconsumed outside cIRC (`setupCMailDetail`); esc
  closes without submitting; Enter on a valid/invalid URL fetches-and-
  advances vs. warns-in-place; `submitSongPrompt` closes and emits the
  correct `SetComposeValueMsg` on valid fields, stays open with a warning
  when a required field is missing; `handleSongMetadataFetched` applies a
  result to an open prompt and drops one for a closed (cancelled) prompt.
- Manual: run the TUI, open a cIRC room, `ctrl+j` on a supporter account —
  paste a real YouTube URL, confirm artist/title populate, edit a field,
  submit, confirm the composer shows the correct `/song` string, send it,
  confirm the message renders with the audio attachment. Also check:
  `ctrl+j` as a non-supporter (local rejection message, no modal), a
  non-YouTube URL (client-side warning), and a private/unfetchable video
  (manual-entry fallback still lets the message send).
