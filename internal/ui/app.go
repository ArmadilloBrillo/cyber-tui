package ui

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	neturl "net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ragnar/cyber-tui/internal/api"
	"github.com/ragnar/cyber-tui/internal/config"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/imgview"
	"github.com/ragnar/cyber-tui/internal/ui/screens"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
	"github.com/ragnar/cyber-tui/internal/ui/urlutil"
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
	screenGuilds
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

const (
	logoOrig          = "ᑕ¥βєяรקค¢є"
	logoHoldFrames    = 5 // frames held fully scrambled (~300ms at 60ms/frame)
	logoFrameInterval = 60 * time.Millisecond
)

var logoOrigRunes = []rune(logoOrig)

var logoCyberPool = []rune{
	'¥', '¢', '€', '£', '₿', '₽', '¤', '§', '©', '®',
	'α', 'β', 'γ', 'δ', 'ε', 'λ', 'μ', 'π', 'σ', 'φ', 'ψ', 'ω',
	'я', 'ю', 'э', 'ш', 'щ', 'ж', 'ф', 'г', 'ц', 'б',
	'א', 'ב', 'ג', 'ד', 'ה', 'כ', 'ל', 'מ', 'נ', 'ק', 'ש',
	'æ', 'ø', 'þ', 'ß', 'ñ', 'ç',
	'ᑕ', 'ᑎ', 'ᒋ', 'ᓯ', 'ᑲ', 'ᒪ',
	'∞', '∑', '√', '≈', '≠', '⊗', '⊕',
	'░', '▒', '▓',
}

func randomCyberRune(exclude rune) rune {
	for len(logoCyberPool) > 1 {
		r := logoCyberPool[rand.Intn(len(logoCyberPool))]
		if r != exclude {
			return r
		}
	}
	return logoCyberPool[0]
}

type App struct {
	layout      Layout
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
	guilds         screens.GuildsModel
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

	// wanderLust is the local config value for wander mode. Defaults to false (off).
	wanderLust bool
	// maxThreadDepth is the local config value for reply nesting depth. Defaults to 3.
	maxThreadDepth int

	// graphicsProtocol is the terminal image display protocol detected at startup.
	// ProtocolNone means no image display is available and URLs open in a browser.
	graphicsProtocol imgview.GraphicsProtocol

	// imageViewer is the user's preference from config.ImageViewer. When "browser",
	// image URLs always open in the OS browser even if a protocol is detected.
	imageViewer string

	// imageModal holds the state for the inline image overlay. When imageModalOpen
	// is true, View composites the encoded image sequence over the base content.
	imageModalOpen     bool
	imageModalEncoded  string
	imageModalCols     int
	imageModalRows     int
	imageNeedsCleanup  bool // true for one frame after modal closes, to delete Kitty placement

	// ephemeral marks an SSH-hosted session whose state must never be read from
	// or written to the host operator's config file.
	ephemeral bool

	// bookmarkedPostIDs and bookmarkedReplyIDs track which posts/replies the current
	// user has bookmarked, populated from the bookmarks list and kept in sync on
	// create/delete. Used to show [★] indicators in feed, postdetail, and topics.
	bookmarkedPostIDs  map[string]struct{}
	bookmarkedReplyIDs map[string]struct{}
	// postBookmarkIDs and replyBookmarkIDs are reverse lookups: content ID → bookmark UUID.
	// Required to call deleteBookmarkCmd when the user toggles off a bookmark with 'b'.
	postBookmarkIDs  map[string]string // postID  → bookmark UUID
	replyBookmarkIDs map[string]string // replyID → bookmark UUID

	// watchedPostIDs tracks which thread-root posts the current user is watching.
	// Populated progressively at login via GET /v1/watches (all pages) and kept in
	// sync on watch/unwatch. Used to show [◉] indicators in feed and post detail.
	watchedPostIDs map[string]struct{}

	// notifyText is the transient global notification shown in place of the status
	// bar. Empty means no notification is visible. notifyGen is bumped on every new
	// notification and on dismissal so a stale expire tick can never clear a newer one.
	notifyText  string
	notifyLevel notifyLevel
	notifyGen   int

	logoText      string
	logoPhase     logoAnimPhase
	logoFrame     int
	logoPositions []int // shuffled index order for the current animation cycle
}

func NewApp(client api.Client) App {
	return App{
		layout:             TabsLayout{},
		client:             client,
		active:             screenLogin,
		focus:              focusMenu,
		loc:                time.UTC,
		wanderLust:         false,
		login:              screens.NewLoginModel(""),
		feed:               screens.NewFeedModel(),
		chatrooms:          screens.NewChatroomsModel(),
		cmail:              screens.NewCMailModel("", client),
		profile:            screens.NewProfileModel(),
		postDetail:         screens.NewPostDetailModel(),
		notifications:      screens.NewNotificationsModel(),
		settingsScreen:     screens.NewSettingsModel(),
		bookmarks:          screens.NewBookmarksModel(),
		guilds:             screens.NewGuildsModel(),
		topics:             screens.NewTopicsModel(),
		journal:            screens.NewJournalModel(0),
		bookmarkedPostIDs:  make(map[string]struct{}),
		bookmarkedReplyIDs: make(map[string]struct{}),
		postBookmarkIDs:    make(map[string]string),
		replyBookmarkIDs:   make(map[string]string),
		watchedPostIDs:     make(map[string]struct{}),
		logoText:           logoOrig,
		logoPhase:          logoPhaseIdle,
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
	a.imageViewer = s.ImageViewer
	return a
}

// WithGraphicsProtocol sets the terminal graphics protocol detected at startup.
// When proto is ProtocolNone the image viewer feature is disabled entirely.
func (a App) WithGraphicsProtocol(proto imgview.GraphicsProtocol) App {
	a.graphicsProtocol = proto
	return a
}

// WithEphemeralSession marks the App as a remote SSH-hosted session. Such a
// session must not persist or read session credentials and display preferences
// from the host operator's config file.
func (a App) WithEphemeralSession() App {
	a.ephemeral = true
	return a
}

// saveConfig loads the persisted config, applies mutate, and writes it back. It
// is a no-op for ephemeral (SSH-hosted) sessions.
func (a *App) saveConfig(mutate func(cfg *config.Config)) {
	if a.ephemeral {
		return
	}
	cfg, err := config.Load()
	if err != nil {
		return
	}
	mutate(&cfg)
	_ = config.Save(cfg)
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
	// imageNeedsCleanup is a one-render-cycle flag: it is set true by the
	// keypress that closes the modal and cleared here at the very start of the
	// next Update call. This guarantees the cleanup frame is rendered before
	// the flag is cleared, with no goroutine race.
	a.imageNeedsCleanup = false

	if m, ok := msg.(tea.WindowSizeMsg); ok {
		a = a.applyWindowSize(m)
		return a, a.delegateUpdate(msg)
	}
	// Any keypress dismisses a visible notification early. We do NOT return here,
	// so the key still flows on to do its normal job; bumping notifyGen neutralizes
	// the pending expire tick.
	if _, ok := msg.(tea.KeyMsg); ok && a.notifyText != "" {
		a.notifyText = ""
		a.notifyGen++
	}
	// Any keypress closes the image modal. Consume the key so it doesn't
	// accidentally trigger another action while the modal is visible.
	if _, ok := msg.(tea.KeyMsg); ok && a.imageModalOpen {
		a.imageModalOpen = false
		a.imageNeedsCleanup = (a.graphicsProtocol == imgview.ProtocolKitty)
		return a, nil
	}
	if a2, cmd, ok := a.handleKeys(msg); ok {
		return a2, cmd
	}
	if a2, cmd, ok := a.handleAuth(msg); ok {
		return a2, cmd
	}
	if a2, cmd, ok := a.handleFeed(msg); ok {
		return a2, cmd
	}
	if a2, cmd, ok := a.handlePostDetail(msg); ok {
		return a2, cmd
	}
	if a2, cmd, ok := a.handleChatrooms(msg); ok {
		return a2, cmd
	}
	if a2, cmd, ok := a.handleCMail(msg); ok {
		return a2, cmd
	}
	if a2, cmd, ok := a.handleProfile(msg); ok {
		return a2, cmd
	}
	if a2, cmd, ok := a.handleNotifications(msg); ok {
		return a2, cmd
	}
	if a2, cmd, ok := a.handleSettings(msg); ok {
		return a2, cmd
	}
	if a2, cmd, ok := a.handleBookmarks(msg); ok {
		return a2, cmd
	}
	if a2, cmd, ok := a.handleWatches(msg); ok {
		return a2, cmd
	}
	if a2, cmd, ok := a.handleGuilds(msg); ok {
		return a2, cmd
	}
	if a2, cmd, ok := a.handleTopics(msg); ok {
		return a2, cmd
	}
	if a2, cmd, ok := a.handleJournal(msg); ok {
		return a2, cmd
	}
	if a2, cmd, ok := a.handleUnauthorized(msg); ok {
		return a2, cmd
	}
	if a2, cmd, ok := a.handleLogoAnim(msg); ok {
		return a2, cmd
	}
	if a2, cmd, ok := a.handleNotify(msg); ok {
		return a2, cmd
	}
	if a2, cmd, ok := a.handleImageViewer(msg); ok {
		return a2, cmd
	}
	if a2, cmd, ok := a.handleErr(msg); ok {
		return a2, cmd
	}
	return a, a.delegateUpdate(msg)
}

// updateAll sends msg to every screen, discarding returned commands.
// Adding a new screen: add one line here. All broadcast helpers call this.
func (a App) updateAll(msg tea.Msg) App {
	a.feed, _ = a.feed.Update(msg)
	a.chatrooms, _ = a.chatrooms.Update(msg)
	a.cmail, _ = a.cmail.Update(msg)
	a.postDetail, _ = a.postDetail.Update(msg)
	a.profile, _ = a.profile.Update(msg)
	a.notifications, _ = a.notifications.Update(msg)
	a.settingsScreen, _ = a.settingsScreen.Update(msg)
	a.bookmarks, _ = a.bookmarks.Update(msg)
	a.guilds, _ = a.guilds.Update(msg)
	a.topics, _ = a.topics.Update(msg)
	a.journal, _ = a.journal.Update(msg)
	return a
}

// broadcastConfig pushes the current display settings to all screens.
// Call this whenever loc, relaxed, or dimensions change outside of a
// WindowSizeMsg (e.g. after login, timezone change, or density toggle).
func (a *App) broadcastConfig() {
	msg := screens.SharedConfigMsg{Width: a.width, Height: a.height, Loc: a.loc, Relaxed: a.relaxed, Settings: a.settings, WanderLust: a.wanderLust, MaxThreadDepth: a.maxThreadDepth, Timezone: a.timezone, ImageViewer: a.imageViewer, OwnGuildSlug: a.currentUser.GuildSlug}
	*a = a.updateAll(msg)
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
	a.guilds, _ = a.guilds.Update(msg)
	a.topics, _ = a.topics.Update(msg)
}

// broadcastWatchedIDs pushes the current watched-post ID set to all screens
// that render posts (feed, postDetail, guilds, topics). Call this whenever
// the set changes (progressive load page, watch, unwatch).
func (a *App) broadcastWatchedIDs() {
	msg := screens.WatchedPostIDsMsg{PostIDs: a.watchedPostIDs}
	a.feed, _ = a.feed.Update(msg)
	a.postDetail, _ = a.postDetail.Update(msg)
	a.guilds, _ = a.guilds.Update(msg)
	a.topics, _ = a.topics.Update(msg)
}

// applyWindowSize stores the new terminal dimensions and broadcasts the size
// to all screens so their viewports initialise before they become active.
// The active screen gets a second update via delegateUpdate, which is harmless.
func (a App) applyWindowSize(m tea.WindowSizeMsg) App {
	a.width = m.Width
	a.height = m.Height
	return a.updateAll(m)
}

// handleKeys processes tea.KeyMsg events: modal intercepts, focused-input
// bypass, and all global keyboard shortcuts.
func (a App) handleKeys(msg tea.Msg) (App, tea.Cmd, bool) {
	m, ok := msg.(tea.KeyMsg)
	if !ok {
		return a, nil, false
	}
	// Modal overlays intercept all keys while open.
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
	case "v":
		if a.active != screenLogin {
			a.relaxed = !a.relaxed
			a.broadcastConfig()
			relaxed := a.relaxed
			return a, func() tea.Msg {
				a.saveConfig(func(cfg *config.Config) {
					if relaxed {
						cfg.Density = "relaxed"
					} else {
						cfg.Density = ""
					}
				})
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
	}
	return a.layout.HandleNav(m, a)
}

// handleAuth processes login/registration flow messages.
func (a App) handleAuth(msg tea.Msg) (App, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case screens.SubmitLoginMsg:
		return a, a.loginCmd(msg.Email, msg.Password), true
	case loginSuccessMsg:
		a.tokens = msg.tokens
		a.currentUser = msg.user
		a.cmail = screens.NewCMailModel(msg.user.Username, a.client)
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
		a.postDetailReturn = a.active
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
		return a, a.createPostCmd(msg.Content, msg.Title, msg.Topics, msg.IsPublic, msg.IsNSFW), true
	case postCreatedMsg:
		return a, a.loadFeedCmd(), true
	case screens.SubmitReplyMsg:
		return a, a.createReplyCmd(msg.PostID, msg.Content, msg.ParentReplyID), true
	case replyCreatedMsg:
		if a.settings.AutoWatchOnReply {
			if _, alreadyWatched := a.watchedPostIDs[msg.postID]; !alreadyWatched {
				newIDs := make(map[string]struct{}, len(a.watchedPostIDs)+1)
				for k := range a.watchedPostIDs {
					newIDs[k] = struct{}{}
				}
				newIDs[msg.postID] = struct{}{}
				a.watchedPostIDs = newIDs
				a.broadcastWatchedIDs()
			}
		}
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
		tz := msg.Timezone
		iv := msg.ImageViewer
		return a, func() tea.Msg {
			if err := a.client.UpdateSettings(s); err != nil {
				return actionErrMsg{err}
			}
			a.saveConfig(func(cfg *config.Config) {
				cfg.WanderLust = wl
				cfg.MaxThreadDepth = td
				cfg.Timezone = tz
				cfg.ImageViewer = iv
			})
			return settingsSavedMsg{settings: s, wanderLust: wl, maxThreadDepth: td, timezone: tz, imageViewer: iv}
		}, true

	case settingsSavedMsg:
		a.settings = msg.settings
		a.wanderLust = msg.wanderLust
		a.maxThreadDepth = msg.maxThreadDepth
		a.timezone = msg.timezone
		a.imageViewer = msg.imageViewer
		a.loc = config.ParseTimezoneLabel(msg.timezone)
		a.settingsScreen = a.settingsScreen.SetSaved(msg.wanderLust, msg.maxThreadDepth, msg.timezone, msg.imageViewer)
		a.broadcastConfig()
		a.refreshViewports()
		return a, nil, true

	case wanderTickMsg:
		return a, tea.Batch(a.checkAndWanderCmd(), a.scheduleWanderCmd()), true

	case wanderDoneMsg:
		if !msg.at.IsZero() {
			a.saveConfig(func(cfg *config.Config) {
				cfg.LastWandered = msg.at
			})
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
		a.bookmarkedPostIDs, a.bookmarkedReplyIDs, a.postBookmarkIDs, a.replyBookmarkIDs = bookmarkIDSets(msg.items)
		a.broadcastBookmarkedIDs()
		return a, nil, true
	case bookmarksPageMsg:
		a.bookmarks = a.bookmarks.AppendBookmarks(msg.items, msg.cursor)
		a.bookmarkedPostIDs, a.bookmarkedReplyIDs, a.postBookmarkIDs, a.replyBookmarkIDs = mergeBookmarkIDSets(
			a.bookmarkedPostIDs, a.bookmarkedReplyIDs, a.postBookmarkIDs, a.replyBookmarkIDs, msg.items)
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
		if msg.ReplyID != "" {
			replyID := msg.ReplyID
			if _, alreadyBookmarked := a.bookmarkedReplyIDs[replyID]; alreadyBookmarked {
				// Toggle off: optimistic remove.
				bookmarkID := a.replyBookmarkIDs[replyID]
				newReplyIDs := make(map[string]struct{}, len(a.bookmarkedReplyIDs))
				for k := range a.bookmarkedReplyIDs {
					if k != replyID {
						newReplyIDs[k] = struct{}{}
					}
				}
				a.bookmarkedReplyIDs = newReplyIDs
				delete(a.replyBookmarkIDs, replyID)
				a.broadcastBookmarkedIDs()
				return a, a.deleteBookmarkCmd(bookmarkID, false), true
			}
			// Toggle on: optimistic add.
			newReplyIDs := make(map[string]struct{}, len(a.bookmarkedReplyIDs)+1)
			for k := range a.bookmarkedReplyIDs {
				newReplyIDs[k] = struct{}{}
			}
			newReplyIDs[replyID] = struct{}{}
			a.bookmarkedReplyIDs = newReplyIDs
			a.broadcastBookmarkedIDs()
			return a, a.createBookmarkCmd("", replyID), true
		}
		postID := msg.PostID
		if _, alreadyBookmarked := a.bookmarkedPostIDs[postID]; alreadyBookmarked {
			// Toggle off: optimistic remove.
			bookmarkID := a.postBookmarkIDs[postID]
			newPostIDs := make(map[string]struct{}, len(a.bookmarkedPostIDs))
			for k := range a.bookmarkedPostIDs {
				if k != postID {
					newPostIDs[k] = struct{}{}
				}
			}
			a.bookmarkedPostIDs = newPostIDs
			delete(a.postBookmarkIDs, postID)
			a.broadcastBookmarkedIDs()
			return a, a.deleteBookmarkCmd(bookmarkID, false), true
		}
		// Toggle on: optimistic add.
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
			// Roll back the optimistic add.
			if msg.replyID != "" {
				newReplyIDs := make(map[string]struct{}, len(a.bookmarkedReplyIDs))
				for k := range a.bookmarkedReplyIDs {
					if k != msg.replyID {
						newReplyIDs[k] = struct{}{}
					}
				}
				a.bookmarkedReplyIDs = newReplyIDs
			} else {
				newPostIDs := make(map[string]struct{}, len(a.bookmarkedPostIDs))
				for k := range a.bookmarkedPostIDs {
					if k != msg.postID {
						newPostIDs[k] = struct{}{}
					}
				}
				a.bookmarkedPostIDs = newPostIDs
			}
			a.broadcastBookmarkedIDs()
			a, cmd := a.notify(notifyError, msg.err.Error())
			return a, cmd, true
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
			delete(a.postBookmarkIDs, msg.PostID)
		}
		if msg.ReplyID != "" {
			newReplyIDs := make(map[string]struct{}, len(a.bookmarkedReplyIDs))
			for k := range a.bookmarkedReplyIDs {
				if k != msg.ReplyID {
					newReplyIDs[k] = struct{}{}
				}
			}
			a.bookmarkedReplyIDs = newReplyIDs
			delete(a.replyBookmarkIDs, msg.ReplyID)
		}
		a.broadcastBookmarkedIDs()
		return a, a.deleteBookmarkCmd(msg.BookmarkID, true), true
	case bookmarkDeletedMsg:
		if !msg.fromBookmarksScreen {
			a.bookmarks = a.bookmarks.SetFetching()
			return a, a.loadBookmarksCmd(""), true
		}
		return a, nil, true
	}
	return a, nil, false
}

// --- Watch messages ---

type watchPageMsg struct {
	postIDs []string
	cursor  string
	err     error
}

type watchResultMsg struct {
	postID string
	err    error
	added  bool // true = watch was added, false = watch was removed
}

// handleWatches processes progressive watch-page loads and watch/unwatch toggle messages.
func (a App) handleWatches(msg tea.Msg) (App, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case watchPageMsg:
		if msg.err != nil {
			// Watches are non-critical; silently ignore load errors.
			return a, nil, true
		}
		newIDs := make(map[string]struct{}, len(a.watchedPostIDs)+len(msg.postIDs))
		for k := range a.watchedPostIDs {
			newIDs[k] = struct{}{}
		}
		for _, id := range msg.postIDs {
			newIDs[id] = struct{}{}
		}
		a.watchedPostIDs = newIDs
		a.broadcastWatchedIDs()
		if msg.cursor != "" {
			return a, a.loadWatchesPageCmd(msg.cursor), true
		}
		return a, nil, true

	case screens.ToggleWatchPostMsg:
		postID := msg.PostID
		if _, alreadyWatched := a.watchedPostIDs[postID]; alreadyWatched {
			// Toggle off: optimistic remove.
			newIDs := make(map[string]struct{}, len(a.watchedPostIDs))
			for k := range a.watchedPostIDs {
				newIDs[k] = struct{}{}
			}
			delete(newIDs, postID)
			a.watchedPostIDs = newIDs
			a.broadcastWatchedIDs()
			return a, a.unwatchPostCmd(postID), true
		}
		// Toggle on: optimistic add.
		newIDs := make(map[string]struct{}, len(a.watchedPostIDs)+1)
		for k := range a.watchedPostIDs {
			newIDs[k] = struct{}{}
		}
		newIDs[postID] = struct{}{}
		a.watchedPostIDs = newIDs
		a.broadcastWatchedIDs()
		return a, a.watchPostCmd(postID), true

	case watchResultMsg:
		if msg.err != nil {
			// Revert the optimistic update.
			newIDs := make(map[string]struct{}, len(a.watchedPostIDs))
			for k := range a.watchedPostIDs {
				newIDs[k] = struct{}{}
			}
			if msg.added {
				delete(newIDs, msg.postID)
			} else {
				newIDs[msg.postID] = struct{}{}
			}
			a.watchedPostIDs = newIDs
			a.broadcastWatchedIDs()
			a2, cmd := a.notify(notifyError, msg.err.Error())
			return a2, cmd, true
		}
		return a, nil, true
	}
	return a, nil, false
}

// bookmarkIDSets builds post/reply ID sets and reverse lookup maps from a fresh bookmark page.
func bookmarkIDSets(items []model.Bookmark) (map[string]struct{}, map[string]struct{}, map[string]string, map[string]string) {
	postIDs := make(map[string]struct{})
	replyIDs := make(map[string]struct{})
	postBookmarks := make(map[string]string)
	replyBookmarks := make(map[string]string)
	for _, b := range items {
		if b.PostID != "" {
			postIDs[b.PostID] = struct{}{}
			postBookmarks[b.PostID] = b.ID
		}
		if b.ReplyID != "" {
			replyIDs[b.ReplyID] = struct{}{}
			replyBookmarks[b.ReplyID] = b.ID
		}
	}
	return postIDs, replyIDs, postBookmarks, replyBookmarks
}

// mergeBookmarkIDSets merges a new page of bookmarks into existing ID sets and reverse maps.
func mergeBookmarkIDSets(postIDs, replyIDs map[string]struct{}, postBookmarks, replyBookmarks map[string]string, items []model.Bookmark) (map[string]struct{}, map[string]struct{}, map[string]string, map[string]string) {
	newPostIDs := make(map[string]struct{}, len(postIDs)+len(items))
	for k := range postIDs {
		newPostIDs[k] = struct{}{}
	}
	newReplyIDs := make(map[string]struct{}, len(replyIDs)+len(items))
	for k := range replyIDs {
		newReplyIDs[k] = struct{}{}
	}
	newPostBookmarks := make(map[string]string, len(postBookmarks)+len(items))
	for k, v := range postBookmarks {
		newPostBookmarks[k] = v
	}
	newReplyBookmarks := make(map[string]string, len(replyBookmarks)+len(items))
	for k, v := range replyBookmarks {
		newReplyBookmarks[k] = v
	}
	for _, b := range items {
		if b.PostID != "" {
			newPostIDs[b.PostID] = struct{}{}
			newPostBookmarks[b.PostID] = b.ID
		}
		if b.ReplyID != "" {
			newReplyIDs[b.ReplyID] = struct{}{}
			newReplyBookmarks[b.ReplyID] = b.ID
		}
	}
	return newPostIDs, newReplyIDs, newPostBookmarks, newReplyBookmarks
}

// handleGuilds processes guild list, guild posts, pagination, and post selection messages.
func (a App) handleGuilds(msg tea.Msg) (App, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case screens.RefreshGuildsMsg:
		return a, a.loadGuildsCmd(""), true

	case guildsLoadedMsg:
		a.guilds = a.guilds.SetGuilds(msg.guilds, msg.cursor)
		return a, nil, true

	case screens.LoadMoreGuildsMsg:
		return a, a.loadMoreGuildsCmd(msg.Cursor), true

	case guildsPageMsg:
		a.guilds = a.guilds.AppendGuilds(msg.guilds, msg.cursor)
		return a, nil, true

	case screens.LoadGuildPostsMsg:
		return a, tea.Batch(a.loadGuildPostsCmd(msg.Slug), a.loadGuildDetailCmd(msg.Slug)), true

	case guildPostsLoadedMsg:
		if msg.slug != a.guilds.ActiveGuild() {
			return a, nil, true
		}
		a.guilds = a.guilds.SetGuildPosts(msg.posts, msg.cursor)
		return a, nil, true

	case screens.LoadMoreGuildPostsMsg:
		return a, a.loadGuildPostsPageCmd(msg.Slug, msg.Cursor), true

	case guildPostsPageMsg:
		if msg.slug != a.guilds.ActiveGuild() {
			return a, nil, true
		}
		a.guilds = a.guilds.AppendGuildPosts(msg.posts, msg.cursor)
		return a, nil, true

	case screens.RefreshGuildPostsMsg:
		return a, a.loadGuildPostsCmd(msg.Slug), true

	case screens.ShowGuildPostMsg:
		a.postDetailReturn = screenGuilds
		a.active = screenPostDetail
		a.postDetail = a.postDetail.SetPost(msg.Post)
		return a, a.loadRepliesCmd(msg.Post.ID), true

	case screens.SubmitGuildPostMsg:
		return a, a.createGuildPostCmd(msg.Slug, msg.Content, msg.Title, msg.Topics), true

	case guildPostCreatedMsg:
		return a, a.loadGuildPostsCmd(msg.slug), true

	case screens.ShowUserProfileMsg:
		a.profileReturn = screenGuilds
		return a, a.loadUserProfileCmd(msg.Username), true

	case screens.LoadGuildMembersMsg:
		return a, a.loadGuildMembersCmd(msg.Slug, ""), true

	case guildMembersLoadedMsg:
		a.guilds = a.guilds.SetGuildMembers(msg.members, msg.cursor)
		return a, nil, true

	case screens.LoadMoreGuildMembersMsg:
		return a, a.loadGuildMembersCmd(msg.Slug, msg.Cursor), true

	case guildMembersPageMsg:
		a.guilds = a.guilds.AppendGuildMembers(msg.members, msg.cursor)
		return a, nil, true

	case guildDetailLoadedMsg:
		a.guilds = a.guilds.SetGuildDetail(msg.guild)
		return a, nil, true

	case screens.JoinGuildMsg:
		return a, a.joinGuildCmd(msg.Slug, a.guilds.GuildDetail().Name), true

	case screens.LeaveGuildMsg:
		return a, a.leaveGuildCmd(msg.Slug, a.guilds.GuildDetail().Name), true

	case guildJoinedMsg:
		detail := a.guilds.GuildDetail()
		detail.IsMember = true
		detail.Role = "member"
		a.guilds = a.guilds.SetGuildDetail(detail)
		a.currentUser.GuildSlug = msg.slug
		a.guilds = a.guilds.SetOwnGuildSlug(msg.slug)
		var notifyCmd tea.Cmd
		a, notifyCmd = a.notify(notifyInfo, "✓ Joined #"+msg.name)
		return a, tea.Batch(notifyCmd, a.loadGuildsCmd("")), true

	case guildLeftMsg:
		a.guilds = a.guilds.BackToGuildList()
		a.currentUser.GuildSlug = ""
		a.guilds = a.guilds.SetOwnGuildSlug("")
		var notifyCmd tea.Cmd
		a, notifyCmd = a.notify(notifyInfo, "✓ Left #"+msg.name)
		return a, tea.Batch(notifyCmd, a.loadGuildsCmd("")), true
	}
	return a, nil, false
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
// notifyTTL is how long a global notification banner stays before auto-dismissing.
const notifyTTL = 4 * time.Second

// notify sets the global banner and returns the timed-expire command. Each call
// bumps notifyGen and captures it in the tick closure, so only the newest
// notification's expire can clear the banner.
func (a App) notify(level notifyLevel, text string) (App, tea.Cmd) {
	a.notifyGen++
	a.notifyText = text
	a.notifyLevel = level
	gen := a.notifyGen
	return a, tea.Tick(notifyTTL, func(time.Time) tea.Msg {
		return notifyExpireMsg{gen: gen}
	})
}

func (a App) handleLogoAnim(msg tea.Msg) (App, tea.Cmd, bool) {
	switch msg.(type) {
	case logoAnimTickMsg:
		positions := make([]int, len(logoOrigRunes))
		for i := range positions {
			positions[i] = i
		}
		rand.Shuffle(len(positions), func(i, j int) { positions[i], positions[j] = positions[j], positions[i] })
		a.logoPositions = positions
		a.logoPhase = logoPhaseScrambling
		a.logoFrame = 0
		return a, logoFrameTickCmd(), true
	case logoFrameTickMsg:
		switch a.logoPhase {
		case logoPhaseScrambling:
			pos := a.logoPositions[a.logoFrame]
			runes := []rune(a.logoText)
			runes[pos] = randomCyberRune(logoOrigRunes[pos])
			a.logoText = string(runes)
			a.logoFrame++
			if a.logoFrame >= len(logoOrigRunes) {
				a.logoPhase = logoPhaseHold
				a.logoFrame = 0
			}
			return a, logoFrameTickCmd(), true
		case logoPhaseHold:
			a.logoFrame++
			if a.logoFrame >= logoHoldFrames {
				a.logoPhase = logoPhaseUnscrambling
				a.logoFrame = 0
			}
			return a, logoFrameTickCmd(), true
		case logoPhaseUnscrambling:
			pos := a.logoPositions[a.logoFrame]
			runes := []rune(a.logoText)
			runes[pos] = logoOrigRunes[pos]
			a.logoText = string(runes)
			a.logoFrame++
			if a.logoFrame >= len(logoOrigRunes) {
				a.logoPhase = logoPhaseIdle
				a.logoFrame = 0
				a.logoText = logoOrig
				return a, scheduleLogoAnimCmd(), true
			}
			return a, logoFrameTickCmd(), true
		default: // logoPhaseIdle — consume stale in-flight tick
			return a, nil, true
		}
	}
	return a, nil, false
}

func (a App) handleNotify(msg tea.Msg) (App, tea.Cmd, bool) {
	switch m := msg.(type) {
	case actionErrMsg:
		a, cmd := a.notify(notifyError, m.err.Error())
		return a, cmd, true
	case notifyMsg:
		a, cmd := a.notify(m.level, m.text)
		return a, cmd, true
	case notifyExpireMsg:
		if m.gen == a.notifyGen {
			a.notifyText = ""
		}
		return a, nil, true
	}
	return a, nil, false
}

// handleUnauthorized intercepts an errMsg or actionErrMsg carrying the
// ErrUnauthorized sentinel — returned by the API client after a token refresh
// fails — and routes the user back to the login screen instead of leaving them
// stranded on an errored screen. The dead refresh token is cleared so the next
// launch starts at the login form rather than retrying a doomed auto-login.
func (a App) handleUnauthorized(msg tea.Msg) (App, tea.Cmd, bool) {
	var err error
	switch m := msg.(type) {
	case errMsg:
		err = m.err
	case actionErrMsg:
		err = m.err
	case notifPostLoadErrMsg:
		err = m.err
	default:
		return a, nil, false
	}
	if !errors.Is(err, api.ErrUnauthorized) || a.active == screenLogin {
		return a, nil, false
	}

	_ = a.client.Logout()
	a.tokens = model.Tokens{}
	a.saveConfig(func(cfg *config.Config) { cfg.RefreshToken = "" })

	a.active = screenLogin
	a.focus = focusMenu
	a.login = screens.NewLoginModel(a.currentUser.Email)

	a, cmd := a.notify(notifyWarn, "session expired — please log in again")
	return a, cmd, true
}

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
	case screenGuilds:
		a.guilds = a.guilds.SetError(m.err)
	case screenTopics:
		a.topics = a.topics.SetError(m.err)
	case screenJournal:
		a.journal = a.journal.SetError(m.err)
	}
	// Errors never block a screen: the per-screen SetError above only feeds an
	// inline "couldn't load" empty-state, while the failure is announced in the
	// transient global banner so it is visible even when content is already shown.
	a, cmd := a.notify(notifyError, friendlyErr(m.err))
	return a, cmd, true
}

// friendlyErr converts an API error into human-facing banner text, softening the
// raw "API error NOT_FOUND (404): …" wording for the common deleted-resource case.
func friendlyErr(err error) string {
	var apiErr *api.APIError
	if errors.As(err, &apiErr) && apiErr.Status == 404 {
		return "Not found — it may have been deleted."
	}
	return err.Error()
}

// tabIndex returns the index of the currently active screen within menuTabs.
func (a App) tabIndex() int { return tabIndexOf(a) }

// navigateTab moves the active tab by delta (-1 or +1), wrapping at the ends.
func (a *App) navigateTab(delta int) tea.Cmd {
	var cmd tea.Cmd
	*a, cmd = navigateTabBy(*a, delta)
	return cmd
}

func (a *App) delegateUpdate(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	*a, cmd = a.layout.DelegateUpdate(msg, *a)
	return cmd
}

// --- view ---

func (a App) View() string { return a.layout.View(a) }

// activeScreenHasFocusedInput returns true when the current screen has a
// text input that is focused, preventing arrow keys from being consumed by
// the tab navigator instead.
func (a App) activeScreenHasFocusedInput() bool { return a.layout.HasFocusedInput(a) }

// --- theme picker ---

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
				a.saveConfig(func(cfg *config.Config) {
					cfg.Theme = selected
				})
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

// --- help modal ---

// handleHelpModalKey closes the help modal on any keypress.
func (a App) handleHelpModalKey(_ tea.KeyMsg) (tea.Model, tea.Cmd) {
	a.helpModalOpen = false
	return a, nil
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
	case screenGuilds:
		p = a.guilds
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
// opens images in the terminal viewer when supported, or falls through to the
// OS default browser.
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
	if urlutil.IsImageURL(rawURL) &&
		a.graphicsProtocol != imgview.ProtocolNone &&
		a.imageViewer != "browser" {
		return a.openImageInTerminal(rawURL)
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

// openImageInTerminal fetches rawURL, encodes it for the detected graphics
// protocol, and returns a command that sends an imageFetchedMsg when done.
func (a App) openImageInTerminal(rawURL string) (App, tea.Cmd) {
	proto := a.graphicsProtocol
	displayCols := a.width * 4 / 5
	return a, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		img, err := imgview.Fetch(ctx, rawURL)
		if err != nil {
			return imageFetchedMsg{rawURL: rawURL, err: err}
		}
		switch proto {
		case imgview.ProtocolKitty:
			encoded, cols, rows := imgview.EncodeKitty(img, displayCols)
			return imageFetchedMsg{rawURL: rawURL, encoded: encoded, cols: cols, rows: rows}
		case imgview.ProtocolITerm2:
			encoded, cols, rows, err := imgview.EncodeITerm2(img, displayCols)
			return imageFetchedMsg{rawURL: rawURL, encoded: encoded, cols: cols, rows: rows, err: err}
		default:
			return imageFetchedMsg{rawURL: rawURL, err: fmt.Errorf("no graphics protocol")}
		}
	}
}

// handleImageViewer processes image fetch results. On success it opens the
// inline modal overlay; on failure it silently falls back to the browser.
func (a App) handleImageViewer(msg tea.Msg) (App, tea.Cmd, bool) {
	switch m := msg.(type) {
	case imageFetchedMsg:
		if m.err != nil {
			return a, openExternalURL(m.rawURL), true
		}
		a.imageModalEncoded = m.encoded
		a.imageModalCols = m.cols
		a.imageModalRows = m.rows
		a.imageModalOpen = true
		return a, nil, true
	}
	return a, nil, false
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

// --- commands ---

// loginSuccessMsg carries the authenticated session back to the update loop so
// App fields are set there rather than mutated from the command goroutine.
type loginSuccessMsg struct {
	tokens model.Tokens
	user   model.User
}

func (a *App) loginCmd(email, password string) tea.Cmd {
	return func() tea.Msg {
		tokens, err := a.client.Login(email, password)
		if err != nil {
			return screens.LoginErrMsg{Err: err}
		}
		// Initialise the RTDB client from the rtdbToken (best effort).
		if hc, ok := a.client.(*api.HTTPClient); ok {
			_ = hc.InitRTDB(tokens.RTDBToken)
		}
		user, err := a.client.GetOwnProfile()
		if err != nil {
			return screens.LoginErrMsg{Err: err}
		}
		// Wire the user ID into the HTTP client for RTDB path construction.
		if hc, ok := a.client.(*api.HTTPClient); ok {
			hc.SetCurrentUID(user.ID)
		}
		// Persist the refresh token so subsequent launches auto-login.
		// Load first so app settings (APIBaseURL, etc.) are preserved.
		density := ""
		if a.relaxed {
			density = "relaxed"
		}
		a.saveConfig(func(cfg *config.Config) {
			cfg.RefreshToken = tokens.RefreshToken
			cfg.Username = user.Username
			cfg.Email = email
			cfg.SavedAt = time.Now().UTC()
			cfg.Density = density
		})
		return loginSuccessMsg{tokens: tokens, user: user}
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
		if hc, ok := a.client.(*api.HTTPClient); ok {
			_ = hc.InitRTDB(tokens.RTDBToken)
		}
		user, err := a.client.GetOwnProfile()
		if err != nil {
			return screens.LoginErrMsg{Err: err}
		}
		if hc, ok := a.client.(*api.HTTPClient); ok {
			hc.SetCurrentUID(user.ID)
		}
		// Update savedAt so we know when the session was last used.
		// Load first so app settings (APIBaseURL, etc.) are preserved.
		density := ""
		if a.relaxed {
			density = "relaxed"
		}
		a.saveConfig(func(cfg *config.Config) {
			cfg.RefreshToken = tokens.RefreshToken
			cfg.Username = user.Username
			cfg.SavedAt = time.Now().UTC()
			cfg.Density = density
		})
		return loginSuccessMsg{tokens: tokens, user: user}
	}
}

func (a *App) afterLoginCmd() tea.Cmd {
	a.active = screenFeed
	a.profile = a.profile.SetUser(a.currentUser)
	a.feed = a.feed.SetCurrentUsername(a.currentUser.Username)
	a.feed = a.feed.SetFetching()
	a.bookmarks = a.bookmarks.SetFetching()
	a.topics = a.topics.SetFetching()
	a.postDetail = a.postDetail.SetCurrentUsername(a.currentUser.Username)
	a.broadcastConfig()
	return tea.Batch(
		a.loadFeedCmd(),
		a.loadBookmarksCmd(""),
		a.loadWatchesPageCmd(""),
		a.loadTopicsCmd(),
		a.loadProfileCmd(),
		a.fetchUnreadCountCmd(),
		a.schedulePollCmd(),
		a.loadSettingsCmd(),
		a.scheduleWanderCmd(),
		a.checkAndWanderCmd(),
		scheduleLogoAnimCmd(),
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
	timezone       string
	imageViewer    string
}
type wanderTickMsg struct{}
type wanderDoneMsg struct{ at time.Time } // zero At means no update was made
type errMsg struct{ err error }

// notifPostLoadErrMsg is the failure of opening a post from the Notifications
// screen. It is handled in handleNotifications so a deleted target surfaces as a
// friendly transient banner ("This post has been deleted") instead of routing
// through handleErr and blanking the list.
type notifPostLoadErrMsg struct{ err error }

// imageFetchedMsg carries the result of fetching and encoding an image for
// terminal display. err is non-nil when the download or decode failed; rawURL
// is retained so a failed decode can fall back to opening the browser.
type imageFetchedMsg struct {
	rawURL  string
	encoded string
	cols    int
	rows    int
	err     error
}


// notifyLevel selects the color of a global notification banner.
type notifyLevel int

const (
	notifyInfo notifyLevel = iota
	notifyWarn
	notifyError
)

type logoAnimPhase int

const (
	logoPhaseIdle         logoAnimPhase = iota
	logoPhaseScrambling
	logoPhaseHold
	logoPhaseUnscrambling
)

// actionErrMsg is a non-fatal failure from a user-initiated action (post, reply,
// delete, follow, …). Like errMsg it surfaces as a transient global banner and
// never blocks a tab; unlike errMsg it does not set any screen's inline
// "couldn't load" empty-state, since there is no load in flight.
type actionErrMsg struct{ err error }

// notifyMsg sets the global banner directly; used for success/info surfacing.
type notifyMsg struct {
	text  string
	level notifyLevel
}

// notifyExpireMsg clears the banner iff gen still matches a.notifyGen.
type notifyExpireMsg struct{ gen int }
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
type bookmarkDeletedMsg struct {
	bookmarkID          string
	fromBookmarksScreen bool
}
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

type guildsLoadedMsg struct {
	guilds []model.Guild
	cursor string
}
type guildsPageMsg struct {
	guilds []model.Guild
	cursor string
}
type guildPostsLoadedMsg struct {
	slug   string
	posts  []model.Post
	cursor string
}
type guildPostsPageMsg struct {
	slug   string
	posts  []model.Post
	cursor string
}
type guildPostCreatedMsg struct{ slug string }
type guildMembersLoadedMsg struct {
	members []model.GuildMember
	cursor  string
}
type guildMembersPageMsg struct {
	members []model.GuildMember
	cursor  string
}
type guildDetailLoadedMsg struct{ guild model.Guild }
type guildJoinedMsg struct{ slug, name string }
type guildLeftMsg struct{ slug, name string }

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
type logoAnimTickMsg struct{}  // 30s idle trigger — begins the scramble animation
type logoFrameTickMsg struct{} // 60ms per-frame tick during scramble/hold/unscramble

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
			return actionErrMsg{err}
		}
		return followResultMsg{followID: followID}
	}
}

func (a *App) unfollowUserCmd(followID string) tea.Cmd {
	return func() tea.Msg {
		if err := a.client.Unfollow(followID); err != nil {
			return actionErrMsg{err}
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
			return actionErrMsg{err}
		}
		return nil
	}
}

func (a *App) sendCMailCmd(convID, body string) tea.Cmd {
	return func() tea.Msg {
		if err := a.client.SendMessage(convID, body); err != nil {
			return actionErrMsg{err}
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
			return actionErrMsg{err}
		}
		return replyCreatedMsg{postID: postID}
	}
}

func (a *App) createPostCmd(content, title string, topics []string, isPublic, isNSFW bool) tea.Cmd {
	return func() tea.Msg {
		_, err := a.client.CreatePost(content, title, topics, isPublic, isNSFW)
		if err != nil {
			return actionErrMsg{err}
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
			return actionErrMsg{err}
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
	case notifPostLoadErrMsg:
		// A dead session must still redirect to login — let handleUnauthorized
		// (which runs later in the dispatch chain) claim it.
		if errors.Is(msg.err, api.ErrUnauthorized) {
			return a, nil, false
		}
		// The target post is gone (or otherwise unfetchable): announce it in the
		// transient banner and leave the notifications list untouched.
		var apiErr *api.APIError
		if errors.As(msg.err, &apiErr) && apiErr.Status == 404 {
			a, cmd := a.notify(notifyWarn, "This post has been deleted")
			return a, cmd, true
		}
		a, cmd := a.notify(notifyError, msg.err.Error())
		return a, cmd, true
	case screens.ShowUserProfileMsg:
		if a.active != screenNotifications {
			return a, nil, false
		}
		a.profileReturn = screenNotifications
		return a, a.loadUserProfileCmd(msg.Username), true
	case pollUnreadTickMsg:
		return a, tea.Batch(a.fetchUnreadCountCmd(), a.schedulePollCmd()), true
	case unreadCountMsg:
		prev := a.polledUnreadCount
		a.polledUnreadCount = msg.count
		if msg.count > prev {
			return a, a.loadNotifsCmd(), true
		}
		return a, nil, true
	}
	return a, nil, false
}

func (a *App) loadNotifsCmd() tea.Cmd {
	unreadOnly := a.notifications.ShowUnreadOnly()
	return func() tea.Msg {
		notifs, cursor, err := a.client.GetNotifications("", unreadOnly)
		if err != nil {
			return errMsg{err}
		}
		return notifsLoadedMsg{notifs: notifs, cursor: cursor}
	}
}

func (a *App) loadNotifsPageCmd(cursor string) tea.Cmd {
	unreadOnly := a.notifications.ShowUnreadOnly()
	return func() tea.Msg {
		notifs, nextCursor, err := a.client.GetNotifications(cursor, unreadOnly)
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

func (a *App) loadWatchesPageCmd(cursor string) tea.Cmd {
	return func() tea.Msg {
		watches, nextCursor, err := a.client.GetWatches(cursor)
		if err != nil {
			return watchPageMsg{err: err}
		}
		ids := make([]string, len(watches))
		for i, w := range watches {
			ids[i] = w.PostID
		}
		return watchPageMsg{postIDs: ids, cursor: nextCursor}
	}
}

func (a *App) watchPostCmd(postID string) tea.Cmd {
	return func() tea.Msg {
		err := a.client.WatchPost(postID)
		return watchResultMsg{postID: postID, err: err, added: true}
	}
}

func (a *App) unwatchPostCmd(postID string) tea.Cmd {
	return func() tea.Msg {
		err := a.client.UnwatchPost(postID)
		return watchResultMsg{postID: postID, err: err, added: false}
	}
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

func (a *App) deleteBookmarkCmd(id string, fromBookmarksScreen bool) tea.Cmd {
	return func() tea.Msg {
		_ = a.client.DeleteBookmark(id) // fire-and-forget; UI already updated
		return bookmarkDeletedMsg{bookmarkID: id, fromBookmarksScreen: fromBookmarksScreen}
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

// --- Guilds commands ---

func (a *App) loadGuildsCmd(cursor string) tea.Cmd {
	return func() tea.Msg {
		guilds, nextCursor, err := a.client.GetGuilds(cursor)
		if err != nil {
			return errMsg{err}
		}
		return guildsLoadedMsg{guilds: guilds, cursor: nextCursor}
	}
}

func (a *App) loadMoreGuildsCmd(cursor string) tea.Cmd {
	return func() tea.Msg {
		guilds, nextCursor, err := a.client.GetGuilds(cursor)
		if err != nil {
			return errMsg{err}
		}
		return guildsPageMsg{guilds: guilds, cursor: nextCursor}
	}
}

func (a *App) loadGuildPostsCmd(slug string) tea.Cmd {
	return func() tea.Msg {
		posts, cursor, err := a.client.GetGuildPosts(slug, "")
		if err != nil {
			return errMsg{err}
		}
		return guildPostsLoadedMsg{slug: slug, posts: posts, cursor: cursor}
	}
}

func (a *App) loadGuildPostsPageCmd(slug, cursor string) tea.Cmd {
	return func() tea.Msg {
		posts, nextCursor, err := a.client.GetGuildPosts(slug, cursor)
		if err != nil {
			return errMsg{err}
		}
		return guildPostsPageMsg{slug: slug, posts: posts, cursor: nextCursor}
	}
}

func (a *App) createGuildPostCmd(slug, content, title string, topics []string) tea.Cmd {
	return func() tea.Msg {
		_, err := a.client.CreateGuildPost(slug, content, title, topics)
		if err != nil {
			return actionErrMsg{err}
		}
		return guildPostCreatedMsg{slug: slug}
	}
}

func (a *App) loadGuildDetailCmd(slug string) tea.Cmd {
	return func() tea.Msg {
		g, err := a.client.GetGuild(slug)
		if err != nil {
			return actionErrMsg{err}
		}
		return guildDetailLoadedMsg{guild: g}
	}
}

func (a *App) joinGuildCmd(slug, name string) tea.Cmd {
	return func() tea.Msg {
		if err := a.client.JoinGuild(slug); err != nil {
			return actionErrMsg{err}
		}
		return guildJoinedMsg{slug: slug, name: name}
	}
}

func (a *App) leaveGuildCmd(slug, name string) tea.Cmd {
	return func() tea.Msg {
		if err := a.client.LeaveGuild(slug); err != nil {
			return actionErrMsg{err}
		}
		return guildLeftMsg{slug: slug, name: name}
	}
}

func (a *App) loadGuildMembersCmd(slug, cursor string) tea.Cmd {
	return func() tea.Msg {
		members, nextCursor, err := a.client.GetGuildMembers(slug, cursor)
		if err != nil {
			return errMsg{err}
		}
		if cursor == "" {
			return guildMembersLoadedMsg{members: members, cursor: nextCursor}
		}
		return guildMembersPageMsg{members: members, cursor: nextCursor}
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
				return actionErrMsg{err}
			}
			return noteCreatedMsg{note: note}
		}
	}
	id := noteID // capture for closure
	return func() tea.Msg {
		if err := a.client.UpdateNote(id, content, topics); err != nil {
			return actionErrMsg{err}
		}
		return noteUpdatedMsg{noteID: id, content: content, topics: topics}
	}
}

func (a *App) deleteNoteCmd(noteID string) tea.Cmd {
	return func() tea.Msg {
		if err := a.client.DeleteNote(noteID); err != nil {
			return actionErrMsg{err}
		}
		return noteDeletedMsg{noteID: noteID}
	}
}

func (a *App) deletePostCmd(postID string, fromFeed bool) tea.Cmd {
	return func() tea.Msg {
		if err := a.client.DeletePost(postID); err != nil {
			return actionErrMsg{err}
		}
		return postDeletedMsg{postID: postID, fromFeed: fromFeed}
	}
}

func (a *App) deleteReplyCmd(replyID string) tea.Cmd {
	return func() tea.Msg {
		if err := a.client.DeleteReply(replyID); err != nil {
			return actionErrMsg{err}
		}
		return replyDeletedMsg{replyID: replyID}
	}
}

// publishNoteCmd creates a post from the note's content and topics.
// Published notes have no title, are private, and not marked NSFW.
func (a *App) publishNoteCmd(content string, topics []string) tea.Cmd {
	return func() tea.Msg {
		_, err := a.client.CreatePost(content, "", topics, false, false)
		if err != nil {
			return actionErrMsg{err}
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

func scheduleLogoAnimCmd() tea.Cmd {
	return tea.Tick(30*time.Second, func(time.Time) tea.Msg { return logoAnimTickMsg{} })
}

func logoFrameTickCmd() tea.Cmd {
	return tea.Tick(logoFrameInterval, func(time.Time) tea.Msg { return logoFrameTickMsg{} })
}

// checkAndWanderCmd fires a profile location update if wander mode is enabled
// and at least 12 hours have elapsed since the last update. All failures are
// silent — the user is never notified.
func (a *App) checkAndWanderCmd() tea.Cmd {
	return func() tea.Msg {
		if a.ephemeral {
			return wanderDoneMsg{}
		}
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
		count, err := a.client.GetUnreadNotificationCount()
		if err != nil {
			return nil
		}
		return unreadCountMsg{count}
	}
}

func (a *App) loadPostAndShowCmd(postID string) tea.Cmd {
	return func() tea.Msg {
		post, err := a.client.GetPost(postID)
		if err != nil {
			return notifPostLoadErrMsg{err}
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
