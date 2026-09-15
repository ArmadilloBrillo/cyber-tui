# Feature 52 — Sent-line recall (cIRC / C-Mail)

Shell-style history recall for the cIRC and C-Mail compose inputs. `Ctrl+↑`
walks the input box back through lines you've already sent this session;
`Ctrl+↓` walks forward again and finally hands back whatever you were
half-typing when you started. It only works while the compose input is the
active thing — not while browsing message history.

## Behaviour

| Key | Action |
|---|---|
| `Ctrl+↑` | Replace the input with the previous sent line. On the first press the current half-typed text is stashed as the "draft". Stops at the oldest entry. |
| `Ctrl+↓` | Replace the input with the next (newer) sent line; stepping past the newest entry restores the stashed draft once. **No-op until the first `Ctrl+↑`** — there is nothing "forward" of a box you haven't stepped back from. |

- **Scope:** one buffer per conversation (C-Mail) / per room (cIRC), for the
  whole session — `Ctrl+↑` in a conversation only walks lines you sent in
  *that* conversation. Keyed by conversation ID / room slug in a
  `map[string]*inputHistory` on the screen model, one entry created lazily per
  thread visited.
- **Reset:** opening a conversation/room resets *its* browse position (so a
  fresh `Ctrl+↓` does nothing) but keeps that thread's recorded lines.
- **Recorded:** every non-empty line submitted with `Enter`, including
  `/`-commands (recorded even when the command is rejected locally as unknown,
  so you can `Ctrl+↑` and fix the typo). Blank lines and an immediate
  duplicate of the last entry are skipped. Capped at the last 100 lines.
- **Not active while browsing messages** (`selectedMsgID != ""`): there the
  arrow keys already move the message selection, and `Ctrl+↑`/`Ctrl+↓` are
  ignored.

## Implementation

`internal/ui/screens/inputhistory.go` — `inputHistory`, a plain struct with
`entries []string`, a `pos` cursor (`== len(entries)` means "not browsing"),
and a stashed `draft`:

| Method | Purpose |
|---|---|
| `record(s)` | Append a sent line; drop blanks and immediate dupes; trim to `inputHistoryMax` (100); reset `pos`. |
| `reset()` | Stop browsing (`pos = len(entries)`, clear draft); keep entries. |
| `prev(current) (string, bool)` | Step back; stash `current` as the draft on the first step; `false` at the oldest entry or on an empty buffer. |
| `next() (string, bool)` | Step forward; yield the draft once past the newest; `false` when not browsing. |

Wiring, identical in `cmail.go` and `chatrooms.go` (both gained a
`sentHistory map[string]*inputHistory` field, initialised in the constructor,
plus a `histFor(id string) *inputHistory` helper that lazily creates the
per-thread buffer — mirrors `chatrooms.go`'s `mutedUsersByRoom` map pattern):

- Detail-mode key switch, while typing: `ctrl+up` →
  `histFor(activeID).prev(input.Value())`, `ctrl+down` → `histFor(activeID).next()`;
  on `ok` the returned string is set as the input value with the cursor at the
  end. `activeID` is `m.activeConvID` / `m.activeRoomID`.
- The `enter` send handler calls `histFor(activeID).record(val)` for any
  non-empty input before it dispatches / rejects.
- `histFor(id).reset()` runs where a conversation/room is opened
  (`CMailModel.SetActiveConversation` and the list-mode `enter` handler;
  `ChatroomsModel.enterRoomDetail`).

`histFor` has a value receiver like every other method on these models; the
lazy `m.sentHistory[id] = &inputHistory{}` insert is visible across model
copies because the map header points at one shared backing table (same reason
`chatBodyCache` writes work on value receivers here).

`ctrl+up`/`ctrl+down` reach the screen because they aren't in
`activeScreenHasFocusedInput()`'s consume list in `app.go` — unlisted keys
fall straight through to the screen's `Update`.

## Tests

`internal/ui/screens/inputhistory_test.go`:

| Test | Covers |
|---|---|
| `TestInputHistoryRecallAndDraft` | Empty-buffer no-ops; dup/blank rejection; `prev` walk + draft stash; `next` walk + draft restore; bounds at both ends. |
| `TestInputHistoryCapAndReset` | 100-line cap (now per thread) holds; `reset()` stops browsing without dropping entries. |
| `TestCMail_SentHistory_PerConversation` | Lines recorded in one conversation are invisible from another. |
| `TestChatrooms_SentHistory_PerRoom` | Same isolation, per room slug. |

## Not done

- No persistence across restarts — session-wide and in-memory only.
- No cap on the *number* of tracked threads (one small `*inputHistory` per
  conversation/room visited). Negligible for a TUI session; add an LRU if a
  session ever tracks thousands.
- Editing a recalled line then pressing `Ctrl+↑`/`Ctrl+↓` discards the edit
  (no readline-style per-slot edit buffer).
