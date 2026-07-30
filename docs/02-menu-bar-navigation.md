# 02 — Menu Bar & Navigation

## Overview

Replaces the original `[1] label` tab bar with a proper fixed menu bar. Active tab is highlighted as a filled block. Left/right arrow keys navigate between tabs. Status bar is anchored to the bottom of the terminal.

The app has grown from 4 tabs to 11, which outgrew a plain `1`-`9` numbering
scheme (see `docs/00-project-reference.md`'s "Keyboard Shortcuts" section
for the full current inventory). Navigation now combines two schemes, both
derived from the same `menuTabs` slice so they can't drift apart:

- **Numeric aliases** (`1`-`9`) for the first 9 tabs, by position.
- **Leader key** (`g` + a mnemonic letter, e.g. `g f` for Feed) reaches all
  11 screens, including the two with no numeric alias (Search, Settings).

Search is further set apart from the other 10: it's a **hidden,
explicit-entry-only destination** — not shown on the tab bar/sidebar and not
part of `←`/`→` (or MillerLayout's `j`/`k`) cycling at all. The only ways in
are `g s` and `/`, and both always land in a focused query box regardless of
whatever state Search was last left in. This avoids a state the screen
doesn't actually handle: landing on Search unfocused (which arrow-cycling
used to be able to do) left the query box unreachable, since query-mode is
only ever supposed to be entered focused.

---

## Menu Bar

- Single fixed line at the top of the screen (TabsLayout) or a vertical
  sidebar (MillerLayout) — both render from `visibleTabs()` (`menuTabs`
  minus any entry marked `hidden` — currently just Search)
- Active tab: filled dim-green background, bright green bold text
- Inactive tabs: muted text, no background
- Each tab's mnemonic letter (the second key of its `g`-chord) is rendered
  highlighted in cyan inline within the label itself — e.g. `feed`'s `f`,
  `c-mail`'s `m` — so the leader-key hint is visible without opening the
  help modal (`?`)
- **"In detail" marker**: a selected tab that's currently one level deep —
  an open Circ room, an open C-Mail conversation, browsing into a Guild or
  Topic, or PostDetail opened from that tab — keeps its active highlight and
  gets an extra marker on top: a trailing `›` in TabsLayout (e.g. `circ ›`),
  or the nav sidebar's `▶` marker becomes `▷` in MillerLayout. Both layouts
  compute this from one shared function, `tabVisualState(a, screen) (selected,
  detail bool)` (`internal/ui/layout.go`), so they can't disagree about which
  state a tab is in. `selected` covers both "this screen is `a.active`" and
  "PostDetail is open and `postDetailReturn` points back to this tab" (since
  PostDetail is a single screen shared by six origins — Feed, Bookmarks,
  Profile, Guilds, Topics, Notifications — rather than duplicated per-origin).
  Before this, Circ/C-Mail silently dropped the active highlight entirely
  once a room/conversation was open (indistinguishable from a tab never
  visited), and Guilds/Topics had no equivalent signal at all.
- All tabs are the same height — no layout jumping on tab change

## Navigation

| Key | Action |
|---|---|
| `←` / `→` | Cycle through the 10 visible tabs (TabsLayout, `focusMenu`) — Search is hidden and excluded, see above |
| `1`-`9` | Jump directly to one of the first 9 tabs — see the numeric table in `docs/00-project-reference.md` |
| `g` + letter | Jump directly to any of the 11 screens via its mnemonic, including Search — see the chord table in `docs/00-project-reference.md` |

Arrow keys and the numeric/leader jumps are blocked from tab navigation when a text input is focused (CIRC, C-Mail, Search's query box, compose panels) so typing works normally in those screens.

## Status Bar

Anchored to the bottom of the terminal at all times. Resizes correctly with the window. Shows the logged-in username on the left and key hints on the right.

---

## Implementation Notes

**`theme.ChromeHeight`** — shared constant (`= 3`: tab bar + separator + status bar) used by all screens to calculate their viewport height. Eliminates magic numbers.

**`focusTarget`** — type on `App` (`focusMenu` / `focusList` / `focusDetail`) gates which component consumes arrow keys.

**`HasFocusedInput(a App) bool`** — method on `Layout` (implemented per layout, delegating to each screen's own `InputFocused()`/`ComposeActive()`), queried by the app before consuming keys that would otherwise navigate.

**`menuTabs` var** (`internal/ui/layout.go`) — single source of truth for tab order, label, leader-key mnemonic, and (via the `hidden` field) whether an entry is part of the visible/cyclable tab set. `screenForNumber` and `screenForMnemonic` both derive from the full slice (so Search stays reachable by mnemonic); `visibleTabs()` filters out `hidden` entries for rendering and cycling. `activateScreen` is the shared jump-to-screen implementation both the numeric and leader-key paths call, so the two layouts and both navigation schemes can never disagree about what a given key does. `navigateTabBy` (arrow/j-k cycling) is a no-op while `screenSearch` is active, the same treatment `screenPostDetail` gets — neither is part of the rotation.

**`tabVisualState(a App, t screen) (selected, detail bool)`** (`internal/ui/layout.go`, next to `menuTabs`) — the shared selected/in-detail computation behind the "in detail" marker described above. Called identically by `TabsLayout.renderTabBar` and `MillerLayout.renderNav`.

**`leaderPending` field on `App`** — armed by the `g` key in `handleKeys` (`internal/ui/app.go`); the next keypress resolves against `screenForMnemonic` or silently cancels. No timeout is needed since `g` has no other binding of its own, so there's no ambiguity to resolve.
