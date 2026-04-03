# 16 — View Profile

## Overview

Press `p` on any highlighted item to view the author's or actor's profile. Works from Feed, Post Detail, and Notifications. Navigating to your own profile always opens it in edit mode.

---

## Trigger points

| Screen | Highlighted item | Profile shown |
|--------|-----------------|---------------|
| Feed | Selected post | Post's author |
| Post Detail | Post selected (no reply) | Post's author |
| Post Detail | Reply selected | Reply's author |
| Notifications | Selected notification | Notification actor |

---

## Behaviour

- `p` emits `ShowUserProfileMsg{Username}` from the active screen.
- App calls `GET /v1/users/:username` and switches to the profile screen.
- **Own profile optimisation**: if the username matches the logged-in user, no API call is made — the cached `currentUser` is used directly.
- The profile screen opens in **read-only mode** for other users: bio editing is disabled, the hint shows `esc · back`.
- Pressing `ESC` returns to whichever screen triggered the navigation (feed, post detail, or notifications).
- Navigating to Profile via the tab bar (key `3` or `←→`) always shows your **own** profile in **edit mode**, regardless of what was previously displayed.

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
| Profile (read-only) | `esc` | Return to previous screen |
