package screens

import (
	"fmt"
	"reflect"
	"slices"
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
	kind    string   // "bool", "enum", "keywordlist" or "keybind"
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
		title: "alerts",
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
			{
				// Words/phrases that raise a Notifications-tab entry (and,
				// subject to the desktop-notifications toggle above, an OSC 9
				// toast) wherever content is scanned — cIRC, C-Mail, posts,
				// and post topics/tags. Edited in place (kind
				// "keywordlist" has its own key handling in Update/View,
				// there being only one such row, unlike bool/enum's
				// getter/setter closures).
				label: "alert keywords", kind: "keywordlist",
			},
			{
				// OS desktop notification (OSC 9 escape) for new C-Mail /
				// activity while backgrounded — see
				// docs/53-desktop-notifications.md. Silently does nothing on
				// terminals without OSC 9 support.
				label: "desktop notifications (OSC 9)", kind: "bool",
				getBool: func(m SettingsModel) bool { return m.local.DesktopNotifications },
				toggle: func(m SettingsModel) SettingsModel {
					m.local.DesktopNotifications = !m.local.DesktopNotifications
					return m
				},
			},
		},
	},
	{
		title: "feed & posts",
		items: []settingsItem{
			{
				label: "filter nsfw", kind: "bool",
				getBool: func(m SettingsModel) bool { return m.settings.FilterNSFW },
				toggle:  func(m SettingsModel) SettingsModel { m.settings.FilterNSFW = !m.settings.FilterNSFW; return m },
			},
			{
				label: "show guild posts in feed", kind: "bool",
				getBool: func(m SettingsModel) bool { return m.settings.ShowGuildPostsInFeed },
				toggle: func(m SettingsModel) SettingsModel {
					m.settings.ShowGuildPostsInFeed = !m.settings.ShowGuildPostsInFeed
					return m
				},
			},
			{
				// Internally tracked as feedManualRefreshOnly (true = off);
				// inverted here so "on" reads as the enabled state — see
				// docs/39-feed-background-poll.md.
				label: "auto-refresh (background poll)", kind: "bool",
				getBool: func(m SettingsModel) bool { return !m.local.FeedManualRefreshOnly },
				toggle: func(m SettingsModel) SettingsModel {
					m.local.FeedManualRefreshOnly = !m.local.FeedManualRefreshOnly
					return m
				},
			},
			{
				label: "always reveal content warnings", kind: "bool",
				getBool: func(m SettingsModel) bool { return m.local.RevealContentWarnings },
				toggle: func(m SettingsModel) SettingsModel {
					m.local.RevealContentWarnings = !m.local.RevealContentWarnings
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
			{
				label: "auto-watch on reply", kind: "bool",
				getBool: func(m SettingsModel) bool { return m.settings.AutoWatchOnReply },
				toggle: func(m SettingsModel) SettingsModel {
					m.settings.AutoWatchOnReply = !m.settings.AutoWatchOnReply
					return m
				},
			},
			{
				label: "show follower count", kind: "bool",
				getBool: func(m SettingsModel) bool { return m.settings.ShowFollowerCount },
				toggle: func(m SettingsModel) SettingsModel {
					m.settings.ShowFollowerCount = !m.settings.ShowFollowerCount
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
				label: "timezone", kind: "enum",
				options: config.AvailableTimezones,
				getEnum: func(m SettingsModel) string {
					if m.local.Timezone == "" {
						return "UTC"
					}
					return m.local.Timezone
				},
				cycle: func(m SettingsModel, delta int) SettingsModel {
					tz := m.local.Timezone
					if tz == "" {
						tz = "UTC"
					}
					m.local.Timezone = cycleStringEnum(tz, config.AvailableTimezones, delta)
					return m
				},
			},
			{
				label: "thread depth", kind: "enum",
				options: []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15", "16", "17", "18", "19", "20"},
				getEnum: func(m SettingsModel) string {
					if m.local.MaxThreadDepth == 0 {
						return "3"
					}
					return fmt.Sprintf("%d", m.local.MaxThreadDepth)
				},
				cycle: func(m SettingsModel, delta int) SettingsModel {
					if m.local.MaxThreadDepth == 0 {
						m.local.MaxThreadDepth = 3
					}
					m.local.MaxThreadDepth = cycleIntEnum(m.local.MaxThreadDepth, []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15", "16", "17", "18", "19", "20"}, delta)
					return m
				},
			},
			{
				label: "color nicknames", kind: "bool",
				getBool: func(m SettingsModel) bool { return m.settings.ColorNicknames },
				toggle: func(m SettingsModel) SettingsModel {
					m.settings.ColorNicknames = !m.settings.ColorNicknames
					return m
				},
			},
			{
				label: "show globe tab", kind: "bool",
				getBool: func(m SettingsModel) bool { return m.local.ShowGlobeTab },
				toggle: func(m SettingsModel) SettingsModel {
					m.local.ShowGlobeTab = !m.local.ShowGlobeTab
					return m
				},
			},
		},
	},
	{
		title: "images",
		items: []settingsItem{
			{
				label: "image viewer", kind: "enum",
				options: []string{"terminal", "browser"},
				getEnum: func(m SettingsModel) string {
					if m.local.ImageViewer == "browser" {
						return "browser"
					}
					return "terminal"
				},
				cycle: func(m SettingsModel, delta int) SettingsModel {
					cur := m.local.ImageViewer
					if cur == "" {
						cur = "terminal"
					}
					m.local.ImageViewer = cycleStringEnum(cur, []string{"terminal", "browser"}, delta)
					return m
				},
			},
			{
				label: "  graphics protocol", kind: "enum",
				options: []string{"auto", "kitty", "iterm2", "sixel"},
				getEnum: func(m SettingsModel) string {
					if m.local.GraphicsProtocol == "" {
						return "auto"
					}
					return m.local.GraphicsProtocol
				},
				cycle: func(m SettingsModel, delta int) SettingsModel {
					cur := m.local.GraphicsProtocol
					if cur == "" {
						cur = "auto"
					}
					next := cycleStringEnum(cur, []string{"auto", "kitty", "iterm2", "sixel"}, delta)
					if next == "auto" {
						next = ""
					}
					m.local.GraphicsProtocol = next
					return m
				},
				showIf: func(m SettingsModel) bool {
					return m.local.ImageViewer != "browser"
				},
			},
			{
				label: "  inline images (experimental)", kind: "bool",
				getBool: func(m SettingsModel) bool { return m.local.InlineImages },
				toggle:  func(m SettingsModel) SettingsModel { m.local.InlineImages = !m.local.InlineImages; return m },
				showIf: func(m SettingsModel) bool {
					return m.local.ImageViewer != "browser"
				},
			},
			{
				label: "  dithering", kind: "bool",
				getBool: func(m SettingsModel) bool { return m.local.Dithering },
				toggle:  func(m SettingsModel) SettingsModel { m.local.Dithering = !m.local.Dithering; return m },
				showIf: func(m SettingsModel) bool {
					return m.local.ImageViewer != "browser"
				},
			},
			{
				label:   "    sharpness",
				kind:    "enum",
				options: []string{"rough", "medium", "sharp", "crisp"},
				getEnum: func(m SettingsModel) string {
					if m.settings.ImagePixelSize == "" {
						return "medium"
					}
					return m.settings.ImagePixelSize
				},
				cycle: func(m SettingsModel, delta int) SettingsModel {
					cur := m.settings.ImagePixelSize
					if cur == "" {
						cur = "medium"
					}
					m.settings.ImagePixelSize = cycleStringEnum(cur, []string{"rough", "medium", "sharp", "crisp"}, delta)
					return m
				},
				showIf: func(m SettingsModel) bool {
					return m.local.ImageViewer != "browser" && m.local.Dithering
				},
			},
			{
				label:   "    color",
				kind:    "enum",
				options: []string{"monochrome", "color"},
				getEnum: func(m SettingsModel) string {
					if m.settings.ImageColor {
						return "color"
					}
					return "monochrome"
				},
				cycle: func(m SettingsModel, delta int) SettingsModel {
					m.settings.ImageColor = !m.settings.ImageColor
					return m
				},
				showIf: func(m SettingsModel) bool {
					return m.local.ImageViewer != "browser" && m.local.Dithering
				},
			},
		},
	},
	{
		title: "chat & compose",
		items: []settingsItem{
			{
				// The key that inserts a line break: a markdown hard break
				// in posts and replies, a plain newline in cIRC and C-Mail.
				// For terminals that swallow the default alt+enter. Captured
				// rather than typed: kind "keybind" has its own key handling
				// in Update/handleCapture and View, like "keywordlist".
				label: "line break key", kind: "keybind",
			},
			{
				label: "typing indicators", kind: "bool",
				getBool: func(m SettingsModel) bool { return m.local.TypingIndicatorsEnabled },
				toggle: func(m SettingsModel) SettingsModel {
					m.local.TypingIndicatorsEnabled = !m.local.TypingIndicatorsEnabled
					return m
				},
			},
		},
	},
	{
		title: "advanced",
		items: []settingsItem{
			{
				label: "wander mode", kind: "bool",
				getBool: func(m SettingsModel) bool { return m.local.WanderLust },
				toggle:  func(m SettingsModel) SettingsModel { m.local.WanderLust = !m.local.WanderLust; return m },
			},
			{
				label: "debug log (on restart)", kind: "bool",
				getBool: func(m SettingsModel) bool { return m.local.DebugLog },
				toggle:  func(m SettingsModel) SettingsModel { m.local.DebugLog = !m.local.DebugLog; return m },
			},
		},
	},
}

// LocalPrefs are the settings-screen preferences persisted to the config
// file rather than the API.
type LocalPrefs struct {
	WanderLust              bool
	FeedManualRefreshOnly   bool // true = feed background poll off
	TypingIndicatorsEnabled bool // positive polarity of config TypingIndicatorsDisabled
	DesktopNotifications    bool
	ShowGlobeTab            bool // positive polarity of config HideGlobeTab
	RevealContentWarnings   bool
	DebugLog                bool
	MaxThreadDepth          int
	Timezone                string // UTC offset label
	ImageViewer             string // "terminal" or "browser"
	GraphicsProtocol        string // "" auto, or "kitty"/"iterm2"/"sixel"
	InlineImages            bool
	Dithering               bool
	HardBreakKey            string
	KeywordAlerts           []string // edited via the KeywordEditorModel popup, not inline
}

// equal compares KeywordAlerts with slices.Equal so a nil and an empty list
// count as equal (the keyword editor can hand back either), then everything
// else with DeepEqual so a newly added field is never left out.
func (p LocalPrefs) equal(o LocalPrefs) bool {
	if !slices.Equal(p.KeywordAlerts, o.KeywordAlerts) {
		return false
	}
	p.KeywordAlerts, o.KeywordAlerts = nil, nil
	return reflect.DeepEqual(p, o)
}

// SettingsModel is the Settings screen.
type SettingsModel struct {
	settings      model.Settings // live/edited values
	original      model.Settings // last saved baseline
	local         LocalPrefs     // live/edited values
	originalLocal LocalPrefs     // last saved baseline
	capturing     bool           // true while the hard-break row is waiting for a keypress
	captureErr    string         // why the last captured key was rejected
	prefsSeeded   bool           // whether the SharedConfigMsg preference fields above have been seeded once
	cursor        int
	width         int
	height        int
	err           error
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
func (m SettingsModel) SetSaved(wanderLust bool, feedManualRefreshOnly bool, typingIndicatorsEnabled bool, desktopNotifications bool, showGlobeTab bool, maxThreadDepth int, timezone, imageViewer, graphicsProtocol string, inlineImages bool, dithering bool, keywordAlerts []string) SettingsModel {
	m.err = nil
	m.original = m.settings
	m.local.WanderLust = wanderLust
	m.originalLocal.WanderLust = wanderLust
	m.local.FeedManualRefreshOnly = feedManualRefreshOnly
	m.originalLocal.FeedManualRefreshOnly = feedManualRefreshOnly
	m.local.TypingIndicatorsEnabled = typingIndicatorsEnabled
	m.originalLocal.TypingIndicatorsEnabled = typingIndicatorsEnabled
	m.local.DesktopNotifications = desktopNotifications
	m.originalLocal.DesktopNotifications = desktopNotifications
	m.local.ShowGlobeTab = showGlobeTab
	m.originalLocal.ShowGlobeTab = showGlobeTab
	m.local.MaxThreadDepth = maxThreadDepth
	m.originalLocal.MaxThreadDepth = maxThreadDepth
	m.local.Timezone = timezone
	m.originalLocal.Timezone = timezone
	m.local.ImageViewer = imageViewer
	m.originalLocal.ImageViewer = imageViewer
	m.local.GraphicsProtocol = graphicsProtocol
	m.originalLocal.GraphicsProtocol = graphicsProtocol
	m.local.InlineImages = inlineImages
	m.originalLocal.InlineImages = inlineImages
	m.local.Dithering = dithering
	m.originalLocal.Dithering = dithering
	m.local.KeywordAlerts = keywordAlerts
	m.originalLocal.KeywordAlerts = keywordAlerts
	return m
}

// SetSavedHardBreakKey advances the hard-break key baseline after a save.
func (m SettingsModel) SetSavedHardBreakKey(key string) SettingsModel {
	m.local.HardBreakKey = key
	m.originalLocal.HardBreakKey = key
	return m
}

// SetSavedDebugLog advances the debug-log baseline after a save.
func (m SettingsModel) SetSavedDebugLog(on bool) SettingsModel {
	m.local.DebugLog = on
	m.originalLocal.DebugLog = on
	return m
}

// SetSavedRevealCW advances the reveal-content-warnings baseline after a save.
func (m SettingsModel) SetSavedRevealCW(reveal bool) SettingsModel {
	m.local.RevealContentWarnings = reveal
	m.originalLocal.RevealContentWarnings = reveal
	return m
}

// Capturing reports whether the hard-break row is waiting for a keypress. App
// uses it to hand every key to this screen instead of its global shortcuts.
func (m SettingsModel) Capturing() bool { return m.capturing }

// SetError sets the error field.
func (m SettingsModel) SetError(err error) SettingsModel {
	m.err = err
	return m
}

// KeywordAlerts returns the current keyword-alerts list, for App to seed the
// KeywordEditorModel popup when it opens.
func (m SettingsModel) KeywordAlerts() []string { return m.local.KeywordAlerts }

// SetKeywordAlerts writes the popup's edited list back into the working
// draft, for App to call right before dispatching a synthetic ctrl+s (see
// App.handleKeywordEditorKey).
func (m SettingsModel) SetKeywordAlerts(keywords []string) SettingsModel {
	m.local.KeywordAlerts = keywords
	return m
}

// IsDirty returns true if the current settings differ from the last saved baseline.
func (m SettingsModel) IsDirty() bool {
	return !settingsEqual(m.settings, m.original) || !m.local.equal(m.originalLocal)
}

// settingsEqual compares only the editable scalar fields.
func settingsEqual(a, b model.Settings) bool {
	return a.Notifications == b.Notifications &&
		a.FilterNSFW == b.FilterNSFW &&
		a.ShowFollowerCount == b.ShowFollowerCount &&
		a.AutoWatchOnReply == b.AutoWatchOnReply &&
		a.ShowGuildPostsInFeed == b.ShowGuildPostsInFeed &&
		a.ColorNicknames == b.ColorNicknames &&
		a.ImagePixelSize == b.ImagePixelSize &&
		a.ImageColor == b.ImageColor &&
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
		// Populate settings from shared config only on first load (when original
		// is zero). Subsequent broadcasts preserve any unsaved edits.
		if m.original.TimeDisplayFormat == "" && (m.original.Notifications == model.NotificationPrefs{}) {
			m = m.SetSettings(msg.Settings)
		}
		// MutedTopics isn't editable here (it's managed on the Topics tab), so
		// track the latest value on both the working copy and the baseline even
		// after first load — otherwise a ctrl+s on this screen would PATCH a
		// stale list back. settingsEqual ignores it, so dirty state is unaffected.
		// See docs/54-muted-topics.md.
		m.settings.MutedTopics = msg.Settings.MutedTopics
		m.original.MutedTopics = msg.Settings.MutedTopics
		// prefsSeeded gates the preference fields below independently of the
		// m.original guard above — SetSettings can run before this handler
		// ever sees a SharedConfigMsg (see settingsLoadedMsg in app.go), which
		// would otherwise trip a shared guard and skip seeding these fields,
		// leaving them at zero values that later get saved back to disk.
		if !m.prefsSeeded {
			m.prefsSeeded = true
			tz := msg.Timezone
			if tz == "" {
				tz = "UTC"
			}
			iv := msg.ImageViewer
			if iv == "" {
				iv = "terminal"
			}
			m.local = LocalPrefs{
				WanderLust:              msg.WanderLust,
				FeedManualRefreshOnly:   msg.FeedManualRefreshOnly,
				TypingIndicatorsEnabled: msg.TypingIndicatorsEnabled,
				DesktopNotifications:    msg.DesktopNotifications,
				ShowGlobeTab:            msg.ShowGlobeTab,
				RevealContentWarnings:   msg.RevealContentWarnings,
				DebugLog:                msg.DebugLog,
				MaxThreadDepth:          msg.MaxThreadDepth,
				Timezone:                tz,
				ImageViewer:             iv,
				GraphicsProtocol:        msg.GraphicsProtocol,
				InlineImages:            msg.InlineImages,
				Dithering:               msg.Dithering,
				HardBreakKey:            msg.HardBreakKey,
				KeywordAlerts:           msg.KeywordAlerts,
			}
			m.originalLocal = m.local
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		if m.capturing {
			return m.handleCapture(msg), nil
		}
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
			if m.cursor < total {
				switch items[m.cursor].kind {
				case "bool":
					m = items[m.cursor].toggle(m)
				case "keywordlist":
					return m, func() tea.Msg { return OpenKeywordEditorMsg{} }
				case "keybind":
					m.capturing, m.captureErr = true, ""
				}
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
				p := m.local
				remoteChanged := !settingsEqual(m.settings, m.original)
				return m, func() tea.Msg {
					return SaveSettingsMsg{Settings: s, WanderLust: p.WanderLust, FeedManualRefreshOnly: p.FeedManualRefreshOnly, TypingIndicatorsEnabled: p.TypingIndicatorsEnabled, DesktopNotifications: p.DesktopNotifications, ShowGlobeTab: p.ShowGlobeTab, MaxThreadDepth: p.MaxThreadDepth, Timezone: p.Timezone, ImageViewer: p.ImageViewer, GraphicsProtocol: p.GraphicsProtocol, InlineImages: p.InlineImages, Dithering: p.Dithering, KeywordAlerts: p.KeywordAlerts, HardBreakKey: p.HardBreakKey, RevealContentWarnings: p.RevealContentWarnings, DebugLog: p.DebugLog, RemoteChanged: remoteChanged}
				}
			}
			return m, nil

		case "esc":
			// Revert to original.
			m.settings = m.original
			m.local = m.originalLocal
			m.err = nil
			return m, nil
		}
	}

	return m, nil
}

// handleCapture consumes one keypress while the hard-break row is capturing.
// The first acceptable key is bound and ends capture; esc cancels. A key the
// compose screens cannot honour is rejected with the reason shown in the
// footer.
func (m SettingsModel) handleCapture(msg tea.KeyMsg) SettingsModel {
	if msg.String() == "esc" {
		m.capturing, m.captureErr = false, ""
		return m
	}
	key, err := HardBreakKeyFromMsg(msg)
	if err == nil {
		err = ValidateHardBreakKey(key)
	}
	if err != nil {
		m.captureErr = err.Error()
		return m
	}
	m.local.HardBreakKey = key
	m.capturing, m.captureErr = false, ""
	return m
}

// View renders the settings screen.
func (m SettingsModel) View() string {
	// Calculate available height (account for chrome and footer)
	availH := max(3, m.height-theme.ChromeHeight-1)

	var rows []string
	cursorRow := -1
	flatIdx := 0
	var selectedKind string

	for _, g := range settingsGroups {
		rows = append(rows, theme.Title.Render(g.title))

		for _, item := range g.items {
			if item.showIf != nil && !item.showIf(m) {
				continue
			}
			selected := m.cursor == flatIdx
			if selected {
				cursorRow = len(rows) // track which row the cursor is on
				selectedKind = item.kind
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
			switch item.kind {
			case "bool":
				if item.getBool(m) {
					rawValue = "[x]"
					value = theme.Highlight.Render("[x]")
				} else {
					rawValue = "[ ]"
					value = theme.Subtle.Render("[ ]")
				}
			case "keywordlist":
				switch len(m.local.KeywordAlerts) {
				case 0:
					rawValue = "(none)"
					value = theme.Subtle.Render(rawValue)
				case 1:
					rawValue = "1 keyword"
					value = theme.Highlight.Render(rawValue)
				default:
					rawValue = fmt.Sprintf("%d keywords", len(m.local.KeywordAlerts))
					value = theme.Highlight.Render(rawValue)
				}
			case "keybind":
				if m.capturing {
					rawValue = "press a key combo…"
				} else {
					rawValue = HardBreakLabel(m.local.HardBreakKey)
				}
				value = theme.Highlight.Render(rawValue)
			default: // "enum"
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
	switch {
	case m.capturing && m.captureErr != "":
		footer = theme.Error.Render(m.captureErr + "   esc · cancel")
	case m.capturing:
		footer = theme.Subtle.Render("press the key combo   esc · cancel")
	case m.err != nil:
		footer = theme.Error.Render("error: " + m.err.Error())
	default:
		hint := "space/enter · toggle   tab · cycle enum"
		switch selectedKind {
		case "keywordlist":
			hint = "enter · edit keywords"
		case "keybind":
			hint = "enter · set combo"
		}
		if m.IsDirty() {
			hint += "   ctrl+s · save   esc · revert"
		}
		footer = theme.Subtle.Render(hint)
	}
	visible = append(visible, footer)

	return lipgloss.JoinVertical(lipgloss.Left, visible...)
}
