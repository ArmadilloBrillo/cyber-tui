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

const feedMaxBodyLines = 4

// LoadMoreFeedMsg is emitted by FeedModel when the viewport reaches the bottom
// and a next-page cursor is available. App intercepts this and fires the API call.
type LoadMoreFeedMsg struct{ Cursor string }

// ShowPostMsg is emitted when the user presses Enter on a selected post.
type ShowPostMsg struct{ Post model.Post }

// ShowPostForReplyMsg is emitted when the user presses 'r' on a selected post.
// App navigates to post detail and opens the compose box immediately.
type ShowPostForReplyMsg struct{ Post model.Post }

// SubmitNewPostMsg is emitted when the user submits a new post from the Feed.
type SubmitNewPostMsg struct{ Content string }

type FeedModel struct {
	posts         []model.Post
	postOffsets   []int // start line of each post within the viewport content
	viewport      viewport.Model
	compose       ComposeModel
	width         int
	height        int
	selectedIndex int
	ready         bool
	err           error
	nextCursor    string
	loading       bool
	exhausted     bool // true once API returned an empty cursor
	relaxed       bool             // true = blank line between posts (relaxed density)
	loc           *time.Location   // timezone for timestamp display; nil = UTC
}

func NewFeedModel() FeedModel {
	return FeedModel{
		compose: NewComposeModel(0),
	}
}

func (m FeedModel) SetPosts(posts []model.Post, cursor string) FeedModel {
	m.posts = posts
	m.nextCursor = cursor
	m.exhausted = cursor == ""
	m.loading = false
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
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.compose = m.compose.SetWidth(msg.Width)
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
		m.compose = m.compose.Close()
		m.viewport.Height = m.viewportHeight()
		return m, func() tea.Msg { return SubmitNewPostMsg{Content: content} }

	case ComposeCancelMsg:
		m.compose = m.compose.Close()
		m.viewport.Height = m.viewportHeight()
		return m, nil

	case tea.KeyMsg:
		if m.compose.IsActive() {
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
		case "n":
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
// the compose box when it is active.
func (m FeedModel) viewportHeight() int {
	h := m.height - theme.ChromeHeight
	if m.compose.IsActive() {
		h -= m.compose.BoxHeight()
	}
	if h < 1 {
		h = 1
	}
	return h
}

// buildContent renders all posts into a single string for the viewport and
// returns the start line of each post so ensureSelectedVisible can scroll accurately.
func (m FeedModel) buildContent() (string, []int) {
	if len(m.posts) == 0 {
		return theme.Subtle.Render("  no posts yet"), nil
	}
	sep := "\n"
	lineInc := 0
	if m.relaxed {
		sep = "\n\n"
		lineInc = 1
	}
	offsets := make([]int, len(m.posts))
	var out string
	currentLine := 0
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
	return out, offsets
}

func (m FeedModel) renderPost(p model.Post, selected bool) string {
	// Border has Padding(0,1) = 1 char each side, plus 1 char border each side = 4 total.
	// innerWidth is the usable text area; 0 means not yet initialised (skip wrapping).
	innerWidth := m.width - 4

	left := lipgloss.JoinHorizontal(lipgloss.Top,
		theme.Highlight.Render("@"+p.AuthorUsername),
		theme.Subtle.Render("  "+formatTime(p.CreatedAt, m.location(), "15:04:05")),
	)
	replies := theme.Subtle.Render(fmt.Sprintf("%d replies", p.RepliesCount))
	var header string
	if innerWidth > 0 {
		gap := innerWidth - lipgloss.Width(left) - lipgloss.Width(replies)
		if gap > 0 {
			header = left + strings.Repeat(" ", gap) + replies
		} else {
			header = left
		}
	} else {
		header = left
	}

	var body string
	if innerWidth > 0 {
		wrapped := theme.Base.Width(innerWidth).Render(p.Content)
		lines := strings.Split(wrapped, "\n")
		if len(lines) > feedMaxBodyLines {
			body = strings.Join(lines[:feedMaxBodyLines], "\n")
			more := len(lines) - feedMaxBodyLines
			body += "\n" + theme.Subtle.Render(fmt.Sprintf("  ▼ %d more lines", more))
		} else {
			body = wrapped
		}
	} else {
		body = theme.Base.Render(p.Content)
	}

	topics := ""
	for _, t := range p.Topics {
		topics += theme.Subtle.Render("#"+t) + " "
	}

	boxStyle := theme.Border
	if selected {
		boxStyle = theme.ActiveBorder
	}
	if innerWidth > 0 {
		// Width on a lipgloss style sets the content+padding area (border excluded).
		boxStyle = boxStyle.Width(m.width - 2)
	}
	return boxStyle.Render(
		lipgloss.JoinVertical(lipgloss.Left,
			header,
			body,
			fmt.Sprintf("\n%s", topics),
		),
	)
}

func (m FeedModel) View() string {
	if m.err != nil {
		return theme.Error.Render(fmt.Sprintf("feed error: %s", m.err))
	}
	if !m.ready {
		return theme.Subtle.Render("loading feed...")
	}
	if m.compose.IsActive() {
		return lipgloss.JoinVertical(lipgloss.Left, m.viewport.View(), m.compose.View())
	}
	return m.viewport.View()
}
