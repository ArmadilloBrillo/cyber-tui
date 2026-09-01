# 53 — Desktop notifications

## Purpose
Raise a real OS desktop notification (toast / notification-centre popup) when
new C-Mail arrives or the activity unread-count climbs while the user has the
app backgrounded. Until now those events only ever surfaced as an in-terminal
tab badge and a 4s bottom-bar banner (`docs/31-global-notifications.md`), so
they were invisible with the app on another workspace.

## How it works — OSC 9, no dependency
The app writes an **OSC 9** escape sequence to stdout:

```
ESC ] 9 ; <text> BEL      →  "\x1b]9;<text>\x07"
```

Supported by iTerm2, WezTerm, Windows Terminal, and kitty; every other
terminal ignores the sequence silently, so an unsupported terminal simply
gets no toast (nothing breaks, nothing is printed). This reuses the existing
"raw OS-directed escape written straight to `os.Stdout`, past the Bubble Tea
renderer" pattern already used for OSC 52 clipboard copy — OSC 9 draws
nothing and never moves the cursor, so it can't corrupt the frame.

`internal/ui/desktopnotify.go`:
- `desktopNotifyString(title, body)` builds the escape. Every control rune
  (the framing ESC/BEL, plus tab/newline) is stripped from `title` and
  `body` so a server-supplied message body can't inject further terminal
  escapes; the combined `"title: body"` text is truncated to 120 runes.
- `desktopNotifyCmd(title, body)` is the `tea.Cmd` that performs the write.
- `App.shouldDesktopNotify()` is the gate every firing point checks.

## When a notification fires
`shouldDesktopNotify()` returns true only when **all** of:

1. **`config.Config.DesktopNotifications` is on.** Off by default — opt-in,
   because it does nothing on non-OSC-9 terminals and would otherwise be
   noise. Toggle at **Settings → desktop → notifications (OSC 9 terminals)**.
2. **The session is not ephemeral (SSH-hosted).** A toast fired from a
   Wish/SSH session would pop on the *host* machine, not the connected
   client — the same reason OSC 52 clipboard copy is gated on
   `a.ephemeral`.
3. **The terminal is not known to be focused.** Focus is tracked via
   `tea.FocusMsg` / `tea.BlurMsg` (`tea.WithReportFocus()` in
   `cmd/cyber-tui/main.go`). Terminals report focus on *change*, not at
   startup, so `focusReported` stays false — and notifications still fire —
   until the first blur/focus event. After that, a toast is suppressed while
   the app is focused.

Two firing points, both in `internal/ui/app.go`:

| Event | Location | Text |
|---|---|---|
| Activity unread-count increased (replies, follows, pokes, bookmarks, **and cIRC `chat_mention`** — mentions are `chat_mention` notifications) | `unreadCountMsg` case | `cyberspace: N new notification(s)` |
| New C-Mail while the C-Mail tab is backgrounded | `handleCMail` default branch (`IsDMStreamMsg`) | `C-Mail: new message` |

Both use a one-shot **baseline guard** (`notifCountBaselined`,
`cmailUnreadBaselined`) so the unread backlog present at login doesn't toast
on the first server poll / first conversation-list snapshot.

## Deliberate limitations
- **Generic text, not per-item.** "2 new notification(s)" / "new message",
  not "Reply from @alice" / "@bob mentioned you in #lobby". Per-type text is
  a straightforward follow-up (read the newest item after `loadNotifsCmd`
  resolves) — deferred.
- **C-Mail delta is naïve.** The C-Mail baseline (`cmailUnreadSeen`) isn't
  refreshed while the C-Mail tab is *active*, so the first new message right
  after leaving that tab may not toast. Best-effort ping; not a delivery
  guarantee. Marked with a `ponytail:` comment on the `App` field.
- **cIRC mention while reading that room.** `unreadCountMsg` fires before
  `suppressActiveRoomMentions` runs, so being mentioned in a room you're
  actively viewing can still toast — but only if focus reporting hasn't
  established that the terminal is focused (once it has, the focus gate
  covers this).
- **No OS-native path.** No `beeep`/dbus/PowerShell-toast fallback — that
  would add a dependency and platform code for a nice-to-have.
- **SSH server mode never notifies** (limitation #2 above).

## Message flow
```
cmd/cyber-tui/main.go
  → tea.NewProgram(app, tea.WithAltScreen(), tea.WithReportFocus())

App.updateInner
  → tea.FocusMsg / tea.BlurMsg → sets a.focused / a.focusReported, stops here

App.Update (unreadCountMsg)
  → count > prev && notifCountBaselined && shouldDesktopNotify()
      → tea.Batch(desktopNotifyCmd("cyberspace", "N new notification(s)"), …)
  → notifCountBaselined = true

App.handleCMail (default branch, a.active != screenCMail && IsDMStreamMsg)
  → a.cmail.Update(msg); n := a.cmail.TotalUnread()
  → n > cmailUnreadSeen && cmailUnreadBaselined && shouldDesktopNotify()
      → tea.Batch(cmd, desktopNotifyCmd("C-Mail", "new message"))
  → cmailUnreadSeen = n; cmailUnreadBaselined = true

desktopNotifyCmd
  → fmt.Fprint(os.Stdout, "\x1b]9;<sanitised, ≤120 runes>\x07")
```

## Key files
| File | Symbol | Role |
|---|---|---|
| `internal/ui/desktopnotify.go` | `desktopNotifyString`, `stripControl`, `desktopNotifyCmd`, `App.shouldDesktopNotify` | Escape builder, control-char stripper, the write command, the gate |
| `internal/ui/app.go` | `App.desktopNotifications` | Local config value (opt-in) |
| `internal/ui/app.go` | `App.focused`, `App.focusReported` | Terminal focus state from `tea.FocusMsg`/`tea.BlurMsg` |
| `internal/ui/app.go` | `App.notifCountBaselined`, `App.cmailUnreadSeen`, `App.cmailUnreadBaselined` | Post-login baseline guards |
| `internal/ui/app.go` | `updateInner` | Handles `tea.FocusMsg` / `tea.BlurMsg` |
| `internal/ui/app.go` | `unreadCountMsg` case, `handleCMail` default branch | The two firing points |
| `internal/ui/app.go` | `WithSavedPreferences`, `broadcastConfig`, `settingsSavedMsg` plumbing | Loads / re-applies the config value (copies `feedManualRefreshOnly`) |
| `internal/config/session.go` | `Config.DesktopNotifications` | Persisted toggle value (`"desktopNotifications"`, omitempty, default false) |
| `internal/ui/screens/settings.go` | `"desktop"` settings group | "notifications (OSC 9 terminals)" toggle row |
| `internal/ui/screens/messages.go` | `SharedConfigMsg.DesktopNotifications`, `SaveSettingsMsg.DesktopNotifications` | Broadcast / save wiring |
| `cmd/cyber-tui/main.go` | `tea.WithReportFocus()` | Enables focus reporting |
| `internal/ui/desktopnotify_test.go` | — | Escape format, injection stripping, truncation, gate truth table |
