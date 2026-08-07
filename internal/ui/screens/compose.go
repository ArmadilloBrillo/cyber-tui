package screens

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
)

// ValidateSlug returns nil for empty slugs (server will generate one) and for
// valid custom slugs. A valid slug is lowercase letters, digits, and hyphens only, max 60 chars.
func ValidateSlug(s string) error {
	if s == "" {
		return nil
	}
	if len([]rune(s)) > 60 {
		return fmt.Errorf("slug too long (max 60 chars)")
	}
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-') {
			return fmt.Errorf("slug: only a-z, 0-9, hyphens allowed")
		}
	}
	return nil
}

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
//   - Ctrl+S submits (emits ComposeSubmitMsg)
//   - Esc cancels (emits ComposeCancelMsg)
type ComposeModel struct {
	textarea     textarea.Model
	context      string // label shown above the editor, e.g. "reply to @username"
	active       bool
	focused      bool // true = active border; false = dimmed border (topics input has focus)
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

// SetFocused controls whether the compose box renders with the active border
// (focused=true) or the dimmed border (focused=false), and blurs/focuses the
// underlying textarea so the cursor hides when another input takes over.
func (m ComposeModel) SetFocused(focused bool) (ComposeModel, tea.Cmd) {
	m.focused = focused
	if focused {
		cmd := m.textarea.Focus()
		return m, cmd
	}
	m.textarea.Blur()
	return m, nil
}

// Open prepares the compose box: sets the context label, clears content,
// resets height to minimum, and focuses the textarea.
// Returns the updated model and a Cmd that starts the cursor blink animation.
func (m ComposeModel) Open(ctx, placeholder string) (ComposeModel, tea.Cmd) {
	m.context = ctx
	m.active = true
	m.focused = true
	m.contentLines = composeMinLines
	m.textarea.Placeholder = placeholder
	m.textarea.SetValue("")
	m.textarea.SetHeight(composeMinLines)
	cmd := m.textarea.Focus()
	return m, cmd
}

// OpenWithContent is like Open but pre-fills the textarea with existing content.
// Use this when editing rather than creating (e.g. bio editing).
func (m ComposeModel) OpenWithContent(ctx, placeholder, content string) (ComposeModel, tea.Cmd) {
	m, cmd := m.Open(ctx, placeholder)
	m.textarea.SetValue(content)
	m = m.recalcHeight() // must assign — value receiver returns updated copy
	return m, cmd
}

// GotoStart moves the textarea cursor to the beginning of the content and
// scrolls the internal viewport to show line 1. Call this after OpenWithContent
// when the editor should open at the top of the note rather than the bottom.
func (m ComposeModel) GotoStart() ComposeModel {
	m.textarea, _ = m.textarea.Update(tea.KeyMsg{Type: tea.KeyCtrlHome})
	return m
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

// SetCharLimit sets the maximum number of characters the textarea will accept.
func (m ComposeModel) SetCharLimit(n int) ComposeModel {
	m.textarea.CharLimit = n
	return m
}

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

	if km, ok := msg.(tea.KeyMsg); ok {
		var keep bool
		km, keep = filterAmbiguousKeyMsg(km)
		if !keep {
			return m, nil
		}
		msg = km
	}

	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "ctrl+s":
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
		growing := newLines > m.contentLines
		m.contentLines = newLines
		m.textarea.SetHeight(newLines)
		// When the editor grows, the new height fits all content —
		// viewport.YOffset must be 0. SetHeight only updates the height; it
		// leaves the internal viewport's YOffset stale from the previous scroll.
		// Re-setting the value resets the viewport to the top (via
		// textarea.Reset → GotoTop) and re-inserts the content with cursor at
		// end. Skipped on shrink to avoid moving the cursor when the user is
		// deleting content mid-text.
		if growing {
			val := m.textarea.Value()
			m.textarea.SetValue(val)
			// SetValue resets viewport.YOffset to 0 (via Reset→GotoTop). When
			// content exceeds the viewport height (e.g. ENTER inserts \n\n,
			// adding 2 lines while the viewport only grows by 1), the cursor
			// lands off-screen. Sending KeyEnd triggers the textarea's internal
			// repositionView(), scrolling the viewport down just enough to show
			// the cursor. It's a no-op for cursor position since the cursor is
			// already at the end of the last line.
			m.textarea, _ = m.textarea.Update(tea.KeyMsg{Type: tea.KeyEnd})
		}
	}
	return m
}

// View renders the compose box. Returns an empty string when not active.
func (m ComposeModel) View() string {
	if !m.active {
		return ""
	}
	m.textarea.FocusedStyle.Text = theme.Base
	m.textarea.FocusedStyle.CursorLine = theme.Base
	m.textarea.BlurredStyle.Text = theme.Base
	m.textarea.BlurredStyle.CursorLine = theme.Base
	m.textarea.FocusedStyle.Placeholder = theme.Subtle
	m.textarea.BlurredStyle.Placeholder = theme.Subtle
	if m.textarea.Focused() {
		_ = (&m.textarea).Focus()
	} else {
		(&m.textarea).Blur()
	}
	inner := lipgloss.JoinVertical(lipgloss.Left,
		theme.Subtle.Render(m.context),
		m.textarea.View(),
	)
	boxStyle := theme.Border
	if m.focused {
		boxStyle = theme.ActiveBorder
	}
	if m.width > 2 {
		boxStyle = boxStyle.Width(m.width - 2)
	}
	return boxStyle.Render(inner)
}

// postField identifies which input has focus inside a PostComposePanel.
type postField int

const (
	postFieldTitle postField = iota
	postFieldSlug
	postFieldBody
	postFieldTopics
	postFieldPublic
	postFieldNSFW
	postFieldCount
)

// PostComposePanel is a unified single-box compose panel for new posts.
// It combines title, body, and topics inputs with public/NSFW toggles
// into a single bordered panel. Tab cycles through all fields; Space
// toggles the public and NSFW checkboxes.
type PostComposePanel struct {
	titleInput  textinput.Model
	slugInput   textinput.Model
	textarea    textarea.Model
	topicsInput textinput.Model
	slugError   string
	isPublic    bool
	isNSFW      bool
	focus       postField
	active      bool
	width       int
	bodyLines   int
}

func NewPostComposePanel(width int) PostComposePanel {
	ti := textinput.New()
	ti.Placeholder = "title (optional)"
	ti.Prompt = "" // the row label acts as the prompt

	sl := textinput.New()
	sl.Placeholder = "slug (optional — a-z 0-9 hyphens, max 60)"
	sl.Prompt = ""
	sl.CharLimit = 60

	ta := textarea.New()
	ta.CharLimit = 32768
	ta.ShowLineNumbers = false
	ta.Placeholder = "what's on your mind…"

	top := textinput.New()
	top.Placeholder = "go, my topic, …  max 3"
	top.Prompt = "" // the row label acts as the prompt

	m := PostComposePanel{
		titleInput:  ti,
		slugInput:   sl,
		textarea:    ta,
		topicsInput: top,
		bodyLines:   composeMinLines,
	}
	return m.SetWidth(width)
}

// Open resets all fields and opens the panel with focus on the title input.
func (m PostComposePanel) Open(defaultPublic bool) (PostComposePanel, tea.Cmd) {
	m.active = true
	m.focus = postFieldTitle
	m.isPublic = defaultPublic
	m.isNSFW = false
	m.titleInput.SetValue("")
	m.slugInput.SetValue("")
	m.slugError = ""
	m.topicsInput.SetValue("")
	m.textarea.SetValue("")
	m.bodyLines = composeMinLines
	m.textarea.SetHeight(composeMinLines)
	m.textarea.Blur()
	m.slugInput.Blur()
	m.topicsInput.Blur()
	return m, m.titleInput.Focus()
}

// Close blurs all inputs and marks the panel inactive.
func (m PostComposePanel) Close() PostComposePanel {
	m.active = false
	m.focus = postFieldTitle
	m.titleInput.SetValue("")
	m.titleInput.Blur()
	m.slugInput.SetValue("")
	m.slugInput.Blur()
	m.slugError = ""
	m.textarea.SetValue("")
	m.textarea.Blur()
	m.topicsInput.Blur()
	return m
}

func (m PostComposePanel) IsActive() bool     { return m.active }
func (m PostComposePanel) Content() string    { return m.textarea.Value() }
func (m PostComposePanel) TitleValue() string { return strings.TrimSpace(m.titleInput.Value()) }
func (m PostComposePanel) SlugValue() string {
	return strings.ToLower(strings.TrimSpace(m.slugInput.Value()))
}
func (m PostComposePanel) TopicsRaw() string  { return m.topicsInput.Value() }
func (m PostComposePanel) IsPublic() bool     { return m.isPublic }
func (m PostComposePanel) IsNSFW() bool       { return m.isNSFW }

// PanelHeight returns the total terminal rows the panel renders:
// 2 (border) + 1 (title row) + 1 (slug row) + 1 (sep) + bodyLines + 1 (sep) + 1 (topics row).
func (m PostComposePanel) PanelHeight() int { return m.bodyLines + 7 }

// SetWidth resizes all inner inputs to fit the new panel width.
func (m PostComposePanel) SetWidth(w int) PostComposePanel {
	m.width = w
	innerW := w - 4 // border chars (2) + horizontal padding (1 each side)
	if innerW < 1 {
		innerW = 1
	}
	const (
		labelW   = 7  // "title  " or "topics " or "slug   "
		togglesW = 22 // "  [x] public  [ ] nsfw"
		cursorW  = 1  // textinput.View() renders Width+1 (cursor always occupies one extra slot)
	)
	titleInputW := innerW - labelW - cursorW
	if titleInputW < 1 {
		titleInputW = 1
	}
	m.titleInput.Width = titleInputW
	m.slugInput.Width = titleInputW

	topicsInputW := innerW - labelW - togglesW - cursorW
	if topicsInputW < 1 {
		topicsInputW = 1
	}
	m.topicsInput.Width = topicsInputW

	m.textarea.SetWidth(innerW)
	return m
}

func (m PostComposePanel) moveFocus(delta int) (PostComposePanel, tea.Cmd) {
	switch m.focus {
	case postFieldTitle:
		m.titleInput.Blur()
	case postFieldSlug:
		m.slugInput.Blur()
	case postFieldBody:
		m.textarea.Blur()
	case postFieldTopics:
		m.topicsInput.Blur()
	}
	m.focus = postField((int(m.focus) + delta + int(postFieldCount)) % int(postFieldCount))
	switch m.focus {
	case postFieldTitle:
		return m, m.titleInput.Focus()
	case postFieldSlug:
		return m, m.slugInput.Focus()
	case postFieldBody:
		return m, m.textarea.Focus()
	case postFieldTopics:
		return m, m.topicsInput.Focus()
	default: // postFieldPublic, postFieldNSFW
		return m, nil
	}
}

func (m PostComposePanel) recalcBodyHeight() PostComposePanel {
	lines := strings.Count(m.textarea.Value(), "\n") + 1
	n := lines
	if n < composeMinLines {
		n = composeMinLines
	}
	if n > composeMaxLines {
		n = composeMaxLines
	}
	if n == m.bodyLines {
		return m
	}
	growing := n > m.bodyLines
	m.bodyLines = n
	m.textarea.SetHeight(n)
	if growing {
		val := m.textarea.Value()
		m.textarea.SetValue(val)
		m.textarea, _ = m.textarea.Update(tea.KeyMsg{Type: tea.KeyEnd})
	}
	return m
}

// Update handles key events and routes them to the focused field.
func (m PostComposePanel) Update(msg tea.Msg) (PostComposePanel, tea.Cmd) {
	if !m.active {
		return m, nil
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "ctrl+s":
			if err := ValidateSlug(m.SlugValue()); err != nil {
				m.slugError = err.Error()
				m.focus = postFieldSlug
				m.titleInput.Blur()
				m.textarea.Blur()
				m.topicsInput.Blur()
				cmd := m.slugInput.Focus()
				return m, cmd
			}
			content := m.textarea.Value()
			return m, func() tea.Msg { return ComposeSubmitMsg{Content: content} }
		case "esc":
			return m, func() tea.Msg { return ComposeCancelMsg{} }
		case "tab":
			return m.moveFocus(1)
		case "shift+tab":
			return m.moveFocus(-1)
		case " ":
			switch m.focus {
			case postFieldPublic:
				m.isPublic = !m.isPublic
				return m, nil
			case postFieldNSFW:
				m.isNSFW = !m.isNSFW
				return m, nil
			}
		}
	}
	switch m.focus {
	case postFieldTitle:
		if km, ok := msg.(tea.KeyMsg); ok {
			filtered, keep := filterAmbiguousKeyMsg(km)
			if !keep {
				return m, nil
			}
			var cmd tea.Cmd
			m.titleInput, cmd = m.titleInput.Update(filtered)
			return m, cmd
		}
		var cmd tea.Cmd
		m.titleInput, cmd = m.titleInput.Update(msg)
		return m, cmd
	case postFieldSlug:
		if km, ok := msg.(tea.KeyMsg); ok {
			filtered, keep := filterSlugCharsKeyMsg(km, "")
			if !keep {
				return m, nil
			}
			var cmd tea.Cmd
			m.slugInput, cmd = m.slugInput.Update(filtered)
			m.slugError = "" // clear error on any edit
			return m, cmd
		}
		var cmd tea.Cmd
		m.slugInput, cmd = m.slugInput.Update(msg)
		return m, cmd
	case postFieldBody:
		if km, ok := msg.(tea.KeyMsg); ok {
			if km.String() == "enter" {
				m.textarea, _ = m.textarea.Update(tea.KeyMsg{Type: tea.KeyEnter})
				m.textarea, _ = m.textarea.Update(tea.KeyMsg{Type: tea.KeyEnter})
				return m.recalcBodyHeight(), nil
			}
			filtered, keep := filterAmbiguousKeyMsg(km)
			if !keep {
				return m, nil
			}
			var cmd tea.Cmd
			m.textarea, cmd = m.textarea.Update(filtered)
			return m.recalcBodyHeight(), cmd
		}
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)
		return m.recalcBodyHeight(), cmd
	case postFieldTopics:
		var cmd tea.Cmd
		m.topicsInput, cmd = updateTopicsInput(m.topicsInput, msg)
		return m, cmd
	}
	// postFieldPublic, postFieldNSFW: no text input, only space handled above.
	return m, nil
}

// View renders the unified compose panel as a single bordered box.
func (m PostComposePanel) View() string {
	if !m.active {
		return ""
	}
	// Re-apply theme styles on the local copy so theme changes are reflected
	// without having to reconstruct the panel.
	m.titleInput.TextStyle = theme.Base
	m.titleInput.PlaceholderStyle = theme.Subtle
	m.slugInput.TextStyle = theme.Base
	m.slugInput.PlaceholderStyle = theme.Subtle
	m.topicsInput.TextStyle = theme.Base
	m.topicsInput.PlaceholderStyle = theme.Subtle
	m.textarea.FocusedStyle.Text = theme.Base
	m.textarea.FocusedStyle.CursorLine = theme.Base
	m.textarea.BlurredStyle.Text = theme.Base
	m.textarea.BlurredStyle.CursorLine = theme.Base
	m.textarea.FocusedStyle.Placeholder = theme.Subtle
	m.textarea.BlurredStyle.Placeholder = theme.Subtle
	// textarea holds an internal *Style pointer set by Focus/Blur pointing at
	// the original stored model's field. Reset it on the local copy so it sees
	// the style changes above.
	if m.textarea.Focused() {
		_ = (&m.textarea).Focus()
	} else {
		(&m.textarea).Blur()
	}

	innerW := m.width - 4
	if innerW < 1 {
		innerW = 1
	}
	sep := theme.Subtle.Render(strings.Repeat("─", innerW))

	active := theme.Base
	inactive := theme.Subtle

	titleStyle := inactive
	if m.focus == postFieldTitle {
		titleStyle = active
	}
	titleRow := titleStyle.Render("title  ") + m.titleInput.View()

	slugStyle := inactive
	if m.focus == postFieldSlug {
		slugStyle = active
	}
	slugRow := slugStyle.Render("slug   ") + m.slugInput.View()
	if m.slugError != "" {
		slugRow += "  " + theme.Error.Render("← "+m.slugError)
	}

	topicsStyle := inactive
	if m.focus == postFieldTopics {
		topicsStyle = active
	}
	pubCheck := "[ ]"
	if m.isPublic {
		pubCheck = "[x]"
	}
	nsfwCheck := "[ ]"
	if m.isNSFW {
		nsfwCheck = "[x]"
	}
	pubStyle := inactive
	if m.focus == postFieldPublic {
		pubStyle = active
	}
	nsfwStyle := inactive
	if m.focus == postFieldNSFW {
		nsfwStyle = active
	}
	topicsRow := topicsStyle.Render("topics ") +
		m.topicsInput.View() +
		"  " + pubStyle.Render(pubCheck+" public") +
		"  " + nsfwStyle.Render(nsfwCheck+" nsfw")

	inner := lipgloss.JoinVertical(lipgloss.Left,
		titleRow,
		slugRow,
		sep,
		m.textarea.View(),
		sep,
		topicsRow,
	)
	boxStyle := theme.ActiveBorder
	if m.width > 2 {
		boxStyle = boxStyle.Width(m.width - 2)
	}
	return boxStyle.Render(inner)
}
