package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/rtdb"
)

// --- typed errors ---

// APIError is returned for any error response from the cyberspace.online API.
type APIError struct {
	Code    string // matches server error code, e.g. "UNAUTHORIZED"
	Message string
	Status  int // HTTP status code
}

func (e *APIError) Error() string {
	return fmt.Sprintf("API error %s (%d): %s", e.Code, e.Status, e.Message)
}

// ErrUnauthorized is returned when login fails or token refresh fails.
// The app should redirect the user to the login screen.
var ErrUnauthorized = &APIError{Code: "UNAUTHORIZED", Status: 401, Message: "session expired or invalid credentials"}

// ErrRateLimited is returned when the server responds with 429.
var ErrRateLimited = &APIError{Code: "RATE_LIMITED", Status: 429, Message: "rate limit exceeded"}

// --- wire types (unexported JSON shapes matching the API) ---

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponseData struct {
	IDToken      string `json:"idToken"`
	RefreshToken string `json:"refreshToken"`
	RTDBToken    string `json:"rtdbToken"`
}

type refreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

type refreshResponseData struct {
	IDToken   string `json:"idToken"`
	RTDBToken string `json:"rtdbToken"`
}

type wirePost struct {
	PostID         string   `json:"postId"`
	AuthorID       string   `json:"authorId"`
	AuthorUsername string   `json:"authorUsername"`
	Content        string   `json:"content"`
	Topics         []string `json:"topics"`
	RepliesCount   int      `json:"repliesCount"`
	BookmarksCount int      `json:"bookmarksCount"`
	IsPublic       bool     `json:"isPublic"`
	IsNSFW         bool     `json:"isNSFW"`
	Deleted        bool     `json:"deleted"`
	CreatedAt      string   `json:"createdAt"`
}

type wireUser struct {
	UserID         string `json:"userId"`
	Username       string `json:"username"`
	DisplayName    string `json:"displayName"`
	Email          string `json:"email"`
	Bio            string `json:"bio"`
	WebsiteUrl     string `json:"websiteUrl"`
	PinnedPostID   string `json:"pinnedPostId"`
	LocationName   string `json:"locationName"`
	FollowersCount int    `json:"followersCount"`
	FollowingCount int    `json:"followingCount"`
	PostsCount     int    `json:"postsCount"`
}

type wireFollow struct {
	FollowID   string `json:"followId"`
	FollowerID string `json:"followerId"`
	FollowedID string `json:"followedId"`
	CreatedAt  string `json:"createdAt"`
}

type wireReply struct {
	ReplyID        string `json:"replyId"`
	PostID         string `json:"postId"`
	AuthorID       string `json:"authorId"`
	AuthorUsername string `json:"authorUsername"`
	Content        string `json:"content"`
	ParentReplyID  string `json:"parentReplyId"`
	CreatedAt      string `json:"createdAt"`
}

type wireBookmark struct {
	BookmarkID string     `json:"bookmarkId"`
	Type       string     `json:"type"`
	PostID     string     `json:"postId"`
	ReplyID    string     `json:"replyId"`
	Post       *wirePost  `json:"post"`
	Reply      *wireReply `json:"reply"`
	CreatedAt  string     `json:"createdAt"`
}

type wireTopic struct {
	TopicID   string `json:"topicId"`
	Name      string `json:"name"`
	PostCount int    `json:"postsCount"`
}

type createBookmarkRequest struct {
	PostID  string `json:"postId,omitempty"`
	ReplyID string `json:"replyId,omitempty"`
	Type    string `json:"type"`
}

type wireNote struct {
	NoteID         string   `json:"noteId"`
	AuthorID       string   `json:"authorId"`
	Content        string   `json:"content"`
	Topics         []string `json:"topics"` // optional; omitted by API when empty
	RevisionNumber int      `json:"revisionNumber"`
	Deleted        bool     `json:"deleted"`
	CreatedAt      string   `json:"createdAt"`
}

type createNoteRequest struct {
	Content string   `json:"content"`
	Topics  []string `json:"topics,omitempty"` // omit when empty, matching API response shape
}

type updateNoteRequest struct {
	Content string   `json:"content"`
	Topics  []string `json:"topics,omitempty"` // omit when empty, matching API response shape
}

type createNoteResponseData struct {
	NoteID string `json:"noteId"`
}

type createBookmarkResponseData struct {
	BookmarkID string `json:"bookmarkId"`
}

type createPostRequest struct {
	Content  string   `json:"content"`
	Topics   []string `json:"topics"`
	IsPublic bool     `json:"isPublic"`
	IsNSFW   bool     `json:"isNSFW"`
}

type createPostResponseData struct {
	PostID string `json:"postId"`
}

type createReplyRequest struct {
	PostID        string `json:"postId"`
	Content       string `json:"content"`
	ParentReplyID string `json:"parentReplyId,omitempty"`
}

type createReplyResponseData struct {
	ReplyID string `json:"replyId"`
}

type wireNotificationMetadata struct {
	ReplyID        string `json:"replyId"`
	AuthorUsername string `json:"authorUsername"`
}

type wireNotification struct {
	ID            string                     `json:"id"`
	Type          string                     `json:"type"`
	Read          bool                       `json:"read"`
	CreatedAt     string                     `json:"createdAt"`
	ActorID       string                     `json:"actorId"`
	ActorUsername string                     `json:"actorUsername"`
	TargetID      string                     `json:"targetId"`
	TargetType    string                     `json:"targetType"`
	Metadata      wireNotificationMetadata   `json:"metadata"`
}

type updateProfileRequest struct {
	Bio          *string `json:"bio,omitempty"`
	DisplayName  *string `json:"displayName,omitempty"`
	PinnedPostID *string `json:"pinnedPostId,omitempty"`
	WebsiteUrl   *string `json:"websiteUrl,omitempty"`
	LocationName *string `json:"locationName,omitempty"`
}

type wireNotificationPrefs struct {
	Bookmark bool `json:"bookmark"`
	Reply    bool `json:"reply"`
	Poke     bool `json:"poke"`
}

// wireSettings is used to decode GET /v1/settings responses — includes all fields.
type wireSettings struct {
	Notifications      wireNotificationPrefs `json:"notifications"`
	FilterNSFW         bool                  `json:"filterNSFW"`
	ShowFollowerCount  bool                  `json:"showFollowerCount"`
	HideImagesInFeed   bool                  `json:"hideImagesInFeed"`
	HideAudioInFeed    bool                  `json:"hideAudioInFeed"`
	AutoWatchOnReply   bool                  `json:"autoWatchOnReply"`
	IconTheme          string                `json:"iconTheme"`
	FollowedTopics     []string              `json:"followedTopics"`
	MutedTopics        []string              `json:"mutedTopics"`
	ImagePixelSize     string                `json:"imagePixelSize"`
	TimeDisplayFormat  string                `json:"timeDisplayFormat"`
	UseLegacyMenuOrder bool                  `json:"useLegacyMenuOrder"`
	DefaultPublicPost  bool                  `json:"defaultPublicPost"`
}

// wirePatchSettings is the PATCH /v1/settings payload — only the 9 fields the
// UI manages. Deferred fields (iconTheme, imagePixelSize, followedTopics,
// mutedTopics) are intentionally excluded so the API never receives them.
type wirePatchSettings struct {
	Notifications      wireNotificationPrefs `json:"notifications"`
	FilterNSFW         bool                  `json:"filterNSFW"`
	ShowFollowerCount  bool                  `json:"showFollowerCount"`
	HideImagesInFeed   bool                  `json:"hideImagesInFeed"`
	HideAudioInFeed    bool                  `json:"hideAudioInFeed"`
	AutoWatchOnReply   bool                  `json:"autoWatchOnReply"`
	TimeDisplayFormat  string                `json:"timeDisplayFormat"`
	UseLegacyMenuOrder bool                  `json:"useLegacyMenuOrder"`
	DefaultPublicPost  bool                  `json:"defaultPublicPost"`
}

type envelope struct {
	Data   json.RawMessage `json:"data"`
	Cursor string          `json:"cursor"`
	Error  *apiError       `json:"error"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// --- client ---

// HTTPClient implements Client against the cyberspace.online REST API.
//
// NOTE: HTTPClient is not safe for concurrent use. The tokens field is mutated
// by Login and the internal refresh logic. In the current app, all API calls
// originate from Bubble Tea command goroutines which may run concurrently.
// A sync.Mutex should be added if concurrent access becomes a problem.
type HTTPClient struct {
	baseURL    string
	httpClient *http.Client
	tokens     model.Tokens
	rtdbClient *rtdb.Client // nil until InitRTDB is called
	currentUID string       // set from GetOwnProfile after login, used for RTDB paths
	debug      bool
}

// NewHTTPClient creates a production HTTPClient with a 15-second timeout.
func NewHTTPClient(baseURL string) *HTTPClient {
	return &HTTPClient{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// NewHTTPClientForTesting creates an HTTPClient with a custom http.Client.
// Intended for use in tests only — inject an httptest.Server client here.
func NewHTTPClientForTesting(baseURL string, hc *http.Client) *HTTPClient {
	return &HTTPClient{baseURL: baseURL, httpClient: hc}
}

// InitRTDB parses the rtdbToken to derive the Firebase project ID, constructs
// an rtdb.Client, and stores it for use by DM/chat methods. Called after login.
func (c *HTTPClient) InitRTDB(rtdbToken string) error {
	projectID, err := rtdb.ParseRTDBToken(rtdbToken)
	if err != nil {
		if c.isDebug() {
			fmt.Printf("[rtdb debug] InitRTDB: ParseRTDBToken failed: %v\n", err)
			// Print first 100 chars of token to help diagnose format.
			preview := rtdbToken
			if len(preview) > 100 {
				preview = preview[:100] + "..."
			}
			fmt.Printf("[rtdb debug] rtdbToken preview: %s\n", preview)
		}
		return fmt.Errorf("api: parse rtdb token: %w", err)
	}
	baseURL := rtdb.BaseURL(projectID)
	if c.isDebug() {
		fmt.Printf("[rtdb debug] InitRTDB: projectID=%q baseURL=%q\n", projectID, baseURL)
	}
	c.rtdbClient = rtdb.New(baseURL, rtdbToken)
	return nil
}

// SetRTDBClientForTesting injects a pre-built rtdb.Client. Test use only.
func (c *HTTPClient) SetRTDBClientForTesting(r *rtdb.Client) {
	c.rtdbClient = r
}

// SetCurrentUID stores the logged-in user ID for use in RTDB paths.
func (c *HTTPClient) SetCurrentUID(uid string) {
	c.currentUID = uid
}

// --- helpers ---

// doRequest sends method+path with bodyBytes, decodes the response envelope,
// and handles error codes. On 401 (outside auth endpoints) it refreshes the
// token and retries the request exactly once.
func (c *HTTPClient) doRequest(method, path string, bodyBytes []byte) (*envelope, error) {
	attempt := func() (*envelope, int, error) {
		var bodyReader io.Reader
		if len(bodyBytes) > 0 {
			bodyReader = bytes.NewReader(bodyBytes)
		}
		req, err := http.NewRequest(method, c.baseURL+path, bodyReader)
		if err != nil {
			return nil, 0, err
		}
		if len(bodyBytes) > 0 {
			req.Header.Set("Content-Type", "application/json")
		}
		if c.tokens.IDToken != "" {
			req.Header.Set("Authorization", "Bearer "+c.tokens.IDToken)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, 0, err
		}
		defer resp.Body.Close()

		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, resp.StatusCode, err
		}

		if resp.StatusCode == 429 {
			return nil, 429, ErrRateLimited
		}

		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return nil, resp.StatusCode, fmt.Errorf("decode response: %w", err)
		}

		if env.Error != nil {
			return nil, resp.StatusCode, &APIError{
				Code:    env.Error.Code,
				Message: env.Error.Message,
				Status:  resp.StatusCode,
			}
		}

		return &env, resp.StatusCode, nil
	}

	env, status, err := attempt()
	if status == 401 && !strings.HasPrefix(path, "/v1/auth/") {
		if refreshErr := c.refresh(); refreshErr != nil {
			return nil, ErrUnauthorized
		}
		env, _, err = attempt()
	}
	return env, err
}

// doJSON marshals body to JSON and calls doRequest.
func (c *HTTPClient) doJSON(method, path string, body any) (*envelope, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return c.doRequest(method, path, b)
}

// refresh calls POST /v1/auth/refresh directly (bypasses doRequest to avoid recursion).
// On success it updates c.tokens.IDToken and c.tokens.RTDBToken.
func (c *HTTPClient) refresh() error {
	b, _ := json.Marshal(refreshRequest{RefreshToken: c.tokens.RefreshToken})
	req, err := http.NewRequest("POST", c.baseURL+"/v1/auth/refresh", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	var env envelope
	json.Unmarshal(raw, &env)

	if env.Error != nil || resp.StatusCode != 200 {
		return ErrUnauthorized
	}

	var data refreshResponseData
	if err := json.Unmarshal(env.Data, &data); err != nil {
		return err
	}
	c.tokens.IDToken = data.IDToken
	c.tokens.RTDBToken = data.RTDBToken
	return nil
}

// --- conversion helpers ---

func wirePostToModel(w wirePost) model.Post {
	t, _ := time.Parse(time.RFC3339Nano, w.CreatedAt)
	return model.Post{
		ID:             w.PostID,
		AuthorID:       w.AuthorID,
		AuthorUsername: w.AuthorUsername,
		Content:        w.Content,
		Topics:         w.Topics,
		RepliesCount:   w.RepliesCount,
		BookmarksCount: w.BookmarksCount,
		IsPublic:       w.IsPublic,
		IsNSFW:         w.IsNSFW,
		Deleted:        w.Deleted,
		CreatedAt:      t,
	}
}

func wireReplyToModel(w wireReply) model.Reply {
	t, _ := time.Parse(time.RFC3339Nano, w.CreatedAt)
	return model.Reply{
		ID:             w.ReplyID,
		PostID:         w.PostID,
		AuthorID:       w.AuthorID,
		AuthorUsername: w.AuthorUsername,
		Content:        w.Content,
		ParentReplyID:  w.ParentReplyID,
		CreatedAt:      t,
	}
}

func wireUserToModel(w wireUser) model.User {
	return model.User{
		ID:             w.UserID,
		Username:       w.Username,
		DisplayName:    w.DisplayName,
		Email:          w.Email,
		Bio:            w.Bio,
		WebsiteUrl:     w.WebsiteUrl,
		PinnedPostID:   w.PinnedPostID,
		LocationName:   w.LocationName,
		FollowersCount: w.FollowersCount,
		FollowingCount: w.FollowingCount,
		PostsCount:     w.PostsCount,
	}
}

func wireFollowToModel(w wireFollow) model.Follow {
	t, _ := time.Parse(time.RFC3339, w.CreatedAt)
	return model.Follow{
		ID:         w.FollowID,
		FollowerID: w.FollowerID,
		FollowedID: w.FollowedID,
		CreatedAt:  t,
	}
}

func wireSettingsToModel(w wireSettings) model.Settings {
	return model.Settings{
		Notifications: model.NotificationPrefs{
			Bookmark: w.Notifications.Bookmark,
			Reply:    w.Notifications.Reply,
			Poke:     w.Notifications.Poke,
		},
		FilterNSFW:         w.FilterNSFW,
		ShowFollowerCount:  w.ShowFollowerCount,
		HideImagesInFeed:   w.HideImagesInFeed,
		HideAudioInFeed:    w.HideAudioInFeed,
		AutoWatchOnReply:   w.AutoWatchOnReply,
		IconTheme:          w.IconTheme,
		FollowedTopics:     w.FollowedTopics,
		MutedTopics:        w.MutedTopics,
		ImagePixelSize:     w.ImagePixelSize,
		TimeDisplayFormat:  w.TimeDisplayFormat,
		UseLegacyMenuOrder: w.UseLegacyMenuOrder,
		DefaultPublicPost:  w.DefaultPublicPost,
	}
}

func wireBookmarkToModel(w wireBookmark) model.Bookmark {
	t, _ := time.Parse(time.RFC3339Nano, w.CreatedAt)
	b := model.Bookmark{
		ID:        w.BookmarkID,
		Type:      w.Type,
		PostID:    w.PostID,
		ReplyID:   w.ReplyID,
		CreatedAt: t,
	}
	if w.Post != nil {
		p := wirePostToModel(*w.Post)
		b.Post = &p
	}
	if w.Reply != nil {
		r := wireReplyToModel(*w.Reply)
		b.Reply = &r
	}
	return b
}

func wireNotificationToModel(w wireNotification) model.Notification {
	t, _ := time.Parse(time.RFC3339Nano, w.CreatedAt)
	return model.Notification{
		ID:         w.ID,
		Type:       w.Type,
		Read:       w.Read,
		CreatedAt:  t,
		Actor:      model.NotificationActor{ID: w.ActorID, Username: w.ActorUsername},
		TargetID:   w.TargetID,
		TargetType: w.TargetType,
		ReplyID:              w.Metadata.ReplyID,
		ThreadAuthorUsername: w.Metadata.AuthorUsername,
	}
}

func wireTopicToModel(w wireTopic) model.Topic {
	return model.Topic{
		Slug:      w.TopicID,
		PostCount: w.PostCount,
	}
}

func wireNoteToModel(w wireNote) model.Note {
	t, _ := time.Parse(time.RFC3339Nano, w.CreatedAt)
	topics := w.Topics
	if topics == nil {
		topics = []string{}
	}
	return model.Note{
		ID:             w.NoteID,
		AuthorID:       w.AuthorID,
		Content:        w.Content,
		Topics:         topics,
		RevisionNumber: w.RevisionNumber,
		Deleted:        w.Deleted,
		CreatedAt:      t,
	}
}

// --- Client interface implementation ---

func (c *HTTPClient) Login(email, password string) (model.Tokens, error) {
	env, err := c.doJSON("POST", "/v1/auth/login", loginRequest{Email: email, Password: password})
	if err != nil {
		return model.Tokens{}, err
	}
	var data loginResponseData
	if err := json.Unmarshal(env.Data, &data); err != nil {
		return model.Tokens{}, err
	}
	c.tokens = model.Tokens{
		IDToken:      data.IDToken,
		RefreshToken: data.RefreshToken,
		RTDBToken:    data.RTDBToken,
	}
	return c.tokens, nil
}

// LoginWithRefreshToken exchanges a saved refresh token for a fresh IDToken and
// RTDBToken without requiring the user's password. On success the new tokens are
// stored in the client and returned. On failure ErrUnauthorized is returned.
func (c *HTTPClient) LoginWithRefreshToken(refreshToken string) (model.Tokens, error) {
	c.tokens.RefreshToken = refreshToken
	if err := c.refresh(); err != nil {
		return model.Tokens{}, err
	}
	return c.tokens, nil
}

// Logout clears the in-memory tokens. The v0.2 API has no server-side logout endpoint.
func (c *HTTPClient) Logout() error {
	c.tokens = model.Tokens{}
	return nil
}

func (c *HTTPClient) GetFeed(cursor string) ([]model.Post, string, error) {
	path := "/v1/posts?limit=20"
	if cursor != "" {
		path += "&cursor=" + url.QueryEscape(cursor)
	}
	env, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, "", err
	}
	var wire []wirePost
	if err := json.Unmarshal(env.Data, &wire); err != nil {
		return nil, "", err
	}
	posts := make([]model.Post, len(wire))
	for i, w := range wire {
		posts[i] = wirePostToModel(w)
	}
	return posts, env.Cursor, nil
}

func (c *HTTPClient) GetPostReplies(postID string) ([]model.Reply, error) {
	path := "/v1/posts/" + url.PathEscape(postID) + "/replies?limit=20"
	env, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}
	var wire []wireReply
	if err := json.Unmarshal(env.Data, &wire); err != nil {
		return nil, err
	}
	replies := make([]model.Reply, len(wire))
	for i, w := range wire {
		replies[i] = wireReplyToModel(w)
	}
	return replies, nil
}

func (c *HTTPClient) CreatePost(content string, topics []string) (model.Post, error) {
	env, err := c.doJSON("POST", "/v1/posts", createPostRequest{
		Content: content,
		Topics:  topics,
	})
	if err != nil {
		return model.Post{}, err
	}
	var data createPostResponseData
	if err := json.Unmarshal(env.Data, &data); err != nil {
		return model.Post{}, err
	}
	// API returns only postId on creation; return a minimal Post.
	return model.Post{ID: data.PostID, Content: content, Topics: topics}, nil
}

func (c *HTTPClient) DeletePost(postID string) error {
	_, err := c.doRequest("DELETE", "/v1/posts/"+url.PathEscape(postID), nil)
	return err
}

func (c *HTTPClient) DeleteReply(replyID string) error {
	_, err := c.doRequest("DELETE", "/v1/replies/"+url.PathEscape(replyID), nil)
	return err
}

func (c *HTTPClient) GetReply(replyID string) (model.Reply, error) {
	env, err := c.doRequest("GET", "/v1/replies/"+url.PathEscape(replyID), nil)
	if err != nil {
		return model.Reply{}, err
	}
	var wire wireReply
	if err := json.Unmarshal(env.Data, &wire); err != nil {
		return model.Reply{}, err
	}
	return wireReplyToModel(wire), nil
}

func (c *HTTPClient) CreateReply(postID, content, parentReplyID string) (model.Reply, error) {
	env, err := c.doJSON("POST", "/v1/replies", createReplyRequest{
		PostID:        postID,
		Content:       content,
		ParentReplyID: parentReplyID,
	})
	if err != nil {
		return model.Reply{}, err
	}
	var data createReplyResponseData
	if err := json.Unmarshal(env.Data, &data); err != nil {
		return model.Reply{}, err
	}
	return model.Reply{ID: data.ReplyID, PostID: postID, Content: content, ParentReplyID: parentReplyID}, nil
}

func (c *HTTPClient) GetPost(postID string) (model.Post, error) {
	env, err := c.doRequest("GET", "/v1/posts/"+url.PathEscape(postID), nil)
	if err != nil {
		return model.Post{}, err
	}
	var wire wirePost
	if err := json.Unmarshal(env.Data, &wire); err != nil {
		return model.Post{}, err
	}
	return wirePostToModel(wire), nil
}

func (c *HTTPClient) GetOwnProfile() (model.User, error) {
	env, err := c.doRequest("GET", "/v1/users/me", nil)
	if err != nil {
		return model.User{}, err
	}
	var wire wireUser
	if err := json.Unmarshal(env.Data, &wire); err != nil {
		return model.User{}, err
	}
	return wireUserToModel(wire), nil
}

func (c *HTTPClient) GetProfile(username string) (model.User, error) {
	env, err := c.doRequest("GET", "/v1/users/"+url.PathEscape(username), nil)
	if err != nil {
		return model.User{}, err
	}
	var wire wireUser
	if err := json.Unmarshal(env.Data, &wire); err != nil {
		return model.User{}, err
	}
	return wireUserToModel(wire), nil
}

func (c *HTTPClient) UpdateProfile(update model.ProfileUpdate) error {
	_, err := c.doJSON("PATCH", "/v1/users/me", updateProfileRequest{
		Bio:          update.Bio,
		DisplayName:  update.DisplayName,
		PinnedPostID: update.PinnedPostID,
		WebsiteUrl:   update.WebsiteUrl,
		LocationName: update.LocationName,
	})
	return err
}

// --- Settings ---

func (c *HTTPClient) GetSettings() (model.Settings, error) {
	env, err := c.doRequest("GET", "/v1/settings", nil)
	if err != nil {
		return model.Settings{}, err
	}
	var wire wireSettings
	if err := json.Unmarshal(env.Data, &wire); err != nil {
		return model.Settings{}, err
	}
	return wireSettingsToModel(wire), nil
}

func (c *HTTPClient) UpdateSettings(update model.Settings) error {
	_, err := c.doJSON("PATCH", "/v1/settings", wirePatchSettings{
		Notifications: wireNotificationPrefs{
			Bookmark: update.Notifications.Bookmark,
			Reply:    update.Notifications.Reply,
			Poke:     update.Notifications.Poke,
		},
		FilterNSFW:         update.FilterNSFW,
		ShowFollowerCount:  update.ShowFollowerCount,
		HideImagesInFeed:   update.HideImagesInFeed,
		HideAudioInFeed:    update.HideAudioInFeed,
		AutoWatchOnReply:   update.AutoWatchOnReply,
		TimeDisplayFormat:  update.TimeDisplayFormat,
		UseLegacyMenuOrder: update.UseLegacyMenuOrder,
		DefaultPublicPost:  update.DefaultPublicPost,
	})
	return err
}

// --- Notifications ---

func (c *HTTPClient) GetNotifications(cursor string) ([]model.Notification, string, error) {
	path := "/v1/notifications?limit=20"
	if cursor != "" {
		path += "&cursor=" + url.QueryEscape(cursor)
	}
	env, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, "", err
	}
	var wire []wireNotification
	if err := json.Unmarshal(env.Data, &wire); err != nil {
		return nil, "", err
	}
	notifs := make([]model.Notification, len(wire))
	for i, w := range wire {
		notifs[i] = wireNotificationToModel(w)
	}
	return notifs, env.Cursor, nil
}

func (c *HTTPClient) MarkNotificationRead(id string) error {
	_, err := c.doRequest("PATCH", "/v1/notifications/"+url.PathEscape(id), nil)
	return err
}

func (c *HTTPClient) MarkAllNotificationsRead() error {
	_, err := c.doRequest("POST", "/v1/notifications/read-all", nil)
	return err
}

// --- Bookmarks ---

func (c *HTTPClient) GetBookmarks(cursor string) ([]model.Bookmark, string, error) {
	path := "/v1/bookmarks?limit=20"
	if cursor != "" {
		path += "&cursor=" + url.QueryEscape(cursor)
	}
	env, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, "", err
	}
	var wire []wireBookmark
	if err := json.Unmarshal(env.Data, &wire); err != nil {
		return nil, "", err
	}
	bookmarks := make([]model.Bookmark, len(wire))
	for i, w := range wire {
		bookmarks[i] = wireBookmarkToModel(w)
	}
	return bookmarks, env.Cursor, nil
}

func (c *HTTPClient) CreateBookmark(postID, replyID string) (string, error) {
	req := createBookmarkRequest{}
	if postID != "" {
		req.PostID = postID
		req.Type = "post"
	} else {
		req.ReplyID = replyID
		req.Type = "reply"
	}
	env, err := c.doJSON("POST", "/v1/bookmarks", req)
	if err != nil {
		return "", err
	}
	var data createBookmarkResponseData
	if err := json.Unmarshal(env.Data, &data); err != nil {
		return "", err
	}
	return data.BookmarkID, nil
}

func (c *HTTPClient) DeleteBookmark(id string) error {
	_, err := c.doRequest("DELETE", "/v1/bookmarks/"+url.PathEscape(id), nil)
	return err
}

// --- Topics ---

func (c *HTTPClient) GetTopics(cursor string) ([]model.Topic, string, error) {
	path := "/v1/topics?limit=50"
	if cursor != "" {
		path += "&cursor=" + url.QueryEscape(cursor)
	}
	env, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, "", err
	}
	var wire []wireTopic
	if err := json.Unmarshal(env.Data, &wire); err != nil {
		return nil, "", err
	}
	topics := make([]model.Topic, len(wire))
	for i, w := range wire {
		topics[i] = wireTopicToModel(w)
	}
	return topics, env.Cursor, nil
}

func (c *HTTPClient) GetTopicPosts(slug string, cursor string) ([]model.Post, string, error) {
	path := "/v1/topics/" + url.PathEscape(slug) + "/posts?limit=20"
	if cursor != "" {
		path += "&cursor=" + url.QueryEscape(cursor)
	}
	env, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, "", err
	}
	var wire []wirePost
	if err := json.Unmarshal(env.Data, &wire); err != nil {
		return nil, "", err
	}
	posts := make([]model.Post, len(wire))
	for i, w := range wire {
		posts[i] = wirePostToModel(w)
	}
	return posts, env.Cursor, nil
}

// --- Chatrooms (RTDB stubs — pending feature/rtdb-chatrooms) ---

func (c *HTTPClient) GetRooms() ([]model.Room, error) {
	return nil, fmt.Errorf("not implemented: chatrooms use Firebase RTDB — see feature/rtdb-chatrooms")
}

func (c *HTTPClient) GetRoomMessages(roomID string, limit int) ([]model.Message, error) {
	return nil, fmt.Errorf("not implemented: chatrooms use Firebase RTDB — see feature/rtdb-chatrooms")
}

func (c *HTTPClient) SendRoomMessage(roomID, body string) error {
	return fmt.Errorf("not implemented: chatrooms use Firebase RTDB — see feature/rtdb-chatrooms")
}

// --- Direct messages (C-Mail) via Firebase RTDB ---
// NOTE: server-side RTDB paths are not yet finalised.
// Full implementation is in git history (commit e41884a).
// Wire types, rtdbOrErr, and wireRTDBMessageToModel are in that commit.

// GetConversations returns empty — server-side RTDB paths not yet finalised.
func (c *HTTPClient) GetConversations() ([]model.Conversation, error) {
	return []model.Conversation{}, nil
}

// GetMessages returns empty — server-side RTDB paths not yet finalised.
func (c *HTTPClient) GetMessages(conversationID string, limit int) ([]model.Message, error) {
	return []model.Message{}, nil
}

// SendMessage is a no-op — server-side RTDB paths not yet finalised.
func (c *HTTPClient) SendMessage(conversationID, body string) error {
	return nil
}

// SubscribeDMs returns an immediately-closed channel — server-side RTDB paths not yet finalised.
func (c *HTTPClient) SubscribeDMs(ctx context.Context, convID string) (<-chan model.Message, context.CancelFunc, error) {
	ch := make(chan model.Message)
	close(ch)
	return ch, func() {}, nil
}

// --- Follows ---

func (c *HTTPClient) GetFollowing(cursor string) ([]model.Follow, string, error) {
	path := "/v1/follows?type=following&limit=50"
	if cursor != "" {
		path += "&cursor=" + url.QueryEscape(cursor)
	}
	env, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, "", err
	}
	var wire []wireFollow
	if err := json.Unmarshal(env.Data, &wire); err != nil {
		return nil, "", err
	}
	follows := make([]model.Follow, len(wire))
	for i, w := range wire {
		follows[i] = wireFollowToModel(w)
	}
	return follows, env.Cursor, nil
}

func (c *HTTPClient) Follow(followedID string) (string, error) {
	env, err := c.doJSON("POST", "/v1/follows", map[string]string{"followedId": followedID})
	if err != nil {
		return "", err
	}
	var result struct {
		FollowID string `json:"followId"`
	}
	if err := json.Unmarshal(env.Data, &result); err != nil {
		return "", err
	}
	return result.FollowID, nil
}

func (c *HTTPClient) Unfollow(followID string) error {
	_, err := c.doRequest("DELETE", "/v1/follows/"+url.PathEscape(followID), nil)
	return err
}

// --- Notes ---

func (c *HTTPClient) GetNotes(cursor string) ([]model.Note, string, error) {
	path := "/v1/notes?limit=20"
	if cursor != "" {
		path += "&cursor=" + url.QueryEscape(cursor)
	}
	env, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, "", err
	}
	var wire []wireNote
	if err := json.Unmarshal(env.Data, &wire); err != nil {
		return nil, "", err
	}
	notes := make([]model.Note, len(wire))
	for i, w := range wire {
		notes[i] = wireNoteToModel(w)
	}
	return notes, env.Cursor, nil
}

func (c *HTTPClient) CreateNote(content string, topics []string) (model.Note, error) {
	env, err := c.doJSON("POST", "/v1/notes", createNoteRequest{Content: content, Topics: topics})
	if err != nil {
		return model.Note{}, err
	}
	var data createNoteResponseData
	if err := json.Unmarshal(env.Data, &data); err != nil {
		return model.Note{}, err
	}
	return model.Note{ID: data.NoteID, Content: content, Topics: topics, RevisionNumber: 1, CreatedAt: time.Now()}, nil
}

func (c *HTTPClient) UpdateNote(noteID, content string, topics []string) error {
	_, err := c.doJSON("PATCH", "/v1/notes/"+url.PathEscape(noteID), updateNoteRequest{Content: content, Topics: topics})
	return err
}

func (c *HTTPClient) DeleteNote(noteID string) error {
	_, err := c.doRequest("DELETE", "/v1/notes/"+url.PathEscape(noteID), nil)
	return err
}

// WithDebug enables or disables verbose RTDB debug output.
func (c *HTTPClient) WithDebug(debug bool) *HTTPClient {
	c.debug = debug
	return c
}

func (c *HTTPClient) isDebug() bool { return c.debug }
