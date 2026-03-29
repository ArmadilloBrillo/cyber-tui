package screens

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
)

type ProfileModel struct {
	user     model.User
	bioInput textinput.Model
	editing  bool
	err      error
	saved    bool
}

type SaveProfileMsg struct{ Bio string }

func NewProfileModel() ProfileModel {
	input := textinput.New()
	input.Placeholder = "bio..."
	input.Width = 50
	return ProfileModel{bioInput: input}
}

func (m ProfileModel) SetUser(u model.User) ProfileModel {
	m.user = u
	m.bioInput.SetValue(u.Bio)
	return m
}

func (m ProfileModel) SetError(err error) ProfileModel {
	m.err = err
	return m
}

func (m ProfileModel) Init() tea.Cmd { return nil }

func (m ProfileModel) Update(msg tea.Msg) (ProfileModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "e":
			if !m.editing {
				m.editing = true
				m.saved = false
				m.bioInput.SetValue(m.user.Bio)
				m.bioInput.Focus()
				return m, textinput.Blink
			}
		case "esc":
			if m.editing {
				m.editing = false
				m.bioInput.Blur()
			}
		case "enter":
			if m.editing {
				m.editing = false
				m.bioInput.Blur()
				bio := m.bioInput.Value()
				return m, func() tea.Msg {
					return SaveProfileMsg{Bio: bio}
				}
			}
		}
	}

	if m.editing {
		var cmd tea.Cmd
		m.bioInput, cmd = m.bioInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m ProfileModel) View() string {
	if m.err != nil {
		return theme.Error.Render(fmt.Sprintf("profile error: %s", m.err))
	}

	username := theme.Title.Render("@" + m.user.Username)

	var bio string
	if m.editing {
		bio = theme.Border.Render(m.bioInput.View())
	} else {
		bio = theme.Base.Render(m.user.Bio)
	}

	var hint string
	if m.editing {
		hint = theme.Subtle.Render("enter · save   esc · cancel")
	} else {
		hint = theme.Subtle.Render("e · edit bio")
	}

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
			hint,
			saved,
		),
	)
}
