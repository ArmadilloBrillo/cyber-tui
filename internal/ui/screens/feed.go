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

// LoadMoreFeedMsg is emitted by FeedModel when the viewport reaches the bottom
// and a next-page cursor is available. App intercepts this and fires the API call.
type LoadMoreFeedMsg struct{ Cursor string }

// RefreshFeedMsg is emitted when the user presses up at the top of the feed.
// App intercepts this and re-fetches the feed from the start.
type RefreshFeedMsg struct{}

// ShowPostMsg is emitted when the user presses Enter on a selected post.
type ShowPostMsg struct{ Post model.Post }

// ShowPostForReplyMsg is emitted when the user presses 'r' on a selected post.
// App navigates to post detail and opens the compose box immediately.
type ShowPostForReplyMsg struct{ Post model.Post }

// SubmitNewPostMsg is emitted when the user submits a new post from the Feed.
type SubmitNewPostMsg struct {
	Content  string
	Title    string // empty = no title
	Topics   []string
	IsPublic bool
	IsNSFW   bool
}

type FeedModel struct {
	posts             []model.Post
	postOffsets       []int // start line of each post within the viewport content
	viewport          viewport.Model
	panel             PostComposePanel
	defaultPublicPost bool // mirrored from settings; initialises panel.isPublic on each open
	width             int
	height            int
	selectedIndex     int
	ready             bool
	err               error
	nextCursor        string
	loading           bool
	fetching          bool           // true while the initial (or tab-switch) load is in flight
	refreshing        bool           // true while re-fetching newest posts (up at top)
	exhausted         bool           // true once API returned an empty cursor
	relaxed           bool           // true = blank line between posts (relaxed density)
	loc               *time.Location // timezone for timestamp display; nil = UTC
	timeDisplayFormat string         // API setting: "datetime", "relative", "unix", "swatch"

	currentUsername  string // set after login; used to guard the delete key
	confirmingDelete bool   // true while the delete-post confirmation overlay is shown

	bookmarkedPostIDs map[string]struct{}
	watchedPostIDs    map[string]struct{}
	filterNSFW        bool
}

func NewFeedModel() FeedModel {
	return FeedModel{
		panel: NewPostComposePanel(0),
	}
}

// ParseTopics splits a comma-separated topic string and caps the result at 3.
// Empty parts are ignored. Leading/trailing whitespace is trimmed.
func ParseTopics(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		t := strings.TrimSpace(part)
		if t != "" {
			out = append(out, t)
		}
	}
	if len(out) > 3 {
		out = out[:3]
	}
	return out
}

func (m FeedModel) SetFetching() FeedModel {
	m.fetching = true
	m.err = nil
	if m.ready {
		m = m.refreshContent()
	}
	return m
}

func (m FeedModel) SetPosts(posts []model.Post, cursor string) FeedModel {
	m.err = nil
	var prevID string
	if oldVisible := m.visiblePosts(); m.selectedIndex < len(oldVisible) {
		prevID = oldVisible[m.selectedIndex].ID
	}

	m.posts = posts
	m.nextCursor = cursor
	m.exhausted = cursor == ""
	m.loading = false
	m.fetching = false
	m.refreshing = false
	m.selectedIndex = 0
	if prevID != "" {
		for i, p := range m.visiblePosts() {
			if p.ID == prevID {
				m.selectedIndex = i
				break
			}
		}
	}
	if m.ready {
		m = m.refreshContent()
		if m.selectedIndex == 0 {
			m.viewport.GotoTop()
		} else {
			m = m.ensureSelectedVisible()
		}
	}
	return m
}

func (m FeedModel) AppendPosts(posts []model.Post, cursor string) FeedModel {
	m.posts = append(m.posts, posts...)
	m.nextCursor = cursor
	m.exhausted = cursor == ""
	m.loading = false
	m.fetching = false
	if m.ready {
		m = m.refreshContent() // selectedIndex preserved; scroll position preserved
	}
	return m
}

func (m FeedModel) SetError(err error) FeedModel {
	m.err = err
	m.loading = false
	m.fetching = false
	m.refreshing = false
	if m.ready {
		m = m.refreshContent()
	}
	return m
}

// SetCurrentUsername records the logged-in user's username so the feed can
// restrict the delete key to the user's own posts.
func (m FeedModel) SetCurrentUsername(username string) FeedModel {
	m.currentUsername = username
	return m
}

// RemovePost removes a post from the local list by ID (called after a
// successful DELETE API call so the list reflects the deletion immediately).
func (m FeedModel) RemovePost(postID string) FeedModel {
	for i, p := range m.posts {
		if p.ID == postID {
			m.posts = append(m.posts[:i], m.posts[i+1:]...)
			if vis := len(m.visiblePosts()); m.selectedIndex >= vis {
				m.selectedIndex = vis - 1
				if m.selectedIndex < 0 {
					m.selectedIndex = 0
				}
			}
			break
		}
	}
	if m.ready {
		m = m.refreshContent()
	}
	return m
}

func (m FeedModel) visiblePosts() []model.Post {
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

func (m FeedModel) SetRelaxed(relaxed bool) FeedModel {
	m.relaxed = relaxed
	if m.ready {
		m = m.refreshContent()
		m = m.ensureSelectedVisible()
	}
	return m
}

func (m FeedModel) location() *time.Location {
	if m.loc == nil {
		return time.UTC
	}
	return m.loc
}

func (m FeedModel) SetLocation(loc *time.Location) FeedModel {
	if loc == nil {
		loc = time.UTC
	}
	m.loc = loc
	if m.ready {
		m = m.refreshContent()
	}
	return m
}

// refreshContent rebuilds the viewport content and updates postOffsets.
// Call this whenever posts, selectedIndex, or width changes.
func (m FeedModel) refreshContent() FeedModel {
	content, offsets := m.buildContent()
	m.postOffsets = offsets
	m.viewport.SetContent(content)
	return m
}

// ensureSelectedVisible scrolls the viewport the minimum amount so the
// selected post is fully visible. If the post is taller than the viewport,
// its top is aligned with the viewport top.
func (m FeedModel) ensureSelectedVisible() FeedModel {
	visible := m.visiblePosts()
	if !m.ready || len(m.postOffsets) == 0 || m.selectedIndex >= len(visible) {
		return m
	}
	postStart := m.postOffsets[m.selectedIndex]
	postHeight := lipgloss.Height(m.renderPost(visible[m.selectedIndex], false))
	postEnd := postStart + postHeight - 1

	viewTop := m.viewport.YOffset
	viewBottom := viewTop + m.viewport.Height - 1

	if postStart < viewTop {
		// Post is above the viewport — scroll up to its top.
		m.viewport.SetYOffset(postStart)
	} else if postEnd > viewBottom {
		// Post is (partially) below the viewport.
		if postHeight <= m.viewport.Height {
			// Post fits — align its bottom with the viewport bottom.
			m.viewport.SetYOffset(postEnd - m.viewport.Height + 1)
		} else {
			// Post is taller than the viewport — show from the top.
			m.viewport.SetYOffset(postStart)
		}
	}
	return m
}

// ComposeActive reports whether the new-post compose panel is open.
func (m FeedModel) ComposeActive() bool { return m.panel.IsActive() }

func (m FeedModel) Init() tea.Cmd { return nil }

func (m FeedModel) Update(msg tea.Msg) (FeedModel, tea.Cmd) {
	switch msg := msg.(type) {
	case SharedConfigMsg:
		m.timeDisplayFormat = msg.Settings.TimeDisplayFormat
		m.defaultPublicPost = msg.Settings.DefaultPublicPost
		m = m.SetRelaxed(msg.Relaxed)
		m = m.SetLocation(msg.Loc)
		if msg.Settings.FilterNSFW != m.filterNSFW {
			m.filterNSFW = msg.Settings.FilterNSFW
			m.selectedIndex = 0
			if m.ready {
				m = m.refreshContent()
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
		content := msg.Content
		title := m.panel.TitleValue()
		topics := ParseTopics(m.panel.TopicsRaw())
		isPublic := m.panel.IsPublic()
		isNSFW := m.panel.IsNSFW()
		m = m.closeCompose()
		return m, func() tea.Msg {
			return SubmitNewPostMsg{Content: content, Title: title, Topics: topics, IsPublic: isPublic, IsNSFW: isNSFW}
		}

	case ComposeCancelMsg:
		m = m.closeCompose()
		return m, nil

	case tea.KeyMsg:
		// Confirmation overlay intercepts all keys while active.
		if m.confirmingDelete {
			switch msg.String() {
			case "y":
				if visible := m.visiblePosts(); m.selectedIndex < len(visible) {
					postID := visible[m.selectedIndex].ID
					m.confirmingDelete = false
					m.viewport.Height = m.viewportHeight()
					return m, func() tea.Msg { return DeletePostMsg{PostID: postID} }
				}
				m.confirmingDelete = false
				m.viewport.Height = m.viewportHeight()
			case "n", "esc":
				m.confirmingDelete = false
				m.viewport.Height = m.viewportHeight()
			}
			return m, nil
		}

		if m.panel.IsActive() {
			oldH := m.panel.PanelHeight()
			var cmd tea.Cmd
			m.panel, cmd = m.panel.Update(msg)
			if m.panel.PanelHeight() != oldH {
				m.viewport.Height = m.viewportHeight()
			}
			return m, cmd
		}
		switch msg.String() {
		case "up", "k":
			if m.selectedIndex > 0 {
				m.selectedIndex--
				m = m.refreshContent()
				m = m.ensureSelectedVisible()
			} else if !m.loading && !m.refreshing {
				m.refreshing = true
				m = m.refreshContent()
				return m, func() tea.Msg { return RefreshFeedMsg{} }
			}
			return m, nil
		case "enter":
			if visible := m.visiblePosts(); len(visible) > 0 && m.selectedIndex < len(visible) {
				post := visible[m.selectedIndex]
				return m, func() tea.Msg { return ShowPostMsg{Post: post} }
			}
		case "r":
			if visible := m.visiblePosts(); len(visible) > 0 && m.selectedIndex < len(visible) {
				post := visible[m.selectedIndex]
				return m, func() tea.Msg { return ShowPostForReplyMsg{Post: post} }
			}
		case "p":
			if visible := m.visiblePosts(); len(visible) > 0 && m.selectedIndex < len(visible) {
				username := visible[m.selectedIndex].AuthorUsername
				return m, func() tea.Msg { return ShowUserProfileMsg{Username: username} }
			}
			return m, nil
		case "b":
			if visible := m.visiblePosts(); len(visible) > 0 && m.selectedIndex < len(visible) {
				postID := visible[m.selectedIndex].ID
				return m, func() tea.Msg { return BookmarkPostMsg{PostID: postID} }
			}
			return m, nil
		case "w":
			if visible := m.visiblePosts(); len(visible) > 0 && m.selectedIndex < len(visible) {
				postID := visible[m.selectedIndex].ID
				return m, func() tea.Msg { return ToggleWatchPostMsg{PostID: postID} }
			}
			return m, nil
		case "d":
			if visible := m.visiblePosts(); len(visible) > 0 && m.selectedIndex < len(visible) &&
				visible[m.selectedIndex].AuthorUsername == m.currentUsername {
				m.confirmingDelete = true
				m.viewport.Height = m.viewportHeight()
			}
			return m, nil
		case "n":
			var cmd tea.Cmd
			m.panel, cmd = m.panel.Open(m.defaultPublicPost)
			m.viewport.Height = m.viewportHeight()
			return m, cmd
		case "down", "j":
			if m.selectedIndex < len(m.visiblePosts())-1 {
				m.selectedIndex++
				m = m.refreshContent()
				m = m.ensureSelectedVisible()
			} else {
				var loadCmd tea.Cmd
				m, loadCmd = m.triggerLoadMore()
				if loadCmd != nil {
					return m, loadCmd
				}
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)

	if m.ready && m.viewport.AtBottom() {
		var loadCmd tea.Cmd
		m, loadCmd = m.triggerLoadMore()
		if loadCmd != nil {
			return m, tea.Batch(cmd, loadCmd)
		}
	}

	return m, cmd
}

// viewportHeight returns the viewport height in rows, shrinking to make room
// for the compose panel and the delete-confirmation overlay when active.
func (m FeedModel) viewportHeight() int {
	h := m.height - theme.ChromeHeight
	if m.panel.IsActive() {
		h -= m.panel.PanelHeight()
	}
	if m.confirmingDelete {
		h -= confirmBoxHeight
	}
	if h < 1 {
		h = 1
	}
	return h
}

func (m FeedModel) closeCompose() FeedModel {
	m.panel = m.panel.Close()
	m.viewport.Height = m.viewportHeight()
	return m
}

func (m FeedModel) triggerLoadMore() (FeedModel, tea.Cmd) {
	if m.loading || m.exhausted || m.nextCursor == "" {
		return m, nil
	}
	m.loading = true
	cursor := m.nextCursor
	m = m.refreshContent()
	m.viewport.ScrollDown(1)
	return m, func() tea.Msg { return LoadMoreFeedMsg{Cursor: cursor} }
}

// buildContent renders all posts into a single string for the viewport and
// returns the start line of each post so ensureSelectedVisible can scroll accurately.
func (m FeedModel) buildContent() (string, []int) {
	if m.fetching {
		return theme.Subtle.Render("  loading feed…"), nil
	}
	var prefix string
	startLine := 0
	if m.refreshing {
		prefix = theme.Subtle.Render("  fetching new posts...") + "\n"
		startLine = 1
	}
	if len(m.posts) == 0 {
		if m.err != nil {
			return prefix + theme.Subtle.Render("  couldn't load feed"), nil
		}
		return prefix + theme.Subtle.Render("  no posts yet"), nil
	}
	sep := "\n"
	lineInc := 0
	if m.relaxed {
		sep = "\n\n"
		lineInc = 1
	}
	visible := m.visiblePosts()
	offsets := make([]int, len(visible))
	var out string
	currentLine := startLine
	for i, p := range visible {
		offsets[i] = currentLine
		rendered := m.renderPost(p, i == m.selectedIndex)
		out += rendered + sep
		currentLine += lipgloss.Height(rendered) + lineInc
	}
	if m.loading {
		out += theme.Subtle.Render("  loading more…") + "\n"
	} else if m.exhausted {
		out += theme.Subtle.Render("  — end of feed —") + "\n"
	}
	return prefix + out, offsets
}

func (m FeedModel) renderPost(p model.Post, selected bool) string {
	_, bookmarked := m.bookmarkedPostIDs[p.ID]
	_, watched := m.watchedPostIDs[p.ID]
	return RenderPost(p, selected, bookmarked, watched, m.width, m.location(), m.timeDisplayFormat, postMaxBodyLines)
}

// renderCompactPost renders a single-line summary of a post for the Miller compact list pane.
// Format: "▶ @username  title_or_first_line" (selected) or "  @username  title_or_first_line".
func (m FeedModel) renderCompactPost(p model.Post, selected bool, width int) string {
	indicator := "  "
	style := theme.Subtle
	if selected {
		indicator = "▶ "
		style = theme.Highlight
	}
	username := "@" + p.AuthorUsername
	var preview string
	if p.Title != "" {
		preview = p.Title
	} else {
		preview = strings.TrimSpace(strings.SplitN(p.Content, "\n", 2)[0])
	}
	prefix := indicator + username + "  "
	remaining := width - lipgloss.Width(prefix)
	if remaining > 1 {
		// ansi.Truncate handles multi-byte runes correctly
		preview = ansiTruncate(preview, remaining)
	} else {
		preview = ""
	}
	return style.Render(prefix + preview)
}

// ansiTruncate truncates s to at most maxWidth terminal columns, appending "…" if truncated.
// Operates on plain text (no ANSI codes in post titles or raw content first lines).
func ansiTruncate(s string, maxWidth int) string {
	runes := []rune(s)
	if len(runes) <= maxWidth {
		return s
	}
	return string(runes[:maxWidth-1]) + "…"
}

// CompactListView returns the compact single-line post list for the Miller reading pane.
// It calculates a sticky-scroll window of height rows without storing extra state.
func (m FeedModel) CompactListView(width, height int) string {
	if !m.ready || m.fetching {
		return theme.Subtle.Render("  loading…")
	}
	visible := m.visiblePosts()
	if len(visible) == 0 {
		return theme.Subtle.Render("  no posts")
	}
	n := len(visible)
	// Sticky scroll: keep selectedIndex visible, scrolling so it stays at the bottom of the window.
	offset := m.selectedIndex - height + 1
	if offset < 0 {
		offset = 0
	}
	if offset+height > n {
		offset = n - height
		if offset < 0 {
			offset = 0
		}
	}
	end := offset + height
	if end > n {
		end = n
	}
	lines := make([]string, 0, end-offset)
	for i := offset; i < end; i++ {
		lines = append(lines, m.renderCompactPost(visible[i], i == m.selectedIndex, width))
	}
	return strings.Join(lines, "\n")
}

// DetailView returns the full post card for the Miller reading pane.
// The selected post is rendered without body truncation (maxBodyLines = 0).
func (m FeedModel) DetailView(width, height int) string {
	if !m.ready {
		return theme.Subtle.Render("  loading…")
	}
	visible := m.visiblePosts()
	if len(visible) == 0 {
		return theme.Subtle.Render("  no posts")
	}
	if m.selectedIndex >= len(visible) {
		return theme.Subtle.Render("  select a post")
	}
	p := visible[m.selectedIndex]
	_, bookmarked := m.bookmarkedPostIDs[p.ID]
	_, watched := m.watchedPostIDs[p.ID]
	card := RenderPost(p, true, bookmarked, watched, width, m.location(), m.timeDisplayFormat, 0)
	if m.panel.IsActive() {
		return lipgloss.JoinVertical(lipgloss.Left, card, m.panel.View())
	}
	return card
}

func (m FeedModel) View() string {
	if !m.ready {
		return theme.Subtle.Render("loading feed...")
	}

	if m.confirmingDelete {
		prompt := theme.Error.Render("Delete this post?") + "  " +
			theme.Base.Render("[y]es") + "  " +
			theme.Subtle.Render("[n]o / esc")
		promptView := theme.ActiveBorder.Width(m.width - 2).Render(prompt)
		return lipgloss.JoinVertical(lipgloss.Left,
			m.viewport.View(),
			promptView,
		)
	}

	if m.panel.IsActive() {
		return lipgloss.JoinVertical(lipgloss.Left,
			m.viewport.View(),
			m.panel.View(),
		)
	}
	return m.viewport.View()
}

// GetFocusedURLs implements URLProvider. Returns URLs from the selected post's content.
func (m FeedModel) GetFocusedURLs() []string {
	visible := m.visiblePosts()
	if m.selectedIndex < 0 || m.selectedIndex >= len(visible) {
		return nil
	}
	p := visible[m.selectedIndex]
	return append(extractURLs(p.Content), attachmentURLs(p.Attachments)...)
}
