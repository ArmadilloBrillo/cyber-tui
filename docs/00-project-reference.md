# cyber-tui Project Reference

Comprehensive map of every module, file, and artifact in this repository. Use this as the starting point when navigating or extending the codebase.

---

## Overview

**cyber-tui** is a terminal user interface (TUI) client for [cyberspace.online](https://cyberspace.online) — a retro text-only social network. It is written in Go, using [Bubble Tea](https://github.com/charmbracelet/bubbletea) for the TUI event loop, [Lip Gloss](https://github.com/charmbracelet/lipgloss) for styling, and [Wish](https://github.com/charmbracelet/wish) to optionally host the client over SSH.

The client talks to the cyberspace.online REST API (current target v0.7; see CLAUDE.md) and to Firebase Realtime Database (RTDB) for live direct messages and public chatrooms (SSE). See `docs/00-latest-api-reference.md` for the current API spec snapshot.

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
│   ├── sanitize/
│   │   ├── sanitize.go          # Strip control chars from untrusted server strings
│   │   └── sanitize_test.go     # Sanitizer tests
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
│       │   ├── reconnect.go     # Shared RTDB reconnect backoff/retry helpers (cmail.go + chatrooms.go)
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
| `Tokens` | IDToken, RefreshToken, RTDBToken, RTDBUrl returned from login |
| `User` | Profile (ID, username, displayName, email, bio, websiteUrl, websiteName, websiteImageUrl, pinnedPostID, locationName, locationLatitude, locationLongitude) |
| `Post` | Feed item (ID, authorID, authorUsername, content, title, slug, guildID, guildSlug, isGuildThread, topics, repliesCount, bookmarksCount, isPublic, isNSFW, deleted, createdAt) |
| `Watch` | Thread-watch record (ID, PostID, CreatedAt) — returned by GET /v1/watches |
| `Reply` | Comment on a post (ID, postID, authorID, authorUsername, content, parentReplyID, createdAt) |
| `ProfileUpdate` | Optional fields for PATCH /v1/users/me (all pointer types, includes new website/location fields) |
| `Message` | DM/chat message (ID, from, body, createdAt) |
| `Conversation` | 1-on-1 DM thread (ID, participants, messages, UnreadCount, LastMessage) |
| `Room` | Public chatroom (ID, slug, name, lastMessageAt, sortOrder) |
| `NotificationPrefs` | Notification subscription toggles (bookmark, reply, poke) |
| `Settings` | All user preferences (notifications, content filters, display options) |
| `Notification` | Alert event (ID, type, read status, actor, targetID, targetType, replyID, threadAuthorUsername, guildName, postSlug, postAuthorUsername, postContent, replyContent) |
| `Bookmark` | Saved post or reply (ID, type, postID/replyID, content snapshot, author, createdAt) |
| `Topic` | Tag with post count (slug, postCount) |
| `Guild` | Guild community (ID, name, slug, icon, bio, memberCount, founderUsername, createdAt, isMember, role, link, linkText, profilePictureUrl) |
| `GuildMember` | Guild membership record (membershipID, guildID, guildSlug, userID, username, role, joinedAt, displayName, profilePictureUrl) |
| `Follow` | Follow relationship (ID, followerID, followedID, followerUsername, followedUsername, createdAt) |
| `Note` | Private journal note (ID, authorID, content, topics, revisionNumber, deleted, createdAt) |
| `NoteRevision` | Single historical revision of a note (revisionNumber, content, topics, createdAt) |
| `SearchPreview` | Grouped `type=all` search response (Users, Posts, Replies — up to 8 each, reusing the existing `User`/`Post`/`Reply` types) |

---

### `internal/api`

Implements all API operations. The UI layer depends only on the `Client` interface, never on a concrete type.

#### `interface.go`

Defines the `Client` interface — the only type the UI layer imports from this package.

**Method groups:**

| Group | Methods |
|---|---|
| Auth | `Login(email, password)`, `LoginWithRefreshToken(token)`, `Logout()` |
| Feed | `GetFeed(cursor)`, `CreatePost(content, title, slug, topics, isPublic, isNSFW)`, `GetPost(postID)`, `DeletePost(postID)` |
| Thread watching | `GetWatches(cursor)`, `WatchPost(postID)`, `UnwatchPost(postID)` |
| Replies | `GetPostReplies(postID)`, `GetReply(replyID)`, `CreateReply(postID, content, parentReplyID)`, `DeleteReply(replyID)` |
| Profile | `GetOwnProfile()`, `GetProfile(username)`, `UpdateProfile(update)` |
| User History | `GetUserPosts(username, cursor)`, `GetUserReplies(username, cursor)` |
| Follows | `GetFollowing(cursor)`, `GetFollowers(cursor)`, `GetUserFollows(userID, followType, cursor)`, `Follow(followedID)`, `Unfollow(followID)` |
| Settings | `GetSettings()`, `UpdateSettings(update)` |
| Rooms | `GetRooms()`, `GetRoomMessages(roomID, limit)`, `SendRoomMessage(roomID, body)` |
| Notifications | `GetNotifications(cursor, unreadOnly, types)`, `GetUnreadNotificationCount()`, `MarkNotificationRead(id)`, `MarkAllNotificationsRead()` |
| Bookmarks | `GetBookmarks(cursor)`, `CreateBookmark(postID, replyID)`, `DeleteBookmark(id)` |
| Topics | `GetTopics(cursor)`, `GetTopicPosts(slug, cursor)` |
| Guilds | `GetGuilds(cursor)`, `GetGuild(slug)`, `GetGuildPosts(slug, cursor)`, `CreateGuildPost(slug, content, title, postSlug, topics)`, `GetGuildMembers(slug, cursor)`, `JoinGuild(slug)`, `LeaveGuild(slug)` |
| Notes | `GetNotes(cursor)`, `GetNote(noteID)`, `GetNoteRevision(noteID, revision)`, `GetNoteRevisions(noteID, cursor)`, `CreateNote(content, topics)`, `UpdateNote(noteID, content, topics)`, `DeleteNote(noteID)` |
| Direct Messages | `GetConversations()`, `GetMessages(convID, limit)`, `SendMessage(convID, body)`, `StartConversation(recipientUsername)`, `MarkCMailRead(convID)`, `SubscribeDMs(ctx, convID) <-chan model.Message` |
| Chatrooms | `GetRooms()`, `GetRoomMessages(roomID, limit, before)`, `SendRoomMessage(roomID, body)`, `MarkRoomRead(roomID)`, `SubscribeRoom(ctx, roomID) <-chan model.Message` |
| Search | `Search(query)` (grouped `type=all` preview), `SearchPosts(query, cursor)`, `SearchReplies(query, cursor)`, `SearchUsers(query, cursor)` |

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

**Wire types (unexported):** `loginRequest`, `loginResponseData`, `wirePost`, `wireUser`, `wireReply`, `wireNotification`, `wireSettings`, `wireNote`, `wireBookmark`, `wireTopic`, `wireFollow`, `wireRoom`, `wireCircMessage`, `wireCMailConversation`, `wireCMailMessage`, `wireRTDBMessage`, `wireRTDBSSEData`, `wireSearchPreview` — match the JSON envelope shapes returned by the API.

**Note:** `HTTPClient.tokens` is guarded by a `sync.Mutex`. Bubble Tea runs commands in concurrent goroutines, so reads in `doRequest` and writes in `Login`/`refresh` go through the token accessor methods (`idToken`, `setTokens`, `snapshotTokens`, `applyRefresh`). See `docs/30-security-hardening.md`.

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
| `Client` | struct | RTDB HTTP client (baseURL, token, idle timeout, httpClient, streamClient) |
| `SSEEvent` | struct | Single SSE event (Event string, Data []byte, Err error). `Event` is `"auth_revoked"`/`"cancel"` on a terminal server event, always paired with `Err` set. |
| `New(baseURL, token)` | func | Production constructor (15s timeout for REST; SSE stream has no overall timeout but a 30s `ResponseHeaderTimeout` on connect) |
| `NewForTesting(baseURL, token, hc)` | func | Test constructor with injected `http.Client` |
| `Get(ctx, path, params)` | method | One-shot GET, returns raw JSON bytes |
| `Put(ctx, path, val)` | method | Marshals val to JSON and PUTs it |
| `Subscribe(ctx, path, params)` | method | Opens SSE stream; returns `<-chan SSEEvent`, closed (with a terminal `Err`) on cancel, server close, `auth_revoked`/`cancel`, or a 10-minute idle-read timeout |
| `SetIdleTimeoutForTesting(d)` | method | Test-only override of the idle-read watchdog duration (default 10 min) |

Internal: `readSSE(ctx, cancel, reader, ch)` parses the SSE wire format, forwards events, and races a per-line idle timer against `ctx.Done()` — any line (including a discarded `:`-comment) resets the timer; `buildURL(path, params)` constructs authenticated URLs under `mu.RLock()`; `SetToken(token)` replaces the auth token under `mu.Lock()` (called by `HTTPClient.applyRefresh` on token refresh) — this only affects *future* connections, since an open SSE stream's token is fixed in its URL at connect time and can't be revived without reconnecting.

#### `client_test.go`

Tests for RTDB REST operations, SSE parsing (including `auth_revoked`/`cancel` terminal events and the idle-read watchdog), and `SetToken` token-update behaviour.

---

### `internal/ssh`

#### `server.go`

SSH server hosting via Wish (Charmbracelet).

`Serve(addr, hostKeyPath, newClient func() api.Client)`:

1. Creates a Wish server with Bubble Tea middleware
2. Each SSH connection gets a fresh `ui.App` built with its own `api.Client` from `newClient`, marked ephemeral via `WithEphemeralSession` so it never reads or writes the host config
3. Listens for SIGTERM/Interrupt and shuts down gracefully with a 5-second timeout; `ListenAndServe` errors are surfaced

SSH mode is experimental and unauthenticated; startup warns accordingly. The SSH and local modes otherwise share all TUI code. See `docs/30-security-hardening.md`.

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

**Screen enum values:** `screenLogin`, `screenFeed`, `screenChatrooms`, `screenCMail`, `screenProfile`, `screenPostDetail`, `screenNotifications`, `screenBookmarks`, `screenGuilds`, `screenTopics`, `screenJournal`, `screenSearch`, `screenSettings`

**Responsibilities:**

- Routes Bubble Tea messages to the active screen or handles them at the app level
- Dispatches API calls in response to screen-emitted messages (`SubmitLoginMsg`, `LoadMoreFeedMsg`, `SubmitNewPostMsg`, etc.)
- Manages screen transitions (e.g., `ShowPostMsg` → navigate to PostDetail)
- Broadcasts `SharedConfigMsg` to all screens on resize, theme change, or settings update
- Renders tab bar, active screen, and status bar
- Manages global shortcuts (`1`–`9` screen jump, `v` density toggle, `?` help, `t` theme picker, `z` timezone picker, `o` URL opener, `q`/`ctrl+c` quit)
- Handles automatic token refresh on `ErrUnauthorized` responses
- Surfaces transient errors via a **global notification banner** that replaces the status-bar row, colored by severity, and auto-dismisses after 4 s or on the next keypress (which still performs its normal action)
- Runs background tick jobs: `schedulePollCmd` (60 s unread count), `scheduleWanderCmd` (1 h wander check)

**Error handling — errors never block a screen:**

- **Load failures** (a fetch that populates a screen returns an error) wrap the error in `errMsg`; `handleErr` fires the transient global banner (text via `friendlyErr`) **and** sets the active screen's `err`. That `err` only feeds a subtle inline "couldn't load …" empty-state — no `View()` collapses to a full-screen error. Every screen clears `err` on the next fetch (`SetFetching`) and on each success setter, so a load error can never trap the user.
- **Post-open from Notifications** uses `notifPostLoadErrMsg` instead of `errMsg`, so a deleted target (404) shows a friendly "This post has been deleted" banner; 401 still redirects to login. The notification payload has no deleted-target field, so the 404 on open is the only signal (API v0.4.1).
- **Action failures** (create/reply/delete/follow/save/submit) wrap the error in `actionErrMsg`; `handleNotify` shows it as the transient global banner without blanking the screen, so the tab stays usable. The banner is driven by `notifyText`/`notifyLevel`/`notifyGen` on `App`; `notifyExpireMsg` carries a generation id so a stale auto-dismiss tick cannot clear a newer notification. `notifyMsg` and the `notify(level, text)` helper allow surfacing info/warning messages directly. See `docs/31-global-notifications.md`.

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
| Content | filterNSFW |
| Social | showFollowerCount, autoWatchOnReply, defaultPublicPost |
| Display | timeDisplayFormat (enum) |

**Deferred fields** (read from API, never patched): `iconTheme`, `imagePixelSize`, `followedTopics`, `mutedTopics`

- `j`/`k` navigate; Space/Enter toggle booleans or cycle enum options
- ctrl+s emits `SaveSettingsMsg` with the updated settings; ESC discards
- Unsaved changes are highlighted

Key types: `SettingsModel`, `SaveSettingsMsg`

#### `cmail.go`

Direct messages (C-Mail) with live Firebase RTDB integration.

- Two-mode flow: `cmailModeList` (full-width conversation cards) → `cmailModeDetail` (history viewport + compose input); ESC returns to list
- Subscribes to RTDB SSE stream via `api.Client.SubscribeDMs()` on conversation selection; cancelled on ESC or screen switch
- `waitForDM(sub)` is a Bubble Tea `Cmd` that blocks on the subscription channel and returns each incoming message as a `tea.Msg`
- Other person's messages left-aligned; my messages right-aligned (driven by `currentUser` field)
- `j`/`k` navigate conversation list in list mode; Enter opens detail mode; Enter sends a message; `↑`/`↓` scroll history in detail mode
- **Starting a conversation:** pressing `c` on any highlighted post, reply, notification, or read-only profile emits `StartConversationMsg{Username}` (defined in `messages.go`); App calls `StartConversation(username)` via REST, then switches to C-Mail and opens the returned conversation in detail mode; self-DMs are dropped in the App handler. Distinct from `g m` (see "Keyboard Shortcuts" → Global): `c` targets the specific highlighted user, `g m` just opens the C-Mail tab's conversation list — the in-app hint for `c` reads "message" rather than "c-mail" to keep the two from being conflated
- **Scroll-to-load history:** reaching the top of the loaded messages via `↑` fetches the next older page (`GetMessages(convID, 50, before)`, `before` = oldest loaded message's timestamp) and prepends it via `PrependMessages`, preserving scroll offset; guarded by `loadingHistory`/`historyExhausted` fields, reset on conversation open. A failed fetch resets `loadingHistory` (so a retry is possible) and sets `err`, which `renderMessages()` surfaces as "couldn't load messages" when the list is still empty.
- **Live-stream reconnect:** when `dmStreamClosedMsg` fires for the still-active conversation (idToken expiry, an idle-read timeout, a terminal `auth_revoked`/`cancel` event, or a network error — see `internal/rtdb`), `reconnectConvCmd` calls `api.Client.RefreshSession()` then re-subscribes; on failure it retries with backoff (`reconnectDelay`/`reconnectBackoffSchedule` in `reconnect.go`, shared with `chatrooms.go`: `1s, 2s, 4s, 8s, 15s`, 6 attempts total) via `scheduleReconnectRetryCmd`/`tea.Tick`, tracked by `m.reconnecting`/`m.reconnectAttempt`/`m.reconnectFailed`. Success emits `CMailReconnectedMsg`, which App turns into a "reconnected to live chat" toast, and clears the retry state. While retrying, `View()` shows `(live updates lost, reconnecting… N/6)` in the header; once attempts are exhausted, `(live updates lost)` persists until the user leaves and re-enters — independent of `renderMessages()`'s empty-list error path, so it's visible even with history already loaded. `cancelDMSub()` (called on navigation away) cancels any in-flight retry sequence via `m.reconnectCancel`. A stale close event (from an abandoned conversation) is a no-op.
- **Slash commands:** normal commands (`/me`, `/dice`, etc.) are expanded server-side; `/help` posts nothing, so `SendMessage`'s reply text is routed through app.go's `cmailCommandReplyMsg` into `AppendSystemMessage`, which injects a local-only `model.Message{IsSystem: true}` rendered via `renderSystemNotice`. `/me`-style messages carry an undocumented `isAction` field (`model.Message.IsAction`, confirmed live for CIRC, parsed defensively here) rendered via `renderActionLine` as classic IRC `* username body *`

Key types: `CMailModel`, `cmailMode` (`cmailModeList` / `cmailModeDetail`), `CMailConvSelectedMsg` (emitted on Enter; App calls `MarkCMailRead`), `SendCMailMsg`, `StartConversationMsg`, `CMailReconnectedMsg`
Key internal types: `dmSubscription` (RTDB channel + cancel func + `ConvID`), `dmSubscribedMsg`, `dmReceivedMsg`, `dmStreamClosedMsg`, `dmReconnectedMsg`, `dmReconnectFailedMsg`, `dmReconnectRetryDueMsg`, `cmailMsgsLoadedMsg`, `cmailOlderMsgsLoadedMsg`

#### `chatrooms.go`

Public chatroom browser and chat — CIRC (tab `4`, key `4`). Full API integration including live RTDB SSE.

- Two-mode flow: `chatroomModeList` (full-width room cards) → `chatroomModeDetail` (header + message viewport + compose input); ESC returns to list
- Room cards show name, `#slug` subtitle, and last-message timestamp (right-aligned)
- IRC-style message rendering via `renderCircMessages` (see `render.go`): `<username>  body` with right-aligned timestamp; long bodies word-wrap to the viewport width, with room reserved so the timestamp trails the last wrapped line instead of overflowing
- Subscribes to RTDB SSE stream via `api.Client.SubscribeRoom()` on room selection; cancelled on ESC or screen switch
- `waitForRoomMsg(sub)` is the Bubble Tea Cmd pump (mirrors `waitForDM` in `cmail.go`)
- `RoomOpenedMsg` is emitted on Enter; App calls `MarkRoomRead` fire-and-forget
- `CancelSubscription()` is called by the layout on every key that navigates away
- **Scroll-to-load history:** reaching the top of the loaded messages via `↑` fetches the next older page (`GetRoomMessages(roomID, 50, before)`, `before` = oldest loaded message's timestamp) and prepends it via `PrependMessages`, preserving scroll offset; guarded by `loadingHistory`/`historyExhausted` fields, reset on room open. A failed fetch resets `loadingHistory` (so a retry is possible) and sets `err`, which `renderMessages()` surfaces as "couldn't load messages" when the list is still empty.
- **Live-stream reconnect:** when `roomStreamClosedMsg` fires for the still-active room (idToken expiry, an idle-read timeout, a terminal `auth_revoked`/`cancel` event, or a network error — see `internal/rtdb`), `reconnectRoomCmd` calls `api.Client.RefreshSession()` then re-subscribes; on failure it retries with backoff (shared `reconnectDelay`/`reconnectBackoffSchedule` from `reconnect.go`: `1s, 2s, 4s, 8s, 15s`, 6 attempts total) via `scheduleRoomReconnectRetryCmd`/`tea.Tick`, tracked by `m.reconnecting`/`m.reconnectAttempt`/`m.reconnectFailed`. Success emits `RoomReconnectedMsg`, which App turns into a "reconnected to live chat" toast, and clears the retry state. While retrying, `View()` shows `(live updates lost, reconnecting… N/6)` in the header; once attempts are exhausted, `(live updates lost)` persists until the user leaves and re-enters — independent of `renderMessages()`'s empty-list error path. `cancelRoomSub()` (called on navigation away) cancels any in-flight retry sequence via `m.reconnectCancel`. A stale close event (from an abandoned room) is a no-op.
- **Admin badge:** `renderCircMessages` shows a `[admin]` tag next to the username when `model.Message.IsChatAdmin` is set (parsed from both the REST and RTDB wire formats)
- **Slash commands:** normal commands (`/me`, `/dice`, etc.) are expanded server-side; `/help` posts nothing, so `SendRoomMessage`'s reply text is routed through app.go's `roomCommandReplyMsg` into `AppendSystemMessage`, which injects a local-only `model.Message{IsSystem: true}` rendered via `renderSystemNotice` (shared with `cmail.go`). `/me`-style messages carry an undocumented `isAction` field (`model.Message.IsAction`, confirmed live) rendered via `renderActionLine` as classic IRC `* username body *` — see `docs/33-circ.md` for the live-testing findings

Key types: `ChatroomsModel`, `chatroomMode` (`chatroomModeList` / `chatroomModeDetail`), `SendRoomMessageMsg`, `RoomOpenedMsg`, `RoomReconnectedMsg`
Key internal types: `roomSubscription` (RTDB channel + cancel func + `RoomID`), `roomSubscribedMsg`, `roomReceivedMsg`, `roomStreamClosedMsg`, `roomReconnectedMsg`, `roomReconnectFailedMsg`, `roomReconnectRetryDueMsg`, `circMsgsLoadedMsg`, `circOlderMsgsLoadedMsg`
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
- Thread feed shows a membership hint bar (`J` to join, `l` to leave) with y/n confirmation; `GetGuild` is fetched alongside the thread list to populate membership state
- Member list is cursor-paginated oldest-joined first; `enter` navigates to the member's profile, `esc` returns to the thread feed

Key types: `GuildsModel`, `LoadMoreGuildsMsg`, `LoadGuildPostsMsg`, `LoadMoreGuildPostsMsg`, `LoadGuildMembersMsg`, `LoadMoreGuildMembersMsg`, `JoinGuildMsg`, `LeaveGuildMsg`  
Key methods: `SetGuilds`, `AppendGuilds`, `SetGuildPosts`, `AppendGuildPosts`, `SetGuildMembers`, `AppendGuildMembers`, `SetGuildDetail`, `BackToGuildList`  
Key accessors: `IsBrowsingGuild()`, `IsBrowsingMembers()`, `IsDetailLoaded()`, `GuildDetail()`, `IsConfirmingJoin()`, `IsConfirmingLeave()`

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

#### `search.go`

Full-text search across users, posts, and replies — tab `search` (no number key; reached via the global `/` shortcut or `←`/`→` cycling). See `docs/34-search.md`.

- Three modes: query (text input) → preview (grouped, up to 8 hits per category) → type-list (one category, fully paginated). Results stay cached across mode changes.
- `esc` peels back one level at a time — type-list → preview → query (focused) — and, from the query box (the outermost level; no result list is showing there, so there's nothing left to peel back), leaves Search entirely and returns to the screen `/` was pressed from (`App.searchReturn`, the same return-to-origin pattern as `profileReturn`/`postDetailReturn`). Also the escape hatch if a search request fails: `SetError` never changes the view, so this is otherwise the only way back to normal tab/quit navigation. `esc` always blurs the query box on its way out so a later arrival via tab-cycling (which doesn't call `FocusQuery`) never inherits a stuck focused state.
- `j`/`k` navigate a flattened row list (section headers are not selectable); `enter` opens a hit or drills into a "see all" row.
- User hits emit the shared `ShowUserProfileMsg`; post hits emit `ShowSearchPostMsg`; reply hits emit `ShowSearchReplyMsg{PostID, ReplyID}` (fetches the parent post, then scrolls to the reply — same mechanism as the Notifications reply deep-link).

Key types: `SearchModel`, `SubmitSearchMsg`, `DrillSearchTypeMsg`, `LoadMoreSearchMsg`, `ShowSearchPostMsg`, `ShowSearchReplyMsg`, `LeaveSearchMsg`  
Key methods: `SetPreview(preview, query)`, `SetTypeResults(hitType, posts, replies, users, cursor)`, `AppendTypeResults(...)`, `FocusQuery()`, `InputFocused()`, `IsInTypeList()`, `LastQuery()`

#### `compose.go`

Reusable multi-line text editor embedded in Feed, PostDetail, Profile, and C-Mail.

- Built on `bubbles/textarea`
- Enter inserts a paragraph break (`\n\n`); ctrl+s emits `ComposeSubmitMsg`; ESC emits `ComposeCancelMsg`
- Auto-expands from `composeMinLines=3` to `composeMaxLines=8` as content grows
- Character limit and placeholder text are configurable per embedding screen
- Active/inactive border styling (cyan when focused, dimmed otherwise)

`PostComposePanel` is a unified single-box compose panel for new posts (title, optional slug, body, topics, public/nsfw). Tab cycles through all fields. The `slug` field accepts `[a-z0-9-]` up to 60 chars; an invalid value blocks submit, focuses the slug field, and shows a red inline error. Empty slug is silently omitted from the wire (server generates one).

Key types: `ComposeModel`, `ComposeSubmitMsg` (Content), `ComposeCancelMsg`, `PostComposePanel`  
Key methods (`ComposeModel`): `Open(ctx, placeholder)`, `OpenWithContent(ctx, placeholder, content)`, `SetCharLimit(n)`, `SetWidth(w)`, `IsActive()`, `Content()`, `Close()`  
Key methods (`PostComposePanel`): `Open(defaultPublic)`, `Close()`, `TitleValue()`, `SlugValue()`, `TopicsRaw()`, `IsPublic()`, `IsNSFW()`, `PanelHeight()`, `SetWidth(w)`  
Key functions: `ValidateSlug(s string) error`

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
| `allowInsecureApi` | bool | `false` | Permit a plain `http://` `apiBaseURL` to a non-loopback host |
| `useMock` | bool | `false` | Use `MockClient` instead of real API |
| `debug` | bool | `false` | Verbose RTDB / HTTP output |
| `autoEmail` | string | — | Pre-fill email on login screen |
| `autoPassword` | string | — | Pre-fill password (plain text; not recommended) |
| `sshListenAddr` | string | — | Enable SSH server mode (e.g. `":2222"`) |
| `sshHostKeyPath` | string | — | Path to SSH host private key |
| `allowRemoteSsh` | bool | `false` | Permit `sshListenAddr` to bind a non-loopback address. SSH server mode is unauthenticated, so this is off by default |
| `wanderLust` | bool | `false` | Wander mode toggle; `true` = on, `false` = off |
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

Two navigation schemes reach the same 11 screens — pick whichever is faster
for a given target. Both are derived from the single `menuTabs` slice in
`internal/ui/layout.go`, so TabsLayout and MillerLayout can never disagree
about what a given key does (a bug that existed before this scheme and was
fixed alongside it).

**Numeric aliases** — quick access to the first 9 screens by position on the
tab bar:

| Key | Action |
|---|---|
| `1` | Feed |
| `2` | Notifications |
| `3` | C-Mail (direct messages) |
| `4` | CIRC (public chatrooms) |
| `5` | Journal (private notes) |
| `6` | Bookmarks |
| `7` | Guilds |
| `8` | Topics |
| `9` | Profile |

**Leader key** — `g` ("go to") arms a pending state; the very next keypress
resolves against a mnemonic map to jump directly to any of the 11 screens,
including Search and Settings which have no numeric alias. An unmapped
follow-up key silently cancels the pending state rather than doing anything.
The mnemonic letter is shown highlighted inline within each tab's label on
the tab bar / nav sidebar as a hint:

| Chord | Screen | Chord | Screen |
|---|---|---|---|
| `g f` | Feed | `g g` | Guilds |
| `g n` | Notifications | `g t` | Topics |
| `g m` | C-Mail | `g p` | Profile |
| `g i` | CIRC | `g s` | Search |
| `g j` | Journal | `g e` | Settings |
| `g b` | Bookmarks | | |

Only plain letters are used — no `alt+`, function keys, or `ctrl+`/`shift+`
combinations — since those are caught inconsistently by terminal emulators,
window managers, or (for `ctrl+`/`shift+` on non-letter keys) aren't
reliably distinguishable without the Kitty keyboard protocol. `g` isn't
bound to anything on its own, so there's no ambiguity or timeout to manage.

**Search is hidden from the tab bar / nav sidebar and from `←`/`→` (and
MillerLayout's `j`/`k`) cycling** — `menuTabs`' `search` entry has
`hidden: true` (`internal/ui/layout.go`), so `visibleTabs()` (what actually
renders and what cycling iterates over) skips it, the same treatment
`screenPostDetail` already had. It's an explicit-entry-only destination:
the only ways in are `g s` and `/`, and both always land in a focused query
box (`SearchModel.FocusQuery()`) regardless of whatever state Search was
last left in — unlike cycling, which for every other screen intentionally
just resumes wherever it was left. `screenForMnemonic` and the help modal's
leader-key legend still list `g s` normally; only the rendered tab
list/cycling exclude it.

Other global keys:

| Key | Action |
|---|---|
| `←` / `→` | Cycle tabs left / right (does not include Search, which is hidden — see above) |
| `/` | Search — jumps to the Search screen with the query box focused. No-op while any screen's compose input is focused (so `/dice`, `/me`, etc. still type normally in CIRC/C-Mail) |
| `v` | Toggle dense / relaxed display |
| `?` | Help modal |
| `t` | Theme picker |
| `o` | Open URLs/images from the focused item (direct-open if one, picker if several) — no-op while any screen's compose input is focused |
| `ctrl+o` | Same as `o`, but reaches the handler even while a compose input is focused — the only way to open links in CIRC/C-Mail, since their input is focused for the entire detail view, not just a transient compose sub-mode |
| `q` / `ctrl+c` | Quit |

Timezone is set from the Settings screen's own field (`tab`/`shift+tab` to
cycle), not a global shortcut.

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
| `c` | Start C-Mail conversation (read-only profiles only) |
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
| `c` | Start C-Mail conversation with post author |
| `w` | Watch / unwatch the selected thread |

### Post Detail

| Key | Action |
|---|---|
| `j` / `↓` | Scroll down / next reply |
| `k` / `↑` | Scroll up / previous reply |
| `r` | Reply to selected post or reply |
| `d` | Delete selected post or reply (own content only — prompts y/n) |
| `p` | View author's profile |
| `c` | Start C-Mail conversation with focused author |
| `w` | Watch / unwatch the thread (root post focused only; no-op on replies) |
| `esc` | Back to feed |

### Compose

**Reply/inline compose** (`ComposeModel` — replies, guild/topic threads,
CIRC/C-Mail messages): `enter` inserts a paragraph break (`\n\n`) rather than
submitting — most terminals can't distinguish `shift+enter` from `enter`
without the Kitty keyboard protocol, so there's no separate hard-line-break
key.

| Key | Action |
|---|---|
| `enter` | Insert paragraph break |
| `ctrl+s` | Submit |
| `esc` | Cancel |

**New post panel** (`PostComposePanel` — Feed's `n`, guild/topic new-thread):
a single panel with title, slug, body, topics, and public/NSFW checkbox
fields.

| Key | Action |
|---|---|
| `tab` / `shift+tab` | Cycle fields: title → slug → body → topics → public → NSFW |
| `space` | Toggle the focused checkbox field (public / NSFW) |
| `ctrl+s` | Submit (validates the slug field first; invalid slug refocuses it with an inline error) |
| `esc` | Cancel |

### Notifications

| Key | Action |
|---|---|
| `j` / `↓` | Next notification |
| `k` / `↑` | Previous (at top → refresh) |
| `enter` | Jump to referenced post/reply |
| `m` | Mark selected as read |
| `M` | Mark all as read |
| `u` | Toggle unread-only filter |
| `c` | Start C-Mail conversation with notification actor |

### C-Mail

**List mode**

| Key | Action |
|---|---|
| `j` / `↓` | Next conversation |
| `k` / `↑` | Previous conversation |
| `enter` | Open conversation (detail mode) |

**Detail mode**

| Key | Action |
|---|---|
| `↑` | Scroll message history up |
| `↓` | Scroll message history down |
| `enter` | Send message (when input non-empty) |
| `esc` | Return to list mode |
| `ctrl+o` | Open URLs/images found across the loaded conversation (plain `o` is captured by the compose input) |
| all other | Forwarded to compose input (`j`/`k` type normally) |

**From other screens**

| Key | Action |
|---|---|
| `c` | Start or open C-Mail conversation with highlighted user (feed, post detail, notifications, read-only profile) — self-DM is a no-op |

### CIRC

**List mode**

| Key | Action |
|---|---|
| `j` / `↓` | Next room |
| `k` / `↑` | Previous room |
| `enter` | Open room (detail mode) |

**Detail mode**

| Key | Action |
|---|---|
| `↑` | Scroll message history up (reaching the top loads older history) |
| `↓` | Scroll message history down |
| `enter` | Send message (when input non-empty) |
| `esc` | Return to list mode |
| `ctrl+o` | Open URLs/images found across the loaded room history (plain `o` is captured by the compose input) |
| all other | Forwarded to compose input |

### Search

`esc` peels back one level at a time, same as everywhere else in the app, and — once there's nothing left to peel back — leaves Search entirely and returns to whichever screen `/` was pressed from (the same return-to-origin pattern `p` → profile → `esc` uses):

type-list → preview → query (focused) → **origin screen**

| Key | Action |
|---|---|
| `j` / `↓` | Next row (skips section headers) |
| `k` / `↑` | Previous row |
| `enter` | Submit the query (in query mode); open the selected hit or drill into a "see all" row (in preview/type-list) |
| `esc` | Step back one level per the chain above |

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
| **HTTPClient thread safety** | Resolved: `tokens` is guarded by a `sync.Mutex` (see `docs/30-security-hardening.md`) |
| **Settings — deferred fields** | `iconTheme`, `imagePixelSize`, `followedTopics`, `mutedTopics` are read from the API but intentionally excluded from PATCH until the server-side feature is finalized |
| **Journal write operations** | Fully operational. `PATCH /v1/notes/:id` was fixed server-side in API v0.4. |
| **Post/reply deletion** | Wired and working — `d` key in Feed (own posts) and Post Detail (own posts and replies) |
| **Attachments** | Image and YouTube audio attachments on posts/replies are not supported in the TUI |
| **Note revision pagination** | `GetNoteRevisions` cursor is implemented in the API client but the UI loads only the first page |
| **Profile navigation depth** | Navigating from a Following/Followers tab to another user's profile is single-level; ESC returns to the original `profileReturn` destination, not the intermediate profile |
| **Feed position — deep pagination** | When returning to the Feed tab, the selected post is restored by ID from the fresh first-page load. If the post was reached via pagination it will not be in page 1 and the feed falls back to the top. Fix options: re-fetch pages sequentially until found (expensive), or skip the tab-switch reload (stale data). Neither is warranted for typical usage. |
| **Ambiguous-width character stripping** | Unicode EAW = "A" characters (kaomoji symbols, `©`, `®`, `™`, Greek letters, etc.) are stripped at two points: (1) `stripAmbiguousRunes` in `shared.go` strips them from post/reply content before display; (2) `filterAmbiguousKeyMsg` in `shared.go` intercepts `tea.KeyRunes` messages before they reach any `textarea` or `textinput` component (compose, topics, profile fields, C-Mail, chatrooms). Their column width is undefined and varies by terminal/font, causing border overflow and cursor misalignment. Wide (CJK), halfwidth, and zero-width characters are unaffected. |

---

## Code & Security Reviews

Periodic full-repository audits, tracked as dated snapshots so successive reviews can be diffed against each other. Findings are actioned separately (see each report's Actionable Recommendations table); this doc's Deferred/Known Limitations table above tracks accepted long-term gaps.

| Date | Commit | Report |
|---|---|---|
| 2026-07-24 | `8835173` | [`docs/reviews/2026-07-24-code-security-review.md`](reviews/2026-07-24-code-security-review.md) |
