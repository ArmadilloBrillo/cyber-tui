package api

import (
	"context"
	"time"

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
	// ResendVerification requests a fresh verification email for an account
	// whose email isn't verified yet (see EMAIL_NOT_VERIFIED).
	ResendVerification(idToken string) error
	Logout() error
	// RefreshSession proactively refreshes the ID token (and RTDB token) using
	// the stored refresh token, without waiting for a failed request to trigger
	// it. Used to reconnect a live RTDB subscription after the token expires.
	RefreshSession() error

	// Feed — pass empty cursor for first page; use returned cursor for next page.
	// Returns empty next-cursor when there are no more pages.
	GetFeed(cursor string) ([]model.Post, string, error)
	CreatePost(content, title, slug string, topics []string, isPublic, isNSFW bool) (model.Post, error)
	// GetPost fetches a single post by ID (used when jumping from a notification).
	GetPost(postID string) (model.Post, error)
	// GetPostBySlug fetches a single post by its author's username and per-author
	// slug (used when opening a post permalink URL from post/reply content).
	GetPostBySlug(username, slug string) (model.Post, error)

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
	// Poke sends a poke notification to a user. Rate limited to 1/hour, 8/day
	// across all users (not per-target).
	Poke(username string) error

	// Settings
	GetSettings() (model.Settings, error)
	UpdateSettings(update model.Settings) error

	// Chatrooms — list/history/send via REST; real-time delivery via RTDB SSE.
	GetRooms() ([]model.Room, error)
	// GetRoomMessages returns up to limit messages for roomID, oldest-first.
	// Pass before=0 for the latest page; pass a previous timestamp cursor for older pages.
	GetRoomMessages(roomID string, limit int, before int64) ([]model.Message, error)
	// SendRoomMessage returns the server's reply text for reply-only commands
	// (e.g. /help, which posts no message); empty for normal sends.
	SendRoomMessage(roomID, body string) (string, error)
	// MarkRoomRead resets the "new messages" indicator for the caller.
	MarkRoomRead(roomID string) error
	// FlagRoomMessage reports a chatroom message for review. Same semantics as FlagPost.
	FlagRoomMessage(roomID, messageID, reason string) (flagID string, alreadyFlagged bool, err error)
	// DeleteRoomMessage soft-deletes a message owned by the authenticated user.
	// Returns 409 if it's already deleted, 403 if it isn't the caller's own.
	DeleteRoomMessage(roomID, messageID string) error
	// SubscribeRoom opens a live RTDB SSE stream for the given chatroom.
	// Returns a channel of incoming messages and a cancel function. A delete
	// (by the caller or another client) arrives as a partial update rather
	// than a new message: only ID and Deleted are set on that model.Message —
	// callers must merge it onto the existing message by ID, not append it.
	SubscribeRoom(ctx context.Context, roomID string) (<-chan model.Message, context.CancelFunc, error)
	// GetRoomUsers returns who's currently present in roomID (server-side
	// staleness-filtered already; no client-side filtering needed).
	GetRoomUsers(roomID string) ([]model.RoomUser, error)
	// AnnouncePresence announces the caller's presence in roomID and reports
	// when they were last active. Returns the heartbeat cadence, staleness
	// window, and idle threshold (all ms) to honor — read these from the
	// response rather than hard-coding them.
	AnnouncePresence(roomID string, lastActivity time.Time) (heartbeatMs, staleAfterMs, idleAfterMs int, err error)
	// LeaveRoomPresence removes the caller from roomID's presence list immediately.
	LeaveRoomPresence(roomID string) error
	// SubscribeRoomPresence opens a live RTDB SSE stream for roomID's presence
	// node. Unlike SubscribeRoom, entries mutate/expire in place, so each
	// receive is a full, filtered (online + fresh) snapshot rather than a
	// single incremental event. staleAfterMs comes from AnnouncePresence's
	// response. initial seeds the merge state (pass the last known-good user
	// list on a reconnect so the panel doesn't flash empty; nil otherwise).
	SubscribeRoomPresence(ctx context.Context, roomID string, staleAfterMs int, initial []model.RoomUser) (<-chan []model.RoomUser, context.CancelFunc, error)

	// Notifications — cursor-paginated; mark-read methods are fire-and-forget.
	// Pass empty cursor for the first page; use the returned cursor for subsequent pages.
	// Set unreadOnly to true to request only unread notifications from the server.
	// Pass non-nil types to filter by notification type (e.g. []string{"reply","bookmark"});
	// pass nil for all types.
	GetNotifications(cursor string, unreadOnly bool, types []string) ([]model.Notification, string, error)
	// GetUnreadNotificationCount returns the server-side count of unread notifications.
	// The value is cached for ~5 s on the server side. exact is false once the true
	// count exceeds 100 — count is capped at 100 in that case; render "99+".
	GetUnreadNotificationCount() (count int, exact bool, err error)
	MarkNotificationRead(id string) error
	// MarkAllNotificationsRead marks up to 5,000 notifications read per call.
	// hasMore is true if the caller should call it again to mark the rest.
	MarkAllNotificationsRead() (hasMore bool, err error)

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
	// EditPost edits a post owned by the authenticated user. Supporter accounts
	// only, within 5 minutes of publishing; returns an *APIError with Status 403
	// otherwise. The response carries no fields worth returning — the caller
	// already has the full edited state and merges it locally.
	EditPost(postID, content, title string, topics []string, isPublic, isNSFW bool) error

	// Replies — deletion.
	// DeleteReply soft-deletes a reply owned by the authenticated user.
	DeleteReply(replyID string) error
	// EditReply edits a reply owned by the authenticated user. Same permission
	// window as EditPost; content is the only editable field.
	EditReply(replyID, content string) error

	// FlagPost reports a post for review. reason is optional (max 500 chars).
	// Idempotent: reporting the same post again returns alreadyFlagged=true
	// instead of an error. Returns 403 if the post is the caller's own.
	FlagPost(postID, reason string) (flagID string, alreadyFlagged bool, err error)
	// FlagReply reports a reply for review. Same semantics as FlagPost.
	FlagReply(replyID, reason string) (flagID string, alreadyFlagged bool, err error)

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
	// SendMessage returns the server's reply text for reply-only commands
	// (e.g. /help, which posts no message); empty for normal sends.
	SendMessage(conversationID, body string) (string, error)
	// StartConversation creates or retrieves a C-Mail conversation with recipientUsername.
	// Returns 201 for a new conversation, 200 for an existing one (both return the conversation).
	StartConversation(recipientUsername string) (model.Conversation, error)
	// MarkCMailRead resets the unread count for the conversation.
	MarkCMailRead(conversationID string) error
	// SubscribeDMs opens a live RTDB SSE stream for the given conversation.
	// Returns a channel of incoming messages and a cancel function.
	// The channel is closed when cancel is called or the stream ends.
	SubscribeDMs(ctx context.Context, convID string) (<-chan model.Message, context.CancelFunc, error)
	// AnnounceTyping announces that the caller is typing in conversationID.
	// Returns the heartbeat cadence and staleness window (both ms) to honor —
	// read these from the response rather than hard-coding them.
	AnnounceTyping(conversationID string) (heartbeatMs, staleAfterMs int, err error)
	// ClearTyping clears the caller's typing flag in conversationID immediately.
	ClearTyping(conversationID string) error
	// SubscribeDMTyping opens a live RTDB SSE stream for conversationID's
	// typing node. Each receive is a full, filtered (typing + fresh) snapshot
	// rather than a single incremental event. staleAfterMs comes from
	// AnnounceTyping's response, or a fixed default if subscribing before ever
	// announcing typing ourselves.
	SubscribeDMTyping(ctx context.Context, conversationID string, staleAfterMs int) (<-chan []model.TypingUser, context.CancelFunc, error)
	// SubscribeUserConversations opens a live RTDB SSE stream for uid's
	// conversation-list summary node. Each receive is the full converted+
	// sorted conversation list (unread first, then most recently active)
	// rather than a single incremental event, since a conversation summary
	// mutates in place. initial seeds the merge state for a reconnect so the
	// list doesn't go blank until the first live event arrives.
	SubscribeUserConversations(ctx context.Context, uid string, initial []model.Conversation) (<-chan []model.Conversation, context.CancelFunc, error)

	// Search — full-text search across users, posts, and replies (v0.7).
	// Search returns the grouped "type=all" preview: up to 8 hits per category,
	// no pagination, no total count. A category at exactly 8 hits may have more —
	// drill into it with SearchPosts/SearchReplies/SearchUsers.
	Search(query string) (model.SearchPreview, error)
	// SearchPosts/SearchReplies/SearchUsers return one paginated category.
	// Pass empty cursor for first page; use the returned cursor for the next page
	// (opaque to the caller — server-side it's a page number, but callers never
	// need to know that, matching every other cursor-paginated method here).
	SearchPosts(query, cursor string) ([]model.Post, string, error)
	SearchReplies(query, cursor string) ([]model.Reply, string, error)
	SearchUsers(query, cursor string) ([]model.User, string, error)
}
