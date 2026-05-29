# ᑕ¥βєяรקค¢є API v0.4

## Access

To use the API your account must either:

- have API access explicitly granted on it, or
- be a Cyberspace **supporter** account.

Without one of these, all authenticated requests will be rejected.

## Terms

By using this API you agree that you will not:

- **Scrape** the API — bulk-collect posts, replies, profiles, or any other content for redistribution, archival, or analysis outside the intended use of a personal client.
- Run **bots** — automated accounts that post, reply, follow, react, or otherwise act without a human driving each action in real time.
- Use the API to feed **AI systems** — no training, fine-tuning, embedding, or evaluation of language models on Cyberspace content; no LLM-driven agents that read or write through the API on your behalf.

Cyberspace is a small, human social network. Accounts that violate these terms will be banned and their content removed. If you're building a personal client (TUI, mobile, desktop) that a real user drives, you're fine — that's exactly what the API is for.

## Authentication

All endpoints except auth routes require a Bearer token:

```
Authorization: Bearer <idToken>
```

### Login

```
POST /v1/auth/login
```

```json
{ "email": "you@example.com", "password": "your_password" }
```

Returns:

```json
{
  "data": {
    "idToken": "eyJhb...",
    "refreshToken": "AMf-...",
    "rtdbToken": "eyJhb..."
  }
}
```

- `idToken` -- use as Bearer token for all API requests
- `refreshToken` -- use to get a new idToken when it expires
- `rtdbToken` -- use to connect to Realtime Database for chat/DMs

### Register

```
POST /v1/auth/register
```

```json
{ "email": "you@example.com", "password": "your_password", "username": "your_username" }
```

Username rules:
- 3-20 characters
- Lowercase letters, numbers, underscores only
- Cannot be a reserved name (admin, system, etc.)
- Cannot contain prohibited words

Returns the same token structure as login (201).

### Refresh Token

```
POST /v1/auth/refresh
```

```json
{ "refreshToken": "AMf-..." }
```

Returns `{ idToken, rtdbToken }`.

### Resend Verification Email

```
POST /v1/auth/resend-verification
```

```json
{ "idToken": "eyJhb..." }
```

Returns `{ "data": { "sent": true } }`.

Rate limit: 1/min, 5/hour.

### Check Username Availability

```
POST /v1/auth/check-username
```

```json
{ "username": "desired_name" }
```

Returns:

```json
{ "data": { "available": true } }
```

or

```json
{ "data": { "available": false, "reason": "Username is already taken" } }
```

No authentication required.

---

## Entries

### List Entries (Feed)

```
GET /v1/posts?limit=20&cursor=<postId>
```

Query params:
- `limit` -- 1-50, default 20
- `cursor` -- entry ID to start after (for pagination)

To list a specific user's entries, use `GET /v1/users/:username/posts` instead.

Returns:

```json
{
  "data": [
    {
      "postId": "abc123",
      "authorId": "uid",
      "authorUsername": "someone",
      "content": "markdown content",
      "title": "Optional Title",
      "slug": "optional-title",
      "topics": ["music", "linux"],
      "repliesCount": 5,
      "bookmarksCount": 2,
      "isPublic": false,
      "isNSFW": false,
      "attachments": [],
      "createdAt": "2026-03-27T10:12:01.516Z",
      "deleted": false
    }
  ],
  "cursor": "xyz789"
}
```

Pass `cursor` from the response to get the next page. `cursor` is `null` when there are no more results.

### Get Entry by ID

```
GET /v1/posts/:id
```

For per-author slug lookup, use `GET /v1/users/:username/posts/:slug`.

### Get Entry by Slug

```
GET /v1/users/:username/posts/:slug
```

Resolves an entry by its per-author URL slug. Returns the same shape as `GET /v1/posts/:id`. 404 if no entry exists for that `(username, slug)` pair.

### Create Entry

```
POST /v1/posts
```

```json
{
  "content": "Your entry content (markdown)",
  "title": "Optional Title",
  "slug": "optional-slug",
  "topics": ["tag1", "tag2"],
  "isPublic": false,
  "isNSFW": false
}
```

- `content` -- required, max 32,768 characters
- `title` -- optional, free-form, max 100 characters
- `slug` -- optional, lowercase `[a-z0-9-]`, max 60 characters, unique per author. If omitted, one is generated server-side from the title, content, or attachments. If the slug is already taken by another of your posts, `-2`, `-3`, … is appended automatically. Reserved slugs (`blog`, `jukebox`, `public`, `replies`, `index`, `edit`, `new`, `admin`, anything starting with `_`) are rejected.
- `topics` -- optional, max 3, must be lowercase
- `isPublic` -- optional, makes entry visible without login
- `isNSFW` -- optional, content warning flag

Returns `{ "data": { "postId": "...", "slug": "...", "title": "..." } }` (201). The `slug` field reflects the final stored slug, which may differ from what you submitted (collision suffix) or be derived from your content if omitted. `title` is only returned when set.

Rate limit: 2/min, 15/day.

### Delete Entry

```
DELETE /v1/posts/:id
```

Deletes the entry. Only the author (or site admin) can delete.

---

## Replies

### List Replies for an Entry

```
GET /v1/posts/:postId/replies?limit=20&cursor=<replyId>
```

Replies are ordered oldest first.

### Get Reply

```
GET /v1/replies/:id
```

### Create Reply

```
POST /v1/replies
```

```json
{
  "postId": "abc123",
  "content": "Your reply (markdown)",
  "parentReplyId": "def456"
}
```

- `content` -- required, max 32,768 characters
- `postId` -- required, must reference an existing entry
- `parentReplyId` -- optional, ID of the reply you're responding to (must belong to the same entry)

Returns `{ "data": { "replyId": "..." } }` (201).

Rate limit: 3/min, 15/day.

### Delete Reply

```
DELETE /v1/replies/:id
```

Deletes the reply. Only the author (or site admin) can delete.

---

## Users

### Get Own Profile

```
GET /v1/users/me
```

### Get User Profile

```
GET /v1/users/:username
```

Rate limit: 30/min.

### List User's Entries

```
GET /v1/users/:username/posts?limit=20&cursor=<postId>
```

Returns paginated entries by the specified user, newest first.

Rate limit: 45/min.

### Get User's Entry by Slug

```
GET /v1/users/:username/posts/:slug
```

Returns a single entry matching the per-author slug. Same response shape as `GET /v1/posts/:id`. 404 if no entry exists for that `(username, slug)` pair.

Rate limit: 45/min.

### List User's Replies

```
GET /v1/users/:username/replies?limit=20&cursor=<replyId>
```

Returns paginated replies by the specified user, newest first.

Rate limit: 45/min.

### Update Own Profile

```
PATCH /v1/users/me
```

```json
{
  "bio": "New bio text",
  "pinnedPostId": "abc123",
  "displayName": "Display Name",
  "websiteUrl": "https://example.com",
  "websiteName": "My Website",
  "websiteImageUrl": "https://example.com/button.png",
  "locationLatitude": 51.5074,
  "locationLongitude": -0.1278,
  "locationName": "London, UK"
}
```

- `bio` -- max 640 characters, or `null` to clear
- `pinnedPostId` -- entry ID to pin, or `null` to unpin (must be your own entry)
- `displayName` -- max 64 characters, or `null` to clear
- `websiteUrl` -- must start with `http://` or `https://`, max 2048 characters, or `null` to clear
- `websiteName` -- max 64 characters, or `null` to clear
- `websiteImageUrl` -- must start with `http://` or `https://`, max 2048 characters, or `null` to clear
- `locationLatitude` -- number between -90 and 90, or `null` to clear (requires `locationLongitude`)
- `locationLongitude` -- number between -180 and 180, or `null` to clear (requires `locationLatitude`)
- `locationName` -- max 64 characters, or `null` to clear

Rate limit: 2/min, 15/day.

---

## Bookmarks

### List Bookmarks

```
GET /v1/bookmarks?limit=20&cursor=<bookmarkId>
```

Rate limit: 30/min.

### Create Bookmark

```
POST /v1/bookmarks
```

```json
{ "postId": "abc123", "type": "post" }
```

or

```json
{ "replyId": "def456", "type": "reply" }
```

Rate limit: 5/min, 75/day.

### Remove Bookmark

```
DELETE /v1/bookmarks/:id
```

---

## Follows

### List Followers or Following

```
GET /v1/follows?type=followers&limit=20&cursor=<followId>
GET /v1/follows?type=following&limit=20&cursor=<followId>
```

- `type` -- required, `"followers"` or `"following"`
- `userId` -- optional, look up another user's followers/following (defaults to your own)
- `limit` -- 1-50, default 20
- `cursor` -- follow ID for pagination

Rate limit: 30/min.

### Follow a User

```
POST /v1/follows
```

```json
{ "followedId": "user_id_to_follow" }
```

Rate limit: 3/min, 15/day.

### Unfollow

```
DELETE /v1/follows/:id
```

`:id` is the follow document ID returned when you followed.

Rate limit: 3/min, 15/day.

---

## Guilds

Guilds are member groups with their own forum of threads. A user can belong to **one guild at a time**. Guilds are identified in the API by their **slug**.

Founding a guild and editing its profile happen on the web, not through the API. The API covers discovery, membership, and the forum.

### List Guilds

```
GET /v1/guilds?limit=20&cursor=<guildId>
```

Returns guilds with at least one member, most populated first. `cursor` is a guild ID.

Each guild object:

```json
{
  "id": "guildId",
  "name": "Night Owls",
  "slug": "night-owls",
  "founderId": "uid",
  "founderUsername": "someone",
  "icon": "🦉",
  "profilePictureUrl": "https://…",
  "bio": "We never sleep",
  "link": "https://…",
  "linkText": "our site",
  "memberCount": 42,
  "createdAt": "2026-03-27T10:12:01.516Z"
}
```

Rate limit: 30/min.

### Get Guild

```
GET /v1/guilds/:slug
```

Returns the guild object plus the caller's membership state: `isMember` (boolean) and `role` (`"founder"`, `"member"`, or `null`). 404 if no guild has that slug.

### List Guild Members

```
GET /v1/guilds/:slug/members?limit=20&cursor=<membershipId>
```

Returns memberships oldest-joined first, enriched with each member's `displayName` and `profilePictureUrl`. Banned and shadow-banned members are omitted. `cursor` is a membership ID.

```json
{
  "data": [
    {
      "membershipId": "guildId_uid",
      "guildId": "guildId",
      "guildSlug": "night-owls",
      "userId": "uid",
      "username": "someone",
      "role": "member",
      "joinedAt": "2026-03-27T10:12:01.516Z",
      "displayName": "Some One",
      "profilePictureUrl": "https://…"
    }
  ],
  "cursor": null
}
```

Rate limit: 30/min.

### List Guild Threads

```
GET /v1/guilds/:slug/posts?limit=20&cursor=<postId>
```

Returns the guild's threads, most recently active first. Threads are entries (same shape as `GET /v1/posts/:id`) carrying `guildId`, `guildSlug`, and `isGuildThread: true`. `cursor` is a post ID.

Rate limit: 45/min.

### Create Guild Thread

```
POST /v1/guilds/:slug/posts
```

You must be a member of the guild (otherwise `403`).

```json
{
  "content": "Thread body (markdown)",
  "title": "Optional Title",
  "slug": "optional-slug",
  "topics": ["tag1", "tag2"]
}
```

- `content` -- required, max 32,768 characters
- `title` -- optional, max 100 characters
- `slug` -- optional; same rules and auto-generation as `POST /v1/posts`
- `topics` -- optional, max 3, lowercase

Returns `{ "data": { "postId": "...", "slug": "...", "title": "..." } }` (201).

**Replying to a thread** uses the normal `POST /v1/replies` with the thread's `postId` — a guild thread is an ordinary entry. Replies posted to a guild thread inherit its `guildId`, and posting a reply bumps the thread's activity so it rises in the thread list.

Rate limit: 2/min, 15/day.

### Join a Guild

```
POST /v1/guilds/:slug/join
```

No body. Joins the guild as a `member`. Returns `{ "data": { "guildId": "...", "role": "member" } }` (201).

A user can only be in one guild. If you're already in another guild, this returns `409` — leave your current guild first.

Rate limit: 3/min, 15/day.

### Leave a Guild

```
POST /v1/guilds/:slug/leave
```

No body. Removes your membership and clears your guild fields. Returns `{ "data": { "guildId": "..." } }`.

Founders cannot leave through the API (`403`) — manage the guild on the web. `404` if you aren't a member.

Rate limit: 3/min, 15/day.

---

## Notifications

### List Notifications

```
GET /v1/notifications?limit=20&cursor=<notificationId>&read=false&type=reply,reply_mention
```

Query params:
- `limit` (1-50, default 20), `cursor` -- standard pagination
- `read` -- `true` or `false` to filter by read status. Omit for all.
- `type` -- comma-separated list of notification types (1-20 values). Omit for all.

Notification types: `bookmark`, `reply`, `thread_reply`, `new_follower`, `unfollowed`, `new_post_following`, `new_post_friend`, `poke`, `chat_mention`, `post_mention`, `reply_mention`, `dm_message`, `guild_new_thread`, `supporter_granted`, `supporter_removed`, `hacker_granted`, `hacker_removed`, `image_permission_granted`, `image_permission_removed`, `attachment_permission_granted`, `attachment_permission_removed`, `system_ban`.

Rate limit: 30/min.

### Unread Count

```
GET /v1/notifications/unread-count
```

Returns `{ "data": { "count": 7 } }` -- the number of unread notifications for the authenticated user.

Cached for 5 seconds. The count is raw and may include notifications whose actor has since been banned or shadow-banned (those are filtered out of `GET /v1/notifications` but not from this count).

### Mark as Read

```
PATCH /v1/notifications/:id
```

No body needed -- marks the notification as read.

### Mark All as Read

```
POST /v1/notifications/read-all
```

No body needed. Marks all unread notifications as read.

Returns `{ "data": { "updated": 12 } }` with the count of notifications marked read.

---

## Notes (Private)

Notes are private to you. No other user can see them.

Notes support **revisions** — editing a note creates a new revision rather than overwriting the original. The API returns the latest revision by default.

### List Notes

```
GET /v1/notes?limit=20&cursor=<cursor>
```

Returns the latest revision of each note. Rate limit: 30/min.

### Get Note

```
GET /v1/notes/:id
GET /v1/notes/:id?revision=2
```

Returns the latest revision by default. Pass `?revision=N` to retrieve a specific revision number.

### List Revisions

```
GET /v1/notes/:id/revisions?limit=20&cursor=<cursor>
```

Returns all revisions for a note, newest first (by revision number).

### Create Note

```
POST /v1/notes
```

```json
{
  "content": "Private note content",
  "topics": ["journal"]
}
```

- `content` -- required, max 32,768 characters
- `topics` -- optional, max 3, lowercase

Rate limit: 3/min, 30/day.

### Update Note

```
PATCH /v1/notes/:id
```

```json
{
  "content": "Updated content",
  "topics": ["updated"]
}
```

Creates a new revision. The previous content is preserved and accessible via the revisions endpoint.

### Delete Note

```
DELETE /v1/notes/:id
```

Soft-deletes all revisions of the note.

---

## Topics

### List All Topics

```
GET /v1/topics
```

Returns all topics sorted by entry count (most popular first).

Rate limit: 30/min.

### List Entries by Topic

```
GET /v1/topics/:slug/posts?limit=20&cursor=<postId>
```

`:slug` is the topic name in lowercase (e.g., `music`, `linux`).

Rate limit: 45/min.

---

## Settings

### Get Settings

```
GET /v1/settings
```

### Update Settings

```
PATCH /v1/settings
```

```json
{
  "notifications": {
    "bookmark": true,
    "reply": true,
    "poke": false
  },
  "filterNSFW": true,
  "autoWatchOnReply": true
}
```

Available fields: `notifications`, `filterNSFW`, `showFollowerCount`, `hideImagesInFeed`, `hideAudioInFeed`, `autoWatchOnReply`, `keyboardBindings`, `keyboardPreset`, `mutedUsersByRoom`, `iconTheme`, `followedTopics`, `mutedTopics`, `imagePixelSize`, `timeDisplayFormat`, `useLegacyMenuOrder`, `defaultPublicPost`.

Rate limit: 2/min, 15/day.

---

## Chat & DMs (Realtime Database)

Chat (cIRC) and direct messages (C-Mail) use Firebase Realtime Database, not this REST API. The `rtdbToken` returned from login grants access.

Full RTDB documentation (endpoints, pagination, presence, and rate limits) is coming soon.

---

## Response Format

All responses follow this structure:

```json
{ "data": { ... } }
```

```json
{ "data": [ ... ], "cursor": "next_page_id" }
```

```json
{ "error": { "code": "VALIDATION_ERROR", "message": "Content cannot be empty" } }
```

## Error Codes

| Code | HTTP | Meaning |
|------|------|---------|
| `UNAUTHORIZED` | 401 | Missing or invalid token |
| `FORBIDDEN` | 403 | Not allowed to perform this action |
| `BANNED` | 403 | Account is banned |
| `NOT_FOUND` | 404 | Resource does not exist |
| `VALIDATION_ERROR` | 400 | Invalid input |
| `CONFLICT` | 409 | Already exists (duplicate follow, taken username) |
| `RATE_LIMITED` | 429 | Too many requests |
| `INTERNAL_ERROR` | 500 | Server error |

## Rate Limits

### Write Actions

| Action | Per Minute | Per Day |
|--------|-----------|---------|
| Entries | 2 | 10 |
| Replies | 3 | 10 |
| Follows | 3 | 10 |
| Unfollows | 3 | 10 |
| Notes | 3 | 20 |
| Bookmarks | 5 | 50 |
| Guild threads | 2 | 15 |
| Guild join | 3 | 15 |
| Guild leave | 3 | 15 |
| Profile updates | 2 | 10 |
| Settings updates | 2 | 10 |

### Read Actions (Anti-Scraping)

| Endpoint | Per Minute |
|----------|-----------|
| List entries | 30 |
| List replies | 30 |
| List user entries | 30 |
| List user replies | 30 |
| List topic entries | 30 |
| List topics | 20 |
| List bookmarks | 20 |
| List notes | 20 |
| List notifications | 30 |
| Unread notification count | 30 |
| List followers/following | 20 |
| View user profile | 20 |
| List guilds / members | 30 |
| List guild threads | 45 |

Exceeding a rate limit returns `429`. Limits use a rolling window (24 hours for daily, 60 seconds for per-minute).

## Content Limits

| Field | Max Length |
|-------|-----------|
| Entry/reply/note content | 32,768 chars |
| Entry title | 100 chars |
| Entry slug | 60 chars, `[a-z0-9-]` |
| Chat/DM message | 2,048 chars |
| Bio | 640 chars |
| Display name | 64 chars |
| Website URL | 2,048 chars |
| Website name | 64 chars |
| Location name | 64 chars |
| Topics per entry | 3 |
| Username | 3-20 chars |
