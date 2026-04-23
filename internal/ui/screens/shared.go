package screens

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/markdown"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
	"github.com/ragnar/cyber-tui/internal/ui/urlutil"
)

// filterAmbiguousKeyMsg replaces ambiguous-width runes in a KeyRunes message with spaces
// before they reach a textarea or textinput component.
func filterAmbiguousKeyMsg(msg tea.KeyMsg) (tea.KeyMsg, bool) {
	if msg.Type != tea.KeyRunes {
		return msg, true
	}
	filtered := make([]rune, len(msg.Runes))
	for i, r := range msg.Runes {
		if runewidth.IsAmbiguousWidth(r) {
			filtered[i] = ' '
		} else {
			filtered[i] = r
		}
	}
	msg.Runes = filtered
	return msg, true
}

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

// BookmarkedIDsMsg is broadcast by App whenever the set of bookmarked post/reply
// IDs changes (load, create, delete). Screens that render posts or replies handle
// this to show the [★] indicator on already-bookmarked items.
type BookmarkedIDsMsg struct {
	PostIDs  map[string]struct{}
	ReplyIDs map[string]struct{}
}

// SharedConfigMsg is broadcast by App whenever display-affecting settings change
// (dimensions, timezone, display density). Each screen handles the fields it cares
// about in its own Update; fields it doesn't use are ignored.
//
// Adding a new screen only requires handling this message in that screen's Update —
// no App call sites need changing.
// URLProvider is implemented by screens that can expose URLs from their
// currently focused item. App calls GetFocusedURLs when the user presses 'o'.
type URLProvider interface {
	GetFocusedURLs() []string
}

type SharedConfigMsg struct {
	Width          int
	Height         int
	Loc            *time.Location
	Relaxed        bool
	Settings       model.Settings
	WanderLust     bool
	MaxThreadDepth int
	Timezone       string
}

// ShowUserProfileMsg is emitted by Feed, PostDetail, and Notifications when
// the user presses 'p' on the highlighted item.
type ShowUserProfileMsg struct{ Username string }

// BackFromProfileMsg is emitted by ProfileModel in read-only mode when ESC is pressed.
type BackFromProfileMsg struct{}

// SaveSettingsMsg is emitted by SettingsModel when the user presses ctrl+s
// with unsaved changes. App.handleSettings calls UpdateSettings and returns
// settingsSavedMsg or errMsg.
type SaveSettingsMsg struct {
	Settings       model.Settings
	WanderLust     bool
	MaxThreadDepth int
	Timezone       string
}

// BookmarkedMsg is sent back to the bookmarks screen after a successful CreateBookmark
// so it can show transient feedback.
type BookmarkedMsg struct{ PostID string }

// FollowUserMsg is emitted by ProfileModel when the user presses 'f' to follow another user.
type FollowUserMsg struct{ UserID string }

// UnfollowUserMsg is emitted by ProfileModel when the user presses 'f' to unfollow.
type UnfollowUserMsg struct{ FollowID string }

// LoadMoreJournalMsg is emitted by JournalModel when the viewport reaches the bottom
// and a next-page cursor is available. App intercepts this and fires the API call.
type LoadMoreJournalMsg struct{ Cursor string }

// SubmitSaveNoteMsg is emitted by JournalModel when the user presses ctrl+s in edit mode.
// NoteID is empty when creating a new note; non-empty when updating an existing one.
type SubmitSaveNoteMsg struct {
	NoteID  string
	Content string
	Topics  []string
}

// SubmitPublishNoteMsg is emitted by JournalModel when the user confirms publishing.
// Publishing creates a post with the note's content via POST /v1/posts.
type SubmitPublishNoteMsg struct {
	Content string
	Topics  []string
}

// SubmitDeleteNoteMsg is emitted by JournalModel when the user confirms deletion.
type SubmitDeleteNoteMsg struct{ NoteID string }

// DeletePostMsg is emitted by FeedModel or PostDetailModel when the user confirms
// deleting their own post.
type DeletePostMsg struct{ PostID string }

// DeleteReplyMsg is emitted by PostDetailModel when the user confirms deleting
// their own reply.
type DeleteReplyMsg struct {
	ReplyID string
	PostID  string
}

// ShowProfilePostMsg is emitted by ProfileModel when the user opens a post from
// the Posts sub-tab. Handled by App in handleProfile (sets return to screenProfile).
type ShowProfilePostMsg struct{ Post model.Post }

// ShowProfileReplyMsg is emitted by ProfileModel when the user opens an entry from
// the Replies sub-tab. App must fetch the full post by PostID and then scroll to ReplyID.
type ShowProfileReplyMsg struct {
	PostID  string
	ReplyID string
}

// Profile sub-tab lazy-load messages. Emitted by ProfileModel the first time a
// tab becomes active; App intercepts and fires the corresponding API call.

type ShowUserPostsMsg struct{ Username string }
type ShowUserRepliesMsg struct{ Username string }
type ShowUserFollowingMsg struct{ UserID string }
type ShowUserFollowersMsg struct{ UserID string }

// Profile sub-tab pagination messages. Emitted when the viewport reaches the
// bottom of a loaded list and more pages are available.

type LoadMoreUserPostsMsg struct {
	Username string
	Cursor   string
}
type LoadMoreUserRepliesMsg struct {
	Username string
	Cursor   string
}
type LoadMoreUserFollowingMsg struct {
	UserID string
	Cursor string
}
type LoadMoreUserFollowersMsg struct {
	UserID string
	Cursor string
}

// LoadNoteRevisionsMsg is emitted by JournalModel when the user presses 'h' on
// a selected note to view its revision history.
type LoadNoteRevisionsMsg struct{ NoteID string }

// LoadNoteRevisionMsg is emitted by JournalModel when the user presses Enter on
// a selected revision to preview it.
type LoadNoteRevisionMsg struct {
	NoteID         string
	RevisionNumber int
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
				if a.Genre != "" {
					label += " (" + a.Genre + ")"
				}
			}
			lines = append(lines, theme.Subtle.Render("[AUDIO]")+"  "+linkStyle.Render(label))
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
