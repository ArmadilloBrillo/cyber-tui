package ui

import (
	"context"
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
	{"profile", screenProfile},
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

	// timezone is the active UTC offset label (e.g. "UTC+2"). Empty = UTC.
	// loc is the parsed *time.Location derived from timezone.
	timezone string
	loc      *time.Location

	// dmSub holds the active RTDB subscription for the open C-Mail conversation.
	// nil when no conversation is selected or when not on the C-Mail screen.
	dmSub        *dmSubscription
	activeConvID string

	login      screens.LoginModel
	feed       screens.FeedModel
	chatrooms  screens.ChatroomsModel
	cmail      screens.CMailModel
	profile    screens.ProfileModel
	postDetail screens.PostDetailModel
}

type dmSubscription struct {
	C      <-chan model.Message
	cancel context.CancelFunc
}

func NewApp(client api.Client) App {
	return App{
		client:     client,
		active:     screenLogin,
		focus:      focusMenu,
		loc:        time.UTC,
		login:      screens.NewLoginModel(),
		feed:       screens.NewFeedModel(),
		chatrooms:  screens.NewChatroomsModel(),
		cmail:      screens.NewCMailModel(""),
		profile:    screens.NewProfileModel(),
		postDetail: screens.NewPostDetailModel(),
	}
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

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		// Broadcast to all screens so their viewports initialise before they
		// become active. The active screen gets a second update via delegateUpdate
		// below, which is harmless (it just re-applies the same size).
		a.feed, _ = a.feed.Update(msg)
		a.chatrooms, _ = a.chatrooms.Update(msg)
		a.cmail, _ = a.cmail.Update(msg)
		a.postDetail, _ = a.postDetail.Update(msg)
		a.profile, _ = a.profile.Update(msg)

	case tea.KeyMsg:
		// Theme picker intercepts all keys while open.
		if a.timezonePickerOpen {
			return a.handleTimezonePickerKey(msg)
		}
		if a.themePickerOpen {
			return a.handleThemePickerKey(msg)
		}
		// When any screen has a focused text input, let it consume all key events.
		// Only ctrl+c is kept as a hard escape hatch.
		if a.activeScreenHasFocusedInput() {
			if msg.String() == "ctrl+c" {
				return a, tea.Quit
			}
			break
		}
		switch msg.String() {
		case "t":
			if a.active != screenLogin {
				a.themePickerOpen = true
				a.themePickerOrig = theme.CurrentName()
				a.themePickerCursor = themeIndex(theme.CurrentName())
			}
		case "z":
			if a.active != screenLogin {
				a.timezonePickerOpen = true
				a.timezonePickerOrig = a.timezone
				a.timezonePickerCursor = timezoneIndex(a.timezone)
			}
		case "v":
			if a.active != screenLogin {
				a.relaxed = !a.relaxed
				a.feed = a.feed.SetRelaxed(a.relaxed)
				a.postDetail = a.postDetail.SetRelaxed(a.relaxed)
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
				}
			}
		case "ctrl+c", "q":
			if a.active != screenLogin {
				return a, tea.Quit
			}
		// Number shortcuts — always jump directly
		case "1":
			if a.active != screenLogin {
				a.cancelDMSubscription()
				a.active = screenFeed
				return a, a.loadFeedCmd()
			}
		case "2":
			if a.active != screenLogin {
				a.cancelDMSubscription()
				a.active = screenProfile
				return a, a.loadProfileCmd()
			}
		// Arrow navigation — left/right cycle tabs when no input is focused
		case "left":
			if a.active != screenLogin && a.active != screenPostDetail && a.focus == focusMenu {
				return a, a.navigateTab(-1)
			}
		case "right":
			if a.active != screenLogin && a.active != screenPostDetail && a.focus == focusMenu {
				return a, a.navigateTab(+1)
			}
		}

	// --- Login flow ---
	case screens.SubmitLoginMsg:
		return a, a.loginCmd(msg.Email, msg.Password)

	case screens.LoginMsg:
		return a, a.afterLoginCmd()

	case screens.LoginErrMsg:
		var cmd tea.Cmd
		a.login, cmd = a.login.Update(msg)
		return a, cmd

	// --- Feed ---
	case feedLoadedMsg:
		a.feed = a.feed.SetPosts(msg.posts, msg.cursor)

	case feedPageMsg:
		a.feed = a.feed.AppendPosts(msg.posts, msg.cursor)

	case screens.LoadMoreFeedMsg:
		return a, a.loadFeedPageCmd(msg.Cursor)

	case screens.ShowPostMsg:
		a.active = screenPostDetail
		a.postDetail = a.postDetail.SetPost(msg.Post)
		return a, a.loadRepliesCmd(msg.Post.ID)

	case screens.ShowPostForReplyMsg:
		a.active = screenPostDetail
		a.postDetail = a.postDetail.SetPost(msg.Post)
		var openCmd tea.Cmd
		a.postDetail, openCmd = a.postDetail.OpenCompose()
		return a, tea.Batch(a.loadRepliesCmd(msg.Post.ID), openCmd)

	case repliesLoadedMsg:
		a.postDetail = a.postDetail.SetReplies(msg.replies)

	case screens.SubmitNewPostMsg:
		return a, a.createPostCmd(msg.Content)

	case postCreatedMsg:
		return a, a.loadFeedCmd()

	case screens.SubmitReplyMsg:
		return a, a.createReplyCmd(msg.PostID, msg.Content, msg.ParentReplyID)

	case replyCreatedMsg:
		return a, a.loadRepliesCmd(msg.postID)

	case screens.BackToFeedMsg:
		a.active = screenFeed

	// --- Chatrooms ---
	case roomsLoadedMsg:
		a.chatrooms = a.chatrooms.SetRooms(msg.rooms)

	case screens.SendRoomMessageMsg:
		return a, a.sendRoomMessageCmd(msg.RoomID, msg.Body)

	// --- C-Mail ---
	case convsLoadedMsg:
		a.cmail = a.cmail.SetConversations(msg.convs)

	case screens.SelectConvMsg:
		a.cancelDMSubscription()
		a.activeConvID = msg.ConversationID
		return a, tea.Batch(
			a.loadConvMessagesCmd(msg.ConversationID),
			a.openDMSubscriptionCmd(msg.ConversationID),
		)

	case dmSubscribedMsg:
		// Stale guard: ignore if user navigated away before subscription connected.
		if msg.convID != a.activeConvID {
			msg.sub.cancel()
			return a, nil
		}
		a.dmSub = msg.sub
		return a, waitForDM(a.dmSub)

	case msgsLoadedMsg:
		if msg.convID == a.activeConvID {
			a.cmail = a.cmail.SetConversationMessages(msg.convID, msg.msgs)
		}

	case dmReceivedMsg:
		a.cmail = a.cmail.AppendMessage(msg.msg)
		if a.dmSub != nil {
			return a, waitForDM(a.dmSub)
		}

	case dmStreamClosedMsg:
		a.dmSub = nil

	case screens.SendCMailMsg:
		return a, a.sendCMailCmd(msg.ConversationID, msg.Body)

	// --- Profile ---
	case profileLoadedMsg:
		a.currentUser = msg.user
		a.profile = a.profile.SetUser(msg.user)

	case screens.SaveProfileMsg:
		return a, a.saveProfileCmd(msg.Bio)

	case errMsg:
		switch a.active {
		case screenFeed:
			a.feed = a.feed.SetError(msg.err)
		case screenCMail:
			a.cmail = a.cmail.SetError(msg.err)
		case screenProfile:
			a.profile = a.profile.SetError(msg.err)
		case screenPostDetail:
			a.postDetail = a.postDetail.SetError(msg.err)
		}
	}

	return a, a.delegateUpdate(msg)
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
		a.cancelDMSubscription()
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
	return base
}

func (a App) renderTabBar() string {
	var bar string
	for _, t := range menuTabs {
		if a.active == t.s {
			bar += theme.ActiveTab.Render(t.label)
		} else {
			bar += theme.Tab.Render(t.label)
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
	user := lipgloss.JoinHorizontal(lipgloss.Top,
		userStyle.Render("@"+a.currentUser.Username),
		metaStyle.Render("  ·  "+densityLabel),
		metaStyle.Render("  ·  "+theme.CurrentName()),
		metaStyle.Render("  ·  "+tzLabel),
	)
	var hintStr string
	switch a.active {
	case screenPostDetail:
		if a.postDetail.ComposeActive() {
			hintStr = "  Alt+Enter · send   Enter · paragraph   Esc · cancel"
		} else {
			hintStr = "  esc · back   r · reply   j/k · scroll/navigate   t · theme   z · timezone"
		}
	case screenProfile:
		if a.profile.ComposeActive() {
			hintStr = "  Alt+Enter · save   Enter · paragraph   Esc · cancel"
		} else {
			hintStr = "  q · quit   v · density   t · theme   z · timezone   ←→ · tabs"
		}
	default:
		hintStr = "  q · quit   r · reply   v · density   t · theme   z · timezone   ←→ · tabs   ↑↓/jk · navigate"
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
		a.feed = a.feed.SetLocation(a.loc)
		a.postDetail = a.postDetail.SetLocation(a.loc)
		a.cmail = a.cmail.SetLocation(a.loc)
		a.chatrooms = a.chatrooms.SetLocation(a.loc)
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
		a.cmail = screens.NewCMailModel(user.Username)
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
		a.cmail = screens.NewCMailModel(user.Username)
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
	a.feed = a.feed.SetRelaxed(a.relaxed)
	a.postDetail = a.postDetail.SetRelaxed(a.relaxed)
	a.feed = a.feed.SetLocation(a.loc)
	a.postDetail = a.postDetail.SetLocation(a.loc)
	a.cmail = a.cmail.SetLocation(a.loc)
	a.chatrooms = a.chatrooms.SetLocation(a.loc)
	return tea.Batch(a.loadFeedCmd(), a.loadProfileCmd())
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
type repliesLoadedMsg struct{ replies []model.Reply }
type replyCreatedMsg struct{ postID string }
type postCreatedMsg struct{}
type errMsg struct{ err error }

// DM subscription message types.
type dmSubscribedMsg struct {
	convID string
	sub    *dmSubscription
}
type dmReceivedMsg struct{ msg model.Message }
type dmStreamClosedMsg struct{}
type msgsLoadedMsg struct {
	convID string
	msgs   []model.Message
}

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

// cancelDMSubscription stops any active RTDB subscription and clears the state.
func (a *App) cancelDMSubscription() {
	if a.dmSub != nil {
		a.dmSub.cancel()
		a.dmSub = nil
	}
	a.activeConvID = ""
}

// openDMSubscriptionCmd starts a live RTDB stream for convID.
func (a *App) openDMSubscriptionCmd(convID string) tea.Cmd {
	return func() tea.Msg {
		ch, cancel, err := a.client.SubscribeDMs(context.Background(), convID)
		if err != nil {
			return errMsg{err}
		}
		return dmSubscribedMsg{convID: convID, sub: &dmSubscription{C: ch, cancel: cancel}}
	}
}

// loadConvMessagesCmd fetches message history for a conversation.
func (a *App) loadConvMessagesCmd(convID string) tea.Cmd {
	return func() tea.Msg {
		msgs, err := a.client.GetMessages(convID, 50)
		if err != nil {
			return errMsg{err}
		}
		return msgsLoadedMsg{convID: convID, msgs: msgs}
	}
}

// waitForDM blocks on the subscription channel and returns the next message.
func waitForDM(sub *dmSubscription) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-sub.C
		if !ok {
			return dmStreamClosedMsg{}
		}
		return dmReceivedMsg{msg: msg}
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

func (a *App) loadProfileCmd() tea.Cmd {
	return func() tea.Msg {
		user, err := a.client.GetOwnProfile()
		if err != nil {
			return errMsg{err}
		}
		return profileLoadedMsg{user}
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

func (a *App) createPostCmd(content string) tea.Cmd {
	return func() tea.Msg {
		_, err := a.client.CreatePost(content, nil)
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
