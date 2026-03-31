package screens

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
)

// BackToFeedMsg is emitted when the user presses Esc to return to the feed.
type BackToFeedMsg struct{}

type PostDetailModel struct {
	post          model.Post
	replies       []model.Reply
	replyOffsets  []int // start line of each reply within the viewport content
	selectedReply int
	viewport      viewport.Model
	width         int
	ready         bool
	loading       bool
	err           error
}

func NewPostDetailModel() PostDetailModel {
	return PostDetailModel{}
}

func (m PostDetailModel) SetPost(post model.Post) PostDetailModel {
	m.post = post
	m.replies = nil
	m.replyOffsets = nil
	m.selectedReply = -1 // post itself is selected by default
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
	m.selectedReply = -1 // keep post selected after replies load
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
	content, offsets := m.buildContent()
	m.replyOffsets = offsets
	m.viewport.SetContent(content)
	return m
}

// ensureSelectedVisible scrolls the viewport the minimum amount so the
// selected item (post or reply) is fully visible.
func (m PostDetailModel) ensureSelectedVisible() PostDetailModel {
	if !m.ready {
		return m
	}
	var itemStart, itemHeight int
	if m.selectedReply == -1 {
		// Post is selected — it always starts at line 0.
		itemStart = 0
		itemHeight = lipgloss.Height(m.renderFullPost(true))
	} else {
		if len(m.replyOffsets) == 0 || m.selectedReply >= len(m.replies) {
			return m
		}
		itemStart = m.replyOffsets[m.selectedReply]
		itemHeight = lipgloss.Height(m.renderReply(m.replies[m.selectedReply], false))
	}
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
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return BackToFeedMsg{} }
		case "up", "k":
			if m.selectedReply >= 0 {
				m.selectedReply--
				m = m.refreshContent()
				m = m.ensureSelectedVisible()
			}
			return m, nil
		case "down", "j":
			if m.selectedReply < len(m.replies)-1 {
				m.selectedReply++
				m = m.refreshContent()
				m = m.ensureSelectedVisible()
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// buildContent renders the full post and all replies into a single string for
// the viewport. It returns the string and the start-line offset of each reply.
func (m PostDetailModel) buildContent() (string, []int) {
	postContent := m.renderFullPost(m.selectedReply == -1)
	repliesHeader := theme.Title.Render(fmt.Sprintf("  %d replies", len(m.replies)))

	var sb strings.Builder
	sb.WriteString(postContent)
	sb.WriteString("\n")
	sb.WriteString(repliesHeader)
	sb.WriteString("\n")

	if m.loading {
		sb.WriteString(theme.Subtle.Render("  loading replies…"))
		sb.WriteString("\n")
		return sb.String(), nil
	}
	if len(m.replies) == 0 {
		sb.WriteString(theme.Subtle.Render("  no replies yet"))
		sb.WriteString("\n")
		return sb.String(), nil
	}

	// Base line where first reply starts: post height + blank line + header height + blank line.
	baseLines := lipgloss.Height(postContent) + 1 + lipgloss.Height(repliesHeader) + 1
	offsets := make([]int, len(m.replies))
	currentLine := baseLines

	for i, r := range m.replies {
		offsets[i] = currentLine
		rendered := m.renderReply(r, i == m.selectedReply)
		sb.WriteString(rendered)
		sb.WriteString("\n")
		currentLine += lipgloss.Height(rendered) + 1
	}

	return sb.String(), offsets
}

func (m PostDetailModel) renderFullPost(selected bool) string {
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

func (m PostDetailModel) renderReply(r model.Reply, selected bool) string {
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
