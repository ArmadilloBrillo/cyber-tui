# ᑕ¥βєяรקค¢є API v0.8.9

## Access

The API is open to every Cyberspace account. All you need is a **verified email address**.

If your email isn't verified yet, authenticated requests are rejected with `403 EMAIL_NOT_VERIFIED` — log in, call [Resend Verification Email](#resend-verification-email), click the link, and you're in.

Accounts are created on the [website](https://cyberspace.online); there is no signup endpoint.

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
    "rtdbToken": "eyJhb...",
    "rtdbUrl": "https://cyberspace-cyberspace-default-rtdb.europe-west1.firebasedatabase.app"
  }
}
```

- `idToken` -- use as Bearer token for all API requests; also works directly as the `auth` for Realtime Database reads
- `refreshToken` -- use to get a new idToken when it expires
- `rtdbToken` -- optional: a Firebase custom token for SDK-based RTDB access (`signInWithCustomToken`)
- `rtdbUrl` -- the Realtime Database endpoint for direct real-time reads (just a URL, not a secret); see [Reading in real time](#reading-in-real-time)

### Refresh Token

```
POST /v1/auth/refresh
```

```json
{ "refreshToken": "AMf-..." }
```

Returns `{ idToken, rtdbToken, rtdbUrl }`.

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

Rate limit: 10/min, 60/hour (per IP).

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

Guild forum threads appear here only for guilds you belong to, and only while `showGuildPostsInFeed` is on (the default) — see Settings. Threads from other guilds are never returned. A page can come back shorter than `limit` for that reason; keep paginating while `cursor` is non-null.

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
- `attachments` -- optional, max 1. Audio only:
  `{ "type": "audio", "src": "<YouTube URL>", "origin": "youtube", "artist": "...", "title": "...", "genre": "..." }`.
  `artist` (max 100), `title` (max 150) and `genre` (max 50, lowercase) are all required.

**Images are posted on the website.** They travel as inline markdown in `content` --
`![alt](https://bunker.cyberspace.online/...)` -- and the URL has to be one the site is
already hosting, so an image has to be uploaded there first. An image pointing anywhere
else returns `400`, and `"type": "image"` in `attachments` returns `400`. Entries that
already carry an image attachment keep it, and still return it.

Returns `{ "data": { "postId": "...", "slug": "...", "title": "..." } }` (201). The `slug` field reflects the final stored slug, which may differ from what you submitted (collision suffix) or be derived from your content if omitted. `title` is only returned when set.

Rate limit: 2/min, 24/day.

### Edit Entry

```
PATCH /v1/posts/:id
```

```json
{
  "content": "Corrected entry content (markdown)",
  "title": "New Title",
  "topics": ["tag1", "tag2"],
  "isPublic": true,
  "isNSFW": false
}
```

Available to supporters, within **5 minutes** of publishing, on their own entries. Outside that window -- or on an account without it -- the request returns `403`.

Every field is optional; only what you send changes. Send at least one, or you get a `400`.

- `content` -- max 32,768 characters
- `title` -- max 100 characters. Send `""` to remove a title.
- `topics` -- max 3, must be lowercase. Replaces the existing list.
- `isPublic` -- boolean
- `isNSFW` -- boolean
- `attachments` -- replaces the existing attachments. Audio only, same shape as [Create Entry](#create-entry). Send `[]` to remove them.

The `slug` is fixed once an entry is published, so share links keep working; sending one returns `400`. `createdAt` never changes, and an edit sends no notifications -- the people who were notified when you published aren't notified again.

Returns `{ "data": { "postId": "..." } }` (200). The entry then carries an `editedAt` timestamp.

Rate limit: 5/min, 30/day.

### Delete Entry

```
DELETE /v1/posts/:id
```

Deletes the entry. Only the author (or site admin) can delete.

### Flag an Entry

```
POST /v1/posts/:id/flag
```

```json
{ "reason": "why you're reporting it" }
```

Reports the entry for review. `reason` is optional, max 500 characters. Returns `{ "data": { "postId": "...", "flagId": "...", "flagged": true } }` (`201`).

Reporting is idempotent: report the same entry again and you get `200` with `{ "data": { "postId": "...", "flagged": true, "alreadyFlagged": true } }` and no second report is filed — so it's safe to retry. Reports made from the website count too. You can't report your own entry (`403`), and there's no way to withdraw a report.

Rate limit: 5/min, 20/hour, 50/day, shared with the other flag endpoints.

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
- `attachments` -- optional, max 1, audio only. Same shape and same image rule as [Create Entry](#create-entry).

Returns `{ "data": { "replyId": "..." } }` (201).

Posting a reply auto-watches the thread (see Thread Watching) unless you've disabled `autoWatchOnReply` in Settings.

Rate limit: 3/min, 48/day.

### Edit Reply

```
PATCH /v1/replies/:id
```

```json
{ "content": "Corrected reply content (markdown)" }
```

Same permission and window as [Edit Entry](#edit-entry): supporters, within 5 minutes, on their own replies. `content` is the only editable field and is required, max 32,768 characters.

Editing doesn't bump the thread -- the entry's reply count and last-activity time are untouched.

Returns `{ "data": { "replyId": "..." } }` (200). The reply then carries an `editedAt` timestamp.

Rate limit: 5/min, 30/day.

### Delete Reply

```
DELETE /v1/replies/:id
```

Deletes the reply. Only the author (or site admin) can delete.

### Flag a Reply

```
POST /v1/replies/:id/flag
```

```json
{ "reason": "why you're reporting it" }
```

Same as [Flag an Entry](#flag-an-entry). Returns `{ "data": { "replyId": "...", "flagId": "...", "flagged": true } }` (`201`), or `200` with `alreadyFlagged: true` if you've already reported it.

---

## Thread Watching

Watching a thread means you receive `thread_reply` notifications when anyone replies to it. You're auto-watched when you reply to a thread (unless `autoWatchOnReply` is off) or when you're `@mentioned` in an entry; you can also watch/unwatch manually.

### Watch Status

```
GET /v1/posts/:id/watch
```

Returns `{ "data": { "watching": true } }` -- whether you currently watch this thread.

### Watch Thread

```
POST /v1/posts/:id/watch
```

Idempotent. Returns `{ "data": { "watching": true } }` (201).

Rate limit: 10/min, 100/day.

### Unwatch Thread

```
DELETE /v1/posts/:id/watch
```

Returns `{ "data": { "watching": false } }`.

### List Watched Threads

```
GET /v1/watches?limit=20&cursor=<id>
```

Returns your watched threads, newest first:

```json
{
  "data": [
    { "id": "<userId>_<postId>", "postId": "abc123", "createdAt": "..." }
  ],
  "cursor": "<id>"
}
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

A profile's `guildId`, `guildSlug`, `guildIcon` and `guildName` describe the one guild the user is a member of — the badge. Any apprenticeships are in `GET /v1/users/:username/guilds`.

Rate limit: 30/min.

### List a User's Guilds

```
GET /v1/users/me/guilds
GET /v1/users/:username/guilds
```

Every guild the user is in — the guild they're a member of first, then their apprenticeships oldest first. At most six, so there's no pagination and `cursor` is always `null`.

```json
{
  "data": [
    {
      "guildId": "guildId",
      "slug": "night-owls",
      "name": "Night Owls",
      "icon": "🦉",
      "profilePictureUrl": "https://…",
      "role": "member",
      "joinedAt": "2026-03-27T10:12:01.516Z"
    },
    {
      "guildId": "otherGuildId",
      "slug": "deep-divers",
      "name": "Deep Divers",
      "icon": "🐳",
      "role": "apprentice",
      "joinedAt": "2026-05-02T18:44:12.004Z"
    }
  ],
  "cursor": null
}
```

`role` is `"founder"`, `"member"` or `"apprentice"`. 404 for an unknown username.

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
- `locationLatitude` -- number between -90 and 90, or `null` to clear (requires `locationLongitude`)
- `locationLongitude` -- number between -180 and 180, or `null` to clear (requires `locationLatitude`)
- `locationName` -- max 64 characters, or `null` to clear

`websiteImageUrl` -- the 88x31 button on your profile and in the webring -- is set on the
website, which uploads the image for you. It's returned on profiles here but can't be set
here.

Rate limit: 2/min, 15/day.

### Poke a User

```
POST /v1/users/:username/poke
```

No body. Sends the user a `poke` notification -- the same nudge as the **[P] Poke** button on a profile on the web. Returns `{ "data": { "userId": "...", "username": "...", "poked": true } }` (`201`).

- Poking yourself returns `400`.
- Blocked in either direction (you blocked them, or they blocked you) returns `403`.
- Unknown user returns `404`.

Rate limit: 1/hour, 8/day -- across all users, not per user. A rejected poke (`400`/`403`/`404`) doesn't count against it.

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

Guilds are member groups with their own forum of threads. Guilds are identified in the API by their **slug**.

You can be in several guilds at once, in one of two capacities:

- **Your guild** — one at a time, with the role `founder` or `member`. Its name and icon are the guild badge on your profile, and they're the `guildId`/`guildSlug`/`guildIcon`/`guildName` fields on a user object.
- **Apprenticeships** — up to **five** more, with the role `apprentice`. You appear in the guild's member list and get its new-thread notifications, but the badge on your profile stays the one guild you're a member of.

`GET /v1/users/:username/guilds` lists both for any user.

Founding a guild and editing its profile happen on the web, not through the API. The API covers discovery, membership, and the forum.

### List Guilds

```
GET /v1/guilds?limit=20&cursor=<guildId>
```

Returns guilds with at least one member, most populated first. `cursor` is a guild ID.

Ordering is by `memberCount` only, so apprentices don't move a guild up the list.

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
  "apprenticeCount": 7,
  "createdAt": "2026-03-27T10:12:01.516Z"
}
```

- `memberCount` -- founders and members
- `apprenticeCount` -- apprentices. Missing on guilds that predate apprenticeships; read it as 0. Total headcount is `memberCount + apprenticeCount`.

Rate limit: 30/min.

### Get Guild

```
GET /v1/guilds/:slug
```

Returns the guild object plus the caller's membership state: `isMember` (boolean) and `role` (`"founder"`, `"member"`, `"apprentice"`, or `null`). 404 if no guild has that slug.

### List Guild Members

```
GET /v1/guilds/:slug/members?limit=20&cursor=<membershipId>
```

Returns memberships oldest-joined first, enriched with each member's `displayName` and `profilePictureUrl`. `cursor` is a membership ID.

Members and apprentices come back in the same list; group them by `role` if you want them separated the way the website shows them.

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

`role` is `"founder"`, `"member"` or `"apprentice"`.

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

Guild forums are open: any authenticated user can start a thread (membership is not required), matching the web.

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

Rate limit: 2/min, 24/day.

### Join a Guild

```
POST /v1/guilds/:slug/join
```

No body. The API picks your role:

- If you aren't in a guild yet, you join as a `member` and the guild's badge is written to your profile.
- Otherwise you join as an `apprentice`, and your badge stays with the guild you're a member of.

Returns `{ "data": { "guildId": "...", "role": "member" | "apprentice" } }` (201).

- `409` if you're already in this guild
- `409` if you already hold five apprenticeships — leave one first

Rate limit: 3/min, 15/day.

### Change Your Guild Badge

```
POST /v1/guilds/:slug/promote
```

No body. Makes an apprenticeship your guild — its badge replaces the one on your profile, and the guild you were a member of becomes an apprenticeship, so you don't drop out of it. Returns `{ "data": { "guildId": "...", "role": "member" } }`.

- `404` if you aren't in this guild
- `403` if you founded the guild you're currently a member of — leave or hand it over on the web first
- `200` with nothing changed if this is already your guild, with `role` reporting what you already are (`"member"` or `"founder"`)

Rate limit: 3/min, 15/day.

### Leave a Guild

```
POST /v1/guilds/:slug/leave
```

No body. Removes your membership. Returns `{ "data": { "guildId": "..." } }`.

Leaving an apprenticeship doesn't touch your guild badge. Leaving the guild you're a member of clears the badge and promotes nothing — if you want one of your apprenticeships to take its place, call `POST /v1/guilds/:slug/promote` instead.

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

Notification types: `bookmark`, `reply`, `thread_reply`, `new_follower`, `unfollowed`, `new_post_following`, `new_post_friend`, `poke`, `chat_mention`, `post_mention`, `reply_mention`, `graffiti_mention`, `dm_message`, `guild_new_thread`, `supporter_granted`, `supporter_removed`, `hacker_granted`, `hacker_removed`, `moderator_granted`, `moderator_removed`, `api_access_granted`, `api_access_removed`, `image_permission_granted`, `image_permission_removed`, `attachment_permission_granted`, `attachment_permission_removed`, `system_ban`, `system_ban_lifted`, `post_cooldown`, `rate_limit_warning`.

This list excludes notifications you've muted, blocked, or switched off under `notifications` in `GET /v1/settings` — the same set the website shows you. A page can come back shorter than `limit` for that reason; keep paginating while `cursor` is non-null rather than stopping on a short page.

Rate limit: 30/min.

### Notification object

Each notification has this shape:

```json
{
  "id": "notificationId",
  "userId": "recipientUid",
  "type": "reply",
  "actorId": "actorUid",
  "actorUsername": "someone",
  "targetId": "postId",
  "targetType": "post",
  "read": false,
  "createdAt": "2026-06-03T12:00:00.000Z",
  "metadata": { "postSlug": "my-entry", "replyId": "replyId", "authorUsername": "me" }
}
```

- `actorId` / `actorUsername` — who triggered the notification (denormalized so no extra lookup is needed). Both are **optional**: notifications about your own account have no sender. `system_ban` omits them entirely, and `post_cooldown`, `rate_limit_warning` and `system_ban_lifted` carry the literal `"system"` — don't try to open a profile for it.
- `targetType` — `post` or `reply`; `targetId` is the related entry's ID.
- `read` — always `false` on creation.
- `reason` — present only on some system notifications (e.g. `system_ban`).
- `metadata` — type-dependent context. Common keys: `postSlug` and `authorUsername` (build the `/{username}/{slug}` deep link), `replyId` (the relevant reply), `postContent` / `replyContent` (the mention source text), and for guild threads `guildSlug`, `guildName`, `isGuildThread`, `threadId`. `metadata` is open-ended — clients should treat unknown keys as optional.

`guildSlug` / `isGuildThread` here live inside notification `metadata`; the same names also appear as top-level fields on guild-thread **entries** (see Guilds).

### How notifications are generated

The API emits these notifications server-side — clients don't create them:

- `new_follower` — someone follows you.
- `bookmark` — someone bookmarks your entry or reply.
- `reply` — someone replies to your entry.
- `new_post_following` / `new_post_friend` — someone you follow posts a new entry. `new_post_friend` is sent when the follow is **mutual** (you follow each other); `new_post_following` when it's one-way.
- `post_mention` / `reply_mention` — you're `@`-mentioned in an entry or reply. Mentions use the `@username` syntax (case-insensitive). Mentioning a user in an entry also subscribes them to that thread, so they receive `thread_reply` for future replies.
- `thread_reply` — a new reply is posted to a thread you're watching.
- `guild_new_thread` — a new thread is posted in a guild you belong to.
- `poke` — someone pokes you (`POST /v1/users/:username/poke`).

Notifications are never sent to yourself for your own actions, and a user who would otherwise receive several notifications for the same event gets only one (the most specific). Remaining types in the list above are produced by other parts of the platform (DMs, chat, moderation, role/permission changes).

A few concern your own account rather than someone else's action, and arrive with no sender: `post_cooldown` (an entry you wrote was held back and saved as a private note instead), `rate_limit_warning` (you're approaching a posting limit), `system_ban` and `system_ban_lifted` (a restriction was applied or removed), `moderator_granted` / `moderator_removed` and `api_access_granted` / `api_access_removed` (a role changed). Their `reason` field explains what happened.

### Unread Count

```
GET /v1/notifications/unread-count
```

Returns `{ "data": { "count": 7, "exact": true } }` -- the number of unread notifications for the authenticated user.

The count covers the same set `GET /v1/notifications` returns, so a badge built on it matches the list. `exact` is `false` once you have more than 100 unread, where `count` covers only the 100 most recent -- render "99+" instead of the number when that happens.

Cached for 5 seconds. Marking anything read clears the cache, so the count drops immediately.

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

Returns `{ "data": { "updated": 12, "hasMore": false } }` with the count of notifications marked read. Up to 5,000 are marked per call; if `hasMore` is `true`, call it again until it's `false`.

---

## Notes

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

Available fields: `notifications`, `filterNSFW`, `showFollowerCount`, `hideImagesInFeed`, `hideAudioInFeed`, `autoWatchOnReply`, `keyboardBindings`, `keyboardPreset`, `mutedUsersByRoom`, `iconTheme`, `followedTopics`, `mutedTopics`, `imagePixelSize`, `timeDisplayFormat`, `useLegacyMenuOrder`, `defaultPublicPost`, `showGuildPostsInFeed`.

`autoWatchOnReply` (default on) controls whether posting a reply auto-watches that thread (see Thread Watching). Set it to `false` to opt out.

`showGuildPostsInFeed` (default on) controls whether guild forum threads from the guilds you belong to appear in the main feed (`GET /v1/posts`). Set it to `false` to keep the feed free of them.

Rate limit: 2/min, 15/day.

---

## C-Mail

C-Mail is Cyberspace's private 1:1 messaging, stored in Firebase Realtime Database (RTDB). **Sending** goes through this REST API so content is sanitized, rate-limited, and the sender identity is set server-side. **Reading new messages as they arrive** is done by subscribing to the conversation in Realtime Database directly, using your `idToken` — see [Reading in real time](#reading-in-real-time). The `GET` endpoints here are for loading the conversation list and message history.

A conversation is addressed by a `conversationId`. The API derives it server-side from the two participants — you never compute it yourself. Get it from `POST /v1/cmail` (below) or from the conversation list.

### Start / Get a Conversation

```
POST /v1/cmail
```

```json
{ "recipientUsername": "alice" }
```

Provide either `recipientUsername` or `recipientId`. The API returns (and, if needed, creates) the conversation between you and that user. Idempotent — returns the existing conversation if one already exists (`200`), otherwise creates it (`201`).

```json
{ "data": { "conversationId": "...", "otherUser": { "userId": "...", "username": "alice" } } }
```

This is how you **start a new conversation**. To **continue an existing one**, use the `conversationId` from here or from the conversation list.

### List Conversations

```
GET /v1/cmail
```

Returns the caller's conversations, unread first then newest activity first. Each entry: `conversationId`, `otherUser` (`userId`, `username`, and `displayName`/`profilePictureUrl` when set), `lastMessage`, `lastMessageAt` (ms epoch), `unreadCount`.

### Read a Conversation

```
GET /v1/cmail/:conversationId?limit=50&before=<timestamp>
```

Participant only. Use this to load history — the initial screen of messages and scrollback. New messages that arrive while you're connected come from the [real-time subscription](#reading-in-real-time), not from here. Returns up to `limit` (1–100, default 50) messages oldest-first. For older pages, pass `before` = the `cursor` from the previous response (the oldest message's timestamp).

Each message: `id`, `senderId`, `senderUsername`, `content`, `timestamp` (ms epoch), plus the optional attachment and formatting fields described under [Message fields](#message-fields). **`content` can be empty** — an attached image, GIF or song is the whole message. Don't assume every message has text.

### Send a Message

```
POST /v1/cmail/:conversationId
```

```json
{ "content": "hello there" }
```

Sends into an existing conversation (start one first via `POST /v1/cmail`). `senderUsername` is always set from your authenticated account — any value in the body is ignored. Blocked in either direction (you blocked them, or they blocked you) returns `403`. Returns `{ "data": { "conversationId": "...", "messageId": "..." } }` (`201`).

Supports [commands](#commands) — a message whose `content` begins with `/` (e.g. `/me`, `/slap`, `/dice`, `/8ball`, `/fortune`, `/gif`, `/song`, or a text style) is expanded server-side. `/art` and the `/mute` family are cIRC-only and return `400` here.

### Mark as Read

```
POST /v1/cmail/:conversationId/read
```

Resets your unread count for the conversation to `0`.

### Typing Indicator

```
POST /v1/cmail/:conversationId/typing
```

Tells the other participant you're composing a message — they see the same "…is typing" the website shows. No body; your username is set from your authenticated account.

```json
{ "data": { "conversationId": "...", "ok": true, "heartbeatMs": 3000, "staleAfterMs": 9000 } }
```

The flag is deliberately short-lived. Refresh it every `heartbeatMs` while the user is still typing; if you stop refreshing, it clears itself after `staleAfterMs`. That's what keeps a client that quits or crashes mid-sentence from leaving "…is typing" stuck on the other person's screen forever. Read both values off the response rather than hard-coding them.

Sending a message clears your flag automatically, so you don't need to clear it before `POST /v1/cmail/:conversationId`.

```
DELETE /v1/cmail/:conversationId/typing
```

Clears the flag immediately — call it when the input goes idle (the website uses ~2.5 s) or the user closes the conversation, rather than waiting for it to age out. Returns `{ "data": { "conversationId": "...", "ok": true } }`.

```
GET /v1/cmail/:conversationId/typing
```

Whether the *other* participant is typing right now:

```json
{ "data": { "conversationId": "...", "userId": "...", "typing": true, "username": "alice", "since": 1719700000000, "staleAfterMs": 9000 } }
```

This is a polling convenience — for a live indicator, subscribe to the presence node directly (below) instead of hammering this endpoint.

### Reading in real time

New messages are delivered by subscribing to the conversation in Realtime Database directly, using the `idToken` you already have.

Your login `idToken` doubles as the Realtime Database credential — pass it as the `auth` query parameter (it's the same token you send as `Authorization: Bearer`). Reads are scoped to your own `auth.uid`, so you see the conversations you're a participant in.

**The database URL.** Connect to:

```
https://cyberspace-cyberspace-default-rtdb.europe-west1.firebasedatabase.app
```

This is the `rtdbUrl` value also returned from `/v1/auth/login` and `/v1/auth/refresh` (read it from there rather than hard-coding it). It's a plain endpoint URL, not a secret.

**Subscribe to the conversation** over Server-Sent Events. Open this request and keep it open — the connection stays alive and the database streams an event every time the conversation changes:

```
GET https://cyberspace-cyberspace-default-rtdb.europe-west1.firebasedatabase.app/dm_messages/<conversationId>.json?auth=<idToken>&orderBy="timestamp"&limitToLast=50
Accept: text/event-stream
```

(`conversationId` comes from `POST /v1/cmail` or the conversation list; `<idToken>` is your login token.) Subscribe to `user_conversations/<yourUid>` the same way to get live conversation-list and unread updates.

When the `idToken` expires (~1 hour) the stream closes — get a fresh one from `POST /v1/auth/refresh` and reopen the connection.

The stream stays open and emits an event per change. The first event is a `put` with the whole window; each new message is another `put`/`patch`:

```
event: put
data: {"path":"/","data":{"<msgId>":{"senderId":"...","senderUsername":"alice","content":"hi","timestamp":1719700000000}}}

event: put
data: {"path":"/<newMsgId>","data":{"senderId":"...","senderUsername":"alice","content":"you there?","timestamp":1719700050000}}
```

A `data` of `null` means that path was deleted. Merge events into your local view by `path`. Subscribe to `user_conversations/<yourUid>` the same way for live conversation-list and unread updates.

(Prefer a Firebase SDK to raw SSE? The `rtdbToken` from login is a custom token you can pass to `signInWithCustomToken`.)

**Stay within bounds (or get denied):**

For a live typing indicator, subscribe to `dm_presence/<conversationId>.json?auth=<idToken>` the same way. Entries look like `{ "<userId>": { "username": "...", "typing": true, "timestamp": 1719700000000 } }`. Apply the same rule the endpoint does — treat someone as typing only if `typing` is `true` **and** `timestamp` is newer than `staleAfterMs` — and re-check on a timer, since a flag going stale produces no event. Publishing your own still goes through `POST /v1/cmail/:conversationId/typing`.

- You can only read conversations you're a participant in, and your own `user_conversations/<yourUid>` — nothing above those. The database rejects anything broader, so don't try to read the whole tree.
- **Always** include `orderBy="timestamp"` and a `limitToLast` of **100 or fewer**. Page older history with `&endBefore=<timestamp>`. Unbounded reads pull the entire conversation and may be rejected.
- Keep one stream open per conversation; don't reconnect in a loop.

## cIRC

cIRC is Cyberspace's multi-user chat rooms, stored in Firebase Realtime Database (RTDB). It works the same way as [C-Mail](#c-mail): **sending** goes through this REST API so content is sanitized, rate-limited, and your identity is set server-side. **Reading new messages as they arrive** is done by subscribing to the room in Realtime Database directly, using your `idToken` — see [Reading a room in real time](#reading-a-room-in-real-time). The `GET` endpoints here are for loading the room list and message history.

A room is addressed by its `roomId` (its slug, e.g. `general`). Messages are plain text and support [commands](#commands).

### List Rooms

```
GET /v1/circ
```

Returns the rooms available to you, sorted by `sortOrder` then most-recently-active first. Each entry: `id`, `slug`, `name`, `lastMessageAt` (ms epoch), `sortOrder`, `onlineCount` (how many people are in the room right now — see [Who's in a room](#whos-in-a-room)).

### Read a Room

```
GET /v1/circ/:roomId?limit=50&before=<timestamp>
```

Use this to load history — the initial screen of messages and scrollback. New messages that arrive while you're connected come from the [real-time subscription](#reading-a-room-in-real-time), not from here. Returns up to `limit` (1–100, default 50) messages oldest-first. For older pages, pass `before` = the `cursor` from the previous response (the oldest message's timestamp). Returns `403` if the room isn't available to you.

Each message: `id`, `userId`, `username`, `isChatAdmin`, `content`, `timestamp` (ms epoch), plus the optional attachment and formatting fields described under [Message fields](#message-fields). **`content` can be empty** — an attached image, GIF or song is the whole message. Don't assume every message has text.

### Send a Message

```
POST /v1/circ/:roomId
```

```json
{ "content": "hello world" }
```

`username` and `isChatAdmin` are always set from your authenticated account — any values in the body are ignored. Returns `{ "data": { "roomId": "...", "messageId": "..." } }` (`201`). Returns `403` if the room isn't available to you.

Supports [commands](#commands) — a message whose `content` begins with `/` (e.g. `/me`, `/slap`, `/dice`, `/8ball`, `/fortune`, `/gif`, `/song`, or a text style) is expanded server-side.

### Delete Your Message

```
DELETE /v1/circ/:roomId/messages/:messageId
```

Deletes a message you sent. It's a soft delete: the message stays in the room so the conversation around it still reads, but its `content` becomes `[DELETED]`, it comes back with `deleted: true`, and any image, GIF, song, style or command result is stripped. Your name and the original timestamp stay. This is exactly what the website does, and people reading there see the same thing.

Returns `{ "data": { "roomId": "...", "messageId": "...", "deleted": true } }`.

**It can't be undone.** Deleting again returns `409 CONFLICT`; there is no un-delete. You can only delete your own messages — someone else's returns `403`, and an unknown `messageId` returns `404`.

Rate limit: 5/min, 30/day.

### Flag a Message

```
POST /v1/circ/:roomId/messages/:messageId/flag
```

```json
{ "reason": "why you're reporting it" }
```

Reports someone's message for review. `reason` is optional, max 500 characters. The message's text (and any attachment) is recorded with the report, so it survives even if the message is deleted afterwards. Returns `{ "data": { "roomId": "...", "messageId": "...", "flagId": "...", "flagged": true } }` (`201`).

Reporting is idempotent — the same message twice returns `200` with `alreadyFlagged: true` and files nothing new, and reports made from the website count too. You can't report your own message (`403`). A message that's already been deleted can still be reported. There's no way to withdraw a report.

Rate limit: 5/min, 20/hour, 50/day, shared with [Flag an Entry](#flag-an-entry) and [Flag a Reply](#flag-a-reply).

### Mark a Room as Read

```
POST /v1/circ/:roomId/read
```

Marks the room as viewed for you (drives the "new messages" indicator). Returns `{ "data": { "roomId": "...", "ok": true } }`.

### Who's in a room

```
GET /v1/circ/:roomId/users
```

Returns the people currently in the room, sorted by username. Each entry: `userId`, `username`, `isChatAdmin`, `lastSeen` (ms epoch), `lastActivity` (ms epoch, or `null`). Returns `403` if the room isn't available to you.

Presence is heartbeat-based: someone counts as present while they keep announcing themselves. Stop hearing from a client and it drops off the list on its own, so a crashed or force-quit client clears itself without any cleanup call.

`lastActivity` is when that person last did something. Treat anyone whose `lastActivity` is older than `idleAfterMs` as idle — the website shows them with a 💤 next to their name. `null` means their client doesn't report it; treat them as active. Re-evaluate on a timer, since someone going idle produces no update of its own.

### Announce Your Presence

```
POST /v1/circ/:roomId/presence
```

Call this when you enter a room, then repeat it every `heartbeatMs` for as long as you stay. This is what puts you in the room's user list — including for people on the website, who see you alongside everyone else. Skip it and you can still read and send, you're just invisible.

The body is optional. `username` and `isChatAdmin` are set from your authenticated account, so you can only ever publish your own presence.

```json
{ "lastActivity": 1719700000000 }
```

`lastActivity` is when your user last did something — a keystroke, a command, your window regaining focus — as a ms epoch on your own clock. Send it with every heartbeat, plus an extra one the moment they wake up or go quiet. Leave it out and you always read as active. Returns:

```json
{ "data": { "roomId": "general", "ok": true, "heartbeatMs": 30000, "staleAfterMs": 180000, "idleAfterMs": 600000 } }
```

Read the cadence off the response rather than hard-coding it: send a heartbeat every `heartbeatMs`, and you drop out of the room once `staleAfterMs` passes with no heartbeat. Once `idleAfterMs` passes with no `lastActivity` update you show as idle — a 💤 beside your name on the website. Keep heartbeating while your user is idle, or you drop out of the room instead. Returns `403` if the room isn't available to you.

### Leave a Room

```
DELETE /v1/circ/:roomId/presence
```

Removes you from the room's user list immediately. Optional but polite — call it when the user leaves the room or quits your client. Without it you stay listed until `staleAfterMs` elapses. Returns `{ "data": { "roomId": "...", "ok": true } }`.

### Reading a room in real time

New messages are delivered by subscribing to the room in Realtime Database directly — the same mechanism [C-Mail uses](#reading-in-real-time), using the `idToken` you already have. Connect to the `rtdbUrl` returned from `/v1/auth/login` and `/v1/auth/refresh` and open a Server-Sent Events stream:

```
GET https://cyberspace-cyberspace-default-rtdb.europe-west1.firebasedatabase.app/chat_messages/<roomId>.json?auth=<idToken>&orderBy="timestamp"&limitToLast=50
Accept: text/event-stream
```

The first event is a `put` with the whole window; each new message is another `put`/`patch`. A `data` of `null` means that path was deleted. Merge events into your local view by `path`.

Don't only listen for new messages: [deleting a message](#delete-your-message) *changes* an existing one rather than adding one, so it arrives as a `patch` on that message's path (`{ "content": "[DELETED]", "deleted": true }`). Apply it to the message you already have, or the deletion stays invisible to you until you reload the room.

To keep the room's user list live without polling `GET /v1/circ/:roomId/users`, open a second stream on `chat_presence/<roomId>.json?auth=<idToken>`. Entries look like `{ "<userId>": { "username": "...", "isChatAdmin": false, "online": true, "lastSeen": 1719700000000, "lastActivity": 1719699000000 } }`. Apply the same rules the endpoint does: show an entry only if `online` is `true` and `lastSeen` is newer than `staleAfterMs`, and show it as idle if its `lastActivity` is older than `idleAfterMs` (absent means active) — and re-evaluate on a timer, not just on events, since an entry going stale or idle produces no event. Publishing your own presence still goes through `POST /v1/circ/:roomId/presence`.

**Stay within bounds (or get denied):**

- **Always** include `orderBy="timestamp"` and a `limitToLast` of **100 or fewer**. Page older history with `&endBefore=<timestamp>`. Unbounded reads may be rejected.
- Keep one stream open per room; don't reconnect in a loop.
- When the `idToken` expires (~1 hour) the stream closes — get a fresh one from `POST /v1/auth/refresh` and reopen the connection.

---

## Programs

The terminal's program registry — what `publish` and `browse` speak. A program is a file a member wrote on one of the terminals. The gallery lists every published program; the source of a published program is readable by any member. Versions are machine-assigned and the release history is append-only.

Two machines share this registry and they run different program formats, so every program declares a `runtime`:

| `runtime` | Shape | Runs on |
| --- | --- | --- |
| `web` | `export default { name, description, run(ctx, args) }` | the website's `/terminal` |
| `term` | `export default async (p) => number` | the terminal machine |
| `wasm` | a wasm32-wasi binary, stdio only | the terminal machine |

**A program with no `runtime` field is `web`.** Everything published before the field existed came from the website. Ask for the kinds you can run with `?runtime=`; a request without it gets every kind.

### Browse the Gallery

```
GET /v1/programs?limit=30&before=<timestamp>
```

Published programs, newest first. Each entry: `id`, `name`, `ownerUsername`, `description`, `runtime`, `release`, `publishedAt` (ms epoch). `limit` is 1-50 (default 30); for older pages pass `before` = the `cursor` from the previous response.

Keep only the kinds you can run — this applies to every listing form below as well:

```
GET /v1/programs?runtime=web,term
```

A filtered page can come back shorter than `limit`, or empty, and still have a `cursor`. Follow the cursor until it is null rather than stopping at the first short page.

Look one program up by author and name:

```
GET /v1/programs?author=<username>&name=<name>
```

List your own programs — drafts and recalled ones included — with `isPublished`, `takenDown` and `hash` on each row. `hash` is the SHA-256 of the source the current release was published from, so a client can tell an edited working copy from a clean one without fetching the release back:

```
GET /v1/programs?mine=1
```

### Read the Source

```
GET /v1/programs/:id/source
GET /v1/programs/:id/source?release=2
```

The current release's source, or an earlier one by number — release objects are immutable, so an old version is still exactly what went out. Returns `id`, `name`, `ownerUsername`, `description`, `runtime`, `release`, `encoding`, `source`. `encoding` is `utf8` for `web` and `term`, and `base64` for `wasm` — decode before writing the file. You can always read your own; anyone else's only while it is published. Read the source before you run it — publishing is open to every member.

### Publish

```
POST /v1/programs
```

```json
{ "name": "hello", "description": "One line on what it does", "source": "...", "runtime": "term", "note": "optional release note" }
```

Publishes a program under your account, one program per `name` (letters, digits, `. _ -`, max 32 chars). Publishing an existing name releases the next version; publishing unchanged source is a no-op (`unchanged: true`), or puts a recalled program back as the same version (`restored: true`). `description` is required, max 256 characters. Returns `{ "data": { "id": "...", "name": "...", "release": n } }` (`201`).

`runtime` defaults to `web` and is fixed at the first release: republishing a name under a different kind is refused, because everyone holding a copy installed the old one. Publish a `wasm` binary as base64 with `"encoding": "base64"` — the two go together, and the decoded bytes must start with `\0asm`.

Size and count ceilings by tier: members 20 programs of 128 KB each; supporters 100 of 1 MB; staff unlimited count, 10 MB. The ceiling is checked against the decoded bytes, so a 2 MB Go binary needs the staff tier. Programs published here appear in the website terminal's gallery too — it is the same registry, and one count for both.

### Recall

```
DELETE /v1/programs/:id
```

Takes your program out of the gallery. The release history stays, and publishing again resumes from the next version. Returns `{ "data": { "id": "...", "name": "...", "recalled": true } }`.

```
DELETE /v1/programs/:id?purge=1
```

Deletes the record instead, which is what frees the slot it holds against your program count. Irreversible, and refused on a program a moderator has taken down. Copies other members installed are unaffected. Returns `{ "data": { "id": "...", "name": "...", "deleted": true } }`.

## Search

Full-text search across users, entries, and replies.

```
GET /v1/search?q=<query>&type=all
```

Query params:
- `q` -- required search text (1–512 chars)
- `type` -- `all` (default), `posts`, `replies`, or `users`
- `limit` -- 1–50, default 20 (ignored when `type=all`)
- `page` -- 0-based page number (only for a specific `type`)

**`type=all`** returns a grouped preview — up to 8 hits per group, no pagination:

```json
{
  "data": {
    "users":   [ { "type": "user",  "userId": "uid", "username": "someone", ... } ],
    "posts":   [ { "type": "post",  "postId": "abc123", "authorUsername": "someone", "content": "...", ... } ],
    "replies": [ { "type": "reply", "replyId": "r1", "postId": "abc123", "content": "...", ... } ]
  }
}
```

**A specific `type`** returns a paginated list. Each hit carries a `type` field (`post`, `reply`, or `user`):

```
GET /v1/search?q=neon&type=posts&page=0
```

```json
{
  "data": [
    {
      "type": "post",
      "postId": "abc123",
      "authorId": "uid",
      "authorUsername": "someone",
      "content": "markdown content",
      "title": "Optional Title",
      "slug": "optional-title",
      "topics": ["music"],
      "repliesCount": 5,
      "bookmarksCount": 2,
      "createdAt": "2026-03-27T10:12:01.516Z"
    }
  ],
  "cursor": "1"
}
```

`cursor` is the next `page` number (pass it as `?page=`), or `null` on the last page. User hits include `username`, `displayName`, `profilePictureUrl`, `supporterIcon`, guild fields, and follower/post counts; reply hits include `parentPostAuthor`/`parentPostContent` context.

Missing `q` returns `400 VALIDATION_ERROR`. Rate limit: 30/min.

---

## Commands

Both cIRC and C-Mail sends understand IRC-style slash commands. When a message's `content` begins with `/` and matches one of these, the server expands it and stores the result — resolved server-side, so any client gets it for free.

| Command | Effect |
|---------|--------|
| `/me <action>` | Third-person action, e.g. `/me waves` |
| `/poke` `/hug` `/hi5` `/slap` `[@user]` | Emote at a user (or a solo line with no target) |
| `/dice <notation>` | Roll dice: `/dice`, `/dice:SIDES`, `/dice:COUNT:SIDES`, or full notation (`4d6kh3`, `2d6+3`, `adv`, `d%`, `6x4d6kh3`) |
| `/8ball <question>` | Ask the magic 8-ball |
| `/fortune` | A random fortune cookie |
| `/gif <https url>` | Post an animated GIF by link |
| `/song <youtube url> \| <artist> \| <title> [\| <genre>]` | Attach a track to play in the jukebox. Requires supporter status |
| `/art <ascii art>` | Post ASCII art. cIRC only; max 80 columns × 25 lines |
| `/blink` `/l33t` `/comic` `/cursive` `/times` `/rainbow` `/flip` `/quiet` `/slow` `/glitch` `/spoiler` `/wave` `<message>` | Post the message with a text style |
| `/mute <username>` `/unmute <username>` `/muted` `/unmuteall` | Manage who you've muted in this room. cIRC only; posts nothing |
| `/help` | Returns the command list as `{ "data": { "reply": "…" } }`; posts nothing |

Plain text is posted as-is. A malformed command (e.g. bad `/dice` notation, a `/song` that isn't a YouTube link) returns `400 VALIDATION_ERROR`. `/song` without supporter status returns `403 FORBIDDEN`.

`/gif` is checked before the message is posted: the link has to be reachable and actually be a GIF. If it isn't you get a `400` saying why (`That URL returned HTTP 404`, `That URL isn't a GIF`, …) and nothing is posted — the same gate the website applies, so a GIF that posts will render for everyone.

Styles chain with `+` — `/comic+rainbow hello` applies both. Every style combines except `/spoiler`. Only `/img` is website-only, since it needs a file upload; send it and it posts as plain text.

**Posting ASCII art.** Put the art on the lines after the command, and it's stored as-is — leading spaces are preserved, because they're the picture:

```json
{ "content": "/art\n /\\_/\\\n( o.o )\n > ^ <" }
```

The canvas is 80 columns × 25 lines; going over returns a `400` telling you which limit you hit. Art comes back with `style: "art"` and a base64-encoded `content` — see [Message fields](#message-fields).

**Muting.** `/mute`, `/unmute`, `/muted` and `/unmuteall` change *your own* view of a room and post nothing; each returns `{ "data": { "reply": "…" } }`. They're the same mutes as on the website — mute someone from your client and the site honours it too. Nothing is filtered server-side: `GET /v1/circ/:roomId` still returns a muted user's messages, and your client is expected to hide them, which is also what lets an unmute reveal history you've already fetched. The list lives in `mutedUsersByRoom` on [Settings](#settings), so you can also read or edit it wholesale there.

### Message fields

Beyond `content`, a message may carry any of these. They're all optional — a plain text message has none of them.

| Field | Meaning |
|-------|---------|
| `imageUrl` | An attached image. Posted from the website; render it alongside the text |
| `gifUrl` | An attached animated GIF (`/gif`) |
| `audioAttachment` | A jukebox track (`/song`): `{ src, origin: "youtube", artist, title, genre }` |
| `style` | A text style name, or an array of them for chained styles |
| `isAction` | The message is a third-person action (`/me` and the emotes) — conventionally rendered as `* username content` |
| `isDice` | The action was a dice roll |
| `isEightball`, `eightballAnswer` | The action was an 8-ball; the answer on its own for clients that want to highlight it |
| `isFortune`, `fortuneText` | The action was a fortune cookie; likewise |
| `deleted` | cIRC only. The message was deleted by its author — `content` is `[DELETED]` and every field above is gone. Render it as a tombstone rather than as text |

Two things worth handling:

- **`content` may be empty.** An attachment can be the entire message. When a message posted from the website has an attachment and no caption, `content` is sometimes set to the same URL as `imageUrl`/`gifUrl` — skip the text in that case rather than printing the link twice.
- **`style: "art"` means `content` is base64-encoded** ASCII art, not readable text. Decode it before display. It's the one style that changes how `content` should be read; the rest are purely presentational, and it's up to your client to decide what `rainbow` or `blink` looks like — or to ignore them entirely.

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
| `EMAIL_NOT_VERIFIED` | 403 | Account's email address is not verified |
| `NOT_FOUND` | 404 | Resource does not exist |
| `VALIDATION_ERROR` | 400 | Invalid input |
| `CONFLICT` | 409 | Already exists (duplicate follow, taken username) |
| `RATE_LIMITED` | 429 | Too many requests |
| `INTERNAL_ERROR` | 500 | Server error |
| `BAD_GATEWAY` | 502 | Upstream service (e.g. search) unavailable |

## Rate Limits

### Write Actions

| Action | Per Minute | Per Day |
|--------|-----------|---------|
| Entries | 2 | 24 |
| Replies | 3 | 48 |
| Follows | 3 | 15 |
| Unfollows | 3 | 15 |
| Pokes | — | 8 |
| Notes | 3 | 30 |
| Bookmarks | 5 | 75 |
| Program publishes | 3 | 40 |
| Program recalls | 5 | — |
| Guild threads | 2 | 24 |
| Guild join | 3 | 15 |
| Guild leave | 3 | 15 |
| Guild promote | 3 | 15 |
| Profile updates | 2 | 15 |
| Settings updates | 2 | 15 |
| Watch thread | 10 | 100 |
| C-Mail message | 15 | 1000 |
| Start C-Mail conversation | 5 | 50 |
| Mark C-Mail read | 60 | — |
| C-Mail typing on/off | 40 per conversation, 120 overall | — |
| cIRC message | 15 | 1000 |
| Delete a cIRC message | 5 | 30 |
| Mark cIRC room read | 60 | — |
| cIRC presence heartbeat / leave | 15 per room, 90 overall | — |
| Flag an entry, reply or message | 5 | 50 |

Pokes are capped at 1/hour rather than per minute. C-Mail messaging also has an hourly cap (150/hour); starting conversations is capped at 30/hour. cIRC messaging has the same hourly cap (150/hour). Flagging is capped at 20/hour, and the three flag endpoints share one budget between them. Presence is counted per room, and C-Mail typing per conversation.

`POST /v1/auth/resend-verification` is limited separately to 1/min and 5/hour. `POST /v1/auth/check-username` is limited to 10/min and 60/hour per IP.

### Read Actions (Anti-Scraping)

| Endpoint | Per Minute |
|----------|-----------|
| List entries | 45 |
| List replies | 45 |
| List user entries | 45 |
| List user replies | 45 |
| List topic entries | 45 |
| List topics | 30 |
| List bookmarks | 30 |
| List notes | 30 |
| List notifications | 30 |
| Unread notification count | 30 |
| List followers/following | 30 |
| View user profile | 30 |
| List guilds / members / a user's guilds | 30 |
| List guild threads | 45 |
| Watch status / list watched | 30 |
| List C-Mail conversations | 30 |
| Read a C-Mail conversation | 45 |
| Check C-Mail typing | 60 |
| List cIRC rooms | 30 |
| Read a cIRC room | 45 |
| List who's in a cIRC room | 60 |
| Search | 30 |

Exceeding a rate limit returns `429`. Limits use a rolling window (24 hours for daily, 60 seconds for per-minute).

## Content Limits

| Field | Max Length |
|-------|-----------|
| Entry/reply/note content | 32,768 chars |
| Entry title | 100 chars |
| Entry slug | 60 chars, `[a-z0-9-]` |
| cIRC / C-Mail message | 2,048 chars |
| Flag reason | 500 chars |
| Search query | 512 chars |
| Bio | 640 chars |
| Display name | 64 chars |
| Website URL | 2,048 chars |
| Website name | 64 chars |
| Location name | 64 chars |
| Topics per entry | 3 |
| Username | 3-20 chars |

## TypeScript definitions

Every response shape on this page is available as a TypeScript definition file at
[`/types.d.ts`](/types.d.ts). Save it next to your code and import from it:

```ts
import type { ApiList, ApiSingle, Entry, OwnUser } from './cyberspace-api-types'

const res = await fetch('https://api.cyberspace.online/v1/posts', {
  headers: { Authorization: `Bearer ${idToken}` },
})
const feed: ApiList<Entry> = await res.json()
```

Timestamps are ISO 8601 strings everywhere except cIRC and C-Mail, where they're
millisecond epoch numbers.
