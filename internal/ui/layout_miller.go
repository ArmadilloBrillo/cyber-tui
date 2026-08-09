package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/ragnar/cyber-tui/internal/ui/screens"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
)

const millerSidebarWidth = 22 // nav pane (21 chars) + "│" separator (1 char)
const millerListMaxWidth = 70 // hard cap on the list pane; above this, excess goes to the detail pane
const millerHeaderHeight = 1  // column title row at the top of the layout

// MillerLayout renders a left navigation sidebar alongside the active screen.
type MillerLayout struct{}

// paneWidths returns the list and detail column widths for the given content area.
// The detail pane is pinned at its preferred width (45); the list takes remaining
// space and collapses first when narrowing. Above millerListMaxWidth the excess
// goes to the detail pane. Add a similar method to any future multi-pane layout
// to keep its collapsing logic self-contained.
func (l MillerLayout) paneWidths(contentW int) (listW, detailW int) {
	const preferredDetailW = 45 // detail width at ~120-col terminal (98 contentW - 52 list - 1 sep)
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
	return termHeight - 2 // millerHeaderHeight + status bar
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
	contentH := a.height - 1 - millerHeaderHeight // full height minus bottom bar and column header
	contentW := a.width - millerSidebarWidth

	// Column header row — active column title in accent, others muted.
	navHdr := l.renderColumnHeader("spaces", a.focus == focusMenu, millerSidebarWidth-1)
	colSep := theme.Subtle.Render("│")

	logo := lipgloss.NewStyle().
		Background(theme.ColorGreen).
		Foreground(theme.ColorBackground).
		Bold(true).
		Padding(0, 1).
		Render(a.logoText)
	logoW := lipgloss.Width(logo)

	var contentPane, hdrRow, composeBar string
	if r := l.activeCompactRenderer(a); r != nil {
		listW, detailW := l.paneWidths(contentW)

		// If compose panel is active, pull it out of DetailView and render it as a
		// full-width bar spanning the list and detail columns (above the status bar).
		if cc, ok := r.(CompactComposer); ok && cc.ComposeActive() {
			contentH = max(0, contentH-cc.ComposeHeight())
			composeBar = cc.ComposeView(contentW)
		}

		listHdr := l.renderColumnHeader(r.ListTitle(), a.focus == focusList, listW)
		detailHdr := l.renderColumnHeader("thread", a.focus == focusDetail, detailW-logoW)
		hdrRow = lipgloss.JoinHorizontal(lipgloss.Top, navHdr, colSep, listHdr, colSep, detailHdr) + logo

		listP := lipgloss.NewStyle().Width(listW).Height(contentH).MaxHeight(contentH).
			Render(r.CompactListView(listW, contentH))
		listSep := theme.Subtle.Render(strings.TrimSuffix(strings.Repeat("│\n", contentH), "\n"))
		detailP := lipgloss.NewStyle().Width(detailW).Height(contentH).MaxHeight(contentH).
			Render(r.DetailView(detailW, contentH))
		contentPane = lipgloss.JoinHorizontal(lipgloss.Top, listP, listSep, detailP)
	} else {
		contentHdr := l.renderColumnHeader(l.screenTitle(a), a.focus != focusMenu, contentW-logoW)
		hdrRow = lipgloss.JoinHorizontal(lipgloss.Top, navHdr, colSep, contentHdr) + logo
		contentPane = lipgloss.NewStyle().Width(contentW).Height(contentH).MaxHeight(contentH).Render(l.renderContent(a))
	}

	// navPane and sep use the (possibly compose-reduced) contentH.
	navPane := lipgloss.NewStyle().Height(contentH).MaxHeight(contentH).Render(l.renderNav(a))
	sep := theme.Subtle.Render(strings.TrimSuffix(strings.Repeat("│\n", contentH), "\n"))
	mainRow := lipgloss.JoinHorizontal(lipgloss.Top, navPane, sep, contentPane)

	var base string
	if composeBar != "" {
		panelH := lipgloss.Height(composeBar)
		composeSep := theme.Subtle.Render(strings.TrimSuffix(strings.Repeat("│\n", panelH), "\n"))
		sideBlank := lipgloss.NewStyle().Width(millerSidebarWidth - 1).Render("")
		composeRow := lipgloss.JoinHorizontal(lipgloss.Top, sideBlank, composeSep, composeBar)
		base = lipgloss.JoinVertical(lipgloss.Left, hdrRow, mainRow, composeRow, l.renderBottomBar(a))
	} else {
		base = lipgloss.JoinVertical(lipgloss.Left, hdrRow, mainRow, l.renderBottomBar(a))
	}

	return compositeOverlays(l, a, base)
}

// InlineImageSlots returns the visible inline-image slots for whichever
// screen is active, plus this layout's screen origin for them — which
// depends on whether the active screen is shown via the compact-list/detail
// split (Feed) or the plain content pane (PostDetail), since the detail
// pane's left edge sits further right when a list pane precedes it. This
// replicates the exact width/height View() computes for the same screen, so
// a slot's position always matches what's actually on screen this frame —
// Feed's detail pane in particular has no other source of truth for its
// current width, since it's derived fresh from paneWidths on every View()
// call rather than stored anywhere Update() can see.
func (l MillerLayout) InlineImageSlots(a App) ([]screens.InlineImageSlot, int, int) {
	const rowOrigin = 2 // 1: header row, so content's own row 0 is ANSI row 2
	contentH := a.height - 1 - millerHeaderHeight
	contentW := a.width - millerSidebarWidth

	if r := l.activeCompactRenderer(a); r != nil {
		if a.active != screenFeed {
			return nil, 0, 0
		}
		listW, detailW := l.paneWidths(contentW)
		if cc, ok := r.(CompactComposer); ok && cc.ComposeActive() {
			contentH = max(0, contentH-cc.ComposeHeight())
		}
		colOrigin := millerSidebarWidth + 1 + listW + 1
		return a.feed.VisibleDetailInlineImages(detailW, contentH), rowOrigin, colOrigin
	}
	if a.active == screenPostDetail {
		return a.postDetail.VisibleInlineImages(), rowOrigin, millerSidebarWidth + 1
	}
	return nil, 0, 0
}

func (l MillerLayout) HandleNav(msg tea.KeyMsg, a App) (App, tea.Cmd, bool) {
	if a.focus == focusMenu {
		if a.active == screenLogin {
			return a, nil, false
		}
		if s, ok := screenForNumber(msg.String()); ok {
			var cmd tea.Cmd
			a, cmd = activateScreen(a, s)
			return a, cmd, true
		}
		switch msg.String() {
		case "j", "down":
			var cmd tea.Cmd
			a, cmd = navigateTabBy(a, +1)
			return a, cmd, true
		case "k", "up":
			var cmd tea.Cmd
			a, cmd = navigateTabBy(a, -1)
			return a, cmd, true
		case "l", "right", "enter":
			a.focus = focusList
			return a, nil, true
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
			if l.activeCompactRenderer(a) != nil {
				a.focus = focusDetail
				return a, nil, true
			}
		}
		return a, nil, false
	}

	// Reading pane focused (3-pane Miller).
	if a.focus == focusDetail {
		paneH := a.height - 1 - millerHeaderHeight
		_, paneW := l.paneWidths(a.width - millerSidebarWidth)
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
	case screenSearch:
		return a.search.InputFocused()
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
	for _, t := range visibleTabs() {
		badge := ""
		if t.s == screenNotifications && a.polledUnreadCount > 0 {
			badge = fmt.Sprintf(" ●%d", a.polledUnreadCount)
		}
		if t.s == screenFeed {
			if n := a.feed.PendingNewCount(); n > 0 {
				badge = fmt.Sprintf(" ●%d", n)
			}
		}
		if t.s == screenCMail {
			if n := a.cmail.TotalUnread(); n > 0 {
				badge = fmt.Sprintf(" ●%d", n)
			}
		}
		if t.s == screenChatrooms {
			if n := a.chatrooms.UnreadCount(); n > 0 {
				badge = fmt.Sprintf(" ●%d", n)
			}
		}
		selected, detail := tabVisualState(a, t.s)
		// ▷ (open) marks "one level deep" — a Circ room, Guilds/Topics
		// browse, a C-Mail conversation, or a PostDetail opened from this
		// tab (see tabVisualState) — vs. ▶ for selected-at-the-top-level, or
		// no marker at all. Mirrors the trailing "›" in TabsLayout's
		// renderTabBar. detail can be true while unselected (Circ/Guilds/
		// Topics persist in the background); base only brightens when
		// selected, so a backgrounded ▷ renders dim — "open elsewhere",
		// distinct from the bright ▷ meaning "open, and you're looking at it".
		marker := "  "
		switch {
		case detail:
			marker = "▷ "
		case selected:
			marker = "▶ "
		}
		base := theme.Subtle
		if selected && a.focus == focusMenu {
			base = theme.Highlight
		}
		before, ch, after := splitMnemonic(t.label, t.mnemonic)
		// No background is set on base/mnemonic here, so — unlike renderTabBar —
		// wrapping the whole nested-ANSI string in one Width call is safe: the
		// worst a lost style could do is leave trailing padding spaces
		// uncolored, which is invisible.
		text := base.Render(marker+before) + theme.NavMnemonic.Render(ch) + base.Render(after+badge)
		rows = append(rows, lipgloss.NewStyle().Width(navW).Render(text))
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
	case screenSearch:
		return "search"
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
	case screenSearch:
		return a.search.View()
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
		return []hint{{"j/k", "nav"}, {"l/↵", "enter"}, {"1-9 / g+", "jump"}, {"?", "more"}}
	case focusDetail:
		hints := []hint{{"h/←", "list"}, {"j/k", "replies"}, {"↵", "thread"}, {"r", "reply"}}
		if a.active == screenPostDetail && a.postDetail.HasTheme() {
			hints = append(hints, hint{"T", "try theme"})
		}
		return hints
	default: // focusList
		return append([]hint{{"h/←", "menu"}, {"→/↵", "preview"}}, TabsLayout{}.screenHints(a)...)
	}
}

func (l MillerLayout) renderStatusBar(a App) string {
	user := sbStyle().Foreground(theme.ColorCyan).Bold(true)
	meta := sbStyle().Foreground(theme.ColorMeta)
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
		theme.Subtle.Render("↑↓ select   enter apply   e edit   x export   i import   esc cancel"),
	)
	return theme.ActiveBorder.Render(body)
}

// renderThemeEditor renders the "custom" theme color editor modal.
func (l MillerLayout) renderThemeEditor(a App) string {
	return a.themeEditor.View()
}

// renderPathPrompt renders the export/import file-path prompt modal.
func (l MillerLayout) renderPathPrompt(a App) string {
	return a.pathPrompt.View()
}

func (l MillerLayout) renderHelpModal(a App) string {
	title := theme.Title.Render("shortcuts")
	sectionStyle := theme.Subtle.Bold(true)
	row := func(key, desc string) string {
		k := theme.Highlight.Render(fmt.Sprintf("%-14s", key))
		return lipgloss.JoinHorizontal(lipgloss.Top, k, theme.Subtle.Render(desc))
	}

	globalRows := append([]string{
		sectionStyle.Render("global"),
		row("j/k", "move nav · select section"),
		row("l / enter", "enter content pane"),
		row("h", "return to nav pane"),
		row("1-9", "jump to section"),
	}, leaderRows(row)...)
	globalRows = append(globalRows,
		row("/", "search"),
		row("t", "theme"),
		row("e", "edit custom theme (in theme picker)"),
		row("x / i", "export / import custom theme (in theme picker)"),
		row("v", "density"),
		row("o", "open url"),
		row("q", "quit"),
	)
	globalSection := lipgloss.JoinVertical(lipgloss.Left, globalRows...)

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
