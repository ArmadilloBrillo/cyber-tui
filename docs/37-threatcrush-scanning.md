# Feature 37: ThreatCrush PR scanning

`.github/workflows/threatcrush.yml` runs [ThreatCrush](https://github.com/profullstack/threatcrush)
static analysis on every pull request into `dev` and `main`: hardcoded
credentials, injection and web-vulnerability patterns, unsafe deserialisation,
crypto/token misuse, and dependency-manifest tampering.

This complements the three existing `ci.yml` gates rather than replacing any of
them — `go vet`/`staticcheck` cover Go correctness, `govulncheck` covers known
CVEs in dependencies, and ThreatCrush covers source-level patterns and secrets
that neither of those look for.

## What runs

| Step | Purpose |
|---|---|
| Install ThreatCrush | `npm i -g @profullstack/threatcrush@latest`, retried 3× with backoff |
| Record the CLI interface | Logs `--version` and `scan --help` into every run |
| Detect the CLI output interface | Aborts if the CLI cannot emit SARIF |
| Scan | `threatcrush scan . --format sarif --fail-on critical,high` |
| Upload to the Security tab | SARIF via `codeql-action/upload-sarif`, category `threatcrush` |
| Build the report | Renders a findings table |
| Job summary / SARIF artifact / PR comment | Three delivery paths for the same report |

## Why the job blocks

`--fail-on critical,high` makes this a gate rather than a report, matching how
this repo already treats `govulncheck`. It is less noisy than it sounds:
ThreatCrush caps `pattern`-confidence findings at medium severity, so a bare
"this construct exists on this line" match cannot break a build. Only
`contextual` findings (the construct sits next to something that looks like
attacker-controlled input) and `evidence` findings (a committed credential is
committed whether or not a request reaches it) reach the failure floor.

To make the scan advisory instead, drop the `--fail-on` flag from the Scan step.

## Failing closed

The workflow is arranged so that a scan which did not run can never be reported
as a scan that found nothing — the failure mode that makes a security gate
worse than no gate, because it produces a green check on an unexamined diff.

- The **SARIF file**, not the exit code, is the evidence a scan happened. No
  file means the job fails with "this diff was NOT scanned".
- The `--format` capability is checked **up front**, not inferred from an exit
  code. CLIs before 0.3.0 exit `1` on `unknown option '--format'` — the same
  code the CLI uses for "findings at or above `--fail-on`".
- The report renders findings only on positive evidence of a completed scan
  (`status` in `clean`/`findings`). Any other state, including the empty string
  produced when an earlier step fails and the scan step is *skipped*, renders
  **NOT RUN**.

## Node 20 is deliberate

ThreatCrush depends on `better-sqlite3`, which ships prebuilt binaries for Node
20 but not for every newer runtime. On Node 24 the install falls through to a
node-gyp source build and fails. A security gate that cannot install is a
security gate that does not run.

## Fork PRs

`pull_request` gives fork PRs a read-only token, so the PR-comment step 403s on
fork submissions and is marked `continue-on-error`. The report is still in the
job summary and the SARIF artifact, and pass/fail is decided by the scan step,
not by whether a comment posted.

This deliberately does **not** use `pull_request_target` to obtain a writable
token: that event runs with repository secrets in scope against a checkout of
untrusted contributor code.

## Suppressions

Inline comments suppress a finding on the following line. The rule id is
optional; without one the whole line is suppressed. Suppressions are counted
and reported in the scan output, so a scan that came back quiet because
somebody silenced forty rules reads differently from a genuinely clean one.

```go
// threatcrush-disable-next-line secret-generic-credential  literal mock string, not a credential
IDToken: "mock-idtoken-" + email,
```

Six pre-existing false positives are suppressed at the point of the finding:

| File | Count | Why |
|---|---|---|
| `internal/api/mock.go` | 5 | Literal `"mock-idtoken-"` / `"mock-refresh-"` / `"mock-rtdb-"` strings in `MockClient`, flagged as hardcoded credentials |
| `docs/19-topics.md` | 1 | The `REFRESH_TOKEN` shell placeholder in a curl example |

Without these, every PR would fail the new gate on day one.

A scan of this repo reports **seven** suppressions, not six: the Go example
above is real text in a real file, so its own `threatcrush-disable-next-line`
line suppresses the mock string it demonstrates.

## Running locally

```bash
npm install -g @profullstack/threatcrush   # Node 20
threatcrush scan .                         # human-readable
threatcrush scan . --fail-on critical,high # what CI runs
```

`threatcrush.sarif` is gitignored so a local `--output` run does not get
committed.

## Provenance

Adapted from the upstream `threatcrush-scan@1.1.0` pack
(`profullstack/threatcrush` → `.github/workflows/threatcrush-scan.yml`), with
two deviations, both to match this repo's conventions:

- Actions are pinned to commit SHAs with version comments, as in `ci.yml`, not
  floating major tags.
- The legacy text-output compatibility path is dropped rather than vendoring
  upstream's Python SARIF converter into a Go repository to support a CLI older
  than the current npm release. An unrecognised interface now stops the job,
  which is the same fail-closed property by a shorter route.
