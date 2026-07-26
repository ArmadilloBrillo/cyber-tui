package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/ragnar/cyber-tui/internal/ui/imgview"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
)

// hintRows converts hints to modal row strings, skipping the "?" entry.
// "↑↓" is expanded to "↑↓ / j/k" so the modal documents both navigation styles.
func hintRows(hints []hint, rowFn func(string, string) string) []string {
	rows := make([]string, 0, len(hints))
	for _, h := range hints {
		if h.key == "?" {
			continue
		}
		key := h.key
		if key == "↑↓" {
			key = "↑↓ / j/k"
		}
		rows = append(rows, rowFn(key, h.desc))
	}
	return rows
}

// TabsLayout implements the classic horizontal tab bar layout.
type TabsLayout struct{}

func (l TabsLayout) NeedsCompactAutoFill(termHeight int) int { return 0 }

// View renders the full terminal output for the tabs layout.
func (l TabsLayout) View(a App) string {
	if a.active == screenLogin {
		return a.login.View()
	}
	contentHeight := a.height - theme.ChromeHeight
	content := lipgloss.NewStyle().Height(contentHeight).MaxHeight(contentHeight).Render(l.renderActiveScreen(a))
	base := lipgloss.JoinVertical(lipgloss.Left,
		l.renderTabBar(a),
		"",
		content,
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
		// Compute the same offsets overlayCenter used so we can position
		// the image sequence inside the border without embedding it in the
		// overlay string (which would corrupt overlayCenter's ANSI splicing).
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
		// theme.ActiveBorder: 1-char border + 1-char horizontal padding on each
		// side. Image content therefore starts 2 cols right of the border edge.
		// ANSI cursor sequences are 1-indexed; the border top row is yOff+1.
		imgRow := yOff + 2
		imgCol := xOff + 3
		return composed + fmt.Sprintf("\x1b[%d;%dH%s\x1b[%d;1H", imgRow, imgCol, a.imageModalEncoded, a.height)
	}
	if a.imageNeedsCleanup && a.graphicsProtocol == imgview.ProtocolKitty {
		// Inject the Kitty delete-all command onto the line that held the modal's
		// top border so Bubble Tea's diff renderer delivers it to the terminal.
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

// HandleNav processes navigation key presses for the tabs layout: the "1"-"9"
// numeric aliases and left/right arrow tab cycling. The "g"+mnemonic leader
// chords are handled globally in app.go's handleKeys, ahead of HandleNav,
// since they apply identically regardless of which layout is active.
func (l TabsLayout) HandleNav(msg tea.KeyMsg, a App) (App, tea.Cmd, bool) {
	if a.active == screenLogin {
		return a, nil, false
	}
	if s, ok := screenForNumber(msg.String()); ok {
		var cmd tea.Cmd
		a, cmd = activateScreen(a, s)
		return a, cmd, true
	}
	switch msg.String() {
	case "left":
		if a.active != screenPostDetail && a.focus == focusMenu {
			var cmd tea.Cmd
			a, cmd = navigateTabBy(a, -1)
			return a, cmd, true
		}
	case "right":
		if a.active != screenPostDetail && a.focus == focusMenu {
			var cmd tea.Cmd
			a, cmd = navigateTabBy(a, +1)
			return a, cmd, true
		}
	}
	return a, nil, false
}

// DelegateUpdate routes a tea.Msg to the currently active screen model.
func (l TabsLayout) DelegateUpdate(msg tea.Msg, a App) (App, tea.Cmd) {
	return delegateScreenUpdate(msg, a)
}

// HasFocusedInput returns true when the active screen has a focused text input.
func (l TabsLayout) HasFocusedInput(a App) bool {
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

func (l TabsLayout) ContentWidth(termWidth int) int   { return termWidth }
func (l TabsLayout) ContentHeight(termHeight int) int { return termHeight }

func (l TabsLayout) renderTabBar(a App) string {
	var tabs string
	for _, t := range visibleTabs() {
		badge := ""
		if t.s == screenNotifications && a.polledUnreadCount > 0 {
			badge = fmt.Sprintf(" (%d)", a.polledUnreadCount)
		}
		if t.s == screenCMail {
			if n := a.cmail.TotalUnread(); n > 0 {
				badge = fmt.Sprintf(" (%d)", n)
			}
		}
		isActive := a.active == t.s &&
			!(t.s == screenCMail && a.cmail.IsShowingDetail()) &&
			!(t.s == screenChatrooms && a.chatrooms.IsShowingDetail())
		text, mnemonic := theme.TabText, theme.TabMnemonic
		if isActive {
			text, mnemonic = theme.ActiveTabText, theme.ActiveTabMnemonic
		}
		before, ch, after := splitMnemonic(t.label, t.mnemonic)
		// Each fragment is rendered independently (rather than wrapping the
		// whole label in one .Padding style) so the active tab's background
		// survives across the mnemonic's own ANSI reset — see TabText's doc.
		tabs += text.Render("  "+before) + mnemonic.Render(ch) + text.Render(after+badge+"  ")
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

func (l TabsLayout) renderActiveScreen(a App) string {
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

func (l TabsLayout) renderBottomBar(a App) string {
	if a.notifyText == "" {
		return l.renderStatusBar(a)
	}
	return l.renderNotification(a)
}

func (l TabsLayout) renderNotification(a App) string {
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

func (l TabsLayout) renderStatusBar(a App) string {
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
	timeFmt := a.settings.TimeDisplayFormat
	if timeFmt == "" {
		timeFmt = "datetime"
	}

	username := user.Render("@" + a.currentUser.Username)
	infoItems := []string{
		sep + meta.Render(densityLabel),
		sep + meta.Render(theme.CurrentName()),
		sep + meta.Render(tzLabel),
		sep + meta.Render(timeFmt),
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
	spacer := bg.Width(a.width - lipgloss.Width(left) - lipgloss.Width(right) - barPad).Render("")
	bar := lipgloss.JoinHorizontal(lipgloss.Top, left, spacer, right)
	return bg.Padding(0, 1).Render(bar)
}

func (l TabsLayout) screenHints(a App) []hint {
	more := hint{"?", "more"}
	switch a.active {
	case screenFeed:
		if a.feed.ComposeActive() {
			return []hint{{"tab", "cycle"}, {"space", "toggle"}, {"Ctrl+s", "send"}, {"Esc", "cancel"}}
		}
		return []hint{{"↑↓", "navigate"}, {"enter", "open"}, {"r", "reply"}, {"n", "new"}, {"b", "bookmark"}, {"w", "watch"}, {"c", "message"}, more}
	case screenPostDetail:
		if a.postDetail.ComposeActive() {
			return []hint{{"Ctrl+s", "send"}, {"Esc", "cancel"}}
		}
		return []hint{{"↑↓", "navigate"}, {"r", "reply"}, {"b", "bookmark"}, {"w", "watch"}, {"c", "message"}, {"esc", "back"}, more}
	case screenProfile:
		if a.profile.ComposeActive() {
			return []hint{{"Ctrl+s", "save"}, {"Esc", "cancel"}, {"tab", "cycle"}}
		}
		if a.profile.IsReadOnly() {
			return []hint{{"↑↓", "navigate"}, {"f", "follow"}, {"c", "message"}, {"tab", "cycle"}, more}
		}
		return []hint{{"↑↓", "navigate"}, {"e", "edit"}, {"tab", "cycle"}, more}
	case screenNotifications:
		return []hint{{"↑↓", "navigate"}, {"enter", "open"}, {"m", "mark read"}, {"u", "toggle unread"}, {"c", "message"}, more}
	case screenJournal:
		if a.journal.ComposeActive() {
			return []hint{{"tab", "cycle"}, {"Ctrl+s", "save"}, {"Ctrl+p", "publish"}, {"Esc", "cancel"}}
		}
		return []hint{{"↑↓", "navigate"}, {"enter", "edit"}, {"n", "new"}, {"d", "delete"}, more}
	case screenBookmarks:
		return []hint{{"↑↓", "navigate"}, {"enter", "open"}, {"d", "delete"}, more}
	case screenGuilds:
		if a.guilds.ComposeActive() {
			return []hint{{"tab", "cycle"}, {"Ctrl+s", "send"}, {"Esc", "cancel"}}
		}
		if a.guilds.IsBrowsingMembers() {
			return []hint{{"↑↓", "navigate"}, {"enter", "view profile"}, {"esc", "back"}, more}
		}
		if a.guilds.IsBrowsingGuild() {
			if a.guilds.IsConfirmingJoin() || a.guilds.IsConfirmingLeave() {
				return []hint{{"y", "confirm"}, {"n/esc", "cancel"}}
			}
			hints := []hint{{"↑↓", "navigate"}, {"enter", "open"}, {"m", "members"}, {"n", "new thread"}, {"esc", "back"}}
			d := a.guilds.GuildDetail()
			if a.guilds.IsDetailLoaded() && !d.IsMember && a.currentUser.GuildSlug == "" {
				hints = append(hints, hint{"J", "join"})
			} else if a.guilds.IsDetailLoaded() && d.IsMember && d.Role != "founder" {
				hints = append(hints, hint{"L", "leave"})
			}
			return append(hints, more)
		}
		return []hint{{"↑↓", "navigate"}, {"enter", "browse"}, more}
	case screenTopics:
		if a.topics.IsBrowsingTopic() {
			return []hint{{"↑↓", "navigate"}, {"enter", "open"}, {"esc", "back"}, more}
		}
		return []hint{{"↑↓", "navigate"}, {"enter", "browse"}, {"esc", "back"}, more}
	case screenSearch:
		if a.search.InputFocused() {
			return []hint{{"enter", "search"}}
		}
		if a.search.IsInTypeList() {
			return []hint{{"↑↓", "navigate"}, {"enter", "open"}, {"esc", "back"}, more}
		}
		return []hint{{"↑↓", "navigate"}, {"enter", "open / see all"}, {"esc", "edit query"}, more}
	case screenSettings:
		base := []hint{{"↑↓", "navigate"}, {"space", "toggle"}, {"tab", "cycle"}, more}
		if a.settingsScreen.IsDirty() {
			return append([]hint{{"Ctrl+s", "save"}, {"Esc", "revert"}}, base...)
		}
		return base
	case screenChatrooms:
		if a.chatrooms.IsShowingDetail() {
			// '?' (help) is unreachable while the compose input is focused
			// here, same as plain 'o' — omit "more" and surface ctrl+o instead.
			return []hint{{"↑↓", "scroll"}, {"enter", "send"}, {"ctrl+o", "open"}, {"esc", "back"}}
		}
		return []hint{{"↑↓/j/k", "navigate"}, {"enter", "open"}, more}
	case screenCMail:
		if a.cmail.IsShowingDetail() {
			return []hint{{"↑↓", "scroll"}, {"enter", "send"}, {"ctrl+o", "open"}, {"esc", "back"}}
		}
		return []hint{{"↑↓/j/k", "navigate"}, {"enter", "open"}, more}
	}
	return []hint{more}
}

func (l TabsLayout) renderThemePicker(a App) string {
	title := theme.Title.Render("theme")
	var items []string
	for i, name := range availableThemes {
		if i == a.themePickerCursor {
			items = append(items, theme.Highlight.Render("▸ "+name))
		} else {
			items = append(items, theme.Subtle.Render("  "+name))
		}
	}
	hint := theme.Subtle.Render("↑↓ preview   enter save   esc cancel")
	body := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		lipgloss.JoinVertical(lipgloss.Left, items...),
		"",
		hint,
	)
	return theme.ActiveBorder.Render(body)
}

func (l TabsLayout) renderHelpModal(a App) string {
	title := theme.Title.Render("shortcuts")
	sectionStyle := theme.Subtle.Bold(true)
	row := func(key, desc string) string {
		k := theme.Highlight.Render(fmt.Sprintf("%-14s", key))
		return lipgloss.JoinHorizontal(lipgloss.Top, k, theme.Subtle.Render(desc))
	}

	globalRows := append([]string{
		sectionStyle.Render("global"),
		row("1-9", "feed · notifs · c-mail · circ · journal · bookmarks · guilds · topics · profile"),
	}, leaderRows(row)...)
	globalRows = append(globalRows,
		row("← →", "cycle tabs"),
		row("/", "search"),
		row("t", "theme"),
		row("v", "density"),
		row("o", "open url"),
		row("q", "quit"),
	)
	globalSection := lipgloss.JoinVertical(lipgloss.Left, globalRows...)

	section := func(title string, extra ...string) string {
		parts := append([]string{sectionStyle.Render(title)}, hintRows(l.screenHints(a), row)...)
		parts = append(parts, extra...)
		return lipgloss.JoinVertical(lipgloss.Left, parts...)
	}

	var localSection string
	switch a.active {
	case screenFeed:
		if a.feed.ComposeActive() {
			localSection = section("feed (compose)", row("Enter", "paragraph"))
		} else {
			localSection = section("feed",
				row("p", "view profile"),
				row("c", "message"),
				row("d", "delete own"),
			)
		}
	case screenPostDetail:
		if a.postDetail.ComposeActive() {
			localSection = section("post detail (compose)", row("Enter", "paragraph"))
		} else {
			localSection = section("post detail",
				row("d", "delete own"),
				row("p", "view profile"),
				row("c", "message"),
			)
		}
	case screenProfile:
		if a.profile.ComposeActive() {
			localSection = section("profile (editing)")
		} else if a.profile.IsReadOnly() {
			localSection = section("profile",
				row("enter", "open"),
				row("c", "message"),
			)
		} else {
			localSection = section("profile (own)",
				row("enter", "open"),
			)
		}
	case screenNotifications:
		localSection = section("notifications",
			row("M", "mark all read"),
			row("p", "view profile"),
			row("c", "message"),
		)
	case screenJournal:
		if a.journal.ComposeActive() {
			localSection = section("journal (editing)", row("Enter", "paragraph"))
		} else {
			localSection = section("journal",
				row("h", "revision history"),
			)
		}
	case screenBookmarks:
		localSection = section("bookmarks")
	case screenGuilds:
		if a.guilds.ComposeActive() {
			localSection = section("guilds (compose)", row("Enter", "paragraph"))
		} else if a.guilds.IsBrowsingMembers() {
			localSection = section("guilds (members)", row("enter", "view profile"))
		} else if a.guilds.IsBrowsingGuild() {
			localSection = section("guilds (browsing)", row("n", "new thread"), row("m", "members"))
		} else {
			localSection = section("guilds")
		}
	case screenTopics:
		if a.topics.IsBrowsingTopic() {
			localSection = section("topics (browsing)")
		} else {
			localSection = section("topics")
		}
	case screenSearch:
		if a.search.InputFocused() {
			localSection = section("search")
		} else {
			localSection = section("search (results)")
		}
	case screenSettings:
		t := "settings"
		if a.settingsScreen.IsDirty() {
			t = "settings (unsaved changes)"
		}
		localSection = section(t)
	case screenChatrooms:
		if a.chatrooms.IsShowingDetail() {
			localSection = section("circ (room)")
		} else {
			localSection = section("circ")
		}
	case screenCMail:
		localSection = section("c-mail")
	}

	body := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		globalSection,
		"",
		localSection,
		"",
		theme.Subtle.Render("any key · close"),
		renderedVersionLine,
	)
	return theme.ActiveBorder.Render(body)
}

func (l TabsLayout) renderURLPicker(a App) string {
	title := theme.Title.Render("open url")
	items := make([]string, len(a.urlPickerItems))
	for i, u := range a.urlPickerItems {
		display := u
		if a.canRenderImageInline(u) {
			display = "[img] " + display
		}
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

// renderImageModal returns the bordered text-only shell for the image overlay.
// The image escape sequence is injected separately in View() via ANSI cursor movement.
func (l TabsLayout) renderImageModal(a App) string {
	blankLine := strings.Repeat(" ", a.imageModalCols)
	lines := make([]string, a.imageModalRows)
	for i := range lines {
		lines[i] = blankLine
	}
	content := strings.Join(lines, "\n")
	if len(a.imageCarouselItems) > 1 {
		// A plain text hint below the image, not overlaid on it — Kitty
		// placements are an independent compositing layer that can hide text
		// drawn into their own cells regardless of z-index, so cycling
		// arrows drawn "on" the image were invisible in practice. This line
		// renders through the normal bordered-box text path instead.
		hint := theme.Subtle.Render(fmt.Sprintf("◂ %d/%d ▸", a.imageCarouselIndex+1, len(a.imageCarouselItems)))
		content += "\n" + lipgloss.PlaceHorizontal(a.imageModalCols, lipgloss.Center, hint)
	}
	return theme.ActiveBorder.Render(content)
}
