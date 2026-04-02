# 12 — Login email pre-fill

## Purpose
When a user has previously logged in and their session has since expired (or been cleared), the login screen is shown again. Their email is already stored in `~/.cyber-tui.json` from the previous session, so there is no reason to make them re-type it. This feature pre-fills the email field and moves the cursor directly to the password field.

## Behaviour
- On startup, if no valid `refreshToken` is present and no `autoEmail`/`autoPassword` is configured, but `cfg.Email` is non-empty, the login screen opens with the email field pre-filled.
- Focus is placed on the password field automatically.
- The user can still edit the email field by pressing `Shift+Tab` / `Up` to move back.

## Priority order (main.go)
1. `cfg.RefreshToken` non-empty → `WithSavedSession` (skip login screen entirely)
2. `cfg.AutoEmail` + `cfg.AutoPassword` both non-empty → `WithAutoLogin` (skip login screen)
3. `cfg.Email` non-empty → `WithSavedEmail` (pre-fill email, show login screen)
4. Fallback → empty login screen

## Key files
| File | Symbol | Role |
|---|---|---|
| `internal/ui/screens/login.go` | `NewLoginModel(email string)` | Accepts optional email; pre-fills and shifts focus when non-empty |
| `internal/ui/app.go` | `App.WithSavedEmail(email string) App` | Sets the login model with the pre-filled email |
| `cmd/cyber-tui/main.go` | startup block | Falls through to `WithSavedEmail(cfg.Email)` when appropriate |
