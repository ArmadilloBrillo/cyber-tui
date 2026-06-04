# CLAUDE.md

This file defines workflow guardrails for Claude. It is not project documentation.

---

## Project Reference

- **What:** TUI client for cyberspace.online (retro text-only social network)
- **Stack:** Go + Bubble Tea (TUI), Lip Gloss (styling), Wish (SSH hosting)
- **Code map:** `docs/00-project-reference.md` — comprehensive architecture, package structure, and file listing; **read this first to understand the codebase before targeting specific files for changes**
- **Project docs:** `docs/` folder — numbered per feature (e.g. `docs/01-auth.md`)
- **API status:** v0.5.0 — docs at https://api.cyberspace.online/docs.md — **always fetch before any API work** to catch changes
- **API snapshot:** `docs/00-latest-api-reference.md` — current baseline we're building against (check for drift against live URL)
- **Key API facts:** login uses email (not username) · chat/DMs use Firebase RTDB (SSE), not REST · cursor-based pagination
- **API backlog:** `docs/00-api-backlog.md` — unimplemented features and known server-side bugs; update whenever issues are found or features land

> Update this section as the project evolves so I can orient without a full code review.

---

## Git Workflow

### Branch structure
- `main` — releases only, merged from `dev` at milestone completion; protected, requires PR + CI
- `dev` — integration branch; protected, requires PR + CI
- `feature/short-description` — new features, branched from `dev`
- `fix/short-description` — bug fixes, branched from `dev`
- `hotfix/short-description` — critical fixes, branched from `main`, merged back to both `main` and `dev`
- `chore/short-description` — non-functional changes (tooling, config, deps)

### PR workflow
**Contributors (fork and PR):**
- Fork the repo, branch from `dev`, submit a PR targeting `dev`
- CI must pass before the maintainer merges

**Maintainer:**
- Direct push to `dev` is allowed for trivial changes (docs, config); CI will not run
- Open a PR when the change is significant enough to warrant CI validation
- Releases (`dev` → `main`): always open a PR so CI runs before merge

### Commit messages — Conventional Commits
- `feat:` new feature
- `fix:` bug fix
- `docs:` documentation only
- `test:` test additions or changes
- `chore:` maintenance, tooling, dependencies
- `refactor:` code restructure with no behavior change

### Merge rules — **HARD STOPS**
- **Never merge to `dev` or `main` without explicit user permission**
- Never merge to `dev` if any test fails
- Never merge to `dev` if the feature is undocumented
- Never merge to `dev` if there are linter warnings
- `dev` → `main` only when a full milestone is complete and user approves

### Releases — API-aligned versioning
- Release tags mirror the cyberspace.online API version they target: `v0.4.1`, `v0.5.0`, etc.
- Current target: **v0.5.0** (see `docs/00-latest-api-reference.md`)
- When the API ships a new version, open a new milestone on `dev` targeting that version
- When the milestone is complete and the user approves, merge `dev` → `main` and tag with the API version
- Incremental TUI releases within an API version use a fourth segment: `v0.5.0.1`, `v0.5.0.2`, etc.
- A milestone is a named set of features forming a coherent releasable version; the user defines scope and approves each release

---

## Coding Rules

- Always explain planned changes before making them
- Always ask when unsure — do not guess
- No monoliths: one responsibility per package, docs per feature
- No linter warnings — run all three before every commit:
  ```
  go test ./...
  go vet ./...
  staticcheck ./...
  ```
- Run `govulncheck ./...` before releases and in CI to catch vulnerable dependencies
- Never commit secrets, tokens, credentials, or SSH keys
- `.env` is gitignored; `.env.example` is committed with placeholder values only

---

## Testing

- Write unit tests for all new features
- After planning a feature, imagine how it could fail — write tests for those failure modes
- Automated smoke tests for end-to-end flows
- All tests must pass before requesting a merge to `dev`
- Always present test results to the user after running tests
- At the end of every development phase, present a summary table of all tests run with columns: Group | Tests | Result

---

## Definition of Done

A feature is complete when:
1. All unit tests pass
2. Automated smoke tests pass
3. No linter warnings
4. Feature is documented in `docs/XX-feature-name.md` (number matches feature number)
5. `docs/00-project-reference.md` is updated to reflect any new screens, types, API methods, keyboard shortcuts, or known limitations added by the feature
6. `docs/00-api-backlog.md` is updated — mark implemented items as done, add any new API issues discovered during development
7. User has explicitly approved the merge to `dev`

---

## Secrets

- Never commit: API tokens, passwords, SSH host keys, `.env` files
- Always commit: `.env.example` with placeholder values
- Credentials are loaded from environment variables at runtime
