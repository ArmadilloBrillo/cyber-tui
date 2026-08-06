package screens

import (
	"time"

	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
)

// BookmarkedIDsMsg is broadcast by App whenever the set of bookmarked post/reply
// IDs changes (load, create, delete). Screens that render posts or replies handle
// this to show the [★] indicator on already-bookmarked items.
type BookmarkedIDsMsg struct {
	PostIDs  map[string]struct{}
	ReplyIDs map[string]struct{}
}

// WatchedPostIDsMsg is broadcast by App whenever the set of watched thread IDs
// changes (progressive load, watch, unwatch). Screens handle this to show the
// [◉] indicator on watched threads.
type WatchedPostIDsMsg struct {
	PostIDs map[string]struct{}
}

// ToggleWatchPostMsg is emitted by Feed or PostDetail when the user presses 'w'
// on a thread-root post to toggle its watch state.
type ToggleWatchPostMsg struct {
	PostID string
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
	LayoutName     string // "tabs" or "miller"; used by settings screen to show current value
}

// URLProvider is implemented by screens that can expose URLs from their
// currently focused item. App calls GetFocusedURLs when the user presses 'o'.
type URLProvider interface {
	GetFocusedURLs() []string
}

// ShowUserProfileMsg is emitted by Feed, PostDetail, and Notifications when
// the user presses 'p' on the highlighted item.
type ShowUserProfileMsg struct{ Username string }

// StartConversationMsg is emitted by any screen when the user presses 'c' on a
// highlighted post, reply, notification, or profile to open (or create) a C-Mail
// conversation with that user. App guards against self-DMs.
type StartConversationMsg struct{ Username string }

// OpenRoomMsg is emitted by Notifications when the user presses Enter on a
// chat_mention notification. RoomSlug identifies the target cIRC room; App
// activates the Chatrooms screen and stashes the slug on ChatroomsModel so it
// can auto-enter detail mode once the room list has (re)loaded. NotifID lets
// App mark the notification read in the same batch.
type OpenRoomMsg struct {
	RoomSlug string
	NotifID  string
}

// BackFromProfileMsg is emitted by ProfileModel in read-only mode when ESC is pressed.
type BackFromProfileMsg struct{}

// LeaveCMailMsg is emitted by CMailModel when ESC is pressed in detail mode
// on a conversation that was reached via a deep link (canGoBack). App
// returns to the screen the user deep-linked from instead of dropping to
// C-Mail's own conversation list.
type LeaveCMailMsg struct{}

// LeaveChatroomsMsg is emitted by ChatroomsModel when ESC is pressed in
// detail mode on a room that was reached via a deep link (canGoBack). App
// returns to the screen the user deep-linked from instead of dropping to
// Chatrooms' own room list.
type LeaveChatroomsMsg struct{}

// SaveSettingsMsg is emitted by SettingsModel when the user presses ctrl+s
// with unsaved changes. App.handleSettings calls UpdateSettings and returns
// settingsSavedMsg or errMsg.
type SaveSettingsMsg struct {
	Settings       model.Settings
	WanderLust     bool
	MaxThreadDepth int
	Timezone       string
	ImageViewer    string
	LayoutName     string // "tabs" or "miller"
	RemoteChanged  bool   // true when API-managed fields differ from the last saved baseline
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

// FlagPostMsg is emitted by FeedModel or PostDetailModel when the user
// confirms reporting a post. Reason is optional (max 500 chars).
type FlagPostMsg struct {
	PostID string
	Reason string
}

// FlagReplyMsg is emitted by PostDetailModel when the user confirms
// reporting a reply. Reason is optional (max 500 chars).
type FlagReplyMsg struct {
	ReplyID string
	PostID  string
	Reason  string
}

// FlagMessageMsg is emitted by ChatroomsModel when the user confirms
// reporting a cIRC message. Reason is optional (max 500 chars).
type FlagMessageMsg struct {
	RoomID    string
	MessageID string
	Reason    string
}

// DeleteRoomMessageMsg is emitted by ChatroomsModel when the user confirms
// deleting their own cIRC message.
type DeleteRoomMessageMsg struct {
	RoomID    string
	MessageID string
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

// PreviewPaletteMsg is emitted by ThemeEditorModel on every edit so App can
// live-preview via theme.SetCustomPalette + refreshViewports, mirroring the
// theme picker's up/down preview.
type PreviewPaletteMsg struct{ Palette theme.Palette }

// SaveThemeMsg is emitted by ThemeEditorModel on ctrl+s with a valid, dirty
// palette. App persists it (unless ephemeral) and applies it as "custom".
type SaveThemeMsg struct{ Palette theme.Palette }

// CloseThemeEditorMsg is emitted by ThemeEditorModel on esc (row-nav mode,
// not mid-field-edit) to close without saving.
type CloseThemeEditorMsg struct{}
