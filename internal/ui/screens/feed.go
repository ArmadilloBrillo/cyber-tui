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

// CopyLinkMsg is emitted when the user presses 'l' on a selected post or
// reply. Always carries the parent post — a reply has no URL of its own,
// so callers pass its parent post's fields instead of the reply itself.
type CopyLinkMsg struct{ Post model.Post }

// mergePendingTickMsg fires after feedMergeAnimDelay to complete a pending-new
// merge, giving the local merge (no network round-trip) the same brief
// "fetching new posts..." transition as a real refresh.
type mergePendingTickMsg struct{}

const feedMergeAnimDelay = 200 * time.Millisecond

// SubmitNewPostMsg is emitted when the user submits a new post from the Feed.
type SubmitNewPostMsg struct {
	Content         string
	Title           string // empty = no title
	Slug            string // empty = server-generated
	Topics          []string
	IsPublic        bool
	IsNSFW          bool
	AudioAttachment *model.Attachment // nil = no audio attachment
}

// SubmitPostEditMsg is emitted when the user submits an edit to an existing
// post via the 'e' key (Feed or PostDetail). Unlike SubmitNewPostMsg, slug is
// not included — it's immutable once published.
type SubmitPostEditMsg struct {
	PostID            string
	Content           string
	Title             string
	Topics            []string
	IsPublic          bool
	IsNSFW            bool
	AttachmentTouched bool // false = leave existing attachments alone
	// AudioAttachment is the pending audio (YouTube) attachment, set via
	// ctrl+j — sent when AttachmentTouched.
	AudioAttachment *model.Attachment
	// OtherAttachments are attachments the edit panel found on the post but
	// doesn't manage (any type besides the one audio slot — e.g. a legacy
	// image attachment) — re-sent alongside AudioAttachment when
	// AttachmentTouched, since the API replaces the whole array.
	OtherAttachments []model.Attachment
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

	currentUsername        string // set after login; used to guard the delete key
	currentUserIsSupporter bool   // set after login/profile load; edit requires supporter status
	confirmingDelete       bool   // true while the delete-post confirmation overlay is shown
	editingPostID          string // non-empty while m.panel is editing this post rather than composing a new one

	flagPrompt       FlagPrompt // active while reporting the selected post
	flagTargetPostID string

	bookmarkedPostIDs map[string]struct{}
	watchedPostIDs    map[string]struct{}
	filterNSFW        bool
	mutedTopics       map[string]struct{} // Settings.MutedTopics; posts tagged with any are hidden

	// bodyCache memoizes renderPostBody per post ID, keyed additionally by
	// whatever else affects its output — see renderPost. Selection state
	// isn't part of the key: moving the cursor only changes the border, not
	// the cached body, so arrow-key navigation doesn't re-parse markdown for
	// every loaded post.
	bodyCache map[string]feedBodyCacheEntry
}

func NewFeedModel() FeedModel {
	return FeedModel{
		panel:      NewPostComposePanel(0),
		flagPrompt: NewFlagPrompt(),
		bodyCache:  make(map[string]feedBodyCacheEntry),
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
	m = m.evictStaleBodyCache()
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

func (m FeedModel) SetCurrentUserIsSupporter(isSupporter bool) FeedModel {
	m.currentUserIsSupporter = isSupporter
	return m
}

// postEditWindow is how long after publishing a post or reply remains
// editable, per the API's documented edit window (also supporter-gated).
const postEditWindow = 5 * time.Minute

// CanEditSelected reports whether the currently selected post is the current
// user's own, published within the edit window, and the account is a
// supporter — the same gate applied to the 'e' keypress, reused by the status
// bar to show/hide the hint live as the selection or clock changes.
func (m FeedModel) CanEditSelected() bool {
	visible := m.visiblePosts()
	if m.selectedIndex >= len(visible) {
		return false
	}
	p := visible[m.selectedIndex]
	return p.AuthorUsername == m.currentUsername && m.currentUserIsSupporter && time.Since(p.CreatedAt) < postEditWindow
}

// ApplyPostEdit overwrites the edited fields of a local post by ID after a
// successful PATCH, leaving AuthorID, CreatedAt, RepliesCount, etc.
// untouched. Attachments is only applied when attachmentsTouched — the edit
// didn't change them server-side otherwise, so the cached value is already
// correct (see EditPost's own attachmentTouched).
func (m FeedModel) ApplyPostEdit(postID, content, title string, topics []string, isPublic, isNSFW bool, editedAt time.Time, attachments []model.Attachment, attachmentsTouched bool) FeedModel {
	for i, p := range m.posts {
		if p.ID == postID {
			p.Content = content
			p.Title = title
			p.Topics = topics
			p.IsPublic = isPublic
			p.IsNSFW = isNSFW
			p.EditedAt = editedAt
			if attachmentsTouched {
				p.Attachments = attachments
			}
			m.posts[i] = p
			break
		}
	}
	if m.ready {
		m = m.refreshContent()
	}
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
func (m FeedModel) ComposeView(width int) string { return m.panel.SetWidth(width).View() }

// PanelActive reports whether the new-post/edit-post panel is open, for
// app.go to decide whether ctrl+g should warn (posts/replies no longer take
// a native image attachment) or ctrl+j should set a native song attachment.
func (m FeedModel) PanelActive() bool { return m.panel.IsActive() }

// SetPanelAudioAttachment sets the panel's pending audio (YouTube) attachment.
func (m FeedModel) SetPanelAudioAttachment(a model.Attachment) FeedModel {
	m.panel = m.panel.SetPendingAudio(&a)
	return m
}

// PanelAudioAttachment returns the panel's pending audio attachment, if any.
func (m FeedModel) PanelAudioAttachment() *model.Attachment { return m.panel.PendingAudio() }

func (m FeedModel) Init() tea.Cmd { return nil }

func (m FeedModel) Update(msg tea.Msg) (FeedModel, tea.Cmd) {
	switch msg := msg.(type) {
	case InsertIconMsg:
		if m.panel.IsActive() {
			m.panel = m.panel.InsertText(msg.Icon)
		}
		return m, nil

	case SharedConfigMsg:
		m.panel = m.panel.SetHardBreakKey(msg.HardBreakKey)
		m.timeDisplayFormat = msg.Settings.TimeDisplayFormat
		m.defaultPublicPost = msg.Settings.DefaultPublicPost
		imagesChanged := msg.InlineImagesEnabled != m.inlineImagesEnabled
		m.inlineImagesEnabled = msg.InlineImagesEnabled
		m = m.SetRelaxed(msg.Relaxed)
		m = m.SetLocation(msg.Loc)
		mutedChanged := !sameMutedSet(m.mutedTopics, msg.Settings.MutedTopics)
		if mutedChanged {
			m.mutedTopics = mutedSet(msg.Settings.MutedTopics)
		}
		if msg.Settings.FilterNSFW != m.filterNSFW || mutedChanged {
			m.filterNSFW = msg.Settings.FilterNSFW
			m.selectedIndex = 0
			if m.ready {
				m = m.refreshContent()
			}
		} else if imagesChanged && m.ready {
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

	case mergePendingTickMsg:
		m.refreshing = false
		m = m.MergePendingNew()
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
		topics := ParseTopics(m.panel.TopicsRaw())
		isPublic := m.panel.IsPublic()
		isNSFW := m.panel.IsNSFW()
		attachmentTouched := m.panel.AttachmentTouched()
		audioAttachment := m.panel.PendingAudio()
		otherAttachments := m.panel.OtherAttachments()
		if m.editingPostID != "" {
			postID := m.editingPostID
			m = m.closeCompose()
			return m, func() tea.Msg {
				return SubmitPostEditMsg{PostID: postID, Content: content, Title: title, Topics: topics, IsPublic: isPublic, IsNSFW: isNSFW, AttachmentTouched: attachmentTouched, AudioAttachment: audioAttachment, OtherAttachments: otherAttachments}
			}
		}
		slug := m.panel.SlugValue()
		// Keep the panel open and populated until App reports the outcome — a
		// rate-limited / failed publish then drops the user straight back into
		// their work (see postSubmitFailedMsg in app.go).
		m.panel = m.panel.MarkSubmitting()
		return m, func() tea.Msg {
			return SubmitNewPostMsg{Content: content, Title: title, Slug: slug, Topics: topics, IsPublic: isPublic, IsNSFW: isNSFW, AudioAttachment: audioAttachment}
		}

	case ComposeSaveAsNoteMsg:
		// Divert the new post into a private Journal note instead of the feed.
		// Notes carry only content + topics, so slug / public / nsfw are
		// dropped; a title becomes a leading markdown heading so the journal
		// list label (markdown.FirstLine) still reads sensibly.
		content := msg.Content
		if title := m.panel.TitleValue(); title != "" {
			content = "# " + title + "\n\n" + content
		}
		topics := ParseTopics(m.panel.TopicsRaw())
		m.panel = m.panel.MarkSubmitting()
		return m, func() tea.Msg {
			return SaveNewPostAsNoteMsg{Content: content, Topics: topics}
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
				return m, nil
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
		case "l":
			if visible := m.visiblePosts(); len(visible) > 0 && m.selectedIndex < len(visible) {
				post := visible[m.selectedIndex]
				return m, func() tea.Msg { return CopyLinkMsg{Post: post} }
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
		case "e":
			if m.CanEditSelected() {
				visible := m.visiblePosts()
				post := visible[m.selectedIndex]
				m.editingPostID = post.ID
				var cmd tea.Cmd
				m.panel, cmd = m.panel.OpenForEdit(post)
				m.viewport.Height = m.viewportHeight()
				return m, cmd
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
				return m, nil
			} else {
				var loadCmd tea.Cmd
				m, loadCmd = m.triggerLoadMore()
				if loadCmd != nil {
					return m, loadCmd
				}
			}
			return m, nil
		case "pgup":
			if m.selectedIndex > 0 {
				m.selectedIndex = max(0, m.selectedIndex-pageJumpItems)
				m = m.refreshContent()
				m = m.ensureSelectedVisible()
				return m, nil
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
		case "pgdown":
			if m.selectedIndex < len(m.visiblePosts())-1 {
				m.selectedIndex = min(len(m.visiblePosts())-1, m.selectedIndex+pageJumpItems)
				m = m.refreshContent()
				m = m.ensureSelectedVisible()
				return m, nil
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
	m.editingPostID = ""
	m.viewport.Height = m.viewportHeight()
	return m
}

// CloseComposeAfterSuccess tears the compose panel down once App has confirmed
// a submit landed (post created, converted to a note, or saved to the Journal).
func (m FeedModel) CloseComposeAfterSuccess() FeedModel { return m.closeCompose() }

// ClearComposeSubmitting re-enables the compose panel after a failed submit,
// leaving every field exactly as the user left it.
func (m FeedModel) ClearComposeSubmitting() FeedModel {
	m.panel = m.panel.ClearSubmitting()
	return m
}

// ComposePanelActive reports whether the new-post compose panel specifically is
// open (unlike ComposeActive, which also covers the flag and delete overlays).
func (m FeedModel) ComposePanelActive() bool { return m.panel.IsActive() }

// ComposeEditing reports whether the open compose panel is editing an existing
// post (vs composing a new one) — used to gate the "save to journal" hint.
func (m FeedModel) ComposeEditing() bool { return m.editingPostID != "" }

// ComposeSubmitting reports whether a compose submit is currently in flight.
func (m FeedModel) ComposeSubmitting() bool { return m.panel.IsSubmitting() }

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
// edited post body/title/topics/visibility, the inline-images setting
// changing, or a theme switch recoloring the baked-in ANSI) can be detected
// and recomputed instead of served.
type feedBodyCacheEntry struct {
	body                string
	imgSlots            []postImageSlot
	width               int
	bookmarked          bool
	watched             bool
	content             string
	title               string
	topics              string // strings.Join(p.Topics, ",") — cheap to compare
	isPublic            bool
	isNSFW              bool
	editedAt            time.Time
	inlineImagesEnabled bool
	themeName           string
}

// evictStaleBodyCache drops bodyCache entries for posts no longer present in
// m.posts. Called from SetPosts, whose wholesale replace of m.posts is the
// only point where a post can permanently drop out of the loaded list —
// without this, bodyCache grows for the life of the session since it's keyed
// by post ID and otherwise only ever gains entries in renderPost.
func (m FeedModel) evictStaleBodyCache() FeedModel {
	live := make(map[string]bool, len(m.posts))
	for _, p := range m.posts {
		live[p.ID] = true
	}
	for id := range m.bodyCache {
		if !live[id] {
			delete(m.bodyCache, id)
		}
	}
	return m
}

func (m FeedModel) renderPost(p model.Post, selected bool) (string, []postImageSlot) {
	_, bookmarked := m.bookmarkedPostIDs[p.ID]
	_, watched := m.watchedPostIDs[p.ID]
	topics := strings.Join(p.Topics, ",")

	body, imgSlots, ok := "", []postImageSlot(nil), false
	if e, hit := m.bodyCache[p.ID]; hit && e.width == m.width && e.bookmarked == bookmarked && e.watched == watched && e.content == p.Content && e.title == p.Title && e.topics == topics && e.isPublic == p.IsPublic && e.isNSFW == p.IsNSFW && e.editedAt.Equal(p.EditedAt) && e.inlineImagesEnabled == m.inlineImagesEnabled && e.themeName == theme.CurrentName() {
		body, imgSlots, ok = e.body, e.imgSlots, true
	}
	if !ok {
		body, imgSlots = renderPostBody(p, bookmarked, watched, m.width, m.location(), m.timeDisplayFormat, postMaxBodyLines, m.inlineImagesEnabled)
		m.bodyCache[p.ID] = feedBodyCacheEntry{body: body, imgSlots: imgSlots, width: m.width, bookmarked: bookmarked, watched: watched, content: p.Content, title: p.Title, topics: topics, isPublic: p.IsPublic, isNSFW: p.IsNSFW, editedAt: p.EditedAt, inlineImagesEnabled: m.inlineImagesEnabled, themeName: theme.CurrentName()}
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

// ansiTruncate truncates s to at most maxWidth terminal columns, appending "…" if truncated.
// Operates on plain text (no ANSI codes in post titles or raw content first lines).
func ansiTruncate(s string, maxWidth int) string {
	runes := []rune(s)
	if len(runes) <= maxWidth {
		return s
	}
	return string(runes[:maxWidth-1]) + "…"
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
