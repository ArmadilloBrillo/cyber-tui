// Package bork rewrites text in the style of the Swedish Chef. It is applied to
// outgoing messages and posts before they are sent.
package bork

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const trailer = " Bork Bork Bork!"

// One pass, leftmost-first in argument order, so "w" -> "v" -> "f" never chains
// and "ow" wins over "o".
var letters = strings.NewReplacer(
	"an", "un", "au", "oo", "ow", "oo",
	"w", "v", "v", "f", "f", "ff", "o", "u", "u", "oo",
)

// Bork returns s with the Chef filter applied and a trailer appended to the
// last line of prose. @mentions, URLs, inline code and fenced code blocks are
// left alone so they keep working after the rewrite.
func Bork(s string) string {
	lines := strings.Split(s, "\n")
	fenced := false
	last := -1
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
			continue
		}
		if fenced || strings.TrimSpace(line) == "" {
			continue
		}
		lines[i] = borkLine(line)
		last = i
	}
	if last >= 0 {
		lines[last] += trailer
	}
	return strings.Join(lines, "\n")
}

func borkLine(line string) string {
	tokens := strings.Split(line, " ")
	inCode := false
	for i, tok := range tokens {
		ticks := strings.Count(tok, "`")
		skip := inCode || ticks > 0
		if ticks%2 == 1 {
			inCode = !inCode
		}
		if skip || strings.HasPrefix(tok, "@") || strings.Contains(tok, "://") {
			continue
		}
		tokens[i] = borkToken(tok)
	}
	return strings.Join(tokens, " ")
}

// borkToken rewrites the letters of tok and keeps surrounding punctuation.
func borkToken(tok string) string {
	start := strings.IndexFunc(tok, unicode.IsLetter)
	if start < 0 {
		return tok
	}
	end := strings.LastIndexFunc(tok, unicode.IsLetter)
	_, size := utf8.DecodeRuneInString(tok[end:])
	end += size
	return tok[:start] + borkWord(tok[start:end]) + tok[end:]
}

func borkWord(word string) string {
	lower := strings.ToLower(word)
	out := chef(lower)
	if len(word) > 1 && word == strings.ToUpper(word) {
		return strings.ToUpper(out)
	}
	if r, _ := utf8.DecodeRuneInString(word); unicode.IsUpper(r) {
		first, _ := utf8.DecodeRuneInString(out)
		return string(unicode.ToUpper(first)) + out[utf8.RuneLen(first):]
	}
	return out
}

func chef(w string) string {
	if w == "the" {
		return "zee"
	}
	stem, tail := w, ""
	switch {
	case strings.HasSuffix(w, "tion"):
		stem, tail = strings.TrimSuffix(w, "tion"), "shun"
	case strings.HasSuffix(w, "en"):
		stem, tail = strings.TrimSuffix(w, "en"), "ee"
	case strings.HasSuffix(w, "e") && len(w) > 2:
		stem, tail = strings.TrimSuffix(w, "e"), "e-a"
	}
	if strings.HasPrefix(stem, "e") {
		return "i" + letters.Replace(stem[1:]) + tail
	}
	return letters.Replace(stem) + tail
}
