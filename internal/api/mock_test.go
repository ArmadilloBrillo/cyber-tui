package api_test

import (
	"strings"
	"testing"

	"github.com/ragnar/cyber-tui/internal/api"
)

func newMock() *api.MockClient {
	return api.NewMockClient()
}

// --- Login ---

func TestMockLogin_Success(t *testing.T) {
	m := newMock()
	token, err := m.Login("neuromancer", "secret")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}
}

func TestMockLogin_EmptyUsername(t *testing.T) {
	m := newMock()
	_, err := m.Login("", "secret")
	if err == nil {
		t.Fatal("expected error for empty username")
	}
}

func TestMockLogin_EmptyPassword(t *testing.T) {
	m := newMock()
	_, err := m.Login("neuromancer", "")
	if err == nil {
		t.Fatal("expected error for empty password")
	}
}

func TestMockLogin_BothEmpty(t *testing.T) {
	m := newMock()
	_, err := m.Login("", "")
	if err == nil {
		t.Fatal("expected error when both fields empty")
	}
}

// --- Logout ---

func TestMockLogout(t *testing.T) {
	m := newMock()
	_, _ = m.Login("neuromancer", "secret")
	if err := m.Logout(); err != nil {
		t.Fatalf("unexpected error on logout: %v", err)
	}
}

// --- Feed ---

func TestMockGetFeed_ReturnsPosts(t *testing.T) {
	m := newMock()
	posts, err := m.GetFeed(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(posts) == 0 {
		t.Fatal("expected at least one post")
	}
}

func TestMockGetFeed_PostsHaveAuthors(t *testing.T) {
	m := newMock()
	posts, _ := m.GetFeed(1)
	for _, p := range posts {
		if p.Author.Username == "" {
			t.Errorf("post %q has empty author username", p.ID)
		}
	}
}

func TestMockGetFeed_PostsHaveBody(t *testing.T) {
	m := newMock()
	posts, _ := m.GetFeed(1)
	for _, p := range posts {
		if strings.TrimSpace(p.Body) == "" {
			t.Errorf("post %q has empty body", p.ID)
		}
	}
}

// --- CreatePost ---

func TestMockCreatePost_ReturnsPost(t *testing.T) {
	m := newMock()
	post, err := m.CreatePost("hello matrix", []string{"test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if post.Body != "hello matrix" {
		t.Errorf("expected body %q, got %q", "hello matrix", post.Body)
	}
}

// --- Rooms ---

func TestMockGetRooms_ReturnsRooms(t *testing.T) {
	m := newMock()
	rooms, err := m.GetRooms()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rooms) == 0 {
		t.Fatal("expected at least one room")
	}
}

func TestMockGetRooms_RoomsHaveNames(t *testing.T) {
	m := newMock()
	rooms, _ := m.GetRooms()
	for _, r := range rooms {
		if r.Name == "" {
			t.Errorf("room %q has empty name", r.ID)
		}
	}
}

// --- Room messages ---

func TestMockGetRoomMessages_ReturnsMessages(t *testing.T) {
	m := newMock()
	msgs, err := m.GetRoomMessages("r1", 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) == 0 {
		t.Fatal("expected at least one message")
	}
}

func TestMockSendRoomMessage_NoError(t *testing.T) {
	m := newMock()
	if err := m.SendRoomMessage("r1", "hello room"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- Conversations ---

func TestMockGetConversations_ReturnsConvs(t *testing.T) {
	m := newMock()
	convs, err := m.GetConversations()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(convs) == 0 {
		t.Fatal("expected at least one conversation")
	}
}

func TestMockGetConversations_ConvsHaveParticipants(t *testing.T) {
	m := newMock()
	convs, _ := m.GetConversations()
	for _, c := range convs {
		if len(c.Participants) < 2 {
			t.Errorf("conversation %q has fewer than 2 participants", c.ID)
		}
	}
}

// --- DMs ---

func TestMockGetMessages_ReturnsMessages(t *testing.T) {
	m := newMock()
	msgs, err := m.GetMessages("c1", 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) == 0 {
		t.Fatal("expected at least one message")
	}
}

func TestMockSendMessage_NoError(t *testing.T) {
	m := newMock()
	if err := m.SendMessage("c1", "hey"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- Profile ---

func TestMockGetProfile_KnownUser(t *testing.T) {
	m := newMock()
	user, err := m.GetProfile("neuromancer")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.Username != "neuromancer" {
		t.Errorf("expected username %q, got %q", "neuromancer", user.Username)
	}
}

func TestMockGetProfile_UnknownUser(t *testing.T) {
	m := newMock()
	_, err := m.GetProfile("nobody")
	if err == nil {
		t.Fatal("expected error for unknown user")
	}
}

func TestMockGetProfile_EmptyUsername(t *testing.T) {
	m := newMock()
	_, err := m.GetProfile("")
	if err == nil {
		t.Fatal("expected error for empty username")
	}
}

func TestMockUpdateProfile_NoError(t *testing.T) {
	m := newMock()
	if err := m.UpdateProfile("new bio"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- Interface compliance ---

func TestMockClient_ImplementsClientInterface(t *testing.T) {
	var _ api.Client = api.NewMockClient()
}

func TestHTTPClient_ImplementsClientInterface(t *testing.T) {
	var _ api.Client = api.NewHTTPClient("http://example.com")
}
