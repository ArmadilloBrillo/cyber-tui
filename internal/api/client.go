package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
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
	UserID       string `json:"userId"`
	Username     string `json:"username"`
	DisplayName  string `json:"displayName"`
	Email        string `json:"email"`
	Bio          string `json:"bio"`
	WebsiteUrl   string `json:"websiteUrl"`
	PinnedPostID string `json:"pinnedPostId"`
	LocationName string `json:"locationName"`
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

type createPostRequest struct {
	Content  string   `json:"content"`
	Topics   []string `json:"topics"`
	IsPublic bool     `json:"isPublic"`
	IsNSFW   bool     `json:"isNSFW"`
}

type createPostResponseData struct {
	PostID string `json:"postId"`
}

type updateProfileRequest struct {
	Bio          *string `json:"bio,omitempty"`
	DisplayName  *string `json:"displayName,omitempty"`
	PinnedPostID *string `json:"pinnedPostId,omitempty"`
	WebsiteUrl   *string `json:"websiteUrl,omitempty"`
	LocationName *string `json:"locationName,omitempty"`
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
		if isDebug() {
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
	if isDebug() {
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
		ID:           w.UserID,
		Username:     w.Username,
		DisplayName:  w.DisplayName,
		Email:        w.Email,
		Bio:          w.Bio,
		WebsiteUrl:   w.WebsiteUrl,
		PinnedPostID: w.PinnedPostID,
		LocationName: w.LocationName,
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

// wireRTDBMessage is the Firebase wire shape for a DM message.
type wireRTDBMessage struct {
	SenderID       string  `json:"senderId"`
	SenderUsername string  `json:"senderUsername"`
	Content        string  `json:"content"`
	Timestamp      float64 `json:"timestamp"` // epoch ms (server timestamp arrives as number after roundtrip)
	Read           bool    `json:"read"`
}

// wireRTDBSSEData is the shape of the data field in a Firebase SSE event.
type wireRTDBSSEData struct {
	Path string          `json:"path"`
	Data json.RawMessage `json:"data"`
}

// wireRTDBConversation is the Firebase shape for a conversation entry under /user_conversations/<uid>.
// TODO: confirm this RTDB path with the API owner — probing /user_conversations/<userId>.
type wireRTDBConversation struct {
	ConversationID   string `json:"conversationId"`
	OtherUserID      string `json:"otherUserId"`
	OtherUsername    string `json:"otherUsername"`
	LastMessage      string `json:"lastMessage"`
	LastTimestamp    float64 `json:"lastTimestamp"`
}

func (c *HTTPClient) rtdbOrErr() (*rtdb.Client, error) {
	if c.rtdbClient == nil {
		return nil, fmt.Errorf("api: RTDB client not initialised (call InitRTDB after login)")
	}
	return c.rtdbClient, nil
}

// GetConversations fetches the list of DM conversations for the logged-in user.
// RTDB path: /user_conversations/<uid> — returns empty slice (not error) when null.
func (c *HTTPClient) GetConversations() ([]model.Conversation, error) {
	r, err := c.rtdbOrErr()
	if err != nil {
		return nil, err
	}
	if c.currentUID == "" {
		return nil, fmt.Errorf("api: currentUID not set — call GetOwnProfile after login")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	path := "/user_conversations/" + c.currentUID
	if isDebug() {
		fmt.Printf("[rtdb debug] GetConversations: querying path %q\n", path)
	}

	body, err := r.Get(ctx, path, nil)
	if err != nil {
		return nil, fmt.Errorf("api: GetConversations: %w", err)
	}

	if isDebug() {
		preview := string(body)
		if len(preview) > 300 {
			preview = preview[:300] + "..."
		}
		fmt.Printf("[rtdb debug] GetConversations: response body: %s\n", preview)
	}

	// Firebase returns "null" when the path doesn't exist — treat as empty.
	if string(body) == "null" {
		return []model.Conversation{}, nil
	}

	// Firebase returns an object keyed by push ID.
	var raw map[string]wireRTDBConversation
	if err := json.Unmarshal(body, &raw); err != nil {
		if isDebug() {
			fmt.Printf("[rtdb debug] GetConversations: unmarshal failed: %v\n", err)
		}
		return []model.Conversation{}, nil
	}

	convs := make([]model.Conversation, 0, len(raw))
	for _, wc := range raw {
		id := wc.ConversationID
		if id == "" {
			continue
		}
		other := model.User{ID: wc.OtherUserID, Username: wc.OtherUsername}
		self := model.User{ID: c.currentUID}
		convs = append(convs, model.Conversation{
			ID:           id,
			Participants: []model.User{self, other},
		})
	}
	return convs, nil
}

// GetMessages fetches up to limit recent messages for a conversation from RTDB.
func (c *HTTPClient) GetMessages(conversationID string, limit int) ([]model.Message, error) {
	r, err := c.rtdbOrErr()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	params := url.Values{
		"orderBy":     {`"timestamp"`},
		"limitToLast": {fmt.Sprintf("%d", limit)},
	}
	body, err := r.Get(ctx, "/dm_messages/"+conversationID, params)
	if err != nil {
		return nil, fmt.Errorf("api: GetMessages: %w", err)
	}

	if string(body) == "null" {
		return []model.Message{}, nil
	}

	var raw map[string]wireRTDBMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("api: GetMessages decode: %w", err)
	}

	msgs := make([]model.Message, 0, len(raw))
	for id, wm := range raw {
		msgs = append(msgs, wireRTDBMessageToModel(id, wm))
	}
	sort.Slice(msgs, func(i, j int) bool {
		return msgs[i].CreatedAt.Before(msgs[j].CreatedAt)
	})
	return msgs, nil
}

// SendMessage sends a DM to conversationID via RTDB PUT.
func (c *HTTPClient) SendMessage(conversationID, body string) error {
	r, err := c.rtdbOrErr()
	if err != nil {
		return err
	}

	msgID := fmt.Sprintf("%s-%d", c.currentUID, time.Now().UnixNano())
	payload := struct {
		SenderID       string `json:"senderId"`
		SenderUsername string `json:"senderUsername"`
		Content        string `json:"content"`
		Timestamp      any    `json:"timestamp"`
		Read           bool   `json:"read"`
	}{
		SenderID:  c.currentUID,
		Content:   body,
		Timestamp: map[string]string{".sv": "timestamp"},
		Read:      false,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := r.Put(ctx, "/dm_messages/"+conversationID+"/"+msgID, payload); err != nil {
		return fmt.Errorf("api: SendMessage: %w", err)
	}
	return nil
}

// SubscribeDMs opens a live SSE stream for a conversation.
// Returns a message channel and a cancel function.
// The channel is closed when cancel is called or the stream ends.
// The initial Firebase snapshot (path="/") is skipped; only new messages are emitted.
func (c *HTTPClient) SubscribeDMs(ctx context.Context, convID string) (<-chan model.Message, context.CancelFunc, error) {
	r, err := c.rtdbOrErr()
	if err != nil {
		return nil, nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	params := url.Values{"orderBy": {`"timestamp"`}}
	sseEvents := r.Subscribe(ctx, "/dm_messages/"+convID, params)

	out := make(chan model.Message, 8)
	go func() {
		defer close(out)
		for ev := range sseEvents {
			if ev.Err != nil {
				return
			}
			if ev.Event != "put" {
				continue
			}
			var d wireRTDBSSEData
			if err := json.Unmarshal(ev.Data, &d); err != nil {
				continue
			}
			// Skip the initial full-snapshot event (path="/").
			if d.Path == "/" {
				continue
			}
			var wm wireRTDBMessage
			if err := json.Unmarshal(d.Data, &wm); err != nil {
				continue
			}
			// Derive the message ID from the path (e.g. "/msgId").
			msgID := strings.TrimPrefix(d.Path, "/")
			select {
			case out <- wireRTDBMessageToModel(msgID, wm):
			case <-ctx.Done():
				return
			}
		}
	}()

	return out, cancel, nil
}

func wireRTDBMessageToModel(id string, wm wireRTDBMessage) model.Message {
	return model.Message{
		ID:        id,
		From:      model.User{ID: wm.SenderID, Username: wm.SenderUsername},
		Body:      wm.Content,
		CreatedAt: time.UnixMilli(int64(wm.Timestamp)),
	}
}

func isDebug() bool {
	v := os.Getenv("CYBERSPACE_DEBUG")
	return v == "1" || strings.EqualFold(v, "true")
}
