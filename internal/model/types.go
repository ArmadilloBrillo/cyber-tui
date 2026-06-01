package model

import "time"

// Tokens holds the three tokens returned by POST /v1/auth/login.
type Tokens struct {
	IDToken      string
	RefreshToken string
	RTDBToken    string
}

type User struct {
	ID                string
	Username          string
	DisplayName       string
	Email             string
	Bio               string
	WebsiteUrl        string
	WebsiteName       string
	WebsiteImageUrl   string
	PinnedPostID      string
	LocationName      string
	LocationLatitude  float64
	LocationLongitude float64
	FollowersCount    int
	FollowingCount    int
	PostsCount        int
	GuildSlug         string // empty when not a guild member
}

// Follow maps to a record returned by GET /v1/follows.
type Follow struct {
	ID               string
	FollowerID       string
	FollowedID       string
	FollowerUsername string
	FollowedUsername string
	CreatedAt        time.Time
}

// Attachment represents a media attachment on a post or reply.
// Type is "image" or "audio". Src is always present.
// Image-specific: Width, Height. Audio-specific: Origin, Artist, Title, Genre.
type Attachment struct {
	Type   string
	Src    string
	Width  int
	Height int
	Origin string
	Artist string
	Title  string
	Genre  string
}

// Post maps to the post shape returned by GET /v1/posts.
// Author is stored as flat fields (authorId + authorUsername) matching the API response.
type Post struct {
	ID             string
	AuthorID       string
	AuthorUsername string
	Content        string
	Title          string
	Slug           string
	GuildID        string
	GuildSlug      string
	IsGuildThread  bool
	Topics         []string
	RepliesCount   int
	BookmarksCount int
	IsPublic       bool
	IsNSFW         bool
	Deleted        bool
	CreatedAt      time.Time
	Attachments    []Attachment
}

// Reply maps to the reply shape returned by GET /v1/posts/:id/replies.
type Reply struct {
	ID             string
	PostID         string
	AuthorID       string
	AuthorUsername string
	Content        string
	ParentReplyID  string
	CreatedAt      time.Time
	Attachments    []Attachment
}

// ProfileUpdate carries the fields accepted by PATCH /v1/users/me.
// Pointer fields: only non-nil values are sent in the real HTTP client.
type ProfileUpdate struct {
	Bio               *string
	DisplayName       *string
	PinnedPostID      *string
	WebsiteUrl        *string
	WebsiteName       *string
	WebsiteImageUrl   *string
	LocationName      *string
	LocationLatitude  *float64
	LocationLongitude *float64
}

// Message, Conversation, Room are used by the RTDB chat/DM layer.
// They are not sourced from the REST API.

type Message struct {
	ID        string
	From      User
	Body      string
	CreatedAt time.Time
}

type Conversation struct {
	ID           string
	Participants []User
	Messages     []Message
}

type Room struct {
	ID          string
	Name        string
	Description string
	Members     int
}

// NotificationPrefs maps to the notifications sub-object in GET/PATCH /v1/settings.
type NotificationPrefs struct {
	Bookmark bool
	Reply    bool
	Poke     bool
}

// Settings maps to the fields returned by GET /v1/settings.
// KeyboardBindings and MutedUsersByRoom are opaque JSON objects — not modelled yet.
type Settings struct {
	Notifications      NotificationPrefs
	FilterNSFW         bool
	ShowFollowerCount  bool
	AutoWatchOnReply   bool
	IconTheme          string
	FollowedTopics     []string
	MutedTopics        []string
	ImagePixelSize     string // named preset or pixel multiplier, e.g. "sharp", "2"
	TimeDisplayFormat string // "datetime", "relative", "unix", or "swatch"
	DefaultPublicPost bool
}

// Bookmark maps to the shape returned by GET /v1/bookmarks.
// Exactly one of Post or Reply is non-nil, depending on Type.
type Bookmark struct {
	ID        string
	Type      string // "post" or "reply"
	PostID    string // set when Type == "post"
	ReplyID   string // set when Type == "reply"
	Post      *Post  // embedded post content (when Type == "post")
	Reply     *Reply // embedded reply content (when Type == "reply")
	CreatedAt time.Time
}

// Topic maps to the shape returned by GET /v1/topics.
type Topic struct {
	Slug      string
	PostCount int
}

// Guild maps to the shape returned by GET /v1/guilds and GET /v1/guilds/:slug.
// IsMember and Role are only populated by the single-guild endpoint.
type Guild struct {
	ID              string
	Name            string
	Slug            string
	Icon            string
	Bio             string
	MemberCount     int
	FounderUsername string
	CreatedAt       time.Time
	IsMember        bool
	Role            string // "founder", "member", or ""
	Link            string
	LinkText        string
}

// GuildMember represents a single membership returned by GET /v1/guilds/:slug/members.
type GuildMember struct {
	MembershipID string
	GuildID      string
	GuildSlug    string
	UserID       string
	Username     string
	Role         string // "founder" or "member"
	JoinedAt     time.Time
	DisplayName  string
}

// Note is a private note visible only to the author.
// RevisionNumber increments each time the note is updated via PATCH /v1/notes/:id.
type Note struct {
	ID             string
	AuthorID       string
	Content        string
	Topics         []string // max 3; sent on create/update but not returned by the list API
	RevisionNumber int
	Deleted        bool
	CreatedAt      time.Time
}

// NoteRevision represents a single historical revision of a note,
// returned by GET /v1/notes/:id/revisions.
type NoteRevision struct {
	RevisionNumber int
	Content        string
	Topics         []string
	CreatedAt      time.Time
}

// NotificationActor is the user who triggered the notification.
type NotificationActor struct {
	ID       string
	Username string
}

// Notification maps to the shape returned by GET /v1/notifications.
// TargetID is always a post ID for navigable notifications.
// TargetType describes the notification category ("post" or "reply"); empty for poke/new_follower.
// ReplyID is set from metadata.replyId for reply/thread_reply notifications and identifies
// the specific reply to scroll to in PostDetail.
type Notification struct {
	ID                   string
	Type                 string // "reply", "reply_mention", "post_mention", "thread_reply", "new_post_friend", "new_post_following", "new_follower", "unfollowed", "bookmark", "poke", "guild_new_thread", "chat_mention", "dm_message", "supporter_granted", "supporter_removed", "hacker_granted", "hacker_removed", "image_permission_granted", "image_permission_removed", "attachment_permission_granted", "attachment_permission_removed", "system_ban"
	Read                 bool
	CreatedAt            time.Time
	Actor                NotificationActor
	TargetID             string
	TargetType           string // "post", "reply", or ""
	ReplyID              string // populated for reply/thread_reply; the specific reply to highlight
	ThreadAuthorUsername string // set for thread_reply; the original thread's author
	GuildName            string // set for guild_new_thread; the guild display name
}
