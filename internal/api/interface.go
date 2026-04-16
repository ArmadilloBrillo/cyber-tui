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
	CreatePost(content string, topics []string) (model.Post, error)
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
	// Follow follows a user by their user ID. Returns the follow document ID (needed for Unfollow).
	Follow(followedID string) (string, error)
	// Unfollow removes a follow relationship by its document ID.
	Unfollow(followID string) error

	// Settings
	GetSettings() (model.Settings, error)
	UpdateSettings(update model.Settings) error

	// Chatrooms — NOTE: real impl uses Firebase RTDB with RTDBToken — pending feature/rtdb-chat
	GetRooms() ([]model.Room, error)
	GetRoomMessages(roomID string, limit int) ([]model.Message, error)
	SendRoomMessage(roomID, body string) error

	// Notifications — cursor-paginated; mark-read methods are fire-and-forget.
	// Pass empty cursor for the first page; use the returned cursor for subsequent pages.
	GetNotifications(cursor string) ([]model.Notification, string, error)
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

	// Posts — deletion.
	// DeletePost soft-deletes a post owned by the authenticated user.
	DeletePost(postID string) error

	// Replies — deletion.
	// DeleteReply soft-deletes a reply owned by the authenticated user.
	DeleteReply(replyID string) error

	// Notes — private notes visible only to the author.
	// GetNotes returns the latest revision of each note, newest first.
	// Pass empty cursor for first page; use returned cursor for next page.
	GetNotes(cursor string) ([]model.Note, string, error)
	// CreateNote creates a new note. topics is optional (max 3, lowercase).
	CreateNote(content string, topics []string) (model.Note, error)
	// UpdateNote creates a new revision on an existing note.
	UpdateNote(noteID, content string, topics []string) error
	// DeleteNote soft-deletes all revisions of a note.
	DeleteNote(noteID string) error

	// Direct messages — backed by Firebase RTDB (see internal/rtdb).
	GetConversations() ([]model.Conversation, error)
	GetMessages(conversationID string, limit int) ([]model.Message, error)
	SendMessage(conversationID, body string) error
	// SubscribeDMs opens a live SSE stream for the given conversation.
	// Returns a channel of incoming messages and a cancel function.
	// The channel is closed when cancel is called or the stream ends.
	SubscribeDMs(ctx context.Context, convID string) (<-chan model.Message, context.CancelFunc, error)
}
