# Feature 42 — Poke

Implements sending a poke to another user from their profile. Receiving pokes (the `poke` notification type) was already implemented alongside general notification support.

---

## Scope

| Capability | Status |
|---|---|
| Poke another user (`p` key on read-only profile) | Done |
| `p` visible in the status bar hints and `?` help modal on a read-only profile | Done |
| Toast confirmation after a successful poke | Done |
| Suppressed on the logged-in user's own profile | Done |
| Friendly toast on rate limit (429) | Done |
| Friendly toast when blocked either direction (403) | Done |
| Receiving a `poke` notification | **Already implemented** — see `docs/15-notifications.md` |

---

## API Endpoints Used

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/v1/users/:username/poke` | Send a poke notification to a user. No body. |

Response body (`{ userId, username, poked }`) is not used — a non-error response is treated as success, matching the client's existing no-body-response actions (e.g. `WatchPost`).

Rate limit: **1/hour, 8/day, across all users** — not per target, and tighter than any other write action in the client. A rejected poke (400/403/404) doesn't count against the limit per the API docs.

---

## UI

### Profile screen

- Viewing another user's profile (`readOnly=true`): pressing `p` sends `PokeUserMsg{Username}` to `App`.
- On success, the global toast banner (bottom of screen — the same one used for "reported", room-joined, link-copied, etc.) shows `poked @username`.
- The poke key is suppressed on the logged-in user's own profile (`readOnly=false`), same as follow/message.
- No confirmation prompt — matches the web, which also pokes immediately on click.
- `p` is visible in the status bar hints (`f · follow  c · message  p · poke  tab · cycle`) and in the `?` help modal's profile section, both only while viewing a read-only profile.

### Key note: `p` is contextual

`p` already means "open this post's author's profile" on Feed, Post Detail, and Notifications. That binding lives on those screens' own models and only fires while they own input; once the app navigates to the Profile screen, `ProfileModel.Update` is the only handler receiving keys, so `p` there means "poke" instead. There's no shared/global keymap in this app — every binding is scoped to whichever screen is active — so the two meanings never compete. The status-bar hint for `p` also changes screen-to-screen for the same reason (`screenHints` in `internal/ui/layout_tabs.go` switches on `a.active`).

### Error handling

Poke result and errors are routed through the app's global toast system (`notifyMsg`/`actionErrMsg`, handled by `handleNotify` in `internal/ui/app.go`) — the same 4-second bottom-of-screen banner used everywhere else (flagging, room join/leave, link copy, etc.), rather than the profile-local feedback text used by Follow/Unfollow.

- Success: info-level toast, `poked @username`.
- `429` (rate limit reached): error-level toast "poke limit reached — try again later" instead of the raw API error text — expected to be hit occasionally given the tight 1/hour cap.
- `403` (blocked either direction): error-level toast "can't poke this user".
- Anything else (e.g. an unreachable-in-practice 404 for an unknown user, or a 400 for poking yourself — both should be prevented client-side by the `readOnly` guard) falls through to the standard `actionErrMsg` toast.

---

## Files Changed

| File | Change |
|---|---|
| `internal/api/interface.go` | Added `Poke(username string) error` to the `Client` interface |
| `internal/api/client.go` | Implemented `Poke` — `POST /v1/users/:username/poke`, no body, ignores response |
| `internal/api/mock.go` | Stub `Poke` implementation |
| `internal/ui/screens/messages.go` | Added `PokeUserMsg` |
| `internal/ui/screens/profile.go` | Added `p` key binding (read-only only), emits `PokeUserMsg` |
| `internal/ui/app.go` | Added `pokeUserCmd` (returns `notifyMsg` on success), `pokeErrorMsg` (429/403 friendly toasts); wired `PokeUserMsg` dispatch in `handleProfile` — result/errors flow through the existing global `handleNotify` toast path |
| `internal/ui/layout_tabs.go` | Added `p · poke` to the read-only-profile status bar hints (`screenHints`, shared by the Miller layout) and to the `?` help modal's profile section |
| `internal/api/client_test.go` | `TestHTTPPoke_CallsCorrectEndpoint`, `TestHTTPPoke_RateLimited` |
| `internal/ui/screens/profile_test.go` | Tests for `p` key emitting `PokeUserMsg`, suppressed on own profile |
| `internal/ui/app_test.go` | Tests for `pokeErrorMsg` 429/403 softening and fallthrough |
