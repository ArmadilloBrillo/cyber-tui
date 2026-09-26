package screens

import (
	"fmt"
	"slices"
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

// Internal view state for the Topics screen
type topicsView int

const (
	viewTopicList topicsView = iota
	viewTopicPosts
)

// topicFilter controls which topics the topic-list view shows. Session-only —
// it resets to topicFilterAll on relaunch, like scroll position. 'f' cycles it.
type topicFilter int

const (
	topicFilterAll topicFilter = iota
	topicFilterHideMuted
	topicFilterOnlyMuted
)

type TopicsModel struct {
	view topicsView

	// Topic list state
	topics           []model.Topic
	topicIndex       int
	topicsNextCursor string
	topicsExhausted  bool
	topicFilter      topicFilter // 'f' cycles all -> hide muted -> only muted

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
	mutedTopics       map[string]struct{} // Settings.MutedTopics; posts tagged with any are hidden, rows show a marker
}

func NewTopicsModel() TopicsModel {
	return TopicsModel{}
}

func (m TopicsModel) visiblePosts() []model.Post {
	if !m.filterNSFW && len(m.mutedTopics) == 0 {
		return m.posts
	}
	out := m.posts[:0:0]
	for _, p := range m.posts {
		if m.filterNSFW && p.IsNSFW {
			continue
		}
		if topicMuted(p.Topics, m.mutedTopics) {
			continue
		}
		out = append(out, p)
	}
	return out
}

// visibleTopics returns the topic-list rows shown for the current topicFilter.
//
//   - topicFilterHideMuted drops muted topics from the fetched pages.
//   - topicFilterOnlyMuted is built from m.mutedTopics (Settings.MutedTopics) —
//     the authoritative muted set, not just whatever's on a loaded page — so it's
//     complete even for a muted topic whose page was never fetched. The real
//     model.Topic is reused where a page has it; the rest are synthesized as
//     {Slug}. The muted view renders "MUTED" in place of the post count, so a
//     zero PostCount on a synthesized entry is never shown.
func (m TopicsModel) visibleTopics() []model.Topic {
	switch m.topicFilter {
	case topicFilterHideMuted:
		out := m.topics[:0:0]
		for _, t := range m.topics {
			if _, muted := m.mutedTopics[t.Slug]; !muted {
				out = append(out, t)
			}
		}
		return out
	case topicFilterOnlyMuted:
		have := make(map[string]model.Topic, len(m.topics))
		for _, t := range m.topics {
			have[t.Slug] = t
		}
		slugs := make([]string, 0, len(m.mutedTopics))
		for s := range m.mutedTopics {
			slugs = append(slugs, s)
		}
		slices.Sort(slugs) // stable display order; map iteration is random
		out := make([]model.Topic, len(slugs))
		for i, s := range slugs {
			if t, ok := have[s]; ok {
				out[i] = t
			} else {
				out[i] = model.Topic{Slug: s}
			}
		}
		return out
	default:
		return m.topics
	}
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

func (m TopicsModel) SetTopicPosts(posts []model.Post, cursor string) TopicsModel {
	m.err = nil
	m.posts = posts
	m.postIndex = 0
	m.nextCursor = cursor
	m.exhausted = cursor == ""
	m.loading = false
	m.fetching = false
	m.refreshing = false
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
		if !sameMutedSet(m.mutedTopics, msg.Settings.MutedTopics) {
			m.mutedTopics = mutedSet(msg.Settings.MutedTopics)
			m.postIndex = 0
			// A filtered topic list can grow or shrink when the muted set
			// changes; keep the selection in range.
			if n := len(m.visibleTopics()); m.topicIndex >= n {
				m.topicIndex = max(0, n-1)
			}
		}
		m.inlineImagesEnabled = msg.InlineImagesEnabled
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
					return m, nil
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
				if m.topicIndex < len(m.visibleTopics())-1 {
					m.topicIndex++
					m = m.refreshContent()
					m = m.ensureSelectedVisible()
				} else if m.topicFilter != topicFilterOnlyMuted && !m.topicsExhausted && !m.loading {
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
					return m, nil
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
					return m, nil
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
				if m.topicIndex < len(m.visibleTopics())-1 {
					m.topicIndex = min(len(m.visibleTopics())-1, m.topicIndex+pageJumpItems)
					m = m.refreshContent()
					m = m.ensureSelectedVisible()
				} else if m.topicFilter != topicFilterOnlyMuted && !m.topicsExhausted && !m.loading {
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
					return m, nil
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
				if visible := m.visibleTopics(); len(visible) > 0 && m.topicIndex < len(visible) {
					slug := visible[m.topicIndex].Slug
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

		case "f":
			// Cycle the topic-list filter: all -> hide muted -> only muted.
			if m.view == viewTopicList {
				m.topicFilter = (m.topicFilter + 1) % 3
				m.topicIndex = 0
				m = m.refreshContent()
				m.viewport.GotoTop()
			}
			return m, nil

		case "m":
			// Mute / unmute the highlighted topic. Only in the topic list — the
			// post list has no single "current topic" to act on.
			if visible := m.visibleTopics(); m.view == viewTopicList && len(visible) > 0 && m.topicIndex < len(visible) {
				slug := visible[m.topicIndex].Slug
				next := toggleMuted(m.mutedTopics, slug)
				m.mutedTopics = mutedSet(next) // optimistic: marker updates now
				// A hide/only filter can drop the row we just toggled.
				if n := len(m.visibleTopics()); m.topicIndex >= n {
					m.topicIndex = max(0, n-1)
				}
				m = m.refreshContent()
				m = m.ensureSelectedVisible()
				return m, func() tea.Msg { return SetMutedTopicsMsg{Topics: next} }
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
	if m.view == viewTopicList && m.topicFilter != topicFilterAll {
		label := "  filter: hiding muted"
		if m.topicFilter == topicFilterOnlyMuted {
			label = "  filter: muted only"
		}
		prefix += theme.Subtle.Render(label) + "\n"
		startLine++
	}

	if m.view == viewTopicList {
		visible := m.visibleTopics()
		if len(visible) == 0 {
			if m.err != nil {
				return prefix + theme.Subtle.Render("  couldn't load topics"), nil, nil
			}
			if len(m.topics) > 0 && m.topicFilter != topicFilterAll {
				return prefix + theme.Subtle.Render("  no topics match this filter"), nil, nil
			}
			return prefix + theme.Subtle.Render("  no topics yet"), nil, nil
		}
		offsets := make([]int, len(visible))
		currentLine := startLine
		var out string
		for i, t := range visible {
			offsets[i] = currentLine
			rendered := m.renderTopicItem(t, i == m.topicIndex)
			out += rendered + sep
			currentLine += lipgloss.Height(rendered) + lineInc - 1
		}
		// Footer — only-muted is fully synthesized, so it's always "exhausted".
		out += listFooter(m.loading, (m.topicsExhausted || m.topicFilter == topicFilterOnlyMuted) && len(visible) > 0)
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

func (m TopicsModel) renderTopicItem(topic model.Topic, isSelected bool) string {
	isSelected = isSelected && m.view == viewTopicList

	innerWidth := m.width - 4

	icon := theme.Subtle.Render("#") + " "
	slugStyle := theme.Base
	if isSelected {
		slugStyle = theme.Highlight
	}
	slugStr := slugStyle.Render(topic.Slug)
	countStr := theme.Subtle.Render(fmt.Sprintf("%d posts", topic.PostCount))
	if _, muted := m.mutedTopics[topic.Slug]; muted {
		countStr = theme.Error.Render("MUTED")
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

func (m TopicsModel) ensureSelectedVisible() TopicsModel {
	if !m.ready || len(m.itemOffsets) == 0 {
		return m
	}

	var selectedIndex int
	var itemHeight int
	if m.view == viewTopicList {
		visible := m.visibleTopics()
		selectedIndex = m.topicIndex
		if selectedIndex >= len(visible) {
			return m
		}
		itemHeight = lipgloss.Height(m.renderTopicItem(visible[selectedIndex], true))
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

// ActiveTopicName returns the slug of the currently active topic.
func (m TopicsModel) ActiveTopicName() string { return m.activeTopic }

// OpenTopic marks slug as the active topic, mirroring what pressing enter on
// a topic-list row does. Callers still need to dispatch the post-list load
// themselves (e.g. via LoadTopicPostsMsg) — this only sets the selection.
func (m TopicsModel) OpenTopic(slug string) TopicsModel {
	m.activeTopic = slug
	return m
}

// --- Helpers ---
// truncate is defined in cmail.go; using same pattern here
// Rather than import it, we inline a simple implementation in renderTopicItem
