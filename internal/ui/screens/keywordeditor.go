package screens

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
)

// keywordEditorVisibleRows is how many existing keywords are shown at once
// below the pinned add-row — scrolled via up/down when there are more.
const keywordEditorVisibleRows = 6

// KeywordEditorModel is the ctrl+s-to-save popup for editing the keyword
// alerts list (opened from the Settings screen's "alert keywords" row).
// cursor 0 is the pinned add-row; cursor 1..len(keywords) selects
// keywords[cursor-1].
type KeywordEditorModel struct {
	keywords []string
	cursor   int
	offset   int // scroll offset into keywords, for cursor > 0
	input    textinput.Model
}

// NewKeywordEditorModel builds an empty popup; Open seeds it with the
// current list each time it's shown.
func NewKeywordEditorModel() KeywordEditorModel {
	ti := textinput.New()
	ti.Placeholder = "add keyword..."
	ti.CharLimit = 64
	return KeywordEditorModel{input: ti}
}

// Open resets the popup with a fresh copy of existing, cursor on the
// add-row, input focused and empty.
func (m KeywordEditorModel) Open(existing []string) KeywordEditorModel {
	m.keywords = append([]string(nil), existing...)
	m.cursor = 0
	m.offset = 0
	m.input.SetValue("")
	m.input.Focus()
	return m
}

// Keywords returns the popup's current working list, for the caller to
// commit back into Settings on ctrl+s.
func (m KeywordEditorModel) Keywords() []string { return m.keywords }

func (m KeywordEditorModel) ensureVisible() KeywordEditorModel {
	if m.cursor == 0 {
		return m
	}
	idx := m.cursor - 1
	if idx < m.offset {
		m.offset = idx
	} else if idx >= m.offset+keywordEditorVisibleRows {
		m.offset = idx - keywordEditorVisibleRows + 1
	}
	return m
}

// Update handles every key except esc/ctrl+s, which App intercepts itself
// (see App.handleKeywordEditorKey) — same split as PathPromptModel/
// IconPickerModel between model-local keys and App-level dismissal/save.
func (m KeywordEditorModel) Update(msg tea.KeyMsg) (KeywordEditorModel, tea.Cmd) {
	key := msg.String()
	// j/k are vim aliases for down/up only on the keyword list; on the
	// add-row they must reach the input like any other letter.
	if m.cursor > 0 {
		switch key {
		case "k":
			key = "up"
		case "j":
			key = "down"
		}
	}
	switch key {
	case "up":
		if m.cursor > 0 {
			m.cursor--
			m = m.ensureVisible()
		}
		return m, nil
	case "down":
		if m.cursor < len(m.keywords) {
			m.cursor++
			m = m.ensureVisible()
		}
		return m, nil
	case "enter":
		if m.cursor == 0 {
			kw := strings.ToLower(strings.TrimSpace(m.input.Value()))
			if kw != "" {
				dup := false
				for _, existing := range m.keywords {
					if existing == kw {
						dup = true
						break
					}
				}
				if !dup {
					m.keywords = append(m.keywords, kw)
				}
			}
			m.input.SetValue("")
		}
		return m, nil
	case "d", "x":
		if m.cursor > 0 {
			i := m.cursor - 1
			m.keywords = append(m.keywords[:i:i], m.keywords[i+1:]...)
			if m.cursor > len(m.keywords) {
				m.cursor = len(m.keywords)
			}
			m = m.ensureVisible()
			return m, nil
		}
		// cursor == 0: the user is typing a literal "d"/"x" into the add
		// box, not deleting — fall through to the input below.
	}
	if m.cursor == 0 {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

// View renders the popup's own bordered box — self-contained like
// IconPickerModel/PathPromptModel's View(), so both layouts can delegate to
// it directly with no duplicated chrome.
func (m KeywordEditorModel) View() string {
	m.input.TextStyle = theme.Base
	m.input.PlaceholderStyle = theme.Subtle

	title := theme.Title.Render("keyword alerts")

	addPrefix := "  "
	if m.cursor == 0 {
		addPrefix = theme.Highlight.Render("▸ ")
	}
	addRow := addPrefix + theme.Base.Render("add: ") + m.input.View()

	rows := []string{title, "", addRow, ""}

	if len(m.keywords) == 0 {
		rows = append(rows, theme.Subtle.Render("  (no keywords yet)"))
	} else {
		end := min(m.offset+keywordEditorVisibleRows, len(m.keywords))
		for i := m.offset; i < end; i++ {
			if i+1 == m.cursor {
				rows = append(rows, theme.Highlight.Render("▸ "+m.keywords[i]))
			} else {
				rows = append(rows, theme.Subtle.Render("  "+m.keywords[i]))
			}
		}
	}

	hint := theme.Subtle.Render("↑↓ select   enter add   d delete   ctrl+s save   esc cancel")
	rows = append(rows, "", hint)

	return theme.ActiveBorder.Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
}
