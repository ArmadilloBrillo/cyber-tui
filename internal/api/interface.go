package api

import (
	"context"

	"github.com/ragnar/cyber-tui/internal/model"
)

// Client defines all interactions with the cyberspace.online API.
// The mock and real implementations both satisfy this interface.
type Client interface {
	// Auth
	Login(email, password string) (model.Tokens, error)
	// LoginWithRefreshToken exchanges a saved refresh token for a fresh set of
	// tokens (IDToken + RTDBToken) without requiring the user's password.
	LoginWithRefreshToken(refreshToken string) (model.Tokens, error)
	Logout() error

	// Feed — pass empty cursor for first page; use returned cursor for next page.
	// Returns empty next-cursor when there are no more pages.
	GetFeed(cursor string) ([]model.Post, string, error)
	CreatePost(content, title, slug string, topics []string, isPublic, isNSFW bool) (model.Post, error)
	// GetPost fetches a single post by ID (used when jumping from a notification).
	GetPost(postID string) (model.Post, error)

	// Replies — first page only (pagination deferred).
	GetPostReplies(postID string) ([]model.Reply, error)
	// GetReply fetches a single reply by ID (used when opening a reply bookmark).
	GetReply(replyID string) (model.Reply, error)
	// CreateReply posts a reply to postID. Pass empty parentReplyID for top-level replies.
	CreateReply(postID, content, parentReplyID string) (model.Reply, error)

	// Profile
	GetOwnProfile() (model.User, error)
	GetProfile(username string) (model.User, error)
	UpdateProfile(update model.ProfileUpdate) error

	// Follows — cursor-paginated list; follow/unfollow are fire-and-forget.
	// Pass empty cursor for first page; use returned cursor for subsequent pages.
	// GetFollowing returns accounts the current user follows.
	GetFollowing(cursor string) ([]model.Follow, string, error)
	// GetFollowers returns accounts that follow the current user.
	GetFollowers(cursor string) ([]model.Follow, string, error)
	// GetUserFollows returns followers or following for any user by their ID.
	// followType must be "followers" or "following".
	GetUserFollows(userID, followType, cursor string) ([]model.Follow, string, error)
	// Follow follows a user by their user ID. Returns the follow document ID (needed for Unfollow).
	Follow(followedID string) (string, error)
	// Unfollow removes a follow relationship by its document ID.
	Unfollow(followID string) error

	// Settings
	GetSettings() (model.Settings, error)
	UpdateSettings(update model.Settings) error

	// Chatrooms — list/history/send via REST; real-time delivery via RTDB SSE.
	GetRooms() ([]model.Room, error)
	// GetRoomMessages returns up to limit messages for roomID, oldest-first.
	// Pass before=0 for the latest page; pass a previous timestamp cursor for older pages.
	GetRoomMessages(roomID string, limit int, before int64) ([]model.Message, error)
	SendRoomMessage(roomID, body string) error
	// MarkRoomRead resets the "new messages" indicator for the caller.
	MarkRoomRead(roomID string) error
	// SubscribeRoom opens a live RTDB SSE stream for the given chatroom.
	// Returns a channel of incoming messages and a cancel function.
	SubscribeRoom(ctx context.Context, roomID string) (<-chan model.Message, context.CancelFunc, error)

	// Notifications — cursor-paginated; mark-read methods are fire-and-forget.
	// Pass empty cursor for the first page; use the returned cursor for subsequent pages.
	// Set unreadOnly to true to request only unread notifications from the server.
	// Pass non-nil types to filter by notification type (e.g. []string{"reply","bookmark"});
	// pass nil for all types.
	GetNotifications(cursor string, unreadOnly bool, types []string) ([]model.Notification, string, error)
	// GetUnreadNotificationCount returns the server-side count of unread notifications.
	// The value is cached for ~5 s on the server side.
	GetUnreadNotificationCount() (int, error)
	MarkNotificationRead(id string) error
	MarkAllNotificationsRead() error

	// Bookmarks — cursor-paginated list; create/delete are fire-and-forget.
	// Pass empty cursor for the first page; use the returned cursor for subsequent pages.
	GetBookmarks(cursor string) ([]model.Bookmark, string, error)
	// CreateBookmark saves a post or reply. Pass postID for posts, replyID for replies (the other empty).
	CreateBookmark(postID, replyID string) (string, error) // returns bookmarkId
	DeleteBookmark(id string) error

	// Topics — browse topics and posts within them.
	// GetTopics returns topics sorted by post count (most popular first).
	// Pass empty cursor for first page; use returned cursor for next page.
	GetTopics(cursor string) ([]model.Topic, string, error)
	// GetTopicPosts returns paginated posts for a topic.
	// Pass empty cursor for first page; use returned cursor for next page.
	GetTopicPosts(slug string, cursor string) ([]model.Post, string, error)

	// Guilds — browse guilds and threads within them.
	// GetGuilds returns guilds with at least one member, most populated first.
	// Pass empty cursor for first page; use returned cursor for next page.
	GetGuilds(cursor string) ([]model.Guild, string, error)
	// GetGuild fetches a single guild by slug including the caller's IsMember and Role.
	GetGuild(slug string) (model.Guild, error)
	// GetGuildPosts returns paginated threads for a guild, most recently active first.
	// Pass empty cursor for first page; use returned cursor for next page.
	GetGuildPosts(slug string, cursor string) ([]model.Post, string, error)
	// CreateGuildPost creates a new thread in a guild. Caller must be a member.
	// postSlug is optional; pass empty string for server-generated slug.
	CreateGuildPost(slug, content, title, postSlug string, topics []string) (model.Post, error)
	// GetGuildMembers returns paginated members for a guild, oldest-joined first.
	// Pass empty cursor for first page; use returned cursor for next page.
	GetGuildMembers(slug, cursor string) ([]model.GuildMember, string, error)
	// JoinGuild joins the guild identified by slug. Returns an error if the user is
	// already a member of another guild (409) or if the slug does not exist (404).
	JoinGuild(slug string) error
	// LeaveGuild leaves the guild identified by slug. Returns an error if the user
	// is the founder (403) or is not a member.
	LeaveGuild(slug string) error

	// Thread watching — watch/unwatch individual threads.
	// GetWatches returns all watched threads, cursor-paginated (limit=50).
	// Pass empty cursor for first page; use returned cursor for next page.
	GetWatches(cursor string) ([]model.Watch, string, error)
	// WatchPost subscribes the user to thread_reply notifications for the given post.
	WatchPost(postID string) error
	// UnwatchPost unsubscribes the user from the given thread.
	UnwatchPost(postID string) error

	// Posts — deletion.
	// DeletePost soft-deletes a post owned by the authenticated user.
	DeletePost(postID string) error

	// Replies — deletion.
	// DeleteReply soft-deletes a reply owned by the authenticated user.
	DeleteReply(replyID string) error

	// User posts/replies — cursor-paginated history for any user.
	GetUserPosts(username, cursor string) ([]model.Post, string, error)
	GetUserReplies(username, cursor string) ([]model.Reply, string, error)

	// Notes — private notes visible only to the author.
	// GetNotes returns the latest revision of each note, newest first.
	// Pass empty cursor for first page; use returned cursor for next page.
	GetNotes(cursor string) ([]model.Note, string, error)
	// GetNote fetches the latest revision of a single note.
	// Use GetNoteRevision to fetch a specific revision number.
	GetNote(noteID string) (model.Note, error)
	// GetNoteRevision fetches a specific revision of a note by revision number.
	GetNoteRevision(noteID string, revision int) (model.Note, error)
	// GetNoteRevisions returns all revisions for a note, newest first.
	// Pass empty cursor for first page; use returned cursor for next page.
	GetNoteRevisions(noteID, cursor string) ([]model.NoteRevision, string, error)
	// CreateNote creates a new note. topics is optional (max 3, lowercase).
	CreateNote(content string, topics []string) (model.Note, error)
	// UpdateNote creates a new revision on an existing note.
	UpdateNote(noteID, content string, topics []string) error
	// DeleteNote soft-deletes all revisions of a note.
	DeleteNote(noteID string) error

	// Direct messages (C-Mail) — list/history/send via REST; real-time delivery via RTDB SSE.
	GetConversations() ([]model.Conversation, error)
	// GetMessages returns up to limit messages for conversationID, oldest-first.
	// Pass before=0 for the latest page; pass a previous message's timestamp for older pages.
	GetMessages(conversationID string, limit int, before int64) ([]model.Message, error)
	SendMessage(conversationID, body string) error
	// StartConversation creates or retrieves a C-Mail conversation with recipientUsername.
	// Returns 201 for a new conversation, 200 for an existing one (both return the conversation).
	StartConversation(recipientUsername string) (model.Conversation, error)
	// MarkCMailRead resets the unread count for the conversation.
	MarkCMailRead(conversationID string) error
	// SubscribeDMs opens a live RTDB SSE stream for the given conversation.
	// Returns a channel of incoming messages and a cancel function.
	// The channel is closed when cancel is called or the stream ends.
	SubscribeDMs(ctx context.Context, convID string) (<-chan model.Message, context.CancelFunc, error)
}
