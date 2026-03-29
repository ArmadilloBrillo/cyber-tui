package api

import "github.com/ragnar/cyber-tui/internal/model"

// Client defines all interactions with the cyberspace.online API.
// The mock and real implementations both satisfy this interface.
type Client interface {
	// Auth
	Login(email, password string) (model.Tokens, error)
	Logout() error

	// Feed — pass empty cursor for first page; use returned cursor for next page.
	// Returns empty next-cursor when there are no more pages.
	GetFeed(cursor string) ([]model.Post, string, error)
	CreatePost(content string, topics []string) (model.Post, error)

	// Profile
	GetOwnProfile() (model.User, error)
	GetProfile(username string) (model.User, error)
	UpdateProfile(update model.ProfileUpdate) error

	// Chatrooms — NOTE: real impl uses Firebase RTDB with RTDBToken — pending feature/rtdb-chat
	GetRooms() ([]model.Room, error)
	GetRoomMessages(roomID string, limit int) ([]model.Message, error)
	SendRoomMessage(roomID, body string) error

	// Direct messages — NOTE: real impl uses Firebase RTDB with RTDBToken — pending feature/rtdb-chat
	GetConversations() ([]model.Conversation, error)
	GetMessages(conversationID string, limit int) ([]model.Message, error)
	SendMessage(conversationID, body string) error
}
