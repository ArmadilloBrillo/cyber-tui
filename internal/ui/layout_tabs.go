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
	contentHeight := a.height - theme.ChromeHeight
	content := lipgloss.NewStyle().Height(contentHeight).MaxHeight(contentHeight).Render(l.renderActiveScreen(a))
	base := lipgloss.JoinVertical(lipgloss.Left,
		l.renderTabBar(a),
		l.renderFeedPendingBar(a),
		content,
		l.renderBottomBar(a),
	)
	return compositeOverlays(l, a, base)
}

// InlineImageSlots returns the active screen's visible inline-image slots and
// this layout's fixed screen origin for them: row 1 is the tab bar, row 2 the
// feed-pending/separator row, so content's own row 0 is ANSI row 3; column 1
// is the content pane's left edge (no left margin in this layout).
func (l TabsLayout) InlineImageSlots(a App) ([]screens.InlineImageSlot, int, int, string) {
	return a.activeInlineImageSlots(), 3, 1, a.activeSelectionKey()
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
		// PostDetail used to be excluded here, forcing ctrl+left to leave it —
		// now it cycles the same as everywhere else (tabIndexOf anchors on
		// postDetailReturn, so this never lands back on the origin tab in one
		// step; see activateScreen's escape hatch for how that's reached).
		if a.focus == focusMenu {
			var cmd tea.Cmd
			a, cmd = navigateTabBy(a, -1)
			return a, cmd, true
		}
	case "right":
		if a.focus == focusMenu {
			var cmd tea.Cmd
			a, cmd = navigateTabBy(a, +1)
			return a, cmd, true
		}
	case "ctrl+left":
		// Unlike plain "left", not gated on focus == focusMenu: this is the
		// ctrl-twin that reaches tab-cycling from CMail/CIRC detail mode,
		// where the compose input holds focus for the entire view.
		var cmd tea.Cmd
		a, cmd = navigateTabBy(a, -1)
		return a, cmd, true
	case "ctrl+right":
		var cmd tea.Cmd
		a, cmd = navigateTabBy(a, +1)
		return a, cmd, true
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

// ModalMaxWidth: no side chrome in this layout (the tab bar is a top row,
// not a side pane), so a modal can use the full terminal width.
func (l TabsLayout) ModalMaxWidth(termWidth int) int { return termWidth }

func (l TabsLayout) renderTabBar(a App) string {
	var tabs string
	for _, t := range visibleTabs() {
		badge := ""
		if t.s == screenNotifications && a.polledUnreadCount > 0 {
			badge = " (" + notifBadgeText(a.polledUnreadCount, a.polledUnreadCountExact) + ")"
		}
		if t.s == screenFeed {
			if n := a.feed.PendingNewCount(); n > 0 {
				badge = fmt.Sprintf(" (%d)", n)
			}
		}
		if t.s == screenCMail {
			if n := a.cmail.TotalUnread(); n > 0 {
				badge = fmt.Sprintf(" (%d)", n)
			}
		}
		if t.s == screenChatrooms {
			if n := a.chatrooms.UnreadCount(); n > 0 {
				badge = fmt.Sprintf(" (%d)", n)
			}
		}
		selected, detail := tabVisualState(a, t.s)
		text, mnemonic := theme.TabText, theme.TabMnemonic
		if selected {
			text, mnemonic = theme.ActiveTabText, theme.ActiveTabMnemonic
		}
		marker := ""
		if detail {
			// A trailing chevron for "one level deep" — a Circ room,
			// Guilds/Topics browse, a C-Mail conversation, or a PostDetail
			// opened from this tab (see tabVisualState). Rendered via `text`
			// below, so it inherits the active highlight when this tab is
			// selected, or the ordinary dim/inactive style when it isn't —
			// Circ/Guilds/Topics report detail even while backgrounded (their
			// state is genuinely still live/persisted there), so the dim
			// variant is what shows a room/browse left open on another tab.
			marker = " ›"
		}
		before, ch, after := splitMnemonic(t.label, t.mnemonic)
		// Each fragment is rendered independently (rather than wrapping the
		// whole label in one .Padding style) so the active tab's background
		// survives across the mnemonic's own ANSI reset — see TabText's doc.
		tabs += text.Render("  "+before) + mnemonic.Render(ch) + text.Render(after+badge+marker+"  ")
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

// renderFeedPendingBar fills the blank separator row below the tab bar with
// the "N new entries" message while posts are staged from the background
// feed poll. Hidden during an active refresh so it doesn't sit alongside the
// viewport's own "fetching new posts..." message for that instant.
func (l TabsLayout) renderFeedPendingBar(a App) string {
	if a.active != screenFeed || a.feed.IsRefreshing() {
		return ""
	}
	if label := a.feed.PendingNewLabel(); label != "" {
		return theme.Subtle.Render(label)
	}
	return ""
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
			if a.feed.ComposePanelActive() && a.feed.ComposeSubmitting() {
				return []hint{{"…", "posting"}}
			}
			h := []hint{{"tab", "cycle"}, {"space", "toggle"}, {"Ctrl+s", "send"}}
			if a.feed.ComposePanelActive() && !a.feed.ComposeEditing() {
				h = append(h, hint{"Ctrl+d", "to journal"})
			}
			return append(h, hint{"Esc", "cancel"})
		}
		hints := []hint{{"↑↓", "navigate"}, {"enter", "open"}, {"r", "reply"}, {"n", "new"}, {"b", "bookmark"}, {"w", "watch"}, {"l", "copy link"}, {"c", "message"}}
		if a.feed.CanEditSelected() {
			hints = append(hints, hint{"e", "edit"})
		}
		return append(hints, more)
	case screenPostDetail:
		if a.postDetail.ComposeActive() {
			return []hint{{"Ctrl+s", "send"}, {"Esc", "cancel"}}
		}
		hints := []hint{{"↑↓", "navigate"}, {"r", "reply"}, {"b", "bookmark"}, {"w", "watch"}, {"l", "copy link"}, {"c", "message"}}
		if a.postDetail.CanEditSelected() {
			hints = append(hints, hint{"e", "edit"})
		}
		if a.postDetail.HasTheme() {
			hints = append(hints, hint{"T", "try theme"})
		}
		return append(hints, hint{"esc", "back"}, more)
	case screenProfile:
		if a.profile.ComposeActive() {
			return []hint{{"Ctrl+s", "save"}, {"Esc", "cancel"}, {"tab", "cycle"}}
		}
		if a.profile.IsReadOnly() {
			return []hint{{"↑↓", "navigate"}, {"f", "follow"}, {"c", "message"}, {"p", "poke"}, {"tab", "cycle"}, more}
		}
		return []hint{{"↑↓", "navigate"}, {"e", "edit"}, {"tab", "cycle"}, more}
	case screenNotifications:
		return []hint{{"↑↓", "navigate"}, {"enter", "open"}, {"m", "mark read"}, {"u", "toggle unread"}, {"f", "filter"}, {"c", "message"}, more}
	case screenJournal:
		if a.journal.ComposeActive() {
			if a.journal.IsPublishing() {
				return []hint{{"…", "publishing"}}
			}
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
			if a.guilds.IsConfirmingJoin() || a.guilds.IsConfirmingLeave() || a.guilds.IsConfirmingPromote() {
				return []hint{{"y", "confirm"}, {"n/esc", "cancel"}}
			}
			hints := []hint{{"↑↓", "navigate"}, {"enter", "open"}, {"m", "members"}, {"n", "new thread"}, {"esc", "back"}}
			d := a.guilds.GuildDetail()
			if a.guilds.IsDetailLoaded() {
				switch d.Role {
				case "":
					hints = append(hints, hint{"J", "join"})
				case "apprentice":
					hints = append(hints, hint{"L", "leave"}, hint{"P", "promote"})
				case "member":
					hints = append(hints, hint{"L", "leave"})
				}
			}
			return append(hints, more)
		}
		return []hint{{"↑↓", "navigate"}, {"enter", "browse"}, more}
	case screenTopics:
		if a.topics.IsBrowsingTopic() {
			return []hint{{"↑↓", "navigate"}, {"enter", "open"}, {"esc", "back"}, more}
		}
		return []hint{{"↑↓", "navigate"}, {"enter", "browse"}, {"m", "mute"}, {"f", "filter"}, {"esc", "back"}, more}
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
			// here, same as plain 'o' — omit "more" and surface the ctrl-twins
			// that reach through instead (renderStatusBar already trims this
			// list on narrow terminals, so listing all of them here is safe).
			return []hint{{"↑↓", "scroll"}, {"enter", "send"}, {"ctrl+o", "open"}, {"ctrl+q", "quit"}, {"ctrl+t", "theme"}, {"ctrl+←→", "tabs"}, {"esc", "back"}}
		}
		return []hint{{"↑↓/j/k", "navigate"}, {"enter", "open"}, more}
	case screenCMail:
		if a.cmail.IsShowingDetail() {
			return []hint{{"↑↓", "scroll"}, {"enter", "send"}, {"ctrl+o", "open"}, {"ctrl+q", "quit"}, {"ctrl+t", "theme"}, {"ctrl+←→", "tabs"}, {"esc", "back"}}
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
	hint := theme.Subtle.Render("↑↓ preview   enter save   e edit   x export   i import   esc cancel")
	body := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		lipgloss.JoinVertical(lipgloss.Left, items...),
		"",
		hint,
	)
	return theme.ActiveBorder.Render(body)
}

// renderThemeEditor renders the "custom" theme color editor modal.
func (l TabsLayout) renderThemeEditor(a App) string {
	return a.themeEditor.View()
}

// renderPathPrompt renders the export/import file-path prompt modal.
func (l TabsLayout) renderPathPrompt(a App) string {
	return a.pathPrompt.View()
}

// renderAttachURLPrompt renders the attach-image/gif-URL prompt modal.
func (l TabsLayout) renderAttachURLPrompt(a App) string {
	return a.attachURLPrompt.View()
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
		row("e", "edit custom theme (in theme picker)"),
		row("x / i", "export / import custom theme (in theme picker)"),
		row("v", "density"),
		row("o", "open url"),
		row("ctrl+]", "icon picker"),
		row("ctrl+g", "attach image/gif"),
		row("ctrl+j", "attach song (circ, supporter)"),
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
				row("e", "edit own, <5min"),
			)
		}
	case screenPostDetail:
		if a.postDetail.ComposeActive() {
			localSection = section("post detail (compose)", row("Enter", "paragraph"))
		} else {
			localSection = section("post detail",
				row("d", "delete own"),
				row("e", "edit own, <5min"),
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
				row("p", "poke"),
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

func (l TabsLayout) renderIconPicker(a App) string {
	return a.iconPicker.View()
}

func (l TabsLayout) renderSongPrompt(a App) string {
	return a.songPrompt.View()
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
