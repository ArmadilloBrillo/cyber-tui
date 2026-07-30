# Code & Security Review — 2026-07-24

**Reviewed commit:** `8835173` (main) · **Baseline for future comparisons — first review in this series.**

Scope: full repository (80 Go files, ~32,400 lines). Automated tooling was re-run fresh rather than trusted from CI history. Manual review focused on trust boundaries: authentication/token handling, the experimental SSH host mode, untrusted-content rendering, outbound network calls, and OS command execution — informed by `docs/30-security-hardening.md`, the prior hardening pass this review checks for drift against.

---

## Executive Summary

| Severity | Count |
|---|---|
| High | 1 |
| Medium | 1 |
| Low | 5 |
| Informational | 3 |

(3 Low + 2 Informational under Security Findings; 2 Low + 1 Informational under Code Quality Findings.)

The standout finding is **High**: chat messages (C-Mail DMs and cIRC chatrooms) bypass the app's central sanitization layer on four separate code paths, contradicting an invariant `docs/30-security-hardening.md` explicitly documents as universal ("`sanitize.Strings` is called at every wire-to-model converter... so all server text is sanitized at the API boundary before it can reach any screen"). This is a real regression against a documented guarantee, not a newly-discovered design gap — it should be the first thing fixed.

Everything else is contained: `go vet` and `staticcheck` are clean, all tests pass, and `govulncheck` surfaces one stdlib finding tied to the local Go toolchain version rather than the app's own code or dependencies. No SQL/command injection, no `InsecureSkipVerify`, no secrets in the repo, no unbounded resource consumption paths found.

---

## Security Findings

### HIGH — Chat message sender usernames and system replies bypass `sanitize.Strings`

**Files:**
- `internal/api/client.go:1496-1523` (`GetRoomMessages` — REST cIRC history)
- `internal/api/client.go:1611-1632` (`wireRTDBMessageToModel`, `wireRTDBCircMessageToModel` — RTDB SSE live messages)
- `internal/api/client.go:1636-1656` (`GetConversations` — C-Mail conversation list)
- `internal/api/client.go:1658-1684` (`GetMessages` — REST C-Mail history)
- `internal/ui/app.go:762, 798` (server-generated command reply text from `SendRoomMessage`/`SendMessage`, e.g. `/help` output — called at `app.go:2484, 2504` — passed to `AppendSystemMessage` unsanitized)
- `internal/ui/screens/render.go:344, 346` (`renderChatMessages` — renders `msg.From.Username` directly via `theme.Highlight.Render`, not through `markdown.Render`)

**The problem:** Every other wire-to-model conversion in `client.go` — posts, users, replies, notifications, guilds, notes (14 call sites) — calls `sanitize.Strings(&w)` before constructing the model value. The five chat-message conversion paths above do not. `msg.Body` gets a compensating strip at render time because `renderChatMessages` (`render.go:358`) passes it through `markdown.Render`, which calls `sanitize.Strip` internally — so message bodies are actually protected by accident, not by the documented boundary control. `msg.From.Username`, however, is rendered directly (`render.go:344,346`) with no sanitization at either layer, and the conversation-list "other user" username (`cmail.go:592`) has the same gap via `GetConversations`.

**Failure scenario:** A user sets their cyberspace.online display name/username to a string containing ANSI/OSC escape sequences (e.g. an OSC 52 clipboard-write sequence, or a cursor/title-manipulation sequence), then DMs the victim or posts in a shared cIRC room. The instant the victim's client renders the header line for that message — no click, no link-open required — the escape sequence executes in the victim's terminal. Depending on terminal emulator, this ranges from cosmetic (spoofed title) to data-exfiltration-adjacent (clipboard write) or a trigger for terminal-emulator-specific escape-sequence bugs.

**Recommendation:** Add `sanitize.Strings(&w)` (or equivalent per-field `sanitize.Strip`) to all five conversion sites listed above, matching the existing pattern used everywhere else in the file. As defense in depth, also route `msg.From.Username` through `sanitize.Strip` (or `markdown.Render`) at the render layer in `render.go`, so the guarantee doesn't depend solely on the API boundary being remembered for every new field. Add a regression test asserting sanitize coverage for these five paths specifically, since `internal/sanitize` currently sits at 100% coverage but that only proves the package itself works — it doesn't prove every call site uses it.

---

### MEDIUM — `apiTimestamp.UnmarshalJSON` logs unrecognized raw server bytes unconditionally, unsanitized

**File:** `internal/api/client.go:132` (also `client.go:716`, `parseTime`)

**The problem:** When a timestamp field on a `/v1/search` hit doesn't match any recognized shape, the raw bytes are logged via `log.Printf("api: apiTimestamp: unrecognized value, leaving empty: %s", b)` — unconditionally (not gated behind `isDebug()`), and before any sanitization. `b` is attacker-influenced (the comment at `client.go:80-90` notes this field is already observed to serialize inconsistently on live search hits).

**Failure scenario:** A crafted timestamp value containing terminal escape sequences reaches this log line and is written to stderr. In local TUI mode, stderr typically isn't the alt-screen the user is actively viewing, which limits impact; in SSH server mode (`internal/ssh/server.go`), stderr is the *host operator's* terminal, not the connecting client's — so this is a narrow but real vector for injecting sequences into the SSH host operator's own session via a value the operator never directly requested.

**Recommendation:** Sanitize `b` (or truncate/hex-encode it) before logging, at both this call site and `client.go:716`.

---

### LOW — `apiBaseURL` scheme downgrade via same-host redirect is not defended

**Files:** `internal/api/client.go:592-648` (`doRequest`), `cmd/cyber-tui/main.go:99-120` (`validateBaseURL`)

**The problem:** `validateBaseURL` correctly rejects a configured `http://` `apiBaseURL` to a non-loopback host, preventing the bearer token from being sent in cleartext by misconfiguration. `HTTPClient`'s underlying `http.Client` (`client.go:550`, `Timeout: 15s` only) uses no custom `CheckRedirect`. Go's default client strips `Authorization` on a cross-*host* redirect, but does **not** strip it on a same-host scheme downgrade (`https://api.example.com` → `http://api.example.com`). If the configured HTTPS host is ever compromised, DNS-hijacked, or misconfigured to issue such a redirect, the token would still be attached.

**Recommendation:** Set an explicit `CheckRedirect` on `HTTPClient`'s `http.Client` that refuses to follow any redirect whose scheme is `http`, closing the gap `validateBaseURL` otherwise only checks once at startup. Low priority — requires the trusted host itself to misbehave — but cheap to fix.

---

### LOW — `govulncheck`: stdlib `crypto/tls` ECH privacy leak (GO-2026-5856) in the active Go toolchain

**Scope:** Go toolchain, not app code or a direct/indirect dependency.

`govulncheck ./...` (fresh run against the locally installed `go1.26.4`) reports:

```
Vulnerability #1: GO-2026-5856
  Invoking Encrypted Client Hello privacy leak in crypto/tls
  Found in: crypto/tls@go1.26.4
  Fixed in: crypto/tls@go1.26.5
  Example traces: internal/rtdb/client.go (Put → http.Client.Do → tls handshake)
```

`go.mod` pins `toolchain go1.25.12` (`go.mod:5`), older than the `go1.26.4` used for this scan — so this specific finding's applicability depends on which Go version actually builds CI/release binaries, not what's pinned in `go.mod`. `docs/30-security-hardening.md §6` records the toolchain being pinned specifically to clear a prior `govulncheck` finding, so this pattern (govulncheck flags a toolchain-level CVE, project bumps `toolchain`) has precedent.

**Recommendation:** Confirm the Go version used by `.github/workflows/ci.yml` and `release.yml`, and bump the `toolchain` directive to `go1.26.5+` (or the fixed patch of whichever minor line is in use). Exploitability is low in practice — ECH is opt-in and this app doesn't appear to configure it — but the fix is a one-line toolchain bump.

---

### LOW — SSH host mode remains unauthenticated by design (unchanged since `docs/30-security-hardening.md`)

**Files:** `internal/ssh/server.go`, `cmd/cyber-tui/main.go:60-73`

No change from the prior hardening pass: any client that can reach the listening address gets a full session, no SSH auth is performed. This is clearly documented in three places (doc comment on `Serve`, a runtime stderr warning on startup, and `docs/30-security-hardening.md §4`), and per-connection API clients plus `WithEphemeralSession()` correctly prevent cross-session credential leakage and host-config writes — the isolation guarantees from the prior review still hold. Rated Low here only because it's already flagged and gated, not because the underlying exposure is small; **do not enable `sshListenAddr` on anything but a trusted/loopback network.**

**Recommendation:** No action required unless multi-user SSH auth becomes a real requirement — tracked as a known, accepted limitation. Consider adding a startup check that refuses to bind a non-loopback address without an explicit opt-in flag, mirroring the `allowInsecureApi` pattern, so the unauthenticated exposure requires two deliberate config choices instead of one.

---

### INFORMATIONAL — Image attachment fetch has no host allowlist (potential SSRF surface, contingent on server behavior)

**File:** `internal/ui/imgview/fetch.go`

`Fetch` downloads whatever URL is in an attachment's `Src` field with no restriction to an expected CDN domain. Size caps (10 MiB) and pixel caps (~52MP) are in place, which mitigate resource-exhaustion, but not the SSRF class: if the API can ever be induced to return an attacker-controlled `Src` (e.g. via an unvalidated user-submitted URL field), the client would fetch it, potentially reaching internal/loopback addresses from the *viewer's* machine. This is speculative — it depends on server-side behavior this review didn't verify — logged for awareness, not as a confirmed exploit.

**Recommendation:** If attachment URLs are always same-origin CDN URLs by server-side construction, this is a non-issue; worth a one-line confirmation from the API side. If not, add a host allowlist in `Fetch`.

---

### INFORMATIONAL — RTDB auth token passed as a URL query parameter

**File:** `internal/rtdb/client.go:311-323` (`buildURL`)

This is Firebase's own REST/SSE convention (`?auth=<token>`), not an app-level choice, so it's not actionable here — but it means the token appears in full URLs, which would be captured by any logging proxy, browser history equivalent, or crash reporter sitting between the client and Firebase. No such intermediary currently exists in this codebase. Noted for awareness if one is ever added.

---

## Code Quality Findings

### LOW — `internal/ui/imgview` undocumented in `docs/00-project-reference.md`

The project reference's Package Reference section (`docs/00-project-reference.md`, `## Package Reference`) lists `cmd/cyber-tui`, `internal/model`, `internal/api`, `internal/config`, `internal/rtdb`, `internal/ssh`, `internal/ui`, `internal/ui/screens`, `internal/ui/theme` — but not `internal/ui/imgview` (7 files) or `internal/ui/markdown`/`internal/ui/urlutil`, all of which exist on disk and are exercised by the app (image viewer support, markdown rendering, URL extraction/opening). Per this project's own Definition of Done, `docs/00-project-reference.md` is supposed to reflect every package. **Recommendation:** add a Package Reference entry for each.

### LOW — Several UI screen files are large, `guilds.go` most notably

Line counts in `internal/ui/screens`: `guilds.go` 1239, `profile.go` 1112, `feed.go` 897, `journal.go` 881, `topics.go` 811, `postdetail.go` 807, `cmail.go` 775, `search.go` 770. `guilds.go` in particular is ~63% larger than the next-largest non-test file. This isn't a defect, but it's worth a look for extraction opportunities (e.g. splitting list-rendering, key-handling, and update logic into separate files per screen, a pattern already used elsewhere in the codebase) before it grows further — consistent with this project's own "no monoliths" coding rule.

### INFORMATIONAL — Test coverage is uneven, concentrated away from the UI layer

```
internal/sanitize        100.0%
internal/ui/theme        100.0%
internal/rtdb              86.4%
internal/config            81.4%
internal/ui/urlutil        80.0%
internal/api                68.2%
internal/ssh                68.8%
internal/ui/markdown        73.4%
internal/ui/imgview         76.4%
internal/ui/screens         49.2%
internal/ui (root)          21.0%
cmd/cyber-tui                19.3%
cmd/apifetch                  0.0%  (dev tool, not shipped)
internal/model               no test files
internal/ui/markdown/cmd      0.0%  (dev tool, not shipped)
```

Core logic packages (`sanitize`, `rtdb`, `config`, `api`) are well covered. `internal/ui` (root — the Bubble Tea message hub in `app.go`) and `internal/ui/screens` are lower, which is common for TUI update/view code but is also exactly the layer where the sanitization-bypass finding above lived undetected. Not a call to chase a coverage number, but worth adding targeted tests for wire-to-model sanitization coverage specifically (see the High finding's recommendation) rather than broad coverage growth.

---

## Automated Tooling Results

All run fresh against commit `8835173` on 2026-07-24, Go `go1.26.4` (windows/amd64):

| Tool | Result |
|---|---|
| `go build ./...` | ✅ Pass |
| `go vet ./...` | ✅ Pass, no output |
| `staticcheck ./...` | ✅ Pass, no output |
| `go test ./... -cover` | ✅ All packages pass (see coverage table above) |
| `govulncheck ./...` | ⚠️ 1 finding — GO-2026-5856, stdlib `crypto/tls`, see LOW finding above. Exit code 3 (vulnerabilities found in code paths actually called). 2 additional advisories noted for imported/required packages the code doesn't call into (not detailed here; re-run with `-show verbose` if a full accounting is needed). |

CI (`.github/workflows/ci.yml`) already runs `go test`, `go vet`, and `staticcheck` on every PR to `dev`/`main`, and a `vulncheck` job runs `govulncheck`. These results should match CI's next run modulo the toolchain-version caveat noted in the govulncheck finding above.

---

## Actionable Recommendations

| Priority | Area | Finding | Recommended Action | Effort |
|---|---|---|---|---|
| 1 | Security | Chat sanitize bypass (High) | Add `sanitize.Strings`/`sanitize.Strip` to the 5 identified chat message/conversation conversion sites and the render-layer username path | Small — mirrors existing pattern, ~1-2 hrs incl. tests |
| 2 | Security | Unsanitized raw-bytes log line (Medium) | Sanitize or truncate `b`/`s` before `log.Printf` at `client.go:132,716` | Trivial |
| 3 | Security | Redirect scheme-downgrade gap (Low) | Add `CheckRedirect` refusing `http` targets on `HTTPClient`'s `http.Client` | Small |
| 4 | Dependencies | `crypto/tls` ECH CVE in active toolchain (Low) | Confirm CI's Go version; bump `toolchain` directive to a patched release | Trivial |
| 5 | Docs | `imgview`/`markdown`/`urlutil` missing from project reference (Low) | Add Package Reference entries per Definition of Done | Small |
| 6 | Code quality | `guilds.go` size (Low) | Evaluate splitting into per-concern files, matching patterns elsewhere in `screens/` | Medium |
| 7 | Test coverage | Low coverage in `internal/ui`/`internal/ui/screens` (Informational) | Prioritize targeted sanitize-coverage tests over broad coverage growth | Small–Medium |
| — | SSH mode | Unauthenticated by design (Low, unchanged) | No action required; consider a non-loopback opt-in flag if this mode sees more use | Small, optional |
| — | Attachments | Possible SSRF via unrestricted `Src` (Informational) | Confirm server-side URL construction; add host allowlist if not server-controlled | Small, pending confirmation |
