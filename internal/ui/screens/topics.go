package screens

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
)

// Message types emitted by Topics screen to App
type RefreshTopicsMsg struct{}

type LoadTopicPostsMsg struct{ Slug string }

type LoadMoreTopicPostsMsg struct {
	Slug   string
	Cursor string
}

type RefreshTopicPostsMsg struct{ Slug string }

type ShowTopicPostMsg struct{ Post model.Post }

type LoadMoreTopicsMsg struct{ Cursor string }

type LoadTopicThreadMsg struct{ PostID string }

type TopicThreadRepliesMsg struct {
	PostID  string
	Replies []model.Reply
}

// TopicThreadDebounceMsg is the delayed message emitted after topicThreadDebounceDelay
// when the selected post changes. The fetch only proceeds if PostID still matches
// the current selection, dropping stale ticks from rapid navigation.
type TopicThreadDebounceMsg struct{ PostID string }

const topicThreadDebounceDelay = time.Second

type TopicThreadNavMsg struct {
	Delta      int
	PaneHeight int
	PaneWidth  int
}

// Internal view state for the Topics screen
type topicsView int

const (
	viewTopicList topicsView = iota
	viewTopicPosts
)

type TopicsModel struct {
	view topicsView

	// Topic list state
	topics           []model.Topic
	topicIndex       int
	topicsNextCursor string
	topicsExhausted  bool

	// Topic posts state
	activeTopic string
	posts       []model.Post
	postIndex   int
	nextCursor  string
	exhausted   bool
	loading     bool
	fetching    bool // true while the initial (or tab-switch) load is in flight
	refreshing  bool
	loaded      bool

	// Miller 3-pane thread state
	threadPostID       string
	maxThreadDepth     int
	threadReplies      []model.Reply
	threadFlatTree     []replyNode
	threadReplyIndex   int
	threadScrollOffset int
	threadLoading      bool

	// Shared
	viewport    viewport.Model
	itemOffsets []int
	width       int

	// postImages is parallel to itemOffsets when view == viewTopicPosts —
	// each post's inline image slots in its own card-local line coordinates,
	// populated only when inlineImagesEnabled is true (see feed.go's
	// postImages field for the same convention).
	postImages          [][]postImageSlot
	inlineImagesEnabled bool

	bookmarkedPostIDs map[string]struct{}
	watchedPostIDs    map[string]struct{}
	height            int
	ready             bool
	err               error
	loc               *time.Location
	relaxed           bool
	timeDisplayFormat string
	filterNSFW        bool
	blockedTopics     map[string]struct{} // Settings.MutedTopics; posts tagged with any are hidden, rows show a marker

	// postBodyCache/replyBodyCache memoize the Miller detail pane's post
	// card and thread replies (cachedPostCard/cachedReplyCard, render.go) so
	// an unrelated re-render doesn't re-parse markdown for content that
	// hasn't changed — mirrors FeedModel.bodyCache/ChatroomsModel.chatBodyCache.
	postBodyCache  map[string]feedBodyCacheEntry
	replyBodyCache map[string]replyBodyCacheEntry
}

func NewTopicsModel() TopicsModel {
	return TopicsModel{
		threadReplyIndex: -1,
		postBodyCache:    make(map[string]feedBodyCacheEntry),
		replyBodyCache:   make(map[string]replyBodyCacheEntry),
	}
}

func (m TopicsModel) visiblePosts() []model.Post {
	if !m.filterNSFW && len(m.blockedTopics) == 0 {
		return m.posts
	}
	out := m.posts[:0:0]
	for _, p := range m.posts {
		if m.filterNSFW && p.IsNSFW {
			continue
		}
		if topicBlocked(p.Topics, m.blockedTopics) {
			continue
		}
		out = append(out, p)
	}
	return out
}

func (m TopicsModel) IsLoaded() bool { return m.loaded }

func (m TopicsModel) SetFetching() TopicsModel {
	m.fetching = true
	m.err = nil
	if m.ready {
		m = m.refreshContent()
	}
	return m
}

func (m TopicsModel) SetTopics(items []model.Topic, cursor string) TopicsModel {
	m.err = nil
	m.topics = items
	m.topicIndex = 0
	m.topicsNextCursor = cursor
	m.topicsExhausted = cursor == ""
	m.loading = false
	m.fetching = false
	m.refreshing = false
	m.loaded = true
	if m.ready {
		m = m.refreshContent()
		m.viewport.GotoTop()
	}
	return m
}

func (m TopicsModel) AppendTopics(items []model.Topic, cursor string) TopicsModel {
	m.err = nil
	m.topics = append(m.topics, items...)
	m.topicsNextCursor = cursor
	m.topicsExhausted = cursor == ""
	m.loading = false
	m.fetching = false
	if m.ready {
		m = m.refreshContent()
	}
	return m
}

// evictStalePostBodyCache drops postBodyCache entries for posts no longer
// present in m.posts — mirrors FeedModel.evictStaleBodyCache (feed.go).
func (m TopicsModel) evictStalePostBodyCache() TopicsModel {
	live := make(map[string]bool, len(m.posts))
	for _, p := range m.posts {
		live[p.ID] = true
	}
	for id := range m.postBodyCache {
		if !live[id] {
			delete(m.postBodyCache, id)
		}
	}
	return m
}

// evictStaleReplyBodyCache drops replyBodyCache entries for replies no
// longer present in m.threadReplies — called whenever a fresh reply page
// replaces the thread (TopicThreadRepliesMsg), the point a reply can
// permanently drop out of the loaded thread.
func (m TopicsModel) evictStaleReplyBodyCache() TopicsModel {
	live := make(map[string]bool, len(m.threadReplies))
	for _, r := range m.threadReplies {
		live[r.ID] = true
	}
	for id := range m.replyBodyCache {
		if !live[id] {
			delete(m.replyBodyCache, id)
		}
	}
	return m
}

func (m TopicsModel) SetTopicPosts(posts []model.Post, cursor string) TopicsModel {
	m.err = nil
	m.posts = posts
	m.postIndex = 0
	m.nextCursor = cursor
	m.exhausted = cursor == ""
	m.loading = false
	m.fetching = false
	m.refreshing = false
	m = m.evictStalePostBodyCache()
	m.view = viewTopicPosts
	if m.ready {
		m = m.refreshContent()
		m.viewport.GotoTop()
	}
	return m
}

func (m TopicsModel) AppendTopicPosts(posts []model.Post, cursor string) TopicsModel {
	m.posts = append(m.posts, posts...)
	m.nextCursor = cursor
	m.exhausted = cursor == ""
	m.loading = false
	m.fetching = false
	if m.ready {
		m = m.refreshContent()
	}
	return m
}

func (m TopicsModel) SetError(err error) TopicsModel {
	m.err = err
	m.loading = false
	m.fetching = false
	m.refreshing = false
	if m.ready {
		m = m.refreshContent()
	}
	return m
}

func (m TopicsModel) Init() tea.Cmd { return nil }

func (m TopicsModel) Update(msg tea.Msg) (TopicsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case SharedConfigMsg:
		m.relaxed = msg.Relaxed
		if msg.Loc != nil {
			m.loc = msg.Loc
		}
		m.timeDisplayFormat = msg.Settings.TimeDisplayFormat
		if msg.Settings.FilterNSFW != m.filterNSFW {
			m.filterNSFW = msg.Settings.FilterNSFW
			m.postIndex = 0
		}
		if !sameBlockedSet(m.blockedTopics, msg.Settings.MutedTopics) {
			m.blockedTopics = blockedSet(msg.Settings.MutedTopics)
			m.postIndex = 0
		}
		m.inlineImagesEnabled = msg.InlineImagesEnabled
		if m.ready {
			m = m.refreshContent()
		}
		if msg.MaxThreadDepth != m.maxThreadDepth {
			m.maxThreadDepth = msg.MaxThreadDepth
			if len(m.threadReplies) > 0 {
				m.threadFlatTree = buildReplyTree(m.threadReplies, m.effectiveMaxDepth())
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

	case TopicThreadRepliesMsg:
		if msg.PostID == m.threadPostID {
			m.threadReplies = msg.Replies
			m.threadFlatTree = buildReplyTree(msg.Replies, m.effectiveMaxDepth())
			m.threadReplyIndex = -1
			m.threadScrollOffset = 0
			m.threadLoading = false
			m = m.evictStaleReplyBodyCache()
		}
		return m, nil

	case TopicThreadDebounceMsg:
		visible := m.visiblePosts()
		if m.postIndex < len(visible) && visible[m.postIndex].ID == msg.PostID {
			m.threadLoading = true
			return m, func() tea.Msg { return LoadTopicThreadMsg(msg) }
		}
		return m, nil

	case TopicThreadNavMsg:
		if msg.PaneHeight > 0 && msg.PaneWidth > 0 {
			m = m.pageThreadNav(msg.Delta, msg.PaneHeight, msg.PaneWidth)
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
			if m.view == viewTopicList {
				if m.topicIndex > 0 {
					m.topicIndex--
					m = m.refreshContent()
					m = m.ensureSelectedVisible()
				}
			} else if m.view == viewTopicPosts {
				if m.postIndex > 0 {
					m.postIndex--
					m = m.refreshContent()
					m = m.ensureSelectedVisible()
					var detailCmd tea.Cmd
					m, detailCmd = m.currentDetailCmd()
					return m, detailCmd
				} else if !m.loading && !m.refreshing {
					slug := m.activeTopic
					m.refreshing = true
					m = m.refreshContent()
					return m, func() tea.Msg { return RefreshTopicPostsMsg{Slug: slug} }
				}
			}
			return m, nil

		case "down", "j":
			if m.view == viewTopicList {
				if m.topicIndex < len(m.topics)-1 {
					m.topicIndex++
					m = m.refreshContent()
					m = m.ensureSelectedVisible()
				} else if !m.topicsExhausted && !m.loading {
					m.loading = true
					m = m.refreshContent()
					m.viewport.ScrollDown(1)
					return m, func() tea.Msg {
						return LoadMoreTopicsMsg{Cursor: m.topicsNextCursor}
					}
				}
			} else if m.view == viewTopicPosts {
				if m.postIndex < len(m.visiblePosts())-1 {
					m.postIndex++
					m = m.refreshContent()
					m = m.ensureSelectedVisible()
					var detailCmd tea.Cmd
					m, detailCmd = m.currentDetailCmd()
					return m, detailCmd
				} else if !m.exhausted && !m.loading {
					m.loading = true
					m = m.refreshContent()
					m.viewport.ScrollDown(1)
					return m, func() tea.Msg {
						return LoadMoreTopicPostsMsg{Slug: m.activeTopic, Cursor: m.nextCursor}
					}
				}
			}
			return m, nil

		case "pgup":
			if m.view == viewTopicList {
				if m.topicIndex > 0 {
					m.topicIndex = max(0, m.topicIndex-pageJumpItems)
					m = m.refreshContent()
					m = m.ensureSelectedVisible()
				}
			} else if m.view == viewTopicPosts {
				if m.postIndex > 0 {
					m.postIndex = max(0, m.postIndex-pageJumpItems)
					m = m.refreshContent()
					m = m.ensureSelectedVisible()
					var detailCmd tea.Cmd
					m, detailCmd = m.currentDetailCmd()
					return m, detailCmd
				} else if !m.loading && !m.refreshing {
					slug := m.activeTopic
					m.refreshing = true
					m = m.refreshContent()
					return m, func() tea.Msg { return RefreshTopicPostsMsg{Slug: slug} }
				}
			}
			return m, nil

		case "pgdown":
			if m.view == viewTopicList {
				if m.topicIndex < len(m.topics)-1 {
					m.topicIndex = min(len(m.topics)-1, m.topicIndex+pageJumpItems)
					m = m.refreshContent()
					m = m.ensureSelectedVisible()
				} else if !m.topicsExhausted && !m.loading {
					m.loading = true
					m = m.refreshContent()
					m.viewport.ScrollDown(1)
					return m, func() tea.Msg {
						return LoadMoreTopicsMsg{Cursor: m.topicsNextCursor}
					}
				}
			} else if m.view == viewTopicPosts {
				if m.postIndex < len(m.visiblePosts())-1 {
					m.postIndex = min(len(m.visiblePosts())-1, m.postIndex+pageJumpItems)
					m = m.refreshContent()
					m = m.ensureSelectedVisible()
					var detailCmd tea.Cmd
					m, detailCmd = m.currentDetailCmd()
					return m, detailCmd
				} else if !m.exhausted && !m.loading {
					m.loading = true
					m = m.refreshContent()
					m.viewport.ScrollDown(1)
					return m, func() tea.Msg {
						return LoadMoreTopicPostsMsg{Slug: m.activeTopic, Cursor: m.nextCursor}
					}
				}
			}
			return m, nil

		case "enter":
			if m.view == viewTopicList {
				if len(m.topics) > 0 && m.topicIndex < len(m.topics) {
					slug := m.topics[m.topicIndex].Slug
					m.activeTopic = slug
					return m, func() tea.Msg { return LoadTopicPostsMsg{Slug: slug} }
				}
			} else {
				if visible := m.visiblePosts(); len(visible) > 0 && m.postIndex < len(visible) {
					post := visible[m.postIndex]
					return m, func() tea.Msg { return ShowTopicPostMsg{Post: post} }
				}
			}
			return m, nil

		case "b":
			// Block / unblock the highlighted topic. Only in the topic list —
			// the post list has no single "current topic" to act on.
			if m.view == viewTopicList && len(m.topics) > 0 && m.topicIndex < len(m.topics) {
				slug := m.topics[m.topicIndex].Slug
				next := toggleBlocked(m.blockedTopics, slug)
				m.blockedTopics = blockedSet(next) // optimistic: marker updates now
				m = m.refreshContent()
				return m, func() tea.Msg { return SetBlockedTopicsMsg{Topics: next} }
			}
			return m, nil

		case "esc":
			if m.view == viewTopicPosts {
				m.view = viewTopicList
				m.activeTopic = ""
				m = m.refreshContent()
				m.viewport.GotoTop()
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)

	// Check if user scrolled to bottom
	if m.view == viewTopicPosts && m.viewport.AtBottom() && !m.exhausted && !m.loading {
		m.loading = true
		m = m.refreshContent()
		m.viewport.ScrollDown(1)
		return m, func() tea.Msg {
			return LoadMoreTopicPostsMsg{Slug: m.activeTopic, Cursor: m.nextCursor}
		}
	}

	return m, cmd
}

func (m TopicsModel) View() string {
	if !m.ready {
		return theme.Subtle.Render("loading topics...")
	}
	return m.viewport.View()
}

// buildContent returns the rendered viewport content, the per-item line
// offsets, and — only when view == viewTopicPosts — each post's inline
// image slots parallel to offsets (nil in the topic-list view).
func (m TopicsModel) buildContent() (string, []int, [][]postImageSlot) {
	if m.fetching {
		return theme.Subtle.Render("  loading topics…"), nil, nil
	}
	sep := "\n"
	lineInc := 1
	if m.relaxed {
		sep = "\n\n"
		lineInc = 2
	}

	// State indicators
	var prefix string
	startLine := 0
	if m.refreshing {
		prefix = theme.Subtle.Render("  refreshing…") + "\n"
		startLine++
	}

	if m.view == viewTopicList {
		if len(m.topics) == 0 {
			if m.err != nil {
				return prefix + theme.Subtle.Render("  couldn't load topics"), nil, nil
			}
			return prefix + theme.Subtle.Render("  no topics yet"), nil, nil
		}
		offsets := make([]int, len(m.topics))
		currentLine := startLine
		var out string
		for i := range m.topics {
			offsets[i] = currentLine
			rendered := m.renderTopicItem(i)
			out += rendered + sep
			currentLine += lipgloss.Height(rendered) + lineInc - 1
		}
		// Footer
		out += listFooter(m.loading, m.topicsExhausted && len(m.topics) > 0)
		return prefix + strings.TrimRight(out, "\n"), offsets, nil
	}

	// viewTopicPosts
	if len(m.posts) == 0 {
		if m.err != nil {
			return prefix + theme.Subtle.Render("  couldn't load posts"), nil, nil
		}
		return prefix + theme.Subtle.Render("  no posts"), nil, nil
	}
	visible := m.visiblePosts()
	offsets := make([]int, len(visible))
	postImages := make([][]postImageSlot, len(visible))
	currentLine := startLine
	var out string
	for i, p := range visible {
		offsets[i] = currentLine
		rendered, imgSlots := m.renderPostItem(p, i == m.postIndex)
		postImages[i] = imgSlots
		out += rendered + sep
		currentLine += lipgloss.Height(rendered) + lineInc - 1
	}
	// Footer
	out += listFooter(m.loading, m.exhausted)
	return prefix + strings.TrimRight(out, "\n"), offsets, postImages
}

func (m TopicsModel) renderTopicItem(index int) string {
	if index < 0 || index >= len(m.topics) {
		return ""
	}

	topic := m.topics[index]
	isSelected := (m.view == viewTopicList && index == m.topicIndex)

	innerWidth := m.width - 4

	icon := theme.Subtle.Render("#") + " "
	slugStyle := theme.Base
	if isSelected {
		slugStyle = theme.Highlight
	}
	slugStr := slugStyle.Render(topic.Slug)
	countStr := theme.Subtle.Render(fmt.Sprintf("%d posts", topic.PostCount))
	if _, blocked := m.blockedTopics[topic.Slug]; blocked {
		countStr = theme.Error.Render("BLOCKED")
	}

	var line string
	if innerWidth > 0 {
		gap := innerWidth - lipgloss.Width(icon) - lipgloss.Width(slugStr) - lipgloss.Width(countStr)
		if gap > 0 {
			line = icon + slugStr + strings.Repeat(" ", gap) + countStr
		} else {
			line = icon + slugStr
		}
	} else {
		line = icon + slugStr
	}

	boxStyle := theme.Border
	if isSelected {
		boxStyle = theme.ActiveBorder
	}
	if innerWidth > 0 {
		boxStyle = boxStyle.Width(m.width - 2)
	}
	return boxStyle.Render(line)
}

func (m TopicsModel) renderPostItem(p model.Post, selected bool) (string, []postImageSlot) {
	_, bookmarked := m.bookmarkedPostIDs[p.ID]
	_, watched := m.watchedPostIDs[p.ID]
	return RenderPost(p, selected, bookmarked, watched, m.width, m.location(), m.timeDisplayFormat, postMaxBodyLines, m.inlineImagesEnabled)
}

func (m TopicsModel) refreshContent() TopicsModel {
	content, offsets, postImages := m.buildContent()
	m.itemOffsets = offsets
	m.postImages = postImages
	m.viewport.SetContent(content)
	return m.ensureSelectedVisible()
}

// SelectedPostID returns the ID of the currently selected topic post, or ""
// when browsing the topic list or nothing is selected — used by App to
// detect a selection-only move (see FeedModel.SelectedPostID's doc comment).
func (m TopicsModel) SelectedPostID() string {
	if m.view != viewTopicPosts {
		return ""
	}
	visible := m.visiblePosts()
	if m.postIndex < 0 || m.postIndex >= len(visible) {
		return ""
	}
	return visible[m.postIndex].ID
}

// VisibleInlineImages returns the inline image slots currently fully within
// the viewport, top to bottom, across every visible topic post — see
// PostDetailModel.VisibleInlineImages for the full contract.
func (m TopicsModel) VisibleInlineImages() []InlineImageSlot {
	if !m.ready || !m.inlineImagesEnabled || m.view != viewTopicPosts {
		return nil
	}
	visible := m.visiblePosts()
	top, bottom := m.viewport.YOffset, m.viewport.YOffset+m.viewport.Height

	var slots []InlineImageSlot
	for i, p := range visible {
		if i >= len(m.postImages) || i >= len(m.itemOffsets) {
			continue
		}
		for j, img := range m.postImages[i] {
			abs := m.itemOffsets[i] + img.Line
			if abs < top || abs+inlineImageMaxRows > bottom {
				continue
			}
			slots = append(slots, InlineImageSlot{
				URL:       img.URL,
				Row:       abs - top,
				ColIndent: 2,
				MaxCols:   m.width - 4,
				MaxRows:   inlineImageEncodeMaxRows,
				Key:       fmt.Sprintf("topicpost:%s:%d", p.ID, j),
			})
		}
	}
	return slots
}

// VisibleDetailInlineImages returns the inline image slots for the selected
// post card in Miller's reading pane (topic post replies aren't
// inline-image-aware — renderDetailReply renders plain markdown, so there's
// nothing to report there). width/height must match what MillerLayout passed
// to DetailView this frame — see FeedModel.VisibleDetailInlineImages for why
// this recomputes rather than caching.
func (m TopicsModel) VisibleDetailInlineImages(width, height int) []InlineImageSlot {
	if !m.ready || !m.inlineImagesEnabled {
		return nil
	}
	visible := m.visiblePosts()
	if m.postIndex >= len(visible) {
		return nil
	}
	p := visible[m.postIndex]
	_, bookmarked := m.bookmarkedPostIDs[p.ID]
	_, watched := m.watchedPostIDs[p.ID]
	postSelected := m.threadReplyIndex < 0
	_, imgSlots := cachedPostCard(m.postBodyCache, p, postSelected, bookmarked, watched, width, m.location(), m.timeDisplayFormat, 0, true)
	if len(imgSlots) == 0 {
		return nil
	}
	top := m.threadScrollOffset
	bottom := top + height
	var slots []InlineImageSlot
	for j, img := range imgSlots {
		if img.Line < top || img.Line+inlineImageMaxRows > bottom {
			continue
		}
		slots = append(slots, InlineImageSlot{
			URL:       img.URL,
			Row:       img.Line - top,
			ColIndent: 2,
			MaxCols:   width - 4,
			MaxRows:   inlineImageEncodeMaxRows,
			Key:       fmt.Sprintf("topicpost:%s:%d", p.ID, j),
		})
	}
	return slots
}

func (m TopicsModel) ensureSelectedVisible() TopicsModel {
	if !m.ready || len(m.itemOffsets) == 0 {
		return m
	}

	var selectedIndex int
	var itemHeight int
	if m.view == viewTopicList {
		selectedIndex = m.topicIndex
		if selectedIndex >= len(m.topics) {
			return m
		}
		itemHeight = lipgloss.Height(m.renderTopicItem(selectedIndex))
	} else {
		visible := m.visiblePosts()
		selectedIndex = m.postIndex
		if selectedIndex >= len(visible) {
			return m
		}
		rendered, _ := m.renderPostItem(visible[selectedIndex], false)
		itemHeight = lipgloss.Height(rendered)
	}

	if selectedIndex >= len(m.itemOffsets) {
		return m
	}

	itemStart := m.itemOffsets[selectedIndex]
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

func (m TopicsModel) viewportHeight() int {
	return m.height - theme.ChromeHeight
}

func (m TopicsModel) location() *time.Location {
	if m.loc == nil {
		return time.UTC
	}
	return m.loc
}

// IsBrowsingTopic reports whether the user is viewing a specific topic's posts.
func (m TopicsModel) IsBrowsingTopic() bool { return m.activeTopic != "" }

// GetFocusedURLs implements URLProvider. Returns URLs from the selected post when
// in post-list view; returns nil when browsing the topic list.
func (m TopicsModel) GetFocusedURLs() []string {
	if m.view != viewTopicPosts {
		return nil
	}
	visible := m.visiblePosts()
	if m.postIndex < 0 || m.postIndex >= len(visible) {
		return nil
	}
	p := visible[m.postIndex]
	return append(extractURLs(p.Content), attachmentURLs(p.Attachments)...)
}

// IsViewingTopicPosts reports whether the topic post list is currently shown (3-pane applies).
func (m TopicsModel) IsViewingTopicPosts() bool { return m.view == viewTopicPosts }

// ActiveTopicName returns the slug of the currently active topic.
func (m TopicsModel) ActiveTopicName() string { return m.activeTopic }

// OpenTopic marks slug as the active topic, mirroring what pressing enter on
// a topic-list row does. Callers still need to dispatch the post-list load
// themselves (e.g. via LoadTopicPostsMsg) — this only sets the selection.
func (m TopicsModel) OpenTopic(slug string) TopicsModel {
	m.activeTopic = slug
	return m
}

func (m TopicsModel) IsCompactListActive() bool { return m.IsViewingTopicPosts() }
func (m TopicsModel) ListTitle() string          { return "posts (# " + m.ActiveTopicName() + ")" }

// IsAtTop reports whether the first post is selected.
func (m TopicsModel) IsAtTop() bool { return m.postIndex == 0 }

// PostCount returns the number of currently visible topic posts.
func (m TopicsModel) PostCount() int { return len(m.visiblePosts()) }

// PostsNextCursor returns the pagination cursor for the next page of topic posts.
func (m TopicsModel) PostsNextCursor() string { return m.nextCursor }

func (m TopicsModel) effectiveMaxDepth() int {
	if m.maxThreadDepth <= 0 {
		return 3
	}
	return m.maxThreadDepth
}

// currentDetailCmd clears the detail pane immediately and starts a debounce timer.
// The API fetch only fires if the selection hasn't changed by the time the timer expires,
// avoiding a flood of calls when the user scrolls quickly through the post list.
func (m TopicsModel) currentDetailCmd() (TopicsModel, tea.Cmd) {
	visible := m.visiblePosts()
	if m.postIndex >= len(visible) {
		return m, nil
	}
	postID := visible[m.postIndex].ID
	if postID == m.threadPostID {
		return m, nil
	}
	m.threadPostID = postID
	m.threadLoading = false
	m.threadReplies = nil
	m.threadFlatTree = nil
	m.threadReplyIndex = -1
	m.threadScrollOffset = 0
	return m, tea.Tick(topicThreadDebounceDelay, func(time.Time) tea.Msg {
		return TopicThreadDebounceMsg{PostID: postID}
	})
}

// CurrentDetailCmd is exported so app.go can trigger the initial detail load when topic posts first arrive.
// Loads immediately without debounce.
func (m TopicsModel) CurrentDetailCmd() (TopicsModel, tea.Cmd) {
	visible := m.visiblePosts()
	if m.postIndex >= len(visible) {
		return m, nil
	}
	postID := visible[m.postIndex].ID
	if postID == m.threadPostID {
		return m, nil
	}
	m.threadPostID = postID
	m.threadLoading = true
	m.threadReplies = nil
	m.threadFlatTree = nil
	m.threadReplyIndex = -1
	m.threadScrollOffset = 0
	return m, func() tea.Msg { return LoadTopicThreadMsg{PostID: postID} }
}

func (m TopicsModel) renderDetailReply(node replyNode, selected bool, width int) string {
	return cachedReplyCard(m.replyBodyCache, node, selected, width, m.location(), m.timeDisplayFormat)
}

func (m TopicsModel) renderCompactPost(p model.Post, selected bool, width int) string {
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
	prefixWidth := 2 + lipgloss.Width(username) + len(sep)
	remaining := width - prefixWidth
	if remaining > 1 {
		preview = ansiTruncate(preview, remaining)
	} else {
		preview = ""
	}
	return indicatorAndName + theme.Subtle.Render(sep+preview)
}

// CompactListView returns the compact single-line topic post list for the Miller list pane.
func (m TopicsModel) CompactListView(width, height int) string {
	if !m.ready || m.fetching {
		return theme.Subtle.Render("  loading…")
	}
	visible := m.visiblePosts()
	if len(visible) == 0 {
		return theme.Subtle.Render("  no posts")
	}
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
	offset := m.postIndex - listH + 1
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
	lines := make([]string, 0, end-offset+headerLines)
	if header != "" {
		lines = append(lines, header)
	}
	for i := offset; i < end; i++ {
		lines = append(lines, m.renderCompactPost(visible[i], i == m.postIndex, width))
	}
	return strings.Join(lines, "\n")
}

// pageThreadNav implements pager-style scrolling for the Miller detail pane.
func (m TopicsModel) pageThreadNav(delta, paneH, paneW int) TopicsModel {
	visible := m.visiblePosts()
	if m.postIndex >= len(visible) {
		return m
	}
	p := visible[m.postIndex]
	_, bookmarked := m.bookmarkedPostIDs[p.ID]
	_, watched := m.watchedPostIDs[p.ID]

	postCard, _ := cachedPostCard(m.postBodyCache, p, false, bookmarked, watched, paneW, m.location(), m.timeDisplayFormat, 0, m.inlineImagesEnabled)
	postH := lipgloss.Height(postCard)

	replyStarts := make([]int, len(m.threadFlatTree))
	replyHeights := make([]int, len(m.threadFlatTree))
	pos := postH
	for i, node := range m.threadFlatTree {
		replyStarts[i] = pos
		rendered := m.renderDetailReply(node, false, paneW)
		replyHeights[i] = lipgloss.Height(rendered)
		pos += replyHeights[i]
	}

	m.threadReplyIndex, m.threadScrollOffset = millerPageNav(
		delta, paneH, postH, replyStarts, replyHeights, m.threadReplyIndex, m.threadScrollOffset,
	)
	return m
}

// DetailView returns the full topic post card + threaded replies for the Miller reading pane.
func (m TopicsModel) DetailView(width, height int) string {
	if !m.ready {
		return theme.Subtle.Render("  loading…")
	}
	visible := m.visiblePosts()
	if len(visible) == 0 {
		return theme.Subtle.Render("  no posts")
	}
	if m.postIndex >= len(visible) {
		return theme.Subtle.Render("  select a post")
	}
	p := visible[m.postIndex]
	_, bookmarked := m.bookmarkedPostIDs[p.ID]
	_, watched := m.watchedPostIDs[p.ID]

	postSelected := m.threadReplyIndex < 0
	card, _ := cachedPostCard(m.postBodyCache, p, postSelected, bookmarked, watched, width, m.location(), m.timeDisplayFormat, 0, m.inlineImagesEnabled)

	var parts []string
	startLines := []int{0}
	lineCount := lipgloss.Height(card)
	parts = append(parts, card)

	if m.threadLoading {
		parts = append(parts, theme.Subtle.Render("  loading replies…"))
	} else {
		for i, node := range m.threadFlatTree {
			rendered := m.renderDetailReply(node, i == m.threadReplyIndex, width)
			startLines = append(startLines, lineCount)
			lineCount += lipgloss.Height(rendered)
			parts = append(parts, rendered)
		}
	}

	fullContent := lipgloss.JoinVertical(lipgloss.Left, parts...)
	return sliceContent(fullContent, m.threadScrollOffset, height, lineCount)
}

// --- Helpers ---
// truncate is defined in cmail.go; using same pattern here
// Rather than import it, we inline a simple implementation in renderTopicItem
