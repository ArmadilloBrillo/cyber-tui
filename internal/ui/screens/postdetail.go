package screens

import (
	"fmt"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
)

// BackToFeedMsg is emitted when the user presses Esc to return to the feed.
type BackToFeedMsg struct{}

type PostDetailModel struct {
	post     model.Post
	replies  []model.Reply
	viewport viewport.Model
	width    int
	ready    bool
	loading  bool
	err      error
}

func NewPostDetailModel() PostDetailModel {
	return PostDetailModel{}
}

func (m PostDetailModel) SetPost(post model.Post) PostDetailModel {
	m.post = post
	m.replies = nil
	m.loading = true
	m.err = nil
	if m.ready {
		m = m.refreshContent()
		m.viewport.GotoTop()
	}
	return m
}

func (m PostDetailModel) SetReplies(replies []model.Reply) PostDetailModel {
	m.replies = replies
	m.loading = false
	if m.ready {
		m = m.refreshContent()
	}
	return m
}

// Loading reports whether replies are still being fetched.
func (m PostDetailModel) Loading() bool { return m.loading }

// Ready reports whether the viewport has been initialised (i.e. a WindowSizeMsg was received).
func (m PostDetailModel) Ready() bool { return m.ready }

func (m PostDetailModel) SetError(err error) PostDetailModel {
	m.err = err
	m.loading = false
	return m
}

func (m PostDetailModel) refreshContent() PostDetailModel {
	m.viewport.SetContent(m.buildContent())
	return m
}

func (m PostDetailModel) Init() tea.Cmd { return nil }

func (m PostDetailModel) Update(msg tea.Msg) (PostDetailModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		if !m.ready {
			m.viewport = viewport.New(msg.Width, msg.Height-theme.ChromeHeight)
			m = m.refreshContent()
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - theme.ChromeHeight
			m = m.refreshContent()
		}
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "esc" {
			return m, func() tea.Msg { return BackToFeedMsg{} }
		}
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m PostDetailModel) buildContent() string {
	// Full post — no truncation, ActiveBorder, full width
	postContent := m.renderFullPost()

	repliesHeader := theme.Title.Render(fmt.Sprintf("  %d replies", len(m.replies)))

	var repliesSection string
	if m.loading {
		repliesSection = theme.Subtle.Render("  loading replies…")
	} else if len(m.replies) == 0 {
		repliesSection = theme.Subtle.Render("  no replies yet")
	} else {
		for _, r := range m.replies {
			repliesSection += m.renderReply(r) + "\n"
		}
	}

	return postContent + "\n" + repliesHeader + "\n" + repliesSection
}

func (m PostDetailModel) renderFullPost() string {
	innerWidth := m.width - 4

	header := lipgloss.JoinHorizontal(lipgloss.Top,
		theme.Highlight.Render("@"+m.post.AuthorUsername),
		theme.Subtle.Render("  "+m.post.CreatedAt.Format("15:04:05")),
	)

	var body string
	if innerWidth > 0 {
		body = theme.Base.Width(innerWidth).Render(m.post.Content)
	} else {
		body = theme.Base.Render(m.post.Content)
	}

	topics := ""
	for _, t := range m.post.Topics {
		topics += theme.Subtle.Render("#"+t) + " "
	}

	boxStyle := theme.ActiveBorder
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

func (m PostDetailModel) renderReply(r model.Reply) string {
	innerWidth := m.width - 4

	header := lipgloss.JoinHorizontal(lipgloss.Top,
		theme.Highlight.Render("@"+r.AuthorUsername),
		theme.Subtle.Render("  "+r.CreatedAt.Format("15:04:05")),
	)

	var body string
	if innerWidth > 0 {
		body = theme.Base.Width(innerWidth).Render(r.Content)
	} else {
		body = theme.Base.Render(r.Content)
	}

	boxStyle := theme.Border
	if innerWidth > 0 {
		boxStyle = boxStyle.Width(m.width - 2)
	}
	return boxStyle.Render(
		lipgloss.JoinVertical(lipgloss.Left,
			header,
			body,
		),
	)
}

func (m PostDetailModel) View() string {
	if m.err != nil {
		return theme.Error.Render(fmt.Sprintf("error: %s", m.err))
	}
	if !m.ready {
		return theme.Subtle.Render("loading…")
	}
	return m.viewport.View()
}
