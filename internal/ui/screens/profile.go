package screens

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/markdown"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
	"github.com/ragnar/cyber-tui/internal/ui/urlutil"
)

const bioCharLimit = 127

// Field indices for the profile edit form.
const (
	fieldBio           = 0
	fieldWebsiteName   = 1
	fieldWebsiteUrl    = 2
	fieldWebsiteImgUrl = 3
	fieldLocationName  = 4
	fieldLatitude      = 5
	fieldLongitude     = 6
	numProfileFields   = 7
)

// inputIdx converts a focusedField value (excluding fieldBio=0) to the
// corresponding index in the inputs slice.
func inputIdx(field int) int {
	return field - 1
}

var profileFieldLabels = [numProfileFields]string{
	"Bio",
	"Website Name",
	"Website URL",
	"Website Img URL",
	"Location",
	"Latitude",
	"Longitude",
}

// profileTab is the active sub-tab within the profile view.
type profileTab int

const (
	tabInfo      profileTab = iota
	tabPosts
	tabReplies
	tabFollowing
	tabFollowers
	numProfileTabs
)

var profileTabLabels = [numProfileTabs]string{"Info", "Posts", "Replies", "Following", "Followers"}

// visibleProfileTabs controls which tabs appear in the UI and are reachable
// via Tab/Shift+Tab. Remove an entry to hide a tab without unwiring its data layer.
var visibleProfileTabs = []profileTab{tabInfo, tabPosts, tabReplies}

// tabItemLines is the number of terminal lines each list item occupies.
// Bordered box items are 3 lines: top border + 1 content line + bottom border.
const tabItemLines = 3

// tabScrollMeta holds the pagination and scroll state for one list tab.
// Kept separate from the typed item slices so it can be stored in an array
// indexed by profileTab, eliminating repeated field groups and switch statements.
type tabScrollMeta struct {
	cursor    string
	loaded    bool
	exhausted bool
	top       int // first visible item index (scroll offset)
}

// profileChrome is the number of lines consumed by the profile header
// (username + counts + blank), tab bar, and blank separator.
const profileChrome = 5

type ProfileModel struct {
	user           model.User
	compose        ComposeModel
	inputs         []textinput.Model // 6 inputs (all fields except bio)
	editMode       bool
	focusedField   int
	width          int
	height         int
	err            error
	saved          bool
	readOnly       bool
	canGoBack      bool
	isFollowing    bool
	followID       string
	followFeedback string

	// Display settings (from SharedConfigMsg).
	timeDisplayFormat string
	loc               *time.Location

	// Sub-tab state (view mode only).
	activeTab   profileTab
	tabMeta     [numProfileTabs]tabScrollMeta // pagination + scroll state per tab
	tabSelected int                           // selected item index within the active list tab

	posts     []model.Post
	replies   []model.Reply
	following []model.Follow
	followers []model.Follow
}

// SaveProfileMsg carries all editable profile fields.
type SaveProfileMsg struct {
	Bio             string
	WebsiteName     string
	WebsiteUrl      string
	WebsiteImageUrl string
	LocationName    string
	Latitude        string // empty or numeric string; parsed in app.go
	Longitude       string
}

func newProfileInputs() []textinput.Model {
	placeholders := [numProfileFields - 1]string{
		"website name (e.g. My Blog)",
		"https://…",
		"https://… (image url)",
		"city, country",
		"e.g. 48.8566",
		"e.g. 2.3522",
	}
	inputs := make([]textinput.Model, numProfileFields-1)
	for i, ph := range placeholders {
		ti := textinput.New()
		ti.Placeholder = ph
		inputs[i] = ti
	}
	return inputs
}

func NewProfileModel() ProfileModel {
	return ProfileModel{
		compose: NewComposeModel(0).SetCharLimit(bioCharLimit),
		inputs:  newProfileInputs(),
	}
}

func (m ProfileModel) SetReadOnly(readOnly bool) ProfileModel {
	m.readOnly = readOnly
	return m
}

func (m ProfileModel) SetCanGoBack(v bool) ProfileModel {
	m.canGoBack = v
	return m
}

func (m ProfileModel) SetFollowState(following bool, followID string) ProfileModel {
	m.isFollowing = following
	m.followID = followID
	return m
}

// IncrementFollowersCount adjusts the displayed follower count by delta (±1).
func (m ProfileModel) IncrementFollowersCount(delta int) ProfileModel {
	m.user.FollowersCount += delta
	return m
}

func (m ProfileModel) SetFollowFeedback(text string) ProfileModel {
	m.followFeedback = text
	return m
}

func (m ProfileModel) SetUser(u model.User) ProfileModel {
	m.user = u
	m.followFeedback = ""
	return m
}

func (m ProfileModel) SetError(err error) ProfileModel {
	m.err = err
	return m
}

// ClearTabs resets all sub-tab data and returns to the Info tab.
// Call this when loading a new user's profile to force fresh data.
func (m ProfileModel) ClearTabs() ProfileModel {
	m.activeTab = tabInfo
	m.tabSelected = 0
	m.tabMeta = [numProfileTabs]tabScrollMeta{}
	m.posts = nil
	m.replies = nil
	m.following = nil
	m.followers = nil
	return m
}

// SetUserPosts stores the first page of posts for the Posts tab.
func (m ProfileModel) SetUserPosts(posts []model.Post, cursor string) ProfileModel {
	m.posts = posts
	m.tabMeta[tabPosts] = tabScrollMeta{cursor: cursor, loaded: true, exhausted: cursor == ""}
	m.tabSelected = 0
	return m
}

// AppendUserPosts adds a next page to the Posts tab.
func (m ProfileModel) AppendUserPosts(posts []model.Post, cursor string) ProfileModel {
	m.posts = append(m.posts, posts...)
	m.tabMeta[tabPosts].cursor = cursor
	m.tabMeta[tabPosts].exhausted = cursor == ""
	return m
}

// SetUserReplies stores the first page of replies for the Replies tab.
func (m ProfileModel) SetUserReplies(replies []model.Reply, cursor string) ProfileModel {
	m.replies = replies
	m.tabMeta[tabReplies] = tabScrollMeta{cursor: cursor, loaded: true, exhausted: cursor == ""}
	m.tabSelected = 0
	return m
}

// AppendUserReplies adds a next page to the Replies tab.
func (m ProfileModel) AppendUserReplies(replies []model.Reply, cursor string) ProfileModel {
	m.replies = append(m.replies, replies...)
	m.tabMeta[tabReplies].cursor = cursor
	m.tabMeta[tabReplies].exhausted = cursor == ""
	return m
}

// SetUserFollowing stores the first page of following for the Following tab.
func (m ProfileModel) SetUserFollowing(follows []model.Follow, cursor string) ProfileModel {
	m.following = follows
	m.tabMeta[tabFollowing] = tabScrollMeta{cursor: cursor, loaded: true, exhausted: cursor == ""}
	m.tabSelected = 0
	return m
}

// AppendUserFollowing adds a next page to the Following tab.
func (m ProfileModel) AppendUserFollowing(follows []model.Follow, cursor string) ProfileModel {
	m.following = append(m.following, follows...)
	m.tabMeta[tabFollowing].cursor = cursor
	m.tabMeta[tabFollowing].exhausted = cursor == ""
	return m
}

// SetUserFollowers stores the first page of followers for the Followers tab.
func (m ProfileModel) SetUserFollowers(follows []model.Follow, cursor string) ProfileModel {
	m.followers = follows
	m.tabMeta[tabFollowers] = tabScrollMeta{cursor: cursor, loaded: true, exhausted: cursor == ""}
	m.tabSelected = 0
	return m
}

// AppendUserFollowers adds a next page to the Followers tab.
func (m ProfileModel) AppendUserFollowers(follows []model.Follow, cursor string) ProfileModel {
	m.followers = append(m.followers, follows...)
	m.tabMeta[tabFollowers].cursor = cursor
	m.tabMeta[tabFollowers].exhausted = cursor == ""
	return m
}

// ComposeActive reports whether the edit form is open, used by app.go to
// route key events past global shortcuts.
func (m ProfileModel) ComposeActive() bool { return m.editMode }

func (m ProfileModel) IsReadOnly() bool { return m.readOnly }

// location returns the configured timezone, defaulting to UTC.
func (m ProfileModel) location() *time.Location {
	if m.loc != nil {
		return m.loc
	}
	return time.UTC
}

func (m ProfileModel) Init() tea.Cmd { return nil }

// openEditForm prepopulates all inputs from the current user and activates
// edit mode, focused on the bio field by default.
func (m ProfileModel) openEditForm() (ProfileModel, tea.Cmd) {
	m.editMode = true
	m.focusedField = fieldBio
	m.saved = false

	// Populate textinputs from user fields.
	m.inputs[inputIdx(fieldWebsiteName)].SetValue(m.user.WebsiteName)
	m.inputs[inputIdx(fieldWebsiteUrl)].SetValue(m.user.WebsiteUrl)
	m.inputs[inputIdx(fieldWebsiteImgUrl)].SetValue(m.user.WebsiteImageUrl)
	m.inputs[inputIdx(fieldLocationName)].SetValue(m.user.LocationName)
	if m.user.LocationLatitude != 0 {
		m.inputs[inputIdx(fieldLatitude)].SetValue(fmt.Sprintf("%g", m.user.LocationLatitude))
	} else {
		m.inputs[inputIdx(fieldLatitude)].SetValue("")
	}
	if m.user.LocationLongitude != 0 {
		m.inputs[inputIdx(fieldLongitude)].SetValue(fmt.Sprintf("%g", m.user.LocationLongitude))
	} else {
		m.inputs[inputIdx(fieldLongitude)].SetValue("")
	}

	// Blur all textinputs (bio is the initial focus).
	for i := range m.inputs {
		m.inputs[i].Blur()
	}

	var cmd tea.Cmd
	m.compose, cmd = m.compose.OpenWithContent("bio", "what's your story…", m.user.Bio)
	return m, cmd
}

// closeEditForm exits edit mode and closes the compose box.
func (m ProfileModel) closeEditForm() ProfileModel {
	m.editMode = false
	m.compose = m.compose.Close()
	for i := range m.inputs {
		m.inputs[i].Blur()
	}
	return m
}

// moveFocus shifts focus by delta (+1 or -1) wrapping around all fields.
func (m ProfileModel) moveFocus(delta int) (ProfileModel, tea.Cmd) {
	// Blur current field.
	if m.focusedField == fieldBio {
		m.compose, _ = m.compose.SetFocused(false)
	} else {
		m.inputs[inputIdx(m.focusedField)].Blur()
	}

	m.focusedField = (m.focusedField + delta + numProfileFields) % numProfileFields

	// Focus new field.
	var cmd tea.Cmd
	if m.focusedField == fieldBio {
		m.compose, cmd = m.compose.SetFocused(true)
	} else {
		cmd = m.inputs[inputIdx(m.focusedField)].Focus()
	}
	return m, cmd
}

// buildSaveMsg collects all current field values into a SaveProfileMsg.
func (m ProfileModel) buildSaveMsg() SaveProfileMsg {
	return SaveProfileMsg{
		Bio:             m.compose.Content(),
		WebsiteName:     m.inputs[inputIdx(fieldWebsiteName)].Value(),
		WebsiteUrl:      m.inputs[inputIdx(fieldWebsiteUrl)].Value(),
		WebsiteImageUrl: m.inputs[inputIdx(fieldWebsiteImgUrl)].Value(),
		LocationName:    m.inputs[inputIdx(fieldLocationName)].Value(),
		Latitude:        m.inputs[inputIdx(fieldLatitude)].Value(),
		Longitude:       m.inputs[inputIdx(fieldLongitude)].Value(),
	}
}

// contentHeight returns the number of lines available for list tab content.
func (m ProfileModel) contentHeight() int {
	h := m.height - theme.ChromeHeight - profileChrome
	if h < 2 {
		h = 2
	}
	return h
}

// switchTab moves to a different sub-tab (delta = ±1) and emits a lazy-load
// message if the destination tab has not been loaded yet.
func (m ProfileModel) switchTab(delta int) (ProfileModel, tea.Cmd) {
	curr := 0
	for i, t := range visibleProfileTabs {
		if t == m.activeTab {
			curr = i
			break
		}
	}
	m.activeTab = visibleProfileTabs[(curr+delta+len(visibleProfileTabs))%len(visibleProfileTabs)]
	m.tabSelected = 0

	var lazyLoad tea.Cmd
	switch m.activeTab {
	case tabPosts:
		if !m.tabMeta[tabPosts].loaded {
			username := m.user.Username
			lazyLoad = func() tea.Msg { return ShowUserPostsMsg{Username: username} }
		}
	case tabReplies:
		if !m.tabMeta[tabReplies].loaded {
			username := m.user.Username
			lazyLoad = func() tea.Msg { return ShowUserRepliesMsg{Username: username} }
		}
	case tabFollowing:
		if !m.tabMeta[tabFollowing].loaded {
			userID := m.user.ID
			lazyLoad = func() tea.Msg { return ShowUserFollowingMsg{UserID: userID} }
		}
	case tabFollowers:
		if !m.tabMeta[tabFollowers].loaded {
			userID := m.user.ID
			lazyLoad = func() tea.Msg { return ShowUserFollowersMsg{UserID: userID} }
		}
	}
	return m, lazyLoad
}

// activeTabLen returns the number of items in the current tab.
func (m ProfileModel) activeTabLen() int {
	switch m.activeTab {
	case tabPosts:
		return len(m.posts)
	case tabReplies:
		return len(m.replies)
	case tabFollowing:
		return len(m.following)
	case tabFollowers:
		return len(m.followers)
	}
	return 0
}

// moveTabSelection moves the selected item by delta and scrolls if needed.
// Returns a pagination command if the user reaches near the end of a loaded list.
func (m ProfileModel) moveTabSelection(delta int) (ProfileModel, tea.Cmd) {
	n := m.activeTabLen()
	if n == 0 {
		return m, nil
	}

	m.tabSelected += delta
	if m.tabSelected < 0 {
		m.tabSelected = 0
	}
	if m.tabSelected >= n {
		m.tabSelected = n - 1
	}

	numVisible := m.contentHeight() / tabItemLines
	if numVisible < 1 {
		numVisible = 1
	}

	// Update scroll offset so selected item is always in view.
	top := m.scrollTopForActiveTab()
	if m.tabSelected < top {
		top = m.tabSelected
	}
	if m.tabSelected >= top+numVisible {
		top = m.tabSelected - numVisible + 1
	}
	m = m.setScrollTopForActiveTab(top)

	// Check for pagination near the bottom of the list.
	var pageCmd tea.Cmd
	if m.tabSelected >= n-3 {
		pageCmd = m.loadMoreCmd()
	}
	return m, pageCmd
}

func (m ProfileModel) scrollTopForActiveTab() int {
	return m.tabMeta[m.activeTab].top
}

func (m ProfileModel) setScrollTopForActiveTab(top int) ProfileModel {
	m.tabMeta[m.activeTab].top = top
	return m
}

// loadMoreCmd returns a pagination command if the active tab has more pages.
func (m ProfileModel) loadMoreCmd() tea.Cmd {
	meta := m.tabMeta[m.activeTab]
	if meta.exhausted || meta.cursor == "" {
		return nil
	}
	cursor := meta.cursor
	switch m.activeTab {
	case tabPosts:
		username := m.user.Username
		return func() tea.Msg { return LoadMoreUserPostsMsg{Username: username, Cursor: cursor} }
	case tabReplies:
		username := m.user.Username
		return func() tea.Msg { return LoadMoreUserRepliesMsg{Username: username, Cursor: cursor} }
	case tabFollowing:
		userID := m.user.ID
		return func() tea.Msg { return LoadMoreUserFollowingMsg{UserID: userID, Cursor: cursor} }
	case tabFollowers:
		userID := m.user.ID
		return func() tea.Msg { return LoadMoreUserFollowersMsg{UserID: userID, Cursor: cursor} }
	}
	return nil
}

// handleTabEnter activates the selected item in a list tab.
func (m ProfileModel) handleTabEnter() (ProfileModel, tea.Cmd) {
	n := m.activeTabLen()
	if n == 0 || m.tabSelected >= n {
		return m, nil
	}
	switch m.activeTab {
	case tabPosts:
		post := m.posts[m.tabSelected]
		return m, func() tea.Msg { return ShowProfilePostMsg{Post: post} }
	case tabReplies:
		reply := m.replies[m.tabSelected]
		// Navigate to the post that contains this reply; App fetches the full post and highlights the reply.
		postID := reply.PostID
		replyID := reply.ID
		return m, func() tea.Msg { return ShowProfileReplyMsg{PostID: postID, ReplyID: replyID} }
	case tabFollowing:
		follow := m.following[m.tabSelected]
		username := follow.FollowedUsername
		if username == "" {
			return m, nil // API doesn't return usernames — profile navigation unavailable
		}
		return m, func() tea.Msg { return ShowUserProfileMsg{Username: username} }
	case tabFollowers:
		follow := m.followers[m.tabSelected]
		username := follow.FollowerUsername
		if username == "" {
			return m, nil // API doesn't return usernames — profile navigation unavailable
		}
		return m, func() tea.Msg { return ShowUserProfileMsg{Username: username} }
	}
	return m, nil
}

func (m ProfileModel) Update(msg tea.Msg) (ProfileModel, tea.Cmd) {
	switch msg := msg.(type) {
	case SharedConfigMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.timeDisplayFormat = msg.Settings.TimeDisplayFormat
		if msg.Loc != nil {
			m.loc = msg.Loc
		}
		w := msg.Width
		if w > 80 {
			w = 80
		}
		m.compose = m.compose.SetWidth(w)
		inputW := w - 20
		if inputW < 10 {
			inputW = 10
		}
		for i := range m.inputs {
			m.inputs[i].Width = inputW
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		w := msg.Width
		if w > 80 {
			w = 80
		}
		m.compose = m.compose.SetWidth(w)
		inputW := w - 20
		if inputW < 10 {
			inputW = 10
		}
		for i := range m.inputs {
			m.inputs[i].Width = inputW
		}
		return m, nil

	case tea.KeyMsg:
		if m.editMode {
			// Navigation and global actions intercepted before field routing.
			switch msg.String() {
			case "tab":
				return m.moveFocus(1)
			case "shift+tab":
				return m.moveFocus(-1)
			case "esc":
				if m.focusedField != fieldBio {
					return m.closeEditForm(), nil
				}
				// Bio field: fall through so compose emits ComposeCancelMsg.
			case "ctrl+s":
				if m.focusedField != fieldBio {
					save := m.buildSaveMsg()
					return m.closeEditForm(), func() tea.Msg { return save }
				}
				// Bio field: fall through so compose emits ComposeSubmitMsg.
			}

			// Route to the active field.
			var cmd tea.Cmd
			if m.focusedField == fieldBio {
				m.compose, cmd = m.compose.Update(msg)
			} else {
				filtered, ok := filterAmbiguousKeyMsg(msg)
				if !ok {
					return m, nil
				}
				idx := inputIdx(m.focusedField)
				m.inputs[idx], cmd = m.inputs[idx].Update(filtered)
			}
			return m, cmd
		}

		// Not in edit mode — handle sub-tab and list navigation.
		switch msg.String() {
		case "tab":
			return m.switchTab(+1)
		case "shift+tab":
			return m.switchTab(-1)
		case "j", "down":
			if m.activeTab != tabInfo {
				return m.moveTabSelection(+1)
			}
		case "k", "up":
			if m.activeTab != tabInfo {
				return m.moveTabSelection(-1)
			}
		case "enter":
			if m.activeTab != tabInfo {
				return m.handleTabEnter()
			}
		case "esc":
			if m.readOnly || m.canGoBack {
				return m, func() tea.Msg { return BackFromProfileMsg{} }
			}
		case "e":
			if !m.readOnly {
				return m.openEditForm()
			}
		case "f":
			if m.readOnly {
				if m.isFollowing {
					return m, func() tea.Msg { return UnfollowUserMsg{FollowID: m.followID} }
				}
				return m, func() tea.Msg { return FollowUserMsg{UserID: m.user.ID} }
			}
		}

	// ComposeSubmitMsg arrives when Ctrl+S is pressed inside the bio compose box.
	case ComposeSubmitMsg:
		save := m.buildSaveMsg()
		save.Bio = msg.Content
		return m.closeEditForm(), func() tea.Msg { return save }

	// ComposeCancelMsg arrives when Esc is pressed inside the bio compose box.
	case ComposeCancelMsg:
		return m.closeEditForm(), nil
	}
	return m, nil
}

// View renders the profile screen.
func (m ProfileModel) View() string {
	if m.err != nil {
		return theme.Error.Render(fmt.Sprintf("profile error: %s", m.err))
	}

	username := theme.Title.Render("@" + m.user.Username)

	if m.editMode {
		return m.editFormView(username)
	}

	// --- View mode: compact header + tab bar + content ---

	counts := theme.Subtle.Render(fmt.Sprintf(
		"%d followers · %d following · %d posts",
		m.user.FollowersCount, m.user.FollowingCount, m.user.PostsCount,
	))

	// Tab bar — pinned to terminal width to prevent terminal-side line wrapping,
	// which would cause Bubble Tea's line-diff renderer to miscalculate cursor
	// positions and leave ghost lines on re-render (observed on WSL/Windows Terminal).
	var tabParts []string
	for _, i := range visibleProfileTabs {
		label := profileTabLabels[i]
		if i == m.activeTab {
			tabParts = append(tabParts, theme.ActiveTab.Render(label))
		} else {
			tabParts = append(tabParts, theme.Tab.Render(label))
		}
	}
	tabBar := strings.Join(tabParts, " ")
	if m.width > 0 {
		tabBar = lipgloss.NewStyle().Width(m.width).Render(tabBar)
	}

	var content string
	switch m.activeTab {
	case tabInfo:
		content = m.infoTabView()
	case tabPosts:
		content = m.postsTabView()
	case tabReplies:
		content = m.repliesTabView()
	case tabFollowing:
		content = m.followListTabView(m.following, tabFollowing, "following")
	case tabFollowers:
		content = m.followListTabView(m.followers, tabFollowers, "followers")
	}

	out := lipgloss.JoinVertical(lipgloss.Left,
		username,
		counts,
		"",
		tabBar,
		"",
		content,
	)
	if availH := m.height - theme.ChromeHeight; availH > 0 {
		out = lipgloss.NewStyle().MaxHeight(availH).Render(out)
	}
	return out
}

// infoTabView renders the Info tab content (bio, website, location, hint).
func (m ProfileModel) infoTabView() string {
	var rows []string

	rows = append(rows, theme.Base.Render(m.user.Bio))

	if m.user.WebsiteUrl != "" || m.user.WebsiteName != "" {
		label := m.user.WebsiteName
		if label == "" {
			label = m.user.WebsiteUrl
		} else if m.user.WebsiteUrl != "" {
			label = label + "  " + theme.Subtle.Render(m.user.WebsiteUrl)
		}
		rows = append(rows, "", theme.Subtle.Render("web: ")+theme.Base.Render(label))
	}

	if m.user.WebsiteImageUrl != "" {
		rows = append(rows, theme.Subtle.Render("img: ")+theme.Base.Render(m.user.WebsiteImageUrl))
	}

	if m.user.LocationName != "" {
		loc := m.user.LocationName
		if m.user.LocationLatitude != 0 || m.user.LocationLongitude != 0 {
			loc += fmt.Sprintf("  (%g, %g)", m.user.LocationLatitude, m.user.LocationLongitude)
		}
		rows = append(rows, theme.Subtle.Render("loc: ")+theme.Base.Render(loc))
	} else if m.user.LocationLatitude != 0 || m.user.LocationLongitude != 0 {
		rows = append(rows, theme.Subtle.Render("loc: ")+
			theme.Base.Render(fmt.Sprintf("%g, %g", m.user.LocationLatitude, m.user.LocationLongitude)))
	}

	rows = append(rows, "")

	if m.followFeedback != "" {
		rows = append(rows, theme.Highlight.Render(m.followFeedback))
	} else if m.saved && !m.readOnly {
		rows = append(rows, theme.Highlight.Render("saved."))
	}

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// tabSliceRange returns the [top, end) index range of items to render for the
// active tab, clipped to n (the total number of items in that tab).
func (m ProfileModel) tabSliceRange(n int) (top, end int) {
	numVisible := m.contentHeight() / tabItemLines
	if numVisible < 1 {
		numVisible = 1
	}
	top = m.tabMeta[m.activeTab].top
	end = top + numVisible
	if end > n {
		end = n
	}
	return
}

// postsTabView renders the Posts tab content.
func (m ProfileModel) postsTabView() string {
	if !m.tabMeta[tabPosts].loaded {
		return theme.Subtle.Render("loading…")
	}
	if len(m.posts) == 0 {
		return theme.Subtle.Render("no posts.")
	}
	top, end := m.tabSliceRange(len(m.posts))
	lines := make([]string, 0, end-top)
	for i := top; i < end; i++ {
		lines = append(lines, m.renderPostItem(m.posts[i], i == m.tabSelected))
	}
	return strings.Join(lines, "\n")
}

// repliesTabView renders the Replies tab content.
func (m ProfileModel) repliesTabView() string {
	if !m.tabMeta[tabReplies].loaded {
		return theme.Subtle.Render("loading…")
	}
	if len(m.replies) == 0 {
		return theme.Subtle.Render("no replies.")
	}
	top, end := m.tabSliceRange(len(m.replies))
	lines := make([]string, 0, end-top)
	for i := top; i < end; i++ {
		lines = append(lines, m.renderReplyItem(m.replies[i], i == m.tabSelected))
	}
	return strings.Join(lines, "\n")
}

// followListTabView renders a Following or Followers tab.
func (m ProfileModel) followListTabView(follows []model.Follow, tab profileTab, kind string) string {
	if !m.tabMeta[tab].loaded {
		return theme.Subtle.Render("loading…")
	}
	if len(follows) == 0 {
		return theme.Subtle.Render("no " + kind + ".")
	}
	top, end := m.tabSliceRange(len(follows))
	lines := make([]string, 0, end-top)
	for i := top; i < end; i++ {
		lines = append(lines, m.renderFollowItem(follows[i], i == m.tabSelected))
	}
	return strings.Join(lines, "\n")
}

// renderPostItem renders a single post as a bordered single-line card.
func (m ProfileModel) renderPostItem(p model.Post, selected bool) string {
	innerWidth := m.width - 4
	if innerWidth < 1 {
		innerWidth = 40
	}
	ts := theme.Subtle.Render(displayTime(p.CreatedAt, m.location(), m.timeDisplayFormat, true))
	previewText := markdown.FirstLine(p.Content)
	if p.Title != "" {
		previewText = p.Title
	}
	left := theme.Highlight.Render("@"+p.AuthorUsername) + "  " + theme.Base.Render(truncateStr(previewText, innerWidth-lipgloss.Width(ts)-lipgloss.Width(theme.Highlight.Render("@"+p.AuthorUsername))-4))
	gap := innerWidth - lipgloss.Width(left) - lipgloss.Width(ts)
	var line string
	if gap > 0 {
		line = left + strings.Repeat(" ", gap) + ts
	} else {
		line = left
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

// renderReplyItem renders a single reply as a bordered single-line card.
func (m ProfileModel) renderReplyItem(r model.Reply, selected bool) string {
	innerWidth := m.width - 4
	if innerWidth < 1 {
		innerWidth = 40
	}
	ts := theme.Subtle.Render(displayTime(r.CreatedAt, m.location(), m.timeDisplayFormat, true))
	tag := theme.Subtle.Render("↩ ")
	previewMaxW := innerWidth - lipgloss.Width(ts) - lipgloss.Width(tag) - 2
	if previewMaxW < 10 {
		previewMaxW = 10
	}
	left := tag + theme.Base.Render(truncateStr(markdown.FirstLine(r.Content), previewMaxW))
	gap := innerWidth - lipgloss.Width(left) - lipgloss.Width(ts)
	var line string
	if gap > 0 {
		line = left + strings.Repeat(" ", gap) + ts
	} else {
		line = left
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

// renderFollowItem renders a single follow relationship as a bordered single-line card.
// If the API did not return a username (only IDs), the truncated user ID is shown.
func (m ProfileModel) renderFollowItem(f model.Follow, selected bool) string {
	innerWidth := m.width - 4
	if innerWidth < 1 {
		innerWidth = 40
	}

	var username string
	var userID string
	if m.activeTab == tabFollowing {
		username = f.FollowedUsername
		userID = f.FollowedID
	} else {
		username = f.FollowerUsername
		userID = f.FollowerID
	}

	var display string
	if username != "" {
		display = theme.Highlight.Render("@" + username)
	} else {
		// API doesn't return usernames — show truncated user ID as a fallback.
		display = theme.Subtle.Render(truncateStr(userID, 16))
	}

	ts := theme.Subtle.Render(displayTime(f.CreatedAt, m.location(), m.timeDisplayFormat, true))
	gap := innerWidth - lipgloss.Width(display) - lipgloss.Width(ts)
	var line string
	if gap > 0 {
		line = display + strings.Repeat(" ", gap) + ts
	} else {
		line = display
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

// truncateStr truncates s to maxW terminal columns, appending "…" if truncated.
func truncateStr(s string, maxW int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	return markdown.TruncateToWidth(s, maxW)
}

func (m ProfileModel) editFormView(username string) string {
	const labelW = 16 // pad all labels to this width

	var rows []string
	rows = append(rows, username, "")

	for field := 0; field < numProfileFields; field++ {
		label := profileFieldLabels[field]
		labelStr := theme.Subtle.Render(fmt.Sprintf("%-*s", labelW, label))

		if field == fieldBio {
			focused := m.focusedField == fieldBio
			borderStyle := theme.Border
			if focused {
				borderStyle = theme.ActiveBorder
			}
			used := len(m.compose.Content())
			counterStr := fmt.Sprintf("%d/%d", used, bioCharLimit)
			var counter string
			if used >= bioCharLimit {
				counter = theme.Error.Render(counterStr)
			} else {
				counter = theme.Subtle.Render(counterStr)
			}
			bioBox := lipgloss.JoinVertical(lipgloss.Left,
				borderStyle.Render(m.compose.View()),
				counter,
			)
			rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, labelStr, bioBox))
		} else {
			idx := inputIdx(field)
			focused := m.focusedField == field
			borderStyle := theme.Border
			if focused {
				borderStyle = theme.ActiveBorder
			}
			inputBox := borderStyle.Render(m.inputs[idx].View())
			rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, labelStr, inputBox))
		}
	}

	rows = append(rows, "", theme.Subtle.Render("ctrl+s · save   esc · cancel   tab · next field"))

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// GetFocusedURLs implements URLProvider. Returns the profile's website URLs.
// Returns nil in edit mode (text input is active).
func (m ProfileModel) GetFocusedURLs() []string {
	if m.editMode {
		return nil
	}
	var urls []string
	if m.user.WebsiteUrl != "" {
		urls = append(urls, urlutil.NormalizeURL(m.user.WebsiteUrl))
	}
	if m.user.WebsiteImageUrl != "" {
		urls = append(urls, urlutil.NormalizeURL(m.user.WebsiteImageUrl))
	}
	return urls
}

