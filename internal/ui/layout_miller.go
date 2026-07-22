package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/ragnar/cyber-tui/internal/ui/imgview"
	"github.com/ragnar/cyber-tui/internal/ui/screens"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
)

const millerListMaxWidth = 70 // hard cap on the list pane; above this, excess goes to the detail pane

// MillerLayout renders a top tab bar above a split content area.
type MillerLayout struct{}

// paneWidths returns the list and detail column widths for the given content area.
// The detail pane is pinned at its preferred width (45); the list takes remaining
// space and collapses first when narrowing. Above millerListMaxWidth the excess
// goes to the detail pane. Add a similar method to any future multi-pane layout
// to keep its collapsing logic self-contained.
func (l MillerLayout) paneWidths(contentW int) (listW, detailW int) {
	const preferredDetailW = 45 // detail width at ~120-col terminal
	detailW = min(preferredDetailW, max(0, contentW*46/100))
	listW = min(millerListMaxWidth, max(0, contentW-detailW-1))
	if listW == millerListMaxWidth {
		detailW = max(0, contentW-listW-1)
	}
	return
}

// NeedsCompactAutoFill returns the minimum item count to fill the compact list
// column. App uses this after each page load to decide whether to fetch more.
func (l MillerLayout) NeedsCompactAutoFill(termHeight int) int {
	return termHeight - theme.ChromeHeight // tab bar + column header + status bar
}

// activeCompactRenderer returns the active screen as a CompactListRenderer if it
// currently supports 3-pane display, or nil for single-pane screens.
func (l MillerLayout) activeCompactRenderer(a App) CompactListRenderer {
	var r CompactListRenderer
	switch a.active {
	case screenFeed:
		r = a.feed
	case screenGuilds:
		r = a.guilds
	case screenTopics:
		r = a.topics
	}
	if r != nil && r.IsCompactListActive() {
		return r
	}
	return nil
}

func (l MillerLayout) View(a App) string {
	if a.active == screenLogin {
		return a.login.View()
	}

	contentH := a.height - theme.ChromeHeight // tab bar + column header + status bar
	contentW := a.width

	colSep := theme.Subtle.Render("│")

	logo := lipgloss.NewStyle().
		Background(theme.ColorGreen).
		Foreground(theme.ColorBackground).
		Bold(true).
		Padding(0, 1).
		Render(a.logoText)
	logoW := lipgloss.Width(logo)

	// focusMenu is treated as focusList in Miller (initial/reset state from app).
	listFocused := a.focus == focusList || a.focus == focusMenu

	var contentPane, hdrRow, composeBar string
	if r := l.activeCompactRenderer(a); r != nil {
		listW, detailW := l.paneWidths(contentW)

		// If compose panel is active, pull it out of DetailView and render it as a
		// full-width bar spanning the list and detail columns (above the status bar).
		if cc, ok := r.(CompactComposer); ok && cc.ComposeActive() {
			contentH = max(0, contentH-cc.ComposeHeight())
			composeBar = cc.ComposeView(contentW)
		}

		listHdr := l.renderColumnHeader(r.ListTitle(), listFocused, listW)
		detailHdr := l.renderColumnHeader("thread", a.focus == focusDetail, detailW-logoW)
		hdrRow = lipgloss.JoinHorizontal(lipgloss.Top, listHdr, colSep, detailHdr) + logo

		listP := lipgloss.NewStyle().Width(listW).Height(contentH).MaxHeight(contentH).
			Render(r.CompactListView(listW, contentH))
		listSep := theme.Subtle.Render(strings.TrimSuffix(strings.Repeat("│\n", contentH), "\n"))
		detailP := lipgloss.NewStyle().Width(detailW).Height(contentH).MaxHeight(contentH).
			Render(r.DetailView(detailW, contentH))
		contentPane = lipgloss.JoinHorizontal(lipgloss.Top, listP, listSep, detailP)
	} else {
		contentHdr := l.renderColumnHeader(l.screenTitle(a), true, contentW-logoW)
		hdrRow = contentHdr + logo
		contentPane = lipgloss.NewStyle().Width(contentW).Height(contentH).MaxHeight(contentH).Render(l.renderContent(a))
	}

	var base string
	if composeBar != "" {
		base = lipgloss.JoinVertical(lipgloss.Left, l.renderTabBar(a), hdrRow, contentPane, composeBar, l.renderBottomBar(a))
	} else {
		base = lipgloss.JoinVertical(lipgloss.Left, l.renderTabBar(a), hdrRow, contentPane, l.renderBottomBar(a))
	}

	if a.themePickerOpen {
		return overlayCenter(base, l.renderThemePicker(a), a.width, a.height)
	}
	if a.helpModalOpen {
		return overlayCenter(base, l.renderHelpModal(a), a.width, a.height)
	}
	if a.urlPickerOpen {
		return overlayCenter(base, l.renderURLPicker(a), a.width, a.height)
	}
	if a.imageModalOpen {
		textModal := l.renderImageModal(a)
		composed := overlayCenter(base, textModal, a.width, a.height)
		modalW := lipgloss.Width(textModal)
		modalH := len(strings.Split(textModal, "\n"))
		xOff := (a.width - modalW) / 2
		yOff := (a.height - modalH) / 2
		xOff = max(0, xOff)
		yOff = max(0, yOff)
		imgRow := yOff + 2
		imgCol := xOff + 3
		return composed + fmt.Sprintf("\x1b[%d;%dH%s\x1b[%d;1H", imgRow, imgCol, a.imageModalEncoded, a.height)
	}
	if a.imageNeedsCleanup && a.graphicsProtocol == imgview.ProtocolKitty {
		modalH := a.imageModalRows + 2
		yOff := max(0, (a.height-modalH)/2)
		lines := strings.Split(base, "\n")
		if yOff < len(lines) {
			lines[yOff] = "\x1b_Ga=d,d=A\x1b\\" + lines[yOff]
		}
		return strings.Join(lines, "\n")
	}
	return base
}

func (l MillerLayout) HandleNav(msg tea.KeyMsg, a App) (App, tea.Cmd, bool) {
	// Tab switching always works regardless of which pane is focused.
	switch msg.String() {
	case "1":
		if a.active != screenLogin {
			a.cmail = a.cmail.CancelSubscription()
			a.active = screenFeed
			if !a.feed.IsLoaded() {
				a.feed = a.feed.SetFetching()
				return a, a.loadFeedCmd(), true
			}
			return a, nil, true
		}
	case "2":
		if a.active != screenLogin {
			a.cmail = a.cmail.CancelSubscription()
			a.active = screenNotifications
			if !a.notifications.HasPaginated() {
				a.notifications = a.notifications.SetFetching()
				return a, a.loadNotifsCmd(), true
			}
			return a, nil, true
		}
	case "3":
		if a.active != screenLogin {
			a.cmail = a.cmail.CancelSubscription()
			a.active = screenJournal
			a.journal = a.journal.SetFetching()
			return a, a.loadJournalCmd(), true
		}
	case "4":
		if a.active != screenLogin {
			a.cmail = a.cmail.CancelSubscription()
			a.active = screenBookmarks
			if !a.bookmarks.IsLoaded() {
				a.bookmarks = a.bookmarks.SetFetching()
				return a, a.loadBookmarksCmd(""), true
			}
			return a, nil, true
		}
	case "5":
		if a.active != screenLogin {
			a.cmail = a.cmail.CancelSubscription()
			a.active = screenGuilds
			if !a.guilds.IsLoaded() {
				a.guilds = a.guilds.SetFetching()
				return a, a.loadGuildsCmd(""), true
			}
			return a, nil, true
		}
	case "6":
		if a.active != screenLogin {
			a.cmail = a.cmail.CancelSubscription()
			a.active = screenTopics
			if !a.topics.IsLoaded() {
				a.topics = a.topics.SetFetching()
				return a, a.loadTopicsCmd(), true
			}
			return a, nil, true
		}
	case "7":
		if a.active != screenLogin {
			a.cmail = a.cmail.CancelSubscription()
			a.active = screenProfile
			return a, a.loadProfileCmd(), true
		}
	case "8":
		if a.active != screenLogin {
			a.cmail = a.cmail.CancelSubscription()
			a.active = screenSettings
			return a, nil, true
		}
	}

	// focusMenu is the app's initial/reset state; treat it as focusList in Miller.
	if a.focus == focusMenu || a.focus == focusList {
		switch msg.String() {
		case "l", "right", "enter":
			if l.activeCompactRenderer(a) != nil {
				a.focus = focusDetail
				return a, nil, true
			}
		}
		return a, nil, false
	}

	// Reading pane focused (3-pane Miller).
	if a.focus == focusDetail {
		paneH := a.height - theme.ChromeHeight
		_, paneW := l.paneWidths(a.width)
		switch msg.String() {
		case "h", "left":
			a.focus = focusList
			return a, nil, true
		case "j", "down":
			switch a.active {
			case screenGuilds:
				ph, pw := paneH, paneW
				return a, func() tea.Msg { return screens.GuildThreadNavMsg{Delta: +1, PaneHeight: ph, PaneWidth: pw} }, true
			case screenTopics:
				ph, pw := paneH, paneW
				return a, func() tea.Msg { return screens.TopicThreadNavMsg{Delta: +1, PaneHeight: ph, PaneWidth: pw} }, true
			default:
				ph, pw := paneH, paneW
				return a, func() tea.Msg { return screens.FeedDetailNavMsg{Delta: +1, PaneHeight: ph, PaneWidth: pw} }, true
			}
		case "k", "up":
			switch a.active {
			case screenGuilds:
				ph, pw := paneH, paneW
				return a, func() tea.Msg { return screens.GuildThreadNavMsg{Delta: -1, PaneHeight: ph, PaneWidth: pw} }, true
			case screenTopics:
				ph, pw := paneH, paneW
				return a, func() tea.Msg { return screens.TopicThreadNavMsg{Delta: -1, PaneHeight: ph, PaneWidth: pw} }, true
			default:
				ph, pw := paneH, paneW
				return a, func() tea.Msg { return screens.FeedDetailNavMsg{Delta: -1, PaneHeight: ph, PaneWidth: pw} }, true
			}
		}
		// enter, r, n, etc. fall through to DelegateUpdate
		return a, nil, false
	}

	return a, nil, false
}

// DelegateUpdate routes a tea.Msg to the currently active screen model.
func (l MillerLayout) DelegateUpdate(msg tea.Msg, a App) (App, tea.Cmd) {
	return delegateScreenUpdate(msg, a)
}

func (l MillerLayout) HasFocusedInput(a App) bool {
	switch a.active {
	case screenChatrooms:
		return a.chatrooms.InputFocused()
	case screenCMail:
		return a.cmail.InputFocused()
	case screenPostDetail:
		return a.postDetail.ComposeActive()
	case screenFeed:
		return a.feed.ComposeActive()
	case screenGuilds:
		return a.guilds.ComposeActive()
	case screenProfile:
		return a.profile.ComposeActive()
	case screenJournal:
		return a.journal.ComposeActive()
	}
	return false
}

func (l MillerLayout) ContentWidth(termWidth int) int { return termWidth }

// ContentHeight passes the terminal height unchanged; Miller now uses the same
// 3-row chrome as TabsLayout (tab bar + column header + status bar = ChromeHeight).
func (l MillerLayout) ContentHeight(termHeight int) int { return termHeight }

func (l MillerLayout) renderTabBar(a App) string {
	var tabs string
	for _, t := range menuTabs {
		label := t.label
		if t.s == screenNotifications && a.polledUnreadCount > 0 {
			label = fmt.Sprintf("%s (%d)", label, a.polledUnreadCount)
		}
		if a.active == t.s {
			tabs += theme.ActiveTab.Render(label)
		} else {
			tabs += theme.Tab.Render(label)
		}
	}
	logo := lipgloss.NewStyle().
		Background(theme.ColorGreen).
		Foreground(theme.ColorBackground).
		Bold(true).
		Padding(0, 1).
		Render(a.logoText)
	spacer := strings.Repeat(" ", max(0, a.width-lipgloss.Width(tabs)-lipgloss.Width(logo)))
	return tabs + spacer + logo
}

func (l MillerLayout) renderColumnHeader(title string, active bool, width int) string {
	if active {
		return theme.Highlight.Width(width).Render(title)
	}
	return theme.Subtle.Width(width).Render(title)
}

func (l MillerLayout) screenTitle(a App) string {
	switch a.active {
	case screenFeed:
		return "posts"
	case screenNotifications:
		return "notifs"
	case screenJournal:
		return "journal"
	case screenBookmarks:
		return "bookmarks"
	case screenGuilds:
		return "guilds"
	case screenTopics:
		return "topics"
	case screenProfile:
		return "profile"
	case screenSettings:
		return "settings"
	case screenCMail:
		return "c-mail"
	case screenChatrooms:
		return "chat"
	case screenPostDetail:
		return "thread"
	}
	return ""
}

func (l MillerLayout) renderContent(a App) string {
	switch a.active {
	case screenFeed:
		return a.feed.View()
	case screenChatrooms:
		return a.chatrooms.View()
	case screenCMail:
		return a.cmail.View()
	case screenProfile:
		return a.profile.View()
	case screenPostDetail:
		return a.postDetail.View()
	case screenNotifications:
		return a.notifications.View()
	case screenSettings:
		return a.settingsScreen.View()
	case screenBookmarks:
		return a.bookmarks.View()
	case screenGuilds:
		return a.guilds.View()
	case screenTopics:
		return a.topics.View()
	case screenJournal:
		return a.journal.View()
	}
	return ""
}

func (l MillerLayout) renderBottomBar(a App) string {
	if a.notifyText == "" {
		return l.renderStatusBar(a)
	}
	return l.renderNotification(a)
}

func (l MillerLayout) renderNotification(a App) string {
	color := theme.ColorGreen
	prefix := "✓ "
	switch a.notifyLevel {
	case notifyWarn:
		color = theme.ColorYellow
		prefix = "! "
	case notifyError:
		color = theme.ColorRed
		prefix = "✕ "
	}
	const suffix = "  (any key to dismiss)"
	budget := a.width - lipgloss.Width(prefix) - lipgloss.Width(suffix) - 2
	text := ansi.Truncate(a.notifyText, max(0, budget), "…")
	return lipgloss.NewStyle().
		Foreground(theme.ColorBackground).
		Background(color).
		Bold(true).
		Width(a.width).
		Padding(0, 1).
		Render(prefix + text + suffix)
}

func (l MillerLayout) screenHints(a App) []hint {
	switch a.focus {
	case focusDetail:
		return []hint{{"h/←", "list"}, {"j/k", "replies"}, {"↵", "thread"}, {"r", "reply"}}
	default: // focusList / focusMenu
		return append([]hint{{"→/↵", "preview"}}, TabsLayout{}.screenHints(a)...)
	}
}

func (l MillerLayout) renderStatusBar(a App) string {
	user := sbStyle().Foreground(theme.ColorCyan).Bold(true)
	meta := sbStyle().Foreground(theme.ColorWhite)
	sep := sbStyle().Foreground(theme.ColorMuted).Render(" · ")

	densityLabel := "dense"
	if a.relaxed {
		densityLabel = "relaxed"
	}
	tzLabel := a.timezone
	if tzLabel == "" {
		tzLabel = "UTC"
	}

	username := user.Render("@" + a.currentUser.Username)
	infoItems := []string{
		sep + meta.Render(densityLabel),
		sep + meta.Render(tzLabel),
		sep + meta.Render("miller"),
	}

	hints := l.screenHints(a)
	const barPad = 2

	measure := func(numInfo, numHints int) int {
		left := lipgloss.Width(username)
		for _, item := range infoItems[:numInfo] {
			left += lipgloss.Width(item)
		}
		right := lipgloss.Width(renderHints(hints[:numHints]))
		return left + right + barPad
	}

	numInfo := len(infoItems)
	numHints := len(hints)
	for numInfo >= 0 {
		if measure(numInfo, numHints) <= a.width {
			break
		}
		numInfo--
	}
	if numInfo < 0 {
		numInfo = 0
		for numHints > 1 && measure(0, numHints) > a.width {
			numHints--
		}
	}

	bg := sbStyle()
	leftParts := []string{username}
	leftParts = append(leftParts, infoItems[:numInfo]...)
	left := lipgloss.JoinHorizontal(lipgloss.Top, leftParts...)
	right := renderHints(hints[:numHints])
	spacer := bg.Width(max(0, a.width-lipgloss.Width(left)-lipgloss.Width(right)-barPad)).Render("")
	bar := lipgloss.JoinHorizontal(lipgloss.Top, left, spacer, right)
	return bg.Padding(0, 1).Render(bar)
}

func (l MillerLayout) renderThemePicker(a App) string {
	var rows []string
	for i, name := range availableThemes {
		if i == a.themePickerCursor {
			rows = append(rows, theme.Highlight.Render("▸ "+name))
		} else {
			rows = append(rows, theme.Subtle.Render("  "+name))
		}
	}
	body := lipgloss.JoinVertical(lipgloss.Left,
		theme.Title.Render("theme"),
		"",
		strings.Join(rows, "\n"),
		"",
		theme.Subtle.Render("↑↓ select   enter apply   esc cancel"),
	)
	return theme.ActiveBorder.Render(body)
}

func (l MillerLayout) renderHelpModal(_ App) string {
	title := theme.Title.Render("shortcuts")
	sectionStyle := theme.Subtle.Bold(true)
	row := func(key, desc string) string {
		k := theme.Highlight.Render(fmt.Sprintf("%-14s", key))
		return lipgloss.JoinHorizontal(lipgloss.Top, k, theme.Subtle.Render(desc))
	}

	globalSection := lipgloss.JoinVertical(lipgloss.Left,
		sectionStyle.Render("global"),
		row("1-8", "jump to section"),
		row("l / enter", "enter detail pane"),
		row("h", "return to list pane"),
		row("t", "theme"),
		row("v", "density"),
		row("o", "open url"),
		row("q", "quit"),
	)

	body := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		globalSection,
		"",
		theme.Subtle.Render("any key · close"),
		renderedVersionLine,
	)
	return theme.ActiveBorder.Render(body)
}

func (l MillerLayout) renderURLPicker(a App) string {
	title := theme.Title.Render("open url")
	items := make([]string, len(a.urlPickerItems))
	for i, u := range a.urlPickerItems {
		display := u
		if len(display) > 60 {
			display = display[:57] + "..."
		}
		if i == a.urlPickerCursor {
			items[i] = theme.Highlight.Render("▸ " + display)
		} else {
			items[i] = theme.Subtle.Render("  " + display)
		}
	}
	hint := theme.Subtle.Render("↑↓ select   enter open   esc cancel")
	rows := append([]string{title, ""}, items...)
	rows = append(rows, "", hint)
	return theme.ActiveBorder.Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
}

func (l MillerLayout) renderImageModal(a App) string {
	blankLine := strings.Repeat(" ", a.imageModalCols)
	lines := make([]string, a.imageModalRows)
	for i := range lines {
		lines[i] = blankLine
	}
	return theme.ActiveBorder.Render(strings.Join(lines, "\n"))
}
