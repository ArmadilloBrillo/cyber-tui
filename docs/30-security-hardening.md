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

## New configuration field

| Field | Type | Default | Purpose |
|---|---|---|---|
| `allowInsecureApi` | bool | `false` | Permit a plain `http://` `apiBaseURL` to a non-loopback host. Off by default. |

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
