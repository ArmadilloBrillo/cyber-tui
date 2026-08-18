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

// SubmitReplyEditMsg is emitted when the user submits an edit to their own
// reply via the 'e' key. content is the only editable field for replies.
type SubmitReplyEditMsg struct {
	ReplyID string
	PostID  string
	Content string
}

type PostDetailModel struct {
	post         model.Post
	replies      []model.Reply
	flatTree     []replyNode // DFS-ordered tree walk; len always == len(replies)
	replyOffsets []int       // start line of each reply within the viewport content
	replyHeights []int       // rendered height of each reply (matches offsets; set by buildContent)
	postHeight   int         // rendered height of the full post block; set by refreshContent

	// inlineImagesEnabled mirrors SharedConfigMsg.InlineImagesEnabled — the
	// fully-gated value (config flag AND protocol available AND imageViewer
	// != "browser" AND not an ephemeral SSH session). postImages/replyImages
	// are only ever populated when this is true.
	inlineImagesEnabled bool
	postImages          []postImageSlot   // every eligible image in the post; set by buildContent
	replyImages         [][]postImageSlot // parallel to flatTree/replyOffsets — one slice per reply; set by buildContent
	selectedReply       int
	viewport            viewport.Model
	width               int
	height              int
	ready               bool
	loading             bool
	err                 error

	compose           ComposeModel
	replyPostID       string           // postID set when compose opens
	replyParentID     string           // parentReplyID set when compose opens (empty = top-level)
	editingReplyID    string           // non-empty while m.compose is editing this reply rather than composing a new one
	editPanel         PostComposePanel // post-edit panel; PostDetail has no "new post" panel, only this edit-mode one
	relaxed           bool             // true = blank lines between post, header, and replies
	loc               *time.Location   // timezone for timestamp display; nil = UTC
	timeDisplayFormat string           // API setting: "datetime", "relative", "unix", "swatch"

	currentUsername        string        // set after login; guards the delete key to own content
	currentUserIsSupporter bool          // set after login/profile load; edit requires supporter status
	confirming             pdConfirmKind // pending delete confirmation
	maxThreadDepth         int           // max visual nesting depth; 0 treated as 3

	flagPrompt        FlagPrompt // active while reporting the selected post or reply
	flagTargetPostID  string     // set when flagging the post itself
	flagTargetReplyID string     // set when flagging a reply (flagTargetPostID also set, as the reply's parent)

	bookmarkedPostIDs  map[string]struct{}
	bookmarkedReplyIDs map[string]struct{}
	watchedPostIDs     map[string]struct{}

	// postTheme is the parsed theme block from the post's body, if any —
	// computed once in SetPost rather than on every keystroke/render. Nil
	// means no theme block was detected.
	postTheme *theme.Palette
}

func NewPostDetailModel() PostDetailModel {
	return PostDetailModel{
		compose:    NewComposeModel(0),
		editPanel:  NewPostComposePanel(0),
		flagPrompt: NewFlagPrompt(),
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
	if p, ok := theme.ParsePost(post.Content); ok {
		m.postTheme = &p
	} else {
		m.postTheme = nil
	}
	if m.ready {
		m = m.refreshContent()
		m.viewport.GotoTop()
	}
	return m
}

// currentTheme returns the detected theme block for whatever's currently
// selected — the post itself (cached in postTheme, computed once in
// SetPost), or the selected reply's content, parsed on demand. Reply content
// is small and the regex scan is cheap, so this isn't cached per selection
// the way postTheme is.
func (m PostDetailModel) currentTheme() *theme.Palette {
	if m.selectedReply < 0 {
		return m.postTheme
	}
	if m.selectedReply >= len(m.flatTree) {
		return nil
	}
	if p, ok := theme.ParsePost(m.flatTree[m.selectedReply].Reply.Content); ok {
		return &p
	}
	return nil
}

// HasTheme reports whether the currently selected post or reply contains a
// detected custom-theme block (see docs/40-custom-themes.md).
func (m PostDetailModel) HasTheme() bool { return m.currentTheme() != nil }

// HasPost reports whether a post is currently open or persisted in the
// background — used by activateScreen to decide whether returning to the
// origin tab should resume it instead of that tab's own list.
func (m PostDetailModel) HasPost() bool { return m.post.ID != "" }

// PostID returns the currently open post's ID (empty if none) — used by
// App to detect a repliesLoadedMsg superseded by navigating to a different
// post before the request resolved.
func (m PostDetailModel) PostID() string { return m.post.ID }

// Close resets PostDetailModel back to "no post open" — called on Esc or on
// re-navigating to the post's own origin tab (the escape hatch out of a
// persisted PostDetail — see activateScreen). Preserves layout/broadcast-only
// fields (width/height/ready/relaxed/loc/timeDisplayFormat/currentUsername/
// the bookmark+watch ID sets) since those aren't re-supplied by the next
// SetPost either — only the SetPost-adjacent fields plus compose/confirming
// (which SetPost itself doesn't reset) are cleared.
func (m PostDetailModel) Close() PostDetailModel {
	m.post = model.Post{}
	m.replies = nil
	m.flatTree = nil
	m.replyOffsets = nil
	m.replyHeights = nil
	m.postHeight = 0
	m.selectedReply = -1
	m.loading = false
	m.err = nil
	m.compose = m.compose.Close()
	m.confirming = pdConfirmNone
	m.flagPrompt = NewFlagPrompt()
	m.flagTargetPostID, m.flagTargetReplyID = "", ""
	return m
}

func (m PostDetailModel) SetReplies(replies []model.Reply) PostDetailModel {
	m.selectedReply = -1 // keep post selected after replies load
	m.loading = false
	m.err = nil
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

// ComposeActive reports whether the compose box, the flag/report overlay, or
// the delete-confirmation overlay is open. Every screen-owned overlay that
// intercepts keys first in Update must be OR'd in here — app.go's global
// shortcuts fire instead of reaching Update whenever this returns false.
func (m PostDetailModel) ComposeActive() bool {
	return m.compose.IsActive() || m.editPanel.IsActive() || m.flagPrompt.Active() || m.confirming != pdConfirmNone
}

// EditPanelActive reports whether the post-edit panel specifically (not the
// reply compose box) is open, for app.go to decide whether ctrl+g should set
// a native post attachment instead of inserting markdown into the reply box.
func (m PostDetailModel) EditPanelActive() bool { return m.editPanel.IsActive() }

// ReplyComposeActive reports whether the reply compose box specifically
// (not the edit panel) is open — see app.go's applyAttachURL, which warns
// instead of inserting here: the reply API has no attachments field at all.
func (m PostDetailModel) ReplyComposeActive() bool { return m.compose.IsActive() }

// SetEditPanelAttachment sets the edit panel's pending image/gif attachment URL.
func (m PostDetailModel) SetEditPanelAttachment(url string) PostDetailModel {
	m.editPanel = m.editPanel.SetAttachmentURL(url)
	return m
}

func (m PostDetailModel) SetError(err error) PostDetailModel {
	m.err = err
	m.loading = false
	if m.ready {
		m = m.refreshContent()
	}
	return m
}

// SetCurrentUsername records the logged-in user's username so PostDetail can
// restrict the delete key to the user's own posts and replies.
func (m PostDetailModel) SetCurrentUsername(username string) PostDetailModel {
	m.currentUsername = username
	return m
}

func (m PostDetailModel) SetCurrentUserIsSupporter(isSupporter bool) PostDetailModel {
	m.currentUserIsSupporter = isSupporter
	return m
}

// CanEditSelected reports whether the currently selected post or reply is the
// current user's own, published within the edit window, and the account is a
// supporter — the same gate applied to the 'e' keypress, reused by the status
// bar to show/hide the hint live as the selection or clock changes.
func (m PostDetailModel) CanEditSelected() bool {
	if !m.currentUserIsSupporter {
		return false
	}
	if m.selectedReply == -1 {
		return m.post.ID != "" && m.post.AuthorUsername == m.currentUsername && time.Since(m.post.CreatedAt) < postEditWindow
	}
	if m.selectedReply >= 0 && m.selectedReply < len(m.flatTree) {
		r := m.flatTree[m.selectedReply].Reply
		return r.AuthorUsername == m.currentUsername && time.Since(r.CreatedAt) < postEditWindow
	}
	return false
}

// ApplyPostEdit overwrites the edited fields of the displayed post after a
// successful PATCH, leaving AuthorID, CreatedAt, RepliesCount, etc. untouched.
// ApplyPostEdit overwrites the edited fields of the open post after a
// successful PATCH. Attachments is only applied when attachmentsTouched —
// see FeedModel.ApplyPostEdit's doc comment for why.
func (m PostDetailModel) ApplyPostEdit(content, title string, topics []string, isPublic, isNSFW bool, editedAt time.Time, attachments []model.Attachment, attachmentsTouched bool) PostDetailModel {
	m.post.Content = content
	m.post.Title = title
	m.post.Topics = topics
	m.post.IsPublic = isPublic
	m.post.IsNSFW = isNSFW
	m.post.EditedAt = editedAt
	if attachmentsTouched {
		m.post.Attachments = attachments
	}
	if m.ready {
		m = m.refreshContent()
	}
	return m
}

// ApplyReplyEdit overwrites a reply's content by ID after a successful PATCH.
func (m PostDetailModel) ApplyReplyEdit(replyID, content string, editedAt time.Time) PostDetailModel {
	for i, r := range m.replies {
		if r.ID == replyID {
			r.Content = content
			r.EditedAt = editedAt
			m.replies[i] = r
			m.flatTree = buildReplyTree(m.replies, m.effectiveMaxDepth())
			break
		}
	}
	if m.ready {
		m = m.refreshContent()
	}
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
	if m.editPanel.IsActive() {
		h -= m.editPanel.PanelHeight()
	}
	if m.confirming != pdConfirmNone {
		h -= confirmBoxHeight
	}
	if m.flagPrompt.Active() {
		h -= m.flagPrompt.Height()
	}
	if h < 1 {
		h = 1
	}
	return h
}

func (m PostDetailModel) refreshContent() PostDetailModel {
	content, offsets, heights, postH, postImgs, replyImgs := m.buildContent()
	m.replyOffsets = offsets
	m.replyHeights = heights
	m.postHeight = postH
	m.postImages = postImgs
	m.replyImages = replyImgs
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
		fullPost, _ := m.renderFullPost(true)
		itemHeight = lipgloss.Height(fullPost)
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
	case InsertIconMsg:
		if m.compose.IsActive() {
			m.compose = m.compose.InsertText(msg.Icon)
		}
		return m, nil

	case SharedConfigMsg:
		m.timeDisplayFormat = msg.Settings.TimeDisplayFormat
		imagesChanged := msg.InlineImagesEnabled != m.inlineImagesEnabled
		m.inlineImagesEnabled = msg.InlineImagesEnabled
		m = m.SetRelaxed(msg.Relaxed)
		m = m.SetLocation(msg.Loc)
		if msg.MaxThreadDepth != m.maxThreadDepth {
			m.maxThreadDepth = msg.MaxThreadDepth
			m = m.applyReplies(m.replies)
		} else if imagesChanged && m.ready {
			m = m.refreshContent()
		}
		return m, nil

	case BookmarkedIDsMsg:
		m.bookmarkedPostIDs = msg.PostIDs
		m.bookmarkedReplyIDs = msg.ReplyIDs
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
		m.compose = m.compose.SetWidth(msg.Width)
		m.editPanel = m.editPanel.SetWidth(msg.Width)
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
		if m.editPanel.IsActive() {
			content := msg.Content
			postID := m.post.ID
			title := m.editPanel.TitleValue()
			topics := ParseTopics(m.editPanel.TopicsRaw())
			isPublic := m.editPanel.IsPublic()
			isNSFW := m.editPanel.IsNSFW()
			attachmentURL := m.editPanel.AttachmentURL()
			attachmentTouched := m.editPanel.AttachmentTouched()
			otherAttachments := m.editPanel.OtherAttachments()
			m.editPanel = m.editPanel.Close()
			if m.ready {
				m.viewport.Height = m.viewportHeight()
			}
			return m, func() tea.Msg {
				return SubmitPostEditMsg{PostID: postID, Content: content, Title: title, Topics: topics, IsPublic: isPublic, IsNSFW: isNSFW, AttachmentURL: attachmentURL, AttachmentTouched: attachmentTouched, OtherAttachments: otherAttachments}
			}
		}
		content := msg.Content
		postID := m.replyPostID
		if m.editingReplyID != "" {
			replyID := m.editingReplyID
			m.editingReplyID = ""
			m.compose = m.compose.Close()
			if m.ready {
				m.viewport.Height = m.viewportHeight()
			}
			return m, func() tea.Msg {
				return SubmitReplyEditMsg{ReplyID: replyID, PostID: postID, Content: content}
			}
		}
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
		m.editPanel = m.editPanel.Close()
		m.editingReplyID = ""
		if m.ready {
			m.viewport.Height = m.viewportHeight()
		}
		return m, nil

	case FlagSubmitMsg:
		postID, replyID := m.flagTargetPostID, m.flagTargetReplyID
		m.flagTargetPostID, m.flagTargetReplyID = "", ""
		if m.ready {
			m.viewport.Height = m.viewportHeight()
		}
		if replyID != "" {
			return m, func() tea.Msg {
				return FlagReplyMsg{ReplyID: replyID, PostID: postID, Reason: msg.Reason}
			}
		}
		return m, func() tea.Msg { return FlagPostMsg{PostID: postID, Reason: msg.Reason} }

	case FlagCancelMsg:
		m.flagTargetPostID, m.flagTargetReplyID = "", ""
		if m.ready {
			m.viewport.Height = m.viewportHeight()
		}
		return m, nil

	case tea.KeyMsg:
		// Flag overlay intercepts all keys while active.
		if m.flagPrompt.Active() {
			var cmd tea.Cmd
			m.flagPrompt, cmd = m.flagPrompt.Update(msg)
			return m, cmd
		}
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

		// When the post-edit panel is open, all key events go to it.
		if m.editPanel.IsActive() {
			prevH := m.editPanel.PanelHeight()
			var cmd tea.Cmd
			m.editPanel, cmd = m.editPanel.Update(msg)
			if m.editPanel.PanelHeight() != prevH && m.ready {
				m.viewport.Height = m.viewportHeight()
			}
			return m, cmd
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
		case "e":
			if !m.CanEditSelected() {
				return m, nil
			}
			if m.selectedReply == -1 {
				var cmd tea.Cmd
				m.editPanel, cmd = m.editPanel.OpenForEdit(m.post)
				m.viewport.Height = m.viewportHeight()
				return m, cmd
			}
			reply := m.flatTree[m.selectedReply].Reply
			m.editingReplyID = reply.ID
			m.replyPostID = m.post.ID
			var cmd tea.Cmd
			m.compose, cmd = m.compose.OpenWithContent("editing reply", "write your reply…", reply.Content)
			m.viewport.Height = m.viewportHeight()
			return m, cmd
		case "!":
			if m.selectedReply == -1 {
				if m.post.ID != "" && m.post.AuthorUsername != m.currentUsername {
					m.flagTargetPostID = m.post.ID
					var cmd tea.Cmd
					m.flagPrompt, cmd = m.flagPrompt.Open(FlagKindPost)
					m.viewport.Height = m.viewportHeight()
					return m, cmd
				}
			} else if m.selectedReply >= 0 && m.selectedReply < len(m.flatTree) {
				reply := m.flatTree[m.selectedReply].Reply
				if reply.AuthorUsername != m.currentUsername {
					m.flagTargetPostID = m.post.ID
					m.flagTargetReplyID = reply.ID
					var cmd tea.Cmd
					m.flagPrompt, cmd = m.flagPrompt.Open(FlagKindReply)
					m.viewport.Height = m.viewportHeight()
					return m, cmd
				}
			}
			return m, nil
		case "r":
			var cmd tea.Cmd
			m, cmd = m.OpenCompose()
			return m, cmd
		case "T":
			if p := m.currentTheme(); p != nil {
				return m, func() tea.Msg { return PreviewPostThemeMsg{Palette: *p} }
			}
			return m, nil
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
		case "c":
			if m.post.ID == "" {
				return m, nil
			}
			var username string
			if m.selectedReply >= 0 && m.selectedReply < len(m.flatTree) {
				username = m.flatTree[m.selectedReply].Reply.AuthorUsername
			} else {
				username = m.post.AuthorUsername
			}
			return m, func() tea.Msg { return StartConversationMsg{Username: username} }
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
		case "w":
			// Watch applies to the thread root only — ignore when a reply is focused.
			if m.selectedReply < 0 && m.post.ID != "" {
				postID := m.post.ID
				return m, func() tea.Msg { return ToggleWatchPostMsg{PostID: postID} }
			}
			return m, nil
		case "l":
			// A reply has no URL of its own — always link to the parent post,
			// regardless of whether the post or one of its replies is selected.
			if m.post.ID != "" {
				return m, func() tea.Msg { return CopyLinkMsg{Post: m.post} }
			}
			return m, nil
		case "up", "k":
			newReply, newOffset := millerPageNav(-1, m.viewport.Height, m.postHeight,
				m.replyOffsets, m.replyHeights, m.selectedReply, m.viewport.YOffset)
			if newReply != m.selectedReply {
				m.selectedReply = newReply
				m = m.refreshContent()
			}
			m.viewport.SetYOffset(newOffset)
			return m, nil
		case "down", "j":
			newReply, newOffset := millerPageNav(+1, m.viewport.Height, m.postHeight,
				m.replyOffsets, m.replyHeights, m.selectedReply, m.viewport.YOffset)
			if newReply != m.selectedReply {
				m.selectedReply = newReply
				m = m.refreshContent()
			}
			m.viewport.SetYOffset(newOffset)
			return m, nil
		case "pgup":
			newReply, newOffset := m.selectedReply, m.viewport.YOffset
			for i := 0; i < m.viewport.Height && newReply > -1; i++ {
				newReply, newOffset = millerPageNav(-1, m.viewport.Height, m.postHeight,
					m.replyOffsets, m.replyHeights, newReply, newOffset)
			}
			if newReply != m.selectedReply {
				m.selectedReply = newReply
				m = m.refreshContent()
			}
			m.viewport.SetYOffset(newOffset)
			return m, nil
		case "pgdown":
			newReply, newOffset := m.selectedReply, m.viewport.YOffset
			for i := 0; i < m.viewport.Height && newReply < len(m.replyOffsets)-1; i++ {
				newReply, newOffset = millerPageNav(+1, m.viewport.Height, m.postHeight,
					m.replyOffsets, m.replyHeights, newReply, newOffset)
			}
			if newReply != m.selectedReply {
				m.selectedReply = newReply
				m = m.refreshContent()
			}
			m.viewport.SetYOffset(newOffset)
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
// the rendered height of each reply, and the rendered height of the post block.
// Heights are measured here once so that ensureSelectedVisible and the pager
// always use the same values that were used to lay out the content.
func (m PostDetailModel) buildContent() (content string, offsets []int, heights []int, postH int, postImgs []postImageSlot, replyImgs [][]postImageSlot) {
	postContent, postImgs := m.renderFullPost(m.selectedReply == -1)
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

	postH = lipgloss.Height(postContent)

	if m.loading {
		sb.WriteString(theme.Subtle.Render("  loading replies…"))
		sb.WriteString("\n")
		return sb.String(), nil, nil, postH, postImgs, nil
	}
	if len(m.replies) == 0 {
		sb.WriteString(theme.Subtle.Render("  no replies yet"))
		sb.WriteString("\n")
		return sb.String(), nil, nil, postH, postImgs, nil
	}

	// Base line where first reply starts.
	// Relaxed: post + blank + header + blank = H_post+1+H_header+1
	// Dense:   post + header (no blank lines) = H_post+H_header
	var baseLines int
	if m.relaxed {
		baseLines = postH + 1 + lipgloss.Height(repliesHeader) + 1
	} else {
		baseLines = postH + lipgloss.Height(repliesHeader)
	}
	offsets = make([]int, len(m.flatTree))
	heights = make([]int, len(m.flatTree))
	replyImgs = make([][]postImageSlot, len(m.flatTree))
	currentLine := baseLines

	for i, node := range m.flatTree {
		offsets[i] = currentLine
		rendered, replyImgsForNode := m.renderReply(node, i == m.selectedReply)
		replyImgs[i] = replyImgsForNode
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

	return sb.String(), offsets, heights, postH, postImgs, replyImgs
}

func (m PostDetailModel) renderFullPost(selected bool) (string, []postImageSlot) {
	innerWidth := m.width - 4

	_, postBookmarked := m.bookmarkedPostIDs[m.post.ID]
	_, postWatched := m.watchedPostIDs[m.post.ID]
	left := lipgloss.JoinHorizontal(lipgloss.Top,
		theme.Highlight.Render("@"+m.post.AuthorUsername),
		theme.Subtle.Render("  "+displayTime(m.post.CreatedAt, m.location(), m.timeDisplayFormat, false)+editedSuffix(m.post.EditedAt)),
	) + imageIcon(m.post.Attachments, m.post.Content) + audioIcon(m.post.Attachments) + bookmarkIcon(postBookmarked) + watchIcon(postWatched)
	header := left

	// Badges line: guild indicator, nsfw, public — omitted when none apply.
	var badgeParts []string
	if m.post.IsGuildThread && m.post.GuildSlug != "" {
		badgeParts = append(badgeParts, theme.Subtle.Render("[#"+m.post.GuildSlug+"]"))
	}
	if m.post.IsNSFW {
		badgeParts = append(badgeParts, theme.Error.Render("[nsfw]"))
	}
	if m.post.IsPublic {
		badgeParts = append(badgeParts, theme.Subtle.Render("[public]"))
	}
	badges := strings.Join(badgeParts, "  ")

	rows := []string{header}
	if badges != "" {
		rows = append(rows, badges)
	}
	if m.post.Title != "" {
		rows = append(rows, theme.Title.Render(m.post.Title))
	}

	body, imgSlots := m.renderBodyWithInlineImage(m.post.Content, innerWidth, 1+lipgloss.Height(lipgloss.JoinVertical(lipgloss.Left, rows...)))
	if att := renderAttachments(m.post.Attachments); att != "" {
		body = body + "\n" + att
	}

	var topicsSB strings.Builder
	for _, t := range m.post.Topics {
		topicsSB.WriteString(theme.Subtle.Render("#"+t) + " ")
	}
	topics := topicsSB.String()

	boxStyle := theme.Border
	if selected {
		boxStyle = theme.ActiveBorder
	}
	if innerWidth > 0 {
		boxStyle = boxStyle.Width(m.width - 2)
	}

	rows = append(rows, body, fmt.Sprintf("\n%s", topics))
	return boxStyle.Render(lipgloss.JoinVertical(lipgloss.Left, rows...)), imgSlots
}

// renderBodyWithInlineImage renders content at innerWidth, splicing in an
// inlineImageBandRows-tall reserved band (spacer + inlineImageMaxRows image
// rows + spacer) in place of each eligible image's placeholder line, when
// inline images are enabled. lineBase is the number of lines that will
// precede body in the card this is embedded in (border top row +
// header/badges/title), used to translate each image's body-local line into
// a card-local one. Returns the plain-Render output, unchanged, when inline
// images are disabled or no eligible image is found.
//
// Splicing shifts every line after the insertion point down by
// (inlineImageBandRows - 1); processing hits in ascending/document order and
// accumulating that shift as we go means each hit's original Line, plus the
// shift accumulated from only the earlier hits already spliced in, is always
// the correct current insertion point — no need to re-scan or recompute
// anything after the fact.
func (m PostDetailModel) renderBodyWithInlineImage(content string, innerWidth, lineBase int) (string, []postImageSlot) {
	if !m.inlineImagesEnabled {
		return markdown.Render(content, innerWidth), nil
	}
	rendered, hits := markdown.RenderLocatingImages(content, innerWidth)
	if len(hits) == 0 {
		return rendered, nil
	}
	lines, slots := spliceInlineImageBands(strings.Split(rendered, "\n"), hits, lineBase)
	return strings.Join(lines, "\n"), slots
}

func (m PostDetailModel) renderReply(node replyNode, selected bool) (string, []postImageSlot) {
	indentW := node.Depth * 3
	cardWidth := m.width - 2 - indentW
	innerWidth := cardWidth - 2

	headerParts := []string{theme.Highlight.Render("@" + node.Reply.AuthorUsername)}
	if node.ParentUsername != "" {
		headerParts = append(headerParts, theme.Subtle.Render("  ↩ @"+node.ParentUsername))
	}
	headerParts = append(headerParts,
		theme.Subtle.Render("  "+displayTime(node.Reply.CreatedAt, m.location(), m.timeDisplayFormat, false)+editedSuffix(node.Reply.EditedAt)),
	)
	_, replyBookmarked := m.bookmarkedReplyIDs[node.Reply.ID]
	left := lipgloss.JoinHorizontal(lipgloss.Top, headerParts...) + imageIcon(node.Reply.Attachments, node.Reply.Content) + audioIcon(node.Reply.Attachments) + bookmarkIcon(replyBookmarked)
	header := left

	// lineBase: 1 border-top row + 1 header row (header is always a single
	// JoinHorizontal line here, unlike the full post's optional badges/title).
	body, imgSlots := m.renderBodyWithInlineImage(node.Reply.Content, innerWidth, 2)
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
		return lipgloss.NewStyle().MarginLeft(indentW).Render(card), imgSlots
	}
	return card, imgSlots
}

func (m PostDetailModel) View() string {
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

	if m.flagPrompt.Active() {
		return lipgloss.JoinVertical(lipgloss.Left,
			m.viewport.View(),
			m.flagPrompt.View(m.width),
		)
	}

	if m.compose.IsActive() {
		return lipgloss.JoinVertical(lipgloss.Left,
			m.viewport.View(),
			m.compose.View(),
		)
	}
	if m.editPanel.IsActive() {
		return lipgloss.JoinVertical(lipgloss.Left,
			m.viewport.View(),
			m.editPanel.View(),
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

// VisibleInlineImages returns the inline image slots (post + replies)
// currently fully within the viewport, top to bottom. It's purely a "where,
// if anywhere" query — App's rendering step owns fetching, encoding, and any
// placement/cache state; this just reports positions for the current frame.
// An image only counts as visible when its entire reserved row band fits in
// [YOffset, YOffset+Height) — no partial-visibility clipping (see the plan).
func (m PostDetailModel) VisibleInlineImages() []InlineImageSlot {
	if !m.ready || !m.inlineImagesEnabled {
		return nil
	}
	top, bottom := m.viewport.YOffset, m.viewport.YOffset+m.viewport.Height

	var slots []InlineImageSlot
	for i, img := range m.postImages {
		if img.Line < top || img.Line+inlineImageMaxRows > bottom {
			continue
		}
		slots = append(slots, InlineImageSlot{
			URL:       img.URL,
			Row:       img.Line - top,
			ColIndent: 2,
			MaxCols:   m.width - 4,
			MaxRows:   inlineImageEncodeMaxRows,
			Key:       fmt.Sprintf("post:%s:%d", m.post.ID, i),
		})
	}
	for i, node := range m.flatTree {
		if i >= len(m.replyImages) {
			continue
		}
		indentW := node.Depth * 3
		for j, img := range m.replyImages[i] {
			abs := m.replyOffsets[i] + img.Line
			if abs < top || abs+inlineImageMaxRows > bottom {
				continue
			}
			slots = append(slots, InlineImageSlot{
				URL:       img.URL,
				Row:       abs - top,
				ColIndent: 2 + indentW,
				MaxCols:   m.width - 4 - indentW,
				MaxRows:   inlineImageEncodeMaxRows,
				Key:       fmt.Sprintf("reply:%s:%d", node.Reply.ID, j),
			})
		}
	}
	return slots
}
