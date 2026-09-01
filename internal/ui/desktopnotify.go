package ui

import (
	"fmt"
	"os"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
)

// desktopNotifyMax bounds the assembled "title: body" text. OS toasts truncate
// long strings anyway, and a short cap stops a hostile message body from
// flooding stdout.
const desktopNotifyMax = 120

// desktopNotifyString builds an OSC 9 desktop-notification escape
// ("\x1b]9;<text>\x07"). Every control character — including the ESC/BEL that
// frame the sequence, plus tab and newline — is stripped from title and body
// so server-supplied text can't inject further terminal escapes; the combined
// text is truncated to desktopNotifyMax runes.
func desktopNotifyString(title, body string) string {
	text := strings.TrimSpace(stripControl(title))
	if b := strings.TrimSpace(stripControl(body)); b != "" {
		if text != "" {
			text += ": "
		}
		text += b
	}
	if r := []rune(text); len(r) > desktopNotifyMax {
		text = strings.TrimSpace(string(r[:desktopNotifyMax]))
	}
	return "\x1b]9;" + text + "\x07"
}

// stripControl drops every C0/C1/DEL control rune (tab and newline included).
func stripControl(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

// shouldDesktopNotify reports whether a desktop (OSC 9) toast should fire for an
// event whose "home" tab is source: the user enabled notifications, this isn't
// an SSH-hosted session (the toast would pop on the host, not the connected
// client — the same reason OSC 52 clipboard copy is gated on a.ephemeral), and
// either the terminal window isn't the focused one, or it is but the user is
// looking at a different tab than the one the event belongs to.
func (a *App) shouldDesktopNotify(source screen) bool {
	if a.ephemeral || !a.desktopNotifications {
		return false
	}
	if !a.focusReported || !a.focused {
		return true // window not focused (or focus unknowable) → always notify
	}
	return a.active != source // focused → only if not already on that tab
}

// notifScreen maps a notification Type to the tab it belongs to for the focus
// gate. ok is false for types that must NOT toast from the notifications path:
// dm_message is owned by the C-Mail / RTDB path (see maybeNotifyNewCMail) so
// routing it here too would double-toast.
func notifScreen(typ string) (s screen, ok bool) {
	switch typ {
	case "dm_message":
		return 0, false
	case "chat_mention":
		return screenChatrooms, true
	default:
		return screenNotifications, true
	}
}

// desktopNotifyCmd returns a tea.Cmd that writes an OSC 9 desktop-notification
// escape to os.Stdout — the same "raw OS-directed escape past the renderer"
// path used for OSC 52 clipboard copy. Terminals without OSC 9 support ignore
// it silently. The caller must gate on App.shouldDesktopNotify(); this always
// writes.
func desktopNotifyCmd(title, body string) tea.Cmd {
	return func() tea.Msg {
		fmt.Fprint(os.Stdout, desktopNotifyString(title, body))
		return nil
	}
}
