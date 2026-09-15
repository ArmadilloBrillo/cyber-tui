# 54 — Muted Topics

## Overview

Users can mute topics they don't want to see. A post tagged with any muted
topic is hidden from every post list, the same way `FilterNSFW` hides NSFW
posts. Muting is managed from the **Topics** tab: press `m` on a topic row to
mute or unmute it.

Muted topics are stored server-side in the settings object's `mutedTopics`
array. The field was already decoded into `model.Settings.MutedTopics` and
broadcast to every screen; this feature wires it into filtering, into a
management UI, and into `PATCH /v1/settings`.

> Naming: the cyberspace.online website labels this "blocked topics"; the TUI
> says **"muted"** to line the vocabulary up with the cIRC per-room user
> **mute** feature (`docs/37-circ-mute.md`) — they're independent features that
> now share a verb. Go identifiers also use `muted`, matching the API field.

---

## Managing the list

Topics tab, topic-list view only (there is no single "current topic" to act on
while browsing a topic's posts):

| Key | Action |
|-----|--------|
| `m` | Mute the highlighted topic, or unmute it if already muted |
| `f` | Cycle the topic-list filter: all → hide muted → only muted |

A muted row shows `MUTED` (in the error colour) in place of its post count.

Pressing `m` updates the marker immediately (optimistic) and emits
`SetMutedTopicsMsg{Topics []string}` carrying the full new list. `App`:

1. sets `a.settings.MutedTopics` and calls `broadcastConfig()` so every post
   list re-filters at once;
2. bumps `a.mutedTopicsSaveSeq` and schedules a `mutedTopicsFlushMsg` tick
   (`mutedTopicsSaveDebounce`, 2 s).

Only the tick whose `seq` still matches persists, via `client.UpdateSettings`,
producing a `mutedTopicsSaveResultMsg`. A burst of toggles therefore
coalesces into a single `PATCH /v1/settings` (rate-limited 2/min, 15/day).

**On save failure the optimistic change is rolled back.** `App` keeps
`mutedTopicsSaved` — the `MutedTopics` list the server last accepted (set at
login and after each successful save). A failed `mutedTopicsSaveResultMsg`
restores `a.settings.MutedTopics` from it, bumps the save seq (cancelling any
still-pending tick), re-broadcasts so every screen and the marker revert, and
shows a `notifyError` banner. `mutedTopicsSaveFailText(err)` names the cause:
the 2/min–15/day rate limit, an expired session, or a 5xx — falling back to
`friendlyErr(err)`, always suffixed "— reverted".

### Filtering the topic list

`f` cycles `TopicsModel.topicFilter` through **all → hide muted → only muted →
all**. It's session-only (a plain field, resets to "all" on relaunch, like
scroll position) and applies only in the topic-list view — `f` is ignored
while browsing a topic's posts. The active mode shows as a `filter: hiding
muted` / `filter: muted only` line above the list.

`visibleTopics()` (topics.go) produces the filtered rows:

- **hide muted** — drops muted topics from the fetched pages. Scrolling to the
  bottom still auto-pages the topic list, same as the NSFW / muted-*post*
  filters; a muted topic further down just never appears.
- **only muted** — built from `m.mutedTopics` (i.e. `Settings.MutedTopics`),
  **not** the fetched pages, so it lists *every* muted topic even one whose
  page was never loaded. Rows are sorted by slug. A real `model.Topic` is
  reused where a loaded page has it; the rest are synthesized as `{Slug}` —
  fine because the muted view renders `MUTED` instead of a post count. This
  view is treated as fully loaded (no load-more).

Toggling `m` while a filter is active can drop the highlighted row; the
`topicIndex` is clamped after both the optimistic toggle and the authoritative
`SharedConfigMsg` update.

---

## Filtering (posts)

Each post-list screen keeps a `mutedTopics map[string]struct{}`, refreshed
from `SharedConfigMsg.Settings.MutedTopics` in its existing `SharedConfigMsg`
handler (right where it already mirrors `FilterNSFW`), resetting the selection
index when the set changes. Its `visible*()` helper drops any post where
`topicMuted(p.Topics, m.mutedTopics)` is true.

| Screen | Helper | Filtered |
|--------|--------|----------|
| Feed (`feed.go`) | `visiblePosts()` | yes |
| Topics (`topics.go`) | `visiblePosts()` | yes (a topic's own post list too) |
| Guilds (`guilds.go`) | `visiblePosts()` | yes |
| Profile (`profile.go`) | `visibleProfilePosts()` | yes (Posts sub-tab) |
| Bookmarks (`bookmarks.go`) | `visibleItems()` | yes (bookmarked posts; replies carry no topics) |
| Post detail (`postdetail.go`) | — | **no** — an explicit open / deep link is honoured |
| Search (`search.go`) | — | **no** — consistent with `FilterNSFW`, which Search also doesn't apply |

Shared helpers live in `internal/ui/screens/shared.go`: `topicMuted`,
`mutedSet`, `sameMutedSet`, `toggleMuted`.

Card render caches are unaffected — they already key on the joined topic list,
and filtering happens before rendering.

---

## API

`GET /v1/settings` → `mutedTopics` (already parsed). `PATCH /v1/settings` now
includes `mutedTopics` (`wirePatchSettings`, `HTTPClient.UpdateSettings`) — sent
as a full array, never `null`, so unmuting the last topic clears it
server-side. The Settings screen doesn't edit the field but re-syncs it from
every `SharedConfigMsg` so a `ctrl+s` there can't PATCH a stale list back.

---

## Tests

| File | Tests |
|------|-------|
| `internal/ui/screens/feed_test.go` | `TestFeed_MutedTopics_HidesMatchingPost`, `_Off_ShowsAll` |
| `internal/ui/screens/topics_test.go` | `TestTopics_MutedTopics_HidesMatchingPost`, `TestTopics_MuteKey_TogglesMutedList`, `TestTopics_Filter_*` (hide-muted, only-muted completeness + sort, cycle, post-view no-op, index clamp) |
| `internal/ui/screens/guilds_test.go` | `TestGuildsModel_MutedTopics_HidesMatchingPost` |
| `internal/ui/screens/profile_test.go` | `TestProfile_MutedTopics_HidesMatchingPost` |
| `internal/ui/screens/bookmarks_test.go` | `TestBookmarks_MutedTopics_HidesMatchingPost` |
| `internal/api/client_test.go` | `TestHTTPUpdateSettings_SendsMutedTopics` (incl. clearing to `[]`) |
| `internal/ui/app_test.go` | `TestHandleTopics_SetMutedTopics_AppliesAndDebouncesSave`, `…_RollsBackOnSaveFailure` |
