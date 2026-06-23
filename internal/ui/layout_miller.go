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

const millerSidebarWidth = 22  // nav pane (21 chars) + "│" separator (1 char)
const millerListWidth = 42     // compact post list pane width in 3-pane Feed view
const millerHeaderHeight = 1   // column title row at the top of the layout

// MillerLayout renders a left navigation sidebar alongside the active screen.
type MillerLayout struct{}

func (l MillerLayout) View(a App) string {
	if a.active == screenLogin {
		return a.login.View()
	}

	contentH := a.height - 1 - millerHeaderHeight // full height minus bottom bar and column header
	contentW := a.width - millerSidebarWidth
	navPane := lipgloss.NewStyle().Height(contentH).MaxHeight(contentH).Render(l.renderNav(a))

	// Column header row — active column title in accent, others muted.
	navHdr := l.renderColumnHeader("spaces", a.focus == focusMenu, millerSidebarWidth-1)
	colSep := theme.Subtle.Render("│")

	var contentPane, hdrRow string
	if a.active == screenFeed {
		listW := millerListWidth
		detailW := contentW - listW - 1

		listHdr := l.renderColumnHeader("posts", a.focus == focusList, listW)
		detailHdr := l.renderColumnHeader("thread", a.focus == focusDetail, detailW)
		hdrRow = lipgloss.JoinHorizontal(lipgloss.Top, navHdr, colSep, listHdr, colSep, detailHdr)

		listP := lipgloss.NewStyle().Width(listW).Height(contentH).MaxHeight(contentH).
			Render(a.feed.CompactListView(listW, contentH))
		listSep := theme.Subtle.Render(strings.TrimSuffix(strings.Repeat("│\n", contentH), "\n"))
		detailP := lipgloss.NewStyle().Width(detailW).Height(contentH).MaxHeight(contentH).
			Render(a.feed.DetailView(detailW, contentH))
		contentPane = lipgloss.JoinHorizontal(lipgloss.Top, listP, listSep, detailP)
	} else if a.active == screenGuilds && a.guilds.IsViewingGuildPosts() {
		listW := millerListWidth
		detailW := contentW - listW - 1

		listHdr := l.renderColumnHeader("posts", a.focus == focusList, listW)
		detailHdr := l.renderColumnHeader("thread", a.focus == focusDetail, detailW)
		hdrRow = lipgloss.JoinHorizontal(lipgloss.Top, navHdr, colSep, listHdr, colSep, detailHdr)

		listP := lipgloss.NewStyle().Width(listW).Height(contentH).MaxHeight(contentH).
			Render(a.guilds.CompactListView(listW, contentH))
		listSep := theme.Subtle.Render(strings.TrimSuffix(strings.Repeat("│\n", contentH), "\n"))
		detailP := lipgloss.NewStyle().Width(detailW).Height(contentH).MaxHeight(contentH).
			Render(a.guilds.DetailView(detailW, contentH))
		contentPane = lipgloss.JoinHorizontal(lipgloss.Top, listP, listSep, detailP)
	} else if a.active == screenTopics && a.topics.IsViewingTopicPosts() {
		listW := millerListWidth
		detailW := contentW - listW - 1

		listHdr := l.renderColumnHeader("posts", a.focus == focusList, listW)
		detailHdr := l.renderColumnHeader("thread", a.focus == focusDetail, detailW)
		hdrRow = lipgloss.JoinHorizontal(lipgloss.Top, navHdr, colSep, listHdr, colSep, detailHdr)

		listP := lipgloss.NewStyle().Width(listW).Height(contentH).MaxHeight(contentH).
			Render(a.topics.CompactListView(listW, contentH))
		listSep := theme.Subtle.Render(strings.TrimSuffix(strings.Repeat("│\n", contentH), "\n"))
		detailP := lipgloss.NewStyle().Width(detailW).Height(contentH).MaxHeight(contentH).
			Render(a.topics.DetailView(detailW, contentH))
		contentPane = lipgloss.JoinHorizontal(lipgloss.Top, listP, listSep, detailP)
	} else {
		contentHdr := l.renderColumnHeader(l.screenTitle(a), a.focus != focusMenu, contentW)
		hdrRow = lipgloss.JoinHorizontal(lipgloss.Top, navHdr, colSep, contentHdr)
		contentPane = lipgloss.NewStyle().Width(contentW).Height(contentH).MaxHeight(contentH).Render(l.renderContent(a))
	}

	sep := theme.Subtle.Render(strings.TrimSuffix(strings.Repeat("│\n", contentH), "\n"))
	base := lipgloss.JoinVertical(lipgloss.Left,
		hdrRow,
		lipgloss.JoinHorizontal(lipgloss.Top, navPane, sep, contentPane),
		l.renderBottomBar(a),
	)

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
		if xOff < 0 {
			xOff = 0
		}
		if yOff < 0 {
			yOff = 0
		}
		imgRow := yOff + 2
		imgCol := xOff + 3
		return composed + fmt.Sprintf("\x1b[%d;%dH%s\x1b[%d;1H", imgRow, imgCol, a.imageModalEncoded, a.height)
	}
	if a.imageNeedsCleanup && a.graphicsProtocol == imgview.ProtocolKitty {
		modalH := a.imageModalRows + 2
		yOff := (a.height - modalH) / 2
		if yOff < 0 {
			yOff = 0
		}
		lines := strings.Split(base, "\n")
		if yOff < len(lines) {
			lines[yOff] = "\x1b_Ga=d,d=A\x1b\\" + lines[yOff]
		}
		return strings.Join(lines, "\n")
	}
	return base
}

func (l MillerLayout) HandleNav(msg tea.KeyMsg, a App) (App, tea.Cmd, bool) {
	if a.focus == focusMenu {
		switch msg.String() {
		case "j", "down":
			if a.active != screenLogin {
				var cmd tea.Cmd
				a, cmd = navigateTabBy(a, +1)
				return a, cmd, true
			}
		case "k", "up":
			if a.active != screenLogin {
				var cmd tea.Cmd
				a, cmd = navigateTabBy(a, -1)
				return a, cmd, true
			}
		case "l", "right", "enter":
			if a.active != screenLogin {
				a.focus = focusList
				return a, nil, true
			}
		case "1":
			if a.active != screenLogin {
				a.cmail = a.cmail.CancelSubscription()
				a.active = screenFeed
				a.feed = a.feed.SetFetching()
				return a, a.loadFeedCmd(), true
			}
		case "2":
			if a.active != screenLogin {
				a.cmail = a.cmail.CancelSubscription()
				a.active = screenNotifications
				a.notifications = a.notifications.SetFetching()
				return a, a.loadNotifsCmd(), true
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
		return a, nil, false
	}

	// List pane focused (Feed/Guilds/Topics 3-pane or other screens).
	if a.focus == focusList {
		switch msg.String() {
		case "h", "left":
			a.focus = focusMenu
			return a, nil, true
		case "l", "right", "enter":
			if a.active == screenFeed ||
				(a.active == screenGuilds && a.guilds.IsViewingGuildPosts()) ||
				(a.active == screenTopics && a.topics.IsViewingTopicPosts()) {
				a.focus = focusDetail
				return a, nil, true
			}
		}
		return a, nil, false
	}

	// Reading pane focused (3-pane Miller).
	if a.focus == focusDetail {
		paneH := a.height - 1 - millerHeaderHeight
		paneW := (a.width - millerSidebarWidth) - millerListWidth - 1
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

func (l MillerLayout) DelegateUpdate(msg tea.Msg, a App) (App, tea.Cmd) {
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

func (l MillerLayout) HasFocusedInput(a App) bool {
	if a.focus == focusMenu {
		return false
	}
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

func (l MillerLayout) ContentWidth(termWidth int) int { return termWidth - millerSidebarWidth }

// ContentHeight inflates the height sent to screens so their viewport (which subtracts
// theme.ChromeHeight = 3) fills the content pane exactly. Miller layout uses 2 chrome rows
// (column header + status bar), so we add back only 1 of the 2 rows TabsLayout uses.
func (l MillerLayout) ContentHeight(termHeight int) int {
	return termHeight + theme.TabBarHeight
}

func (l MillerLayout) renderNav(a App) string {
	navW := millerSidebarWidth - 1 // leave 1 col for the "│" separator

	var rows []string
	for _, t := range menuTabs {
		label := t.label
		if t.s == screenNotifications && a.polledUnreadCount > 0 {
			label = fmt.Sprintf("%s ●%d", label, a.polledUnreadCount)
		}
		var row string
		if a.active == t.s {
			if a.focus == focusMenu {
				row = theme.Highlight.Width(navW).Render("▶ " + label)
			} else {
				row = theme.Subtle.Width(navW).Render("▶ " + label)
			}
		} else {
			row = theme.Subtle.Width(navW).Render("  " + label)
		}
		rows = append(rows, row)
	}
	return strings.Join(rows, "\n")
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
	case focusMenu:
		return []hint{{"j/k", "nav"}, {"l/↵", "enter"}, {"1-8", "jump"}, {"?", "more"}}
	case focusDetail:
		return []hint{{"h/←", "list"}, {"j/k", "replies"}, {"↵", "thread"}, {"r", "reply"}}
	default: // focusList
		return append([]hint{{"h/←", "menu"}, {"→/↵", "preview"}}, TabsLayout{}.screenHints(a)...)
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

func (l MillerLayout) renderHelpModal(a App) string {
	title := theme.Title.Render("shortcuts")
	sectionStyle := theme.Subtle.Bold(true)
	row := func(key, desc string) string {
		k := theme.Highlight.Render(fmt.Sprintf("%-14s", key))
		return lipgloss.JoinHorizontal(lipgloss.Top, k, theme.Subtle.Render(desc))
	}

	globalSection := lipgloss.JoinVertical(lipgloss.Left,
		sectionStyle.Render("global"),
		row("j/k", "move nav · select section"),
		row("l / enter", "enter content pane"),
		row("h", "return to nav pane"),
		row("1-8", "jump to section"),
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
