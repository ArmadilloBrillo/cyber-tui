package ui

import (
	"fmt"
	"math"
	"math/rand"
	neturl "net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/x/ansi"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ragnar/cyber-tui/internal/api"
	"github.com/ragnar/cyber-tui/internal/config"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/screens"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
	"github.com/ragnar/cyber-tui/internal/ui/urlutil"
	"github.com/ragnar/cyber-tui/internal/version"
)

type screen int

const (
	screenLogin screen = iota
	screenFeed
	screenChatrooms
	screenCMail
	screenProfile
	screenPostDetail
	screenNotifications
	screenSettings
	screenBookmarks
	screenTopics
	screenJournal
)

type focusTarget int

const (
	focusMenu focusTarget = iota
	focusList             // reserved for future list navigation
)

// availableThemes is the ordered list of selectable themes shown in the picker.
var availableThemes = []string{"cyber", "c64", "vt320"}

var renderedVersionLine = theme.Subtle.Render("version " + version.Version + " (" + version.Commit + ")")

// menuTabs is the ordered list of navigable screens, shared by the
// renderer and key handler so the order is never out of sync.
var menuTabs = []struct {
	label string
	s     screen
}{
	{"feed", screenFeed},
	{"notifications", screenNotifications},
	{"journal", screenJournal},
	{"bookmarks", screenBookmarks},
	{"topics", screenTopics},
	{"profile", screenProfile},
	{"settings", screenSettings},
}

type App struct {
	client      api.Client
	tokens      model.Tokens
	currentUser model.User
	active      screen
	focus       focusTarget
	width       int
	height      int

	// autoEmail and autoPassword are set from the config file.
	// When both are non-empty, Init fires loginCmd immediately.
	autoEmail    string
	autoPassword string

	// savedSession is set when a config file was loaded at startup.
	// When non-nil, Init fires tokenLoginCmd instead of showing the login screen.
	savedSession *config.Config

	// relaxed controls display density: false = dense (default), true = blank lines between items.
	relaxed bool

	// themePicker state — open with 't', close with Enter/Esc.
	themePickerOpen   bool
	themePickerCursor int    // index into availableThemes
	themePickerOrig   string // theme name when picker was opened (for Esc revert)

	// timezonePicker state — open with 'z', close with Enter/Esc.
	timezonePickerOpen   bool
	timezonePickerCursor int    // index into config.AvailableTimezones
	timezonePickerOrig   string // timezone label when picker was opened (for Esc revert)

	// helpModal state — open with '?', close with any key.
	helpModalOpen bool

	// urlPicker state — open with 'o' when multiple URLs are available.
	urlPickerOpen   bool
	urlPickerItems  []string
	urlPickerCursor int

	// timezone is the active UTC offset label (e.g. "UTC+2"). Empty = UTC.
	// loc is the parsed *time.Location derived from timezone.
	timezone string
	loc      *time.Location

	login          screens.LoginModel
	feed           screens.FeedModel
	chatrooms      screens.ChatroomsModel
	cmail          screens.CMailModel
	profile        screens.ProfileModel
	postDetail     screens.PostDetailModel
	notifications  screens.NotificationsModel
	settingsScreen screens.SettingsModel
	bookmarks      screens.BookmarksModel
	topics         screens.TopicsModel
	journal        screens.JournalModel

	// postDetailReturn is the screen to go back to when ESC is pressed in PostDetail.
	postDetailReturn screen

	// profileReturn is the screen to go back to when ESC is pressed in a read-only profile.
	profileReturn screen

	// pendingReplyID is set when navigating to PostDetail from a reply/thread_reply
	// notification. After replies load, PostDetail scrolls to this reply, then it is cleared.
	pendingReplyID string

	// polledUnreadCount is the single source of truth for the tab badge unread count.
	// It is synced from: initial/page load, 60-second poll, m/M key, and enter on a notification.
	polledUnreadCount int

	// settings holds the user's preferences fetched from GET /v1/settings on login.
	settings model.Settings

	// wanderLust is the local config value for wander mode. Defaults to true.
	wanderLust bool
	// maxThreadDepth is the local config value for reply nesting depth. Defaults to 3.
	maxThreadDepth int

	// bookmarkedPostIDs and bookmarkedReplyIDs track which posts/replies the current
	// user has bookmarked, populated from the bookmarks list and kept in sync on
	// create/delete. Used to show [★] indicators in feed, postdetail, and topics.
	bookmarkedPostIDs  map[string]struct{}
	bookmarkedReplyIDs map[string]struct{}
}

func NewApp(client api.Client) App {
	return App{
		client:     client,
		active:     screenLogin,
		focus:      focusMenu,
		loc:        time.UTC,
		wanderLust: true,
		login:          screens.NewLoginModel(""),
		feed:           screens.NewFeedModel(),
		chatrooms:      screens.NewChatroomsModel(),
		cmail:          screens.NewCMailModel("", client),
		profile:        screens.NewProfileModel(),
		postDetail:     screens.NewPostDetailModel(),
		notifications:  screens.NewNotificationsModel(),
		settingsScreen: screens.NewSettingsModel(),
		bookmarks:      screens.NewBookmarksModel(),
		topics:         screens.NewTopicsModel(),
		journal:        screens.NewJournalModel(0),
		bookmarkedPostIDs:  make(map[string]struct{}),
		bookmarkedReplyIDs: make(map[string]struct{}),
	}
}

// WithSavedEmail pre-fills the email field on the login screen.
// Used when a previous session email is known but no token is available.
func (a App) WithSavedEmail(email string) App {
	if email != "" {
		a.login = screens.NewLoginModel(email)
	}
	return a
}

// WithAutoLogin pre-fills credentials loaded from the environment.
// When both email and password are non-empty, Init skips the login screen.
func (a App) WithAutoLogin(email, password string) App {
	a.autoEmail = email
	a.autoPassword = password
	return a
}

// WithSavedSession attaches a persisted session loaded from ~/.cyber-tui.json.
// When set, Init attempts to resume the session via token refresh instead of
// showing the login screen.
func (a App) WithSavedSession(s config.Config) App {
	a.savedSession = &s
	a.relaxed = s.Density == "relaxed"
	a.timezone = s.Timezone
	a.loc = s.GetLocation()
	a.wanderLust = s.WanderLust
	a.maxThreadDepth = s.GetMaxThreadDepth()
	return a
}

// --- init ---

func (a App) Init() tea.Cmd {
	if a.savedSession != nil && a.savedSession.RefreshToken != "" {
		return a.tokenLoginCmd(a.savedSession.RefreshToken)
	}
	if a.autoEmail != "" && a.autoPassword != "" {
		return a.loginCmd(a.autoEmail, a.autoPassword)
	}
	return a.login.Init()
}

// --- update ---

// Update is the top-level Bubble Tea update function. It chains domain
// handlers so each can claim the message and return early. WindowSizeMsg is
// handled first and always falls through to delegateUpdate so the active
// screen can also react to it.
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m, ok := msg.(tea.WindowSizeMsg); ok {
		a = a.applyWindowSize(m)
		return a, a.delegateUpdate(msg)
	}
	if a2, cmd, ok := a.handleKeys(msg);       ok { return a2, cmd }
	if a2, cmd, ok := a.handleAuth(msg);       ok { return a2, cmd }
	if a2, cmd, ok := a.handleFeed(msg);       ok { return a2, cmd }
	if a2, cmd, ok := a.handlePostDetail(msg); ok { return a2, cmd }
	if a2, cmd, ok := a.handleChatrooms(msg);  ok { return a2, cmd }
	if a2, cmd, ok := a.handleCMail(msg);      ok { return a2, cmd }
	if a2, cmd, ok := a.handleProfile(msg);        ok { return a2, cmd }
	if a2, cmd, ok := a.handleNotifications(msg); ok { return a2, cmd }
	if a2, cmd, ok := a.handleSettings(msg);       ok { return a2, cmd }
	if a2, cmd, ok := a.handleBookmarks(msg);      ok { return a2, cmd }
	if a2, cmd, ok := a.handleTopics(msg);         ok { return a2, cmd }
	if a2, cmd, ok := a.handleJournal(msg);        ok { return a2, cmd }
	if a2, cmd, ok := a.handleErr(msg);            ok { return a2, cmd }
	return a, a.delegateUpdate(msg)
}

// broadcastConfig pushes the current display settings to all screens.
// Call this whenever loc, relaxed, or dimensions change outside of a
// WindowSizeMsg (e.g. after login, timezone change, or density toggle).
// Adding a new screen only requires handling SharedConfigMsg in that
// screen's Update — no changes here are needed.
func (a *App) broadcastConfig() {
	msg := screens.SharedConfigMsg{Width: a.width, Height: a.height, Loc: a.loc, Relaxed: a.relaxed, Settings: a.settings, WanderLust: a.wanderLust, MaxThreadDepth: a.maxThreadDepth}
	a.feed, _ = a.feed.Update(msg)
	a.chatrooms, _ = a.chatrooms.Update(msg)
	a.cmail, _ = a.cmail.Update(msg)
	a.postDetail, _ = a.postDetail.Update(msg)
	a.profile, _ = a.profile.Update(msg)
	a.notifications, _ = a.notifications.Update(msg)
	a.settingsScreen, _ = a.settingsScreen.Update(msg)
	a.bookmarks, _ = a.bookmarks.Update(msg)
	a.topics, _ = a.topics.Update(msg)
	a.journal, _ = a.journal.Update(msg)
}

// broadcastBookmarkedIDs pushes the current bookmarked-ID sets to all screens
// that render posts or replies (feed, postDetail, topics). Call this whenever
// the sets change (bookmark loaded, created, or deleted).
func (a *App) broadcastBookmarkedIDs() {
	msg := screens.BookmarkedIDsMsg{
		PostIDs:  a.bookmarkedPostIDs,
		ReplyIDs: a.bookmarkedReplyIDs,
	}
	a.feed, _ = a.feed.Update(msg)
	a.postDetail, _ = a.postDetail.Update(msg)
	a.topics, _ = a.topics.Update(msg)
}

// applyWindowSize stores the new terminal dimensions and broadcasts the size
// to all screens so their viewports initialise before they become active.
func (a App) applyWindowSize(m tea.WindowSizeMsg) App {
	a.width = m.Width
	a.height = m.Height
	// Broadcast to all screens; the active screen gets a second update via
	// delegateUpdate in Update, which is harmless (re-applies the same size).
	a.feed, _ = a.feed.Update(m)
	a.chatrooms, _ = a.chatrooms.Update(m)
	a.cmail, _ = a.cmail.Update(m)
	a.postDetail, _ = a.postDetail.Update(m)
	a.profile, _ = a.profile.Update(m)
	a.notifications, _ = a.notifications.Update(m)
	a.settingsScreen, _ = a.settingsScreen.Update(m)
	a.bookmarks, _ = a.bookmarks.Update(m)
	a.topics, _ = a.topics.Update(m)
	a.journal, _ = a.journal.Update(m)
	return a
}

// handleKeys processes tea.KeyMsg events: modal intercepts, focused-input
// bypass, and all global keyboard shortcuts.
func (a App) handleKeys(msg tea.Msg) (App, tea.Cmd, bool) {
	m, ok := msg.(tea.KeyMsg)
	if !ok {
		return a, nil, false
	}
	// Modal overlays intercept all keys while open.
	if a.timezonePickerOpen {
		model, cmd := a.handleTimezonePickerKey(m)
		return model.(App), cmd, true
	}
	if a.themePickerOpen {
		model, cmd := a.handleThemePickerKey(m)
		return model.(App), cmd, true
	}
	if a.helpModalOpen {
		model, cmd := a.handleHelpModalKey(m)
		return model.(App), cmd, true
	}
	if a.urlPickerOpen {
		model, cmd := a.handleURLPickerKey(m)
		return model.(App), cmd, true
	}
	// When a screen has a focused text input, let it consume all keys.
	// ctrl+c is kept as a hard escape hatch.
	if a.activeScreenHasFocusedInput() {
		if m.String() == "ctrl+c" {
			return a, tea.Quit, true
		}
		return a, nil, false // fall through to delegateUpdate
	}
	switch m.String() {
	case "t":
		if a.active != screenLogin {
			a.themePickerOpen = true
			a.themePickerOrig = theme.CurrentName()
			a.themePickerCursor = themeIndex(theme.CurrentName())
			return a, nil, true
		}
	case "z":
		if a.active != screenLogin {
			a.timezonePickerOpen = true
			a.timezonePickerOrig = a.timezone
			a.timezonePickerCursor = timezoneIndex(a.timezone)
			return a, nil, true
		}
	case "v":
		if a.active != screenLogin {
			a.relaxed = !a.relaxed
			a.broadcastConfig()
			relaxed := a.relaxed
			return a, func() tea.Msg {
				if sess, err := config.Load(); err == nil {
					if relaxed {
						sess.Density = "relaxed"
					} else {
						sess.Density = ""
					}
					_ = config.Save(sess)
				}
				return nil
			}, true
		}
	case "?":
		if a.active != screenLogin {
			a.helpModalOpen = true
			return a, nil, true
		}
	case "o":
		if a.active != screenLogin {
			app, cmd := a.handleOpenURL(a.getFocusedURLs())
			return app, cmd, true
		}
	case "ctrl+c", "q":
		if a.active != screenLogin {
			return a, tea.Quit, true
		}
	case "1":
		if a.active != screenLogin {
			a.cmail = a.cmail.CancelSubscription()
			a.active = screenFeed
			return a, a.loadFeedCmd(), true
		}
	case "2":
		if a.active != screenLogin {
			a.cmail = a.cmail.CancelSubscription()
			a.active = screenNotifications
			return a, a.loadNotifsCmd(), true
		}
	case "3":
		if a.active != screenLogin {
			a.cmail = a.cmail.CancelSubscription()
			a.active = screenJournal
			return a, a.loadJournalCmd(), true
		}
	case "4":
		if a.active != screenLogin {
			a.cmail = a.cmail.CancelSubscription()
			a.active = screenBookmarks
			if !a.bookmarks.IsLoaded() {
				a.bookmarks = a.bookmarks.SetFetching()
				return a, a.loadBookmarksCmd(""), true
			}
			return a, nil, true
		}
	case "5":
		if a.active != screenLogin {
			a.cmail = a.cmail.CancelSubscription()
			a.active = screenTopics
			return a, a.loadTopicsCmd(), true
		}
	case "6":
		if a.active != screenLogin {
			a.cmail = a.cmail.CancelSubscription()
			a.active = screenProfile
			return a, a.loadProfileCmd(), true
		}
	case "7":
		if a.active != screenLogin {
			a.cmail = a.cmail.CancelSubscription()
			a.active = screenSettings
			return a, nil, true // no load cmd; settings already in memory
		}
	case "left":
		if a.active != screenLogin && a.active != screenPostDetail && a.focus == focusMenu {
			return a, a.navigateTab(-1), true
		}
	case "right":
		if a.active != screenLogin && a.active != screenPostDetail && a.focus == focusMenu {
			return a, a.navigateTab(+1), true
		}
	}
	return a, nil, false
}

// handleAuth processes login/registration flow messages.
func (a App) handleAuth(msg tea.Msg) (App, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case screens.SubmitLoginMsg:
		return a, a.loginCmd(msg.Email, msg.Password), true
	case screens.LoginMsg:
		return a, a.afterLoginCmd(), true
	case screens.LoginErrMsg:
		var cmd tea.Cmd
		a.login, cmd = a.login.Update(msg)
		return a, cmd, true
	}
	return a, nil, false
}

// handleFeed processes feed and post-navigation messages.
func (a App) handleFeed(msg tea.Msg) (App, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case feedLoadedMsg:
		a.feed = a.feed.SetPosts(msg.posts, msg.cursor)
		return a, nil, true
	case feedPageMsg:
		a.feed = a.feed.AppendPosts(msg.posts, msg.cursor)
		return a, nil, true
	case screens.RefreshFeedMsg:
		return a, a.loadFeedCmd(), true
	case screens.LoadMoreFeedMsg:
		return a, a.loadFeedPageCmd(msg.Cursor), true
	case screens.ShowPostMsg:
		a.postDetailReturn = screenFeed
		a.active = screenPostDetail
		a.postDetail = a.postDetail.SetPost(msg.Post)
		return a, a.loadRepliesCmd(msg.Post.ID), true
	case screens.ShowPostForReplyMsg:
		a.postDetailReturn = screenFeed
		a.active = screenPostDetail
		a.postDetail = a.postDetail.SetPost(msg.Post)
		var openCmd tea.Cmd
		a.postDetail, openCmd = a.postDetail.OpenCompose()
		return a, tea.Batch(a.loadRepliesCmd(msg.Post.ID), openCmd), true
	case screens.ShowUserProfileMsg:
		if a.active != screenFeed {
			return a, nil, false
		}
		a.profileReturn = screenFeed
		return a, a.loadUserProfileCmd(msg.Username), true
	case screens.DeletePostMsg:
		if a.active != screenFeed {
			return a, nil, false
		}
		postID := msg.PostID
		return a, a.deletePostCmd(postID, true), true
	case postDeletedMsg:
		if msg.fromFeed {
			// Deleted from feed: remove locally.
			a.feed = a.feed.RemovePost(msg.postID)
		} else {
			// Deleted from post detail: navigate to feed and reload.
			a.active = screenFeed
			return a, a.loadFeedCmd(), true
		}
		return a, nil, true
	}
	return a, nil, false
}

// handlePostDetail processes post detail, reply, and compose messages.
func (a App) handlePostDetail(msg tea.Msg) (App, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case repliesLoadedMsg:
		a.postDetail = a.postDetail.SetReplies(msg.replies)
		if a.pendingReplyID != "" {
			a.postDetail = a.postDetail.ScrollToReply(a.pendingReplyID)
			a.pendingReplyID = ""
		}
		return a, nil, true
	case screens.SubmitNewPostMsg:
		return a, a.createPostCmd(msg.Content, msg.Topics), true
	case postCreatedMsg:
		return a, a.loadFeedCmd(), true
	case screens.SubmitReplyMsg:
		return a, a.createReplyCmd(msg.PostID, msg.Content, msg.ParentReplyID), true
	case replyCreatedMsg:
		return a, a.loadRepliesCmd(msg.postID), true
	case screens.BackToFeedMsg:
		a.active = a.postDetailReturn
		return a, nil, true
	case screens.ShowUserProfileMsg:
		if a.active != screenPostDetail {
			return a, nil, false
		}
		a.profileReturn = screenPostDetail
		return a, a.loadUserProfileCmd(msg.Username), true
	case screens.DeletePostMsg:
		if a.active != screenPostDetail {
			return a, nil, false
		}
		postID := msg.PostID
		return a, a.deletePostCmd(postID, false), true
	case screens.DeleteReplyMsg:
		return a, a.deleteReplyCmd(msg.ReplyID), true
	case replyDeletedMsg:
		a.postDetail = a.postDetail.RemoveReply(msg.replyID)
		return a, nil, true
	}
	return a, nil, false
}

// handleChatrooms processes chatroom messages.
func (a App) handleChatrooms(msg tea.Msg) (App, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case roomsLoadedMsg:
		a.chatrooms = a.chatrooms.SetRooms(msg.rooms)
		return a, nil, true
	case screens.SendRoomMessageMsg:
		return a, a.sendRoomMessageCmd(msg.RoomID, msg.Body), true
	}
	return a, nil, false
}

// handleCMail processes C-Mail messages. DM subscription lifecycle is managed
// entirely within CMailModel; only the conversation list load and message send
// are coordinated here.
func (a App) handleCMail(msg tea.Msg) (App, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case convsLoadedMsg:
		a.cmail = a.cmail.SetConversations(msg.convs)
		return a, nil, true
	case screens.SendCMailMsg:
		return a, a.sendCMailCmd(msg.ConversationID, msg.Body), true
	}
	return a, nil, false
}

// handleProfile processes profile load, save, and sub-tab messages.
func (a App) handleProfile(msg tea.Msg) (App, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case profileLoadedMsg:
		a.currentUser = msg.user
		a.profile = a.profile.ClearTabs().SetUser(msg.user).SetCanGoBack(false)
		// Propagate the confirmed username to screens that guard own-content actions.
		a.feed = a.feed.SetCurrentUsername(msg.user.Username)
		a.postDetail = a.postDetail.SetCurrentUsername(msg.user.Username)
		return a, nil, true
	case userProfileLoadedMsg:
		isOwn := msg.user.Username == a.currentUser.Username
		// Clear stale sub-tab data whenever a different profile is loaded.
		a.profile = a.profile.ClearTabs().SetUser(msg.user).SetReadOnly(!isOwn).SetCanGoBack(true).SetFollowState(msg.isFollowing, msg.followID)
		a.active = screenProfile
		return a, nil, true
	case screens.BackFromProfileMsg:
		a.active = a.profileReturn
		a.profile = a.profile.SetReadOnly(false).SetCanGoBack(false).SetFollowState(false, "")
		return a, nil, true
	case screens.SaveProfileMsg:
		return a, a.saveProfileCmd(msg), true
	case screens.FollowUserMsg:
		return a, a.followUserCmd(msg.UserID), true
	case screens.UnfollowUserMsg:
		return a, a.unfollowUserCmd(msg.FollowID), true
	case followResultMsg:
		a.profile = a.profile.SetFollowState(true, msg.followID).IncrementFollowersCount(1).SetFollowFeedback("following.")
		return a, nil, true
	case unfollowResultMsg:
		a.profile = a.profile.SetFollowState(false, "").IncrementFollowersCount(-1).SetFollowFeedback("unfollowed.")
		return a, nil, true

	// --- sub-tab lazy-load triggers ---
	case screens.ShowUserPostsMsg:
		return a, a.loadUserPostsCmd(msg.Username, ""), true
	case screens.ShowUserRepliesMsg:
		return a, a.loadUserRepliesCmd(msg.Username, ""), true
	case screens.ShowUserFollowingMsg:
		return a, a.loadUserFollowingCmd(msg.UserID, ""), true
	case screens.ShowUserFollowersMsg:
		return a, a.loadUserFollowersCmd(msg.UserID, ""), true

	// --- sub-tab pagination ---
	case screens.LoadMoreUserPostsMsg:
		return a, a.loadUserPostsCmd(msg.Username, msg.Cursor), true
	case screens.LoadMoreUserRepliesMsg:
		return a, a.loadUserRepliesCmd(msg.Username, msg.Cursor), true
	case screens.LoadMoreUserFollowingMsg:
		return a, a.loadUserFollowingCmd(msg.UserID, msg.Cursor), true
	case screens.LoadMoreUserFollowersMsg:
		return a, a.loadUserFollowersCmd(msg.UserID, msg.Cursor), true

	// --- sub-tab data results ---
	case userPostsLoadedMsg:
		a.profile = a.profile.SetUserPosts(msg.posts, msg.cursor)
		return a, nil, true
	case userPostsPageMsg:
		a.profile = a.profile.AppendUserPosts(msg.posts, msg.cursor)
		return a, nil, true
	case userRepliesLoadedMsg:
		a.profile = a.profile.SetUserReplies(msg.replies, msg.cursor)
		return a, nil, true
	case userRepliesPageMsg:
		a.profile = a.profile.AppendUserReplies(msg.replies, msg.cursor)
		return a, nil, true
	case userFollowingLoadedMsg:
		a.profile = a.profile.SetUserFollowing(msg.follows, msg.cursor)
		return a, nil, true
	case userFollowingPageMsg:
		a.profile = a.profile.AppendUserFollowing(msg.follows, msg.cursor)
		return a, nil, true
	case userFollowersLoadedMsg:
		a.profile = a.profile.SetUserFollowers(msg.follows, msg.cursor)
		return a, nil, true
	case userFollowersPageMsg:
		a.profile = a.profile.AppendUserFollowers(msg.follows, msg.cursor)
		return a, nil, true

	// --- navigation from sub-tabs ---
	case screens.ShowProfilePostMsg:
		// Navigate to post detail; return to profile when ESC is pressed.
		a.postDetailReturn = screenProfile
		a.active = screenPostDetail
		a.postDetail = a.postDetail.SetPost(msg.Post)
		return a, a.loadRepliesCmd(msg.Post.ID), true
	case screens.ShowProfileReplyMsg:
		// Navigate to a post thread from the Replies tab; fetch the full post and scroll to the reply.
		a.postDetailReturn = screenProfile
		a.active = screenPostDetail
		a.pendingReplyID = msg.ReplyID
		a.postDetail = a.postDetail.SetPost(model.Post{ID: msg.PostID})
		return a, tea.Batch(a.loadProfilePostCmd(msg.PostID), a.loadRepliesCmd(msg.PostID)), true
	case profilePostLoadedMsg:
		a.postDetail = a.postDetail.SetPost(msg.post)
		return a, nil, true
	case screens.ShowUserProfileMsg:
		// Only intercept when the profile screen is active (e.g. from Following/Followers tab).
		if a.active != screenProfile {
			return a, nil, false
		}
		// Navigate to the new user's profile; returning will go to the current profileReturn.
		return a, a.loadUserProfileCmd(msg.Username), true
	}
	return a, nil, false
}

func (a App) handleSettings(msg tea.Msg) (App, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case settingsLoadedMsg:
		a.settings = msg.settings
		a.settingsScreen = a.settingsScreen.SetSettings(msg.settings)
		a.broadcastConfig()
		return a, nil, true

	case screens.SaveSettingsMsg:
		s := msg.Settings
		wl := msg.WanderLust
		td := msg.MaxThreadDepth
		return a, func() tea.Msg {
			if err := a.client.UpdateSettings(s); err != nil {
				return errMsg{err}
			}
			if cfg, err := config.Load(); err == nil {
				cfg.WanderLust = wl
				cfg.MaxThreadDepth = td
				_ = config.Save(cfg)
			}
			return settingsSavedMsg{settings: s, wanderLust: wl, maxThreadDepth: td}
		}, true

	case settingsSavedMsg:
		a.settings = msg.settings
		a.wanderLust = msg.wanderLust
		a.maxThreadDepth = msg.maxThreadDepth
		a.settingsScreen = a.settingsScreen.SetSaved(msg.wanderLust, msg.maxThreadDepth)
		a.broadcastConfig()
		return a, nil, true

	case wanderTickMsg:
		return a, tea.Batch(a.checkAndWanderCmd(), a.scheduleWanderCmd()), true

	case wanderDoneMsg:
		if !msg.at.IsZero() {
			if cfg, err := config.Load(); err == nil {
				cfg.LastWandered = msg.at
				_ = config.Save(cfg)
			}
		}
		return a, nil, true
	}
	return a, nil, false
}

// handleBookmarks processes bookmark load, create, and delete messages.
func (a App) handleBookmarks(msg tea.Msg) (App, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case bookmarksLoadedMsg:
		a.bookmarks = a.bookmarks.SetBookmarks(msg.items, msg.cursor)
		a.bookmarkedPostIDs, a.bookmarkedReplyIDs = bookmarkIDSets(msg.items)
		a.broadcastBookmarkedIDs()
		return a, nil, true
	case bookmarksPageMsg:
		a.bookmarks = a.bookmarks.AppendBookmarks(msg.items, msg.cursor)
		a.bookmarkedPostIDs, a.bookmarkedReplyIDs = mergeBookmarkIDSets(
			a.bookmarkedPostIDs, a.bookmarkedReplyIDs, msg.items)
		a.broadcastBookmarkedIDs()
		return a, nil, true
	case screens.LoadMoreBookmarksMsg:
		return a, a.loadBookmarksPageCmd(msg.Cursor), true
	case screens.OpenBookmarkMsg:
		if msg.PostID != "" {
			return a, a.loadBookmarkPostCmd(msg.PostID), true
		}
		if msg.ReplyID != "" {
			return a, a.loadBookmarkReplyCmd(msg.ReplyID), true
		}
		return a, nil, true
	case bookmarkPostLoadedMsg:
		a.postDetailReturn = screenBookmarks
		a.active = screenPostDetail
		a.postDetail = a.postDetail.SetPost(msg.post)
		return a, a.loadRepliesCmd(msg.post.ID), true
	case bookmarkReplyLoadedMsg:
		a.postDetailReturn = screenBookmarks
		a.active = screenPostDetail
		a.postDetail = a.postDetail.SetPost(msg.post)
		a.pendingReplyID = msg.replyID
		return a, a.loadRepliesCmd(msg.post.ID), true
	case screens.BookmarkPostMsg:
		postID := msg.PostID
		newPostIDs := make(map[string]struct{}, len(a.bookmarkedPostIDs)+1)
		for k := range a.bookmarkedPostIDs {
			newPostIDs[k] = struct{}{}
		}
		newPostIDs[postID] = struct{}{}
		a.bookmarkedPostIDs = newPostIDs
		a.broadcastBookmarkedIDs()
		return a, a.createBookmarkCmd(postID, ""), true
	case bookmarkCreatedMsg:
		if msg.err != nil {
			newPostIDs := make(map[string]struct{}, len(a.bookmarkedPostIDs))
			for k := range a.bookmarkedPostIDs {
				if k != msg.postID {
					newPostIDs[k] = struct{}{}
				}
			}
			a.bookmarkedPostIDs = newPostIDs
			a.broadcastBookmarkedIDs()
			return a, nil, true
		}
		a.bookmarks = a.bookmarks.SetFetching()
		return a, a.loadBookmarksCmd(""), true
	case screens.DeleteBookmarkMsg:
		// Optimistic update already applied in BookmarksModel.Update; remove from sets.
		if msg.PostID != "" {
			newPostIDs := make(map[string]struct{}, len(a.bookmarkedPostIDs))
			for k := range a.bookmarkedPostIDs {
				if k != msg.PostID {
					newPostIDs[k] = struct{}{}
				}
			}
			a.bookmarkedPostIDs = newPostIDs
		}
		if msg.ReplyID != "" {
			newReplyIDs := make(map[string]struct{}, len(a.bookmarkedReplyIDs))
			for k := range a.bookmarkedReplyIDs {
				if k != msg.ReplyID {
					newReplyIDs[k] = struct{}{}
				}
			}
			a.bookmarkedReplyIDs = newReplyIDs
		}
		a.broadcastBookmarkedIDs()
		return a, a.deleteBookmarkCmd(msg.BookmarkID), true
	case bookmarkDeletedMsg:
		// Fire-and-forget; UI already updated.
		return a, nil, true
	}
	return a, nil, false
}

// bookmarkIDSets builds post and reply ID sets from a fresh bookmark page.
func bookmarkIDSets(items []model.Bookmark) (map[string]struct{}, map[string]struct{}) {
	postIDs := make(map[string]struct{})
	replyIDs := make(map[string]struct{})
	for _, b := range items {
		if b.PostID != "" {
			postIDs[b.PostID] = struct{}{}
		}
		if b.ReplyID != "" {
			replyIDs[b.ReplyID] = struct{}{}
		}
	}
	return postIDs, replyIDs
}

// mergeBookmarkIDSets merges a new page of bookmarks into existing ID sets.
func mergeBookmarkIDSets(postIDs, replyIDs map[string]struct{}, items []model.Bookmark) (map[string]struct{}, map[string]struct{}) {
	newPostIDs := make(map[string]struct{}, len(postIDs)+len(items))
	for k := range postIDs {
		newPostIDs[k] = struct{}{}
	}
	newReplyIDs := make(map[string]struct{}, len(replyIDs)+len(items))
	for k := range replyIDs {
		newReplyIDs[k] = struct{}{}
	}
	for _, b := range items {
		if b.PostID != "" {
			newPostIDs[b.PostID] = struct{}{}
		}
		if b.ReplyID != "" {
			newReplyIDs[b.ReplyID] = struct{}{}
		}
	}
	return newPostIDs, newReplyIDs
}

// handleTopics processes topic list, topic posts, pagination, and post selection messages.
func (a App) handleTopics(msg tea.Msg) (App, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case screens.RefreshTopicsMsg:
		return a, a.loadTopicsCmd(), true

	case topicsLoadedMsg:
		a.topics = a.topics.SetTopics(msg.topics, msg.cursor)
		return a, nil, true

	case screens.LoadMoreTopicsMsg:
		return a, a.loadMoreTopicsCmd(msg.Cursor), true

	case topicsPageMsg:
		a.topics = a.topics.AppendTopics(msg.topics, msg.cursor)
		return a, nil, true

	case screens.LoadTopicPostsMsg:
		return a, a.loadTopicPostsCmd(msg.Slug), true

	case topicPostsLoadedMsg:
		a.topics = a.topics.SetTopicPosts(msg.posts, msg.cursor)
		return a, nil, true

	case screens.LoadMoreTopicPostsMsg:
		return a, a.loadTopicPostsPageCmd(msg.Slug, msg.Cursor), true

	case topicPostsPageMsg:
		a.topics = a.topics.AppendTopicPosts(msg.posts, msg.cursor)
		return a, nil, true

	case screens.RefreshTopicPostsMsg:
		return a, a.loadTopicPostsCmd(msg.Slug), true

	case screens.ShowTopicPostMsg:
		a.postDetailReturn = screenTopics
		a.active = screenPostDetail
		a.postDetail = a.postDetail.SetPost(msg.Post)
		return a, a.loadRepliesCmd(msg.Post.ID), true
	}
	return a, nil, false
}

// handleJournal processes journal (Notes) load, save, delete, and publish messages.
func (a App) handleJournal(msg tea.Msg) (App, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case journalLoadedMsg:
		a.journal = a.journal.SetNotes(msg.notes, msg.cursor)
		return a, nil, true
	case journalPageMsg:
		a.journal = a.journal.AppendNotes(msg.notes, msg.cursor)
		return a, nil, true
	case screens.LoadMoreJournalMsg:
		return a, a.loadJournalPageCmd(msg.Cursor), true
	case screens.SubmitSaveNoteMsg:
		return a, a.saveNoteCmd(msg.NoteID, msg.Content, msg.Topics), true
	case noteCreatedMsg:
		a.journal = a.journal.PrependNote(msg.note)
		return a, nil, true
	case noteUpdatedMsg:
		a.journal = a.journal.UpdateNoteContent(msg.noteID, msg.content, msg.topics)
		return a, nil, true
	case screens.SubmitDeleteNoteMsg:
		return a, a.deleteNoteCmd(msg.NoteID), true
	case noteDeletedMsg:
		a.journal = a.journal.DeleteNote(msg.noteID)
		return a, nil, true
	case screens.SubmitPublishNoteMsg:
		return a, a.publishNoteCmd(msg.Content, msg.Topics), true
	case notePublishedMsg:
		return a, nil, true
	case screens.LoadNoteRevisionsMsg:
		return a, a.loadNoteRevisionsCmd(msg.NoteID, ""), true
	case screens.LoadNoteRevisionMsg:
		return a, a.loadNoteRevisionCmd(msg.NoteID, msg.RevisionNumber), true
	case noteRevisionsLoadedMsg:
		a.journal = a.journal.SetRevisions(msg.noteID, msg.revisions, msg.cursor)
		return a, nil, true
	case noteRevisionPreviewMsg:
		a.journal = a.journal.SetRevisionPreview(msg.note)
		return a, nil, true
	}
	return a, nil, false
}

// handleErr routes API error messages to the active screen's error display.
func (a App) handleErr(msg tea.Msg) (App, tea.Cmd, bool) {
	m, ok := msg.(errMsg)
	if !ok {
		return a, nil, false
	}
	switch a.active {
	case screenFeed:
		a.feed = a.feed.SetError(m.err)
	case screenCMail:
		a.cmail = a.cmail.SetError(m.err)
	case screenProfile:
		a.profile = a.profile.SetError(m.err)
	case screenPostDetail:
		a.postDetail = a.postDetail.SetError(m.err)
	case screenNotifications:
		a.notifications = a.notifications.SetError(m.err)
	case screenSettings:
		a.settingsScreen = a.settingsScreen.SetError(m.err)
	case screenBookmarks:
		a.bookmarks = a.bookmarks.SetError(m.err)
	case screenTopics:
		a.topics = a.topics.SetError(m.err)
	case screenJournal:
		a.journal = a.journal.SetError(m.err)
	}
	return a, nil, true
}

// activeScreenHasFocusedInput returns true when the current screen has a
// text input that is focused, preventing arrow keys from being consumed by
// the tab navigator instead.
func (a App) activeScreenHasFocusedInput() bool {
	switch a.active {
	case screenChatrooms:
		return a.chatrooms.InputFocused()
	case screenCMail:
		return a.cmail.InputFocused()
	case screenPostDetail:
		return a.postDetail.ComposeActive()
	case screenFeed:
		return a.feed.ComposeActive()
	case screenProfile:
		return a.profile.ComposeActive()
	case screenJournal:
		return a.journal.ComposeActive()
	}
	return false
}

// tabIndex returns the index of the currently active screen within menuTabs.
func (a App) tabIndex() int {
	for i, t := range menuTabs {
		if t.s == a.active {
			return i
		}
	}
	return 0
}

// navigateTab moves the active tab by delta (-1 or +1), wrapping at the ends.
func (a *App) navigateTab(delta int) tea.Cmd {
	if a.active == screenCMail {
		a.cmail = a.cmail.CancelSubscription()
	}
	idx := (a.tabIndex() + delta + len(menuTabs)) % len(menuTabs)
	a.active = menuTabs[idx].s
	switch a.active {
	case screenFeed:
		return a.loadFeedCmd()
	case screenChatrooms:
		return a.loadRoomsCmd()
	case screenCMail:
		return a.loadConvsCmd()
	case screenProfile:
		return a.loadProfileCmd()
	case screenNotifications:
		return a.loadNotifsCmd()
	case screenSettings:
		return nil // no load cmd; settings already in memory
	case screenBookmarks:
		if !a.bookmarks.IsLoaded() {
			a.bookmarks = a.bookmarks.SetFetching()
			return a.loadBookmarksCmd("")
		}
		return nil
	case screenTopics:
		return a.loadTopicsCmd()
	case screenJournal:
		return a.loadJournalCmd()
	}
	return nil
}

func (a *App) delegateUpdate(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	switch a.active {
	case screenLogin:
		a.login, cmd = a.login.Update(msg)
	case screenFeed:
		a.feed, cmd = a.feed.Update(msg)
	case screenChatrooms:
		a.chatrooms, cmd = a.chatrooms.Update(msg)
	case screenCMail:
		a.cmail, cmd = a.cmail.Update(msg)
	case screenProfile:
		a.profile, cmd = a.profile.Update(msg)
	case screenPostDetail:
		a.postDetail, cmd = a.postDetail.Update(msg)
	case screenNotifications:
		a.notifications, cmd = a.notifications.Update(msg)
	case screenSettings:
		a.settingsScreen, cmd = a.settingsScreen.Update(msg)
	case screenBookmarks:
		a.bookmarks, cmd = a.bookmarks.Update(msg)
	case screenTopics:
		a.topics, cmd = a.topics.Update(msg)
	case screenJournal:
		a.journal, cmd = a.journal.Update(msg)
	}
	return cmd
}

// --- view ---

func (a App) View() string {
	if a.active == screenLogin {
		return a.login.View()
	}
	// Fix the content area to the available height so the status bar
	// is always anchored to the bottom regardless of content length.
	contentHeight := a.height - theme.ChromeHeight
	content := lipgloss.NewStyle().Height(contentHeight).MaxHeight(contentHeight).Render(a.renderActiveScreen())
	base := lipgloss.JoinVertical(lipgloss.Left,
		a.renderTabBar(),
		"", // separator row
		content,
		a.renderStatusBar(),
	)
	if a.themePickerOpen {
		return overlayCenter(base, a.renderThemePicker(), a.width, a.height)
	}
	if a.timezonePickerOpen {
		return overlayCenter(base, a.renderTimezonePicker(), a.width, a.height)
	}
	if a.helpModalOpen {
		return overlayCenter(base, a.renderHelpModal(), a.width, a.height)
	}
	if a.urlPickerOpen {
		return overlayCenter(base, a.renderURLPicker(), a.width, a.height)
	}
	return base
}

func (a App) renderTabBar() string {
	var bar string
	for _, t := range menuTabs {
		label := t.label
		if t.s == screenNotifications && a.polledUnreadCount > 0 {
			label = fmt.Sprintf("%s (%d)", label, a.polledUnreadCount)
		}
		if a.active == t.s {
			bar += theme.ActiveTab.Render(label)
		} else {
			bar += theme.Tab.Render(label)
		}
	}
	return bar
}

func (a App) renderActiveScreen() string {
	switch a.active {
	case screenFeed:
		return a.feed.View()
	case screenChatrooms:
		return a.chatrooms.View()
	case screenCMail:
		return a.cmail.View()
	case screenProfile:
		return a.profile.View()
	case screenPostDetail:
		return a.postDetail.View()
	case screenNotifications:
		return a.notifications.View()
	case screenSettings:
		return a.settingsScreen.View()
	case screenBookmarks:
		return a.bookmarks.View()
	case screenTopics:
		return a.topics.View()
	case screenJournal:
		return a.journal.View()
	}
	return ""
}

func (a App) renderStatusBar() string {
	userStyle := theme.StatusBar.Copy().Foreground(theme.ColorCyan)
	metaStyle := theme.StatusBar.Copy().Foreground(theme.ColorMuted)
	densityLabel := "dense"
	if a.relaxed {
		densityLabel = "relaxed"
	}
	tzLabel := a.timezone
	if tzLabel == "" {
		tzLabel = "UTC"
	}
	timeFmt := a.settings.TimeDisplayFormat
	if timeFmt == "" {
		timeFmt = "datetime"
	}
	user := lipgloss.JoinHorizontal(lipgloss.Top,
		userStyle.Render("@"+a.currentUser.Username),
		metaStyle.Render("  ·  "+densityLabel),
		metaStyle.Render("  ·  "+theme.CurrentName()),
		metaStyle.Render("  ·  "+tzLabel),
		metaStyle.Render("  ·  "+timeFmt),
	)
	hint := theme.StatusBar.Render("  ? · show shortcuts")
	spacer := theme.StatusBar.Width(a.width - lipgloss.Width(user) - lipgloss.Width(hint)).Render("")
	return lipgloss.JoinHorizontal(lipgloss.Top, user, spacer, hint)
}

// --- theme picker ---

// themeIndex returns the index of name in availableThemes, defaulting to 0.
func themeIndex(name string) int {
	for i, t := range availableThemes {
		if t == name {
			return i
		}
	}
	return 0
}

// handleThemePickerKey processes keyboard input while the theme picker is open.
func (a App) handleThemePickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	refreshCmd := func() tea.Msg { return tea.WindowSizeMsg{Width: a.width, Height: a.height} }
	switch msg.String() {
	case "up", "k":
		a.themePickerCursor = (a.themePickerCursor - 1 + len(availableThemes)) % len(availableThemes)
		theme.Set(availableThemes[a.themePickerCursor])
		a.refreshViewports()
	case "down", "j":
		a.themePickerCursor = (a.themePickerCursor + 1) % len(availableThemes)
		theme.Set(availableThemes[a.themePickerCursor])
		a.refreshViewports()
	case "enter":
		selected := availableThemes[a.themePickerCursor]
		a.themePickerOpen = false
		return a, tea.Batch(
			refreshCmd,
			func() tea.Msg {
				if cfg, err := config.Load(); err == nil {
					cfg.Theme = selected
					_ = config.Save(cfg)
				}
				return nil
			},
		)
	case "esc":
		theme.Set(a.themePickerOrig)
		a.themePickerOpen = false
		return a, refreshCmd
	}
	return a, nil
}

// --- timezone picker ---

// timezoneIndex returns the index of label in config.AvailableTimezones,
// defaulting to the index of "UTC".
func timezoneIndex(label string) int {
	for i, t := range config.AvailableTimezones {
		if t == label {
			return i
		}
	}
	for i, t := range config.AvailableTimezones {
		if t == "UTC" {
			return i
		}
	}
	return 0
}

// handleTimezonePickerKey processes keyboard input while the timezone picker is open.
func (a App) handleTimezonePickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	refreshCmd := func() tea.Msg { return tea.WindowSizeMsg{Width: a.width, Height: a.height} }
	n := len(config.AvailableTimezones)
	switch msg.String() {
	case "up", "k":
		a.timezonePickerCursor = (a.timezonePickerCursor - 1 + n) % n
	case "down", "j":
		a.timezonePickerCursor = (a.timezonePickerCursor + 1) % n
	case "enter":
		selected := config.AvailableTimezones[a.timezonePickerCursor]
		a.timezone = selected
		a.loc = config.ParseTimezoneLabel(selected)
		a.timezonePickerOpen = false
		a.broadcastConfig()
		a.refreshViewports()
		return a, tea.Batch(
			refreshCmd,
			func() tea.Msg {
				if cfg, err := config.Load(); err == nil {
					cfg.Timezone = selected
					_ = config.Save(cfg)
				}
				return nil
			},
		)
	case "esc":
		a.timezonePickerOpen = false
		return a, refreshCmd
	}
	return a, nil
}

// renderTimezonePicker returns the centered overlay box for timezone selection.
// Shows a scrolling window of 13 items centered on the cursor.
func (a App) renderTimezonePicker() string {
	title := theme.Title.Render("timezone")
	zones := config.AvailableTimezones
	n := len(zones)
	const window = 13
	start := a.timezonePickerCursor - window/2
	if start < 0 {
		start = 0
	}
	if start+window > n {
		start = n - window
	}
	var items []string
	for i := start; i < start+window; i++ {
		if i == a.timezonePickerCursor {
			items = append(items, theme.Highlight.Render("▸ "+zones[i]))
		} else {
			items = append(items, theme.Subtle.Render("  "+zones[i]))
		}
	}
	hint := theme.Subtle.Render("↑↓ select   enter save   esc cancel")
	body := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		lipgloss.JoinVertical(lipgloss.Left, items...),
		"",
		hint,
	)
	return theme.ActiveBorder.Render(body)
}

// refreshViewports forces all screen viewports to re-render with the current
// theme by re-broadcasting the current terminal size. Called synchronously so
// View() sees fresh content in the same frame.
func (a *App) refreshViewports() {
	msg := tea.WindowSizeMsg{Width: a.width, Height: a.height}
	a.feed, _ = a.feed.Update(msg)
	a.chatrooms, _ = a.chatrooms.Update(msg)
	a.cmail, _ = a.cmail.Update(msg)
	a.postDetail, _ = a.postDetail.Update(msg)
	a.profile, _ = a.profile.Update(msg)
	a.notifications, _ = a.notifications.Update(msg)
	a.bookmarks, _ = a.bookmarks.Update(msg)
	a.topics, _ = a.topics.Update(msg)
	a.journal, _ = a.journal.Update(msg)
}

// renderThemePicker returns the centered overlay box for theme selection.
func (a App) renderThemePicker() string {
	title := theme.Title.Render("theme")
	var items []string
	for i, name := range availableThemes {
		if i == a.themePickerCursor {
			items = append(items, theme.Highlight.Render("▸ "+name))
		} else {
			items = append(items, theme.Subtle.Render("  "+name))
		}
	}
	hint := theme.Subtle.Render("↑↓ preview   enter save   esc cancel")
	body := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		lipgloss.JoinVertical(lipgloss.Left, items...),
		"",
		hint,
	)
	return theme.ActiveBorder.Render(body)
}

// --- help modal ---

// handleHelpModalKey closes the help modal on any keypress.
func (a App) handleHelpModalKey(_ tea.KeyMsg) (tea.Model, tea.Cmd) {
	a.helpModalOpen = false
	return a, nil
}

// renderHelpModal returns the centered overlay box listing keyboard shortcuts
// for the global scope and the currently active screen.
func (a App) renderHelpModal() string {
	title := theme.Title.Render("shortcuts")
	sectionStyle := theme.Subtle.Copy().Bold(true)
	row := func(key, desc string) string {
		k := theme.Highlight.Render(fmt.Sprintf("%-14s", key))
		return lipgloss.JoinHorizontal(lipgloss.Top, k, theme.Subtle.Render(desc))
	}

	globalSection := lipgloss.JoinVertical(lipgloss.Left,
		sectionStyle.Render("global"),
		row("1-7", "feed · notifs · journal · bookmarks · topics · profile · settings"),
		row("← →", "cycle tabs"),
		row("t", "theme"),
		row("z", "timezone"),
		row("v", "density"),
		row("o", "open url"),
		row("q", "quit"),
	)

	var localSection string
	switch a.active {
	case screenFeed:
		if a.feed.ComposeActive() {
			localSection = lipgloss.JoinVertical(lipgloss.Left,
				sectionStyle.Render("feed (compose)"),
				row("Ctrl+S", "send"),
				row("Tab", "topics"),
				row("Enter", "paragraph"),
				row("Esc", "cancel"),
			)
		} else {
			localSection = lipgloss.JoinVertical(lipgloss.Left,
				sectionStyle.Render("feed"),
				row("↑↓ / jk", "navigate"),
				row("enter", "open post"),
				row("p", "view profile"),
				row("b", "bookmark"),
				row("n", "new post"),
				row("r", "reply"),
				row("d", "delete own"),
			)
		}
	case screenPostDetail:
		if a.postDetail.ComposeActive() {
			localSection = lipgloss.JoinVertical(lipgloss.Left,
				sectionStyle.Render("post detail (compose)"),
				row("Ctrl+S", "send"),
				row("Enter", "paragraph"),
				row("Esc", "cancel"),
			)
		} else {
			localSection = lipgloss.JoinVertical(lipgloss.Left,
				sectionStyle.Render("post detail"),
				row("↑↓ / jk", "scroll / navigate"),
				row("r", "reply"),
				row("d", "delete own"),
				row("p", "view profile"),
				row("b", "bookmark"),
				row("esc", "back"),
			)
		}
	case screenProfile:
		if a.profile.ComposeActive() {
			localSection = lipgloss.JoinVertical(lipgloss.Left,
				sectionStyle.Render("profile (editing)"),
				row("tab/shift+tab", "cycle fields"),
				row("Ctrl+S", "save"),
				row("Esc", "cancel"),
			)
		} else if a.profile.IsReadOnly() {
			localSection = lipgloss.JoinVertical(lipgloss.Left,
				sectionStyle.Render("profile"),
				row("tab/shift+tab", "switch tab"),
				row("j/k", "navigate"),
				row("enter", "open"),
				row("f", "follow / unfollow"),
				row("esc", "back"),
			)
		} else {
			localSection = lipgloss.JoinVertical(lipgloss.Left,
				sectionStyle.Render("profile (own)"),
				row("tab/shift+tab", "switch tab"),
				row("j/k", "navigate"),
				row("enter", "open"),
				row("e", "edit profile"),
				row("esc", "back"),
			)
		}
	case screenNotifications:
		localSection = lipgloss.JoinVertical(lipgloss.Left,
			sectionStyle.Render("notifications"),
			row("↑↓ / jk", "navigate"),
			row("enter", "open"),
			row("m", "mark read"),
			row("M", "mark all read"),
			row("u", "toggle unread filter"),
			row("p", "view profile"),
		)
	case screenJournal:
		if a.journal.ComposeActive() {
			localSection = lipgloss.JoinVertical(lipgloss.Left,
				sectionStyle.Render("journal (editing)"),
				row("Ctrl+S", "save"),
				row("Ctrl+P", "publish as post"),
				row("Tab", "topics"),
				row("Enter", "paragraph"),
				row("Esc", "cancel"),
			)
		} else {
			localSection = lipgloss.JoinVertical(lipgloss.Left,
				sectionStyle.Render("journal"),
				row("↑↓ / jk", "navigate"),
				row("enter", "edit note"),
				row("n", "new note"),
				row("d", "delete"),
				row("h", "revision history"),
			)
		}
	case screenBookmarks:
		localSection = lipgloss.JoinVertical(lipgloss.Left,
			sectionStyle.Render("bookmarks"),
			row("↑↓ / jk", "navigate"),
			row("enter", "open post"),
			row("d", "delete"),
		)
	case screenTopics:
		localSection = lipgloss.JoinVertical(lipgloss.Left,
			sectionStyle.Render("topics"),
			row("↑↓ / jk", "navigate"),
			row("enter", "browse / open"),
			row("esc", "back"),
		)
	case screenSettings:
		if a.settingsScreen.IsDirty() {
			localSection = lipgloss.JoinVertical(lipgloss.Left,
				sectionStyle.Render("settings (unsaved changes)"),
				row("Ctrl+S", "save"),
				row("Esc", "revert"),
			)
		} else {
			localSection = lipgloss.JoinVertical(lipgloss.Left,
				sectionStyle.Render("settings"),
				row("↑↓ / jk", "navigate"),
				row("space/enter", "toggle"),
				row("tab/shift+tab", "cycle"),
			)
		}
	case screenCMail:
		localSection = lipgloss.JoinVertical(lipgloss.Left,
			sectionStyle.Render("c-mail"),
			row("← →", "switch pane"),
			row("j/k", "navigate"),
			row("enter", "send"),
		)
	}

	body := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		globalSection,
		"",
		localSection,
		"",
		theme.Subtle.Render("? or any key · close"),
		renderedVersionLine,
	)
	return theme.ActiveBorder.Render(body)
}

// getFocusedURLs returns URLs from the currently selected item on the active screen.
func (a App) getFocusedURLs() []string {
	var p screens.URLProvider
	switch a.active {
	case screenFeed:
		p = a.feed
	case screenPostDetail:
		p = a.postDetail
	case screenProfile:
		p = a.profile
	case screenBookmarks:
		p = a.bookmarks
	case screenTopics:
		p = a.topics
	case screenJournal:
		p = a.journal
	}
	if p == nil {
		return nil
	}
	return p.GetFocusedURLs()
}

// handleOpenURL opens the given URLs: nothing if empty, direct open if one,
// or shows the picker if multiple.
func (a App) handleOpenURL(urls []string) (App, tea.Cmd) {
	if len(urls) == 0 {
		return a, nil
	}
	if len(urls) == 1 {
		return a.routeURL(urls[0])
	}
	a.urlPickerOpen = true
	a.urlPickerItems = urls
	a.urlPickerCursor = 0
	return a, nil
}

// routeURL navigates to an internal screen for known cyberspace.online paths,
// or opens the URL in the OS browser for everything else.
func (a App) routeURL(rawURL string) (App, tea.Cmd) {
	parsed, err := neturl.Parse(rawURL)
	if err != nil {
		return a, openExternalURL(rawURL)
	}
	if parsed.Host == "cyberspace.online" || parsed.Host == "www.cyberspace.online" {
		parts := strings.SplitN(strings.TrimPrefix(parsed.Path, "/"), "/", 3)
		if len(parts) >= 2 && parts[0] == "u" && parts[1] != "" {
			a.profileReturn = a.active
			return a, a.loadUserProfileCmd(parts[1])
		}
	}
	return a, openExternalURL(rawURL)
}

// openExternalURL opens u in the OS default browser as a fire-and-forget command.
func openExternalURL(u string) tea.Cmd {
	return func() tea.Msg {
		_ = urlutil.OpenURL(u)
		return nil
	}
}

// handleURLPickerKey processes keyboard input while the URL picker overlay is open.
func (a App) handleURLPickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := len(a.urlPickerItems)
	switch msg.String() {
	case "up", "k":
		a.urlPickerCursor = (a.urlPickerCursor - 1 + n) % n
	case "down", "j":
		a.urlPickerCursor = (a.urlPickerCursor + 1) % n
	case "enter":
		u := a.urlPickerItems[a.urlPickerCursor]
		a.urlPickerOpen = false
		a.urlPickerItems = nil
		return a.routeURL(u)
	case "esc":
		a.urlPickerOpen = false
		a.urlPickerItems = nil
	}
	return a, nil
}

// renderURLPicker returns the URL picker overlay shown when the focused item has
// multiple openable URLs.
func (a App) renderURLPicker() string {
	title := theme.Title.Render("open url")
	items := make([]string, len(a.urlPickerItems))
	for i, u := range a.urlPickerItems {
		display := u
		if len(display) > 60 {
			display = display[:57] + "..."
		}
		if i == a.urlPickerCursor {
			items[i] = theme.Highlight.Render("▸ " + display)
		} else {
			items[i] = theme.Subtle.Render("  " + display)
		}
	}
	hint := theme.Subtle.Render("↑↓ select   enter open   esc cancel")
	rows := append([]string{title, ""}, items...)
	rows = append(rows, "", hint)
	return theme.ActiveBorder.Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
}

// overlayCenter composites fg centered over bg using ANSI-aware string splicing.
// Each line of fg replaces the corresponding characters in bg at the centered
// position, preserving ANSI colour codes on both sides of the splice point.
func overlayCenter(bg, fg string, bgW, bgH int) string {
	fgW := lipgloss.Width(fg)
	fgLines := strings.Split(fg, "\n")
	fgH := len(fgLines)
	bgLines := strings.Split(bg, "\n")

	xOff := (bgW - fgW) / 2
	yOff := (bgH - fgH) / 2
	if xOff < 0 {
		xOff = 0
	}
	if yOff < 0 {
		yOff = 0
	}

	result := make([]string, len(bgLines))
	copy(result, bgLines)

	for i, fgLine := range fgLines {
		bi := yOff + i
		if bi < 0 || bi >= len(result) {
			continue
		}
		bgLine := result[bi]
		// Pad the background line if it's shorter than the splice end point.
		bgLineW := ansi.StringWidth(bgLine)
		needed := xOff + fgW
		if bgLineW < needed {
			bgLine += strings.Repeat(" ", needed-bgLineW)
		}
		left := ansi.Truncate(bgLine, xOff, "")
		right := ansi.TruncateLeft(bgLine, xOff+fgW, "")
		result[bi] = left + fgLine + right
	}
	return strings.Join(result, "\n")
}

// --- commands ---

func (a *App) loginCmd(email, password string) tea.Cmd {
	return func() tea.Msg {
		tokens, err := a.client.Login(email, password)
		if err != nil {
			return screens.LoginErrMsg{Err: err}
		}
		a.tokens = tokens
		// Initialise the RTDB client from the rtdbToken (best effort).
		if hc, ok := a.client.(*api.HTTPClient); ok {
			_ = hc.InitRTDB(tokens.RTDBToken)
		}
		user, err := a.client.GetOwnProfile()
		if err != nil {
			return screens.LoginErrMsg{Err: err}
		}
		a.currentUser = user
		// Wire the user ID into the HTTP client for RTDB path construction.
		if hc, ok := a.client.(*api.HTTPClient); ok {
			hc.SetCurrentUID(user.ID)
		}
		a.cmail = screens.NewCMailModel(user.Username, a.client)
		// Persist the refresh token so subsequent launches auto-login.
		// Load first so app settings (APIBaseURL, etc.) are preserved.
		density := ""
		if a.relaxed {
			density = "relaxed"
		}
		if cfg, err := config.Load(); err == nil {
			cfg.RefreshToken = tokens.RefreshToken
			cfg.Username = user.Username
			cfg.Email = email
			cfg.SavedAt = time.Now().UTC()
			cfg.Density = density
			_ = config.Save(cfg)
		}
		return screens.LoginMsg{}
	}
}

// tokenLoginCmd resumes a saved session by exchanging the stored refresh token
// for fresh API tokens, then fetches the user profile. On failure it falls back
// to the login screen by returning a LoginErrMsg.
func (a *App) tokenLoginCmd(refreshToken string) tea.Cmd {
	return func() tea.Msg {
		tokens, err := a.client.LoginWithRefreshToken(refreshToken)
		if err != nil {
			return screens.LoginErrMsg{Err: err}
		}
		a.tokens = tokens
		if hc, ok := a.client.(*api.HTTPClient); ok {
			_ = hc.InitRTDB(tokens.RTDBToken)
		}
		user, err := a.client.GetOwnProfile()
		if err != nil {
			return screens.LoginErrMsg{Err: err}
		}
		a.currentUser = user
		if hc, ok := a.client.(*api.HTTPClient); ok {
			hc.SetCurrentUID(user.ID)
		}
		a.cmail = screens.NewCMailModel(user.Username, a.client)
		// Update savedAt so we know when the session was last used.
		// Load first so app settings (APIBaseURL, etc.) are preserved.
		density := ""
		if a.relaxed {
			density = "relaxed"
		}
		if cfg, err := config.Load(); err == nil {
			cfg.RefreshToken = tokens.RefreshToken
			cfg.Username = user.Username
			cfg.SavedAt = time.Now().UTC()
			cfg.Density = density
			_ = config.Save(cfg)
		}
		return screens.LoginMsg{}
	}
}

func (a *App) afterLoginCmd() tea.Cmd {
	a.active = screenFeed
	a.profile = a.profile.SetUser(a.currentUser)
	a.feed = a.feed.SetCurrentUsername(a.currentUser.Username)
	a.postDetail = a.postDetail.SetCurrentUsername(a.currentUser.Username)
	a.broadcastConfig()
	return tea.Batch(
		a.loadFeedCmd(),
		a.loadProfileCmd(),
		a.schedulePollCmd(),
		a.loadSettingsCmd(),
		a.scheduleWanderCmd(),
		a.checkAndWanderCmd(),
	)
}

type feedLoadedMsg struct {
	posts  []model.Post
	cursor string
}
type feedPageMsg struct {
	posts  []model.Post
	cursor string
}
type roomsLoadedMsg struct{ rooms []model.Room }
type convsLoadedMsg struct{ convs []model.Conversation }
type profileLoadedMsg struct{ user model.User }
type userProfileLoadedMsg struct {
	user        model.User
	isFollowing bool
	followID    string
}
type followResultMsg struct{ followID string }
type unfollowResultMsg struct{}
type repliesLoadedMsg struct{ replies []model.Reply }
type replyCreatedMsg struct{ postID string }
type replyDeletedMsg struct{ replyID string }
type postCreatedMsg struct{}
type postDeletedMsg struct {
	postID   string
	fromFeed bool // true = delete was triggered from the feed; false = from post detail
}
type settingsLoadedMsg struct{ settings model.Settings }
type settingsSavedMsg struct {
	settings       model.Settings
	wanderLust     bool
	maxThreadDepth int
}
type wanderTickMsg struct{}
type wanderDoneMsg struct{ at time.Time } // zero At means no update was made
type errMsg struct{ err error }
type bookmarksLoadedMsg struct {
	items  []model.Bookmark
	cursor string
}
type bookmarksPageMsg struct {
	items  []model.Bookmark
	cursor string
}
type bookmarkCreatedMsg struct {
	bookmarkID string
	postID     string
	replyID    string
	err        error
}
type bookmarkDeletedMsg struct{ bookmarkID string }
type bookmarkPostLoadedMsg struct{ post model.Post }
type bookmarkReplyLoadedMsg struct {
	post    model.Post
	replyID string
}

type journalLoadedMsg struct {
	notes  []model.Note
	cursor string
}
type journalPageMsg struct {
	notes  []model.Note
	cursor string
}
type noteCreatedMsg struct{ note model.Note }
type noteUpdatedMsg struct {
	noteID  string
	content string
	topics  []string
}
type noteDeletedMsg struct{ noteID string }
type notePublishedMsg struct{}

// --- profile sub-tab result messages ---

type userPostsLoadedMsg struct {
	posts  []model.Post
	cursor string
}
type userPostsPageMsg struct {
	posts  []model.Post
	cursor string
}
type userRepliesLoadedMsg struct {
	replies []model.Reply
	cursor  string
}
type userRepliesPageMsg struct {
	replies []model.Reply
	cursor  string
}
type userFollowingLoadedMsg struct {
	follows []model.Follow
	cursor  string
}
type userFollowingPageMsg struct {
	follows []model.Follow
	cursor  string
}
type userFollowersLoadedMsg struct {
	follows []model.Follow
	cursor  string
}
type userFollowersPageMsg struct {
	follows []model.Follow
	cursor  string
}

// --- note revision result messages ---

type noteRevisionsLoadedMsg struct {
	noteID    string
	revisions []model.NoteRevision
	cursor    string
}
type noteRevisionPreviewMsg struct{ note model.Note }

type topicsLoadedMsg struct {
	topics []model.Topic
	cursor string
}
type topicsPageMsg struct {
	topics []model.Topic
	cursor string
}
type topicPostsLoadedMsg struct {
	posts  []model.Post
	cursor string
}
type topicPostsPageMsg struct {
	posts  []model.Post
	cursor string
}

type notifsLoadedMsg struct {
	notifs []model.Notification
	cursor string
}
type notifsPageMsg struct {
	notifs []model.Notification
	cursor string
}
type notifPostLoadedMsg struct{ post model.Post }
type profilePostLoadedMsg struct{ post model.Post }
type pollUnreadTickMsg struct{}
type unreadCountMsg struct{ count int }

func (a *App) loadFeedCmd() tea.Cmd {
	return func() tea.Msg {
		posts, cursor, err := a.client.GetFeed("")
		if err != nil {
			return errMsg{err}
		}
		return feedLoadedMsg{posts: posts, cursor: cursor}
	}
}

func (a *App) loadFeedPageCmd(cursor string) tea.Cmd {
	return func() tea.Msg {
		posts, nextCursor, err := a.client.GetFeed(cursor)
		if err != nil {
			return errMsg{err}
		}
		return feedPageMsg{posts: posts, cursor: nextCursor}
	}
}

func (a *App) loadRoomsCmd() tea.Cmd {
	return func() tea.Msg {
		rooms, err := a.client.GetRooms()
		if err != nil {
			return errMsg{err}
		}
		return roomsLoadedMsg{rooms}
	}
}

func (a *App) loadConvsCmd() tea.Cmd {
	return func() tea.Msg {
		convs, err := a.client.GetConversations()
		if err != nil {
			return errMsg{err}
		}
		return convsLoadedMsg{convs}
	}
}

func (a *App) loadSettingsCmd() tea.Cmd {
	return func() tea.Msg {
		s, err := a.client.GetSettings()
		if err != nil {
			return errMsg{err}
		}
		return settingsLoadedMsg{s}
	}
}

func (a *App) loadProfileCmd() tea.Cmd {
	return func() tea.Msg {
		user, err := a.client.GetOwnProfile()
		if err != nil {
			return errMsg{err}
		}
		return profileLoadedMsg{user}
	}
}

func (a *App) loadUserProfileCmd(username string) tea.Cmd {
	return func() tea.Msg {
		// Skip the API call if this is the logged-in user's own profile.
		if username == a.currentUser.Username {
			return userProfileLoadedMsg{user: a.currentUser}
		}
		user, err := a.client.GetProfile(username)
		if err != nil {
			return errMsg{err}
		}
		// Detect whether the logged-in user follows this profile by scanning
		// the first page of the following list (up to 50 entries).
		var isFollowing bool
		var followID string
		follows, _, err := a.client.GetFollowing("")
		if err == nil {
			for _, f := range follows {
				if f.FollowedID == user.ID {
					isFollowing = true
					followID = f.ID
					break
				}
			}
		}
		return userProfileLoadedMsg{user: user, isFollowing: isFollowing, followID: followID}
	}
}

func (a *App) followUserCmd(userID string) tea.Cmd {
	return func() tea.Msg {
		followID, err := a.client.Follow(userID)
		if err != nil {
			return errMsg{err}
		}
		return followResultMsg{followID: followID}
	}
}

func (a *App) unfollowUserCmd(followID string) tea.Cmd {
	return func() tea.Msg {
		if err := a.client.Unfollow(followID); err != nil {
			return errMsg{err}
		}
		return unfollowResultMsg{}
	}
}

func (a *App) loadUserPostsCmd(username, cursor string) tea.Cmd {
	return func() tea.Msg {
		posts, next, err := a.client.GetUserPosts(username, cursor)
		if err != nil {
			return errMsg{err}
		}
		if cursor == "" {
			return userPostsLoadedMsg{posts: posts, cursor: next}
		}
		return userPostsPageMsg{posts: posts, cursor: next}
	}
}

func (a *App) loadUserRepliesCmd(username, cursor string) tea.Cmd {
	return func() tea.Msg {
		replies, next, err := a.client.GetUserReplies(username, cursor)
		if err != nil {
			return errMsg{err}
		}
		if cursor == "" {
			return userRepliesLoadedMsg{replies: replies, cursor: next}
		}
		return userRepliesPageMsg{replies: replies, cursor: next}
	}
}

func (a *App) loadUserFollowingCmd(userID, cursor string) tea.Cmd {
	return func() tea.Msg {
		follows, next, err := a.client.GetUserFollows(userID, "following", cursor)
		if err != nil {
			return errMsg{err}
		}
		if cursor == "" {
			return userFollowingLoadedMsg{follows: follows, cursor: next}
		}
		return userFollowingPageMsg{follows: follows, cursor: next}
	}
}

func (a *App) loadUserFollowersCmd(userID, cursor string) tea.Cmd {
	return func() tea.Msg {
		follows, next, err := a.client.GetUserFollows(userID, "followers", cursor)
		if err != nil {
			return errMsg{err}
		}
		if cursor == "" {
			return userFollowersLoadedMsg{follows: follows, cursor: next}
		}
		return userFollowersPageMsg{follows: follows, cursor: next}
	}
}

func (a *App) sendRoomMessageCmd(roomID, body string) tea.Cmd {
	return func() tea.Msg {
		if err := a.client.SendRoomMessage(roomID, body); err != nil {
			return errMsg{err}
		}
		return nil
	}
}

func (a *App) sendCMailCmd(convID, body string) tea.Cmd {
	return func() tea.Msg {
		if err := a.client.SendMessage(convID, body); err != nil {
			return errMsg{err}
		}
		return nil
	}
}

func (a *App) loadRepliesCmd(postID string) tea.Cmd {
	return func() tea.Msg {
		replies, err := a.client.GetPostReplies(postID)
		if err != nil {
			return errMsg{err}
		}
		return repliesLoadedMsg{replies: replies}
	}
}

func (a *App) createReplyCmd(postID, content, parentReplyID string) tea.Cmd {
	return func() tea.Msg {
		_, err := a.client.CreateReply(postID, content, parentReplyID)
		if err != nil {
			return errMsg{err}
		}
		return replyCreatedMsg{postID: postID}
	}
}

func (a *App) createPostCmd(content string, topics []string) tea.Cmd {
	return func() tea.Msg {
		_, err := a.client.CreatePost(content, topics)
		if err != nil {
			return errMsg{err}
		}
		return postCreatedMsg{}
	}
}

func (a *App) saveProfileCmd(msg screens.SaveProfileMsg) tea.Cmd {
	return func() tea.Msg {
		update := model.ProfileUpdate{
			Bio:          &msg.Bio,
			WebsiteName:  &msg.WebsiteName,
			LocationName: &msg.LocationName,
		}
		// URL fields: send only when non-empty — the API rejects empty strings
		// as invalid URLs. Leaving them nil means the existing value is unchanged.
		if msg.WebsiteUrl != "" {
			update.WebsiteUrl = &msg.WebsiteUrl
		}
		if msg.WebsiteImageUrl != "" {
			update.WebsiteImageUrl = &msg.WebsiteImageUrl
		}
		if msg.Latitude != "" {
			if lat, err := strconv.ParseFloat(msg.Latitude, 64); err == nil {
				update.LocationLatitude = &lat
			}
		}
		if msg.Longitude != "" {
			if lon, err := strconv.ParseFloat(msg.Longitude, 64); err == nil {
				update.LocationLongitude = &lon
			}
		}
		if err := a.client.UpdateProfile(update); err != nil {
			return errMsg{err}
		}
		a.currentUser.Bio = msg.Bio
		a.currentUser.WebsiteName = msg.WebsiteName
		a.currentUser.WebsiteUrl = msg.WebsiteUrl
		a.currentUser.WebsiteImageUrl = msg.WebsiteImageUrl
		a.currentUser.LocationName = msg.LocationName
		if update.LocationLatitude != nil {
			a.currentUser.LocationLatitude = *update.LocationLatitude
		}
		if update.LocationLongitude != nil {
			a.currentUser.LocationLongitude = *update.LocationLongitude
		}
		return profileLoadedMsg{a.currentUser}
	}
}

// --- notifications ---

// handleNotifications processes notification load, mark-read, jump-to-post, and poll messages.
func (a App) handleNotifications(msg tea.Msg) (App, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case notifsLoadedMsg:
		a.notifications = a.notifications.SetNotifs(msg.notifs, msg.cursor)
		a.polledUnreadCount = a.notifications.UnreadCount()
		return a, nil, true
	case notifsPageMsg:
		a.notifications = a.notifications.AppendNotifs(msg.notifs, msg.cursor)
		a.polledUnreadCount = a.notifications.UnreadCount()
		return a, nil, true
	case screens.RefreshNotifsMsg:
		return a, a.loadNotifsCmd(), true
	case screens.LoadMoreNotifsMsg:
		return a, a.loadNotifsPageCmd(msg.Cursor), true
	case screens.MarkNotifReadMsg:
		// Optimistic update already applied in NotificationsModel.Update; fire API call.
		if a.polledUnreadCount > 0 {
			a.polledUnreadCount--
		}
		return a, a.markNotifReadCmd(msg.ID), true
	case screens.MarkAllNotifsReadMsg:
		// Optimistic update already applied in NotificationsModel.Update; fire API call.
		a.polledUnreadCount = 0
		return a, a.markAllNotifsReadCmd(), true
	case screens.ShowNotificationPostMsg:
		// Optimistic mark-read already applied in NotificationsModel.Update; confirm with API.
		if a.polledUnreadCount > 0 {
			a.polledUnreadCount--
		}
		a.pendingReplyID = msg.ReplyID
		return a, tea.Batch(a.markNotifReadCmd(msg.NotifID), a.loadPostAndShowCmd(msg.PostID)), true
	case notifPostLoadedMsg:
		a.postDetailReturn = screenNotifications
		a.active = screenPostDetail
		a.postDetail = a.postDetail.SetPost(msg.post)
		return a, a.loadRepliesCmd(msg.post.ID), true
	case screens.ShowUserProfileMsg:
		if a.active != screenNotifications {
			return a, nil, false
		}
		a.profileReturn = screenNotifications
		return a, a.loadUserProfileCmd(msg.Username), true
	case pollUnreadTickMsg:
		return a, tea.Batch(a.fetchUnreadCountCmd(), a.schedulePollCmd()), true
	case unreadCountMsg:
		a.polledUnreadCount = msg.count
		return a, nil, true
	}
	return a, nil, false
}

func (a *App) loadNotifsCmd() tea.Cmd {
	return func() tea.Msg {
		notifs, cursor, err := a.client.GetNotifications("")
		if err != nil {
			return errMsg{err}
		}
		return notifsLoadedMsg{notifs: notifs, cursor: cursor}
	}
}

func (a *App) loadNotifsPageCmd(cursor string) tea.Cmd {
	return func() tea.Msg {
		notifs, nextCursor, err := a.client.GetNotifications(cursor)
		if err != nil {
			return errMsg{err}
		}
		return notifsPageMsg{notifs: notifs, cursor: nextCursor}
	}
}

func (a *App) markNotifReadCmd(id string) tea.Cmd {
	return func() tea.Msg {
		_ = a.client.MarkNotificationRead(id) // fire-and-forget; UI already updated
		return nil
	}
}

func (a *App) markAllNotifsReadCmd() tea.Cmd {
	return func() tea.Msg {
		_ = a.client.MarkAllNotificationsRead() // fire-and-forget; UI already updated
		return nil
	}
}

func (a *App) loadBookmarkPostCmd(postID string) tea.Cmd {
	return func() tea.Msg {
		post, err := a.client.GetPost(postID)
		if err != nil {
			return errMsg{err}
		}
		return bookmarkPostLoadedMsg{post: post}
	}
}

func (a *App) loadBookmarkReplyCmd(replyID string) tea.Cmd {
	return func() tea.Msg {
		reply, err := a.client.GetReply(replyID)
		if err != nil {
			return errMsg{err}
		}
		post, err := a.client.GetPost(reply.PostID)
		if err != nil {
			return errMsg{err}
		}
		return bookmarkReplyLoadedMsg{post: post, replyID: replyID}
	}
}

// enrichBookmarks fetches embedded post/reply content for any bookmark that the
// list API returned without it. All fetches run in parallel; failures are silently
// skipped so the list still shows with whatever data is available.
func enrichBookmarks(client api.Client, items []model.Bookmark) []model.Bookmark {
	type result struct {
		idx   int
		post  *model.Post
		reply *model.Reply
	}
	ch := make(chan result, len(items))
	var wg sync.WaitGroup
	for i, b := range items {
		if b.Post != nil || b.Reply != nil {
			continue
		}
		wg.Add(1)
		i, b := i, b
		go func() {
			defer wg.Done()
			if b.PostID != "" {
				if p, err := client.GetPost(b.PostID); err == nil {
					ch <- result{idx: i, post: &p}
				}
			} else if b.ReplyID != "" {
				if r, err := client.GetReply(b.ReplyID); err == nil {
					ch <- result{idx: i, reply: &r}
				}
			}
		}()
	}
	wg.Wait()
	close(ch)
	out := make([]model.Bookmark, len(items))
	copy(out, items)
	for r := range ch {
		if r.post != nil {
			out[r.idx].Post = r.post
		}
		if r.reply != nil {
			out[r.idx].Reply = r.reply
		}
	}
	return out
}

func (a *App) loadBookmarksCmd(cursor string) tea.Cmd {
	return func() tea.Msg {
		items, nextCursor, err := a.client.GetBookmarks(cursor)
		if err != nil {
			return errMsg{err}
		}
		items = enrichBookmarks(a.client, items)
		return bookmarksLoadedMsg{items: items, cursor: nextCursor}
	}
}

func (a *App) loadBookmarksPageCmd(cursor string) tea.Cmd {
	return func() tea.Msg {
		items, nextCursor, err := a.client.GetBookmarks(cursor)
		if err != nil {
			return errMsg{err}
		}
		items = enrichBookmarks(a.client, items)
		return bookmarksPageMsg{items: items, cursor: nextCursor}
	}
}

func (a *App) createBookmarkCmd(postID, replyID string) tea.Cmd {
	return func() tea.Msg {
		id, err := a.client.CreateBookmark(postID, replyID)
		return bookmarkCreatedMsg{bookmarkID: id, postID: postID, replyID: replyID, err: err}
	}
}

func (a *App) deleteBookmarkCmd(id string) tea.Cmd {
	return func() tea.Msg {
		_ = a.client.DeleteBookmark(id) // fire-and-forget; UI already updated
		return bookmarkDeletedMsg{bookmarkID: id}
	}
}

// --- Topics commands ---

func (a *App) loadTopicsCmd() tea.Cmd {
	return func() tea.Msg {
		topics, cursor, err := a.client.GetTopics("")
		if err != nil {
			return errMsg{err}
		}
		return topicsLoadedMsg{topics: topics, cursor: cursor}
	}
}

func (a *App) loadMoreTopicsCmd(cursor string) tea.Cmd {
	return func() tea.Msg {
		topics, nextCursor, err := a.client.GetTopics(cursor)
		if err != nil {
			return errMsg{err}
		}
		return topicsPageMsg{topics: topics, cursor: nextCursor}
	}
}

func (a *App) loadTopicPostsCmd(slug string) tea.Cmd {
	return func() tea.Msg {
		posts, cursor, err := a.client.GetTopicPosts(slug, "")
		if err != nil {
			return errMsg{err}
		}
		return topicPostsLoadedMsg{posts: posts, cursor: cursor}
	}
}

func (a *App) loadTopicPostsPageCmd(slug, cursor string) tea.Cmd {
	return func() tea.Msg {
		posts, nextCursor, err := a.client.GetTopicPosts(slug, cursor)
		if err != nil {
			return errMsg{err}
		}
		return topicPostsPageMsg{posts: posts, cursor: nextCursor}
	}
}

// --- Journal (Notes) commands ---

func (a *App) loadJournalCmd() tea.Cmd {
	return func() tea.Msg {
		notes, cursor, err := a.client.GetNotes("")
		if err != nil {
			return errMsg{err}
		}
		return journalLoadedMsg{notes: notes, cursor: cursor}
	}
}

func (a *App) loadJournalPageCmd(cursor string) tea.Cmd {
	return func() tea.Msg {
		notes, nextCursor, err := a.client.GetNotes(cursor)
		if err != nil {
			return errMsg{err}
		}
		return journalPageMsg{notes: notes, cursor: nextCursor}
	}
}

// saveNoteCmd creates a new note (noteID == "") or updates an existing one.
func (a *App) saveNoteCmd(noteID, content string, topics []string) tea.Cmd {
	if noteID == "" {
		return func() tea.Msg {
			note, err := a.client.CreateNote(content, topics)
			if err != nil {
				return errMsg{err}
			}
			return noteCreatedMsg{note: note}
		}
	}
	id := noteID // capture for closure
	return func() tea.Msg {
		if err := a.client.UpdateNote(id, content, topics); err != nil {
			return errMsg{err}
		}
		return noteUpdatedMsg{noteID: id, content: content, topics: topics}
	}
}

func (a *App) deleteNoteCmd(noteID string) tea.Cmd {
	return func() tea.Msg {
		if err := a.client.DeleteNote(noteID); err != nil {
			return errMsg{err}
		}
		return noteDeletedMsg{noteID: noteID}
	}
}

func (a *App) deletePostCmd(postID string, fromFeed bool) tea.Cmd {
	return func() tea.Msg {
		if err := a.client.DeletePost(postID); err != nil {
			return errMsg{err}
		}
		return postDeletedMsg{postID: postID, fromFeed: fromFeed}
	}
}

func (a *App) deleteReplyCmd(replyID string) tea.Cmd {
	return func() tea.Msg {
		if err := a.client.DeleteReply(replyID); err != nil {
			return errMsg{err}
		}
		return replyDeletedMsg{replyID: replyID}
	}
}

// publishNoteCmd creates a post from the note's content and topics.
func (a *App) publishNoteCmd(content string, topics []string) tea.Cmd {
	return func() tea.Msg {
		_, err := a.client.CreatePost(content, topics)
		if err != nil {
			return errMsg{err}
		}
		return notePublishedMsg{}
	}
}

func (a *App) loadNoteRevisionsCmd(noteID, cursor string) tea.Cmd {
	return func() tea.Msg {
		revisions, next, err := a.client.GetNoteRevisions(noteID, cursor)
		if err != nil {
			return errMsg{err}
		}
		return noteRevisionsLoadedMsg{noteID: noteID, revisions: revisions, cursor: next}
	}
}

func (a *App) loadNoteRevisionCmd(noteID string, revision int) tea.Cmd {
	return func() tea.Msg {
		note, err := a.client.GetNoteRevision(noteID, revision)
		if err != nil {
			return errMsg{err}
		}
		return noteRevisionPreviewMsg{note: note}
	}
}

func (a *App) schedulePollCmd() tea.Cmd {
	return tea.Tick(60*time.Second, func(time.Time) tea.Msg { return pollUnreadTickMsg{} })
}

func (a *App) scheduleWanderCmd() tea.Cmd {
	return tea.Tick(1*time.Hour, func(time.Time) tea.Msg { return wanderTickMsg{} })
}

// checkAndWanderCmd fires a profile location update if wander mode is enabled
// and at least 12 hours have elapsed since the last update. All failures are
// silent — the user is never notified.
func (a *App) checkAndWanderCmd() tea.Cmd {
	return func() tea.Msg {
		cfg, err := config.Load()
		if err != nil {
			return wanderDoneMsg{}
		}
		if !config.ShouldWanderNow(cfg) {
			return wanderDoneMsg{}
		}
		lat := math.Round((rand.Float64()*180-90)*1e4) / 1e4
		lon := math.Round((rand.Float64()*360-180)*1e4) / 1e4
		name := "Wandering the world..."
		update := model.ProfileUpdate{
			LocationLatitude:  &lat,
			LocationLongitude: &lon,
			LocationName:      &name,
		}
		if err := a.client.UpdateProfile(update); err != nil {
			return wanderDoneMsg{}
		}
		return wanderDoneMsg{at: time.Now().UTC()}
	}
}

func (a *App) fetchUnreadCountCmd() tea.Cmd {
	return func() tea.Msg {
		notifs, _, err := a.client.GetNotifications("")
		if err != nil {
			return nil
		}
		count := 0
		for _, n := range notifs {
			if !n.Read {
				count++
			}
		}
		return unreadCountMsg{count}
	}
}

func (a *App) loadPostAndShowCmd(postID string) tea.Cmd {
	return func() tea.Msg {
		post, err := a.client.GetPost(postID)
		if err != nil {
			return errMsg{err}
		}
		return notifPostLoadedMsg{post: post}
	}
}

// loadProfilePostCmd fetches a post for display when navigating from a profile Replies tab.
func (a *App) loadProfilePostCmd(postID string) tea.Cmd {
	return func() tea.Msg {
		post, err := a.client.GetPost(postID)
		if err != nil {
			return errMsg{err}
		}
		return profilePostLoadedMsg{post: post}
	}
}
