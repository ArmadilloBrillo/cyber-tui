package screens

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ragnar/cyber-tui/internal/model"
)

// initNotifs creates a ready NotificationsModel with the given notifications loaded.
func initNotifs(notifs []model.Notification) NotificationsModel {
	m := NewNotificationsModel()
	// Initialise viewport via WindowSizeMsg.
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = m.SetNotifs(notifs, "")
	return m
}

func makeNotif(id, typ, targetID string, read bool) model.Notification {
	return model.Notification{
		ID:        id,
		Type:      typ,
		Read:      read,
		CreatedAt: time.Now(),
		Actor:     model.NotificationActor{ID: "u1", Username: "testuser"},
		TargetID:  targetID,
	}
}

// runKey sends a single key to the model and returns the updated model + resolved message.
func runKey(m NotificationsModel, key string) (NotificationsModel, tea.Msg) {
	var msg tea.KeyMsg
	switch key {
	case "enter", "\r":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	default:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
	m2, cmd := m.Update(msg)
	if cmd == nil {
		return m2, nil
	}
	return m2, cmd()
}

// runKeyAll sends a single key and returns all resolved messages (handles tea.Batch).
func runKeyAll(m NotificationsModel, key string) (NotificationsModel, []tea.Msg) {
	m2, first := runKey(m, key)
	if first == nil {
		return m2, nil
	}
	if batch, ok := first.(tea.BatchMsg); ok {
		var msgs []tea.Msg
		for _, cmd := range batch {
			if cmd != nil {
				msgs = append(msgs, cmd())
			}
		}
		return m2, msgs
	}
	return m2, []tea.Msg{first}
}

// --- navigation ---

func TestNotifs_CursorDown_Increments(t *testing.T) {
	notifs := []model.Notification{makeNotif("n1", "reply", "p1", false), makeNotif("n2", "poke", "", false)}
	m := initNotifs(notifs)
	m2, _ := runKey(m, "j")
	if m2.selectedIndex != 1 {
		t.Errorf("expected selectedIndex 1, got %d", m2.selectedIndex)
	}
}

func TestNotifs_CursorUp_AtTop_EmitsRefreshMsg(t *testing.T) {
	notifs := []model.Notification{makeNotif("n1", "reply", "p1", false)}
	m := initNotifs(notifs)
	_, msg := runKey(m, "k")
	_, ok := msg.(RefreshNotifsMsg)
	if !ok {
		t.Fatalf("expected RefreshNotifsMsg when pressing k at index 0, got %T", msg)
	}
}

func TestNotifs_CursorUp_AtTop_SetsRefreshing(t *testing.T) {
	notifs := []model.Notification{makeNotif("n1", "reply", "p1", false)}
	m := initNotifs(notifs)
	m2, _ := runKey(m, "k")
	if !m2.refreshing {
		t.Error("expected refreshing to be true after k at index 0")
	}
}

func TestNotifs_CursorUp_NotAtTop_Decrements(t *testing.T) {
	notifs := []model.Notification{makeNotif("n1", "reply", "p1", false), makeNotif("n2", "poke", "", false)}
	m := initNotifs(notifs)
	m.selectedIndex = 1
	m2, _ := runKey(m, "k")
	if m2.selectedIndex != 0 {
		t.Errorf("expected selectedIndex 0, got %d", m2.selectedIndex)
	}
}

func TestNotifs_CursorDown_AtBottom_EmitsLoadMore(t *testing.T) {
	notifs := []model.Notification{makeNotif("n1", "reply", "p1", false)}
	m := initNotifs(notifs)
	m.nextCursor = "cursor-abc"
	m.exhausted = false
	_, msg := runKey(m, "j")
	lm, ok := msg.(LoadMoreNotifsMsg)
	if !ok {
		t.Fatalf("expected LoadMoreNotifsMsg, got %T", msg)
	}
	if lm.Cursor != "cursor-abc" {
		t.Errorf("expected cursor cursor-abc, got %s", lm.Cursor)
	}
}

func TestNotifs_CursorDown_Exhausted_NoCmd(t *testing.T) {
	notifs := []model.Notification{makeNotif("n1", "reply", "p1", false)}
	m := initNotifs(notifs)
	m.exhausted = true
	_, msg := runKey(m, "j")
	if msg != nil {
		t.Errorf("expected nil cmd when exhausted, got %T", msg)
	}
}

// --- mark read ---

func TestNotifs_MarkRead_EmitsMsg(t *testing.T) {
	notifs := []model.Notification{makeNotif("n1", "reply", "p1", false)}
	m := initNotifs(notifs)
	_, msg := runKey(m, "m")
	mr, ok := msg.(MarkNotifReadMsg)
	if !ok {
		t.Fatalf("expected MarkNotifReadMsg, got %T", msg)
	}
	if mr.ID != "n1" {
		t.Errorf("expected ID n1, got %s", mr.ID)
	}
}

func TestNotifs_MarkRead_OptimisticUpdate(t *testing.T) {
	notifs := []model.Notification{makeNotif("n1", "reply", "p1", false)}
	m := initNotifs(notifs)
	m2, _ := runKey(m, "m")
	if !m2.notifs[0].Read {
		t.Error("expected notifs[0].Read to be true after 'm'")
	}
}

func TestNotifs_MarkAllRead_EmitsMsg(t *testing.T) {
	notifs := []model.Notification{makeNotif("n1", "reply", "p1", false), makeNotif("n2", "poke", "", false)}
	m := initNotifs(notifs)
	_, msg := runKey(m, "M")
	_, ok := msg.(MarkAllNotifsReadMsg)
	if !ok {
		t.Fatalf("expected MarkAllNotifsReadMsg, got %T", msg)
	}
}

func TestNotifs_MarkAllRead_OptimisticUpdate(t *testing.T) {
	notifs := []model.Notification{makeNotif("n1", "reply", "p1", false), makeNotif("n2", "poke", "", false)}
	m := initNotifs(notifs)
	m2, _ := runKey(m, "M")
	for i, n := range m2.notifs {
		if !n.Read {
			t.Errorf("expected notifs[%d].Read to be true after 'M'", i)
		}
	}
}

// --- jump to post ---

func TestNotifs_Enter_Reply_EmitsShowPost(t *testing.T) {
	notifs := []model.Notification{makeNotif("n1", "reply", "p1", false)}
	m := initNotifs(notifs)
	_, msg := runKey(m, "enter")
	sp, ok := msg.(ShowNotificationPostMsg)
	if !ok {
		t.Fatalf("expected ShowNotificationPostMsg, got %T", msg)
	}
	if sp.PostID != "p1" {
		t.Errorf("expected PostID p1, got %s", sp.PostID)
	}
}

func TestNotifs_Enter_ShowPostMsg_HasNotifID(t *testing.T) {
	notifs := []model.Notification{makeNotif("n1", "reply", "p1", false)}
	m := initNotifs(notifs)
	_, msg := runKey(m, "enter")
	sp, ok := msg.(ShowNotificationPostMsg)
	if !ok {
		t.Fatalf("expected ShowNotificationPostMsg, got %T", msg)
	}
	if sp.NotifID != "n1" {
		t.Errorf("expected NotifID n1, got %s", sp.NotifID)
	}
}

func TestNotifs_Enter_ShowPostMsg_HasReplyID(t *testing.T) {
	notif := model.Notification{
		ID: "n1", Type: "reply", Read: false,
		CreatedAt: time.Now(),
		Actor:     model.NotificationActor{ID: "u1", Username: "testuser"},
		TargetID:  "p1",
		ReplyID:   "r99",
	}
	m := initNotifs([]model.Notification{notif})
	_, msg := runKey(m, "enter")
	sp, ok := msg.(ShowNotificationPostMsg)
	if !ok {
		t.Fatalf("expected ShowNotificationPostMsg, got %T", msg)
	}
	if sp.ReplyID != "r99" {
		t.Errorf("expected ReplyID r99, got %s", sp.ReplyID)
	}
}

func TestNotifs_Enter_AutoMarksRead(t *testing.T) {
	notifs := []model.Notification{makeNotif("n1", "reply", "p1", false)}
	m := initNotifs(notifs)
	m2, _ := runKey(m, "enter")
	if !m2.notifs[0].Read {
		t.Error("expected notification to be marked read optimistically on enter")
	}
}

func TestNotifs_Enter_Mention_EmitsShowPost(t *testing.T) {
	notifs := []model.Notification{makeNotif("n1", "mention", "p2", false)}
	m := initNotifs(notifs)
	_, msg := runKey(m, "enter")
	_, ok := msg.(ShowNotificationPostMsg)
	if !ok {
		t.Fatalf("expected ShowNotificationPostMsg, got %T", msg)
	}
}

func TestNotifs_Enter_Poke_EmitsShowUserProfileMsg(t *testing.T) {
	notifs := []model.Notification{makeNotif("n1", "poke", "", false)}
	m := initNotifs(notifs)
	m2, msgs := runKeyAll(m, "enter")
	var gotProfile bool
	var gotMarkRead bool
	for _, msg := range msgs {
		switch msg.(type) {
		case ShowUserProfileMsg:
			gotProfile = true
		case MarkNotifReadMsg:
			gotMarkRead = true
		}
	}
	if !gotProfile {
		t.Error("expected ShowUserProfileMsg for poke enter")
	}
	if !gotMarkRead {
		t.Error("expected MarkNotifReadMsg for poke enter")
	}
	if m2.notifs[0].Read != true {
		t.Error("poke notification should be marked read optimistically")
	}
}

func TestNotifs_Enter_NewFollower_EmitsShowUserProfileMsg(t *testing.T) {
	notifs := []model.Notification{makeNotif("n1", "new_follower", "", false)}
	m := initNotifs(notifs)
	m2, msgs := runKeyAll(m, "enter")
	var gotProfile bool
	var gotMarkRead bool
	for _, msg := range msgs {
		switch msg.(type) {
		case ShowUserProfileMsg:
			gotProfile = true
		case MarkNotifReadMsg:
			gotMarkRead = true
		}
	}
	if !gotProfile {
		t.Error("expected ShowUserProfileMsg for new_follower enter")
	}
	if !gotMarkRead {
		t.Error("expected MarkNotifReadMsg for new_follower enter")
	}
	if m2.notifs[0].Read != true {
		t.Error("new_follower notification should be marked read optimistically")
	}
}

func TestNotifs_Enter_EmptyTargetID_IsNoop(t *testing.T) {
	notifs := []model.Notification{makeNotif("n1", "mention", "", false)}
	m := initNotifs(notifs)
	_, msg := runKey(m, "enter")
	if msg != nil {
		t.Errorf("expected nil when TargetID is empty, got %T", msg)
	}
}

// --- view profile ---

func TestNotifs_P_EmitsShowUserProfileMsg(t *testing.T) {
	notifs := []model.Notification{makeNotif("n1", "reply", "p1", false)}
	m := initNotifs(notifs)
	_, msg := runKey(m, "p")
	sp, ok := msg.(ShowUserProfileMsg)
	if !ok {
		t.Fatalf("expected ShowUserProfileMsg, got %T", msg)
	}
	if sp.Username != "testuser" {
		t.Errorf("expected Username testuser, got %s", sp.Username)
	}
}

func TestNotifs_P_EmptyList_IsNoop(t *testing.T) {
	m := initNotifs(nil)
	_, msg := runKey(m, "p")
	if msg != nil {
		t.Errorf("expected nil when no notifications, got %T", msg)
	}
}

// --- notifSummary ---

func TestNotifSummary_ThreadReply_WithAuthor(t *testing.T) {
	n := model.Notification{Type: "thread_reply", ThreadAuthorUsername: "7spires"}
	result := notifSummary(n)
	if result != "replied in @7spires's thread." {
		t.Errorf("unexpected summary: %q", result)
	}
}

func TestNotifSummary_ThreadReply_NoAuthor(t *testing.T) {
	n := model.Notification{Type: "thread_reply"}
	result := notifSummary(n)
	if result != "replied in a thread you're following." {
		t.Errorf("unexpected summary: %q", result)
	}
}

// --- notifIcon ---

func TestNotifIcon_Reply_Unread(t *testing.T) {
	n := model.Notification{Type: "reply", Read: false}
	icon := notifIcon(n)
	if !strings.Contains(icon, "↩") {
		t.Errorf("expected ↩ in icon, got %q", icon)
	}
}

func TestNotifIcon_Bookmark_Read(t *testing.T) {
	n := model.Notification{Type: "bookmark", Read: true}
	icon := notifIcon(n)
	if !strings.Contains(icon, "♥") {
		t.Errorf("expected ♥ in icon, got %q", icon)
	}
}

func TestNotifIcon_NewFollower(t *testing.T) {
	n := model.Notification{Type: "new_follower", Read: false}
	icon := notifIcon(n)
	if !strings.Contains(icon, "+") {
		t.Errorf("expected + in icon, got %q", icon)
	}
}

func TestNotifSummary_ReplyMention(t *testing.T) {
	n := model.Notification{Type: "reply_mention"}
	result := notifSummary(n)
	if result != "mentioned you in a reply." {
		t.Errorf("expected 'mentioned you in a reply.', got %q", result)
	}
}

func TestNotifIcon_ReplyMention(t *testing.T) {
	n := model.Notification{Type: "reply_mention", Read: false}
	icon := notifIcon(n)
	if !strings.Contains(icon, "@") {
		t.Errorf("expected @ in icon, got %q", icon)
	}
}

func TestNotifs_Enter_ReplyMention_EmitsShowPost(t *testing.T) {
	notifs := []model.Notification{makeNotif("n1", "reply_mention", "p2", false)}
	m := initNotifs(notifs)
	_, msg := runKey(m, "enter")
	_, ok := msg.(ShowNotificationPostMsg)
	if !ok {
		t.Fatalf("expected ShowNotificationPostMsg, got %T", msg)
	}
}

func TestNotifs_Enter_GuildNewThread_EmitsShowPost(t *testing.T) {
	notifs := []model.Notification{makeNotif("n1", "guild_new_thread", "p1", false)}
	m := initNotifs(notifs)
	_, msg := runKey(m, "enter")
	sp, ok := msg.(ShowNotificationPostMsg)
	if !ok {
		t.Fatalf("expected ShowNotificationPostMsg, got %T", msg)
	}
	if sp.PostID != "p1" {
		t.Errorf("expected PostID p1, got %s", sp.PostID)
	}
}

func TestNotifSummary_GuildNewThread_WithName(t *testing.T) {
	n := model.Notification{Type: "guild_new_thread", GuildName: "technica"}
	result := notifSummary(n)
	if result != "posted a new thread in technica." {
		t.Errorf("unexpected summary: %q", result)
	}
}

func TestNotifSummary_GuildNewThread_NoName(t *testing.T) {
	n := model.Notification{Type: "guild_new_thread"}
	result := notifSummary(n)
	if result != "posted a new thread." {
		t.Errorf("unexpected summary: %q", result)
	}
}

func TestNotifIcon_GuildNewThread(t *testing.T) {
	n := model.Notification{Type: "guild_new_thread", Read: false}
	icon := notifIcon(n)
	if !strings.Contains(icon, "#") {
		t.Errorf("expected # in icon, got %q", icon)
	}
}

// --- unread filter ---

func TestNotifs_UnreadFilter_Toggle(t *testing.T) {
	notifs := []model.Notification{makeNotif("n1", "reply", "p1", false)}
	m := initNotifs(notifs)
	if !m.showUnreadOnly {
		t.Fatal("expected showUnreadOnly to start true")
	}
	m2, _ := runKey(m, "u")
	if m2.showUnreadOnly {
		t.Error("expected showUnreadOnly to be false after u")
	}
	m3, _ := runKey(m2, "u")
	if !m3.showUnreadOnly {
		t.Error("expected showUnreadOnly to be true after second u")
	}
}

func TestNotifs_UnreadFilter_ToggleEmitsRefresh(t *testing.T) {
	notifs := []model.Notification{makeNotif("n1", "reply", "p1", false)}
	m := initNotifs(notifs)
	_, msg := runKey(m, "u")
	_, ok := msg.(RefreshNotifsMsg)
	if !ok {
		t.Fatalf("expected RefreshNotifsMsg after u toggle, got %T", msg)
	}
}

func TestNotifs_UnreadFilter_ToggleClearsNotifs(t *testing.T) {
	notifs := []model.Notification{makeNotif("n1", "reply", "p1", false)}
	m := initNotifs(notifs)
	m2, _ := runKey(m, "u")
	if len(m2.notifs) != 0 {
		t.Errorf("expected notifs to be cleared after toggle, got %d", len(m2.notifs))
	}
}

func TestNotifs_ShowUnreadOnly_Accessor(t *testing.T) {
	m := NewNotificationsModel()
	if !m.ShowUnreadOnly() {
		t.Error("expected ShowUnreadOnly true by default")
	}
	m.showUnreadOnly = false
	if m.ShowUnreadOnly() {
		t.Error("expected ShowUnreadOnly false after setting field")
	}
}

func TestNotifs_UnreadFilter_ResetsIndex(t *testing.T) {
	notifs := []model.Notification{
		makeNotif("n1", "reply", "p1", false),
		makeNotif("n2", "poke", "", false),
	}
	m := initNotifs(notifs)
	m.selectedIndex = 1
	m2, _ := runKey(m, "u")
	if m2.selectedIndex != 0 {
		t.Errorf("expected selectedIndex 0 after filter toggle, got %d", m2.selectedIndex)
	}
}

func TestNotifs_UnreadFilter_HidesRead(t *testing.T) {
	notifs := []model.Notification{
		makeNotif("n1", "reply", "p1", true),  // read
		makeNotif("n2", "poke", "", false),     // unread
		makeNotif("n3", "bookmark", "p3", true), // read
	}
	m := initNotifs(notifs)
	m.showUnreadOnly = true
	visible := m.visibleNotifs()
	if len(visible) != 1 {
		t.Fatalf("expected 1 visible notification, got %d", len(visible))
	}
	if visible[0].ID != "n2" {
		t.Errorf("expected n2 to be the only visible notification, got %s", visible[0].ID)
	}
}

func TestNotifs_UnreadFilter_AllRead_EmptyState(t *testing.T) {
	notifs := []model.Notification{
		makeNotif("n1", "reply", "p1", true),
	}
	m := initNotifs(notifs)
	m.showUnreadOnly = true
	content, _ := m.buildContent()
	if !strings.Contains(content, "all caught up") {
		t.Errorf("expected 'all caught up' empty state, got: %s", content)
	}
}

// --- relative timestamps ---

func TestFormatRelativeTime_JustNow(t *testing.T) {
	now := time.Now()
	result := formatRelativeTime(now.Add(-30*time.Second), now, time.UTC)
	if result != "just now" {
		t.Errorf("expected 'just now', got %q", result)
	}
}

func TestFormatRelativeTime_Minutes(t *testing.T) {
	now := time.Now()
	result := formatRelativeTime(now.Add(-5*time.Minute), now, time.UTC)
	if result != "5m ago" {
		t.Errorf("expected '5m ago', got %q", result)
	}
}

func TestFormatRelativeTime_Hours(t *testing.T) {
	now := time.Now()
	result := formatRelativeTime(now.Add(-2*time.Hour), now, time.UTC)
	if result != "2h ago" {
		t.Errorf("expected '2h ago', got %q", result)
	}
}

func TestFormatRelativeTime_Days(t *testing.T) {
	now := time.Now()
	result := formatRelativeTime(now.Add(-3*24*time.Hour), now, time.UTC)
	if result != "3d ago" {
		t.Errorf("expected '3d ago', got %q", result)
	}
}

func TestFormatRelativeTime_OlderFallback(t *testing.T) {
	now := time.Now()
	old := now.Add(-8 * 24 * time.Hour)
	result := formatRelativeTime(old, now, time.UTC)
	expected := old.In(time.UTC).Format("02-Jan")
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

// --- day separators ---

func TestDayLabel_Today(t *testing.T) {
	now := time.Now()
	result := dayLabel(now, now, time.UTC)
	if result != "today" {
		t.Errorf("expected 'today', got %q", result)
	}
}

func TestDayLabel_Yesterday(t *testing.T) {
	now := time.Now()
	yesterday := now.Add(-24 * time.Hour)
	result := dayLabel(yesterday, now, time.UTC)
	if result != "yesterday" {
		t.Errorf("expected 'yesterday', got %q", result)
	}
}

func TestDayLabel_OlderDate(t *testing.T) {
	now := time.Now()
	old := now.Add(-5 * 24 * time.Hour)
	result := dayLabel(old, now, time.UTC)
	expected := old.In(time.UTC).Format("Mon 2 Jan")
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestDaySeparator_AppearsOnDayChange(t *testing.T) {
	now := time.Now()
	notifs := []model.Notification{
		{ID: "n1", Type: "reply", Actor: model.NotificationActor{Username: "u"}, CreatedAt: now, TargetID: "p1"},
		{ID: "n2", Type: "poke", Actor: model.NotificationActor{Username: "u"}, CreatedAt: now.Add(-48 * time.Hour)},
	}
	m := initNotifs(notifs)
	content, _ := m.buildContent()
	if !strings.Contains(content, "today") {
		t.Error("expected 'today' separator in content")
	}
	if !strings.Contains(content, "──") {
		t.Error("expected separator line (──) in content")
	}
}

// --- setters ---

func TestNotifs_SetNotifs_ResetsIndex(t *testing.T) {
	notifs := []model.Notification{makeNotif("n1", "reply", "p1", false), makeNotif("n2", "poke", "", false)}
	m := initNotifs(notifs)
	m.selectedIndex = 1
	m = m.SetNotifs(notifs, "")
	if m.selectedIndex != 0 {
		t.Errorf("expected selectedIndex 0 after SetNotifs, got %d", m.selectedIndex)
	}
}

func TestNotifs_SetNotifs_ClearsRefreshing(t *testing.T) {
	notifs := []model.Notification{makeNotif("n1", "reply", "p1", false)}
	m := initNotifs(notifs)
	m.refreshing = true
	m = m.SetNotifs(notifs, "")
	if m.refreshing {
		t.Error("expected refreshing to be false after SetNotifs")
	}
}

func TestNotifs_AppendNotifs_PreservesIndex(t *testing.T) {
	notifs := []model.Notification{makeNotif("n1", "reply", "p1", false), makeNotif("n2", "poke", "", false)}
	m := initNotifs(notifs)
	m.selectedIndex = 1
	extra := []model.Notification{makeNotif("n3", "like", "p3", false)}
	m = m.AppendNotifs(extra, "")
	if m.selectedIndex != 1 {
		t.Errorf("expected selectedIndex 1 after AppendNotifs, got %d", m.selectedIndex)
	}
}

func TestNotifs_SetError_SetsErr(t *testing.T) {
	m := NewNotificationsModel()
	err := errForTest("something went wrong")
	m = m.SetError(err)
	if m.err != err {
		t.Error("expected err to be set")
	}
}

func TestNotifs_UnreadCount(t *testing.T) {
	notifs := []model.Notification{
		makeNotif("n1", "reply", "p1", false),
		makeNotif("n2", "poke", "", true),
		makeNotif("n3", "bookmark", "p2", false),
	}
	m := initNotifs(notifs)
	if m.UnreadCount() != 2 {
		t.Errorf("expected UnreadCount 2, got %d", m.UnreadCount())
	}
}

func TestNotifs_IsReady(t *testing.T) {
	m := NewNotificationsModel()
	if m.IsReady() {
		t.Error("expected IsReady false before WindowSizeMsg")
	}
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if !m.IsReady() {
		t.Error("expected IsReady true after WindowSizeMsg")
	}
}

// errForTest is a minimal error value for testing SetError.
type errForTest string

func (e errForTest) Error() string { return string(e) }
