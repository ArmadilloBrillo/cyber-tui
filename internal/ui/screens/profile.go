package screens

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
)

type ProfileModel struct {
	user    model.User
	compose ComposeModel
	width   int
	err     error
	saved   bool
}

type SaveProfileMsg struct{ Bio string }

func NewProfileModel() ProfileModel {
	return ProfileModel{compose: NewComposeModel(0)}
}

func (m ProfileModel) SetUser(u model.User) ProfileModel {
	m.user = u
	return m
}

func (m ProfileModel) SetError(err error) ProfileModel {
	m.err = err
	return m
}

// ComposeActive reports whether the bio editor is open, used by app.go to
// route key events directly to the compose box.
func (m ProfileModel) ComposeActive() bool { return m.compose.IsActive() }

func (m ProfileModel) Init() tea.Cmd { return nil }

func (m ProfileModel) Update(msg tea.Msg) (ProfileModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.compose = m.compose.SetWidth(msg.Width)
		return m, nil

	case tea.KeyMsg:
		if m.compose.IsActive() {
			var cmd tea.Cmd
			m.compose, cmd = m.compose.Update(msg)
			return m, cmd
		}
		if msg.String() == "e" {
			m.saved = false
			var cmd tea.Cmd
			m.compose, cmd = m.compose.OpenWithContent("bio", "what's your story…", m.user.Bio)
			return m, cmd
		}

	case ComposeSubmitMsg:
		bio := msg.Content
		m.compose = m.compose.Close()
		return m, func() tea.Msg { return SaveProfileMsg{Bio: bio} }

	case ComposeCancelMsg:
		m.compose = m.compose.Close()
		return m, nil
	}
	return m, nil
}

func (m ProfileModel) View() string {
	if m.err != nil {
		return theme.Error.Render(fmt.Sprintf("profile error: %s", m.err))
	}

	username := theme.Title.Render("@" + m.user.Username)

	if m.compose.IsActive() {
		return lipgloss.JoinVertical(lipgloss.Left,
			username,
			"",
			m.compose.View(),
		)
	}

	bio := theme.Base.Render(m.user.Bio)

	var saved string
	if m.saved {
		saved = theme.Highlight.Render("saved.")
	}

	return theme.Border.Render(
		lipgloss.JoinVertical(lipgloss.Left,
			username,
			"",
			bio,
			"",
			theme.Subtle.Render("e · edit bio"),
			saved,
		),
	)
}
