package screens

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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
// PostSlug and AuthorUsername come from v0.7 notification metadata and are threaded
// through for future use (slug-based navigation, pre-fetched display).
type ShowNotificationPostMsg struct {
	PostID         string
	NotifID        string
	ReplyID        string
	PostSlug       string
	AuthorUsername string
}

// notifFilterDebounceDelay is how long the filter panel cursor must sit still
// on a row before it's actually applied (fetched). Without this, quickly
// scrolling through categories fires a fetch per keypress, which can arrive
// out of order and briefly flash a stale category's results.
const notifFilterDebounceDelay = 1 * time.Second

// notifFilterDebounceMsg fires notifFilterDebounceDelay after a filter panel
// cursor move. Update only applies it if the cursor hasn't moved again since
// (checked via filterMoveAt) — a newer move's own scheduled tick will apply
// instead, so at most one fetch happens per pause.
type notifFilterDebounceMsg struct{}

func scheduleNotifFilterDebounceCmd() tea.Cmd {
	return tea.Tick(notifFilterDebounceDelay, func(time.Time) tea.Msg {
		return notifFilterDebounceMsg{}
	})
}

// notifCategory groups related notification types under one filterable label.
// Kept as an ordered slice (not a map) so the filter panel and header summary
// render in a stable, deterministic order.
type notifCategory struct {
	label string
	types []string
}

var notifCategories = []notifCategory{
	{label: "mentions", types: []string{"reply_mention", "post_mention", "chat_mention", "graffiti_mention"}},
	{label: "social", types: []string{"new_follower", "unfollowed", "poke", "bookmark"}},
	{label: "threads", types: []string{"reply", "thread_reply", "guild_new_thread", "new_post_friend", "new_post_following"}},
	{label: "c-mail", types: []string{"dm_message"}},
	{label: "account/system", types: []string{
		"supporter_granted", "supporter_removed", "hacker_granted", "hacker_removed",
		"image_permission_granted", "image_permission_removed",
		"attachment_permission_granted", "attachment_permission_removed",
		"system_ban", "system_ban_lifted", "moderator_granted", "moderator_removed",
		"api_access_granted", "api_access_removed", "post_cooldown", "rate_limit_warning",
	}},
}

// notifFilterOptionCount returns the number of rows in the filter panel: the
// synthetic "all" option (index 0) plus one per category.
func notifFilterOptionCount() int { return len(notifCategories) + 1 }

// notifFilterOptionLabel returns the display label for filter panel row i.
func notifFilterOptionLabel(i int) string {
	if i == 0 {
		return "all"
	}
	return notifCategories[i-1].label
}

type NotificationsModel struct {
	notifs              []model.Notification
	notifOffsets        []int // start line of each notification within the viewport content
	viewport            viewport.Model
	width               int
	height              int
	selectedIndex       int
	ready               bool
	loading             bool
	fetching            bool // true while the initial (or tab-switch) load is in flight
	refreshing          bool
	exhausted           bool
	nextCursor          string
	hasPaginated        bool
	showUnreadOnly      bool
	err                 error
	relaxed             bool
	loc                 *time.Location
	filterCategoryIndex int // 0 = all (no filter); 1..len(notifCategories) map to notifCategories[i-1]; the last APPLIED (fetched) selection
	filterOpen          bool
	filterCursor        int       // row currently highlighted in the panel; may lag filterCategoryIndex while debouncing
	filterOrigIndex     int       // filterCategoryIndex snapshot taken when the panel opened, for esc revert
	filterMoveAt        time.Time // time of the most recent cursor move, for debouncing the fetch
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
	m.hasPaginated = false
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
	m.hasPaginated = true
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

// ActiveTypeFilter returns the []string the API expects for GetNotifications'
// types param for the currently selected category. Returns nil when "all" is
// selected, matching the "no filter" query shape/behavior the client used
// before this feature existed. notifCategories is never mutated, so the
// underlying slice is returned directly rather than copied.
func (m NotificationsModel) ActiveTypeFilter() []string {
	if m.filterCategoryIndex == 0 {
		return nil
	}
	return notifCategories[m.filterCategoryIndex-1].types
}

// FilterSummary returns the active category's label, or "" when "all" is
// selected (no filter applied).
func (m NotificationsModel) FilterSummary() string {
	if m.filterCategoryIndex == 0 {
		return ""
	}
	return notifCategories[m.filterCategoryIndex-1].label
}

// HasPaginated reports whether the user has loaded pages beyond the first.
func (m NotificationsModel) HasPaginated() bool { return m.hasPaginated }

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

	case notifFilterDebounceMsg:
		if !m.filterOpen || time.Since(m.filterMoveAt) < notifFilterDebounceDelay {
			return m, nil // panel closed, or a newer move already reset the timer
		}
		if m.filterCursor == m.filterCategoryIndex {
			return m, nil // cursor settled back on the already-applied category
		}
		return m.applyCategoryFilter(m.filterCursor)

	case tea.KeyMsg:
		if m.filterOpen {
			return m.handleFilterPanelKey(msg)
		}
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
		case "pgup":
			if m.selectedIndex > 0 {
				m.selectedIndex = max(0, m.selectedIndex-pageJumpItems)
				m = m.refreshContent()
				m = m.ensureSelectedVisible()
			} else if !m.loading && !m.refreshing {
				m.refreshing = true
				m = m.refreshContent()
				return m, func() tea.Msg { return RefreshNotifsMsg{} }
			}
			return m, nil
		case "pgdown":
			if m.selectedIndex < len(visible)-1 {
				m.selectedIndex = min(len(visible)-1, m.selectedIndex+pageJumpItems)
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
		case "f":
			m.filterOpen = true
			m.filterOrigIndex = m.filterCategoryIndex
			m.filterCursor = m.filterCategoryIndex
			return m, nil
		case "enter":
			if len(visible) == 0 || m.selectedIndex >= len(visible) {
				return m, nil
			}
			n := visible[m.selectedIndex]
			switch n.Type {
			case "chat_mention":
				// Jump straight to the cIRC room the mention happened in.
				m = m.MarkRead(n.ID)
				notifID, slug := n.ID, n.RoomSlug
				return m, func() tea.Msg { return OpenRoomMsg{RoomSlug: slug, NotifID: notifID} }
			case "dm_message":
				// Open (or start) the C-Mail conversation with the sender — same
				// flow as the 'c' key elsewhere.
				m = m.MarkRead(n.ID)
				notifID, username := n.ID, n.Actor.Username
				return m, tea.Batch(
					func() tea.Msg { return MarkNotifReadMsg{ID: notifID} },
					func() tea.Msg { return StartConversationMsg{Username: username} },
				)
			case "poke", "new_follower", "unfollowed":
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
			slug, author := n.PostSlug, n.PostAuthorUsername
			return m, func() tea.Msg {
				return ShowNotificationPostMsg{PostID: postID, NotifID: notifID, ReplyID: replyID, PostSlug: slug, AuthorUsername: author}
			}
		case "p":
			if len(visible) > 0 && m.selectedIndex < len(visible) && hasActor(visible[m.selectedIndex]) {
				username := visible[m.selectedIndex].Actor.Username
				return m, func() tea.Msg { return ShowUserProfileMsg{Username: username} }
			}
			return m, nil
		case "c":
			if len(visible) > 0 && m.selectedIndex < len(visible) && hasActor(visible[m.selectedIndex]) {
				username := visible[m.selectedIndex].Actor.Username
				return m, func() tea.Msg { return StartConversationMsg{Username: username} }
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

// handleFilterPanelKey handles key input while the category filter panel is
// open. Selection is live: every cursor move immediately applies and
// refetches (mirrors the theme picker's live-preview-on-cursor-move
// pattern in internal/ui/app.go's handleThemePickerKey). Esc reverts to
// whichever category was active when the panel opened; enter just closes,
// since the current cursor position is already applied.
func (m NotificationsModel) handleFilterPanelKey(msg tea.KeyMsg) (NotificationsModel, tea.Cmd) {
	n := notifFilterOptionCount()
	switch msg.String() {
	case "up", "k":
		m.filterCursor = (m.filterCursor - 1 + n) % n
		m.filterMoveAt = time.Now()
		return m, scheduleNotifFilterDebounceCmd()
	case "down", "j":
		m.filterCursor = (m.filterCursor + 1) % n
		m.filterMoveAt = time.Now()
		return m, scheduleNotifFilterDebounceCmd()
	case "esc":
		m.filterOpen = false
		if m.filterCategoryIndex != m.filterOrigIndex {
			return m.applyCategoryFilter(m.filterOrigIndex)
		}
	case "enter":
		m.filterOpen = false
		if m.filterCursor != m.filterCategoryIndex {
			// Apply the highlighted-but-not-yet-debounced selection
			// immediately rather than making the user wait it out.
			return m.applyCategoryFilter(m.filterCursor)
		}
	}
	return m, nil
}

// applyCategoryFilter sets the active filter category, resets pagination
// state, and refetches — the same reset shape as the existing 'u' unread-only
// toggle. Shared by cursor movement and the esc-revert path.
func (m NotificationsModel) applyCategoryFilter(index int) (NotificationsModel, tea.Cmd) {
	m.filterCategoryIndex = index
	m.selectedIndex = 0
	m.notifs = nil
	m.nextCursor = ""
	m.exhausted = false
	m.fetching = true
	m = m.refreshContent()
	return m, func() tea.Msg { return RefreshNotifsMsg{} }
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
	if summary := m.FilterSummary(); summary != "" {
		prefix += theme.Subtle.Render("  filter: "+summary+"  (press 'f' to change)") + "\n"
		startLine++
	}
	if m.refreshing {
		prefix += theme.Subtle.Render("  fetching notifications...") + "\n"
		startLine++
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
		if g := guildLabel(n); g != "" {
			return withGuild(base, g)
		}
	}
	return base
}

// hasActor reports whether a notification has a real, navigable actor.
// Several v0.8.5 system-only types arrive with no sender: the actor fields
// are either omitted (empty string) or carry the literal "system".
func hasActor(n model.Notification) bool {
	return n.Actor.Username != "" && n.Actor.Username != "system"
}

// guildLabel returns the guild handle to display, preferring the slug — the stable
// lowercase handle the server sends for guild replies/posts (metadata.guildSlug) —
// over the rarer display name. Empty when the notification has no guild context.
func guildLabel(n model.Notification) string {
	if n.GuildSlug != "" {
		return n.GuildSlug
	}
	return n.GuildName
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
		if n.RoomName != "" {
			return "mentioned you in #" + n.RoomName + "."
		}
		if n.RoomSlug != "" {
			return "mentioned you in #" + n.RoomSlug + "."
		}
		return "mentioned you in chat."
	case "dm_message":
		return "sent you a message."
	case "thread_reply":
		if n.ThreadAuthorUsername != "" {
			return "replied in @" + n.ThreadAuthorUsername + "'s thread."
		}
		return "replied in a thread you're following."
	case "guild_new_thread":
		if g := guildLabel(n); g != "" {
			return "posted a new thread in #" + g + "."
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
	case "graffiti_mention":
		return "mentioned you in a graffiti wall post."
	case "moderator_granted":
		return "granted you Moderator status."
	case "moderator_removed":
		return "removed your Moderator status."
	case "api_access_granted":
		return "granted you API access."
	case "api_access_removed":
		return "revoked your API access."
	case "system_ban_lifted":
		return "your ban has been lifted."
	case "post_cooldown":
		return "a post was rate-limited and saved as a note instead."
	case "rate_limit_warning":
		return "you're approaching a posting limit."
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
	case "graffiti_mention":
		sym = "@"
	case "moderator_granted", "moderator_removed":
		sym = "!"
	case "api_access_granted", "api_access_removed":
		sym = "/"
	case "system_ban_lifted":
		sym = "✓"
	case "post_cooldown":
		sym = "⏱"
	case "rate_limit_warning":
		sym = "⚠"
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
	ts := theme.Subtle.Render(formatRelativeTime(n.CreatedAt, time.Now(), m.location()))

	var left string
	if hasActor(n) {
		actor := theme.Highlight.Render("@" + n.Actor.Username)
		summary := theme.Base.Render(" " + notifSummary(n))
		left = lipgloss.JoinHorizontal(lipgloss.Top, dot, actor, summary)
	} else {
		// System-only notifications (v0.8.5+) carry no actor — render the
		// summary alone rather than an empty or literal "@system" handle.
		summary := theme.Base.Render(notifSummary(n))
		left = lipgloss.JoinHorizontal(lipgloss.Top, dot, summary)
	}
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

	// For mention types, show an inline content preview so the user can read
	// what mentioned them without navigating away — see notifContent for which
	// field carries the body per type.
	if mentionContent := notifContent(n); mentionContent != "" && innerWidth > 4 {
		flat := strings.ReplaceAll(mentionContent, "\n", " ")
		maxLen := innerWidth - 2 // room for "> " prefix
		r := []rune(flat)
		if len(r) > maxLen {
			flat = string(r[:maxLen-1]) + "…"
		}
		line += "\n" + theme.Subtle.Render("> "+flat)
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

// notifContent returns the notification's body text for a preview: the first
// non-empty of PostContent (post_mention), ReplyContent (reply_mention),
// MessageContent (chat_mention), or Reason (actorless system notifications).
func notifContent(n model.Notification) string {
	for _, s := range []string{n.PostContent, n.ReplyContent, n.MessageContent, n.Reason} {
		if s != "" {
			return s
		}
	}
	return ""
}

// notifPreview returns notifContent flattened to a single line and truncated to
// max runes (ellipsis added). Empty when the notification carries no body text.
func notifPreview(n model.Notification, max int) string {
	c := notifContent(n)
	if c == "" {
		return ""
	}
	flat := strings.ReplaceAll(c, "\n", " ")
	if r := []rune(flat); max > 0 && len(r) > max {
		flat = string(r[:max-1]) + "…"
	}
	return flat
}

// NotifToastText is the one-line desktop-notification text for n, mirroring a
// Notifications-tab row minus the icon and timestamp: "@actor <summary>" (or
// just "<summary>" for actorless system notifications), plus " — <preview>"
// when the notification carries body text.
func NotifToastText(n model.Notification) string {
	s := notifSummary(n)
	if hasActor(n) {
		s = "@" + n.Actor.Username + " " + s
	}
	if p := notifPreview(n, 60); p != "" {
		s += " — " + p
	}
	return s
}

func (m NotificationsModel) View() string {
	if !m.ready {
		return theme.Subtle.Render("loading notifications...")
	}
	if m.filterOpen {
		return overlayCenter(m.viewport.View(), m.renderFilterPanel(), m.width, m.viewportHeight())
	}
	return m.viewport.View()
}

func (m NotificationsModel) renderFilterPanel() string {
	title := theme.Title.Render("Filter Notifications")
	var lines []string
	for i := 0; i < notifFilterOptionCount(); i++ {
		line := notifFilterOptionLabel(i)
		if i == m.filterCursor {
			line = theme.Highlight.Render("▸ " + line)
		} else {
			line = theme.Subtle.Render("  " + line)
		}
		lines = append(lines, line)
	}
	hint := theme.Subtle.Render("↑↓ select   enter confirm   esc cancel")
	body := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		lipgloss.JoinVertical(lipgloss.Left, lines...),
		"",
		hint,
	)
	return theme.ActiveBorder.Render(body)
}

// overlayCenter splices fg, centered, on top of bg — used to keep the
// notification list visible behind the filter panel rather than blanking it,
// matching how the app's other modals (icon picker, theme picker, etc.)
// overlay on top of the screen behind them instead of replacing it.
func overlayCenter(bg, fg string, bgW, bgH int) string {
	fgW := lipgloss.Width(fg)
	fgLines := strings.Split(fg, "\n")
	bgLines := strings.Split(bg, "\n")

	xOff := (bgW - fgW) / 2
	yOff := (bgH - len(fgLines)) / 2
	if xOff < 0 {
		xOff = 0
	}
	if yOff < 0 {
		yOff = 0
	}

	result := make([]string, len(bgLines))
	copy(result, bgLines)

	for i, fgLine := range fgLines {
		bi := yOff + i
		if bi < 0 || bi >= len(result) {
			continue
		}
		bgLine := result[bi]
		bgLineW := ansi.StringWidth(bgLine)
		needed := xOff + fgW
		if bgLineW < needed {
			bgLine += strings.Repeat(" ", needed-bgLineW)
		}
		left := ansi.Truncate(bgLine, xOff, "")
		right := ansi.TruncateLeft(bgLine, xOff+fgW, "")
		result[bi] = left + fgLine + right
	}
	return strings.Join(result, "\n")
}
