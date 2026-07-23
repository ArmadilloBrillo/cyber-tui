package screens

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/ragnar/cyber-tui/internal/model"
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
func renderCircMessages(msgs []model.Message, loc *time.Location, timeDisplayFormat string, viewportWidth int) string {
	if viewportWidth < 20 {
		viewportWidth = 80
	}
	const tsGap = 2 // minimum space between the wrapped text and the timestamp
	var sb strings.Builder
	for _, msg := range msgs {
		ts := displayTime(msg.CreatedAt, loc, timeDisplayFormat, true)
		tsWidth := lipgloss.Width(ts)

		// Styled prefix: <username>  (plain width = len(username) + 4)
		styledPrefix := "<" + theme.Highlight.Render(msg.From.Username) + ">  "
		rawPrefixWidth := len(msg.From.Username) + 4
		indent := strings.Repeat(" ", rawPrefixWidth)

		bodyWidth := max(viewportWidth-rawPrefixWidth-tsWidth-tsGap, 10)

		body := strings.TrimRight(msg.Body, "\n")
		lines := strings.Split(lipgloss.NewStyle().Width(bodyWidth).Render(body), "\n")
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
	}
	return sb.String()
}

// renderChatMessages renders a list of chat messages as bordered bubbles sized
// to their content (up to 75% of viewportWidth).
// Messages from currentUser use ActiveBorder (cyan) and are right-aligned.
// Others use Border (dim green) on the left.
// Pass currentUser="" to render all messages left-aligned (chatrooms).
func renderChatMessages(msgs []model.Message, currentUser string, loc *time.Location, timeDisplayFormat string, viewportWidth int) string {
	if viewportWidth < 8 {
		viewportWidth = 80
	}
	// Maximum inner content width: 3/4 of viewport, minus 4 for border(2) + padding(2).
	maxContentW := max(viewportWidth*3/4-4, 4)

	var sb strings.Builder
	for _, msg := range msgs {
		ts := displayTime(msg.CreatedAt, loc, timeDisplayFormat, true)
		isMe := currentUser != "" && msg.From.Username == currentUser

		var header string
		if isMe {
			header = theme.Subtle.Render(ts) + "  " + theme.Highlight.Render("@"+msg.From.Username)
		} else {
			header = theme.Highlight.Render("@"+msg.From.Username) + "  " + theme.Subtle.Render(ts)
		}

		// Natural inner width: widest of the header and each raw body line, capped at max.
		naturalW := lipgloss.Width(header)
		for line := range strings.SplitSeq(msg.Body, "\n") {
			if w := lipgloss.Width(line); w > naturalW {
				naturalW = w
			}
		}
		naturalW = min(naturalW, maxContentW)

		body := strings.TrimRight(markdown.Render(msg.Body, naturalW), "\n")
		content := lipgloss.JoinVertical(lipgloss.Left, header, body)

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
