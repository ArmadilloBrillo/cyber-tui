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
	post, err := m.CreatePost("hello matrix", "", "", []string{"test"}, false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if post.Content != "hello matrix" {
		t.Errorf("expected content %q, got %q", "hello matrix", post.Content)
	}
}

func TestMockCreatePost_TitleAndFlags(t *testing.T) {
	m := newMock()
	post, err := m.CreatePost("body text", "My Title", "", []string{"test"}, true, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if post.Title != "My Title" {
		t.Errorf("expected title %q, got %q", "My Title", post.Title)
	}
	if !post.IsPublic {
		t.Error("expected IsPublic=true")
	}
	if !post.IsNSFW {
		t.Error("expected IsNSFW=true")
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

// --- Replies ---

func TestMockGetPostReplies_ReturnsReplies(t *testing.T) {
	m := newMock()
	replies, err := m.GetPostReplies("p1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(replies) == 0 {
		t.Fatal("expected at least one reply")
	}
}

func TestMockGetPostReplies_AnyPostID(t *testing.T) {
	m := newMock()
	r1, _ := m.GetPostReplies("post-a")
	r2, _ := m.GetPostReplies("post-b")
	if len(r1) != len(r2) {
		t.Errorf("expected same number of replies for any postID, got %d vs %d", len(r1), len(r2))
	}
}

// --- CreateReply ---

func TestMockCreateReply_ReturnsReply(t *testing.T) {
	m := newMock()
	r, err := m.CreateReply("p1", "great post", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.ID == "" {
		t.Error("expected non-empty reply ID")
	}
	if r.PostID != "p1" {
		t.Errorf("expected PostID=%q, got %q", "p1", r.PostID)
	}
	if r.Content != "great post" {
		t.Errorf("expected Content=%q, got %q", "great post", r.Content)
	}
}

func TestMockCreateReply_WithParent(t *testing.T) {
	m := newMock()
	r, err := m.CreateReply("p1", "nested reply", "r1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.ParentReplyID != "r1" {
		t.Errorf("expected ParentReplyID=%q, got %q", "r1", r.ParentReplyID)
	}
}

// --- Settings ---

func TestMockGetSettings_ReturnsNonZero(t *testing.T) {
	m := newMock()
	s, err := m.GetSettings()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.TimeDisplayFormat == "" {
		t.Error("expected non-empty TimeDisplayFormat")
	}
	if s.ImagePixelSize == "" {
		t.Error("expected non-empty ImagePixelSize")
	}
}

func TestMockGetSettings_NotificationsEnabled(t *testing.T) {
	m := newMock()
	s, _ := m.GetSettings()
	if !s.Notifications.Bookmark {
		t.Error("expected Notifications.Bookmark to be true in mock")
	}
	if !s.Notifications.Reply {
		t.Error("expected Notifications.Reply to be true in mock")
	}
}

func TestMockUpdateSettings_NoError(t *testing.T) {
	m := newMock()
	if err := m.UpdateSettings(model.Settings{TimeDisplayFormat: "12h"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- Bookmarks ---

func TestMockGetBookmarks_ReturnsList(t *testing.T) {
	m := newMock()
	bookmarks, cursor, err := m.GetBookmarks("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bookmarks) == 0 {
		t.Fatal("expected non-empty bookmarks list")
	}
	_ = cursor // cursor may be empty for mock
}

func TestMockCreateBookmark_Post(t *testing.T) {
	m := newMock()
	before, _, _ := m.GetBookmarks("")
	id, err := m.CreateBookmark("p-new", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty bookmarkId")
	}
	after, _, _ := m.GetBookmarks("")
	if len(after) != len(before)+1 {
		t.Errorf("expected %d bookmarks after create, got %d", len(before)+1, len(after))
	}
}

func TestMockDeleteBookmark_RemovesEntry(t *testing.T) {
	m := newMock()
	before, _, _ := m.GetBookmarks("")
	if len(before) == 0 {
		t.Skip("no bookmarks to delete")
	}
	id := before[0].ID
	if err := m.DeleteBookmark(id); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	after, _, _ := m.GetBookmarks("")
	if len(after) != len(before)-1 {
		t.Errorf("expected %d bookmarks after delete, got %d", len(before)-1, len(after))
	}
}

func TestMockDeleteBookmark_NotFound(t *testing.T) {
	m := newMock()
	if err := m.DeleteBookmark("no-such-id"); err == nil {
		t.Fatal("expected error for missing bookmark, got nil")
	}
}

// --- Notes ---

func TestMockGetNotes_ReturnsSeedData(t *testing.T) {
	m := newMock()
	notes, cursor, err := m.GetNotes("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(notes) == 0 {
		t.Fatal("expected seed notes, got none")
	}
	// cursor is empty (single page of mock data)
	if cursor != "" {
		t.Errorf("cursor = %q, want empty string for mock", cursor)
	}
	// All seed notes must have non-empty IDs and content.
	for i, n := range notes {
		if n.ID == "" {
			t.Errorf("notes[%d].ID is empty", i)
		}
		if n.Content == "" {
			t.Errorf("notes[%d].Content is empty", i)
		}
	}
}

func TestMockCreateNote_PrependedToList(t *testing.T) {
	m := newMock()
	before, _, _ := m.GetNotes("")

	note, err := m.CreateNote("brand new note", []string{"test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if note.ID == "" {
		t.Fatal("expected non-empty note ID")
	}
	if note.Content != "brand new note" {
		t.Errorf("Content = %q, want 'brand new note'", note.Content)
	}

	after, _, _ := m.GetNotes("")
	if len(after) != len(before)+1 {
		t.Errorf("expected %d notes after create, got %d", len(before)+1, len(after))
	}
	// New note should be first.
	if after[0].ID != note.ID {
		t.Errorf("first note ID = %q, want new note ID %q", after[0].ID, note.ID)
	}
}

func TestMockUpdateNote_UpdatesContent(t *testing.T) {
	m := newMock()
	notes, _, _ := m.GetNotes("")
	if len(notes) == 0 {
		t.Skip("no seed notes available")
	}
	target := notes[0]

	err := m.UpdateNote(target.ID, "revised content", []string{"updated"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	after, _, _ := m.GetNotes("")
	if after[0].Content != "revised content" {
		t.Errorf("Content = %q, want 'revised content'", after[0].Content)
	}
	if after[0].RevisionNumber != target.RevisionNumber+1 {
		t.Errorf("RevisionNumber = %d, want %d", after[0].RevisionNumber, target.RevisionNumber+1)
	}
}

func TestMockUpdateNote_NotFound(t *testing.T) {
	m := newMock()
	if err := m.UpdateNote("no-such-id", "content", nil); err == nil {
		t.Fatal("expected error for missing note, got nil")
	}
}

func TestMockDeleteNote_RemovesFromList(t *testing.T) {
	m := newMock()
	before, _, _ := m.GetNotes("")
	if len(before) == 0 {
		t.Skip("no seed notes available")
	}

	if err := m.DeleteNote(before[0].ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	after, _, _ := m.GetNotes("")
	if len(after) != len(before)-1 {
		t.Errorf("expected %d notes after delete, got %d", len(before)-1, len(after))
	}
}

func TestMockDeleteNote_NotFound(t *testing.T) {
	m := newMock()
	if err := m.DeleteNote("no-such-id"); err == nil {
		t.Fatal("expected error for missing note, got nil")
	}
}

// --- DeletePost ---

func TestMockDeletePost_ReturnsNil(t *testing.T) {
	m := newMock()
	if err := m.DeletePost("p1"); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestMockDeletePost_UnknownIDReturnsNil(t *testing.T) {
	m := newMock()
	// Mock is a no-op; any ID is accepted without error.
	if err := m.DeletePost("no-such-post"); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

// --- DeleteReply ---

func TestMockDeleteReply_ReturnsNil(t *testing.T) {
	m := newMock()
	if err := m.DeleteReply("r1"); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestMockDeleteReply_UnknownIDReturnsNil(t *testing.T) {
	m := newMock()
	if err := m.DeleteReply("no-such-reply"); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

// --- GetFollowers ---

func TestMockGetFollowers_ReturnsNonEmpty(t *testing.T) {
	m := newMock()
	follows, cursor, err := m.GetFollowers("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(follows) == 0 {
		t.Fatal("expected at least one follower")
	}
	_ = cursor
	for _, f := range follows {
		if f.FollowerUsername == "" {
			t.Error("FollowerUsername should be set")
		}
	}
}

// --- GetUserFollows ---

func TestMockGetUserFollows_Following(t *testing.T) {
	m := newMock()
	follows, _, err := m.GetUserFollows("user-1", "following", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = follows
}

func TestMockGetUserFollows_Followers(t *testing.T) {
	m := newMock()
	follows, _, err := m.GetUserFollows("user-1", "followers", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = follows
}

// --- GetUserPosts ---

func TestMockGetUserPosts_ReturnsPostsForKnownUser(t *testing.T) {
	m := newMock()
	posts, cursor, err := m.GetUserPosts("neuromancer", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = cursor
	for _, p := range posts {
		if p.AuthorUsername != "neuromancer" {
			t.Errorf("got post by %q, expected only neuromancer", p.AuthorUsername)
		}
	}
}

func TestMockGetUserPosts_EmptyForUnknownUser(t *testing.T) {
	m := newMock()
	posts, _, err := m.GetUserPosts("nobody", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(posts) != 0 {
		t.Errorf("expected 0 posts for unknown user, got %d", len(posts))
	}
}

// --- GetUserReplies ---

func TestMockGetUserReplies_ReturnsMatchingReplies(t *testing.T) {
	m := newMock()
	replies, _, err := m.GetUserReplies("molly_millions", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, r := range replies {
		if r.AuthorUsername != "molly_millions" {
			t.Errorf("got reply by %q, expected only molly_millions", r.AuthorUsername)
		}
	}
}

// --- GetNote ---

func TestMockGetNote_KnownID(t *testing.T) {
	m := newMock()
	notes, _, _ := m.GetNotes("")
	if len(notes) == 0 {
		t.Skip("no seed notes available")
	}
	note, err := m.GetNote(notes[0].ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if note.ID != notes[0].ID {
		t.Errorf("ID = %q, want %q", note.ID, notes[0].ID)
	}
}

func TestMockGetNote_NotFound(t *testing.T) {
	m := newMock()
	_, err := m.GetNote("no-such-note")
	if err == nil {
		t.Fatal("expected error for missing note, got nil")
	}
}

// --- GetNoteRevision ---

func TestMockGetNoteRevision_KnownID(t *testing.T) {
	m := newMock()
	notes, _, _ := m.GetNotes("")
	if len(notes) == 0 {
		t.Skip("no seed notes available")
	}
	note, err := m.GetNoteRevision(notes[0].ID, notes[0].RevisionNumber)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if note.ID != notes[0].ID {
		t.Errorf("ID = %q, want %q", note.ID, notes[0].ID)
	}
}

func TestMockGetNoteRevision_NotFound(t *testing.T) {
	m := newMock()
	_, err := m.GetNoteRevision("no-such-note", 1)
	if err == nil {
		t.Fatal("expected error for missing note, got nil")
	}
}

// --- GetNoteRevisions ---

func TestMockGetNoteRevisions_KnownNote(t *testing.T) {
	m := newMock()
	// "note2" has explicit revision history
	revs, cursor, err := m.GetNoteRevisions("note2", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(revs) == 0 {
		t.Fatal("expected at least one revision")
	}
	_ = cursor
	for _, r := range revs {
		if r.RevisionNumber == 0 {
			t.Error("RevisionNumber should be > 0")
		}
	}
}

func TestMockGetNoteRevisions_FallbackForOtherNotes(t *testing.T) {
	m := newMock()
	// "note1" has no explicit revision history — should synthesize one
	revs, _, err := m.GetNoteRevisions("note1", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(revs) == 0 {
		t.Fatal("expected synthesized revision for note1")
	}
}

func TestMockGetNoteRevisions_NotFound(t *testing.T) {
	m := newMock()
	_, _, err := m.GetNoteRevisions("no-such-note", "")
	if err == nil {
		t.Fatal("expected error for missing note, got nil")
	}
}

// --- Interface compliance ---

func TestMockClient_ImplementsClientInterface(t *testing.T) {
	var _ api.Client = api.NewMockClient()
}

func TestHTTPClient_ImplementsClientInterface(t *testing.T) {
	var _ api.Client = api.NewHTTPClient("http://example.com")
}
