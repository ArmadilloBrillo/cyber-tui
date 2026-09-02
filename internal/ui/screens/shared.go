package screens

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/urlutil"
)

// pageJumpItems is how many rows a PgUp/PgDn press moves a card-list cursor
// (feed, bookmarks, guilds, notifications, topics, journal, chatrooms/cmail
// list mode) by. Cards vary in rendered height, so this is an approximate
// "jump ahead a bunch" step rather than an exact one-viewport-height jump —
// matching how htop/k9s/lazygit-style TUIs page a variable-height list.
const pageJumpItems = 10

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

// filterSlugCharsKeyMsg drops any rune not in [a-zA-Z0-9-] (plus
// extraAllowed, for fields that need a delimiter — e.g. topics' comma/space
// separators) before it reaches a textinput. Case is left as typed — the
// API's documented slug/topics character rules
// (docs/00-latest-api-reference.md) require lowercase, but that's applied
// once at the submit boundary (SlugValue, ParseTopics) rather than live, so
// what's on screen matches what was typed. If every rune in the message is
// filtered out, returns keep=false so nothing is inserted.
func filterSlugCharsKeyMsg(msg tea.KeyMsg, extraAllowed string) (tea.KeyMsg, bool) {
	if msg.Type != tea.KeyRunes {
		return msg, true
	}
	filtered := make([]rune, 0, len(msg.Runes))
	for _, r := range msg.Runes {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-':
			filtered = append(filtered, r)
		case strings.ContainsRune(extraAllowed, r):
			filtered = append(filtered, r)
		}
	}
	if len(filtered) == 0 {
		return msg, false
	}
	msg.Runes = filtered
	return msg, true
}

// insertAtCursor splices s into ti's value at the current cursor position
// and moves the cursor to just after it. textinput.Model has no public
// InsertString (unlike textarea.Model), so this does the splice by hand —
// same shape as chatrooms.go's spliceMention.
func insertAtCursor(ti textinput.Model, s string) textinput.Model {
	val, pos := []rune(ti.Value()), ti.Position()
	if pos < 0 {
		pos = 0
	}
	if pos > len(val) {
		pos = len(val)
	}
	ins := []rune(s)
	out := make([]rune, 0, len(val)+len(ins))
	out = append(out, val[:pos]...)
	out = append(out, ins...)
	out = append(out, val[pos:]...)
	ti.SetValue(string(out))
	ti.SetCursor(pos + len(ins))
	return ti
}

// topicsCount returns the number of non-empty, trimmed comma-separated
// segments in s — mirrors ParseTopics' counting rule without allocating the
// slice ParseTopics returns.
func topicsCount(s string) int {
	n := 0
	for _, part := range strings.Split(s, ",") {
		if strings.TrimSpace(part) != "" {
			n++
		}
	}
	return n
}

// lastTopicSegment returns the portion of current after its last comma (or
// the whole string, if there's no comma yet) — the topic currently being
// typed.
func lastTopicSegment(current string) string {
	if idx := strings.LastIndex(current, ","); idx != -1 {
		return current[idx+1:]
	}
	return current
}

// filterTopicsKeyMsg applies filterSlugCharsKeyMsg (allowing ", " as topic
// delimiters) and additionally drops:
//   - a comma that would open a 4th topic (current already holds 3 —
//     mirrors ParseTopics' cap live instead of silently truncating at
//     submit time), or finalize a topic that's empty/space-only or ends in
//     a space;
//   - a space that would lead a topic, or immediately follow another space
//     — a topic may contain single spaces, never a run of 2+.
//
// current is the field's value before this keystroke is applied.
func filterTopicsKeyMsg(msg tea.KeyMsg, current string) (tea.KeyMsg, bool) {
	filtered, keep := filterSlugCharsKeyMsg(msg, ", ")
	if !keep || msg.Type != tea.KeyRunes {
		return filtered, keep
	}
	segment := lastTopicSegment(current)
	segmentEmpty := strings.TrimSpace(segment) == ""
	trailingSpace := strings.HasSuffix(segment, " ")
	blockComma := segmentEmpty || trailingSpace || topicsCount(current) >= 3
	blockSpace := segmentEmpty || trailingSpace
	if !blockComma && !blockSpace {
		return filtered, true
	}
	runes := make([]rune, 0, len(filtered.Runes))
	for _, r := range filtered.Runes {
		if r == ',' && blockComma {
			continue
		}
		if r == ' ' && blockSpace {
			continue
		}
		runes = append(runes, r)
	}
	if len(runes) == 0 {
		return filtered, false
	}
	filtered.Runes = runes
	return filtered, true
}

// updateTopicsInput processes one message against a topics textinput,
// applying filterTopicsKeyMsg's rules. If the key would type a comma right
// after an accidental trailing space, the space is silently trimmed first
// so the comma still lands, instead of leaving the comma key dead — a
// previously reported annoyance. The "no trailing space" rule still holds;
// it's just enforced by auto-correction here rather than refusal. The trim
// is only committed to the field once the comma is confirmed to pass
// filterTopicsKeyMsg — a comma that's still going to be rejected anyway
// (e.g. it would open an empty topic) leaves the field untouched, same as
// any other blocked keystroke.
func updateTopicsInput(input textinput.Model, msg tea.Msg) (textinput.Model, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return input.Update(msg)
	}
	current := input.Value()
	trimCandidate := current
	if km.Type == tea.KeyRunes && strings.ContainsRune(string(km.Runes), ',') {
		trimCandidate = strings.TrimRight(current, " ")
	}
	filtered, keep := filterTopicsKeyMsg(km, trimCandidate)
	if !keep {
		return input, nil
	}
	if trimCandidate != current {
		input.SetValue(trimCandidate)
	}
	return input.Update(filtered)
}

// topicBlocked reports whether any of a post's topics is in the blocked set —
// the blocked-topics analogue of the FilterNSFW check each post-list screen
// applies in its visible*() helper. See docs/54-blocked-topics.md.
func topicBlocked(topics []string, blocked map[string]struct{}) bool {
	for _, t := range topics {
		if _, ok := blocked[t]; ok {
			return true
		}
	}
	return false
}

// blockedSet builds the lookup set from Settings.MutedTopics; returns nil for an
// empty list so callers can cheaply short-circuit with len() == 0.
func blockedSet(topics []string) map[string]struct{} {
	if len(topics) == 0 {
		return nil
	}
	m := make(map[string]struct{}, len(topics))
	for _, t := range topics {
		m[t] = struct{}{}
	}
	return m
}

// sameBlockedSet reports whether set already holds exactly the slugs in list
// (order-independent) — used by SharedConfigMsg handlers to skip a needless
// selection reset + re-render when the blocked list hasn't changed.
func sameBlockedSet(set map[string]struct{}, list []string) bool {
	if len(set) != len(list) {
		return false
	}
	for _, t := range list {
		if _, ok := set[t]; !ok {
			return false
		}
	}
	return true
}

// toggleBlocked returns a new slice with slug removed from the current blocked
// set if present, or appended if not. Order of the retained entries is not
// guaranteed (the API doesn't care).
func toggleBlocked(set map[string]struct{}, slug string) []string {
	if _, blocked := set[slug]; blocked {
		out := make([]string, 0, len(set)-1)
		for t := range set {
			if t != slug {
				out = append(out, t)
			}
		}
		return out
	}
	out := make([]string, 0, len(set)+1)
	for t := range set {
		out = append(out, t)
	}
	return append(out, slug)
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

// messageCopyText returns the text 'y' should copy for msg: its body, or —
// for an attachment-only message with no body (an /gif or /song posted with
// no caption) — the attachment's URL, so copying still does something
// useful rather than copying an empty string.
func messageCopyText(msg model.Message) string {
	if msg.Body != "" {
		return msg.Body
	}
	if msg.ImageUrl != "" {
		return msg.ImageUrl
	}
	if msg.GifUrl != "" {
		return msg.GifUrl
	}
	if msg.AudioAttachment != nil {
		return msg.AudioAttachment.Src
	}
	return ""
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

// chatInlineImageURL returns the URL a chat message's inline image (if any)
// should be fetched from: the explicit ImageUrl/GifUrl attachment field
// first, else the first image-looking URL typed into the message body.
// Returns "" when none apply — the message's existing attachment badge or
// body text is never altered by this; it's purely a lookup for the
// additive inline-image band C-Mail/cIRC splice in alongside it.
func chatInlineImageURL(msg model.Message) string {
	if msg.ImageUrl != "" {
		return msg.ImageUrl
	}
	if msg.GifUrl != "" {
		return msg.GifUrl
	}
	for _, u := range extractURLs(msg.Body) {
		if urlutil.IsImageURL(u) {
			return u
		}
	}
	return ""
}

// sanitizeChatMessageForInlineImage returns a copy of msg with the text that
// would otherwise duplicate its inline image (url, from chatInlineImageURL)
// stripped, so a caller rendering that copy instead of msg doesn't show the
// image's own URL/attachment badge sitting redundantly next to the actual
// image. Only clears what's unambiguously that image:
//   - ImageUrl/GifUrl, whichever matches url, so messageAttachments produces
//     no [image]/[gif] badge for it (an unrelated AudioAttachment is untouched)
//   - Body, but only when it's nothing but the URL itself (trimmed) — a URL
//     embedded alongside other text is left alone, since surgically removing
//     a substring from already word-wrapped/styled text isn't attempted here
func sanitizeChatMessageForInlineImage(msg model.Message, url string) model.Message {
	if msg.ImageUrl == url {
		msg.ImageUrl = ""
	}
	if msg.GifUrl == url {
		msg.GifUrl = ""
	}
	if strings.TrimSpace(msg.Body) == url {
		msg.Body = ""
	}
	return msg
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
