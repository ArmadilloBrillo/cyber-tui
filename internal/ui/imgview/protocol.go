// Package imgview fetches and displays images in the terminal using native
// graphics protocols (Kitty, iTerm2, or Sixel). Kitty/iTerm2 detection is done
// once at startup via environment variables; Sixel has no such env-var signal,
// so it's detected via an active DA1 terminal query instead (see ProbeSixel).
// Display uses tea.Exec to suspend Bubble Tea while the image is shown, then
// resumes on keypress.
package imgview

import (
	"bytes"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/muesli/cancelreader"
	"golang.org/x/term"
)

// GraphicsProtocol identifies the terminal image display protocol available in
// the current execution environment.
type GraphicsProtocol int

const (
	ProtocolNone   GraphicsProtocol = iota
	ProtocolKitty                   // Kitty terminal and Ghostty
	ProtocolITerm2                  // iTerm2 and WezTerm
	ProtocolSixel                   // xterm, foot, mlterm, contour, mintty, yaft, ...
)

// DetectProtocol examines environment variables to identify the running
// terminal's graphics protocol support. Returns ProtocolNone when the terminal
// is unknown or unsupported. Sixel terminals don't reliably set an env var, so
// they're not detected here — see ProbeSixel.
//
// WezTerm supports both the Kitty and iTerm2 protocols on Linux, but its
// Windows build does not implement Kitty graphics at all
// (wezterm/wezterm#5757) — only iTerm2-protocol images work there. Mapped to
// ProtocolITerm2 unconditionally rather than splitting on GOOS, since that's
// the one protocol guaranteed to work across all of WezTerm's platforms; see
// docs/plan-inline-images-improvements.md for the Kitty-protocol experiment
// this reverts (it hit that Windows limitation) and the still-open black-
// image-render investigation on the iTerm2 path.
func DetectProtocol() GraphicsProtocol {
	if os.Getenv("KITTY_WINDOW_ID") != "" {
		return ProtocolKitty
	}
	switch os.Getenv("TERM_PROGRAM") {
	case "ghostty":
		return ProtocolKitty
	case "iTerm.app":
		return ProtocolITerm2
	case "WezTerm":
		return ProtocolITerm2
	}
	return ProtocolNone
}

// ProtocolFromName maps a config override string ("kitty", "iterm2", "sixel",
// "none") to a GraphicsProtocol, for when autodetection is unreliable (see
// Config.GraphicsProtocol). ok is false for an empty or unrecognized name.
func ProtocolFromName(name string) (proto GraphicsProtocol, ok bool) {
	switch name {
	case "kitty":
		return ProtocolKitty, true
	case "iterm2":
		return ProtocolITerm2, true
	case "sixel":
		return ProtocolSixel, true
	case "none":
		return ProtocolNone, true
	default:
		return ProtocolNone, false
	}
}

// da1ProbeTimeout bounds how long ProbeSixel waits for a terminal to answer a
// DA1 query. A local pty round-trip normally takes low single-digit
// milliseconds, but Windows terminals (mintty/Git Bash over ConPTY) have been
// observed to occasionally exceed 150ms, causing intermittent false
// negatives that flip Sixel detection off for the whole session; this is a
// generous, hand-picked ceiling, not a measured value — adjust if real
// terminals need more slack.
const da1ProbeTimeout = 500 * time.Millisecond

// ProbeSixel checks whether the terminal connected to stdin/stdout supports
// Sixel graphics by sending a DA1 (Primary Device Attributes) query and
// inspecting the response for attribute 4. It must be called before Bubble
// Tea takes over stdin, since afterwards raw reads would race Bubble Tea's
// own input reader. Returns false on any error, timeout, or non-terminal
// stdin — it never blocks longer than da1ProbeTimeout and never leaves the
// terminal in raw mode.
// TEMPORARY: log.Printf calls below are a diagnostic for a Windows/mintty
// intermittent-detection report, mirroring the CYBER_TUI_DEBUG_LOG pattern
// used (and later removed) for the WezTerm investigation in
// docs/plan-inline-images-improvements.md §9. Remove once resolved.
func ProbeSixel(stdin, stdout *os.File) bool {
	fd := int(stdin.Fd())
	if !term.IsTerminal(fd) {
		log.Printf("imgview: ProbeSixel: IsTerminal(stdin)=false, skipping probe")
		return false
	}
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		log.Printf("imgview: ProbeSixel: MakeRaw failed: %v", err)
		return false
	}
	defer term.Restore(fd, oldState)

	if _, err := stdout.WriteString("\x1b[c"); err != nil {
		log.Printf("imgview: ProbeSixel: DA1 query write failed: %v", err)
		return false
	}

	cr, err := cancelreader.NewReader(stdin)
	if err != nil {
		log.Printf("imgview: ProbeSixel: cancelreader.NewReader failed: %v", err)
		return false
	}
	defer cr.Close()

	type readResult struct {
		buf []byte
		err error
	}
	done := make(chan readResult, 1)
	go func() {
		buf := make([]byte, 64)
		n, err := cr.Read(buf)
		done <- readResult{buf[:n], err}
	}()

	start := time.Now()
	select {
	case res := <-done:
		elapsed := time.Since(start)
		if res.err != nil {
			log.Printf("imgview: ProbeSixel: read error after %v: %v", elapsed, res.err)
			return false
		}
		supported := ParseDA1SixelSupport(res.buf)
		log.Printf("imgview: ProbeSixel: DA1 response after %v: %q sixel=%v", elapsed, res.buf, supported)
		return supported
	case <-time.After(da1ProbeTimeout):
		cr.Cancel()
		log.Printf("imgview: ProbeSixel: timed out after %v with no response", da1ProbeTimeout)
		return false
	}
}

// ParseDA1SixelSupport reports whether a raw DA1 response (as read from the
// terminal, e.g. "\x1b[?62;1;2;4;6;9;15;18;21;22c") declares Sixel support
// (attribute 4). It searches for the "\x1b[?...c" pattern anywhere in the
// buffer, tolerating stray leading bytes, and never panics on malformed,
// empty, or non-DA1 input. Exported so ProbeSixel's response-parsing logic
// has a pure, unit-testable seam independent of the live terminal I/O.
func ParseDA1SixelSupport(resp []byte) bool {
	start := bytes.Index(resp, []byte("\x1b[?"))
	if start < 0 {
		return false
	}
	body := resp[start+len("\x1b[?"):]
	end := bytes.IndexByte(body, 'c')
	if end < 0 {
		return false
	}
	for _, field := range bytes.Split(body[:end], []byte(";")) {
		n, err := strconv.Atoi(string(field))
		if err == nil && n == 4 {
			return true
		}
	}
	return false
}
