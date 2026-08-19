# Battery Audit — Client-Side Power Findings

Tracks client-side battery/power findings from the 2026-08-19 battery audit (5-agent parallel review: network, rendering, goroutine lifecycle, disk I/O, settings architecture). Scope is `internal/ui`, `internal/api`, `internal/rtdb`, `internal/config`, `internal/ssh` — no code changed as part of the audit itself. Update this file as items are fixed or new ones are found.

Unlike `docs/00-api-backlog.md`, everything here is client-side and fixable without server involvement.

---

## Fix today (confirmed bugs — no config toggle needed, no UX tradeoff)

| # | Finding | Location | Status |
|---|---|---|---|
| 1 | `handleUnauthorized` (the app's only logout path) never cancels the open C-Mail/CIRC RTDB subscriptions. If a session expires (401) while a DM or chat room is open, the typing-heartbeat, idle-check, and presence-heartbeat `tea.Tick` chains (500ms/heartbeat cadence) keep firing real API calls against the invalidated token indefinitely, and the underlying RTDB SSE goroutines (reader + scanner + 5s presence ticker) are orphaned — reaped only by the 10-minute idle watchdog, later if traffic resets it. On the Wish-hosted SSH server this compounds across every abrupt client disconnect over the process's lifetime. **Fix:** added `a.cmail = a.cmail.CancelSubscription()` and `a.chatrooms = a.chatrooms.CancelSubscription()` to `handleUnauthorized`. Regression-guarded by `TestHandleUnauthorized_CancelsLiveDMSubscription`/`TestHandleUnauthorized_CancelsLiveRoomSubscription` in `internal/ui/app_test.go` (a spy client verifies the subscription's cancel func actually fires). | `internal/ui/app.go:2528-2560` (`handleUnauthorized`); teardown methods at `internal/ui/screens/cmail.go:344` and the Chatrooms equivalent | **Resolved** |
| 2 | DM typing-stream reconnect (`dmTypingStreamClosedMsg`) retries immediately with no backoff or attempt cap — the only reconnect path in the codebase without one. The comment at the call site claims parity with CIRC's presence reconnect, but that one *does* have full backoff + a 6-attempt cap via the shared `reconnect.go` machinery; this is the odd one out. Low likelihood in practice (requires the server to accept-then-instantly-drop the stream repeatedly) but unbounded-fast-retry if triggered. **Fix:** now routes through the same `reconnectDelay`/`scheduleReconnectRetryCmd`/`maxReconnectAttempts` machinery as the other four streams (new `reconnectTypingCmd`/`scheduleTypingReconnectRetryCmd`, new `dmTypingReconnected/Failed/RetryDueMsg` types, new `typingReconnect*` fields on `CMailModel`). Deliberately no UI indicator, matching CIRC presence's actual (not the stale comment's claimed) precedent. Covered by 5 new tests in `internal/ui/screens/reconnect_test.go` (`TestCMailTypingReconnect_*`). | `internal/ui/screens/cmail.go:1066-1077` (now the `dmTypingStreamClosedMsg` handler + new reconnect chain) | **Resolved** |

**Follow-up, resolved as part of #1:** the exported `CancelSubscription()` methods on `CMailModel`/`ChatroomsModel` previously had no call sites outside test files. They're now wired into `handleUnauthorized` as their intended logout-path hook, so this is no longer dead code.

---

## Fix soon (real cost, no tradeoff, just not urgent)

| # | Finding | Location | Status |
|---|---|---|---|
| 3 | C-Mail has no message-render cache. Chatrooms already has `chatBodyCache` so a 150ms animated-style tick only re-renders the animated message; C-Mail re-parses the *entire* loaded conversation history 6.7×/sec while any styled message is loaded. **Fix:** ported the pattern — `cmailBodyCacheEntry` (new type) + a `cache` param on `renderChatMessagesWithSelection`, with the same animated-skip guard; `CMailModel.chatBodyCache` field, evicted in `SetConversationMessages`/`trimMessageBuffer` mirroring `ChatroomsModel`'s eviction exactly. Covered by 3 new render_test.go tests (hit/stale/animated-skip) + 2 new cmail_test.go eviction tests. | `internal/ui/screens/render.go` (`renderChatMessagesWithSelection`) vs. `render.go` (Chatrooms' `chatBodyCache`) | **Resolved** |
| 4 | Guilds and Topics have no thread-render cache. Feed already has `bodyCache` (keyed by post id + width + content + theme) specifically so an unrelated background tick doesn't force a re-parse. Guilds/Topics never got it — viewing a long thread means every 15s feed poll, 60s unread poll, or backgrounded chat message forces a full markdown re-parse of the visible reply tree. **Fix:** scoped to the Miller detail/thread pane (`DetailView`, `renderDetailReply`, `pageThreadNav`) — two new shared free functions in `render.go`, `cachedPostCard` (reuses Feed's existing `feedBodyCacheEntry`) and `cachedReplyCard` (new `replyBodyCacheEntry` type), used identically by both `GuildsModel` and `TopicsModel` (previously byte-for-byte duplicated, uncached, in both files) via new `postBodyCache`/`replyBodyCache` fields, evicted in `SetGuildPosts`/`SetTopicPosts` and the `GuildThreadRepliesMsg`/`TopicThreadRepliesMsg` handlers. The flat-list and Miller compact-list surfaces were left alone (out of scope — not what this item flagged, and the compact list does no markdown rendering to cache). Covered by 4 new render_test.go tests (hit/stale, both functions) + 4 new eviction tests split across new `guilds_internal_test.go`/`topics_internal_test.go`. | `internal/ui/screens/guilds.go` (`DetailView`/`renderDetailReply`/`pageThreadNav`), `topics.go` (same) vs. Feed's `bodyCache` (`feed.go:140-145`) | **Resolved** |
| 5 | Feed background poll fetches a full page (20 complete posts — body, author, images) every 15 seconds, unconditionally, regardless of which tab is active. Largest network offender found. **Fix:** a hard tab-gate was ruled out — it would silently defeat the documented cross-tab "(N) new posts" badge (`docs/39-feed-background-poll.md`), which is deliberately global. Instead: interval lengthened 15s → 60s (`feedPollInterval`, matches the notifications poll's cadence, a straight 4x cut in radio wakes with zero feature change), plus a new Settings → feed → "auto-refresh (background poll)" toggle (`Config.FeedManualRefreshOnly`) for users who want the poll off entirely — live in both directions (stops immediately when disabled mid-session, restarts immediately when re-enabled). Covered by 2 new app_test.go tests (tick self-termination, manual→auto restart) and 4 new settings_test.go tests (toggle/dirty/save/setSaved round-trip). | `internal/ui/app.go` (`scheduleFeedPollCmd`, `feedPollTickMsg`, `afterLoginCmd`) | **Resolved** |
| 6 | C-Mail runs two independent `tea.Tick` chains at the same 500ms granularity (typing idle-check, typing-dots animation) — coalesce into one. **Fix:** merged into a single tick (the idle-check's clear-on-timeout logic folded into `typingAnimTickMsg`'s handler; `typingIdleCheckMsg`/`scheduleTypingIdleCheckCmd`/`dmTypingIdleCheckInterval` deleted). Also added a **Settings → c-mail → typing indicators** toggle (not originally scoped, added on request) — the bigger battery win: when off, C-Mail skips the inbound typing-presence RTDB subscription, the outbound announce/clear calls and their heartbeat re-announce chain, and the merged tick never starts; live in both directions while a conversation is open. Covered by 4 new render/handler tests + 6 new toggle-gating tests in cmail_test.go, 5 new settings_test.go round-trip tests. | `internal/ui/screens/cmail.go` (`typingAnimTickMsg` handler, `ConvOpenCmds`, Enter-key handler, `handleTypingInputChanged`, `case SharedConfigMsg:`) | **Resolved** |

---

## Deliberate tradeoffs (candidates for a `BatterySaver` config toggle, not bugs)

These are documented, working-as-designed features that trade battery for liveness. Don't change default behavior without a toggle — each removes something a user may actively want.

| # | Behavior | Location | Toggle effect if enabled |
|---|---|---|---|
| 7 | CIRC room (message SSE + presence SSE + 30s heartbeat) and open DM conversation (message + typing SSE) both deliberately stay live across tab switches, not just while focused. | `internal/ui/screens/chatrooms.go:496/508`, `cmail.go` message/typing SSE | Idle timeout: drop the stream after N minutes unfocused, reconnect on return |
| 8 | Account-wide conversation-list stream (`SubscribeUserConversations`) opens eagerly at login regardless of whether C-Mail is ever opened, and stays open for the whole session. Firebase RTDB sends keepalive lines roughly every ~30s for its whole lifetime. Hardest one to soften without changing what login gives you (instant, always-current unread badge). | `internal/ui/app.go:4147` | Would need explicit product decision — not a pure client tweak |
| 9 | Unread-notification poll runs every 60s for the whole session regardless of which tab is active. | `internal/ui/app.go:5731` | Lengthen interval (e.g. 60s → 5min) |
| 10 | Idle logo-scramble animation bursts to ~16.7Hz for 2-3s every 30s of idle time, for the whole session, on every screen's chrome. Purely cosmetic, ~8% average duty cycle, no existing toggle. | `internal/ui/app.go:66-67,2451-2504,5741-5747` | Disable entirely — zero UX cost |

### Recommended toggle design

One master `BatterySaver` bool bundling items 5/7/9/10 above (not a page of individual dials — split later only if someone actually needs one lever independent of the rest). Local-only config field, follows the exact pattern already used by `WanderLust`: one row in the Settings screen's existing "display" group, no new section. Config-file-first (this app has no CLI flags beyond `--version`).

Touch points (mirrors `WanderLust`'s plumbing exactly):
1. `internal/config/session.go` — add `BatterySaver bool` to `Config`.
2. `internal/ui/app.go` — hydrate an `App` field from it alongside `WanderLust`.
3. `internal/ui/screens/messages.go` — add to `SharedConfigMsg` and `SaveSettingsMsg`.
4. `internal/ui/screens/settings.go` — one new `settingsItem` row in the "display" group + the usual dirty-tracking fields.
5. `app.go`'s `broadcastConfig` — include it in the `SharedConfigMsg` literal.
6. `app.go`'s `SaveSettingsMsg` handler — write through the existing `saveConfig` mutate closure.
7. Each gated call site reads the flag off the `SharedConfigMsg` it already receives — no new plumbing.

**Document explicitly wherever this ships:** the toggle has zero effect over an SSH-hosted connection (`internal/ssh/server.go`'s Wish mode) — each hosted session is ephemeral and never reads the host's `~/.cyber-tui.json`.

---

## Confirmed clean (no action)

- **Disk I/O / config persistence** — all `config.Save` call sites are user-triggered or the twice-daily wander update; nothing polls into a disk write. Token refresh is in-memory only, never touches disk between logins. No image or terminal-capability disk caching exists. (Minor unrelated hygiene item: a few unconditional `log.Printf` calls on rare parse-error paths — `internal/api/client.go:169,904,971` — write to stderr regardless of debug mode; a display-corruption risk over SSH, not a power one.)
- **HTTP/RTDB client config** — default transport, keep-alives on, no reconnect churn from polling itself.
- **Reconnect bounding** — every sequence except item #2 above is correctly capped at 6 attempts with 1/2/4/8/15s backoff.
- **Wander check** — hourly local-only check, only writes/POSTs when actually due (every ~12h); 23 of 24 checks are a no-op.
- **Goroutine hygiene** — no busy-loops or sleep-polling anywhere in production code; context cancellation is threaded correctly through the RTDB client outside of items #1/#2.
- **Inline image cache** — fetch/decode/encode cached by (URL, protocol, dither settings) with eviction and in-flight dedup; never redecoded on scroll.
- **Sixel full-screen repaint on scroll** — protocol-inherent (documented, load-bearing fix after live hardware testing), not fixable without regressing to worse flicker. Already covered by the existing graphics-protocol/inline-images config.
