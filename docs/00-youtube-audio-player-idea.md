# YouTube Audio Playback in cyber-tui

## Context

Posts on cyberspace.online can have audio attachments with `origin: "youtube"` — the API returns a YouTube URL in `Attachment.Src` along with metadata (title, artist, genre). Currently the TUI renders these as a text line with `[AUDIO]` badge and metadata. The user is asking whether a real player could be embedded.

## Feasibility Assessment

**True embedded YouTube player: not possible.** Terminals render text/ANSI only; there is no way to embed a browser-based video player widget.

**What IS possible: background audio via `mpv`**

`mpv` (cross-platform media player) can play YouTube URLs directly when `yt-dlp` is installed alongside it. It supports:
- `--no-video` flag for audio-only playback
- IPC socket (`--input-ipc-server`) for sending pause/stop/seek commands from the TUI
- Background process mode (no terminal takeover)

This would let the TUI launch `mpv` in the background, show a "now playing" bar, and accept keyboard commands to control playback.

## Critical Constraint: SSH Mode

The app runs over SSH via Wish. If launched over SSH, `mpv` would play audio on the **server** — not the client's speakers. This is a fundamental OS-level limitation and cannot be worked around in the TUI. Background audio playback is only useful in **local mode**.

## Proposed Implementation (local mode only)

### External dependencies required
- `mpv` installed and on `PATH`
- `yt-dlp` installed (mpv uses it to resolve YouTube URLs)

### TUI changes

1. **Play key on audio attachment** (`p` key in Feed/PostDetail when cursor is on a post with audio attachment)
   - Extract YouTube URL from `Attachment.Src` where `Type == "audio"`
   - Spawn `mpv --no-video --really-quiet <url>` as background process via `os/exec`
   - Store PID + track metadata in app state

2. **Now Playing bar** — thin overlay at the bottom of the screen (above status bar)
   - Shows: `♫ Now playing: <artist> – <title>` 
   - Keys: `space` = pause/resume (via mpv IPC), `s` = stop (kill process), `]`/`[` = volume up/down

3. **mpv IPC** — on Linux/macOS use Unix socket (`/tmp/cyber-mpv.sock`); on Windows use named pipe
   - Send JSON commands: `{"command": ["cycle", "pause"]}`, `{"command": ["quit"]}`

4. **Graceful degradation** — if `mpv` is not found on PATH, show a toast: `mpv not found — install mpv + yt-dlp to enable audio playback`

### Files to modify
- `internal/ui/screens/shared.go` — add play key hint in audio attachment rendering
- `internal/ui/screens/feed.go` — handle `p` key for audio posts
- `internal/ui/screens/postdetail.go` — handle `p` key for audio posts/replies
- `internal/ui/app.go` — hold now-playing state, handle player process lifecycle
- New: `internal/player/mpv.go` — mpv process management + IPC

### Alternative: simpler approach (no IPC)

Just open the YouTube URL in the default browser (already works via `'o'` key). No new dependencies, no SSH conflict, trivially simple. The downside is it leaves the TUI to open a browser window.

## Recommendation

The simplest useful improvement is to **detect `origin: "youtube"` attachments and make `'o'` open the YouTube URL directly** (this may already work since URLs are extracted from attachment `Src` fields). The mpv approach is more immersive but adds real complexity, external dependencies, and only works locally.

## Verification
- Test with a post that has `origin: "youtube"` attachment
- Verify `p` key spawns mpv and audio plays
- Verify `s` key stops playback cleanly
- Verify graceful degradation when mpv is absent
- Test SSH mode: confirm no audio-on-server issue (no player should be spawned over SSH)
