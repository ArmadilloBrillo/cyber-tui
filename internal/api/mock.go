package api

import (
	"fmt"
	"time"

	"github.com/ragnar/cyber-tui/internal/model"
)

// MockClient implements Client with static fake data.
// Used during development before the real API is available.
type MockClient struct {
	token string
}

func NewMockClient() *MockClient {
	return &MockClient{}
}

var mockUsers = []model.User{
	{ID: "1", Username: "neuromancer", Bio: "the sky above the port was the color of television"},
	{ID: "2", Username: "molly_millions", Bio: "street samurai. don't stare."},
	{ID: "3", Username: "wintermute", Bio: "i am the one who arranges things"},
}

func (m *MockClient) Login(username, password string) (string, error) {
	if username == "" || password == "" {
		return "", fmt.Errorf("username and password required")
	}
	m.token = "mock-token-" + username
	return m.token, nil
}

func (m *MockClient) Logout() error {
	m.token = ""
	return nil
}

func (m *MockClient) GetFeed(page int) ([]model.Post, error) {
	return []model.Post{
		{
			ID:        "p1",
			Author:    mockUsers[0],
			Body:      "flatline is not death, it is elsewhere",
			CreatedAt: time.Now().Add(-10 * time.Minute),
			Topics:    []string{"cyberspace", "philosophy"},
		},
		{
			ID:        "p2",
			Author:    mockUsers[1],
			Body:      "the matrix has its roots in primitive arcade games",
			CreatedAt: time.Now().Add(-1 * time.Hour),
			Topics:    []string{"history"},
		},
		{
			ID:        "p3",
			Author:    mockUsers[2],
			Body:      "i need you to plug in and find what's waiting on the other side",
			CreatedAt: time.Now().Add(-3 * time.Hour),
			Topics:    []string{"mission"},
		},
	}, nil
}

func (m *MockClient) CreatePost(body string, topics []string) (model.Post, error) {
	return model.Post{
		ID:        "new-1",
		Author:    mockUsers[0],
		Body:      body,
		CreatedAt: time.Now(),
		Topics:    topics,
	}, nil
}

func (m *MockClient) GetRooms() ([]model.Room, error) {
	return []model.Room{
		{ID: "r1", Name: "#zion", Description: "the last human city", Members: 42},
		{ID: "r2", Name: "#sprawl", Description: "boston-atlanta metropolitan axis", Members: 17},
		{ID: "r3", Name: "#freeside", Description: "orbital pleasure dome", Members: 8},
	}, nil
}

func (m *MockClient) GetRoomMessages(roomID string, limit int) ([]model.Message, error) {
	return []model.Message{
		{ID: "m1", From: mockUsers[0], Body: "anybody else getting lag in the matrix tonight?", CreatedAt: time.Now().Add(-5 * time.Minute)},
		{ID: "m2", From: mockUsers[1], Body: "always. use a slower deck.", CreatedAt: time.Now().Add(-3 * time.Minute)},
		{ID: "m3", From: mockUsers[2], Body: "...", CreatedAt: time.Now().Add(-1 * time.Minute)},
	}, nil
}

func (m *MockClient) SendRoomMessage(roomID, body string) error {
	return nil
}

func (m *MockClient) GetConversations() ([]model.Conversation, error) {
	return []model.Conversation{
		{
			ID:           "c1",
			Participants: []model.User{mockUsers[0], mockUsers[1]},
			Messages: []model.Message{
				{ID: "dm1", From: mockUsers[1], Body: "we need to talk about the job", CreatedAt: time.Now().Add(-1 * time.Hour)},
			},
		},
	}, nil
}

func (m *MockClient) GetMessages(conversationID string, limit int) ([]model.Message, error) {
	return []model.Message{
		{ID: "dm1", From: mockUsers[1], Body: "we need to talk about the job", CreatedAt: time.Now().Add(-1 * time.Hour)},
		{ID: "dm2", From: mockUsers[0], Body: "i'm listening", CreatedAt: time.Now().Add(-55 * time.Minute)},
	}, nil
}

func (m *MockClient) SendMessage(conversationID, body string) error {
	return nil
}

func (m *MockClient) GetProfile(username string) (model.User, error) {
	for _, u := range mockUsers {
		if u.Username == username {
			return u, nil
		}
	}
	return model.User{}, fmt.Errorf("user %q not found", username)
}

func (m *MockClient) UpdateProfile(bio string) error {
	return nil
}
