//go:build !windows

package imgview

import "golang.org/x/sys/unix"

// TerminalCellPixelSize queries the terminal's real per-cell pixel dimensions
// via TIOCGWINSZ. Unlike Kitty/iTerm2, Sixel has no terminal-side scale-to-fit,
// so callers must know the terminal's actual cell size to size the display box
// correctly. Returns ok=false if the terminal doesn't report pixel geometry
// (some terminals/multiplexers report zero), in which case callers should fall
// back to an assumed cell size. Safe to call at any point — this is a
// read-only kernel ioctl unrelated to the stdin byte stream or raw-mode state.
func TerminalCellPixelSize(fd int) (cellW, cellH int, ok bool) {
	ws, err := unix.IoctlGetWinsize(fd, unix.TIOCGWINSZ)
	if err != nil || ws.Col == 0 || ws.Row == 0 || ws.Xpixel == 0 || ws.Ypixel == 0 {
		return 0, 0, false
	}
	return int(ws.Xpixel) / int(ws.Col), int(ws.Ypixel) / int(ws.Row), true
}
