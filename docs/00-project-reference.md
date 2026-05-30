# cyber-tui Project Reference

Comprehensive map of every module, file, and artifact in this repository. Use this as the starting point when navigating or extending the codebase.

---

## Overview

**cyber-tui** is a terminal user interface (TUI) client for [cyberspace.online](https://cyberspace.online) — a retro text-only social network. It is written in Go, using [Bubble Tea](https://github.com/charmbracelet/bubbletea) for the TUI event loop, [Lip Gloss](https://github.com/charmbracelet/lipgloss) for styling, and [Wish](https://github.com/charmbracelet/wish) to optionally host the client over SSH.

The client talks to the cyberspace.online REST API (v0.2) and to Firebase Realtime Database (RTDB) for live direct messages. See `docs/03-api-reference.md` for the baseline API spec.

---

## Repository Layout

```
cyber-tui/
├── cmd/
│   └── cyber-tui/
│       └── main.go              # Application entry point
├── internal/
│   ├── api/
│   │   ├── interface.go         # Client interface (contracts)
│   │   ├── client.go            # Production HTTP client
│   │   ├── mock.go              # Mock client (development / tests)
│   │   ├── client_test.go       # HTTP client tests
│   │   └── mock_test.go         # Mock client tests
│   ├── config/
│   │   ├── session.go           # Config load / save / timezone helpers
│   │   ├── session_test.go      # Config tests
│   │   └── timezone_test.go     # Timezone parsing tests
│   ├── model/
│   │   └── types.go             # Shared domain types
│   ├── rtdb/
│   │   ├── client.go            # Firebase RTDB REST + SSE client
│   │   ├── jwt.go               # JWT decode for RTDB project ID
│   │   └── client_test.go       # RTDB client tests
│   ├── ssh/
│   │   └── server.go            # SSH server (Wish)
│   └── ui/
│       ├── app.go               # Root Bubble Tea model
│       ├── app_test.go          # App integration tests
│       ├── screens/
│       │   ├── shared.go        # Cross-screen messages & types
│       │   ├── login.go         # Login screen
│       │   ├── feed.go          # Home feed screen
│       │   ├── postdetail.go    # Post + replies screen
│       │   ├── profile.go       # Profile view / edit screen
│       │   ├── notifications.go # Notification feed screen
│       │   ├── settings.go      # Settings screen
│       │   ├── cmail.go         # Direct messages (C-Mail) screen
│       │   ├── chatrooms.go     # Public chatrooms screen
│       │   ├── compose.go       # Reusable multi-line text editor
│       │   ├── timeutil.go      # Time formatting helpers
│       │   ├── timeutil_test.go # Time formatting tests
│       │   ├── notifications_test.go
│       │   ├── settings_test.go
│       │   ├── timezone_test.go
│       │   └── screens_test.go
│       ├── markdown/
│       │   ├── renderer.go      # GFM markdown → ANSI renderer + @mention extension
│       │   └── renderer_test.go # Renderer unit tests
│       ├── urlutil/
│       │   ├── extract.go       # ExtractURLs (goldmark AST walk) + NormalizeURL
│       │   ├── open.go          # OpenURL — OS browser launch (xdg-open / open)
│       │   └── extract_test.go  # URL extraction unit tests
│       └── theme/
│           ├── theme.go         # Color palettes + Lip Gloss styles
│           └── theme_test.go    # Theme tests
├── docs/                        # Feature documentation (numbered)
│   ├── 00-project-reference.md  # This file
│   ├── 00-api-backlog.md        # Unimplemented features and known server bugs
│   ├── 00-latest-api-reference.md # Live API spec snapshot
│   ├── 01-scaffold.md
│   ├── 02-menu-bar-navigation.md
│   ├── 03-api-reference.md
│   ├── 24-profile-sub-tabs.md   # Feature 24: profile Posts/Replies/Following/Followers tabs
│   ├── 25-note-revisions.md     # Feature 25: journal revision history
│   ├── 26-markdown-rendering.md # Feature 26: GFM markdown rendering
│   └── ... (see docs/ listing)
├── go.mod                       # Go module definition
├── go.sum                       # Dependency checksums
├── CLAUDE.md                    # Workflow guardrails for Claude
├── README.md                    # User-facing guide
└── cyber-tui                    # Compiled binary (gitignored in practice)
```

---

## Package Reference

---

### `cmd/cyber-tui`

#### `main.go`

Application entry point. Orchestrates startup:

1. Loads `~/.cyber-tui.json` via `config.Load()`
2. Applies theme via `theme.Set(config.Theme)`
3. Selects API client: `api.MockClient` when `config.UseMock == true`, otherwise `api.HTTPClient`
4. Decides which mode to start:
   - **SSH server mode** — if `config.SSHListenAddr` is set, calls `ssh.Serve()`
   - **Saved session** — if a refresh token exists, constructs `App` with `WithSavedSession()`
   - **Auto-login** — if `AutoEmail` + `AutoPassword` are set, calls `WithAutoLogin()`
   - **Fresh login** — otherwise starts at the login screen, optionally with `WithSavedEmail()`

---

### `internal/model`

#### `types.go`

Shared domain types used by both the API client and the UI. All types map 1-to-1 to cyberspace.online API response shapes; conversion is handled in `api/client.go`.

| Type | Purpose |
|---|---|
| `Tokens` | IDToken, RefreshToken, RTDBToken returned from login |
| `User` | Profile (ID, username, displayName, email, bio, websiteUrl, websiteName, websiteImageUrl, pinnedPostID, locationName, locationLatitude, locationLongitude) |
| `Post` | Feed item (ID, authorID, authorUsername, content, title, slug, guildID, guildSlug, isGuildThread, topics, repliesCount, bookmarksCount, isPublic, isNSFW, deleted, createdAt) |
| `Reply` | Comment on a post (ID, postID, authorID, authorUsername, content, parentReplyID, createdAt) |
| `ProfileUpdate` | Optional fields for PATCH /v1/users/me (all pointer types, includes new website/location fields) |
| `Message` | DM/chat message (ID, from, body, createdAt) |
| `Conversation` | 1-on-1 DM thread (ID, participants, messages) |
| `Room` | Public chatroom (ID, name, description, member count) |
| `NotificationPrefs` | Notification subscription toggles (bookmark, reply, poke) |
| `Settings` | All user preferences (notifications, content filters, display options) |
| `Notification` | Alert event (ID, type, read status, actor, targetID, targetType, replyID, threadAuthorUsername, guildName) |
| `Bookmark` | Saved post or reply (ID, type, postID/replyID, content snapshot, author, createdAt) |
| `Topic` | Tag with post count (slug, postCount) |
| `Guild` | Guild community (ID, name, slug, icon, bio, memberCount, founderUsername, createdAt, isMember, role) |
| `GuildMember` | Guild membership record (membershipID, guildID, guildSlug, userID, username, role, joinedAt, displayName) |
| `Follow` | Follow relationship (ID, followerID, followedID, followerUsername, followedUsername, createdAt) |
| `Note` | Private journal note (ID, authorID, content, topics, revisionNumber, deleted, createdAt) |
| `NoteRevision` | Single historical revision of a note (revisionNumber, content, topics, createdAt) |

---

### `internal/api`

Implements all API operations. The UI layer depends only on the `Client` interface, never on a concrete type.

#### `interface.go`

Defines the `Client` interface — the only type the UI layer imports from this package.

**Method groups:**

| Group | Methods |
|---|---|
| Auth | `Login(email, password)`, `LoginWithRefreshToken(token)`, `Logout()` |
| Feed | `GetFeed(cursor)`, `CreatePost(content, title, topics, isPublic, isNSFW)`, `GetPost(postID)`, `DeletePost(postID)` |
| Replies | `GetPostReplies(postID)`, `GetReply(replyID)`, `CreateReply(postID, content, parentReplyID)`, `DeleteReply(replyID)` |
| Profile | `GetOwnProfile()`, `GetProfile(username)`, `UpdateProfile(update)` |
| User History | `GetUserPosts(username, cursor)`, `GetUserReplies(username, cursor)` |
| Follows | `GetFollowing(cursor)`, `GetFollowers(cursor)`, `GetUserFollows(userID, followType, cursor)`, `Follow(followedID)`, `Unfollow(followID)` |
| Settings | `GetSettings()`, `UpdateSettings(update)` |
| Rooms | `GetRooms()`, `GetRoomMessages(roomID, limit)`, `SendRoomMessage(roomID, body)` |
| Notifications | `GetNotifications(cursor)`, `MarkNotificationRead(id)`, `MarkAllNotificationsRead()` |
| Bookmarks | `GetBookmarks(cursor)`, `CreateBookmark(postID, replyID)`, `DeleteBookmark(id)` |
| Topics | `GetTopics(cursor)`, `GetTopicPosts(slug, cursor)` |
| Notes | `GetNotes(cursor)`, `GetNote(noteID)`, `GetNoteRevision(noteID, revision)`, `GetNoteRevisions(noteID, cursor)`, `CreateNote(content, topics)`, `UpdateNote(noteID, content, topics)`, `DeleteNote(noteID)` |
| Direct Messages | `GetConversations()`, `GetMessages(convID, limit)`, `SendMessage(convID, body)`, `SubscribeDMs(ctx, convID) <-chan model.Message` |

#### `client.go`

Production REST HTTP client. Exported type: `HTTPClient`.

**Key exported identifiers:**

| Identifier | Kind | Purpose |
|---|---|---|
| `HTTPClient` | struct | HTTP client with baseURL, tokens, rtdbClient, debug flag |
| `APIError` | struct | Typed API error with Code, Message, Status |
| `ErrUnauthorized` | var | Sentinel for 401 responses |
| `ErrRateLimited` | var | Sentinel for 429 responses |
| `NewHTTPClient(baseURL)` | func | Production constructor |
| `NewHTTPClientForTesting(baseURL, hc)` | func | Test constructor with injected `http.Client` |
| `WithDebug(bool)` | method | Enables verbose request/response logging |

**Internal helpers (unexported):**

- `doRequest(method, path, body)` — core HTTP wrapper; automatically retries on 401 by calling `refresh()`
- `doJSON(method, path, body)` — marshals request body and delegates to `doRequest`
- `refresh()` — token refresh path; does not recurse
- `wirePostToModel()`, `wireReplyToModel()`, `wireUserToModel()`, `wireSettingsToModel()`, `wireNotificationToModel()`, `wireNoteToModel()` — convert API JSON wire types to `model.*` types

**Wire types (unexported):** `loginRequest`, `loginResponseData`, `wirePost`, `wireUser`, `wireReply`, `wireNotification`, `wireSettings`, `wireNote`, `wireBookmark`, `wireTopic`, `wireFollow` — match the JSON envelope shapes returned by the API.

**Note:** `HTTPClient` is not goroutine-safe. The `tokens` field is mutated by `Login` and `refresh`. Bubble Tea's single-update-loop model largely prevents concurrent mutation, but this may need a `sync.Mutex` if command goroutines become truly parallel.

#### `mock.go`

Development and test mock implementing `Client`. Exported type: `MockClient`.

- `NewMockClient()` creates a client pre-populated with static data: 3 users (neuromancer, molly_millions, wintermute), posts, replies, notifications, and conversations.
- Enabled by setting `useMock: true` in `~/.cyber-tui.json`.
- Allows full UI development without API credentials.

#### `client_test.go` / `mock_test.go`

Unit tests covering login/refresh flows, error handling (401, rate limiting, malformed responses), cursor pagination, type conversions, and mock client behavior.

---

### `internal/config`

#### `session.go`

Reads and writes the user's persistent configuration at `~/.cyber-tui.json`.

**Exported identifiers:**

| Identifier | Kind | Purpose |
|---|---|---|
| `Config` | struct | All persisted state (see Configuration File section below) |
| `Load()` | func | Reads config from disk; returns empty Config if missing |
| `Save(Config)` | func | Writes config with mode 0600 |
| `DefaultPath()` | func | Returns `~/.cyber-tui.json` |
| `Clear()` | func | Deletes the config file |
| `GetLocation()` | method | Parses `Config.Timezone` to `*time.Location` |
| `ParseTimezoneLabel(label)` | func | Converts "UTC+5:30" string to `*time.Location` |
| `AvailableTimezones` | var | Slice of 32 UTC offset labels (-12:00 to +14:00) |
| `IsRandomizeLocationEnabled()` | method | Returns true when `RandomizeLocation` is nil or true |
| `ShouldWanderNow(cfg)` | func | Returns true when wander is enabled and ≥12 h since last update |
| `WanderInterval` | const | Minimum time between wander updates (12 h) |

Passwords are not stored. The refresh token is used for auto-login; on 401 the app refreshes silently.

#### `session_test.go` / `timezone_test.go`

Tests for config round-trip (load/save) and timezone string parsing.

---

### `internal/rtdb`

Firebase Realtime Database client. Communicates with RTDB using its REST API and SSE (Server-Sent Events) for live subscriptions. Knows nothing about the application domain.

#### `client.go`

| Identifier | Kind | Purpose |
|---|---|---|
| `Client` | struct | RTDB HTTP client (baseURL, token, httpClient, streamClient) |
| `SSEEvent` | struct | Single SSE event (Event string, Data []byte, Err error) |
| `New(baseURL, token)` | func | Production constructor (15s timeout for REST, 0s for SSE) |
| `NewForTesting(baseURL, token, hc)` | func | Test constructor with injected `http.Client` |
| `Get(ctx, path, params)` | method | One-shot GET, returns raw JSON bytes |
| `Put(ctx, path, val)` | method | Marshals val to JSON and PUTs it |
| `Subscribe(ctx, path, params)` | method | Opens SSE stream; returns `<-chan SSEEvent` |

Internal: `readSSE(ctx, reader, ch)` parses the SSE wire format and forwards events; `buildURL(path, params)` constructs authenticated URLs.

#### `jwt.go`

| Identifier | Kind | Purpose |
|---|---|---|
| `ParseRTDBToken(token)` | func | Decodes RTDB JWT (no signature verify), extracts "aud" (Firebase project ID) |
| `BaseURL(projectID)` | func | Returns `https://{projectID}-default-rtdb.firebaseio.com` |

Used by `HTTPClient.InitRTDB` immediately after login to derive the RTDB base URL from the RTDB JWT.

#### `client_test.go`

Tests for RTDB REST operations, SSE parsing, and JWT decoding.

---

### `internal/ssh`

#### `server.go`

SSH server hosting via Wish (Charmbracelet).

`Serve(addr, hostKeyPath, client api.Client)`:

1. Creates a Wish server with Bubble Tea middleware
2. Each SSH connection gets a fresh `ui.App` instance — identical to the local TUI
3. Listens for SIGTERM/Interrupt and shuts down gracefully with a 5-second timeout

The SSH and local modes share all TUI code; no conditional logic in `ui/`.

---

### `internal/ui`

#### `app.go`

Root Bubble Tea model. Acts as the message hub and screen lifecycle manager.

**Exported identifiers:**

| Identifier | Kind | Purpose |
|---|---|---|
| `App` | struct | Root model; holds all sub-models, active screen, display state |
| `NewApp(client)` | func | Creates App |
| `WithSavedSession(config)` | method | Pre-loads session (tokens, username, timezone, etc.) |
| `WithAutoLogin(email, password)` | method | Pre-fills credentials for programmatic login |
| `WithSavedEmail(email)` | method | Pre-fills email field on login screen |

**Screen enum values:** `screenLogin`, `screenFeed`, `screenChatrooms`, `screenCMail`, `screenProfile`, `screenPostDetail`, `screenNotifications`, `screenBookmarks`, `screenGuilds`, `screenTopics`, `screenJournal`, `screenSettings`

**Responsibilities:**

- Routes Bubble Tea messages to the active screen or handles them at the app level
- Dispatches API calls in response to screen-emitted messages (`SubmitLoginMsg`, `LoadMoreFeedMsg`, `SubmitNewPostMsg`, etc.)
- Manages screen transitions (e.g., `ShowPostMsg` → navigate to PostDetail)
- Broadcasts `SharedConfigMsg` to all screens on resize, theme change, or settings update
- Renders tab bar, active screen, and status bar
- Manages global shortcuts (`1`–`4` screen jump, `v` density toggle, `?` help, `t` theme picker, `z` timezone picker, `o` URL opener, `q`/`ctrl+c` quit)
- Handles automatic token refresh on `ErrUnauthorized` responses
- Runs background tick jobs: `schedulePollCmd` (60 s unread count), `scheduleWanderCmd` (1 h wander check)

#### `app_test.go`

Integration tests covering screen transitions, token refresh, notification handling, and settings persistence.

---

### `internal/ui/screens`

All screen models implement Bubble Tea's `Model` interface (`Init`, `Update`, `View`). None hold API client references; they emit messages and App dispatches API calls.

#### `shared.go`

Cross-screen message types and shared enums.

| Type | Purpose |
|---|---|
| `SharedConfigMsg` | Broadcast by App on resize or settings change (Width, Height, Loc, Relaxed, Settings) |
| `ShowUserProfileMsg` | Emitted by Feed/PostDetail/Notifications on `p` (Username to look up) |
| `BackFromProfileMsg` | Emitted by Profile on ESC in read-only mode |
| `SaveSettingsMsg` | Emitted by Settings on ctrl+s (carries updated Settings) |

#### `login.go`

Email + password login form.

- Two `textinput` fields (email, password)
- Tab/Up/Down navigate between fields; Enter on the password field emits `SubmitLoginMsg` (picked up by App)
- Supports email prefill from config
- Shows loading spinner and error messages
- Renders a retro ASCII banner

Key types: `LoginModel`, `SubmitLoginMsg` (email + password), `LoginErrMsg` (error from App after failed login)

#### `feed.go`

Home feed of posts from followed users.

- Cursor-based pagination: emits `LoadMoreFeedMsg` when viewport reaches the bottom; App appends new posts
- Emits `RefreshFeedMsg` when pressing Up at the top
- Emits `ShowPostMsg` on Enter → App navigates to PostDetail
- Emits `SubmitNewPostMsg` on compose submit (content + topics)
- `n` opens compose for a new post (with topics input); `r` opens compose for a reply
- `d` on the selected post (own posts only) shows a y/n confirmation overlay; on `y` emits `DeletePostMsg`
- Dense/relaxed display modes; post content truncated to 4 lines in list view
- Timezone-aware timestamps via `displayTime()`

Key types: `FeedModel`, `LoadMoreFeedMsg`, `RefreshFeedMsg`, `ShowPostMsg`, `ShowPostForReplyMsg`, `SubmitNewPostMsg` (Content, Title, Topics, IsPublic, IsNSFW), `DeletePostMsg`  
Key function: `ParseTopics(s string) []string` — splits comma-separated string, caps at 3  
Key methods: `SetCurrentUsername(username)`, `RemovePost(postID)`

#### `postdetail.go`

Single post with all its replies in a scrollable pager.

- Post rendered at top; replies below with nesting (indicated by `parentReplyID`)
- `j`/`k` (or arrows) navigate/select post or individual replies
- `r` on a selected item opens the compose box (top-level or nested reply)
- `d` on the selected item (own post or own reply) shows a y/n confirmation overlay; on `y` emits `DeletePostMsg` or `DeleteReplyMsg`
- ESC emits `BackToFeedMsg` → App returns to feed
- `ScrollToReply(replyID)` scrolls to a specific reply (used by Notifications to deep-link)

Key types: `PostDetailModel`, `BackToFeedMsg`, `SubmitReplyMsg` (postID, parentReplyID, content), `DeletePostMsg`, `DeleteReplyMsg`  
Key methods: `SetCurrentUsername(username)`, `RemoveReply(replyID)`

#### `profile.go`

View and edit user profiles (own or others'). Includes five sub-tabs accessible in view mode.

- **Info tab** (default): username, displayName, bio, website, location, follow/edit hint
- **Posts tab**: paginated post history for the viewed user
- **Replies tab**: paginated reply history
- **Following tab**: users this person follows
- **Followers tab**: users who follow this person
- `tab` / `shift+tab` cycle sub-tabs in view mode
- `j` / `k` navigate items in list tabs; `enter` opens a post or navigates to a user profile
- `e` opens the multi-field inline editor (Info tab, own profile only)
- Tab/Shift+Tab in **edit mode** navigate between the 7 edit form fields
- Ctrl+S or ComposeSubmitMsg saves all fields via `SaveProfileMsg`; ESC cancels
- `SetReadOnly(true)` hides edit controls when viewing another user's profile
- `SetCanGoBack(true)` allows ESC to emit `BackFromProfileMsg`
- `ClearTabs()` resets sub-tab state (called when switching to a new user's profile)

Key types: `ProfileModel`, `SaveProfileMsg`, `ShowProfilePostMsg`, `ShowUserPostsMsg`, `ShowUserRepliesMsg`, `ShowUserFollowingMsg`, `ShowUserFollowersMsg`, `LoadMoreUserPostsMsg`, `LoadMoreUserRepliesMsg`, `LoadMoreUserFollowingMsg`, `LoadMoreUserFollowersMsg`

#### `notifications.go`

Notification feed (replies, new followers, pokes, bookmarks, thread replies).

- Cursor-based pagination matching the feed pattern
- `j`/`k` navigate; Up at top emits `RefreshNotifsMsg`
- `enter` emits `ShowNotificationPostMsg` → App navigates to PostDetail (includes ReplyID for scroll-to)
- `m` emits `MarkNotifReadMsg`; `M` emits `MarkAllNotifsReadMsg`
- `1` toggles unread-only filter

Key types: `NotificationsModel`, `LoadMoreNotifsMsg`, `RefreshNotifsMsg`, `MarkNotifReadMsg`, `MarkAllNotifsReadMsg`, `ShowNotificationPostMsg`  
Key methods: `SetNotifs(notifs, cursor)`, `AppendNotifs(notifs, cursor)`

#### `settings.go`

Editable user preferences rendered as a navigable list.

Settings are organised into static `settingsGroups`, each containing `settingsItem` rows with a label, a kind (`"bool"` or `"enum"`), and options for enum items.

**Managed fields (sent to API via PATCH):**

| Group | Field |
|---|---|
| Notifications | bookmark, reply, poke |
| Content | filterNSFW, hideImagesInFeed, hideAudioInFeed |
| Social | showFollowerCount, autoWatchOnReply, defaultPublicPost |
| Display | timeDisplayFormat (enum), useLegacyMenuOrder |

**Deferred fields** (read from API, never patched): `iconTheme`, `imagePixelSize`, `followedTopics`, `mutedTopics`

- `j`/`k` navigate; Space/Enter toggle booleans or cycle enum options
- ctrl+s emits `SaveSettingsMsg` with the updated settings; ESC discards
- Unsaved changes are highlighted

Key types: `SettingsModel`, `SaveSettingsMsg`

#### `cmail.go`

Direct messages (C-Mail) with live Firebase RTDB integration.

- Two-pane layout: conversation list (left) + active conversation chat (right)
- Subscribes to RTDB SSE stream via `api.Client.SubscribeDMs()` on conversation selection
- `waitForDM(sub)` is a Bubble Tea `Cmd` that blocks on the subscription channel and returns each incoming message as a `tea.Msg`
- `←`/`→` switch focus between panes; `j`/`k` navigate conversations; Enter sends a message

Key types: `CMailModel`, `CMailFocus` (`FocusCMailLeft` / `FocusCMailRight`)  
Key internal types: `dmSubscription` (RTDB channel + cancel func), `dmSubscribedMsg`, `dmReceivedMsg`, `dmStreamClosedMsg`, `cmailMsgsLoadedMsg`

#### `chatrooms.go`

Public chatroom browser and chat (UI complete; API integration deferred).

- Two-pane layout matching C-Mail
- Room selected with arrow keys or Enter; Enter in the input pane sends via `SendRoomMessageMsg`
- App handles `SendRoomMessageMsg` → `api.Client.SendRoomMessage()`

Key types: `ChatroomsModel`, `SendRoomMessageMsg`  
Key methods: `SetRooms(rooms)`, `SetActiveRoom(room, messages)`, `InputFocused()`

#### `bookmarks.go`

Saved posts and replies, cursor-paginated.

- `j`/`k` navigate items; `enter` opens the bookmarked post in PostDetail
- `d` removes the selected bookmark (emits `DeleteBookmarkMsg`)
- `b` on a post in Feed/PostDetail toggles a bookmark (emits `ToggleBookmarkMsg`)

Key types: `BookmarksModel`, `DeleteBookmarkMsg`, `ToggleBookmarkMsg`  
Key methods: `SetBookmarks(items, cursor)`, `AppendBookmarks(items, cursor)`, `RemoveBookmark(id)`

#### `guilds.go`

Browse the guild directory, drill into threads, and view guild members.

- Three-mode screen: guild list → guild thread feed → member list
- Guild list sorted by member count, cursor-paginated; `enter` opens the thread feed
- Thread feed is a standard paginated post list; `m` opens the member list, `esc` returns to the guild list
- Member list is cursor-paginated oldest-joined first; `enter` navigates to the member's profile, `esc` returns to the thread feed

Key types: `GuildsModel`, `LoadMoreGuildsMsg`, `LoadGuildPostsMsg`, `LoadMoreGuildPostsMsg`, `LoadGuildMembersMsg`, `LoadMoreGuildMembersMsg`  
Key methods: `SetGuilds`, `AppendGuilds`, `SetGuildPosts`, `AppendGuildPosts`, `SetGuildMembers`, `AppendGuildMembers`  
Key accessors: `IsBrowsingGuild()`, `IsBrowsingMembers()`

#### `topics.go`

Browse all topics (tags) and drill into posts for a selected topic.

- Two-mode screen: topic list → topic feed
- Topic list sorted by post count, cursor-paginated; `enter` opens the topic feed
- Topic feed is a standard paginated post list; `esc` returns to the topic list

Key types: `TopicsModel`, `LoadMoreTopicsMsg`, `LoadTopicPostsMsg`, `LoadMoreTopicPostsMsg`  
Key methods: `SetTopics(topics, cursor)`, `AppendTopics(topics, cursor)`, `SetTopicPosts(posts, cursor)`, `AppendTopicPosts(posts, cursor)`

#### `journal.go`

Private notes (Journal), cursor-paginated. Notes are visible only to the author.

- List mode: `j`/`k` navigate notes; `d` prompts to delete
- Edit mode (currently disabled): embeds `ComposeModel` (Ctrl+S saves, Ctrl+P publishes note as a post with confirmation, Esc cancels); `tab` toggles between compose and topics input
- Confirmation overlay (y/n) for publish and delete actions
- Viewport height dynamically adjusts when compose box grows or confirmation overlay appears

Note creation (`n`), editing (`enter`), deletion (`d`), and revision history (`h`) are all active. `PATCH /v1/notes/:id` was fixed server-side in API v0.4.

Key types: `JournalModel`, `SubmitSaveNoteMsg`, `SubmitPublishNoteMsg`, `SubmitDeleteNoteMsg`, `LoadMoreJournalMsg`, `LoadNoteRevisionsMsg`, `LoadNoteRevisionMsg`  
Key methods: `SetNotes(notes, cursor)`, `AppendNotes(notes, cursor)`, `PrependNote(note)`, `UpdateNoteContent(noteID, content, topics)`, `DeleteNote(noteID)`, `SetRevisions(noteID, revisions, cursor)`, `SetRevisionPreview(note)`

#### `compose.go`

Reusable multi-line text editor embedded in Feed, PostDetail, Profile, and C-Mail.

- Built on `bubbles/textarea`
- Enter inserts a paragraph break (`\n\n`); ctrl+s or alt+enter emits `ComposeSubmitMsg`; ESC emits `ComposeCancelMsg`
- Auto-expands from `composeMinLines=3` to `composeMaxLines=8` as content grows
- Character limit and placeholder text are configurable per embedding screen
- Active/inactive border styling (cyan when focused, dimmed otherwise)

Key types: `ComposeModel`, `ComposeSubmitMsg` (Content), `ComposeCancelMsg`  
Key methods: `Open(ctx, placeholder)`, `OpenWithContent(ctx, placeholder, content)`, `SetCharLimit(n)`, `SetWidth(w)`, `IsActive()`, `Content()`, `Close()`

#### `timeutil.go`

Time formatting helpers used by all screens.

| Function | Purpose |
|---|---|
| `formatTime(t, loc, fmt)` | Same-day → time only; older → "02-Jan-2006 HH:MM" |
| `formatRelativeTime(t, now, loc)` | Returns "Xm ago", "Xh ago", "Xd ago", or "02-Jan" |
| `dayLabel(t, now, loc)` | Returns "today", "yesterday", or "Mon 2 Jan" |
| `swatchBeats(t)` | Swatch Internet Time in @000–@999 format |
| `displayTime(t, loc, setting, compact)` | Routes to one of the above based on `Settings.TimeDisplayFormat` |

---

### `internal/ui/theme`

#### `theme.go`

Color palettes and Lip Gloss style objects for three retro themes.

**Layout constants:**

| Constant | Value | Purpose |
|---|---|---|
| `TabBarHeight` | 1 | Height of the tab/menu bar |
| `StatusBarHeight` | 1 | Height of the status bar |
| `SeparatorHeight` | 1 | Height of any horizontal separator |
| `ChromeHeight` | 3 | Sum of the above; used by screens to compute usable height |

**Color variables** (package-level; reassigned by `Set()`):  
`ColorGreen`, `ColorDimGreen`, `ColorCyan`, `ColorYellow`, `ColorRed`, `ColorBackground`, `ColorMuted`, `ColorWhite`

**Style objects** (Lip Gloss; auto-update when `Set()` is called):  
`Base`, `Title`, `Subtle`, `Highlight`, `Error`, `Border`, `ActiveBorder`, `StatusBar`, `Tab`, `ActiveTab`

**Functions:**

| Function | Purpose |
|---|---|
| `Set(name string)` | Applies a theme by name ("cyber", "c64", "vt320"); defaults to "cyber" |
| `CurrentName() string` | Returns the active theme name |

**Themes:**

| Name | Palette |
|---|---|
| **cyber** | Bright green-on-black (#00FF41) with cyan accents (#00FFFF) — default |
| **c64** | Commodore 64 purple background with cyan and bright magenta |
| **vt320** | VT320 terminal dim green with amber accents |

Because all colors are package variables, styles automatically inherit the new palette when `Set()` is called — no re-initialization needed.

---

## Configuration File

Location: `~/.cyber-tui.json`  
Permissions: `0600` (owner read/write only)

| Field | Type | Default | Purpose |
|---|---|---|---|
| `refreshToken` | string | — | Saved on login; used for auto-login |
| `username` | string | — | Saved on login |
| `email` | string | — | Saved on login |
| `savedAt` | string | — | ISO timestamp of last login |
| `density` | string | `""` | `""` = dense, `"relaxed"` = blank lines between items |
| `timezone` | string | `"UTC"` | UTC offset label (e.g. "UTC+5:30") |
| `theme` | string | `"cyber"` | `"cyber"`, `"c64"`, or `"vt320"` |
| `apiBaseURL` | string | `"https://api.cyberspace.online"` | Override for development |
| `useMock` | bool | `false` | Use `MockClient` instead of real API |
| `debug` | bool | `false` | Verbose RTDB / HTTP output |
| `autoEmail` | string | — | Pre-fill email on login screen |
| `autoPassword` | string | — | Pre-fill password (plain text; not recommended) |
| `sshListenAddr` | string | — | Enable SSH server mode (e.g. `":2222"`) |
| `sshHostKeyPath` | string | — | Path to SSH host private key |
| `wanderLust` | bool | `true` | Wander mode toggle; `true` = on, `false` = off |
| `lastWandered` | string | `""` (= never) | ISO timestamp of last wander mode update |

---

## Key Architectural Patterns

### Message-Driven Updates

All state changes flow through Bubble Tea messages. Screens emit domain-specific messages (`ShowPostMsg`, `SubmitLoginMsg`, `LoadMoreFeedMsg`). `App.Update()` intercepts these, dispatches API calls (as Bubble Tea `Cmd`s), and triggers screen transitions. Screens never call the API directly.

### Single Responsibility Packages

Each package has one job: `model/` holds types only, `rtdb/` knows nothing about app logic, `theme/` knows nothing about screens. The `ui/app.go` file is the only place that wires them together.

### Shared Configuration Broadcast

`App` sends `SharedConfigMsg` to every screen after a window resize, theme change, or settings save. Screens read only the fields they care about. This avoids point-to-point coupling between App and each screen.

### Transparent Token Refresh

`HTTPClient.doRequest()` catches 401 responses, calls `refresh()`, and retries the original request once. The UI layer never sees the refresh; it only ever receives the final success or a non-401 error.

### Cursor-Based Pagination

Feed and Notifications use opaque cursors (not page offsets). `App` stores the last cursor per screen and fires a load-more command when the viewport reaches the bottom. An empty cursor signals the last page.

### Two-Client Model

`HTTPClient` and `MockClient` both satisfy the `api.Client` interface. `main.go` selects one at startup based on config. The entire UI layer — App, screens, SSH server — is written against the interface, not the concrete type.

### Screen Polymorphism via Composition

All screens implement Bubble Tea's `Model` interface. `ComposeModel` is embedded directly in Feed, PostDetail, Profile, and C-Mail rather than being a separate screen, giving each a self-contained text editor without duplication.

---

## Keyboard Shortcuts

### Global

| Key | Action |
|---|---|
| `1` | Feed |
| `2` | Notifications |
| `3` | Journal (private notes) |
| `4` | Bookmarks |
| `5` | Topics |
| `6` | Profile |
| `7` | Settings |
| `←` / `→` | Cycle tabs left / right |
| `v` | Toggle dense / relaxed display |
| `?` | Help modal |
| `t` | Theme picker |
| `z` | Timezone picker |
| `q` / `ctrl+c` | Quit |

### Login

| Key | Action |
|---|---|
| `tab` / `↓` | Next field |
| `↑` | Previous field |
| `enter` | Submit (on password field) |

### Profile (view mode)

| Key | Action |
|---|---|
| `tab` | Next sub-tab (Info → Posts → Replies → Following → Followers) |
| `shift+tab` | Previous sub-tab |
| `j` / `↓` | Next item in list tab |
| `k` / `↑` | Previous item in list tab |
| `enter` | Open post (Posts/Replies tabs) or view user profile (Following/Followers tabs) |
| `e` | Edit profile (own profile, Info tab) |
| `f` | Follow / unfollow (read-only profiles) |
| `esc` | Back to previous screen |

### Feed

| Key | Action |
|---|---|
| `j` / `↓` | Next post |
| `k` / `↑` | Previous post (at top → refresh) |
| `enter` | Open post detail |
| `r` | Reply to selected post |
| `n` | New post |
| `d` | Delete selected post (own posts only — prompts y/n) |
| `p` | View author's profile |

### Post Detail

| Key | Action |
|---|---|
| `j` / `↓` | Scroll down / next reply |
| `k` / `↑` | Scroll up / previous reply |
| `r` | Reply to selected post or reply |
| `d` | Delete selected post or reply (own content only — prompts y/n) |
| `p` | View author's profile |
| `esc` | Back to feed |

### Compose (embedded)

| Key | Action |
|---|---|
| `enter` | Insert paragraph break |
| `tab` | Cycle focus: compose → title → topics → compose (Feed only) |
| `alt+enter` / `ctrl+s` | Submit |
| `alt+p` | Toggle public flag (Feed compose only) |
| `alt+s` | Toggle NSFW flag (Feed compose only) |
| `esc` | Cancel |

### Notifications

| Key | Action |
|---|---|
| `j` / `↓` | Next notification |
| `k` / `↑` | Previous (at top → refresh) |
| `enter` | Jump to referenced post/reply |
| `m` | Mark selected as read |
| `M` | Mark all as read |
| `1` | Toggle unread-only filter |

### C-Mail

| Key | Action |
|---|---|
| `j` / `k` or `↓` / `↑` | Navigate conversations (left pane) |
| `→` | Focus message input |
| `←` | Focus conversation list |
| `enter` | Send message |

### Settings

| Key | Action |
|---|---|
| `j` / `↓` | Next setting |
| `k` / `↑` | Previous setting |
| `space` / `enter` | Toggle boolean / cycle enum |
| `ctrl+s` | Save |
| `esc` | Discard |

---

## Dependencies

Listed from `go.mod`. Only direct dependencies:

| Module | Version | Purpose |
|---|---|---|
| `github.com/charmbracelet/bubbles` | v1.0.0 | UI components: textinput, viewport, textarea |
| `github.com/charmbracelet/bubbletea` | v1.3.10 | TUI event loop and model framework |
| `github.com/charmbracelet/lipgloss` | v1.1.0 | Terminal styling: colors, borders, layout |
| `github.com/charmbracelet/ssh` | v0.0.0-20250826160808-ebfa259c7309 | SSH protocol library |
| `github.com/charmbracelet/wish` | v1.4.7 | SSH server middleware wrapping Bubble Tea |

32 transitive dependencies cover color detection, terminal input, crypto (SSH), and other library support.

---

## Build & Test

```bash
# Build with version injection (recommended)
make build          # outputs to dist/cyber-tui

# Build without version (dev/quick)
go build -o cyber-tui ./cmd/cyber-tui

# Print version
./dist/cyber-tui --version

# Run all tests
go test ./...

# Static analysis
go vet ./...
```

### Versioning

Version metadata is injected at build time via `-ldflags` from `Makefile`. The version package (`internal/version`) defines three vars — `Version`, `Commit`, `Date` — defaulting to `"dev"/"none"/"unknown"` for untagged builds.

Release tags follow semver: `git tag -a v0.1.0 -m "v0.1.0"`. The `--version` flag and the help modal (`?`) both display the current version.

---

## Deferred / Known Limitations

| Area | Status |
|---|---|
| **Chatrooms API** | UI fully built; REST integration deferred (server paths not finalized) |
| **C-Mail REST** | Conversation list + history loaded from mock; RTDB subscribe wired; full path confirmed post-beta |
| **HTTPClient thread safety** | Tokens field mutated by Login/refresh with no mutex; acceptable under Bubble Tea's single-update loop, but may need `sync.Mutex` if command goroutines become truly concurrent |
| **Settings — deferred fields** | `iconTheme`, `imagePixelSize`, `followedTopics`, `mutedTopics` are read from the API but intentionally excluded from PATCH until the server-side feature is finalized |
| **Journal write operations** | Fully operational. `PATCH /v1/notes/:id` was fixed server-side in API v0.4. |
| **Post/reply deletion** | Wired and working — `d` key in Feed (own posts) and Post Detail (own posts and replies) |
| **Attachments** | Image and YouTube audio attachments on posts/replies are not supported in the TUI |
| **Note revision pagination** | `GetNoteRevisions` cursor is implemented in the API client but the UI loads only the first page |
| **Profile navigation depth** | Navigating from a Following/Followers tab to another user's profile is single-level; ESC returns to the original `profileReturn` destination, not the intermediate profile |
| **Feed position — deep pagination** | When returning to the Feed tab, the selected post is restored by ID from the fresh first-page load. If the post was reached via pagination it will not be in page 1 and the feed falls back to the top. Fix options: re-fetch pages sequentially until found (expensive), or skip the tab-switch reload (stale data). Neither is warranted for typical usage. |
| **Ambiguous-width character stripping** | Unicode EAW = "A" characters (kaomoji symbols, `©`, `®`, `™`, Greek letters, etc.) are stripped at two points: (1) `stripAmbiguousRunes` in `shared.go` strips them from post/reply content before display; (2) `filterAmbiguousKeyMsg` in `shared.go` intercepts `tea.KeyRunes` messages before they reach any `textarea` or `textinput` component (compose, topics, profile fields, C-Mail, chatrooms). Their column width is undefined and varies by terminal/font, causing border overflow and cursor misalignment. Wide (CJK), halfwidth, and zero-width characters are unaffected. |
