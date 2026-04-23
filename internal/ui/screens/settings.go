package screens

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ragnar/cyber-tui/internal/config"
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
			{
				label:   "thread depth",
				kind:    "enum",
				options: []string{"1", "2", "3", "4", "5"},
			},
			{
				label:   "timezone",
				kind:    "enum",
				options: config.AvailableTimezones,
			},
		},
	},
	{
		title: "wander",
		items: []settingsItem{
			{label: "wander mode", kind: "bool"},
		},
	},
}

// SettingsModel is the Settings screen.
type SettingsModel struct {
	settings               model.Settings // live/edited values
	original               model.Settings // last saved baseline
	wanderLust             bool           // live local config value
	originalWanderLust     bool           // last saved baseline for wanderLust
	maxThreadDepth         int            // live local config value (1–5)
	originalMaxThreadDepth int            // last saved baseline
	timezone               string         // live local config value (UTC offset label)
	originalTimezone       string         // last saved baseline
	cursor                 int
	width                  int
	height                 int
	saved                  bool
	err                    error
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
func (m SettingsModel) SetSaved(wanderLust bool, maxThreadDepth int, timezone string) SettingsModel {
	m.saved = true
	m.err = nil
	m.original = m.settings
	m.wanderLust = wanderLust
	m.originalWanderLust = wanderLust
	m.maxThreadDepth = maxThreadDepth
	m.originalMaxThreadDepth = maxThreadDepth
	m.timezone = timezone
	m.originalTimezone = timezone
	return m
}

// SetError sets the error field.
func (m SettingsModel) SetError(err error) SettingsModel {
	m.err = err
	return m
}

// IsDirty returns true if the current settings differ from the last saved baseline.
func (m SettingsModel) IsDirty() bool {
	return !settingsEqual(m.settings, m.original) ||
		m.wanderLust != m.originalWanderLust ||
		m.maxThreadDepth != m.originalMaxThreadDepth ||
		m.timezone != m.originalTimezone
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

// cycleIntEnum cycles a plain int value through a set of string options.
// The current value is matched by its string representation; 0 defaults to options[0].
func cycleIntEnum(cur int, options []string, delta int) int {
	curStr := fmt.Sprintf("%d", cur)
	pos := 0
	for i, o := range options {
		if o == curStr {
			pos = i
			break
		}
	}
	pos = (pos + delta + len(options)) % len(options)
	val := 0
	fmt.Sscanf(options[pos], "%d", &val)
	return val
}

// cycleStringEnum cycles a plain string value through a set of options (wraps around).
func cycleStringEnum(cur string, options []string, delta int) string {
	pos := 0
	for i, o := range options {
		if o == cur {
			pos = i
			break
		}
	}
	pos = (pos + delta + len(options)) % len(options)
	return options[pos]
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
			m.wanderLust = msg.WanderLust
			m.originalWanderLust = msg.WanderLust
			m.maxThreadDepth = msg.MaxThreadDepth
			m.originalMaxThreadDepth = msg.MaxThreadDepth
			tz := msg.Timezone
			if tz == "" {
				tz = "UTC"
			}
			m.timezone = tz
			m.originalTimezone = tz
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
			m.cursor = (m.cursor - 1 + total) % total
			return m, nil

		case "down", "j":
			m.cursor = (m.cursor + 1) % total
			return m, nil

		case " ", "enter": // space (bubbletea KeySpace.String() == " ") or enter
			if m.cursor < total && items[m.cursor].kind == "bool" {
				if m.cursor == 13 {
					m.wanderLust = !m.wanderLust
				} else {
					m.settings = setBool(m.settings, m.cursor, !getBool(m.settings, m.cursor))
				}
				m.saved = false
			}
			return m, nil

		case "tab":
			if m.cursor < total && items[m.cursor].kind == "enum" {
				if m.cursor == 11 {
					m.maxThreadDepth = cycleIntEnum(m.maxThreadDepth, items[m.cursor].options, +1)
				} else if m.cursor == 12 {
					m.timezone = cycleStringEnum(m.timezone, items[m.cursor].options, +1)
				} else {
					m.settings = cycleEnum(m.settings, m.cursor, items[m.cursor].options, +1)
				}
				m.saved = false
			}
			return m, nil

		case "shift+tab":
			if m.cursor < total && items[m.cursor].kind == "enum" {
				if m.cursor == 11 {
					m.maxThreadDepth = cycleIntEnum(m.maxThreadDepth, items[m.cursor].options, -1)
				} else if m.cursor == 12 {
					m.timezone = cycleStringEnum(m.timezone, items[m.cursor].options, -1)
				} else {
					m.settings = cycleEnum(m.settings, m.cursor, items[m.cursor].options, -1)
				}
				m.saved = false
			}
			return m, nil

		case "ctrl+s":
			if m.IsDirty() {
				s := m.settings
				wl := m.wanderLust
				td := m.maxThreadDepth
				tz := m.timezone
				return m, func() tea.Msg {
					return SaveSettingsMsg{Settings: s, WanderLust: wl, MaxThreadDepth: td, Timezone: tz}
				}
			}
			return m, nil

		case "esc":
			// Revert to original.
			m.settings = m.original
			m.wanderLust = m.originalWanderLust
			m.maxThreadDepth = m.originalMaxThreadDepth
			m.timezone = m.originalTimezone
			m.saved = false
			m.err = nil
			return m, nil
		}
	}

	return m, nil
}

// View renders the settings screen.
func (m SettingsModel) View() string {
	// Calculate available height (account for chrome and footer)
	availH := max(3, m.height-theme.ChromeHeight-1)

	var rows []string
	cursorRow := -1
	flatIdx := 0

	for _, g := range settingsGroups {
		rows = append(rows, theme.Title.Render(g.title))

		for _, item := range g.items {
			selected := m.cursor == flatIdx
			if selected {
				cursorRow = len(rows) // track which row the cursor is on
			}

			var cursor, value, label string

			// Cursor marker
			if selected {
				cursor = theme.Highlight.Render("▸ ")
			} else {
				cursor = "  "
			}

			// Value rendering — compute raw text first so the selected row can
			// use plain text with a uniform background (pre-rendered ANSI segments
			// don't inherit an outer background style).
			var rawValue string
			if item.kind == "bool" {
				var boolVal bool
				if flatIdx == 13 {
					boolVal = m.wanderLust
				} else {
					boolVal = getBool(m.settings, flatIdx)
				}
				if boolVal {
					rawValue = "[x]"
					value = theme.Highlight.Render("[x]")
				} else {
					rawValue = "[ ]"
					value = theme.Subtle.Render("[ ]")
				}
			} else {
				var cur string
				if flatIdx == 11 {
					cur = fmt.Sprintf("%d", m.maxThreadDepth)
					if cur == "0" {
						cur = "3"
					}
				} else if flatIdx == 12 {
					cur = m.timezone
					if cur == "" {
						cur = "UTC"
					}
				} else {
					cur = getEnum(m.settings, flatIdx)
				}
				rawValue = "< " + cur + " >"
				value = theme.Highlight.Render(rawValue)
			}

			// Label rendering (highlight if selected)
			labelStyle := theme.Base
			if selected {
				labelStyle = theme.Highlight
			}
			label = labelStyle.Render(item.label)

			// Layout: cursor + label + gap + value, right-aligned
			innerW := max(20, m.width-2) // 2 for cursor prefix
			gap := max(1, innerW-lipgloss.Width(label)-lipgloss.Width(value))
			var line string
			if selected {
				plain := "▸ " + item.label + strings.Repeat(" ", gap) + rawValue
				line = theme.SelectedRow.Width(m.width).Render(plain)
			} else {
				line = cursor + label + strings.Repeat(" ", gap) + value
			}

			rows = append(rows, line)
			flatIdx++
		}

		rows = append(rows, "") // blank line between groups
	}

	// Compute scroll offset to keep cursor visible
	offset := 0
	if cursorRow >= availH {
		offset = cursorRow - availH + 1
	}

	// Slice visible rows
	visible := rows
	if offset > 0 && offset < len(rows) {
		visible = rows[offset:]
	}
	if len(visible) > availH {
		visible = visible[:availH]
	}

	// Add footer
	var footer string
	if m.err != nil {
		footer = theme.Error.Render("error: " + m.err.Error())
	} else if m.saved {
		footer = theme.Highlight.Render("saved!")
	} else if m.IsDirty() {
		footer = theme.Subtle.Render("ctrl+s · save   esc · revert")
	} else {
		footer = theme.Subtle.Render("space/enter · toggle   tab · cycle enum")
	}
	visible = append(visible, footer)

	return lipgloss.JoinVertical(lipgloss.Left, visible...)
}
