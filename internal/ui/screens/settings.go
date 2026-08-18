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
// Each item carries its own typed accessors so that ordering in settingsGroups
// has no impact on correctness — no flat-index arithmetic needed.
type settingsItem struct {
	label   string
	kind    string   // "bool" or "enum"
	options []string // populated for kind=="enum"
	// Bool items: getBool reads, toggle flips.
	getBool func(m SettingsModel) bool
	toggle  func(m SettingsModel) SettingsModel
	// Enum items: getEnum reads, cycle advances by delta (wraps).
	getEnum func(m SettingsModel) string
	cycle   func(m SettingsModel, delta int) SettingsModel
	// showIf, when set, hides the item from the flat list unless it returns true.
	showIf func(m SettingsModel) bool
}

// settingsGroup is a named section of related rows.
type settingsGroup struct {
	title string
	items []settingsItem
}

var settingsGroups = []settingsGroup{
	{
		title: "notifications",
		items: []settingsItem{
			{
				label: "bookmark alerts", kind: "bool",
				getBool: func(m SettingsModel) bool { return m.settings.Notifications.Bookmark },
				toggle: func(m SettingsModel) SettingsModel {
					m.settings.Notifications.Bookmark = !m.settings.Notifications.Bookmark
					return m
				},
			},
			{
				label: "reply alerts", kind: "bool",
				getBool: func(m SettingsModel) bool { return m.settings.Notifications.Reply },
				toggle: func(m SettingsModel) SettingsModel {
					m.settings.Notifications.Reply = !m.settings.Notifications.Reply
					return m
				},
			},
			{
				label: "poke alerts", kind: "bool",
				getBool: func(m SettingsModel) bool { return m.settings.Notifications.Poke },
				toggle: func(m SettingsModel) SettingsModel {
					m.settings.Notifications.Poke = !m.settings.Notifications.Poke
					return m
				},
			},
		},
	},
	{
		title: "content",
		items: []settingsItem{
			{
				label: "filter nsfw", kind: "bool",
				getBool: func(m SettingsModel) bool { return m.settings.FilterNSFW },
				toggle:  func(m SettingsModel) SettingsModel { m.settings.FilterNSFW = !m.settings.FilterNSFW; return m },
			},
		},
	},
	{
		title: "social",
		items: []settingsItem{
			{
				label: "show follower count", kind: "bool",
				getBool: func(m SettingsModel) bool { return m.settings.ShowFollowerCount },
				toggle: func(m SettingsModel) SettingsModel {
					m.settings.ShowFollowerCount = !m.settings.ShowFollowerCount
					return m
				},
			},
			{
				label: "auto-watch on reply", kind: "bool",
				getBool: func(m SettingsModel) bool { return m.settings.AutoWatchOnReply },
				toggle: func(m SettingsModel) SettingsModel {
					m.settings.AutoWatchOnReply = !m.settings.AutoWatchOnReply
					return m
				},
			},
			{
				label: "default public post", kind: "bool",
				getBool: func(m SettingsModel) bool { return m.settings.DefaultPublicPost },
				toggle: func(m SettingsModel) SettingsModel {
					m.settings.DefaultPublicPost = !m.settings.DefaultPublicPost
					return m
				},
			},
		},
	},
	{
		title: "display",
		items: []settingsItem{
			{
				label: "time format", kind: "enum",
				options: []string{"datetime", "relative", "unix", "swatch"},
				getEnum: func(m SettingsModel) string { return m.settings.TimeDisplayFormat },
				cycle: func(m SettingsModel, delta int) SettingsModel {
					m.settings.TimeDisplayFormat = cycleStringEnum(m.settings.TimeDisplayFormat, []string{"datetime", "relative", "unix", "swatch"}, delta)
					return m
				},
			},
			{
				label: "thread depth", kind: "enum",
				options: []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15", "16", "17", "18", "19", "20"},
				getEnum: func(m SettingsModel) string {
					if m.maxThreadDepth == 0 {
						return "3"
					}
					return fmt.Sprintf("%d", m.maxThreadDepth)
				},
				cycle: func(m SettingsModel, delta int) SettingsModel {
					if m.maxThreadDepth == 0 {
						m.maxThreadDepth = 3
					}
					m.maxThreadDepth = cycleIntEnum(m.maxThreadDepth, []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15", "16", "17", "18", "19", "20"}, delta)
					return m
				},
			},
			{
				label: "timezone", kind: "enum",
				options: config.AvailableTimezones,
				getEnum: func(m SettingsModel) string {
					if m.timezone == "" {
						return "UTC"
					}
					return m.timezone
				},
				cycle: func(m SettingsModel, delta int) SettingsModel {
					tz := m.timezone
					if tz == "" {
						tz = "UTC"
					}
					m.timezone = cycleStringEnum(tz, config.AvailableTimezones, delta)
					return m
				},
			},
			{
				label: "image viewer", kind: "enum",
				options: []string{"terminal", "browser"},
				getEnum: func(m SettingsModel) string {
					if m.imageViewer == "browser" {
						return "browser"
					}
					return "terminal"
				},
				cycle: func(m SettingsModel, delta int) SettingsModel {
					cur := m.imageViewer
					if cur == "" {
						cur = "terminal"
					}
					m.imageViewer = cycleStringEnum(cur, []string{"terminal", "browser"}, delta)
					return m
				},
			},
			{
				label: "  graphics protocol", kind: "enum",
				options: []string{"auto", "kitty", "iterm2", "sixel"},
				getEnum: func(m SettingsModel) string {
					if m.graphicsProtocol == "" {
						return "auto"
					}
					return m.graphicsProtocol
				},
				cycle: func(m SettingsModel, delta int) SettingsModel {
					cur := m.graphicsProtocol
					if cur == "" {
						cur = "auto"
					}
					next := cycleStringEnum(cur, []string{"auto", "kitty", "iterm2", "sixel"}, delta)
					if next == "auto" {
						next = ""
					}
					m.graphicsProtocol = next
					return m
				},
				showIf: func(m SettingsModel) bool {
					return m.imageViewer != "browser"
				},
			},
			{
				label: "  inline images (experimental)", kind: "bool",
				getBool: func(m SettingsModel) bool { return m.inlineImages },
				toggle:  func(m SettingsModel) SettingsModel { m.inlineImages = !m.inlineImages; return m },
				showIf: func(m SettingsModel) bool {
					return m.imageViewer != "browser"
				},
			},
			{
				label: "  dithering", kind: "bool",
				getBool: func(m SettingsModel) bool { return m.dithering },
				toggle:  func(m SettingsModel) SettingsModel { m.dithering = !m.dithering; return m },
				showIf: func(m SettingsModel) bool {
					return m.imageViewer != "browser"
				},
			},
			{
				label:   "    sharpness",
				kind:    "enum",
				options: []string{"rough", "medium", "sharp", "crisp"},
				getEnum: func(m SettingsModel) string {
					if m.ditherSharpness == "" {
						return "medium"
					}
					return m.ditherSharpness
				},
				cycle: func(m SettingsModel, delta int) SettingsModel {
					cur := m.ditherSharpness
					if cur == "" {
						cur = "medium"
					}
					m.ditherSharpness = cycleStringEnum(cur, []string{"rough", "medium", "sharp", "crisp"}, delta)
					return m
				},
				showIf: func(m SettingsModel) bool {
					return m.imageViewer != "browser" && m.dithering
				},
			},
		},
	},
	{
		title: "wander",
		items: []settingsItem{
			{
				label: "wander mode", kind: "bool",
				getBool: func(m SettingsModel) bool { return m.wanderLust },
				toggle:  func(m SettingsModel) SettingsModel { m.wanderLust = !m.wanderLust; return m },
			},
		},
	},
}

// SettingsModel is the Settings screen.
type SettingsModel struct {
	settings                 model.Settings // live/edited values
	original                 model.Settings // last saved baseline
	wanderLust               bool           // live local config value
	originalWanderLust       bool           // last saved baseline for wanderLust
	maxThreadDepth           int            // live local config value (1–5)
	originalMaxThreadDepth   int            // last saved baseline
	timezone                 string         // live local config value (UTC offset label)
	originalTimezone         string         // last saved baseline
	imageViewer              string         // live local config value ("terminal" or "browser")
	originalImageViewer      string         // last saved baseline
	graphicsProtocol         string         // live local config value ("" auto, or "kitty"/"iterm2"/"sixel")
	originalGraphicsProtocol string         // last saved baseline
	inlineImages             bool           // live local config value
	originalInlineImages     bool           // last saved baseline
	dithering                bool           // live local config value
	originalDithering        bool           // last saved baseline
	ditherSharpness          string         // live local config value ("rough"/"medium"/"sharp")
	originalDitherSharpness  string         // last saved baseline
	layoutName               string         // live local config value ("tabs" or "miller")
	originalLayoutName       string         // last saved baseline
	cursor                   int
	width                    int
	height                   int
	err                      error
}

// NewSettingsModel creates a new SettingsModel.
func NewSettingsModel() SettingsModel {
	return SettingsModel{}
}

// SetSettings sets both the working settings and the original baseline.
func (m SettingsModel) SetSettings(s model.Settings) SettingsModel {
	m.settings = s
	m.original = s
	m.err = nil
	return m
}

// SetSaved marks the current settings as saved and advances the baseline.
func (m SettingsModel) SetSaved(wanderLust bool, maxThreadDepth int, timezone, imageViewer, graphicsProtocol string, inlineImages bool, dithering bool, ditherSharpness string, layoutName string) SettingsModel {
	m.err = nil
	m.original = m.settings
	m.wanderLust = wanderLust
	m.originalWanderLust = wanderLust
	m.maxThreadDepth = maxThreadDepth
	m.originalMaxThreadDepth = maxThreadDepth
	m.timezone = timezone
	m.originalTimezone = timezone
	m.imageViewer = imageViewer
	m.originalImageViewer = imageViewer
	m.graphicsProtocol = graphicsProtocol
	m.originalGraphicsProtocol = graphicsProtocol
	m.inlineImages = inlineImages
	m.originalInlineImages = inlineImages
	m.dithering = dithering
	m.originalDithering = dithering
	m.ditherSharpness = ditherSharpness
	m.originalDitherSharpness = ditherSharpness
	m.layoutName = layoutName
	m.originalLayoutName = layoutName
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
		m.timezone != m.originalTimezone ||
		m.imageViewer != m.originalImageViewer ||
		m.graphicsProtocol != m.originalGraphicsProtocol ||
		m.inlineImages != m.originalInlineImages ||
		m.dithering != m.originalDithering ||
		m.ditherSharpness != m.originalDitherSharpness ||
		m.layoutName != m.originalLayoutName
}

// settingsEqual compares only the editable scalar fields.
func settingsEqual(a, b model.Settings) bool {
	return a.Notifications == b.Notifications &&
		a.FilterNSFW == b.FilterNSFW &&
		a.ShowFollowerCount == b.ShowFollowerCount &&
		a.AutoWatchOnReply == b.AutoWatchOnReply &&
		a.DefaultPublicPost == b.DefaultPublicPost &&
		a.TimeDisplayFormat == b.TimeDisplayFormat
}

// flatItems returns the flat ordered list of all items across all groups,
// skipping any item whose showIf returns false for m.
func flatItems(m SettingsModel) []settingsItem {
	var out []settingsItem
	for _, g := range settingsGroups {
		for _, item := range g.items {
			if item.showIf != nil && !item.showIf(m) {
				continue
			}
			out = append(out, item)
		}
	}
	return out
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
			iv := msg.ImageViewer
			if iv == "" {
				iv = "terminal"
			}
			m.imageViewer = iv
			m.originalImageViewer = iv
			m.graphicsProtocol = msg.GraphicsProtocol
			m.originalGraphicsProtocol = msg.GraphicsProtocol
			m.inlineImages = msg.InlineImages
			m.originalInlineImages = msg.InlineImages
			m.dithering = msg.Dithering
			m.originalDithering = msg.Dithering
			m.ditherSharpness = msg.DitherSharpness
			m.originalDitherSharpness = msg.DitherSharpness
			m.layoutName = msg.LayoutName
			m.originalLayoutName = msg.LayoutName
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		items := flatItems(m)
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
				m = items[m.cursor].toggle(m)
			}
			return m, nil

		case "tab":
			if m.cursor < total && items[m.cursor].kind == "enum" {
				m = items[m.cursor].cycle(m, +1)
				m.cursor = min(m.cursor, len(flatItems(m))-1)
			}
			return m, nil

		case "shift+tab":
			if m.cursor < total && items[m.cursor].kind == "enum" {
				m = items[m.cursor].cycle(m, -1)
				m.cursor = min(m.cursor, len(flatItems(m))-1)
			}
			return m, nil

		case "ctrl+s":
			if m.IsDirty() {
				s := m.settings
				wl := m.wanderLust
				td := m.maxThreadDepth
				tz := m.timezone
				iv := m.imageViewer
				gp := m.graphicsProtocol
				ii := m.inlineImages
				dt := m.dithering
				ds := m.ditherSharpness
				ln := m.layoutName
				remoteChanged := !settingsEqual(m.settings, m.original)
				return m, func() tea.Msg {
					return SaveSettingsMsg{Settings: s, WanderLust: wl, MaxThreadDepth: td, Timezone: tz, ImageViewer: iv, GraphicsProtocol: gp, InlineImages: ii, Dithering: dt, DitherSharpness: ds, LayoutName: ln, RemoteChanged: remoteChanged}
				}
			}
			return m, nil

		case "esc":
			// Revert to original.
			m.settings = m.original
			m.wanderLust = m.originalWanderLust
			m.maxThreadDepth = m.originalMaxThreadDepth
			m.timezone = m.originalTimezone
			m.imageViewer = m.originalImageViewer
			m.graphicsProtocol = m.originalGraphicsProtocol
			m.inlineImages = m.originalInlineImages
			m.dithering = m.originalDithering
			m.ditherSharpness = m.originalDitherSharpness
			m.layoutName = m.originalLayoutName
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
			if item.showIf != nil && !item.showIf(m) {
				continue
			}
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
				if item.getBool(m) {
					rawValue = "[x]"
					value = theme.Highlight.Render("[x]")
				} else {
					rawValue = "[ ]"
					value = theme.Subtle.Render("[ ]")
				}
			} else {
				cur := item.getEnum(m)
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
	} else if m.IsDirty() {
		footer = theme.Subtle.Render("ctrl+s · save   esc · revert")
	} else {
		footer = theme.Subtle.Render("space/enter · toggle   tab · cycle enum")
	}
	visible = append(visible, footer)

	return lipgloss.JoinVertical(lipgloss.Left, visible...)
}
