package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ragnar/cyber-tui/internal/api"
	"github.com/ragnar/cyber-tui/internal/config"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/screens"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
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
)

type focusTarget int

const (
	focusMenu focusTarget = iota
	focusList             // reserved for future list navigation
)

// availableThemes is the ordered list of selectable themes shown in the picker.
var availableThemes = []string{"cyber", "c64", "vt320"}

// menuTabs is the ordered list of navigable screens, shared by the
// renderer and key handler so the order is never out of sync.
var menuTabs = []struct {
	label string
	s     screen
}{
	{"feed", screenFeed},
	{"notifications", screenNotifications},
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

	// timezone is the active UTC offset label (e.g. "UTC+2"). Empty = UTC.
	// loc is the parsed *time.Location derived from timezone.
	timezone string
	loc      *time.Location

	login         screens.LoginModel
	feed          screens.FeedModel
	chatrooms     screens.ChatroomsModel
	cmail         screens.CMailModel
	profile       screens.ProfileModel
	postDetail    screens.PostDetailModel
	notifications screens.NotificationsModel
	settingsScreen screens.SettingsModel

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
}

func NewApp(client api.Client) App {
	return App{
		client:     client,
		active:     screenLogin,
		focus:      focusMenu,
		loc:        time.UTC,
		login:          screens.NewLoginModel(""),
		feed:           screens.NewFeedModel(),
		chatrooms:      screens.NewChatroomsModel(),
		cmail:          screens.NewCMailModel("", client),
		profile:        screens.NewProfileModel(),
		postDetail:     screens.NewPostDetailModel(),
		notifications:  screens.NewNotificationsModel(),
		settingsScreen: screens.NewSettingsModel(),
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
	if a2, cmd, ok := a.handleErr(msg);            ok { return a2, cmd }
	return a, a.delegateUpdate(msg)
}

// broadcastConfig pushes the current display settings to all screens.
// Call this whenever loc, relaxed, or dimensions change outside of a
// WindowSizeMsg (e.g. after login, timezone change, or density toggle).
// Adding a new screen only requires handling SharedConfigMsg in that
// screen's Update — no changes here are needed.
func (a *App) broadcastConfig() {
	msg := screens.SharedConfigMsg{Width: a.width, Height: a.height, Loc: a.loc, Relaxed: a.relaxed, Settings: a.settings}
	a.feed, _ = a.feed.Update(msg)
	a.chatrooms, _ = a.chatrooms.Update(msg)
	a.cmail, _ = a.cmail.Update(msg)
	a.postDetail, _ = a.postDetail.Update(msg)
	a.profile, _ = a.profile.Update(msg)
	a.notifications, _ = a.notifications.Update(msg)
	a.settingsScreen, _ = a.settingsScreen.Update(msg)
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
			a.active = screenProfile
			return a, a.loadProfileCmd(), true
		}
	case "4":
		if a.active != screenLogin {
			a.cmail = a.cmail.CancelSubscription()
			a.active = screenSettings
			return a, nil, true  // no load cmd; settings already in memory
		}
	case "left":
		if a.active != screenLogin && a.active != screenPostDetail && a.active != screenSettings && a.focus == focusMenu {
			return a, a.navigateTab(-1), true
		}
	case "right":
		if a.active != screenLogin && a.active != screenPostDetail && a.active != screenSettings && a.focus == focusMenu {
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

// handleProfile processes profile load and save messages.
func (a App) handleProfile(msg tea.Msg) (App, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case profileLoadedMsg:
		a.currentUser = msg.user
		a.profile = a.profile.SetUser(msg.user).SetCanGoBack(false)
		return a, nil, true
	case userProfileLoadedMsg:
		isOwn := msg.user.Username == a.currentUser.Username
		a.profile = a.profile.SetUser(msg.user).SetReadOnly(!isOwn).SetCanGoBack(true)
		a.active = screenProfile
		return a, nil, true
	case screens.BackFromProfileMsg:
		a.active = a.profileReturn
		a.profile = a.profile.SetReadOnly(false).SetCanGoBack(false)
		return a, nil, true
	case screens.SaveProfileMsg:
		return a, a.saveProfileCmd(msg.Bio), true
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
		return a, func() tea.Msg {
			if err := a.client.UpdateSettings(s); err != nil {
				return errMsg{err}
			}
			return settingsSavedMsg{settings: s}
		}, true

	case settingsSavedMsg:
		a.settings = msg.settings
		a.settingsScreen = a.settingsScreen.SetSaved()
		a.broadcastConfig()
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
		return nil  // no load cmd; settings already in memory
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
	content := lipgloss.NewStyle().Height(contentHeight).Render(a.renderActiveScreen())
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
	var hintStr string
	switch a.active {
	case screenFeed:
		if a.feed.ComposeActive() {
			hintStr = "  Ctrl+S · send   Tab · topics   Enter · paragraph   Esc · cancel"
		} else {
			hintStr = "  p · profile   ? · help"
		}
	case screenPostDetail:
		if a.postDetail.ComposeActive() {
			hintStr = "  Ctrl+S · send   Enter · paragraph   Esc · cancel"
		} else {
			hintStr = "  p · profile   ? · help"
		}
	case screenProfile:
		if a.profile.ComposeActive() {
			hintStr = "  Ctrl+S · save   Enter · paragraph   Esc · cancel"
		} else {
			hintStr = "  ? · help"
		}
	case screenNotifications:
		hintStr = "  m · mark read   M · mark all   u · unread filter   enter · open   p · profile   ? · help"
	case screenSettings:
		if a.settingsScreen.IsDirty() {
			hintStr = "  ctrl+s · save   esc · revert   ? · help"
		} else {
			hintStr = "  space/enter · toggle   ←→ · cycle   ? · help"
		}
	default:
		hintStr = "  ? · help"
	}
	hint := theme.StatusBar.Render(hintStr)
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
	hint := theme.Subtle.Render("↑↓ select   ⏎ save   esc cancel")
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
	a.notifications, _ = a.notifications.Update(msg)
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
	hint := theme.Subtle.Render("↑↓ preview   ⏎ save   esc cancel")
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

// renderHelpModal returns the centered overlay box listing all keyboard shortcuts.
func (a App) renderHelpModal() string {
	title := theme.Title.Render("shortcuts")
	sectionStyle := theme.Subtle.Copy().Bold(true)
	row := func(key, desc string) string {
		k := theme.Highlight.Render(fmt.Sprintf("%-10s", key))
		return lipgloss.JoinHorizontal(lipgloss.Top, k, theme.Subtle.Render(desc))
	}
	col1 := lipgloss.JoinVertical(lipgloss.Left,
		sectionStyle.Render("global"),
		row("1 / 2 / 3", "feed / notifs / profile"),
		row("←→", "cycle tabs"),
		row("t", "theme"),
		row("z", "timezone"),
		row("v", "density"),
		row("q", "quit"),
		"",
		sectionStyle.Render("compose"),
		row("Ctrl+S", "send"),
		row("Enter", "paragraph"),
		row("Esc", "cancel"),
		"",
		sectionStyle.Render("notifications"),
		row("↑↓ / jk", "navigate"),
		row("enter", "open post / profile"),
		row("m", "mark read"),
		row("M", "mark all read"),
		row("u", "toggle unread filter"),
		row("p", "view profile"),
	)
	col2 := lipgloss.JoinVertical(lipgloss.Left,
		sectionStyle.Render("feed"),
		row("↑↓ / jk", "navigate"),
		row("enter", "open post"),
		row("p", "view profile"),
		row("n", "new post"),
		row("r", "reply"),
		"",
		sectionStyle.Render("post detail"),
		row("↑↓ / jk", "scroll / navigate"),
		row("p", "view profile"),
		row("r", "reply"),
		row("esc", "back"),
		"",
		sectionStyle.Render("profile"),
		row("e", "edit bio"),
		row("esc", "back (other profiles)"),
		row("←→", "tabs"),
	)
	columns := lipgloss.JoinHorizontal(lipgloss.Top, col1, "    ", col2)
	body := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		columns,
		"",
		theme.Subtle.Render("? or any key · close"),
	)
	return theme.ActiveBorder.Render(body)
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
	a.broadcastConfig()
	return tea.Batch(a.loadFeedCmd(), a.loadProfileCmd(), a.schedulePollCmd(), a.loadSettingsCmd())
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
type userProfileLoadedMsg struct{ user model.User }
type repliesLoadedMsg struct{ replies []model.Reply }
type replyCreatedMsg struct{ postID string }
type postCreatedMsg struct{}
type settingsLoadedMsg struct{ settings model.Settings }
type settingsSavedMsg struct{ settings model.Settings }
type errMsg struct{ err error }
type notifsLoadedMsg struct {
	notifs []model.Notification
	cursor string
}
type notifsPageMsg struct {
	notifs []model.Notification
	cursor string
}
type notifPostLoadedMsg struct{ post model.Post }
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
		return userProfileLoadedMsg{user: user}
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

func (a *App) saveProfileCmd(bio string) tea.Cmd {
	return func() tea.Msg {
		update := model.ProfileUpdate{Bio: &bio}
		if err := a.client.UpdateProfile(update); err != nil {
			return errMsg{err}
		}
		a.currentUser.Bio = bio
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

func (a *App) schedulePollCmd() tea.Cmd {
	return tea.Tick(60*time.Second, func(time.Time) tea.Msg { return pollUnreadTickMsg{} })
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
