package ui

import (
	"context"
	"errors"
	"fmt"
	"image"
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
	"github.com/ragnar/cyber-tui/internal/sanitize"
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
	screenSearch
)

type focusTarget int

const (
	focusMenu   focusTarget = iota
	focusList               // list pane (compact post list in 3-pane Miller)
	focusDetail             // reading pane (full post view in 3-pane Miller)
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
	layoutName  string // "tabs" (default) or "miller"; used when persisting to config
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

	// leaderPending is armed by the "g" ("go to") leader key and resolved by
	// the very next keypress against screenForMnemonic — e.g. "g" then "f"
	// jumps to Feed. An unmapped follow-up key silently cancels it.
	leaderPending bool

	// urlPicker state — open with 'o' when multiple URLs are available.
	urlPickerOpen   bool
	urlPickerItems  []string
	urlPickerCursor int

	// imageCarousel state — populated when an image is opened from a picker
	// containing more than one image, letting left/right cycle between them
	// without closing the image modal. Nil imageCarouselItems means a plain
	// single-image view (existing behavior, arrows never shown).
	imageCarouselItems []string
	imageCarouselIndex int
	// imageCache holds decoded images already fetched during the current
	// modal's lifetime, keyed by URL, so cycling back to one skips the
	// network fetch. Cleared whenever the modal closes.
	imageCache map[string]image.Image

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
	search         screens.SearchModel

	// postDetailReturn is the screen to go back to when ESC is pressed in PostDetail.
	postDetailReturn screen

	// profileReturn is the screen to go back to when ESC is pressed in a read-only profile.
	profileReturn screen

	// searchReturn is the screen to go back to when ESC is pressed at Search's
	// outermost level (blurred query, nothing left to peel back). Set whenever
	// '/' switches into Search from somewhere else.
	searchReturn screen

	// cmailReturn is the screen to go back to when ESC is pressed in a
	// deep-linked C-Mail conversation (see CMailModel.canGoBack).
	cmailReturn screen

	// chatroomsReturn is the screen to go back to when ESC is pressed in a
	// deep-linked Chatrooms room (see ChatroomsModel.canGoBack).
	chatroomsReturn screen

	// pendingReplyID is set when navigating to PostDetail from a reply/thread_reply
	// notification. After replies load, PostDetail scrolls to this reply, then it is cleared.
	pendingReplyID string

	// polledUnreadCount is the single source of truth for the tab badge unread count.
	// It is synced from: 60-second server poll, m/M key, and enter on a notification.
	// Never overwrite with the local list count — the server count is always authoritative.
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
	imageNeedsCleanup  bool // true after modal closes until a delete-placement frame reaches the terminal
	imageFetchGen      int  // bumped on every fetch and on close; stale imageFetchedMsg results are dropped

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
		layoutName:         "tabs",
		client:             client,
		active:             screenLogin,
		focus:              focusMenu,
		loc:                time.UTC,
		wanderLust:         false,
		login:              screens.NewLoginModel(""),
		feed:               screens.NewFeedModel(),
		chatrooms:          screens.NewChatroomsModel("", client),
		cmail:              screens.NewCMailModel("", client),
		profile:            screens.NewProfileModel(),
		postDetail:         screens.NewPostDetailModel(),
		notifications:      screens.NewNotificationsModel(),
		settingsScreen:     screens.NewSettingsModel(),
		bookmarks:          screens.NewBookmarksModel(),
		guilds:             screens.NewGuildsModel(),
		topics:             screens.NewTopicsModel(),
		journal:            screens.NewJournalModel(0),
		search:             screens.NewSearchModel(),
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
	a.layoutName = s.Layout
	a.layout = layoutFromName(s.Layout)
	return a
}

func layoutFromName(_ string) Layout {
	return TabsLayout{}
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
	if m, ok := msg.(tea.WindowSizeMsg); ok {
		a = a.applyWindowSize(m)
		contentMsg := tea.WindowSizeMsg{Width: a.layout.ContentWidth(m.Width), Height: a.layout.ContentHeight(m.Height)}
		return a, a.delegateUpdate(contentMsg)
	}
	// Any keypress dismisses a visible notification early. We do NOT return here,
	// so the key still flows on to do its normal job; bumping notifyGen neutralizes
	// the pending expire tick.
	if _, ok := msg.(tea.KeyMsg); ok && a.notifyText != "" {
		a.notifyText = ""
		a.notifyGen++
	}
	// Left/right cycle through a picker-opened image carousel without closing
	// the modal. Any other keypress closes it — consume the key so it doesn't
	// accidentally trigger another action while the modal is visible.
	if km, ok := msg.(tea.KeyMsg); ok && a.imageModalOpen {
		if len(a.imageCarouselItems) > 1 {
			switch km.String() {
			case "left":
				return a.cycleImageCarousel(-1)
			case "right":
				return a.cycleImageCarousel(+1)
			}
		}
		a.imageModalOpen = false
		a.imageNeedsCleanup = (a.graphicsProtocol == imgview.ProtocolKitty)
		a.imageCarouselItems = nil
		a.imageCarouselIndex = 0
		a.imageFetchGen++ // invalidate anything still in flight
		a.imageCache = nil
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
	if a2, cmd, ok := a.handleSearch(msg); ok {
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
	a.search, _ = a.search.Update(msg)
	return a
}

// broadcastConfig pushes the current display settings to all screens.
// Call this whenever loc, relaxed, or dimensions change outside of a
// WindowSizeMsg (e.g. after login, timezone change, or density toggle).
func (a *App) broadcastConfig() {
	msg := screens.SharedConfigMsg{Width: a.layout.ContentWidth(a.width), Height: a.height, Loc: a.loc, Relaxed: a.relaxed, Settings: a.settings, WanderLust: a.wanderLust, MaxThreadDepth: a.maxThreadDepth, Timezone: a.timezone, ImageViewer: a.imageViewer, OwnGuildSlug: a.currentUser.GuildSlug, LayoutName: a.layoutName}
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
	a.search, _ = a.search.Update(msg)
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
	a.search, _ = a.search.Update(msg)
}

// applyWindowSize stores the new terminal dimensions and broadcasts the size
// to all screens so their viewports initialise before they become active.
// The active screen gets a second update via delegateUpdate, which is harmless.
func (a App) applyWindowSize(m tea.WindowSizeMsg) App {
	a.width = m.Width
	a.height = m.Height
	contentMsg := tea.WindowSizeMsg{Width: a.layout.ContentWidth(m.Width), Height: a.layout.ContentHeight(m.Height)}
	return a.updateAll(contentMsg)
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
	// ctrl+c is kept as a hard escape hatch; a handful of other global
	// shortcuts get a ctrl-prefixed twin that reaches through too, since
	// their bare key is unreachable while chatting (CIRC/C-Mail's compose
	// input is focused for the entire detail view, not just a transient
	// sub-mode like Feed's reply box): ctrl+o (open link), ctrl+q (quit),
	// ctrl+t (theme picker), ctrl+/ (search), ctrl+left/right (cycle tabs).
	// The physical ctrl+/ keystroke isn't sent as one universal byte: most
	// terminals send 0x1F (bubbletea names it "ctrl+_"), but Git Bash/MinTTY
	// on Windows sends a literal NUL byte instead — bubbletea has no name for
	// that byte, so it comes through as an ordinary KeyRunes key whose
	// .String() is the raw "\x00" (confirmed via CYBERSPACE_DEBUG_KEYS).
	// Both are accepted so the shortcut works on either. Caveat: ctrl+space,
	// ctrl+2, and ctrl+@ conventionally send that same NUL byte on most
	// terminals too — genuinely indistinguishable from ctrl+/ once it's a
	// single byte with no other terminal reliably distinguishing it. None of
	// those three are bound to anything else today, so this is a latent
	// conflict, not a live one — if any of them are ever bound to a shortcut,
	// they'd also fire Search-jump under MinTTY.
	if a.activeScreenHasFocusedInput() {
		if m.String() == "ctrl+c" {
			return a, tea.Quit, true
		}
		switch m.String() {
		case "ctrl+o", "ctrl+q", "ctrl+t", "ctrl+_", "\x00", "ctrl+left", "ctrl+right":
			// fall through to the global switch below
		default:
			return a, nil, false // fall through to delegateUpdate
		}
	}
	// "g" arms the leader key; the very next keypress resolves against
	// screenForMnemonic regardless of what it is (even another global key
	// like "t" or "q"), so it must be checked ahead of the switch below.
	if a.leaderPending {
		a.leaderPending = false
		if a.active != screenLogin {
			if s, ok := screenForMnemonic(m.String()); ok {
				var cmd tea.Cmd
				a, cmd = activateScreen(a, s)
				if s == screenSearch {
					// activateScreen leaves Search in whatever state it was
					// last left in (correct for arrow-cycling, which no
					// longer reaches Search at all) — "g s" is a deliberate
					// "go to Search" action like '/', so it must always focus
					// the query box the same way '/' already does below.
					a.search = a.search.FocusQuery()
				}
				return a, cmd, true
			}
		}
		return a, nil, true
	}
	if m.String() == "g" {
		if a.active != screenLogin {
			a.leaderPending = true
			return a, nil, true
		}
	}
	switch m.String() {
	case "t", "ctrl+t":
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
	case "o", "ctrl+o":
		if a.active != screenLogin {
			app, cmd := a.handleOpenURL(a.getFocusedURLs())
			return app, cmd, true
		}
	case "/", "ctrl+_", "\x00": // ctrl+_ (0x1F) and "\x00" (NUL, e.g. Git Bash/MinTTY) are both bytes a physical ctrl+/ keystroke can send
		if a.active != screenLogin {
			if a.active != screenSearch {
				a.searchReturn = a.active
			}
			a.cmail = a.cmail.CancelSubscription()
			a.chatrooms = a.chatrooms.CancelSubscription()
			a.active = screenSearch
			a.search = a.search.FocusQuery()
			return a, nil, true
		}
	case "ctrl+c", "q", "ctrl+q":
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
		a.chatrooms = screens.NewChatroomsModel(msg.user.Username, a.client)
		// Initialize the fresh models' viewports with the current terminal size.
		if a.width > 0 {
			contentMsg := tea.WindowSizeMsg{Width: a.layout.ContentWidth(a.width), Height: a.layout.ContentHeight(a.height)}
			a.cmail, _ = a.cmail.Update(contentMsg)
			a.chatrooms, _ = a.chatrooms.Update(contentMsg)
		}
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
		var detailCmd tea.Cmd
		a.feed, detailCmd = a.feed.CurrentDetailCmd()
		// Auto-fill the compact list column if the initial page is shorter than it.
		if min := a.layout.NeedsCompactAutoFill(a.height); min > 0 && msg.cursor != "" && a.feed.PostCount() < min {
			return a, tea.Batch(detailCmd, a.loadFeedPageCmd(msg.cursor)), true
		}
		return a, detailCmd, true
	case feedPageMsg:
		a.feed = a.feed.AppendPosts(msg.posts, msg.cursor)
		// Keep auto-filling until the compact list column is full.
		if min := a.layout.NeedsCompactAutoFill(a.height); min > 0 && msg.cursor != "" && a.feed.PostCount() < min {
			return a, a.loadFeedPageCmd(msg.cursor), true
		}
		return a, nil, true
	case screens.RefreshFeedMsg:
		return a, a.loadFeedCmd(), true
	case screens.LoadMoreFeedMsg:
		return a, a.loadFeedPageCmd(msg.Cursor), true
	case screens.LoadFeedDetailMsg:
		return a, a.loadFeedDetailCmd(msg.PostID), true
	case screens.FeedDetailRepliesMsg:
		a.feed, _ = a.feed.Update(msg)
		return a, nil, true
	case screens.FeedDetailNavMsg:
		a.feed, _ = a.feed.Update(msg)
		return a, nil, true
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
		return a, a.createPostCmd(msg.Content, msg.Title, msg.Slug, msg.Topics, msg.IsPublic, msg.IsNSFW), true
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
		var cmd tea.Cmd
		a.chatrooms, cmd = a.chatrooms.OpenPendingRoom()
		return a, cmd, true
	case screens.OpenRoomMsg:
		// Optimistic mark-read already applied in NotificationsModel.Update; confirm with API.
		if a.polledUnreadCount > 0 {
			a.polledUnreadCount--
		}
		a.chatroomsReturn = a.active
		a.chatrooms = a.chatrooms.SetPendingRoomSlug(msg.RoomSlug)
		// activateScreen resets canGoBack for ordinary tab/leader entry into
		// Chatrooms, so it must be set true *after* that call, not before.
		a, activateCmd := activateScreen(a, screenChatrooms)
		a.chatrooms = a.chatrooms.SetCanGoBack(true)
		return a, tea.Batch(a.markNotifReadCmd(msg.NotifID), activateCmd), true
	case screens.SendRoomMessageMsg:
		return a, a.sendRoomMessageCmd(msg.RoomID, msg.Body), true
	case screens.RoomOpenedMsg:
		return a, a.markRoomReadCmd(msg.RoomID), true
	case screens.RoomReconnectedMsg:
		a, cmd := a.notify(notifyInfo, "reconnected to live chat")
		return a, cmd, true
	case roomCommandReplyMsg:
		a.chatrooms = a.chatrooms.AppendSystemMessage(msg.roomID, sanitize.Strip(msg.reply))
		return a, nil, true
	case screens.LeaveChatroomsMsg:
		a.active = a.chatroomsReturn
		return a, nil, true
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
	case screens.CMailConvSelectedMsg:
		return a, a.markCMailReadCmd(msg.ConversationID), true
	case screens.StartConversationMsg:
		if msg.Username == "" || msg.Username == a.currentUser.Username {
			return a, nil, true
		}
		a.cmailReturn = a.active
		a.cmail = a.cmail.SetCanGoBack(true)
		return a, a.startConversationCmd(msg.Username), true
	case conversationStartedMsg:
		a.active = screenCMail
		a.cmail = a.cmail.SetActiveConversation(msg.conv)
		convID := msg.conv.ID
		return a, tea.Batch(
			a.loadConvsCmd(),
			a.cmail.ConvOpenCmds(convID),
			func() tea.Msg { return screens.CMailConvSelectedMsg{ConversationID: convID} },
		), true
	case screens.CMailReconnectedMsg:
		a, cmd := a.notify(notifyInfo, "reconnected to live chat")
		return a, cmd, true
	case cmailCommandReplyMsg:
		a.cmail = a.cmail.AppendSystemMessage(msg.convID, sanitize.Strip(msg.reply))
		return a, nil, true
	case screens.LeaveCMailMsg:
		a.active = a.cmailReturn
		return a, nil, true
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
		ln := msg.LayoutName
		return a, func() tea.Msg {
			if msg.RemoteChanged {
				if err := a.client.UpdateSettings(s); err != nil {
					return actionErrMsg{err}
				}
			}
			a.saveConfig(func(cfg *config.Config) {
				cfg.WanderLust = wl
				cfg.MaxThreadDepth = td
				cfg.Timezone = tz
				cfg.ImageViewer = iv
				cfg.Layout = ln
			})
			return settingsSavedMsg{settings: s, wanderLust: wl, maxThreadDepth: td, timezone: tz, imageViewer: iv, layoutName: ln}
		}, true

	case settingsSavedMsg:
		a.settings = msg.settings
		a.wanderLust = msg.wanderLust
		a.maxThreadDepth = msg.maxThreadDepth
		a.timezone = msg.timezone
		a.imageViewer = msg.imageViewer
		a.layoutName = msg.layoutName
		a.layout = layoutFromName(msg.layoutName)
		a.focus = focusMenu
		a.loc = config.ParseTimezoneLabel(msg.timezone)
		a.settingsScreen = a.settingsScreen.SetSaved(msg.wanderLust, msg.maxThreadDepth, msg.timezone, msg.imageViewer, msg.layoutName)
		a.broadcastConfig()
		a.refreshViewports()
		if min := a.layout.NeedsCompactAutoFill(a.height); min > 0 {
			var cmds []tea.Cmd
			if cursor := a.feed.NextCursor(); cursor != "" && a.feed.PostCount() < min {
				cmds = append(cmds, a.loadFeedPageCmd(cursor))
			}
			if a.guilds.IsViewingGuildPosts() {
				if cursor := a.guilds.PostsNextCursor(); cursor != "" && a.guilds.PostCount() < min {
					cmds = append(cmds, a.loadGuildPostsPageCmd(a.guilds.ActiveGuild(), cursor))
				}
			}
			if a.topics.IsViewingTopicPosts() {
				if cursor := a.topics.PostsNextCursor(); cursor != "" && a.topics.PostCount() < min {
					cmds = append(cmds, a.loadTopicPostsPageCmd(a.topics.ActiveTopicName(), cursor))
				}
			}
			if len(cmds) > 0 {
				return a, tea.Batch(cmds...), true
			}
		}
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
		var detailCmd tea.Cmd
		a.guilds, detailCmd = a.guilds.CurrentDetailCmd()
		if min := a.layout.NeedsCompactAutoFill(a.height); min > 0 && msg.cursor != "" && a.guilds.PostCount() < min {
			return a, tea.Batch(detailCmd, a.loadGuildPostsPageCmd(msg.slug, msg.cursor)), true
		}
		if detailCmd != nil {
			return a, detailCmd, true
		}
		return a, nil, true

	case screens.LoadGuildThreadMsg:
		return a, a.loadGuildThreadCmd(msg.PostID), true

	case screens.GuildThreadRepliesMsg:
		a.guilds, _ = a.guilds.Update(msg)
		return a, nil, true

	case screens.GuildThreadNavMsg:
		a.guilds, _ = a.guilds.Update(msg)
		return a, nil, true

	case screens.LoadMoreGuildPostsMsg:
		return a, a.loadGuildPostsPageCmd(msg.Slug, msg.Cursor), true

	case guildPostsPageMsg:
		if msg.slug != a.guilds.ActiveGuild() {
			return a, nil, true
		}
		a.guilds = a.guilds.AppendGuildPosts(msg.posts, msg.cursor)
		if min := a.layout.NeedsCompactAutoFill(a.height); min > 0 && msg.cursor != "" && a.guilds.PostCount() < min {
			return a, a.loadGuildPostsPageCmd(msg.slug, msg.cursor), true
		}
		return a, nil, true

	case screens.RefreshGuildPostsMsg:
		return a, a.loadGuildPostsCmd(msg.Slug), true

	case screens.ShowGuildPostMsg:
		a.postDetailReturn = screenGuilds
		a.active = screenPostDetail
		a.postDetail = a.postDetail.SetPost(msg.Post)
		return a, a.loadRepliesCmd(msg.Post.ID), true

	case screens.SubmitGuildPostMsg:
		return a, a.createGuildPostCmd(msg.Slug, msg.Content, msg.Title, msg.PostSlug, msg.Topics), true

	case guildPostCreatedMsg:
		return a, a.loadGuildPostsCmd(msg.slug), true

	case screens.ShowUserProfileMsg:
		if a.active != screenGuilds {
			return a, nil, false
		}
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
		var detailCmd tea.Cmd
		a.topics, detailCmd = a.topics.CurrentDetailCmd()
		if min := a.layout.NeedsCompactAutoFill(a.height); min > 0 && msg.cursor != "" && a.topics.PostCount() < min {
			return a, tea.Batch(detailCmd, a.loadTopicPostsPageCmd(a.topics.ActiveTopicName(), msg.cursor)), true
		}
		if detailCmd != nil {
			return a, detailCmd, true
		}
		return a, nil, true

	case screens.LoadTopicThreadMsg:
		return a, a.loadTopicThreadCmd(msg.PostID), true

	case screens.TopicThreadRepliesMsg:
		a.topics, _ = a.topics.Update(msg)
		return a, nil, true

	case screens.TopicThreadNavMsg:
		a.topics, _ = a.topics.Update(msg)
		return a, nil, true

	case screens.LoadMoreTopicPostsMsg:
		return a, a.loadTopicPostsPageCmd(msg.Slug, msg.Cursor), true

	case topicPostsPageMsg:
		a.topics = a.topics.AppendTopicPosts(msg.posts, msg.cursor)
		if min := a.layout.NeedsCompactAutoFill(a.height); min > 0 && msg.cursor != "" && a.topics.PostCount() < min {
			return a, a.loadTopicPostsPageCmd(a.topics.ActiveTopicName(), msg.cursor), true
		}
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

// handleSearch processes search query, preview, drill-down, and pagination messages.
func (a App) handleSearch(msg tea.Msg) (App, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case screens.SubmitSearchMsg:
		return a, a.searchCmd(msg.Query), true

	case searchPreviewLoadedMsg:
		a.search = a.search.SetPreview(msg.preview, msg.query)
		return a, nil, true

	case screens.DrillSearchTypeMsg:
		return a, a.searchTypeCmd(msg.Type, a.search.LastQuery()), true

	case searchTypeLoadedMsg:
		a.search = a.search.SetTypeResults(msg.hitType, msg.posts, msg.replies, msg.users, msg.cursor)
		return a, nil, true

	case screens.LoadMoreSearchMsg:
		return a, a.searchTypePageCmd(msg.Type, a.search.LastQuery(), msg.Cursor), true

	case searchTypePageMsg:
		a.search = a.search.AppendTypeResults(msg.hitType, msg.posts, msg.replies, msg.users, msg.cursor)
		return a, nil, true

	case screens.ShowSearchPostMsg:
		a.postDetailReturn = screenSearch
		a.active = screenPostDetail
		a.postDetail = a.postDetail.SetPost(msg.Post)
		return a, a.loadRepliesCmd(msg.Post.ID), true

	case screens.ShowSearchReplyMsg:
		a.postDetailReturn = screenSearch
		a.active = screenPostDetail
		a.pendingReplyID = msg.ReplyID
		a.postDetail = a.postDetail.SetPost(model.Post{ID: msg.PostID})
		return a, tea.Batch(a.loadProfilePostCmd(msg.PostID), a.loadRepliesCmd(msg.PostID)), true

	case screens.ShowUserProfileMsg:
		if a.active != screenSearch {
			return a, nil, false
		}
		a.profileReturn = screenSearch
		return a, a.loadUserProfileCmd(msg.Username), true

	case screens.LeaveSearchMsg:
		a.active = a.searchReturn
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
	case screenSearch:
		a.search = a.search.SetError(m.err)
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
	msg := tea.WindowSizeMsg{Width: a.layout.ContentWidth(a.width), Height: a.layout.ContentHeight(a.height)}
	a.feed, _ = a.feed.Update(msg)
	a.chatrooms, _ = a.chatrooms.Update(msg)
	a.cmail, _ = a.cmail.Update(msg)
	a.postDetail, _ = a.postDetail.Update(msg)
	a.profile, _ = a.profile.Update(msg)
	a.notifications, _ = a.notifications.Update(msg)
	a.bookmarks, _ = a.bookmarks.Update(msg)
	a.topics, _ = a.topics.Update(msg)
	a.guilds, _ = a.guilds.Update(msg)
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
	case screenChatrooms:
		p = a.chatrooms
	case screenCMail:
		p = a.cmail
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
		if a.ephemeral {
			return a.notify(notifyInfo, "Opening links is disabled in SSH sessions")
		}
		return a, openExternalURL(rawURL)
	}
	if parsed.Host == "cyberspace.online" || parsed.Host == "www.cyberspace.online" {
		parts := strings.SplitN(strings.TrimPrefix(parsed.Path, "/"), "/", 3)
		if len(parts) >= 2 && parts[0] == "u" && parts[1] != "" {
			a.profileReturn = a.active
			return a, a.loadUserProfileCmd(parts[1])
		}
	}
	// Ephemeral (SSH-hosted) sessions must never launch host processes or make
	// the host fetch remote-chosen URLs (browser spawn / SSRF).
	if a.ephemeral {
		return a.notify(notifyInfo, "Opening links is disabled in SSH sessions")
	}
	if a.canRenderImageInline(rawURL) {
		return a.openImageInTerminal(rawURL)
	}
	return a, openExternalURL(rawURL)
}

// canRenderImageInline reports whether u should be displayed in the inline
// terminal image viewer rather than the OS browser: it must look like an
// image, the terminal must support a graphics protocol, the user's image
// viewer setting must not be "browser", and the session must not be an
// ephemeral SSH-hosted one (which must never have the host fetch a
// remote-chosen URL — see the SSRF guard in routeURL).
func (a App) canRenderImageInline(u string) bool {
	return !a.ephemeral &&
		urlutil.IsImageURL(u) &&
		a.graphicsProtocol != imgview.ProtocolNone &&
		a.imageViewer != "browser"
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
	displayRows := a.height*4/5 - 2 // reserve 2 rows for the modal border
	if displayRows < 1 {
		displayRows = 1
	}
	a.imageFetchGen++
	gen := a.imageFetchGen
	cached, hit := a.imageCache[rawURL]
	return a, func() tea.Msg {
		img := cached
		if !hit {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			var err error
			img, err = imgview.Fetch(ctx, rawURL)
			if err != nil {
				return imageFetchedMsg{rawURL: rawURL, gen: gen, err: err}
			}
		}
		switch proto {
		case imgview.ProtocolKitty:
			encoded, cols, rows := imgview.EncodeKitty(img, displayCols, displayRows)
			return imageFetchedMsg{rawURL: rawURL, gen: gen, decoded: img, encoded: encoded, cols: cols, rows: rows}
		case imgview.ProtocolITerm2:
			encoded, cols, rows, err := imgview.EncodeITerm2(img, displayCols, displayRows)
			return imageFetchedMsg{rawURL: rawURL, gen: gen, decoded: img, encoded: encoded, cols: cols, rows: rows, err: err}
		default:
			return imageFetchedMsg{rawURL: rawURL, gen: gen, err: fmt.Errorf("no graphics protocol")}
		}
	}
}

// handleImageViewer processes image fetch results. On success it opens the
// inline modal overlay; on failure it falls back to the browser, unless a
// carousel is already showing an image, in which case it just notifies and
// leaves the current image displayed rather than surprising the user with a
// browser tab mid-cycle.
func (a App) handleImageViewer(msg tea.Msg) (App, tea.Cmd, bool) {
	switch m := msg.(type) {
	case imageFetchedMsg:
		if m.gen != a.imageFetchGen {
			return a, nil, true // superseded by a later cycle or a close
		}
		if m.err != nil {
			if a.imageModalOpen {
				a2, cmd := a.notify(notifyInfo, "couldn't load image")
				return a2, cmd, true
			}
			return a, openExternalURL(m.rawURL), true
		}
		a.imageModalEncoded = m.encoded
		a.imageModalCols = m.cols
		a.imageModalRows = m.rows
		a.imageModalOpen = true
		a.imageNeedsCleanup = false
		if a.imageCache == nil {
			a.imageCache = make(map[string]image.Image)
		}
		a.imageCache[m.rawURL] = m.decoded
		if a.graphicsProtocol == imgview.ProtocolITerm2 && len(a.imageCarouselItems) > 1 {
			// iTerm2/WezTerm has no Kitty-style delete-all self-heal; force a
			// full repaint so a cycled-to smaller image can't leave stray
			// pixels from the previous one in rows the new frame skips.
			return a, tea.ClearScreen, true
		}
		return a, nil, true
	}
	return a, nil, false
}

// cycleImageCarousel moves to the next/prev image in imageCarouselItems
// (wrapping around) and starts fetching it. The currently displayed image
// stays on screen until the new one arrives.
func (a App) cycleImageCarousel(delta int) (App, tea.Cmd) {
	n := len(a.imageCarouselItems)
	a.imageCarouselIndex = (a.imageCarouselIndex + delta + n) % n
	return a.openImageInTerminal(a.imageCarouselItems[a.imageCarouselIndex])
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
		if a.canRenderImageInline(u) {
			var images []string
			idx := 0
			for _, item := range a.urlPickerItems {
				if a.canRenderImageInline(item) {
					if item == u {
						idx = len(images)
					}
					images = append(images, item)
				}
			}
			a.urlPickerOpen = false
			a.urlPickerItems = nil
			if len(images) > 1 {
				a.imageCarouselItems = images
				a.imageCarouselIndex = idx
			}
			return a.openImageInTerminal(u)
		}
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
		// Initialise the RTDB client using the URL returned by the API (best effort).
		if hc, ok := a.client.(*api.HTTPClient); ok {
			_ = hc.InitRTDB(tokens.IDToken, tokens.RTDBUrl)
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
			_ = hc.InitRTDB(tokens.IDToken, tokens.RTDBUrl)
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
		a.loadConvsCmd(),
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

// roomCommandReplyMsg/cmailCommandReplyMsg carry a reply-only slash command's
// text (e.g. /help) back from the send response, for local display only —
// nothing was posted, so nothing arrives via the RTDB subscription.
type roomCommandReplyMsg struct {
	roomID string
	reply  string
}
type cmailCommandReplyMsg struct {
	convID string
	reply  string
}
type conversationStartedMsg struct{ conv model.Conversation }
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
	layoutName     string
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
	gen     int
	decoded image.Image
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

type searchPreviewLoadedMsg struct {
	preview model.SearchPreview
	query   string
}

// searchTypeLoadedMsg/searchTypePageMsg carry one paginated search category's
// results. Exactly one of posts/replies/users is populated, matching hitType.
type searchTypeLoadedMsg struct {
	hitType string
	posts   []model.Post
	replies []model.Reply
	users   []model.User
	cursor  string
}

type searchTypePageMsg struct {
	hitType string
	posts   []model.Post
	replies []model.Reply
	users   []model.User
	cursor  string
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
		reply, err := a.client.SendRoomMessage(roomID, body)
		if err != nil {
			return actionErrMsg{err}
		}
		if reply != "" {
			return roomCommandReplyMsg{roomID: roomID, reply: reply}
		}
		return nil
	}
}

func (a *App) markRoomReadCmd(roomID string) tea.Cmd {
	return func() tea.Msg {
		_ = a.client.MarkRoomRead(roomID) // fire-and-forget
		return nil
	}
}

func (a *App) sendCMailCmd(convID, body string) tea.Cmd {
	return func() tea.Msg {
		reply, err := a.client.SendMessage(convID, body)
		if err != nil {
			return actionErrMsg{err}
		}
		if reply != "" {
			return cmailCommandReplyMsg{convID: convID, reply: reply}
		}
		return nil
	}
}

func (a App) startConversationCmd(username string) tea.Cmd {
	return func() tea.Msg {
		conv, err := a.client.StartConversation(username)
		if err != nil {
			return actionErrMsg{err}
		}
		return conversationStartedMsg{conv: conv}
	}
}

func (a *App) markCMailReadCmd(convID string) tea.Cmd {
	return func() tea.Msg {
		_ = a.client.MarkCMailRead(convID)
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

func (a *App) loadFeedDetailCmd(postID string) tea.Cmd {
	return func() tea.Msg {
		replies, err := a.client.GetPostReplies(postID)
		if err != nil {
			return screens.FeedDetailRepliesMsg{PostID: postID}
		}
		return screens.FeedDetailRepliesMsg{PostID: postID, Replies: replies}
	}
}

func (a *App) loadGuildThreadCmd(postID string) tea.Cmd {
	return func() tea.Msg {
		replies, err := a.client.GetPostReplies(postID)
		if err != nil {
			return screens.GuildThreadRepliesMsg{PostID: postID}
		}
		return screens.GuildThreadRepliesMsg{PostID: postID, Replies: replies}
	}
}

func (a *App) loadTopicThreadCmd(postID string) tea.Cmd {
	return func() tea.Msg {
		replies, err := a.client.GetPostReplies(postID)
		if err != nil {
			return screens.TopicThreadRepliesMsg{PostID: postID}
		}
		return screens.TopicThreadRepliesMsg{PostID: postID, Replies: replies}
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

func (a *App) createPostCmd(content, title, slug string, topics []string, isPublic, isNSFW bool) tea.Cmd {
	return func() tea.Msg {
		_, err := a.client.CreatePost(content, title, slug, topics, isPublic, isNSFW)
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
		a, cmd := a.suppressActiveRoomMentions(msg.notifs)
		a.notifications = a.notifications.SetNotifs(msg.notifs, msg.cursor)
		return a, cmd, true
	case notifsPageMsg:
		a, cmd := a.suppressActiveRoomMentions(msg.notifs)
		a.notifications = a.notifications.AppendNotifs(msg.notifs, msg.cursor)
		return a, cmd, true
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
		return a, tea.Batch(a.fetchUnreadCountCmd(), a.loadConvsCmd(), a.schedulePollCmd()), true
	case unreadCountMsg:
		prev := a.polledUnreadCount
		a.polledUnreadCount = msg.count
		if msg.count > prev && !a.notifications.HasPaginated() {
			return a, a.loadNotifsCmd(), true
		}
		return a, nil, true
	}
	return a, nil, false
}

// suppressActiveRoomMentions marks read (locally + via API) any unread
// chat_mention notifications for the cIRC room the user currently has open,
// so being mentioned in a room you're already reading doesn't also notify.
func (a App) suppressActiveRoomMentions(notifs []model.Notification) (App, tea.Cmd) {
	roomSlug := a.chatrooms.ActiveRoomSlug()
	if a.active != screenChatrooms || roomSlug == "" {
		return a, nil
	}
	var cmds []tea.Cmd
	for i, n := range notifs {
		if n.Type == "chat_mention" && !n.Read && n.RoomSlug == roomSlug {
			notifs[i].Read = true
			if a.polledUnreadCount > 0 {
				a.polledUnreadCount--
			}
			cmds = append(cmds, a.markNotifReadCmd(n.ID))
		}
	}
	return a, tea.Batch(cmds...)
}

func (a *App) loadNotifsCmd() tea.Cmd {
	unreadOnly := a.notifications.ShowUnreadOnly()
	return func() tea.Msg {
		notifs, cursor, err := a.client.GetNotifications("", unreadOnly, nil)
		if err != nil {
			return errMsg{err}
		}
		return notifsLoadedMsg{notifs: notifs, cursor: cursor}
	}
}

func (a *App) loadNotifsPageCmd(cursor string) tea.Cmd {
	unreadOnly := a.notifications.ShowUnreadOnly()
	return func() tea.Msg {
		notifs, nextCursor, err := a.client.GetNotifications(cursor, unreadOnly, nil)
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

// --- Search commands ---

func (a *App) searchCmd(query string) tea.Cmd {
	return func() tea.Msg {
		preview, err := a.client.Search(query)
		if err != nil {
			return errMsg{err}
		}
		return searchPreviewLoadedMsg{preview: preview, query: query}
	}
}

// searchTypeCmd fetches the first page of one search category (a "see all" drill-down).
func (a *App) searchTypeCmd(hitType, query string) tea.Cmd {
	return func() tea.Msg {
		posts, replies, users, cursor, err := a.searchByType(hitType, query, "")
		if err != nil {
			return errMsg{err}
		}
		return searchTypeLoadedMsg{hitType: hitType, posts: posts, replies: replies, users: users, cursor: cursor}
	}
}

// searchTypePageCmd fetches a subsequent page of one search category.
func (a *App) searchTypePageCmd(hitType, query, cursor string) tea.Cmd {
	return func() tea.Msg {
		posts, replies, users, nextCursor, err := a.searchByType(hitType, query, cursor)
		if err != nil {
			return errMsg{err}
		}
		return searchTypePageMsg{hitType: hitType, posts: posts, replies: replies, users: users, cursor: nextCursor}
	}
}

// searchByType dispatches to the matching typed search client method. Exactly
// one of the three returned slices is populated, matching hitType.
func (a *App) searchByType(hitType, query, cursor string) (posts []model.Post, replies []model.Reply, users []model.User, nextCursor string, err error) {
	switch hitType {
	case "posts":
		posts, nextCursor, err = a.client.SearchPosts(query, cursor)
	case "replies":
		replies, nextCursor, err = a.client.SearchReplies(query, cursor)
	case "users":
		users, nextCursor, err = a.client.SearchUsers(query, cursor)
	}
	return
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

func (a *App) createGuildPostCmd(slug, content, title, postSlug string, topics []string) tea.Cmd {
	return func() tea.Msg {
		_, err := a.client.CreateGuildPost(slug, content, title, postSlug, topics)
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
		_, err := a.client.CreatePost(content, "", "", topics, false, false)
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
