package screens

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/urlutil"
)

// pageJumpItems is how many rows a PgUp/PgDn press moves a card-list cursor
// (feed, bookmarks, guilds, notifications, topics, journal, chatrooms/cmail
// list mode) by. Cards vary in rendered height, so this is an approximate
// "jump ahead a bunch" step rather than an exact one-viewport-height jump —
// matching how htop/k9s/lazygit-style TUIs page a variable-height list.
const pageJumpItems = 10

// typographicPunctOK mirrors the whitelist in the markdown package: EAW=A characters
// that are consistently 1 column in Western terminals and safe to type/display.
var typographicPunctOK = map[rune]bool{
	'\u2018': true, '\u2019': true, // curly single quotes
	'\u201C': true, '\u201D': true, // curly double quotes
	'\u2013': true, '\u2014': true, // en/em dash
	'\u2026': true, // ellipsis
}

// filterAmbiguousKeyMsg replaces ambiguous-width runes in a KeyRunes message with spaces
// before they reach a textarea or textinput component.
func filterAmbiguousKeyMsg(msg tea.KeyMsg) (tea.KeyMsg, bool) {
	if msg.Type != tea.KeyRunes {
		return msg, true
	}
	filtered := make([]rune, len(msg.Runes))
	for i, r := range msg.Runes {
		if runewidth.IsAmbiguousWidth(r) && !typographicPunctOK[r] {
			filtered[i] = ' '
		} else {
			filtered[i] = r
		}
	}
	msg.Runes = filtered
	return msg, true
}

// filterSlugCharsKeyMsg drops any rune not in [a-zA-Z0-9-] (plus
// extraAllowed, for fields that need a delimiter — e.g. topics' comma/space
// separators) before it reaches a textinput. Case is left as typed — the
// API's documented slug/topics character rules
// (docs/00-latest-api-reference.md) require lowercase, but that's applied
// once at the submit boundary (SlugValue, ParseTopics) rather than live, so
// what's on screen matches what was typed. If every rune in the message is
// filtered out, returns keep=false so nothing is inserted.
func filterSlugCharsKeyMsg(msg tea.KeyMsg, extraAllowed string) (tea.KeyMsg, bool) {
	if msg.Type != tea.KeyRunes {
		return msg, true
	}
	filtered := make([]rune, 0, len(msg.Runes))
	for _, r := range msg.Runes {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-':
			filtered = append(filtered, r)
		case strings.ContainsRune(extraAllowed, r):
			filtered = append(filtered, r)
		}
	}
	if len(filtered) == 0 {
		return msg, false
	}
	msg.Runes = filtered
	return msg, true
}

// insertAtCursor splices s into ti's value at the current cursor position
// and moves the cursor to just after it. textinput.Model has no public
// InsertString (unlike textarea.Model), so this does the splice by hand.
func insertAtCursor(ti textinput.Model, s string) textinput.Model {
	val, pos := []rune(ti.Value()), ti.Position()
	if pos < 0 {
		pos = 0
	}
	if pos > len(val) {
		pos = len(val)
	}
	ins := []rune(s)
	out := make([]rune, 0, len(val)+len(ins))
	out = append(out, val[:pos]...)
	out = append(out, ins...)
	out = append(out, val[pos:]...)
	ti.SetValue(string(out))
	ti.SetCursor(pos + len(ins))
	return ti
}

// Chat message limits from the API (cIRC and cMail send-message): 10 lines
// and 2048 characters, except /art, whose canvas is 25 lines below the
// command line.
const (
	chatMaxLines    = 10
	chatMaxChars    = 2048
	chatArtMaxLines = 26
)

var crlfToLF = strings.NewReplacer("\r\n", "\n", "\r", "\n")

// newChatInput builds the multi-line compose box shared by cIRC and cMail.
// Enter is left to the screen (send); the newline key goes through
// updateChatInput.
func newChatInput(placeholder string) textarea.Model {
	ta := textarea.New()
	ta.Prompt = "> "
	ta.Placeholder = placeholder
	ta.ShowLineNumbers = false
	// ponytail: CharLimit counts display width, so CJK and emoji count as 2
	// against the server's 2048; switch to a rune count if that bites.
	ta.CharLimit = chatMaxChars
	ta.KeyMap.InsertNewline.SetEnabled(false)
	// textarea's defaults dim the prompt and blurred text and shade the
	// cursor line; keep the plain look of the single-line input it replaced.
	plain := lipgloss.NewStyle()
	for _, st := range []*textarea.Style{&ta.FocusedStyle, &ta.BlurredStyle} {
		st.Prompt, st.Text, st.CursorLine = plain, plain, plain
	}
	ta.SetHeight(1)
	return ta
}

// isArtCommand reports whether a chat body is an /art post, which the server
// stores as-is.
func isArtCommand(s string) bool {
	f := strings.Fields(s)
	return len(f) > 0 && strings.ToLower(f[0]) == "/art"
}

// normalizeChatBody applies the server's line rules so the compose box shows
// exactly what will be stored: runs of blank lines collapse to one and lines
// past the tenth join it with spaces. /art bodies pass through unchanged.
func normalizeChatBody(s string) string {
	s = crlfToLF.Replace(s)
	if isArtCommand(s) {
		return s
	}
	var out []string
	prevBlank := false
	for line := range strings.SplitSeq(s, "\n") {
		blank := strings.TrimSpace(line) == ""
		if blank && prevBlank {
			continue
		}
		prevBlank = blank
		out = append(out, line)
	}
	if len(out) > chatMaxLines {
		out[chatMaxLines-1] = strings.Join(out[chatMaxLines-1:], " ")
		out = out[:chatMaxLines]
	}
	return strings.Join(out, "\n")
}

// chatInputOffset returns the cursor's rune offset into ta.Value().
func chatInputOffset(ta textarea.Model) int {
	lines := strings.Split(ta.Value(), "\n")
	off := 0
	for _, l := range lines[:ta.Line()] {
		off += len([]rune(l)) + 1
	}
	li := ta.LineInfo()
	return off + li.StartColumn + li.ColumnOffset
}

// updateChatInput routes a message to a chat compose box: newlineKey inserts
// a line break when the result still obeys the server's line rules, pastes
// have CRLF normalised first, and anything that changes the value other than
// typing (a paste) is normalised afterwards.
func updateChatInput(ta textarea.Model, msg tea.Msg, newlineKey string) (textarea.Model, tea.Cmd) {
	// Edit at full height so textarea only scrolls once the content really
	// outgrows the box; fitChatInput shrinks it back afterwards. Editing at
	// the current (smaller) height scrolls line 1 away as soon as line 2
	// appears, and growing the box later never scrolls it back.
	ta.SetHeight(chatInputMaxRows)
	km, isKey := msg.(tea.KeyMsg)
	if isKey && !km.Paste && km.String() == newlineKey {
		r := []rune(ta.Value())
		off := chatInputOffset(ta)
		next := string(r[:off]) + "\n" + string(r[off:])
		// The line the cursor lands on is still being typed, so it doesn't
		// count as blank: "a" + two newlines must allow typing "b" below.
		lines := strings.Split(next, "\n")
		lines[strings.Count(string(r[:off]), "\n")+1] = "x"
		probe := strings.Join(lines, "\n")
		ok := normalizeChatBody(probe) == probe
		if isArtCommand(next) {
			ok = strings.Count(next, "\n") < chatArtMaxLines
		}
		if ok {
			ta.InsertString("\n")
			ta, _ = ta.Update(nil) // InsertString alone doesn't scroll to the cursor
		}
		return ta, nil
	}
	if isKey && km.Paste {
		// textarea's sanitizer maps \r and \n each to a newline, so a CRLF
		// paste would otherwise gain a blank line after every line.
		km.Runes = []rune(crlfToLF.Replace(string(km.Runes)))
		msg = km
	}
	old := ta.Value()
	ta, cmd := ta.Update(msg)
	if v := ta.Value(); v != old && (!isKey || km.Paste) {
		if n := normalizeChatBody(v); n != v {
			// ponytail: SetValue leaves the cursor at the end; keep the
			// offset if mid-text pastes that need trimming turn out common.
			ta.SetValue(n)
		}
	}
	return ta, cmd
}

// chatInputMaxRows is how tall the chat compose box grows before its
// content scrolls inside it.
const chatInputMaxRows = 5

// chatInputRow is one display row of the compose box: a soft-wrapped slice
// of one line of the value.
type chatInputRow struct {
	text  []rune // the row's runes, whitespace shown as ' ' like textarea draws it
	first bool   // first row of its line (gets the prompt)
}

// chatInputRows lists the compose box's display rows using textarea's own
// word-wrap: it is unexported, so a scratch one-line textarea per line
// reports each row's start and width through LineInfo as its cursor steps
// from row to row.
func chatInputRows(ta textarea.Model) []chatInputRow {
	probe := textarea.New()
	probe.Prompt = ""
	probe.ShowLineNumbers = false
	probe.SetWidth(ta.Width())
	var rows []chatInputRow
	for line := range strings.SplitSeq(ta.Value(), "\n") {
		r := []rune(strings.Map(func(c rune) rune {
			if unicode.IsSpace(c) {
				return ' '
			}
			return c
		}, line))
		probe.SetValue(line)
		probe.SetCursor(0)
		n, start := probe.LineInfo().Height, 0
		// Loop a fixed n times: SetCursor clamps, so a column-based stop
		// could spin forever.
		for i := range n {
			probe.SetCursor(start)
			li := probe.LineInfo()
			// wrap() pads every line's last row with one extra space, so
			// Width can run one past the runes actually there.
			rows = append(rows, chatInputRow{text: r[li.StartColumn:min(li.StartColumn+li.Width, len(r))], first: i == 0})
			start = li.StartColumn + li.Width
		}
	}
	return rows
}

// chatInputCursorRow returns the display row the cursor is on: the rows of
// every earlier line plus its row within its own line, the same count
// textarea's cursorLineNumber makes.
func chatInputCursorRow(ta textarea.Model, rows []chatInputRow) int {
	line, row := 0, 0
	for row < len(rows) && line < ta.Line() {
		row++
		if row < len(rows) && rows[row].first {
			line++
		}
	}
	return row + ta.LineInfo().RowOffset
}

// chatInputScroll keeps the cursor row inside the h visible rows starting at
// y, and never scrolls past the last row, so shrinking the content brings
// the top rows back into view.
func chatInputScroll(y, cursorRow, h, total int) int {
	y = min(max(y, cursorRow-h+1), cursorRow)
	return max(0, min(y, total-h))
}

// scrollChatInput returns the compose box's scroll offset for its current
// content and cursor, starting from the previous offset y.
func scrollChatInput(ta textarea.Model, y int) int {
	rows := chatInputRows(ta)
	return chatInputScroll(y, chatInputCursorRow(ta, rows), ta.Height(), len(rows))
}

// renderChatInput draws the compose box itself instead of textarea.View, so
// the caller controls the scroll offset (textarea keeps its own private)
// and can draw beyond what textarea offers. The textarea stays the editing
// engine: value, cursor and blink. Rows are padded or cut to the box width
// like textarea's viewport does, so the box never changes width.
func renderChatInput(ta textarea.Model, y int) string {
	if ta.Value() == "" {
		return ta.View() // textarea's placeholder rendering, scrolled to top by Reset
	}
	rows := chatInputRows(ta)
	cursorRow := chatInputCursorRow(ta, rows)
	h := ta.Height()
	y = chatInputScroll(y, cursorRow, h, len(rows))
	promptW := lipgloss.Width(ta.Prompt)
	boxW := promptW + ta.Width()
	out := make([]string, 0, h)
	for i := y; i < y+h; i++ {
		var line string
		if i < len(rows) {
			row := rows[i]
			prompt := strings.Repeat(" ", promptW)
			if row.first {
				prompt = ta.Prompt
			}
			body := string(row.text)
			if i == cursorRow {
				col := ta.LineInfo().ColumnOffset
				cur := ta.Cursor
				cur.SetChar(" ")
				after := ""
				if col < len(row.text) {
					cur.SetChar(string(row.text[col]))
					after = string(row.text[col+1:])
				}
				body = string(row.text[:min(col, len(row.text))]) + cur.View() + after
			}
			line = prompt + body
		} else {
			line = ta.Prompt
		}
		if w := lipgloss.Width(line); w < boxW {
			line += strings.Repeat(" ", boxW-w)
		} else if w > boxW {
			line = ansi.Truncate(line, boxW, "")
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// fitChatInput sizes the compose box to its content, 1..chatInputMaxRows
// rows including soft-wrapped ones, and shows the "> " prompt only on the
// first row of each line, so wrapped rows read as the same line they will
// be sent as.
func fitChatInput(ta textarea.Model) textarea.Model {
	rows := chatInputRows(ta)
	ta.SetPromptFunc(lipgloss.Width(ta.Prompt), func(row int) string {
		if row < len(rows) && !rows[row].first {
			return ""
		}
		return ta.Prompt
	})
	// ponytail: in C-Mail (which still draws with textarea.View), shrinking
	// back from past the cap can leave the top row scrolled off until the
	// cursor moves; cIRC draws via renderChatInput, which clamps its own
	// offset. Switch C-Mail over if it bites.
	if h := min(len(rows), chatInputMaxRows); h != ta.Height() {
		ta.SetHeight(h)
		ta, _ = ta.Update(nil) // re-scroll to the cursor at the new height
	}
	return ta
}

// chatInputOnFirstRow and chatInputOnLastRow report whether the cursor sits
// on the compose box's first or last visual row, where up/down leave the
// input for the message list instead of moving the cursor.
func chatInputOnFirstRow(ta textarea.Model) bool {
	return ta.Line() == 0 && ta.LineInfo().RowOffset == 0
}

func chatInputOnLastRow(ta textarea.Model) bool {
	li := ta.LineInfo()
	return ta.Line() == ta.LineCount()-1 && li.RowOffset == li.Height-1
}

// topicsCount returns the number of non-empty, trimmed comma-separated
// segments in s — mirrors ParseTopics' counting rule without allocating the
// slice ParseTopics returns.
func topicsCount(s string) int {
	n := 0
	for _, part := range strings.Split(s, ",") {
		if strings.TrimSpace(part) != "" {
			n++
		}
	}
	return n
}

// lastTopicSegment returns the portion of current after its last comma (or
// the whole string, if there's no comma yet) — the topic currently being
// typed.
func lastTopicSegment(current string) string {
	if idx := strings.LastIndex(current, ","); idx != -1 {
		return current[idx+1:]
	}
	return current
}

// filterTopicsKeyMsg applies filterSlugCharsKeyMsg (allowing ", " as topic
// delimiters) and additionally drops:
//   - a comma that would open a 4th topic (current already holds 3 —
//     mirrors ParseTopics' cap live instead of silently truncating at
//     submit time), or finalize a topic that's empty/space-only or ends in
//     a space;
//   - a space that would lead a topic, or immediately follow another space
//     — a topic may contain single spaces, never a run of 2+.
//
// current is the field's value before this keystroke is applied.
func filterTopicsKeyMsg(msg tea.KeyMsg, current string) (tea.KeyMsg, bool) {
	filtered, keep := filterSlugCharsKeyMsg(msg, ", ")
	if !keep || msg.Type != tea.KeyRunes {
		return filtered, keep
	}
	segment := lastTopicSegment(current)
	segmentEmpty := strings.TrimSpace(segment) == ""
	trailingSpace := strings.HasSuffix(segment, " ")
	blockComma := segmentEmpty || trailingSpace || topicsCount(current) >= 3
	blockSpace := segmentEmpty || trailingSpace
	if !blockComma && !blockSpace {
		return filtered, true
	}
	runes := make([]rune, 0, len(filtered.Runes))
	for _, r := range filtered.Runes {
		if r == ',' && blockComma {
			continue
		}
		if r == ' ' && blockSpace {
			continue
		}
		runes = append(runes, r)
	}
	if len(runes) == 0 {
		return filtered, false
	}
	filtered.Runes = runes
	return filtered, true
}

// updateTopicsInput processes one message against a topics textinput,
// applying filterTopicsKeyMsg's rules. If the key would type a comma right
// after an accidental trailing space, the space is silently trimmed first
// so the comma still lands, instead of leaving the comma key dead — a
// previously reported annoyance. The "no trailing space" rule still holds;
// it's just enforced by auto-correction here rather than refusal. The trim
// is only committed to the field once the comma is confirmed to pass
// filterTopicsKeyMsg — a comma that's still going to be rejected anyway
// (e.g. it would open an empty topic) leaves the field untouched, same as
// any other blocked keystroke.
func updateTopicsInput(input textinput.Model, msg tea.Msg) (textinput.Model, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return input.Update(msg)
	}
	current := input.Value()
	trimCandidate := current
	if km.Type == tea.KeyRunes && strings.ContainsRune(string(km.Runes), ',') {
		trimCandidate = strings.TrimRight(current, " ")
	}
	filtered, keep := filterTopicsKeyMsg(km, trimCandidate)
	if !keep {
		return input, nil
	}
	if trimCandidate != current {
		input.SetValue(trimCandidate)
	}
	return input.Update(filtered)
}

// topicMuted reports whether any of a post's topics is in the muted set —
// the muted-topics analogue of the FilterNSFW check each post-list screen
// applies in its visible*() helper. See docs/54-muted-topics.md.
func topicMuted(topics []string, muted map[string]struct{}) bool {
	for _, t := range topics {
		if _, ok := muted[t]; ok {
			return true
		}
	}
	return false
}

// mutedSet builds the lookup set from Settings.MutedTopics; returns nil for an
// empty list so callers can cheaply short-circuit with len() == 0.
func mutedSet(topics []string) map[string]struct{} {
	if len(topics) == 0 {
		return nil
	}
	m := make(map[string]struct{}, len(topics))
	for _, t := range topics {
		m[t] = struct{}{}
	}
	return m
}

// sameMutedSet reports whether set already holds exactly the slugs in list
// (order-independent) — used by SharedConfigMsg handlers to skip a needless
// selection reset + re-render when the muted list hasn't changed.
func sameMutedSet(set map[string]struct{}, list []string) bool {
	if len(set) != len(list) {
		return false
	}
	for _, t := range list {
		if _, ok := set[t]; !ok {
			return false
		}
	}
	return true
}

// toggleMuted returns a new slice with slug removed from the current muted
// set if present, or appended if not. Order of the retained entries is not
// guaranteed (the API doesn't care).
func toggleMuted(set map[string]struct{}, slug string) []string {
	if _, muted := set[slug]; muted {
		out := make([]string, 0, len(set)-1)
		for t := range set {
			if t != slug {
				out = append(out, t)
			}
		}
		return out
	}
	out := make([]string, 0, len(set)+1)
	for t := range set {
		out = append(out, t)
	}
	return append(out, slug)
}

// extractURLs is a package-internal helper used by URLProvider implementations.
// It normalizes each extracted URL so relative paths get the cyberspace.online prefix.
func extractURLs(content string) []string {
	raw := urlutil.ExtractURLs(content)
	if len(raw) == 0 {
		return nil
	}
	out := make([]string, len(raw))
	for i, u := range raw {
		out[i] = urlutil.NormalizeURL(u)
	}
	return out
}

// attachmentURLs returns the Src URL for each attachment.
func attachmentURLs(attachments []model.Attachment) []string {
	if len(attachments) == 0 {
		return nil
	}
	out := make([]string, len(attachments))
	for i, a := range attachments {
		out[i] = a.Src
	}
	return out
}

// messageAttachments builds a []model.Attachment from a chat message's flat
// ImageUrl/GifUrl/AudioAttachment fields, so the existing renderAttachments
// (used for post/reply attachments) can render them too.
func messageAttachments(msg model.Message) []model.Attachment {
	var out []model.Attachment
	if msg.ImageUrl != "" {
		out = append(out, model.Attachment{Type: "image", Src: msg.ImageUrl})
	}
	if msg.GifUrl != "" {
		out = append(out, model.Attachment{Type: "gif", Src: msg.GifUrl})
	}
	if msg.AudioAttachment != nil {
		out = append(out, *msg.AudioAttachment)
	}
	return out
}

// messageURLs returns every openable URL in msg: links in the body plus
// ImageUrl/GifUrl/AudioAttachment.Src. Not normalized via urlutil.NormalizeURL —
// matches attachmentURLs' existing behavior for Attachment.Src, which is
// already an absolute URL per the API spec.
func messageURLs(msg model.Message) []string {
	urls := extractURLs(msg.Body)
	if msg.ImageUrl != "" {
		urls = append(urls, msg.ImageUrl)
	}
	if msg.GifUrl != "" {
		urls = append(urls, msg.GifUrl)
	}
	if msg.AudioAttachment != nil && msg.AudioAttachment.Src != "" {
		urls = append(urls, msg.AudioAttachment.Src)
	}
	return urls
}

// messageCopyText returns the text 'y' should copy for msg: its body, or —
// for an attachment-only message with no body (an /gif or /song posted with
// no caption) — the attachment's URL, so copying still does something
// useful rather than copying an empty string.
func messageCopyText(msg model.Message) string {
	if msg.Body != "" {
		return msg.Body
	}
	if msg.ImageUrl != "" {
		return msg.ImageUrl
	}
	if msg.GifUrl != "" {
		return msg.GifUrl
	}
	if msg.AudioAttachment != nil {
		return msg.AudioAttachment.Src
	}
	return ""
}

// messageDisplayBody returns "" when Body merely duplicates ImageUrl or
// GifUrl (an attachment-only message posted with no text), so callers can
// skip printing a redundant line of URL text next to the attachment badge
// rendered separately via messageAttachments. Returns Body unchanged otherwise.
func messageDisplayBody(msg model.Message) string {
	if msg.Body != "" && (msg.Body == msg.ImageUrl || msg.Body == msg.GifUrl) {
		return ""
	}
	return msg.Body
}

// chatInlineImageURL returns the URL a chat message's inline image (if any)
// should be fetched from: the explicit ImageUrl/GifUrl attachment field
// first, else the first image-looking URL typed into the message body.
// Returns "" when none apply — the message's existing attachment badge or
// body text is never altered by this; it's purely a lookup for the
// additive inline-image band C-Mail/cIRC splice in alongside it.
func chatInlineImageURL(msg model.Message) string {
	if msg.ImageUrl != "" {
		return msg.ImageUrl
	}
	if msg.GifUrl != "" {
		return msg.GifUrl
	}
	for _, u := range extractURLs(msg.Body) {
		if urlutil.IsImageURL(u) {
			return u
		}
	}
	return ""
}

// sanitizeChatMessageForInlineImage returns a copy of msg with the text that
// would otherwise duplicate its inline image (url, from chatInlineImageURL)
// stripped, so a caller rendering that copy instead of msg doesn't show the
// image's own URL/attachment badge sitting redundantly next to the actual
// image. Only clears what's unambiguously that image:
//   - ImageUrl/GifUrl, whichever matches url, so messageAttachments produces
//     no [image]/[gif] badge for it (an unrelated AudioAttachment is untouched)
//   - Body, but only when it's nothing but the URL itself (trimmed) — a URL
//     embedded alongside other text is left alone, since surgically removing
//     a substring from already word-wrapped/styled text isn't attempted here
func sanitizeChatMessageForInlineImage(msg model.Message, url string) model.Message {
	if msg.ImageUrl == url {
		msg.ImageUrl = ""
	}
	if msg.GifUrl == url {
		msg.GifUrl = ""
	}
	if strings.TrimSpace(msg.Body) == url {
		msg.Body = ""
	}
	return msg
}

// dedupeURLs removes repeated URLs while preserving first-seen order. Used by
// URLProvider implementations that aggregate across many items (e.g. an
// entire loaded chat history) rather than a single focused one, where the
// same link posted more than once would otherwise show up repeatedly in the
// open-link picker.
func dedupeURLs(urls []string) []string {
	if len(urls) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(urls))
	out := make([]string, 0, len(urls))
	for _, u := range urls {
		if seen[u] {
			continue
		}
		seen[u] = true
		out = append(out, u)
	}
	return out
}
