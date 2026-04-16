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
	Content string
	Topics  []string
}

type FeedModel struct {
	posts              []model.Post
	postOffsets        []int // start line of each post within the viewport content
	viewport           viewport.Model
	compose            ComposeModel
	topicsInput        textinput.Model
	topicsFocused      bool
	width              int
	height            int
	selectedIndex      int
	ready              bool
	err                error
	nextCursor         string
	loading            bool
	refreshing         bool // true while re-fetching newest posts (up at top)
	exhausted          bool // true once API returned an empty cursor
	relaxed            bool           // true = blank line between posts (relaxed density)
	loc                *time.Location // timezone for timestamp display; nil = UTC
	timeDisplayFormat  string         // API setting: "datetime", "relative", "unix", "swatch"
}

func NewFeedModel() FeedModel {
	ti := textinput.New()
	ti.Placeholder = "add topics  (go, my topic, …  max 3)"
	return FeedModel{
		compose:     NewComposeModel(0),
		topicsInput: ti,
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

func (m FeedModel) SetPosts(posts []model.Post, cursor string) FeedModel {
	m.posts = posts
	m.nextCursor = cursor
	m.exhausted = cursor == ""
	m.loading = false
	m.refreshing = false
	m.selectedIndex = 0
	if m.ready {
		m = m.refreshContent()
		m.viewport.GotoTop()
	}
	return m
}

func (m FeedModel) AppendPosts(posts []model.Post, cursor string) FeedModel {
	m.posts = append(m.posts, posts...)
	m.nextCursor = cursor
	m.exhausted = cursor == ""
	m.loading = false
	if m.ready {
		m = m.refreshContent() // selectedIndex preserved; scroll position preserved
	}
	return m
}

func (m FeedModel) SetError(err error) FeedModel {
	m.err = err
	m.loading = false
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
		m = m.SetRelaxed(msg.Relaxed)
		m = m.SetLocation(msg.Loc)
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
		topics := ParseTopics(m.topicsInput.Value())
		m.compose = m.compose.Close()
		m.topicsFocused = false
		m.topicsInput.Blur()
		m.viewport.Height = m.viewportHeight()
		return m, func() tea.Msg { return SubmitNewPostMsg{Content: content, Topics: topics} }

	case ComposeCancelMsg:
		m.compose = m.compose.Close()
		m.topicsFocused = false
		m.topicsInput.Blur()
		m.viewport.Height = m.viewportHeight()
		return m, nil

	case tea.KeyMsg:
		if m.compose.IsActive() {
			switch msg.String() {
			case "tab":
				if m.topicsFocused {
					m.topicsFocused = false
					m.topicsInput.Blur()
					var cmd tea.Cmd
					m.compose, cmd = m.compose.SetFocused(true)
					return m, cmd
				}
				m.topicsFocused = true
				m.compose, _ = m.compose.SetFocused(false)
				cmd := m.topicsInput.Focus()
				return m, cmd
			case "ctrl+s":
				if m.topicsFocused {
					content := m.compose.Content()
					topics := ParseTopics(m.topicsInput.Value())
					m.compose = m.compose.Close()
					m.topicsFocused = false
					m.topicsInput.Blur()
					m.viewport.Height = m.viewportHeight()
					return m, func() tea.Msg { return SubmitNewPostMsg{Content: content, Topics: topics} }
				}
			case "esc":
				if m.topicsFocused {
					m.topicsFocused = false
					m.topicsInput.Blur()
					m.compose = m.compose.Close()
					m.viewport.Height = m.viewportHeight()
					return m, nil
				}
			}
			if m.topicsFocused {
				var cmd tea.Cmd
				m.topicsInput, cmd = m.topicsInput.Update(msg)
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
		case "n":
			m.topicsInput.SetValue("tui")
			m.topicsFocused = false
			m.topicsInput.Blur()
			var cmd tea.Cmd
			m.compose, cmd = m.compose.Open("new post", "what's on your mind…")
			m.viewport.Height = m.viewportHeight()
			return m, cmd
		case "down", "j":
			if m.selectedIndex < len(m.posts)-1 {
				m.selectedIndex++
				m = m.refreshContent()
				m = m.ensureSelectedVisible()
			} else if !m.loading && !m.exhausted && m.nextCursor != "" {
				m.loading = true
				cursor := m.nextCursor
				return m, func() tea.Msg { return LoadMoreFeedMsg{Cursor: cursor} }
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)

	if m.ready && !m.loading && !m.exhausted && m.viewport.AtBottom() && m.nextCursor != "" {
		m.loading = true
		cursor := m.nextCursor // capture before closure
		return m, tea.Batch(cmd, func() tea.Msg {
			return LoadMoreFeedMsg{Cursor: cursor}
		})
	}

	return m, cmd
}

// viewportHeight returns the viewport height in rows, shrinking to make room for
// the compose box and tags input when active.
func (m FeedModel) viewportHeight() int {
	h := m.height - theme.ChromeHeight
	if m.compose.IsActive() {
		h -= m.compose.BoxHeight()
		h -= 3 // tags input row: border top + content + border bottom
	}
	if h < 1 {
		h = 1
	}
	return h
}

// buildContent renders all posts into a single string for the viewport and
// returns the start line of each post so ensureSelectedVisible can scroll accurately.
func (m FeedModel) buildContent() (string, []int) {
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
	return RenderPost(p, selected, m.width, m.location(), m.timeDisplayFormat)
}

func (m FeedModel) View() string {
	if m.err != nil {
		return theme.Error.Render(fmt.Sprintf("feed error: %s", m.err))
	}
	if !m.ready {
		return theme.Subtle.Render("loading feed...")
	}
	if m.compose.IsActive() {
		topicsStyle := theme.Border
		if m.topicsFocused {
			topicsStyle = theme.ActiveBorder
		}
		if m.width > 2 {
			topicsStyle = topicsStyle.Width(m.width - 2)
		}
		topicsBox := topicsStyle.Render(m.topicsInput.View())
		return lipgloss.JoinVertical(lipgloss.Left,
			m.viewport.View(),
			m.compose.View(),
			topicsBox,
		)
	}
	return m.viewport.View()
}
