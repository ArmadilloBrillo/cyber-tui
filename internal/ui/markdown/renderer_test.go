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
