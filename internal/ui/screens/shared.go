package screens

import (
	"github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/urlutil"
)

// typographicPunctOK mirrors the whitelist in the markdown package: EAW=A characters
// that are consistently 1 column in Western terminals and safe to type/display.
var typographicPunctOK = map[rune]bool{
	'\u2018': true, '\u2019': true, // curly single quotes
	'\u201C': true, '\u201D': true, // curly double quotes
	'\u2013': true, '\u2014': true, // en/em dash
	'\u2026': true, // ellipsis
}

// filterAmbiguousKeyMsg replaces ambiguous-width runes in a KeyRunes message with spaces
// before they reach a textarea or textinput component.
func filterAmbiguousKeyMsg(msg tea.KeyMsg) (tea.KeyMsg, bool) {
	if msg.Type != tea.KeyRunes {
		return msg, true
	}
	filtered := make([]rune, len(msg.Runes))
	for i, r := range msg.Runes {
		if runewidth.IsAmbiguousWidth(r) && !typographicPunctOK[r] {
			filtered[i] = ' '
		} else {
			filtered[i] = r
		}
	}
	msg.Runes = filtered
	return msg, true
}

// extractURLs is a package-internal helper used by URLProvider implementations.
// It normalizes each extracted URL so relative paths get the cyberspace.online prefix.
func extractURLs(content string) []string {
	raw := urlutil.ExtractURLs(content)
	if len(raw) == 0 {
		return nil
	}
	out := make([]string, len(raw))
	for i, u := range raw {
		out[i] = urlutil.NormalizeURL(u)
	}
	return out
}

// attachmentURLs returns the Src URL for each attachment.
func attachmentURLs(attachments []model.Attachment) []string {
	if len(attachments) == 0 {
		return nil
	}
	out := make([]string, len(attachments))
	for i, a := range attachments {
		out[i] = a.Src
	}
	return out
}
