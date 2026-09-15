# 16 — View Profile

## Overview

Press `p` on any highlighted item to view the author's or actor's profile. Works from Feed, Post Detail, Notifications, Guilds, Search, Chatrooms (Circ), C-Mail, and Bookmarks. Navigating to your own profile always opens it in edit mode.

Circ, C-Mail, and Bookmarks also accept `ctrl+p` as an alias — needed in C-Mail's open-conversation view, where a focused compose box swallows a bare `p` as a typed character (see Behaviour below); offered everywhere else too for muscle-memory consistency across screens.

---

## Trigger points

| Screen | Highlighted item | Profile shown | Keys |
|--------|-----------------|---------------|------|
| Feed | Selected post | Post's author | `p` |
| Post Detail | Post selected (no reply) | Post's author | `p` |
| Post Detail | Reply selected | Reply's author | `p` |
| Notifications | Selected notification | Notification actor | `p` |
| Guilds | Selected member | Member's profile | `p` |
| Search | Selected hit | User hit's profile, or post/reply author | `p` |
| Chatrooms (Circ) | Message selected (browsing mode) | Message sender | `p` / `ctrl+p` |
| C-Mail | Conversation list, highlighted conversation | Other participant | `p` / `ctrl+p` |
| C-Mail | Open conversation, compose focused (no message selected) | Conversation's other participant | `ctrl+p` |
| C-Mail | Open conversation, message selected (browsing mode) | Selected message's sender | `p` / `ctrl+p` |
| Bookmarks | Selected bookmark (post or reply) | Bookmarked item's author | `p` / `ctrl+p` |

---

## Behaviour

- `p`/`ctrl+p` emits `ShowUserProfileMsg{Username}` from the active screen.
- App calls `GET /v1/users/:username` and switches to the profile screen.
- **Own profile optimisation**: if the username matches the logged-in user, no API call is made — the cached `currentUser` is used directly.
- The profile screen opens in **read-only mode** for other users: bio editing is disabled, the hint shows `esc · back`.
- Pressing `ESC` returns to whichever screen triggered the navigation.
- Navigating to Profile via the tab bar (key `3` or `←→`) always shows your **own** profile in **edit mode**, regardless of what was previously displayed.
- Every screen's `ShowUserProfileMsg` handler in `App.Update`'s dispatch chain guards on `a.active` before claiming the message — required because the same message type is shared across all these screens' handlers, and an earlier handler in the chain with a missing guard would otherwise hijack `profileReturn` from whichever screen is actually active (see `docs/00-project-reference.md`'s note on this class of bug, first caught in Guilds).
- In C-Mail's open conversation, `ctrl+p` reads the conversation's other participant directly off `CMailModel.activeConv` (via `OtherParticipant`) rather than requiring a message to be selected first — so it works immediately on opening a conversation, compose box still focused. Falls back to a no-op for the synthetic `"unknown"` participant placeholder (a stale RTDB conversation entry with no resolvable username).

---

## Profile screen modes

| Mode | How entered | Edit bio | ESC behaviour |
|------|-------------|----------|---------------|
| Own profile | Tab 3 / `←→` | ✓ (`e` key) | No-op |
| Other profile | `p` key | ✗ | Returns to source screen |

---

## Key bindings added

| Screen | Key | Action |
|--------|-----|--------|
| Feed | `p` | View post author's profile |
| Post Detail | `p` | View selected item's author profile |
| Notifications | `p` | View notification actor's profile |
| Guilds | `p` | View selected member's profile |
| Search | `p` | View selected hit's profile |
| Chatrooms (Circ) | `p` / `ctrl+p` | View selected message sender's profile (browsing mode) |
| C-Mail | `p` / `ctrl+p` | View highlighted conversation's other participant (list mode) |
| C-Mail | `ctrl+p` | View open conversation's other participant (compose focused) |
| C-Mail | `p` / `ctrl+p` | View selected message sender's profile (browsing mode) |
| Bookmarks | `p` / `ctrl+p` | View selected bookmark's author profile |
| Profile (read-only) | `esc` | Return to previous screen |
