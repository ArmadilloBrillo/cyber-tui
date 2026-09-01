# 53 — Desktop notifications

## Purpose
Raise a real OS desktop notification (toast / notification-centre popup) for new
C-Mail and new activity (replies, mentions, follows, pokes, …) so the user
notices them with the app on another workspace or behind other windows. Until
this feature those events only surfaced as an in-terminal tab badge and a 4s
bottom-bar banner (`docs/31-global-notifications.md`).

## How it works — OSC 9, no dependency
The app writes an **OSC 9** escape sequence to stdout:

```
ESC ] 9 ; <text> BEL      →  "\x1b]9;<text>\x07"
```

Supported by iTerm2, WezTerm, Windows Terminal, and kitty; every other terminal
ignores it silently (nothing breaks, nothing is printed). Reuses the existing
"raw OS-directed escape written straight to `os.Stdout`, past the Bubble Tea
renderer" pattern used for OSC 52 clipboard copy — OSC 9 draws nothing and never
moves the cursor, so it can't corrupt the frame.

`internal/ui/desktopnotify.go`:
- `desktopNotifyString(title, body)` builds the escape. Every control rune (the
  framing ESC/BEL, plus tab/newline) is stripped from `title`/`body` so a
  server-supplied body can't inject further terminal escapes; the combined
  `"title: body"` is truncated to 120 runes.
- `desktopNotifyCmd(title, body)` is the `tea.Cmd` that performs the write.
- `App.shouldDesktopNotify(source screen)` is the focus/tab gate every firing
  point checks.
- `notifScreen(type)` maps a notification's `Type` to the tab it "belongs to".

## When a notification fires

`shouldDesktopNotify(source)` returns true only when **all** of:

1. **`config.Config.DesktopNotifications` is on.** Off by default — opt-in,
   because it does nothing on non-OSC-9 terminals and would otherwise be noise.
   Toggle at **Settings → desktop → notifications (OSC 9 terminals)**.
2. **The session is not ephemeral (SSH-hosted).** A toast from a Wish/SSH
   session would pop on the *host*, not the connected client — the same reason
   OSC 52 clipboard copy is gated on `a.ephemeral`.
3. **Either the terminal window is not the focused one, or it is but the user is
   on a different tab than the one the event belongs to.**
   - Focus is tracked via `tea.FocusMsg` / `tea.BlurMsg` (`tea.WithReportFocus()`
     in `cmd/cyber-tui/main.go`). Terminals report focus on *change*, not at
     startup, so `focusReported` stays false — and notifications fire regardless
     of tab — until the first blur/focus event.
   - Once focus is known: `focused && a.active == source` → **silent** (you're
     already looking at it); anything else → **toast**.

So: focused on the Feed, mentioned in cIRC → toast. Focused on the cIRC tab,
mentioned in cIRC → silent. Window unfocused → toast, whatever tab.

> **Focused-window toasts are terminal-dependent.** A desktop notification
> emitted while the terminal window itself is focused is at the mercy of the
> terminal's own policy: Ghostty (macOS) shows the banner for ~3s then
> auto-dismisses it, and some terminals don't banner a focused-window
> notification at all. So the "focused, but on a different tab" toast is
> best-effort — the reliable signal in that case is the live tab badge (which
> updates in the background regardless). Unfocused-window toasts are shown
> normally by every terminal that supports OSC 9.

### Activity notifications — content mirrors the Notifications tab
The activity toast carries the **same one-line text a Notifications-tab row
shows** ("@alice replied to your post.", "@bob mentioned you in #lobby — <first
60 chars of the message>"), built by `screens.NotifToastText(n)` — which reuses
the row helpers `notifSummary` / `hasActor` / `notifPreview`.

This is produced in the `notifsLoadedMsg` handler (`app.go`,
`desktopNotifyForNewNotifs`) — the single point where a fresh notification list
reaches `a.notifications`. It rides the list the tab already fetches (background
60s poll via `unreadCountMsg`, manual refresh, or tab switch-in): **no extra
API call**, no new message type. The `unreadCountMsg` case just triggers that
refresh (as it already did) and updates the badge.

- A `time.Time` high-water mark, `lastNotifiedAt`, tracks the newest handled
  notification (`model.Notification.CreatedAt`). Items newer than it, still
  unread, are toasted. `notifBaselined` swallows the first list after login so
  the backlog doesn't toast — and `afterLoginCmd` batches `loadNotifsCmd()` so
  that baseline is set from server time immediately at login, not lazily on the
  first poll (otherwise, with zero unread at login, the first notification of
  the session would only set the baseline and never toast).
- `notifScreen(type)` routes the focus gate per type: `chat_mention` →
  `screenChatrooms`, `dm_message` → **skipped** (owned by the C-Mail path
  below; routing it here too would double-toast), everything else →
  `screenNotifications`.
- `suppressActiveRoomMentions` has already marked active-room mentions `Read`
  in the same handler, so a mention in the room you're viewing is skipped.
- More than 3 new at once collapse to one `"N new notifications"` toast.

### C-Mail — separate path
C-Mail is RTDB-driven and never reaches the notifications list, so its toast is
detected in `App.Update` (`maybeNotifyNewCMail`) by an exact before/after
`a.cmail.TotalUnread()` snapshot around `updateInner`, for any
`screens.IsDMStreamMsg(msg)`. Because `App.Update` wraps both the backgrounded
path and the active-tab path, a new C-Mail toasts whether or not the C-Mail tab
is the active one — so an unfocused window always notifies. `cmailUnreadBaselined`
swallows the first post-login conversation-list snapshot. Text is
`C-Mail: new message` (the delta doesn't identify which conversation grew —
sender/preview is a possible follow-up).

Opening/viewing a conversation zeroes its unread, so a message in the
conversation you're actively reading keeps `TotalUnread()` flat → no toast.

## Deliberate limitations
- **Activity toasts inherit the Notifications-tab filter.** The list they ride
  respects the tab's unread-only / type filter, so filtering the tab to
  "mentions only" means only mentions toast. Reasonable; documented.
- **Paginated notification list.** While the user has scrolled the notifications
  list past page 1 (`HasPaginated()` true), the background refresh is
  suppressed, so new items don't toast until a manual refresh / tab re-entry.
  Negligible — you're on the Notifications tab, so a focused toast would be
  suppressed anyway.
- **`.After` is strict.** A second notification created in the same wall-clock
  second as the high-water mark can be missed. Rare, accepted.
- **cIRC mentions lag up to ~60s.** They arrive only via the notification poll
  (no account-wide room stream), unlike C-Mail's near-instant dedicated path.
- **C-Mail toast text is generic.** No sender/preview yet.
- **No OS-native path.** No `beeep`/dbus/PowerShell-toast fallback.
- **SSH server mode never notifies** (gate #2).

## Message flow
```
cmd/cyber-tui/main.go
  → tea.NewProgram(app, tea.WithAltScreen(), tea.WithReportFocus())

App.updateInner
  → tea.FocusMsg / tea.BlurMsg → set a.focused / a.focusReported, stop here

--- activity ---
afterLoginCmd                    batches loadNotifsCmd()  // first notifsLoadedMsg baselines
App.Update (unreadCountMsg)      badge + (count rose & !HasPaginated) → loadNotifsCmd()
App.Update (notifsLoadedMsg)
  → suppressActiveRoomMentions(msg.notifs)      // marks active-room mentions Read
  → desktopNotifyForNewNotifs(msg.notifs)
       first list  → set notifBaselined + lastNotifiedAt, no toast
       otherwise   → for each n newer than lastNotifiedAt, unread:
                       src, ok := notifScreen(n.Type)
                       ok && shouldDesktopNotify(src)
                         → desktopNotifyCmd("cyberspace", screens.NotifToastText(n))
                     advance lastNotifiedAt; >3 → single "N new notifications"
  → a.notifications.SetNotifs(...)

--- C-Mail ---
App.Update
  → if screens.IsDMStreamMsg(msg): before := a.cmail.TotalUnread()
  → updateInner(msg)   // background path (handleCMail) or active path (delegateUpdate)
  → maybeNotifyNewCMail(before, cmd)
       cmailUnreadBaselined && TotalUnread() > before && shouldDesktopNotify(screenCMail)
         → desktopNotifyCmd("C-Mail", "new message")

desktopNotifyCmd → fmt.Fprint(os.Stdout, "\x1b]9;<sanitised, ≤120 runes>\x07")
```

## Key files
| File | Symbol | Role |
|---|---|---|
| `internal/ui/desktopnotify.go` | `desktopNotifyString`, `stripControl`, `desktopNotifyCmd` | Escape builder + writer |
| `internal/ui/desktopnotify.go` | `App.shouldDesktopNotify(source screen)` | Enabled + not-ephemeral + (unfocused OR not on `source`) |
| `internal/ui/desktopnotify.go` | `notifScreen(type)` | Notification type → home tab; `ok=false` for `dm_message` |
| `internal/ui/app.go` | `App.desktopNotifyForNewNotifs` | Diffs a fresh notification list vs `lastNotifiedAt`, toasts new unread items (cap 3) |
| `internal/ui/app.go` | `App.maybeNotifyNewCMail` | Before/after `TotalUnread()` toast, called from `Update` for DM-stream msgs |
| `internal/ui/app.go` | `App.notifBaselined`, `App.lastNotifiedAt`, `App.cmailUnreadBaselined` | Post-login baseline + high-water mark |
| `internal/ui/app.go` | `App.focused`, `App.focusReported` | Terminal focus from `tea.FocusMsg`/`tea.BlurMsg` |
| `internal/ui/app.go` | `updateInner` (`tea.FocusMsg`/`tea.BlurMsg`), `Update` (C-Mail snapshot), `notifsLoadedMsg` case | Wiring |
| `internal/ui/screens/notifications.go` | `NotifToastText`, `notifPreview`, `notifContent` | One-line toast text mirroring a row |
| `internal/config/session.go` | `Config.DesktopNotifications` | Opt-in toggle value |
| `internal/ui/screens/settings.go` | `"desktop"` settings group | The toggle row |
| `cmd/cyber-tui/main.go` | `tea.WithReportFocus()` | Enables focus reporting |
| `internal/ui/desktopnotify_test.go`, `internal/ui/screens/notifications_test.go` | — | Gate matrix, `notifScreen`, diff/baseline/cap, `NotifToastText` |
