package screens

import (
	"fmt"
	"strings"
	"time"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/markdown"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
)

// LoadMoreBookmarksMsg is emitted when the user scrolls to the last item
// and more pages are available.
type LoadMoreBookmarksMsg struct{ Cursor string }

// DeleteBookmarkMsg is emitted when the user presses 'd' on the selected bookmark.
// PostID or ReplyID identifies the content so App can update its bookmarked-IDs set.
type DeleteBookmarkMsg struct {
	BookmarkID string
	PostID     string
	ReplyID    string
}

// BookmarkPostMsg is emitted from Feed or PostDetail when the user presses 'b'
// to toggle a bookmark. Exactly one of PostID or ReplyID is set.
type BookmarkPostMsg struct {
	PostID  string
	ReplyID string
}

// OpenBookmarkMsg is emitted when the user presses Enter on a bookmark that has
// no embedded post/reply content. App fetches the item by ID then navigates to PostDetail.
// Exactly one of PostID or ReplyID is set.
type OpenBookmarkMsg struct {
	PostID  string
	ReplyID string
}

type BookmarksModel struct {
	items         []model.Bookmark
	itemOffsets   []int
	viewport      viewport.Model
	width         int
	height        int
	selectedIndex int
	ready         bool
	loaded        bool
	loading       bool
	fetching      bool
	exhausted     bool
	nextCursor    string
	err           error
	loc           *time.Location
	relaxed       bool
}

func NewBookmarksModel() BookmarksModel {
	return BookmarksModel{}
}

func (m BookmarksModel) IsLoaded() bool { return m.loaded }

func (m BookmarksModel) SetFetching() BookmarksModel {
	m.fetching = true
	if m.ready {
		m = m.refreshContent()
	}
	return m
}

func (m BookmarksModel) SetBookmarks(items []model.Bookmark, cursor string) BookmarksModel {
	m.items = items
	m.nextCursor = cursor
	m.exhausted = cursor == ""
	m.loading = false
	m.fetching = false
	m.loaded = true
	m.selectedIndex = 0
	if m.ready {
		m = m.refreshContent()
		m.viewport.GotoTop()
	}
	return m
}

func (m BookmarksModel) AppendBookmarks(items []model.Bookmark, cursor string) BookmarksModel {
	m.items = append(m.items, items...)
	m.nextCursor = cursor
	m.exhausted = cursor == ""
	m.loading = false
	if m.ready {
		m = m.refreshContent()
	}
	return m
}

// MarkDeleted removes the bookmark with the given ID from the in-memory list (optimistic update).
func (m BookmarksModel) MarkDeleted(id string) BookmarksModel {
	for i, b := range m.items {
		if b.ID == id {
			m.items = append(m.items[:i], m.items[i+1:]...)
			if m.selectedIndex >= len(m.items) && m.selectedIndex > 0 {
				m.selectedIndex--
			}
			break
		}
	}
	if m.ready {
		m = m.refreshContent()
	}
	return m
}

func (m BookmarksModel) SetError(err error) BookmarksModel {
	m.err = err
	m.loading = false
	m.fetching = false
	return m
}

func (m BookmarksModel) Init() tea.Cmd { return nil }

func (m BookmarksModel) Update(msg tea.Msg) (BookmarksModel, tea.Cmd) {
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
			if m.selectedIndex > 0 {
				m.selectedIndex--
				m = m.refreshContent()
				m = m.ensureSelectedVisible()
			}
			return m, nil
		case "down", "j":
			if m.selectedIndex < len(m.items)-1 {
				m.selectedIndex++
				m = m.refreshContent()
				m = m.ensureSelectedVisible()
			} else if !m.loading && !m.exhausted && m.nextCursor != "" {
				m.loading = true
				cursor := m.nextCursor
				m = m.refreshContent()
				m.viewport.ScrollDown(1)
				return m, func() tea.Msg { return LoadMoreBookmarksMsg{Cursor: cursor} }
			}
			return m, nil
		case "d":
			if len(m.items) == 0 || m.selectedIndex >= len(m.items) {
				return m, nil
			}
			b := m.items[m.selectedIndex]
			id := b.ID
			m = m.MarkDeleted(id)
			return m, func() tea.Msg {
				return DeleteBookmarkMsg{BookmarkID: id, PostID: b.PostID, ReplyID: b.ReplyID}
			}
		case "enter":
			if len(m.items) == 0 || m.selectedIndex >= len(m.items) {
				return m, nil
			}
			b := m.items[m.selectedIndex]
			if b.Post != nil {
				post := *b.Post
				return m, func() tea.Msg { return ShowPostMsg{Post: post} }
			}
			if b.PostID != "" {
				postID := b.PostID
				return m, func() tea.Msg { return OpenBookmarkMsg{PostID: postID} }
			}
			if b.ReplyID != "" {
				replyID := b.ReplyID
				return m, func() tea.Msg { return OpenBookmarkMsg{ReplyID: replyID} }
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)

	if m.ready && !m.loading && !m.exhausted && m.viewport.AtBottom() && m.nextCursor != "" {
		m.loading = true
		cursor := m.nextCursor
		m = m.refreshContent()
		m.viewport.ScrollDown(1)
		return m, tea.Batch(cmd, func() tea.Msg {
			return LoadMoreBookmarksMsg{Cursor: cursor}
		})
	}

	return m, cmd
}

func (m BookmarksModel) viewportHeight() int {
	return max(m.height-theme.ChromeHeight, 1)
}

func (m BookmarksModel) location() *time.Location {
	if m.loc == nil {
		return time.UTC
	}
	return m.loc
}


func (m BookmarksModel) refreshContent() BookmarksModel {
	content, offsets := m.buildContent()
	m.itemOffsets = offsets
	m.viewport.SetContent(content)
	return m
}

func (m BookmarksModel) ensureSelectedVisible() BookmarksModel {
	if !m.ready || len(m.itemOffsets) == 0 || m.selectedIndex >= len(m.items) {
		return m
	}
	itemStart := m.itemOffsets[m.selectedIndex]
	itemHeight := lipgloss.Height(m.renderItem(m.items[m.selectedIndex], false))
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

func (m BookmarksModel) buildContent() (string, []int) {
	if m.fetching {
		return theme.Subtle.Render("  Loading bookmarks…"), nil
	}
	if len(m.items) == 0 {
		return theme.Subtle.Render("  no bookmarks yet — press b on a post to save it"), nil
	}

	sep := "\n"
	lineInc := 0
	if m.relaxed {
		sep = "\n\n"
		lineInc = 1
	}

	offsets := make([]int, len(m.items))
	var sb strings.Builder
	currentLine := 0
	for i, b := range m.items {
		offsets[i] = currentLine
		rendered := m.renderItem(b, i == m.selectedIndex)
		sb.WriteString(rendered)
		sb.WriteString(sep)
		currentLine += lipgloss.Height(rendered) + lineInc
	}
	if m.loading {
		sb.WriteString(theme.Subtle.Render("  loading more…") + "\n")
	} else if m.exhausted {
		sb.WriteString(theme.Subtle.Render("  — end —") + "\n")
	}
	return sb.String(), offsets
}

func (m BookmarksModel) renderItem(b model.Bookmark, selected bool) string {
	innerWidth := m.width - 4
	now := time.Now()
	loc := m.location()

	var typeTag string
	if b.Type == "reply" {
		typeTag = theme.Subtle.Render("[reply]")
	} else {
		typeTag = theme.Subtle.Render("[post] ")
	}

	// Derive display fields from embedded post/reply or fallback.
	var author, content string
	var createdAt time.Time
	var attachments []model.Attachment
	var topics []string
	switch {
	case b.Post != nil:
		author = b.Post.AuthorUsername
		content = b.Post.Content
		createdAt = b.Post.CreatedAt
		attachments = b.Post.Attachments
		topics = b.Post.Topics
	case b.Reply != nil:
		author = b.Reply.AuthorUsername
		content = b.Reply.Content
		createdAt = b.Reply.CreatedAt
		attachments = b.Reply.Attachments
	default:
		content = "(content unavailable)"
		createdAt = b.CreatedAt
	}

	// Line 1 left: [type]  @author  [img][yt]
	var authorStyled string
	if author != "" {
		authorStyled = "  " + theme.Highlight.Render("@"+author)
	}
	attInd := attachmentIndicator(attachments)
	left1 := typeTag + authorStyled
	if attInd != "" {
		left1 += "  " + attInd
	}

	// Line 1 right: "posted Xh ago · saved Yd ago"
	postedStr := formatRelativeTime(createdAt, now, loc)
	savedStr := formatRelativeTime(b.CreatedAt, now, loc)
	right1 := theme.Subtle.Render("posted " + postedStr + " · saved " + savedStr)

	var line1 string
	if innerWidth > 0 {
		gap := innerWidth - lipgloss.Width(left1) - lipgloss.Width(right1)
		if gap > 0 {
			line1 = left1 + strings.Repeat(" ", gap) + right1
		} else {
			line1 = left1
		}
	} else {
		line1 = left1
	}

	// Line 2: content preview truncated to align under the gap before the timestamp.
	rightWidth := lipgloss.Width(right1)
	previewMax := max(innerWidth-rightWidth-1, 1)
	preview := strings.ReplaceAll(markdown.FirstLine(content), "\n", " ")
	line2 := theme.Base.Render(markdown.TruncateToWidth(preview, previewMax))

	// Line 3: topics or "no topics" — always rendered so card height is fixed.
	// Topics are truncated to innerWidth to prevent wrapping (which would break the 5-line guarantee).
	var line3 string
	if len(topics) > 0 {
		var parts []string
		for _, t := range topics {
			parts = append(parts, theme.Subtle.Render("#"+t))
		}
		line3 = strings.Join(parts, " ")
		if innerWidth > 0 && lipgloss.Width(line3) > innerWidth {
			line3 = ""
			for _, p := range parts {
				candidate := line3
				if candidate != "" {
					candidate += " "
				}
				candidate += p
				if lipgloss.Width(candidate) > innerWidth {
					break
				}
				line3 = candidate
			}
			if line3 == "" {
				line3 = theme.Subtle.Render("no topics")
			}
		}
	} else {
		line3 = theme.Subtle.Render("no topics")
	}

	rows := []string{line1, line2, line3}

	body := lipgloss.JoinVertical(lipgloss.Left, rows...)

	boxStyle := theme.Border
	if selected {
		boxStyle = theme.ActiveBorder
	}
	if innerWidth > 0 {
		boxStyle = boxStyle.Width(m.width - 2)
	}
	return boxStyle.Render(body)
}

func (m BookmarksModel) View() string {
	if m.err != nil {
		return theme.Error.Render(fmt.Sprintf("bookmarks error: %s", m.err))
	}
	if !m.ready {
		return theme.Subtle.Render("  Loading bookmarks…")
	}
	return m.viewport.View()
}

// GetFocusedURLs implements URLProvider. Returns URLs from the selected bookmark's content.
func (m BookmarksModel) GetFocusedURLs() []string {
	if len(m.items) == 0 || m.selectedIndex < 0 || m.selectedIndex >= len(m.items) {
		return nil
	}
	b := m.items[m.selectedIndex]
	if b.Post != nil {
		return append(extractURLs(b.Post.Content), attachmentURLs(b.Post.Attachments)...)
	}
	if b.Reply != nil {
		return append(extractURLs(b.Reply.Content), attachmentURLs(b.Reply.Attachments)...)
	}
	return nil
}
