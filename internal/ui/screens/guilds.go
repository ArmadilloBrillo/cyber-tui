package screens

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ragnar/cyber-tui/internal/model"
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
	activeGuild string
	posts       []model.Post
	postIndex   int
	nextCursor  string
	exhausted   bool
	loading     bool
	fetching    bool
	refreshing  bool
	loaded      bool

	// Guild members state
	members           []model.GuildMember
	memberIndex       int
	membersNextCursor string
	membersExhausted  bool

	// Compose panel for new guild threads (visible in posts view).
	panel PostComposePanel

	// Shared
	viewport          viewport.Model
	itemOffsets       []int
	width             int
	bookmarkedPostIDs map[string]struct{}
	height            int
	ready             bool
	err               error
	loc               *time.Location
	relaxed           bool
	timeDisplayFormat string
}

// NewGuildsModel returns a zero-value GuildsModel ready for first use.
func NewGuildsModel() GuildsModel {
	return GuildsModel{
		panel: NewPostComposePanel(0),
	}
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

// SetError stores an error and clears the loading state.
func (m GuildsModel) SetError(err error) GuildsModel {
	m.err = err
	m.loading = false
	m.fetching = false
	m.refreshing = false
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
		if m.ready {
			m = m.refreshContent()
		}
		return m, nil

	case BookmarkedIDsMsg:
		m.bookmarkedPostIDs = msg.PostIDs
		if m.ready {
			m = m.refreshContent()
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
				if m.postIndex < len(m.posts)-1 {
					m.postIndex++
					m = m.refreshContent()
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
				if len(m.posts) > 0 && m.postIndex < len(m.posts) {
					post := m.posts[m.postIndex]
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
	if m.err != nil {
		return theme.Error.Render(fmt.Sprintf("guilds error: %s", m.err))
	}
	if !m.ready {
		return theme.Subtle.Render("loading guilds...")
	}
	if m.panel.IsActive() {
		return lipgloss.JoinVertical(lipgloss.Left,
			m.viewport.View(),
			m.panel.View(),
		)
	}
	return m.viewport.View()
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
		return prefix + theme.Subtle.Render("  no threads"), nil
	}
	offsets := make([]int, len(m.posts))
	currentLine := startLine
	var out string
	for i := range m.posts {
		offsets[i] = currentLine
		rendered := m.renderPostItem(m.posts[i], i == m.postIndex)
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
	return RenderPost(p, selected, bookmarked, m.width, m.location(), m.timeDisplayFormat)
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
		selectedIndex = m.postIndex
		if selectedIndex >= len(m.posts) {
			return m
		}
		itemHeight = lipgloss.Height(m.renderPostItem(m.posts[selectedIndex], false))
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
	if m.view != viewGuildPosts || len(m.posts) == 0 {
		return nil
	}
	if m.postIndex < 0 || m.postIndex >= len(m.posts) {
		return nil
	}
	p := m.posts[m.postIndex]
	return append(extractURLs(p.Content), attachmentURLs(p.Attachments)...)
}
