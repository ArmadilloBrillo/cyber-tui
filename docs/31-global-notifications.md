# 31 · Global Notifications

A single transient banner surfaces non-fatal errors (and info/warning messages) without blocking the screen they occurred on.

**Errors never block a screen.** Previously, any *load* error was rendered as permanent full-screen red text that replaced the whole tab until the app restarted — and several screens never cleared it, trapping the user (the canonical case: a notification pointing to a deleted post, which 404s on open). Now every failure surfaces in the transient banner, and a screen with no content shows a subtle inline "couldn't load …" line instead of a blocking error.

---

## Two error-handling paths

| Failure kind | Message | Routed by | Display |
|---|---|---|---|
| **Load failure** — a fetch that populates a screen returns an error | `errMsg` | `handleErr` → global banner **and** active screen's `SetError` | Transient banner (via `friendlyErr`); the screen stays usable. If it has no content, its empty-state reads "couldn't load …" instead of the normal "nothing here yet" |
| **Action failure** — create / reply / delete / follow / save / submit / send is rejected (screen still has content) | `actionErrMsg` | `handleNotify` → global banner | Transient banner; screen content stays visible and usable |

`handleErr` still calls each screen's `SetError`, but that error now only feeds the inline empty-state — `View()` never collapses to a bare error string. The error is always cleared on the next load (see *Self-healing* below), so no screen can stay stuck.

### Deleted-post notifications

Opening a notification whose target post was deleted is special-cased so the banner reads a friendly **"This post has been deleted"** rather than the raw 404. The post-open fetch returns `notifPostLoadErrMsg`, handled in `handleNotifications`: a `*api.APIError` with `Status == 404` → friendly warning banner; `ErrUnauthorized` falls through to the login redirect; anything else → error banner. The notification API exposes no "deleted target" field, so the 404 on open is the only signal (confirmed against API v0.4.1).

---

## Banner behaviour

| Aspect | Behaviour |
|---|---|
| Placement | Replaces the status-bar row while visible, then the status bar returns. `ChromeHeight` is unchanged (one row in, one row out). |
| Severity colour | `notifyError` → red, `notifyWarn` → yellow, `notifyInfo` → green |
| Auto-dismiss | Clears after 4 s (`notifyTTL`) |
| Manual dismiss | Any keypress clears it early — and that same keystroke still performs its normal action (it is not swallowed) |
| Truncation | Long messages are truncated to the terminal width with `…` |

### Stale-tick safety

Each notification bumps `notifyGen` and captures that value in its auto-dismiss `tea.Tick`. When an older tick fires, its `gen` no longer matches `App.notifyGen`, so it is ignored — only the newest notification's tick can clear the newest notification. Dismissing via keypress also bumps `notifyGen`, neutralizing the pending tick.

---

## App state and messages

`internal/ui/app.go`

| Symbol | Purpose |
|---|---|
| `App.notifyText` / `notifyLevel` / `notifyGen` | Current banner text, severity, and generation counter (empty text = hidden) |
| `actionErrMsg{err}` | Non-fatal action failure; surfaces as a banner |
| `notifPostLoadErrMsg{err}` | Failure opening a post from Notifications; friendly-handled in `handleNotifications` (404 → "This post has been deleted") |
| `friendlyErr(err)` | Softens raw API errors for the banner (404 → "Not found — it may have been deleted.") |
| `notifyMsg{text, level}` | Set the banner directly (e.g. info/success surfacing) |
| `notifyExpireMsg{gen}` | Auto-dismiss tick; clears the banner iff `gen == App.notifyGen` |
| `notify(level, text)` | Helper: sets banner state and returns the timed-expire command |
| `handleNotify` | Update-chain handler owning the three message types |
| `renderBottomBar` / `renderNotification` | Render the banner in place of the status bar |

---

## Commands routed to the banner

All mutation commands return `actionErrMsg` on failure: guild post, post, reply, post/reply delete, follow/unfollow, profile save, settings save, note create/update/delete, note publish, send room message, send C-Mail. A failed bookmark create (which rolls back its optimistic add) also raises a banner.

All `load*` commands return `errMsg`, which `handleErr` now routes to the banner (plus the inline empty-state). The post-open path from Notifications uses `notifPostLoadErrMsg` instead, for friendly deleted-post handling.

---

## Self-healing load errors

No screen blocks on a load error, and every screen clears its stored `err` on the next fetch (`SetFetching`) and on each successful setter, so a stale error can never persist:

- Every list/detail screen's `SetFetching` clears `err` before re-fetching.
- Success setters clear `err`: `feed.SetPosts`; `notifications.SetNotifs`; `bookmarks.SetBookmarks`; `guilds.SetGuilds`/`SetGuildPosts`/`SetGuildMembers`; `topics.SetTopics`/`SetTopicPosts`; `journal.SetNotes`; `profile.SetUser` + the four sub-tab setters; `postdetail.SetPost`/`SetReplies`; `cmail.SetConversations`.

When a screen has no content and an `err` is set, its `View()`/empty-state renders a subtle "couldn't load …" line. `chatrooms` carried a dead, never-set `err` field and blocking branch — both removed.

---

## Tests

`internal/ui/notify_test.go`

| Test | Verifies |
|---|---|
| `TestNotify_SetsTextAndReturnsTick` | `actionErrMsg` sets text/level/gen and returns a non-nil expire command |
| `TestNotify_AutoExpireClears` | A matching `notifyExpireMsg` clears the banner |
| `TestNotify_StaleExpireDoesNotClearNewer` | A stale expire does not clear a newer notification |
| `TestNotify_KeypressDismissesButKeyStillActs` | A keypress clears the banner and still performs its action (`?` opens help) |
| `TestNotify_ExpireAfterKeypressIsNoOp` | A stale tick after keypress-dismiss is a no-op |
| `TestActionErrMsg_DoesNotBlankScreen` | An action error keeps guild content visible while showing the banner |

`internal/ui/app_test.go`

| Test | Verifies |
|---|---|
| `TestNotifPostLoadErr_DeletedPostShowsBanner` | A 404 on post-open → "This post has been deleted" warning banner; stays on Notifications |
| `TestNotifPostLoadErr_OtherErrorShowsBanner` | Any other post-open failure → error banner |
| `TestNotifPostLoadErr_UnauthorizedRedirectsToLogin` | A 401 on post-open still redirects to login |
| `TestHandleErr_FiresBannerWithoutBlocking` | A load error fires the banner and keeps the user on their screen |
| `TestFriendlyErr_404IsSoftened` | `friendlyErr` softens a 404 and passes other errors through |

`internal/ui/screens/notifications_test.go`

| Test | Verifies |
|---|---|
| `TestNotifs_SetError_ShowsInlineEmptyState` | An errored empty screen shows "couldn't load …" inline, not the raw error |
| `TestNotifs_SetFetchingClearsError` | `SetFetching` and `SetNotifs` clear a prior error (no permanent trap) |
