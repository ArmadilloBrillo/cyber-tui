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
	activeTopic  string
	posts        []model.Post
	postIndex    int
	nextCursor   string
	exhausted    bool
	loading      bool
	refreshing   bool

	// Shared
	viewport         viewport.Model
	itemOffsets      []int
	width            int
	height           int
	selectedIndex    int
	ready            bool
	err              error
	loc              *time.Location
	relaxed          bool
	timeDisplayFormat string
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
		m.timeDisplayFormat = msg.Settings.TimeDisplayFormat
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
	if m.err != nil {
		return theme.Error.Render(fmt.Sprintf("topics error: %s", m.err))
	}
	if !m.ready {
		return theme.Subtle.Render("loading topics...")
	}
	return m.viewport.View()
}

func (m TopicsModel) buildContent() (string, []int) {
	var lines []string
	var offsets []int

	// Add header line
	if m.view == viewTopicList {
		lines = append(lines, theme.Subtle.Render("  topics"))
		offsets = append(offsets, 0)
	} else {
		lines = append(lines, theme.Subtle.Render(fmt.Sprintf("  ← %s", m.activeTopic)))
		offsets = append(offsets, 0)
	}

	sep := ""
	if m.relaxed {
		sep = "\n"
	}

	if m.view == viewTopicList {
		for i := range m.topics {
			if i > 0 {
				lines = append(lines, sep)
				offsets = append(offsets, len(lines))
			}
			line := m.renderTopicItem(i)
			lines = append(lines, line)
			if i < len(m.topics)-1 {
				offsets = append(offsets, len(lines))
			}
		}
	} else {
		for i := range m.posts {
			if i > 0 {
				lines = append(lines, sep)
				offsets = append(offsets, len(lines))
			}
			rendered := m.renderPostItem(m.posts[i], i == m.postIndex)
			for _, l := range strings.Split(rendered, "\n") {
				lines = append(lines, l)
			}
			if i < len(m.posts)-1 {
				offsets = append(offsets, len(lines))
			}
		}
	}

	// Add footer
	if m.loading {
		lines = append(lines, theme.Subtle.Render("  loading…"))
	} else if m.refreshing {
		lines = append(lines, theme.Subtle.Render("  refreshing…"))
	} else if m.view == viewTopicPosts && m.exhausted && len(m.posts) > 0 {
		lines = append(lines, theme.Subtle.Render("  — end —"))
	} else if len(lines) == 1 {
		// Only header, no items
		if m.view == viewTopicList {
			lines = append(lines, theme.Subtle.Render("  no topics"))
		} else {
			lines = append(lines, theme.Subtle.Render("  no posts"))
		}
	}

	return strings.Join(lines, "\n"), offsets
}

func (m TopicsModel) renderTopicItem(index int) string {
	if index < 0 || index >= len(m.topics) {
		return ""
	}

	topic := m.topics[index]
	isSelected := (m.view == viewTopicList && index == m.topicIndex)

	slug := topic.Slug
	maxWidth := m.width - 20
	if len(slug) > maxWidth {
		slug = slug[:maxWidth]
	}

	content := fmt.Sprintf("%-*s %6d posts", maxWidth, slug, topic.PostCount)

	if isSelected {
		return theme.Highlight.Render("▸ " + content)
	}
	return "  " + content
}

func (m TopicsModel) renderPostItem(p model.Post, selected bool) string {
	// Adapted from feed.go renderPost
	innerWidth := m.width - 4

	left := lipgloss.JoinHorizontal(lipgloss.Top,
		theme.Highlight.Render("@"+p.AuthorUsername),
		theme.Subtle.Render("  "+displayTime(p.CreatedAt, m.location(), m.timeDisplayFormat, false)),
	)
	var repliesLabel string
	switch p.RepliesCount {
	case 0:
		// show nothing
	case 1:
		repliesLabel = theme.Subtle.Render("1 reply")
	default:
		repliesLabel = theme.Subtle.Render(fmt.Sprintf("%d replies", p.RepliesCount))
	}
	var header string
	if innerWidth > 0 {
		gap := innerWidth - lipgloss.Width(left) - lipgloss.Width(repliesLabel)
		if gap > 0 {
			header = left + strings.Repeat(" ", gap) + repliesLabel
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

func (m TopicsModel) refreshContent() TopicsModel {
	content, offsets := m.buildContent()
	m.itemOffsets = offsets
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

func (m TopicsModel) location() *time.Location {
	if m.loc == nil {
		return time.UTC
	}
	return m.loc
}

// --- Helpers ---
// truncate is defined in cmail.go; using same pattern here
// Rather than import it, we inline a simple implementation in renderTopicItem
