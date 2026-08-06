package screens

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
)

// isAlnum/upperRune are deliberately ASCII-only (not unicode.IsLetter) — hex
// digits are always ASCII, and keeping the buffer single-byte-per-rune keeps
// renderBuffer's cursor-position math trivial.
func isAlnum(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func upperRune(r rune) rune {
	if r >= 'a' && r <= 'z' {
		return r - 'a' + 'A'
	}
	return r
}

// themeEditorField binds one row of the editor to a Palette field via typed
// accessors, so field order in themeEditorFields has no impact on correctness.
type themeEditorField struct {
	label string
	get   func(p theme.Palette) string
	set   func(p *theme.Palette, v string)
}

// themeEditorFields lists only the palette roles the TUI actually renders.
// Palette.Background/CodeBackground are reserved for a future post-import
// feature and have no visible effect here, so they're deliberately excluded
// from the editor — showing an editable field that does nothing would be the
// exact confusion this labeling is meant to fix.
var themeEditorFields = []themeEditorField{
	{"foreground", func(p theme.Palette) string { return p.Foreground }, func(p *theme.Palette, v string) { p.Foreground = v }},
	{"border", func(p theme.Palette) string { return p.Border }, func(p *theme.Palette, v string) { p.Border = v }},
	{"accent", func(p theme.Palette) string { return p.Accent }, func(p *theme.Palette, v string) { p.Accent = v }},
	{"highlight", func(p theme.Palette) string { return p.Highlight }, func(p *theme.Palette, v string) { p.Highlight = v }},
	{"error", func(p theme.Palette) string { return p.Error }, func(p *theme.Palette, v string) { p.Error = v }},
	{"bar text", func(p theme.Palette) string { return p.BarText }, func(p *theme.Palette, v string) { p.BarText = v }},
	{"dimmed", func(p theme.Palette) string { return p.Dimmed }, func(p *theme.Palette, v string) { p.Dimmed = v }},
	{"self", func(p theme.Palette) string { return p.Self }, func(p *theme.Palette, v string) { p.Self = v }},
	{"meta", func(p theme.Palette) string { return p.Meta }, func(p *theme.Palette, v string) { p.Meta = v }},
}

// hexDigits is the fixed width of each row's editable buffer — 6 hex digits,
// the "#" itself is a fixed prompt and never part of the buffer.
const hexDigits = 6

// ThemeEditorModel is the "custom" theme color editor, opened as a modal
// overlay from the theme picker. Each row is a fixed 6-character hex buffer
// edited in overwrite mode (like a masked input): typing a character
// replaces whatever is under the cursor and advances to the next slot, and
// the cursor can't move past either end of the buffer.
type ThemeEditorModel struct {
	values        [9]string // each exactly hexDigits runes; ' ' marks an unset slot
	cursor        int       // row cursor, 0..len(themeEditorFields)-1
	editing       bool      // true while values[cursor] is being edited
	charCursor    int       // position within values[cursor], 0..hexDigits-1
	original      theme.Palette
	width, height int
	saved         bool
	err           string
}

// padHex uppercases s and pads/truncates it to exactly hexDigits runes,
// using ' ' for any missing trailing digits.
func padHex(s string) string {
	r := []rune(strings.ToUpper(s))
	if len(r) > hexDigits {
		r = r[:hexDigits]
	}
	for len(r) < hexDigits {
		r = append(r, ' ')
	}
	return string(r)
}

// NewThemeEditorModel creates an editor prefilled with p (either the saved
// custom palette, or the currently active theme's colors if none is saved yet).
func NewThemeEditorModel(p theme.Palette) ThemeEditorModel {
	// Normalize the 8 editable fields to uppercase up front, so the initial
	// values (uppercased) match m.original exactly and IsDirty doesn't fire
	// on a palette that was saved in lowercase.
	for _, f := range themeEditorFields {
		f.set(&p, strings.ToUpper(f.get(p)))
	}
	m := ThemeEditorModel{original: p}
	for i, f := range themeEditorFields {
		m.values[i] = padHex(strings.TrimPrefix(f.get(p), "#"))
	}
	return m
}

// hexValue returns row i's value as a full "#RRGGBB" string. Unset slots
// (' ') make this fail ValidHex, exactly like a partially-typed field should.
func (m ThemeEditorModel) hexValue(i int) string { return "#" + m.values[i] }

// setChar overwrites the rune at pos in row's buffer.
func (m *ThemeEditorModel) setChar(row, pos int, ch rune) {
	r := []rune(m.values[row])
	r[pos] = ch
	m.values[row] = string(r)
}

// currentPalette builds a Palette from the raw (possibly invalid, mid-edit)
// buffer values, starting from m.original so the reserved Background/
// CodeBackground fields (not shown as editor rows) pass through unchanged.
func (m ThemeEditorModel) currentPalette() theme.Palette {
	p := m.original
	for i, f := range themeEditorFields {
		f.set(&p, m.hexValue(i))
	}
	return p
}

// livePreviewPalette is like currentPalette but substitutes any field that
// isn't currently well-formed hex (e.g. mid-edit, unset slots) with its
// last-known-good value, so partial input never reaches lipgloss as a
// malformed color.
func (m ThemeEditorModel) livePreviewPalette() theme.Palette {
	p := m.currentPalette()
	for i, f := range themeEditorFields {
		if !theme.ValidHex(m.hexValue(i)) {
			f.set(&p, f.get(m.original))
		}
	}
	return p
}

// IsDirty reports whether any field differs from the last saved baseline.
func (m ThemeEditorModel) IsDirty() bool { return m.currentPalette() != m.original }

// IsValid reports whether every field is currently well-formed hex.
func (m ThemeEditorModel) IsValid() bool { return m.currentPalette().Valid() }

// SetSaved advances the baseline after a successful save.
func (m ThemeEditorModel) SetSaved(p theme.Palette) ThemeEditorModel {
	m.original = p
	m.saved = true
	m.err = ""
	return m
}

func (m ThemeEditorModel) Init() tea.Cmd { return nil }

func (m ThemeEditorModel) Update(msg tea.Msg) (ThemeEditorModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.KeyMsg:
		// ctrl+s always attempts to save, whether or not a field is
		// currently focused — currentPalette() already reads the live
		// buffer, so there's nothing to "flush" first. Checked ahead of the
		// editing/nav split so it can't be silently swallowed mid-edit.
		if msg.String() == "ctrl+s" {
			m.editing = false
			if !m.IsValid() {
				m.err = "all colors must be #RRGGBB"
				return m, nil
			}
			p := m.currentPalette()
			return m, func() tea.Msg { return SaveThemeMsg{Palette: p} }
		}

		if m.editing {
			return m.updateEditingKey(msg)
		}

		switch msg.String() {
		case "up", "k":
			m.cursor = (m.cursor - 1 + len(themeEditorFields)) % len(themeEditorFields)
		case "down", "j":
			m.cursor = (m.cursor + 1) % len(themeEditorFields)
		case "enter":
			m.editing = true
			m.charCursor = 0
			return m, nil
		case "esc":
			return m, func() tea.Msg { return CloseThemeEditorMsg{} }
		}
	}
	return m, nil
}

// updateEditingKey handles keystrokes while a row's hex buffer is focused:
// left/right move the cursor within the 6 slots, backspace clears and steps
// back, a typed alphanumeric rune overwrites the current slot (uppercased)
// and advances — except on the last slot, which stays put since there's
// nowhere further to advance to. enter/esc commit the field and return to
// row navigation.
func (m ThemeEditorModel) updateEditingKey(msg tea.KeyMsg) (ThemeEditorModel, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter, tea.KeyEsc:
		m.editing = false
		return m, nil
	case tea.KeyLeft:
		if m.charCursor > 0 {
			m.charCursor--
		}
		return m, nil
	case tea.KeyRight:
		if m.charCursor < hexDigits-1 {
			m.charCursor++
		}
		return m, nil
	case tea.KeyBackspace:
		if m.charCursor > 0 {
			m.charCursor--
		}
		m.setChar(m.cursor, m.charCursor, ' ')
		return m.previewCmd()
	case tea.KeyRunes:
		if len(msg.Runes) != 1 || !isAlnum(msg.Runes[0]) {
			return m, nil
		}
		m.setChar(m.cursor, m.charCursor, upperRune(msg.Runes[0]))
		if m.charCursor < hexDigits-1 {
			m.charCursor++
		}
		return m.previewCmd()
	}
	return m, nil
}

// previewCmd clears transient save state and emits a PreviewPaletteMsg for
// the buffer's current (possibly still mid-edit) contents.
func (m ThemeEditorModel) previewCmd() (ThemeEditorModel, tea.Cmd) {
	m.saved = false
	m.err = ""
	p := m.livePreviewPalette()
	return m, func() tea.Msg { return PreviewPaletteMsg{Palette: p} }
}

func (m ThemeEditorModel) View() string {
	var rows []string
	rows = append(rows, theme.Title.Render("custom theme"))

	for i, f := range themeEditorFields {
		selected := m.cursor == i
		var cursor string
		if selected {
			cursor = theme.Highlight.Render("▸ ")
		} else {
			cursor = "  "
		}

		labelStyle := theme.Base
		if selected {
			labelStyle = theme.Highlight
		}
		label := labelStyle.Render(f.label)

		val := m.hexValue(i)
		var swatch string
		if theme.ValidHex(val) {
			swatch = lipgloss.NewStyle().Background(lipgloss.Color(val)).Render("  ")
		} else {
			swatch = "  "
		}

		var value string
		if selected && m.editing {
			value = "#" + m.renderBuffer(i)
		} else {
			value = theme.Subtle.Render(val)
		}

		innerW := max(24, m.width-2)
		content := swatch + " " + value
		gap := max(1, innerW-lipgloss.Width(label)-lipgloss.Width(content))
		rows = append(rows, cursor+label+strings.Repeat(" ", gap)+content)
	}

	var footer string
	switch {
	case m.err != "":
		footer = theme.Error.Render("error: " + m.err)
	case m.saved:
		footer = theme.Highlight.Render("saved!")
	case m.editing:
		footer = theme.Subtle.Render("type · overwrite & advance   ←→ · move   ctrl+s · save   enter/esc · commit")
	default:
		footer = theme.Subtle.Render("enter · edit field   j/k · move   ctrl+s · save   esc · close")
	}
	rows = append(rows, "", footer)

	return theme.ActiveBorder.Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
}

// renderBuffer renders row i's 6-slot hex buffer with the overwrite cursor
// highlighted at m.charCursor. Unset slots (' ') show as "_" so the empty
// positions are visible instead of collapsing to blank space.
func (m ThemeEditorModel) renderBuffer(i int) string {
	var sb strings.Builder
	for pos, ch := range []rune(m.values[i]) {
		display := string(ch)
		if ch == ' ' {
			display = "_"
		}
		style := theme.Base
		if pos == m.charCursor {
			style = theme.SelectedRow
		}
		sb.WriteString(style.Render(display))
	}
	return sb.String()
}
