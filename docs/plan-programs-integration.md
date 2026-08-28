# Plan: Programs Integration (possible future addition)

## Status: not started — feasibility notes only

This is not an approved feature and no implementation work has begun. It exists
to capture a feasibility assessment done while researching API v0.8.7 (see
`docs/00-api-backlog.md`'s "Programs (new in v0.8.7)" row), so the analysis
isn't lost if this is picked up later.

## What Programs is

API v0.8.7 added a program registry (`docs/00-latest-api-reference.md`,
"Programs" section) backing the website's `/terminal` `publish`/`browse`
gallery. A program is source code a member wrote and published; any member
can browse the gallery and read any published program's source. Every
program declares a `runtime`:

| `runtime` | Shape | Executes on |
|---|---|---|
| `web` | `export default { name, description, run(ctx, args) }` | the website's `/terminal` (browser JS) |
| `term` | `export default async (p) => number` | cyberspace's own "terminal machine" |
| `wasm` | a `wasm32-wasi` binary, stdio only | the terminal machine |

Endpoints: `GET /v1/programs` (browse/filter by `runtime`/`author`/`name`/
`mine`), `GET /v1/programs/:id/source` (read, `base64` for wasm/`utf8`
otherwise), `POST /v1/programs` (publish, tiered size/count limits by account
tier), `DELETE /v1/programs/:id` (recall/purge).

## Why this doesn't split cleanly into "implement it or don't"

For all three runtimes, *execution* already happens somewhere other than
cyber-tui — a browser or cyberspace's own terminal machine. The registry
itself is plain REST CRUD, no different in kind from Guilds or Topics. What's
actually in question is how far past "browse the registry" cyber-tui should
go, and the three runtimes have very different answers:

### Tier 1 — Browse / read / publish / recall (registry only)

Low effort, no new dependencies, no execution surface. A "Programs" screen
(list, filter by runtime/author/mine, view a program's description and
source, publish/recall your own) is the same shape as every other list
screen in this client. Buildable at any time regardless of what (if
anything) happens with the tiers below.

### Tier 2 — Run `wasm` programs locally

The one tier where cyber-tui could plausibly *execute* something itself,
because WASI's stdio contract is small and fully specified: a `wasm32-wasi`
binary reads bytes from stdin and writes bytes to stdout, nothing else (this
API explicitly scopes it to "stdio only" — no filesystem, network, clock, or
env access implied or required).

Sketch, if ever pursued:

1. Add [`wazero`](https://github.com/tetratelabs/wazero) — a WASI runtime
   written in pure Go, no cgo — as a new dependency. This is the only
   plausible runtime choice for this codebase: it compiles straight into the
   `cyber-tui` binary the way every other dependency does, with no external
   `libwasmtime`/`libwasmer` binary to ship, which matters given this
   project cross-compiles to 6 platforms in CI (`.github/workflows/release.yml`)
   with zero cgo today.
2. Fetch a program's source via `GET /v1/programs/:id/source`, base64-decode
   to the raw `.wasm` bytes.
3. Wire wazero's WASI config to `io.Pipe`s instead of the real OS stdio, and
   run `Instantiate`+entrypoint inside a `tea.Cmd` (this project's existing
   pattern for any async work) — never on the Bubble Tea `Update`/`View`
   goroutine.
4. Stream stdout into a pane (viewport), feed stdin from the pane's compose
   box while it has focus — effectively a small terminal-within-the-terminal.
5. Bound it: wazero supports execution limits (instruction/fuel counting) and
   a `context.Context` timeout, both necessary since this runs arbitrary code
   any member published, not code this project's own contributors wrote.

This is a real, scoped feature — new dependency, new execution/sandboxing
lifecycle, real UI design for a program's pane (focus, scroll, kill, lifetime)
— not a small addition on top of Tier 1.

### Tier 3 — Run `web` / `term` programs — not realistically supportable

Both need a JS engine, and worse, their entire capability surface is the
`ctx`/`p` parameter passed into the program — and the API docs never define
that object's shape. There's no way to know what a `term` program can
actually do (filesystem? more API calls? cyberspace-specific bindings?)
without reverse-engineering cyberspace's own terminal-machine implementation,
which this project has no visibility into. Not attempted; not planned.

## Open questions if this is picked up

- Does a TUI social client actually want a plugin-execution surface at all —
  is Tier 1 (browse-only) the right stopping point regardless of Tier 2's
  technical feasibility?
- Account-tier size/count limits (members 20×128KB, supporters 100×1MB,
  staff unlimited×10MB) — mirror the existing supporter-gating pattern used
  for `/song` attach (`docs/49-song-attach.md`) if publish is built.
- Tier 2's sandboxing bar: is a fuel/instruction limit and a timeout enough,
  or does this need a stricter resource story before shipping to real users
  running arbitrary community-published code?

## Critical files (if Tier 1 is built)

| File | Change |
|---|---|
| `internal/api/interface.go` + `internal/api/client.go` | `ListPrograms`, `GetProgramSource`, `CreateProgram`, `RecallProgram` |
| `internal/model/types.go` | `Program` type |
| `internal/ui/screens/programs.go` (new) | List/detail/publish screens, mirroring `guilds.go`'s or `topics.go`'s structure |
| `internal/ui/app.go` | Tab wiring, commands |
| `docs/5X-programs.md` (new) | Feature doc, once/if built |
