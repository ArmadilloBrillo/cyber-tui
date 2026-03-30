# 07 — UI Prototype

Layout proposals for the three main interaction surfaces. Each surface has its own layout; they do not share a common template.

---

## App Chrome (all screens)

```
[ feed ]  [ rooms ]  [ mail ]  [ profile ]   ← tab bar (row 1)
                                              ← blank separator (row 2)
[content area]
@username ...hints...                         ← status bar (last row)
```

`theme.ChromeHeight = 3`. Content area = `terminalHeight - 3`.

Navigation: number keys `1–4` jump directly to tabs; `←/→` cycles tabs (only when no text input is focused).

---

## 1. Feed — Stack (Sequential States)

Full-width at all sizes. Posts need the full terminal width to be readable. A drill-down state machine avoids column-width trade-offs at 80 cols.

### State 1 — Feed list (default)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ [ feed ]  [ rooms ]  [ mail ]  [ profile ]                                  │
│                                                                              │
│╔═══════════════════════════════════════════════════════════════════════════╗│  ← selected (ActiveBorder)
│║ @ghostrunner                                          2026-03-30 14:22   ║│
│║                                                                           ║│
│║ The mesh is leaking. Three relays went dark overnight and nobody's        ║│
│║ talking about it.                                                         ║│
│║ #network #surveillance                           [2 replies] [Enter:open] ║│
│╚═══════════════════════════════════════════════════════════════════════════╝│
│╔═══════════════════════════════════════════════════════════════════════════╗│  ← dim border
│║ @nyx_404                                              2026-03-30 14:18   ║│
│║ Running packet sniffs all night. Nothing. The silence IS the signal.      ║│
│║ #opsec                                               [0 replies]          ║│
│╚═══════════════════════════════════════════════════════════════════════════╝│
│ @username   ↑↓:select   Enter:open   n:new post   1-4:tab                  │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Keys:** `↑/↓` move cursor · `Enter` open post · `n` new post

### State 2 — Post detail + replies

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ [ feed ]  [ rooms ]  [ mail ]  [ profile ]                                  │
│                                                                              │
│ ◀ Esc:back to feed                                                           │
│╔═══════════════════════════════════════════════════════════════════════════╗│
│║ @ghostrunner                                          2026-03-30 14:22   ║│
│║ The mesh is leaking. Three relays went dark overnight.                    ║│
│║ #network #surveillance                                        2 replies   ║│
│╚═══════════════════════════════════════════════════════════════════════════╝│
│──────────────────────────────── REPLIES ──────────────────────────────────  │
│14:31  @nyx_404    Which sector? I had a signal drop in zone 7               │
│14:35  @phantom_hz Same. Looked deliberate.                                  │
│                                                                              │
│──────────────────────────────── REPLY ────────────────────────────────────  │
│╔═══════════════════════════════════════════════════════════════════════════╗│
│║> _                                                                        ║│
│╚═══════════════════════════════════════════════════════════════════════════╝│
│ @username   Esc:back   ↑↓:scroll replies   Enter:send   1-4:tab            │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Keys:** `↑/↓` scroll replies · `Tab/r` focus input · `Enter` send reply · `Esc` back to list (scroll position preserved)

### State 3 — New post compose

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ [ feed ]  [ rooms ]  [ mail ]  [ profile ]                                  │
│                                                                              │
│ ◀ Esc:cancel   NEW POST                                                      │
│╔═══════════════════════════════════════════════════════════════════════════╗│
│║                                                                           ║│
│║ _                                                                         ║│
│║                                                                           ║│
│╚═══════════════════════════════════════════════════════════════════════════╝│
│ Ctrl+Enter:submit   Esc:cancel                                              │
│ @username                                                        1-4:tab   │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Keys:** `Ctrl+Enter` submit · `Esc` cancel

---

## 2. C-Mail (DMs) — Refined Two-Pane

Sidebar always visible. Sidebar width: `clamp(terminalWidth/4, 20, 32)`. Input width: dynamic (`rightPaneInnerWidth - 2`). Active pane uses `theme.ActiveBorder`.

```
At 80 columns (sidebar = 20, chat = 58):
┌─────────────────────────────────────────────────────────────────────────────┐
│ [ feed ]  [ rooms ]  [ mail ]  [ profile ]                                  │
│                                                                              │
│╔══════════════════╗ ╔════════════════════════════════════════════════════╗ │
│║ C-MAIL           ║ ║ @nyx_404                               ← Esc:list ║ │  ← focused pane = ActiveBorder
│║                  ║ ╠════════════════════════════════════════════════════╣ │
│║>@nyx_404         ║ ║ 14:10  @nyx_404    hey, you saw the relay news?   ║ │
│║  hey, you saw…   ║ ║ 14:12  @you        yeah, sector 7. not good       ║ │
│║                  ║ ║ 14:15  @nyx_404    you think it's intentional?    ║ │
│║ @ghostrunner     ║ ║ 14:18  @you        has to be. three at once?      ║ │
│║  the patch is…   ║ ║                                                    ║ │
│║                  ║ ╠════════════════════════════════════════════════════╣ │
│║                  ║ ║> _                                                 ║ │
│╚══════════════════╝ ╚════════════════════════════════════════════════════╝ │
│ @username   Tab:switch pane   ↑↓:navigate   Enter:open/send   1-4:tab      │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Keys (list pane):** `↑/↓` cursor · `Enter` open conversation + focus chat · `Tab` shift focus

**Keys (chat pane):** `↑/↓` scroll (when input not focused) · `Enter` send · `Esc` blur input (second Esc → list) · `Tab` shift focus

**Changes from current:** proportional sidebar width · dynamic input width · `focusPane` field with ActiveBorder indicator · selection cursor on conversation list

---

## 3. cIRC (Chatrooms) — Three-Pane (responsive)

Member list shown at `terminalWidth >= 100`; collapses below that with count in header. Room list uses single-line `name  ●N` format.

```
At 120+ columns:
┌──────────────────────────────────────────────────────────────────────────────────────────────────────────┐
│ [ feed ]  [ rooms ]  [ mail ]  [ profile ]                                                               │
│                                                                                                          │
│╔════════════════╗ ╔══════════════════════════════════════════════════════════════╗ ╔══════════════════╗ │
│║ ROOMS          ║ ║ #cyberspace                         Ctrl+←→: prev/next room ║ ║ ONLINE  (4)      ║ │
│╠════════════════╣ ╠══════════════════════════════════════════════════════════════╣ ╠══════════════════╣ │
│║>#cyberspace ●47║ ║ 14:10  @ghostrunner  anyone monitoring the relay cluster?   ║ ║ @ghostrunner     ║ │
│║ #hacking    ●23║ ║ 14:11  @nyx_404      yeah, been watching all morning        ║ ║ @nyx_404         ║ │
│║ #tech       ●11║ ║ 14:12  @phantom_hz   three nodes went dark. sector 7.       ║ ║ @phantom_hz      ║ │
│║ #random     ●8 ║ ║                                                              ║ ║ @you             ║ │
│║                ║ ╠══════════════════════════════════════════════════════════════╣ ║                  ║ │
│║                ║ ║> _                                                           ║ ║                  ║ │
│╚════════════════╝ ╚══════════════════════════════════════════════════════════════╝ ╚══════════════════╝ │
│ @username   Tab:panes   Ctrl+←→:rooms   Enter:send/DM   1-4:tab                                        │
└──────────────────────────────────────────────────────────────────────────────────────────────────────────┘

At 80 columns (member pane hidden):
┌─────────────────────────────────────────────────────────────────────────────┐
│ [ feed ]  [ rooms ]  [ mail ]  [ profile ]                                  │
│                                                                              │
│╔════════════════╗ ╔══════════════════════════════════════════════════════╗  │
│║ ROOMS          ║ ║ #cyberspace                            47 online     ║  │
│╠════════════════╣ ╠══════════════════════════════════════════════════════╣  │
│║>#cyberspace ●47║ ║ 14:10  @ghostrunner  anyone monitoring the relay?   ║  │
│║ #hacking    ●23║ ║ 14:11  @nyx_404      yeah, all morning              ║  │
│║ #tech       ●11║ ╠══════════════════════════════════════════════════════╣  │
│║ #random     ●8 ║ ║> _                                                   ║  │
│╚════════════════╝ ╚══════════════════════════════════════════════════════╝  │
│ @username   Tab:panes   Ctrl+←→:rooms   Enter:send   1-4:tab               │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Focus cycle:** `Tab` — rooms → messages → members (if visible) → rooms

**Keys (rooms pane):** `↑/↓` cursor · `Enter` join room

**Keys (messages pane):** `Ctrl+←/→` switch rooms · `↑/↓` scroll · `Enter` send

**Keys (members pane):** `↑/↓` cursor · `Enter` open DM with user

**Changes from current:** single-line room list · responsive member pane · `focusPane` enum · dynamic input width · `Ctrl+←/→` room switching shortcut

---

## Cross-Cutting Improvements (all screens)

| Issue | Fix |
|---|---|
| No active-pane focus indicator | `theme.ActiveBorder` on focused pane, `theme.Border` on inactive |
| Hardcoded input width 60 | `rightPaneInnerWidth - 2` computed on `WindowSizeMsg` |
| Static status bar hints | Each screen exports `Hints() string`, called from `App.View()` |
| `Esc` not gated at app level | Hierarchy: blur input → close sub-view → tab navigation |
| No selection cursor in DM list | Add `selectedConv int` field |
