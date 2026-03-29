package model

import "time"

// Tokens holds the three tokens returned by POST /v1/auth/login.
type Tokens struct {
	IDToken      string
	RefreshToken string
	RTDBToken    string
}

type User struct {
	ID           string
	Username     string
	DisplayName  string
	Email        string
	Bio          string
	WebsiteUrl   string
	PinnedPostID string
	LocationName string
}

// Post maps to the post shape returned by GET /v1/posts.
// Author is stored as flat fields (authorId + authorUsername) matching the API response.
type Post struct {
	ID             string
	AuthorID       string
	AuthorUsername string
	Content        string
	Topics         []string
	RepliesCount   int
	BookmarksCount int
	IsPublic       bool
	IsNSFW         bool
	Deleted        bool
	CreatedAt      time.Time
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
}

// ProfileUpdate carries the fields accepted by PATCH /v1/users/me.
// Pointer fields: only non-nil values are sent in the real HTTP client.
type ProfileUpdate struct {
	Bio          *string
	DisplayName  *string
	PinnedPostID *string
	WebsiteUrl   *string
	LocationName *string
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
