# 02 — Menu Bar & Navigation

## Overview

Replaces the original `[1] label` tab bar with a proper fixed menu bar. Active tab is highlighted as a filled block. Left/right arrow keys navigate between tabs. Status bar is anchored to the bottom of the terminal.

---

## Menu Bar

- Single fixed line at the top of the screen
- Active tab: filled dim-green background, bright green bold text
- Inactive tabs: muted text, no background
- All tabs are the same height — no layout jumping on tab change

## Navigation

| Key | Action |
|---|---|
| `←` / `→` | Cycle through tabs |
| `1` | Jump to Feed |
| `2` | Jump to Rooms |
| `3` | Jump to Mail |
| `4` | Jump to Profile |

Arrow keys are blocked from tab navigation when a text input is focused (Chatrooms, Mail) so cursor movement works normally in those screens.

## Status Bar

Anchored to the bottom of the terminal at all times. Resizes correctly with the window. Shows the logged-in username on the left and key hints on the right.

---

## Implementation Notes

**`theme.ChromeHeight`** — shared constant (`= 3`: tab bar + separator + status bar) used by all screens to calculate their viewport height. Eliminates magic numbers.

**`focusTarget`** — type on `App` (`focusMenu` / `focusList`) gates which component consumes arrow keys. `focusList` is reserved for future list navigation (e.g. scrollable feed items).

**`InputFocused() bool`** — method on `ChatroomsModel` and `DMsModel`, queried by the app before consuming left/right keys.

**`menuTabs` var** — single source of truth for tab order, shared by the renderer and the key handler.
