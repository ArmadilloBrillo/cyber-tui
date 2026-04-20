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

## Theme Compatibility

Styles are built from `theme.*` color and style vars at render time. When the user switches theme via settings, the next render automatically uses the new colors — no renderer reconfiguration required.

---

## Tests

`internal/ui/markdown/renderer_test.go` — 31 unit tests covering:
- Empty input, plain text passthrough
- Bold, italic, links, images, inline code, code blocks, blockquotes
- Bullet lists, ordered lists, H1/H2/H3, horizontal rule
- `@mention` parsing, mention inside code block (no highlight), mention inside blockquote
- Strikethrough, ambiguous-rune stripping
- ANSI truncation safety, theme switching, width clamping
- `FirstLine`: plain text, heading strip, markdown strip, empty line skip, no-ANSI assertion

Tests force `lipgloss.SetColorProfile(termenv.TrueColor)` in `TestMain` so ANSI codes are emitted in a non-TTY test environment.
