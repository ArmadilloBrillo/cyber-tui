// Package imgview fetches and displays images in the terminal using native
// graphics protocols (Kitty, iTerm2, or Sixel). Kitty/iTerm2 detection is done
// once at startup via environment variables; Sixel has no such env-var signal,
// so it's detected via an active DA1 terminal query instead (see ProbeSixel).
// Display uses tea.Exec to suspend Bubble Tea while the image is shown, then
// resumes on keypress.
package imgview

import (
	"bytes"
	"os"
	"strconv"
	"time"

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

// da1ProbeTimeout bounds how long ProbeSixel waits for a terminal to answer a
// DA1 query. A local pty round-trip normally takes low single-digit
// milliseconds; this is a generous, hand-picked ceiling, not a measured
// value — adjust if real terminals need more slack.
const da1ProbeTimeout = 150 * time.Millisecond

// ProbeSixel checks whether the terminal connected to stdin/stdout supports
// Sixel graphics by sending a DA1 (Primary Device Attributes) query and
// inspecting the response for attribute 4. It must be called before Bubble
// Tea takes over stdin, since afterwards raw reads would race Bubble Tea's
// own input reader. Returns false on any error, timeout, or non-terminal
// stdin — it never blocks longer than da1ProbeTimeout and never leaves the
// terminal in raw mode.
func ProbeSixel(stdin, stdout *os.File) bool {
	fd := int(stdin.Fd())
	if !term.IsTerminal(fd) {
		return false
	}
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return false
	}
	defer term.Restore(fd, oldState)

	if _, err := stdout.WriteString("\x1b[c"); err != nil {
		return false
	}

	type readResult struct {
		buf []byte
		err error
	}
	done := make(chan readResult, 1)
	go func() {
		buf := make([]byte, 64)
		n, err := stdin.Read(buf)
		done <- readResult{buf[:n], err}
	}()

	select {
	case res := <-done:
		if res.err != nil {
			return false
		}
		return ParseDA1SixelSupport(res.buf)
	case <-time.After(da1ProbeTimeout):
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
