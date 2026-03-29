# 03 — API Reference (v0.2 snapshot)

> **Source:** https://api.cyberspace.online/docs.md
> **Snapshot date:** 2026-03-29
> **Status:** Beta — subject to change. Always fetch the live URL before implementing new API features.

---

## Gaps vs current scaffold

| Area | Current scaffold | Reality |
|---|---|---|
| Login field | `username` + `password` | `email` + `password` |
| Chat / DMs | REST methods in `api.Client` | Firebase Realtime Database (SSE streams) — not REST |
| Pagination | `GetFeed(page int)` | Cursor-based: `cursor` param + `cursor` in response |
| `model.Post.Body` | `Body string` | `content string` (markdown) |
| `model.User` | Minimal | Missing: `displayName`, `email`, `websiteUrl`, `pinnedPostId`, `locationName` |
| Replies | Not modelled | Full `Reply` type needed |
| Features missing | — | Bookmarks, Follows, Notifications, Notes, Topics, Settings |

These gaps will be addressed in a dedicated `feature/api-alignment` branch before HTTPClient is implemented.

---

# ᑕ¥βєяรקค¢є API v0.2

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

Returns `{ "data": { "available": true } }` or `{ "data": { "available": false, "reason": "..." } }`.

No authentication required.

---

## Posts

### List Posts (Feed)

```
GET /v1/posts?limit=20&cursor=<postId>
```

- `limit` -- 1-50, default 20
- `cursor` -- post ID to start after (pagination)

Returns array of posts + `cursor` (null when no more results).

Post shape:
```json
{
  "postId": "abc123",
  "authorId": "uid",
  "authorUsername": "someone",
  "content": "markdown content",
  "topics": ["music", "linux"],
  "repliesCount": 5,
  "bookmarksCount": 2,
  "isPublic": false,
  "isNSFW": false,
  "attachments": [],
  "createdAt": "2026-03-27T10:12:01.516Z",
  "deleted": false
}
```

### Get Post

```
GET /v1/posts/:id
```

### Create Post

```
POST /v1/posts
```

```json
{
  "content": "Your post content (markdown)",
  "topics": ["tag1", "tag2"],
  "isPublic": false,
  "isNSFW": false,
  "attachments": []
}
```

- `content` -- required, max 32,768 chars
- `topics` -- optional, max 3, lowercase
- `attachments` -- optional, max 1

Returns `{ "data": { "postId": "..." } }` (201). Rate limit: 2/min, 10/day.

### Delete Post

```
DELETE /v1/posts/:id
```

---

## Replies

### List Replies for a Post

```
GET /v1/posts/:postId/replies?limit=20&cursor=<replyId>
```

Ordered oldest first.

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

Returns `{ "data": { "replyId": "..." } }` (201). Rate limit: 3/min, 10/day.

### Delete Reply

```
DELETE /v1/replies/:id
```

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

### List User's Posts

```
GET /v1/users/:username/posts?limit=20&cursor=<postId>
```

### List User's Replies

```
GET /v1/users/:username/replies?limit=20&cursor=<replyId>
```

### Update Own Profile

```
PATCH /v1/users/me
```

Fields: `bio` (max 127), `displayName` (max 64), `pinnedPostId`, `websiteUrl`, `websiteName`, `websiteImageUrl`, `locationLatitude`, `locationLongitude`, `locationName`.

Rate limit: 2/min, 10/day.

---

## Bookmarks

```
GET  /v1/bookmarks
POST /v1/bookmarks       { "postId": "...", "type": "post" }
DELETE /v1/bookmarks/:id
```

---

## Follows

```
GET    /v1/follows?type=followers|following
POST   /v1/follows       { "followedId": "..." }
DELETE /v1/follows/:id
```

---

## Notifications

```
GET  /v1/notifications
PATCH /v1/notifications/:id        (mark read)
POST /v1/notifications/read-all    (mark all read)
```

---

## Notes (Private)

```
GET    /v1/notes
GET    /v1/notes/:id
POST   /v1/notes    { "content": "...", "topics": [] }
PATCH  /v1/notes/:id
DELETE /v1/notes/:id
```

---

## Topics

```
GET /v1/topics
GET /v1/topics/:slug/posts
```

---

## Settings

```
GET   /v1/settings
PATCH /v1/settings
```

---

## Chat & DMs (Firebase Realtime Database)

Chat and DMs do **not** go through the REST API. Use `rtdbToken` from login.

### Chat rooms (cIRC)

```
# Stream messages
GET https://<project>.firebaseio.com/chat_messages/<roomSlug>.json?auth=<rtdbToken>&orderBy="timestamp"
Accept: text/event-stream

# Send message
PUT https://<project>.firebaseio.com/chat_messages/<roomSlug>/<msgId>.json?auth=<rtdbToken>
{ "authorId": "...", "username": "...", "content": "...", "timestamp": { ".sv": "timestamp" } }
```

### Direct Messages (C-Mail)

```
# Stream messages
GET https://<project>.firebaseio.com/dm_messages/<conversationId>.json?auth=<rtdbToken>&orderBy="timestamp"
Accept: text/event-stream

# Send DM
PUT https://<project>.firebaseio.com/dm_messages/<conversationId>/<msgId>.json?auth=<rtdbToken>
{ "senderId": "...", "senderUsername": "...", "content": "...", "timestamp": { ".sv": "timestamp" }, "read": false }
```

Max message length: 2,048 characters.

---

## Response Format

```json
{ "data": { ... } }
{ "data": [ ... ], "cursor": "next_page_id" }
{ "error": { "code": "VALIDATION_ERROR", "message": "..." } }
```

## Error Codes

| Code | HTTP | Meaning |
|---|---|---|
| `UNAUTHORIZED` | 401 | Missing or invalid token |
| `FORBIDDEN` | 403 | Not allowed |
| `BANNED` | 403 | Account banned |
| `NOT_FOUND` | 404 | Resource does not exist |
| `VALIDATION_ERROR` | 400 | Invalid input |
| `CONFLICT` | 409 | Already exists |
| `RATE_LIMITED` | 429 | Too many requests |
| `INTERNAL_ERROR` | 500 | Server error |

## Rate Limits

| Action | Per Minute | Per Day |
|---|---|---|
| Posts | 2 | 10 |
| Replies | 3 | 10 |
| Follows/Unfollows | 3 | 10 |
| Notes | 3 | 20 |
| Bookmarks | 5 | 50 |
| Profile/Settings updates | 2 | 10 |

Read endpoints: 20–30 requests/min depending on endpoint.

## Content Limits

| Field | Max |
|---|---|
| Post/reply/note content | 32,768 chars |
| Chat/DM message | 2,048 chars |
| Bio | 127 chars |
| Display name | 64 chars |
| Topics per post | 3 |
| Username | 3–20 chars |
| Attachments per post/reply | 1 |
