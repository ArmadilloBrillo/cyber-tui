# 38 — Email Verification (403 EMAIL_NOT_VERIFIED)

Since API v0.8, account access is gated on a verified email address. An unverified account's authenticated requests are rejected with `403 EMAIL_NOT_VERIFIED`. Verification itself is a web-only click-through link flow — there's no code-entry endpoint — but the client can trigger a fresh verification email via `POST /v1/auth/resend-verification`.

## Where the error actually surfaces

`POST /v1/auth/login` itself succeeds regardless of verification status — an `idToken` is required to call `resend-verification`, which only makes sense if login didn't already block on this. The `403` shows up on the very next authenticated call, `GET /v1/users/me`, which both `loginCmd` and `tokenLoginCmd` (`internal/ui/app.go`) make immediately after a successful login to fetch the profile.

`loginErrMsgFor(err, idToken)` (`app.go`) is the shared helper both call sites route their `GetOwnProfile` failure through: it detects `Code == "EMAIL_NOT_VERIFIED"` via `errors.As` against `*api.APIError` and, if so, returns `screens.LoginErrMsg{EmailNotVerified: true, IDToken: idToken}` instead of a generic error — carrying the already-issued `idToken` forward, since that's what `resend-verification` needs and there's no other route to get one for an unverified account (a plain login doesn't return one on its own — you need the successful `Login()` call that just happened).

## Login screen UX

`LoginModel` (`internal/ui/screens/login.go`) tracks `emailNotVerified`, `idToken`, `resending`, and `resendResult`. When `EmailNotVerified` is set:

- The status line switches from the generic `error: ...` text to `your email isn't verified yet — check your inbox for the verification link`, plus a resend hint/status line beneath it.
- Pressing **`r`** (only live in this state — otherwise it's just a character typed into whichever input is focused) emits `ResendVerificationMsg{IDToken}` up to the app, which calls `client.ResendVerification(idToken)` (`resendVerificationCmd`, `app.go`) and returns `ResendVerificationResultMsg{Err}`.
- While in flight, the status line shows `sending verification email...`; on completion it shows either `sent — check your inbox` or `error: <message>` (e.g. a `RATE_LIMITED` hit — the endpoint is capped at 1/min, 5/hour server-side).
- Starting a fresh login attempt (`submitCmd`) clears `emailNotVerified`/`resendResult`, so a stale resend hint doesn't linger across a retry with different credentials.

## Mid-session defense-in-depth

Verification status can't normally change *after* a session starts (you can't get past the profile fetch above unless already verified), but as a cheap safety net, `friendlyErr` (`app.go`) — the same chokepoint that softens a `404` into "Not found — it may have been deleted." for the global error banner — also softens `EMAIL_NOT_VERIFIED` into "Please verify your email — check your inbox for the verification link." for any authenticated call made during normal use, rather than showing the raw `API error EMAIL_NOT_VERIFIED (403): ...` string.

## API surface

| Endpoint | Method | Client method | Notes |
|---|---|---|---|
| `/v1/auth/resend-verification` | POST | `Client.ResendVerification(idToken string) error` | Body `{"idToken": "..."}`. Rate limit 1/min, 5/hour — a `RATE_LIMITED` `*APIError` is returned as-is for the caller to display. |

## Files

| File | Purpose |
|---|---|
| `internal/api/client.go` | `ResendVerification`, `resendVerificationRequest` wire type |
| `internal/api/interface.go` / `mock.go` | `Client` interface + mock stub |
| `internal/ui/screens/login.go` | `LoginErrMsg` (`EmailNotVerified`/`IDToken`), `ResendVerificationMsg`/`ResendVerificationResultMsg`, resend UI + `r` keybinding |
| `internal/ui/app.go` | `loginErrMsgFor`, `resendVerificationCmd`, `friendlyErr`'s `EMAIL_NOT_VERIFIED` branch |

## Known limitations

- No client-side cooldown/disabling of the `r` key between attempts — a user mashing `r` faster than 1/min will just see the server's `RATE_LIMITED` error surface via `resendResult` rather than being pre-emptively blocked. Low-stakes: the worst case is a slightly less friendly error message, not a broken flow.
