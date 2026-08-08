//go:build windows

package imgview

// TerminalCellPixelSize always reports unavailable on Windows: there's no
// TIOCGWINSZ equivalent wired up here, so callers fall back to an assumed
// cell size (the same behavior as before this function existed).
func TerminalCellPixelSize(fd int) (cellW, cellH int, ok bool) {
	return 0, 0, false
}
