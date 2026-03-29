package api

import (
	"fmt"

	"github.com/ragnar/cyber-tui/internal/model"
)

// HTTPClient implements Client against the real cyberspace.online API.
// All methods are stubs until the RTDB and HTTP implementation feature branches are complete.
type HTTPClient struct {
	baseURL string
	tokens  model.Tokens
}

func NewHTTPClient(baseURL string) *HTTPClient {
	return &HTTPClient{baseURL: baseURL}
}

func (c *HTTPClient) Login(email, password string) (model.Tokens, error) {
	return model.Tokens{}, fmt.Errorf("real API not yet implemented")
}

func (c *HTTPClient) Logout() error {
	return fmt.Errorf("real API not yet implemented")
}

func (c *HTTPClient) GetFeed(cursor string) ([]model.Post, error) {
	return nil, fmt.Errorf("real API not yet implemented")
}

func (c *HTTPClient) CreatePost(content string, topics []string) (model.Post, error) {
	return model.Post{}, fmt.Errorf("real API not yet implemented")
}

func (c *HTTPClient) GetOwnProfile() (model.User, error) {
	return model.User{}, fmt.Errorf("real API not yet implemented")
}

func (c *HTTPClient) GetProfile(username string) (model.User, error) {
	return model.User{}, fmt.Errorf("real API not yet implemented")
}

func (c *HTTPClient) UpdateProfile(update model.ProfileUpdate) error {
	return fmt.Errorf("real API not yet implemented")
}

func (c *HTTPClient) GetRooms() ([]model.Room, error) {
	return nil, fmt.Errorf("real API not yet implemented")
}

func (c *HTTPClient) GetRoomMessages(roomID string, limit int) ([]model.Message, error) {
	return nil, fmt.Errorf("real API not yet implemented")
}

func (c *HTTPClient) SendRoomMessage(roomID, body string) error {
	return fmt.Errorf("real API not yet implemented")
}

func (c *HTTPClient) GetConversations() ([]model.Conversation, error) {
	return nil, fmt.Errorf("real API not yet implemented")
}

func (c *HTTPClient) GetMessages(conversationID string, limit int) ([]model.Message, error) {
	return nil, fmt.Errorf("real API not yet implemented")
}

func (c *HTTPClient) SendMessage(conversationID, body string) error {
	return fmt.Errorf("real API not yet implemented")
}
