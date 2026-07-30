package markdown

import (
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
)

func TestMain(m *testing.M) {
	// Force 24-bit color so ANSI codes are emitted even without a real TTY.
	lipgloss.SetColorProfile(termenv.TrueColor)
	theme.Set("cyber")
	os.Exit(m.Run())
}

// strip removes ANSI CSI escape sequences (ESC[...m) from s for plain-text
// content checking. Used by tests that need to assert on visible text content
// without caring about styling codes.
func strip(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			if i < len(s) {
				i++ // skip the 'm'
			}
		} else {
			b.WriteByte(s[i])
			i++
		}
	}
	return b.String()
}

func TestRender_EmptyInput(t *testing.T) {
	if got := Render("", 80); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
	if got := Render("   \n\n  ", 80); got != "" {
		t.Errorf("expected empty string for whitespace-only input, got %q", got)
	}
}

func TestRender_PlainText(t *testing.T) {
	out := strip(Render("Hello world", 80))
	if !strings.Contains(out, "Hello world") {
		t.Errorf("plain text not preserved in output: %q", out)
	}
}

func TestRender_Bold(t *testing.T) {
	raw := Render("**bold text**", 80)
	if !strings.Contains(strip(raw), "bold text") {
		t.Errorf("bold text not in output: %q", strip(raw))
	}
	if !strings.Contains(raw, "\x1b[1m") && !strings.Contains(raw, "\x1b[1;") {
		t.Errorf("no bold ANSI code in output: %q", raw)
	}
	if strings.Contains(strip(raw), "**") {
		t.Errorf("raw ** markers remain in output: %q", strip(raw))
	}
}

func TestRender_Italic(t *testing.T) {
	raw := Render("*italic text*", 80)
	if !strings.Contains(strip(raw), "italic text") {
		t.Errorf("italic text not in output: %q", strip(raw))
	}
	if strings.Contains(strip(raw), "*italic text*") {
		t.Errorf("raw * markers remain in output")
	}
}

func TestRender_LinkWithText(t *testing.T) {
	raw := Render("[click here](https://example.com)", 80)
	if !strings.Contains(strip(raw), "click here") {
		t.Errorf("link text not in output: %q", strip(raw))
	}
	if strings.Contains(strip(raw), "[click here](") {
		t.Errorf("raw link syntax remains in output")
	}
	if !strings.Contains(raw, "\x1b[") {
		t.Errorf("no ANSI codes in link output")
	}
}

func TestRender_LinkBareDest(t *testing.T) {
	raw := Render("[](https://example.com)", 80)
	if !strings.Contains(strip(raw), "https://example.com") {
		t.Errorf("URL not in output for empty link text: %q", strip(raw))
	}
}

func TestRender_Image(t *testing.T) {
	raw := Render("![alt text](https://example.com/img.png)", 80)
	plain := strip(raw)
	if !strings.Contains(plain, "[IMG: alt text]") {
		t.Errorf("IMG label not in output: %q", plain)
	}
	// URL should NOT be shown inline — too noisy for long CDN links
	if strings.Contains(plain, "https://example.com/img.png") {
		t.Errorf("image URL should not appear in output: %q", plain)
	}
	if strings.Contains(plain, "![alt text](") {
		t.Errorf("raw image syntax remains in output")
	}
}

func TestRender_ImageNoAlt(t *testing.T) {
	raw := Render("![](https://example.com/img.png)", 80)
	if !strings.Contains(strip(raw), "[IMG]") {
		t.Errorf("[IMG] label not in output: %q", strip(raw))
	}
}

func TestRender_InlineCode(t *testing.T) {
	raw := Render("Use `fmt.Println` here", 80)
	plain := strip(raw)
	if !strings.Contains(plain, "fmt.Println") {
		t.Errorf("inline code text not in output: %q", plain)
	}
	if strings.Contains(plain, "`fmt.Println`") {
		t.Errorf("raw backtick markers remain")
	}
	if !strings.Contains(raw, "\x1b[") {
		t.Errorf("no ANSI codes in inline code output")
	}
}

func TestRender_FencedCodeBlock(t *testing.T) {
	md := "```go\nfmt.Println(\"hello\")\n```"
	raw := Render(md, 80)
	plain := strip(raw)
	if !strings.Contains(plain, "fmt.Println") {
		t.Errorf("code block content not in output: %q", plain)
	}
	if !strings.Contains(raw, "│") {
		t.Errorf("gutter character │ not in code block output")
	}
	if strings.Contains(plain, "```") {
		t.Errorf("raw ``` markers remain in output")
	}
}

func TestRender_Blockquote(t *testing.T) {
	raw := Render("> quoted text", 80)
	plain := strip(raw)
	if !strings.Contains(plain, "quoted text") {
		t.Errorf("blockquote content not in output: %q", plain)
	}
	if !strings.Contains(raw, "│") {
		t.Errorf("blockquote gutter │ not in output")
	}
	if strings.Contains(plain, "> quoted text") {
		t.Errorf("raw > marker remains in output")
	}
}

func TestRender_BulletList(t *testing.T) {
	md := "- first item\n- second item\n- third item"
	raw := Render(md, 80)
	plain := strip(raw)
	if !strings.Contains(plain, "first item") {
		t.Errorf("list item text not in output: %q", plain)
	}
	if !strings.Contains(plain, "•") {
		t.Errorf("bullet character • not in output: %q", plain)
	}
}

func TestRender_OrderedList(t *testing.T) {
	md := "1. first\n2. second\n3. third"
	raw := Render(md, 80)
	plain := strip(raw)
	if !strings.Contains(plain, "first") || !strings.Contains(plain, "second") {
		t.Errorf("ordered list items not in output: %q", plain)
	}
	if !strings.Contains(plain, "1.") || !strings.Contains(plain, "2.") {
		t.Errorf("ordered list numbers not in output: %q", plain)
	}
}

func TestRender_H1(t *testing.T) {
	raw := Render("# My Heading", 40)
	plain := strip(raw)
	if !strings.Contains(plain, "MY HEADING") {
		t.Errorf("H1 text not uppercased in output: %q", plain)
	}
	if !strings.Contains(plain, "═") {
		t.Errorf("H1 separator ═ not in output: %q", plain)
	}
}

func TestRender_H2(t *testing.T) {
	raw := Render("## Section Header", 40)
	plain := strip(raw)
	if !strings.Contains(plain, "Section Header") {
		t.Errorf("H2 text not in output: %q", plain)
	}
	if !strings.Contains(plain, "─") {
		t.Errorf("H2 separator ─ not in output: %q", plain)
	}
}

func TestRender_H3(t *testing.T) {
	raw := Render("### Sub-heading", 40)
	plain := strip(raw)
	if !strings.Contains(plain, "Sub-heading") {
		t.Errorf("H3 text not in output: %q", plain)
	}
	if strings.Contains(plain, "═") || strings.Contains(plain, "─") {
		t.Errorf("H3 must not have a separator line: %q", plain)
	}
}

func TestRender_HorizontalRule(t *testing.T) {
	raw := Render("text\n\n---\n\nmore text", 40)
	if !strings.Contains(raw, "─") {
		t.Errorf("horizontal rule ─ not in output")
	}
}

func TestRender_Mention(t *testing.T) {
	raw := Render("Hello @alice and @bob_123!", 80)
	plain := strip(raw)
	if !strings.Contains(plain, "@alice") {
		t.Errorf("@alice mention not in output: %q", plain)
	}
	if !strings.Contains(plain, "@bob_123") {
		t.Errorf("@bob_123 mention not in output: %q", plain)
	}
	if !strings.Contains(raw, "\x1b[") {
		t.Errorf("no ANSI codes in mention output")
	}
}

func TestRender_MentionInCodeBlockNotHighlighted(t *testing.T) {
	md := "```\n@alice is here\n```"
	raw := Render(md, 80)
	if !strings.Contains(strip(raw), "@alice") {
		t.Errorf("@alice not in code block output: %q", strip(raw))
	}
	if !strings.Contains(raw, "│") {
		t.Errorf("code block gutter missing")
	}
}

func TestRender_MentionInBlockquote(t *testing.T) {
	raw := Render("> Reply by @charlie", 80)
	plain := strip(raw)
	if !strings.Contains(plain, "@charlie") {
		t.Errorf("@charlie not in blockquote output: %q", plain)
	}
	if !strings.Contains(raw, "│") {
		t.Errorf("blockquote gutter missing")
	}
}

func TestRender_Strikethrough(t *testing.T) {
	raw := Render("~~deleted text~~", 80)
	plain := strip(raw)
	if !strings.Contains(plain, "deleted text") {
		t.Errorf("strikethrough text not in output: %q", plain)
	}
	// Raw ~~ markers must not appear in plain text output
	if strings.Contains(plain, "~~deleted text~~") {
		t.Errorf("raw strikethrough markers remain: %q", plain)
	}
}

func TestRender_AmbiguousRunesStripped(t *testing.T) {
	// U+00B0 DEGREE SIGN (°) is ambiguous-width per go-runewidth.
	raw := Render("Hello \u00B0 world", 80)
	if strings.ContainsRune(strip(raw), '\u00B0') {
		t.Errorf("ambiguous rune U+00B0 should be stripped: %q", strip(raw))
	}
	if !strings.Contains(strip(raw), "Hello") {
		t.Errorf("surrounding text not preserved: %q", strip(raw))
	}
}

func TestRender_TypographicPunctPreserved(t *testing.T) {
	cases := []struct {
		r    rune
		name string
	}{
		{'‘', "LEFT SINGLE QUOTATION MARK"},
		{'’', "RIGHT SINGLE QUOTATION MARK"},
		{'“', "LEFT DOUBLE QUOTATION MARK"},
		{'”', "RIGHT DOUBLE QUOTATION MARK"},
		{'–', "EN DASH"},
		{'—', "EM DASH"},
		{'…', "HORIZONTAL ELLIPSIS"},
	}
	for _, tc := range cases {
		raw := Render("Hello "+string(tc.r)+" world", 80)
		if !strings.ContainsRune(strip(raw), tc.r) {
			t.Errorf("%s (U+%04X) should be preserved: %q", tc.name, tc.r, strip(raw))
		}
	}
}

func TestRender_HalfwidthKatakanaModifierStripped(t *testing.T) {
	// U+FF9F HALFWIDTH KATAKANA VOICED ITERATION MARK has GCB=Extend:
	// go-runewidth says width=1 (not ambiguous), but rivo/uniseg (used by lipgloss)
	// measures it as 0. It must be stripped so layout width calculations are consistent.
	raw := Render("kaomoji ﾟ test", 80)
	if strings.ContainsRune(strip(raw), 'ﾟ') {
		t.Errorf("grapheme-extend modifier U+FF9F should be stripped: %q", strip(raw))
	}
	if !strings.Contains(strip(raw), "kaomoji") {
		t.Errorf("surrounding text not preserved: %q", strip(raw))
	}
}

func TestRender_CfFormatCharacterStripped(t *testing.T) {
	// U+06DD ARABIC END OF AYAH is a Unicode Cf (Format) character. Terminal fonts
	// lack a glyph for it and substitute a wide fallback (enclosing mark / tofu box)
	// that overflows the measured column count. It must be stripped.
	raw := Render("hello \u06dd world", 80)
	if strings.ContainsRune(strip(raw), '\u06dd') {
		t.Errorf("Cf character U+06DD should be stripped: %q", strip(raw))
	}
	if !strings.Contains(strip(raw), "hello") || !strings.Contains(strip(raw), "world") {
		t.Errorf("surrounding text not preserved: %q", strip(raw))
	}
}

func TestRender_SpacingModifierLetterReplacedWithSpace(t *testing.T) {
	// U+02D5 MODIFIER LETTER DOWN TACK is in the Spacing Modifier Letters block
	// (U+02B0–U+02FF). Terminal fonts substitute it with a wide fallback glyph
	// (e.g. ▼) that overflows the measured column count. It must become a space.
	raw := Render("hello ˕ world", 80)
	if strings.ContainsRune(strip(raw), '˕') {
		t.Errorf("Spacing Modifier Letter U+02D5 should be replaced: %q", strip(raw))
	}
	if !strings.Contains(strip(raw), "hello") || !strings.Contains(strip(raw), "world") {
		t.Errorf("surrounding text not preserved: %q", strip(raw))
	}
}

func TestRender_KaomojiCardHeight(t *testing.T) {
	// Regression: reply HYZ9p6qMWRynM608LhhS contained U+06DD and U+02D5 which
	// caused terminal line overflow, displacing the card's bottom border.
	content := "༼;´༎ຶ \u06dd ༎ຶ༽ ---- (•˕ •マ.ᐟ"
	out := Render(content, 76)
	if h := lipgloss.Height(out); h != 1 {
		t.Errorf("kaomoji should render as 1 line, got %d: %q", h, out)
	}
}

func TestRender_DoubleWidthPreserved(t *testing.T) {
	// Double-wide characters (CJK, fullwidth) must pass through unchanged:
	// runewidth, lipgloss, and terminal wcwidth all agree on 2 columns, so
	// they render correctly inside lipgloss width-constrained boxes.
	raw := Render("kaomoji ヮ test", 80)
	if !strings.ContainsRune(strip(raw), 'ヮ') {
		t.Errorf("double-wide rune U+30EE should be preserved: %q", strip(raw))
	}
}

func TestRender_TruncationSafeANSI(t *testing.T) {
	md := "Para one.\n\nPara two.\n\nPara three.\n\nPara four.\n\nPara five."
	out := Render(md, 80)
	lines := strings.Split(out, "\n")
	if len(lines) < 4 {
		t.Skipf("output has fewer than 4 lines: %d", len(lines))
	}
	first4 := strings.Join(lines[:4], "\n")
	// No ESC byte should appear without a following '[' (that would be a truncated sequence).
	for i := 0; i < len(first4)-1; i++ {
		if first4[i] == '\x1b' && first4[i+1] != '[' {
			t.Errorf("potentially broken ANSI sequence at byte %d", i)
		}
	}
}

func TestRender_ThemeSwitching(t *testing.T) {
	theme.Set("cyber")
	out1 := Render("@user", 80)

	theme.Set("c64")
	out2 := Render("@user", 80)

	theme.Set("cyber") // restore
	out3 := Render("@user", 80)

	for _, o := range []string{out1, out2, out3} {
		if !strings.Contains(strip(o), "@user") {
			t.Errorf("mention missing after theme switch: %q", strip(o))
		}
	}
	// With forced TrueColor, different themes produce different ANSI colors.
	if out1 == out2 {
		t.Errorf("theme switching had no effect: cyber and c64 outputs are identical")
	}
	// Restoring cyber should reproduce the original output.
	if out1 != out3 {
		t.Errorf("restore to cyber produced different output:\n  first:   %q\n  restored: %q", out1, out3)
	}
}

func TestRender_WidthRespected(t *testing.T) {
	long := strings.Repeat("word ", 30)
	out := Render(long, 40)
	for _, line := range strings.Split(out, "\n") {
		w := lipgloss.Width(line)
		if w > 42 {
			t.Errorf("line exceeds width 40 (got %d): %q", w, line)
		}
	}
}

func TestFirstLine_PlainText(t *testing.T) {
	if got := FirstLine("Hello world"); got != "Hello world" {
		t.Errorf("FirstLine(%q) = %q, want %q", "Hello world", got, "Hello world")
	}
}

func TestFirstLine_StripsHeading(t *testing.T) {
	if got := FirstLine("# My Title\nSecond line"); got != "My Title" {
		t.Errorf("FirstLine should strip heading marker: got %q", got)
	}
}

func TestFirstLine_StripsMarkdown(t *testing.T) {
	got := FirstLine("**bold** and *italic* text")
	if strings.Contains(got, "**") || strings.Contains(got, "*") {
		t.Errorf("FirstLine should strip markdown markers: got %q", got)
	}
	if !strings.Contains(got, "bold") || !strings.Contains(got, "italic") {
		t.Errorf("FirstLine should preserve text content: got %q", got)
	}
}

func TestFirstLine_SkipsEmptyLines(t *testing.T) {
	if got := FirstLine("\n\n\nActual content"); got != "Actual content" {
		t.Errorf("FirstLine should skip empty lines: got %q", got)
	}
}

func TestFirstLine_EmptyInput(t *testing.T) {
	if got := FirstLine(""); got != "" {
		t.Errorf("FirstLine(\"\") = %q, want \"\"", got)
	}
}

func TestFirstLine_NoANSI(t *testing.T) {
	got := FirstLine("**bold** text with [link](https://example.com)")
	if strings.Contains(got, "\x1b") {
		t.Errorf("FirstLine must not contain ANSI codes: %q", got)
	}
}

func TestFirstLine_MentionOnly(t *testing.T) {
	got := FirstLine("@alice")
	if got != "@alice" {
		t.Errorf("FirstLine(%q) = %q, want %q", "@alice", got, "@alice")
	}
	if strings.Contains(got, "\x1b") {
		t.Errorf("FirstLine must not contain ANSI codes: %q", got)
	}
}

func TestFirstLine_MentionAtStart(t *testing.T) {
	got := FirstLine("@alice: hello world")
	if got != "@alice: hello world" {
		t.Errorf("FirstLine(%q) = %q, want %q", "@alice: hello world", got, "@alice: hello world")
	}
	if strings.Contains(got, "\x1b") {
		t.Errorf("FirstLine must not contain ANSI codes: %q", got)
	}
}

// HTML numeric entities (e.g. &#27;) survive the API-boundary control-char
// sanitization because they are printable ASCII, and html.UnescapeString decodes
// them back into raw control characters. Render/FirstLine must re-strip them so a
// remote author cannot inject terminal escape sequences (clear-screen CSI,
// window-title/clipboard OSC) into a viewer's terminal.
func TestRender_EntityEncodedEscapesStripped(t *testing.T) {
	// CSI clear-screen via decimal and hex entities. The injected ESC must not
	// reach the output; styling SGR codes always end in 'm', so a raw "\x1b[2J"
	// can only come from the decoded payload.
	for _, in := range []string{"&#27;[2J", "&#x1b;[2J"} {
		out := Render(in, 40)
		if strings.Contains(out, "\x1b[2J") {
			t.Errorf("Render(%q) leaked a raw clear-screen escape: %q", in, out)
		}
	}
	// OSC window-title/clipboard injection: ESC ]0;...BEL. Styling never emits
	// an OSC introducer, so "\x1b]" in the output means the payload leaked.
	out := Render("&#x1b;]0;pwneddone", 40)
	if strings.Contains(out, "\x1b]") {
		t.Errorf("Render leaked a raw OSC introducer: %q", out)
	}
}

func TestFirstLine_EntityEncodedEscapesStripped(t *testing.T) {
	// FirstLine returns plain text with no styling, so no ESC may appear at all.
	got := FirstLine("&#27;[2J cleared")
	if strings.ContainsRune(got, '\x1b') {
		t.Errorf("FirstLine leaked a raw escape: %q", got)
	}
}

// --- RenderInline ---

func TestRenderInline_EmptyInput(t *testing.T) {
	if got := RenderInline("", ""); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestRenderInline_Bold(t *testing.T) {
	raw := RenderInline("this is **bold** text", "")
	plain := strip(raw)
	if !strings.Contains(plain, "bold") {
		t.Errorf("bold text not in output: %q", plain)
	}
	if strings.Contains(plain, "**") {
		t.Errorf("raw ** markers remain in output: %q", plain)
	}
	if !strings.Contains(raw, "\x1b[1m") && !strings.Contains(raw, "\x1b[1;") {
		t.Errorf("no bold ANSI code in output: %q", raw)
	}
}

func TestRenderInline_Italic(t *testing.T) {
	raw := RenderInline("*sigh* whatever", "")
	plain := strip(raw)
	if !strings.Contains(plain, "sigh") {
		t.Errorf("italic text not in output: %q", plain)
	}
	if strings.Contains(plain, "*sigh*") {
		t.Errorf("raw * markers remain in output: %q", plain)
	}
}

func TestRenderInline_InlineCode(t *testing.T) {
	raw := RenderInline("run `go test ./...` please", "")
	plain := strip(raw)
	if !strings.Contains(plain, "go test ./...") {
		t.Errorf("inline code text not in output: %q", plain)
	}
	if strings.Contains(plain, "`") {
		t.Errorf("raw backtick markers remain in output: %q", plain)
	}
}

func TestRenderInline_MarkdownLink(t *testing.T) {
	raw := RenderInline("see [the docs](https://example.com)", "")
	plain := strip(raw)
	if !strings.Contains(plain, "the docs") {
		t.Errorf("link text not in output: %q", plain)
	}
	if strings.Contains(plain, "[the docs](") {
		t.Errorf("raw link syntax remains in output: %q", plain)
	}
}

func TestRenderInline_BareURLAutolinked(t *testing.T) {
	raw := RenderInline("check https://example.com/path for details", "")
	plain := strip(raw)
	if !strings.Contains(plain, "https://example.com/path") {
		t.Errorf("bare URL not preserved in output: %q", plain)
	}
	if !strings.Contains(raw, "\x1b[") {
		t.Errorf("expected bare URL to be styled (underlined), got no ANSI codes: %q", raw)
	}
}

// TestRenderInline_LeadingBlockCharsStayLiteral guards the entire reason
// RenderInline exists instead of reusing Render: lines that happen to start
// with markdown block syntax must render as plain chat text, not be
// reinterpreted as a heading/list/blockquote/thematic-break/code-fence.
func TestRenderInline_LeadingBlockCharsStayLiteral(t *testing.T) {
	cases := []string{
		"# thoughts for today",
		"- get milk",
		"> what did you say",
		"1. first thing",
		"---",
		"```go",
	}
	for _, in := range cases {
		got := strip(RenderInline(in, ""))
		if got != in {
			t.Errorf("RenderInline(%q) = %q, want unchanged literal text (no block reinterpretation)", in, got)
		}
	}
}

// TestRenderInline_MultiLineKeepsLineBoundaries guards against reflow: a
// literal "\n" in chat content (e.g. a multi-line /fortune reply) must
// produce the same number of lines back, not get merged into one paragraph
// with soft line breaks collapsed to spaces (which is what Render would do).
func TestRenderInline_MultiLineKeepsLineBoundaries(t *testing.T) {
	in := "first line\nsecond line\nthird line"
	got := strip(RenderInline(in, ""))
	lines := strings.Split(got, "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines preserved, got %d: %q", len(lines), got)
	}
	if lines[0] != "first line" || lines[1] != "second line" || lines[2] != "third line" {
		t.Errorf("line content/order not preserved: %q", got)
	}
}

func TestRenderInline_NoMentionNodeStyling(t *testing.T) {
	// RenderInline deliberately excludes the @mention AST node (CIRC has its
	// own bespoke mention system layered on top by the caller) — an @mention
	// should pass through as literal text, not get theme.Highlight styling.
	raw := RenderInline("hey @alice look at this", "")
	if !strings.Contains(strip(raw), "@alice") {
		t.Errorf("expected @alice to remain in output: %q", strip(raw))
	}
}

// --- RenderInline highlightUser ---

func TestRenderInline_HighlightUserBoldsMatch(t *testing.T) {
	raw := RenderInline("@ragnar hi there", "ragnar")
	if !strings.Contains(raw, theme.MeHighlight.Render("@ragnar")) {
		t.Errorf("expected @ragnar to be styled with theme.MeHighlight, got: %q", raw)
	}
}

// TestRenderInline_HighlightUserPreservesSurroundingStyle is the regression
// test for the reported bug: "@ragnar 1" rendered "1" correctly in
// theme.Base's color, but "@ragnar 1 2" rendered "1" unstyled and only "2"
// back in theme.Base — because highlighting used to be a second pass that
// spliced a fresh Render() call into already-rendered text, and that inner
// call's own SGR reset silently killed the surrounding style for everything
// after the match. Note: extension.NewLinkifyParser's Trigger() fires on
// every space (to check for a URL starting right after it), so goldmark
// splits "@ragnar 1 2" into three separate ast.Text nodes ("@ragnar", " 1",
// " 2") even though nothing links — each is still individually rendered
// through renderPlainText, so "1" and "2" each get their own theme.Base call
// rather than one merged span. Visually identical (adjacent same-styled ANSI
// spans render seamlessly), so this asserts both are present, not that
// they're merged into one Render() call.
func TestRenderInline_HighlightUserPreservesSurroundingStyle(t *testing.T) {
	raw := RenderInline("@ragnar 1 2", "ragnar")
	if !strings.Contains(raw, theme.MeHighlight.Render("@ragnar")) {
		t.Errorf("expected @ragnar to be styled with theme.MeHighlight, got: %q", raw)
	}
	if !strings.Contains(raw, theme.Base.Render(" 1")) {
		t.Errorf("expected the text right after the mention (' 1') to keep theme.Base styling, got: %q", raw)
	}
	if !strings.Contains(raw, theme.Base.Render(" 2")) {
		t.Errorf("expected trailing text (' 2') to also keep theme.Base styling, not left unstyled by a broken reset after the mention, got: %q", raw)
	}
}

func TestRenderInline_HighlightUserCaseInsensitiveWordBounded(t *testing.T) {
	raw := RenderInline("hey Ragnar and ragnarwessels, is ragnar around?", "ragnar")
	if !strings.Contains(raw, theme.MeHighlight.Render("Ragnar")) {
		t.Errorf("expected case-insensitive match to be highlighted, got: %q", raw)
	}
	if !strings.Contains(raw, theme.MeHighlight.Render("ragnar")) {
		t.Errorf("expected bare-word match to be highlighted, got: %q", raw)
	}
	if strings.Contains(raw, theme.MeHighlight.Render("ragnarwessels")) {
		t.Errorf("did not expect a substring match (ragnarwessels) to be highlighted, got: %q", raw)
	}
}

func TestRenderInline_NoHighlightUserUnaffected(t *testing.T) {
	raw := RenderInline("@ragnar 1 2", "")
	if strings.Contains(raw, theme.MeHighlight.Render("@ragnar")) {
		t.Errorf("expected no highlighting when highlightUser is empty, got: %q", raw)
	}
	if !strings.Contains(strip(raw), "@ragnar 1 2") {
		t.Errorf("expected the literal text to survive unchanged, got: %q", strip(raw))
	}
	if !strings.Contains(raw, "\x1b[") {
		t.Errorf("expected the line to still carry theme.Base ANSI styling, got: %q", raw)
	}
}
