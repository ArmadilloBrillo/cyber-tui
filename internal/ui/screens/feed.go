package screens

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/markdown"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
)

// LoadMoreFeedMsg is emitted by FeedModel when the viewport reaches the bottom
// and a next-page cursor is available. App intercepts this and fires the API call.
type LoadMoreFeedMsg struct{ Cursor string }

// RefreshFeedMsg is emitted when the user presses up at the top of the feed.
// App intercepts this and re-fetches the feed from the start.
type RefreshFeedMsg struct{}

// ShowPostMsg is emitted when the user presses Enter on a selected post.
type ShowPostMsg struct{ Post model.Post }

// ShowPostForReplyMsg is emitted when the user presses 'r' on a selected post.
// App navigates to post detail and opens the compose box immediately.
type ShowPostForReplyMsg struct{ Post model.Post }

// LoadFeedDetailMsg is emitted when the selected post changes so the app can
// fetch replies for the Miller reading pane.
type LoadFeedDetailMsg struct{ PostID string }

// FeedDetailRepliesMsg delivers fetched replies back to FeedModel for the reading pane.
type FeedDetailRepliesMsg struct {
	PostID  string
	Replies []model.Reply
}

// FeedDetailNavMsg is emitted by the Miller layout when the user presses j/k in
// focusDetail. PaneHeight and PaneWidth are the detail column dimensions so the
// handler can implement pager-style line-by-line scrolling.
type FeedDetailNavMsg struct {
	Delta      int
	PaneHeight int
	PaneWidth  int
}

// FeedDetailDebounceMsg is the delayed message emitted after feedDetailDebounceDelay
// when the selected post changes. The fetch only proceeds if PostID still matches
// the current selection, dropping stale ticks from rapid navigation.
type FeedDetailDebounceMsg struct{ PostID string }

const feedDetailDebounceDelay = time.Second

// mergePendingTickMsg fires after feedMergeAnimDelay to complete a pending-new
// merge, giving the local merge (no network round-trip) the same brief
// "fetching new posts..." transition as a real refresh.
type mergePendingTickMsg struct{}

const feedMergeAnimDelay = 200 * time.Millisecond

// SubmitNewPostMsg is emitted when the user submits a new post from the Feed.
type SubmitNewPostMsg struct {
	Content  string
	Title    string // empty = no title
	Slug     string // empty = server-generated
	Topics   []string
	IsPublic bool
	IsNSFW   bool
}

type FeedModel struct {
	posts             []model.Post
	postOffsets       []int // start line of each post within the viewport content
	viewport          viewport.Model
	panel             PostComposePanel
	defaultPublicPost bool // mirrored from settings; initialises panel.isPublic on each open
	width             int
	height            int
	selectedIndex     int
	ready             bool
	err               error
	nextCursor        string
	loading           bool
	fetching          bool           // true while the initial (or tab-switch) load is in flight
	refreshing        bool           // true while re-fetching newest posts (up at top)
	pendingNew        []model.Post   // new posts detected by the background poll, staged but not yet merged into posts
	pendingCapped     bool           // true when pendingNew hit the single-page poll limit — count is a floor, not exact
	loaded            bool           // true once the first page has successfully loaded
	exhausted         bool           // true once API returned an empty cursor
	relaxed           bool           // true = blank line between posts (relaxed density)
	loc               *time.Location // timezone for timestamp display; nil = UTC
	timeDisplayFormat string         // API setting: "datetime", "relative", "unix", "swatch"

	// inlineImagesEnabled mirrors SharedConfigMsg.InlineImagesEnabled — see
	// PostDetailModel's field of the same name. postImages is parallel to
	// postOffsets/visiblePosts(); only ever populated when this is true.
	inlineImagesEnabled bool
	postImages          [][]postImageSlot

	currentUsername  string // set after login; used to guard the delete key
	confirmingDelete bool   // true while the delete-post confirmation overlay is shown

	flagPrompt       FlagPrompt // active while reporting the selected post
	flagTargetPostID string

	bookmarkedPostIDs map[string]struct{}
	watchedPostIDs    map[string]struct{}
	filterNSFW        bool

	// bodyCache memoizes renderPostBody per post ID, keyed additionally by
	// whatever else affects its output — see renderPost. Selection state
	// isn't part of the key: moving the cursor only changes the border, not
	// the cached body, so arrow-key navigation doesn't re-parse markdown for
	// every loaded post.
	bodyCache map[string]feedBodyCacheEntry

	// Miller reading pane: replies for the currently selected post.
	detailPostID       string
	detailReplies      []model.Reply
	detailFlatTree     []replyNode // DFS-ordered tree built from detailReplies
	detailReplyIndex   int         // -1 = post selected; 0+ = index into detailFlatTree
	detailScrollOffset int         // raw line offset for pager scrolling in the detail pane
	detailLoading      bool
	maxThreadDepth     int
}

func NewFeedModel() FeedModel {
	return FeedModel{
		panel:            NewPostComposePanel(0),
		detailReplyIndex: -1,
		flagPrompt:       NewFlagPrompt(),
		bodyCache:        make(map[string]feedBodyCacheEntry),
	}
}

// ParseTopics splits a comma-separated topic string and caps the result at 3.
// Empty parts are ignored. Leading/trailing whitespace is trimmed. Topics are
// lowercased here — the field itself preserves whatever case was typed (see
// filterSlugCharsKeyMsg), matching the API's documented "must be lowercase"
// rule only at this submit boundary.
func ParseTopics(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		t := strings.ToLower(strings.TrimSpace(part))
		if t != "" {
			out = append(out, t)
		}
	}
	if len(out) > 3 {
		out = out[:3]
	}
	return out
}

func (m FeedModel) IsLoaded() bool     { return m.loaded }
func (m FeedModel) IsRefreshing() bool { return m.refreshing }

func (m FeedModel) SetFetching() FeedModel {
	m.fetching = true
	m.err = nil
	if m.ready {
		m = m.refreshContent()
	}
	return m
}

func (m FeedModel) SetPosts(posts []model.Post, cursor string) FeedModel {
	m.err = nil
	var prevID string
	if oldVisible := m.visiblePosts(); m.selectedIndex < len(oldVisible) {
		prevID = oldVisible[m.selectedIndex].ID
	}

	m.posts = posts
	m.nextCursor = cursor
	m.exhausted = cursor == ""
	m.loading = false
	m.fetching = false
	m.refreshing = false
	m.pendingNew = nil
	m.pendingCapped = false
	m.loaded = true
	m.selectedIndex = 0
	if prevID != "" {
		for i, p := range m.visiblePosts() {
			if p.ID == prevID {
				m.selectedIndex = i
				break
			}
		}
	}
	if m.ready {
		m = m.refreshContent()
		if m.selectedIndex == 0 {
			m.viewport.GotoTop()
		} else {
			m = m.ensureSelectedVisible()
		}
	}
	return m
}

func (m FeedModel) AppendPosts(posts []model.Post, cursor string) FeedModel {
	m.posts = append(m.posts, posts...)
	m.nextCursor = cursor
	m.exhausted = cursor == ""
	m.loading = false
	m.fetching = false
	if m.ready {
		m = m.refreshContent() // selectedIndex preserved; scroll position preserved
	}
	return m
}

// feedPeekPageSize is the page size the background poll fetches (GetFeed("")
// always requests limit=20). Used to detect when a peek page is entirely new
// posts — meaning the real count may exceed what this one page can show.
const feedPeekPageSize = 20

// SetPendingNew stages newly-detected posts (from the background feed poll)
// without inserting them into the viewport. Posts already present in m.posts
// are filtered out. Call MergePendingNew to bring them into view.
//
// If every post in the fetched page turns out to be new, the previously-known
// top post wasn't found within one page — the real count could be higher than
// what this single-request poll can see, so pendingCapped is set and the
// count is displayed as a floor ("20+") rather than an exact (and likely
// wrong) number.
func (m FeedModel) SetPendingNew(posts []model.Post) FeedModel {
	existing := make(map[string]struct{}, len(m.posts))
	for _, p := range m.posts {
		existing[p.ID] = struct{}{}
	}
	var fresh []model.Post
	for _, p := range posts {
		if _, ok := existing[p.ID]; !ok {
			fresh = append(fresh, p)
		}
	}
	m.pendingNew = fresh
	m.pendingCapped = len(posts) == feedPeekPageSize && len(fresh) == len(posts)
	if m.ready {
		m = m.refreshContent()
	}
	return m
}

// PendingNewCount reports how many staged-but-unmerged posts are pending,
// for the tab-bar badge.
func (m FeedModel) PendingNewCount() int { return len(m.pendingNew) }

// PendingNewLabel returns the "↑ load N new entries ↑" chrome message for
// the separator bar, or "" if nothing is pending. N is a floor ("N+") when
// pendingCapped is set — see SetPendingNew.
func (m FeedModel) PendingNewLabel() string {
	n := len(m.pendingNew)
	if n == 0 {
		return ""
	}
	label := strconv.Itoa(n)
	if m.pendingCapped {
		label += "+"
	}
	return fmt.Sprintf("  ↑ load %s new entries ↑", label)
}

// MergePendingNew prepends the staged posts onto the visible list and clears
// the pending count. Called when the user presses up at the top of the feed
// while entries are pending.
func (m FeedModel) MergePendingNew() FeedModel {
	if len(m.pendingNew) == 0 {
		return m
	}
	m.posts = append(append([]model.Post{}, m.pendingNew...), m.posts...)
	m.pendingNew = nil
	m.pendingCapped = false
	m.selectedIndex = 0
	if m.ready {
		m = m.refreshContent()
		m.viewport.GotoTop()
	}
	return m
}

func (m FeedModel) SetError(err error) FeedModel {
	m.err = err
	m.loading = false
	m.fetching = false
	m.refreshing = false
	if m.ready {
		m = m.refreshContent()
	}
	return m
}

// SetCurrentUsername records the logged-in user's username so the feed can
// restrict the delete key to the user's own posts.
func (m FeedModel) SetCurrentUsername(username string) FeedModel {
	m.currentUsername = username
	return m
}

// RemovePost removes a post from the local list by ID (called after a
// successful DELETE API call so the list reflects the deletion immediately).
func (m FeedModel) RemovePost(postID string) FeedModel {
	for i, p := range m.posts {
		if p.ID == postID {
			m.posts = append(m.posts[:i], m.posts[i+1:]...)
			if vis := len(m.visiblePosts()); m.selectedIndex >= vis {
				m.selectedIndex = vis - 1
				if m.selectedIndex < 0 {
					m.selectedIndex = 0
				}
			}
			break
		}
	}
	if m.ready {
		m = m.refreshContent()
	}
	return m
}

func (m FeedModel) visiblePosts() []model.Post {
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

func (m FeedModel) SetRelaxed(relaxed bool) FeedModel {
	m.relaxed = relaxed
	if m.ready {
		m = m.refreshContent()
		m = m.ensureSelectedVisible()
	}
	return m
}

func (m FeedModel) location() *time.Location {
	if m.loc == nil {
		return time.UTC
	}
	return m.loc
}

func (m FeedModel) SetLocation(loc *time.Location) FeedModel {
	if loc == nil {
		loc = time.UTC
	}
	m.loc = loc
	if m.ready {
		m = m.refreshContent()
	}
	return m
}

// refreshContent rebuilds the viewport content and updates postOffsets.
// Call this whenever posts, selectedIndex, or width changes.
func (m FeedModel) refreshContent() FeedModel {
	content, offsets, postImages := m.buildContent()
	m.postOffsets = offsets
	m.postImages = postImages
	m.viewport.SetContent(content)
	return m
}

// ensureSelectedVisible scrolls the viewport the minimum amount so the
// selected post is fully visible. If the post is taller than the viewport,
// its top is aligned with the viewport top.
func (m FeedModel) ensureSelectedVisible() FeedModel {
	visible := m.visiblePosts()
	if !m.ready || len(m.postOffsets) == 0 || m.selectedIndex >= len(visible) {
		return m
	}
	postStart := m.postOffsets[m.selectedIndex]
	rendered, _ := m.renderPost(visible[m.selectedIndex], false)
	postHeight := lipgloss.Height(rendered)
	postEnd := postStart + postHeight - 1

	viewTop := m.viewport.YOffset
	viewBottom := viewTop + m.viewport.Height - 1

	if postStart < viewTop {
		// Post is above the viewport — scroll up to its top.
		m.viewport.SetYOffset(postStart)
	} else if postEnd > viewBottom {
		// Post is (partially) below the viewport.
		if postHeight <= m.viewport.Height {
			// Post fits — align its bottom with the viewport bottom.
			m.viewport.SetYOffset(postEnd - m.viewport.Height + 1)
		} else {
			// Post is taller than the viewport — show from the top.
			m.viewport.SetYOffset(postStart)
		}
	}
	return m
}

// ComposeActive reports whether the new-post compose panel, the flag/report
// overlay, or the delete-confirmation overlay is open. Every screen-owned
// overlay that intercepts keys first in Update (see the top of the
// tea.KeyMsg case) must be OR'd in here — app.go's global shortcuts fire
// instead of reaching Update whenever this returns false.
func (m FeedModel) ComposeActive() bool {
	return m.panel.IsActive() || m.flagPrompt.Active() || m.confirmingDelete
}
func (m FeedModel) ComposeHeight() int           { return m.panel.PanelHeight() }
func (m FeedModel) ComposeView(width int) string { return m.panel.SetWidth(width).View() }

func (m FeedModel) Init() tea.Cmd { return nil }

func (m FeedModel) Update(msg tea.Msg) (FeedModel, tea.Cmd) {
	switch msg := msg.(type) {
	case SharedConfigMsg:
		m.timeDisplayFormat = msg.Settings.TimeDisplayFormat
		m.defaultPublicPost = msg.Settings.DefaultPublicPost
		imagesChanged := msg.InlineImagesEnabled != m.inlineImagesEnabled
		m.inlineImagesEnabled = msg.InlineImagesEnabled
		m = m.SetRelaxed(msg.Relaxed)
		m = m.SetLocation(msg.Loc)
		if msg.Settings.FilterNSFW != m.filterNSFW {
			m.filterNSFW = msg.Settings.FilterNSFW
			m.selectedIndex = 0
			if m.ready {
				m = m.refreshContent()
			}
		} else if imagesChanged && m.ready {
			m = m.refreshContent()
		}
		if msg.MaxThreadDepth != m.maxThreadDepth {
			m.maxThreadDepth = msg.MaxThreadDepth
			if len(m.detailReplies) > 0 {
				m.detailFlatTree = buildReplyTree(m.detailReplies, m.effectiveMaxDepth())
			}
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

	case FeedDetailRepliesMsg:
		visible := m.visiblePosts()
		if m.selectedIndex < len(visible) && visible[m.selectedIndex].ID == msg.PostID {
			m.detailPostID = msg.PostID
			m.detailReplies = msg.Replies
			m.detailFlatTree = buildReplyTree(msg.Replies, m.effectiveMaxDepth())
			m.detailReplyIndex = -1
			m.detailScrollOffset = 0
			m.detailLoading = false
		}
		return m, nil

	case FeedDetailDebounceMsg:
		visible := m.visiblePosts()
		if m.selectedIndex < len(visible) && visible[m.selectedIndex].ID == msg.PostID {
			m.detailLoading = true
			return m, func() tea.Msg { return LoadFeedDetailMsg(msg) }
		}
		return m, nil

	case mergePendingTickMsg:
		m.refreshing = false
		m = m.MergePendingNew()
		return m, nil

	case FeedDetailNavMsg:
		if msg.PaneHeight > 0 && msg.PaneWidth > 0 {
			m = m.pageDetailNav(msg.Delta, msg.PaneHeight, msg.PaneWidth)
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.panel = m.panel.SetWidth(msg.Width)
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
		title := m.panel.TitleValue()
		slug := m.panel.SlugValue()
		topics := ParseTopics(m.panel.TopicsRaw())
		isPublic := m.panel.IsPublic()
		isNSFW := m.panel.IsNSFW()
		m = m.closeCompose()
		return m, func() tea.Msg {
			return SubmitNewPostMsg{Content: content, Title: title, Slug: slug, Topics: topics, IsPublic: isPublic, IsNSFW: isNSFW}
		}

	case ComposeCancelMsg:
		m = m.closeCompose()
		return m, nil

	case FlagSubmitMsg:
		postID := m.flagTargetPostID
		m.flagTargetPostID = ""
		m.viewport.Height = m.viewportHeight()
		return m, func() tea.Msg { return FlagPostMsg{PostID: postID, Reason: msg.Reason} }

	case FlagCancelMsg:
		m.flagTargetPostID = ""
		m.viewport.Height = m.viewportHeight()
		return m, nil

	case tea.KeyMsg:
		// Flag overlay intercepts all keys while active.
		if m.flagPrompt.Active() {
			var cmd tea.Cmd
			m.flagPrompt, cmd = m.flagPrompt.Update(msg)
			return m, cmd
		}
		// Confirmation overlay intercepts all keys while active.
		if m.confirmingDelete {
			switch msg.String() {
			case "y":
				if visible := m.visiblePosts(); m.selectedIndex < len(visible) {
					postID := visible[m.selectedIndex].ID
					m.confirmingDelete = false
					m.viewport.Height = m.viewportHeight()
					return m, func() tea.Msg { return DeletePostMsg{PostID: postID} }
				}
				m.confirmingDelete = false
				m.viewport.Height = m.viewportHeight()
			case "n", "esc":
				m.confirmingDelete = false
				m.viewport.Height = m.viewportHeight()
			}
			return m, nil
		}

		if m.panel.IsActive() {
			oldH := m.panel.PanelHeight()
			var cmd tea.Cmd
			m.panel, cmd = m.panel.Update(msg)
			if m.panel.PanelHeight() != oldH {
				m.viewport.Height = m.viewportHeight()
			}
			return m, cmd
		}
		switch msg.String() {
		case "up", "k":
			if m.selectedIndex > 0 {
				m.selectedIndex--
				m = m.refreshContent()
				m = m.ensureSelectedVisible()
				var detailCmd tea.Cmd
				m, detailCmd = m.currentDetailCmd()
				return m, detailCmd
			} else if len(m.pendingNew) > 0 && !m.refreshing {
				m.refreshing = true
				m = m.refreshContent()
				return m, tea.Tick(feedMergeAnimDelay, func(time.Time) tea.Msg { return mergePendingTickMsg{} })
			} else if !m.loading && !m.refreshing {
				m.refreshing = true
				m = m.refreshContent()
				return m, func() tea.Msg { return RefreshFeedMsg{} }
			}
			return m, nil
		case "enter":
			if visible := m.visiblePosts(); len(visible) > 0 && m.selectedIndex < len(visible) {
				post := visible[m.selectedIndex]
				return m, func() tea.Msg { return ShowPostMsg{Post: post} }
			}
		case "r":
			if visible := m.visiblePosts(); len(visible) > 0 && m.selectedIndex < len(visible) {
				post := visible[m.selectedIndex]
				return m, func() tea.Msg { return ShowPostForReplyMsg{Post: post} }
			}
		case "p":
			if visible := m.visiblePosts(); len(visible) > 0 && m.selectedIndex < len(visible) {
				username := visible[m.selectedIndex].AuthorUsername
				return m, func() tea.Msg { return ShowUserProfileMsg{Username: username} }
			}
			return m, nil
		case "c":
			if visible := m.visiblePosts(); len(visible) > 0 && m.selectedIndex < len(visible) {
				username := visible[m.selectedIndex].AuthorUsername
				return m, func() tea.Msg { return StartConversationMsg{Username: username} }
			}
			return m, nil
		case "b":
			if visible := m.visiblePosts(); len(visible) > 0 && m.selectedIndex < len(visible) {
				postID := visible[m.selectedIndex].ID
				return m, func() tea.Msg { return BookmarkPostMsg{PostID: postID} }
			}
			return m, nil
		case "w":
			if visible := m.visiblePosts(); len(visible) > 0 && m.selectedIndex < len(visible) {
				postID := visible[m.selectedIndex].ID
				return m, func() tea.Msg { return ToggleWatchPostMsg{PostID: postID} }
			}
			return m, nil
		case "d":
			if visible := m.visiblePosts(); len(visible) > 0 && m.selectedIndex < len(visible) &&
				visible[m.selectedIndex].AuthorUsername == m.currentUsername {
				m.confirmingDelete = true
				m.viewport.Height = m.viewportHeight()
			}
			return m, nil
		case "!":
			if visible := m.visiblePosts(); len(visible) > 0 && m.selectedIndex < len(visible) &&
				visible[m.selectedIndex].AuthorUsername != m.currentUsername {
				m.flagTargetPostID = visible[m.selectedIndex].ID
				var cmd tea.Cmd
				m.flagPrompt, cmd = m.flagPrompt.Open(FlagKindPost)
				m.viewport.Height = m.viewportHeight()
				return m, cmd
			}
			return m, nil
		case "n":
			var cmd tea.Cmd
			m.panel, cmd = m.panel.Open(m.defaultPublicPost)
			m.viewport.Height = m.viewportHeight()
			return m, cmd
		case "down", "j":
			if m.selectedIndex < len(m.visiblePosts())-1 {
				m.selectedIndex++
				m = m.refreshContent()
				m = m.ensureSelectedVisible()
				var detailCmd tea.Cmd
				m, detailCmd = m.currentDetailCmd()
				return m, detailCmd
			} else {
				var loadCmd tea.Cmd
				m, loadCmd = m.triggerLoadMore()
				if loadCmd != nil {
					return m, loadCmd
				}
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)

	if m.ready && m.viewport.AtBottom() {
		var loadCmd tea.Cmd
		m, loadCmd = m.triggerLoadMore()
		if loadCmd != nil {
			return m, tea.Batch(cmd, loadCmd)
		}
	}

	return m, cmd
}

// viewportHeight returns the viewport height in rows, shrinking to make room
// for the compose panel and the delete-confirmation overlay when active.
func (m FeedModel) viewportHeight() int {
	h := m.height - theme.ChromeHeight
	if m.panel.IsActive() {
		h -= m.panel.PanelHeight()
	}
	if m.confirmingDelete {
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

func (m FeedModel) closeCompose() FeedModel {
	m.panel = m.panel.Close()
	m.viewport.Height = m.viewportHeight()
	return m
}

func (m FeedModel) triggerLoadMore() (FeedModel, tea.Cmd) {
	if m.loading || m.exhausted || m.nextCursor == "" {
		return m, nil
	}
	m.loading = true
	cursor := m.nextCursor
	m = m.refreshContent()
	m.viewport.ScrollDown(1)
	return m, func() tea.Msg { return LoadMoreFeedMsg{Cursor: cursor} }
}

// buildContent renders all posts into a single string for the viewport and
// returns the start line of each post so ensureSelectedVisible can scroll
// accurately, plus each post's inline image slots (parallel to offsets).
func (m FeedModel) buildContent() (string, []int, [][]postImageSlot) {
	if m.fetching {
		return theme.Subtle.Render("  loading feed…"), nil, nil
	}
	var prefix string
	startLine := 0
	if m.refreshing {
		prefix = theme.Subtle.Render("  fetching new posts...") + "\n"
		startLine = 1
	}
	if len(m.posts) == 0 {
		if m.err != nil {
			return prefix + theme.Subtle.Render("  couldn't load feed"), nil, nil
		}
		return prefix + theme.Subtle.Render("  no posts yet"), nil, nil
	}
	sep := "\n"
	lineInc := 0
	if m.relaxed {
		sep = "\n\n"
		lineInc = 1
	}
	visible := m.visiblePosts()
	offsets := make([]int, len(visible))
	postImages := make([][]postImageSlot, len(visible))
	var out strings.Builder
	out.WriteString(prefix)
	currentLine := startLine
	for i, p := range visible {
		offsets[i] = currentLine
		rendered, imgSlots := m.renderPost(p, i == m.selectedIndex)
		postImages[i] = imgSlots
		out.WriteString(rendered)
		out.WriteString(sep)
		currentLine += lipgloss.Height(rendered) + lineInc
	}
	if m.loading {
		out.WriteString(theme.Subtle.Render("  loading more…") + "\n")
	} else if m.exhausted {
		out.WriteString(theme.Subtle.Render("  — end of feed —") + "\n")
	}
	return out.String(), offsets, postImages
}

// feedBodyCacheEntry is a memoized renderPostBody result plus the inputs it
// was computed from, so a stale hit (width resize, bookmark/watch toggle, an
// edited post body, or the inline-images setting changing) can be detected
// and recomputed instead of served.
type feedBodyCacheEntry struct {
	body                string
	imgSlots            []postImageSlot
	width               int
	bookmarked          bool
	watched             bool
	content             string
	inlineImagesEnabled bool
}

func (m FeedModel) renderPost(p model.Post, selected bool) (string, []postImageSlot) {
	_, bookmarked := m.bookmarkedPostIDs[p.ID]
	_, watched := m.watchedPostIDs[p.ID]

	body, imgSlots, ok := "", []postImageSlot(nil), false
	if e, hit := m.bodyCache[p.ID]; hit && e.width == m.width && e.bookmarked == bookmarked && e.watched == watched && e.content == p.Content && e.inlineImagesEnabled == m.inlineImagesEnabled {
		body, imgSlots, ok = e.body, e.imgSlots, true
	}
	if !ok {
		body, imgSlots = renderPostBody(p, bookmarked, watched, m.width, m.location(), m.timeDisplayFormat, postMaxBodyLines, m.inlineImagesEnabled)
		m.bodyCache[p.ID] = feedBodyCacheEntry{body: body, imgSlots: imgSlots, width: m.width, bookmarked: bookmarked, watched: watched, content: p.Content, inlineImagesEnabled: m.inlineImagesEnabled}
	}

	boxStyle := theme.Border
	if selected {
		boxStyle = theme.ActiveBorder
	}
	if m.width-4 > 0 {
		boxStyle = boxStyle.Width(m.width - 2)
	}
	return boxStyle.Render(body), imgSlots
}

// currentDetailCmd clears the detail pane immediately and starts a debounce timer.
// The API fetch only fires if the selection hasn't changed by the time the timer expires,
// avoiding a flood of calls when the user scrolls quickly through the post list.
func (m FeedModel) currentDetailCmd() (FeedModel, tea.Cmd) {
	visible := m.visiblePosts()
	if m.selectedIndex >= len(visible) {
		return m, nil
	}
	postID := visible[m.selectedIndex].ID
	if postID == m.detailPostID {
		return m, nil
	}
	m.detailPostID = postID
	m.detailLoading = false
	m.detailReplies = nil
	m.detailFlatTree = nil
	m.detailReplyIndex = -1
	m.detailScrollOffset = 0
	return m, tea.Tick(feedDetailDebounceDelay, func(time.Time) tea.Msg {
		return FeedDetailDebounceMsg{PostID: postID}
	})
}

// CurrentDetailCmd is exported so app.go can trigger the initial detail load after
// the feed's first page arrives. Loads immediately without debounce.
func (m FeedModel) CurrentDetailCmd() (FeedModel, tea.Cmd) {
	visible := m.visiblePosts()
	if m.selectedIndex >= len(visible) {
		return m, nil
	}
	postID := visible[m.selectedIndex].ID
	if postID == m.detailPostID {
		return m, nil
	}
	m.detailPostID = postID
	m.detailLoading = true
	m.detailReplies = nil
	m.detailFlatTree = nil
	m.detailReplyIndex = -1
	m.detailScrollOffset = 0
	return m, func() tea.Msg { return LoadFeedDetailMsg{PostID: postID} }
}

// renderDetailPost renders the selected post's card for Miller's reading
// pane, mirroring RenderPost's border/selection styling but — unlike
// RenderPost, whose non-Feed-list callers (Search/Guilds/Topics/the
// fullscreen modal) always pass inlineImagesEnabled=false — opts into
// inline-image-aware rendering via m.inlineImagesEnabled, so Miller's
// detail pane can composite inline images the same way Feed's own list
// view and PostDetail already do.
func (m FeedModel) renderDetailPost(p model.Post, selected, bookmarked, watched bool, width int) (string, []postImageSlot) {
	content, imgSlots := renderPostBody(p, bookmarked, watched, width, m.location(), m.timeDisplayFormat, 0, m.inlineImagesEnabled)
	boxStyle := theme.Border
	if selected {
		boxStyle = theme.ActiveBorder
	}
	if width-4 > 0 {
		boxStyle = boxStyle.Width(width - 2)
	}
	return boxStyle.Render(content), imgSlots
}

// renderDetailReply renders a reply in the Miller reading pane using the same tree-aware
// card style as the post-detail screen: depth indentation, parent back-reference, active
// border. lineBase is 2 (border top row + the single header row) — reply cards, unlike
// posts, never have optional badge/title rows, so this is always fixed (see
// PostDetailModel.renderReply's identical convention).
func (m FeedModel) renderDetailReply(node replyNode, selected bool, width int) (string, []postImageSlot) {
	indentW := node.Depth * 3
	cardWidth := width - 2 - indentW
	if cardWidth < 4 {
		cardWidth = 4
	}
	innerWidth := cardWidth - 2
	if innerWidth < 1 {
		innerWidth = 1
	}

	header := theme.Highlight.Render("@" + node.Reply.AuthorUsername)
	if node.ParentUsername != "" {
		header += theme.Subtle.Render("  ↩ @" + node.ParentUsername)
	}
	header += theme.Subtle.Render("  " + displayTime(node.Reply.CreatedAt, m.location(), m.timeDisplayFormat, false))

	var body string
	var imgSlots []postImageSlot
	if m.inlineImagesEnabled {
		rendered, hits := markdown.RenderLocatingImages(node.Reply.Content, innerWidth)
		if len(hits) > 0 {
			lines, slots := spliceInlineImageBands(strings.Split(rendered, "\n"), hits, 2)
			rendered = strings.Join(lines, "\n")
			imgSlots = slots
		}
		body = strings.TrimRight(rendered, "\n")
	} else {
		body = strings.TrimRight(markdown.Render(node.Reply.Content, innerWidth), "\n")
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

// detailContent is everything Miller's reading pane needs to render, page,
// and locate inline images for the selected post + its reply tree — computed
// once so all three stay in exact agreement. Computing pager heights (or
// image positions) from separately re-rendered content risks desyncing from
// what's actually on screen, e.g. if inline images make a card taller than a
// non-image-aware height measurement would predict.
type detailContent struct {
	parts       []string // card strings: post first, then one per m.detailFlatTree entry
	startLines  []int    // startLines[i] = the line parts[i] begins at within the joined content
	lineCount   int      // total lines across all parts joined
	postH       int
	postImages  []postImageSlot
	replyImages [][]postImageSlot // parallel to m.detailFlatTree; nil while detailLoading
}

// buildDetailContent renders the selected post's card and reply tree at
// width. postSelected/selectedReply control which card gets the active
// border; callers that only need geometry (pageDetailNav) pass false/-1
// since border style never changes a card's rendered height.
func (m FeedModel) buildDetailContent(width int, postSelected bool, selectedReply int) detailContent {
	visible := m.visiblePosts()
	if m.selectedIndex >= len(visible) {
		return detailContent{}
	}
	p := visible[m.selectedIndex]
	_, bookmarked := m.bookmarkedPostIDs[p.ID]
	_, watched := m.watchedPostIDs[p.ID]

	card, postImgs := m.renderDetailPost(p, postSelected, bookmarked, watched, width)
	postH := lipgloss.Height(card)

	parts := []string{card}
	startLines := []int{0}
	lineCount := postH
	var replyImgs [][]postImageSlot
	if !m.detailLoading {
		replyImgs = make([][]postImageSlot, len(m.detailFlatTree))
		for i, node := range m.detailFlatTree {
			rendered, imgs := m.renderDetailReply(node, i == selectedReply, width)
			replyImgs[i] = imgs
			startLines = append(startLines, lineCount)
			lineCount += lipgloss.Height(rendered)
			parts = append(parts, rendered)
		}
	}
	return detailContent{parts: parts, startLines: startLines, lineCount: lineCount, postH: postH, postImages: postImgs, replyImages: replyImgs}
}

// renderCompactPost renders a single-line summary of a post for the Miller compact list pane.
// Selected: "▶ @username" in accent + preview in subtle.
// Unselected: "  @username" in base colour + preview in subtle.
func (m FeedModel) renderCompactPost(p model.Post, selected bool, width int) string {
	username := "@" + p.AuthorUsername
	var preview string
	if p.Title != "" {
		preview = p.Title
	} else {
		preview = strings.TrimSpace(strings.SplitN(p.Content, "\n", 2)[0])
	}

	const sep = "  "
	var indicatorAndName string
	if selected {
		indicatorAndName = theme.Highlight.Render("▶ " + username)
	} else {
		indicatorAndName = theme.Subtle.Render("  ") + theme.Base.Render(username)
	}
	prefixWidth := 2 + lipgloss.Width(username) + len(sep) // indicator + username + separator
	remaining := width - prefixWidth
	if remaining > 1 {
		preview = ansiTruncate(preview, remaining)
	} else {
		preview = ""
	}
	return indicatorAndName + theme.Subtle.Render(sep+preview)
}

// IsAtTop reports whether the first post is selected (used by the Miller layout to suppress
// pull-to-refresh when navigating the compact list).
func (m FeedModel) IsAtTop() bool { return m.selectedIndex == 0 }

// PostCount returns the number of currently visible posts (respects NSFW filter).
func (m FeedModel) PostCount() int { return len(m.visiblePosts()) }

// SelectedPostID returns the ID of the currently selected post in the list,
// or "" if none — used by App to detect a selection-only move that doesn't
// otherwise change VisibleInlineImages' signature (a selection change
// recolors the (de)selected card's border, including its inline-image band
// rows, without moving anything).
func (m FeedModel) SelectedPostID() string {
	visible := m.visiblePosts()
	if m.selectedIndex < 0 || m.selectedIndex >= len(visible) {
		return ""
	}
	return visible[m.selectedIndex].ID
}

// DetailSelectionKey returns the ID of whatever's selected in the Miller
// detail pane — the post itself (detailReplyIndex < 0) or the selected
// reply — so App can check whether a selection change could have recolored
// a card hosting a currently-visible inline image (see
// App.selectionTouchesSlot): slot keys embed a reply's actual ID, not its
// index into detailFlatTree, so this must return the ID to be matchable.
func (m FeedModel) DetailSelectionKey() string {
	if m.detailReplyIndex < 0 || m.detailReplyIndex >= len(m.detailFlatTree) {
		return m.SelectedPostID()
	}
	return m.detailFlatTree[m.detailReplyIndex].Reply.ID
}

// NextCursor returns the pagination cursor for the next page of feed posts.
func (m FeedModel) NextCursor() string { return m.nextCursor }

func (m FeedModel) effectiveMaxDepth() int {
	if m.maxThreadDepth <= 0 {
		return 3
	}
	return m.maxThreadDepth
}

// ansiTruncate truncates s to at most maxWidth terminal columns, appending "…" if truncated.
// Operates on plain text (no ANSI codes in post titles or raw content first lines).
func ansiTruncate(s string, maxWidth int) string {
	runes := []rune(s)
	if len(runes) <= maxWidth {
		return s
	}
	return string(runes[:maxWidth-1]) + "…"
}

func (m FeedModel) IsCompactListActive() bool { return true }
func (m FeedModel) ListTitle() string         { return "posts" }

// CompactListView returns the compact single-line post list for the Miller reading pane.
// It calculates a sticky-scroll window of height rows without storing extra state.
func (m FeedModel) CompactListView(width, height int) string {
	if !m.ready || m.fetching {
		return theme.Subtle.Render("  loading…")
	}
	visible := m.visiblePosts()
	if len(visible) == 0 {
		return theme.Subtle.Render("  no posts")
	}

	// When refreshing, reserve the first row for the status message and push
	// posts down by one row, matching the behaviour of the tabbed layout.
	headerLines := 0
	var header string
	if m.refreshing {
		header = theme.Subtle.Render("  fetching new posts...")
		headerLines = 1
	}

	n := len(visible)
	listH := height - headerLines
	if listH < 1 {
		listH = 1
	}
	// Sticky scroll: keep selectedIndex visible, scrolling so it stays at the bottom of the window.
	offset := m.selectedIndex - listH + 1
	if offset < 0 {
		offset = 0
	}
	if offset+listH > n {
		offset = n - listH
		if offset < 0 {
			offset = 0
		}
	}
	end := offset + listH
	if end > n {
		end = n
	}
	lines := make([]string, 0, end-offset)
	if header != "" {
		lines = append(lines, header)
	}
	for i := offset; i < end; i++ {
		lines = append(lines, m.renderCompactPost(visible[i], i == m.selectedIndex, width))
	}
	return strings.Join(lines, "\n")
}

// pageDetailNav implements pager-style scrolling for the Miller detail pane.
func (m FeedModel) pageDetailNav(delta, paneH, paneW int) FeedModel {
	dc := m.buildDetailContent(paneW, false, -1)
	if len(dc.parts) == 0 {
		return m
	}
	replyStarts := dc.startLines[1:]
	replyHeights := make([]int, len(dc.parts)-1)
	for i := range replyHeights {
		replyHeights[i] = lipgloss.Height(dc.parts[i+1])
	}

	m.detailReplyIndex, m.detailScrollOffset = millerPageNav(
		delta, paneH, dc.postH, replyStarts, replyHeights, m.detailReplyIndex, m.detailScrollOffset,
	)
	return m
}

// DetailView returns the full post card + threaded replies for the Miller reading pane.
// The post body is rendered without truncation (maxBodyLines = 0). The selected item
// (post or reply, determined by detailReplyIndex) is scrolled into view.
func (m FeedModel) DetailView(width, height int) string {
	if !m.ready {
		return theme.Subtle.Render("  loading…")
	}
	visible := m.visiblePosts()
	if len(visible) == 0 {
		return theme.Subtle.Render("  no posts")
	}
	if m.selectedIndex >= len(visible) {
		return theme.Subtle.Render("  select a post")
	}
	dc := m.buildDetailContent(width, m.detailReplyIndex < 0, m.detailReplyIndex)
	parts := dc.parts
	if m.detailLoading {
		parts = append(parts, theme.Subtle.Render("  loading replies…"))
	}
	fullContent := lipgloss.JoinVertical(lipgloss.Left, parts...)
	return sliceContent(fullContent, m.detailScrollOffset, height, dc.lineCount)
}

// effectiveScrollTop mirrors sliceContent's own offset clamping (see
// miller_pager.go) so callers computing positions against the *actual*
// displayed window agree with what sliceContent will show — near the
// bottom of a pane, sliceContent clamps the raw scroll offset inward, and
// using the raw offset instead would misplace inline images.
func effectiveScrollTop(offset, height, lineCount int) int {
	if lineCount <= height {
		return 0
	}
	if offset < 0 {
		offset = 0
	}
	if offset+height > lineCount {
		offset = lineCount - height
	}
	if offset < 0 {
		offset = 0
	}
	return offset
}

// VisibleDetailInlineImages returns the inline image slots currently fully
// within Miller's reading pane for the selected post + reply tree. width and
// height must match what MillerLayout passed to DetailView this frame (see
// MillerLayout.InlineImageSlots) — this recomputes buildDetailContent rather
// than caching, since the pane width Miller uses depends on the current
// list/detail split and isn't known until View() runs, and Bubble Tea's
// value-receiver View() can't persist it back onto the model for Update() to
// see next (see FeedModel.VisibleInlineImages for the Tabs-viewport analog).
func (m FeedModel) VisibleDetailInlineImages(width, height int) []InlineImageSlot {
	if !m.ready || !m.inlineImagesEnabled {
		return nil
	}
	visible := m.visiblePosts()
	if m.selectedIndex >= len(visible) {
		return nil
	}
	dc := m.buildDetailContent(width, m.detailReplyIndex < 0, m.detailReplyIndex)
	if len(dc.parts) == 0 {
		return nil
	}
	top := effectiveScrollTop(m.detailScrollOffset, height, dc.lineCount)
	bottom := top + height

	p := visible[m.selectedIndex]
	var slots []InlineImageSlot
	for j, img := range dc.postImages {
		if img.Line < top || img.Line+inlineImageMaxRows > bottom {
			continue
		}
		slots = append(slots, InlineImageSlot{
			URL:       img.URL,
			Row:       img.Line - top,
			ColIndent: 2,
			MaxCols:   width - 4,
			MaxRows:   inlineImageEncodeMaxRows,
			Key:       fmt.Sprintf("post:%s:%d", p.ID, j),
		})
	}
	for i, node := range m.detailFlatTree {
		if i >= len(dc.replyImages) || i+1 >= len(dc.startLines) {
			continue
		}
		indentW := node.Depth * 3
		replyStart := dc.startLines[i+1]
		for j, img := range dc.replyImages[i] {
			abs := replyStart + img.Line
			if abs < top || abs+inlineImageMaxRows > bottom {
				continue
			}
			slots = append(slots, InlineImageSlot{
				URL:       img.URL,
				Row:       abs - top,
				ColIndent: 2 + indentW,
				MaxCols:   width - 4 - indentW,
				MaxRows:   inlineImageEncodeMaxRows,
				Key:       fmt.Sprintf("reply:%s:%d", node.Reply.ID, j),
			})
		}
	}
	return slots
}

func (m FeedModel) View() string {
	if !m.ready {
		return theme.Subtle.Render("loading feed...")
	}

	if m.confirmingDelete {
		prompt := theme.Error.Render("Delete this post?") + "  " +
			theme.Base.Render("[y]es") + "  " +
			theme.Subtle.Render("[n]o / esc")
		promptView := theme.ActiveBorder.Width(m.width - 2).Render(prompt)
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

	if m.panel.IsActive() {
		return lipgloss.JoinVertical(lipgloss.Left,
			m.viewport.View(),
			m.panel.View(),
		)
	}
	return m.viewport.View()
}

// GetFocusedURLs implements URLProvider. Returns URLs from the selected post's content.
func (m FeedModel) GetFocusedURLs() []string {
	visible := m.visiblePosts()
	if m.selectedIndex < 0 || m.selectedIndex >= len(visible) {
		return nil
	}
	p := visible[m.selectedIndex]
	return append(extractURLs(p.Content), attachmentURLs(p.Attachments)...)
}

// VisibleInlineImages returns the inline image slots currently fully within
// the viewport, top to bottom, across every visible post — see
// PostDetailModel.VisibleInlineImages for the full contract (this is purely
// a "where, if anywhere" query; App owns fetching/encoding/caching).
func (m FeedModel) VisibleInlineImages() []InlineImageSlot {
	if !m.ready || !m.inlineImagesEnabled {
		return nil
	}
	visible := m.visiblePosts()
	top, bottom := m.viewport.YOffset, m.viewport.YOffset+m.viewport.Height

	var slots []InlineImageSlot
	for i, p := range visible {
		if i >= len(m.postImages) || i >= len(m.postOffsets) {
			continue
		}
		for j, img := range m.postImages[i] {
			abs := m.postOffsets[i] + img.Line
			if abs < top || abs+inlineImageMaxRows > bottom {
				continue
			}
			slots = append(slots, InlineImageSlot{
				URL:       img.URL,
				Row:       abs - top,
				ColIndent: 2,
				MaxCols:   m.width - 4,
				MaxRows:   inlineImageEncodeMaxRows,
				Key:       fmt.Sprintf("post:%s:%d", p.ID, j),
			})
		}
	}
	return slots
}
