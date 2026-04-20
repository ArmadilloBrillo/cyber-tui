package screens

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

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

// RefreshBookmarksMsg is emitted when the user presses up at the top of the list.
type RefreshBookmarksMsg struct{}

// DeleteBookmarkMsg is emitted when the user presses 'd' on the selected bookmark.
type DeleteBookmarkMsg struct{ BookmarkID string }

// BookmarkPostMsg is emitted from Feed or PostDetail when the user presses 'b'
// to bookmark the currently selected post.
type BookmarkPostMsg struct{ PostID string }

// OpenBookmarkMsg is emitted when the user presses Enter on a bookmark that has
// no embedded post/reply content. App fetches the item by ID then navigates to PostDetail.
// Exactly one of PostID or ReplyID is set.
type OpenBookmarkMsg struct {
	PostID  string
	ReplyID string
}

type BookmarksModel struct {
	items         []model.Bookmark
	viewport      viewport.Model
	itemOffsets   []int // start line of each item in viewport content
	width         int
	height        int
	selectedIndex int
	ready         bool
	loading       bool
	refreshing    bool
	exhausted     bool
	nextCursor    string
	err           error
	loc           *time.Location
	relaxed       bool
	// statusMsg is a transient message (e.g. "bookmarked") shown in the list header.
	statusMsg string
}

func NewBookmarksModel() BookmarksModel {
	return BookmarksModel{}
}

func (m BookmarksModel) SetBookmarks(items []model.Bookmark, cursor string) BookmarksModel {
	m.items = items
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

// SetStatusMsg stores a transient feedback message (displayed at top of list).
func (m BookmarksModel) SetStatusMsg(msg string) BookmarksModel {
	m.statusMsg = msg
	if m.ready {
		m = m.refreshContent()
	}
	return m
}

func (m BookmarksModel) SetError(err error) BookmarksModel {
	m.err = err
	m.loading = false
	m.refreshing = false
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
			} else if !m.loading && !m.refreshing {
				m.refreshing = true
				m = m.refreshContent()
				return m, func() tea.Msg { return RefreshBookmarksMsg{} }
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
				return m, func() tea.Msg { return LoadMoreBookmarksMsg{Cursor: cursor} }
			}
			return m, nil
		case "d":
			if len(m.items) == 0 || m.selectedIndex >= len(m.items) {
				return m, nil
			}
			id := m.items[m.selectedIndex].ID
			m = m.MarkDeleted(id)
			return m, func() tea.Msg { return DeleteBookmarkMsg{BookmarkID: id} }
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
		return m, tea.Batch(cmd, func() tea.Msg {
			return LoadMoreBookmarksMsg{Cursor: cursor}
		})
	}

	return m, cmd
}

func (m BookmarksModel) viewportHeight() int {
	h := m.height - theme.ChromeHeight
	if h < 1 {
		h = 1
	}
	return h
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
	var prefix string
	startLine := 0

	if m.statusMsg != "" {
		prefix = theme.Highlight.Render("  "+m.statusMsg) + "\n"
		startLine = 1
	}
	if m.refreshing {
		prefix += theme.Subtle.Render("  fetching bookmarks...") + "\n"
		startLine++
	}

	if len(m.items) == 0 {
		return prefix + theme.Subtle.Render("  no bookmarks yet — press b on a post to save it"), nil
	}

	sep := "\n"
	lineInc := 0
	if m.relaxed {
		sep = "\n\n"
		lineInc = 1
	}

	offsets := make([]int, len(m.items))
	var out string
	currentLine := startLine

	for i, b := range m.items {
		offsets[i] = currentLine
		rendered := m.renderItem(b, i == m.selectedIndex)
		out += rendered + sep
		currentLine += lipgloss.Height(rendered) + lineInc
	}
	if m.loading {
		out += theme.Subtle.Render("  loading more…") + "\n"
	} else if m.exhausted {
		out += theme.Subtle.Render("  — end —") + "\n"
	}
	return prefix + out, offsets
}

func (m BookmarksModel) renderItem(b model.Bookmark, selected bool) string {
	innerWidth := m.width - 4
	now := time.Now()
	loc := m.location()

	var typeTag string
	if b.Type == "reply" {
		typeTag = theme.Subtle.Render("[reply] ")
	} else {
		typeTag = theme.Subtle.Render("[post]  ")
	}

	// Derive display fields from embedded post/reply or fallback to IDs.
	var author, content string
	var createdAt time.Time
	switch {
	case b.Post != nil:
		author = b.Post.AuthorUsername
		content = b.Post.Content
		createdAt = b.Post.CreatedAt
	case b.Reply != nil:
		author = b.Reply.AuthorUsername
		content = b.Reply.Content
		createdAt = b.Reply.CreatedAt
	default:
		// No embedded content from API — don't show a raw ID as the username.
		author = ""
		if b.PostID != "" || b.ReplyID != "" {
			content = "(press enter to open)"
		} else {
			content = "(content unavailable)"
		}
		createdAt = b.CreatedAt
	}

	var authorStyled string
	if author != "" {
		authorStyled = theme.Highlight.Render("@" + author)
	}
	ts := theme.Subtle.Render(formatRelativeTime(createdAt, now, loc))

	// Truncate content preview to one line (markdown syntax stripped).
	preview := strings.ReplaceAll(markdown.FirstLine(content), "\n", " ")
	maxPreview := innerWidth - lipgloss.Width(typeTag) - lipgloss.Width(authorStyled) - lipgloss.Width(ts) - 3
	if maxPreview < 10 {
		maxPreview = 10
	}
	if utf8.RuneCountInString(preview) > maxPreview {
		preview = string([]rune(preview)[:maxPreview-1]) + "…"
	}
	previewStyled := theme.Base.Render("  " + preview)

	top := lipgloss.JoinHorizontal(lipgloss.Top, typeTag, authorStyled, previewStyled)
	var line string
	if innerWidth > 0 {
		gap := innerWidth - lipgloss.Width(top) - lipgloss.Width(ts)
		if gap > 0 {
			line = top + strings.Repeat(" ", gap) + ts
		} else {
			line = top
		}
	} else {
		line = top
	}

	boxStyle := theme.Border
	if selected {
		boxStyle = theme.ActiveBorder
	}
	if innerWidth > 0 {
		boxStyle = boxStyle.Width(m.width - 2)
	}
	return boxStyle.Render(line)
}

func (m BookmarksModel) View() string {
	if m.err != nil {
		return theme.Error.Render(fmt.Sprintf("bookmarks error: %s", m.err))
	}
	if !m.ready {
		return theme.Subtle.Render("loading bookmarks...")
	}
	return m.viewport.View()
}
