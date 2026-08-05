# Feature 37: cIRC `/mute` family

Client-side per-room message hiding for muted users, driven by `mutedUsersByRoom` on `Settings`.

## How it works

`/mute <username>`, `/unmute <username>`, `/muted`, and `/unmuteall` are ordinary cIRC slash commands — typed into the compose box and sent through the existing `SendRoomMessage` path exactly like `/dice` or `/help`. **cIRC only**: sending any of them in a C-Mail conversation returns `400` from the server (surfaced as the existing generic error banner — no special-casing needed).

The server does all the bookkeeping: each command posts nothing and returns `{"data":{"reply":"..."}}` with confirmation text, which the client already shows as a local system message via the existing `roomCommandReplyMsg`/`AppendSystemMessage` mechanism (the same one `/help` uses) — no new plumbing needed for that part.

**Critically, the server does not filter messages.** `GET /v1/circ/:roomId` still returns a muted user's messages — the API contract explicitly puts filtering on the client. The mute list itself lives in `mutedUsersByRoom` (`map[roomID][]username]`) on `Settings`, shared with the website.

## Implementation

- `model.Settings.MutedUsersByRoom` — new field, decoded from `GET /v1/settings` (`wireSettings`/`wireSettingsToModel` in `internal/api/client.go`). Deliberately **excluded** from `wirePatchSettings`/`UpdateSettings`, same treatment as `mutedTopics` — it's server-managed via the slash commands, never PATCHed by the client.
- After any room command reply (`roomCommandReplyMsg` in `internal/ui/app.go`), the client re-fetches `Settings` so a `/mute`/`/unmute`/`/unmuteall` takes effect immediately without a relogin. This isn't command-specific — any reply-producing command triggers the refresh, which is cheap and idempotent.
- `ChatroomsModel` picks up `Settings.MutedUsersByRoom` via the existing `SharedConfigMsg` broadcast and re-renders the active room immediately if one is open.
- Filtering happens in `renderCircMessagesStyled` (`internal/ui/screens/render.go`): a muted sender's message contributes **zero output**, not a removed slice entry — this keeps `msgOffsets`/`msgHeights` 1:1 with `m.messages`, which selection/scrolling/pagination code depends on. `selectableMessageIndices` also excludes muted senders, so a hidden message can't be selected, flagged, or deleted while muted. Unmuting re-reveals any already-fetched history for free, since nothing was ever removed from `m.messages`.

## Out of scope

- No dedicated keybinding — muting is slash-command-only, matching every other cIRC command. The client only checks that `/mute` etc. are *recognized* command names (`circOnlySlashCommands`, `docs/33-circ.md`); there's no argument parsing or autocomplete for the username.
- No local-only config file storage — unlike `Timezone` (which has no server equivalent), `mutedUsersByRoom` is genuinely server-synced and shared with the website, so it lives in `model.Settings`.
