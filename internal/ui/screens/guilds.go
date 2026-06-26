package screens

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/markdown"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
)

// Message types emitted by Guilds screen to App.
type RefreshGuildsMsg struct{}

type LoadMoreGuildsMsg struct{ Cursor string }

type LoadGuildPostsMsg struct{ Slug string }

type LoadMoreGuildPostsMsg struct {
	Slug   string
	Cursor string
}

type RefreshGuildPostsMsg struct{ Slug string }

type ShowGuildPostMsg struct{ Post model.Post }

// SubmitGuildPostMsg is emitted when the user submits a new thread from the guild posts view.
type SubmitGuildPostMsg struct {
	Slug    string
	Content string
	Title   string
	Topics  []string
}

// LoadGuildMembersMsg is emitted when the user requests the member list for a guild.
type LoadGuildMembersMsg struct{ Slug string }

// LoadMoreGuildMembersMsg is emitted when the user scrolls to the end of the member list.
type LoadMoreGuildMembersMsg struct {
	Slug   string
	Cursor string
}

// JoinGuildMsg is emitted when the user confirms joining the active guild.
type JoinGuildMsg struct{ Slug string }

// LeaveGuildMsg is emitted when the user confirms leaving the active guild.
type LeaveGuildMsg struct{ Slug string }

// LoadGuildThreadMsg is emitted when the selected guild post changes so the app can
// fetch replies for the Miller reading pane.
type LoadGuildThreadMsg struct{ PostID string }

// GuildThreadRepliesMsg delivers fetched replies back to GuildsModel for the reading pane.
type GuildThreadRepliesMsg struct {
	PostID  string
	Replies []model.Reply
}

// GuildThreadNavMsg is emitted by the Miller layout when j/k is pressed in focusDetail
// while the guilds screen is active. PaneHeight and PaneWidth enable pager-style scrolling.
type GuildThreadNavMsg struct {
	Delta      int
	PaneHeight int
	PaneWidth  int
}

// GuildThreadDebounceMsg is the delayed message emitted after guildThreadDebounceDelay
// when the selected post changes. The fetch only proceeds if PostID still matches
// the current selection, dropping stale ticks from rapid navigation.
type GuildThreadDebounceMsg struct{ PostID string }

const guildThreadDebounceDelay = time.Second

// guildsConfirm tracks whether a join/leave confirmation prompt is active.
type guildsConfirm int

const (
	confirmNoneG guildsConfirm = iota
	confirmJoinG
	confirmLeaveG
)

// Internal view state for the Guilds screen.
type guildsView int

const (
	viewGuildList guildsView = iota
	viewGuildPosts
	viewGuildMembers
)

// GuildsModel is the Bubble Tea model for the guilds browser.
type GuildsModel struct {
	view guildsView

	// Guild list state
	guilds           []model.Guild
	guildIndex       int
	guildsNextCursor string
	guildsExhausted  bool

	// Guild posts state
	activeGuild       string
	activeGuildDetail model.Guild
	guildDetailLoaded bool
	confirming        guildsConfirm
	ownGuildSlug      string // logged-in user's current guild membership (empty = no guild)
	posts             []model.Post
	postIndex         int
	nextCursor        string
	exhausted         bool
	loading           bool
	fetching          bool
	refreshing        bool
	loaded            bool

	// Guild members state
	members           []model.GuildMember
	memberIndex       int
	membersNextCursor string
	membersExhausted  bool

	// Compose panel for new guild threads (visible in posts view).
	panel PostComposePanel

	// Miller reading pane: replies for the currently selected guild post.
	threadPostID       string
	maxThreadDepth     int
	threadReplies      []model.Reply
	threadFlatTree     []replyNode
	threadReplyIndex   int
	threadScrollOffset int
	threadLoading      bool

	// Shared
	viewport          viewport.Model
	itemOffsets       []int
	width             int
	bookmarkedPostIDs map[string]struct{}
	watchedPostIDs    map[string]struct{}
	height            int
	ready             bool
	err               error
	loc               *time.Location
	relaxed           bool
	timeDisplayFormat string
	filterNSFW        bool
}

// NewGuildsModel returns a zero-value GuildsModel ready for first use.
func NewGuildsModel() GuildsModel {
	return GuildsModel{
		panel:            NewPostComposePanel(0),
		threadReplyIndex: -1,
	}
}

func (m GuildsModel) visiblePosts() []model.Post {
	if !m.filterNSFW {
		return m.posts
	}
	out := m.posts[:0:0]
	for _, p := range m.posts {
		if !p.IsNSFW {
			out = append(out, p)
		}
	}
	return out
}

// ComposeActive reports whether the new-thread compose panel is open.
func (m GuildsModel) ComposeActive() bool { return m.panel.IsActive() }

// ActiveGuild returns the slug of the guild whose posts are currently displayed, or "" when in list view.
func (m GuildsModel) ActiveGuild() string { return m.activeGuild }

// IsLoaded reports whether the guild list has been fetched at least once.
func (m GuildsModel) IsLoaded() bool { return m.loaded }

// SetFetching marks the model as loading and refreshes the loading indicator.
func (m GuildsModel) SetFetching() GuildsModel {
	m.fetching = true
	m.err = nil
	if m.ready {
		m = m.refreshContent()
	}
	return m
}

// SetGuilds replaces the guild list with a fresh page.
func (m GuildsModel) SetGuilds(items []model.Guild, cursor string) GuildsModel {
	m.err = nil
	m.guilds = items
	m.guildIndex = 0
	m.guildsNextCursor = cursor
	m.guildsExhausted = cursor == ""
	m.loading = false
	m.fetching = false
	m.refreshing = false
	m.loaded = true
	if m.ready {
		m = m.refreshContent()
		m.viewport.GotoTop()
	}
	return m
}

// AppendGuilds adds a pagination page to the guild list.
func (m GuildsModel) AppendGuilds(items []model.Guild, cursor string) GuildsModel {
	m.err = nil
	m.guilds = append(m.guilds, items...)
	m.guildsNextCursor = cursor
	m.guildsExhausted = cursor == ""
	m.loading = false
	m.fetching = false
	if m.ready {
		m = m.refreshContent()
	}
	return m
}

// SetGuildPosts replaces the post list for a guild and switches to posts view.
func (m GuildsModel) SetGuildPosts(posts []model.Post, cursor string) GuildsModel {
	m.err = nil
	m.posts = posts
	m.postIndex = 0
	m.nextCursor = cursor
	m.exhausted = cursor == ""
	m.loading = false
	m.fetching = false
	m.refreshing = false
	if !m.panel.IsActive() {
		m.panel = m.panel.Close()
	}
	m.view = viewGuildPosts
	if m.ready {
		m = m.refreshContent()
		m.viewport.GotoTop()
	}
	return m
}

// AppendGuildPosts adds a pagination page to the guild post list.
func (m GuildsModel) AppendGuildPosts(posts []model.Post, cursor string) GuildsModel {
	m.err = nil
	m.posts = append(m.posts, posts...)
	m.nextCursor = cursor
	m.exhausted = cursor == ""
	m.loading = false
	m.fetching = false
	if m.ready {
		m = m.refreshContent()
	}
	return m
}

// SetGuildMembers replaces the member list and switches to the members view.
func (m GuildsModel) SetGuildMembers(members []model.GuildMember, cursor string) GuildsModel {
	m.err = nil
	m.members = members
	m.memberIndex = 0
	m.membersNextCursor = cursor
	m.membersExhausted = cursor == ""
	m.loading = false
	m.fetching = false
	m.view = viewGuildMembers
	if m.ready {
		m = m.refreshContent()
		m.viewport.GotoTop()
	}
	return m
}

// AppendGuildMembers adds a pagination page to the member list.
func (m GuildsModel) AppendGuildMembers(members []model.GuildMember, cursor string) GuildsModel {
	m.err = nil
	m.members = append(m.members, members...)
	m.membersNextCursor = cursor
	m.membersExhausted = cursor == ""
	m.loading = false
	m.fetching = false
	if m.ready {
		m = m.refreshContent()
	}
	return m
}

// SetGuildDetail stores the guild detail (including IsMember and Role) fetched from
// the single-guild endpoint and marks the detail as loaded.
func (m GuildsModel) SetGuildDetail(g model.Guild) GuildsModel {
	m.activeGuildDetail = g
	m.guildDetailLoaded = true
	if m.ready {
		m = m.refreshContent()
	}
	return m
}

// GuildDetail returns the most recently fetched guild detail.
func (m GuildsModel) GuildDetail() model.Guild { return m.activeGuildDetail }

// IsDetailLoaded reports whether guild detail has been fetched for the active guild.
func (m GuildsModel) IsDetailLoaded() bool { return m.guildDetailLoaded }

// SetOwnGuildSlug updates the logged-in user's guild membership slug. Called by
// app.go after a successful join or leave so the J key guard stays current without
// requiring a full SharedConfigMsg broadcast.
func (m GuildsModel) SetOwnGuildSlug(slug string) GuildsModel {
	m.ownGuildSlug = slug
	return m
}

// IsConfirmingJoin reports whether the join confirmation prompt is active.
func (m GuildsModel) IsConfirmingJoin() bool { return m.confirming == confirmJoinG }

// IsConfirmingLeave reports whether the leave confirmation prompt is active.
func (m GuildsModel) IsConfirmingLeave() bool { return m.confirming == confirmLeaveG }

// SetError stores an error and clears the loading state.
func (m GuildsModel) SetError(err error) GuildsModel {
	m.err = err
	m.loading = false
	m.fetching = false
	m.refreshing = false
	if m.ready {
		m = m.refreshContent()
	}
	return m
}

// Init satisfies tea.Model.
func (m GuildsModel) Init() tea.Cmd { return nil }

// Update handles messages for the guilds screen.
func (m GuildsModel) Update(msg tea.Msg) (GuildsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case SharedConfigMsg:
		m.relaxed = msg.Relaxed
		if msg.Loc != nil {
			m.loc = msg.Loc
		}
		m.timeDisplayFormat = msg.Settings.TimeDisplayFormat
		if msg.Settings.FilterNSFW != m.filterNSFW {
			m.filterNSFW = msg.Settings.FilterNSFW
			m.postIndex = 0
		}
		m.ownGuildSlug = msg.OwnGuildSlug
		if m.ready {
			m = m.refreshContent()
		}
		if msg.MaxThreadDepth != m.maxThreadDepth {
			m.maxThreadDepth = msg.MaxThreadDepth
			if len(m.threadReplies) > 0 {
				m.threadFlatTree = buildReplyTree(m.threadReplies, m.effectiveMaxDepth())
			}
		}
		return m, nil

	case BookmarkedIDsMsg:
		m.bookmarkedPostIDs = msg.PostIDs
		if m.ready {
			m = m.refreshContent()
		}
		return m, nil

	case WatchedPostIDsMsg:
		m.watchedPostIDs = msg.PostIDs
		if m.ready {
			m = m.refreshContent()
		}
		return m, nil

	case GuildThreadRepliesMsg:
		visible := m.visiblePosts()
		if m.postIndex < len(visible) && visible[m.postIndex].ID == msg.PostID {
			m.threadPostID = msg.PostID
			m.threadReplies = msg.Replies
			m.threadFlatTree = buildReplyTree(msg.Replies, m.effectiveMaxDepth())
			m.threadReplyIndex = -1
			m.threadScrollOffset = 0
			m.threadLoading = false
		}
		return m, nil

	case GuildThreadDebounceMsg:
		visible := m.visiblePosts()
		if m.postIndex < len(visible) && visible[m.postIndex].ID == msg.PostID {
			m.threadLoading = true
			return m, func() tea.Msg { return LoadGuildThreadMsg(msg) }
		}
		return m, nil

	case GuildThreadNavMsg:
		if msg.PaneHeight > 0 && msg.PaneWidth > 0 {
			m = m.pageThreadNav(msg.Delta, msg.PaneHeight, msg.PaneWidth)
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.panel = m.panel.SetWidth(msg.Width)
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

	case ComposeSubmitMsg:
		if !m.panel.IsActive() {
			return m, nil
		}
		title := m.panel.TitleValue()
		topics := ParseTopics(m.panel.TopicsRaw())
		content := msg.Content
		slug := m.activeGuild
		m.panel = m.panel.Close()
		m = m.refreshContent()
		return m, func() tea.Msg {
			return SubmitGuildPostMsg{Slug: slug, Content: content, Title: title, Topics: topics}
		}

	case ComposeCancelMsg:
		m.panel = m.panel.Close()
		m = m.refreshContent()
		return m, nil

	case tea.KeyMsg:
		// Route to panel when compose is open.
		if m.panel.IsActive() {
			var cmd tea.Cmd
			m.panel, cmd = m.panel.Update(msg)
			return m, cmd
		}
		// Route to confirm handler when a join/leave prompt is active.
		if m.confirming != confirmNoneG {
			return m.handleConfirmKey(msg)
		}
		switch msg.String() {
		case "up", "k":
			if m.view == viewGuildList {
				if m.guildIndex > 0 {
					m.guildIndex--
					m = m.refreshContent()
				}
			} else if m.view == viewGuildPosts {
				if m.postIndex > 0 {
					m.postIndex--
					m = m.refreshContent()
					var detailCmd tea.Cmd
					m, detailCmd = m.currentDetailCmd()
					return m, detailCmd
				} else if !m.loading && !m.refreshing {
					slug := m.activeGuild
					m.refreshing = true
					m = m.refreshContent()
					return m, func() tea.Msg { return RefreshGuildPostsMsg{Slug: slug} }
				}
			} else { // viewGuildMembers
				if m.memberIndex > 0 {
					m.memberIndex--
					m = m.refreshContent()
				}
			}
			return m, nil

		case "down", "j":
			if m.view == viewGuildList {
				if m.guildIndex < len(m.guilds)-1 {
					m.guildIndex++
					m = m.refreshContent()
				} else if !m.guildsExhausted && !m.loading {
					cursor := m.guildsNextCursor
					m.loading = true
					m = m.refreshContent()
					m.viewport.ScrollDown(1)
					return m, func() tea.Msg {
						return LoadMoreGuildsMsg{Cursor: cursor}
					}
				}
			} else if m.view == viewGuildPosts {
				if m.postIndex < len(m.visiblePosts())-1 {
					m.postIndex++
					m = m.refreshContent()
					var detailCmd tea.Cmd
					m, detailCmd = m.currentDetailCmd()
					return m, detailCmd
				} else if !m.exhausted && !m.loading {
					slug, cursor := m.activeGuild, m.nextCursor
					m.loading = true
					m = m.refreshContent()
					m.viewport.ScrollDown(1)
					return m, func() tea.Msg {
						return LoadMoreGuildPostsMsg{Slug: slug, Cursor: cursor}
					}
				}
			} else { // viewGuildMembers
				if m.memberIndex < len(m.members)-1 {
					m.memberIndex++
					m = m.refreshContent()
				} else if !m.membersExhausted && !m.loading {
					slug, cursor := m.activeGuild, m.membersNextCursor
					m.loading = true
					m = m.refreshContent()
					m.viewport.ScrollDown(1)
					return m, func() tea.Msg {
						return LoadMoreGuildMembersMsg{Slug: slug, Cursor: cursor}
					}
				}
			}
			return m, nil

		case "enter":
			if m.view == viewGuildList {
				if len(m.guilds) > 0 && m.guildIndex < len(m.guilds) {
					slug := m.guilds[m.guildIndex].Slug
					m.activeGuild = slug
					return m, func() tea.Msg { return LoadGuildPostsMsg{Slug: slug} }
				}
			} else if m.view == viewGuildPosts {
				if visible := m.visiblePosts(); len(visible) > 0 && m.postIndex < len(visible) {
					post := visible[m.postIndex]
					return m, func() tea.Msg { return ShowGuildPostMsg{Post: post} }
				}
			} else { // viewGuildMembers
				if len(m.members) > 0 && m.memberIndex < len(m.members) {
					username := m.members[m.memberIndex].Username
					return m, func() tea.Msg { return ShowUserProfileMsg{Username: username} }
				}
			}
			return m, nil

		case "m":
			if m.view == viewGuildPosts {
				slug := m.activeGuild
				m.loading = true
				m = m.refreshContent()
				return m, func() tea.Msg { return LoadGuildMembersMsg{Slug: slug} }
			}
			return m, nil

		case "J":
			if m.view == viewGuildPosts && m.guildDetailLoaded && !m.activeGuildDetail.IsMember && m.ownGuildSlug == "" {
				m.confirming = confirmJoinG
				m = m.refreshContent()
			}
			return m, nil

		case "L":
			if m.view == viewGuildPosts && m.guildDetailLoaded && m.activeGuildDetail.IsMember && m.activeGuildDetail.Role != "founder" {
				m.confirming = confirmLeaveG
				m = m.refreshContent()
			}
			return m, nil

		case "n":
			if m.view == viewGuildPosts {
				var cmd tea.Cmd
				m.panel, cmd = m.panel.Open(false)
				m = m.refreshContent()
				return m, cmd
			}
			return m, nil

		case "esc":
			if m.view == viewGuildMembers {
				m.view = viewGuildPosts
				m.loading = false
				m.fetching = false
				m = m.refreshContent()
				return m, nil
			}
			if m.view == viewGuildPosts {
				m.view = viewGuildList
				m.activeGuild = ""
				m.activeGuildDetail = model.Guild{}
				m.guildDetailLoaded = false
				m.confirming = confirmNoneG
				m.loading = false
				m.fetching = false
				m.panel = m.panel.Close()
				m = m.refreshContent()
				m.viewport.GotoTop()
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)

	if m.view == viewGuildPosts && m.viewport.AtBottom() && !m.exhausted && !m.loading {
		slug, cursor := m.activeGuild, m.nextCursor
		m.loading = true
		m = m.refreshContent()
		m.viewport.ScrollDown(1)
		return m, func() tea.Msg {
			return LoadMoreGuildPostsMsg{Slug: slug, Cursor: cursor}
		}
	}

	if m.view == viewGuildMembers && m.viewport.AtBottom() && !m.membersExhausted && !m.loading {
		slug, cursor := m.activeGuild, m.membersNextCursor
		m.loading = true
		m = m.refreshContent()
		m.viewport.ScrollDown(1)
		return m, func() tea.Msg {
			return LoadMoreGuildMembersMsg{Slug: slug, Cursor: cursor}
		}
	}

	return m, cmd
}

// View renders the guilds screen.
func (m GuildsModel) View() string {
	if !m.ready {
		return theme.Subtle.Render("loading guilds...")
	}
	if m.panel.IsActive() {
		return lipgloss.JoinVertical(lipgloss.Left,
			m.viewport.View(),
			m.panel.View(),
		)
	}
	if m.confirming != confirmNoneG {
		return lipgloss.JoinVertical(lipgloss.Left,
			m.viewport.View(),
			m.renderConfirmBar(),
		)
	}
	return m.viewport.View()
}

// BackToGuildList resets state and returns to the guild list view. Used by app.go
// after a successful leave so the screen navigates back without a keypress.
func (m GuildsModel) BackToGuildList() GuildsModel {
	m.view = viewGuildList
	m.activeGuild = ""
	m.activeGuildDetail = model.Guild{}
	m.guildDetailLoaded = false
	m.confirming = confirmNoneG
	m.loading = false
	m.fetching = false
	m.panel = m.panel.Close()
	if m.ready {
		m = m.refreshContent()
		m.viewport.GotoTop()
	}
	return m
}

func (m GuildsModel) renderConfirmBar() string {
	var content string
	if m.confirming == confirmJoinG {
		content = theme.Highlight.Render("Join "+m.activeGuildDetail.Name+"?") + "  " +
			theme.Base.Render("[y]es") + "  " +
			theme.Subtle.Render("[n]o / esc")
	} else {
		content = theme.Error.Render("Leave "+m.activeGuildDetail.Name+"?") + "  " +
			theme.Base.Render("[y]es") + "  " +
			theme.Subtle.Render("[n]o / esc")
	}
	style := theme.ActiveBorder
	if m.width > 2 {
		style = style.Width(m.width - 2)
	}
	return style.Render(content)
}

func (m GuildsModel) handleConfirmKey(msg tea.KeyMsg) (GuildsModel, tea.Cmd) {
	switch msg.String() {
	case "y":
		kind := m.confirming
		m.confirming = confirmNoneG
		m = m.refreshContent()
		slug := m.activeGuild
		if kind == confirmJoinG {
			return m, func() tea.Msg { return JoinGuildMsg{Slug: slug} }
		}
		return m, func() tea.Msg { return LeaveGuildMsg{Slug: slug} }
	case "n", "esc":
		m.confirming = confirmNoneG
		m = m.refreshContent()
	}
	return m, nil
}

func (m GuildsModel) buildContent() (string, []int) {
	if m.fetching {
		return theme.Subtle.Render("  loading guilds…"), nil
	}
	sep := "\n"
	lineInc := 1
	if m.relaxed {
		sep = "\n\n"
		lineInc = 2
	}

	var prefix string
	startLine := 0
	if m.refreshing {
		prefix = theme.Subtle.Render("  refreshing…") + "\n"
		startLine++
	}

	if m.view == viewGuildList {
		if len(m.guilds) == 0 {
			if m.err != nil {
				return prefix + theme.Subtle.Render("  couldn't load guilds"), nil
			}
			return prefix + theme.Subtle.Render("  no guilds yet"), nil
		}
		offsets := make([]int, len(m.guilds))
		currentLine := startLine
		var out string
		for i := range m.guilds {
			offsets[i] = currentLine
			rendered := m.renderGuildItem(i)
			out += rendered + sep
			currentLine += lipgloss.Height(rendered) + lineInc - 1
		}
		out += listFooter(m.loading, m.guildsExhausted && len(m.guilds) > 0)
		return prefix + strings.TrimRight(out, "\n"), offsets
	}

	if m.view == viewGuildMembers {
		if len(m.members) == 0 {
			if m.err != nil {
				return prefix + theme.Subtle.Render("  couldn't load members"), nil
			}
			return prefix + theme.Subtle.Render("  no members"), nil
		}
		offsets := make([]int, len(m.members))
		currentLine := startLine
		var out string
		for i := range m.members {
			offsets[i] = currentLine
			rendered := m.renderMemberItem(m.members[i], i == m.memberIndex)
			out += rendered + sep
			currentLine += lipgloss.Height(rendered) + lineInc - 1
		}
		out += listFooter(m.loading, m.membersExhausted && len(m.members) > 0)
		return prefix + strings.TrimRight(out, "\n"), offsets
	}

	// viewGuildPosts
	if len(m.posts) == 0 {
		if m.err != nil {
			return prefix + theme.Subtle.Render("  couldn't load threads"), nil
		}
		return prefix + theme.Subtle.Render("  no threads"), nil
	}
	visible := m.visiblePosts()
	offsets := make([]int, len(visible))
	currentLine := startLine
	var out string
	for i, p := range visible {
		offsets[i] = currentLine
		rendered := m.renderPostItem(p, i == m.postIndex)
		out += rendered + sep
		currentLine += lipgloss.Height(rendered) + lineInc - 1
	}
	out += listFooter(m.loading, m.exhausted)
	return prefix + strings.TrimRight(out, "\n"), offsets
}

// guildIcon returns the icon string if it contains non-ASCII characters (i.e. an
// emoji), or "◆" when the API has returned a plain icon name like "code-filled".
func guildIcon(s string) string {
	for _, r := range s {
		if r > 127 {
			return s
		}
	}
	return "◆"
}

func (m GuildsModel) renderGuildItem(index int) string {
	if index < 0 || index >= len(m.guilds) {
		return ""
	}

	guild := m.guilds[index]
	isSelected := index == m.guildIndex
	innerWidth := m.width - 4

	subtleStyle := theme.Subtle
	if isSelected {
		subtleStyle = theme.Base
	}

	iconStr := guildIcon(guild.Icon)
	icon := subtleStyle.Render(iconStr) + " "
	iconW := lipgloss.Width(icon)

	nameStyle := theme.Base
	if isSelected {
		nameStyle = theme.Highlight
	}
	nameStr := nameStyle.Render(guild.Name)
	countStr := subtleStyle.Render(fmt.Sprintf("%d members", guild.MemberCount))
	countW := lipgloss.Width(countStr)

	var line1 string
	if innerWidth > 0 {
		gap := innerWidth - iconW - lipgloss.Width(nameStr) - countW
		if gap > 0 {
			line1 = icon + nameStr + strings.Repeat(" ", gap) + countStr
		} else {
			line1 = icon + nameStr
		}
	} else {
		line1 = icon + nameStr
	}

	content := line1
	if guild.Bio != "" && innerWidth > 0 {
		bioW := innerWidth - iconW - countW - 2
		if bioW > 0 {
			indent := strings.Repeat(" ", iconW)
			content += "\n" + indent + subtleStyle.Render(truncateStr(guild.Bio, bioW))
		}
	}

	boxStyle := theme.Border
	if isSelected {
		boxStyle = theme.ActiveBorder
	}
	if innerWidth > 0 {
		boxStyle = boxStyle.Width(m.width - 2)
	}
	return boxStyle.Render(content)
}

func (m GuildsModel) renderMemberItem(mem model.GuildMember, selected bool) string {
	innerWidth := m.width - 4

	iconStr := "•"
	if mem.Role == "founder" {
		iconStr = "◆"
	}
	icon := theme.Subtle.Render(iconStr) + " "

	nameStyle := theme.Base
	if selected {
		nameStyle = theme.Highlight
	}
	nameStr := nameStyle.Render("@" + mem.Username)

	roleStr := theme.Subtle.Render(mem.Role)
	joinedStr := theme.Subtle.Render(displayTime(mem.JoinedAt, m.location(), m.timeDisplayFormat, true))
	right := roleStr + "  " + joinedStr

	var line string
	if innerWidth > 0 {
		gap := innerWidth - lipgloss.Width(icon) - lipgloss.Width(nameStr) - lipgloss.Width(right)
		if gap > 0 {
			line = icon + nameStr + strings.Repeat(" ", gap) + right
		} else {
			line = icon + nameStr
		}
	} else {
		line = icon + nameStr
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

func (m GuildsModel) renderPostItem(p model.Post, selected bool) string {
	_, bookmarked := m.bookmarkedPostIDs[p.ID]
	_, watched := m.watchedPostIDs[p.ID]
	return RenderPost(p, selected, bookmarked, watched, m.width, m.location(), m.timeDisplayFormat, postMaxBodyLines)
}

func (m GuildsModel) refreshContent() GuildsModel {
	m.viewport.Height = m.viewportHeight()
	content, offsets := m.buildContent()
	m.itemOffsets = offsets
	m.viewport.SetContent(content)
	return m.ensureSelectedVisible()
}

func (m GuildsModel) ensureSelectedVisible() GuildsModel {
	if !m.ready || len(m.itemOffsets) == 0 {
		return m
	}

	var selectedIndex int
	var itemHeight int
	if m.view == viewGuildList {
		selectedIndex = m.guildIndex
		if selectedIndex >= len(m.guilds) {
			return m
		}
		itemHeight = lipgloss.Height(m.renderGuildItem(selectedIndex))
	} else if m.view == viewGuildMembers {
		selectedIndex = m.memberIndex
		if selectedIndex >= len(m.members) {
			return m
		}
		itemHeight = lipgloss.Height(m.renderMemberItem(m.members[selectedIndex], false))
	} else {
		visible := m.visiblePosts()
		selectedIndex = m.postIndex
		if selectedIndex >= len(visible) {
			return m
		}
		itemHeight = lipgloss.Height(m.renderPostItem(visible[selectedIndex], false))
	}

	if selectedIndex >= len(m.itemOffsets) {
		return m
	}

	itemStart := m.itemOffsets[selectedIndex]
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

func (m GuildsModel) viewportHeight() int {
	h := m.height - theme.ChromeHeight
	if m.panel.IsActive() {
		h -= m.panel.PanelHeight()
	} else if m.confirming != confirmNoneG {
		h -= 3 // confirm prompt: border-top + content + border-bottom
	}
	if h < 1 {
		h = 1
	}
	return h
}

func (m GuildsModel) location() *time.Location {
	if m.loc == nil {
		return time.UTC
	}
	return m.loc
}

// IsBrowsingGuild reports whether the user is viewing a specific guild's threads or members.
func (m GuildsModel) IsBrowsingGuild() bool { return m.activeGuild != "" }

// IsBrowsingMembers reports whether the member list is the active view.
func (m GuildsModel) IsBrowsingMembers() bool { return m.view == viewGuildMembers }

// GetFocusedURLs implements URLProvider. Returns the guild link when browsing
// the guild list, URLs from the selected post when in post-list view.
func (m GuildsModel) GetFocusedURLs() []string {
	if m.view == viewGuildList && len(m.guilds) > 0 && m.guildIndex < len(m.guilds) {
		if link := m.guilds[m.guildIndex].Link; link != "" {
			return []string{link}
		}
		return nil
	}
	if m.view != viewGuildPosts {
		return nil
	}
	visible := m.visiblePosts()
	if m.postIndex < 0 || m.postIndex >= len(visible) {
		return nil
	}
	p := visible[m.postIndex]
	return append(extractURLs(p.Content), attachmentURLs(p.Attachments)...)
}

// IsViewingGuildPosts reports whether the guild post list is currently shown (3-pane applies).
func (m GuildsModel) IsViewingGuildPosts() bool { return m.view == viewGuildPosts }

func (m GuildsModel) IsCompactListActive() bool { return m.IsViewingGuildPosts() }
func (m GuildsModel) ListTitle() string          { return "posts (◆ " + m.ActiveGuildName() + ")" }

// ActiveGuildName returns the display name of the active guild, falling back to the slug if detail has not yet loaded.
func (m GuildsModel) ActiveGuildName() string {
	if m.guildDetailLoaded {
		return m.activeGuildDetail.Name
	}
	return m.activeGuild
}

// IsAtTop reports whether the first post is selected (used to suppress pull-to-refresh in Miller).
func (m GuildsModel) IsAtTop() bool { return m.postIndex == 0 }

// PostCount returns the number of currently visible guild posts.
func (m GuildsModel) PostCount() int { return len(m.visiblePosts()) }

// PostsNextCursor returns the pagination cursor for the next page of guild posts.
func (m GuildsModel) PostsNextCursor() string { return m.nextCursor }

func (m GuildsModel) effectiveMaxDepth() int {
	if m.maxThreadDepth <= 0 {
		return 3
	}
	return m.maxThreadDepth
}

// currentDetailCmd clears the detail pane immediately and starts a debounce timer.
// The API fetch only fires if the selection hasn't changed by the time the timer expires,
// avoiding a flood of calls when the user scrolls quickly through the post list.
func (m GuildsModel) currentDetailCmd() (GuildsModel, tea.Cmd) {
	visible := m.visiblePosts()
	if m.postIndex >= len(visible) {
		return m, nil
	}
	postID := visible[m.postIndex].ID
	if postID == m.threadPostID {
		return m, nil
	}
	m.threadPostID = postID
	m.threadLoading = false
	m.threadReplies = nil
	m.threadFlatTree = nil
	m.threadReplyIndex = -1
	m.threadScrollOffset = 0
	return m, tea.Tick(guildThreadDebounceDelay, func(time.Time) tea.Msg {
		return GuildThreadDebounceMsg{PostID: postID}
	})
}

// CurrentDetailCmd is exported so app.go can trigger the initial detail load when guild posts first arrive.
// Loads immediately without debounce.
func (m GuildsModel) CurrentDetailCmd() (GuildsModel, tea.Cmd) {
	visible := m.visiblePosts()
	if m.postIndex >= len(visible) {
		return m, nil
	}
	postID := visible[m.postIndex].ID
	if postID == m.threadPostID {
		return m, nil
	}
	m.threadPostID = postID
	m.threadLoading = true
	m.threadReplies = nil
	m.threadFlatTree = nil
	m.threadReplyIndex = -1
	m.threadScrollOffset = 0
	return m, func() tea.Msg { return LoadGuildThreadMsg{PostID: postID} }
}

func (m GuildsModel) renderDetailReply(node replyNode, selected bool, width int) string {
	indentW := node.Depth * 3
	cardWidth := width - 2 - indentW
	if cardWidth < 4 {
		cardWidth = 4
	}
	innerWidth := cardWidth - 2
	if innerWidth < 1 {
		innerWidth = 1
	}
	header := theme.Highlight.Render("@" + node.Reply.AuthorUsername)
	if node.ParentUsername != "" {
		header += theme.Subtle.Render("  ↩ @" + node.ParentUsername)
	}
	header += theme.Subtle.Render("  " + displayTime(node.Reply.CreatedAt, m.location(), m.timeDisplayFormat, false))
	body := strings.TrimRight(markdown.Render(node.Reply.Content, innerWidth), "\n")
	boxStyle := theme.Border
	if selected {
		boxStyle = theme.ActiveBorder
	}
	if cardWidth > 0 {
		boxStyle = boxStyle.Width(cardWidth)
	}
	card := boxStyle.Render(lipgloss.JoinVertical(lipgloss.Left, header, body))
	if indentW > 0 {
		return lipgloss.NewStyle().MarginLeft(indentW).Render(card)
	}
	return card
}

func (m GuildsModel) renderCompactPost(p model.Post, selected bool, width int) string {
	username := "@" + p.AuthorUsername
	var preview string
	if p.Title != "" {
		preview = p.Title
	} else {
		preview = strings.TrimSpace(strings.SplitN(p.Content, "\n", 2)[0])
	}
	const sep = "  "
	var indicatorAndName string
	if selected {
		indicatorAndName = theme.Highlight.Render("▶ " + username)
	} else {
		indicatorAndName = theme.Subtle.Render("  ") + theme.Base.Render(username)
	}
	prefixWidth := 2 + lipgloss.Width(username) + len(sep)
	remaining := width - prefixWidth
	if remaining > 1 {
		preview = ansiTruncate(preview, remaining)
	} else {
		preview = ""
	}
	return indicatorAndName + theme.Subtle.Render(sep+preview)
}

// CompactListView returns the compact single-line guild post list for the Miller list pane.
func (m GuildsModel) CompactListView(width, height int) string {
	if !m.ready || m.fetching {
		return theme.Subtle.Render("  loading…")
	}
	visible := m.visiblePosts()
	if len(visible) == 0 {
		return theme.Subtle.Render("  no posts")
	}
	headerLines := 0
	var header string
	if m.refreshing {
		header = theme.Subtle.Render("  fetching new posts...")
		headerLines = 1
	}
	n := len(visible)
	listH := height - headerLines
	if listH < 1 {
		listH = 1
	}
	offset := m.postIndex - listH + 1
	if offset < 0 {
		offset = 0
	}
	if offset+listH > n {
		offset = n - listH
		if offset < 0 {
			offset = 0
		}
	}
	end := offset + listH
	if end > n {
		end = n
	}
	lines := make([]string, 0, end-offset+headerLines)
	if header != "" {
		lines = append(lines, header)
	}
	for i := offset; i < end; i++ {
		lines = append(lines, m.renderCompactPost(visible[i], i == m.postIndex, width))
	}
	return strings.Join(lines, "\n")
}

// pageThreadNav implements pager-style scrolling for the Miller detail pane.
func (m GuildsModel) pageThreadNav(delta, paneH, paneW int) GuildsModel {
	visible := m.visiblePosts()
	if m.postIndex >= len(visible) {
		return m
	}
	p := visible[m.postIndex]
	_, bookmarked := m.bookmarkedPostIDs[p.ID]
	_, watched := m.watchedPostIDs[p.ID]

	postCard := RenderPost(p, false, bookmarked, watched, paneW, m.location(), m.timeDisplayFormat, 0)
	postH := lipgloss.Height(postCard)

	replyStarts := make([]int, len(m.threadFlatTree))
	replyHeights := make([]int, len(m.threadFlatTree))
	pos := postH
	for i, node := range m.threadFlatTree {
		replyStarts[i] = pos
		rendered := m.renderDetailReply(node, false, paneW)
		replyHeights[i] = lipgloss.Height(rendered)
		pos += replyHeights[i]
	}

	m.threadReplyIndex, m.threadScrollOffset = millerPageNav(
		delta, paneH, postH, replyStarts, replyHeights, m.threadReplyIndex, m.threadScrollOffset,
	)
	return m
}

// DetailView returns the full guild post card + threaded replies for the Miller reading pane.
func (m GuildsModel) DetailView(width, height int) string {
	if !m.ready {
		return theme.Subtle.Render("  loading…")
	}
	visible := m.visiblePosts()
	if len(visible) == 0 {
		return theme.Subtle.Render("  no posts")
	}
	if m.postIndex >= len(visible) {
		return theme.Subtle.Render("  select a post")
	}
	p := visible[m.postIndex]
	_, bookmarked := m.bookmarkedPostIDs[p.ID]
	_, watched := m.watchedPostIDs[p.ID]

	postSelected := m.threadReplyIndex < 0
	card := RenderPost(p, postSelected, bookmarked, watched, width, m.location(), m.timeDisplayFormat, 0)

	var parts []string
	startLines := []int{0}
	lineCount := lipgloss.Height(card)
	parts = append(parts, card)

	if m.threadLoading {
		parts = append(parts, theme.Subtle.Render("  loading replies…"))
	} else {
		for i, node := range m.threadFlatTree {
			rendered := m.renderDetailReply(node, i == m.threadReplyIndex, width)
			startLines = append(startLines, lineCount)
			lineCount += lipgloss.Height(rendered)
			parts = append(parts, rendered)
		}
	}
	if m.panel.IsActive() {
		parts = append(parts, m.panel.SetWidth(width).View())
	}

	fullContent := lipgloss.JoinVertical(lipgloss.Left, parts...)
	return sliceContent(fullContent, m.threadScrollOffset, height, lineCount)
}
