# 54 — Blocked Topics

## Overview

Users can block topics they don't want to see. A post tagged with any blocked
topic is hidden from every post list, the same way `FilterNSFW` hides NSFW
posts. Blocking is managed from the **Topics** tab: press `b` on a topic row to
block or unblock it.

Blocked topics are stored server-side in the settings object's `mutedTopics`
array (the cyberspace.online website labels this "blocked topics"). The field
was already decoded into `model.Settings.MutedTopics` and broadcast to every
screen; this feature wires it into filtering, into a management UI, and into
`PATCH /v1/settings`.

> Naming: the UI says **"blocked"**; Go identifiers keep the existing
> `MutedTopics` name. Unrelated to the cIRC per-room user **mute** feature
> (`docs/37-circ-mute.md`).

---

## Managing the list

Topics tab, topic-list view only (there is no single "current topic" to act on
while browsing a topic's posts):

| Key | Action |
|-----|--------|
| `b` | Block the highlighted topic, or unblock it if already blocked |

A blocked row shows `BLOCKED` (in the error colour) in place of its post count.

Pressing `b` updates the marker immediately (optimistic) and emits
`SetBlockedTopicsMsg{Topics []string}` carrying the full new list. `App`:

1. sets `a.settings.MutedTopics` and calls `broadcastConfig()` so every post
   list re-filters at once;
2. bumps `a.blockedTopicsSaveSeq` and schedules a `blockedTopicsFlushMsg` tick
   (`blockedTopicsSaveDebounce`, 2 s).

Only the tick whose `seq` still matches persists, via `client.UpdateSettings`,
producing a `blockedTopicsSaveResultMsg`. A burst of toggles therefore
coalesces into a single `PATCH /v1/settings` (rate-limited 2/min, 15/day).

**On save failure the optimistic change is rolled back.** `App` keeps
`blockedTopicsSaved` — the `MutedTopics` list the server last accepted (set at
login and after each successful save). A failed `blockedTopicsSaveResultMsg`
restores `a.settings.MutedTopics` from it, bumps the save seq (cancelling any
still-pending tick), re-broadcasts so every screen and the marker revert, and
shows a `notifyError` banner. `blockedTopicsSaveFailText(err)` names the cause:
the 2/min–15/day rate limit, an expired session, or a 5xx — falling back to
`friendlyErr(err)`, always suffixed "— reverted".

---

## Filtering

Each post-list screen keeps a `blockedTopics map[string]struct{}`, refreshed
from `SharedConfigMsg.Settings.MutedTopics` in its existing `SharedConfigMsg`
handler (right where it already mirrors `FilterNSFW`), resetting the selection
index when the set changes. Its `visible*()` helper drops any post where
`topicBlocked(p.Topics, m.blockedTopics)` is true.

| Screen | Helper | Filtered |
|--------|--------|----------|
| Feed (`feed.go`) | `visiblePosts()` | yes |
| Topics (`topics.go`) | `visiblePosts()` | yes (a topic's own post list too) |
| Guilds (`guilds.go`) | `visiblePosts()` | yes |
| Profile (`profile.go`) | `visibleProfilePosts()` | yes (Posts sub-tab) |
| Bookmarks (`bookmarks.go`) | `visibleItems()` | yes (bookmarked posts; replies carry no topics) |
| Post detail (`postdetail.go`) | — | **no** — an explicit open / deep link is honoured |
| Search (`search.go`) | — | **no** — consistent with `FilterNSFW`, which Search also doesn't apply |

Shared helpers live in `internal/ui/screens/shared.go`: `topicBlocked`,
`blockedSet`, `sameBlockedSet`, `toggleBlocked`.

Card render caches are unaffected — they already key on the joined topic list,
and filtering happens before rendering.

---

## API

`GET /v1/settings` → `mutedTopics` (already parsed). `PATCH /v1/settings` now
includes `mutedTopics` (`wirePatchSettings`, `HTTPClient.UpdateSettings`) — sent
as a full array, never `null`, so unblocking the last topic clears it
server-side. The Settings screen doesn't edit the field but re-syncs it from
every `SharedConfigMsg` so a `ctrl+s` there can't PATCH a stale list back.

---

## Tests

| File | Tests |
|------|-------|
| `internal/ui/screens/feed_test.go` | `TestFeed_BlockedTopics_HidesMatchingPost`, `_Off_ShowsAll` |
| `internal/ui/screens/topics_test.go` | `TestTopics_BlockedTopics_HidesMatchingPost`, `TestTopics_BlockKey_TogglesBlockedList` |
| `internal/ui/screens/guilds_test.go` | `TestGuildsModel_BlockedTopics_HidesMatchingPost` |
| `internal/ui/screens/profile_test.go` | `TestProfile_BlockedTopics_HidesMatchingPost` |
| `internal/ui/screens/bookmarks_test.go` | `TestBookmarks_BlockedTopics_HidesMatchingPost` |
| `internal/api/client_test.go` | `TestHTTPUpdateSettings_SendsMutedTopics` (incl. clearing to `[]`) |
| `internal/ui/app_test.go` | `TestHandleTopics_SetBlockedTopics_AppliesAndDebouncesSave`, `…_RollsBackOnSaveFailure` |
