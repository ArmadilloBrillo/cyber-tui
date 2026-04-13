package screens

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
)

// settingsItem describes one editable row.
type settingsItem struct {
	label   string
	kind    string   // "bool" or "enum"
	options []string // populated for kind=="enum"
}

// settingsGroup is a named section of related rows.
type settingsGroup struct {
	title string
	items []settingsItem
}

// settingsGroups is the ordered, static definition of all editable fields.
// Index position is used by get/set helpers — do not reorder.
var settingsGroups = []settingsGroup{
	{
		title: "notifications",
		items: []settingsItem{
			{label: "bookmark alerts", kind: "bool"},
			{label: "reply alerts", kind: "bool"},
			{label: "poke alerts", kind: "bool"},
		},
	},
	{
		title: "content",
		items: []settingsItem{
			{label: "filter nsfw", kind: "bool"},
			{label: "hide images in feed", kind: "bool"},
			{label: "hide audio in feed", kind: "bool"},
		},
	},
	{
		title: "social",
		items: []settingsItem{
			{label: "show follower count", kind: "bool"},
			{label: "auto-watch on reply", kind: "bool"},
			{label: "default public post", kind: "bool"},
		},
	},
	{
		title: "display",
		items: []settingsItem{
			{
				label:   "time format",
				kind:    "enum",
				options: []string{"datetime", "relative", "unix", "swatch"},
			},
			{label: "legacy menu order", kind: "bool"},
		},
	},
}

// SettingsModel is the Settings screen.
type SettingsModel struct {
	settings model.Settings // live/edited values
	original model.Settings // last saved baseline
	cursor   int
	width    int
	height   int
	saved    bool
	err      error
}

// NewSettingsModel creates a new SettingsModel.
func NewSettingsModel() SettingsModel {
	return SettingsModel{}
}

// SetSettings sets both the working settings and the original baseline.
func (m SettingsModel) SetSettings(s model.Settings) SettingsModel {
	m.settings = s
	m.original = s
	m.saved = false
	m.err = nil
	return m
}

// SetSaved marks the current settings as saved and advances the baseline.
func (m SettingsModel) SetSaved() SettingsModel {
	m.saved = true
	m.err = nil
	m.original = m.settings
	return m
}

// SetError sets the error field.
func (m SettingsModel) SetError(err error) SettingsModel {
	m.err = err
	return m
}

// IsDirty returns true if the current settings differ from the last saved baseline.
func (m SettingsModel) IsDirty() bool {
	return !settingsEqual(m.settings, m.original)
}

// settingsEqual compares only the editable scalar fields.
func settingsEqual(a, b model.Settings) bool {
	return a.Notifications == b.Notifications &&
		a.FilterNSFW == b.FilterNSFW &&
		a.HideImagesInFeed == b.HideImagesInFeed &&
		a.HideAudioInFeed == b.HideAudioInFeed &&
		a.ShowFollowerCount == b.ShowFollowerCount &&
		a.AutoWatchOnReply == b.AutoWatchOnReply &&
		a.DefaultPublicPost == b.DefaultPublicPost &&
		a.TimeDisplayFormat == b.TimeDisplayFormat &&
		a.UseLegacyMenuOrder == b.UseLegacyMenuOrder
}

// flatItems returns the flat ordered list of all items across all groups.
func flatItems() []settingsItem {
	var out []settingsItem
	for _, g := range settingsGroups {
		out = append(out, g.items...)
	}
	return out
}

// getBool returns the bool field value for a given flat index.
func getBool(s model.Settings, idx int) bool {
	switch idx {
	case 0:
		return s.Notifications.Bookmark
	case 1:
		return s.Notifications.Reply
	case 2:
		return s.Notifications.Poke
	case 3:
		return s.FilterNSFW
	case 4:
		return s.HideImagesInFeed
	case 5:
		return s.HideAudioInFeed
	case 6:
		return s.ShowFollowerCount
	case 7:
		return s.AutoWatchOnReply
	case 8:
		return s.DefaultPublicPost
	case 10:
		return s.UseLegacyMenuOrder
	}
	return false
}

// setBool sets a bool field for a given flat index.
func setBool(s model.Settings, idx int, v bool) model.Settings {
	switch idx {
	case 0:
		s.Notifications.Bookmark = v
	case 1:
		s.Notifications.Reply = v
	case 2:
		s.Notifications.Poke = v
	case 3:
		s.FilterNSFW = v
	case 4:
		s.HideImagesInFeed = v
	case 5:
		s.HideAudioInFeed = v
	case 6:
		s.ShowFollowerCount = v
	case 7:
		s.AutoWatchOnReply = v
	case 8:
		s.DefaultPublicPost = v
	case 10:
		s.UseLegacyMenuOrder = v
	}
	return s
}

// getEnum returns the enum field value for a given flat index.
func getEnum(s model.Settings, idx int) string {
	if idx == 9 {
		return s.TimeDisplayFormat
	}
	return ""
}

// setEnum sets an enum field for a given flat index.
func setEnum(s model.Settings, idx int, v string) model.Settings {
	if idx == 9 {
		s.TimeDisplayFormat = v
	}
	return s
}

// cycleEnum cycles an enum option by delta (wraps around).
func cycleEnum(s model.Settings, idx int, options []string, delta int) model.Settings {
	cur := getEnum(s, idx)
	pos := 0
	for i, o := range options {
		if o == cur {
			pos = i
			break
		}
	}
	pos = (pos + delta + len(options)) % len(options)
	return setEnum(s, idx, options[pos])
}

// Init initializes the model.
func (m SettingsModel) Init() tea.Cmd {
	return nil
}

// Update handles messages.
func (m SettingsModel) Update(msg tea.Msg) (SettingsModel, tea.Cmd) {
	switch msg := msg.(type) {

	case SharedConfigMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Populate from shared config only on first load (when original is zero).
		// Subsequent broadcasts preserve any unsaved edits.
		if m.original.TimeDisplayFormat == "" && (m.original.Notifications == model.NotificationPrefs{}) {
			m = m.SetSettings(msg.Settings)
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		items := flatItems()
		total := len(items)

		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil

		case "down", "j":
			if m.cursor < total-1 {
				m.cursor++
			}
			return m, nil

		case "space", "enter":
			if m.cursor < total && items[m.cursor].kind == "bool" {
				m.settings = setBool(m.settings, m.cursor, !getBool(m.settings, m.cursor))
				m.saved = false
			}
			return m, nil

		case "left":
			if m.cursor < total && items[m.cursor].kind == "enum" {
				m.settings = cycleEnum(m.settings, m.cursor, items[m.cursor].options, -1)
				m.saved = false
			}
			return m, nil

		case "right":
			if m.cursor < total && items[m.cursor].kind == "enum" {
				m.settings = cycleEnum(m.settings, m.cursor, items[m.cursor].options, +1)
				m.saved = false
			}
			return m, nil

		case "ctrl+s":
			if m.IsDirty() {
				s := m.settings
				return m, func() tea.Msg { return SaveSettingsMsg{Settings: s} }
			}
			return m, nil

		case "esc":
			// Revert to original.
			m.settings = m.original
			m.saved = false
			m.err = nil
			return m, nil
		}
	}

	return m, nil
}

// View renders the settings screen.
func (m SettingsModel) View() string {
	var rows []string
	flatIdx := 0

	for _, g := range settingsGroups {
		rows = append(rows, theme.Title.Render(g.title))

		for _, item := range g.items {
			selected := m.cursor == flatIdx
			var value string

			if item.kind == "bool" {
				if getBool(m.settings, flatIdx) {
					value = theme.Highlight.Render("[x]")
				} else {
					value = theme.Subtle.Render("[ ]")
				}
			} else {
				cur := getEnum(m.settings, flatIdx)
				value = theme.Highlight.Render("< " + cur + " >")
			}

			label := theme.Base.Render(item.label)
			rowStyle := theme.Border
			if selected {
				rowStyle = theme.ActiveBorder
			}

			// Right-align the value within the available width.
			innerW := max(20, m.width-6) // account for border + padding, min 20
			gap := max(1, innerW-lipgloss.Width(label)-lipgloss.Width(value))

			line := label + strings.Repeat(" ", gap) + value
			rows = append(rows, rowStyle.Width(innerW).Render(line))
			flatIdx++
		}

		rows = append(rows, "") // blank line between groups
	}

	// Status footer
	var footer string
	if m.err != nil {
		footer = theme.Error.Render("error: " + m.err.Error())
	} else if m.saved {
		footer = theme.Highlight.Render("saved!")
	} else if m.IsDirty() {
		footer = theme.Subtle.Render("ctrl+s · save   esc · revert")
	} else {
		footer = theme.Subtle.Render("space/enter · toggle   ←→ · cycle enum")
	}
	rows = append(rows, footer)

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}
