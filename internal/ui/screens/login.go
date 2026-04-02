package screens

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
)

type LoginMsg struct{}
type LoginErrMsg struct{ Err error }

type LoginModel struct {
	inputs  [2]textinput.Model
	focused int
	err     error
	loading bool
}

func NewLoginModel(email string) LoginModel {
	user := textinput.New()
	user.Placeholder = "email"
	user.Width = 30

	pass := textinput.New()
	pass.Placeholder = "password"
	pass.EchoMode = textinput.EchoPassword
	pass.Width = 30

	focused := 0
	if email != "" {
		user.SetValue(email)
		focused = 1
	}

	if focused == 0 {
		user.Focus()
	} else {
		pass.Focus()
	}

	return LoginModel{inputs: [2]textinput.Model{user, pass}, focused: focused}
}

func (m LoginModel) Init() tea.Cmd { return textinput.Blink }

func (m LoginModel) Update(msg tea.Msg) (LoginModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab", "down":
			m.focused = (m.focused + 1) % 2
		case "shift+tab", "up":
			m.focused = (m.focused + 1) % 2
		case "enter":
			if m.focused == 0 {
				m.focused = 1
			} else {
				return m, m.submitCmd()
			}
		}
		for i := range m.inputs {
			if i == m.focused {
				m.inputs[i].Focus()
			} else {
				m.inputs[i].Blur()
			}
		}
	case LoginErrMsg:
		m.err = msg.Err
		m.loading = false
	}

	var cmds [2]tea.Cmd
	for i := range m.inputs {
		m.inputs[i], cmds[i] = m.inputs[i].Update(msg)
	}
	return m, tea.Batch(cmds[0], cmds[1])
}

func (m *LoginModel) submitCmd() tea.Cmd {
	m.loading = true
	m.err = nil
	// Login is handled by the root app which has the API client.
	// We send credentials up via a message.
	return func() tea.Msg {
		return SubmitLoginMsg{
			Email:    m.inputs[0].Value(),
			Password: m.inputs[1].Value(),
		}
	}
}

type SubmitLoginMsg struct {
	Email    string
	Password string
}

func (m LoginModel) View() string {
	banner := theme.Title.Render(`
  ██████╗██╗   ██╗██████╗ ███████╗██████╗ ███████╗██████╗  █████╗  ██████╗███████╗
 ██╔════╝╚██╗ ██╔╝██╔══██╗██╔════╝██╔══██╗██╔════╝██╔══██╗██╔══██╗██╔════╝██╔════╝
 ██║      ╚████╔╝ ██████╔╝█████╗  ██████╔╝███████╗██████╔╝███████║██║     █████╗
 ██║       ╚██╔╝  ██╔══██╗██╔══╝  ██╔══██╗╚════██║██╔═══╝ ██╔══██║██║     ██╔══╝
 ╚██████╗   ██║   ██████╔╝███████╗██║  ██║███████║██║      ██║  ██║╚██████╗███████╗
  ╚═════╝   ╚═╝   ╚═════╝ ╚══════╝╚═╝  ╚═╝╚══════╝╚═╝      ╚═╝  ╚═╝ ╚═════╝╚══════╝`)

	subtitle := theme.Subtle.Render("  social media de-imagined  ·  cyberspace.online")

	userLabel := theme.Subtle.Render("email")
	passLabel := theme.Subtle.Render("password")

	form := lipgloss.JoinVertical(lipgloss.Left,
		userLabel,
		theme.Border.Render(m.inputs[0].View()),
		"",
		passLabel,
		theme.Border.Render(m.inputs[1].View()),
	)

	hint := theme.Subtle.Render("tab · navigate   enter · confirm")

	var status string
	if m.loading {
		status = theme.Highlight.Render("connecting...")
	} else if m.err != nil {
		status = theme.Error.Render(fmt.Sprintf("error: %s", m.err))
	}

	return lipgloss.JoinVertical(lipgloss.Center,
		"",
		banner,
		subtitle,
		"",
		"",
		form,
		"",
		hint,
		status,
	)
}
