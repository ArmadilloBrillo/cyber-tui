# 08 — C-Mail

Private 1-on-1 conversations. Accessed via tab `3` or the `c-mail` tab in the navigation bar.

---

## Layout

Two-pane horizontal split:

```
╔══════════════════╗ ╔════════════════════════════════════════╗
║ c-mail           ║ ║ @otheruser                             ║  ← focused pane = ActiveBorder
║                  ║ ╠════════════════════════════════════════╣
║>@molly           ║ ║ 14:10  @molly   hey, you saw the news? ║
║  hey, you saw…   ║ ║ 14:12  @you     yeah, not good         ║
║                  ║ ╠════════════════════════════════════════╣
║ @wintermute      ║ ║> _                                     ║
║  i am the one…   ║ ╚════════════════════════════════════════╝
╚══════════════════╝
```

- **Left pane** — conversation list with `ActiveBorder` (bright green) when focused
- **Right pane** — message thread + compose input; input box uses `ActiveBorder` when focused
- Sidebar width: `clamp(terminalWidth/4, 20, 32)` — scales with terminal, min 20, max 32
- Compose input width: fills right pane (dynamic, recalculated on resize)

---

## Focus Model

| Pane | Constant | Visual indicator |
|---|---|---|
| Conversation list | `FocusCMailLeft` | Sidebar border = bright green |
| Chat + input | `FocusCMailRight` | Input box border = bright green |

Initial focus on screen load: `FocusCMailLeft`.

---

## Key Bindings

### Left pane (`FocusCMailLeft`)

| Key | Action |
|---|---|
| `↑` / `k` | Move cursor up the conversation list |
| `↓` / `j` | Move cursor down the conversation list |
| `Enter` | Open selected conversation → shift focus to right pane, auto-focus input |
| `Tab` | Shift focus to right pane, focus input |
| `←` / `→` | Fall through to tab navigation (switch screens) |

### Right pane (`FocusCMailRight`)

| Key | Action |
|---|---|
| `Enter` | Send message (when input focused and non-empty) |
| `↑` / `↓` | Scroll message history (when input not focused) |
| `Esc` | First press: blur input. Second press: return focus to left pane |
| `Tab` | Blur input, shift focus to left pane |
| `←` / `→` | Fall through to tab navigation |

---

## Width Calculation

```
sidebarWidth  = clamp(terminalWidth / 4, 20, 32)   // inner content
sidebarOuter  = sidebarWidth + 4                    // + border(2) + padding(2)
gap           = 2
vpWidth       = terminalWidth - sidebarOuter - gap  // = terminalWidth - sidebarWidth - 6
input.Width   = vpWidth - 4                         // account for input's own border+padding
```

At 80 cols: sidebar inner = 20, viewport = 54, input = 50.
At 120 cols: sidebar inner = 30, viewport = 84, input = 80.
At 200 cols: sidebar inner = 32 (clamped), viewport = 162, input = 158.

---

## Screen Model

**File:** `internal/ui/screens/cmail.go`

**Type:** `CMailModel`
**Constructor:** `NewCMailModel(currentUser string) CMailModel`
**Message emitted:** `SendCMailMsg{ConversationID, Body}`
**App field:** `a.cmail`
**Screen constant:** `screenCMail`

### Exported accessors (for testing)

| Method | Returns |
|---|---|
| `FocusPane() CMailFocus` | Current pane focus |
| `SelectedConv() int` | Cursor index in conversation list |
| `HasActiveConv() bool` | Whether a conversation is open |
| `SidebarWidth() int` | Computed sidebar inner width |
| `InputFocused() bool` | Whether the compose input is active |
