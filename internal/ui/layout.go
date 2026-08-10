package ui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/ragnar/cyber-tui/internal/ui/imgview"
	"github.com/ragnar/cyber-tui/internal/ui/screens"
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
	// InlineImageSlots returns the active screen's visible inline-image slots,
	// this layout's screen origin for them, and a selection identity for the
	// active screen — see modalRenderer's method of the same name (Layout and
	// modalRenderer both need it: App.syncInlineImages calls it via the
	// Layout interface to know what to fetch and to detect a selection-only
	// move that requires a repaint, independent of rendering, while
	// compositeOverlays calls it via modalRenderer to know where to composite
	// the result).
	InlineImageSlots(a App) (slots []screens.InlineImageSlot, rowOrigin, colOrigin int, selKey string)
}

// modalRenderer is implemented by both TabsLayout and MillerLayout so
// compositeOverlays can render each layout's own modal content while the
// compositing order — and, critically, the inline-image injection step —
// lives in exactly one place. Everything here genuinely varies per layout
// (chrome, colors, sizing); the order overlays get checked and composited
// in does not, and used to be duplicated by hand between layout_tabs.go and
// layout_miller.go, which is how MillerLayout ended up never calling
// injectInlineImages at all.
type modalRenderer interface {
	renderThemePicker(a App) string
	renderThemeEditor(a App) string
	renderPathPrompt(a App) string
	renderHelpModal(a App) string
	renderURLPicker(a App) string
	renderImageModal(a App) string
	// InlineImageSlots returns the active screen's visible inline-image
	// slots plus this layout's screen origin (rowOrigin, colOrigin) for
	// them — the origin varies per layout (and, within Miller, per screen)
	// since it depends on chrome that differs between layouts. selKey is
	// unused by compositeOverlays (it only needs positioning) but is part of
	// the signature since this is the same physical method as Layout's.
	InlineImageSlots(a App) (slots []screens.InlineImageSlot, rowOrigin, colOrigin int, selKey string)
}

// compositeOverlays applies every overlay — the five simple modals, the
// fullscreen image modal, the Kitty placement-cleanup delete, and inline
// image injection — on top of base, in that fixed order. Called once as the
// last line of each Layout's View(), so no implementation can skip a step
// or return early partway through.
func compositeOverlays(l modalRenderer, a App, base string) string {
	switch {
	case a.themePickerOpen:
		return overlayCenter(base, l.renderThemePicker(a), a.width, a.height)
	case a.themeEditorOpen:
		return overlayCenter(base, l.renderThemeEditor(a), a.width, a.height)
	case a.pathPromptOpen:
		return overlayCenter(base, l.renderPathPrompt(a), a.width, a.height)
	case a.helpModalOpen:
		return overlayCenter(base, l.renderHelpModal(a), a.width, a.height)
	case a.urlPickerOpen:
		return overlayCenter(base, l.renderURLPicker(a), a.width, a.height)
	}
	if a.imageModalOpen {
		if (a.graphicsProtocol == imgview.ProtocolITerm2 || a.graphicsProtocol == imgview.ProtocolSixel) &&
			a.imageModalPrevRows != 0 &&
			(a.imageModalPrevRows != a.imageModalRows || a.imageModalPrevCols != a.imageModalCols) {
			// A cycled-to image is a different size than the one it
			// replaced. iTerm2/Sixel have no Kitty-style delete-by-placement
			// primitive, so stray raster pixels from the previous (possibly
			// larger or differently-positioned) box can persist outside the
			// new box's footprint. Force the previous box's full row range
			// dirty so Bubble Tea's own diff resends real current content
			// there — same technique as the inline-image fix
			// (forceRowsDirty) — instead of a tea.ClearScreen, which used to
			// flash the whole screen on every cycle. Rendered via
			// l.renderImageModal with the previous dimensions substituted
			// in, not a duplicated size formula, since the rendered height
			// differs by layout (TabsLayout adds a carousel-index hint
			// line, MillerLayout doesn't).
			prevApp := a
			prevApp.imageModalRows = a.imageModalPrevRows
			prevApp.imageModalCols = a.imageModalPrevCols
			prevModal := l.renderImageModal(prevApp)
			prevH := len(strings.Split(prevModal, "\n"))
			prevYOff := (a.height - prevH) / 2
			if prevYOff < 0 {
				prevYOff = 0
			}
			rows := make([]int, 0, prevH)
			for r := prevYOff + 1; r <= prevYOff+prevH; r++ {
				rows = append(rows, r)
			}
			base = forceRowsDirty(base, rows)
		}
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
		// Inject the targeted delete for the modal's own reserved placement
		// (kittyModalPlacementID) onto the line that held the modal's top
		// border so Bubble Tea's diff renderer delivers it to the terminal.
		// Never a blunt delete-all: that would also erase any inline
		// images' placements, which can be on screen at the same time.
		//
		// This must fall through to the inline-image injection below rather
		// than returning early: imageNeedsCleanup stays true indefinitely
		// (see its doc comment — it's never auto-cleared, only cleared by the
		// next modal image opening), so an early return here would silently
		// stop all inline-image rendering for the rest of the session after
		// the very first Kitty modal close.
		modalH := a.imageModalRows + 2
		yOff := (a.height - modalH) / 2
		if yOff < 0 {
			yOff = 0
		}
		lines := strings.Split(base, "\n")
		if yOff < len(lines) {
			lines[yOff] = imgview.DeleteKittyPlacement(kittyModalPlacementID) + lines[yOff]
		}
		base = strings.Join(lines, "\n")
	}
	slots, rowOrigin, colOrigin, _ := l.InlineImageSlots(a)
	if len(slots) > 0 || len(a.pendingKittyDeletes) > 0 || len(a.inlineImageStaleRows) > 0 {
		return injectInlineImages(a, base, slots, rowOrigin, colOrigin)
	}
	return base
}

// forceRowsDirty appends an inert SGR-reset marker to each of the given
// absolute (1-indexed) rows in base, forcing Bubble Tea's per-line diff to
// resend that row's real, always-correct content instead of leaving whatever
// an earlier out-of-band inline-image write left behind — see
// syncInlineImageErasures. Mirrors the Kitty-modal-cleanup line edit above
// (strings.Split/Join by "\n", index by absolute row - 1).
func forceRowsDirty(base string, rows []int) string {
	if len(rows) == 0 {
		return base
	}
	lines := strings.Split(base, "\n")
	for _, row := range rows {
		if idx := row - 1; idx >= 0 && idx < len(lines) {
			lines[idx] += "\x1b[0m"
		}
	}
	return strings.Join(lines, "\n")
}

// injectInlineImages appends absolute-cursor-positioned escape sequences for
// each slot with a cache hit, same technique as the modal's imgRow/imgCol
// (base + "\x1b[row;colH" + encoded, cursor parked at the bottom line after).
// A slot with no cache hit yet is skipped — its reserved band just stays the
// blank text the screen already rendered into base. rowOrigin/colOrigin are
// the calling layout's screen origin for slot-relative coordinates (see
// modalRenderer.InlineImageSlots) — they differ per layout, and within
// Miller, per which screen/pane is active.
//
// Kitty's pendingKittyDeletes don't need cursor positioning — placements are
// addressed by id, not screen coordinates — so they're just appended
// anywhere in the frame. This is why the caller invokes this function even
// when slots is empty: a delete can be pending with nothing currently
// visible (e.g. every inline image just scrolled off-screen).
//
// A moved or removed image's stale rows (a.inlineImageStaleRows — see
// syncInlineImageErasures) are forced dirty in base directly, via
// forceRowsDirty, before any of the below is appended — that goes through
// Bubble Tea's own per-line diff/resend for those specific lines, so it's
// unaffected by the "one physical line" note below.
//
// Everything else this function builds — image draws, Kitty deletes, the
// trailing paint-gen marker — lands on one physical line: the whole return
// value is appended after base's last "\n", so Bubble Tea's renderer treats
// it as a single line in its per-frame line-diff. That diff skips resending
// a line whose bytes are byte-identical to last frame (canSkip in
// bubbletea's standard_renderer.go) — normally the right call, but for
// iTerm2/Sixel it's actively wrong when a selection change recolors a band
// row elsewhere without touching which images are visible or their cache
// contents: this line's bytes don't change, so the diff skips it, and the
// image's raster pixels (already overwritten by the recolored band row's
// own line, written earlier in the same frame) never get repainted.
// syncInlineImages tracks that condition and bumps inlineImagePaintGen when
// it happens; the trailing inert SGR-reset below toggles in step with that
// generation purely to make this line's bytes differ from last frame on
// exactly those frames, so Bubble Tea's own per-line diff reissues it — the
// same seamless single-line repaint (erase-to-end-of-line, not a
// full-screen erase) an ordinary scroll already gets for free. This used to
// be a tea.ClearScreen instead, which does a real full-screen erase and was
// visibly flashing on every touching selection change.
func injectInlineImages(a App, base string, slots []screens.InlineImageSlot, rowOrigin, colOrigin int) string {
	base = forceRowsDirty(base, a.inlineImageStaleRows)
	var sb strings.Builder
	sb.WriteString(base)
	for _, slot := range slots {
		encoded, ok := a.inlineImageCache[inlineImageCacheKey(slot, a.graphicsProtocol)]
		if !ok {
			continue
		}
		row := rowOrigin + slot.Row
		col := colOrigin + slot.ColIndent
		sb.WriteString(fmt.Sprintf("\x1b[%d;%dH%s", row, col, encoded))
	}
	for id := range a.pendingKittyDeletes {
		sb.WriteString(imgview.DeleteKittyPlacement(id))
	}
	sb.WriteString(fmt.Sprintf("\x1b[%d;1H", a.height))
	if a.inlineImagePaintGen%2 == 1 {
		// ponytail: a synthetic dirty marker working around the lack of a
		// public "force this line dirty" API in Bubble Tea (it has an
		// internal repaintMsg that does exactly this, but it's unexported —
		// see renderer.go). Replace with a direct call if one ever ships.
		sb.WriteString("\x1b[0m")
	}
	return sb.String()
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
// detail is reported even while t isn't selected for Circ/C-Mail/Guilds/
// Topics/PostDetail, since their detail state is genuinely still
// live/persisted in the background: Circ's open room and C-Mail's open
// conversation both keep their RTDB subscriptions streaming regardless of
// the active tab (see IsRoomStreamMsg/IsDMStreamMsg in app.go), Guilds/
// Topics' browse state is simply never reset by activateScreen on tab-away,
// and a post opened via t stays open (PostDetailModel.HasPost) until closed
// via Esc or re-navigating to t from PostDetail itself (activateScreen's
// escape hatch).
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
		detail = a.cmail.IsShowingDetail()
	}
	if a.postDetail.HasPost() && a.postDetailReturn == t {
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
	desc := sbStyle().Foreground(theme.ColorMeta)
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
// screenPostDetail isn't itself a tab either, so it resolves to
// postDetailReturn instead — otherwise cycling away from PostDetail would
// always be anchored to Feed's position rather than wherever the post was
// actually opened from.
func tabIndexOf(a App) int {
	active := a.active
	if active == screenPostDetail {
		active = a.postDetailReturn
	}
	for i, t := range visibleTabs() {
		if t.s == active {
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
	if a.active == screenCMail && s != screenCMail {
		a.cmail = a.cmail.SetFocused(false)
	}
	if a.active == screenChatrooms && s != screenChatrooms {
		a.chatrooms = a.chatrooms.SetFocused(false)
	}
	prev := a.active
	// A post left open (see PostDetailModel.HasPost) resumes automatically
	// when navigation lands back on the tab it was opened from — mirrors
	// Circ's background-room persistence. Re-navigating to that same tab
	// *from PostDetail itself* (prev == screenPostDetail) is instead the
	// explicit "close and show the list" escape hatch, matching Circ/C-Mail's
	// re-press-the-tab-key convention. Cycling can never land exactly back on
	// the origin tab in one step (see tabIndexOf), so it only ever hits the
	// resume branch, never the close one.
	if prev == screenPostDetail && s == a.postDetailReturn {
		a.postDetail = a.postDetail.Close()
	} else if prev != screenPostDetail && s == a.postDetailReturn && a.postDetail.HasPost() {
		a.active = screenPostDetail
		return a, nil
	}
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
		a.cmail = a.cmail.SetFocused(true)
		// A conversation left open when the user last switched away to a
		// *different* tab kept its RTDB subscription live in the background
		// (see IsDMStreamMsg) — resume it as-is instead of bouncing back to
		// the conversation list. Re-pressing the C-Mail key while already on
		// it (prev == screenCMail) is the deliberate escape hatch out of a
		// deep link, so that case still resets to the list.
		if prev != screenCMail && a.cmail.HasLiveConv() {
			return a, nil
		}
		a.cmail = a.cmail.ResetToList()
		// No REST refetch: the live user_conversations subscription keeps the
		// list current continuously, regardless of which tab is active — see
		// OpenUserConvsSubscription.
		return a, nil
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
