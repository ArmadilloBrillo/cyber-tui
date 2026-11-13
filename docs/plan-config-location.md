# Plan: move config to the standard per-OS location

## Context

The config file lives at `~/.cyber-tui.json`, a single dotfile in the home directory. The new debug and crash logs (`docs/plan-debug-log.md`) went to the user cache dir, which is hard for users to find. The user wants the config in the standard location for each OS, the logs next to it, and the client to move an existing config there automatically.

Decisions (confirmed with user):

- Linux and macOS: `$XDG_CONFIG_HOME/cyber-tui/` if set, otherwise `~/.config/cyber-tui/`. macOS uses `~/.config` (like gh, git, nvim) rather than `~/Library/Application Support`.
- Windows: `%AppData%\cyber-tui\` (`os.UserConfigDir`).
- File name: `config.json`. Logs: `logs/` inside the same directory.

## Approach

### 1. Paths (`internal/config/session.go`)
- New `Dir()`: the rules above; created with mode 0700 when needed.
- `DefaultPath()` returns `Dir()/config.json`. `Load` and `Save` already go through it (`session.go:216`, `:236`), and so does the `apifetch` tool.
- `Save` creates the directory (0700) before writing, for fresh installs.

### 2. Migration (inside `Load`, the first config access on every start)
- New file missing and `~/.cyber-tui.json` present: create the directory and `os.Rename` the old file to the new path. If the rename fails because the two paths are on different filesystems, copy it (written 0600, `O_EXCL`) and then remove the old file.
- Both present: the new file wins, and the old file is left untouched (it may hold different data, e.g. after a downgrade wrote to it).
- Migration fails: `Load` returns an error naming both paths, so the user can move the file by hand. Silently falling back to the old path would split state between two files.
- `Load` reports that a move happened (e.g. a `Migrated` path returned alongside the config, or a package-level hook). `main.go` prints one line to stderr before the TUI starts: "config moved from ~/.cyber-tui.json to <path>".
- The old file is moved, not copied: an older binary will not find the config after an upgrade (logged out, local settings back to defaults). This goes in the release notes.

### 3. Logs (`cmd/cyber-tui/logfiles.go`)
- `logDir()` uses `config.Dir()/logs` instead of `os.UserCacheDir()/cyber-tui`, keeping the `os.TempDir()` fallback. Existing logs in the old cache dir are left alone.

### 4. References
- Code comments that name `~/.cyber-tui.json` (`session.go`, `app.go`, `main.go`, `theme.go`).
- `install.sh:149`: the `remove` message checks for and names the new path, and the old one as well.
- `README.md` (:51, :91, :125, :131), `docs/00-project-reference.md` (config section, `DefaultPath` row, the `debug` row log path), `.claude/commands/apifetch.md`.
- Older feature notes (`docs/11-session.md` etc.) stay as they are, because they describe what was true when they were written.

### Process
- Branch `feature/config-location`. The first commit is this plan, then a review agent checks it for edge cases before coding.
- Implement in the order 1+2 (with tests), 3, 4, testing after each step.

## Verification
- `go build ./...`, `go vet ./...`, `go test ./...`.
- Config tests set `HOME`/`USERPROFILE` (`withTempHome`, `session_test.go:17`) and must also clear `XDG_CONFIG_HOME`. Existing tests that write `~/.cyber-tui.json` directly move to the new path.
- New tests:
  - The old file only: moved, content and 0600 mode kept, the old file gone, the move reported.
  - Both files: the new one is loaded, the old one untouched.
  - Neither file: empty config, and `Save` creates the directory and file.
  - `XDG_CONFIG_HOME` set: used.
- Manual: copy your real `~/.cyber-tui.json` aside first, start the app, confirm the stderr line, that you're still logged in, and that the file is at `~/.config/cyber-tui/config.json` (0600, dir 0700). Turn on the debug log and restart; logs appear in `~/.config/cyber-tui/logs/`.
