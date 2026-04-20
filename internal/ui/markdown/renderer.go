// Package markdown renders GitHub Flavored Markdown content as ANSI-styled strings
// for TUI display. Styles are read from the theme package at render time so all
// three runtime themes (cyber, c64, vt320) are supported without reconfiguration.
package markdown

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
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

// FirstLine strips markdown syntax and returns the first non-empty line as
// plain text. Use this for compact single-line previews where full rendering
// is not appropriate (bookmarks, profile post lists).
func FirstLine(content string) string {
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
		if n.SoftLineBreak() {
			return theme.Base.Render(val) + " "
		}
		if n.HardLineBreak() {
			return theme.Base.Render(val) + "\n"
		}
		return theme.Base.Render(val)

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
		default:
			sb.WriteString(r.rawTextNode(child))
		}
	}
	return sb.String()
}

// stripAmbiguousRunes replaces Unicode ambiguous-width characters with a space so that
// rendered output does not overflow width-constrained TUI boxes while keeping text readable.
func stripAmbiguousRunes(s string) string {
	var b strings.Builder
	for _, r := range s {
		if runewidth.IsAmbiguousWidth(r) {
			b.WriteRune(' ')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
