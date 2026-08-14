# Feature 32: Security and Code Review (July 2026)

An independent code and security review of the cyber-tui client, performed on the
`dev` branch. This review is separate from and follows the earlier hardening pass
recorded in [30-security-hardening.md](30-security-hardening.md). It re-verifies
the six fixes from that pass, runs the full analysis toolchain, and reports new
findings. No code was changed as part of this review; every item below is a
recommendation for follow-up work.

## Scope and method

- Static and dynamic analysis: `go build`, `go test ./... -race`, `go vet`,
  `staticcheck`, `govulncheck`, and `gosec`.
- Secret scan of the working tree and full git history (256 commits) with
  `gitleaks`, plus targeted `git grep` and `git log` heuristics.
- Manual review by trust boundary: API client and token lifecycle, config and
  session persistence, the Firebase RTDB client and JWT parsing, SSH host mode,
  rendering paths (markdown, image viewer, URL opener), and CLI tooling.
- Supply chain review of the GitHub Actions workflows and dependencies.

Tool versions: Go 1.25.10, staticcheck (latest), govulncheck (latest),
gosec dev build, gitleaks v8.

## Verification of the previous hardening pass

All six fixes documented in 30-security-hardening.md are still present and
correct on `dev`, with one caveat noted in Finding 1:

| # | Fix from doc 30 | Status | Evidence |
|---|---|---|---|
| 1 | Sanitize server text at the API boundary | Holds | `sanitize.Strings` called in every wire-to-model converter, [internal/api/client.go](../internal/api/client.go) |
| 2 | URL opener scheme allowlist | Holds | `HTTPURL` rejects non-http(s), [internal/ui/urlutil/open.go:16](../internal/ui/urlutil/open.go) |
| 3 | HTTPClient token concurrency safety | Holds | mutex accessors, [internal/api/client.go:363](../internal/api/client.go); `-race` suite passes |
| 4 | SSH session isolation and gating | Holds for config and wander; see Finding 2 | per-connection client factory, [internal/ssh/server.go:28](../internal/ssh/server.go) |
| 5 | Token, transport, and input hardening | Holds | `io.LimitReader` caps, `aud` validation, `allowInsecureApi` gate |
| 6 | Dependencies cleared by govulncheck | No longer true | see Finding 1 |

## Findings

Severity reflects impact and exploitability in this client's context. This is a
local-first TUI, so most attacker-supplied input arrives as API response content
or as a crafted local config file.

### Finding 1 (High): Three reachable vulnerabilities have appeared since the last pass

`govulncheck` now reports three vulnerabilities that its call-graph analysis
marks as reachable from this code. Doc 30 stated that the toolchain pin and the
`x/crypto` upgrade "cleared all vulnerabilities"; that is no longer the case, and
because `govulncheck` is not part of CI (Finding 7), the regression was silent.

- GO-2026-5061 in `golang.org/x/image` v0.42.0: a panic on a VP8 alpha-channel
  size mismatch in the WebP decoder. Reachable through
  [internal/ui/imgview/fetch.go:30](../internal/ui/imgview/fetch.go), which calls
  `image.Decode` on attacker-controlled image bytes. Impact is a client crash
  triggered by a malicious image attachment. Fixed in `x/image` v0.43.0.
  Source: https://pkg.go.dev/vuln/GO-2026-5061
- GO-2026-5039 in the standard library `net/textproto`, reachable via the RTDB
  HTTP path. Fixed in go1.25.11.
  Source: https://pkg.go.dev/vuln/GO-2026-5039
- GO-2026-5037 in the standard library `crypto/x509`. Fixed in go1.25.11.
  Source: https://pkg.go.dev/vuln/GO-2026-5037

Recommendation: raise `golang.org/x/image` to v0.43.0 or later, bump the toolchain
line in [go.mod](../go.mod) to go1.25.11 or later, and add `govulncheck` to CI so
future advisories fail the build rather than going unnoticed.

### Finding 2 (Medium): SSH sessions can drive host-side process launch and outbound fetches

The SSH isolation work correctly makes remote sessions ephemeral so they never
read or write the host config, and wander mode is gated on `a.ephemeral`
([internal/ui/app.go:2899](../internal/ui/app.go)). Two side-effecting paths are
not gated, however, and run in the host process for every session:

- `urlutil.OpenURL` launches `xdg-open`, `open`, or `rundll32` on the host
  ([internal/ui/urlutil/open.go:33](../internal/ui/urlutil/open.go)). A remote,
  unauthenticated SSH user who opens a link in a viewed post causes the host to
  spawn a browser process.
- `imgview.Fetch` performs an outbound HTTP GET to a URL taken from post content
  ([internal/ui/app.go:1720](../internal/ui/app.go),
  [internal/ui/imgview/fetch.go:16](../internal/ui/imgview/fetch.go)). In SSH
  mode this is a server-side request forgery primitive: the host fetches
  attacker-chosen http(s) URLs, including internal addresses reachable from the
  host.

The URL allowlist limits schemes to http and https, so this is not arbitrary
command execution, but process launch and host-side fetching from unauthenticated
input are still meaningful for anyone who enables SSH mode. The startup warning
([cmd/cyber-tui/main.go:66](../cmd/cyber-tui/main.go)) covers exposure but not
these specific behaviors.

Recommendation: gate `OpenURL` and terminal image fetching on `!a.ephemeral`, or
disable both in SSH-hosted sessions, and document the SSRF consideration.

### Finding 3 (Medium): Image fetch has no size limit or SSRF guard

Unlike the API and RTDB clients, which cap response bodies with
`io.LimitReader` at 10 MiB, `imgview.Fetch` reads the full body into
`image.Decode` with no cap and uses `http.DefaultClient`
([internal/ui/imgview/fetch.go:22](../internal/ui/imgview/fetch.go)). A hostile
endpoint can return a very large body or a small file that declares large
dimensions, driving memory allocation during decode. There is also no restriction
on the destination host, which compounds Finding 2 in SSH mode. The fetch relies
solely on the 20-second context timeout set by the caller
([internal/ui/app.go:1724](../internal/ui/app.go)).

Recommendation: wrap the body in `io.LimitReader` with a sane image cap, bound
decoded pixel dimensions before allocation, and use a dedicated `http.Client`
rather than `http.DefaultClient`.

### Finding 4 (Low): A crafted local config can redirect the refresh token

`validateBaseURL` correctly blocks cleartext http to non-loopback hosts, but any
`https` `apiBaseURL` is accepted ([cmd/cyber-tui/main.go:107](../cmd/cyber-tui/main.go)).
On startup with a saved session, the client sends the persisted refresh token to
whatever host `apiBaseURL` names ([internal/api/client.go:511](../internal/api/client.go),
via `LoginWithRefreshToken`). An attacker who can write `~/.cyber-tui.json` can
therefore exfiltrate the long-lived refresh token to a host they control. Writing
that file already implies local account compromise, so severity is low, but the
refresh token is the only persisted credential and the file mode (0600) is its
sole protection.

Recommendation: this is an accepted local-trust boundary; consider documenting it,
and consider pinning the production host or warning when `apiBaseURL` points away
from `api.cyberspace.online`.

### Finding 5 (Low): RTDB auth token is passed in the URL query string

`rtdb.Client.buildURL` appends the RTDB JWT as `?auth=<token>`
([internal/rtdb/client.go:199](../internal/rtdb/client.go)). This is Firebase's
documented REST mechanism, but tokens in URLs are more exposed than tokens in
headers: they can appear in intermediary proxy logs and server access logs. The
RTDB chat and DM endpoints that would exercise this path are currently stubbed
([internal/api/client.go:1246](../internal/api/client.go)), so the live exposure
today is limited. Note this before the chat feature ships.

### Finding 6 (Low): Plaintext auto-login credentials

`autoEmail` and `autoPassword` are stored in cleartext in `~/.cyber-tui.json`
([internal/config/session.go:46](../internal/config/session.go)). This is an
explicit, documented convenience tradeoff protected only by file mode 0600.
Preferring the saved refresh-token session over stored credentials
([cmd/cyber-tui/main.go:79](../cmd/cyber-tui/main.go)) is the right default.
Recommendation: keep the feature opt-in and documented; consider a note steering
users toward token-based sessions instead.

### Finding 7 (Medium): CI does not run vulnerability or security scanning

[.github/workflows/ci.yml](../.github/workflows/ci.yml) runs `go test`, `go vet`,
and `staticcheck`, but not `govulncheck` or `gosec`. Doc 30 listed `govulncheck`
as a manual verification step only. Finding 1 is the direct consequence: three
advisories became reachable without any signal. Recommendation: add a
`govulncheck ./...` job (and optionally `gosec`) to the PR workflow.

### Finding 8 (Medium): GitHub Actions supply-chain hardening gaps

- Actions are pinned by mutable major-version tags (`actions/checkout@v4`,
  `actions/setup-go@v5`, `softprops/action-gh-release@v2`) and
  `dominikh/staticcheck-action@v1` with `version: latest`. A moved tag or a
  compromised upstream would flow into builds. Recommendation: pin to full commit
  SHAs and pin staticcheck to a fixed version.
- Neither workflow declares a `permissions:` block at the top level; `ci.yml` has
  none at all, so the job token inherits the repository default. Recommendation:
  set `permissions: contents: read` as the default and grant `contents: write`
  only to the release job (which already scopes it,
  [.github/workflows/release.yml:17](../.github/workflows/release.yml)).
- The release workflow interpolates `${{ github.event.inputs.tag }}` directly
  into `run:` shell commands ([.github/workflows/release.yml:29](../.github/workflows/release.yml)).
  This is the standard script-injection pattern: a crafted `workflow_dispatch`
  input is evaluated by the shell. It requires collaborator-level dispatch access,
  so impact is bounded, but the input should be passed through an `env:` variable
  and referenced as `"$TAG"` instead of interpolated inline.
- Release artifacts are published without checksums or signatures
  ([.github/workflows/release.yml:67](../.github/workflows/release.yml)).
  Recommendation: generate a `SHA256SUMS` file and consider signing (for example
  with cosign).

### Finding 9 (Informational): Personal data committed in captured API responses

The repository commits real API captures under
[docs/api-reponses/](../docs/api-reponses/), including
`users.me.api.reponse.json.md`, which contains a real account's user ID, serial
number, bio, and precise `locationLatitude`/`locationLongitude`. This is the
repository owner's own data and no credentials are exposed, but on a public repo
it is a privacy consideration. Recommendation: redact coordinates and account
identifiers in committed samples.

### Finding 10 (Informational): gosec low-severity items and confirmed false positives

`gosec` reported 19 items. The security-relevant triage:

- G404 weak RNG at [internal/ui/app.go:74](../internal/ui/app.go) and 2909-2910:
  used only for the cosmetic wander-mode location. Not security-relevant.
- G115 uint32-to-byte conversions in
  [internal/ui/imgview/kitty.go:24](../internal/ui/imgview/kitty.go): RGBA channel
  bytes, expected truncation. False positive.
- G101 "hardcoded credentials" in [internal/api/mock.go](../internal/api/mock.go):
  developer-controlled mock data. False positive.
- G204 subprocess-with-variable in
  [internal/ui/urlutil/open.go](../internal/ui/urlutil/open.go): mitigated by the
  `HTTPURL` allowlist; see Finding 2 for the residual SSH concern.
- G304 file-inclusion in [internal/config/session.go:99](../internal/config/session.go):
  the config path is derived from the user's own home directory. Low.
- G104 unhandled errors: `fmt.Sscanf` in the timezone parser
  ([internal/config/session.go:158](../internal/config/session.go)) and
  `io.Copy(io.Discard, ...)` in the RTDB PUT drain
  ([internal/rtdb/client.go:105](../internal/rtdb/client.go)). Quality items, not
  security. The timezone parser silently accepts malformed labels rather than
  reporting them.

## What is solid

The following were reviewed and found correct, worth recording so the next
reviewer does not re-derive them:

- Token handling: only the refresh token is persisted; the ID and RTDB tokens are
  in-memory behind a mutex ([internal/api/client.go:351](../internal/api/client.go)).
  The 401 refresh-and-retry path is bounded to a single retry and skips
  `/v1/auth/` endpoints ([internal/api/client.go:491](../internal/api/client.go)).
- All path parameters are escaped with `url.PathEscape` and cursors with
  `url.QueryEscape` throughout the API client.
- The `aud` claim is charset-validated before it is interpolated into the RTDB
  hostname ([internal/rtdb/jwt.go:54](../internal/rtdb/jwt.go)).
- Markdown rendering drops raw HTML nodes to the empty string
  ([internal/ui/markdown/renderer.go:396](../internal/ui/markdown/renderer.go)),
  and content is control-character sanitized before it reaches the renderer, so
  the ANSI output cannot carry injected terminal escapes.
- The secret scan found no leaked credentials in the working tree or in 256
  commits of history. `.env` never contained real secrets, `.claude/settings.local.json`
  is gitignored and untracked, and no host keys or `dist/` artifacts are tracked.

## Code quality observations

Separate from security, and consistent with the project's own Go conventions:

- Error wrapping and the transport-versus-domain type split (`wireX` types with
  `wireXToModel` converters) are applied consistently. Package responsibilities
  are clean and interfaces live apart from implementations.
- The RTDB chat and DM surface is stubbed: `GetRooms`, `GetRoomMessages`,
  `SendRoomMessage`, `GetConversations`, `GetMessages`, `SendMessage`, and
  `SubscribeDMs` return not-implemented errors or a closed channel
  ([internal/api/client.go:1246](../internal/api/client.go)). This is dead surface
  area today; it should either land behind a feature flag or be tracked so it does
  not rot.
- Test coverage is strong on the security-relevant packages (api, config, rtdb,
  ssh, sanitize, urlutil each have tests). The gap is `imgview.Fetch`, which has
  no test despite being the reachable path for Finding 1 and Finding 3. The
  network fetch is hard to unit test, but a size-limit guard would be testable
  with an `httptest` server.
- The timezone parser uses unchecked `fmt.Sscanf`
  ([internal/config/session.go:144](../internal/config/session.go)); a malformed
  label silently yields a zero offset rather than an error.

## Prioritized recommendations

1. Update `golang.org/x/image` to v0.43.0+ and the toolchain to go1.25.11+
   (Finding 1).
2. Add `govulncheck` to CI so advisory regressions fail the build (Findings 1, 7).
3. Gate `OpenURL` and image fetch on non-ephemeral sessions and add a response
   size limit to `imgview.Fetch` (Findings 2, 3).
4. Harden the workflows: pin actions to SHAs, add a least-privilege
   `permissions:` block, and pass the release tag input through `env:` rather than
   inline interpolation (Finding 8).
5. Redact personal coordinates and identifiers from committed API samples
   (Finding 9).
