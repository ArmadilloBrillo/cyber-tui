package ui

import (
	"strings"
	"unicode/utf8"

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

// navTab is one entry in menuTabs.
type navTab struct {
	label    string
	mnemonic rune
	s        screen
	// hidden excludes this entry from the rendered tab bar/nav sidebar and
	// from arrow-key cycling (see visibleTabs, navigateTabBy) while keeping
	// it reachable via its mnemonic leader chord and listed in the help
	// modal's leader legend (leaderRows). Used for Search: it's an
	// explicit-entry-only destination ("g s" or "/"), not a tab you park on
	// or arrow past while browsing — see docs/02-menu-bar-navigation.md.
	hidden bool
}

// menuTabs is the ordered list of navigable screens — the single source of
// truth for tab-bar rendering, the "1"-"9" numeric aliases (the first 9
// entries, by index), and the "g"+mnemonic leader-key chords (all entries,
// via their mnemonic rune). Keeping all three derived from this one slice is
// what keeps TabsLayout and MillerLayout from drifting apart. mnemonic must
// be a rune that appears in label, since the tab bar renders it highlighted
// inline within the label text.
var menuTabs = []navTab{
	{label: "feed", mnemonic: 'f', s: screenFeed},
	{label: "notifications", mnemonic: 'n', s: screenNotifications},
	{label: "c-mail", mnemonic: 'm', s: screenCMail},
	{label: "circ", mnemonic: 'i', s: screenChatrooms},
	{label: "journal", mnemonic: 'j', s: screenJournal},
	{label: "bookmarks", mnemonic: 'b', s: screenBookmarks},
	{label: "guilds", mnemonic: 'g', s: screenGuilds},
	{label: "topics", mnemonic: 't', s: screenTopics},
	{label: "profile", mnemonic: 'p', s: screenProfile},
	{label: "search", mnemonic: 's', s: screenSearch, hidden: true},
	{label: "settings", mnemonic: 'e', s: screenSettings},
}

// visibleTabs returns the menuTabs entries shown on the tab bar/nav sidebar
// and reachable by arrow-key cycling — i.e. everything except hidden entries.
func visibleTabs() []navTab {
	out := make([]navTab, 0, len(menuTabs))
	for _, t := range menuTabs {
		if !t.hidden {
			out = append(out, t)
		}
	}
	return out
}

// tabVisualState reports whether tab t is the one currently selected, and
// whether it's one level deep in a detail sub-view — an open Circ room, an
// open C-Mail conversation, a Guilds/Topics browse, or PostDetail opened from
// t (postDetailReturn == t, since PostDetail is a single shared screen reused
// by six origin tabs rather than duplicated per-origin). Both TabsLayout and
// MillerLayout call this so the two layouts can never disagree about which
// state a tab is in.
//
// detail is reported even while t isn't selected for Circ/Guilds/Topics,
// since their detail state is genuinely still live in the background: Circ's
// open room keeps its RTDB subscription streaming regardless of the active
// tab (see IsRoomStreamMsg in app.go), and Guilds/Topics' browse state is
// simply never reset by activateScreen on tab-away. C-Mail and PostDetail are
// selected-only instead: CMailModel.CancelSubscription (tab-away) tears the
// conversation's subscription down immediately but doesn't reset m.mode, so
// it lingers as cmailModeDetail — stale, not live — until the next
// ResetToList(); surfacing that in the background would claim a conversation
// is still open when it's already been torn down (C-Mail's actual "something
// happened" signal is the aggregate unread badge, not a left-open
// conversation). PostDetail has no background resumption at all.
func tabVisualState(a App, t screen) (selected, detail bool) {
	selected = a.active == t || (a.active == screenPostDetail && a.postDetailReturn == t)

	switch t {
	case screenChatrooms:
		detail = a.chatrooms.IsShowingDetail()
	case screenGuilds:
		detail = a.guilds.IsBrowsingGuild() || a.guilds.IsBrowsingMembers()
	case screenTopics:
		detail = a.topics.IsBrowsingTopic()
	case screenCMail:
		detail = selected && a.cmail.IsShowingDetail()
	}
	if selected && a.active == screenPostDetail {
		detail = true
	}
	return selected, detail
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

// tabIndexOf returns the index of a.active within visibleTabs, defaulting to
// 0. a.active won't normally be a hidden screen (Search) here, since
// navigateTabBy — the only caller — is a no-op while Search is active; if it
// ever is, this defaults to 0 (Feed) same as any other not-found screen.
func tabIndexOf(a App) int {
	for i, t := range visibleTabs() {
		if t.s == a.active {
			return i
		}
	}
	return 0
}

// screenForNumber resolves a "1"-"9" key to its menuTabs entry, by index.
// Only the first 9 of menuTabs' 11 entries have a numeric alias; Search and
// Settings are reachable only via the "g"+mnemonic leader chord (see
// screenForMnemonic) or, for Search, "/".
func screenForNumber(key string) (screen, bool) {
	if len(key) != 1 || key[0] < '1' || key[0] > '9' {
		return 0, false
	}
	idx := int(key[0] - '1')
	if idx >= len(menuTabs) {
		return 0, false
	}
	return menuTabs[idx].s, true
}

// screenForMnemonic resolves the second keystroke of a "g"-prefixed leader
// chord (e.g. "g f" for Feed) to its menuTabs entry. Derived from menuTabs
// so it can never drift from what's shown highlighted on the tab bar.
func screenForMnemonic(key string) (screen, bool) {
	if len(key) != 1 {
		return 0, false
	}
	for _, t := range menuTabs {
		if rune(key[0]) == t.mnemonic {
			return t.s, true
		}
	}
	return 0, false
}

// splitMnemonic locates mnemonic within label and returns it split into three
// parts (before, the mnemonic character itself, after) so a renderer can
// style the mnemonic distinctly as an inline "go to" hint. If mnemonic isn't
// found, ch is empty and before holds the full label.
func splitMnemonic(label string, mnemonic rune) (before, ch, after string) {
	idx := strings.IndexRune(label, mnemonic)
	if idx < 0 {
		return label, "", ""
	}
	n := utf8.RuneLen(mnemonic)
	return label[:idx], label[idx : idx+n], label[idx+n:]
}

// leaderRows formats every "g"+mnemonic chord as a help-modal row via row
// (see TabsLayout/MillerLayout's renderHelpModal), derived from menuTabs so
// the help text can never drift from what the leader key actually does.
func leaderRows(row func(key, desc string) string) []string {
	rows := make([]string, 0, len(menuTabs))
	for _, t := range menuTabs {
		rows = append(rows, row("g "+string(t.mnemonic), t.label))
	}
	return rows
}

// activateScreen switches directly to screen s (as opposed to navigateTabBy's
// relative cycling), cancelling any live subscription being left behind and
// running the same lazy-load-on-entry side effects as cycling would. Used by
// the "1"-"9" numeric aliases and the "g"+mnemonic leader chords in both
// layouts, so a direct jump behaves identically to arriving via cycling.
func activateScreen(a App, s screen) (App, tea.Cmd) {
	if a.active == screenCMail {
		a.cmail = a.cmail.CancelSubscription()
	}
	if a.active == screenChatrooms && s != screenChatrooms {
		a.chatrooms = a.chatrooms.SetFocused(false)
	}
	prev := a.active
	a.active = s
	if a.active == screenSearch && prev != screenSearch {
		a.searchReturn = prev
	}
	switch a.active {
	case screenFeed:
		if !a.feed.IsLoaded() {
			a.feed = a.feed.SetFetching()
			return a, a.loadFeedCmd()
		}
		return a, nil
	case screenChatrooms:
		a.chatrooms = a.chatrooms.SetFocused(true)
		// A room left open when the user last switched away to a *different*
		// tab kept its RTDB subscription live in the background (see
		// IsRoomStreamMsg) — resume it as-is instead of bouncing back to the
		// room list. Re-pressing the Chatrooms key while already on it (prev
		// == screenChatrooms) is the deliberate escape hatch out of a
		// chat_mention deep link, so that case still resets to the list.
		if prev != screenChatrooms && a.chatrooms.HasLiveRoom() {
			return a, nil
		}
		a.chatrooms = a.chatrooms.ResetToList()
		return a, a.loadRoomsCmd()
	case screenCMail:
		a.cmail = a.cmail.ResetToList()
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
	case screenSearch:
		// No auto-fetch: Search only has meaning once a query is submitted.
		// Jumping in just shows whatever state it was last left in.
		return a, nil
	}
	return a, nil
}

// navigateTabBy computes the App state and load command for moving delta
// steps through visibleTabs from the current active screen. A no-op while
// Search is active: it's a hidden, explicit-entry-only destination (reached
// via "g s" or "/", see handleKeys in app.go), not part of the cyclable tab
// rotation — the same reason screenPostDetail was never part of it either.
func navigateTabBy(a App, delta int) (App, tea.Cmd) {
	if a.active == screenSearch {
		return a, nil
	}
	tabs := visibleTabs()
	idx := (tabIndexOf(a) + delta + len(tabs)) % len(tabs)
	return activateScreen(a, tabs[idx].s)
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
	case screenSearch:
		a.search, cmd = a.search.Update(msg)
	}
	return a, cmd
}
