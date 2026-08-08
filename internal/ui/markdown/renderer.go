// Package markdown renders GitHub Flavored Markdown content as ANSI-styled strings
// for TUI display. Styles are read from the theme package at render time so all
// three runtime themes (cyber, c64, vt320) are supported without reconfiguration.
package markdown

import (
	"fmt"
	"html"
	"regexp"
	"strings"
	"sync"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/ragnar/cyber-tui/internal/sanitize"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	"golang.org/x/text/unicode/norm"
)

// --- @mention extension ---

var KindMention = ast.NewNodeKind("Mention")

// MentionNode is a custom inline AST node representing @username mentions.
type MentionNode struct {
	ast.BaseInline
	Username []byte
}

func (n *MentionNode) Kind() ast.NodeKind { return KindMention }
func (n *MentionNode) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{"Username": string(n.Username)}, nil)
}

type mentionParser struct{}

func (p *mentionParser) Trigger() []byte { return []byte{'@'} }

func (p *mentionParser) Parse(_ ast.Node, block text.Reader, _ parser.Context) ast.Node {
	line, _ := block.PeekLine()
	if len(line) < 2 || line[0] != '@' {
		return nil
	}
	i := 1
	for i < len(line) && i <= 30 && isUsernameChar(line[i]) {
		i++
	}
	if i == 1 {
		return nil
	}
	username := make([]byte, i-1)
	copy(username, line[1:i])
	block.Advance(i)
	return &MentionNode{Username: username}
}

func isUsernameChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}

type mentionExtender struct{}

func (e *mentionExtender) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(parser.WithInlineParsers(
		util.Prioritized(&mentionParser{}, 999),
	))
}

// --- Global markdown instance ---

var mdInstance = goldmark.New(
	goldmark.WithExtensions(
		extension.GFM,
		&mentionExtender{},
	),
)

// --- Public API ---

// Render parses content as GFM and returns an ANSI-styled string suitable for
// viewport display. width is the inner content width for word-wrapping.
func Render(content string, width int) string {
	content = html.UnescapeString(content)
	content = sanitize.Strip(content)
	content = norm.NFC.String(content)
	content = stripAmbiguousRunes(content)
	if strings.TrimSpace(content) == "" {
		return ""
	}
	if width < 1 {
		width = 80
	}

	src := []byte(content)
	reader := text.NewReader(src)
	doc := mdInstance.Parser().Parse(reader)

	r := &renderer{source: src, width: width}
	docNode, ok := doc.(*ast.Document)
	if !ok {
		return theme.Base.Width(width).Render(content)
	}
	return r.renderDocument(docNode)
}

// mdInlineParser registers only a paragraph block parser (no heading, list,
// blockquote, fenced-code, thematic-break, or HTML-block parsers), so a line
// starting with "#", "-", ">", "```", or "---" is never reinterpreted as
// block syntax — it stays literal chat text. Inline parsing still runs the
// normal CommonMark set (parser.DefaultInlineParsers: code span, link,
// autolink, raw HTML, emphasis/strong) plus bare-URL detection
// (extension.NewLinkifyParser, the same one extension.GFM's Linkify wraps).
// No @mention node here — CIRC has its own bespoke mention system layered on
// top by the caller.
var mdInlineParser = parser.NewParser(
	parser.WithBlockParsers(util.Prioritized(parser.NewParagraphParser(), 1000)),
	parser.WithInlineParsers(
		append(parser.DefaultInlineParsers(), util.Prioritized(extension.NewLinkifyParser(), 999))...,
	),
)

// RenderInline renders only inline markdown spans (emphasis/strong, code
// spans, links, bare-URL autolinks) — no block-level reinterpretation
// (headings, lists, blockquotes, tables, code fences) and no line-break
// reflow. Each line is parsed independently and rejoined with "\n" verbatim,
// since goldmark's soft-line-break-to-space behavior only triggers when
// multiple lines reach the parser as one paragraph — for freeform one-line
// chat text, unlike Render's full GFM document parsing, this is what a chat
// message actually needs: styling within a line, not restructuring of it.
//
// highlightUser, if non-empty, bolds case-insensitive, word-bounded
// occurrences of that username (bare or "@"-prefixed) in theme.MeHighlight
// instead of the default theme.Base — done here, in the same pass as the
// rest of the styling, rather than as a separate post-render string-splice:
// splicing a fresh Render() call into already-ANSI-rendered text is broken,
// since that inner call's own SGR reset terminates all active attributes,
// not just what it set, silently killing any surrounding style for
// everything after the match. Pass "" for no highlighting.
func RenderInline(content, highlightUser string) string {
	content = html.UnescapeString(content)
	content = sanitize.Strip(content)
	content = norm.NFC.String(content)
	content = stripAmbiguousRunes(content)
	if content == "" {
		return ""
	}
	var highlightRe *regexp.Regexp
	if highlightUser != "" {
		highlightRe = compiledHighlightRegex(highlightUser)
	}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = renderInlineLine(line, highlightRe)
	}
	return strings.Join(lines, "\n")
}

// highlightRegexCache memoizes the per-username regex RenderInline uses to
// bold mentions of the current user, keyed by username. That username is
// constant for the life of a session, so without this every call recompiled
// the same pattern from scratch — and RenderInline runs on every line of
// every rendered message.
var highlightRegexCache sync.Map // string (username) -> *regexp.Regexp

func compiledHighlightRegex(username string) *regexp.Regexp {
	if v, ok := highlightRegexCache.Load(username); ok {
		return v.(*regexp.Regexp)
	}
	re := regexp.MustCompile(`(?i)@?\b` + regexp.QuoteMeta(username) + `\b`)
	actual, _ := highlightRegexCache.LoadOrStore(username, re)
	return actual.(*regexp.Regexp)
}

func renderInlineLine(line string, highlightRe *regexp.Regexp) string {
	if strings.TrimSpace(line) == "" {
		return line
	}
	src := []byte(line)
	reader := text.NewReader(src)
	doc := mdInlineParser.Parse(reader)
	docNode, ok := doc.(*ast.Document)
	if !ok {
		return line
	}
	r := &renderer{source: src, width: 0, highlightRe: highlightRe}
	return r.renderDocument(docNode)
}

// FirstLine strips markdown syntax and returns the first non-empty line as
// plain text. Use this for compact single-line previews where full rendering
// is not appropriate (bookmarks, profile post lists).
func FirstLine(content string) string {
	content = html.UnescapeString(content)
	content = sanitize.Strip(content)
	content = norm.NFC.String(content)
	content = stripAmbiguousRunes(content)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		src := []byte(line)
		reader := text.NewReader(src)
		doc := mdInstance.Parser().Parse(reader)
		rr := &renderer{source: src, width: 0}
		if p := doc.FirstChild(); p != nil {
			raw := rr.rawTextNode(p)
			if raw != "" {
				return raw
			}
		}
		return line
	}
	return ""
}

// --- renderer ---

type renderer struct {
	source []byte
	width  int

	// highlightRe, if non-nil, matches occurrences of a username (case-
	// insensitive, word-bounded, bare or "@"-prefixed) within plain text runs
	// to bold in theme.MeHighlight instead of theme.Base — see RenderInline's
	// doc comment for why this happens here rather than as a post-render
	// pass. Compiled once per RenderInline call, not per text node.
	highlightRe *regexp.Regexp
}

func (r *renderer) renderDocument(doc *ast.Document) string {
	var parts []string
	for child := doc.FirstChild(); child != nil; child = child.NextSibling() {
		if rendered := r.renderBlock(child); rendered != "" {
			parts = append(parts, rendered)
		}
	}
	return strings.Join(parts, "\n")
}

func (r *renderer) renderBlock(node ast.Node) string {
	switch n := node.(type) {
	case *ast.Paragraph:
		inline := r.collectInlines(n)
		if r.width > 0 {
			return lipgloss.NewStyle().Width(r.width).Render(inline)
		}
		return inline

	case *ast.TextBlock:
		return r.collectInlines(n)

	case *ast.Heading:
		return r.renderHeading(n)

	case *ast.FencedCodeBlock:
		return r.renderCodeLines(n.Lines())

	case *ast.CodeBlock:
		return r.renderCodeLines(n.Lines())

	case *ast.Blockquote:
		return r.renderBlockquote(n)

	case *ast.List:
		return r.renderList(n, 0)

	case *ast.ThematicBreak:
		w := r.width
		if w < 1 {
			w = 9
		}
		return theme.Subtle.Render(strings.Repeat("─", w))

	case *ast.HTMLBlock:
		return ""

	default:
		// Handle GFM table
		if node.Kind() == extast.KindTable {
			return r.renderTable(node)
		}
		// Unknown block — recurse into children
		var parts []string
		for child := node.FirstChild(); child != nil; child = child.NextSibling() {
			if rendered := r.renderBlock(child); rendered != "" {
				parts = append(parts, rendered)
			}
		}
		return strings.Join(parts, "\n")
	}
}

func (r *renderer) renderHeading(n *ast.Heading) string {
	raw := r.rawTextNode(n)
	w := r.width
	if w < 1 {
		w = 9
	}

	switch n.Level {
	case 1:
		header := theme.Title.Render(strings.ToUpper(raw))
		sep := lipgloss.NewStyle().Foreground(theme.ColorDimGreen).Render(strings.Repeat("═", w))
		return header + "\n" + sep
	case 2:
		header := theme.Title.Render(raw)
		sep := lipgloss.NewStyle().Foreground(theme.ColorDimGreen).Render(strings.Repeat("─", w))
		return header + "\n" + sep
	default:
		return theme.Title.Render(raw)
	}
}

func (r *renderer) renderCodeLines(lines *text.Segments) string {
	codeStyle := lipgloss.NewStyle().Foreground(theme.ColorYellow)
	gutterStyle := lipgloss.NewStyle().Foreground(theme.ColorDimGreen)

	var result []string
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		line := strings.TrimRight(string(seg.Value(r.source)), "\n")
		result = append(result, gutterStyle.Render("│")+" "+codeStyle.Render(line))
	}
	return strings.Join(result, "\n")
}

func (r *renderer) renderBlockquote(n *ast.Blockquote) string {
	innerWidth := r.width - 2
	if innerWidth < 10 {
		innerWidth = 10
	}
	inner := &renderer{source: r.source, width: innerWidth}

	var parts []string
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		if rendered := inner.renderBlock(child); rendered != "" {
			parts = append(parts, rendered)
		}
	}
	content := strings.Join(parts, "\n")

	gutter := lipgloss.NewStyle().Foreground(theme.ColorDimGreen).Render("│")
	var result []string
	for _, line := range strings.Split(content, "\n") {
		result = append(result, gutter+" "+line)
	}
	return strings.Join(result, "\n")
}

func (r *renderer) renderList(n *ast.List, depth int) string {
	var parts []string
	indent := strings.Repeat("  ", depth)
	itemNum := n.Start

	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		li, ok := child.(*ast.ListItem)
		if !ok {
			continue
		}

		var bullet string
		if n.IsOrdered() {
			bullet = fmt.Sprintf("%s%d. ", indent, itemNum)
			itemNum++
		} else {
			bullet = indent + "• "
		}

		var inlineContent string
		var nested []string

		for liChild := li.FirstChild(); liChild != nil; liChild = liChild.NextSibling() {
			switch lc := liChild.(type) {
			case *ast.TextBlock:
				inlineContent = r.collectInlines(lc)
			case *ast.Paragraph:
				inlineContent = r.collectInlines(lc)
			case *ast.List:
				nested = append(nested, r.renderList(lc, depth+1))
			}
		}

		line := theme.Base.Render(bullet) + inlineContent
		if len(nested) > 0 {
			line += "\n" + strings.Join(nested, "\n")
		}
		parts = append(parts, line)
	}

	return strings.Join(parts, "\n")
}

func (r *renderer) renderTable(node ast.Node) string {
	var rows []string
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch child.Kind() {
		case extast.KindTableHeader:
			var cells []string
			for cell := child.FirstChild(); cell != nil; cell = cell.NextSibling() {
				cells = append(cells, theme.Title.Render(r.rawTextNode(cell)))
			}
			rows = append(rows, strings.Join(cells, " │ "))
			if r.width > 0 {
				rows = append(rows, theme.Subtle.Render(strings.Repeat("─", r.width)))
			}
		case extast.KindTableRow:
			var cells []string
			for cell := child.FirstChild(); cell != nil; cell = cell.NextSibling() {
				cells = append(cells, r.collectInlines(cell))
			}
			rows = append(rows, strings.Join(cells, " │ "))
		}
	}
	return strings.Join(rows, "\n")
}

// collectInlines renders all inline child nodes of a block node into one string.
func (r *renderer) collectInlines(node ast.Node) string {
	var sb strings.Builder
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		sb.WriteString(r.renderInline(child))
	}
	return sb.String()
}

func (r *renderer) renderInline(node ast.Node) string {
	switch n := node.(type) {
	case *ast.Text:
		val := strings.TrimRight(string(n.Value(r.source)), "\n")
		rendered := r.renderPlainText(val)
		if n.SoftLineBreak() {
			return rendered + " "
		}
		if n.HardLineBreak() {
			return rendered + "\n"
		}
		return rendered

	case *ast.String:
		return theme.Base.Render(string(n.Value))

	case *ast.Emphasis:
		raw := r.rawTextNode(n)
		if n.Level == 2 {
			return lipgloss.NewStyle().Bold(true).Foreground(theme.ColorGreen).Render(raw)
		}
		return lipgloss.NewStyle().Italic(true).Foreground(theme.ColorGreen).Render(raw)

	case *ast.CodeSpan:
		raw := r.rawTextNode(n)
		return lipgloss.NewStyle().Foreground(theme.ColorYellow).Render(raw)

	case *ast.Link:
		dest := string(n.Destination)
		linkText := r.rawTextNode(n)
		linkStyle := lipgloss.NewStyle().Underline(true).Foreground(theme.ColorCyan)
		if linkText == "" || linkText == dest {
			return linkStyle.Render(dest)
		}
		return linkStyle.Render(linkText)

	case *ast.Image:
		alt := r.rawTextNode(n)
		if alt != "" {
			return theme.Subtle.Render("[IMG: " + alt + "]")
		}
		return theme.Subtle.Render("[IMG]")

	case *ast.AutoLink:
		url := strings.TrimRight(string(n.URL(r.source)), "\n")
		return lipgloss.NewStyle().Underline(true).Foreground(theme.ColorCyan).Render(url)

	case *extast.Strikethrough:
		raw := r.rawTextNode(n)
		return lipgloss.NewStyle().Strikethrough(true).Foreground(theme.ColorMuted).Render(raw)

	case *MentionNode:
		return theme.Highlight.Render("@" + string(n.Username))

	case *ast.RawHTML:
		return ""

	default:
		return r.collectInlines(node)
	}
}

// renderPlainText renders a run of plain (non-emphasis/code/link) text,
// splitting out and bolding any occurrence of r.highlightRe in
// theme.MeHighlight — done at this raw-text level, before any ANSI is
// emitted, rather than by re-rendering an already-styled string (which is
// broken: an inner Render call's own SGR reset would terminate all active
// attributes, not just what it set, silently breaking the surrounding style
// for everything after the match).
func (r *renderer) renderPlainText(val string) string {
	if r.highlightRe == nil {
		return theme.Base.Render(val)
	}
	locs := r.highlightRe.FindAllStringIndex(val, -1)
	if locs == nil {
		return theme.Base.Render(val)
	}
	var sb strings.Builder
	last := 0
	for _, loc := range locs {
		if loc[0] > last {
			sb.WriteString(theme.Base.Render(val[last:loc[0]]))
		}
		sb.WriteString(theme.MeHighlight.Render(val[loc[0]:loc[1]]))
		last = loc[1]
	}
	if last < len(val) {
		sb.WriteString(theme.Base.Render(val[last:]))
	}
	return sb.String()
}

// rawTextNode extracts plain unformatted text from a node's inline children.
// Used for emphasis, links, and other inline containers where we apply the
// outer style to the complete text rather than nesting ANSI codes.
func (r *renderer) rawTextNode(node ast.Node) string {
	var sb strings.Builder
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch n := child.(type) {
		case *ast.Text:
			sb.WriteString(strings.TrimRight(string(n.Value(r.source)), "\n"))
			if n.SoftLineBreak() {
				sb.WriteByte(' ')
			}
		case *ast.String:
			sb.Write(n.Value)
		case *MentionNode:
			sb.WriteByte('@')
			sb.Write(n.Username)
		default:
			sb.WriteString(r.rawTextNode(child))
		}
	}
	return sb.String()
}

// typographicPunct is the set of EAW=A (ambiguous-width) characters that are
// consistently measured as 1 column in modern Western terminals and must not
// be replaced with spaces.
var typographicPunct = map[rune]bool{
	'\u2018': true, // ‘  LEFT SINGLE QUOTATION MARK
	'\u2019': true, // ’  RIGHT SINGLE QUOTATION MARK / apostrophe
	'\u201C': true, // “  LEFT DOUBLE QUOTATION MARK
	'\u201D': true, // ”  RIGHT DOUBLE QUOTATION MARK
	'\u2013': true, // –  EN DASH
	'\u2014': true, // —  EM DASH
	'\u2026': true, // …  HORIZONTAL ELLIPSIS
}

// stripAmbiguousRunes normalises Unicode characters whose display width cannot be
// reliably determined so that rendered output does not overflow width-constrained
// TUI boxes. Double-wide characters (CJK, fullwidth) are passed through unchanged
// because all three measurement layers — runewidth, lipgloss, and terminal wcwidth —
// agree on their 2-column width.
func stripAmbiguousRunes(s string) string {
	var b strings.Builder
	for _, r := range s {
		rw := runewidth.RuneWidth(r)
		switch {
		case r == '\t' || r == '\n':
			b.WriteRune(r) // preserve legitimate whitespace used by the parser
		case rw == 0 || runewidth.StringWidth("x"+string(r)) < 1+rw:
			// strip: zero-width chars, and grapheme-extend modifiers (e.g. ﾟ U+FF9F)
			// that runewidth.StringWidth treats as 0-width in context but the terminal
			// renders as 1 column (wcwidth), causing layout overflow
		case unicode.Is(unicode.Cf, r):
			// strip: Unicode Format characters (e.g. U+06DD ARABIC END OF AYAH) carry
			// no visible glyph in terminal fonts; the terminal renders a wide fallback
			// (enclosing mark, tofu box) that overflows the measured column count.
		case r >= 'ʰ' && r <= '˿':
			// Spacing Modifier Letters (IPA/phonetic diacritics) are absent from most
			// terminal fonts; the terminal substitutes a wide fallback glyph
			// (e.g. ▼ for U+02D5 MODIFIER LETTER DOWN TACK) that overflows the
			// measured column count.
			b.WriteRune(' ')
		case runewidth.IsAmbiguousWidth(r) && !unicode.IsLetter(r):
			if typographicPunct[r] {
				b.WriteRune(r) // common typographic punct — 1 column in Western terminals
			} else {
				b.WriteRune(' ') // ambiguous EAW=A non-letter symbol — normalise to space to avoid CJK layout overflow
			}
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// TruncateToWidth shortens s to at most maxWidth terminal columns, appending "…" if
// truncation occurs. Uses visual (runewidth) column measurement so double-width characters
// (CJK, full-width) are handled correctly — unlike rune-count truncation which
// under-counts their display width.
func TruncateToWidth(s string, maxWidth int) string {
	if maxWidth < 1 {
		return s
	}
	var w int
	runes := []rune(s)
	for i, r := range runes {
		rw := runewidth.RuneWidth(r)
		if w+rw > maxWidth-1 {
			return string(runes[:i]) + "…"
		}
		w += rw
	}
	return s
}
