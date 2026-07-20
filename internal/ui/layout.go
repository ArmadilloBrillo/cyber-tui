package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
	"github.com/ragnar/cyber-tui/internal/version"
)

// Layout arranges the app's screens and handles navigation for a specific UI paradigm.
// All application state lives on App; Layout provides only method implementations.
type Layout interface {
	View(a App) string
	HandleNav(msg tea.KeyMsg, a App) (App, tea.Cmd, bool)
	DelegateUpdate(msg tea.Msg, a App) (App, tea.Cmd)
	HasFocusedInput(a App) bool
	ContentWidth(termWidth int) int
	// ContentHeight returns the height to send to screens in WindowSizeMsg. Screens subtract
	// theme.ChromeHeight to get viewport height; layouts that use fewer chrome rows must compensate
	// so the viewport fills the available content pane exactly.
	ContentHeight(termHeight int) int
	// NeedsCompactAutoFill returns the minimum number of items needed to fill the compact list
	// column at the given terminal height. Returns 0 if the layout has no compact list column.
	// App uses this to auto-fetch additional pages after the initial load.
	NeedsCompactAutoFill(termHeight int) int
}

// CompactListRenderer is optionally implemented by screens that can display as a compact
// item list beside a detail reading pane. Layouts supporting 3-pane views should
// retrieve the active screen via activeCompactRenderer rather than casting concrete types.
type CompactListRenderer interface {
	// IsCompactListActive reports whether the screen is currently in a state where a
	// compact list should be shown (e.g., a guild/topic has been drilled into).
	IsCompactListActive() bool
	// ListTitle returns the column header for the compact list pane.
	ListTitle() string
	CompactListView(width, height int) string
	DetailView(width, height int) string
}

// CompactComposer is an optional extension of CompactListRenderer for screens that have
// a compose panel. In Miller mode the layout pulls the panel out of DetailView and
// renders it as a full-width row spanning the list and detail columns, making it clear
// the user is composing a new post rather than a reply.
type CompactComposer interface {
	ComposeActive() bool
	ComposeHeight() int           // total rows the panel occupies (for contentH budget)
	ComposeView(width int) string // panel rendered at the given spanning width
}

// menuTabs is the ordered list of navigable screens.
var menuTabs = []struct {
	label string
	s     screen
}{
	{"feed", screenFeed},
	{"notifications", screenNotifications},
	{"journal", screenJournal},
	{"bookmarks", screenBookmarks},
	{"guilds", screenGuilds},
	{"topics", screenTopics},
	{"profile", screenProfile},
	{"settings", screenSettings},
}

var renderedVersionLine = theme.Subtle.Render("version " + version.Version + " (" + version.Commit + ")")

// hint is a compact key+description pair shown in the status bar and help modal.
type hint struct{ key, desc string }

// sbStyle returns a bare style with the status-bar background.
func sbStyle() lipgloss.Style {
	return lipgloss.NewStyle().Background(theme.ColorDimGreen)
}

// renderHints formats a []hint slice as a compact styled string.
func renderHints(hints []hint) string {
	key := sbStyle().Foreground(theme.ColorCyan).Bold(true)
	desc := sbStyle().Foreground(theme.ColorWhite)
	sep := sbStyle().Foreground(theme.ColorMuted).Render(" · ")
	parts := make([]string, 0, len(hints)*3)
	for i, h := range hints {
		if i > 0 {
			parts = append(parts, sep)
		}
		parts = append(parts, key.Render(h.key))
		if h.desc != "" {
			parts = append(parts, desc.Render(" "+h.desc))
		}
	}
	return strings.Join(parts, "")
}

// overlayCenter composites fg centered over bg using ANSI-aware string splicing.
// Each line of fg replaces the corresponding characters in bg at the centered
// position, preserving ANSI colour codes on both sides of the splice point.
func overlayCenter(bg, fg string, bgW, bgH int) string {
	fgW := lipgloss.Width(fg)
	fgLines := strings.Split(fg, "\n")
	fgH := len(fgLines)
	bgLines := strings.Split(bg, "\n")

	xOff := (bgW - fgW) / 2
	yOff := (bgH - fgH) / 2
	if xOff < 0 {
		xOff = 0
	}
	if yOff < 0 {
		yOff = 0
	}

	result := make([]string, len(bgLines))
	copy(result, bgLines)

	for i, fgLine := range fgLines {
		bi := yOff + i
		if bi < 0 || bi >= len(result) {
			continue
		}
		bgLine := result[bi]
		// Pad the background line if it's shorter than the splice end point.
		bgLineW := ansi.StringWidth(bgLine)
		needed := xOff + fgW
		if bgLineW < needed {
			bgLine += strings.Repeat(" ", needed-bgLineW)
		}
		left := ansi.Truncate(bgLine, xOff, "")
		right := ansi.TruncateLeft(bgLine, xOff+fgW, "")
		result[bi] = left + fgLine + right
	}
	return strings.Join(result, "\n")
}

// themeIndex returns the index of name in availableThemes, defaulting to 0.
func themeIndex(name string) int {
	for i, t := range availableThemes {
		if t == name {
			return i
		}
	}
	return 0
}

// tabIndexOf returns the index of a.active within menuTabs, defaulting to 0.
func tabIndexOf(a App) int {
	for i, t := range menuTabs {
		if t.s == a.active {
			return i
		}
	}
	return 0
}

// navigateTabBy computes the App state and load command for moving delta steps
// through menuTabs from the current active screen.
func navigateTabBy(a App, delta int) (App, tea.Cmd) {
	if a.active == screenCMail {
		a.cmail = a.cmail.CancelSubscription()
	}
	idx := (tabIndexOf(a) + delta + len(menuTabs)) % len(menuTabs)
	a.active = menuTabs[idx].s
	switch a.active {
	case screenFeed:
		if !a.feed.IsLoaded() {
			a.feed = a.feed.SetFetching()
			return a, a.loadFeedCmd()
		}
		return a, nil
	case screenChatrooms:
		return a, a.loadRoomsCmd()
	case screenCMail:
		return a, a.loadConvsCmd()
	case screenProfile:
		return a, a.loadProfileCmd()
	case screenNotifications:
		if !a.notifications.HasPaginated() {
			a.notifications = a.notifications.SetFetching()
			return a, a.loadNotifsCmd()
		}
		return a, nil
	case screenSettings:
		return a, nil
	case screenBookmarks:
		if !a.bookmarks.IsLoaded() {
			a.bookmarks = a.bookmarks.SetFetching()
			return a, a.loadBookmarksCmd("")
		}
		return a, nil
	case screenGuilds:
		if !a.guilds.IsLoaded() {
			a.guilds = a.guilds.SetFetching()
			return a, a.loadGuildsCmd("")
		}
		return a, nil
	case screenTopics:
		if !a.topics.IsLoaded() {
			a.topics = a.topics.SetFetching()
			return a, a.loadTopicsCmd()
		}
		return a, nil
	case screenJournal:
		a.journal = a.journal.SetFetching()
		return a, a.loadJournalCmd()
	}
	return a, nil
}

// delegateScreenUpdate routes a message to the currently active screen model.
// Both TabsLayout and MillerLayout have identical routing; this function
// centralises it so adding a new screen only requires one edit here.
func delegateScreenUpdate(msg tea.Msg, a App) (App, tea.Cmd) {
	var cmd tea.Cmd
	switch a.active {
	case screenLogin:
		a.login, cmd = a.login.Update(msg)
	case screenFeed:
		a.feed, cmd = a.feed.Update(msg)
	case screenChatrooms:
		a.chatrooms, cmd = a.chatrooms.Update(msg)
	case screenCMail:
		a.cmail, cmd = a.cmail.Update(msg)
	case screenProfile:
		a.profile, cmd = a.profile.Update(msg)
	case screenPostDetail:
		a.postDetail, cmd = a.postDetail.Update(msg)
	case screenNotifications:
		a.notifications, cmd = a.notifications.Update(msg)
	case screenSettings:
		a.settingsScreen, cmd = a.settingsScreen.Update(msg)
	case screenBookmarks:
		a.bookmarks, cmd = a.bookmarks.Update(msg)
	case screenGuilds:
		a.guilds, cmd = a.guilds.Update(msg)
	case screenTopics:
		a.topics, cmd = a.topics.Update(msg)
	case screenJournal:
		a.journal, cmd = a.journal.Update(msg)
	}
	return a, cmd
}
