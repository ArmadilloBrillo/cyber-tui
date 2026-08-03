package screens

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
)

// LoginErrMsg reports a failed login attempt. EmailNotVerified/IDToken are
// set when the failure was specifically a 403 EMAIL_NOT_VERIFIED — Login
// itself succeeded (so IDToken is valid), but the follow-up profile fetch
// was rejected because the account's email isn't verified yet. IDToken is
// what ResendVerificationMsg needs to request a fresh verification email.
type LoginErrMsg struct {
	Err              error
	EmailNotVerified bool
	IDToken          string
}

// ResendVerificationMsg asks the app to call POST /v1/auth/resend-verification
// for the given (already-issued) idToken.
type ResendVerificationMsg struct{ IDToken string }

// ResendVerificationResultMsg reports the outcome of a resend request.
type ResendVerificationResultMsg struct{ Err error }

type LoginModel struct {
	inputs  [2]textinput.Model
	focused int
	err     error
	loading bool

	// Set once a login attempt fails with EMAIL_NOT_VERIFIED, so the view can
	// show a distinct message + resend hint instead of the generic error.
	emailNotVerified bool
	idToken          string
	resending        bool
	resendResult     string // "" (not attempted), "sent", or an error string
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
		if m.emailNotVerified && !m.resending && msg.String() == "r" {
			m.resending = true
			m.resendResult = ""
			return m, m.resendCmd()
		}
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
		m.emailNotVerified = msg.EmailNotVerified
		m.idToken = msg.IDToken
		m.resendResult = ""
	case ResendVerificationResultMsg:
		m.resending = false
		if msg.Err != nil {
			m.resendResult = "error: " + msg.Err.Error()
		} else {
			m.resendResult = "sent — check your inbox"
		}
	}

	var cmds [2]tea.Cmd
	for i := range m.inputs {
		m.inputs[i], cmds[i] = m.inputs[i].Update(msg)
	}
	return m, tea.Batch(cmds[0], cmds[1])
}

// resendCmd asks the root app to call ResendVerification for the idToken
// obtained from the just-completed (but unverified) login. Like submitCmd,
// LoginModel has no direct API access — the actual call happens in app.go.
func (m *LoginModel) resendCmd() tea.Cmd {
	idToken := m.idToken
	return func() tea.Msg {
		return ResendVerificationMsg{IDToken: idToken}
	}
}

func (m *LoginModel) submitCmd() tea.Cmd {
	m.loading = true
	m.err = nil
	m.emailNotVerified = false
	m.resendResult = ""
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
	switch {
	case m.loading:
		status = theme.Highlight.Render("connecting...")
	case m.emailNotVerified:
		msg := "your email isn't verified yet — check your inbox for the verification link"
		switch {
		case m.resending:
			msg += "\n" + "sending verification email..."
		case m.resendResult != "":
			msg += "\n" + m.resendResult
		default:
			msg += "\n" + "press r to resend it"
		}
		status = theme.Error.Render(msg)
	case m.err != nil:
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
