package api

import "github.com/ragnar/cyber-tui/internal/model"

// Client defines all interactions with the cyberspace.online API.
// The mock and real implementations both satisfy this interface.
type Client interface {
	// Auth
	Login(username, password string) (token string, err error)
	Logout() error

	// Feed
	GetFeed(page int) ([]model.Post, error)
	CreatePost(body string, topics []string) (model.Post, error)

	// Chatrooms
	GetRooms() ([]model.Room, error)
	GetRoomMessages(roomID string, limit int) ([]model.Message, error)
	SendRoomMessage(roomID, body string) error

	// Direct messages
	GetConversations() ([]model.Conversation, error)
	GetMessages(conversationID string, limit int) ([]model.Message, error)
	SendMessage(conversationID, body string) error

	// Profile
	GetProfile(username string) (model.User, error)
	UpdateProfile(bio string) error
}
