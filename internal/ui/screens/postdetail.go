package screens

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/markdown"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
)

// replyNode holds a reply together with its computed tree position.
type replyNode struct {
	Reply          model.Reply
	Depth          int    // display depth 0–3 (capped)
	ParentUsername string // AuthorUsername of the parent reply; "" for top-level or orphans
}

// buildReplyTree converts a flat reply list into a depth-first ordered slice of
// replyNodes. Children at each level are sorted chronologically. Orphaned
// replies (whose parent is not in the list) are treated as top-level.
// maxDepth caps the display depth; replies deeper than maxDepth are shown at maxDepth.
func buildReplyTree(replies []model.Reply, maxDepth int) []replyNode {
	if len(replies) == 0 {
		return nil
	}

	idToIdx := make(map[string]int, len(replies))
	for i, r := range replies {
		idToIdx[r.ID] = i
	}

	children := make(map[string][]int)
	for i, r := range replies {
		if r.ParentReplyID != "" {
			if _, ok := idToIdx[r.ParentReplyID]; ok {
				children[r.ParentReplyID] = append(children[r.ParentReplyID], i)
				continue
			}
		}
		children[""] = append(children[""], i)
	}

	for key := range children {
		sl := children[key]
		sort.Slice(sl, func(a, b int) bool {
			return replies[sl[a]].CreatedAt.Before(replies[sl[b]].CreatedAt)
		})
	}

	result := make([]replyNode, 0, len(replies))

	var walk func(idx, depth int)
	walk = func(idx, depth int) {
		r := replies[idx]
		d := depth
		if d > maxDepth {
			d = maxDepth
		}
		var parentUsername string
		if r.ParentReplyID != "" {
			if pidx, ok := idToIdx[r.ParentReplyID]; ok {
				parentUsername = replies[pidx].AuthorUsername
			}
		}
		result = append(result, replyNode{Reply: r, Depth: d, ParentUsername: parentUsername})
		for _, childIdx := range children[r.ID] {
			walk(childIdx, depth+1)
		}
	}

	for _, idx := range children[""] {
		walk(idx, 0)
	}

	return result
}

// BackToFeedMsg is emitted when the user presses Esc to return to the feed.
type BackToFeedMsg struct{}

// pdConfirmKind tracks which delete action is awaiting confirmation in PostDetail.
type pdConfirmKind int

const (
	pdConfirmNone        pdConfirmKind = iota
	pdConfirmDeletePost                // d pressed while post is selected
	pdConfirmDeleteReply               // d pressed while a reply is selected
)

// SubmitReplyMsg is emitted when the compose box is submitted.
// App intercepts this, calls CreateReply, then reloads replies.
type SubmitReplyMsg struct {
	PostID        string
	ParentReplyID string
	Content       string
}

type PostDetailModel struct {
	post          model.Post
	replies       []model.Reply
	flatTree      []replyNode   // DFS-ordered tree walk; len always == len(replies)
	replyOffsets  []int         // start line of each reply within the viewport content
	replyHeights  []int         // rendered height of each reply (matches offsets; set by buildContent)
	selectedReply int
	viewport      viewport.Model
	width         int
	height        int
	ready         bool
	loading       bool
	err           error

	compose            ComposeModel
	replyPostID        string         // postID set when compose opens
	replyParentID      string         // parentReplyID set when compose opens (empty = top-level)
	relaxed            bool           // true = blank lines between post, header, and replies
	loc                *time.Location // timezone for timestamp display; nil = UTC
	timeDisplayFormat  string         // API setting: "datetime", "relative", "unix", "swatch"

	currentUsername string        // set after login; guards the delete key to own content
	confirming      pdConfirmKind // pending delete confirmation
	maxThreadDepth  int           // max visual nesting depth; 0 treated as 3

	bookmarkedPostIDs  map[string]struct{}
	bookmarkedReplyIDs map[string]struct{}
}

func NewPostDetailModel() PostDetailModel {
	return PostDetailModel{
		compose: NewComposeModel(0),
	}
}

func (m PostDetailModel) SetPost(post model.Post) PostDetailModel {
	m.post = post
	m.replies = nil
	m.flatTree = nil
	m.replyOffsets = nil
	m.replyHeights = nil
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
	m.selectedReply = -1 // keep post selected after replies load
	m.loading = false
	if len(replies) > m.post.RepliesCount {
		m.post.RepliesCount = len(replies)
	}
	return m.applyReplies(replies)
}

func (m PostDetailModel) applyReplies(replies []model.Reply) PostDetailModel {
	m.replies = replies
	m.flatTree = buildReplyTree(replies, m.effectiveMaxDepth())
	if m.ready {
		m = m.refreshContent()
	}
	return m
}

func (m PostDetailModel) effectiveMaxDepth() int {
	if m.maxThreadDepth <= 0 {
		return 3
	}
	return m.maxThreadDepth
}

// ScrollToReply selects and scrolls to the reply with the given ID.
// If the ID is not found or empty, the model is returned unchanged.
func (m PostDetailModel) ScrollToReply(replyID string) PostDetailModel {
	if replyID == "" {
		return m
	}
	for i, node := range m.flatTree {
		if node.Reply.ID == replyID {
			m.selectedReply = i
			if m.ready {
				m = m.refreshContent()
				m = m.ensureSelectedVisible()
			}
			return m
		}
	}
	return m
}

// Loading reports whether replies are still being fetched.
func (m PostDetailModel) Loading() bool { return m.loading }

// SelectedReplyID returns the ID of the currently selected reply, or "" if the post itself is selected.
func (m PostDetailModel) SelectedReplyID() string {
	if m.selectedReply >= 0 && m.selectedReply < len(m.flatTree) {
		return m.flatTree[m.selectedReply].Reply.ID
	}
	return ""
}

// Ready reports whether the viewport has been initialised (i.e. a WindowSizeMsg was received).
func (m PostDetailModel) Ready() bool { return m.ready }

// ComposeActive reports whether the compose box is currently open.
func (m PostDetailModel) ComposeActive() bool { return m.compose.IsActive() }

func (m PostDetailModel) SetError(err error) PostDetailModel {
	m.err = err
	m.loading = false
	return m
}

// SetCurrentUsername records the logged-in user's username so PostDetail can
// restrict the delete key to the user's own posts and replies.
func (m PostDetailModel) SetCurrentUsername(username string) PostDetailModel {
	m.currentUsername = username
	return m
}

// RemoveReply removes a reply from the local list by ID (called after a
// successful DELETE API call). Adjusts selectedReply to stay in bounds.
func (m PostDetailModel) RemoveReply(replyID string) PostDetailModel {
	for i, r := range m.replies {
		if r.ID == replyID {
			m.replies = append(m.replies[:i], m.replies[i+1:]...)
			m.flatTree = buildReplyTree(m.replies, m.effectiveMaxDepth())
			switch {
			case len(m.flatTree) == 0:
				m.selectedReply = -1
			case m.selectedReply >= len(m.flatTree):
				m.selectedReply = len(m.flatTree) - 1
			}
			break
		}
	}
	if m.ready {
		m = m.refreshContent()
	}
	return m
}

func (m PostDetailModel) SetRelaxed(relaxed bool) PostDetailModel {
	m.relaxed = relaxed
	if m.ready {
		m = m.refreshContent()
		m = m.ensureSelectedVisible()
	}
	return m
}

func (m PostDetailModel) location() *time.Location {
	if m.loc == nil {
		return time.UTC
	}
	return m.loc
}

func (m PostDetailModel) SetLocation(loc *time.Location) PostDetailModel {
	if loc == nil {
		loc = time.UTC
	}
	m.loc = loc
	if m.ready {
		m = m.refreshContent()
	}
	return m
}

// OpenCompose opens the compose box targeting the currently selected item.
// Returns (model, cmd) where cmd starts the cursor blink animation.
func (m PostDetailModel) OpenCompose() (PostDetailModel, tea.Cmd) {
	m.replyPostID = m.post.ID
	var ctx string
	if m.selectedReply >= 0 && m.selectedReply < len(m.flatTree) {
		m.replyParentID = m.flatTree[m.selectedReply].Reply.ID
		ctx = "replying to @" + m.flatTree[m.selectedReply].Reply.AuthorUsername
	} else {
		m.replyParentID = ""
		ctx = "replying to @" + m.post.AuthorUsername
	}
	var cmd tea.Cmd
	m.compose, cmd = m.compose.Open(ctx, "write your reply…")
	if m.ready {
		m.viewport.Height = m.viewportHeight()
	}
	return m, cmd
}

// viewportHeight returns the number of lines the viewport should occupy,
// accounting for the compose box and delete confirmation overlay when active.
func (m PostDetailModel) viewportHeight() int {
	h := m.height - theme.ChromeHeight
	if m.compose.IsActive() {
		h -= m.compose.BoxHeight()
	}
	if m.confirming != pdConfirmNone {
		h -= confirmBoxHeight
	}
	if h < 1 {
		h = 1
	}
	return h
}

func (m PostDetailModel) refreshContent() PostDetailModel {
	content, offsets, heights := m.buildContent()
	m.replyOffsets = offsets
	m.replyHeights = heights
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
		if len(m.replyOffsets) == 0 || m.selectedReply >= len(m.flatTree) {
			return m
		}
		itemStart = m.replyOffsets[m.selectedReply]
		itemHeight = m.replyHeights[m.selectedReply]
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
	case SharedConfigMsg:
		m.timeDisplayFormat = msg.Settings.TimeDisplayFormat
		m = m.SetRelaxed(msg.Relaxed)
		m = m.SetLocation(msg.Loc)
		if msg.MaxThreadDepth != m.maxThreadDepth {
			m.maxThreadDepth = msg.MaxThreadDepth
			m = m.applyReplies(m.replies)
		}
		return m, nil

	case BookmarkedIDsMsg:
		m.bookmarkedPostIDs = msg.PostIDs
		m.bookmarkedReplyIDs = msg.ReplyIDs
		if m.ready {
			m = m.refreshContent()
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.compose = m.compose.SetWidth(msg.Width)
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

	case ComposeSubmitMsg:
		content := msg.Content
		postID := m.replyPostID
		parentID := m.replyParentID
		m.compose = m.compose.Close()
		if m.ready {
			m.viewport.Height = m.viewportHeight()
		}
		return m, func() tea.Msg {
			return SubmitReplyMsg{
				PostID:        postID,
				ParentReplyID: parentID,
				Content:       content,
			}
		}

	case ComposeCancelMsg:
		m.compose = m.compose.Close()
		if m.ready {
			m.viewport.Height = m.viewportHeight()
		}
		return m, nil

	case tea.KeyMsg:
		// Confirmation overlay intercepts all keys while active.
		if m.confirming != pdConfirmNone {
			switch msg.String() {
			case "y":
				action := m.confirming
				m.confirming = pdConfirmNone
				m.viewport.Height = m.viewportHeight()
				switch action {
				case pdConfirmDeletePost:
					postID := m.post.ID
					return m, func() tea.Msg { return DeletePostMsg{PostID: postID} }
				case pdConfirmDeleteReply:
					if m.selectedReply >= 0 && m.selectedReply < len(m.flatTree) {
						replyID := m.flatTree[m.selectedReply].Reply.ID
						postID := m.post.ID
						return m, func() tea.Msg {
							return DeleteReplyMsg{ReplyID: replyID, PostID: postID}
						}
					}
				}
			case "n", "esc":
				m.confirming = pdConfirmNone
				m.viewport.Height = m.viewportHeight()
			}
			return m, nil
		}

		// When compose is open, all key events go to the compose box.
		if m.compose.IsActive() {
			prevH := m.compose.BoxHeight()
			var cmd tea.Cmd
			m.compose, cmd = m.compose.Update(msg)
			if m.compose.BoxHeight() != prevH && m.ready {
				m.viewport.Height = m.viewportHeight()
			}
			return m, cmd
		}

		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return BackToFeedMsg{} }
		case "d":
			if m.selectedReply == -1 {
				// Post selected — only allow delete if it's the user's own post.
				if m.post.ID != "" && m.post.AuthorUsername == m.currentUsername {
					m.confirming = pdConfirmDeletePost
					m.viewport.Height = m.viewportHeight()
				}
			} else if m.selectedReply >= 0 && m.selectedReply < len(m.flatTree) {
				// Reply selected — only allow delete if it's the user's own reply.
				if m.flatTree[m.selectedReply].Reply.AuthorUsername == m.currentUsername {
					m.confirming = pdConfirmDeleteReply
					m.viewport.Height = m.viewportHeight()
				}
			}
			return m, nil
		case "r":
			var cmd tea.Cmd
			m, cmd = m.OpenCompose()
			return m, cmd
		case "p":
			if m.post.ID == "" {
				return m, nil
			}
			var username string
			if m.selectedReply >= 0 && m.selectedReply < len(m.flatTree) {
				username = m.flatTree[m.selectedReply].Reply.AuthorUsername
			} else {
				username = m.post.AuthorUsername
			}
			return m, func() tea.Msg { return ShowUserProfileMsg{Username: username} }
		case "b":
			if m.selectedReply >= 0 && m.selectedReply < len(m.flatTree) {
				replyID := m.flatTree[m.selectedReply].Reply.ID
				if replyID != "" {
					return m, func() tea.Msg { return BookmarkPostMsg{ReplyID: replyID} }
				}
			}
			if m.post.ID != "" {
				postID := m.post.ID
				return m, func() tea.Msg { return BookmarkPostMsg{PostID: postID} }
			}
			return m, nil
		case "up", "k":
			if m.selectedReply >= 0 {
				// Reply is selected — scroll through it first (pager behaviour).
				replyTop := m.replyOffsets[m.selectedReply]
				if replyTop < m.viewport.YOffset {
					// Reply top is above the visible area — scroll up.
					m.viewport.LineUp(1)
				} else {
					// Reply top is visible — move to previous item.
					m.selectedReply--
					m = m.refreshContent()
					m = m.ensureSelectedVisible()
				}
			} else {
				// Post is selected — scroll viewport up (pager behaviour).
				m.viewport.LineUp(1)
			}
			return m, nil
		case "down", "j":
			if m.selectedReply == -1 {
				// Post is selected — scroll through it first, then advance to replies.
				postH := lipgloss.Height(m.renderFullPost(true))
				viewBottom := m.viewport.YOffset + m.viewport.Height - 1
				if viewBottom >= postH-1 && len(m.replies) > 0 {
					// Post bottom is visible — jump to first reply.
					m.selectedReply = 0
					m = m.refreshContent()
					m = m.ensureSelectedVisible()
				} else {
					m.viewport.LineDown(1)
				}
			} else {
				// Reply is selected — scroll through it first (pager behaviour).
				replyH := m.replyHeights[m.selectedReply]
				replyBottom := m.replyOffsets[m.selectedReply] + replyH - 1
				viewBottom := m.viewport.YOffset + m.viewport.Height - 1
				if replyBottom > viewBottom {
					// Reply bottom is below the visible area — scroll down.
					m.viewport.LineDown(1)
				} else if m.selectedReply < len(m.flatTree)-1 {
					// Reply bottom is visible — advance to next reply.
					m.selectedReply++
					m = m.refreshContent()
					m = m.ensureSelectedVisible()
				}
			}
			return m, nil
		}
	}

	// Viewport scrolling only when compose is closed.
	if !m.compose.IsActive() {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}
	return m, nil
}

// buildContent renders the full post and all replies into a single string for
// the viewport. It returns the string, the start-line offset of each reply,
// and the rendered height of each reply. Heights are measured here once so
// that ensureSelectedVisible and the pager always use the same values that
// were used to lay out the content.
func (m PostDetailModel) buildContent() (string, []int, []int) {
	postContent := m.renderFullPost(m.selectedReply == -1)
	var repliesHeaderText string
	total := m.post.RepliesCount
	loaded := len(m.replies)
	switch total {
	case 0:
		repliesHeaderText = "  no replies"
	case 1:
		repliesHeaderText = "  1 reply"
	default:
		if loaded < total {
			repliesHeaderText = fmt.Sprintf("  %d replies  (showing %d)", total, loaded)
		} else {
			repliesHeaderText = fmt.Sprintf("  %d replies", total)
		}
	}
	repliesHeader := theme.Title.Render(repliesHeaderText)

	sep := "\n"
	if m.relaxed {
		sep = "\n\n"
	}

	var sb strings.Builder
	sb.WriteString(postContent)
	sb.WriteString(sep)
	sb.WriteString(repliesHeader)
	sb.WriteString(sep)

	if m.loading {
		sb.WriteString(theme.Subtle.Render("  loading replies…"))
		sb.WriteString("\n")
		return sb.String(), nil, nil
	}
	if len(m.replies) == 0 {
		sb.WriteString(theme.Subtle.Render("  no replies yet"))
		sb.WriteString("\n")
		return sb.String(), nil, nil
	}

	// Base line where first reply starts.
	// Relaxed: post + blank + header + blank = H_post+1+H_header+1
	// Dense:   post + header (no blank lines) = H_post+H_header
	var baseLines int
	if m.relaxed {
		baseLines = lipgloss.Height(postContent) + 1 + lipgloss.Height(repliesHeader) + 1
	} else {
		baseLines = lipgloss.Height(postContent) + lipgloss.Height(repliesHeader)
	}
	offsets := make([]int, len(m.flatTree))
	heights := make([]int, len(m.flatTree))
	currentLine := baseLines

	for i, node := range m.flatTree {
		offsets[i] = currentLine
		rendered := m.renderReply(node, i == m.selectedReply)
		h := lipgloss.Height(rendered)
		heights[i] = h
		sb.WriteString(rendered)
		sb.WriteString(sep)
		if m.relaxed {
			currentLine += h + 1
		} else {
			currentLine += h
		}
	}

	return sb.String(), offsets, heights
}

func (m PostDetailModel) renderFullPost(selected bool) string {
	innerWidth := m.width - 4

	_, postBookmarked := m.bookmarkedPostIDs[m.post.ID]
	left := lipgloss.JoinHorizontal(lipgloss.Top,
		theme.Highlight.Render("@"+m.post.AuthorUsername),
		theme.Subtle.Render("  "+displayTime(m.post.CreatedAt, m.location(), m.timeDisplayFormat, false)),
	) + audioIcon(m.post.Attachments) + bookmarkIcon(postBookmarked)
	var rightParts []string
	if ind := attachmentIndicator(m.post.Attachments); ind != "" {
		rightParts = append(rightParts, ind)
	}
	right := strings.Join(rightParts, " ")
	var header string
	if right != "" && innerWidth > 0 {
		gap := innerWidth - lipgloss.Width(left) - lipgloss.Width(right)
		if gap > 0 {
			header = left + strings.Repeat(" ", gap) + right
		} else {
			header = left
		}
	} else {
		header = left
	}

	body := markdown.Render(m.post.Content, innerWidth)
	if att := renderAttachments(m.post.Attachments); att != "" {
		body = body + "\n" + att
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

func (m PostDetailModel) renderReply(node replyNode, selected bool) string {
	indentW := node.Depth * 3
	cardWidth := m.width - 2 - indentW
	innerWidth := cardWidth - 2

	var headerParts []string
	if node.ParentUsername != "" {
		headerParts = append(headerParts, theme.Subtle.Render("↩ @"+node.ParentUsername+"  "))
	}
	headerParts = append(headerParts,
		theme.Highlight.Render("@"+node.Reply.AuthorUsername),
		theme.Subtle.Render("  "+displayTime(node.Reply.CreatedAt, m.location(), m.timeDisplayFormat, false)),
	)
	_, replyBookmarked := m.bookmarkedReplyIDs[node.Reply.ID]
	left := lipgloss.JoinHorizontal(lipgloss.Top, headerParts...) + audioIcon(node.Reply.Attachments) + bookmarkIcon(replyBookmarked)
	var replyRightParts []string
	if ind := attachmentIndicator(node.Reply.Attachments); ind != "" {
		replyRightParts = append(replyRightParts, ind)
	}
	replyRight := strings.Join(replyRightParts, " ")
	header := left
	if replyRight != "" && innerWidth > 0 {
		gap := innerWidth - lipgloss.Width(left) - lipgloss.Width(replyRight)
		if gap > 0 {
			header = left + strings.Repeat(" ", gap) + replyRight
		}
	}

	body := markdown.Render(node.Reply.Content, innerWidth)
	if att := renderAttachments(node.Reply.Attachments); att != "" {
		body = body + "\n" + att
	}

	boxStyle := theme.Border
	if selected {
		boxStyle = theme.ActiveBorder
	}
	if cardWidth > 0 {
		boxStyle = boxStyle.Width(cardWidth)
	}
	card := boxStyle.Render(lipgloss.JoinVertical(lipgloss.Left, header, body))
	if indentW > 0 {
		return lipgloss.NewStyle().MarginLeft(indentW).Render(card)
	}
	return card
}

func (m PostDetailModel) View() string {
	if m.err != nil {
		return theme.Error.Render(fmt.Sprintf("error: %s", m.err))
	}
	if !m.ready {
		return theme.Subtle.Render("loading…")
	}

	if m.confirming != pdConfirmNone {
		var promptText string
		switch m.confirming {
		case pdConfirmDeletePost:
			promptText = theme.Error.Render("Delete this post?") + "  " +
				theme.Base.Render("[y]es") + "  " +
				theme.Subtle.Render("[n]o / esc")
		case pdConfirmDeleteReply:
			promptText = theme.Error.Render("Delete this reply?") + "  " +
				theme.Base.Render("[y]es") + "  " +
				theme.Subtle.Render("[n]o / esc")
		}
		promptView := theme.ActiveBorder.Width(m.width - 2).Render(promptText)
		if m.compose.IsActive() {
			return lipgloss.JoinVertical(lipgloss.Left,
				m.viewport.View(),
				m.compose.View(),
				promptView,
			)
		}
		return lipgloss.JoinVertical(lipgloss.Left,
			m.viewport.View(),
			promptView,
		)
	}

	if m.compose.IsActive() {
		return lipgloss.JoinVertical(lipgloss.Left,
			m.viewport.View(),
			m.compose.View(),
		)
	}
	return m.viewport.View()
}

// GetFocusedURLs implements URLProvider. Returns URLs from the currently focused
// item: the post itself when no reply is selected, or the selected reply.
func (m PostDetailModel) GetFocusedURLs() []string {
	if m.post.ID == "" {
		return nil
	}
	if m.selectedReply >= 0 && m.selectedReply < len(m.flatTree) {
		r := m.flatTree[m.selectedReply].Reply
		return append(extractURLs(r.Content), attachmentURLs(r.Attachments)...)
	}
	return append(extractURLs(m.post.Content), attachmentURLs(m.post.Attachments)...)
}
