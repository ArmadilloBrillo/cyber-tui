// Package imgview fetches and displays images in the terminal using native
// graphics protocols (Kitty or iTerm2). Detection is done once at startup via
// environment variables; display uses tea.Exec to suspend Bubble Tea while the
// image is shown, then resumes on keypress.
package imgview

import "os"

// GraphicsProtocol identifies the terminal image display protocol available in
// the current execution environment.
type GraphicsProtocol int

const (
	ProtocolNone   GraphicsProtocol = iota
	ProtocolKitty                   // Kitty terminal and Ghostty
	ProtocolITerm2                  // iTerm2 and WezTerm
)

// DetectProtocol examines environment variables to identify the running
// terminal's graphics protocol support. Returns ProtocolNone when the terminal
// is unknown or unsupported.
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
