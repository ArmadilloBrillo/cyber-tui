//go:build windows

package imgview

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// consoleFontInfoEx mirrors the Win32 CONSOLE_FONT_INFOEX struct.
// golang.org/x/sys/windows doesn't wrap GetCurrentConsoleFontEx directly, so
// the struct and the call itself are done by hand here.
type consoleFontInfoEx struct {
	cbSize      uint32
	nFont       uint32
	dwFontSizeX int16
	dwFontSizeY int16
	fontFamily  uint32
	fontWeight  uint32
	faceName    [32]uint16 // LF_FACESIZE
}

var (
	kernel32                    = windows.NewLazySystemDLL("kernel32.dll")
	procGetCurrentConsoleFontEx = kernel32.NewProc("GetCurrentConsoleFontEx")
)

// TerminalCellPixelSize queries the terminal's real per-cell pixel
// dimensions via the console host's current font size
// (GetCurrentConsoleFontEx). Unlike Kitty/iTerm2, Sixel has no terminal-side
// scale-to-fit, so callers must know the terminal's actual cell size to size
// the display box correctly. Returns ok=false if the API call fails or
// reports a degenerate size (fd isn't a real console screen buffer handle —
// e.g. redirected output — or the console host doesn't support the query),
// in which case callers fall back to an assumed cell size.
//
// Caveat: this reports the classic Win32 console host's font size. Many
// modern Windows terminal emulators (Windows Terminal, mintty, others) run
// the app over ConPTY rather than the legacy console host directly, and
// ConPTY's compatibility layer doesn't guarantee this reflects the actual
// terminal emulator's rendered font pixel size — it may be closer to
// accurate than the previous hardcoded guess, but isn't pixel-perfect for
// every terminal/configuration the way the real TIOCGWINSZ ioctl is on Unix.
func TerminalCellPixelSize(fd int) (cellW, cellH int, ok bool) {
	var info consoleFontInfoEx
	info.cbSize = uint32(unsafe.Sizeof(info))
	r, _, _ := procGetCurrentConsoleFontEx.Call(
		uintptr(fd),
		0, // bMaximumWindow = FALSE: current font, not the maximized-window one
		uintptr(unsafe.Pointer(&info)),
	)
	if r == 0 || info.dwFontSizeX <= 0 || info.dwFontSizeY <= 0 {
		return 0, 0, false
	}
	return int(info.dwFontSizeX), int(info.dwFontSizeY), true
}
