package screens

import (
	"time"

	"github.com/ragnar/cyber-tui/internal/model"
)

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
type SharedConfigMsg struct {
	Width          int
	Height         int
	Loc            *time.Location
	Relaxed        bool
	Settings       model.Settings
	WanderLust     bool
	MaxThreadDepth int
	Timezone       string
	ImageViewer    string
	OwnGuildSlug   string
}

// URLProvider is implemented by screens that can expose URLs from their
// currently focused item. App calls GetFocusedURLs when the user presses 'o'.
type URLProvider interface {
	GetFocusedURLs() []string
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
	ImageViewer    string
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
