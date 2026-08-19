# 47 — Startup update check

## Purpose
Let a user on a released binary know a newer version exists, without polling,
without any config, and without ever blocking or interrupting startup.

## Behaviour
- Runs once, right after login succeeds (`afterLoginCmd`, `internal/ui/app.go`)
  — batched alongside the other startup background checks (unread count,
  wander mode). Not on a repeating schedule; a fresh check only happens on
  the next app launch.
- Gated on `version.Version != "dev"` (`internal/version`) — only released
  builds check. A `go run`/untagged build (`Version == "dev"`) never makes
  the network call at all.
- `internal/update.Latest` fetches
  `https://api.github.com/repos/ArmadilloBrillo/cyber-tui/releases/latest`
  with a dedicated 8s-timeout HTTP client (never the authenticated
  cyberspace.online API client) and compares the returned `tag_name` against
  `version.Version` by plain string inequality — a release build's version
  is always exactly the tag it was built from (see `.github/workflows/release.yml`
  ldflags), so any difference means a newer tag was published.
- If newer, the existing global banner (`App.notify`, `notifyWarn`) shows
  once: `"update available: vX.Y.Z (you have vA.B.C) — <release URL>"`. It
  auto-dismisses like every other notification (4s or next keypress) — no
  persistent UI, no dismiss-forever state.
- Every failure mode — network error, non-200, malformed JSON, already on
  the latest tag — silently produces no message. The user is never shown an
  error for this.
- Skipped entirely for ephemeral (SSH-hosted) sessions, matching wander
  mode's convention of never touching state for those sessions.

## Non-goals
No throttle/persistence (`LastUpdateCheck`, etc.) — deliberately dropped in
favor of "just check on startup," since a fresh app launch is already a
natural, infrequent cadence. No semver dependency — tags aren't strict
semver (`v0.8.6.4`), and string inequality against the one authoritative
"latest" tag is sufficient without ordering logic.
