# Feature 26 — Markdown Rendering

Post content, replies, C-Mail messages, and journal notes are stored as GitHub Flavored Markdown (GFM). This feature renders that markdown as styled ANSI text in the TUI instead of displaying raw syntax.

---

## Package

`internal/ui/markdown` — new package, no circular dependencies.

### Exported API

```go
// Render parses content as GFM and returns an ANSI-styled string for viewport display.
// width is the inner content width (after border/padding) for word-wrapping.
func Render(content string, width int) string

// FirstLine strips markdown syntax and returns the first non-empty line as plain text.
// Use this for compact single-line previews (bookmarks, profile list items).
func FirstLine(content string) string
```

---

## Library Choice

`github.com/yuin/goldmark` — direct goldmark AST walker, not glamour.

**Why not glamour:**
- The app has three runtime-switchable themes (cyber/c64/vt320). The goldmark walker reads `theme.ColorGreen`, `theme.ColorCyan` etc. at render time; theme switching is free with no reconfiguration.
- Glamour adds top/bottom margin around documents. The existing 4-line feed truncation slices on `\n` and would interact badly with glamour's extra whitespace.
- The `@mention` extension requires a custom goldmark inline parser regardless.
- `github.com/yuin/goldmark` is a zero-transitive-dependency library vs. glamour's ~10 extra packages.

---

## Visual Rendering Reference

| Markdown element | TUI rendering |
|---|---|
| `**bold**` | Bold + `ColorGreen` |
| `*italic*` | Italic + `ColorGreen` |
| `[text](url)` | `text` underlined in `ColorCyan` |
| `[](url)` | URL underlined in `ColorCyan` |
| `![alt](url)` | `[IMG: alt]` (subtle) + underlined URL |
| `@username` | `theme.Highlight` (yellow bold) |
| `# H1` | Uppercase `ColorCyan` bold + `═══` line in `ColorDimGreen` |
| `## H2` | `ColorCyan` bold + `───` line in `ColorDimGreen` |
| `### H3` | `ColorCyan` bold, no separator |
| `` `code` `` | `ColorYellow` foreground |
| ` ```block``` ` | `ColorYellow` + `│` left gutter in `ColorDimGreen` per line |
| `> quote` | `│ ` gutter in `ColorDimGreen` + paragraph content |
| `- item` / `1. item` | `• item` / `1. item` (nested lists: 2-space indent per level) |
| `---` | `─────` in `ColorMuted` (full inner width) |
| ~~strike~~ | Strikethrough ANSI + `ColorMuted` |
| GFM table | Cells separated by ` │ `, header row in `theme.Title` |

---

## @mention Extension

`@username` patterns are parsed by a custom goldmark inline extension (`mentionParser`). Rules:
- Trigger character: `@`
- Valid username: `[a-zA-Z0-9_]{1,30}` following `@`
- Rendered as `theme.Highlight.Render("@username")` (yellow bold)
- Mentions inside fenced code blocks and code spans are NOT highlighted (handled correctly by goldmark's inline parse pipeline)

---

## Integration Points

### Full rendering (viewport content)

| Screen | Function | Notes |
|---|---|---|
| Feed | `RenderPost` in `shared.go` | 4-line truncation applied after render |
| Post detail | `renderFullPost`, `renderReply` in `postdetail.go` | Full content, no truncation |
| Journal | `buildRevisionPreviewContent` in `journal.go` | Full note content |
| C-Mail | `renderMessages` in `cmail.go` | Body width = viewport width |
| Chatrooms | `renderMessages` in `chatrooms.go` | Body width = viewport width |

### FirstLine only (compact list rows)

| Screen | Function | Notes |
|---|---|---|
| Bookmarks | `renderItem` in `bookmarks.go` | Strips markdown before collapse/truncate |
| Profile Posts | `renderPostItem` in `profile.go` | Strips markdown before truncateStr |
| Profile Replies | `renderReplyItem` in `profile.go` | Strips markdown before truncateStr |
| Journal list | `renderNote` in `journal.go` | Strips markdown for title row |
| Journal revisions | `buildRevisionListContent` in `journal.go` | Strips markdown for preview row |

---

## 4-Line Truncation Compatibility

The existing truncation logic in `RenderPost` splits rendered output on `"\n"` and slices to `postMaxBodyLines = 4`. This works safely with ANSI-coded goldmark output because:
- Lipgloss closes all ANSI sequences at the end of each styled span, never across a newline.
- Goldmark emits block-level elements separated by `"\n"`.

No changes were needed to the truncation logic.

---

## Width Handling

- `Render(content, width)` is called with `innerWidth = totalWidth - 4` (border + padding) everywhere.
- Paragraph word-wrapping uses `lipgloss.NewStyle().Width(width).Render(inline)`.
- Blockquotes use `width - 2` for inner content (to account for the `│ ` gutter).
- Code block lines are not word-wrapped (code is rendered verbatim per source line).
- When `width ≤ 0`, a safe default of 80 is used.

---

## Unicode Width Normalisation — `stripAmbiguousRunes`

All content passes through `stripAmbiguousRunes` before parsing. This function is the single point of truth for making sure what the **code measures** and what the **terminal renders** agree. Without it, post content containing certain Unicode characters causes rendered lines to overflow their lipgloss boxes by one column, which causes the terminal to wrap the overflow to a new line and corrupts all height-based layout calculations (bookmarks card height, postdetail scroll position).

### Three measurement layers

There are three independent systems that each give a "column width" for a string, and they do not always agree:

| Layer | Library | How it works |
|---|---|---|
| **`runewidth.RuneWidth(r)`** | `mattn/go-runewidth` | Looks up the EastAsianWidth Unicode property for a single rune in isolation. Returns 0, 1, or 2. |
| **`runewidth.StringWidth(s)`** | `mattn/go-runewidth` | Iterates grapheme clusters using the Unicode grapheme break algorithm. Returns the sum of each cluster's width. A grapheme-extend rune that follows another rune is treated as part of that cluster and contributes 0. |
| **`lipgloss.Width(s)`** / `ansi.StringWidth(s)` | `charmbracelet/x/ansi` → `clipperhouse/displaywidth` | Strips ANSI, then processes bytes. **ASCII bytes are always counted individually** (they never enter grapheme-cluster mode). Non-ASCII bytes trigger `FirstGraphemeCluster(remainingBytes)`, which may absorb subsequent non-ASCII extending runes into the same cluster. |
| **Terminal `wcwidth`** | glibc / OS | Called per codepoint, no grapheme-cluster awareness. Returns 1 for every halfwidth character regardless of context. |

### The `ﾟ` (U+FF9F) problem

`ﾟ` (HALFWIDTH KATAKANA VOICED ITERATION MARK) has Unicode property `GCB=Extend` — it is a grapheme-cluster extender that attaches to the preceding grapheme cluster. This creates a three-way inconsistency:

```
"･ﾟ"  (U+FF65 HALFWIDTH KATAKANA MIDDLE DOT  +  U+FF9F)

  runewidth.RuneWidth('ﾟ')      = 1   ← counted as 1 in isolation (wrong for layout)
  runewidth.StringWidth("･ﾟ")   = 1   ← ﾟ extends ･, cluster width = 1  ✓ (per Unicode)
  lipgloss.Width("･ﾟ")          = 1   ← ･ is non-ASCII, triggers FirstGraphemeCluster("･ﾟ"),
                                         returns the pair as one cluster, width = 1  ✓
  terminal wcwidth per codepoint = 2   ← wcwidth('･')=1 + wcwidth('ﾟ')=1 = 2  ✗ mismatch!
```

The critical case: when `ﾟ` **follows a non-ASCII character**, `lipgloss.Width` and `runewidth.StringWidth` both say 1, but the terminal renders 2 columns.

When `ﾟ` **follows an ASCII character**, lipgloss.Width correctly says 2 (ASCII is counted individually, then `ﾟ` is a standalone non-ASCII cluster of width 1), which agrees with the terminal. No overflow in this case.

The overflow case (`ﾟ` after non-ASCII) happens because:
1. `Render(content, w)` calls `lipgloss.NewStyle().Width(w).Render(inline)`
2. lipgloss measures `inline` as `w-1` columns (under-counts the `ﾟ`)
3. lipgloss pads 1 extra trailing space to reach `w`
4. The terminal renders `inline` as `w` columns (from wcwidth) **plus** the extra space = `w+1`
5. The terminal wraps the final character to a new line
6. Height calculations based on the rendered block are now wrong by 1 line

This broke bookmarks card rendering (5-line height guarantee violated) and postdetail scroll calculations for any post containing kaomoji patterns like `･ﾟ` or `｡ﾟ`.

### The detection condition

`stripAmbiguousRunes` uses `runewidth.StringWidth("x"+string(r)) < 1+runewidth.RuneWidth(r)` to detect grapheme-extend modifiers:

- For `ﾟ`: `runewidth.StringWidth("xﾟ")` = 1 (ﾟ extends x → contributes 0), `1+RuneWidth('ﾟ')` = 2. `1 < 2` → **strip**.
- For `a`: `runewidth.StringWidth("xa")` = 2 (normal), `1+1` = 2. `2 < 2` is false → keep.
- For double-wide CJK: caught earlier by the `rw > 1` branch, never reaches this check.
- For ambiguous-width symbols: caught by `runewidth.IsAmbiguousWidth(r)`, never reaches this check.

The `"x"` prefix is a stable ASCII base. Any rune `r` that reduces `StringWidth("x"+string(r))` below `1+RuneWidth(r)` is a rune that the grapheme cluster algorithm treats as a zero-width extension of the preceding character — but which the terminal would render with nonzero width. Such runes are stripped before the markdown parser sees the content.

### Full normalisation table

`stripAmbiguousRunes` resolves content into three outcomes:

| Condition | Action | Examples |
|---|---|---|
| `r == '\t' \|\| r == '\n'` | Keep — goldmark needs these | Tabs, newlines |
| `rw == 0` | Strip — genuinely invisible | `\r`, ESC, U+200B zero-width space, bidi marks |
| `runewidth.StringWidth("x"+string(r)) < 1+rw` | Strip — grapheme extender that the terminal renders wider than tools measure | `ﾟ` U+FF9F, `ﾞ` U+FF9E |
| `runewidth.IsAmbiguousWidth(r)` | Replace with space — Unicode EAW=A | `°` U+00B0, `∩` U+2229, `★` U+2605 |
| Everything else | Keep as-is | ASCII, halfwidth katakana `ﾉ･｡`, Latin, Cyrillic, CJK `ヮ` U+30EE, fullwidth `Ａ` U+FF21 |

**Why replace ambiguous-width with space rather than stripping?** A space preserves the visual blank — readers understand there was a character there. Stripping would silently collapse surrounding text (e.g. `foo★bar` → `foobar`).

**Why strip grapheme extenders rather than replacing?** An extender such as `ﾟ` has no visual presence of its own; it modifies the preceding character. Replacing it with a space would introduce a spurious gap that wasn't in the original text.

**Why pass double-wide characters (CJK, fullwidth) through unchanged?** All three measurement layers — `runewidth.RuneWidth`, `runewidth.StringWidth`, `lipgloss.Width`, and terminal `wcwidth` — agree that double-wide characters occupy exactly 2 columns. There is no measurement mismatch and therefore no overflow risk. Replacing them with spaces would degrade kaomoji and CJK text for no benefit.

---

## Theme Compatibility

Styles are built from `theme.*` color and style vars at render time. When the user switches theme via settings, the next render automatically uses the new colors — no renderer reconfiguration required.

---

## Tests

`internal/ui/markdown/renderer_test.go` — 33 unit tests covering:
- Empty input, plain text passthrough
- Bold, italic, links, images, inline code, code blocks, blockquotes
- Bullet lists, ordered lists, H1/H2/H3, horizontal rule
- `@mention` parsing, mention inside code block (no highlight), mention inside blockquote
- Strikethrough, ambiguous-rune stripping, halfwidth katakana modifier stripping, double-wide character preservation
- ANSI truncation safety, theme switching, width clamping
- `FirstLine`: plain text, heading strip, markdown strip, empty line skip, no-ANSI assertion

Tests force `lipgloss.SetColorProfile(termenv.TrueColor)` in `TestMain` so ANSI codes are emitted in a non-TTY test environment.
