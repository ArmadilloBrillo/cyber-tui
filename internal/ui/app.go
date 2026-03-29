package ui

import (
	"fmt"

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
	screenDMs
	screenProfile
)

type App struct {
	client      api.Client
	currentUser model.User
	active      screen
	width       int
	height      int

	login     screens.LoginModel
	feed      screens.FeedModel
	chatrooms screens.ChatroomsModel
	dms       screens.DMsModel
	profile   screens.ProfileModel
}

func NewApp(client api.Client) App {
	return App{
		client:    client,
		active:    screenLogin,
		login:     screens.NewLoginModel(),
		feed:      screens.NewFeedModel(),
		chatrooms: screens.NewChatroomsModel(),
		dms:       screens.NewDMsModel(""),
		profile:   screens.NewProfileModel(),
	}
}

// --- init ---

func (a App) Init() tea.Cmd {
	return a.login.Init()
}

// --- update ---

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height

	case tea.KeyMsg:
		// Global keys
		switch msg.String() {
		case "ctrl+c", "q":
			if a.active != screenLogin {
				return a, tea.Quit
			}
		// Tab navigation (only when logged in)
		case "1":
			if a.active != screenLogin {
				a.active = screenFeed
				return a, a.loadFeedCmd()
			}
		case "2":
			if a.active != screenLogin {
				a.active = screenChatrooms
				return a, a.loadRoomsCmd()
			}
		case "3":
			if a.active != screenLogin {
				a.active = screenDMs
				return a, a.loadConvsCmd()
			}
		case "4":
			if a.active != screenLogin {
				a.active = screenProfile
				return a, a.loadProfileCmd()
			}
		}

	// --- Login flow ---
	case screens.SubmitLoginMsg:
		return a, a.loginCmd(msg.Username, msg.Password)

	case screens.LoginMsg:
		return a, a.afterLoginCmd()

	case screens.LoginErrMsg:
		var cmd tea.Cmd
		a.login, cmd = a.login.Update(msg)
		return a, cmd

	// --- Feed ---
	case feedLoadedMsg:
		a.feed = a.feed.SetPosts(msg.posts)

	// --- Chatrooms ---
	case roomsLoadedMsg:
		a.chatrooms = a.chatrooms.SetRooms(msg.rooms)

	case screens.SendRoomMessageMsg:
		return a, a.sendRoomMessageCmd(msg.RoomID, msg.Body)

	// --- DMs ---
	case convsLoadedMsg:
		a.dms = a.dms.SetConversations(msg.convs)

	case screens.SendDMMsg:
		return a, a.sendDMCmd(msg.ConversationID, msg.Body)

	// --- Profile ---
	case profileLoadedMsg:
		a.profile = a.profile.SetUser(msg.user)

	case screens.SaveProfileMsg:
		return a, a.saveProfileCmd(msg.Bio)

	case errMsg:
		// surface errors to the active screen
		switch a.active {
		case screenFeed:
			a.feed = a.feed.SetError(msg.err)
		case screenProfile:
			a.profile = a.profile.SetError(msg.err)
		}
	}

	// Delegate to active screen
	return a, a.delegateUpdate(msg)
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
	case screenDMs:
		a.dms, cmd = a.dms.Update(msg)
	case screenProfile:
		a.profile, cmd = a.profile.Update(msg)
	}
	return cmd
}

// --- view ---

func (a App) View() string {
	if a.active == screenLogin {
		return a.login.View()
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		a.renderTabBar(),
		a.renderActiveScreen(),
		a.renderStatusBar(),
	)
}

func (a App) renderTabBar() string {
	tabs := []struct {
		key   string
		label string
		s     screen
	}{
		{"1", "feed", screenFeed},
		{"2", "rooms", screenChatrooms},
		{"3", "mail", screenDMs},
		{"4", "profile", screenProfile},
	}

	var bar string
	for _, t := range tabs {
		label := fmt.Sprintf("[%s] %s", t.key, t.label)
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
	case screenDMs:
		return a.dms.View()
	case screenProfile:
		return a.profile.View()
	}
	return ""
}

func (a App) renderStatusBar() string {
	user := theme.StatusBar.Render("@" + a.currentUser.Username)
	hint := theme.StatusBar.Render("  q · quit   1-4 · navigate")
	spacer := theme.StatusBar.Width(a.width - lipgloss.Width(user) - lipgloss.Width(hint)).Render("")
	return lipgloss.JoinHorizontal(lipgloss.Top, user, spacer, hint)
}

// --- commands ---

func (a *App) loginCmd(username, password string) tea.Cmd {
	return func() tea.Msg {
		token, err := a.client.Login(username, password)
		if err != nil {
			return screens.LoginErrMsg{Err: err}
		}
		_ = token
		user, err := a.client.GetProfile(username)
		if err != nil {
			return screens.LoginErrMsg{Err: err}
		}
		a.currentUser = user
		a.dms = screens.NewDMsModel(username)
		return screens.LoginMsg{}
	}
}

func (a *App) afterLoginCmd() tea.Cmd {
	a.active = screenFeed
	a.profile = a.profile.SetUser(a.currentUser)
	return a.loadFeedCmd()
}

type feedLoadedMsg struct{ posts []model.Post }
type roomsLoadedMsg struct{ rooms []model.Room }
type convsLoadedMsg struct{ convs []model.Conversation }
type profileLoadedMsg struct{ user model.User }
type errMsg struct{ err error }

func (a *App) loadFeedCmd() tea.Cmd {
	return func() tea.Msg {
		posts, err := a.client.GetFeed(1)
		if err != nil {
			return errMsg{err}
		}
		return feedLoadedMsg{posts}
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
		user, err := a.client.GetProfile(a.currentUser.Username)
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

func (a *App) sendDMCmd(convID, body string) tea.Cmd {
	return func() tea.Msg {
		if err := a.client.SendMessage(convID, body); err != nil {
			return errMsg{err}
		}
		return nil
	}
}

func (a *App) saveProfileCmd(bio string) tea.Cmd {
	return func() tea.Msg {
		if err := a.client.UpdateProfile(bio); err != nil {
			return errMsg{err}
		}
		a.currentUser.Bio = bio
		return profileLoadedMsg{a.currentUser}
	}
}
