package ui

import (
	"strings"
	"testing"
)

func TestDesktopNotifyString_Format(t *testing.T) {
	got := desktopNotifyString("C-Mail", "new message")
	want := "\x1b]9;C-Mail: new message\x07"
	if got != want {
		t.Errorf("desktopNotifyString = %q, want %q", got, want)
	}
}

func TestDesktopNotifyString_TitleOnly(t *testing.T) {
	got := desktopNotifyString("cyberspace", "")
	want := "\x1b]9;cyberspace\x07"
	if got != want {
		t.Errorf("desktopNotifyString = %q, want %q", got, want)
	}
}

func TestDesktopNotifyString_StripsInjectedControls(t *testing.T) {
	// A hostile message body trying to close the OSC 9 sequence early and
	// start its own (window title / clipboard / cursor moves) plus newlines.
	got := desktopNotifyString("C-Mail", "evil\x1b]9;pwned\x07\r\nsecond line\x1b[2J")

	// Exactly one framing ESC and one framing BEL, both at the ends.
	if strings.Count(got, "\x1b") != 1 || strings.Count(got, "\x07") != 1 {
		t.Fatalf("framing not unique: %q", got)
	}
	if !strings.HasPrefix(got, "\x1b]9;") || !strings.HasSuffix(got, "\x07") {
		t.Fatalf("not a well-formed OSC 9 sequence: %q", got)
	}
	inner := strings.TrimSuffix(strings.TrimPrefix(got, "\x1b]9;"), "\x07")
	for _, r := range inner {
		if r < 0x20 || r == 0x7f {
			t.Fatalf("control rune %U survived in payload %q", r, inner)
		}
	}
}

func TestDesktopNotifyString_Truncates(t *testing.T) {
	body := strings.Repeat("x", 500)
	got := desktopNotifyString("t", body)
	inner := strings.TrimSuffix(strings.TrimPrefix(got, "\x1b]9;"), "\x07")
	if len([]rune(inner)) > desktopNotifyMax {
		t.Errorf("payload len = %d, want <= %d", len([]rune(inner)), desktopNotifyMax)
	}
}

// The first unreadCountMsg after login carries the existing unread backlog;
// notifCountBaselined must swallow it so no desktop notification fires for
// notifications the user has already had.
func TestDesktopNotify_UnreadCountBaselineGuard(t *testing.T) {
	a := loggedInApp()
	a.desktopNotifications = true
	if a.notifCountBaselined {
		t.Fatal("notifCountBaselined should start unset")
	}
	a, _, _ = a.handleNotifications(unreadCountMsg{count: 9, exact: true})
	if !a.notifCountBaselined {
		t.Error("first unreadCountMsg must set notifCountBaselined")
	}
	if a.polledUnreadCount != 9 {
		t.Errorf("polledUnreadCount = %d, want 9", a.polledUnreadCount)
	}
}

func TestShouldDesktopNotify(t *testing.T) {
	tests := []struct {
		name                                       string
		ephemeral, enabled, focusReported, focused bool
		want                                       bool
	}{
		{"disabled", false, false, false, false, false},
		{"ephemeral SSH session", true, true, false, false, false},
		{"enabled, focus unknown", false, true, false, false, true},
		{"enabled, known unfocused", false, true, true, false, true},
		{"enabled, known focused", false, true, true, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &App{
				ephemeral:            tt.ephemeral,
				desktopNotifications: tt.enabled,
				focusReported:        tt.focusReported,
				focused:              tt.focused,
			}
			if got := a.shouldDesktopNotify(); got != tt.want {
				t.Errorf("shouldDesktopNotify() = %v, want %v", got, tt.want)
			}
		})
	}
}
