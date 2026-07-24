# Feature 30: Security Hardening

A security and engineering review identified several issues across the client's
trust boundaries: rendering of untrusted content, the OS URL opener, the SSH
host mode, token handling, and dependencies. This document records the fixes.

## Summary of changes

### 1. Sanitize server-supplied text (terminal escape injection)

Display strings from the API (usernames, display names, bios, post titles, guild
names, topics, attachment metadata) were rendered verbatim through Lip Gloss,
which styles but does not strip terminal escape sequences. A remote user could
embed ESC/OSC control sequences that execute in a viewer's terminal.

New package `internal/sanitize`:

- `Strip(s string) string` removes C0/C1 control characters and DEL while
  preserving tab and newline.
- `Strings(v any)` reflectively cleans every string field of a decoded value,
  recursing through nested structs, pointers, and slices.

`sanitize.Strings` is called at every wire-to-model converter in
`internal/api/client.go` and at the post-create response paths, so all server
text is sanitized at the API boundary before it can reach any screen. Mock data
is developer-controlled and is not sanitized.

### 2. URL opener scheme allowlist

`urlutil.OpenURL` now refuses any URL that is not an absolute http or https URL
without control characters (`urlutil.HTTPURL`). Other schemes (`file:`,
`javascript:`, `mailto:`, OS-registered handlers) are never launched from post
content. On Windows the opener uses `rundll32 url.dll,FileProtocolHandler`
instead of `cmd /c start`, so the URL is not re-parsed by a shell.

### 3. HTTPClient concurrency safety

Bubble Tea runs each command in its own goroutine, so batched API calls and the
401 refresh they trigger accessed `HTTPClient.tokens` concurrently. The field is
now guarded by a `sync.Mutex` via accessor methods (`idToken`, `setTokens`,
`snapshotTokens`, `applyRefresh`). Login state is threaded back to the update
loop through `loginSuccessMsg` instead of being mutated on a detached App copy
from the command goroutine.

### 4. SSH-hosted mode isolation and gating (experimental)

SSH mode previously shared one API client across all connections, so one user's
login set the tokens every session used, and remote logins were written to the
host's config file. `ssh.Serve` now takes a per-connection client factory and
builds a fresh client for each session. Sessions are marked ephemeral
(`App.WithEphemeralSession`) and never read from or write to the host config; all
config persistence is routed through `App.saveConfig`, which is a no-op for
ephemeral sessions. `ListenAndServe` errors are surfaced, and startup prints a
warning that the server is experimental and unauthenticated. Full multi-user SSH
authentication remains out of scope.

### 5. Token, transport, and input hardening

- `InitRTDB` no longer logs token material in debug mode.
- API and RTDB one-shot response bodies are read through `io.LimitReader`
  (10 MiB cap).
- The RTDB JWT `aud` claim is validated against the Firebase project-id charset
  before it is interpolated into the RTDB hostname.
- A plain `http://` `apiBaseURL` to a non-loopback host is rejected unless the
  new `allowInsecureApi` config flag is set, so bearer tokens are not sent in
  cleartext by misconfiguration.

### 6. Dependencies

`golang.org/x/crypto` was upgraded to v0.52.0 and the toolchain pinned to
`go1.25.10`, clearing all vulnerabilities reported by `govulncheck` (8 in
x/crypto, including the SSH server path, plus standard-library issues fixed in
recent Go patch releases).

## Follow-up — 2026-07-24 review remediation

The 2026-07-24 code and security review (`docs/reviews/2026-07-24-code-security-review.md`)
found five issues; this section records the fixes.

### 7. Chat message sanitize-bypass (High)

Section 1 above states `sanitize.Strings` is called at every wire-to-model
converter — but five chat-related conversion paths in `internal/api/client.go`
were missed: `GetRoomMessages`, `GetMessages`, `GetConversations`,
`wireRTDBMessageToModel`, and `wireRTDBCircMessageToModel`. A sixth,
`GetRooms`, was found missing the same call while fixing the others. All six
now call `sanitize.Strings` before building the model value, matching every
other converter in the file. As defense in depth, `internal/ui/screens/render.go`
now also strips control characters from the sender username and action-line
username directly at render time (`sanitize.Strip`), and `internal/ui/app.go`
sanitizes server-generated slash-command reply text before displaying it as a
system message — so a future field added to either path doesn't silently
regress the same way.

### 8. Unsanitized raw bytes in log output (Medium)

`apiTimestamp.UnmarshalJSON`'s fallback log line used to write an
unrecognized server value straight to stderr with `%s`. Changed to `%q`,
which renders control/escape characters as safe Go-quoted text instead of
passing them through raw.

### 9. Redirect scheme downgrade (Low)

`NewHTTPClient`'s `http.Client` now sets `CheckRedirect` to refuse any
redirect whose target scheme is not `https` (replicating the standard
10-redirect cap that Go's default `nil` `CheckRedirect` provides). Closes a
gap where `validateBaseURL` only checked the configured `apiBaseURL` once at
startup — a same-host HTTPS→HTTP redirect issued by a compromised or
misconfigured host would otherwise still carry the bearer token, since Go
only strips `Authorization` on a cross-host redirect, not a same-host scheme
downgrade.

### 10. `crypto/tls` toolchain CVE (Low)

`govulncheck` flagged GO-2026-5856 (Encrypted Client Hello privacy leak) in
the Go standard library at the toolchain version then in use. Toolchain
bumped to `go1.26.5`, which CI picks up automatically via `setup-go`'s
`go-version-file: go.mod`. Re-running `govulncheck` after the bump confirms
the finding is gone.

### 11. SSH server mode — opt-in gate for non-loopback binds

SSH server mode remains unauthenticated by design (see §4) — that isn't
changing. What changed: binding `sshListenAddr` to anything but a loopback
address now requires the new `allowRemoteSsh` config flag, mirroring the
`allowInsecureApi` pattern in §5, so a single misconfigured `sshListenAddr`
(e.g. `":2222"`, which binds all interfaces) can't expose an unauthenticated
session to the network by itself.

## New configuration field

| Field | Type | Default | Purpose |
|---|---|---|---|
| `allowInsecureApi` | bool | `false` | Permit a plain `http://` `apiBaseURL` to a non-loopback host. Off by default. |
| `allowRemoteSsh` | bool | `false` | Permit `sshListenAddr` to bind a non-loopback address. SSH server mode is unauthenticated. Off by default. |

## Verification

```
go test ./...
go vet ./...
staticcheck ./...
govulncheck ./...
```

Targeted tests added by this work:

- `internal/sanitize`: control-character stripping and nested-struct walking.
- `internal/ui/urlutil`: `HTTPURL` accept/reject cases.
- `internal/api`: concurrent token read/refresh under `-race`.
- `internal/rtdb`: rejection of host-injecting `aud` claims.
- `internal/ssh`: surfaced listen error.
- `cmd/cyber-tui`: `validateBaseURL` scheme rules.

Targeted tests added by the 2026-07-24 follow-up (§7-11):

- `internal/api`: control-character stripping for `GetRoomMessages`, `GetMessages`,
  `GetConversations`, `GetRooms`, and the RTDB SSE `SubscribeDMs`/`SubscribeRoom`
  paths; `NewHTTPClient` refusing a redirect to a non-https URL.
- `cmd/cyber-tui`: `validateSSHAddr` loopback/non-loopback/opt-in rules.
