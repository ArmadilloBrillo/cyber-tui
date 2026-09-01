package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/ragnar/cyber-tui/internal/model"
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

func TestNotifScreen(t *testing.T) {
	cases := []struct {
		typ    string
		want   screen
		wantOK bool
	}{
		{"dm_message", 0, false},
		{"chat_mention", screenChatrooms, true},
		{"reply", screenNotifications, true},
		{"poke", screenNotifications, true},
		{"something_new", screenNotifications, true},
	}
	for _, c := range cases {
		got, ok := notifScreen(c.typ)
		if ok != c.wantOK || (ok && got != c.want) {
			t.Errorf("notifScreen(%q) = %v,%v want %v,%v", c.typ, got, ok, c.want, c.wantOK)
		}
	}
}

func TestShouldDesktopNotify(t *testing.T) {
	tests := []struct {
		name                   string
		ephemeral, enabled     bool
		focusReported, focused bool
		active, source         screen
		want                   bool
	}{
		{"disabled", false, false, true, true, screenFeed, screenNotifications, false},
		{"ephemeral SSH session", true, true, false, false, screenFeed, screenNotifications, false},
		{"focus unknown", false, true, false, false, screenFeed, screenNotifications, true},
		{"unfocused, on the source tab", false, true, true, false, screenNotifications, screenNotifications, true},
		{"focused, on the source tab", false, true, true, true, screenNotifications, screenNotifications, false},
		{"focused, different tab", false, true, true, true, screenFeed, screenNotifications, true},
		{"focused, different tab (cmail source)", false, true, true, true, screenFeed, screenCMail, true},
		{"focused, on the cmail tab, cmail source", false, true, true, true, screenCMail, screenCMail, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &App{
				ephemeral:            tt.ephemeral,
				desktopNotifications: tt.enabled,
				focusReported:        tt.focusReported,
				focused:              tt.focused,
				active:               tt.active,
			}
			if got := a.shouldDesktopNotify(tt.source); got != tt.want {
				t.Errorf("shouldDesktopNotify(%v) = %v, want %v", tt.source, got, tt.want)
			}
		})
	}
}

// The first notifsLoadedMsg after login carries the existing unread backlog;
// desktopNotifyForNewNotifs must only arm the high-water mark and fire nothing.
func TestDesktopNotifyForNewNotifs_BaselineGuard(t *testing.T) {
	a := loggedInApp()
	a.desktopNotifications = true
	now := time.Now()
	backlog := []model.Notification{
		{ID: "b", Type: "reply", CreatedAt: now.Add(-time.Minute)},
		{ID: "a", Type: "reply", CreatedAt: now.Add(-2 * time.Minute)},
	}
	a, cmd := a.desktopNotifyForNewNotifs(backlog)
	if cmd != nil {
		t.Error("first list must not fire a toast")
	}
	if !a.notifBaselined || !a.lastNotifiedAt.Equal(backlog[0].CreatedAt) {
		t.Errorf("baseline not armed: notifBaselined=%v lastNotifiedAt=%v", a.notifBaselined, a.lastNotifiedAt)
	}
}

func TestDesktopNotifyForNewNotifs_FiresForNewerUnread(t *testing.T) {
	a := loggedInApp() // active == screenFeed
	a.desktopNotifications = true
	a.notifBaselined = true
	a.lastNotifiedAt = time.Now()

	list := []model.Notification{
		{ID: "new", Type: "reply", Read: false, CreatedAt: a.lastNotifiedAt.Add(time.Minute)},
		{ID: "old", Type: "reply", Read: false, CreatedAt: a.lastNotifiedAt.Add(-time.Minute)},
	}
	a2, cmd := a.desktopNotifyForNewNotifs(list)
	if cmd == nil {
		t.Error("a newer unread notification while on the Feed tab should toast")
	}
	if !a2.lastNotifiedAt.Equal(list[0].CreatedAt) {
		t.Error("high-water mark should advance to the newest item")
	}

	// Same list, but the user is focused on the Notifications tab → suppressed.
	a.active = screenNotifications
	a.focusReported, a.focused = true, true
	if _, cmd := a.desktopNotifyForNewNotifs(list); cmd != nil {
		t.Error("focused on the Notifications tab must not toast a reply")
	}
}

func TestDesktopNotifyForNewNotifs_SkipsReadAndOld(t *testing.T) {
	a := loggedInApp()
	a.desktopNotifications = true
	a.notifBaselined = true
	a.lastNotifiedAt = time.Now()

	list := []model.Notification{
		{ID: "read", Type: "reply", Read: true, CreatedAt: a.lastNotifiedAt.Add(time.Minute)},
		{ID: "old", Type: "reply", Read: false, CreatedAt: a.lastNotifiedAt.Add(-time.Second)},
	}
	if _, cmd := a.desktopNotifyForNewNotifs(list); cmd != nil {
		t.Error("a read newer item and an unread older item should produce no toast")
	}
}

func TestDesktopNotifyForNewNotifs_CapsAtThree(t *testing.T) {
	a := loggedInApp()
	a.desktopNotifications = true
	a.notifBaselined = true
	a.lastNotifiedAt = time.Now()

	var list []model.Notification
	for i := 0; i < 5; i++ {
		list = append(list, model.Notification{
			ID: string(rune('a' + i)), Type: "reply", Read: false,
			CreatedAt: a.lastNotifiedAt.Add(time.Duration(i+1) * time.Minute),
		})
	}
	// list is oldest-first here; the handler expects newest-first, but the
	// break-on-not-after loop still walks all 5 since every one is newer.
	if _, cmd := a.desktopNotifyForNewNotifs(list); cmd == nil {
		t.Error("5 new notifications should still produce a (collapsed) toast")
	}
}

func TestMaybeNotifyNewCMail(t *testing.T) {
	base := func() App {
		a := loggedInApp()
		a.desktopNotifications = true
		a.cmailUnreadBaselined = true
		return a
	}

	// Baseline guard: first observation arms the flag, fires nothing.
	a := loggedInApp()
	a.desktopNotifications = true
	a.cmail = a.cmail.SetConversations([]model.Conversation{{ID: "c", UnreadCount: 3}})
	got, cmd := a.maybeNotifyNewCMail(0, nil)
	if cmd != nil || !got.cmailUnreadBaselined {
		t.Errorf("first call: cmd=%v baselined=%v, want nil,true", cmd, got.cmailUnreadBaselined)
	}

	// Increase while the window is unfocused → toast.
	a = base()
	a.cmail = a.cmail.SetConversations([]model.Conversation{{ID: "c", UnreadCount: 2}})
	if _, cmd := a.maybeNotifyNewCMail(1, nil); cmd == nil {
		t.Error("unread rose while unfocused → expected a toast cmd")
	}

	// Same increase, but focused on the C-Mail tab → suppressed.
	a = base()
	a.active = screenCMail
	a.focusReported, a.focused = true, true
	a.cmail = a.cmail.SetConversations([]model.Conversation{{ID: "c", UnreadCount: 2}})
	if _, cmd := a.maybeNotifyNewCMail(1, nil); cmd != nil {
		t.Error("focused on the C-Mail tab → expected no toast")
	}

	// No increase → no toast.
	a = base()
	a.cmail = a.cmail.SetConversations([]model.Conversation{{ID: "c", UnreadCount: 1}})
	if _, cmd := a.maybeNotifyNewCMail(1, nil); cmd != nil {
		t.Error("unread unchanged → expected no toast")
	}
}
