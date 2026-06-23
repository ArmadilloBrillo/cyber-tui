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

type LoadMoreTopicsMsg struct{ Cursor string }

// Internal view state for the Topics screen
type topicsView int

const (
	viewTopicList topicsView = iota
	viewTopicPosts
)

type TopicsModel struct {
	view topicsView

	// Topic list state
	topics           []model.Topic
	topicIndex       int
	topicsNextCursor string
	topicsExhausted  bool

	// Topic posts state
	activeTopic string
	posts       []model.Post
	postIndex   int
	nextCursor  string
	exhausted   bool
	loading     bool
	fetching    bool // true while the initial (or tab-switch) load is in flight
	refreshing  bool
	loaded      bool

	// Shared
	viewport    viewport.Model
	itemOffsets []int
	width       int

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

func NewTopicsModel() TopicsModel {
	return TopicsModel{}
}

func (m TopicsModel) visiblePosts() []model.Post {
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

func (m TopicsModel) IsLoaded() bool { return m.loaded }

func (m TopicsModel) SetFetching() TopicsModel {
	m.fetching = true
	m.err = nil
	if m.ready {
		m = m.refreshContent()
	}
	return m
}

func (m TopicsModel) SetTopics(items []model.Topic, cursor string) TopicsModel {
	m.err = nil
	m.topics = items
	m.topicIndex = 0
	m.topicsNextCursor = cursor
	m.topicsExhausted = cursor == ""
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

func (m TopicsModel) AppendTopics(items []model.Topic, cursor string) TopicsModel {
	m.err = nil
	m.topics = append(m.topics, items...)
	m.topicsNextCursor = cursor
	m.topicsExhausted = cursor == ""
	m.loading = false
	m.fetching = false
	if m.ready {
		m = m.refreshContent()
	}
	return m
}

func (m TopicsModel) SetTopicPosts(posts []model.Post, cursor string) TopicsModel {
	m.err = nil
	m.posts = posts
	m.postIndex = 0
	m.nextCursor = cursor
	m.exhausted = cursor == ""
	m.loading = false
	m.fetching = false
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
	m.fetching = false
	if m.ready {
		m = m.refreshContent()
	}
	return m
}

func (m TopicsModel) SetError(err error) TopicsModel {
	m.err = err
	m.loading = false
	m.fetching = false
	m.refreshing = false
	if m.ready {
		m = m.refreshContent()
	}
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
		if msg.Settings.FilterNSFW != m.filterNSFW {
			m.filterNSFW = msg.Settings.FilterNSFW
			m.postIndex = 0
		}
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

	case WatchedPostIDsMsg:
		m.watchedPostIDs = msg.PostIDs
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
					m = m.refreshContent()
					m = m.ensureSelectedVisible()
				}
			} else {
				if m.postIndex > 0 {
					m.postIndex--
					m = m.refreshContent()
					m = m.ensureSelectedVisible()
				}
			}
			return m, nil

		case "down", "j":
			if m.view == viewTopicList {
				if m.topicIndex < len(m.topics)-1 {
					m.topicIndex++
					m = m.refreshContent()
					m = m.ensureSelectedVisible()
				} else if !m.topicsExhausted && !m.loading {
					m.loading = true
					m = m.refreshContent()
					m.viewport.ScrollDown(1)
					return m, func() tea.Msg {
						return LoadMoreTopicsMsg{Cursor: m.topicsNextCursor}
					}
				}
			} else {
				if m.postIndex < len(m.visiblePosts())-1 {
					m.postIndex++
					m = m.refreshContent()
					m = m.ensureSelectedVisible()
				} else if !m.exhausted && !m.loading {
					m.loading = true
					m = m.refreshContent()
					m.viewport.ScrollDown(1)
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
				if visible := m.visiblePosts(); len(visible) > 0 && m.postIndex < len(visible) {
					post := visible[m.postIndex]
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
		m = m.refreshContent()
		m.viewport.ScrollDown(1)
		return m, func() tea.Msg {
			return LoadMoreTopicPostsMsg{Slug: m.activeTopic, Cursor: m.nextCursor}
		}
	}

	return m, cmd
}

func (m TopicsModel) View() string {
	if !m.ready {
		return theme.Subtle.Render("loading topics...")
	}
	return m.viewport.View()
}

func (m TopicsModel) buildContent() (string, []int) {
	if m.fetching {
		return theme.Subtle.Render("  loading topics…"), nil
	}
	sep := "\n"
	lineInc := 1
	if m.relaxed {
		sep = "\n\n"
		lineInc = 2
	}

	// State indicators
	var prefix string
	startLine := 0
	if m.refreshing {
		prefix = theme.Subtle.Render("  refreshing…") + "\n"
		startLine++
	}

	if m.view == viewTopicList {
		if len(m.topics) == 0 {
			if m.err != nil {
				return prefix + theme.Subtle.Render("  couldn't load topics"), nil
			}
			return prefix + theme.Subtle.Render("  no topics yet"), nil
		}
		offsets := make([]int, len(m.topics))
		currentLine := startLine
		var out string
		for i := range m.topics {
			offsets[i] = currentLine
			rendered := m.renderTopicItem(i)
			out += rendered + sep
			currentLine += lipgloss.Height(rendered) + lineInc - 1
		}
		// Footer
		out += listFooter(m.loading, m.topicsExhausted && len(m.topics) > 0)
		return prefix + strings.TrimRight(out, "\n"), offsets
	}

	// viewTopicPosts
	if len(m.posts) == 0 {
		if m.err != nil {
			return prefix + theme.Subtle.Render("  couldn't load posts"), nil
		}
		return prefix + theme.Subtle.Render("  no posts"), nil
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
	// Footer
	out += listFooter(m.loading, m.exhausted)
	return prefix + strings.TrimRight(out, "\n"), offsets
}

func (m TopicsModel) renderTopicItem(index int) string {
	if index < 0 || index >= len(m.topics) {
		return ""
	}

	topic := m.topics[index]
	isSelected := (m.view == viewTopicList && index == m.topicIndex)

	innerWidth := m.width - 4

	icon := theme.Subtle.Render("#") + " "
	slugStyle := theme.Base
	if isSelected {
		slugStyle = theme.Highlight
	}
	slugStr := slugStyle.Render(topic.Slug)
	countStr := theme.Subtle.Render(fmt.Sprintf("%d posts", topic.PostCount))

	var line string
	if innerWidth > 0 {
		gap := innerWidth - lipgloss.Width(icon) - lipgloss.Width(slugStr) - lipgloss.Width(countStr)
		if gap > 0 {
			line = icon + slugStr + strings.Repeat(" ", gap) + countStr
		} else {
			line = icon + slugStr
		}
	} else {
		line = icon + slugStr
	}

	boxStyle := theme.Border
	if isSelected {
		boxStyle = theme.ActiveBorder
	}
	if innerWidth > 0 {
		boxStyle = boxStyle.Width(m.width - 2)
	}
	return boxStyle.Render(line)
}

func (m TopicsModel) renderPostItem(p model.Post, selected bool) string {
	_, bookmarked := m.bookmarkedPostIDs[p.ID]
	_, watched := m.watchedPostIDs[p.ID]
	return RenderPost(p, selected, bookmarked, watched, m.width, m.location(), m.timeDisplayFormat, postMaxBodyLines)
}

func (m TopicsModel) refreshContent() TopicsModel {
	content, offsets := m.buildContent()
	m.itemOffsets = offsets
	m.viewport.SetContent(content)
	return m.ensureSelectedVisible()
}

func (m TopicsModel) ensureSelectedVisible() TopicsModel {
	if !m.ready || len(m.itemOffsets) == 0 {
		return m
	}

	var selectedIndex int
	var itemHeight int
	if m.view == viewTopicList {
		selectedIndex = m.topicIndex
		if selectedIndex >= len(m.topics) {
			return m
		}
		itemHeight = lipgloss.Height(m.renderTopicItem(selectedIndex))
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

func (m TopicsModel) viewportHeight() int {
	return m.height - theme.ChromeHeight
}

func (m TopicsModel) location() *time.Location {
	if m.loc == nil {
		return time.UTC
	}
	return m.loc
}

// IsBrowsingTopic reports whether the user is viewing a specific topic's posts.
func (m TopicsModel) IsBrowsingTopic() bool { return m.activeTopic != "" }

// GetFocusedURLs implements URLProvider. Returns URLs from the selected post when
// in post-list view; returns nil when browsing the topic list.
func (m TopicsModel) GetFocusedURLs() []string {
	if m.view != viewTopicPosts {
		return nil
	}
	visible := m.visiblePosts()
	if m.postIndex < 0 || m.postIndex >= len(visible) {
		return nil
	}
	p := visible[m.postIndex]
	return append(extractURLs(p.Content), attachmentURLs(p.Attachments)...)
}

// --- Helpers ---
// truncate is defined in cmail.go; using same pattern here
// Rather than import it, we inline a simple implementation in renderTopicItem
