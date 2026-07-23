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
	tokens    model.Tokens
	bookmarks []model.Bookmark
	notes     []model.Note
	settings  model.Settings
}

func NewMockClient() *MockClient {
	return &MockClient{
		bookmarks: []model.Bookmark{
			{
				ID:        "bm1",
				Type:      "post",
				PostID:    "p1",
				Post:      &model.Post{ID: "p1", AuthorUsername: "neuromancer", Content: "flatline is not death, it is elsewhere"},
				CreatedAt: time.Now().Add(-30 * time.Minute),
			},
			{
				ID:        "bm2",
				Type:      "post",
				PostID:    "p2",
				Post:      &model.Post{ID: "p2", AuthorUsername: "molly_millions", Content: "the matrix has its roots in primitive arcade games"},
				CreatedAt: time.Now().Add(-2 * time.Hour),
			},
		},
		notes: []model.Note{
			{
				ID:             "note1",
				AuthorID:       "1",
				Content:        "flatline is not death, it is elsewhere\n\nremember to follow up on this thought.",
				Topics:         []string{"philosophy"},
				RevisionNumber: 1,
				Deleted:        false,
				CreatedAt:      time.Now().Add(-2 * time.Hour),
			},
			{
				ID:             "note2",
				AuthorID:       "1",
				Content:        "the matrix has its roots in primitive arcade games\n\nbut what are its roots in the human condition?",
				Topics:         []string{"history", "cyberspace"},
				RevisionNumber: 2,
				Deleted:        false,
				CreatedAt:      time.Now().Add(-24 * time.Hour),
			},
			{
				ID:             "note3",
				AuthorID:       "1",
				Content:        "shopping list:\n- coffee\n- ice\n- a new deck",
				Topics:         []string{},
				RevisionNumber: 1,
				Deleted:        false,
				CreatedAt:      time.Now().Add(-72 * time.Hour),
			},
		},
		settings: model.Settings{
			Notifications:     model.NotificationPrefs{Bookmark: true, Reply: true, Poke: true},
			ShowFollowerCount: true,
			TimeDisplayFormat: "datetime",
			ImagePixelSize:    "2",
		},
	}
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

func (m *MockClient) LoginWithRefreshToken(refreshToken string) (model.Tokens, error) {
	m.tokens = model.Tokens{
		IDToken:      "mock-idtoken-refreshed",
		RefreshToken: refreshToken,
		RTDBToken:    "mock-rtdb-refreshed",
	}
	return m.tokens, nil
}

func (m *MockClient) Logout() error {
	m.tokens = model.Tokens{}
	return nil
}

func (m *MockClient) RefreshSession() error {
	return nil
}

var mockNotifications = []model.Notification{
	{
		ID: "n1", Type: "reply", Read: false,
		CreatedAt:  time.Now().Add(-5 * time.Minute),
		Actor:      model.NotificationActor{ID: "2", Username: "molly_millions"},
		TargetID:   "p1",
		TargetType: "reply",
		ReplyID:    "r1",
	},
	{
		ID: "n6", Type: "thread_reply", Read: false,
		CreatedAt:            time.Now().Add(-10 * time.Minute),
		Actor:                model.NotificationActor{ID: "3", Username: "wintermute"},
		TargetID:             "p1",
		TargetType:           "reply",
		ReplyID:              "r2",
		ThreadAuthorUsername: "neuromancer",
	},
	{
		ID: "n2", Type: "new_post_friend", Read: false,
		CreatedAt:  time.Now().Add(-20 * time.Minute),
		Actor:      model.NotificationActor{ID: "3", Username: "wintermute"},
		TargetID:   "p2",
		TargetType: "post",
	},
	{
		ID: "n3", Type: "new_follower", Read: true,
		CreatedAt:  time.Now().Add(-2 * time.Hour),
		Actor:      model.NotificationActor{ID: "1", Username: "neuromancer"},
		TargetID:   "",
		TargetType: "",
	},
	{
		ID: "n4", Type: "poke", Read: false,
		CreatedAt:  time.Now().Add(-30 * time.Minute),
		Actor:      model.NotificationActor{ID: "2", Username: "molly_millions"},
		TargetID:   "",
		TargetType: "",
	},
	{
		ID: "n5", Type: "bookmark", Read: true,
		CreatedAt:  time.Now().AddDate(0, 0, -1).Add(-3 * time.Hour),
		Actor:      model.NotificationActor{ID: "3", Username: "wintermute"},
		TargetID:   "p1",
		TargetType: "post",
	},
}

func (m *MockClient) GetFeed(cursor string) ([]model.Post, string, error) {
	return []model.Post{
		{
			ID:             "p1",
			AuthorID:       "1",
			AuthorUsername: "neuromancer",
			Title:          "On Flatline",
			Content:        "flatline is not death, it is elsewhere",
			Slug:           "on-flatline",
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
			IsNSFW:         true,
		},
		{
			ID:             "p3",
			AuthorID:       "3",
			AuthorUsername: "wintermute",
			Content:        "i need you to plug in and find what's waiting on the other side",
			CreatedAt:      time.Now().Add(-3 * time.Hour),
			Topics:         []string{"mission"},
			IsPublic:       true,
		},
	}, "", nil
}

func (m *MockClient) CreateReply(postID, content, parentReplyID string) (model.Reply, error) {
	return model.Reply{
		ID:             "reply-new-1",
		PostID:         postID,
		AuthorID:       mockUsers[0].ID,
		AuthorUsername: mockUsers[0].Username,
		Content:        content,
		ParentReplyID:  parentReplyID,
		CreatedAt:      time.Now(),
	}, nil
}

func (m *MockClient) GetReply(replyID string) (model.Reply, error) {
	replies, _ := m.GetPostReplies("")
	for _, r := range replies {
		if r.ID == replyID {
			return r, nil
		}
	}
	return model.Reply{}, fmt.Errorf("reply not found: %s", replyID)
}

func (m *MockClient) GetPostReplies(postID string) ([]model.Reply, error) {
	return []model.Reply{
		{ID: "r1", PostID: postID, AuthorID: "2", AuthorUsername: "molly_millions",
			Content: "interesting perspective", CreatedAt: time.Now().Add(-8 * time.Minute)},
		{ID: "r2", PostID: postID, AuthorID: "3", AuthorUsername: "wintermute",
			Content: "i arranged for this conversation to happen", CreatedAt: time.Now().Add(-4 * time.Minute)},
	}, nil
}

func (m *MockClient) CreatePost(content, title, slug string, topics []string, isPublic, isNSFW bool) (model.Post, error) {
	return model.Post{
		ID:             "new-1",
		AuthorID:       mockUsers[0].ID,
		AuthorUsername: mockUsers[0].Username,
		Content:        content,
		Title:          title,
		CreatedAt:      time.Now(),
		Topics:         topics,
		IsPublic:       isPublic,
		IsNSFW:         isNSFW,
	}, nil
}

func (m *MockClient) DeletePost(postID string) error {
	return nil // no-op: in-memory feed is rebuilt on each GetFeed call
}

func (m *MockClient) DeleteReply(replyID string) error {
	return nil // no-op: in-memory replies are rebuilt on each GetPostReplies call
}

func (m *MockClient) GetPost(postID string) (model.Post, error) {
	posts, _, _ := m.GetFeed("")
	for _, p := range posts {
		if p.ID == postID {
			return p, nil
		}
	}
	return model.Post{ID: postID, AuthorUsername: "unknown", Content: "[post not found]"}, nil
}

func (m *MockClient) GetBookmarks(cursor string) ([]model.Bookmark, string, error) {
	return m.bookmarks, "", nil
}

func (m *MockClient) CreateBookmark(postID, replyID string) (string, error) {
	id := fmt.Sprintf("bm-new-%d", len(m.bookmarks)+1)
	b := model.Bookmark{
		ID:      id,
		Type:    "post",
		PostID:  postID,
		ReplyID: replyID,
	}
	if replyID != "" {
		b.Type = "reply"
	}
	if postID != "" {
		if p, err := m.GetPost(postID); err == nil {
			b.Post = &p
		}
	}
	m.bookmarks = append(m.bookmarks, b)
	return id, nil
}

func (m *MockClient) DeleteBookmark(id string) error {
	for i, b := range m.bookmarks {
		if b.ID == id {
			m.bookmarks = append(m.bookmarks[:i], m.bookmarks[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("bookmark not found: %s", id)
}

func (m *MockClient) GetTopics(cursor string) ([]model.Topic, string, error) {
	return mockTopics, "", nil
}

func (m *MockClient) GetTopicPosts(slug string, cursor string) ([]model.Post, string, error) {
	// Return a filtered subset of posts for the topic
	feed, _, _ := m.GetFeed("")
	var topicPosts []model.Post
	for _, p := range feed {
		for _, t := range p.Topics {
			if t == slug {
				topicPosts = append(topicPosts, p)
				break
			}
		}
	}
	return topicPosts, "", nil
}

func (m *MockClient) GetGuilds(cursor string) ([]model.Guild, string, error) {
	return nil, "", nil
}

func (m *MockClient) GetGuild(slug string) (model.Guild, error) {
	return model.Guild{}, nil
}

func (m *MockClient) GetGuildPosts(slug string, cursor string) ([]model.Post, string, error) {
	return nil, "", nil
}

func (m *MockClient) CreateGuildPost(slug, content, title, postSlug string, topics []string) (model.Post, error) {
	return model.Post{}, nil
}

func (m *MockClient) GetGuildMembers(slug, cursor string) ([]model.GuildMember, string, error) {
	return nil, "", nil
}

func (m *MockClient) JoinGuild(slug string) error  { return nil }
func (m *MockClient) LeaveGuild(slug string) error { return nil }

func (m *MockClient) GetWatches(cursor string) ([]model.Watch, string, error) {
	return nil, "", nil
}
func (m *MockClient) WatchPost(postID string) error   { return nil }
func (m *MockClient) UnwatchPost(postID string) error { return nil }

func (m *MockClient) GetNotifications(cursor string, unreadOnly bool, types []string) ([]model.Notification, string, error) {
	if !unreadOnly {
		return mockNotifications, "", nil
	}
	var out []model.Notification
	for _, n := range mockNotifications {
		if !n.Read {
			out = append(out, n)
		}
	}
	return out, "", nil
}

func (m *MockClient) GetUnreadNotificationCount() (int, error) {
	count := 0
	for _, n := range mockNotifications {
		if !n.Read {
			count++
		}
	}
	return count, nil
}

func (m *MockClient) MarkNotificationRead(id string) error { return nil }

func (m *MockClient) MarkAllNotificationsRead() error { return nil }

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

var mockTopics = []model.Topic{
	{Slug: "go", PostCount: 195},
	{Slug: "programming", PostCount: 195},
	{Slug: "cyberspace", PostCount: 127},
	{Slug: "music", PostCount: 87},
}

func (m *MockClient) GetSettings() (model.Settings, error) {
	return m.settings, nil
}

func (m *MockClient) UpdateSettings(update model.Settings) error {
	m.settings = update
	return nil
}

func (m *MockClient) GetRooms() ([]model.Room, error) {
	return []model.Room{
		{ID: "r1", Slug: "zion", Name: "Zion", LastMessageAt: time.Now().Add(-2 * time.Minute), SortOrder: 1},
		{ID: "r2", Slug: "sprawl", Name: "Sprawl", LastMessageAt: time.Now().Add(-15 * time.Minute), SortOrder: 2},
		{ID: "r3", Slug: "freeside", Name: "Freeside", LastMessageAt: time.Now().Add(-1 * time.Hour), SortOrder: 3},
	}, nil
}

func (m *MockClient) GetRoomMessages(roomID string, limit int, before int64) ([]model.Message, error) {
	if before > 0 {
		return nil, nil
	}
	return []model.Message{
		{ID: "m1", From: mockUsers[0], Body: "anybody else getting lag in the matrix tonight?", CreatedAt: time.Now().Add(-5 * time.Minute)},
		{ID: "m2", From: mockUsers[1], Body: "always. use a slower deck.", CreatedAt: time.Now().Add(-3 * time.Minute), IsChatAdmin: true},
		{ID: "m3", From: mockUsers[2], Body: "...", CreatedAt: time.Now().Add(-1 * time.Minute)},
	}, nil
}

func (m *MockClient) SendRoomMessage(roomID, body string) error {
	return nil
}

func (m *MockClient) MarkRoomRead(roomID string) error {
	return nil
}

// SubscribeRoom returns a channel that delivers one fake incoming message after
// 2 seconds (to exercise the live-stream UI path), then closes.
func (m *MockClient) SubscribeRoom(ctx context.Context, roomID string) (<-chan model.Message, context.CancelFunc, error) {
	ctx, cancel := context.WithCancel(ctx)
	ch := make(chan model.Message, 1)
	go func() {
		defer close(ch)
		select {
		case <-time.After(2 * time.Second):
			select {
			case ch <- model.Message{
				ID:        "mock-room-live-1",
				From:      mockUsers[1],
				Body:      "incoming mock room message",
				CreatedAt: time.Now(),
			}:
			case <-ctx.Done():
			}
		case <-ctx.Done():
		}
	}()
	return ch, cancel, nil
}

func (m *MockClient) GetConversations() ([]model.Conversation, error) {
	return []model.Conversation{
		{
			ID:            "c1",
			Participants:  []model.User{mockUsers[0], mockUsers[1]},
			LastMessage:   "we need to talk about the job",
			LastMessageAt: time.Now().Add(-1 * time.Hour),
			Messages: []model.Message{
				{ID: "dm1", From: mockUsers[1], Body: "we need to talk about the job", CreatedAt: time.Now().Add(-1 * time.Hour)},
			},
		},
	}, nil
}

func (m *MockClient) GetMessages(conversationID string, limit int, before int64) ([]model.Message, error) {
	if before > 0 {
		return nil, nil
	}
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

func (m *MockClient) StartConversation(recipientUsername string) (model.Conversation, error) {
	return model.Conversation{
		ID:           "c-new",
		Participants: []model.User{{Username: recipientUsername}},
	}, nil
}

func (m *MockClient) MarkCMailRead(conversationID string) error {
	return nil
}

var mockFollowing = []model.Follow{
	{ID: "fw1", FollowerID: "1", FollowedID: "2", FollowerUsername: "neuromancer", FollowedUsername: "molly_millions", CreatedAt: time.Now().Add(-48 * time.Hour)},
	{ID: "fw2", FollowerID: "1", FollowedID: "3", FollowerUsername: "neuromancer", FollowedUsername: "wintermute", CreatedAt: time.Now().Add(-72 * time.Hour)},
}

var mockFollowers = []model.Follow{
	{ID: "fw3", FollowerID: "2", FollowedID: "1", FollowerUsername: "molly_millions", FollowedUsername: "neuromancer", CreatedAt: time.Now().Add(-36 * time.Hour)},
	{ID: "fw4", FollowerID: "3", FollowedID: "1", FollowerUsername: "wintermute", FollowedUsername: "neuromancer", CreatedAt: time.Now().Add(-60 * time.Hour)},
}

func (m *MockClient) GetFollowing(cursor string) ([]model.Follow, string, error) {
	return mockFollowing, "", nil
}

func (m *MockClient) GetFollowers(cursor string) ([]model.Follow, string, error) {
	return mockFollowers, "", nil
}

func (m *MockClient) GetUserFollows(userID, followType, cursor string) ([]model.Follow, string, error) {
	if followType == "followers" {
		return mockFollowers, "", nil
	}
	return mockFollowing, "", nil
}

func (m *MockClient) Follow(followedID string) (string, error) {
	return "mock-follow-id", nil
}

func (m *MockClient) Unfollow(followID string) error {
	return nil
}

func (m *MockClient) GetNotes(cursor string) ([]model.Note, string, error) {
	return m.notes, "", nil
}

func (m *MockClient) CreateNote(content string, topics []string) (model.Note, error) {
	note := model.Note{
		ID:             fmt.Sprintf("note-new-%d", len(m.notes)+1),
		AuthorID:       mockUsers[0].ID,
		Content:        content,
		Topics:         topics,
		RevisionNumber: 1,
		Deleted:        false,
		CreatedAt:      time.Now(),
	}
	m.notes = append([]model.Note{note}, m.notes...)
	return note, nil
}

func (m *MockClient) UpdateNote(noteID, content string, topics []string) error {
	for i, n := range m.notes {
		if n.ID == noteID {
			m.notes[i].Content = content
			m.notes[i].Topics = topics
			m.notes[i].RevisionNumber++
			return nil
		}
	}
	return fmt.Errorf("note not found: %s", noteID)
}

func (m *MockClient) DeleteNote(noteID string) error {
	for i, n := range m.notes {
		if n.ID == noteID {
			m.notes = append(m.notes[:i], m.notes[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("note not found: %s", noteID)
}

func (m *MockClient) GetNote(noteID string) (model.Note, error) {
	for _, n := range m.notes {
		if n.ID == noteID {
			return n, nil
		}
	}
	return model.Note{}, fmt.Errorf("note not found: %s", noteID)
}

func (m *MockClient) GetNoteRevision(noteID string, revision int) (model.Note, error) {
	for _, n := range m.notes {
		if n.ID == noteID {
			// Return a copy with the requested revision number and modified content.
			copy := n
			copy.RevisionNumber = revision
			if revision < n.RevisionNumber {
				copy.Content = fmt.Sprintf("[revision %d of note %s]\n\n%s", revision, noteID, n.Content)
			}
			return copy, nil
		}
	}
	return model.Note{}, fmt.Errorf("note not found: %s", noteID)
}

var mockNoteRevisions = map[string][]model.NoteRevision{
	"note2": {
		{RevisionNumber: 2, Content: "the matrix has its roots in primitive arcade games\n\nbut what are its roots in the human condition?", Topics: []string{"history", "cyberspace"}, CreatedAt: time.Now().Add(-24 * time.Hour)},
		{RevisionNumber: 1, Content: "the matrix has its roots in primitive arcade games", Topics: []string{"history"}, CreatedAt: time.Now().Add(-48 * time.Hour)},
	},
}

func (m *MockClient) GetNoteRevisions(noteID, cursor string) ([]model.NoteRevision, string, error) {
	if revs, ok := mockNoteRevisions[noteID]; ok {
		return revs, "", nil
	}
	// For notes without explicit revision history, synthesize one revision.
	for _, n := range m.notes {
		if n.ID == noteID {
			return []model.NoteRevision{
				{RevisionNumber: n.RevisionNumber, Content: n.Content, Topics: n.Topics, CreatedAt: n.CreatedAt},
			}, "", nil
		}
	}
	return nil, "", fmt.Errorf("note not found: %s", noteID)
}

func (m *MockClient) GetUserPosts(username, cursor string) ([]model.Post, string, error) {
	feed, _, _ := m.GetFeed("")
	var posts []model.Post
	for _, p := range feed {
		if p.AuthorUsername == username {
			posts = append(posts, p)
		}
	}
	return posts, "", nil
}

func (m *MockClient) GetUserReplies(username, cursor string) ([]model.Reply, string, error) {
	replies, _ := m.GetPostReplies("")
	var userReplies []model.Reply
	for _, r := range replies {
		if r.AuthorUsername == username {
			userReplies = append(userReplies, r)
		}
	}
	return userReplies, "", nil
}
