# Feature 36: cIRC message selection, flagging, and delete

Per-message navigation in cIRC chat rooms, flagging a selected message for moderation review (`POST /v1/circ/:roomId/messages/:messageId/flag`, API v0.8), and deleting your own message (`DELETE /v1/circ/:roomId/messages/:messageId`). This is the message-level counterpart to `docs/35-flagging.md`'s post/reply flagging.

## The problem

cIRC's compose box is always focused the moment a room is open (unlike Feed/Post Detail, where compose is a separate panel you explicitly open) — so an action key like `!` risks being typed into a message instead of triggering an action. Two alternatives were rejected: a brand-new control key (breaks consistency with the `!` used elsewhere) and gating `!`/`d` on "input is currently empty" (silently steals the ability to start a message with a literal `!`, and would be much worse for `d` — a very common English sentence-starter).

## Design

Entry/exit are anchored to keys already meaningful in this context, not an arbitrary toggle:

- **`up`** from the normal typing state (`selectedMsgID == ""`, the sentinel): if there's at least one selectable message, blurs the compose input, selects the newest one, and scrolls it into view. If there's nothing selectable, falls back to the old raw scroll-by-one-line.
- **While browsing** (`selectedMsgID != ""`): `up`/`down` move the selection (via `millerPageNav`, the same pager math `PostDetailModel` uses for replies); `!` reports the selected message; `d` deletes it (own messages only); `esc` clears the selection and refocuses the input; anything else is swallowed rather than typed. Both `!` and `d` no-op on an already-deleted (tombstoned) message.
- **`down`** past the newest selectable message exits browsing the same way `esc` does — "scrolled back to live" means "back to typing."

This works because `bubbles/textinput.Update` already no-ops on any key while blurred (verified against its source) — blurring on entering "browsing" is the actual mechanism that prevents `!`/`d` (or anything else) from being typed, not a heuristic. `Blur()`/`Focus()` never touch the input's value, so an in-progress draft survives a trip into history and back untouched. Typing in the normal state is completely unrestricted: every key, including `!`, types exactly as it always has.

Selection is tracked by the message's **ID**, not a slice index — `PrependMessages` splices older history onto the *front* of the message list, which would silently invalidate a stored index (replies in `PostDetailModel` only ever append, so that bug class doesn't apply there). This also means delete works cleanly: a message is never removed from the slice (see Delete below), so the selected ID always still resolves.

System notices (locally-injected, e.g. a `/help` reply) have no ID and are never selectable, flaggable, or deletable.

## Bugs found during manual testing (fixed)

Three real bugs surfaced only under live use, not the initial test suite — each got a regression test once understood:

1. **Pagination fired on almost every `up` press.** The older-history fetch was triggered by checking `viewport.AtTop()`, but a room whose content already fits the viewport is trivially "at top" from the moment it renders, regardless of which message is selected. Fixed by only firing the fetch when the selection actually lands on the oldest loaded message (`newPos == 0` / `curPos == 0`), not whenever the raw scroll offset happens to read 0.
2. **`down` got permanently stuck partway through a long room.** Root cause was in `renderCircMessagesWithSelection`: each message's height was measured with `lipgloss.Height()` (`strings.Count(s, "\n") + 1`), which is correct once for a whole rendered block but wrong per-message-then-summed, since every message already ends in its own `\n`. The inflated, summed offsets slowly desynced from the viewport's real line count until `millerPageNav` and the viewport's own scroll clamp permanently disagreed. Fixed by measuring with `strings.Count(rendered, "\n")` (no phantom `+1`).
3. **Left/right arrow inside the flag-reason field (and later the delete-confirm box) jumped tabs instead of moving the cursor.** `ChatroomsModel.ComposeEmpty()` — which `app.go` uses to decide whether a bare left/right arrow can escape to tab-cycling — only checked the compose box's own value. Since the compose box is genuinely empty while flagging/deleting, the escape fired even though focus was actually on the overlay. Fixed by making `ComposeEmpty()` also return `false` while `flagPrompt.Active()` or `confirmingDeleteMsg`.

## Visual feedback

The selected message is highlighted with `theme.SelectedRow` (the same style `settings.go` uses for its selected-row highlight) rather than a bordered box — cIRC's tight IRC-log rendering has no per-message bounding box, and a border would look inconsistent with that. The highlight can't simply wrap the normally-colorized message text (the inner `theme.Highlight`/`markdown.RenderInline` calls emit ANSI resets that would cut off an outer background mid-line), so the selected message is stripped back to plain text (`ansi.Strip`) and only that plain text gets the highlight style — same approach `settings.go` uses.

## Flag flow

On `!`, the shared `FlagPrompt` overlay opens with a `FlagKindMessage` kind ("Report this message?"). On submit, `ChatroomsModel` emits `FlagMessageMsg{RoomID, MessageID, Reason}` (`messages.go`). `App` routes it to `flagRoomMessageCmd`, reusing the same `flagErrorMsg`/`flagResultText` helpers posts/replies use — the documented self-report 403 becomes "you can't report your own content," and everything else shows "reported" or "already reported."

## Delete flow

On `d` (own messages only, guarded like flag but the opposite way), a plain y/n confirm opens (`confirmingDeleteMsg`, matching Feed/Post Detail's existing delete-confirm style — no reason field, since there's nothing to type). On confirm, `ChatroomsModel` emits `DeleteRoomMessageMsg{RoomID, MessageID}`, routed to `deleteRoomMessageCmd` → `api.Client.DeleteRoomMessage`. On success, `App` calls `ChatroomsModel.ApplyMessageDeleted(messageID)`.

Delete is a **soft** delete per the API: the message stays in the list (`ApplyMessageDeleted` finds it by ID and sets `Deleted: true` / `Body: "[DELETED]"` in place — it never splices the message out), and renders as a muted tombstone (`renderDeletedTombstone` in `render.go`) that keeps the author and original timestamp but drops the body, action/style flags, and attachments — matching what the API documents ("your name and the original timestamp stay").

**Live propagation to other users.** The API delivers a delete as an RTDB `patch` event, not a new message — `{"content":"[DELETED]","deleted":true}`, with sender/timestamp omitted since they don't change. `SubscribeRoom` previously only handled `put` events; `patch` was silently ignored, so a message would vanish from the sender's own view but never update for anyone else already in the room until they reopened it. Fixed: `SubscribeRoom` now also handles `patch`, and for a delete patch specifically emits a `model.Message` carrying **only** `{ID, Deleted: true}` — never a zeroed-out full message that could be mistaken for a real (if empty) one. `ChatroomsModel`'s `roomReceivedMsg` handler branches on `msg.msg.Deleted`: `true` merges onto the existing message via `ApplyMessageDeleted`, otherwise it's a genuinely new message and goes through the normal `AppendMessage` append path.

## API / model

- `api.Client` gained `FlagRoomMessage(roomID, messageID, reason string) (flagID string, alreadyFlagged bool, err error)` and `DeleteRoomMessage(roomID, messageID string) error`, mirroring `FlagPost`/`FlagReply` and `DeletePost`/`DeleteReply` respectively.
- `model.Message` gained `Deleted bool` (cIRC only). Populated from REST history (`wireCircMessage.Deleted`) and the RTDB stream (`wireRTDBCircMessage.Deleted`), so a message already deleted before you opened the room also renders as a tombstone.
- The mock client's live-injected message previously reused one hardcoded ID (`"mock-room-live-1"`) on every delivery; since the app reconnects and redelivers whenever that mock channel closes, a room left open for a while would accumulate several messages sharing one ID — harmless for rendering, but breaks anything keyed by message ID. Fixed to generate a fresh ID per delivery.
