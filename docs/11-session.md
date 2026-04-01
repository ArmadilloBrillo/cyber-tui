# 11 — Persistent Session

After the first successful login the app writes a session file to `~/.cyber-tui.json` so that subsequent launches auto-login without prompting for credentials.

## Session file

```json
{
  "refreshToken": "<long-lived token>",
  "username":     "alostviking",
  "email":        "alostviking@gmail.com",
  "savedAt":      "2026-04-01T10:00:00Z"
}
```

**Only the `refreshToken` is persisted.** The short-lived `idToken` and `rtdbToken` (~1 hour TTL) are obtained fresh from `/v1/auth/refresh` on every startup.

The file is written with mode `0600` (owner read/write only).

## Startup flow

```
Session file found with refreshToken
  → POST /v1/auth/refresh
  → success → GET /v1/users/me → main feed   (no login screen)
  → failure → login screen (refresh token expired or revoked)

No session file
  → login screen → manual login → POST /v1/auth/login → save session → main feed
```

## Implementation

| Package | Purpose |
|---|---|
| `internal/config` | `Session` struct · `Load` / `Save` / `Clear` |
| `internal/api.Client` | `LoginWithRefreshToken(refreshToken string)` |
| `internal/ui.App` | `WithSavedSession` · `tokenLoginCmd` · saves session after login |
| `cmd/cyber-tui/main.go` | Loads session at startup; env-var credentials remain as dev/CI fallback |

## Clearing the session

Delete the file manually to force a full re-login:

```sh
rm ~/.cyber-tui.json
```

## Dev / CI mode

If no session file is present but `CYBERSPACE_EMAIL` and `CYBERSPACE_PASSWORD` are set in the environment (or `.env`), the app falls back to credential-based auto-login. This path is intended for development and automated testing only — it is not used when a session file exists.
