package screens

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
)

// LoadMoreNotifsMsg is emitted when the viewport hits the bottom and more pages are available.
type LoadMoreNotifsMsg struct{ Cursor string }

// RefreshNotifsMsg is emitted when the user presses up at the top of the list.
type RefreshNotifsMsg struct{}

// MarkNotifReadMsg is emitted when the user presses 'm' on a highlighted notification.
type MarkNotifReadMsg struct{ ID string }

// MarkAllNotifsReadMsg is emitted when the user presses 'M'.
type MarkAllNotifsReadMsg struct{}

// ShowNotificationPostMsg is emitted when the user presses Enter on a navigable notification.
// NotifID is included so the app can mark the notification read as part of the same action.
// ReplyID is non-empty for reply/thread_reply notifications; the app uses it to scroll
// PostDetail to the specific reply after replies have loaded.
type ShowNotificationPostMsg struct {
	PostID  string
	NotifID string
	ReplyID string
}

type NotificationsModel struct {
	notifs         []model.Notification
	notifOffsets   []int // start line of each notification within the viewport content
	viewport       viewport.Model
	width          int
	height         int
	selectedIndex  int
	ready          bool
	loading        bool
	fetching       bool // true while the initial (or tab-switch) load is in flight
	refreshing     bool
	exhausted      bool
	nextCursor     string
	showUnreadOnly bool
	err            error
	relaxed        bool
	loc            *time.Location
}

func NewNotificationsModel() NotificationsModel {
	return NotificationsModel{showUnreadOnly: true}
}

// IsReady reports whether the viewport has been initialised.
func (m NotificationsModel) IsReady() bool { return m.ready }

func (m NotificationsModel) SetFetching() NotificationsModel {
	m.fetching = true
	m.err = nil
	if m.ready {
		m = m.refreshContent()
	}
	return m
}

func (m NotificationsModel) SetNotifs(notifs []model.Notification, cursor string) NotificationsModel {
	m.notifs = notifs
	m.nextCursor = cursor
	m.exhausted = cursor == ""
	m.err = nil
	m.loading = false
	m.fetching = false
	m.refreshing = false
	m.selectedIndex = 0
	if m.ready {
		m = m.refreshContent()
		m.viewport.GotoTop()
	}
	return m
}

func (m NotificationsModel) AppendNotifs(notifs []model.Notification, cursor string) NotificationsModel {
	m.notifs = append(m.notifs, notifs...)
	m.nextCursor = cursor
	m.exhausted = cursor == ""
	m.loading = false
	m.fetching = false
	if m.ready {
		m = m.refreshContent() // selectedIndex preserved; scroll position preserved
	}
	return m
}

func (m NotificationsModel) SetError(err error) NotificationsModel {
	m.err = err
	m.loading = false
	m.fetching = false
	m.refreshing = false
	if m.ready {
		m = m.refreshContent()
	}
	return m
}

func (m NotificationsModel) SetRelaxed(relaxed bool) NotificationsModel {
	m.relaxed = relaxed
	if m.ready {
		m = m.refreshContent()
		m = m.ensureSelectedVisible()
	}
	return m
}

func (m NotificationsModel) SetLocation(loc *time.Location) NotificationsModel {
	if loc == nil {
		loc = time.UTC
	}
	m.loc = loc
	if m.ready {
		m = m.refreshContent()
	}
	return m
}

// ShowUnreadOnly reports whether the screen is currently filtering to unread-only.
func (m NotificationsModel) ShowUnreadOnly() bool { return m.showUnreadOnly }

// UnreadCount returns the number of unread notifications in the current page.
func (m NotificationsModel) UnreadCount() int {
	n := 0
	for _, notif := range m.notifs {
		if !notif.Read {
			n++
		}
	}
	return n
}

func (m NotificationsModel) location() *time.Location {
	if m.loc == nil {
		return time.UTC
	}
	return m.loc
}

// MarkRead marks a single notification as read in the in-memory slice (optimistic update).
func (m NotificationsModel) MarkRead(id string) NotificationsModel {
	for i, n := range m.notifs {
		if n.ID == id {
			m.notifs[i].Read = true
			break
		}
	}
	// Clamp before rebuilding content so the highlight renders at the correct index.
	if visible := m.visibleNotifs(); m.selectedIndex >= len(visible) && len(visible) > 0 {
		m.selectedIndex = len(visible) - 1
	}
	if m.ready {
		m = m.refreshContent()
	}
	return m
}

// MarkAllRead marks every notification as read in the in-memory slice (optimistic update).
func (m NotificationsModel) MarkAllRead() NotificationsModel {
	for i := range m.notifs {
		m.notifs[i].Read = true
	}
	if m.ready {
		m = m.refreshContent()
	}
	return m
}

// visibleNotifs returns the slice of notifications to display, respecting the unread filter.
func (m NotificationsModel) visibleNotifs() []model.Notification {
	if !m.showUnreadOnly {
		return m.notifs
	}
	var out []model.Notification
	for _, n := range m.notifs {
		if !n.Read {
			out = append(out, n)
		}
	}
	return out
}

func (m NotificationsModel) refreshContent() NotificationsModel {
	content, offsets := m.buildContent()
	m.notifOffsets = offsets
	m.viewport.SetContent(content)
	return m
}

func (m NotificationsModel) ensureSelectedVisible() NotificationsModel {
	visible := m.visibleNotifs()
	if !m.ready || len(m.notifOffsets) == 0 || m.selectedIndex >= len(visible) {
		return m
	}
	itemStart := m.notifOffsets[m.selectedIndex]
	itemHeight := lipgloss.Height(m.renderNotif(visible[m.selectedIndex], false))
	itemEnd := itemStart + itemHeight - 1

	viewTop := m.viewport.YOffset
	viewBottom := viewTop + m.viewport.Height - 1

	if itemStart < viewTop {
		m.viewport.SetYOffset(itemStart)
	} else if itemEnd > viewBottom {
		if itemHeight <= m.viewport.Height {
			m.viewport.SetYOffset(itemEnd - m.viewport.Height + 1)
		} else {
			m.viewport.SetYOffset(itemStart)
		}
	}
	return m
}

func (m NotificationsModel) Init() tea.Cmd { return nil }

func (m NotificationsModel) Update(msg tea.Msg) (NotificationsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case SharedConfigMsg:
		m = m.SetRelaxed(msg.Relaxed)
		m = m.SetLocation(msg.Loc)
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if !m.ready {
			m.viewport = viewport.New(msg.Width, m.viewportHeight())
			m = m.refreshContent()
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = m.viewportHeight()
			m = m.refreshContent()
		}
		return m, nil

	case tea.KeyMsg:
		visible := m.visibleNotifs()
		switch msg.String() {
		case "up", "k":
			if m.selectedIndex > 0 {
				m.selectedIndex--
				m = m.refreshContent()
				m = m.ensureSelectedVisible()
			} else if !m.loading && !m.refreshing {
				m.refreshing = true
				m = m.refreshContent()
				return m, func() tea.Msg { return RefreshNotifsMsg{} }
			}
			return m, nil
		case "down", "j":
			if m.selectedIndex < len(visible)-1 {
				m.selectedIndex++
				m = m.refreshContent()
				m = m.ensureSelectedVisible()
			} else if !m.loading && !m.exhausted && m.nextCursor != "" {
				m.loading = true
				cursor := m.nextCursor
				m = m.refreshContent()
				m.viewport.ScrollDown(1)
				return m, func() tea.Msg { return LoadMoreNotifsMsg{Cursor: cursor} }
			}
			return m, nil
		case "m":
			if len(visible) == 0 || m.selectedIndex >= len(visible) {
				return m, nil
			}
			n := visible[m.selectedIndex]
			m = m.MarkRead(n.ID)
			return m, func() tea.Msg { return MarkNotifReadMsg{ID: n.ID} }
		case "M":
			m = m.MarkAllRead()
			return m, func() tea.Msg { return MarkAllNotifsReadMsg{} }
		case "u":
			m.showUnreadOnly = !m.showUnreadOnly
			m.selectedIndex = 0
			m.notifs = nil
			m.nextCursor = ""
			m.exhausted = false
			m.fetching = true
			m = m.refreshContent()
			return m, func() tea.Msg { return RefreshNotifsMsg{} }
		case "enter":
			if len(visible) == 0 || m.selectedIndex >= len(visible) {
				return m, nil
			}
			n := visible[m.selectedIndex]
			if n.Type == "poke" || n.Type == "new_follower" || n.Type == "unfollowed" ||
				n.Type == "chat_mention" || n.Type == "dm_message" {
				// No post to open — navigate to actor's profile and mark read.
				m = m.MarkRead(n.ID)
				notifID, username := n.ID, n.Actor.Username
				return m, tea.Batch(
					func() tea.Msg { return MarkNotifReadMsg{ID: notifID} },
					func() tea.Msg { return ShowUserProfileMsg{Username: username} },
				)
			}
			if n.TargetID == "" {
				return m, nil
			}
			// Optimistically mark as read and navigate to post.
			m = m.MarkRead(n.ID)
			notifID, postID, replyID := n.ID, n.TargetID, n.ReplyID
			return m, func() tea.Msg {
				return ShowNotificationPostMsg{PostID: postID, NotifID: notifID, ReplyID: replyID}
			}
		case "p":
			if len(visible) > 0 && m.selectedIndex < len(visible) {
				username := visible[m.selectedIndex].Actor.Username
				return m, func() tea.Msg { return ShowUserProfileMsg{Username: username} }
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)

	visible := m.visibleNotifs()
	if m.ready && !m.loading && !m.exhausted && m.viewport.AtBottom() && m.nextCursor != "" && len(visible) == len(m.notifs) {
		m.loading = true
		cursor := m.nextCursor
		m = m.refreshContent()
		m.viewport.ScrollDown(1)
		return m, tea.Batch(cmd, func() tea.Msg {
			return LoadMoreNotifsMsg{Cursor: cursor}
		})
	}

	return m, cmd
}

func (m NotificationsModel) viewportHeight() int {
	h := m.height - theme.ChromeHeight
	if h < 1 {
		h = 1
	}
	return h
}

func (m NotificationsModel) buildContent() (string, []int) {
	if m.fetching {
		return theme.Subtle.Render("  loading notifications…"), nil
	}
	visible := m.visibleNotifs()

	var prefix string
	startLine := 0
	if m.refreshing {
		prefix = theme.Subtle.Render("  fetching notifications...") + "\n"
		startLine = 1
	}

	if len(visible) == 0 {
		if m.err != nil {
			return prefix + theme.Subtle.Render("  couldn't load notifications"), nil
		}
		if m.showUnreadOnly {
			return prefix + theme.Subtle.Render("  all caught up"), nil
		}
		return prefix + theme.Subtle.Render("  no notifications"), nil
	}

	sep := "\n"
	lineInc := 0
	if m.relaxed {
		sep = "\n\n"
		lineInc = 1
	}

	offsets := make([]int, len(visible))
	var out string
	currentLine := startLine
	now := time.Now()
	loc := m.location()

	var prevDay time.Time // zero value = no previous day yet
	for i, n := range visible {
		day := n.CreatedAt.In(loc).Truncate(24 * time.Hour)
		if !day.Equal(prevDay) {
			sep := theme.Subtle.Render("  ── "+dayLabel(n.CreatedAt, now, loc)+" ──") + "\n"
			out += sep
			currentLine++
			prevDay = day
		}
		offsets[i] = currentLine
		rendered := m.renderNotif(n, i == m.selectedIndex)
		out += rendered + sep
		currentLine += lipgloss.Height(rendered) + lineInc
	}
	if m.loading {
		out += theme.Subtle.Render("  loading more…") + "\n"
	} else if m.exhausted {
		out += theme.Subtle.Render("  — end —") + "\n"
	}
	return prefix + out, offsets
}

// notifSummary returns the action text for a notification. For post/reply
// activity inside a guild it appends an " in #<guild>" clause (see withGuild).
func notifSummary(n model.Notification) string {
	base := baseNotifSummary(n)
	switch n.Type {
	case "new_post_friend", "new_post_following", "reply", "thread_reply":
		if n.GuildName != "" {
			return withGuild(base, n.GuildName)
		}
	}
	return base
}

// withGuild inserts " in #<guild>" before a single trailing period. The guild
// name is used verbatim (guilds choose their own casing); the leading # marks it
// as a guild, matching the app's existing convention (join/leave banners, icons).
func withGuild(base, guild string) string {
	clause := " in #" + guild
	if strings.HasSuffix(base, ".") {
		return base[:len(base)-1] + clause + "."
	}
	return base + clause
}

// baseNotifSummary returns the action text for a notification without any guild clause.
func baseNotifSummary(n model.Notification) string {
	switch n.Type {
	case "new_post_friend", "new_post_following":
		return "published something."
	case "bookmark":
		return "saved your entry."
	case "new_follower":
		return "started following you."
	case "unfollowed":
		return "unfollowed you."
	case "reply":
		return "replied to your post."
	case "reply_mention":
		return "mentioned you in a reply."
	case "post_mention":
		return "mentioned you in a post."
	case "chat_mention":
		return "mentioned you in chat."
	case "dm_message":
		return "sent you a message."
	case "thread_reply":
		if n.ThreadAuthorUsername != "" {
			return "replied in @" + n.ThreadAuthorUsername + "'s thread."
		}
		return "replied in a thread you're following."
	case "guild_new_thread":
		if n.GuildName != "" {
			return "posted a new thread in #" + n.GuildName + "."
		}
		return "posted a new thread."
	case "poke":
		return `poked you ¯\_(ツ)_/¯`
	case "supporter_granted":
		return "granted you Supporter status."
	case "supporter_removed":
		return "removed your Supporter status."
	case "hacker_granted":
		return "granted you Hacker status."
	case "hacker_removed":
		return "removed your Hacker status."
	case "image_permission_granted":
		return "granted you image permissions."
	case "image_permission_removed":
		return "removed your image permissions."
	case "attachment_permission_granted":
		return "granted you attachment permissions."
	case "attachment_permission_removed":
		return "removed your attachment permissions."
	case "system_ban":
		return "your account has been banned."
	default:
		return n.Type
	}
}

// notifIcon returns a styled type-specific symbol for the notification.
// Unread notifications are rendered in highlight colour; read ones in subtle.
func notifIcon(n model.Notification) string {
	var sym string
	switch n.Type {
	case "reply", "thread_reply":
		sym = "↩"
	case "reply_mention", "post_mention":
		sym = "@"
	case "new_post_friend", "new_post_following":
		sym = "★"
	case "bookmark":
		sym = "♥"
	case "new_follower":
		sym = "+"
	case "unfollowed":
		sym = "☹"
	case "guild_new_thread":
		sym = "#"
	case "poke":
		sym = "~"
	case "chat_mention":
		sym = "»"
	case "dm_message":
		sym = "✉"
	case "supporter_granted", "supporter_removed":
		sym = "$"
	case "hacker_granted", "hacker_removed":
		sym = "^"
	case "image_permission_granted", "image_permission_removed",
		"attachment_permission_granted", "attachment_permission_removed":
		sym = "%"
	case "system_ban":
		sym = "☠"
	default:
		sym = "·"
	}
	if !n.Read {
		return theme.Highlight.Render(sym) + " "
	}
	return theme.Subtle.Render(sym) + " "
}

func (m NotificationsModel) renderNotif(n model.Notification, selected bool) string {
	innerWidth := m.width - 4

	dot := notifIcon(n)
	actor := theme.Highlight.Render("@" + n.Actor.Username)
	summary := theme.Base.Render(" " + notifSummary(n))
	ts := theme.Subtle.Render(formatRelativeTime(n.CreatedAt, time.Now(), m.location()))

	left := lipgloss.JoinHorizontal(lipgloss.Top, dot, actor, summary)
	var line string
	if innerWidth > 0 {
		gap := innerWidth - lipgloss.Width(left) - lipgloss.Width(ts)
		if gap > 0 {
			line = left + strings.Repeat(" ", gap) + ts
		} else {
			line = left
		}
	} else {
		line = left
	}

	boxStyle := theme.Border
	if selected {
		boxStyle = theme.ActiveBorder
	}
	if innerWidth > 0 {
		boxStyle = boxStyle.Width(m.width - 2)
	}
	return boxStyle.Render(line)
}

func (m NotificationsModel) View() string {
	if !m.ready {
		return theme.Subtle.Render("loading notifications...")
	}
	return m.viewport.View()
}
