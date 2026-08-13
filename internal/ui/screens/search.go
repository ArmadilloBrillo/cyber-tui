package screens

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
)

// Message types emitted by Search screen to App.

// SubmitSearchMsg is emitted on Enter in the query box.
type SubmitSearchMsg struct{ Query string }

// DrillSearchTypeMsg is emitted on Enter over a "see all" row in the preview.
// Type is "users", "posts", or "replies".
type DrillSearchTypeMsg struct{ Type string }

// LoadMoreSearchMsg is emitted when the viewport reaches the bottom of a
// drilled-into type list and a next-page cursor is available.
type LoadMoreSearchMsg struct {
	Type   string
	Cursor string
}

// ShowSearchPostMsg is emitted when the user opens a post hit.
type ShowSearchPostMsg struct{ Post model.Post }

// ShowSearchReplyMsg is emitted when the user opens a reply hit; App fetches
// the parent post by PostID and scrolls to ReplyID (same shape as
// ShowProfileReplyMsg / the Notifications reply deep-link).
type ShowSearchReplyMsg struct {
	PostID  string
	ReplyID string
}

// LeaveSearchMsg is emitted on 'esc' pressed at the outermost level (query
// view, already blurred) — App navigates back to the screen '/' was pressed
// from, the same return-to-origin pattern used by BackFromProfileMsg.
type LeaveSearchMsg struct{}

type searchView int

const (
	searchViewQuery searchView = iota
	searchViewPreview
	searchViewTypeList
)

type searchRowKind int

const (
	rowHeader searchRowKind = iota
	rowHit
	rowSeeAll
)

// searchRow is one flattened, renderable row in the preview or type-list view.
// Header rows are not selectable; Update skips over them during j/k navigation.
type searchRow struct {
	kind    searchRowKind
	hitType string // "users" | "posts" | "replies"
	index   int    // index into the matching slice, valid for kind == rowHit
}

// searchCategoryCap mirrors the API's "up to 8 hits per group" limit on the
// type=all preview. A category landing on exactly this many hits may have
// more results — the API gives no total count, so this is the only signal.
const searchCategoryCap = 8

type SearchModel struct {
	view searchView

	query textinput.Model

	lastQuery   string
	preview     model.SearchPreview
	hasSearched bool

	activeType string // "users" | "posts" | "replies" when view == searchViewTypeList

	posts          []model.Post
	postsCursor    string
	postsExhausted bool

	replies          []model.Reply
	repliesCursor    string
	repliesExhausted bool

	users          []model.User
	usersCursor    string
	usersExhausted bool

	rows     []searchRow
	selected int
	loading  bool
	err      error

	viewport    viewport.Model
	itemOffsets []int
	width       int
	height      int
	ready       bool

	bookmarkedPostIDs map[string]struct{}
	watchedPostIDs    map[string]struct{}
	loc               *time.Location
	relaxed           bool
	timeDisplayFormat string
	filterNSFW        bool
}

func NewSearchModel() SearchModel {
	ti := textinput.New()
	ti.Placeholder = "search users, posts, replies..."
	ti.Width = 40
	ti.Focus()
	return SearchModel{query: ti}
}

// FocusQuery switches to the query-edit mode and (re)focuses the input,
// keeping the previous query text and any cached results intact. Used by the
// global '/' shortcut, whether entering Search fresh or refocusing while
// already on it.
func (m SearchModel) FocusQuery() SearchModel {
	m.view = searchViewQuery
	m.query.Focus()
	m.query.CursorEnd()
	return m
}

// InputFocused reports whether the query box is currently capturing keys.
// Tracks the textinput's actual focus state rather than the view enum, so
// 'esc' can blur it — regaining normal tab/quit navigation — without needing
// a full view transition (e.g. after a failed search, which leaves the user
// in query view with nothing to show).
func (m SearchModel) InputFocused() bool { return m.query.Focused() }

// LastQuery returns the most recently submitted search query, used by App to
// re-issue a typed search (drill-down or pagination) against the same terms.
func (m SearchModel) LastQuery() string { return m.lastQuery }

// IsInTypeList reports whether a single drilled-into category (posts/replies/
// users) is currently shown, as opposed to the grouped preview.
func (m SearchModel) IsInTypeList() bool { return m.view == searchViewTypeList }

// firstSelectableRow returns the index of the first non-header row, or 0 if
// rows is empty or has no selectable rows.
func firstSelectableRow(rows []searchRow) int {
	for i, r := range rows {
		if r.kind != rowHeader {
			return i
		}
	}
	return 0
}

// sortPostsByRecency and sortRepliesByRecency sort in place, newest first.
// The search API's result ordering isn't documented, so the client sorts
// explicitly to guarantee a predictable, recency-based order for post/reply
// hits (users aren't date-ordered — relevance/follower count matter more
// there). Stable so hits with equal (or equally missing) timestamps keep the
// order the API returned them in, rather than shuffling arbitrarily.
func sortPostsByRecency(posts []model.Post) {
	slices.SortStableFunc(posts, func(a, b model.Post) int { return b.CreatedAt.Compare(a.CreatedAt) })
}

func sortRepliesByRecency(replies []model.Reply) {
	slices.SortStableFunc(replies, func(a, b model.Reply) int { return b.CreatedAt.Compare(a.CreatedAt) })
}

func (m SearchModel) SetPreview(preview model.SearchPreview, query string) SearchModel {
	m.err = nil
	sortPostsByRecency(preview.Posts)
	sortRepliesByRecency(preview.Replies)
	m.preview = preview
	m.lastQuery = query
	m.hasSearched = true
	m.loading = false
	m.view = searchViewPreview
	m.query.Blur()
	m.selected = 0
	if m.ready {
		m = m.refreshContent()
		m.selected = firstSelectableRow(m.rows)
		m = m.refreshContent()
		m.viewport.GotoTop()
	}
	return m
}

func (m SearchModel) SetTypeResults(hitType string, posts []model.Post, replies []model.Reply, users []model.User, cursor string) SearchModel {
	m.err = nil
	m.activeType = hitType
	switch hitType {
	case "posts":
		sortPostsByRecency(posts)
		m.posts = posts
		m.postsCursor = cursor
		m.postsExhausted = cursor == ""
	case "replies":
		sortRepliesByRecency(replies)
		m.replies = replies
		m.repliesCursor = cursor
		m.repliesExhausted = cursor == ""
	case "users":
		m.users = users
		m.usersCursor = cursor
		m.usersExhausted = cursor == ""
	}
	m.loading = false
	m.view = searchViewTypeList
	m.selected = 0
	m.query.Blur()
	if m.ready {
		m = m.refreshContent()
		m.selected = firstSelectableRow(m.rows)
		m = m.refreshContent()
		m.viewport.GotoTop()
	}
	return m
}

func (m SearchModel) AppendTypeResults(hitType string, posts []model.Post, replies []model.Reply, users []model.User, cursor string) SearchModel {
	m.err = nil
	switch hitType {
	case "posts":
		m.posts = append(m.posts, posts...)
		sortPostsByRecency(m.posts) // re-sort the whole accumulated list, not just the new page
		m.postsCursor = cursor
		m.postsExhausted = cursor == ""
	case "replies":
		m.replies = append(m.replies, replies...)
		sortRepliesByRecency(m.replies)
		m.repliesCursor = cursor
		m.repliesExhausted = cursor == ""
	case "users":
		m.users = append(m.users, users...)
		m.usersCursor = cursor
		m.usersExhausted = cursor == ""
	}
	m.loading = false
	if m.ready {
		m = m.refreshContent()
	}
	return m
}

func (m SearchModel) SetError(err error) SearchModel {
	m.err = err
	m.loading = false
	if m.ready {
		m = m.refreshContent()
	}
	return m
}

func (m SearchModel) Init() tea.Cmd { return nil }

func (m SearchModel) Update(msg tea.Msg) (SearchModel, tea.Cmd) {
	switch msg := msg.(type) {
	case SharedConfigMsg:
		m.relaxed = msg.Relaxed
		if msg.Loc != nil {
			m.loc = msg.Loc
		}
		m.timeDisplayFormat = msg.Settings.TimeDisplayFormat
		m.filterNSFW = msg.Settings.FilterNSFW
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
		m.query.Width = min(60, max(10, msg.Width-10))
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
		if m.query.Focused() {
			switch msg.String() {
			case "enter":
				q := strings.TrimSpace(m.query.Value())
				if q == "" {
					return m, nil
				}
				m.loading = true
				return m, func() tea.Msg { return SubmitSearchMsg{Query: q} }
			case "esc":
				// Leave Search immediately, back to whichever screen '/' was
				// pressed from — there's no result list showing at this point
				// (query mode fully replaces the view), so there's nothing to
				// peel back first. Blur so a later arrival via tab-cycling
				// (which doesn't call FocusQuery) doesn't inherit a stuck
				// focused state.
				m.query.SetValue("")
				m.query.Blur()
				return m, func() tea.Msg { return LeaveSearchMsg{} }
			}
			var cmd tea.Cmd
			m.query, cmd = m.query.Update(msg)
			return m, cmd
		}

		switch msg.String() {
		case "up", "k":
			m = m.moveSelection(-1)
			return m, nil

		case "down", "j":
			if m.selected < len(m.rows)-1 {
				m = m.moveSelection(1)
				return m, nil
			}
			if m.view == searchViewTypeList && !m.loading {
				if cmd := m.loadMoreCmd(); cmd != nil {
					m.loading = true
					m = m.refreshContent()
					return m, cmd
				}
			}
			return m, nil

		case "enter":
			return m.handleEnter()

		case "esc":
			// Only reachable in preview/type-list — this branch only runs when
			// the query box isn't focused, and query view is only ever entered
			// focused (see FocusQuery/NewSearchModel), so searchViewQuery never
			// appears here.
			switch m.view {
			case searchViewTypeList:
				m.view = searchViewPreview
				m = m.refreshContent()
				m.selected = firstSelectableRow(m.rows)
				m = m.refreshContent()
				m.viewport.GotoTop()
			case searchViewPreview:
				m = m.FocusQuery()
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)

	if m.view == searchViewTypeList && m.viewport.AtBottom() && !m.loading {
		if loadCmd := m.loadMoreCmd(); loadCmd != nil {
			m.loading = true
			m = m.refreshContent()
			return m, loadCmd
		}
	}

	return m, cmd
}

// loadMoreCmd returns the pagination command for the active type list, or nil
// when that category is exhausted.
func (m SearchModel) loadMoreCmd() tea.Cmd {
	var cursor string
	var exhausted bool
	switch m.activeType {
	case "posts":
		cursor, exhausted = m.postsCursor, m.postsExhausted
	case "replies":
		cursor, exhausted = m.repliesCursor, m.repliesExhausted
	case "users":
		cursor, exhausted = m.usersCursor, m.usersExhausted
	default:
		return nil
	}
	if exhausted {
		return nil
	}
	activeType := m.activeType
	return func() tea.Msg { return LoadMoreSearchMsg{Type: activeType, Cursor: cursor} }
}

// moveSelection shifts m.selected by delta rows, skipping over header rows,
// and re-renders so the newly selected row is highlighted and scrolled into view.
func (m SearchModel) moveSelection(delta int) SearchModel {
	next := m.selected + delta
	for next >= 0 && next < len(m.rows) && m.rows[next].kind == rowHeader {
		next += delta
	}
	if next < 0 || next >= len(m.rows) || m.rows[next].kind == rowHeader {
		return m
	}
	m.selected = next
	m = m.refreshContent()
	return m.ensureSelectedVisible()
}

// postAt, replyAt, and userAt resolve a rowHit's underlying model value from
// either the grouped preview or the drilled-into type list, whichever is
// currently showing. Callers must only pass a row whose hitType matches.
func (m SearchModel) postAt(row searchRow) model.Post {
	if m.view == searchViewPreview {
		return m.preview.Posts[row.index]
	}
	return m.posts[row.index]
}

func (m SearchModel) replyAt(row searchRow) model.Reply {
	if m.view == searchViewPreview {
		return m.preview.Replies[row.index]
	}
	return m.replies[row.index]
}

func (m SearchModel) userAt(row searchRow) model.User {
	if m.view == searchViewPreview {
		return m.preview.Users[row.index]
	}
	return m.users[row.index]
}

func (m SearchModel) handleEnter() (SearchModel, tea.Cmd) {
	if m.selected < 0 || m.selected >= len(m.rows) {
		return m, nil
	}
	row := m.rows[m.selected]
	switch row.kind {
	case rowSeeAll:
		hitType := row.hitType
		return m, func() tea.Msg { return DrillSearchTypeMsg{Type: hitType} }
	case rowHit:
		switch row.hitType {
		case "users":
			username := m.userAt(row).Username
			return m, func() tea.Msg { return ShowUserProfileMsg{Username: username} }
		case "posts":
			p := m.postAt(row)
			return m, func() tea.Msg { return ShowSearchPostMsg{Post: p} }
		case "replies":
			r := m.replyAt(row)
			postID, replyID := r.PostID, r.ID
			return m, func() tea.Msg { return ShowSearchReplyMsg{PostID: postID, ReplyID: replyID} }
		}
	}
	return m, nil
}

func (m SearchModel) View() string {
	if !m.ready {
		return theme.Subtle.Render("loading...")
	}
	if m.view == searchViewQuery {
		return m.renderQuery()
	}
	return m.viewport.View()
}

func (m SearchModel) renderQuery() string {
	label := theme.Subtle.Render("search")
	box := theme.ActiveBorder.Render(m.query.View())
	hint := theme.Subtle.Render("enter · search")
	var status string
	if m.err != nil {
		status = theme.Error.Render(fmt.Sprintf("error: %s", m.err))
	} else if m.hasSearched {
		status = theme.Subtle.Render(fmt.Sprintf("last search: %q", m.lastQuery))
	}
	return lipgloss.JoinVertical(lipgloss.Left, "", label, box, "", hint, status)
}

// buildContent renders the preview or type-list view and returns the rendered
// string, the per-row line offsets, and the flattened row descriptors — all
// three indexed identically (including header rows, which are never selected).
func (m SearchModel) buildContent() (string, []int, []searchRow) {
	switch m.view {
	case searchViewPreview:
		return m.buildPreviewContent()
	case searchViewTypeList:
		return m.buildTypeListContent()
	default:
		return "", nil, nil
	}
}

func (m SearchModel) buildPreviewContent() (string, []int, []searchRow) {
	if !m.hasSearched {
		return theme.Subtle.Render("  no search yet"), nil, nil
	}
	if len(m.preview.Users) == 0 && len(m.preview.Posts) == 0 && len(m.preview.Replies) == 0 {
		if m.err != nil {
			return theme.Subtle.Render("  couldn't load results"), nil, nil
		}
		return theme.Subtle.Render(fmt.Sprintf("  no results for %q", m.lastQuery)), nil, nil
	}

	var rows []searchRow
	var offsets []int
	var lines []string
	line := 0

	addRow := func(row searchRow, rendered string) {
		rows = append(rows, row)
		offsets = append(offsets, line)
		lines = append(lines, rendered)
		line += lipgloss.Height(rendered)
	}

	appendSection := func(title, hitType string, n int) {
		if n == 0 {
			return
		}
		addRow(searchRow{kind: rowHeader}, theme.Subtle.Bold(true).Render(fmt.Sprintf("-- %s (%d) --", title, n)))
		for i := range n {
			selected := len(rows) == m.selected
			var rendered string
			switch hitType {
			case "users":
				rendered = m.renderUserHit(m.preview.Users[i], selected)
			case "posts":
				rendered = m.renderPostHit(m.preview.Posts[i], selected)
			case "replies":
				rendered = m.renderReplyHit(m.preview.Replies[i], selected)
			}
			addRow(searchRow{kind: rowHit, hitType: hitType, index: i}, rendered)
		}
		if n == searchCategoryCap {
			selected := len(rows) == m.selected
			addRow(searchRow{kind: rowSeeAll, hitType: hitType}, m.renderSeeAllRow(hitType, selected))
		}
	}

	appendSection("users", "users", len(m.preview.Users))
	appendSection("posts", "posts", len(m.preview.Posts))
	appendSection("replies", "replies", len(m.preview.Replies))

	return strings.Join(lines, "\n"), offsets, rows
}

func (m SearchModel) buildTypeListContent() (string, []int, []searchRow) {
	var rows []searchRow
	var offsets []int
	var lines []string
	line := 0

	addRow := func(row searchRow, rendered string) {
		rows = append(rows, row)
		offsets = append(offsets, line)
		lines = append(lines, rendered)
		line += lipgloss.Height(rendered)
	}

	title := theme.Subtle.Bold(true).Render(fmt.Sprintf("-- %s matching %q --", m.activeType, m.lastQuery))
	addRow(searchRow{kind: rowHeader}, title)

	var n int
	var exhausted bool
	switch m.activeType {
	case "users":
		n, exhausted = len(m.users), m.usersExhausted
	case "posts":
		n, exhausted = len(m.posts), m.postsExhausted
	case "replies":
		n, exhausted = len(m.replies), m.repliesExhausted
	}

	if n == 0 {
		if m.err != nil {
			lines = append(lines, theme.Subtle.Render("  couldn't load results"))
		} else {
			lines = append(lines, theme.Subtle.Render("  no results"))
		}
	}

	for i := range n {
		selected := len(rows) == m.selected
		var rendered string
		switch m.activeType {
		case "users":
			rendered = m.renderUserHit(m.users[i], selected)
		case "posts":
			rendered = m.renderPostHit(m.posts[i], selected)
		case "replies":
			rendered = m.renderReplyHit(m.replies[i], selected)
		}
		addRow(searchRow{kind: rowHit, hitType: m.activeType, index: i}, rendered)
	}
	lines = append(lines, listFooter(m.loading, exhausted && n > 0))

	return strings.Join(lines, "\n"), offsets, rows
}

func (m SearchModel) renderUserHit(u model.User, selected bool) string {
	innerWidth := m.width - 4
	name := theme.Highlight.Render("@" + u.Username)
	if u.DisplayName != "" {
		name += theme.Subtle.Render(" (" + u.DisplayName + ")")
	}
	meta := theme.Subtle.Render(fmt.Sprintf("%d followers", u.FollowersCount))
	var line string
	if innerWidth > 0 {
		gap := innerWidth - lipgloss.Width(name) - lipgloss.Width(meta)
		if gap > 0 {
			line = name + strings.Repeat(" ", gap) + meta
		} else {
			line = name
		}
	} else {
		line = name
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

func (m SearchModel) renderPostHit(p model.Post, selected bool) string {
	_, bookmarked := m.bookmarkedPostIDs[p.ID]
	_, watched := m.watchedPostIDs[p.ID]
	return RenderPost(p, selected, bookmarked, watched, m.width, m.location(), m.timeDisplayFormat, postMaxBodyLines)
}

func (m SearchModel) renderReplyHit(r model.Reply, selected bool) string {
	innerWidth := m.width - 4
	header := theme.Highlight.Render("@" + r.AuthorUsername)
	header += theme.Subtle.Render("  " + displayTime(r.CreatedAt, m.location(), m.timeDisplayFormat, false) + editedSuffix(r.EditedAt))
	body := r.Content
	if innerWidth > 0 {
		body = ansiTruncate(body, innerWidth)
	}
	boxStyle := theme.Border
	if selected {
		boxStyle = theme.ActiveBorder
	}
	if innerWidth > 0 {
		boxStyle = boxStyle.Width(m.width - 2)
	}
	return boxStyle.Render(lipgloss.JoinVertical(lipgloss.Left, header, body))
}

func (m SearchModel) renderSeeAllRow(hitType string, selected bool) string {
	label := "→ see all " + hitType
	style := theme.Subtle
	boxStyle := theme.Border
	if selected {
		style = theme.Highlight
		boxStyle = theme.ActiveBorder
	}
	if m.width > 4 {
		boxStyle = boxStyle.Width(m.width - 2)
	}
	return boxStyle.Render(style.Render(label))
}

func (m SearchModel) refreshContent() SearchModel {
	content, offsets, rows := m.buildContent()
	m.rows = rows
	m.itemOffsets = offsets
	m.viewport.SetContent(content)
	return m.ensureSelectedVisible()
}

// selectedItemHeight measures the rendered height of the currently selected
// row (re-rendering it, same as topics.go's ensureSelectedVisible does for
// its own items) so ensureSelectedVisible can keep the whole multi-line card
// in view, not just its top line.
func (m SearchModel) selectedItemHeight() int {
	if m.selected < 0 || m.selected >= len(m.rows) {
		return 1
	}
	row := m.rows[m.selected]
	switch row.kind {
	case rowSeeAll:
		return lipgloss.Height(m.renderSeeAllRow(row.hitType, true))
	case rowHit:
		switch row.hitType {
		case "users":
			return lipgloss.Height(m.renderUserHit(m.userAt(row), true))
		case "posts":
			return lipgloss.Height(m.renderPostHit(m.postAt(row), true))
		case "replies":
			return lipgloss.Height(m.renderReplyHit(m.replyAt(row), true))
		}
	}
	return 1
}

func (m SearchModel) ensureSelectedVisible() SearchModel {
	if !m.ready || m.selected < 0 || m.selected >= len(m.itemOffsets) {
		return m
	}
	itemStart := m.itemOffsets[m.selected]
	itemEnd := itemStart + m.selectedItemHeight() - 1
	viewTop := m.viewport.YOffset
	viewBottom := viewTop + m.viewport.Height - 1
	if itemStart < viewTop {
		m.viewport.SetYOffset(itemStart)
	} else if itemEnd > viewBottom {
		if itemEnd-itemStart+1 <= m.viewport.Height {
			m.viewport.SetYOffset(itemEnd - m.viewport.Height + 1)
		} else {
			m.viewport.SetYOffset(itemStart)
		}
	}
	return m
}

func (m SearchModel) viewportHeight() int {
	return m.height - theme.ChromeHeight
}

func (m SearchModel) location() *time.Location {
	if m.loc == nil {
		return time.UTC
	}
	return m.loc
}

// GetFocusedURLs implements URLProvider for the 'o'/'ctrl+o' opener.
func (m SearchModel) GetFocusedURLs() []string {
	if m.selected < 0 || m.selected >= len(m.rows) {
		return nil
	}
	row := m.rows[m.selected]
	if row.kind != rowHit {
		return nil
	}
	switch row.hitType {
	case "posts":
		p := m.postAt(row)
		return append(extractURLs(p.Content), attachmentURLs(p.Attachments)...)
	case "replies":
		r := m.replyAt(row)
		return append(extractURLs(r.Content), attachmentURLs(r.Attachments)...)
	}
	return nil
}
