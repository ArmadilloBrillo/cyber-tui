package screens

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
)

const (
	composeMinLines = 3
	composeMaxLines = 8
)

// ComposeSubmitMsg is emitted when the user presses Ctrl+Enter to send.
type ComposeSubmitMsg struct{ Content string }

// ComposeCancelMsg is emitted when the user presses Esc to cancel.
type ComposeCancelMsg struct{}

// ComposeModel is a reusable expanding multi-line text editor.
// Embed it in any screen that needs a compose area.
//   - Enter inserts a paragraph break (\n\n → <p> on the website)
//   - Alt+Enter (or Ctrl+Enter with Kitty) submits (emits ComposeSubmitMsg)
//   - Esc cancels (emits ComposeCancelMsg)
type ComposeModel struct {
	textarea     textarea.Model
	context      string // label shown above the editor, e.g. "reply to @username"
	active       bool
	width        int
	contentLines int // current textarea height in lines, clamped [composeMinLines, composeMaxLines]
}

// NewComposeModel creates a ComposeModel. Width is set correctly when the first
// WindowSizeMsg arrives (compose.SetWidth is called by the host screen).
func NewComposeModel(width int) ComposeModel {
	ta := textarea.New()
	ta.CharLimit = 32768
	ta.ShowLineNumbers = false
	innerW := width - 4
	if innerW < 1 {
		innerW = 1
	}
	ta.SetWidth(innerW)
	ta.SetHeight(composeMinLines)
	ta.Placeholder = "write your reply…"
	return ComposeModel{
		textarea:     ta,
		width:        width,
		contentLines: composeMinLines,
	}
}

// Open prepares the compose box: sets the context label, clears content,
// resets height to minimum, and focuses the textarea.
// Returns the updated model and a Cmd that starts the cursor blink animation.
func (m ComposeModel) Open(ctx, placeholder string) (ComposeModel, tea.Cmd) {
	m.context = ctx
	m.active = true
	m.contentLines = composeMinLines
	m.textarea.Placeholder = placeholder
	m.textarea.SetValue("")
	m.textarea.SetHeight(composeMinLines)
	cmd := m.textarea.Focus()
	return m, cmd
}

// Close blurs the textarea and marks the compose box inactive.
func (m ComposeModel) Close() ComposeModel {
	m.active = false
	m.textarea.Blur()
	return m
}

// IsActive reports whether the compose box is open and focused.
func (m ComposeModel) IsActive() bool { return m.active }

// Content returns the current textarea value.
func (m ComposeModel) Content() string { return m.textarea.Value() }

// BoxHeight returns the total number of terminal rows this component renders:
// 2 border rows + 1 context label row + contentLines textarea rows.
func (m ComposeModel) BoxHeight() int { return m.contentLines + 3 }

// SetWidth resizes the compose area to fit the new terminal width.
func (m ComposeModel) SetWidth(w int) ComposeModel {
	m.width = w
	innerW := w - 4
	if innerW < 1 {
		innerW = 1
	}
	m.textarea.SetWidth(innerW)
	return m
}

// Update handles key events and delegates to textarea.
// ComposeSubmitMsg and ComposeCancelMsg are returned as commands (not inline) so
// the host screen receives them as Bubble Tea messages in its own Update loop.
func (m ComposeModel) Update(msg tea.Msg) (ComposeModel, tea.Cmd) {
	if !m.active {
		return m, nil
	}

	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "ctrl+enter", "alt+enter":
			// ctrl+enter requires Kitty keyboard protocol; alt+enter (ESC+CR)
			// works in virtually all terminals and is the reliable fallback.
			content := m.textarea.Value()
			return m, func() tea.Msg { return ComposeSubmitMsg{Content: content} }
		case "esc":
			return m, func() tea.Msg { return ComposeCancelMsg{} }
		case "enter":
			// Paragraph break: insert \n\n so the website renderer (GFM breaks: true)
			// wraps this in <p> tags, matching the website's own Enter behaviour.
			// Note: shift+enter cannot be distinguished from enter in most terminals
			// without Kitty keyboard protocol, so hard line breaks (\n → <br>) are
			// not supported unless the terminal negotiates Kitty.
			m.textarea, _ = m.textarea.Update(tea.KeyMsg{Type: tea.KeyEnter})
			m.textarea, _ = m.textarea.Update(tea.KeyMsg{Type: tea.KeyEnter})
			return m.recalcHeight(), nil
		}
	}

	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	return m.recalcHeight(), cmd
}

// recalcHeight adjusts contentLines and the textarea height to fit the current
// content, clamped to [composeMinLines, composeMaxLines].
func (m ComposeModel) recalcHeight() ComposeModel {
	lines := strings.Count(m.textarea.Value(), "\n") + 1
	newLines := lines
	if newLines < composeMinLines {
		newLines = composeMinLines
	}
	if newLines > composeMaxLines {
		newLines = composeMaxLines
	}
	if newLines != m.contentLines {
		m.contentLines = newLines
		m.textarea.SetHeight(newLines)
	}
	return m
}

// View renders the compose box. Returns an empty string when not active.
func (m ComposeModel) View() string {
	if !m.active {
		return ""
	}
	inner := lipgloss.JoinVertical(lipgloss.Left,
		theme.Subtle.Render(m.context),
		m.textarea.View(),
	)
	boxStyle := theme.ActiveBorder
	if m.width > 2 {
		boxStyle = boxStyle.Width(m.width - 2)
	}
	return boxStyle.Render(inner)
}
