# API v0.7 Alignment

Maintenance pass aligning the TUI client with the cyberspace.online API v0.7 release. The TUI was built against ~v0.51; this session closes the gap for all existing features. New v0.7 features (C-Mail, cIRC, Search) are tracked in `docs/00-api-backlog.md` and addressed separately.

---

## MUST_UPDATE — Latent Bugs Fixed

These bugs were invisible because the affected features (C-Mail, cIRC) are still stubs. They would have broken silently the moment those features landed.

### 1. RTDB URL now from API response, not JWT

**Bug:** `InitRTDB()` derived the Firebase base URL by decoding the RTDB JWT and constructing `https://{projectID}-default-rtdb.firebaseio.com`. The real URL is `https://cyberspace-cyberspace-default-rtdb.europe-west1.firebasedatabase.app` — a different regional domain entirely.

**Fix:**
- `loginResponseData` and `refreshResponseData` in `client.go` now decode `rtdbUrl`
- `model.Tokens` gains `RTDBUrl string`
- `InitRTDB(token, rtdbUrl string)` — second parameter added; URL comes from the API response
- `internal/rtdb/jwt.go` deleted; `ParseRTDBToken()` and `BaseURL()` are no longer used
- `internal/rtdb/client.go` — `SetToken(token string)` added (mutex-protected) so `applyRefresh()` can push renewed tokens into the live RTDB client

**Call sites updated:** `loginCmd()` and `tokenLoginCmd()` in `app.go`.

### 2. Token refresh did not update RTDB client

**Bug:** `applyRefresh()` updated `c.tokens.RTDBToken` but never called anything on `c.rtdbClient`. After ~1 hour, the RTDB SSE streams would fail with 401s as soon as C-Mail or cIRC went live.

**Fix:** `applyRefresh(idToken, rtdbToken, rtdbUrl string)` — signature expanded; now also calls `c.rtdbClient.SetToken(rtdbToken)` and updates `c.tokens.RTDBUrl` when a new URL arrives.

### 3. Notification metadata fields

**Bug:** `post_mention` and `reply_mention` notifications carry `postContent` and `replyContent` inline since v0.7. Without them, opening a mention required a follow-up `GET /v1/posts/:id` round trip.

**Fix:**
- `model.Notification` gains: `PostSlug`, `PostAuthorUsername`, `PostContent`, `ReplyContent`
- `wireNotificationMetadata` in `client.go` decodes all four fields
- `notifications.go` renders `postContent` / `replyContent` as a quoted preview line below the notification summary — no extra API call needed
- `PostSlug` and `PostAuthorUsername` are carried in `ShowNotificationPostMsg` for future deep-link navigation; they are not displayed (slugs are kebab-case URL tokens, not human-readable titles)

---

## CAN_UPDATE — Optional Improvements

### A. Guild and GuildMember `profilePictureUrl`

v0.7 (actually v0.4.1) exposes `profilePictureUrl` on both the guild list and member list responses. The field is now decoded and stored in `model.Guild` and `model.GuildMember`. Rendering is deferred — the guild list screen has no imgview support yet.

### B. `GetNotifications` — `type` filter parameter

`GET /v1/notifications` now accepts `?type=reply,reply_mention`. `GetNotifications` signature updated:

```go
GetNotifications(cursor string, unreadOnly bool, types []string) ([]model.Notification, string, error)
```

Pass `nil` for all types (current default). A UI control (filter bar, cycling key) is deferred.

### C. Compose — optional `slug` field

`POST /v1/posts` and `POST /v1/guilds/:slug/posts` now accept an optional `slug` field. `PostComposePanel` gains a `slug` input row between title and body.

**Validation:** `ValidateSlug(s string) error` — rejects chars outside `[a-z0-9-]` and length > 60. Empty slug is always valid (server generates one). On Ctrl+S with an invalid slug the submit is blocked, the slug field gains focus, and an inline red error is shown.

**Wire:** `createPostRequest` and `createGuildPostRequest` gain `Slug string \`json:"slug,omitempty"\`` — blank slugs never reach the wire.

**Signatures updated:**
- `CreatePost(content, title, slug string, topics []string, isPublic, isNSFW bool)`
- `CreateGuildPost(slug, content, title, postSlug string, topics []string)`

---

## Files Changed

| File | Change |
|---|---|
| `internal/model/types.go` | `Tokens.RTDBUrl`; `Notification` +4 fields; `Guild.ProfilePictureUrl`; `GuildMember.ProfilePictureUrl` |
| `internal/api/interface.go` | `GetNotifications` +`types`; `CreatePost` +`slug`; `CreateGuildPost` +`postSlug` |
| `internal/api/client.go` | `applyRefresh` signature; `InitRTDB` signature; wire types; `wireNotificationToModel`; `wireGuildToModel`; `wireGuildMemberToModel`; request structs |
| `internal/api/mock.go` | `CreatePost` and `CreateGuildPost` signatures |
| `internal/api/client_test.go` | Call sites updated; `TestHTTPGetNotifications_TypeFilterInURL` added |
| `internal/api/mock_test.go` | `CreatePost` call sites updated |
| `internal/rtdb/client.go` | `SetToken(token string)` added; `mu sync.RWMutex` added; `buildURL` reads token under read-lock |
| `internal/rtdb/jwt.go` | **Deleted** — `ParseRTDBToken` and `BaseURL` no longer used |
| `internal/rtdb/client_test.go` | JWT tests removed; `TestSetToken_UpdatesAuthParam` added |
| `internal/ui/app.go` | `InitRTDB` call sites; `GetNotifications` call sites; `createPostCmd` and `createGuildPostCmd` signatures |
| `internal/ui/screens/compose.go` | `ValidateSlug`; `postFieldSlug` in focus cycle; `slugInput`/`slugError` on `PostComposePanel`; `SlugValue()`; `PanelHeight` +1; `SetWidth`, `Open`, `Close`, `moveFocus`, `Update`, `View` updated |
| `internal/ui/screens/feed.go` | `SubmitNewPostMsg.Slug` added; `ComposeSubmitMsg` handler captures slug |
| `internal/ui/screens/guilds.go` | `SubmitGuildPostMsg.PostSlug` added; `ComposeSubmitMsg` handler captures slug |
| `internal/ui/screens/notifications.go` | Inline content preview for mention notifications |
| `internal/ui/screens/screens_test.go` | `TestValidateSlug_AcceptsValidSlugs`; `TestFeed_ComposeSubmit_SlugPassedInMsg` added |
| `docs/00-api-backlog.md` | All affected rows updated |

---

## Keyboard Shortcuts

No new shortcuts. The slug field in `PostComposePanel` is reached via Tab (after title) and participates in the existing Tab/Shift+Tab focus cycle.

---

## Known Limitations

- Guild and GuildMember `profilePictureUrl` is captured in the model but not displayed.
- Notification type filter API param is wired but no UI control exists yet.
- C-Mail, cIRC, and Search (new in v0.7) are tracked in `docs/00-api-backlog.md` and not implemented here.
