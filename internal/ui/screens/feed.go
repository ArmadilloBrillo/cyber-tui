package screens

import (
	"fmt"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
)

// LoadMoreFeedMsg is emitted by FeedModel when the viewport reaches the bottom
// and a next-page cursor is available. App intercepts this and fires the API call.
type LoadMoreFeedMsg struct{ Cursor string }

type FeedModel struct {
	posts      []model.Post
	viewport   viewport.Model
	width      int
	ready      bool
	err        error
	nextCursor string
	loading    bool
	exhausted  bool // true once API returned an empty cursor
}

func NewFeedModel() FeedModel {
	return FeedModel{}
}

func (m FeedModel) SetPosts(posts []model.Post, cursor string) FeedModel {
	m.posts = posts
	m.nextCursor = cursor
	m.exhausted = cursor == ""
	m.loading = false
	if m.ready {
		m.viewport.SetContent(m.renderPosts())
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
		m.viewport.SetContent(m.renderPosts())
		// No GotoTop — preserve scroll position
	}
	return m
}

func (m FeedModel) SetError(err error) FeedModel {
	m.err = err
	m.loading = false
	return m
}

func (m FeedModel) Init() tea.Cmd { return nil }

func (m FeedModel) Update(msg tea.Msg) (FeedModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		if !m.ready {
			m.viewport = viewport.New(msg.Width, msg.Height-theme.ChromeHeight)
			m.viewport.SetContent(m.renderPosts())
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - theme.ChromeHeight
			m.viewport.SetContent(m.renderPosts())
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

func (m FeedModel) renderPosts() string {
	if len(m.posts) == 0 {
		return theme.Subtle.Render("  no posts yet")
	}
	var out string
	for _, p := range m.posts {
		out += m.renderPost(p) + "\n"
	}
	if m.loading {
		out += theme.Subtle.Render("  loading more…") + "\n"
	} else if m.exhausted {
		out += theme.Subtle.Render("  — end of feed —") + "\n"
	}
	return out
}

func (m FeedModel) renderPost(p model.Post) string {
	// Border has Padding(0,1) = 1 char each side, plus 1 char border each side = 4 total.
	// innerWidth is the usable text area; 0 means not yet initialised (skip wrapping).
	innerWidth := m.width - 4

	header := lipgloss.JoinHorizontal(lipgloss.Top,
		theme.Highlight.Render("@"+p.AuthorUsername),
		theme.Subtle.Render("  "+p.CreatedAt.Format("15:04:05")),
	)

	var body string
	if innerWidth > 0 {
		body = theme.Base.Width(innerWidth).Render(p.Content)
	} else {
		body = theme.Base.Render(p.Content)
	}

	topics := ""
	for _, t := range p.Topics {
		topics += theme.Subtle.Render("#"+t) + " "
	}

	boxStyle := theme.Border
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
	return m.viewport.View()
}
