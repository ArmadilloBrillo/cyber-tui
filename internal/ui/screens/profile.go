package screens

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
)

const bioCharLimit = 127

type ProfileModel struct {
	user     model.User
	compose  ComposeModel
	width    int
	err      error
	saved    bool
	readOnly  bool
	canGoBack bool
}

func (m ProfileModel) SetReadOnly(readOnly bool) ProfileModel {
	m.readOnly = readOnly
	return m
}

func (m ProfileModel) SetCanGoBack(v bool) ProfileModel {
	m.canGoBack = v
	return m
}

type SaveProfileMsg struct{ Bio string }

func NewProfileModel() ProfileModel {
	return ProfileModel{compose: NewComposeModel(0).SetCharLimit(bioCharLimit)}
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
	case SharedConfigMsg:
		w := msg.Width
		if w > 80 {
			w = 80
		}
		m.compose = m.compose.SetWidth(w)
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		w := msg.Width
		if w > 80 {
			w = 80
		}
		m.compose = m.compose.SetWidth(w)
		return m, nil

	case tea.KeyMsg:
		if m.compose.IsActive() {
			var cmd tea.Cmd
			m.compose, cmd = m.compose.Update(msg)
			return m, cmd
		}
		switch msg.String() {
		case "esc":
			if m.readOnly || m.canGoBack {
				return m, func() tea.Msg { return BackFromProfileMsg{} }
			}
		case "e":
			if !m.readOnly {
				m.saved = false
				var cmd tea.Cmd
				m.compose, cmd = m.compose.OpenWithContent("bio", "what's your story…", m.user.Bio)
				return m, cmd
			}
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
		used := len(m.compose.Content())
		counterStr := fmt.Sprintf("%d / %d", used, bioCharLimit)
		var counter string
		if used >= bioCharLimit {
			counter = theme.Error.Render(counterStr)
		} else {
			counter = theme.Subtle.Render(counterStr)
		}
		return lipgloss.JoinVertical(lipgloss.Left,
			username,
			"",
			m.compose.View(),
			counter,
		)
	}

	bio := theme.Base.Render(m.user.Bio)

	var saved string
	if m.saved && !m.readOnly {
		saved = theme.Highlight.Render("saved.")
	}

	var hint string
	switch {
	case m.readOnly:
		hint = theme.Subtle.Render("esc · back")
	case m.canGoBack:
		hint = theme.Subtle.Render("esc · back   e · edit bio")
	default:
		hint = theme.Subtle.Render("e · edit bio")
	}

	return theme.Border.Width(76).Render(
		lipgloss.JoinVertical(lipgloss.Left,
			username,
			"",
			bio,
			"",
			hint,
			saved,
		),
	)
}
