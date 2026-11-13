# Plan: debug log setting + crash capture

## Context

Users need a way to collect diagnostics for crashes and errors and send them to the developer. A partial version exists: `"debug": true` in `~/.cyber-tui.json` sends the `log` package to `cyber-tui-debug.log` (`cmd/cyber-tui/main.go:90`). Gaps:

- The file lands in the current working directory, so users can't find it.
- Panics (in Bubble Tea's Update/View or in our own stream goroutines) print only to the terminal and are never saved.
- Failed API requests are not logged.
- `InitRTDB` debug output uses `fmt.Printf` to stdout (`internal/api/client.go:771`), which corrupts the TUI.
- With debug off, the stray `log.Printf` calls (e.g. `client.go:169`, `:919`) write to stderr over the TUI.
- Debug mode logs raw RTDB event payloads (`client.go:2300`, `:2571`), which include private message content.
- It can only be turned on by hand-editing the config file.

Decisions (confirmed with user):

- Settings screen toggle; takes effect on next launch.
- The log is metadata only: no message bodies, tokens or passwords.
- Crash stack traces are always written to a file, even with debug off.

## Approach

### 1. Log location and setup (`cmd/cyber-tui/main.go`)
- Directory: `os.UserCacheDir()/cyber-tui/` (Linux `~/.cache/cyber-tui`, macOS `~/Library/Caches/cyber-tui`, Windows `%LocalAppData%\cyber-tui`), created with mode 0700. If that fails (e.g. `$HOME` unset), fall back to `os.TempDir()/cyber-tui/`. If that fails too, skip file logging and crash capture: the app must still start.
- One dated file per run. The startup timestamp is in local time and sortable, plus the PID so two instances started in the same second never share a file, e.g. `2026-11-13T14-05-22-12345` (no colons, so it is valid on Windows). The same stamp is used for both files of a run.
- Debug on: open `debug-<stamp>.log` (0600) and point the `log` package at it. This replaces the `tea.LogToFile("cyber-tui-debug.log")` call.
- Pruning at startup: keep the newest 10 `debug-*.log` and the newest 10 `crash-*.log`, and delete older ones. Names sort chronologically, so a sorted `filepath.Glob` is enough.
- Debug off: `log.SetOutput(io.Discard)`, which stops stray logs from drawing over the TUI. This runs before the SSH-server branch in `main.go`, so SSH mode is silenced too. The debug file and crash capture stay local-TUI only.
- `CYBERSPACE_DEBUG_KEYS` gets its own `log.New(keysFile, ...)` logger instead of `tea.LogToFile`, which would take over the global logger and pull the debug lines into the keys log.
- First lines in the log: app version (`internal/version`), Go version, GOOS/GOARCH, TERM, TERM_PROGRAM, and the graphics protocol that was detected. The existing imgview `log.Printf` at `main.go:113` folds into this.

### 2. Crash capture (always on)
- `crash-<stamp>.log` in the same directory, opened (0600) at startup and passed to `debug.SetCrashOutput` (Go 1.23+). `SetCrashOutput` needs the file open before any crash happens. On a normal exit the file is still empty, so call `debug.SetCrashOutput(nil, ...)` first (the runtime holds a duplicate handle, which blocks deletion on Windows) and then remove it. Only runs that actually crashed leave a crash file. This catches fatal panics in our own goroutines (`internal/api`, `internal/rtdb` streams), which Bubble Tea does not recover. Those panics also print to the real terminal, because the runtime writes to fd 2 directly, but the process is dead by then.
- Bubble Tea recovers panics in Update and View itself. It prints the stack via `debug.PrintStack` (captured by the `os.Stderr` swap below) and the panic value to stdout (not captured). Add a deferred recover in `Root.Update` and `Root.View` (`internal/ui/app.go:861`, `:865`) that appends only a timestamp and the panic value to the crash file, then re-panics. Bubble Tea then restores the terminal and writes the stack, so the stack is not written twice.
- While the TUI is running, point `os.Stderr` at the crash file and restore it after `p.Run()` returns. Anything written to stderr while the alt-screen owns the display is either invisible or corrupts the screen, so it should go to the file instead. This also captures the stack Bubble Tea prints (`debug.PrintStack`, which writes to `os.Stderr`) for panics inside `tea.Cmd` goroutines. Only the panic value is lost there, because Bubble Tea prints that to stdout. Runtime fatal output is unaffected, since `SetCrashOutput` already writes to both.
- After `p.Run()` returns an error, write it to the crash file as well, then print to the restored stderr: the error plus "crash details written to <path>".
- Messages printed before the TUI starts (config, SSH, base-URL errors in `main.go`) stay terminal-only, because they appear on a normal screen the user can read.
- Rendered screen content (what `View()` draws) is never logged, because it contains posts and messages (metadata-only decision).

### 3. API error logging (`internal/api/client.go`, `doRequest` at :792)
- Debug on only: log method, path without the query string, status code, duration, and the error if any. Never log headers or request/response bodies.

### 4. Privacy cleanup (`internal/api/client.go`)
- `:771`: `fmt.Printf` becomes `log.Printf`, logging only the RTDB host. The URL is not expected to carry a token, but logging only the host is safe either way.
- `:2300` and `:2571`: log the event name and payload length instead of the payload.

### 5. Settings toggle
- New bool row "debug log" in `internal/ui/screens/settings.go`, under an existing app/advanced group, with the hint "applies on restart, writes to <path>". It saves to the existing `Config.Debug` field (`internal/config/session.go:48`). It follows the plumbing of the `wanderLust` local-config bool: SettingsModel field plus original baseline, `SetSaved`, `SharedConfigMsg`, `settingsSavedMsg`, and the App field and save in `internal/ui/app.go`.
- Update the `Debug` doc comment and the `debug` rows in `docs/00-project-reference.md` (:195, :780).

### Process
- Branch `feature/debug-log`. The first commit is this plan as `docs/plan-debug-log.md`, then a review agent checks it for edge cases before coding (per the project workflow).
- Implement in the order 4, 1, 2, 3, 5, testing after each step.

## Verification
- `go build ./...`, `go vet ./...`, `go test ./...`.
- New test (security path): an httptest server that returns a 500, with the client in debug mode and the log captured in a buffer. It asserts the line has method, path and status, and contains neither the bearer token nor body text.
- Settings test: toggling "debug log" changes the saved value (pattern in `settings_test.go`).
- Manual:
  - With debug on, run the app and confirm a dated `debug-*.log` in `~/.cache/cyber-tui/` has the header and API lines, the screen is not corrupted, a normal exit leaves no crash file, and the 11th launch prunes the oldest debug log.
  - Add a temporary uncommitted `panic` once in `Root.Update` and once in a stream goroutine. Also one in a `tea.Cmd`. In every case a dated `crash-*.log` gets the stack, the terminal is restored, and the stderr hint is printed. Repeat with debug off.
  - Grep the log after using C-Mail and chat to confirm no message text appears.
