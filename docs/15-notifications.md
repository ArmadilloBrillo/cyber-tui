# 15 — Notifications

## Overview

A full notifications screen accessible from the menu bar at tab position 2, between Feed and Profile.

---

## UI

### Tab bar

The notifications tab shows an unread count badge when there are unread notifications:

```
feed   notifications (3)   profile
```

The count is maintained by:
- Optimistic in-memory updates (mark read, mark all) when the screen is loaded.
- A background poll every 60 seconds that fetches the first page of notifications and counts unread items when the screen is not yet loaded.

Above 100 unread, `GET /v1/notifications/unread-count` caps `count` at 100 and reports `exact: false` (v0.8.5+). The badge shows `99+` instead of a specific number in that case (`App.polledUnreadCountExact`, formatted by `notifBadgeText` in `internal/ui/app.go`).

### Layout

Each notification renders as a single line inside a bordered box:

```
↩ @molly_millions replied to your post.                         5m ago
★ @wintermute published something.                             20m ago
```

- Leading symbol is type-specific (see table below); highlighted when unread, subtle when read.
- Actor username rendered in highlight colour.
- Right-aligned relative timestamp.
- Selected notification uses the active border style.

### Type icons

| Type | Icon |
|------|------|
| `reply` / `thread_reply` | `↩` |
| `reply_mention` / `post_mention` | `@` |
| `new_post_friend` / `new_post_following` | `★` |
| `bookmark` | `♥` |
| `new_follower` | `+` |
| `unfollowed` | `☹` |
| `guild_new_thread` | `#` |
| `poke` | `~` |
| `chat_mention` | `»` |
| `dm_message` | `✉` |
| `supporter_granted` / `supporter_removed` | `$` |
| `hacker_granted` / `hacker_removed` | `^` |
| `image_permission_*` / `attachment_permission_*` | `%` |
| `system_ban` | `☠` |
| `graffiti_mention` | `@` |
| `moderator_granted` / `moderator_removed` | `!` |
| `api_access_granted` / `api_access_removed` | `/` |
| `system_ban_lifted` | `✓` |
| `post_cooldown` | `⏱` |
| `rate_limit_warning` | `⚠` |

### Day separators

Notifications are grouped by calendar day in the user's configured timezone:

```
  ── today ──
● @molly_millions replied to your post.    5m ago
  ── yesterday ──
  @wintermute saved your entry.           3h ago
```

### Relative timestamps

| Age | Format |
|-----|--------|
| < 1 min | `just now` |
| < 1 hour | `5m ago` |
| < 24 hours | `2h ago` |
| < 7 days | `3d ago` |
| ≥ 7 days | `02-Jan` |

---

## Key Bindings

| Key | Action |
|-----|--------|
| `j` / `↓` | Move to next notification |
| `k` / `↑` | Move to previous notification; at top: refresh |
| `enter` | Open post/reply for navigable notifications; jump to the room for `chat_mention`; open the C-Mail conversation for `dm_message` |
| `p` | View actor's profile |
| `m` | Mark selected notification as read |
| `M` | Mark all notifications as read |
| `u` | Toggle unread-only filter |
| `f` | Open the category filter panel |
| `esc` | (from PostDetail or profile) return to notifications |

`M` marks the badge as read optimistically, then fires `POST /v1/notifications/read-all` in the background. That endpoint caps at 5,000 marked per call (v0.8.5+) and returns `hasMore`; `markAllNotifsReadCmd` (`internal/ui/app.go`) loops calling it while `hasMore` is `true`, bounded at `markAllNotifsReadMaxCalls` (20 calls / 100,000 notifications) as a defensive guard.

### Non-navigable types

Pressing `enter` on `poke`, `new_follower`, or `unfollowed` notifications (or any with an empty target ID) opens the actor's profile; it's a no-op only if the actor is unknown.

### chat_mention and dm_message navigation

`chat_mention` and `dm_message` are navigable, but not to a post — `enter` jumps to their respective live surface instead:

- `chat_mention` → emits `OpenRoomMsg{RoomSlug, NotifID}`. App switches to the Chatrooms screen, reloads the room list, and auto-enters detail mode for the matching room (`ChatroomsModel.SetPendingRoomSlug` / `OpenPendingRoom`), the same as if the user had cursored to that room and pressed `enter` themselves.
- `dm_message` → emits the existing `StartConversationMsg{Username}` (the same message the `c` key already sends from any screen), opening or creating the C-Mail conversation with the sender.

Both deep-links record Notifications as the return screen (`App.chatroomsReturn` / `App.cmailReturn`) and mark the destination model's `canGoBack = true`, so pressing `Esc` from the room/conversation returns straight back to Notifications instead of dropping to Chatrooms'/C-Mail's own list — see `docs/33-circ.md` and `docs/08-cmail.md`.

### chat_mention suppression for the room currently open

The API has no concept of room presence — there's no join/leave endpoint, so it generates `chat_mention` unconditionally, even for a user actively reading the room the mention happened in (see "No online-users list" in `docs/33-circ.md`). Since that's redundant (the message is already on screen), the client auto-suppresses it: any unread `chat_mention` whose `RoomSlug` matches the room currently open in Chatrooms detail view is marked read (both locally and via `MarkNotificationRead`) as soon as it's fetched, so it never bumps the tab badge or appears unread in the list (`App.suppressActiveRoomMentions` in `internal/ui/app.go`).

"Currently open" requires both: Chatrooms is the foreground screen (`App.active == screenChatrooms`) and that exact room is in detail view (`ChatroomsModel.ActiveRoomSlug()`). Switching to any other tab, or pressing `Esc` back to the room list, immediately stops the suppression for that room — mentions notify normally again from that point on.

---

## Notification Types

| Type | Summary text |
|------|-------------|
| `new_post_friend` / `new_post_following` | published something. |
| `bookmark` | saved your entry. |
| `new_follower` | started following you. |
| `unfollowed` | unfollowed you. |
| `reply` | replied to your post. |
| `reply_mention` | mentioned you in a reply. |
| `post_mention` | mentioned you in a post. |
| `chat_mention` | mentioned you in #\<roomName\>. (falls back to `#<roomSlug>`, then "mentioned you in chat." if neither is present) |
| `dm_message` | sent you a message. |
| `thread_reply` | replied in @username's thread. (falls back to "a thread you're following" if author unknown) |
| `guild_new_thread` | posted a new thread in #\<guildName\>. (falls back to "posted a new thread." if guild name absent) |
| `poke` | poked you. ¯\_(ツ)_/¯ |
| `supporter_granted` / `supporter_removed` | granted/removed your Supporter status. |
| `hacker_granted` / `hacker_removed` | granted/removed your Hacker status. |
| `image_permission_granted` / `image_permission_removed` | granted/removed your image permissions. |
| `attachment_permission_granted` / `attachment_permission_removed` | granted/removed your attachment permissions. |
| `system_ban` | your account has been banned. |
| `graffiti_mention` | mentioned you in a graffiti wall post. |
| `moderator_granted` / `moderator_removed` | granted/removed your Moderator status. |
| `api_access_granted` / `api_access_removed` | granted your API access. / revoked your API access. |
| `system_ban_lifted` | your ban has been lifted. |
| `post_cooldown` | a post was rate-limited and saved as a note instead. |
| `rate_limit_warning` | you're approaching a posting limit. |

### Actorless notifications (v0.8.5+)

`system_ban`, `system_ban_lifted`, `post_cooldown`, `rate_limit_warning`, `moderator_granted`/`moderator_removed`, and `api_access_granted`/`api_access_removed` are account-level events, not something another user did — the API sends them with either no `actorId`/`actorUsername` at all, or the literal string `"system"`. `graffiti_mention` is the one new type with a real actor, same shape as `post_mention`/`reply_mention`.

The client's `hasActor` helper (`internal/ui/screens/notifications.go`) treats an empty or literal-`"system"` `actorUsername` as "no actor":
- `renderNotif` omits the `@username` handle entirely for these and shows only the summary text.
- `p` (view profile) and `c` (start C-Mail conversation) no-op rather than opening a profile/conversation for user "system".
- If the notification's `reason` field is set, it's shown as the inline preview line (the same `"> …"` treatment mentions get from their content field) since these types explain themselves through `reason` rather than mention text.

### Inline content preview

`post_mention`, `reply_mention`, and `chat_mention` show a truncated `"> …"` preview line under the summary, sourced from `PostContent`, `ReplyContent`, and `MessageContent` respectively — the text that mentioned the user, without navigating away.

### Guild context

When a post or reply notification happens inside a guild, the summary appends an `in #<guild>` clause — e.g. `replied to your post in #chooms.` or `posted a new thread in #technica.` This applies to `new_post_friend`, `new_post_following`, `reply`, and `thread_reply` whenever the notification carries guild context; `guild_new_thread` always shows it.

The guild handle comes from `metadata.guildSlug` (the lowercase handle the server sends for guild replies/posts, alongside `metadata.isGuildThread: true`); `metadata.guildName` is a rarer display-name variant. The UI prefers the slug (`guildLabel` in `internal/ui/screens/notifications.go`), so the value is rendered **verbatim** with a leading `#`, matching the app's existing convention (join/leave banners, the `guild_new_thread` icon). `*_mention` types are excluded to avoid awkward phrasing. The clause is applied by `notifSummary` / `withGuild`.

---

## Pagination

- Initial load: fetches first page (`GET /v1/notifications?limit=20`).
- Pressing `j` at the last item, or scrolling to the bottom of the viewport, emits `LoadMoreNotifsMsg` which appends the next page.
- When no more pages are available, a `— end —` footer is shown.
- Pressing `k` at the top emits `RefreshNotifsMsg`, which re-fetches from the beginning and shows `fetching notifications...` while in progress.

---

## Mark as Read

Both actions use optimistic updates: the in-memory state is updated immediately, and the API call runs in the background.

- `m` — mark selected read → `PATCH /v1/notifications/:id`
- `M` — mark all read → `POST /v1/notifications/read-all`
- Opening a post via `enter` also marks that notification read optimistically.

---

## Unread Filter

The notifications screen opens in unread-only mode by default. Press `u` to toggle between unread-only and all notifications. When the filter is active and all notifications are read, the screen shows `all caught up`.

---

## Category Filter

Press `f` to open a panel that filters the list down to one notification category at a time, useful when there are too many notifications and only some kinds are relevant right now:

| Category | Types |
|---|---|
| `all` | (no filter — every type) |
| `mentions` | `reply_mention`, `post_mention`, `chat_mention`, `graffiti_mention` |
| `social` | `new_follower`, `unfollowed`, `poke`, `bookmark` |
| `threads` | `reply`, `thread_reply`, `guild_new_thread`, `new_post_friend`, `new_post_following` |
| `c-mail` | `dm_message` |
| `account/system` | `supporter_granted`/`removed`, `hacker_granted`/`removed`, `image_permission_granted`/`removed`, `attachment_permission_granted`/`removed`, `system_ban`, `system_ban_lifted`, `moderator_granted`/`removed`, `api_access_granted`/`removed`, `post_cooldown`, `rate_limit_warning` |

The panel is **single-select and live**: `↑`/`↓` (or `j`/`k`) move the cursor and wrap around at both ends (`up` from `all` jumps to `account/system`; `down` from `account/system` jumps back to `all`). The cursor moves instantly, but the actual fetch is **debounced by ~1 second** — the highlighted category is only applied once the cursor sits still for that long, so quickly scrolling through categories doesn't fire a fetch per keypress (which could arrive out of order and briefly flash a stale category's results). `enter` applies the highlighted category immediately, bypassing the debounce, and closes. `esc` reverts to whichever category was actually applied when the panel was opened (re-fetching back to it, not just whatever the cursor last touched) and closes. This mirrors the theme picker's live-preview-with-revert pattern (`internal/ui/app.go`'s `handleThemePickerKey`) rather than a checkbox-and-commit flow.

Filtering is server-side via `GET /v1/notifications?type=...` (single category's types, or no `type` param at all for `all`), so pagination stays correct against the filtered set — same "reset pagination state, refetch" shape as the `u` unread-only toggle.

Only one category's types are ever sent to the server at once. The API caps `type` at 1-20 comma-separated values (`docs/00-latest-api-reference.md`); the largest category, `account/system`, is 16 types, safely under that cap — so, unlike an earlier multi-select design that could combine categories past the limit, this cap is not user-reachable and needs no client-side guard.

A `filter: <category>  (press 'f' to change)` line appears above the list whenever a filter is active, so a short list doesn't read as broken.

**Not persisted across sessions** — the filter always resets to `all` (no filter) on every app launch, same as `showUnreadOnly`. This is deliberate: the tab badge (`polledUnreadCount`) always shows the true global unread count regardless of any active filter, so a persisted filter would leave the badge and the visible list silently mismatched every time the app opens.

---

## Back Navigation

Pressing `enter` on a navigable notification opens PostDetail. Pressing `esc` in PostDetail returns to the notifications screen with the previously selected notification visible. This uses a shared `postDetailReturn` field in the App to track which screen to return to (same mechanism used by Feed → PostDetail → Feed).

### Deleted-post notifications

A notification can point to a post that has since been deleted; the notification list carries no "deleted target" field, so this only surfaces when the post is opened and `GET /v1/posts/:id` returns `404 NOT_FOUND`. Rather than blocking the screen, the post-open fetch returns `notifPostLoadErrMsg`; `handleNotifications` shows a transient banner **"This post has been deleted"** and leaves the notifications list intact and usable. (A 401 here still redirects to login; other errors show their message in the banner.) See [31-global-notifications.md](31-global-notifications.md) for the banner mechanism and the broader "errors never block a screen" model.

### System notification unreachable via REST (post-too-soon conversion)

When a post is submitted too soon after a previous one, the server silently
converts it into a journal entry (see `docs/28-extended-posts.md` /
`docs/00-api-backlog.md`) and generates a "System" notification about it —
visible on the website and reflected in `GET /v1/notifications/unread-count`.
Confirmed live (2026-08-12) that this notification is **never returned by
`GET /v1/notifications`**, under any filter (unfiltered, `read=false`, or a
full 50-item page) — not a caching delay, a persistent gap between the two
endpoints. No REST client can fetch or render it; this is not a client-side
bug. The count/list badge can legitimately be off by however many of these
exist unread.

The v0.8.5 docs now document a `post_cooldown` type ("an entry you wrote was
held back and saved as a private note instead") that matches this scenario's
description exactly — likely the undocumented type behind this "System"
notification. Not confirmed live (the type still can't be fetched via REST
per the above), so the client's `post_cooldown` handling (icon/summary) is
in place but unverified against a real payload.

---

## API Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/v1/notifications?limit=20&cursor=…&type=…` | Fetch notification page; `type` is an optional comma-separated list of notification types, used by the category filter (`f` key) — see "Category Filter" above |
| `PATCH` | `/v1/notifications/:id` | Mark one notification read |
| `POST` | `/v1/notifications/read-all` | Mark all notifications read |
| `GET` | `/v1/posts/:id` | Load post when opening from notification |

### Wire format (`GET /v1/notifications`)

```json
{
  "notifications": [
    {
      "id": "abc",
      "type": "reply",
      "read": false,
      "createdAt": "2026-04-03T10:00:00Z",
      "actorId": "user-123",
      "actorUsername": "molly_millions",
      "targetId": "post-456",
      "targetType": "reply",
      "metadata": {
        "replyId": "reply-789",
        "authorUsername": "ragnar",
        "guildSlug": "chooms",
        "isGuildThread": true
      }
    }
  ],
  "nextCursor": "cursor-xyz"
}
```

`reason` (v0.8.5+) is a top-level field, not inside `metadata`. It's present only on system-generated, actorless notifications (`system_ban`, `system_ban_lifted`, `post_cooldown`, `rate_limit_warning`, and presumably `moderator_granted`/`removed`, `api_access_granted`/`removed`) and explains what happened — the client shows it as the inline preview line in place of mention content. See "Actorless notifications" above.

A `chat_mention` notification's `metadata` carries room context and the mentioning message instead:

```json
{
  "id": "def",
  "type": "chat_mention",
  "read": false,
  "createdAt": "2026-07-24T09:40:05.206Z",
  "actorId": "user-456",
  "actorUsername": "tangelic",
  "targetId": "cyberspace",
  "metadata": {
    "roomSlug": "cyberspace",
    "roomName": "The Sprawl",
    "messageContent": "@ragnar you here?"
  }
}
```

Note: the actor is returned as flat fields (`actorId`, `actorUsername`), not a nested object.

As of API **v0.5.0** the server `docs.md` documents this notification object and its `metadata` keys (previously reverse-engineered); the shape above matches the documented schema. `metadata` is open-ended — unknown keys are treated as optional.

---

## Model

```go
type Notification struct {
    ID                   string
    Type                 string
    Read                 bool
    CreatedAt            time.Time
    Actor                NotificationActor
    TargetID             string
    TargetType           string // "post", "reply", or ""
    ReplyID              string // metadata.replyId; reply to scroll to in PostDetail
    ThreadAuthorUsername string // metadata.authorUsername; set for thread_reply
    GuildName            string // metadata.guildName; rarer display-name variant
    GuildSlug            string // metadata.guildSlug; guild handle shown as #slug
    PostSlug             string // metadata.postSlug; slug of the target post (v0.7+)
    PostAuthorUsername   string // metadata.authorUsername; author of the target post (v0.7+)
    PostContent          string // metadata.postContent; non-empty for post_mention (v0.7+)
    ReplyContent         string // metadata.replyContent; non-empty for reply_mention (v0.7+)
    RoomSlug             string // metadata.roomSlug; chat_mention room to jump to
    RoomName             string // metadata.roomName; chat_mention room display name
    MessageContent       string // metadata.messageContent; non-empty for chat_mention
    Reason               string // reason; explains actorless system notifications (v0.8.5+)
}

type NotificationActor struct {
    ID       string
    Username string
}
```

`TargetType` is included for future use (e.g., scroll-to-reply when a `GET /v1/replies/:id` endpoint becomes available).

---

## Background Poll

After login, a `tea.Tick(60 * time.Second)` is scheduled. On each tick:
1. `GET /v1/notifications` (first page) is fetched.
2. The unread count is computed and stored in `App.polledUnreadCount`.
3. The tab badge is updated from `polledUnreadCount` when the notifications screen has not yet been loaded (`!notifications.IsReady()`).
4. The tick reschedules itself.
