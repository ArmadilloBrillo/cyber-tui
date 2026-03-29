package screens

import (
	"fmt"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
)

type FeedModel struct {
	posts    []model.Post
	viewport viewport.Model
	ready    bool
	err      error
}

func NewFeedModel() FeedModel {
	return FeedModel{}
}

func (m FeedModel) SetPosts(posts []model.Post) FeedModel {
	m.posts = posts
	if m.ready {
		m.viewport.SetContent(m.renderPosts())
	}
	return m
}

func (m FeedModel) SetError(err error) FeedModel {
	m.err = err
	return m
}

func (m FeedModel) Init() tea.Cmd { return nil }

func (m FeedModel) Update(msg tea.Msg) (FeedModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if !m.ready {
			m.viewport = viewport.New(msg.Width, msg.Height-4)
			m.viewport.SetContent(m.renderPosts())
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - 4
		}
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m FeedModel) renderPosts() string {
	if len(m.posts) == 0 {
		return theme.Subtle.Render("  no posts yet")
	}
	var out string
	for _, p := range m.posts {
		out += renderPost(p) + "\n"
	}
	return out
}

func renderPost(p model.Post) string {
	header := lipgloss.JoinHorizontal(lipgloss.Top,
		theme.Highlight.Render("@"+p.Author.Username),
		theme.Subtle.Render("  "+p.CreatedAt.Format("15:04")),
	)
	body := theme.Base.Render(p.Body)

	topics := ""
	for _, t := range p.Topics {
		topics += theme.Subtle.Render("#"+t) + " "
	}

	return theme.Border.Render(
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
