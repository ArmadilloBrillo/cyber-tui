# 48 — Badge icons

## Purpose
Render a user's **supporter badge** as a real inline image next to their
username — reusing the same terminal-graphics pipeline (Kitty/iTerm2/Sixel)
that already renders avatars and post images, gated behind the same
`inlineImages` setting. Guild badges were attempted in the same pass but
reverted — see "Guild badges: not implemented" below.

## What a supporter badge actually is
`model.User.SupporterIcon` is an **icon-font identifier**, not an image URL
or emoji — e.g. `"pi"`, `"ph:crown"`. Confirmed convention: `[prefix:]name`,
where no prefix means **Lucide**, `lucide-lab:` means **Lucide Lab**, and
`ph:` means **Phosphor Icons** — all three are open-source SVG icon sets (not
fonts), so a name resolves to one static SVG asset once the library is known.
This was confirmed against real `SupporterIcon` values only (`"pi"`,
`"ph:crown"` on a live account) — see the Guild-icon section for why the same
assumption does *not* extend to `GuildIcon`.

## Icon resolution and rasterization (`internal/ui/imgview/badgeicon.go`)
- `ResolveBadgeIconURL(code string) (string, bool)` parses the prefix and
  builds the icon's raw SVG source URL:
  - Lucide: `https://cdn.jsdelivr.net/npm/lucide-static/icons/{name}.svg`
  - Lucide Lab: `https://raw.githubusercontent.com/lucide-icons/lucide-lab/main/icons/{name}.svg`
  - Phosphor: `https://raw.githubusercontent.com/phosphor-icons/core/main/assets/regular/{name}.svg`
    (always the "regular" weight — a bare `ph:crown` doesn't specify one)
  - Rejects (`ok=false`) anything outside the plain lowercase-kebab charset
    real icon names use (`badgeIconNameRe`) before it gets near an HTTP
    request, and any unrecognized prefix — it doesn't guess.
- `FetchBadgeIcon` downloads the SVG and rasterizes it via
  `github.com/srwiley/oksvg` + `github.com/srwiley/rasterx` (new
  dependencies — pure Go, no cgo, MIT-licensed; nothing already in `go.mod`
  parses SVG). `oksvg.ReadReplacingCurrentColor` substitutes a fixed color
  (`badgeIconColor`) for every icon's `currentColor` fill/stroke, since these
  icons carry no color of their own and terminal image protocols render
  fixed pixels with no access to the caller's theme. Output is a fixed
  64×64 `image.Image` — large enough to downscale cleanly to the 1-2
  terminal cells a badge actually occupies.
- `imgview.Fetch` dispatches to `FetchBadgeIcon` when the URL carries the
  `imgview.BadgeURLPrefix` (`"badge:"`) sentinel — everywhere else in the
  inline-image pipeline (`App.fetchInlineImageCmd`, the fetch/encode/cache
  machinery) is completely unchanged. A slot's `URL` is simply `"badge:" +
  code` (e.g. `"badge:pi"`).

## Placement: inline-in-text, not a row band
Every other inline image reserves a *row band* — blank lines spliced above
or below text. A badge sits *inline within* an existing line, immediately
after a username. This didn't need a new mechanism: `InlineImageSlot{Row,
ColIndent, MaxCols, MaxRows}` already models "exact row, exact column, small
box" — a badge is just a slot with `MaxRows=1` and a small `MaxCols`
(`badgeIconCols = 2`), and `App`'s placement code already draws via an
absolute cursor move independent of the text stream, so it doesn't care
whether the target is a reserved band or a gap mid-line.

Two small shared helpers in `internal/ui/screens/inlineimage.go` do the rest:
- `badgeGap(n int) string` — returns the literal blank space a rendered line
  must reserve after some text for `n` badges to be composited over later.
- `badgeSlot(code string, row, col int, key string) (InlineImageSlot, bool)`
  — builds one badge's slot; `ok=false` for an empty code.
- `userBadgeCodes(u model.User) []string` — the supporter code to show for a
  user (a 0- or 1-element slice). Deliberately supporter-only — see below.

Each screen appends `badgeGap(n)` to the rendered text (so downstream width
math — meta alignment, box padding — accounts for the reserved space) and
reports the matching slot from `VisibleInlineImages()`, at the exact row
that line renders on and the column immediately following the text it
follows.

## Where badges render
- **Profile** (`profile.go`): the username line and the Info tab's
  "Supporter" row (in place of the old `"yes (pi)"` text). The Supporter
  row's badge position is dynamic (bio length, which tab is active, etc.),
  so `infoTabView`/`viewBodyBeforeWebsiteBand` return a
  `profileBadgeSpot{Row, Col}` for it — computed by measuring the real
  rendered height up to that row, the same "recompute fresh each call"
  approach this file already used for the `WebsiteImageUrl` band.
- **C-Mail** (`cmail.go`): only the detail view's `"@username"` header —
  never the conversation list card, never per-message. See "C-Mail: the
  conversation-data gap" below for why this needed an extra fetch, unlike
  Profile.

Explicitly **not** covered this round: posts/replies (Feed, Post Detail,
Search, Topics, Bookmarks). Post/reply API payloads carry no author badge
data at all — only `authorId`/`authorUsername` — confirmed against the live
API docs. Showing a badge there would mean a separate `GET
/v1/users/:username` fetch-and-cache subsystem per distinct author; logged
in `docs/00-api-backlog.md` as something to revisit if the API adds author
badge fields to posts/replies directly.

## Guild badges: not implemented
Guild badges (Guilds list icon, Profile's username-line guild half, Profile's
"Guild" row) were built in the same pass as supporter badges, on the
assumption that `GuildIcon`/`Guild.Icon` followed the same Lucide/Lucide-Lab/
Phosphor convention as `SupporterIcon`. **That assumption was wrong** — a
live `GET /v1/guilds` fetch (all 20 real guilds on the platform) shows:

- 18/20 are bare kebab-case strings that are Unicode/CLDR character names —
  `crown`, `rocket`, `crab`, `beer-mug`, `four-leaf-clover`, `cat-face`,
  `black-sun-with-rays` (literally ☀️'s official Unicode 1.1 name), etc.
- 2/20 use an unrecognized `dinkie-icons:` prefix — an obscure pixel-art icon
  pack with no confirmed hosting/CDN.
- None match Lucide/Lucide-Lab/Phosphor naming, confirmed by the "code-filled"
  sample in `docs/api-reponses/users.me.api.reponse.json.md` also being an
  icon the *web UI itself* renders nothing for.

`GuildIcon` on `model.User` is a direct copy of the member's guild's own
`icon` field (per `docs/00-latest-api-reference.md`: "guildId, guildSlug,
guildIcon and guildName describe the one guild the user is a member of"), so
this is the same broken assumption everywhere `GuildIcon` appears, not just
on the Guilds screen.

Fixing this properly would need either a curated CLDR-name→emoji text lookup
(covers the 18/20 observed — no image fetch needed at all, since these were
apparently designed as emoji all along) or identifying the `dinkie-icons`
source — neither attempted this round. See the `docs/00-api-backlog.md` entry.
`userBadgeCodes` deliberately never includes `GuildIcon`; `renderGuildItem`'s
pre-existing `◆`/★/☆ text fallback (`guildIcon()`/`isEmojiIcon()`,
`guilds.go`) is untouched and still the only thing shown for a guild icon.

## C-Mail: the conversation-data gap
Separately from the guild-icon mismatch above, C-Mail's conversation data
never carries badge fields at all: `Conversation.Participants []model.User`
is populated from `wireCMailOtherUser` (`GetConversations`,
`internal/api/client.go`) — a thin shape with only
`userId`/`username`/`displayName`/`profilePictureUrl`, confirmed against
`docs/00-latest-api-reference.md`. Unlike Profile's data path
(`wireUserToModel`, a full `wireUser`), `SupporterIcon`/`IsSupporter` are
left at Go zero values.

Fixed with a small extra fetch rather than reverting: opening a conversation
(`CMailConvSelectedMsg`, both the "open existing conversation" and "start a
new one" paths) now also carries `OtherUsername string` —
`CMailModel.OtherProfileFetchTarget(conv)` returns the participant's
username, or `""` if unresolvable (`"unknown"`) or already cached
(`HasOtherProfile`). When non-empty, `App`'s `loadCMailOtherProfileCmd`
fetches their full profile via the existing `GetProfile(username)` and
`SetOtherProfile` caches it on `CMailModel.otherProfiles map[string]model.User`
(never evicted — bounded by the number of distinct people DMed in a
session). The header (`View()`) and its badge slot (`VisibleInlineImages()`)
read from `otherParticipantBadgeUser`, which overlays the cache onto the thin
`otherParticipantUser` when present, else falls back to it (no badge) until
the fetch resolves. A failed fetch just means the badge silently stays
absent — not worth a user-facing error toast.

## Fallback behavior
Unchanged whenever `inlineImages` is off, the terminal has no detected
graphics protocol, or (for C-Mail) the profile fetch hasn't resolved yet:
Profile's Supporter row still shows `"yes (pi)"`, and C-Mail's header shows
just the plain username until the badge is ready.

## Verification
- `go test ./...`, `go vet ./...`, `staticcheck ./...`, `govulncheck ./...`.
- `internal/ui/imgview/badgeicon_test.go`: `ResolveBadgeIconURL` prefix
  parsing and charset rejection (path traversal, unknown prefixes, etc.).
- Per-screen tests: `profile_test.go` (username supporter badge, no second
  guild slot, Supporter-row badge, `IsSupporter=false` gate, no Guild-row
  badge even with `GuildIcon` set, all off when inline images disabled),
  `cmail_test.go` (header badge only after `SetOtherProfile`/the real fetch
  cmd resolves, absent before that, `OtherUsername` populated/omitted
  correctly by the Enter-key handler depending on cache state),
  `guilds_test.go` (list never produces a badge slot for any `Icon` value).
  `app_test.go`: `CMailConvSelectedMsg` with a non-empty `OtherUsername`
  fires and applies the profile fetch; an empty one doesn't fetch.
- Manual, with `inlineImages: true` and a real graphics-capable terminal:
  confirm Profile's username line and Supporter row show a real supporter's
  badge; confirm opening a C-Mail conversation with a supporter shows their
  badge in the header shortly after opening, and reopening it later doesn't
  re-fetch; confirm Guilds list and Profile's Guild row show exactly the
  pre-feature `◆`/★/☆/plain-text behavior.
