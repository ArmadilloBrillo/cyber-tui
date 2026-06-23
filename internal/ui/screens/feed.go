package screens

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/markdown"
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

// LoadFeedDetailMsg is emitted when the selected post changes so the app can
// fetch replies for the Miller reading pane.
type LoadFeedDetailMsg struct{ PostID string }

// FeedDetailRepliesMsg delivers fetched replies back to FeedModel for the reading pane.
type FeedDetailRepliesMsg struct {
	PostID  string
	Replies []model.Reply
}

// FeedDetailNavMsg is emitted by the Miller layout when the user presses j/k in
// focusDetail. PaneHeight and PaneWidth are the detail column dimensions so the
// handler can implement pager-style line-by-line scrolling.
type FeedDetailNavMsg struct {
	Delta      int
	PaneHeight int
	PaneWidth  int
}

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
	loaded            bool           // true once the first page has successfully loaded
	exhausted         bool           // true once API returned an empty cursor
	relaxed           bool           // true = blank line between posts (relaxed density)
	loc               *time.Location // timezone for timestamp display; nil = UTC
	timeDisplayFormat string         // API setting: "datetime", "relative", "unix", "swatch"

	currentUsername  string // set after login; used to guard the delete key
	confirmingDelete bool   // true while the delete-post confirmation overlay is shown

	bookmarkedPostIDs map[string]struct{}
	watchedPostIDs    map[string]struct{}
	filterNSFW        bool

	// Miller reading pane: replies for the currently selected post.
	detailPostID     string
	detailReplies    []model.Reply
	detailFlatTree   []replyNode // DFS-ordered tree built from detailReplies
	detailReplyIndex int         // -1 = post selected; 0+ = index into detailFlatTree
	detailScrollOffset int       // raw line offset for pager scrolling in the detail pane
	detailLoading    bool
}

func NewFeedModel() FeedModel {
	return FeedModel{
		panel:            NewPostComposePanel(0),
		detailReplyIndex: -1,
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

func (m FeedModel) IsLoaded() bool { return m.loaded }

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
	m.loaded = true
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

	case FeedDetailRepliesMsg:
		visible := m.visiblePosts()
		if m.selectedIndex < len(visible) && visible[m.selectedIndex].ID == msg.PostID {
			m.detailPostID = msg.PostID
			m.detailReplies = msg.Replies
			m.detailFlatTree = buildReplyTree(msg.Replies, 3)
			m.detailReplyIndex = -1
			m.detailScrollOffset = 0
			m.detailLoading = false
		}
		return m, nil

	case FeedDetailNavMsg:
		if msg.PaneHeight > 0 && msg.PaneWidth > 0 {
			m = m.pageDetailNav(msg.Delta, msg.PaneHeight, msg.PaneWidth)
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
				var detailCmd tea.Cmd
				m, detailCmd = m.currentDetailCmd()
				return m, detailCmd
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
				var detailCmd tea.Cmd
				m, detailCmd = m.currentDetailCmd()
				return m, detailCmd
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

// currentDetailCmd emits LoadFeedDetailMsg for the currently selected post and marks
// the detail pane as loading. Returns the updated model and the command to run.
func (m FeedModel) currentDetailCmd() (FeedModel, tea.Cmd) {
	visible := m.visiblePosts()
	if m.selectedIndex >= len(visible) {
		return m, nil
	}
	postID := visible[m.selectedIndex].ID
	if postID == m.detailPostID {
		return m, nil // already loaded/loading this post
	}
	m.detailPostID = postID
	m.detailLoading = true
	m.detailReplies = nil
	m.detailFlatTree = nil
	m.detailReplyIndex = -1
	m.detailScrollOffset = 0
	return m, func() tea.Msg { return LoadFeedDetailMsg{PostID: postID} }
}

// CurrentDetailCmd is exported so app.go can trigger the initial detail load after
// the feed's first page arrives.
func (m FeedModel) CurrentDetailCmd() (FeedModel, tea.Cmd) {
	return m.currentDetailCmd()
}

// renderDetailReply renders a reply in the Miller reading pane using the same tree-aware
// card style as the post-detail screen: depth indentation, parent back-reference, active border.
func (m FeedModel) renderDetailReply(node replyNode, selected bool, width int) string {
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

// renderCompactPost renders a single-line summary of a post for the Miller compact list pane.
// Selected: "▶ @username" in accent + preview in subtle.
// Unselected: "  @username" in base colour + preview in subtle.
func (m FeedModel) renderCompactPost(p model.Post, selected bool, width int) string {
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
	prefixWidth := 2 + lipgloss.Width(username) + len(sep) // indicator + username + separator
	remaining := width - prefixWidth
	if remaining > 1 {
		preview = ansiTruncate(preview, remaining)
	} else {
		preview = ""
	}
	return indicatorAndName + theme.Subtle.Render(sep+preview)
}

// IsAtTop reports whether the first post is selected (used by the Miller layout to suppress
// pull-to-refresh when navigating the compact list).
func (m FeedModel) IsAtTop() bool { return m.selectedIndex == 0 }

// PostCount returns the number of currently visible posts (respects NSFW filter).
func (m FeedModel) PostCount() int { return len(m.visiblePosts()) }

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

	// When refreshing, reserve the first row for the status message and push
	// posts down by one row, matching the behaviour of the tabbed layout.
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
	// Sticky scroll: keep selectedIndex visible, scrolling so it stays at the bottom of the window.
	offset := m.selectedIndex - listH + 1
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
	lines := make([]string, 0, end-offset)
	if header != "" {
		lines = append(lines, header)
	}
	for i := offset; i < end; i++ {
		lines = append(lines, m.renderCompactPost(visible[i], i == m.selectedIndex, width))
	}
	return strings.Join(lines, "\n")
}

// pageDetailNav implements pager-style scrolling for the Miller detail pane.
func (m FeedModel) pageDetailNav(delta, paneH, paneW int) FeedModel {
	visible := m.visiblePosts()
	if m.selectedIndex >= len(visible) {
		return m
	}
	p := visible[m.selectedIndex]
	_, bookmarked := m.bookmarkedPostIDs[p.ID]
	_, watched := m.watchedPostIDs[p.ID]

	postCard := RenderPost(p, false, bookmarked, watched, paneW, m.location(), m.timeDisplayFormat, 0)
	postH := lipgloss.Height(postCard)

	replyStarts := make([]int, len(m.detailFlatTree))
	replyHeights := make([]int, len(m.detailFlatTree))
	pos := postH
	for i, node := range m.detailFlatTree {
		replyStarts[i] = pos
		rendered := m.renderDetailReply(node, false, paneW)
		replyHeights[i] = lipgloss.Height(rendered)
		pos += replyHeights[i]
	}

	m.detailReplyIndex, m.detailScrollOffset = millerPageNav(
		delta, paneH, postH, replyStarts, replyHeights, m.detailReplyIndex, m.detailScrollOffset,
	)
	return m
}

// DetailView returns the full post card + threaded replies for the Miller reading pane.
// The post body is rendered without truncation (maxBodyLines = 0). The selected item
// (post or reply, determined by detailReplyIndex) is scrolled into view.
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

	// Render all items and track each item's start line for scroll computation.
	postSelected := m.detailReplyIndex < 0
	card := RenderPost(p, postSelected, bookmarked, watched, width, m.location(), m.timeDisplayFormat, 0)

	var parts []string
	startLines := []int{0} // startLines[0] = post start line (always 0)
	lineCount := lipgloss.Height(card)
	parts = append(parts, card)

	if m.detailLoading {
		parts = append(parts, theme.Subtle.Render("  loading replies…"))
	} else {
		for i, node := range m.detailFlatTree {
			rendered := m.renderDetailReply(node, i == m.detailReplyIndex, width)
			startLines = append(startLines, lineCount)
			lineCount += lipgloss.Height(rendered)
			parts = append(parts, rendered)
		}
	}
	if m.panel.IsActive() {
		parts = append(parts, m.panel.View())
	}

	fullContent := lipgloss.JoinVertical(lipgloss.Left, parts...)
	return sliceContent(fullContent, m.detailScrollOffset, height, lineCount)
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
