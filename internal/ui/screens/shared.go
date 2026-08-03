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

// messageAttachments builds a []model.Attachment from a chat message's flat
// ImageUrl/GifUrl/AudioAttachment fields, so the existing renderAttachments
// (used for post/reply attachments) can render them too.
func messageAttachments(msg model.Message) []model.Attachment {
	var out []model.Attachment
	if msg.ImageUrl != "" {
		out = append(out, model.Attachment{Type: "image", Src: msg.ImageUrl})
	}
	if msg.GifUrl != "" {
		out = append(out, model.Attachment{Type: "gif", Src: msg.GifUrl})
	}
	if msg.AudioAttachment != nil {
		out = append(out, *msg.AudioAttachment)
	}
	return out
}

// messageURLs returns every openable URL in msg: links in the body plus
// ImageUrl/GifUrl/AudioAttachment.Src. Not normalized via urlutil.NormalizeURL —
// matches attachmentURLs' existing behavior for Attachment.Src, which is
// already an absolute URL per the API spec.
func messageURLs(msg model.Message) []string {
	urls := extractURLs(msg.Body)
	if msg.ImageUrl != "" {
		urls = append(urls, msg.ImageUrl)
	}
	if msg.GifUrl != "" {
		urls = append(urls, msg.GifUrl)
	}
	if msg.AudioAttachment != nil && msg.AudioAttachment.Src != "" {
		urls = append(urls, msg.AudioAttachment.Src)
	}
	return urls
}

// messageDisplayBody returns "" when Body merely duplicates ImageUrl or
// GifUrl (an attachment-only message posted with no text), so callers can
// skip printing a redundant line of URL text next to the attachment badge
// rendered separately via messageAttachments. Returns Body unchanged otherwise.
func messageDisplayBody(msg model.Message) string {
	if msg.Body != "" && (msg.Body == msg.ImageUrl || msg.Body == msg.GifUrl) {
		return ""
	}
	return msg.Body
}

// dedupeURLs removes repeated URLs while preserving first-seen order. Used by
// URLProvider implementations that aggregate across many items (e.g. an
// entire loaded chat history) rather than a single focused one, where the
// same link posted more than once would otherwise show up repeatedly in the
// open-link picker.
func dedupeURLs(urls []string) []string {
	if len(urls) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(urls))
	out := make([]string, 0, len(urls))
	for _, u := range urls {
		if seen[u] {
			continue
		}
		seen[u] = true
		out = append(out, u)
	}
	return out
}
