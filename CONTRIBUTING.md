# Contributing to cyber-tui

cyber-tui is a TUI client for [cyberspace.online](https://cyberspace.online) built with Go and Bubble Tea.

## Branch workflow

1. Fork the repo and clone your fork
2. Branch from `dev`:
   ```
   git checkout dev && git pull
   git checkout -b feature/your-feature
   ```
3. Make your changes, commit with [Conventional Commits](#commit-messages)
4. Push and open a PR targeting `dev`

**Never push directly to `dev` or `main`.** Both branches require a PR and passing CI.

### Branch naming

| Prefix | Purpose |
|--------|---------|
| `feature/` | New functionality |
| `fix/` | Bug fixes |
| `chore/` | Tooling, config, dependencies |
| `hotfix/` | Critical fixes against `main` — must also merge back to `dev` |

Keep branches short-lived. Rebase onto the latest `dev` before opening a PR to avoid conflicts in shared docs.

## Commit messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add topic subscription toggle
fix: clamp selectedIndex before refresh
docs: add API backlog entry for pagination bug
test: add unit tests for cursor navigation
chore: upgrade bubble tea to v0.26
refactor: extract auth token refresh into middleware
```

## Running tests and linter locally

```bash
go test ./...
go vet ./...
staticcheck ./...     # install: go install honnef.co/go/tools/cmd/staticcheck@latest
```

All three must be clean before your PR can merge.

## Definition of Done

A PR is ready to merge when:

1. `go test ./...` passes
2. `go vet ./...` and `staticcheck` are clean
3. New behaviour is documented in `docs/XX-feature-name.md`
4. `docs/00-project-reference.md` is updated if you added screens, types, shortcuts, or API methods
5. `docs/00-api-backlog.md` is updated if you found API issues or landed a previously-tracked feature
6. The PR checklist is complete and a maintainer has approved

## Building from source

```bash
make build        # current platform → dist/cyber-tui
make build-all    # all platforms   → dist/
```

Pre-built binaries for Linux (amd64/arm64) and Windows (amd64) are attached to each [GitHub Release](https://github.com/ArmadilloBrillo/cyber-tui/releases).

## Versioning

Release tags mirror the cyberspace.online API version they target (e.g. `v0.4.1`). Patch releases (`v0.4.2`) are used for TUI-only fixes between API updates. The current target API version is always recorded in `docs/00-latest-api-reference.md`.

## Questions

Open an issue or start a discussion — we're friendly.
