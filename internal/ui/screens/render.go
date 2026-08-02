package screens

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/sanitize"
	"github.com/ragnar/cyber-tui/internal/ui/markdown"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
)

const postMaxBodyLines = 4

// RenderPost renders a model.Post as a bordered card matching the feed style.
// selected controls the border colour (active vs inactive).
// bookmarked adds a [★] badge; watched adds a [◉] badge — both in the header.
// width is the full terminal width; loc and timeFormat control the timestamp display.
// maxBodyLines caps body text at that many lines (postMaxBodyLines for list views,
// 0 for the reading pane where the full content should be shown).
func RenderPost(p model.Post, selected bool, bookmarked bool, watched bool, width int, loc *time.Location, timeFormat string, maxBodyLines int) string {
	innerWidth := width - 4

	left := lipgloss.JoinHorizontal(lipgloss.Top,
		theme.Highlight.Render("@"+p.AuthorUsername),
		theme.Subtle.Render("  "+displayTime(p.CreatedAt, loc, timeFormat, false)),
	) + audioIcon(p.Attachments) + bookmarkIcon(bookmarked) + watchIcon(watched)
	var rightParts []string
	if ind := attachmentIndicator(p.Attachments); ind != "" {
		rightParts = append(rightParts, ind)
	}
	switch p.RepliesCount {
	case 1:
		rightParts = append(rightParts, theme.Subtle.Render("1 reply"))
	default:
		if p.RepliesCount > 0 {
			rightParts = append(rightParts, theme.Subtle.Render(fmt.Sprintf("%d replies", p.RepliesCount)))
		}
	}
	right := strings.Join(rightParts, " ")
	var header string
	if innerWidth > 0 {
		gap := innerWidth - lipgloss.Width(left) - lipgloss.Width(right)
		if gap > 0 {
			header = left + strings.Repeat(" ", gap) + right
		} else {
			header = left
		}
	} else {
		header = left
	}

	// Badges line: guild indicator, nsfw, public — omitted when none apply.
	var badgeParts []string
	if p.IsGuildThread && p.GuildSlug != "" {
		badgeParts = append(badgeParts, theme.Subtle.Render("[#"+p.GuildSlug+"]"))
	}
	if p.IsNSFW {
		badgeParts = append(badgeParts, theme.Error.Render("[nsfw]"))
	}
	if p.IsPublic {
		badgeParts = append(badgeParts, theme.Subtle.Render("[public]"))
	}
	badges := strings.Join(badgeParts, "  ")

	var body string
	if innerWidth > 0 {
		rendered := markdown.Render(p.Content, innerWidth)
		lines := strings.Split(rendered, "\n")
		if maxBodyLines > 0 && len(lines) > maxBodyLines {
			body = strings.Join(lines[:maxBodyLines], "\n")
			more := len(lines) - maxBodyLines
			body += "\n" + theme.Subtle.Render(fmt.Sprintf("  ▼ %d more lines", more))
		} else {
			body = rendered
		}
	} else {
		body = markdown.Render(p.Content, 0)
	}

	if att := renderAttachments(p.Attachments); att != "" {
		body = body + "\n" + att
	}

	var topicsSB strings.Builder
	for _, t := range p.Topics {
		topicsSB.WriteString(theme.Subtle.Render("#"+t) + " ")
	}
	topics := topicsSB.String()

	boxStyle := theme.Border
	if selected {
		boxStyle = theme.ActiveBorder
	}
	if innerWidth > 0 {
		boxStyle = boxStyle.Width(width - 2)
	}

	body = strings.TrimRight(body, "\n")
	rows := []string{header}
	if badges != "" {
		rows = append(rows, badges)
	}
	if p.Title != "" {
		rows = append(rows, theme.Highlight.Render(p.Title))
	}
	rows = append(rows, body)
	if topics != "" {
		rows = append(rows, "\n"+topics)
	}
	return boxStyle.Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
}

// attachmentIndicator returns a compact header badge for any attachments present,
// e.g. "[img]". Returns "" when there are no attachments.
func attachmentIndicator(attachments []model.Attachment) string {
	for _, a := range attachments {
		if a.Type == "image" {
			return theme.Subtle.Render("[img]")
		}
	}
	return ""
}

// audioIcon returns a ♫ icon (with leading spaces) when any attachment is audio, else "".
func audioIcon(attachments []model.Attachment) string {
	for _, a := range attachments {
		if a.Type == "audio" {
			return "  " + theme.Highlight.Render("♫")
		}
	}
	return ""
}

// bookmarkIcon returns a ★ icon (with leading spaces) when bookmarked, else "".
func bookmarkIcon(bookmarked bool) string {
	if bookmarked {
		return "  " + theme.Highlight.Render("★")
	}
	return ""
}

// watchIcon returns a ◉ icon (with leading spaces) when the thread is watched, else "".
func watchIcon(watched bool) string {
	if watched {
		return "  " + theme.Highlight.Render("◉")
	}
	return ""
}

// renderAttachments returns a formatted block for each attachment,
// rendered below post/reply content. Returns "" when there are no attachments.
func renderAttachments(attachments []model.Attachment) string {
	if len(attachments) == 0 {
		return ""
	}
	linkStyle := lipgloss.NewStyle().Underline(true).Foreground(theme.ColorCyan)
	var lines []string
	for _, a := range attachments {
		switch a.Type {
		case "image":
			lines = append(lines, theme.Subtle.Render("[image]")+"  "+linkStyle.Render(a.Src))
		case "gif":
			lines = append(lines, theme.Subtle.Render("[gif]")+"  "+linkStyle.Render(a.Src))
		case "audio":
			label := a.Src
			if a.Artist != "" || a.Title != "" {
				label = a.Artist + " – " + a.Title
			}
			line := theme.Subtle.Render("[AUDIO]") + "  " + linkStyle.Render(label)
			if a.Genre != "" {
				line += theme.Subtle.Render(" (" + a.Genre + ")")
			}
			lines = append(lines, line)
		default:
			lines = append(lines, theme.Subtle.Render("[attachment]")+"  "+linkStyle.Render(a.Src))
		}
	}
	return strings.Join(lines, "\n")
}

// listFooter returns a styled "loading more…" or "— end —" line, or "" when neither applies.
func listFooter(loading, exhausted bool) string {
	if loading {
		return theme.Subtle.Render("  loading more…")
	}
	if exhausted {
		return theme.Subtle.Render("  — end —")
	}
	return ""
}

// renderCircMessages renders a list of chatroom messages in IRC style:
// <username>  message body                                     14:32
// The timestamp is right-aligned on the message's last line; the username is
// highlighted. Bodies word-wrap to fit viewportWidth, with room reserved on
// every wrapped line for the timestamp column so long messages never push it
// off-screen; continuation lines are indented to align under the body.
func renderCircMessages(msgs []model.Message, loc *time.Location, timeDisplayFormat string, viewportWidth int, currentUser string) string {
	return renderCircMessagesStyled(msgs, loc, timeDisplayFormat, viewportWidth, currentUser, nil, 0)
}

// renderCircMessagesStyled is renderCircMessages plus style/attachment
// support: revealed gates which spoiler-styled messages show their real
// body and which l33t-styled messages show their original, unsubstituted
// text (keyed by message ID; nil means none revealed), and frame drives
// the slow/wave/glitch animated styles. renderCircMessages is a thin wrapper
// passing (nil, 0), so its many existing call sites are unaffected.
func renderCircMessagesStyled(msgs []model.Message, loc *time.Location, timeDisplayFormat string, viewportWidth int, currentUser string, revealed map[string]bool, frame int) string {
	if viewportWidth < 20 {
		viewportWidth = 80
	}
	const tsGap = 2 // minimum space between the wrapped text and the timestamp
	var sb strings.Builder
	for _, msg := range msgs {
		if msg.IsSystem {
			sb.WriteString(renderSystemNotice(msg.Body, viewportWidth))
			continue
		}
		ts := displayTime(msg.CreatedAt, loc, timeDisplayFormat, true)
		tsWidth := lipgloss.Width(ts)

		if slices.Contains(msg.Style, styleArt) {
			sb.WriteString(renderArtMessage(msg.From.Username, msg.Body, ts, viewportWidth))
			continue
		}

		if msg.Deleted {
			sb.WriteString(renderDeletedTombstone(msg.From.Username, ts, viewportWidth))
			continue
		}

		if msg.IsAction {
			sb.WriteString(renderActionLine(msg.From.Username, msg.Body, ts, viewportWidth, currentUser))
			continue
		}

		usernameStyle := theme.Highlight
		if currentUser != "" && msg.From.Username == currentUser {
			usernameStyle = theme.MeHighlight
		}

		// Styled prefix: <username>  (plain width = len(username) + 4)
		styledPrefix := "<" + usernameStyle.Render(msg.From.Username) + ">  "
		rawPrefixWidth := len(msg.From.Username) + 4
		indent := strings.Repeat(" ", rawPrefixWidth)

		bodyWidth := max(viewportWidth-rawPrefixWidth-tsWidth-tsGap, 10)

		displayBody := messageDisplayBody(msg)
		att := renderAttachments(messageAttachments(msg))

		var lines []string
		switch {
		case displayBody == "" && att != "":
			// Attachment-only message: use the attachment block itself as
			// the "body" so the username/timestamp land on it directly
			// instead of leaving a blank-looking line above it. Still runs
			// through the same Width(bodyWidth).Render wrapping as normal
			// text bodies — skipping it left long URLs pushing the
			// timestamp far past viewportWidth instead of wrapping under it.
			lines = strings.Split(lipgloss.NewStyle().Width(bodyWidth).Render(att), "\n")
			att = ""
		case slices.Contains(msg.Style, styleSpoiler) && !revealed[msg.ID]:
			body := theme.Subtle.Render(maskSpoilerBody(strings.TrimRight(displayBody, "\n")))
			lines = strings.Split(lipgloss.NewStyle().Width(bodyWidth).Render(body), "\n")
		default:
			// l33t substitution is reveal-gated like spoiler's masking; every
			// other substitution/attribute style (cursive, flip, glitch,
			// blink, quiet, rainbow) is not, so only l33t is dropped once
			// the message is revealed.
			effectiveStyles := msg.Style
			if revealed[msg.ID] {
				effectiveStyles = slices.DeleteFunc(slices.Clone(msg.Style), func(s string) bool { return s == styleL33t })
			}
			raw := substituteChars(displayBody, msg.ID, effectiveStyles, frame)
			body := applyAttributeStyle(markdown.RenderInline(strings.TrimRight(raw, "\n"), currentUser), msg.Style)
			lines = strings.Split(lipgloss.NewStyle().Width(bodyWidth).Render(body), "\n")
		}

		// Blink toggling runs after wrapping, blanking each already-wrapped
		// line to its own rendered width, so hiding the message never
		// changes its line count/wrap structure (blanking pre-wrap risks
		// lipgloss collapsing an all-space string into fewer lines than the
		// real text wrapped to).
		if slices.Contains(msg.Style, styleBlink) && !blinkVisible(frame) {
			for i := range lines {
				lines[i] = strings.Repeat(" ", lipgloss.Width(lines[i]))
			}
		}
		last := len(lines) - 1

		for i, line := range lines {
			prefix := indent
			if i == 0 {
				prefix = styledPrefix
			}
			if i == last {
				sb.WriteString(prefix + line + strings.Repeat(" ", tsGap) + theme.Subtle.Render(ts) + "\n")
			} else {
				sb.WriteString(prefix + line + "\n")
			}
		}

		if att != "" {
			for line := range strings.SplitSeq(att, "\n") {
				sb.WriteString(indent + line + "\n")
			}
		}
	}
	return sb.String()
}

// renderDeletedTombstone renders a soft-deleted cIRC message: the author and
// original timestamp stay (per the API), but the body is replaced with a
// muted "[DELETED]" marker — no markdown, no attachments, no text style.
func renderDeletedTombstone(username, ts string, viewportWidth int) string {
	const tsGap = 2 // minimum space between the body and the timestamp
	const plainBody = "[DELETED]"
	tsWidth := lipgloss.Width(ts)
	prefix := "<" + theme.Subtle.Render(username) + ">  "
	rawPrefixWidth := len(username) + 4
	// Match renderCircMessages' layout exactly: reserve an elastic body field
	// of bodyFieldWidth (not the [DELETED] marker's own short width), so the
	// timestamp lands at the same right-aligned column as every other line.
	bodyFieldWidth := max(viewportWidth-rawPrefixWidth-tsWidth-tsGap, len(plainBody))
	pad := max(bodyFieldWidth-len(plainBody), 0) + tsGap
	return prefix + theme.Subtle.Render(plainBody) + strings.Repeat(" ", pad) + theme.Subtle.Render(ts) + "\n"
}

// renderCircMessagesWithSelection renders msgs exactly like renderCircMessages
// (byte-identical when selectedID == ""), additionally returning each
// message's start-line offset and rendered line-height (1:1 with msgs, so
// indices stay aligned even though system notices are never selectable), and
// highlighting the message whose ID matches selectedID with theme.SelectedRow.
//
// The highlight can't simply wrap the normally-styled block: theme.Highlight/
// theme.MeHighlight/markdown.RenderInline already emit their own ANSI reset
// codes, which would terminate an outer background style mid-line. Instead,
// the selected message's block is stripped back to plain text (ansi.Strip)
// and only that plain text is wrapped in theme.SelectedRow — the same
// approach settings.go uses for its selected-row highlight.
func renderCircMessagesWithSelection(msgs []model.Message, loc *time.Location, timeDisplayFormat string, viewportWidth int, currentUser string, selectedID string, revealed map[string]bool, frame int) (content string, offsets []int, heights []int) {
	offsets = make([]int, len(msgs))
	heights = make([]int, len(msgs))
	var sb strings.Builder
	var lineCount int
	for i, msg := range msgs {
		rendered := renderCircMessagesStyled([]model.Message{msg}, loc, timeDisplayFormat, viewportWidth, currentUser, revealed, frame)
		if selectedID != "" && msg.ID == selectedID {
			plain := strings.TrimSuffix(ansi.Strip(rendered), "\n")
			rendered = theme.SelectedRow.Width(viewportWidth).Render(plain) + "\n"
		}
		offsets[i] = lineCount
		// Not lipgloss.Height: it's strings.Count(s, "\n")+1, which treats
		// rendered's own trailing "\n" as a phantom extra line. That's right
		// for a whole-content string measured once, but wrong per-message
		// here — concatenating N such strings and re-splitting on "\n" (as
		// the viewport does) yields N real lines + 1 trailing empty line
		// total, not N*(realLines+1). Summing the inflated per-message
		// heights desyncs these offsets from the viewport's actual line
		// count, which silently breaks scrolling (millerPageNav computes a
		// YOffset the viewport's own maxYOffset() clamps right back down).
		h := strings.Count(rendered, "\n")
		heights[i] = h
		lineCount += h
		sb.WriteString(rendered)
	}
	return sb.String(), offsets, heights
}

// renderActionLine renders a /me-style action message in classic IRC form:
// "* username body *", right-aligned timestamp trailing the last wrapped
// line — no username bracket, matching how real IRC clients narrate actions
// in the third person. The API returns IsAction messages with Body already
// stripped of the username (just the action text), so it's assembled here.
func renderActionLine(username, body, ts string, viewportWidth int, currentUser string) string {
	username = sanitize.Strip(username)
	const suffix = " *"
	tsWidth := lipgloss.Width(ts)
	const tsGap = 2

	usernameStyle := theme.Highlight
	if currentUser != "" && username == currentUser {
		usernameStyle = theme.MeHighlight
	}

	prefix := "* " + usernameStyle.Render(username) + " "
	rawPrefixWidth := len(username) + 3 // "* " + " "
	indent := strings.Repeat(" ", rawPrefixWidth)

	bodyWidth := max(viewportWidth-rawPrefixWidth-len(suffix)-tsWidth-tsGap, 10)

	body = markdown.RenderInline(strings.TrimRight(body, "\n"), currentUser)
	lines := strings.Split(lipgloss.NewStyle().Width(bodyWidth).Render(body), "\n")
	last := len(lines) - 1

	var sb strings.Builder
	for i, line := range lines {
		p := indent
		if i == 0 {
			p = prefix
		}
		if i == last {
			// lipgloss pads every wrapped line to bodyWidth; trim that back off
			// so the closing "*" sits right after the text instead of being
			// pushed out to the right edge, then re-pad after it so the
			// timestamp still lands flush right.
			trimmed := strings.TrimRight(line, " ")
			content := trimmed + suffix
			pad := max(bodyWidth+len(suffix)-lipgloss.Width(content), 0)
			sb.WriteString(p + content + strings.Repeat(" ", pad) + strings.Repeat(" ", tsGap) + theme.Subtle.Render(ts) + "\n")
		} else {
			sb.WriteString(p + line + "\n")
		}
	}
	return sb.String()
}

// renderSystemNotice renders a local-only notice (e.g. a /help reply) as a
// muted, word-wrapped block prefixed with "*** " — distinct from real chat
// messages: no username bracket/bubble, no timestamp column, since it was
// never sent to or stored by the server.
func renderSystemNotice(body string, viewportWidth int) string {
	if viewportWidth < 10 {
		viewportWidth = 80
	}
	const prefix = "*** "
	bodyWidth := max(viewportWidth-len(prefix), 10)
	indent := strings.Repeat(" ", len(prefix))

	lines := strings.Split(lipgloss.NewStyle().Width(bodyWidth).Render(strings.TrimRight(body, "\n")), "\n")
	var sb strings.Builder
	for i, line := range lines {
		p := indent
		if i == 0 {
			p = prefix
		}
		sb.WriteString(theme.Subtle.Render(p+line) + "\n")
	}
	return sb.String()
}

// renderChatMessages renders a list of chat messages as bordered bubbles sized
// to their content (up to 75% of viewportWidth).
// Messages from currentUser use ActiveBorder (cyan) and are right-aligned.
// Others use Border (dim green) on the left.
// Pass currentUser="" to render all messages left-aligned (chatrooms).
func renderChatMessages(msgs []model.Message, currentUser string, loc *time.Location, timeDisplayFormat string, viewportWidth int) string {
	return renderChatMessagesStyled(msgs, currentUser, loc, timeDisplayFormat, viewportWidth, 0)
}

// renderChatMessagesStyled is renderChatMessages plus style/attachment
// support; frame drives the slow/wave/glitch animated styles.
// renderChatMessages is a thin wrapper passing 0, so its existing call sites
// are unaffected. Unlike cIRC's renderCircMessagesStyled, this has no
// spoiler or "art" handling — C-Mail doesn't support either yet (see the
// plan's Trade-offs section).
func renderChatMessagesStyled(msgs []model.Message, currentUser string, loc *time.Location, timeDisplayFormat string, viewportWidth int, frame int) string {
	if viewportWidth < 8 {
		viewportWidth = 80
	}
	// Maximum inner content width: 3/4 of viewport, minus 4 for border(2) + padding(2).
	maxContentW := max(viewportWidth*3/4-4, 4)

	var sb strings.Builder
	for _, msg := range msgs {
		if msg.IsSystem {
			sb.WriteString(renderSystemNotice(msg.Body, viewportWidth))
			sb.WriteString("\n")
			continue
		}
		ts := displayTime(msg.CreatedAt, loc, timeDisplayFormat, true)
		if msg.IsAction {
			sb.WriteString(renderActionLine(msg.From.Username, msg.Body, ts, viewportWidth, currentUser))
			sb.WriteString("\n")
			continue
		}
		isMe := currentUser != "" && msg.From.Username == currentUser
		username := sanitize.Strip(msg.From.Username)

		var header string
		if isMe {
			header = theme.Subtle.Render(ts) + "  " + theme.Highlight.Render("@"+username)
		} else {
			header = theme.Highlight.Render("@"+username) + "  " + theme.Subtle.Render(ts)
		}

		displayBody := messageDisplayBody(msg)
		attachments := renderAttachments(messageAttachments(msg))

		// Natural inner width: widest of the header, each raw body line, and
		// each attachment line, capped at max.
		naturalW := lipgloss.Width(header)
		for line := range strings.SplitSeq(displayBody, "\n") {
			if w := lipgloss.Width(line); w > naturalW {
				naturalW = w
			}
		}
		for line := range strings.SplitSeq(attachments, "\n") {
			if w := lipgloss.Width(line); w > naturalW {
				naturalW = w
			}
		}
		naturalW = min(naturalW, maxContentW)

		raw := substituteChars(displayBody, msg.ID, msg.Style, frame)
		body := applyAttributeStyle(strings.TrimRight(markdown.Render(raw, naturalW), "\n"), msg.Style)
		if slices.Contains(msg.Style, styleBlink) && !blinkVisible(frame) {
			bodyLines := strings.Split(body, "\n")
			for i := range bodyLines {
				bodyLines[i] = strings.Repeat(" ", lipgloss.Width(bodyLines[i]))
			}
			body = strings.Join(bodyLines, "\n")
		}
		rows := []string{header}
		if body != "" {
			rows = append(rows, body)
		}
		if attachments != "" {
			rows = append(rows, attachments)
		}
		content := lipgloss.JoinVertical(lipgloss.Left, rows...)

		if isMe {
			bubble := theme.ActiveBorder.Render(content)
			leftPad := strings.Repeat(" ", max(viewportWidth-naturalW-4, 0))
			for line := range strings.SplitSeq(bubble, "\n") {
				sb.WriteString(leftPad + line + "\n")
			}
		} else {
			sb.WriteString(theme.Border.Render(content) + "\n")
		}
		sb.WriteString("\n")
	}
	return sb.String()
}
