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
