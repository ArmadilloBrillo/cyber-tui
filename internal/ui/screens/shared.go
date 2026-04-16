package screens

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
)

const postMaxBodyLines = 4

// RenderPost renders a model.Post as a bordered card matching the feed style.
// selected controls the border colour (active vs inactive).
// width is the full terminal width; loc and timeFormat control the timestamp display.
func RenderPost(p model.Post, selected bool, width int, loc *time.Location, timeFormat string) string {
	innerWidth := width - 4

	left := lipgloss.JoinHorizontal(lipgloss.Top,
		theme.Highlight.Render("@"+p.AuthorUsername),
		theme.Subtle.Render("  "+displayTime(p.CreatedAt, loc, timeFormat, false)),
	)
	var repliesLabel string
	switch p.RepliesCount {
	case 0:
		// show nothing
	case 1:
		repliesLabel = theme.Subtle.Render("1 reply")
	default:
		repliesLabel = theme.Subtle.Render(fmt.Sprintf("%d replies", p.RepliesCount))
	}
	var header string
	if innerWidth > 0 {
		gap := innerWidth - lipgloss.Width(left) - lipgloss.Width(repliesLabel)
		if gap > 0 {
			header = left + strings.Repeat(" ", gap) + repliesLabel
		} else {
			header = left
		}
	} else {
		header = left
	}

	var body string
	if innerWidth > 0 {
		wrapped := theme.Base.Width(innerWidth).Render(p.Content)
		lines := strings.Split(wrapped, "\n")
		if len(lines) > postMaxBodyLines {
			body = strings.Join(lines[:postMaxBodyLines], "\n")
			more := len(lines) - postMaxBodyLines
			body += "\n" + theme.Subtle.Render(fmt.Sprintf("  ▼ %d more lines", more))
		} else {
			body = wrapped
		}
	} else {
		body = theme.Base.Render(p.Content)
	}

	topics := ""
	for _, t := range p.Topics {
		topics += theme.Subtle.Render("#"+t) + " "
	}

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

// SharedConfigMsg is broadcast by App whenever display-affecting settings change
// (dimensions, timezone, display density). Each screen handles the fields it cares
// about in its own Update; fields it doesn't use are ignored.
//
// Adding a new screen only requires handling this message in that screen's Update —
// no App call sites need changing.
type SharedConfigMsg struct {
	Width    int
	Height   int
	Loc      *time.Location
	Relaxed  bool
	Settings model.Settings
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
	Settings model.Settings
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
