package screens

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
)

// PathPromptModel is a minimal single-line file-path prompt, reused for both
// theme export and import. It has no purpose-specific behavior itself —
// App sets an appropriate title/default/warning via the fields below.
type PathPromptModel struct {
	title   string // e.g. "export theme to" / "import theme from"
	input   textinput.Model
	warning string // set by App (e.g. "file exists — enter again to overwrite"); cleared on the next keystroke
}

func NewPathPromptModel() PathPromptModel {
	ti := textinput.New()
	ti.Width = 40
	return PathPromptModel{input: ti}
}

// Open resets and focuses the prompt with the given title and prefilled path.
func (m PathPromptModel) Open(title, defaultPath string) (PathPromptModel, tea.Cmd) {
	m.title = title
	m.warning = ""
	m.input.SetValue(defaultPath)
	m.input.CursorEnd()
	return m, m.input.Focus()
}

// SetWarning attaches a warning line (e.g. an overwrite confirmation
// prompt) shown below the input until the next keystroke.
func (m PathPromptModel) SetWarning(text string) PathPromptModel {
	m.warning = text
	return m
}

// Value returns the current input text. Callers that intercept enter/esc
// themselves (rather than relying on PathPromptSubmitMsg/PathPromptCancelMsg
// — e.g. because that message type is already claimed by another open
// prompt) read the submitted value through here.
func (m PathPromptModel) Value() string { return m.input.Value() }

func (m PathPromptModel) Update(msg tea.KeyMsg) (PathPromptModel, tea.Cmd) {
	switch msg.String() {
	case "enter":
		path := m.input.Value()
		return m, func() tea.Msg { return PathPromptSubmitMsg{Path: path} }
	case "esc":
		return m, func() tea.Msg { return PathPromptCancelMsg{} }
	}
	m.warning = ""
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m PathPromptModel) View() string {
	rows := []string{theme.Title.Render(m.title), "", m.input.View()}
	if m.warning != "" {
		rows = append(rows, "", theme.Error.Render(m.warning))
	}
	rows = append(rows, "", theme.Subtle.Render("enter · confirm   esc · cancel"))
	return theme.ActiveBorder.Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
}
