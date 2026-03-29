package api_test

import (
	"strings"
	"testing"

	"github.com/ragnar/cyber-tui/internal/api"
	"github.com/ragnar/cyber-tui/internal/model"
)

func newMock() *api.MockClient {
	return api.NewMockClient()
}

// --- Login ---

func TestMockLogin_Success(t *testing.T) {
	m := newMock()
	tokens, err := m.Login("neo@matrix.net", "secret")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if tokens.IDToken == "" {
		t.Fatal("expected non-empty IDToken")
	}
}

func TestMockLogin_AllTokensNonEmpty(t *testing.T) {
	m := newMock()
	tokens, err := m.Login("neo@matrix.net", "secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tokens.IDToken == "" {
		t.Error("IDToken is empty")
	}
	if tokens.RefreshToken == "" {
		t.Error("RefreshToken is empty")
	}
	if tokens.RTDBToken == "" {
		t.Error("RTDBToken is empty")
	}
}

func TestMockLogin_EmptyEmail(t *testing.T) {
	m := newMock()
	_, err := m.Login("", "secret")
	if err == nil {
		t.Fatal("expected error for empty email")
	}
}

func TestMockLogin_EmptyPassword(t *testing.T) {
	m := newMock()
	_, err := m.Login("neo@matrix.net", "")
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
	_, _ = m.Login("neo@matrix.net", "secret")
	if err := m.Logout(); err != nil {
		t.Fatalf("unexpected error on logout: %v", err)
	}
}

// --- Feed ---

func TestMockGetFeed_ReturnsPosts(t *testing.T) {
	m := newMock()
	posts, _, err := m.GetFeed("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(posts) == 0 {
		t.Fatal("expected at least one post")
	}
}

func TestMockGetFeed_PostsHaveID(t *testing.T) {
	m := newMock()
	posts, _, _ := m.GetFeed("")
	for _, p := range posts {
		if p.ID == "" {
			t.Errorf("post has empty ID")
		}
	}
}

func TestMockGetFeed_PostsHaveAuthors(t *testing.T) {
	m := newMock()
	posts, _, _ := m.GetFeed("")
	for _, p := range posts {
		if p.AuthorUsername == "" {
			t.Errorf("post %q has empty AuthorUsername", p.ID)
		}
	}
}

func TestMockGetFeed_PostsHaveContent(t *testing.T) {
	m := newMock()
	posts, _, _ := m.GetFeed("")
	for _, p := range posts {
		if strings.TrimSpace(p.Content) == "" {
			t.Errorf("post %q has empty content", p.ID)
		}
	}
}

func TestMockGetFeed_CursorIgnored(t *testing.T) {
	m := newMock()
	posts1, _, _ := m.GetFeed("")
	posts2, _, _ := m.GetFeed("some-cursor")
	if len(posts1) != len(posts2) {
		t.Error("mock should return same posts regardless of cursor")
	}
}

func TestMockGetFeed_ReturnsEmptyCursor(t *testing.T) {
	m := newMock()
	_, cursor, err := m.GetFeed("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cursor != "" {
		t.Errorf("mock cursor = %q, want empty string", cursor)
	}
}

// --- CreatePost ---

func TestMockCreatePost_ReturnsPost(t *testing.T) {
	m := newMock()
	post, err := m.CreatePost("hello matrix", []string{"test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if post.Content != "hello matrix" {
		t.Errorf("expected content %q, got %q", "hello matrix", post.Content)
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

func TestMockGetOwnProfile_ReturnsUser(t *testing.T) {
	m := newMock()
	user, err := m.GetOwnProfile()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.Username == "" {
		t.Error("expected non-empty username from GetOwnProfile")
	}
}

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
	user, err := m.GetProfile("nobody")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.Username != "nobody" {
		t.Errorf("expected generated username %q, got %q", "nobody", user.Username)
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
	bio := "new bio"
	if err := m.UpdateProfile(model.ProfileUpdate{Bio: &bio}); err != nil {
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
