package screens

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ragnar/cyber-tui/internal/emoji"
	"github.com/ragnar/cyber-tui/internal/ui/markdown"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
)

// InsertIconMsg is emitted when the user picks an icon from the icon picker
// (Ctrl+]). It's a plain screen message, not consumed by App directly — it
// falls through to delegateScreenUpdate, which routes it to whichever
// screen is currently active.
type InsertIconMsg struct{ Icon string }

// iconPickerChrome is the number of non-list lines the picker always
// renders (title, blank, tab row, blank, search box, blank, blank, hint)
// plus the border's top/bottom lines — subtracted from the height budget
// SetSize is given to compute how many list rows actually fit.
const iconPickerChrome = 10

type iconRowKind int

const (
	iconRowHeader iconRowKind = iota
	iconRowIcon
)

type iconRow struct {
	kind  iconRowKind
	label string     // header text, valid for kind == iconRowHeader
	icon  emoji.Icon // valid for kind == iconRowIcon
}

// IconPickerModel is the Ctrl+] icon picker: tabs between icon sets (emoji,
// kaomoji), a live search box, and a scrollable list grouped into
// "Common X" / "All X" sections.
type IconPickerModel struct {
	sets     []emoji.Set
	tab      int
	query    textinput.Model
	rows     []iconRow
	selected int
	offset   int // index of the first visible row
	width    int // content width list rows are truncated to
	height   int // number of visible list rows
}

// NewIconPickerModel builds the picker over the emoji and kaomoji sets.
func NewIconPickerModel() IconPickerModel {
	ti := textinput.New()
	ti.Prompt = "" // the "search ›" label acts as the prompt
	ti.Width = 30
	m := IconPickerModel{
		sets:  []emoji.Set{emoji.Emoji, emoji.Kaomoji},
		query: ti,
	}
	return m.refiltered()
}

// Open resets the picker to a blank query on the first tab and focuses the
// search box. Returns a Cmd that starts the cursor blink, same as other
// modals' Open methods (e.g. ComposeModel.Open).
func (m IconPickerModel) Open() (IconPickerModel, tea.Cmd) {
	m.tab = 0
	m.query.SetValue("")
	cmd := m.query.Focus()
	m = m.refiltered()
	return m, cmd
}

// SetSize fits the picker within a maxWidth x maxHeight budget (the same
// clamp App applies to other modals).
func (m IconPickerModel) SetSize(maxWidth, maxHeight int) IconPickerModel {
	m.width = maxWidth - 4 // border + padding
	if m.width < 20 {
		m.width = 20
	}
	if m.width > 70 {
		m.width = 70
	}
	m.height = maxHeight - iconPickerChrome
	if m.height < 3 {
		m.height = 3
	}
	return m.ensureVisible()
}

func (m IconPickerModel) activeSet() emoji.Set { return m.sets[m.tab] }

// refiltered rebuilds rows from the current tab + query: a "Common X"
// section (query empty only — a partially-filtered common list reads oddly
// once most of it no longer matches) followed by "All X", each icon row
// kept only when its name contains the query.
func (m IconPickerModel) refiltered() IconPickerModel {
	set := m.activeSet()
	query := strings.ToLower(strings.TrimSpace(m.query.Value()))

	matching := func(icons []emoji.Icon) []emoji.Icon {
		if query == "" {
			return icons
		}
		var out []emoji.Icon
		for _, ic := range icons {
			if strings.Contains(strings.ToLower(ic.Name), query) {
				out = append(out, ic)
			}
		}
		return out
	}
	addSection := func(rows []iconRow, label string, icons []emoji.Icon) []iconRow {
		if len(icons) == 0 {
			return rows
		}
		rows = append(rows, iconRow{kind: iconRowHeader, label: label})
		for _, ic := range icons {
			rows = append(rows, iconRow{kind: iconRowIcon, icon: ic})
		}
		return rows
	}

	var rows []iconRow
	if query == "" {
		var common []emoji.Icon
		for _, ic := range set.Icons {
			if set.CommonNames[ic.Name] {
				common = append(common, ic)
			}
		}
		rows = addSection(rows, "Common "+set.Label, common)
	}
	rows = addSection(rows, "All "+set.Label, matching(set.Icons))

	m.rows = rows
	m.selected = 0
	for m.selected < len(m.rows) && m.rows[m.selected].kind == iconRowHeader {
		m.selected++
	}
	m.offset = 0
	return m
}

// moveSelection shifts the selection by delta rows, skipping over header
// rows, and scrolls just enough to keep it visible.
func (m IconPickerModel) moveSelection(delta int) IconPickerModel {
	next := m.selected + delta
	for next >= 0 && next < len(m.rows) && m.rows[next].kind == iconRowHeader {
		next += delta
	}
	if next < 0 || next >= len(m.rows) || m.rows[next].kind == iconRowHeader {
		return m
	}
	m.selected = next
	return m.ensureVisible()
}

func (m IconPickerModel) ensureVisible() IconPickerModel {
	if m.height <= 0 {
		return m
	}
	if m.selected < m.offset {
		m.offset = m.selected
	} else if m.selected >= m.offset+m.height {
		m.offset = m.selected - m.height + 1
	}
	return m
}

func (m IconPickerModel) switchTab(delta int) IconPickerModel {
	n := len(m.sets)
	m.tab = ((m.tab+delta)%n + n) % n
	return m.refiltered()
}

// Selected returns the glyph of the currently selected row, if any is
// selected and it's an icon (not a header, and not an empty result list).
func (m IconPickerModel) Selected() (string, bool) {
	if m.selected < 0 || m.selected >= len(m.rows) || m.rows[m.selected].kind != iconRowIcon {
		return "", false
	}
	return m.rows[m.selected].icon.Glyph, true
}

// Update handles every key except esc/enter, which App intercepts itself
// (see App.handleIconPickerKey) — mirrors PathPromptModel/ComposeModel's
// split between model-local keys and App-level dismissal/submission.
func (m IconPickerModel) Update(msg tea.KeyMsg) (IconPickerModel, tea.Cmd) {
	switch msg.String() {
	case "tab":
		return m.switchTab(1), nil
	case "shift+tab":
		return m.switchTab(-1), nil
	case "up":
		return m.moveSelection(-1), nil
	case "down":
		return m.moveSelection(1), nil
	}
	if km, ok := filterAmbiguousKeyMsg(msg); ok {
		msg = km
	}
	var cmd tea.Cmd
	m.query, cmd = m.query.Update(msg)
	m = m.refiltered()
	return m, cmd
}

func (m IconPickerModel) View() string {
	title := theme.Title.Render("Icon Picker")

	tabParts := make([]string, len(m.sets))
	for i, set := range m.sets {
		mark, style := "[ ]", theme.Subtle
		if i == m.tab {
			mark, style = "[•]", theme.Highlight
		}
		tabParts[i] = style.Render(mark + " " + strings.ToLower(set.Label))
	}
	tabRow := strings.Join(tabParts, "  ")

	search := theme.Subtle.Render("search › ") + m.query.View()

	var lines []string
	end := min(m.offset+m.height, len(m.rows))
	for i := m.offset; i < end; i++ {
		row := m.rows[i]
		switch row.kind {
		case iconRowHeader:
			lines = append(lines, theme.Subtle.Bold(true).Render(markdown.TruncateToWidth(row.label, m.width)))
		case iconRowIcon:
			// Truncate only the name, never the glyph: names are plain text
			// (safe for rune-based truncation) while glyphs are always
			// filtered to a handful of display columns by emoji.safeWidth —
			// truncating the two as one combined string risked slicing a
			// glyph's own codepoints when a long CLDR name pushed the cutoff
			// back into it.
			nameWidth := m.width - 2 - lipgloss.Width(row.icon.Glyph) - 2
			line := row.icon.Glyph + "  " + markdown.TruncateToWidth(row.icon.Name, nameWidth)
			if i == m.selected {
				lines = append(lines, theme.Highlight.Render("▸ "+line))
			} else {
				lines = append(lines, theme.Subtle.Render("  "+line))
			}
		}
	}
	if len(m.rows) == 0 {
		lines = append(lines, theme.Subtle.Render("  no matches"))
	}

	hint := theme.Subtle.Render("↑↓ select   tab switch   enter insert   esc close")

	body := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		tabRow,
		"",
		search,
		"",
		lipgloss.JoinVertical(lipgloss.Left, lines...),
		"",
		hint,
	)
	return theme.ActiveBorder.Render(body)
}
