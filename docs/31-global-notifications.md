# 31 · Global Notifications

A single transient banner surfaces non-fatal action errors (and info/warning messages) without blocking the screen they occurred on.

Previously, any API error — including a failed action like posting to a guild the user isn't a member of — was rendered as permanent full-screen red text that replaced the whole tab until the app restarted.

---

## Two error-handling paths

| Failure kind | Message | Routed by | Display |
|---|---|---|---|
| **Load failure** — a fetch that populates a screen returns an error (screen would be empty) | `errMsg` | `handleErr` → active screen's `SetError` | Full-screen message explaining the empty screen |
| **Action failure** — create / reply / delete / follow / save / submit / send is rejected (screen still has content) | `actionErrMsg` | `handleNotify` → global banner | Transient banner; screen content stays visible and usable |

The distinguishing question for a command: *if this fails, does the screen still have valid content to show?* If yes, it uses `actionErrMsg`.

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
| `notifyMsg{text, level}` | Set the banner directly (e.g. info/success surfacing) |
| `notifyExpireMsg{gen}` | Auto-dismiss tick; clears the banner iff `gen == App.notifyGen` |
| `notify(level, text)` | Helper: sets banner state and returns the timed-expire command |
| `handleNotify` | Update-chain handler owning the three message types |
| `renderBottomBar` / `renderNotification` | Render the banner in place of the status bar |

---

## Commands routed to the banner

All mutation commands return `actionErrMsg` on failure: guild post, post, reply, post/reply delete, follow/unfollow, profile save, settings save, note create/update/delete, note publish, send room message, send C-Mail. A failed bookmark create (which rolls back its optimistic add) also raises a banner.

All `load*` commands keep `errMsg` (full-screen `SetError`).

---

## Self-healing load errors

Screen success setters clear their stored `err` so a stale full-screen load error can no longer persist:

- `guilds.go`: `SetGuilds`, `AppendGuilds`, `SetGuildPosts`, `AppendGuildPosts`, `SetGuildMembers`, `AppendGuildMembers`
- `feed.go`: `SetPosts`
- `topics.go`: `SetTopics`, `AppendTopics`, `SetTopicPosts`

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
