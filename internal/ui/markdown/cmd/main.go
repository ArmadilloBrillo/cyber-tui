package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/ragnar/cyber-tui/internal/ui/markdown"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
)

func main() {
	content := "*AWESOME.* (ﾉ◕ヮ◕)ﾉ*:10,000!･ﾟ✧\n\n\n\n/off to try cool new stuff ᕕ( ᐛ )ᕗ"
	width := 120
	innerWidth := width - 4

	typeTag := theme.Subtle.Render("[reply]")
	authorStyled := "  " + theme.Highlight.Render("@twist")
	left1 := typeTag + authorStyled
	right1 := theme.Subtle.Render("posted 2d ago · saved 1d ago")

	gap := innerWidth - lipgloss.Width(left1) - lipgloss.Width(right1)
	var line1 string
	if gap > 0 {
		line1 = left1 + strings.Repeat(" ", gap) + right1
	} else {
		line1 = left1
	}

	rightWidth := lipgloss.Width(right1)
	previewMax := max(innerWidth-rightWidth-1, 1)
	preview := strings.ReplaceAll(markdown.FirstLine(content), "\n", " ")
	line2 := theme.Base.Render(markdown.TruncateToWidth(preview, previewMax))
	line3 := theme.Subtle.Render("no topics")

	body := lipgloss.JoinVertical(lipgloss.Left, line1, line2, line3)
	box := theme.Border.Width(width - 2).Render(body)

	fmt.Printf("lipgloss.Height=%d\n", lipgloss.Height(box))
	fmt.Printf("strings.Count(newlines)=%d\n", strings.Count(box, "\n"))
	fmt.Printf("HasTrailingNewline=%v\n", strings.HasSuffix(box, "\n"))

	// Check each raw byte at the end
	b := []byte(box)
	fmt.Printf("Last 10 bytes: %v\n", b[max(0, len(b)-10):])

	// Simulate what buildContent does: card + "\n"
	combined := box + "\n"
	fmt.Printf("\nAfter appending sep newline:\n")
	fmt.Printf("  strings.Count(newlines)=%d\n", strings.Count(combined, "\n"))
	fmt.Printf("  lipgloss.Height=%d\n", lipgloss.Height(combined))

	// What if we put two cards together?
	combined2 := box + "\n" + box + "\n"
	fmt.Printf("\nTwo cards concatenated:\n")
	fmt.Printf("  lipgloss.Height=%d (expected 10)\n", lipgloss.Height(combined2))
	fmt.Printf("  strings.Count(newlines)=%d\n", strings.Count(combined2, "\n"))
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
