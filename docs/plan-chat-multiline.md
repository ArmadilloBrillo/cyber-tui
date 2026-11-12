# Plan: multi-line messages in cIRC and cMail

## Context
API v0.9.1 (snapshot committed on `chore/api-docs-v0.9.1`, `17207e6`) lets the cIRC and cMail send-message `content` contain `\n`: up to 10 lines and 2048 characters. The server collapses runs of blank lines into one and joins lines past the 10th with spaces, and the docs say clients should render line breaks. Styled cIRC messages get their line breaks turned into spaces by the server. cIRC `/art` puts its art on the lines after the command and stores it as-is, up to 80 columns by 25 lines.

Today both chat screens use a single-line bubbles `textinput`. Its sanitizer turns `\n` into a space, so users cannot send line breaks (and cannot post `/art` at all). cMail bubbles also render a lone `\n` as a space, because markdown treats it as a soft break.

Goal: users can type and send multi-line chat messages, and both screens display them. Where they fit, reuse the post/reply editor's building blocks.

## Decisions (confirmed with the user)
- **Input box size.** The box grows from 1 to 5 visible rows, counting soft-wrapped rows, then scrolls inside. Up to 10 lines can be entered.
- **Newline key.** It reuses the configurable hard-break key (`config.HardBreakKey`, default alt+enter) and inserts a plain `\n`. Enter still sends.
- **Mirror the server rules.** The input applies the same rules as the server, so the box always shows exactly what will be stored. Blank-line runs collapse to one, and lines past the 10th are joined with spaces. A blank line is one that is empty or whitespace-only.
- **`/art` is exempt.** When the input starts with `/art`, there is no normalisation and up to 26 lines are allowed (the command line plus 25 art lines). Blank lines and leading spaces are kept. The server enforces the 80x25 limit.
- **cIRC @mention autocomplete.** Tab and Space work on every line and at every cursor position. The suggestion moves from inline grey ghost text to a one-row hint line above the input, for example `@ragnar · tab next · space accept`. The hand-drawn ghost overlay is deleted.

## Reuse from the post/reply editor
- The bubbles `textarea`, as used by `ComposeModel` (`internal/ui/screens/compose.go:63` field, `:75` constructor).
- `insertHardBreak`'s rescroll trick (`hardbreak.go:117`): `InsertString` followed by `Update(nil)`.
- The hard-break key plumbing: `SharedConfigMsg.HardBreakKey` (`internal/ui/app.go:1045`), `resolveHardBreakKey` and `DefaultHardBreakKey` (`hardbreak.go`). Both chat screens already receive `SharedConfigMsg` (`chatrooms.go:1287`, `cmail.go:971`) and currently ignore this key.
- `convertPaste`'s fence-aware newline-to-hard-break logic (`hardbreak.go:67`). It is extracted into a string function so the cMail renderer can reuse it.
- `truncateStr` (`profile.go:1234`), for one-line previews.

`ComposeModel` itself is not reused. In it, Enter inserts a paragraph, ctrl+s and esc submit and cancel, line breaks are stored as markdown `"  \n"`, and there is a label row. Instead, a few small helpers in `shared.go` are shared by the two chat screens.

## Steps
For each step: implement it, run `go test ./...`, `go vet ./...` and `staticcheck ./...`, show the results table, then wait for approval.

Branch: `feature/chat-multiline`, off `dev` once `chore/api-docs-v0.9.1` is merged (or off that branch).

### 1. cMail renders line breaks (display only)
- **Extract the conversion.** In `hardbreak.go`, pull `convertPaste`'s string logic into `hardBreaks(s string) string`. `convertPaste` then calls it, so its behaviour and existing tests are unchanged.
- **Use it in the bubble renderer.** In `render.go:753` (`renderChatMessagesStyled`), apply `hardBreaks` after `substituteChars`, because the flip style reverses the text. Fenced code is left untouched. Posts use a different render path and are not affected.
- **Fix soft breaks inside formatting.** In `markdown/renderer.go:687` (`rawTextNode`), write a space on a hard line break. Otherwise `**a\nb**` renders as "ab"; today it renders as "a b".
- **Card preview.** `cmail.go:1767` uses `truncateStr` instead of `truncate`, so a multi-line last message doesn't break the card height.
- **cIRC needs no change.** Normal, styled and action messages go through `RenderInline`, which splits on `\n`. System notices (`render.go:670`) and art (`chatstyle.go:325`) keep `\n` through their own paths. The render caches need no change.
- **Tests.**
  - `render_test.go`: a cMail body `"a\nb"` renders as two lines; a fenced block gets no trailing spaces; a cIRC regression guard.
  - `hardbreak_test.go`: `hardBreaks` table.
  - `markdown/renderer_test.go`: the `**a\nb**` case.
  - `cmail_test.go`: the card preview stays one line.

### 2. Shared chat-input helpers and cMail switches to textarea
In `shared.go`, next to `insertAtCursor`, which `compose.go` still uses:
- **`newChatInput(placeholder)`**:
  - prompt `"> "`, no line numbers, `CharLimit` 2048, height 1
  - `KeyMap.InsertNewline` disabled
  - `Prompt`, `Text` and `CursorLine` set to a plain style on both `FocusedStyle` and `BlurredStyle`, before any `Focus()`. textarea's defaults dim the prompt and text and highlight the cursor line, which differs from today's look.
  - Note in a `ponytail:` comment that `CharLimit` counts display width, so CJK and emoji count as 2.
- **`normalizeChatBody(s)`**: CRLF/CR become LF, blank-line runs collapse to one, and lines past the 10th are joined onto the 10th with spaces. Values starting with `/art` pass through unchanged.
- **`chatInputOffset(ta)`**: the absolute rune offset of the cursor. It sums the earlier rows (length + 1 each), then adds `LineInfo().StartColumn + ColumnOffset`.
- **`updateChatInput(ta, msg, newlineKey)`**:
  - Newline key: build the candidate value with `\n` at the offset. Insert only if normalising leaves it unchanged; this one check refuses both an 11th line and a second blank line. For `/art`, the only check is a 26-line cap. Insert with `InsertString`, then `Update(nil)`. Never test against a copied textarea, because copies share row memory.
  - Paste key message: rewrite `\r\n` and `\r` to `\n` in the runes before passing it on. The textarea's own sanitizer would otherwise turn CRLF into blank lines.
  - Afterwards, if a paste key message or any non-key message (textarea's internal ctrl+v paste result) changed the value, and normalising changes it, call `SetValue` with the normalised value. The cursor jumps to the end; mark this with a `ponytail:` comment.
- **`fitChatInput(ta)`**: the row count is the sum over lines of `max(1, ceil((width(line)+1)/ta.Width()))`, clamped to 1..5. Set the height, then call `Update(nil)` to rescroll. Mark with `ponytail:` that word-wrap can use more rows than this character-width estimate.
- **`shared_test.go`**: table tests for `normalizeChatBody` (blank runs, whitespace-only lines, 12 lines, `/art` passthrough) and for `chatInputOffset` across multiple lines.

cMail changes:
- **Field and setup.** Field type at `:212`; construction at `:371-385` via `newChatInput`; `Init` returns `textarea.Blink`.
- **Width.** `SetWidth(msg.Width - 4)` at `:969`.
- **Icons.** Icon insert (`:931`) uses `InsertString`.
- **Cursor calls.** Drop the redundant `CursorEnd` calls (`:938`, `:1389`, `:1395`).
- **View.** Delete the placeholder width hack in `View` (`:2177-2200`).
- **Key passthrough.** At `cmail.go:1485-1491`, `m.input.Update` is replaced by `updateChatInput`. The newline key is not a case in the key switch, so the existing before/after value comparison drives the typing indicator for newlines and pastes too.
- **`newlineKey` field.** It defaults to `DefaultHardBreakKey` and is set from `SharedConfigMsg` with `resolveHardBreakKey`.
- **Box size.** The box can already hold multiple lines in this step but stays 1 row until step 4. That is acceptable for an intermediate step.
- **Tests.** The existing `cmail_test.go` suite passes, including `TestCMailInputBox_WidthConstantBetweenEmptyAndTyped` (`:172`); update its comment. Fix the stale textinput comments (`cmail_test.go:213`, `cmail.go:559`).

### 3. cIRC switches to textarea; mention hint line
- **Mechanical edits** in `chatrooms.go`, the same as step 2:
  - field `:197`, construction `:452`, width `:1285`
  - icon and compose-value handling `:1245-1254`
  - drop `CursorEnd` at `:1661`/`:1667`
  - delete the placeholder branch `:2512-2535`
  - passthrough at `:1763` via `updateChatInput`
  - `newlineKey` from `SharedConfigMsg`
- **Mentions** (this is new behaviour: today it only works when the cursor is at the end, `chatrooms.go:2356`):
  - `mentionActiveCandidate` uses `chatInputOffset` and drops the end-of-value requirement.
  - Tab and Space commit no longer use `SetValue` + `SetCursor`, which only works on the last row. They delete the query with backspace key messages (`cursor - atPos` times; a mention token never spans a `\n`), then `InsertString("@"+name)`.
  - Space adds a trailing space only when the next rune is not whitespace.
  - Space commits only when the query is non-empty, so a bare `@` followed by Space stays a literal space.
  - `mentionCycle` keeps working on absolute offsets.
- **Hint line.**
  - Delete the ghost overlay (`:2481-2511`) and `mentionGhostText`. Render a one-row hint, truncated to `m.width`, between the message area and the input.
  - A single predicate decides when the hint shows, used by both `View` and `viewportHeight` (`:2213`): not browsing (`selectedMsgID == ""`), the value contains `@`, and a candidate exists. The `@` check is a cheap guard, because `viewportHeight` runs on every tick.
- **Viewport sync, added here because the hint toggles without the box changing size.**
  - In the existing `Update` wrapper (`chatrooms.go:1117-1141`), after `updateInner` and before `maybeStartStyleAnim` and the unread-badge `AtBottom` check: when ready and in detail mode, and `m.viewportHeight() != m.viewport.Height`, record `atBottom`, set the new height, and call `GotoBottom` if it was at the bottom.
- **Tests** in `chatrooms_test.go`:
  - Rewrite the roughly 12 `mentionGhostText` tests (`:1060`, `:1101-1175`, `:1213`, `:1285-1328`) against `mentionActiveCandidate`'s candidate (for example `"alice"`, not `"ice"`). The "cursor not at end" case (`:1297`) flips to expect a match.
  - Replace the `Position()` calls at `:1095` and `:1192`.
  - Replace the ghost render tests (`:1350`, `:1500`) with hint-line tests: the hint shows and hides, and the viewport shrinks by 1 row while it shows.
  - New cases: Tab on line 2; Space mid-text with no double space; a bare `@` plus Space stays literal.
  - `TestInputBox_WidthConstantBetweenEmptyAndTyped` (`:896`) keeps passing; update its comment. Fix the stale comment at `:286`.

### 4. Growing box, arrow keys and send (both screens)
- **Grow the box.**
  - The existing `Update` wrappers (`chatrooms.go:1117`, `cmail.go:885-901`) call `fitChatInput` after `updateInner`, in detail mode.
  - The same viewport-height sync from step 3 then applies the new box height.
  - cMail gets a `viewportHeight()` (header + input height + 2 borders), used at `:946`/`:956` and by the sync.
  - cIRC's `viewportHeight()` uses the input height in place of the constant.
  - Delete `chatroomInputRows`/`chatroomDetailChrome` and `cmailInputRows`/`cmailDetailChrome`.
  - Generalise the `inputBoxLines` test helper (`chatrooms_test.go:867`) for multi-row boxes.
  - Fix the "chrome = 5" comments (`chatrooms_test.go:2597/2614`, `cmail_test.go:1504`).
- **Arrow keys** (`chatrooms.go:1710/1727` and cMail's up/down):
  - "up" enters browsing or scrolls only when the cursor is on the first visual row (`Line()==0 && LineInfo().RowOffset==0`). Otherwise it moves the cursor inside the input.
  - "down" mirrors this on the last row.
  - pgup and pgdown are unchanged.
- **Enter.**
  - The value goes through `normalizeChatBody` (a safety net, for example after a deletion joins two blank lines). Then leading and trailing `\n` are trimmed, and the result is checked for empty.
  - The normalised value is recorded in history (`chatrooms.go:1675`, `cmail.go:1402`), so recalled values never need normalising. It is then sent.
  - Slash-command parsing (`strings.Fields`) already handles newlines.
- **Known limitation**, marked with a `ponytail:` comment: shrinking back from more than 5 rows can leave the top row hidden until the cursor moves.
- **Tests**, for both screens:
  - The newline key inserts `\n`, the box grows from 3 to 4 rows, and the viewport shrinks by 1.
  - A long single line grows the box by its wrapped rows.
  - Enter sends the multi-line body, and the box resets to 1 row afterwards.
  - An 11th line is refused, and a second consecutive blank line is refused.
  - `/art` accepts 26 lines with blank lines kept, and a 27th is refused.
  - A CRLF paste becomes LF. A paste with blank runs is collapsed. A 12-line paste is joined to 10.
  - Up on line 2 moves the cursor; up on line 1 enters browsing.
  - A multi-line history recall resizes the box.
  - cMail: the typing indicator fires on a newline.

### 5. Hints, docs and small follow-ups
- **Key hints and help.** Add the newline key to the chat status hints (`internal/ui/layout_tabs.go:426`, `:431`) and to the help sections (`:590-600`). The settings label "hard line break key" (`settings.go:309`) now also covers chat, so adjust the wording.
- **Reserved keys.** Add `ctrl+up`/`ctrl+down` (chat history) to `hardBreakReserved` (`hardbreak.go:20-28`).
- **Desktop notifications.** Replace `\n` with a space before `desktopNotifyString` is called (`app.go:1582`), so multi-line mention toasts don't glue words together.
- **Docs.** Add `docs/plan-chat-multiline.md` (this plan, stored when plan mode ends) and a v0.9.1 section in `docs/00-api-backlog.md`.

## Critical files
- `internal/ui/screens/shared.go`: chat input helpers
- `internal/ui/screens/hardbreak.go`: `hardBreaks` extraction
- `internal/ui/screens/cmail.go`, `internal/ui/screens/chatrooms.go`
- `internal/ui/screens/render.go`, `internal/ui/markdown/renderer.go`
- Tests: `shared_test.go`, `hardbreak_test.go`, `cmail_test.go`, `chatrooms_test.go`, `render_test.go`, `markdown/renderer_test.go`

## Verification
After each step:
- `go test ./...`, `go vet ./...` and `staticcheck ./...`
- the grouped Group|Tests|Result table

Live checks after steps 3 and 4, in a real terminal:
- **Basic send.** In a cIRC room and a cMail conversation, type a 3-line message with alt+enter. The box grows, Enter sends, and the message renders on 3 lines for both sender and recipient.
- **Server normalisation.** Send `"a\n \n\nb"` and a 12-line paste, then compare the result with what the server returns. This confirms the server's exact rules: whitespace-only lines, collapse before join, trimming.
- **Long line.** A long single line grows the box as it wraps.
- **Arrow keys.** Up and down inside a multi-line input move the cursor; from the top row they enter browsing.
- **Mentions.** In cIRC, type `@ra` on line 2 and mid-text. The hint line appears, Tab cycles, and Space accepts.
- **`/art`.** Post 20 lines with blank lines. Known risk: `filterAmbiguousKeyMsg` (`shared.go:31`) already replaces box-drawing characters with spaces.
- **Styling.** A styled cIRC message with a newline (the server turns it into spaces). cMail lists, blockquotes and code fences with line breaks.
- **Layout.** Inline images near the bottom while the box grows or the hint toggles. Short and narrow terminals.
- **Terminal quirks.** alt+enter is fullscreen in Windows Terminal; macOS Terminal needs Option-as-Meta.

## Implementation notes (changes from the plan)
- Step 1: no separate `hardBreaks` table test; `TestConvertPaste` already runs every case through it.
- Step 2: the newline check treats the line the cursor lands on as text in progress, otherwise typing a paragraph break ("a", newline, newline, "b") was refused.
- Step 3: Space with a bare `@` commits the hinted candidate, as before this feature. Refusing it broke picking a name with Tab after a bare `@`.
- Step 4, after live testing:
  - The textarea edits at the full 5-row height and `fitChatInput` shrinks it afterwards. Editing at the current height scrolled line 1 away as soon as line 2 appeared.
  - The `> ` prompt shows only on the first row of each line, so wrapped rows read as the line they will be sent as.
  - Wrapped rows are measured with a scratch textarea (textarea's word-wrap is unexported), so the box height is exact rather than estimated.
- Step 5: line breaks in desktop notifications become spaces in `stripControl` itself, which covers every toast, not only cIRC mentions.
