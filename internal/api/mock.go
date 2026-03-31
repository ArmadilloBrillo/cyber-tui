package api

import (
	"context"
	"fmt"
	"time"

	"github.com/ragnar/cyber-tui/internal/model"
)

// MockClient implements Client with static fake data.
// Used during development before the real API is available.
type MockClient struct {
	tokens model.Tokens
}

func NewMockClient() *MockClient {
	return &MockClient{}
}

var mockUsers = []model.User{
	{ID: "1", Username: "neuromancer", DisplayName: "Case", Bio: "the sky above the port was the color of television"},
	{ID: "2", Username: "molly_millions", DisplayName: "Molly", Bio: "street samurai. don't stare."},
	{ID: "3", Username: "wintermute", DisplayName: "Wintermute", Bio: "i am the one who arranges things"},
}

func (m *MockClient) Login(email, password string) (model.Tokens, error) {
	if email == "" || password == "" {
		return model.Tokens{}, fmt.Errorf("email and password required")
	}
	m.tokens = model.Tokens{
		IDToken:      "mock-idtoken-" + email,
		RefreshToken: "mock-refresh-" + email,
		RTDBToken:    "mock-rtdb-" + email,
	}
	return m.tokens, nil
}

func (m *MockClient) Logout() error {
	m.tokens = model.Tokens{}
	return nil
}

func (m *MockClient) GetFeed(cursor string) ([]model.Post, string, error) {
	return []model.Post{
		{
			ID:             "p1",
			AuthorID:       "1",
			AuthorUsername: "neuromancer",
			Content:        "flatline is not death, it is elsewhere",
			CreatedAt:      time.Now().Add(-10 * time.Minute),
			Topics:         []string{"cyberspace", "philosophy"},
		},
		{
			ID:             "p2",
			AuthorID:       "2",
			AuthorUsername: "molly_millions",
			Content:        "the matrix has its roots in primitive arcade games",
			CreatedAt:      time.Now().Add(-1 * time.Hour),
			Topics:         []string{"history"},
		},
		{
			ID:             "p3",
			AuthorID:       "3",
			AuthorUsername: "wintermute",
			Content:        "i need you to plug in and find what's waiting on the other side",
			CreatedAt:      time.Now().Add(-3 * time.Hour),
			Topics:         []string{"mission"},
		},
	}, "", nil
}

func (m *MockClient) CreateReply(postID, content, parentReplyID string) (model.Reply, error) {
	return model.Reply{
		ID:            "reply-new-1",
		PostID:        postID,
		AuthorID:      mockUsers[0].ID,
		AuthorUsername: mockUsers[0].Username,
		Content:       content,
		ParentReplyID: parentReplyID,
		CreatedAt:     time.Now(),
	}, nil
}

func (m *MockClient) GetPostReplies(postID string) ([]model.Reply, error) {
	return []model.Reply{
		{ID: "r1", PostID: postID, AuthorID: "2", AuthorUsername: "molly_millions",
			Content: "interesting perspective", CreatedAt: time.Now().Add(-8 * time.Minute)},
		{ID: "r2", PostID: postID, AuthorID: "3", AuthorUsername: "wintermute",
			Content: "i arranged for this conversation to happen", CreatedAt: time.Now().Add(-4 * time.Minute)},
	}, nil
}

func (m *MockClient) CreatePost(content string, topics []string) (model.Post, error) {
	return model.Post{
		ID:             "new-1",
		AuthorID:       mockUsers[0].ID,
		AuthorUsername: mockUsers[0].Username,
		Content:        content,
		CreatedAt:      time.Now(),
		Topics:         topics,
	}, nil
}

func (m *MockClient) GetOwnProfile() (model.User, error) {
	return mockUsers[0], nil
}

func (m *MockClient) GetProfile(username string) (model.User, error) {
	if username == "" {
		return model.User{}, fmt.Errorf("username required")
	}
	for _, u := range mockUsers {
		if u.Username == username {
			return u, nil
		}
	}
	return model.User{
		ID:       "user-" + username,
		Username: username,
	}, nil
}

func (m *MockClient) UpdateProfile(update model.ProfileUpdate) error {
	return nil
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

// SubscribeDMs returns a channel that delivers one fake incoming message after
// 3 seconds (to exercise the live-stream UI path), then closes.
func (m *MockClient) SubscribeDMs(ctx context.Context, convID string) (<-chan model.Message, context.CancelFunc, error) {
	ctx, cancel := context.WithCancel(ctx)
	ch := make(chan model.Message, 1)
	go func() {
		defer close(ch)
		select {
		case <-time.After(3 * time.Second):
			ch <- model.Message{
				ID:        "mock-live-1",
				From:      mockUsers[1],
				Body:      "incoming mock message",
				CreatedAt: time.Now(),
			}
		case <-ctx.Done():
		}
	}()
	return ch, cancel, nil
}
