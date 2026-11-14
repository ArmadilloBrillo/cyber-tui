# Plan: move config to the standard per-OS location

## Context

The config file lives at `~/.cyber-tui.json`, a single dotfile in the home directory. The new debug and crash logs (`docs/plan-debug-log.md`) went to the user cache dir, which is hard for users to find. The user wants the config in the standard location for each OS, the logs next to it, and the client to move an existing config there automatically.

Decisions (confirmed with user):

- Linux and macOS: `$XDG_CONFIG_HOME/cyber-tui/` if set, otherwise `~/.config/cyber-tui/`. macOS uses `~/.config` (like gh, git, nvim) rather than `~/Library/Application Support`.
- Windows: `%AppData%\cyber-tui\` (`os.UserConfigDir`).
- File name: `config.json`. Logs: `logs/` inside the same directory.

## Approach

### 1. Paths (`internal/config/session.go`)
- New `Dir()`, a pure path function with no side effects. On Linux and macOS it must not call `os.UserConfigDir()`, which returns `~/Library/Application Support` on macOS. Instead it uses `$XDG_CONFIG_HOME` if that is an absolute path (the XDG spec says to ignore relative values), otherwise `~/.config`. `os.UserConfigDir()` is used only when `runtime.GOOS == "windows"`.
- `DefaultPath()` returns `Dir()/config.json`. `Load` and `Save` already go through it (`session.go:216`, `:236`), and so does the `apifetch` tool.
- `Save` creates the directory (0700) before writing, for fresh installs. Only `Save` and the migration create directories.

### 2. Migration (explicit `config.Migrate()`)
- `Migrate() (from, to string, err error)` is a separate function, not part of `Load`. `Load` is also called periodically (`saveConfig`, the wander check), so it stays free of side effects. `main.go` calls `Migrate` before the SSH branch, and `cmd/apifetch/main.go` calls it before `Load`. A non-empty `from` means a move happened.
- New file missing and `~/.cyber-tui.json` present: create the directory and `os.Rename` the old file to the new path. If the rename fails because the two paths are on different filesystems, copy it (written 0600, `O_EXCL`) and then remove the old file. A failure to remove after a successful copy is not fatal: the config is safe at the new path, and the "both present" rule covers later starts.
- Symlinked old file (dotfile managers such as stow or chezmoi): a plain rename would move a relative link into a different directory and break it. Instead, create a new symlink at the new path pointing to the resolved absolute target, then remove the old link. This keeps the file under the dotfile manager. If creating the symlink fails (e.g. Windows without privileges), copy the contents instead.
- Two instances migrating at once: after any rename or copy failure, check whether the new file now exists. If it does, another instance won the race, and this one carries on normally.
- Both present: the new file wins, and the old file is left untouched (it may hold different data, e.g. after a downgrade wrote to it).
- Migration fails for any other reason: `Migrate` returns an error naming both paths, so the user can move the file by hand. Silently falling back to the old path would split state between two files.
- `main.go` prints one line to stderr before the TUI starts: "config moved from ~/.cyber-tui.json to <path>".
- The old file is moved, not copied: an older binary will not find the config after an upgrade (logged out, local settings back to defaults). After a downgrade and a second upgrade, both files exist and the new one wins, so a login done on the old version is ignored (the user appears logged out). Both symptoms go in the release notes.

### 3. Logs (`cmd/cyber-tui/logfiles.go`)
- `logDir()` uses `config.Dir()/logs` instead of `os.UserCacheDir()/cyber-tui`, keeping the `os.TempDir()` fallback. Existing logs in the old cache dir are left alone.

### 4. References
- Code comments that name `~/.cyber-tui.json` (`session.go`, `app.go`, `main.go`, `theme.go`).
- `install.sh:149`: the `remove` message checks both `$HOME/.cyber-tui.json` and `${XDG_CONFIG_HOME:-$HOME/.config}/cyber-tui/config.json`, and names whichever exist.
- `README.md` (:51, :91, :125, :131), `docs/00-project-reference.md` (config section, `DefaultPath` row, the `debug` row log path), `.claude/commands/apifetch.md`.
- Older feature notes (`docs/11-session.md` etc.) stay as they are, because they describe what was true when they were written.

### Process
- Branch `feature/config-location`. The first commit is this plan, then a review agent checks it for edge cases before coding.
- Implement in the order 1+2 (with tests), 3, 4, testing after each step.

## Verification
- `go build ./...`, `go vet ./...`, `go test ./...`.
- `withTempHome` (`session_test.go:17`) also sets `XDG_CONFIG_HOME` to "", so tests never touch a developer's real config. Four existing tests write `~/.cyber-tui.json` directly and move to the new path: `TestLoad_CustomPaletteAbsentIsNil`, `TestLoad_WanderLustDefaultsFalse`, `TestLoad_WanderLustExplicitFalsePreserved` and `TestFilePermissions`. Without that change the first three would still pass, because `Load` returns an empty config, so their checks would prove nothing.
- New tests:
  - The old file only: moved, content and 0600 mode kept, the old file gone, the move reported.
  - Both files: the new one is loaded, the old one untouched.
  - Neither file: empty config, and `Save` creates the directory and file.
  - `XDG_CONFIG_HOME` absolute: used. Relative: ignored.
  - The new file already exists when the rename runs (simulated race): no error.
  - Symlinked old file with a relative target: the new path is a working link to the same file, and the old link is gone.
- Manual: copy your real `~/.cyber-tui.json` aside first, start the app, confirm the stderr line, that you're still logged in, and that the file is at `~/.config/cyber-tui/config.json` (0600, dir 0700). Turn on the debug log and restart; logs appear in `~/.config/cyber-tui/logs/`.
