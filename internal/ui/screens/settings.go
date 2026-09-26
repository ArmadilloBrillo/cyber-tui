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
			{
				label: "show guild posts in feed", kind: "bool",
				getBool: func(m SettingsModel) bool { return m.settings.ShowGuildPostsInFeed },
				toggle: func(m SettingsModel) SettingsModel {
					m.settings.ShowGuildPostsInFeed = !m.settings.ShowGuildPostsInFeed
					return m
				},
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
	{
		title: "feed",
		items: []settingsItem{
			{
				// Internally tracked as feedManualRefreshOnly (true = off);
				// inverted here so "on" reads as the enabled state — see
				// docs/39-feed-background-poll.md.
				label: "auto-refresh (background poll)", kind: "bool",
				getBool: func(m SettingsModel) bool { return !m.feedManualRefreshOnly },
				toggle: func(m SettingsModel) SettingsModel {
					m.feedManualRefreshOnly = !m.feedManualRefreshOnly
					return m
				},
			},
		},
	},
	{
		title: "compose",
		items: []settingsItem{
			{
				// The key that inserts a markdown hard line break, for
				// terminals that swallow the default alt+enter. Captured
				// rather than typed: kind "keybind" has its own key handling
				// in Update/handleCapture and View, like "keywordlist".
				label: "hard line break key", kind: "keybind",
			},
		},
	},
	{
		title: "c-mail",
		items: []settingsItem{
			{
				label: "typing indicators", kind: "bool",
				getBool: func(m SettingsModel) bool { return m.typingIndicatorsEnabled },
				toggle: func(m SettingsModel) SettingsModel {
					m.typingIndicatorsEnabled = !m.typingIndicatorsEnabled
					return m
				},
			},
		},
	},
	{
		title: "desktop",
		items: []settingsItem{
			{
				// OS desktop notification (OSC 9 escape) for new C-Mail /
				// activity while backgrounded — see
				// docs/53-desktop-notifications.md. Silently does nothing on
				// terminals without OSC 9 support.
				label: "notifications (OSC 9 terminals)", kind: "bool",
				getBool: func(m SettingsModel) bool { return m.desktopNotifications },
				toggle: func(m SettingsModel) SettingsModel {
					m.desktopNotifications = !m.desktopNotifications
					return m
				},
			},
		},
	},
	{
		title: "globe",
		items: []settingsItem{
			{
				label: "show globe tab", kind: "bool",
				getBool: func(m SettingsModel) bool { return m.showGlobeTab },
				toggle: func(m SettingsModel) SettingsModel {
					m.showGlobeTab = !m.showGlobeTab
					return m
				},
			},
		},
	},
	{
		title: "keyword alerts",
		items: []settingsItem{
			{
				// Words/phrases that raise a Notifications-tab entry (and,
				// subject to the desktop-notifications toggle above, an OSC 9
				// toast) wherever content is scanned — cIRC, C-Mail, posts,
				// replies, and post topics/tags. Edited in place (kind
				// "keywordlist" has its own key handling in Update/View,
				// there being only one such row, unlike bool/enum's
				// getter/setter closures).
				label: "alert keywords", kind: "keywordlist",
			},
		},
	},
}

// SettingsModel is the Settings screen.
type SettingsModel struct {
	settings                        model.Settings // live/edited values
	original                        model.Settings // last saved baseline
	wanderLust                      bool           // live local config value
	originalWanderLust              bool           // last saved baseline for wanderLust
	maxThreadDepth                  int            // live local config value (1–5)
	originalMaxThreadDepth          int            // last saved baseline
	timezone                        string         // live local config value (UTC offset label)
	originalTimezone                string         // last saved baseline
	imageViewer                     string         // live local config value ("terminal" or "browser")
	originalImageViewer             string         // last saved baseline
	graphicsProtocol                string         // live local config value ("" auto, or "kitty"/"iterm2"/"sixel")
	originalGraphicsProtocol        string         // last saved baseline
	inlineImages                    bool           // live local config value
	originalInlineImages            bool           // last saved baseline
	dithering                       bool           // live local config value
	originalDithering               bool           // last saved baseline
	ditherSharpness                 string         // live local config value ("rough"/"medium"/"sharp")
	originalDitherSharpness         string         // last saved baseline
	feedManualRefreshOnly           bool           // live local config value (true = feed background poll off)
	originalFeedManualRefreshOnly   bool           // last saved baseline
	typingIndicatorsEnabled         bool           // live local config value (positive polarity)
	originalTypingIndicatorsEnabled bool           // last saved baseline
	desktopNotifications            bool           // live local config value (OSC 9 desktop notifications)
	originalDesktopNotifications    bool           // last saved baseline
	showGlobeTab                    bool           // live local config value (Globe tab shown on the tab bar/cycling)
	originalShowGlobeTab            bool           // last saved baseline
	keywordAlerts                   []string       // live local config value (config.Config.KeywordAlerts); edited via the KeywordEditorModel popup, not inline
	originalKeywordAlerts           []string       // last saved baseline
	hardBreakKey                    string         // live local config value (config.Config.HardBreakKey)
	originalHardBreakKey            string         // last saved baseline
	capturing                       bool           // true while the hard-break row is waiting for a keypress
	captureErr                      string         // why the last captured key was rejected
	prefsSeeded                     bool           // whether the SharedConfigMsg preference fields above have been seeded once
	cursor                          int
	width                           int
	height                          int
	err                             error
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
func (m SettingsModel) SetSaved(wanderLust bool, feedManualRefreshOnly bool, typingIndicatorsEnabled bool, desktopNotifications bool, showGlobeTab bool, maxThreadDepth int, timezone, imageViewer, graphicsProtocol string, inlineImages bool, dithering bool, ditherSharpness string, keywordAlerts []string) SettingsModel {
	m.err = nil
	m.original = m.settings
	m.wanderLust = wanderLust
	m.originalWanderLust = wanderLust
	m.feedManualRefreshOnly = feedManualRefreshOnly
	m.originalFeedManualRefreshOnly = feedManualRefreshOnly
	m.typingIndicatorsEnabled = typingIndicatorsEnabled
	m.originalTypingIndicatorsEnabled = typingIndicatorsEnabled
	m.desktopNotifications = desktopNotifications
	m.originalDesktopNotifications = desktopNotifications
	m.showGlobeTab = showGlobeTab
	m.originalShowGlobeTab = showGlobeTab
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
	m.keywordAlerts = keywordAlerts
	m.originalKeywordAlerts = keywordAlerts
	return m
}

// SetSavedHardBreakKey advances the hard-break key baseline after a save.
func (m SettingsModel) SetSavedHardBreakKey(key string) SettingsModel {
	m.hardBreakKey = key
	m.originalHardBreakKey = key
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
func (m SettingsModel) KeywordAlerts() []string { return m.keywordAlerts }

// SetKeywordAlerts writes the popup's edited list back into the working
// draft, for App to call right before dispatching a synthetic ctrl+s (see
// App.handleKeywordEditorKey).
func (m SettingsModel) SetKeywordAlerts(keywords []string) SettingsModel {
	m.keywordAlerts = keywords
	return m
}

// IsDirty returns true if the current settings differ from the last saved baseline.
func (m SettingsModel) IsDirty() bool {
	return !settingsEqual(m.settings, m.original) ||
		m.wanderLust != m.originalWanderLust ||
		m.feedManualRefreshOnly != m.originalFeedManualRefreshOnly ||
		m.typingIndicatorsEnabled != m.originalTypingIndicatorsEnabled ||
		m.desktopNotifications != m.originalDesktopNotifications ||
		m.showGlobeTab != m.originalShowGlobeTab ||
		m.maxThreadDepth != m.originalMaxThreadDepth ||
		m.timezone != m.originalTimezone ||
		m.imageViewer != m.originalImageViewer ||
		m.graphicsProtocol != m.originalGraphicsProtocol ||
		m.inlineImages != m.originalInlineImages ||
		m.dithering != m.originalDithering ||
		m.ditherSharpness != m.originalDitherSharpness ||
		m.hardBreakKey != m.originalHardBreakKey ||
		!stringSlicesEqual(m.keywordAlerts, m.originalKeywordAlerts)
}

// stringSlicesEqual compares two string slices element-by-element in order.
func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// settingsEqual compares only the editable scalar fields.
func settingsEqual(a, b model.Settings) bool {
	return a.Notifications == b.Notifications &&
		a.FilterNSFW == b.FilterNSFW &&
		a.ShowFollowerCount == b.ShowFollowerCount &&
		a.AutoWatchOnReply == b.AutoWatchOnReply &&
		a.ShowGuildPostsInFeed == b.ShowGuildPostsInFeed &&
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
			m.wanderLust = msg.WanderLust
			m.originalWanderLust = msg.WanderLust
			m.feedManualRefreshOnly = msg.FeedManualRefreshOnly
			m.originalFeedManualRefreshOnly = msg.FeedManualRefreshOnly
			m.typingIndicatorsEnabled = msg.TypingIndicatorsEnabled
			m.originalTypingIndicatorsEnabled = msg.TypingIndicatorsEnabled
			m.desktopNotifications = msg.DesktopNotifications
			m.originalDesktopNotifications = msg.DesktopNotifications
			m.showGlobeTab = msg.ShowGlobeTab
			m.originalShowGlobeTab = msg.ShowGlobeTab
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
			m.keywordAlerts = msg.KeywordAlerts
			m.originalKeywordAlerts = msg.KeywordAlerts
			m.hardBreakKey = msg.HardBreakKey
			m.originalHardBreakKey = msg.HardBreakKey
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
				wl := m.wanderLust
				fmro := m.feedManualRefreshOnly
				tie := m.typingIndicatorsEnabled
				dn := m.desktopNotifications
				sgt := m.showGlobeTab
				td := m.maxThreadDepth
				tz := m.timezone
				iv := m.imageViewer
				gp := m.graphicsProtocol
				ii := m.inlineImages
				dt := m.dithering
				ds := m.ditherSharpness
				ka := m.keywordAlerts
				hb := m.hardBreakKey
				remoteChanged := !settingsEqual(m.settings, m.original)
				return m, func() tea.Msg {
					return SaveSettingsMsg{Settings: s, WanderLust: wl, FeedManualRefreshOnly: fmro, TypingIndicatorsEnabled: tie, DesktopNotifications: dn, ShowGlobeTab: sgt, MaxThreadDepth: td, Timezone: tz, ImageViewer: iv, GraphicsProtocol: gp, InlineImages: ii, Dithering: dt, DitherSharpness: ds, KeywordAlerts: ka, HardBreakKey: hb, RemoteChanged: remoteChanged}
				}
			}
			return m, nil

		case "esc":
			// Revert to original.
			m.settings = m.original
			m.wanderLust = m.originalWanderLust
			m.feedManualRefreshOnly = m.originalFeedManualRefreshOnly
			m.typingIndicatorsEnabled = m.originalTypingIndicatorsEnabled
			m.desktopNotifications = m.originalDesktopNotifications
			m.showGlobeTab = m.originalShowGlobeTab
			m.maxThreadDepth = m.originalMaxThreadDepth
			m.timezone = m.originalTimezone
			m.imageViewer = m.originalImageViewer
			m.graphicsProtocol = m.originalGraphicsProtocol
			m.inlineImages = m.originalInlineImages
			m.dithering = m.originalDithering
			m.ditherSharpness = m.originalDitherSharpness
			m.keywordAlerts = m.originalKeywordAlerts
			m.hardBreakKey = m.originalHardBreakKey
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
	m.hardBreakKey = key
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
				switch len(m.keywordAlerts) {
				case 0:
					rawValue = "(none)"
					value = theme.Subtle.Render(rawValue)
				case 1:
					rawValue = "1 keyword"
					value = theme.Highlight.Render(rawValue)
				default:
					rawValue = fmt.Sprintf("%d keywords", len(m.keywordAlerts))
					value = theme.Highlight.Render(rawValue)
				}
			case "keybind":
				if m.capturing {
					rawValue = "press a key combo…"
				} else {
					rawValue = HardBreakLabel(m.hardBreakKey)
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
	case m.IsDirty():
		footer = theme.Subtle.Render("ctrl+s · save   esc · revert")
	case selectedKind == "keywordlist":
		footer = theme.Subtle.Render("enter · edit keywords")
	case selectedKind == "keybind":
		footer = theme.Subtle.Render("enter · set combo")
	default:
		footer = theme.Subtle.Render("space/enter · toggle   tab · cycle enum")
	}
	visible = append(visible, footer)

	return lipgloss.JoinVertical(lipgloss.Left, visible...)
}
