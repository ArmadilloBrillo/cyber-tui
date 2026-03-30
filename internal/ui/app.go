package ui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ragnar/cyber-tui/internal/api"
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

// menuTabs is the ordered list of navigable screens, shared by the
// renderer and key handler so the order is never out of sync.
var menuTabs = []struct {
	label string
	s     screen
}{
	{"feed", screenFeed},
	{"rooms", screenChatrooms},
	{"c-mail", screenCMail},
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

	// autoEmail and autoPassword are set from environment variables.
	// When both are non-empty, Init fires loginCmd immediately.
	autoEmail    string
	autoPassword string

	// dmSub holds the active RTDB subscription for the open C-Mail conversation.
	// nil when no conversation is selected or when not on the C-Mail screen.
	dmSub       *dmSubscription
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

// --- init ---

func (a App) Init() tea.Cmd {
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

	case tea.KeyMsg:
		switch msg.String() {
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
				a.active = screenChatrooms
				return a, a.loadRoomsCmd()
			}
		case "3":
			if a.active != screenLogin {
				a.active = screenCMail
				return a, a.loadConvsCmd()
			}
		case "4":
			if a.active != screenLogin {
				a.cancelDMSubscription()
				a.active = screenProfile
				return a, a.loadProfileCmd()
			}
		// Arrow navigation — left/right cycle tabs when no input is focused
		case "left":
			if a.active != screenLogin && a.active != screenPostDetail && a.focus == focusMenu && !a.activeScreenHasFocusedInput() {
				return a, a.navigateTab(-1)
			}
		case "right":
			if a.active != screenLogin && a.active != screenPostDetail && a.focus == focusMenu && !a.activeScreenHasFocusedInput() {
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

	case repliesLoadedMsg:
		a.postDetail = a.postDetail.SetReplies(msg.replies)

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
	return lipgloss.JoinVertical(lipgloss.Left,
		a.renderTabBar(),
		"", // separator row
		content,
		a.renderStatusBar(),
	)
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
	user := theme.StatusBar.Render("@" + a.currentUser.Username)
	var hintStr string
	switch a.active {
	case screenCMail:
		hintStr = "  Tab · switch pane   ↑↓ · navigate   Enter · open/send   1-4 · jump"
	default:
		hintStr = "  q · quit   ←→ · tabs   ↑↓/jk · navigate   1-4 · jump"
	}
	hint := theme.StatusBar.Render(hintStr)
	spacer := theme.StatusBar.Width(a.width - lipgloss.Width(user) - lipgloss.Width(hint)).Render("")
	return lipgloss.JoinHorizontal(lipgloss.Top, user, spacer, hint)
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
		return screens.LoginMsg{}
	}
}

func (a *App) afterLoginCmd() tea.Cmd {
	a.active = screenFeed
	a.profile = a.profile.SetUser(a.currentUser)
	return a.loadFeedCmd()
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
