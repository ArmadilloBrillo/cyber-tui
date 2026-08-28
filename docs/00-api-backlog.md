# API Backlog — Outstanding Features & Known Issues

Tracks gaps between the cyberspace.online API (v0.8.7) and what is currently implemented in the TUI client.
Update this file whenever a feature is implemented or an issue is discovered/resolved.

---

## Known API Issues (Server-Side Bugs)

These bugs exist in the server — no client-side fix is possible. Report to the API maintainer.

| Endpoint | Method | Status | Description | Discovered |
|---|---|---|---|---|
| `/v1/follows` | GET | **Open** | Response does not include `followerUsername` or `followedUsername`. Confirmed still missing in v0.4 (re-tested 2026-05-29). The profile Following/Followers tabs fall back to showing a truncated user ID; profile navigation from those tabs is disabled until the API returns usernames. | 2026-04-17 |
| `/v1/notifications` | GET | **Open (by design?)** | Notifications can point to posts that have since been deleted, and the notification object exposes no "target deleted/unavailable" flag — opening one is the only way to discover the target is gone (`GET /v1/posts/:id` → 404). The client now handles this gracefully (friendly "This post has been deleted" banner, non-blocking). A `targetDeleted` field (or server-side filtering of dead-target notifications) would let the client mark/skip them up front. | 2026-06-03 |
| Rate limits (spec) | — | **Resolved** | The v0.4.1 inline-vs-table contradiction is gone in v0.5.0: the consolidated Rate Limits table now matches the inline per-endpoint limits (Entries 15/day, Replies 15/day, Notes 30/day, Bookmarks 75/day, Profile/Settings 15/day). Read limits were also raised (most list endpoints 30→45/min; profile/follows/topics/bookmarks/notes 20→30/min). Resolved 2026-06-04. | 2026-05-29 |
| `/v1/search` | GET | **Open** | `createdAt` is inconsistent across hit types, and doesn't match the RFC3339 string every other user/post/reply-returning endpoint uses. Confirmed live: a numeric epoch (assumed ms) on user hits, and a raw Firestore Timestamp object (`{"_seconds":N,"_nanoseconds":N}`) on post hits — apparently un-normalized before being sent to the client. Not documented in the API spec. Client-side workaround: `apiTimestamp` (`internal/api/client.go`) accepts string, number, or object for `wireUser`/`wirePost`/`wireReply`'s date fields, and degrades to an empty timestamp rather than failing the whole response for any other shape. | 2026-07-23 |
| `/v1/notifications?type=...` | GET | **Resolved** | The `type` query param (comma-separated notification-type filter), 500ing for every value since 2026-07-24, now works. Re-tested live via `apifetch`: `?type=reply&limit=5` returned 5 filtered `reply` items, and the exact combo that used to fail, `?type=dm_message,chat_mention&limit=5`, returned 5 correctly-filtered `chat_mention` items — no `500`. The client's `types []string` param plumbing (`GetNotifications`) was already in place unaffected, waiting for this fix; a type-filter UI control can now be built whenever prioritized (see `GetNotifications` row in Notifications v0.4.1 below). | 2026-08-13 |
| `/v1/users/me`, `/v1/users/:username` | GET | **Resolved (client-side removal)** | `postsCount` was deprecated and no longer returned reliable data; the field is also absent from the current API docs snapshot (`followersCount`/`followingCount` still present). Removed the `posts` segment from the profile counts line and the `PostsCount` field from `model.User`/`wireUser`. | 2026-07-29 |
| `/docs.md` | GET | **Resolved** | `docs.md` now reports v0.8 live. `docs/00-latest-api-reference.md` re-fetched and diffed — new surface (flagging, cIRC message delete, message attachments/styles/mute commands, `EMAIL_NOT_VERIFIED`) added to Unimplemented API Features below. | 2026-07-31 |
| `/v1/notifications` vs `/v1/notifications/unread-count` | GET | **Open (by design, per docs)** | The two endpoints disagree on a fresh, unpaginated session: `unread-count` returned `{"count": 5}` while `?limit=20&read=false` returned only 3 items (2 `thread_reply`, 1 `new_post_following`) — a `new_follower` and a `poke` notification counted in the badge were absent from the list. Confirmed live via `apifetch` 2026-08-03. Matches the documented caveat at `docs/00-latest-api-reference.md:690` ("the count may be slightly higher... which applies additional filtering"), so this is expected server behavior rather than a bug, but it means the TUI's badge and its notification list can legitimately disagree — not a TUI regression. No client-side fix possible; investigated after a user report of "webui shows 5 unread, tui shows 3." | 2026-08-03 |
| `/docs.md` | GET | **Resolved** | `docs.md` now reports v0.8.1 live. `docs/00-latest-api-reference.md` re-fetched and diffed — only change is cIRC presence idle tracking (`lastActivity`/`idleAfterMs`) and reworked presence/typing rate limits (per-room/per-conversation caps replacing flat per-minute ones); added to Unimplemented API Features below. | 2026-08-03 |
| `/v1/cmail` | GET, RTDB `/user_conversations/<uid>` | **Open (client-side mitigation shipped)** | Some accounts have conversation entries that are fully empty stubs: `otherUser.userId`/`otherUser.username`/`lastMessage` all `""`, `lastMessageAt: 0` — a valid `conversationId` with no other content at all (confirmed live via `apifetch GET /v1/cmail`, a dozen such entries on one account). These render as blank `@` (or `@unknown`) rows with a bogus `01-Jan-1970` date. Client now filters conversations with zero identity/content (`isEmptyConversation` in `internal/api/client.go`) rather than rendering them; the underlying orphaned/corrupted records still exist server-side. | 2026-08-04 |
| `/v1/search` | GET | **Resolved** | The `502 BAD_GATEWAY` reported 2026-08-04 has cleared — re-tested live via `apifetch GET /v1/search?q=test&type=all`, returns normal grouped results. | 2026-08-12 |
| `/docs.md` | GET | **Resolved** | `docs.md` now reports v0.8.2 live. `docs/00-latest-api-reference.md` re-fetched and diffed — only change is a newly-documented rate limit (10/min, 60/hour per IP) on `POST /v1/auth/check-username`, which is already out of scope for this client (web-only registration flow). No code changes needed. | 2026-08-03 |
| `/v1/auth/refresh` | POST | **Resolved** | Was returning `500 FUNCTION_INVOCATION_FAILED` for every refresh token (2026-08-04). Re-tested live via `apifetch` — `LoginWithRefreshToken` (which calls `POST /v1/auth/refresh` as its first step) now succeeds cleanly and a follow-up `GET /v1/users/me` returns full profile data, no error. Auto-login/session-resume works again. The defensive fix in `refresh()` (`internal/api/client.go` — check `resp.StatusCode` before attempting JSON decode) stays in place regardless, since a plain-text error body from any endpoint would otherwise still produce a confusing decode error. | 2026-08-13 |
| `/docs.md` | GET | **Resolved** | `docs.md` now reports v0.8.3 live. `docs/00-latest-api-reference.md` re-fetched and diffed byte-for-byte (UTF-8) against the previous v0.8.2 snapshot — only the version header changed, no endpoint, field, rate-limit, or content-limit differences. No code changes needed. | 2026-08-05 |
| `/docs.md` | GET | **Resolved** | `docs.md` now reports v0.8.4 live. `docs/00-latest-api-reference.md` re-fetched and diffed — new surface: `PATCH /v1/posts/:id` and `PATCH /v1/replies/:id` (supporter-only edit within 5 minutes of publishing), `POST /v1/users/:username/poke` (new `poke` notification type, 1/hour + 8/day rate limit), and a published `/types.d.ts` TypeScript definitions file. Added to Unimplemented API Features below. | 2026-08-12 |
| `GET /v1/posts/:id` | GET | **Resolved (confirmed live)** | Confirmed via `apifetch`: created a throwaway post, `PATCH`ed it, then `GET` returned a persisted `editedAt` field (e.g. `"editedAt": "2026-08-12T14:53:30.848Z"`) alongside the original `createdAt` — matches the docs' prose claim and the client's existing optimistic parsing in `wirePost`/`wireReply`. Test post deleted afterward. The `(edited)` badge premise in `docs/43-edit-post.md` is now safe to build on whenever prioritized. | 2026-08-12 |
| `POST /v1/posts` | POST | **Open — undocumented, client-side mitigation shipped** | Posting too soon after a previous post is not rejected with `429`. The response still returns a normal `{ postId, slug }` success shape, but the ID doesn't resolve (`GET /v1/posts/:id` → 404) — the content was silently saved as a journal/note entry instead. Confirmed live via `apifetch` (two posts seconds apart): the second call's `postId` 404s on `GET`, and the same content appears via `GET /v1/notes`. The orphaned note record itself is also malformed: it's missing the `noteId` field every other note in the list has, so it can't be fetched, edited, or deleted individually via `GET/PATCH/DELETE /v1/notes/:id` — a dead, un-cleanable entry sitting in the account's journal. Client-side, `createPostCmd` (`internal/ui/app.go`) now follows every `CreatePost` success with a `GetPost(id)` check; a 404 there surfaces a warning banner instead of the silent-success flow. See `docs/28-extended-posts.md`. | 2026-08-12 |
| `GET /v1/notifications` vs `/v1/notifications/unread-count` | GET | **Narrowed — reachable via `type` filter, still excluded from the default list** | Originally logged as totally unreachable via REST. Now that the `type`-filter `500` bug is fixed (2026-08-13), re-tested: `?type=post_cooldown&limit=5` returns real live entries — confirming the mystery type is exactly `post_cooldown` (`docs/15-notifications.md` guessed this from the v0.8.5 docs description; now confirmed with a real payload). Each has `actorUsername: "system"` and a populated `reason` (e.g. `"You can only post once per 10 minutes. Your draft was saved as a private Note."`), matching the client's `hasActor`/`Reason` handling exactly. But `GET /v1/notifications` with no filter, or `?read=false`, still omits these entries from the page — so the type-filtered fetch is the only way to see them; the default list continues to silently exclude the whole type. No client-side fix needed beyond what's already built (the `post_cooldown` icon/summary is now confirmed correct against a real payload); a future type-filter UI (see Notifications v0.4.1 above) would let a user opt into seeing these. | 2026-08-13 |
| `/docs.md` | GET | **Resolved** | `docs.md` now reports v0.8.5 live. `docs/00-latest-api-reference.md` re-fetched and diffed (previous file was also mojibake-corrupted at rest — non-ASCII bytes double-encoded — re-saved clean as part of this fetch). Real content changes are all in Notifications: 8 new types (`graffiti_mention`, `moderator_granted`/`removed`, `api_access_granted`/`removed`, `system_ban_lifted`, `post_cooldown`, `rate_limit_warning`); `GET /v1/notifications` now documented as excluding muted/blocked/off types (list can come back shorter than `limit` — must paginate on non-null `cursor`, not short-page length); `actorId`/`actorUsername` now documented optional (absent or literal `"system"` for account-level notifications); `unread-count` gains `exact` (`false` once unread > 100, count caps at 100 — render "99+"); `read-all` gains `hasMore` (caps at 5,000/call, loop until false). None of this is implemented client-side yet — see the new Notifications v0.8.5 row below. | 2026-08-13 |
| `GET /v1/guilds` | GET | **Open (by design?) — not a bug, a naming-scheme mismatch** | `icon` values are never the Lucide/Lucide-Lab/Phosphor scheme `SupporterIcon` uses (confirmed separately live, e.g. `"pi"`, `"ph:crown"`). Confirmed live via `apifetch` across all 20 real guilds: 18/20 are bare kebab-case Unicode/CLDR character names (`crown`, `rocket`, `crab`, `black-sun-with-rays` — literally ☀️'s official Unicode 1.1 name — `beer-mug`, `four-leaf-clover`, `cat-face`, etc.), and 2/20 use an unidentified `dinkie-icons:`-prefixed set (an obscure pixel-art icon pack, no confirmed hosting/CDN). Not documented anywhere which icon-naming scheme(s) `icon` draws from. `GuildIcon` on `model.User` is a direct copy of the member's guild's own `icon`, so this is the same data everywhere it appears (Guilds list, Profile's guild badge). Attempted as real inline images in feature 48 and reverted client-side — see `docs/48-badge-icons.md`. A real fix needs either a curated CLDR-name→emoji text lookup (covers 18/20, no image fetch needed) or identifying the `dinkie-icons` source; neither attempted. | 2026-08-25 |
| `/docs.md` | GET | **Resolved — client updated same day** | `docs.md` now reports v0.8.7 live (previous snapshot on file was v0.8.6, one version behind — that jump wasn't separately logged here). `docs/00-latest-api-reference.md` re-fetched and diffed. Real content changes: (1) `attachments` on `POST`/`PATCH /v1/posts` and `POST /v1/replies` now rejects `type: "image"` with `400` — images are website-upload-only, travelling as inline markdown (`![alt](https://bunker.cyberspace.online/...)`), and even that markdown 400s if the URL isn't bunker-hosted; `artist`/`title`/`genre` on a `type: "audio"` attachment are now documented as all required (max 100/150/50 chars, genre lowercase) — client updated same day, see `docs/50-post-reply-attachments.md`. (2) `websiteImageUrl` removed from `PATCH /v1/users/me` — the profile's 88×31 button is now website-set only (still returned on reads) — **client updated same day**: `updateProfileRequest`/`ProfileUpdate` no longer send it, and the profile edit form's now-dead input field was removed entirely (`internal/ui/screens/profile.go`), guarded by `TestProfileEditForm_NoWebsiteImageUrlField`. (3) New "Programs" section (`GET`/`POST`/`DELETE /v1/programs`, `GET /v1/programs/:id/source`) — a terminal program registry (`web`/`term`/`wasm` runtimes) for the website's `/terminal` gallery; entirely new surface, not evaluated for cyber-tui relevance yet — see Unimplemented API Features below for a placeholder if prioritized. | 2026-08-28 |

---

## Unimplemented API Features

Features present in the v0.4 API spec that are not yet implemented in this client.
Ordered roughly by implementation effort / priority.

### Auth

| Endpoint | Method | Description | Priority |
|---|---|---|---|
| `/v1/auth/register` | POST | ~~Removed in v0.5.1~~ — registration is web-only; endpoint no longer in the API spec | N/A |
| `/v1/auth/resend-verification` | POST | Resend email verification link | Out of scope — web-only flow |
| `/v1/auth/check-username` | POST | Check if a username is available (no auth required) | Out of scope — only relevant alongside registration |

### Posts

| Endpoint | Method | Description | Priority |
|---|---|---|---|
| `/v1/users/:username/posts` | GET | Paginated post history for a user | **Done** — profile Posts tab |
| `/v1/users/:username/replies` | GET | Paginated reply history for a user | **Done** — profile Replies tab |

### Posts (extended — v0.4)

| Endpoint / Area | Description | Priority |
|---|---|---|
| `model.Post` fields | `Title`, `Slug`, `GuildID`, `GuildSlug`, `IsGuildThread` | **Done** — feature 28 |
| `POST /v1/posts` signature | Extended: `CreatePost(content, title, topics, isPublic, isNSFW)` | **Done** — feature 28 |
| `GET /v1/users/:username/posts/:slug` | Slug-based post lookup not in Client interface. Useful for deep-linking; not needed for core navigation. | Low |
| `GET /v1/users/:username/posts/:slug` — no ID fallback | Server bug/gap: when a post has no custom/generated slug, its website permalink is `/{username}/{postId}` — a raw post ID in the slug position, indistinguishable from a real slug on the wire. The slug-lookup endpoint 404s for that shape rather than also matching by ID (confirmed live: same segment 404s here, succeeds against `GET /v1/posts/:id`). Client-side workaround shipped — see `docs/27-url-opener.md`. | Client-side fixed; server could also just accept an ID in this endpoint |
| `POST /v1/posts` — optional `slug` field (v0.7) | Custom slug (`a-z0-9-`, max 60 chars); server generates one if omitted. Compose panel (`PostComposePanel`) now includes a slug field with inline validation; empty slug is silently omitted from the wire. Same applies to `POST /v1/guilds/:slug/posts`. | **Done** — v0.7 alignment |
| Post/reply objects — no author badge fields | Confirmed (docs + live) that a post/reply carries only `authorId`/`authorUsername` — no `authorIsSupporter`/`authorSupporterIcon`/`authorGuildIcon`. Badge icons (feature 48, `docs/48-badge-icons.md`) render on Profile/C-Mail, where the full author `model.User` is already available, but can't extend to Feed/Post Detail/Search/Topics/Bookmarks without either a per-author `GET /v1/users/:username` fetch-and-cache layer or the API adding these fields directly to post/reply payloads. | Feature request to API maintainer — not blocking |

### Attachments — shape & validation, posts and replies (confirmed live, 2026-08-27)

> **2026-08-28 (API v0.8.7):** `docs.md` now documents `type: "image"` in a post/reply's `attachments` as a `400` — image attachment is website-upload-only (inline markdown pointing at `bunker.cyberspace.online`), and `type: "image"` in `attachments` is rejected outright. This closes several rows below as **moot rather than resolved**: the `⚠ EXPERIMENTAL` faked-dimensions behavior (row below referencing `resolveAttachment`) no longer applies — that code path was removed from cyber-tui — and the `"gif"`-not-in-the-`type`-union question is moot for the same reason (no client-constructed image/gif attachment exists anymore). `docs.md` also now documents `artist`/`title`/`genre` as all **required** (max 100/150/50 chars, genre lowercase) for a `type: "audio"` attachment — cyber-tui's `SongPromptModel` now enforces this (`docs/50-post-reply-attachments.md`). The rows below are left as a historical record of the pre-v0.8.7 investigation.

`docs.md`'s prose never documents the `attachments` object shape, and never mentions `attachments` at all under Replies (only under Posts, and only as "replaces the existing attachments" on `PATCH`). `GET /types.d.ts` (published since v0.8.4, previously logged below as "not applicable to this Go client" and never actually pulled for content until now) fills the gap:

```typescript
export interface Attachment {
  type: 'audio' | 'image'
  src: string
  origin?: 'youtube'
  artist?: string
  title?: string
  genre?: string
  width?: number
  height?: number
}
```

Matches `wireAttachment` (`internal/api/client.go:76-83`) field-for-field — that shape was reverse-engineered correctly by this client via live testing before this type file was ever checked.

Live-tested via `apifetch` against throwaway posts/replies (all created, tested, deleted within the same session):

| Finding | Confirmed behavior |
|---|---|
| **Replies accept `attachments`** | `POST /v1/replies` with an `attachments` array succeeds and `GET` echoes it back intact, despite zero mention in `docs.md`'s Replies section. `internal/ui/app.go`'s `applyAttachURL` (line ~4132) currently hard-rejects `ctrl+g` image attach on Post Detail's reply compose with *"replies don't support image attachments — there's no attachments field in the reply API"* — that message is now known to be factually wrong; the API supports it, only the client doesn't wire it up. |
| **640px cap is real, inclusive, per-dimension** | `width`/`height` > 640 → `400`: `"Image width must be an integer between 1 and 640px"` / `"Image height must be an integer between 1 and 640px"` (checked independently, own message each). Exactly `640` is accepted. Confirmed identical on both `PATCH /v1/posts/:id` and `POST /v1/replies` (create). |
| **`width`/`height` are required for `type: "image"`, despite the `?` in `/types.d.ts`** | Omitting either field entirely (not just setting it small) produces the same `400` as an out-of-range value. The type file's optionality marker is inaccurate — both dimensions are mandatory on the wire for a `type: "image"` attachment. |
| **`width`/`height` are ignored (and unchecked) for `type: "audio"`** | A `type: "audio"` attachment (`{src, origin: "youtube", artist, title, genre}`) with **no** `width`/`height` keys at all was accepted and stored cleanly — the 640px dimension validation is `type`-conditional, image-only. Also confirmed live in the same test: `hasAudioAttachment: true` and `audioAttachmentGenre` (both undocumented in `docs.md`, present in `/types.d.ts`) round-tripped correctly on `GET`. |
| **The 640px check is metadata-only — the server never fetches the image** | Sent `width: 10, height: 10` with `src` pointing at a real 2000×2000 image → accepted with no error, and `GET` echoed the lied-about `10x10` dimensions next to the real (much larger) URL. The cap is honesty-based: a client that declares small dimensions bypasses it entirely, since nothing server-side verifies the claim against the actual file. **As of 2026-08-27, `resolveAttachment` (`internal/ui/app.go`) deliberately exploits this** — see the `⚠ EXPERIMENTAL` section of `docs/50-post-reply-attachments.md` — always declaring `640x640` regardless of the real image, at the user's explicit request, rather than the honest fetch-and-report behavior this row originally described. |
| **`type` union is `'audio' \| 'image'` only — no `'gif'`** | `attachmentTypeForURL` (`internal/ui/app.go:5137`) currently sends the literal string `"gif"` as an attachment's `type` for post/reply attachments with a `.gif`-extension URL. Not spot-checked against the live server (no test posted a `.gif` URL as a post/reply attachment this session) — worth confirming whether the server accepts `"gif"` as an undocumented extension of the union or silently coerces/rejects it. |
| **`hasImageAttachment`/`hasAudioAttachment` are computed for posts, not replies** | Confirmed live 2026-08-27: a post with a real `attachments` entry returns `hasImageAttachment: true` (or `hasAudioAttachment: true`) automatically — never sent by the client, so it's server-derived at write time. A reply with an identical `attachments` entry returns *neither* flag, and explicitly sending `hasImageAttachment: true` in the `POST /v1/replies` body is silently ignored (not present in the response either — confirmed the field isn't just omitted-when-false, it's genuinely absent). `/types.d.ts` declares both flags on `Reply` too, so this looks like the derivation step was simply never wired up for the reply write path. **Likely explains a live report**: an image attached to a reply via `ctrl+g` (feature 50) was stored correctly (`attachments` populated, confirmed via `apifetch`) but did not render on the website — if the site's reply view keys off `hasImageAttachment` rather than checking `attachments` directly, it would never fire for a reply. Unconfirmed whether that's the actual website mechanism; the flag gap itself is confirmed. |
| **Reply permalink shows `undefined` in place of username** | Observed 2026-08-27 on a reply detail URL (`https://cyberspace.online/undefined/<post-slug>?reply=<id>`) — the reply's own `authorUsername` was populated correctly server-side (confirmed via `apifetch`), so this looks like a website-side link-building bug (a JS template picking up a missing value at render time) rather than anything server- or cyber-tui-side. Not investigated further — cyber-tui has no website code to inspect. |

### Guilds (new in v0.4)

Guilds are member groups with their own forum of threads. A user can belong to one guild at a time. `Guild` model type added; read-only browsing implemented in feature 29.

| Endpoint | Method | Description | Status |
|---|---|---|---|
| `/v1/guilds` | GET | List guilds (paginated, most-populated first) | **Done** — feature 29 |
| `/v1/guilds/:slug` | GET | Get guild detail + caller's `isMember` / `role` | **Done** — fetched alongside thread list; used for membership hint bar |
| `/v1/guilds/:slug/members` | GET | List guild members (paginated, oldest-joined first) | **Done** — feature 29 |
| `/v1/guilds/:slug/posts` | GET | List guild threads (most recently active first) | **Done** — feature 29 |
| `/v1/guilds/:slug/posts` | POST | Create guild thread (title + topics supported) | **Done** — feature 29 |
| `/v1/guilds/:slug/join` | POST | Join a guild (one per user; 409 if already in one) | **Done** — `J` key in guild threads view |
| `/v1/guilds/:slug/leave` | POST | Leave a guild (founders blocked via API; 403) | **Done** — `l` key in guild threads view; founders see no action key |

Notes:
- Guild threads are ordinary posts with `guildId`, `guildSlug`, `isGuildThread: true`; replying uses `POST /v1/replies` as normal.
- `guild_new_thread` notifications are display-ready (`notifSummary`/`notifIcon` have explicit cases) and navigation is wired via `TargetID` → `ShowNotificationPostMsg`.
- Notification metadata for guild **replies/posts** uses `metadata.guildSlug` + `metadata.isGuildThread: true` (observed 2026-06-03), **not** `metadata.guildName`. The client decodes `guildSlug` and shows `in #<slug>` on `reply`/`thread_reply`/`new_post_*` (prefers slug over the rarer `guildName`). As of API **v0.5.0** the server documents the notification object and its `metadata` keys (incl. `guildSlug`, `guildName`, `isGuildThread`, `threadId`, `postSlug`, `authorUsername`), closing the earlier doc gap; the client's slug-preference behavior matches the documented schema.
- The `isMember` / `role` fields on `GET /v1/guilds/:slug` were broken in v0.4 but are **fixed in v0.4.1** (verified 2026-06-01 — see Resolved Issues). The `GetGuild()` client method could now be called to read accurate membership state, though the current `User.GuildSlug` approach also works.
- Join/leave are now official v0.4.1 API endpoints. Any authenticated user can also create a guild thread without being a member (explicitly stated in v0.4.1 spec).
- v0.4.1 adds `profilePictureUrl` to both the guild list response and the guild members list response. `Guild` and `GuildMember` model types and wire layer now carry this field (v0.7 alignment); rendering is deferred until imgview support lands in the guild list.

### Guilds (new in v0.4.1)

| Area | Description | Priority |
|---|---|---|
| `profilePictureUrl` on Guild / GuildMember | v0.4.1 adds this field to the guild list response and the member list response. Captured in model and wire layer; rendering deferred (no imgview in guild list yet). | **Done** — v0.7 alignment |
| Guild join (`POST /v1/guilds/:slug/join`) | Now an official API endpoint. One guild per user; 409 if already in one. | **Done** |
| Guild leave (`POST /v1/guilds/:slug/leave`) | Now an official API endpoint. Founders get 403 — must use web. | **Done** |

### Guilds — apprenticeships (new in v0.8.6)

A user's own guild (founder/member, still one at a time — the profile badge) can now be supplemented with up to 5 "apprenticeships" (role `apprentice`) in other guilds. Apprentices appear in the guild's member list and get its thread notifications, but the badge only follows the founder/member guild.

| Endpoint | Method | Description | Status |
|---|---|---|---|
| `/v1/users/:username/guilds` | GET | All guilds a user belongs to (badge guild first, apprenticeships oldest-first), max 6, unpaginated | **Done** — feature 29 (apprenticeships), profile Info tab |
| `/v1/guilds/:slug/promote` | POST | Make an apprenticeship the new badge guild | **Done** — `P` key in guild threads view |
| `/v1/guilds/:slug/join` | POST | Changed: no longer 409s just because the caller is already in a guild — joins as apprentice instead. 409 only for "already in this guild" or "already 5 apprenticeships" | **Done** |
| `/v1/guilds/:slug/leave` | POST | Changed: leaving an apprenticeship no longer touches the badge; leaving the badge guild clears it with no auto-promote | **Done** |

Notes:
- `Guild.apprenticeCount` added to the guild list/detail response; missing on guilds that predate apprenticeships — client treats missing as 0.
- The 5-apprenticeship cap and duplicate-membership checks are enforced server-side only; the client does not pre-fetch a count to gate the `J` key, matching the existing pattern of leaving founder-leave 403s to the server.

### C-Mail (new in v0.7)

All REST endpoints and the RTDB SSE subscription are fully implemented. See `docs/08-cmail.md` for details.

| Endpoint | Method | Description | Status |
|---|---|---|---|
| `POST /v1/cmail` | POST | Start or get a conversation by `recipientUsername` (idempotent) | **Done** — `StartConversation` |
| `GET /v1/cmail` | GET | List conversations (unread first, then newest activity) | **Done** — `GetConversations`; populates `UnreadCount`, `LastMessage` |
| `GET /v1/cmail/:conversationId` | GET | Load message history | **Done** — `GetMessages` |
| `POST /v1/cmail/:conversationId` | POST | Send a message | **Done** — `SendMessage` |
| `POST /v1/cmail/:conversationId/read` | POST | Mark conversation as read | **Done** — `MarkCMailRead`; called on conversation open |
| RTDB `dm_messages/<conversationId>` | SSE | Real-time new messages | **Done** — `SubscribeDMs`; skips initial snapshot |
| RTDB `user_conversations/<uid>` | SSE | Live conversation list / unread updates | **Done** — `SubscribeUserConversations`; replaces the old 60s REST poll (`docs/08-cmail.md`, `docs/09-rtdb-cmail.md`) |
| `POST /v1/cmail/:conversationId/messages/:messageId/flag` (or similar) | POST | Report a CMail message for review | **Missing** — no such endpoint in the API. cIRC has `POST /v1/circ/:roomId/messages/:messageId/flag`, but nothing analogous exists for CMail. Per-message browsing/selection (`docs/08-cmail.md`) deliberately stops short of adding a flag action for this reason — wire it up if the API adds one. |
| `DELETE /v1/cmail/:conversationId/messages/:messageId` (or similar) | DELETE | Soft-delete a CMail message | **Missing** — same as above; cIRC has room-message delete, CMail has no equivalent. |

### cIRC (new in v0.7)

cIRC REST API is now fully documented. A room is addressed by its `roomId` (slug, e.g. `general`). Real-time reading uses Firebase RTDB SSE.

| Endpoint | Method | Description | Status |
|---|---|---|---|
| `GET /v1/circ` | GET | List rooms available to you (sorted by `sortOrder`, then newest activity) | **Done** — feature 33 |
| `GET /v1/circ/:roomId` | GET | Load room message history (paginated, oldest-first, `before` cursor) | **Done** — feature 33 |
| `POST /v1/circ/:roomId` | POST | Send a message to a room (supports slash commands) | **Done** — feature 33 |
| `POST /v1/circ/:roomId/read` | POST | Mark room as read (drives "new messages" indicator) | **Done** — feature 33 |
| RTDB `chat_messages/<roomId>` | SSE | Subscribe to real-time new messages | **Done** — feature 33 |
| `GET /v1/circ/:roomId/users` | GET | List who's currently in a room | **Done** — cIRC presence |
| `POST`/`DELETE /v1/circ/:roomId/presence` | POST/DELETE | Announce/heartbeat and leave presence | **Done** — cIRC presence |
| RTDB `chat_presence/<roomId>` | SSE | Subscribe to real-time presence changes | **Done** — cIRC presence |

Notes:
- Each room message includes `isChatAdmin` flag — parsed into `model.Message.IsChatAdmin`. No longer shown as a `[admin]` badge on the message line; admin status now lives only in the online-users side panel (see `docs/33-circ.md`).
- Rate limits: 15 sends/min, 300/day, 150/hour; 60 mark-read/min; 60 list-users/min; presence heartbeat/leave 15/min per room (90/min overall, as of v0.8.1 — previously a flat 30/min).
- 403 if room isn't available to you.
- Online-users list: implemented (cIRC presence) — `GET .../users` for the initial list, `chat_presence` RTDB stream for live updates, `POST`/`DELETE .../presence` for announce/heartbeat/leave. Room-list cards also show `onlineCount` from `GET /v1/circ`.
- Slash command rendering: server expands `/me`, `/poke`, `/dice` etc. server-side; no client-side preview yet.
- **Fixed: a single user leaving a room used to wipe the entire presence sidebar.** Confirmed via the `cfg.Debug` raw-event logging: Firebase frames a single user's removal from `chat_presence/<roomId>` as a **`patch`** event at path `/` with just that one key set to `null` (a multi-location update — "touch only these keys"), not a full-snapshot replace. `applyPresenceEvent` (and the identical `applyTypingEvent` for C-Mail's typing indicator) treated *any* event at path `/` — `put` or `patch` — as a full wipe-and-replace, so one person leaving cleared every other online user out of the local state; everyone else then only reappeared as their own next individual heartbeat re-added them — the exact "whole sidebar disappears, then trickles back in" symptom reported. Fixed by distinguishing `put` (genuine full snapshot — replace) from `patch` (merge only the listed keys, `null` deletes that key) at path `/`; the (also unconfirmed at the time) `@mention`-correlation was coincidental — the real trigger is simply anyone leaving the room. See `TestHTTPSubscribeRoomPresence_RootPatchMergesInsteadOfReplacing`/`TestHTTPSubscribeDMTyping_RootPatchMergesInsteadOfReplacing` in `client_test.go`. The temporary `cyber-tui-debug.log` raw-event logging (`internal/api/client.go`'s `SubscribeRoomPresence`, wired in `main.go`) is left in place behind `cfg.Debug` since it proved useful and costs nothing when off.

### Search (new in v0.7)

| Endpoint | Method | Description | Status |
|---|---|---|---|
| `GET /v1/search?q=<query>&type=all` | GET | Full-text search across users, posts, and replies — grouped preview | **Done** — feature 34, `Search()` |
| `GET /v1/search?q=<query>&type=posts\|replies\|users` | GET | Paginated single-category search | **Done** — feature 34, `SearchPosts`/`SearchReplies`/`SearchUsers()` |

Notes:
- `type=all` returns up to 8 hits per group (users/posts/replies), no pagination, no total count. The client treats "exactly 8 hits" as the only available signal that a category may have more — see `docs/34-search.md`.
- `type=posts|replies|users` returns paginated results; the client sends `page` (0-based) and treats the response `cursor` (next page number, or null) as an opaque cursor string, same as every other paginated endpoint — no special-casing needed.
- Search hits reuse the existing `model.User`/`model.Post`/`model.Reply` types; no dedicated hit types were needed. The doc-mentioned extra reply-hit context (`parentPostAuthor`/`parentPostContent`) and user-hit guild fields were not captured — not needed for the current UI (reply hits navigate to the parent post directly; user hits already carry guild fields via the existing `User` type).
- Rate limit: 30/min. Missing `q` → 400 VALIDATION_ERROR.

### Commands (new in v0.7)

Both cIRC and C-Mail support IRC-style slash commands expanded server-side: `/me`, `/poke`, `/hug`, `/hi5`, `/slap` (with optional `[@user]`), `/dice <notation>`, `/8ball <question>`, `/fortune`, `/help`. Malformed commands return 400. `/help` posts nothing; its `{ data: { reply } }` is captured by `SendRoomMessage`/`SendMessage` (**Done**) and shown as a local system notice — see `docs/33-circ.md` / `docs/08-cmail.md`.

`/me` and the other emotes set an `isAction` field (with `content` stripped of the username). Previously observed only via live-testing; as of v0.8 this is officially documented under [Message fields](#message-fields) in `docs/00-latest-api-reference.md`, along with `isDice`, `isEightball`/`eightballAnswer`, `isFortune`/`fortuneText`. `model.Message.IsAction` (**Done**) renders these as classic IRC `* username body *` lines.

### Flagging / Reporting (new in v0.8)

`POST /v1/posts/:id/flag`, `POST /v1/replies/:id/flag`, `POST /v1/circ/:roomId/messages/:messageId/flag` — report content for review. Idempotent (200 + `alreadyFlagged` on repeat), optional `reason` (max 500 chars), can't flag your own content, no way to withdraw. Shared rate limit: 5/min, 20/hour, 50/day.

| Endpoint | Method | Description | Priority |
|---|---|---|---|
| `/v1/posts/:id/flag` | POST | Report a post | **Done** — `!` key in Feed and Post Detail; see `docs/35-flagging.md` |
| `/v1/replies/:id/flag` | POST | Report a reply | **Done** — `!` key in Post Detail; see `docs/35-flagging.md` |
| `/v1/circ/:roomId/messages/:messageId/flag` | POST | Report a cIRC message | **Done** — `!` while browsing messages (`up`/`down`); see `docs/36-circ-message-flagging.md` |

### cIRC message delete (new in v0.8)

| Endpoint | Method | Description | Priority |
|---|---|---|---|
| `/v1/circ/:roomId/messages/:messageId` | DELETE | Soft-delete own cIRC message (`content` → `[DELETED]`, attachments stripped); arrives to other clients as an RTDB `patch`, not a new message | **Done** — `d` while browsing messages (own messages only), with live propagation to other clients via the now-handled RTDB `patch` event; see `docs/36-circ-message-flagging.md` |

### Message attachments & styles (new in v0.8)

cIRC/C-Mail messages can now carry `imageUrl`, `gifUrl` (`/gif <url>`), `audioAttachment` (`/song ... — supporter-only`), `style` (chainable text styles via `/blink`, `/l33t`, `/comic`, `/cursive`, `/times`, `/rainbow`, `/flip`, `/quiet`, `/slow`, `/glitch`, `/spoiler`, `/wave`), and ASCII art (`/art`, cIRC-only, base64-encoded `content` when `style: "art"`). `/mute`/`/unmute`/`/muted`/`/unmuteall` manage a per-room, client-side-enforced mute list (also stored in `mutedUsersByRoom` under Settings — currently intentionally omitted from the TUI per the Settings row below).

| Area | Description | Priority |
|---|---|---|
| `gifUrl`, `audioAttachment`, `style`, chained styles | Render/decode in message view; `style: "art"` needs base64 decode | **Done** — wire/model fields across all four message shapes, attachment badges reusing `renderAttachments`, and a middle-fidelity style pipeline (ANSI attributes for blink/quiet/rainbow, Unicode substitution for l33t/cursive/flip, ASCII-safe jitter for glitch, `tea.Tick`-driven slow/wave/glitch animation, select-to-reveal spoiler in cIRC only — see `internal/ui/screens/chatstyle.go`) |
| `/mute` family + `mutedUsersByRoom` | Client-side message filtering by muted user | **Done** — cIRC only (C-Mail 400s per API spec); see `docs/37-circ-mute.md` |
| Empty `content` with attachment-only messages | Message rendering must not assume non-empty `content` | **Done** — covered by the same change; `messageDisplayBody` skips duplicate URL text and empty bodies render without assuming non-empty `content` |
| `/song` composer UI | A guided way to build the `/song <url> \| <artist> \| <title> [\| <genre>]` command instead of hand-typing it | **Done** — `ctrl+j` Song Prompt modal in cIRC (supporter-gated client-side ahead of the server's 403), artist/title auto-filled via YouTube's public oEmbed endpoint (`internal/youtube`); see `docs/49-song-attach.md`. Extended to Feed/Post Detail's native audio attachment (feature 50, `docs/50-post-reply-attachments.md`). C-Mail's own composer still not covered (out of scope, though the API accepts `/song` there too) |

### cIRC idle presence (new in v0.8.1)

`GET /v1/circ/:roomId/users` and the `chat_presence/<roomId>` RTDB stream now carry `lastActivity` (ms epoch, or `null`) per user. `POST /v1/circ/:roomId/presence` accepts an optional `{ "lastActivity": <ms epoch> }` body and its response gains `idleAfterMs`. A user is idle once `lastActivity` is older than `idleAfterMs`; the website shows a 💤 badge. Also reworked: C-Mail typing on/off is now rate-limited 40/min per conversation (120/min overall) rather than a flat 45/min, and cIRC presence heartbeat/leave is 15/min per room (90/min overall) rather than a flat 30/min.

| Area | Description | Priority |
|---|---|---|
| `lastActivity`/`idleAfterMs` idle tracking | Send `lastActivity` on every presence heartbeat (tracked from any keypress while a room is open), plus an extra out-of-cycle, cooldown-guarded heartbeat on every keypress that finds the panel currently showing our own entry as idle (`ChatroomsModel.selfShownIdle`) — corrects a stale server-recorded `lastActivity` immediately rather than waiting for the next scheduled beat. Going idle needs no push of its own; the server computes it passively from the aging last-reported timestamp. Decode `lastActivity` from `GET .../users` and the `chat_presence` RTDB stream (nil = always active). Render a 💤 badge for idle users in the online-users panel, computed at render time off `idleAfterMs` — idle users are never filtered out of the list, only flagged. See `docs/33-circ.md`. | **Done** — 2026-08-03; corrected 2026-08-03 (self-idle badge could get stuck showing idle while actively typing — see `docs/33-circ.md`'s "Waking from idle" bullet) |

### TypeScript definitions (new in v0.8.4)

`/types.d.ts` now publishes TypeScript types for every documented response shape. Not consumed programmatically by this Go client, but proved genuinely useful as a documentation source on 2026-08-27: `docs.md`'s prose never spells out the `attachments` object shape or mentions it on Replies at all, while `/types.d.ts`'s `Attachment`/`Entry`/`Reply` interfaces filled both gaps (and turned up two fields — `hasImageAttachment`/`hasAudioAttachment`/`audioAttachmentGenre` — not yet modeled client-side; see the Attachments section above). Worth checking again whenever a field's real shape is undocumented in prose.

### Programs (new in v0.8.7)

Feasibility notes and a tiered scope (registry browse/publish vs. running `wasm`
programs locally vs. `web`/`term`, which isn't realistically supportable —
their `ctx`/`p` runtime-contract shape is undocumented) are written up in
`docs/plan-programs-integration.md`. Not started; revisit if prioritized.

| Endpoint | Method | Description | Priority |
|---|---|---|---|
| `/v1/programs` | GET, POST | New terminal program registry — `web`/`term`/`wasm` runtimes, backs the website `/terminal`'s `publish`/`browse` gallery. Browse (with `runtime`/`author`/`name`/`mine` filters), publish (tiered size/count limits by account tier), recall/purge. | Not evaluated — no `cmd/`-side terminal-program concept exists in cyber-tui today; unclear whether this is in scope for a TUI social client at all. Revisit if a user asks. |
| `/v1/programs/:id/source` | GET | Read a program's source at its current release or an earlier one by number; `base64` encoding for `wasm`, `utf8` otherwise. | Same as above |
| `/v1/programs/:id` | DELETE | Recall (soft) or purge (hard, frees the account's program-count slot) a published program. | Same as above |

### Auth (new in v0.8)

| Error | Description | Priority |
|---|---|---|
| `403 EMAIL_NOT_VERIFIED` | New error code — account access now gated on email verification instead of supporter/API-access-grant. Surfaces from the profile fetch immediately after a successful login (login itself doesn't gate on this); the login screen shows a distinct message with an `r` keybinding to call `POST /v1/auth/resend-verification`, and `friendlyErr` softens the same code for any mid-session authenticated call. See `docs/38-email-verification.md`. | **Done** — 2026-08-03 |

### Thread Watching (new in v0.5.1)

Watching a thread means you receive `thread_reply` notifications when anyone replies to it. Posting a reply auto-watches the thread (controlled by the `autoWatchOnReply` setting, default on).

| Endpoint | Method | Description | Priority |
|---|---|---|---|
| `GET /v1/posts/:id/watch` | GET | Check whether the current user is watching a thread | **Done** |
| `POST /v1/posts/:id/watch` | POST | Watch a thread (idempotent; rate limit: 10/min, 100/day) | **Done** |
| `DELETE /v1/posts/:id/watch` | DELETE | Unwatch a thread | **Done** |
| `GET /v1/watches` | GET | List watched threads — used at startup to populate `◉` icons | **Done** |

Notes:
- `w` key in feed and post detail (root post only) toggles watch with optimistic update. `◉` icon displayed in feed, post detail, guild threads, and topics.
- All pages of `GET /v1/watches` are fetched progressively at login; icon set updates after each page.
- A dedicated "Watched Threads" screen (similar to bookmarks) remains a future low-priority option.
- The `autoWatchOnReply` settings field (v0.5.1) is already surfaced in the Settings screen.
- **Feed filter (Index/Watching), considered and deferred (2026-08-17):** a Feed-screen filter mirroring the Notifications category filter's single-select/live-apply UX (`docs/15-notifications.md`) was proposed with two options — `Index` (today's default, unfiltered feed) and `Watching`. `Index` is trivial (the existing `GetFeed` call, no change needed). `Watching` is blocked: `GET /v1/watches` returns only `{id, postId, createdAt}` (`wireWatch`, `internal/api/client.go:263-267`) — no embedded post content — unlike `GET /v1/bookmarks`, which embeds a full `post`/`reply` object per item (`wireBookmark.Post *wirePost`, `internal/api/client.go:253-261`), which is exactly why Bookmarks works as a clean single-fetch screen and Watching doesn't. Building it today would mean paging `GET /v1/watches` and resolving each page's `postId`s via individual `GetPost` calls — an N+1 pattern burning ~20 requests per page of watched threads with no clean cursor pagination over real post content, too API-intensive to build now. **Revisit once `GET /v1/watches` (or a new batch-fetch endpoint) embeds post content the way `GET /v1/bookmarks` does.** `Friends`/`Following` and `Guild` feed-filter options were also considered and ruled out entirely (not deferred, not buildable): the API has no endpoint to scope the main feed by follow-relationship or guild membership — `GET /v1/follows` only lists follow *relationships*, not a feed of their posts, and guild content is only reachable per-guild via `GET /v1/guilds/:slug/posts`.

### Notifications (new in v0.4.1)

| Area | Description | Priority |
|---|---|---|
| `type` filter on `GET /v1/notifications` | `?type=reply,reply_mention` — comma-separated list of notification types to fetch. API param is already wired client-side (`GetNotifications(..., types []string)`); the server-side `500` bug that blocked it is now fixed (confirmed live 2026-08-13, see Known API Issues above). A category filter panel (`f` key on the Notifications screen) groups the 30 known types into 5 categories (mentions/social/threads/c-mail/account-system) plus `all`, server-side filtered, not persisted across sessions (deliberate — keeps the tab badge's global unread count from silently diverging from a stale filtered view). Initial design (2026-08-17) was multi-select/checkbox with a commit-on-enter step; hit the API's 20-type cap on `type` live (`account/system` + `threads` = 21 → `400 VALIDATION_ERROR`) since a combination of categories could exceed it. Redesigned same-day to **single-select, live-apply** (cursor move immediately re-filters, `esc` reverts to the pre-open category — mirrors the theme picker's live-preview pattern) at the user's request, which structurally eliminates the cap issue: only one category's types are ever sent at once, and the largest (`account/system`, 16) is safely under 20 — the guard code was removed as dead rather than left unreachable. See `docs/15-notifications.md`. | **Done** — 2026-08-17 |
| `chat_mention` / `dm_message` navigation and context | `chat_mention` now carries `metadata.roomSlug`/`roomName`/`messageContent` (confirmed live 2026-07-24) — captured as `Notification.RoomSlug`/`RoomName`/`MessageContent`, shown as an inline `#room` summary + message preview, and `enter` jumps straight to the room (`OpenRoomMsg`). `dm_message` reuses the existing `StartConversationMsg` conversation-open flow on `enter` (same as the `c` key) rather than adding new metadata fields, since no live `dm_message` example has been observed to confirm its metadata shape. | **Done** — see `docs/15-notifications.md` |
| `dm_message` content preview | No confirmed metadata field carries message content/conversation ID for `dm_message` (unlike `chat_mention`'s `messageContent`). If a future live sighting reveals one, add a `docs/15-notifications.md`-style inline preview matching `post_mention`/`reply_mention`/`chat_mention`. | Low — blocked on live confirmation |

### Notifications (new in v0.8.5)

| Area | Description | Priority |
|---|---|---|
| 8 new notification types | `graffiti_mention`, `moderator_granted`/`moderator_removed`, `api_access_granted`/`api_access_removed`, `system_ban_lifted`, `post_cooldown`, `rate_limit_warning` — `notifSummary`/`notifIcon` cases added in `notifications.go`. `post_cooldown` is now confirmed against a real live payload (`?type=post_cooldown`, see the narrowed row below) — icon/text/`hasActor`/`Reason` handling all match. The remaining 6 are inherently rare/admin-triggered and still unverified against a live payload. | **Done** — 2026-08-13 |
| `actorId`/`actorUsername` optional, or literal `"system"` | `model.Notification` gains a `Reason` field (top-level `reason` on the wire, not in `metadata`); a new `hasActor` helper (`internal/ui/screens/notifications.go`) treats an empty or literal-`"system"` `actorUsername` as no actor. `renderNotif` omits the `@handle` and shows `Reason` as the inline preview instead; the `p`/`c` key handlers no-op rather than opening a profile/conversation for "system". | **Done** — 2026-08-13 |
| `unread-count` gains `exact: false` above 100 unread | `GetUnreadNotificationCount()` now returns `(count, exact, error)`; `App.polledUnreadCountExact` tracks it and both badge render sites (`layout_tabs.go`, `layout_miller.go`) go through a shared `notifBadgeText` helper that renders "99+" when inexact. | **Done** — 2026-08-13 |
| `read-all` gains `hasMore` | `MarkAllNotificationsRead()` now returns `(hasMore, error)`; `markAllNotifsReadCmd` loops (bounded at `markAllNotifsReadMaxCalls` = 20, as a defensive cap against a hypothetical server bug) while `hasMore` is `true`. Still fire-and-forget from the UI's perspective — optimistic update already applied before the command runs. | **Done** — 2026-08-13 |
| `GET /v1/notifications` documented as excluding muted/blocked/off types | Verified non-issue: `fetchPage` (`internal/api/client.go`) and both `NotificationsModel.SetNotifs`/`AppendNotifs` (`internal/ui/screens/notifications.go`) already key pagination off `cursor == ""`, never off page length vs `limit` — the client was already correct, this row is closed as a documentation-only clarification. Explains (and supersedes) the count/list skew logged 2026-08-03 above. | **Done (verified, no code change)** — 2026-08-13 |

### Replies

| Endpoint | Method | Description | Priority |
|---|---|---|---|
| `/v1/posts/:postId/replies` | GET | Cursor-paginated replies (oldest-first by reply ID) | Deferred — replies are rendered as a tree grouped by `parentReplyId`; paginated loads arrive interleaved across parent/child, requiring tree re-parenting and a full re-render on each page. Cost outweighs benefit at current network scale. |
| `POST /v1/replies` — `attachments` field | Live-confirmed 2026-08-27 (see Attachments section above): replies accept an `attachments` array identically to posts, undocumented in `docs.md` but present in `/types.d.ts` and functional. `ctrl+g` (image/gif) and `ctrl+j` (audio/song) both now attach natively on the reply compose box — feature 50, `docs/50-post-reply-attachments.md`. | **Done** — feature 50 |

### Follows

| Endpoint | Method | Description | Priority |
|---|---|---|---|
| `/v1/follows?type=followers` | GET | List the current user's followers | **Done** — profile Followers tab |
| `/v1/follows?userId=...` | GET | Look up followers/following for another user | **Done** — profile Following/Followers tabs |

### Notes

| Endpoint | Method | Description | Priority | Blocker |
|---|---|---|---|---|
| `/v1/notes/:id` | GET | Fetch a single note (optionally a specific revision) | **Done** — used by revision preview |
| `/v1/notes/:id/revisions` | GET | List all revisions for a note | **Done** — journal `h` key |
| `/v1/notes/:id` | PATCH | Update note | **Done** | Fixed in v0.4; re-tested 2026-05-29 |

---

## Partially Implemented Features

| Feature         | What's Done                           | What's Missing                                                       |
| --------------- | ------------------------------------- | -------------------------------------------------------------------- |
| Notes (Journal) | List, create, edit, delete, revision history | — |
| Profile         | View and edit all fields              | —                                                                    |
| Settings        | All TUI-relevant fields editable; `mutedUsersByRoom` read and enforced (feature 37) | `keyboardBindings`, `keyboardPreset` — web UI concepts with no TUI equivalent; intentionally omitted |
| Follows         | Follow, unfollow, list following and followers | — |
| Notifications   | All v0.4 types received and displayed with dedicated text and icons; unread-only toggle (`u`); category filter panel (`f`) | — |

---

## Resolved Issues

| Endpoint | Description | Resolved |
|---|---|---|
| `GET /v1/guilds/:slug` — `isMember`/`role` | Server-side bug: always returned `false`/`null` in v0.4. Fixed in v0.4.1 — verified 2026-06-01: `isMember: true`, `role: "member"` returned correctly for authenticated member of `technica`. TUI `GetOwnProfile`→`guildSlug` workaround stays as belt-and-braces. | 2026-06-01 |
| v0.4.1 notification types | All 9 new types (`supporter_granted/removed`, `hacker_granted/removed`, `image_permission_granted/removed`, `attachment_permission_granted/removed`, `system_ban`) have dedicated text (`notifSummary`) and icons (`notifIcon`) in `notifications.go`. Implemented prior to v0.4.1 doc update. | — |
| `DELETE /v1/posts/:id` | Delete own post — wired in client, feed, and post detail; `d` key with y/n confirmation | 2026-04-16 |
| `DELETE /v1/replies/:id` | Delete own reply — wired in client and post detail; `d` key with y/n confirmation | 2026-04-16 |
| `PATCH /v1/users/me` (extended) | `websiteName`, `websiteImageUrl`, `locationLatitude`, `locationLongitude` added to model, wire layer, and profile edit form (`e` key) | 2026-04-16 |
| `GET /v1/users/:username/posts` | User post history — profile Posts sub-tab; `tab` to navigate; feature 24 | 2026-04-17 |
| `GET /v1/users/:username/replies` | User reply history — profile Replies sub-tab; feature 24 | 2026-04-17 |
| `GET /v1/follows?type=followers` | Own followers list — profile Followers sub-tab; feature 24 | 2026-04-17 |
| `GET /v1/follows?userId=…` | Any user's follows — profile Following/Followers sub-tabs; feature 24 | 2026-04-17 |
| `GET /v1/notes/:id` | Single note fetch — used for revision preview; feature 25 | 2026-04-17 |
| `GET /v1/notes/:id/revisions` | Note revision history — journal `h` key; feature 25 | 2026-04-17 |
| `PATCH /v1/notes/:id` | Server-side 500 bug resolved in API v0.4. Note editing and revision history fully operational. | 2026-05-29 |
| `POST /v1/posts` (extended) | `CreatePost` now accepts `title`, `isPublic`, `isNSFW`. `Post` model gained `Title`, `Slug`, `GuildID`, `GuildSlug`, `IsGuildThread`. Title rendered in feed/detail/profile/bookmarks. Feature 28. | 2026-05-29 |
| Login / refresh — `rtdbUrl` (v0.7) | Login and token-refresh responses now return `rtdbUrl` (e.g. `https://…europe-west1.firebasedatabase.app`). Previously the RTDB URL was derived from the JWT's project ID, producing the wrong `firebaseio.com` regional domain. `Tokens` model gains `RTDBUrl`; `InitRTDB()` now takes the URL from the API response; `applyRefresh()` also updates `rtdbClient.token` via `SetToken()`. | 2026-07-20 |
| Notification metadata — `postContent` / `replyContent` (v0.7) | `post_mention` and `reply_mention` notifications now carry `postContent` and `replyContent` inline, eliminating the need for a follow-up `GET /v1/posts/:id` round trip. `Notification` model gains `PostSlug`, `PostAuthorUsername`, `PostContent`, `ReplyContent`; content preview is rendered inline in the notification list row. | 2026-07-20 |
| `GET /v1/guilds/:slug/members` | Guild member list — paginated, oldest-joined first; `m` from guild posts view; `enter` navigates to profile. Feature 29. | 2026-05-30 |
| `GET /v1/guilds/:slug` (join/leave flow) | Guild detail (`isMember`, `role`) fetched alongside thread list. `J` to join, `l` to leave with y/n confirmation and membership hint bar. Feature 29. | 2026-06-01 |
| `POST /v1/guilds/:slug/join` | Join guild — `J` key in guild thread feed with confirmation prompt; success banner "✓ Joined #name". Feature 29. | 2026-06-01 |
| `POST /v1/guilds/:slug/leave` | Leave guild — `l` key in guild thread feed with confirmation prompt; success banner "✓ Left #name"; navigates back to guild list. Feature 29. | 2026-06-01 |
| Attachments (image/audio) | Attachment URLs surfaced via `GetFocusedURLs` and opened with `o`. Best-effort handling for a TUI — no further work needed. | 2026-05-30 |
| `POST /v1/posts`/`/v1/threads`/`/v1/notes` `topics`, and `slug` | Client-side fix, not a server bug: `topics` is documented as "must be lowercase" and `slug` as `[a-z0-9-]`, but neither field restricted input client-side — an uppercase or punctuation-laced tag round-tripped to the server and came back a `400 VALIDATION_ERROR`. Compose's slug and topics inputs, and journal's topics input, now filter keystrokes live (`filterSlugCharsKeyMsg`/`filterTopicsKeyMsg`, `internal/ui/screens/shared.go`): uppercase auto-lowercases, any character outside `[a-z0-9-]` (plus `, ` as topic delimiters) is dropped as typed. Topics' existing 3-item cap (`ParseTopics`) is now also enforced live — a comma that would open a 4th topic is blocked — instead of only silently truncating at submit. Closes #94 ("Posts with uppercase tags cause API error"). | 2026-08-07 |
| `POST /v1/users/:username/poke` (v0.8.4) | Send a poke — `p` key on a read-only profile; brief toast feedback; suppressed on own profile. 429 (1/hour, 8/day cap) and 403 (blocked either direction) get friendly banners instead of raw API error text. Feature 42. | 2026-08-12 |
| `PATCH /v1/posts/:id`, `PATCH /v1/replies/:id` (v0.8.4) | Edit own post (content/title/topics/public/NSFW) or reply (content only) — `e` key on Feed and Post Detail, gated to own content, supporter account, within 5 minutes of publishing; hint dynamically shown/hidden per-selection in the status bar. 403 (outside window or not a supporter) gets a friendly banner. Feature 43. | 2026-08-12 |
