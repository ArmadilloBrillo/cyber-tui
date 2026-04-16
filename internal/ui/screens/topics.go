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

// Message types emitted by Topics screen to App
type RefreshTopicsMsg struct{}

type LoadTopicPostsMsg struct{ Slug string }

type LoadMoreTopicPostsMsg struct {
	Slug   string
	Cursor string
}

type RefreshTopicPostsMsg struct{ Slug string }

type ShowTopicPostMsg struct{ Post model.Post }

// Internal view state for the Topics screen
type topicsView int

const (
	viewTopicList  topicsView = iota
	viewTopicPosts
)

type TopicsModel struct {
	view topicsView

	// Topic list state
	topics      []model.Topic
	topicIndex  int

	// Topic posts state
	activeTopic string
	posts       []model.Post
	postIndex   int
	nextCursor  string
	exhausted   bool
	loading     bool
	refreshing  bool

	// Shared
	viewport    viewport.Model
	itemOffsets []int
	width       int
	height      int
	selectedIndex int
	ready       bool
	err         error
	loc         *time.Location
	relaxed     bool
}

func NewTopicsModel() TopicsModel {
	return TopicsModel{}
}

func (m TopicsModel) SetTopics(items []model.Topic) TopicsModel {
	m.topics = items
	m.topicIndex = 0
	m.loading = false
	m.refreshing = false
	if m.ready {
		m = m.refreshContent()
		m.viewport.GotoTop()
	}
	return m
}

func (m TopicsModel) SetTopicPosts(posts []model.Post, cursor string) TopicsModel {
	m.posts = posts
	m.postIndex = 0
	m.nextCursor = cursor
	m.exhausted = cursor == ""
	m.loading = false
	m.refreshing = false
	m.view = viewTopicPosts
	if m.ready {
		m = m.refreshContent()
		m.viewport.GotoTop()
	}
	return m
}

func (m TopicsModel) AppendTopicPosts(posts []model.Post, cursor string) TopicsModel {
	m.posts = append(m.posts, posts...)
	m.nextCursor = cursor
	m.exhausted = cursor == ""
	m.loading = false
	if m.ready {
		m = m.refreshContent()
	}
	return m
}

func (m TopicsModel) SetError(err error) TopicsModel {
	m.err = err
	m.loading = false
	m.refreshing = false
	return m
}

func (m TopicsModel) Init() tea.Cmd { return nil }

func (m TopicsModel) Update(msg tea.Msg) (TopicsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case SharedConfigMsg:
		m.relaxed = msg.Relaxed
		if msg.Loc != nil {
			m.loc = msg.Loc
		}
		if m.ready {
			m = m.refreshContent()
		}
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
		switch msg.String() {
		case "up", "k":
			if m.view == viewTopicList {
				if m.topicIndex > 0 {
					m.topicIndex--
					m = m.ensureSelectedVisible()
				} else {
					return m, func() tea.Msg { return RefreshTopicsMsg{} }
				}
			} else {
				if m.postIndex > 0 {
					m.postIndex--
					m = m.ensureSelectedVisible()
				} else {
					return m, func() tea.Msg { return RefreshTopicPostsMsg{Slug: m.activeTopic} }
				}
			}
			return m, nil

		case "down", "j":
			if m.view == viewTopicList {
				if m.topicIndex < len(m.topics)-1 {
					m.topicIndex++
					m = m.ensureSelectedVisible()
				}
			} else {
				if m.postIndex < len(m.posts)-1 {
					m.postIndex++
					m = m.ensureSelectedVisible()
				} else if !m.exhausted && !m.loading {
					m.loading = true
					return m, func() tea.Msg {
						return LoadMoreTopicPostsMsg{Slug: m.activeTopic, Cursor: m.nextCursor}
					}
				}
			}
			return m, nil

		case "enter":
			if m.view == viewTopicList {
				if len(m.topics) > 0 && m.topicIndex < len(m.topics) {
					slug := m.topics[m.topicIndex].Slug
					m.activeTopic = slug
					return m, func() tea.Msg { return LoadTopicPostsMsg{Slug: slug} }
				}
			} else {
				if len(m.posts) > 0 && m.postIndex < len(m.posts) {
					post := m.posts[m.postIndex]
					return m, func() tea.Msg { return ShowTopicPostMsg{Post: post} }
				}
			}
			return m, nil

		case "esc":
			if m.view == viewTopicPosts {
				m.view = viewTopicList
				m = m.refreshContent()
				m.viewport.GotoTop()
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)

	// Check if user scrolled to bottom
	if m.view == viewTopicPosts && m.viewport.AtBottom() && !m.exhausted && !m.loading {
		m.loading = true
		return m, func() tea.Msg {
			return LoadMoreTopicPostsMsg{Slug: m.activeTopic, Cursor: m.nextCursor}
		}
	}

	return m, cmd
}

func (m TopicsModel) View() string {
	if !m.ready {
		return ""
	}

	var header string
	if m.view == viewTopicList {
		header = "  topics  "
	} else {
		header = fmt.Sprintf("  %s (%d) ", m.activeTopic, len(m.posts))
	}

	statusBar := theme.Subtle.Render(fmt.Sprintf("%-*s", m.width, header))

	content := m.viewport.View()

	footer := m.renderFooter()

	return lipgloss.JoinVertical(lipgloss.Left, statusBar, content, footer)
}

func (m TopicsModel) renderFooter() string {
	if m.err != nil {
		return theme.Error.Render(fmt.Sprintf("Error: %v", m.err))
	}

	var status string
	if m.loading {
		status = "loading…"
	} else if m.refreshing {
		status = "refreshing…"
	} else if m.view == viewTopicPosts && m.exhausted {
		status = "— end —"
	} else if len(m.posts) == 0 && m.view == viewTopicPosts {
		status = "no posts"
	} else {
		status = ""
	}

	return theme.Subtle.Render(fmt.Sprintf("%-*s", m.width, status))
}

func (m TopicsModel) buildContent() string {
	var lines []string
	m.itemOffsets = []int{0}

	if m.view == viewTopicList {
		for i := range m.topics {
			line := m.renderTopicItem(i)
			lines = append(lines, line)
			if i < len(m.topics)-1 {
				m.itemOffsets = append(m.itemOffsets, len(lines))
			}
		}
	} else {
		for i := range m.posts {
			line := m.renderPostItem(i)
			lines = append(lines, line)
			if i < len(m.posts)-1 {
				m.itemOffsets = append(m.itemOffsets, len(lines))
			}
		}
	}

	if len(lines) == 0 {
		return ""
	}

	return strings.Join(lines, "\n")
}

func (m TopicsModel) renderTopicItem(index int) string {
	if index < 0 || index >= len(m.topics) {
		return ""
	}

	topic := m.topics[index]
	isSelected := (m.view == viewTopicList && index == m.topicIndex)

	content := fmt.Sprintf("%-*s %6d posts",
		m.width-20,
		truncateStr(topic.Slug, m.width-20),
		topic.PostCount)

	if isSelected {
		return theme.Highlight.Render("▸ " + content)
	}
	return "  " + content
}

func (m TopicsModel) renderPostItem(index int) string {
	if index < 0 || index >= len(m.posts) {
		return ""
	}

	post := m.posts[index]
	isSelected := (m.view == viewTopicPosts && index == m.postIndex)

	// Format: ▸ @author: preview...
	author := post.AuthorUsername
	preview := strings.TrimSpace(post.Content)
	if len(preview) > 50 {
		preview = preview[:47] + "..."
	}

	content := fmt.Sprintf("@%-*s %s",
		15,
		truncateStr(author, 15),
		truncateStr(preview, m.width-25))

	if isSelected {
		return theme.Highlight.Render("▸ " + content)
	}
	return "  " + content
}

func (m TopicsModel) refreshContent() TopicsModel {
	content := m.buildContent()
	m.viewport.SetContent(content)
	return m.ensureSelectedVisible()
}

func (m TopicsModel) ensureSelectedVisible() TopicsModel {
	var selectedIndex int
	if m.view == viewTopicList {
		selectedIndex = m.topicIndex
	} else {
		selectedIndex = m.postIndex
	}

	if selectedIndex >= len(m.itemOffsets) {
		return m
	}

	itemStart := m.itemOffsets[selectedIndex]
	itemEnd := itemStart + 1
	if selectedIndex < len(m.itemOffsets)-1 {
		itemEnd = m.itemOffsets[selectedIndex+1]
	}

	vpTop := m.viewport.YOffset
	vpBottom := vpTop + m.viewport.Height

	if itemStart < vpTop {
		m.viewport.SetYOffset(itemStart)
	} else if itemEnd > vpBottom {
		m.viewport.SetYOffset(itemEnd - m.viewport.Height)
	}

	return m
}

func (m TopicsModel) viewportHeight() int {
	return m.height - theme.ChromeHeight
}

// --- Helpers ---

func truncateStr(s string, maxWidth int) string {
	if len(s) <= maxWidth {
		return s
	}
	return s[:maxWidth]
}
