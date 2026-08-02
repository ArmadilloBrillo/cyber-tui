package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/rtdb"
	"github.com/ragnar/cyber-tui/internal/sanitize"
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

// maxResponseBytes caps how much of a response body is read into memory, guarding
// against a malicious or compromised endpoint returning an enormous body.
const maxResponseBytes = 10 << 20 // 10 MiB

// --- wire types (unexported JSON shapes matching the API) ---

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponseData struct {
	IDToken      string `json:"idToken"`
	RefreshToken string `json:"refreshToken"`
	RTDBToken    string `json:"rtdbToken"`
	RTDBUrl      string `json:"rtdbUrl"`
}

type refreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

type refreshResponseData struct {
	IDToken   string `json:"idToken"`
	RTDBToken string `json:"rtdbToken"`
	RTDBUrl   string `json:"rtdbUrl"`
}

type wireAttachment struct {
	Type   string `json:"type"`
	Src    string `json:"src"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
	Origin string `json:"origin,omitempty"`
	Artist string `json:"artist,omitempty"`
	Title  string `json:"title,omitempty"`
	Genre  string `json:"genre,omitempty"`
}

// wireStyle decodes a message's `style` field, sent as either a single
// string or an array of strings for chained styles (e.g. "rainbow" or
// ["comic","rainbow"]).
type wireStyle []string

func (s *wireStyle) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		*s = nil
		return nil
	}
	if b[0] == '[' {
		var arr []string
		if err := json.Unmarshal(b, &arr); err != nil {
			return err
		}
		*s = arr
		return nil
	}
	var str string
	if err := json.Unmarshal(b, &str); err != nil {
		return err
	}
	if str != "" {
		*s = []string{str}
	} else {
		*s = nil
	}
	return nil
}

// apiTimestamp decodes a JSON value that may be an RFC3339 string (the
// documented shape, used by every other endpoint), a numeric epoch-ms value,
// or a raw Firestore Timestamp object (`{"_seconds":N,"_nanoseconds":N}`, or
// the unprefixed `{"seconds":N,"nanoseconds":N}` variant) — all observed live
// on different hit types from GET /v1/search, which appears to serialize
// createdAt inconsistently. Undocumented drift from every other
// user/post/reply-returning endpoint; see docs/00-api-backlog.md.
//
// An unrecognized shape is logged and left empty rather than failing the
// whole decode — a malformed timestamp on one hit must never break the rest
// of a search response.
type apiTimestamp string

func (t *apiTimestamp) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		*t = ""
		return nil
	}
	switch b[0] {
	case '"':
		var s string
		if err := json.Unmarshal(b, &s); err == nil {
			*t = apiTimestamp(s)
			return nil
		}
	case '{':
		var fs struct {
			Seconds        int64 `json:"_seconds"`
			Nanoseconds    int64 `json:"_nanoseconds"`
			SecondsAlt     int64 `json:"seconds"`
			NanosecondsAlt int64 `json:"nanoseconds"`
		}
		if err := json.Unmarshal(b, &fs); err == nil {
			sec, nsec := fs.Seconds, fs.Nanoseconds
			if sec == 0 {
				sec = fs.SecondsAlt
			}
			if nsec == 0 {
				nsec = fs.NanosecondsAlt
			}
			if sec != 0 || nsec != 0 {
				*t = apiTimestamp(time.Unix(sec, nsec).UTC().Format(time.RFC3339Nano))
				return nil
			}
		}
	default:
		var ms int64
		if err := json.Unmarshal(b, &ms); err == nil {
			*t = apiTimestamp(time.UnixMilli(ms).UTC().Format(time.RFC3339Nano))
			return nil
		}
	}
	log.Printf("api: apiTimestamp: unrecognized value, leaving empty: %q", b)
	*t = ""
	return nil
}

type wirePost struct {
	PostID         string           `json:"postId"`
	AuthorID       string           `json:"authorId"`
	AuthorUsername string           `json:"authorUsername"`
	Content        string           `json:"content"`
	Title          string           `json:"title"`
	Slug           string           `json:"slug"`
	GuildID        string           `json:"guildId"`
	GuildSlug      string           `json:"guildSlug"`
	IsGuildThread  bool             `json:"isGuildThread"`
	Topics         []string         `json:"topics"`
	RepliesCount   int              `json:"repliesCount"`
	BookmarksCount int              `json:"bookmarksCount"`
	IsPublic       bool             `json:"isPublic"`
	IsNSFW         bool             `json:"isNSFW"`
	Deleted        bool             `json:"deleted"`
	CreatedAt      apiTimestamp     `json:"createdAt"`
	Attachments    []wireAttachment `json:"attachments"`
}

type wireUser struct {
	UserID            string       `json:"userId"`
	Username          string       `json:"username"`
	DisplayName       string       `json:"displayName"`
	Email             string       `json:"email"`
	Bio               string       `json:"bio"`
	WebsiteUrl        string       `json:"websiteUrl"`
	WebsiteName       string       `json:"websiteName"`
	WebsiteImageUrl   string       `json:"websiteImageUrl"`
	PinnedPostID      string       `json:"pinnedPostId"`
	LocationName      string       `json:"locationName"`
	LocationLatitude  float64      `json:"locationLatitude"`
	LocationLongitude float64      `json:"locationLongitude"`
	FollowersCount    int          `json:"followersCount"`
	FollowingCount    int          `json:"followingCount"`
	GuildSlug         string       `json:"guildSlug"`
	GuildID           string       `json:"guildId"`
	GuildName         string       `json:"guildName"`
	GuildIcon         string       `json:"guildIcon"`
	ProfilePictureUrl string       `json:"profilePictureUrl"`
	IsSupporter       bool         `json:"isSupporter"`
	SupporterIcon     string       `json:"supporterIcon"`
	SerialNumber      int          `json:"serialNumber"`
	PublicPostsCount  int          `json:"publicPostsCount"`
	HasPublicPosts    bool         `json:"hasPublicPosts"`
	CreatedAt         apiTimestamp `json:"createdAt"`
	LastActiveAt      apiTimestamp `json:"lastActiveAt"`
	UpdatedAt         apiTimestamp `json:"updatedAt"`
}

type wireFollow struct {
	FollowID         string `json:"followId"`
	FollowerID       string `json:"followerId"`
	FollowedID       string `json:"followedId"`
	FollowerUsername string `json:"followerUsername"`
	FollowedUsername string `json:"followedUsername"`
	CreatedAt        string `json:"createdAt"`
}

type wireNoteRevision struct {
	RevisionNumber int      `json:"revisionNumber"`
	Content        string   `json:"content"`
	Topics         []string `json:"topics"`
	CreatedAt      string   `json:"createdAt"`
}

type wireReply struct {
	ReplyID        string           `json:"replyId"`
	PostID         string           `json:"postId"`
	AuthorID       string           `json:"authorId"`
	AuthorUsername string           `json:"authorUsername"`
	Content        string           `json:"content"`
	ParentReplyID  string           `json:"parentReplyId"`
	CreatedAt      apiTimestamp     `json:"createdAt"`
	Attachments    []wireAttachment `json:"attachments"`
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

type wireWatch struct {
	ID        string `json:"id"`
	PostID    string `json:"postId"`
	CreatedAt string `json:"createdAt"`
}

type wireTopic struct {
	TopicID   string `json:"topicId"`
	Name      string `json:"name"`
	PostCount int    `json:"postsCount"`
}

type wireGuild struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Slug              string `json:"slug"`
	Icon              string `json:"icon"`
	Bio               string `json:"bio"`
	MemberCount       int    `json:"memberCount"`
	FounderUsername   string `json:"founderUsername"`
	CreatedAt         string `json:"createdAt"`
	IsMember          bool   `json:"isMember"`
	Role              string `json:"role"`
	Link              string `json:"link"`
	LinkText          string `json:"linkText"`
	ProfilePictureUrl string `json:"profilePictureUrl"`
}

type wireGuildMember struct {
	MembershipID      string `json:"membershipId"`
	GuildID           string `json:"guildId"`
	GuildSlug         string `json:"guildSlug"`
	UserID            string `json:"userId"`
	Username          string `json:"username"`
	Role              string `json:"role"`
	JoinedAt          string `json:"joinedAt"`
	DisplayName       string `json:"displayName"`
	ProfilePictureURL string `json:"profilePictureUrl"`
}

type createGuildPostRequest struct {
	Content string   `json:"content"`
	Title   string   `json:"title,omitempty"`
	Topics  []string `json:"topics"`
	Slug    string   `json:"slug,omitempty"`
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

type flagRequest struct {
	Reason string `json:"reason,omitempty"`
}

type flagResponseData struct {
	FlagID         string `json:"flagId"`
	AlreadyFlagged bool   `json:"alreadyFlagged"`
}

type createPostRequest struct {
	Content  string   `json:"content"`
	Title    string   `json:"title,omitempty"`
	Topics   []string `json:"topics"`
	IsPublic bool     `json:"isPublic"`
	IsNSFW   bool     `json:"isNSFW"`
	Slug     string   `json:"slug,omitempty"`
}

type createPostResponseData struct {
	PostID string `json:"postId"`
	Slug   string `json:"slug"`
	Title  string `json:"title"`
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
	GuildName      string `json:"guildName"`
	GuildSlug      string `json:"guildSlug"`
	PostSlug       string `json:"postSlug"`
	PostContent    string `json:"postContent"`
	ReplyContent   string `json:"replyContent"`
	RoomSlug       string `json:"roomSlug"`
	RoomName       string `json:"roomName"`
	MessageContent string `json:"messageContent"`
}

type wireNotification struct {
	ID            string                   `json:"id"`
	Type          string                   `json:"type"`
	Read          bool                     `json:"read"`
	CreatedAt     string                   `json:"createdAt"`
	ActorID       string                   `json:"actorId"`
	ActorUsername string                   `json:"actorUsername"`
	TargetID      string                   `json:"targetId"`
	TargetType    string                   `json:"targetType"`
	Metadata      wireNotificationMetadata `json:"metadata"`
}

type updateProfileRequest struct {
	Bio               *string  `json:"bio,omitempty"`
	DisplayName       *string  `json:"displayName,omitempty"`
	PinnedPostID      *string  `json:"pinnedPostId,omitempty"`
	WebsiteUrl        *string  `json:"websiteUrl,omitempty"`
	WebsiteName       *string  `json:"websiteName,omitempty"`
	WebsiteImageUrl   *string  `json:"websiteImageUrl,omitempty"`
	LocationName      *string  `json:"locationName,omitempty"`
	LocationLatitude  *float64 `json:"locationLatitude,omitempty"`
	LocationLongitude *float64 `json:"locationLongitude,omitempty"`
}

type wireNotificationPrefs struct {
	Bookmark bool `json:"bookmark"`
	Reply    bool `json:"reply"`
	Poke     bool `json:"poke"`
}

// wireSettings is used to decode GET /v1/settings responses — includes all fields.
type wireSettings struct {
	Notifications     wireNotificationPrefs `json:"notifications"`
	FilterNSFW        bool                  `json:"filterNSFW"`
	ShowFollowerCount bool                  `json:"showFollowerCount"`
	AutoWatchOnReply  bool                  `json:"autoWatchOnReply"`
	IconTheme         string                `json:"iconTheme"`
	FollowedTopics    []string              `json:"followedTopics"`
	MutedTopics       []string              `json:"mutedTopics"`
	ImagePixelSize    string                `json:"imagePixelSize"`
	TimeDisplayFormat string                `json:"timeDisplayFormat"`
	DefaultPublicPost bool                  `json:"defaultPublicPost"`
}

// wirePatchSettings is the PATCH /v1/settings payload — only the fields the
// UI manages. Deferred fields (iconTheme, imagePixelSize, followedTopics,
// mutedTopics) are intentionally excluded so the API never receives them.
type wirePatchSettings struct {
	Notifications     wireNotificationPrefs `json:"notifications"`
	FilterNSFW        bool                  `json:"filterNSFW"`
	ShowFollowerCount bool                  `json:"showFollowerCount"`
	AutoWatchOnReply  bool                  `json:"autoWatchOnReply"`
	TimeDisplayFormat string                `json:"timeDisplayFormat"`
	DefaultPublicPost bool                  `json:"defaultPublicPost"`
}

// --- CIRC wire types ---

// wireRoom is a single entry from GET /v1/circ.
type wireRoom struct {
	ID            string `json:"id"`
	Slug          string `json:"slug"`
	Name          string `json:"name"`
	LastMessageAt int64  `json:"lastMessageAt"` // epoch ms
	SortOrder     int    `json:"sortOrder"`
	OnlineCount   int    `json:"onlineCount"`
}

// wireRoomUser is a single entry from GET /v1/circ/:roomId/users.
type wireRoomUser struct {
	UserID      string `json:"userId"`
	Username    string `json:"username"`
	IsChatAdmin bool   `json:"isChatAdmin"`
	LastSeen    int64  `json:"lastSeen"` // epoch ms
}

// wirePresenceResponse is returned by POST/DELETE /v1/circ/:roomId/presence.
type wirePresenceResponse struct {
	RoomID       string `json:"roomId"`
	Ok           bool   `json:"ok"`
	HeartbeatMs  int    `json:"heartbeatMs"`
	StaleAfterMs int    `json:"staleAfterMs"`
}

// wireRTDBPresenceEntry is the Firebase shape for one user's entry in
// /chat_presence/<roomId>/<userId>.
type wireRTDBPresenceEntry struct {
	Username    string  `json:"username"`
	IsChatAdmin bool    `json:"isChatAdmin"`
	Online      bool    `json:"online"`
	LastSeen    float64 `json:"lastSeen"` // epoch ms as a Firebase number
}

// wireCircMessage is a single message from GET /v1/circ/:roomId.
type wireCircMessage struct {
	ID              string          `json:"id"`
	UserID          string          `json:"userId"`
	Username        string          `json:"username"`
	IsChatAdmin     bool            `json:"isChatAdmin"`
	IsAction        bool            `json:"isAction"` // undocumented; true for /me and other emote commands
	Content         string          `json:"content"`
	Timestamp       int64           `json:"timestamp"` // epoch ms
	Deleted         bool            `json:"deleted"`
	ImageUrl        string          `json:"imageUrl,omitempty"`
	GifUrl          string          `json:"gifUrl,omitempty"`
	AudioAttachment *wireAttachment `json:"audioAttachment,omitempty"`
	Style           wireStyle       `json:"style,omitempty"`
}

// --- C-Mail wire types ---

type wireCMailOtherUser struct {
	UserID   string `json:"userId"`
	Username string `json:"username"`
}

// wireCMailConversation is a single entry from GET /v1/cmail.
type wireCMailConversation struct {
	ConversationID string             `json:"conversationId"`
	OtherUser      wireCMailOtherUser `json:"otherUser"`
	LastMessage    string             `json:"lastMessage"`
	LastMessageAt  int64              `json:"lastMessageAt"` // epoch ms
	UnreadCount    int                `json:"unreadCount"`
}

// wireCMailMessage is a single message from GET /v1/cmail/:id.
type wireCMailMessage struct {
	ID              string          `json:"id"`
	SenderID        string          `json:"senderId"`
	SenderUsername  string          `json:"senderUsername"`
	IsAction        bool            `json:"isAction"` // undocumented; true for /me and other emote commands
	Content         string          `json:"content"`
	Timestamp       int64           `json:"timestamp"` // epoch ms
	ImageUrl        string          `json:"imageUrl,omitempty"`
	GifUrl          string          `json:"gifUrl,omitempty"`
	AudioAttachment *wireAttachment `json:"audioAttachment,omitempty"`
	Style           wireStyle       `json:"style,omitempty"`
}

// wireCMailStartResponse is returned by POST /v1/cmail.
type wireCMailStartResponse struct {
	ConversationID string             `json:"conversationId"`
	OtherUser      wireCMailOtherUser `json:"otherUser"`
}

// wireTypingResponse is returned by POST/DELETE /v1/cmail/:conversationId/typing.
type wireTypingResponse struct {
	ConversationID string `json:"conversationId"`
	Ok             bool   `json:"ok"`
	HeartbeatMs    int    `json:"heartbeatMs"`
	StaleAfterMs   int    `json:"staleAfterMs"`
}

// wireRTDBTypingEntry is the Firebase shape for one user's entry in
// /dm_presence/<conversationId>/<userId>.
type wireRTDBTypingEntry struct {
	Username  string  `json:"username"`
	Typing    bool    `json:"typing"`
	Timestamp float64 `json:"timestamp"` // epoch ms as a Firebase number
}

// wireRTDBMessage is the Firebase shape for a DM message in /dm_messages/<convId>/<msgId>.
type wireRTDBMessage struct {
	SenderID        string          `json:"senderId"`
	SenderUsername  string          `json:"senderUsername"`
	IsAction        bool            `json:"isAction"` // undocumented; true for /me and other emote commands
	Content         string          `json:"content"`
	Timestamp       float64         `json:"timestamp"` // epoch ms as a Firebase number
	Read            bool            `json:"read"`
	ImageUrl        string          `json:"imageUrl,omitempty"`
	GifUrl          string          `json:"gifUrl,omitempty"`
	AudioAttachment *wireAttachment `json:"audioAttachment,omitempty"`
	Style           wireStyle       `json:"style,omitempty"`
}

// wireRTDBCircMessage is the Firebase shape for a CIRC chatroom message in /chat_messages/<roomId>/<msgId>.
// Field names differ from DM messages (userId/username vs senderId/senderUsername).
type wireRTDBCircMessage struct {
	UserID          string          `json:"userId"`
	Username        string          `json:"username"`
	IsChatAdmin     bool            `json:"isChatAdmin"`
	IsAction        bool            `json:"isAction"` // undocumented; true for /me and other emote commands
	Content         string          `json:"content"`
	Timestamp       float64         `json:"timestamp"` // epoch ms as a Firebase number
	Deleted         bool            `json:"deleted"`
	ImageUrl        string          `json:"imageUrl,omitempty"`
	GifUrl          string          `json:"gifUrl,omitempty"`
	AudioAttachment *wireAttachment `json:"audioAttachment,omitempty"`
	Style           wireStyle       `json:"style,omitempty"`
}

// wireRTDBSSEData is the outer wrapper of a Firebase "put" SSE event's data field.
type wireRTDBSSEData struct {
	Path string          `json:"path"`
	Data json.RawMessage `json:"data"`
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
// Bubble Tea runs each command in its own goroutine, so API calls (and the 401
// refresh they may trigger) can execute concurrently. The tokens field is read
// by doRequest and written by Login/refresh/Logout; mu guards every access to it
// via the accessor methods below so reads and writes never race.
type HTTPClient struct {
	baseURL    string
	httpClient *http.Client
	mu         sync.Mutex
	tokens     model.Tokens
	rtdbClient *rtdb.Client // nil until InitRTDB is called
	currentUID string       // set from GetOwnProfile after login, used for RTDB paths
	debug      bool
}

// --- concurrency-safe token access ---

func (c *HTTPClient) idToken() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.tokens.IDToken
}

func (c *HTTPClient) currentRefreshToken() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.tokens.RefreshToken
}

func (c *HTTPClient) setTokens(t model.Tokens) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tokens = t
}

func (c *HTTPClient) snapshotTokens() model.Tokens {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.tokens
}

func (c *HTTPClient) applyRefresh(idToken, rtdbToken, rtdbUrl string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tokens.IDToken = idToken
	c.tokens.RTDBToken = rtdbToken
	if rtdbUrl != "" {
		c.tokens.RTDBUrl = rtdbUrl
	}
	if c.rtdbClient != nil {
		c.rtdbClient.SetToken(idToken)
	}
}

// NewHTTPClient creates a production HTTPClient with a 15-second timeout.
func NewHTTPClient(baseURL string) *HTTPClient {
	return &HTTPClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout:       15 * time.Second,
			CheckRedirect: refuseInsecureRedirect,
		},
	}
}

// refuseInsecureRedirect stops the bearer-token-bearing Authorization header
// from following a redirect to a non-https URL. validateBaseURL only checks
// the configured apiBaseURL once at startup; without this, a same-host
// scheme downgrade (https -> http) issued by the server itself would still
// carry the token, since Go's default client only strips Authorization on a
// cross-host redirect, not a same-host scheme change.
func refuseInsecureRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("api: stopped after 10 redirects")
	}
	if req.URL.Scheme != "https" {
		return fmt.Errorf("api: refusing redirect to non-https URL: %s", req.URL)
	}
	return nil
}

// NewHTTPClientForTesting creates an HTTPClient with a custom http.Client.
// Intended for use in tests only — inject an httptest.Server client here.
func NewHTTPClientForTesting(baseURL string, hc *http.Client) *HTTPClient {
	return &HTTPClient{baseURL: baseURL, httpClient: hc}
}

// InitRTDB initialises the Firebase RTDB client using the URL from the login/refresh
// response and the user's ID token. rtdbUrl must be the value from the API response —
// it must not be derived from the token, as the regional URL format differs from
// what JWT-based derivation would produce. idToken (not the API's rtdbToken field,
// which is a custom token for signInWithCustomToken) is what Firebase RTDB's REST/SSE
// auth query parameter actually accepts.
func (c *HTTPClient) InitRTDB(idToken, rtdbUrl string) error {
	if rtdbUrl == "" {
		return fmt.Errorf("api: InitRTDB: rtdbUrl is empty")
	}
	if c.isDebug() {
		fmt.Printf("[rtdb debug] InitRTDB: url=%q\n", rtdbUrl)
	}
	c.rtdbClient = rtdb.New(rtdbUrl, idToken)
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
		if tok := c.idToken(); tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, 0, err
		}
		defer resp.Body.Close()

		raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
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
	b, err := json.Marshal(refreshRequest{RefreshToken: c.currentRefreshToken()})
	if err != nil {
		return err
	}
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

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return fmt.Errorf("refresh: read body: %w", err)
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("refresh: decode response: %w", err)
	}

	if env.Error != nil || resp.StatusCode != 200 {
		return ErrUnauthorized
	}

	var data refreshResponseData
	if err := json.Unmarshal(env.Data, &data); err != nil {
		return err
	}
	c.applyRefresh(data.IDToken, data.RTDBToken, data.RTDBUrl)
	return nil
}

// RefreshSession proactively refreshes the ID token (and RTDB token) using the
// stored refresh token, without waiting for a failed request to trigger it.
// Safe to call concurrently with other requests.
func (c *HTTPClient) RefreshSession() error {
	return c.refresh()
}

// --- conversion helpers ---

// parseTime parses an RFC3339 timestamp and logs a warning if the value is
// non-empty but unparseable (indicates a server-side format change).
func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		log.Printf("api: parseTime %q: %v", s, err)
	}
	return t
}

func wireAttachmentsToModel(ws []wireAttachment) []model.Attachment {
	if len(ws) == 0 {
		return nil
	}
	out := make([]model.Attachment, len(ws))
	for i, w := range ws {
		out[i] = model.Attachment{
			Type:   w.Type,
			Src:    w.Src,
			Width:  w.Width,
			Height: w.Height,
			Origin: w.Origin,
			Artist: w.Artist,
			Title:  w.Title,
			Genre:  w.Genre,
		}
	}
	return out
}

// wireAudioAttachmentToModel converts a message's optional audioAttachment
// object to the model type. Returns nil when w is nil.
func wireAudioAttachmentToModel(w *wireAttachment) *model.Attachment {
	if w == nil {
		return nil
	}
	return &model.Attachment{
		Type:   "audio",
		Src:    w.Src,
		Origin: w.Origin,
		Artist: w.Artist,
		Title:  w.Title,
		Genre:  w.Genre,
	}
}

// decodeArtBody base64-decodes content when styles contains "art" (the
// /art command's response shape sends its ASCII art body base64-encoded).
// Returns content unchanged otherwise. An undecodable payload falls back to
// the raw string rather than dropping the message — same "never break the
// rest of the decode" philosophy as apiTimestamp's unrecognized-shape fallback.
func decodeArtBody(content string, styles []string) string {
	if !slices.Contains(styles, "art") {
		return content
	}
	decoded, err := base64.StdEncoding.DecodeString(content)
	if err != nil {
		log.Printf("api: decodeArtBody: bad base64 for art message")
		return content
	}
	return string(decoded)
}

func wirePostToModel(w wirePost) model.Post {
	sanitize.Strings(&w)
	t := parseTime(string(w.CreatedAt))
	return model.Post{
		ID:             w.PostID,
		AuthorID:       w.AuthorID,
		AuthorUsername: w.AuthorUsername,
		Content:        w.Content,
		Title:          w.Title,
		Slug:           w.Slug,
		GuildID:        w.GuildID,
		GuildSlug:      w.GuildSlug,
		IsGuildThread:  w.IsGuildThread,
		Topics:         w.Topics,
		RepliesCount:   w.RepliesCount,
		BookmarksCount: w.BookmarksCount,
		IsPublic:       w.IsPublic,
		IsNSFW:         w.IsNSFW,
		Deleted:        w.Deleted,
		CreatedAt:      t,
		Attachments:    wireAttachmentsToModel(w.Attachments),
	}
}

func wireReplyToModel(w wireReply) model.Reply {
	sanitize.Strings(&w)
	t := parseTime(string(w.CreatedAt))
	return model.Reply{
		ID:             w.ReplyID,
		PostID:         w.PostID,
		AuthorID:       w.AuthorID,
		AuthorUsername: w.AuthorUsername,
		Content:        w.Content,
		ParentReplyID:  w.ParentReplyID,
		CreatedAt:      t,
		Attachments:    wireAttachmentsToModel(w.Attachments),
	}
}

func wireUserToModel(w wireUser) model.User {
	sanitize.Strings(&w)
	return model.User{
		ID:                w.UserID,
		Username:          w.Username,
		DisplayName:       w.DisplayName,
		Email:             w.Email,
		Bio:               w.Bio,
		WebsiteUrl:        w.WebsiteUrl,
		WebsiteName:       w.WebsiteName,
		WebsiteImageUrl:   w.WebsiteImageUrl,
		PinnedPostID:      w.PinnedPostID,
		LocationName:      w.LocationName,
		LocationLatitude:  w.LocationLatitude,
		LocationLongitude: w.LocationLongitude,
		FollowersCount:    w.FollowersCount,
		FollowingCount:    w.FollowingCount,
		GuildSlug:         w.GuildSlug,
		GuildID:           w.GuildID,
		GuildName:         w.GuildName,
		GuildIcon:         w.GuildIcon,
		ProfilePictureUrl: w.ProfilePictureUrl,
		IsSupporter:       w.IsSupporter,
		SupporterIcon:     w.SupporterIcon,
		SerialNumber:      w.SerialNumber,
		PublicPostsCount:  w.PublicPostsCount,
		HasPublicPosts:    w.HasPublicPosts,
		CreatedAt:         parseTime(string(w.CreatedAt)),
		LastActiveAt:      parseTime(string(w.LastActiveAt)),
		UpdatedAt:         parseTime(string(w.UpdatedAt)),
	}
}

func wireFollowToModel(w wireFollow) model.Follow {
	sanitize.Strings(&w)
	t := parseTime(w.CreatedAt)
	return model.Follow{
		ID:               w.FollowID,
		FollowerID:       w.FollowerID,
		FollowedID:       w.FollowedID,
		FollowerUsername: w.FollowerUsername,
		FollowedUsername: w.FollowedUsername,
		CreatedAt:        t,
	}
}

func wireNoteRevisionToModel(w wireNoteRevision) model.NoteRevision {
	sanitize.Strings(&w)
	t := parseTime(w.CreatedAt)
	topics := w.Topics
	if topics == nil {
		topics = []string{}
	}
	return model.NoteRevision{
		RevisionNumber: w.RevisionNumber,
		Content:        w.Content,
		Topics:         topics,
		CreatedAt:      t,
	}
}

func wireSettingsToModel(w wireSettings) model.Settings {
	sanitize.Strings(&w)
	return model.Settings{
		Notifications: model.NotificationPrefs{
			Bookmark: w.Notifications.Bookmark,
			Reply:    w.Notifications.Reply,
			Poke:     w.Notifications.Poke,
		},
		FilterNSFW:        w.FilterNSFW,
		ShowFollowerCount: w.ShowFollowerCount,
		AutoWatchOnReply:  w.AutoWatchOnReply,
		IconTheme:         w.IconTheme,
		FollowedTopics:    w.FollowedTopics,
		MutedTopics:       w.MutedTopics,
		ImagePixelSize:    w.ImagePixelSize,
		TimeDisplayFormat: w.TimeDisplayFormat,
		DefaultPublicPost: w.DefaultPublicPost,
	}
}

func wireBookmarkToModel(w wireBookmark) model.Bookmark {
	sanitize.Strings(&w)
	t := parseTime(w.CreatedAt)
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
	sanitize.Strings(&w)
	t := parseTime(w.CreatedAt)
	return model.Notification{
		ID:                   w.ID,
		Type:                 w.Type,
		Read:                 w.Read,
		CreatedAt:            t,
		Actor:                model.NotificationActor{ID: w.ActorID, Username: w.ActorUsername},
		TargetID:             w.TargetID,
		TargetType:           w.TargetType,
		ReplyID:              w.Metadata.ReplyID,
		ThreadAuthorUsername: w.Metadata.AuthorUsername,
		GuildName:            w.Metadata.GuildName,
		GuildSlug:            w.Metadata.GuildSlug,
		PostSlug:             w.Metadata.PostSlug,
		PostAuthorUsername:   w.Metadata.AuthorUsername,
		PostContent:          w.Metadata.PostContent,
		ReplyContent:         w.Metadata.ReplyContent,
		RoomSlug:             w.Metadata.RoomSlug,
		RoomName:             w.Metadata.RoomName,
		MessageContent:       w.Metadata.MessageContent,
	}
}

func wireTopicToModel(w wireTopic) model.Topic {
	sanitize.Strings(&w)
	return model.Topic{
		Slug:      w.TopicID,
		PostCount: w.PostCount,
	}
}

func wireGuildToModel(w wireGuild) model.Guild {
	sanitize.Strings(&w)
	return model.Guild{
		ID:                w.ID,
		Name:              w.Name,
		Slug:              w.Slug,
		Icon:              w.Icon,
		Bio:               w.Bio,
		MemberCount:       w.MemberCount,
		FounderUsername:   w.FounderUsername,
		CreatedAt:         parseTime(w.CreatedAt),
		IsMember:          w.IsMember,
		Role:              w.Role,
		Link:              w.Link,
		LinkText:          w.LinkText,
		ProfilePictureUrl: w.ProfilePictureUrl,
	}
}

func wireGuildMemberToModel(w wireGuildMember) model.GuildMember {
	sanitize.Strings(&w)
	return model.GuildMember{
		MembershipID:      w.MembershipID,
		GuildID:           w.GuildID,
		GuildSlug:         w.GuildSlug,
		UserID:            w.UserID,
		Username:          w.Username,
		Role:              w.Role,
		JoinedAt:          parseTime(w.JoinedAt),
		DisplayName:       w.DisplayName,
		ProfilePictureUrl: w.ProfilePictureURL,
	}
}

func wireNoteToModel(w wireNote) model.Note {
	sanitize.Strings(&w)
	t := parseTime(w.CreatedAt)
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
	t := model.Tokens{
		IDToken:      data.IDToken,
		RefreshToken: data.RefreshToken,
		RTDBToken:    data.RTDBToken,
		RTDBUrl:      data.RTDBUrl,
	}
	c.setTokens(t)
	return t, nil
}

// LoginWithRefreshToken exchanges a saved refresh token for a fresh IDToken and
// RTDBToken without requiring the user's password. On success the new tokens are
// stored in the client and returned. On failure ErrUnauthorized is returned.
func (c *HTTPClient) LoginWithRefreshToken(refreshToken string) (model.Tokens, error) {
	c.setTokens(model.Tokens{RefreshToken: refreshToken})
	if err := c.refresh(); err != nil {
		return model.Tokens{}, err
	}
	return c.snapshotTokens(), nil
}

// Logout clears the in-memory tokens. The v0.2 API has no server-side logout endpoint.
func (c *HTTPClient) Logout() error {
	c.setTokens(model.Tokens{})
	return nil
}

// RawRequest makes an authenticated request and returns the raw JSON data from
// the response envelope. Intended for developer tooling; the token refresh/retry
// logic in doRequest applies normally.
func (c *HTTPClient) RawRequest(method, path string, body []byte) (json.RawMessage, error) {
	env, err := c.doRequest(method, path, body)
	if err != nil {
		return nil, err
	}
	return env.Data, nil
}

// fetchPage is a generic helper for all paginated GET endpoints that return a
// JSON array. It unmarshals the wire slice, converts each element, and returns
// the model slice together with the next-page cursor from the envelope.
func fetchPage[W, M any](c *HTTPClient, path string, convert func(W) M) ([]M, string, error) {
	env, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, "", err
	}
	var wire []W
	if err := json.Unmarshal(env.Data, &wire); err != nil {
		return nil, "", err
	}
	out := make([]M, len(wire))
	for i, w := range wire {
		out[i] = convert(w)
	}
	return out, env.Cursor, nil
}

func (c *HTTPClient) GetFeed(cursor string) ([]model.Post, string, error) {
	path := "/v1/posts?limit=20"
	if cursor != "" {
		path += "&cursor=" + url.QueryEscape(cursor)
	}
	return fetchPage(c, path, wirePostToModel)
}

func (c *HTTPClient) GetPostReplies(postID string) ([]model.Reply, error) {
	var all []model.Reply
	cursor := ""
	for {
		path := "/v1/posts/" + url.PathEscape(postID) + "/replies?limit=20"
		if cursor != "" {
			path += "&cursor=" + url.QueryEscape(cursor)
		}
		env, err := c.doRequest("GET", path, nil)
		if err != nil {
			return nil, err
		}
		var wire []wireReply
		if err := json.Unmarshal(env.Data, &wire); err != nil {
			return nil, err
		}
		for _, w := range wire {
			all = append(all, wireReplyToModel(w))
		}
		if env.Cursor == "" || len(wire) == 0 {
			break
		}
		cursor = env.Cursor
	}
	return all, nil
}

func (c *HTTPClient) CreatePost(content, title, slug string, topics []string, isPublic, isNSFW bool) (model.Post, error) {
	env, err := c.doJSON("POST", "/v1/posts", createPostRequest{
		Content:  content,
		Title:    title,
		Topics:   topics,
		IsPublic: isPublic,
		IsNSFW:   isNSFW,
		Slug:     slug,
	})
	if err != nil {
		return model.Post{}, err
	}
	var data createPostResponseData
	if err := json.Unmarshal(env.Data, &data); err != nil {
		return model.Post{}, err
	}
	sanitize.Strings(&data)
	return model.Post{
		ID:       data.PostID,
		Title:    data.Title,
		Slug:     data.Slug,
		Content:  content,
		Topics:   topics,
		IsPublic: isPublic,
		IsNSFW:   isNSFW,
	}, nil
}

func (c *HTTPClient) DeletePost(postID string) error {
	_, err := c.doRequest("DELETE", "/v1/posts/"+url.PathEscape(postID), nil)
	return err
}

func (c *HTTPClient) DeleteReply(replyID string) error {
	_, err := c.doRequest("DELETE", "/v1/replies/"+url.PathEscape(replyID), nil)
	return err
}

func (c *HTTPClient) FlagPost(postID, reason string) (string, bool, error) {
	env, err := c.doJSON("POST", "/v1/posts/"+url.PathEscape(postID)+"/flag", flagRequest{Reason: reason})
	if err != nil {
		return "", false, err
	}
	var data flagResponseData
	if err := json.Unmarshal(env.Data, &data); err != nil {
		return "", false, err
	}
	return data.FlagID, data.AlreadyFlagged, nil
}

func (c *HTTPClient) FlagReply(replyID, reason string) (string, bool, error) {
	env, err := c.doJSON("POST", "/v1/replies/"+url.PathEscape(replyID)+"/flag", flagRequest{Reason: reason})
	if err != nil {
		return "", false, err
	}
	var data flagResponseData
	if err := json.Unmarshal(env.Data, &data); err != nil {
		return "", false, err
	}
	return data.FlagID, data.AlreadyFlagged, nil
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
		Bio:               update.Bio,
		DisplayName:       update.DisplayName,
		PinnedPostID:      update.PinnedPostID,
		WebsiteUrl:        update.WebsiteUrl,
		WebsiteName:       update.WebsiteName,
		WebsiteImageUrl:   update.WebsiteImageUrl,
		LocationName:      update.LocationName,
		LocationLatitude:  update.LocationLatitude,
		LocationLongitude: update.LocationLongitude,
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
		FilterNSFW:        update.FilterNSFW,
		ShowFollowerCount: update.ShowFollowerCount,
		AutoWatchOnReply:  update.AutoWatchOnReply,
		TimeDisplayFormat: update.TimeDisplayFormat,
		DefaultPublicPost: update.DefaultPublicPost,
	})
	return err
}

// --- Notifications ---

func (c *HTTPClient) GetNotifications(cursor string, unreadOnly bool, types []string) ([]model.Notification, string, error) {
	path := "/v1/notifications?limit=20"
	if unreadOnly {
		path += "&read=false"
	}
	if len(types) > 0 {
		path += "&type=" + url.QueryEscape(strings.Join(types, ","))
	}
	if cursor != "" {
		path += "&cursor=" + url.QueryEscape(cursor)
	}
	return fetchPage(c, path, wireNotificationToModel)
}

func (c *HTTPClient) MarkNotificationRead(id string) error {
	_, err := c.doRequest("PATCH", "/v1/notifications/"+url.PathEscape(id), nil)
	return err
}

func (c *HTTPClient) MarkAllNotificationsRead() error {
	_, err := c.doRequest("POST", "/v1/notifications/read-all", nil)
	return err
}

func (c *HTTPClient) GetUnreadNotificationCount() (int, error) {
	env, err := c.doRequest("GET", "/v1/notifications/unread-count", nil)
	if err != nil {
		return 0, err
	}
	var data struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		return 0, err
	}
	return data.Count, nil
}

// --- Bookmarks ---

func (c *HTTPClient) GetBookmarks(cursor string) ([]model.Bookmark, string, error) {
	path := "/v1/bookmarks?limit=20"
	if cursor != "" {
		path += "&cursor=" + url.QueryEscape(cursor)
	}
	return fetchPage(c, path, wireBookmarkToModel)
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

// --- Thread watching ---

func wireWatchToModel(w wireWatch) model.Watch {
	return model.Watch{
		ID:        w.ID,
		PostID:    w.PostID,
		CreatedAt: parseTime(w.CreatedAt),
	}
}

func (c *HTTPClient) GetWatches(cursor string) ([]model.Watch, string, error) {
	path := "/v1/watches?limit=50"
	if cursor != "" {
		path += "&cursor=" + url.QueryEscape(cursor)
	}
	return fetchPage(c, path, wireWatchToModel)
}

func (c *HTTPClient) WatchPost(postID string) error {
	_, err := c.doRequest("POST", "/v1/posts/"+url.PathEscape(postID)+"/watch", nil)
	return err
}

func (c *HTTPClient) UnwatchPost(postID string) error {
	_, err := c.doRequest("DELETE", "/v1/posts/"+url.PathEscape(postID)+"/watch", nil)
	return err
}

// --- Topics ---

func (c *HTTPClient) GetTopics(cursor string) ([]model.Topic, string, error) {
	path := "/v1/topics?limit=50"
	if cursor != "" {
		path += "&cursor=" + url.QueryEscape(cursor)
	}
	return fetchPage(c, path, wireTopicToModel)
}

func (c *HTTPClient) GetTopicPosts(slug string, cursor string) ([]model.Post, string, error) {
	path := "/v1/topics/" + url.PathEscape(slug) + "/posts?limit=20"
	if cursor != "" {
		path += "&cursor=" + url.QueryEscape(cursor)
	}
	return fetchPage(c, path, wirePostToModel)
}

// --- Search ---

type wireSearchPreview struct {
	Users   []wireUser  `json:"users"`
	Posts   []wirePost  `json:"posts"`
	Replies []wireReply `json:"replies"`
}

// Search returns the grouped GET /v1/search?type=all preview.
func (c *HTTPClient) Search(query string) (model.SearchPreview, error) {
	path := "/v1/search?type=all&q=" + url.QueryEscape(query)
	env, err := c.doRequest("GET", path, nil)
	if err != nil {
		return model.SearchPreview{}, err
	}
	var data wireSearchPreview
	if err := json.Unmarshal(env.Data, &data); err != nil {
		return model.SearchPreview{}, err
	}
	out := model.SearchPreview{
		Users:   make([]model.User, len(data.Users)),
		Posts:   make([]model.Post, len(data.Posts)),
		Replies: make([]model.Reply, len(data.Replies)),
	}
	for i, w := range data.Users {
		out.Users[i] = wireUserToModel(w)
	}
	for i, w := range data.Posts {
		out.Posts[i] = wirePostToModel(w)
	}
	for i, w := range data.Replies {
		out.Replies[i] = wireReplyToModel(w)
	}
	return out, nil
}

// searchPath builds the /v1/search path for a typed (paginated) search.
// cursor, when non-empty, is passed back as the API's page-number param.
func searchPath(searchType, query, cursor string) string {
	path := "/v1/search?type=" + searchType + "&q=" + url.QueryEscape(query)
	if cursor != "" {
		path += "&page=" + url.QueryEscape(cursor)
	}
	return path
}

func (c *HTTPClient) SearchPosts(query, cursor string) ([]model.Post, string, error) {
	return fetchPage(c, searchPath("posts", query, cursor), wirePostToModel)
}

func (c *HTTPClient) SearchReplies(query, cursor string) ([]model.Reply, string, error) {
	return fetchPage(c, searchPath("replies", query, cursor), wireReplyToModel)
}

func (c *HTTPClient) SearchUsers(query, cursor string) ([]model.User, string, error) {
	return fetchPage(c, searchPath("users", query, cursor), wireUserToModel)
}

// --- Guilds ---

func (c *HTTPClient) GetGuilds(cursor string) ([]model.Guild, string, error) {
	path := "/v1/guilds?limit=20"
	if cursor != "" {
		path += "&cursor=" + url.QueryEscape(cursor)
	}
	return fetchPage(c, path, wireGuildToModel)
}

func (c *HTTPClient) GetGuild(slug string) (model.Guild, error) {
	env, err := c.doRequest("GET", "/v1/guilds/"+url.PathEscape(slug), nil)
	if err != nil {
		return model.Guild{}, err
	}
	var wire wireGuild
	if err := json.Unmarshal(env.Data, &wire); err != nil {
		return model.Guild{}, err
	}
	return wireGuildToModel(wire), nil
}

func (c *HTTPClient) GetGuildPosts(slug string, cursor string) ([]model.Post, string, error) {
	path := "/v1/guilds/" + url.PathEscape(slug) + "/posts?limit=20"
	if cursor != "" {
		path += "&cursor=" + url.QueryEscape(cursor)
	}
	return fetchPage(c, path, wirePostToModel)
}

func (c *HTTPClient) GetGuildMembers(slug, cursor string) ([]model.GuildMember, string, error) {
	path := "/v1/guilds/" + url.PathEscape(slug) + "/members?limit=20"
	if cursor != "" {
		path += "&cursor=" + url.QueryEscape(cursor)
	}
	return fetchPage(c, path, wireGuildMemberToModel)
}

func (c *HTTPClient) JoinGuild(slug string) error {
	_, err := c.doRequest("POST", "/v1/guilds/"+url.PathEscape(slug)+"/join", nil)
	return err
}

func (c *HTTPClient) LeaveGuild(slug string) error {
	_, err := c.doRequest("POST", "/v1/guilds/"+url.PathEscape(slug)+"/leave", nil)
	return err
}

func (c *HTTPClient) CreateGuildPost(slug, content, title, postSlug string, topics []string) (model.Post, error) {
	if topics == nil {
		topics = []string{}
	}
	env, err := c.doJSON("POST", "/v1/guilds/"+url.PathEscape(slug)+"/posts", createGuildPostRequest{
		Content: content,
		Title:   title,
		Topics:  topics,
		Slug:    postSlug,
	})
	if err != nil {
		return model.Post{}, err
	}
	var data createPostResponseData
	if err := json.Unmarshal(env.Data, &data); err != nil {
		return model.Post{}, err
	}
	sanitize.Strings(&data)
	return model.Post{
		ID:            data.PostID,
		Title:         data.Title,
		Slug:          data.Slug,
		Content:       content,
		Topics:        topics,
		GuildSlug:     slug,
		IsGuildThread: true,
	}, nil
}

// --- Chatrooms (CIRC) ---

// GetRooms lists the chatrooms available to the caller via GET /v1/circ.
func (c *HTTPClient) GetRooms() ([]model.Room, error) {
	env, err := c.doRequest("GET", "/v1/circ", nil)
	if err != nil {
		return nil, err
	}
	var wire []wireRoom
	if err := json.Unmarshal(env.Data, &wire); err != nil {
		return nil, err
	}
	out := make([]model.Room, len(wire))
	for i, w := range wire {
		sanitize.Strings(&w)
		out[i] = model.Room{
			ID:            w.ID,
			Slug:          w.Slug,
			Name:          w.Name,
			LastMessageAt: time.UnixMilli(w.LastMessageAt),
			SortOrder:     w.SortOrder,
			OnlineCount:   w.OnlineCount,
		}
	}
	return out, nil
}

// GetRoomMessages returns up to limit messages for roomID via GET /v1/circ/:roomId.
// Pass before=0 for the latest page; pass a previous message timestamp for older pages.
func (c *HTTPClient) GetRoomMessages(roomID string, limit int, before int64) ([]model.Message, error) {
	path := "/v1/circ/" + url.PathEscape(roomID) + fmt.Sprintf("?limit=%d", limit)
	if before > 0 {
		path += fmt.Sprintf("&before=%d", before)
	}
	env, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}
	var wire []wireCircMessage
	if err := json.Unmarshal(env.Data, &wire); err != nil {
		return nil, err
	}
	out := make([]model.Message, len(wire))
	for i, w := range wire {
		out[i] = wireCircMessageToModel(w)
	}
	return out, nil
}

// wireCircMessageToModel converts a REST cIRC chatroom message to the model type.
func wireCircMessageToModel(w wireCircMessage) model.Message {
	sanitize.Strings(&w)
	return model.Message{
		ID:              w.ID,
		From:            model.User{ID: w.UserID, Username: w.Username},
		Body:            decodeArtBody(w.Content, w.Style),
		CreatedAt:       time.UnixMilli(w.Timestamp),
		IsChatAdmin:     w.IsChatAdmin,
		IsAction:        w.IsAction,
		Deleted:         w.Deleted,
		ImageUrl:        w.ImageUrl,
		GifUrl:          w.GifUrl,
		AudioAttachment: wireAudioAttachmentToModel(w.AudioAttachment),
		Style:           []string(w.Style),
	}
}

// SendRoomMessage sends a message to a chatroom via POST /v1/circ/:roomId.
// Returns the server's reply text for reply-only commands (e.g. /help, which
// posts no message); empty for normal sends.
func (c *HTTPClient) SendRoomMessage(roomID, body string) (string, error) {
	env, err := c.doJSON("POST", "/v1/circ/"+url.PathEscape(roomID), map[string]string{"content": body})
	if err != nil {
		return "", err
	}
	var data struct {
		Reply string `json:"reply"`
	}
	_ = json.Unmarshal(env.Data, &data)
	return data.Reply, nil
}

// MarkRoomRead resets the unread indicator for roomID via POST /v1/circ/:roomId/read.
func (c *HTTPClient) MarkRoomRead(roomID string) error {
	_, err := c.doRequest("POST", "/v1/circ/"+url.PathEscape(roomID)+"/read", nil)
	return err
}

func (c *HTTPClient) FlagRoomMessage(roomID, messageID, reason string) (string, bool, error) {
	env, err := c.doJSON("POST", "/v1/circ/"+url.PathEscape(roomID)+"/messages/"+url.PathEscape(messageID)+"/flag", flagRequest{Reason: reason})
	if err != nil {
		return "", false, err
	}
	var data flagResponseData
	if err := json.Unmarshal(env.Data, &data); err != nil {
		return "", false, err
	}
	return data.FlagID, data.AlreadyFlagged, nil
}

// DeleteRoomMessage soft-deletes a message via DELETE /v1/circ/:roomId/messages/:messageId.
func (c *HTTPClient) DeleteRoomMessage(roomID, messageID string) error {
	_, err := c.doRequest("DELETE", "/v1/circ/"+url.PathEscape(roomID)+"/messages/"+url.PathEscape(messageID), nil)
	return err
}

// SubscribeRoom opens a live RTDB SSE stream for the given chatroom.
// New messages arrive on the returned channel; call cancel to close the stream.
// The initial full-snapshot event is skipped — load history via GetRoomMessages instead.
// The channel closes when the stream ends for any reason — a network error,
// an idle-read timeout, or the server sending a terminal auth_revoked/cancel
// event (see rtdb.Client.Subscribe) — not only on an outright disconnect.
// Callers should treat any close as "needs reconnect."
func (c *HTTPClient) SubscribeRoom(ctx context.Context, roomID string) (<-chan model.Message, context.CancelFunc, error) {
	r, err := c.rtdbOrErr()
	if err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	params := url.Values{
		"orderBy":     {`"timestamp"`},
		"limitToLast": {"50"},
	}
	sseEvents := r.Subscribe(ctx, "/chat_messages/"+roomID, params)
	out := make(chan model.Message, 8)
	go func() {
		defer close(out)
		for ev := range sseEvents {
			if ev.Err != nil {
				return
			}
			if ev.Event != "put" && ev.Event != "patch" {
				continue
			}
			var d wireRTDBSSEData
			if err := json.Unmarshal(ev.Data, &d); err != nil {
				continue
			}
			if d.Path == "/" {
				// Initial full-snapshot; history is loaded via REST (GetRoomMessages).
				continue
			}
			if len(d.Data) == 0 || string(d.Data) == "null" {
				// Deletion event — skip.
				continue
			}
			msgID := strings.TrimPrefix(d.Path, "/")
			var wm wireRTDBCircMessage
			if err := json.Unmarshal(d.Data, &wm); err != nil {
				continue
			}
			var msg model.Message
			if ev.Event == "patch" {
				// A patch on an existing message's path is currently only
				// ever a delete ({"content":"[DELETED]","deleted":true}),
				// which omits sender/timestamp/etc. entirely — unmarshaling
				// it as a full message would zero those fields out. Send
				// just {ID, Deleted}; callers must merge this onto the
				// existing message by ID, never append it as new.
				if !wm.Deleted {
					continue
				}
				msg = model.Message{ID: msgID, Deleted: true}
			} else {
				msg = wireRTDBCircMessageToModel(msgID, wm)
			}
			select {
			case out <- msg:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, cancel, nil
}

// GetRoomUsers returns who's currently present in roomID via GET /v1/circ/:roomId/users.
// The response is already staleness-filtered server-side.
func (c *HTTPClient) GetRoomUsers(roomID string) ([]model.RoomUser, error) {
	env, err := c.doRequest("GET", "/v1/circ/"+url.PathEscape(roomID)+"/users", nil)
	if err != nil {
		return nil, err
	}
	var wire []wireRoomUser
	if err := json.Unmarshal(env.Data, &wire); err != nil {
		return nil, err
	}
	out := make([]model.RoomUser, len(wire))
	for i, w := range wire {
		sanitize.Strings(&w)
		out[i] = model.RoomUser{
			UserID:      w.UserID,
			Username:    w.Username,
			IsChatAdmin: w.IsChatAdmin,
			LastSeen:    time.UnixMilli(w.LastSeen),
		}
	}
	return out, nil
}

// AnnouncePresence announces the caller's presence in roomID via POST
// /v1/circ/:roomId/presence. Returns the heartbeat cadence and staleness
// window (both ms) the caller should honor.
func (c *HTTPClient) AnnouncePresence(roomID string) (heartbeatMs, staleAfterMs int, err error) {
	env, err := c.doRequest("POST", "/v1/circ/"+url.PathEscape(roomID)+"/presence", nil)
	if err != nil {
		return 0, 0, err
	}
	var data wirePresenceResponse
	if err := json.Unmarshal(env.Data, &data); err != nil {
		return 0, 0, err
	}
	return data.HeartbeatMs, data.StaleAfterMs, nil
}

// LeaveRoomPresence removes the caller from roomID's presence list immediately
// via DELETE /v1/circ/:roomId/presence.
func (c *HTTPClient) LeaveRoomPresence(roomID string) error {
	_, err := c.doRequest("DELETE", "/v1/circ/"+url.PathEscape(roomID)+"/presence", nil)
	return err
}

// applyPresenceEvent merges one RTDB event into state, keyed by userId.
// A "put" at "/" replaces the whole map (a genuine full-snapshot event, or a
// full-tree deletion). A "patch" at "/" is Firebase's shape for a
// multi-location update touching several top-level keys at once (e.g. one
// user leaving, or several updating together) — only those listed keys are
// merged in (a null value deletes that key), leaving every other entry in
// state untouched. Conflating the two used to wipe the entire room's presence
// on a single-user "patch" removal, with everyone else only trickling back as
// their own next heartbeat re-added them. A "/<userId>" path replaces or
// deletes that one entry directly, regardless of event type. Deeper nested
// paths (e.g. a patch to just "/<userId>/lastSeen") are not specially handled
// and are ignored — a documented limitation; in practice each presence write
// replaces a user's whole entry.
func applyPresenceEvent(state map[string]wireRTDBPresenceEntry, event string, d wireRTDBSSEData) {
	empty := len(d.Data) == 0 || string(d.Data) == "null"
	if d.Path == "/" {
		if event != "put" {
			if empty {
				return
			}
			var patch map[string]json.RawMessage
			if err := json.Unmarshal(d.Data, &patch); err != nil {
				return
			}
			for k, raw := range patch {
				if len(raw) == 0 || string(raw) == "null" {
					delete(state, k)
					continue
				}
				var v wireRTDBPresenceEntry
				if err := json.Unmarshal(raw, &v); err != nil {
					continue
				}
				sanitize.Strings(&v)
				state[k] = v
			}
			return
		}
		for k := range state {
			delete(state, k)
		}
		if empty {
			return
		}
		var full map[string]wireRTDBPresenceEntry
		if err := json.Unmarshal(d.Data, &full); err != nil {
			return
		}
		for k, v := range full {
			sanitize.Strings(&v)
			state[k] = v
		}
		return
	}
	userID := strings.TrimPrefix(d.Path, "/")
	if strings.ContainsRune(userID, '/') {
		return // nested field patch — see doc comment above
	}
	if empty {
		delete(state, userID)
		return
	}
	var entry wireRTDBPresenceEntry
	if err := json.Unmarshal(d.Data, &entry); err != nil {
		return
	}
	sanitize.Strings(&entry)
	state[userID] = entry
}

// filterFreshPresence converts state to the sorted-by-caller snapshot of
// currently-online, non-stale users, per the docs' rule: online == true and
// lastSeen newer than staleAfterMs.
func filterFreshPresence(state map[string]wireRTDBPresenceEntry, staleAfterMs int) []model.RoomUser {
	now := time.Now().UnixMilli()
	out := make([]model.RoomUser, 0, len(state))
	for userID, e := range state {
		if !e.Online || now-int64(e.LastSeen) >= int64(staleAfterMs) {
			continue
		}
		out = append(out, model.RoomUser{
			UserID:      userID,
			Username:    e.Username,
			IsChatAdmin: e.IsChatAdmin,
			LastSeen:    time.UnixMilli(int64(e.LastSeen)),
		})
	}
	return out
}

// SubscribeRoomPresence opens a live RTDB SSE stream for roomID's presence
// node. Unlike SubscribeRoom, presence entries mutate and expire in place, so
// each receive is a full, filtered snapshot rather than a single incremental
// event — the internal state map is re-filtered and re-sent both on every
// RTDB event and on a periodic timer (an entry going stale produces no event
// of its own). The channel is only ever sent the latest snapshot (older
// pending ones are dropped) so a slow consumer never blocks the stream.
//
// initial seeds the merge state so a resubscribe (reconnect after a dropped
// stream) doesn't render an empty panel until the first fresh snapshot
// arrives — callers should pass the last known-good user list on reconnect,
// or nil for a brand-new subscription. Seeded entries age out normally via
// the existing staleAfterMs filter if they turn out to be gone.
func (c *HTTPClient) SubscribeRoomPresence(ctx context.Context, roomID string, staleAfterMs int, initial []model.RoomUser) (<-chan []model.RoomUser, context.CancelFunc, error) {
	r, err := c.rtdbOrErr()
	if err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	sseEvents := r.Subscribe(ctx, "/chat_presence/"+roomID, nil)
	out := make(chan []model.RoomUser, 1)
	send := func(users []model.RoomUser) {
		select {
		case out <- users:
		default:
			select {
			case <-out:
			default:
			}
			select {
			case out <- users:
			case <-ctx.Done():
			}
		}
	}
	go func() {
		defer close(out)
		state := make(map[string]wireRTDBPresenceEntry, len(initial))
		for _, u := range initial {
			state[u.UserID] = wireRTDBPresenceEntry{
				Username:    u.Username,
				IsChatAdmin: u.IsChatAdmin,
				Online:      true,
				LastSeen:    float64(u.LastSeen.UnixMilli()),
			}
		}
		if len(state) > 0 {
			// Make the seeded (last known-good) list visible right away, rather
			// than leaving the panel showing nothing until the first fresh
			// event or ticker tick — that gap is what made a resubscribe look
			// like a mass user drop.
			send(filterFreshPresence(state, staleAfterMs))
		}
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case ev, ok := <-sseEvents:
				if !ok {
					if c.isDebug() {
						log.Printf("[chat_presence %s] stream channel closed", roomID)
					}
					return
				}
				if ev.Err != nil {
					if c.isDebug() {
						log.Printf("[chat_presence %s] stream error: %v", roomID, ev.Err)
					}
					return
				}
				if c.isDebug() {
					log.Printf("[chat_presence %s] event=%q data=%s", roomID, ev.Event, ev.Data)
				}
				if ev.Event != "put" && ev.Event != "patch" {
					continue
				}
				var d wireRTDBSSEData
				if err := json.Unmarshal(ev.Data, &d); err != nil {
					continue
				}
				applyPresenceEvent(state, ev.Event, d)
				send(filterFreshPresence(state, staleAfterMs))
			case <-ticker.C:
				send(filterFreshPresence(state, staleAfterMs))
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, cancel, nil
}

// --- Direct messages (C-Mail) ---

// rtdbOrErr returns the RTDB client or an error if InitRTDB has not been called.
func (c *HTTPClient) rtdbOrErr() (*rtdb.Client, error) {
	if c.rtdbClient == nil {
		return nil, fmt.Errorf("api: RTDB client not initialised (call InitRTDB after login)")
	}
	return c.rtdbClient, nil
}

// wireRTDBMessageToModel converts a Firebase DM message to the model type.
func wireRTDBMessageToModel(id string, wm wireRTDBMessage) model.Message {
	sanitize.Strings(&wm)
	return model.Message{
		ID:              id,
		From:            model.User{ID: wm.SenderID, Username: wm.SenderUsername},
		Body:            decodeArtBody(wm.Content, wm.Style),
		CreatedAt:       time.UnixMilli(int64(wm.Timestamp)),
		IsAction:        wm.IsAction,
		ImageUrl:        wm.ImageUrl,
		GifUrl:          wm.GifUrl,
		AudioAttachment: wireAudioAttachmentToModel(wm.AudioAttachment),
		Style:           []string(wm.Style),
	}
}

// wireRTDBCircMessageToModel converts a Firebase CIRC chatroom message to the model type.
func wireRTDBCircMessageToModel(id string, wm wireRTDBCircMessage) model.Message {
	sanitize.Strings(&wm)
	return model.Message{
		ID:              id,
		From:            model.User{ID: wm.UserID, Username: wm.Username},
		Body:            decodeArtBody(wm.Content, wm.Style),
		CreatedAt:       time.UnixMilli(int64(wm.Timestamp)),
		IsChatAdmin:     wm.IsChatAdmin,
		IsAction:        wm.IsAction,
		Deleted:         wm.Deleted,
		ImageUrl:        wm.ImageUrl,
		GifUrl:          wm.GifUrl,
		AudioAttachment: wireAudioAttachmentToModel(wm.AudioAttachment),
		Style:           []string(wm.Style),
	}
}

// GetConversations lists the caller's C-Mail conversations via GET /v1/cmail.
// Sorted: unread first, then most recently active.
func (c *HTTPClient) GetConversations() ([]model.Conversation, error) {
	env, err := c.doRequest("GET", "/v1/cmail", nil)
	if err != nil {
		return nil, err
	}
	var wire []wireCMailConversation
	if err := json.Unmarshal(env.Data, &wire); err != nil {
		return nil, err
	}
	out := make([]model.Conversation, len(wire))
	for i, w := range wire {
		sanitize.Strings(&w)
		out[i] = model.Conversation{
			ID:            w.ConversationID,
			Participants:  []model.User{{ID: w.OtherUser.UserID, Username: w.OtherUser.Username}},
			UnreadCount:   w.UnreadCount,
			LastMessage:   w.LastMessage,
			LastMessageAt: time.UnixMilli(w.LastMessageAt),
		}
	}
	return out, nil
}

// GetMessages returns history for a conversation via GET /v1/cmail/:id.
// Messages are returned oldest-first.
func (c *HTTPClient) GetMessages(conversationID string, limit int, before int64) ([]model.Message, error) {
	path := "/v1/cmail/" + url.PathEscape(conversationID) + fmt.Sprintf("?limit=%d", limit)
	if before > 0 {
		path += fmt.Sprintf("&before=%d", before)
	}
	env, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}
	var wire []wireCMailMessage
	if err := json.Unmarshal(env.Data, &wire); err != nil {
		return nil, err
	}
	out := make([]model.Message, len(wire))
	for i, w := range wire {
		out[i] = wireCMailMessageToModel(w)
	}
	return out, nil
}

// wireCMailMessageToModel converts a REST C-Mail message to the model type.
func wireCMailMessageToModel(w wireCMailMessage) model.Message {
	sanitize.Strings(&w)
	return model.Message{
		ID:              w.ID,
		From:            model.User{ID: w.SenderID, Username: w.SenderUsername},
		Body:            decodeArtBody(w.Content, w.Style),
		CreatedAt:       time.UnixMilli(w.Timestamp),
		IsAction:        w.IsAction,
		ImageUrl:        w.ImageUrl,
		GifUrl:          w.GifUrl,
		AudioAttachment: wireAudioAttachmentToModel(w.AudioAttachment),
		Style:           []string(w.Style),
	}
}

// SendMessage sends a C-Mail message via POST /v1/cmail/:id. Returns the
// server's reply text for reply-only commands (e.g. /help, which posts no
// message); empty for normal sends.
func (c *HTTPClient) SendMessage(conversationID, body string) (string, error) {
	env, err := c.doJSON("POST", "/v1/cmail/"+url.PathEscape(conversationID), map[string]string{"content": body})
	if err != nil {
		return "", err
	}
	var data struct {
		Reply string `json:"reply"`
	}
	_ = json.Unmarshal(env.Data, &data)
	return data.Reply, nil
}

// StartConversation creates or retrieves a C-Mail conversation with recipientUsername
// via POST /v1/cmail. Returns 200 for an existing conversation, 201 for a new one.
func (c *HTTPClient) StartConversation(recipientUsername string) (model.Conversation, error) {
	env, err := c.doJSON("POST", "/v1/cmail", map[string]string{"recipientUsername": recipientUsername})
	if err != nil {
		return model.Conversation{}, err
	}
	var data wireCMailStartResponse
	if err := json.Unmarshal(env.Data, &data); err != nil {
		return model.Conversation{}, err
	}
	return model.Conversation{
		ID:           data.ConversationID,
		Participants: []model.User{{ID: data.OtherUser.UserID, Username: data.OtherUser.Username}},
	}, nil
}

// MarkCMailRead resets the unread count for conversationID via POST /v1/cmail/:id/read.
func (c *HTTPClient) MarkCMailRead(conversationID string) error {
	_, err := c.doRequest("POST", "/v1/cmail/"+url.PathEscape(conversationID)+"/read", nil)
	return err
}

// SubscribeDMs opens a live RTDB SSE stream for the given conversation.
// New messages arrive on the returned channel; call cancel to close the stream.
// The initial full-snapshot event is skipped — load history via GetMessages instead.
// The channel closes when the stream ends for any reason — a network error,
// an idle-read timeout, or the server sending a terminal auth_revoked/cancel
// event (see rtdb.Client.Subscribe) — not only on an outright disconnect.
// Callers should treat any close as "needs reconnect."
func (c *HTTPClient) SubscribeDMs(ctx context.Context, convID string) (<-chan model.Message, context.CancelFunc, error) {
	r, err := c.rtdbOrErr()
	if err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	params := url.Values{
		"orderBy":     {`"timestamp"`},
		"limitToLast": {"50"},
	}
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
			if d.Path == "/" {
				// Initial full-snapshot; history is loaded via REST (GetMessages).
				continue
			}
			if len(d.Data) == 0 || string(d.Data) == "null" {
				// Deletion event — skip.
				continue
			}
			var wm wireRTDBMessage
			if err := json.Unmarshal(d.Data, &wm); err != nil {
				continue
			}
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

// AnnounceTyping announces the caller is typing in conversationID via POST
// /v1/cmail/:conversationId/typing. Returns the heartbeat cadence and
// staleness window (both ms) the caller should honor.
func (c *HTTPClient) AnnounceTyping(conversationID string) (heartbeatMs, staleAfterMs int, err error) {
	env, err := c.doRequest("POST", "/v1/cmail/"+url.PathEscape(conversationID)+"/typing", nil)
	if err != nil {
		return 0, 0, err
	}
	var data wireTypingResponse
	if err := json.Unmarshal(env.Data, &data); err != nil {
		return 0, 0, err
	}
	return data.HeartbeatMs, data.StaleAfterMs, nil
}

// ClearTyping clears the caller's typing flag in conversationID immediately
// via DELETE /v1/cmail/:conversationId/typing.
func (c *HTTPClient) ClearTyping(conversationID string) error {
	_, err := c.doRequest("DELETE", "/v1/cmail/"+url.PathEscape(conversationID)+"/typing", nil)
	return err
}

// applyTypingEvent merges one RTDB event into state, keyed by userId. Mirrors
// applyPresenceEvent's "put"-vs-"patch" full-replace vs multi-key-merge
// handling at "/" (see that function's doc comment for why the two must be
// told apart) and its "/<userId>" single-entry handling — kept as a separate
// function since wireRTDBTypingEntry's shape (typing+timestamp) differs
// enough from wireRTDBPresenceEntry (online+lastSeen) that sharing code would
// cost more than the ~15 lines it'd save.
func applyTypingEvent(state map[string]wireRTDBTypingEntry, event string, d wireRTDBSSEData) {
	empty := len(d.Data) == 0 || string(d.Data) == "null"
	if d.Path == "/" {
		if event != "put" {
			if empty {
				return
			}
			var patch map[string]json.RawMessage
			if err := json.Unmarshal(d.Data, &patch); err != nil {
				return
			}
			for k, raw := range patch {
				if len(raw) == 0 || string(raw) == "null" {
					delete(state, k)
					continue
				}
				var v wireRTDBTypingEntry
				if err := json.Unmarshal(raw, &v); err != nil {
					continue
				}
				sanitize.Strings(&v)
				state[k] = v
			}
			return
		}
		for k := range state {
			delete(state, k)
		}
		if empty {
			return
		}
		var full map[string]wireRTDBTypingEntry
		if err := json.Unmarshal(d.Data, &full); err != nil {
			return
		}
		for k, v := range full {
			sanitize.Strings(&v)
			state[k] = v
		}
		return
	}
	userID := strings.TrimPrefix(d.Path, "/")
	if strings.ContainsRune(userID, '/') {
		return // nested field patch — see doc comment above
	}
	if empty {
		delete(state, userID)
		return
	}
	var entry wireRTDBTypingEntry
	if err := json.Unmarshal(d.Data, &entry); err != nil {
		return
	}
	sanitize.Strings(&entry)
	state[userID] = entry
}

// filterFreshTyping converts state to the snapshot of users currently typing
// and not yet stale: typing == true and timestamp newer than staleAfterMs.
func filterFreshTyping(state map[string]wireRTDBTypingEntry, staleAfterMs int) []model.TypingUser {
	now := time.Now().UnixMilli()
	out := make([]model.TypingUser, 0, len(state))
	for userID, e := range state {
		if !e.Typing || now-int64(e.Timestamp) >= int64(staleAfterMs) {
			continue
		}
		out = append(out, model.TypingUser{
			UserID:    userID,
			Username:  e.Username,
			Timestamp: time.UnixMilli(int64(e.Timestamp)),
		})
	}
	return out
}

// SubscribeDMTyping opens a live RTDB SSE stream for conversationID's typing
// node. Unlike SubscribeDMs, typing entries mutate and expire in place, so
// each receive is a full, filtered snapshot rather than a single incremental
// event — the internal state map is re-filtered and re-sent both on every
// RTDB event and on a periodic timer (an entry going stale produces no event
// of its own), mirroring SubscribeRoomPresence.
func (c *HTTPClient) SubscribeDMTyping(ctx context.Context, conversationID string, staleAfterMs int) (<-chan []model.TypingUser, context.CancelFunc, error) {
	r, err := c.rtdbOrErr()
	if err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	sseEvents := r.Subscribe(ctx, "/dm_presence/"+conversationID, nil)
	out := make(chan []model.TypingUser, 1)
	send := func(users []model.TypingUser) {
		select {
		case out <- users:
		default:
			select {
			case <-out:
			default:
			}
			select {
			case out <- users:
			case <-ctx.Done():
			}
		}
	}
	go func() {
		defer close(out)
		state := map[string]wireRTDBTypingEntry{}
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case ev, ok := <-sseEvents:
				if !ok || ev.Err != nil {
					return
				}
				if ev.Event != "put" && ev.Event != "patch" {
					continue
				}
				var d wireRTDBSSEData
				if err := json.Unmarshal(ev.Data, &d); err != nil {
					continue
				}
				applyTypingEvent(state, ev.Event, d)
				send(filterFreshTyping(state, staleAfterMs))
			case <-ticker.C:
				send(filterFreshTyping(state, staleAfterMs))
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, cancel, nil
}

// --- Follows ---

func (c *HTTPClient) GetFollowing(cursor string) ([]model.Follow, string, error) {
	path := "/v1/follows?type=following&limit=50"
	if cursor != "" {
		path += "&cursor=" + url.QueryEscape(cursor)
	}
	return fetchPage(c, path, wireFollowToModel)
}

func (c *HTTPClient) GetFollowers(cursor string) ([]model.Follow, string, error) {
	path := "/v1/follows?type=followers&limit=20"
	if cursor != "" {
		path += "&cursor=" + url.QueryEscape(cursor)
	}
	return fetchPage(c, path, wireFollowToModel)
}

func (c *HTTPClient) GetUserFollows(userID, followType, cursor string) ([]model.Follow, string, error) {
	path := "/v1/follows?userId=" + url.QueryEscape(userID) + "&type=" + url.QueryEscape(followType) + "&limit=20"
	if cursor != "" {
		path += "&cursor=" + url.QueryEscape(cursor)
	}
	return fetchPage(c, path, wireFollowToModel)
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
	return fetchPage(c, path, wireNoteToModel)
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

func (c *HTTPClient) GetNote(noteID string) (model.Note, error) {
	env, err := c.doRequest("GET", "/v1/notes/"+url.PathEscape(noteID), nil)
	if err != nil {
		return model.Note{}, err
	}
	var wire wireNote
	if err := json.Unmarshal(env.Data, &wire); err != nil {
		return model.Note{}, err
	}
	return wireNoteToModel(wire), nil
}

func (c *HTTPClient) GetNoteRevision(noteID string, revision int) (model.Note, error) {
	path := fmt.Sprintf("/v1/notes/%s?revision=%d", url.PathEscape(noteID), revision)
	env, err := c.doRequest("GET", path, nil)
	if err != nil {
		return model.Note{}, err
	}
	var wire wireNote
	if err := json.Unmarshal(env.Data, &wire); err != nil {
		return model.Note{}, err
	}
	return wireNoteToModel(wire), nil
}

func (c *HTTPClient) GetNoteRevisions(noteID, cursor string) ([]model.NoteRevision, string, error) {
	path := "/v1/notes/" + url.PathEscape(noteID) + "/revisions?limit=20"
	if cursor != "" {
		path += "&cursor=" + url.QueryEscape(cursor)
	}
	return fetchPage(c, path, wireNoteRevisionToModel)
}

// --- User posts and replies ---

func (c *HTTPClient) GetUserPosts(username, cursor string) ([]model.Post, string, error) {
	path := "/v1/users/" + url.PathEscape(username) + "/posts?limit=20"
	if cursor != "" {
		path += "&cursor=" + url.QueryEscape(cursor)
	}
	return fetchPage(c, path, wirePostToModel)
}

func (c *HTTPClient) GetUserReplies(username, cursor string) ([]model.Reply, string, error) {
	path := "/v1/users/" + url.PathEscape(username) + "/replies?limit=20"
	if cursor != "" {
		path += "&cursor=" + url.QueryEscape(cursor)
	}
	return fetchPage(c, path, wireReplyToModel)
}

// WithDebug enables or disables verbose RTDB debug output.
func (c *HTTPClient) WithDebug(debug bool) *HTTPClient {
	c.debug = debug
	return c
}

func (c *HTTPClient) isDebug() bool { return c.debug }
