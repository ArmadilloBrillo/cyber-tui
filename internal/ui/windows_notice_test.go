package ui

import (
	"runtime"
	"testing"

	"github.com/ragnar/cyber-tui/internal/ui/imgview"
)

// TestLoginSuccess_WindowsImageNotice covers the Windows-only startup notice
// added after the WezTerm/Windows inline-image investigation (see
// docs/plan-inline-images-improvements.md §9): every graphics protocol this
// app supports turned up a confirmed or documented Windows-specific
// rendering bug, so the notice is scoped to the OS, not a specific
// terminal or even a successfully detected protocol — it should fire
// whenever the user has terminal image viewing enabled on Windows,
// regardless of TERM_PROGRAM or whether imgview.DetectProtocol happened to
// recognize the terminal (mintty/Git Bash sets no TERM_PROGRAM at all and
// detects as ProtocolNone, but still goes through the same ConPTY layer as
// any other Windows terminal) — and never fire on other platforms or when
// the user has already opted into browser-based viewing.
func TestLoginSuccess_WindowsImageNotice(t *testing.T) {
	base := func() App {
		a := newTestApp()
		a.graphicsProtocol = imgview.ProtocolITerm2
		a.imageViewer = "terminal"
		a.ephemeral = false
		return a
	}

	t.Run("fires when images enabled", func(t *testing.T) {
		a, _, claimed := base().handleAuth(loginSuccessMsg{})
		if !claimed {
			t.Fatal("expected handleAuth to claim loginSuccessMsg")
		}
		if runtime.GOOS == "windows" {
			if a.notifyText == "" {
				t.Error("expected a startup notice on Windows with images enabled")
			}
		} else if a.notifyText != "" {
			t.Errorf("notifyText = %q, want empty on non-Windows", a.notifyText)
		}
	})

	t.Run("does not fire when imageViewer is browser", func(t *testing.T) {
		a := base()
		a.imageViewer = "browser"
		a, _, _ = a.handleAuth(loginSuccessMsg{})
		if a.notifyText != "" {
			t.Errorf("notifyText = %q, want empty when imageViewer=browser", a.notifyText)
		}
	})

	t.Run("still fires when no graphics protocol was detected (e.g. Git Bash/mintty)", func(t *testing.T) {
		a := base()
		a.graphicsProtocol = imgview.ProtocolNone
		a, _, _ = a.handleAuth(loginSuccessMsg{})
		if runtime.GOOS == "windows" {
			if a.notifyText == "" {
				t.Error("expected the notice even when DetectProtocol found nothing, on Windows")
			}
		} else if a.notifyText != "" {
			t.Errorf("notifyText = %q, want empty on non-Windows", a.notifyText)
		}
	})

	t.Run("does not fire for ephemeral SSH sessions", func(t *testing.T) {
		a := base()
		a.ephemeral = true
		a, _, _ = a.handleAuth(loginSuccessMsg{})
		if a.notifyText != "" {
			t.Errorf("notifyText = %q, want empty for ephemeral sessions", a.notifyText)
		}
	})
}
