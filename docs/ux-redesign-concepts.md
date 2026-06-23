# UX Redesign Concepts

Design exploration for a novel reimagining of cyber-tui's interface. Three concepts are described below with ASCII mockups. These are not committed to implementation — they're design references to pick up later.

---

## Concept A — Miller Columns (Recommended)

Borrowed from Ranger (file manager) and macOS Finder column view. Three persistent panes — no tabs, no context switches. You always see: what section you're in, what's in it, and what you've selected.

```
┌──────────────┬──────────────────────────────┬──────────────────────────────────┐
│ SPACES       │ FEED                    ●3   │ @molly_millions · 2h             │
│              │ ─────────────────────────    │ ──────────────────────────────   │
│ ▶ Feed       │ ■ @molly · 2h ago            │ Just jacked in and the matrix    │
│   Notifs  ●3 │   @winter · 3h ago           │ looks cleaner than ever tonight. │
│   C-Mail     │   @neuro · 4h ago            │ Something's moving in Chiba City.│
│   ─────────  │   @molly · 5h ago            │                                  │
│   Topics     │   @winter · 6h ago           │ #cyberpunk #nightlife            │
│   Guilds     │   @neuro · 7h ago            │ 2h ago · public · 4 replies      │
│   Journal    │   @molly · 8h ago            │ ──────────────────────────────   │
│   Saved      │   @winter · 9h ago           │ ↳ @wintermute · 1h              │
│   Profile    │                              │   The ICE shifts at night.       │
│   Settings   │                              │                                  │
│              │                              │ ↳ @neuromancer · 45m            │
│              │                              │   You noticed it too?            │
│              │                              │                                  │
│              │                              │ [r] reply  [b] bookmark  [w] watch│
└──────────────┴──────────────────────────────┴──────────────────────────────────┘
 ←/→ or h/l move focus   j/k select   enter drill in   esc back   n new post
```

### How it works

- **Left pane** — section selector. `h`/`l` (or `←`/`→`) shifts focus between panes. Pressing right from the section list lands in the content list.
- **Center pane** — content list for the selected section (feed posts, notifications, topics, etc.). Moving `j`/`k` here immediately previews the item in the right pane — no Enter needed.
- **Right pane** — reading pane. Shows the selected post + its replies inline, or the selected notification + context. Pressing right/Enter moves focus into the reading pane for scrolling or composing.
- Composing pops up as a bottom overlay on the right pane (same `ComposeModel` as today).
- Notifications, C-Mail, and Journal all follow the same three-column layout — the section just changes what the center list shows.
- The left pane unread badge makes the global notification count visible without ever leaving the feed.

### Why this is novel for TUI social clients

Nobody builds social clients this way. File managers (Ranger, lf) and email clients (aerc, mutt w/ sidebar) use it — but social feeds don't. The payoff: zero context loss. You're never "on" notifications and "away from" the feed — both are visible simultaneously, one column click away.

### Fits the existing architecture

- Maps cleanly to the existing `screens/` models — the App layer would route pane focus instead of `activeScreen`.
- `SharedConfigMsg` would carry a `paneWidth` for each of the three panes instead of the full terminal width.
- The same `ComposeModel` embedding pattern works unchanged.
- Narrow terminal fallback (<80 cols): collapse to single-pane, `h`/`l` switches between them.

---

## Concept B — The Stream (No Screens, Just River)

No screens at all. Everything is a continuous chronological stream in one pane. Posts expand *inline* when selected; replies nest below. Notifications slide in at the top as ephemeral banners. You never "go" anywhere — content comes to you.

```
┌──────────────────────────────────────────────────────────────────────────┐
│ ◈ CYBER.SPACE  @neuromancer  ▲3 new  /feed  @beat 547                   │
├──────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  ─────────────────────── TODAY ─────────────────────────────            │
│                                                                          │
│  @molly_millions · 2h ──────────────────────────── [4↩][★2][w]         │
│  Just jacked in and the matrix looks cleaner than ever tonight.          │
│  Something's moving in Chiba City.  #cyberpunk #nightlife               │
│  │                                                                       │
│  │  ↳ @wintermute · 1h    The ICE shifts at night.                      │
│  │  ↳ @neuromancer · 45m  You noticed it too?          [r]              │
│  │  ↳ @molly · 30m        Every 30 nights, like clockwork.              │
│  │  ↳ @wintermute · 15m   Something's watching back.                    │
│  │                                                                       │
│  ════════════════════════════════════════════════════════════════        │
│  @wintermute · 3h ──────────────────────────────── [1↩][★0][w]         │
│  Running a new construct in Freeside. Anyone tried the tessellation      │
│  patch from last week?                                                   │
│                                                                          │
│  @neuromancer · 4h ─────────────────────────────── [0↩][★1]            │
│  Chiba nights. #photography                                              │
│                                                                          │
├──────────────────────────────────────────────────────────────────────────┤
│ [compose: n]  [filter: /feed /notifs /cmail /guild:name]  [reply: r]    │
└──────────────────────────────────────────────────────────────────────────┘
```

### How it works

- The breadcrumb at the top (`/feed`) shows your current filter. Typing `/notifs`, `/cmail`, `/#cyberpunk`, `/@username` switches what stream you see.
- `j`/`k` selects a post; its replies collapse/expand with Enter.
- No other screens — Profile, Settings, and Compose all pop up as full-screen overlays (`p`, `,`, `n`).
- Notifications arrive inline at the top of the current view: `▲ @molly replied · [enter] open  [esc] dismiss`.

### Why this works

Eliminates the mental "I have to switch to notifications to check" tax. You stay in the stream and the network comes to you. Natural for people who think of social networks as live rivers, not filing systems.

---

## Concept C — The Cyberdeck Dashboard

Bloomberg/htop aesthetic: multiple panes always visible simultaneously, all live. No navigation away from the main view — just quadrant focus-shifting. Leans into the retro-futuristic cyberpunk identity.

```
╔════════════════════════════╦═══════════════════════════════════════╗
║ FEED                  ▶ 3 ║ @molly_millions · 2h                  ║
║ ──────────────────────     ║ ─────────────────────────────────     ║
║ ■ @molly · Just jacked in  ║ Just jacked in and the matrix looks   ║
║   @winter · Running a new  ║ cleaner than ever tonight.            ║
║   @neuro · Chiba nights    ║ Something's moving in Chiba City.     ║
║   @molly · Anyone seen the ║                                       ║
║   @winter · The old city   ║ #cyberpunk #nightlife                 ║
║   @neuro · Tessellation    ║ 2h ago · public · ★2                  ║
║   @molly · ICE at the gate ║ ─────────────────────────────────     ║
║   @winter · Midnight run   ║ ↳ @wintermute  The ICE shifts         ║
╠════════════════════════════║   at night.                           ║
║ NOTIFS            ●3  [ ] ║ ↳ @neuromancer  You noticed too?      ║
║ ──────────────────────     ║ ↳ @molly  Every 30 nights.           ║
║ ■ @molly replied · 1h      ║                                       ║
║   @winter poked · 2h       ║ [r] reply   [b] bookmark   [w] watch  ║
║   @molly bookmarked · 3h   ╠═══════════════════════════════════════╣
║   (mark all: M)            ║ TRENDING         GUILDS               ║
╠════════════════════════════║ #cyberpunk 142   Zion Cluster      ▶  ║
║ C-MAIL                     ║ #matrix     87   Wintermute Corp   ▶  ║
║ ──────────────────────     ║ #chiba      54   Tessier-Ashpool  ▶  ║
║ @molly  hey, you there?    ║ #freeside   31   Maas-Neotek      ▶  ║
╚════════════════════════════╩═══════════════════════════════════════╝
 tab=cycle panes  j/k select  enter expand  r reply  n post  q quit
```

### How it works

- `tab` cycles pane focus (Feed → Notifs → C-Mail → Reading pane → back).
- Reading pane (right) always shows the selected item from whichever left-side pane is focused.
- Trending and Guilds mini-panels at bottom-right are ambient summaries; they update on a background tick.
- Compose, Profile, and Settings open as full-screen overlays.

---

## Comparison

| | Miller Columns | Stream | Cyberdeck |
|---|---|---|---|
| Navigation model | Spatial (left/center/right) | Filter-based (`/feed`, `/#tag`) | Quadrant focus cycling |
| Context switching | None — all sections one column away | None — filter changes the river | Minimal — panes always visible |
| Learning curve | Low (Ranger users feel at home) | Low (IRC/weechat feel) | Medium (quadrant focus is unfamiliar) |
| Small terminal | Degrades to single-pane gracefully | No change needed | Breaks below ~120 cols |
| Implementation delta | Medium — pane routing replaces screen routing | High — replaces entire screen model | High — requires layout engine |
| Novelty | High in social context | Medium (IRC does this) | High |

**Recommended: Concept A (Miller Columns).** Proven UX, novel in social context, maps cleanly to the existing Bubble Tea architecture, and degrades gracefully on narrow terminals.

---

## Narrow Terminal Handling (Concept A)

Below 80 cols, the three-pane layout collapses to single-pane with explicit `h`/`l` pane-switch keys:

```
┌──────────────────────────────────┐
│ FEED ← [spaces] [reading] →     │
│ ──────────────────────────────── │
│ ■ @molly_millions · 2h ago       │
│   @wintermute · 3h ago           │
│   @neuromancer · 4h ago          │
│                                  │
│ j/k select  enter preview  h/l   │
└──────────────────────────────────┘
```
