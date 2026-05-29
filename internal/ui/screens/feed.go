package screens

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
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
	posts              []model.Post
	postOffsets        []int // start line of each post within the viewport content
	viewport           viewport.Model
	compose            ComposeModel
	topicsInput        textinput.Model
	topicsFocused      bool
	titleInput         textinput.Model
	titleFocused       bool
	composePublic      bool
	composeNSFW        bool
	defaultPublicPost  bool // mirrored from settings; initialises composePublic on each open
	width              int
	height             int
	selectedIndex      int
	ready              bool
	err                error
	nextCursor         string
	loading            bool
	fetching           bool // true while the initial (or tab-switch) load is in flight
	refreshing         bool // true while re-fetching newest posts (up at top)
	exhausted          bool // true once API returned an empty cursor
	relaxed            bool           // true = blank line between posts (relaxed density)
	loc                *time.Location // timezone for timestamp display; nil = UTC
	timeDisplayFormat  string         // API setting: "datetime", "relative", "unix", "swatch"

	currentUsername  string // set after login; used to guard the delete key
	confirmingDelete bool   // true while the delete-post confirmation overlay is shown

	bookmarkedPostIDs map[string]struct{}
}

func NewFeedModel() FeedModel {
	ti := textinput.New()
	ti.Placeholder = "add topics  (go, my topic, …  max 3)"
	titleTI := textinput.New()
	titleTI.Placeholder = "title (optional)"
	return FeedModel{
		compose:     NewComposeModel(0),
		topicsInput: ti,
		titleInput:  titleTI,
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
	if m.ready {
		m = m.refreshContent()
	}
	return m
}

func (m FeedModel) SetPosts(posts []model.Post, cursor string) FeedModel {
	var prevID string
	if m.selectedIndex < len(m.posts) {
		prevID = m.posts[m.selectedIndex].ID
	}

	m.posts = posts
	m.nextCursor = cursor
	m.exhausted = cursor == ""
	m.loading = false
	m.fetching = false
	m.refreshing = false
	m.selectedIndex = 0
	if prevID != "" {
		for i, p := range m.posts {
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
			if m.selectedIndex >= len(m.posts) && m.selectedIndex > 0 {
				m.selectedIndex = len(m.posts) - 1
			}
			break
		}
	}
	if m.ready {
		m = m.refreshContent()
	}
	return m
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
	if !m.ready || len(m.postOffsets) == 0 || m.selectedIndex >= len(m.posts) {
		return m
	}
	postStart := m.postOffsets[m.selectedIndex]
	postHeight := lipgloss.Height(m.renderPost(m.posts[m.selectedIndex], false))
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

// ComposeActive reports whether the new-post compose box is open.
func (m FeedModel) ComposeActive() bool { return m.compose.IsActive() }

func (m FeedModel) Init() tea.Cmd { return nil }

func (m FeedModel) Update(msg tea.Msg) (FeedModel, tea.Cmd) {
	switch msg := msg.(type) {
	case SharedConfigMsg:
		m.timeDisplayFormat = msg.Settings.TimeDisplayFormat
		m.defaultPublicPost = msg.Settings.DefaultPublicPost
		m = m.SetRelaxed(msg.Relaxed)
		m = m.SetLocation(msg.Loc)
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
		m.compose = m.compose.SetWidth(msg.Width)
		innerW := msg.Width - 4
		if innerW < 1 {
			innerW = 1
		}
		m.topicsInput.Width = innerW
		m.titleInput.Width = innerW
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
		title := strings.TrimSpace(m.titleInput.Value())
		topics := ParseTopics(m.topicsInput.Value())
		isPublic := m.composePublic
		isNSFW := m.composeNSFW
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
				if m.selectedIndex < len(m.posts) {
					postID := m.posts[m.selectedIndex].ID
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

		if m.compose.IsActive() {
			switch msg.String() {
			case "alt+p":
				m.composePublic = !m.composePublic
				return m, nil
			case "alt+s":
				m.composeNSFW = !m.composeNSFW
				return m, nil
			case "tab":
				switch {
				case m.topicsFocused:
					m.topicsFocused = false
					m.topicsInput.Blur()
					var cmd tea.Cmd
					m.compose, cmd = m.compose.SetFocused(true)
					return m, cmd
				case m.titleFocused:
					m.titleFocused = false
					m.titleInput.Blur()
					m.topicsFocused = true
					cmd := m.topicsInput.Focus()
					return m, cmd
				default:
					m.compose, _ = m.compose.SetFocused(false)
					m.titleFocused = true
					cmd := m.titleInput.Focus()
					return m, cmd
				}
			case "ctrl+s":
				if m.topicsFocused || m.titleFocused {
					content := m.compose.Content()
					title := strings.TrimSpace(m.titleInput.Value())
					topics := ParseTopics(m.topicsInput.Value())
					isPublic := m.composePublic
					isNSFW := m.composeNSFW
					m = m.closeCompose()
					return m, func() tea.Msg {
						return SubmitNewPostMsg{Content: content, Title: title, Topics: topics, IsPublic: isPublic, IsNSFW: isNSFW}
					}
				}
			case "esc":
				if m.topicsFocused || m.titleFocused {
					m = m.closeCompose()
					return m, nil
				}
			}
			if m.topicsFocused {
				var cmd tea.Cmd
				filtered, ok := filterAmbiguousKeyMsg(msg)
				if !ok {
					return m, nil
				}
				m.topicsInput, cmd = m.topicsInput.Update(filtered)
				return m, cmd
			}
			if m.titleFocused {
				var cmd tea.Cmd
				filtered, ok := filterAmbiguousKeyMsg(msg)
				if !ok {
					return m, nil
				}
				m.titleInput, cmd = m.titleInput.Update(filtered)
				return m, cmd
			}
			var cmd tea.Cmd
			m.compose, cmd = m.compose.Update(msg)
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
			if len(m.posts) > 0 && m.selectedIndex < len(m.posts) {
				post := m.posts[m.selectedIndex]
				return m, func() tea.Msg { return ShowPostMsg{Post: post} }
			}
		case "r":
			if len(m.posts) > 0 && m.selectedIndex < len(m.posts) {
				post := m.posts[m.selectedIndex]
				return m, func() tea.Msg { return ShowPostForReplyMsg{Post: post} }
			}
		case "p":
			if len(m.posts) > 0 {
				username := m.posts[m.selectedIndex].AuthorUsername
				return m, func() tea.Msg { return ShowUserProfileMsg{Username: username} }
			}
			return m, nil
		case "b":
			if len(m.posts) > 0 && m.selectedIndex < len(m.posts) {
				postID := m.posts[m.selectedIndex].ID
				return m, func() tea.Msg { return BookmarkPostMsg{PostID: postID} }
			}
			return m, nil
		case "d":
			if len(m.posts) > 0 && m.selectedIndex < len(m.posts) &&
				m.posts[m.selectedIndex].AuthorUsername == m.currentUsername {
				m.confirmingDelete = true
				m.viewport.Height = m.viewportHeight()
			}
			return m, nil
		case "n":
			m.titleInput.SetValue("")
			m.titleFocused = false
			m.titleInput.Blur()
			m.topicsInput.SetValue("tui")
			m.topicsFocused = false
			m.topicsInput.Blur()
			m.composePublic = m.defaultPublicPost
			m.composeNSFW = false
			var cmd tea.Cmd
			m.compose, cmd = m.compose.Open("new post", "what's on your mind…")
			m.viewport.Height = m.viewportHeight()
			return m, cmd
		case "down", "j":
			if m.selectedIndex < len(m.posts)-1 {
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

// viewportHeight returns the viewport height in rows, shrinking to make room for
// the compose box, tags input, and the delete-confirmation overlay when active.
func (m FeedModel) viewportHeight() int {
	h := m.height - theme.ChromeHeight
	if m.compose.IsActive() {
		h -= m.compose.BoxHeight()
		h -= 3 // title input: border top + content + border bottom
		h -= 3 // topics input: border top + content + border bottom
		h -= 1 // toggle line (public/nsfw)
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
	m.compose = m.compose.Close()
	m.titleFocused = false
	m.titleInput.SetValue("")
	m.titleInput.Blur()
	m.topicsFocused = false
	m.topicsInput.Blur()
	m.composeNSFW = false
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
		return prefix + theme.Subtle.Render("  no posts yet"), nil
	}
	sep := "\n"
	lineInc := 0
	if m.relaxed {
		sep = "\n\n"
		lineInc = 1
	}
	offsets := make([]int, len(m.posts))
	var out string
	currentLine := startLine
	for i, p := range m.posts {
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
	return RenderPost(p, selected, bookmarked, m.width, m.location(), m.timeDisplayFormat)
}

func (m FeedModel) View() string {
	if m.err != nil {
		return theme.Error.Render(fmt.Sprintf("feed error: %s", m.err))
	}
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

	if m.compose.IsActive() {
		titleStyle := theme.Border
		if m.titleFocused {
			titleStyle = theme.ActiveBorder
		}
		topicsStyle := theme.Border
		if m.topicsFocused {
			topicsStyle = theme.ActiveBorder
		}
		if m.width > 2 {
			titleStyle = titleStyle.Width(m.width - 2)
			topicsStyle = topicsStyle.Width(m.width - 2)
		}
		titleBox := titleStyle.Render(m.titleInput.View())
		topicsBox := topicsStyle.Render(m.topicsInput.View())
		pubMark := "[ ]"
		if m.composePublic {
			pubMark = "[x]"
		}
		nsfwMark := "[ ]"
		if m.composeNSFW {
			nsfwMark = "[x]"
		}
		toggleLine := theme.Subtle.Render("  " + pubMark + " public  " + nsfwMark + " nsfw  (alt+p / alt+s)")
		return lipgloss.JoinVertical(lipgloss.Left,
			m.viewport.View(),
			m.compose.View(),
			titleBox,
			topicsBox,
			toggleLine,
		)
	}
	return m.viewport.View()
}

// GetFocusedURLs implements URLProvider. Returns URLs from the selected post's content.
func (m FeedModel) GetFocusedURLs() []string {
	if len(m.posts) == 0 || m.selectedIndex < 0 || m.selectedIndex >= len(m.posts) {
		return nil
	}
	p := m.posts[m.selectedIndex]
	return append(extractURLs(p.Content), attachmentURLs(p.Attachments)...)
}
