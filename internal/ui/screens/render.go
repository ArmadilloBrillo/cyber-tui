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
// bookmarked adds a [★] badge to the header right side.
// width is the full terminal width; loc and timeFormat control the timestamp display.
func RenderPost(p model.Post, selected bool, bookmarked bool, width int, loc *time.Location, timeFormat string) string {
	innerWidth := width - 4

	left := lipgloss.JoinHorizontal(lipgloss.Top,
		theme.Highlight.Render("@"+p.AuthorUsername),
		theme.Subtle.Render("  "+displayTime(p.CreatedAt, loc, timeFormat, false)),
	) + audioIcon(p.Attachments) + bookmarkIcon(bookmarked)
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

	var body string
	if innerWidth > 0 {
		rendered := markdown.Render(p.Content, innerWidth)
		lines := strings.Split(rendered, "\n")
		if len(lines) > postMaxBodyLines {
			body = strings.Join(lines[:postMaxBodyLines], "\n")
			more := len(lines) - postMaxBodyLines
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
	return boxStyle.Render(
		lipgloss.JoinVertical(lipgloss.Left,
			header,
			body,
			fmt.Sprintf("\n%s", topics),
		),
	)
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
				line += theme.Subtle.Render(" ("+a.Genre+")")
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

// renderChatMessages renders a list of chat messages (used by both CMail and Chatrooms).
func renderChatMessages(msgs []model.Message, loc *time.Location, timeDisplayFormat string, viewportWidth int) string {
	var sb strings.Builder
	for _, msg := range msgs {
		ts := theme.Subtle.Render(displayTime(msg.CreatedAt, loc, timeDisplayFormat, true))
		author := theme.Highlight.Render("@" + msg.From.Username)
		body := markdown.Render(msg.Body, viewportWidth)
		sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, ts, "  ", author, "  ", body) + "\n")
	}
	return sb.String()
}
