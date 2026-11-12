# Plan: inline @mention suggestion variant (cIRC), for comparison with the hint line

## Context
`feature/chat-multiline` (tip `8b91726`) ships multi-line cIRC and cMail input. On it, @mention suggestions appear on a hint row above the compose box. You want a second version where the suggestion is drawn fully inline: grey text right after the cursor, on any line and at any cursor position, with the edge cases covered. You'll run both versions side by side and pick one for `dev`, then `main` and a release.

This plan has been reviewed against the code and the bubbles v1.0.0 textarea source. The review findings are folded in.

## Git setup (step 0)
- Branch `feature/chat-multiline-inline` from `feature/chat-multiline` (`8b91726`). Add a worktree for it at `/home/ragnar/git/cyber-tui-inline`.
  - Work there using absolute paths.
  - The worktree has no `CLAUDE.md`, `.claude/` or `.env`, because they're gitignored. That's fine for this work.
  - The main checkout stays on the hint-line version. It can't check out the inline branch while the worktree holds it.
- First commit on the branch: this plan as `docs/plan-chat-multiline-inline.md`.
- Build each version with `go build -o cyber-tui ./cmd/cyber-tui`. `.gitignore` already ignores `/cyber-tui` in each worktree.
  - `/home/ragnar/git/cyber-tui/cyber-tui` is the hint-line version.
  - `/home/ragnar/git/cyber-tui-inline/cyber-tui` is the inline version.
- After you decide:
  - The PR into `dev` comes from the chosen branch. The inline branch already contains every hint-line commit.
  - The other branch and the worktree get removed, with your confirmation.

## Why a custom input renderer
`textarea` draws itself in one pass (`textarea.go:1093-1199`) with no hook between characters. Which row the cursor is drawn on depends on the textarea's private scroll position. That value only changes against content from the last `View()` call (`viewport.go:137, 255-263`), so we can't reliably track it.

The review also looked at two simpler alternatives and rejected both:
- Mirroring the private scroll position.
- Making the textarea as tall as its content and splicing only the cursor row of `ta.View()`.

So cIRC draws the compose box itself. The textarea stays the editing engine (value, cursor, keys, blink). Our code draws the rows and owns the scroll offset. This also fixes the shrink limitation recorded in a `ponytail:` comment.

## Reuse
- **`fitChatInput`'s scratch-textarea measurement** (`internal/ui/screens/shared.go`). It's extended to return each wrapped row's text as well as the count.
- **`chatInputOffset`, `mentionActiveCandidate`, Tab cycling and Space commit**, unchanged.
- **`matchMentionCandidates`** already guarantees a case-insensitive prefix match (`chatrooms.go:2329-2338`), and usernames are ASCII. So the ghost text is simply `candidate[len(query runes):]`.
- **The per-line `> ` prompt rule** and **the `Update`-wrapper choke point**.
- **The old ghost cursor trick** (git history at `6ef64fc`). The cursor cell shows the ghost's first character, with `Cursor.TextStyle = theme.Subtle`, so it stays grey during the blink-off phase.
- **`ta.View()` itself for an empty value.** It keeps textarea's own placeholder rendering.

## Steps
For each step: implement it, run `go test ./...`, `go vet ./...` and `staticcheck ./...`, show the results table, then wait for approval before committing.

### 1. Custom compose-box renderer for cIRC (no ghost yet; looks the same as today)
- **`chatInputRows(ta)`** in `shared.go` returns the display rows: line index, row text (as runes), and whether the row is the first of its line. `fitChatInput` uses its length.
  - Rows are listed per line with a one-line scratch textarea, `probe.SetWidth(ta.Width())`.
  - Take the first `LineInfo()`'s `Height` as the row count, then step `SetCursor(start + width)` through it. Loop a fixed `Height` times rather than stopping on a column check, because `SetCursor` clamps and the loop could otherwise spin forever.
  - Slice the row text as `line[start : min(start+width, len(line))]`. textarea's `wrap` adds an extra space to every line's last row (`textarea.go:1452-1457`), so `Width` is one more than the runes left on that row.
  - Map every `unicode.IsSpace` rune to `' '`, matching how `wrap` emits spaces. Ideographic space and NBSP would otherwise change widths.
- **`renderChatInput(ta, yOffset, ghost) (string, int)`** in `shared.go` returns the drawn box and the offset it used.
  - Empty value: return `ta.View()`. Reset and SetValue put the textarea's own scroll back at the top, so this is safe.
  - It computes the rows once, then calls `chatInputScroll` itself. That way a `View()` straight after `SetValue` without an `Update` is still correct.
  - Each visible row is the prompt (`> `, or 2 spaces on a wrapped row) plus the row text. On the cursor row it's the text before the cursor, then `ta.Cursor.View()` with the character under the cursor (`" "` past the end), then the rest.
  - Every row is padded or cut to the prompt width plus `ta.Width()`, exactly as textarea's viewport does. textarea's trailing-space trim and full-row cursor branches can't occur and are left out.
  - Blurred and browsing states need no separate code. `Cursor.Blur()` already makes `Cursor.View()` draw plain text.
- **`chatInputScroll(y, cursorRow, h, total) int`**: `y = clamp(y, cursorRow-h+1, cursorRow)`, then `min(y, max(0, total-h))`.
- **cIRC wiring.**
  - `ChatroomsModel.inputYOffset` stores the offset returned by the render. Its `View()` uses the renderer in place of `m.input.View()`.
  - The cursor row is the number of wrapped rows before the cursor's line plus `LineInfo().RowOffset`. This matches textarea's `cursorLineNumber` (`textarea.go:1318-1327`).
- **cMail stays on `ta.View()`.** Reword the `ponytail:` comment in `fitChatInput` to say the shrink limitation now applies to cMail only. `updateChatInput`'s full-height editing stays for cMail.
- **Tests (`shared_test.go`):**
  - An equivalence table: `renderChatInput` must equal `ta.View()` (ANSI-stripped) after `fitChatInput`, for at most 5 rows and nothing scrolled. Cases:
    - one line;
    - multi-line;
    - an empty middle line (`"a\n\nb"`), which must not panic;
    - a wrapped line;
    - a line exactly as wide as the box;
    - a spaces-only line longer than the width;
    - a single word longer than the width (hard split);
    - cursor mid-row;
    - wide characters;
    - U+3000 and NBSP;
    - blurred;
    - placeholder, focused and blurred.
  - Scrolling, tested separately:
    - shrinking from 7 lines to 5 shows line 1 again;
    - `setInput` with 7 lines, then `View()` without `Update`, gives exactly `ta.Height()` rows, each at box width;
    - `Reset` after sending from a scrolled box gives offset 0 and 1 row.
  - The existing cIRC width tests and `inputBoxLines` tests keep passing.

### 2. Inline ghost text replaces the hint line
- **`mentionGhost()`** (cIRC) returns `candidate[len([]rune(query)):]`. It returns `""` when:
  - there's no candidate;
  - the name is already fully typed;
  - the input isn't focused (this covers browsing, the flag prompt and confirm-delete);
  - the rune after the cursor is a username character `[A-Za-z0-9_]`. With the cursor inside `@a|l`, a ghost would read `@alicel`. `@al,` and `@al there` still get a ghost.
- **Cursor row with a ghost.**
  - Draw the text before the cursor, then the cursor cell showing the ghost's first character in `theme.Subtle`, then the rest of the ghost in `theme.Subtle`, then the rest of the typed row shifted right.
  - At the end of a line, the cursor sits on wrap's extra space, so the ghost's first character reuses that cell.
- **Overflow rule: typed text is never hidden.**
  - Free cells = `ta.Width()` minus the width of the row's real runes (the extra space excluded).
  - The ghost is cut to fit the free cells. With no free cell, no ghost is drawn, and Tab/Space still work.
  - Known and accepted: trailing wrap spaces count as typed text, so a row ending in a space can hide a ghost that would only push a space off.
- **Delete the hint line.** Remove `mentionHint`, its row in `View()` and its `h--` in `viewportHeight()`. As a side effect, the message area no longer changes height when a suggestion appears, which is better for the scroll position and the unread badge.
- **Update comments** that mention "hint line" or "the mention hint toggling" (`chatrooms.go:1116`, `mentionActiveCandidate`, the `mentionPreview` test helper).
- **Tests, one per edge case:**
  1. Ghost at the end of a one-line message.
  2. Ghost mid-text: `"@al,"` draws the ghost and shifts the comma right; nothing typed is cut off.
  3. Cursor inside the token (`@a|l`): no ghost.
  4. Cursor on a wrapped row of a long line.
  5. Cursor on line 2 after a newline.
  6. Line exactly as wide as the box: the cursor and ghost land on the next row at column 0.
  7. Box scrolled past 5 rows: the ghost is on the visible cursor row.
  8. Full row: the ghost is cut, or hidden when no cell is free.
  9. Ghost on the row with the extra end-of-line space: the total width equals the box width.
  10. Wide characters before the cursor: the ghost lands in the right column.
  11. A bare `@`: the ghost is the whole first name, and the row still fits.
  12. Two `@` tokens: the ghost appears only for the token at the cursor.
  13. Blink-off phase: the overlaid character is still grey (the old `CursorOverlayUsesGhostColor` test, restored).
  14. Every row is exactly the box width, with and without a ghost, so nothing jitters.
  15. Uppercase `@AL` shows `ice`; a fully typed name shows nothing; browsing (blurred) shows nothing.
  - `TestMentionHint_ShowsAboveInputAndShrinksViewport` (`chatrooms_test.go:1318`) is the only test that breaks. It becomes a ghost test. The mention-logic tests stay unchanged.

### 3. Docs
Add an "Implementation notes" section to `docs/plan-chat-multiline-inline.md` for anything that changed during the work.

## Critical files (in the worktree)
- `internal/ui/screens/shared.go`: row listing, renderer, scroll rule
- `internal/ui/screens/chatrooms.go`: `inputYOffset`, `View`, `viewportHeight`, `mentionGhost`, removing the hint
- `internal/ui/screens/shared_test.go`, `internal/ui/screens/chatrooms_test.go`

## Verification
- Each step: `go test ./...`, `go vet ./...`, `staticcheck ./...`, and the grouped results table.
- Step 1 live: the inline build behaves exactly like the hint build in cIRC, plus the shrink fix.
- Step 2 live, side by side (`/home/ragnar/git/cyber-tui/cyber-tui` and `/home/ragnar/git/cyber-tui-inline/cyber-tui`):
  - `@ra` at the end of a line, mid-text before a comma, inside a token, on line 2, on a wrapped row, and with the box scrolled.
  - Tab cycling and Space accept.
  - A narrow terminal.
  - Watch for jitter or a wrong column while the cursor blinks.

## Implementation notes
- `renderChatInput` returns only the drawn box, not the offset. The offset is stored by `scrollChatInput`, called from the cIRC `Update` wrapper. `renderChatInput` still applies `chatInputScroll` itself, so a `View()` straight after `SetValue` without an `Update` stays correct.
- The rows record only the text and whether a row is the first of its line. The line index from the plan was not needed.
- The ghost is cut to the free cells before the row is padded or cut to the box width. That final cut hides a missing ghost limit in the "ghost cut" test, so a test with typed text after the cursor was added: without the limit, that text gets pushed off the row and the test fails. Both this case and the inside-a-name rule were checked by breaking the code on purpose.
- Two test comments still said "hint line" and now say "ghost text".
