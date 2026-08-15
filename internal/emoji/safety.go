package emoji

import (
	"github.com/mattn/go-runewidth"
	"github.com/rivo/uniseg"
)

// safeWidth reports whether s is safe to insert into a bubbles
// textinput.Model/textarea.Model. Those components use uniseg.StringWidth
// (grapheme-cluster aware) for whole-line width but compute cursor/scroll
// position by summing go-runewidth's RuneWidth per individual rune — for a
// multi-codepoint grapheme cluster (ZWJ ligatures, skin-tone modifiers, flag
// pairs, keycaps, and some base+variation-selector pairs) those two numbers
// disagree, desyncing the library's own cursor math from its own line-width
// math and corrupting the rendered box.
//
// Rejecting on East-Asian-Ambiguous width too (matching
// filterAmbiguousKeyMsg's typed-input guard, internal/ui/screens/shared.go)
// was tried and reverted: kaomoji are built almost entirely out of
// ambiguous-width symbols (▽ ω ◕ ° …), so that check alone excluded 95 of
// 140 curated kaomoji — including harmless, universally-fine ones like
// "table flip" — while barely changing the emoji count. The cluster-width
// mismatch above is what actually desyncs bubbles' cursor math; a lone
// ambiguous rune (no multi-rune cluster involved) doesn't trigger it.
func safeWidth(s string) bool {
	var summed int
	for _, r := range s {
		if isConjoiningJamo(r) {
			return false
		}
		summed += runewidth.RuneWidth(r)
	}
	return summed == uniseg.StringWidth(s)
}

// isConjoiningJamo reports whether r is a raw Hangul Jamo codepoint (U+1100-
// U+11FF, plus the Extended-A/B blocks) — these exist only to algorithmically
// compose into a Hangul Syllables block (U+AC00-U+D7A3) character; used in
// isolation (as in a kaomoji, next to unrelated punctuation) most fonts have
// no standalone glyph for them and terminals render them unpredictably, even
// though both runewidth and uniseg agree they're 1 column wide — so this
// isn't caught by the cluster-mismatch check above. The visually similar
// Hangul Compatibility Jamo block (U+3130-U+318F) is designed for standalone
// display and renders fine, so it's deliberately not included here.
func isConjoiningJamo(r rune) bool {
	return (r >= 0x1100 && r <= 0x11FF) ||
		(r >= 0xA960 && r <= 0xA97F) ||
		(r >= 0xD7B0 && r <= 0xD7FF)
}

// filterSafe returns the subset of icons whose glyph passes safeWidth.
func filterSafe(icons []Icon) []Icon {
	out := make([]Icon, 0, len(icons))
	for _, ic := range icons {
		if safeWidth(ic.Glyph) {
			out = append(out, ic)
		}
	}
	return out
}

// requireKnownWide rejects any single-codepoint icon go-runewidth doesn't
// classify as wide (2 columns). Emoji-only: every glyph in that dataset
// comes from Unicode's own emoji-test.txt, which only lists codepoints
// Unicode itself defines as emoji — always wide once assigned — so a narrow
// result means the codepoint is newer than what the pinned go-runewidth
// version's table covers, not a genuinely narrow character. Confirmed
// empirically: go-runewidth v0.0.19's compiled wide-range table
// (runewidth_table.go) ends at 0x1FAE8, one codepoint short of U+1FAE9
// ("face with bags under eyes", Emoji 16.0) — every single-codepoint emoji
// from Emoji 0.6 through 15.1 matched go-runewidth's table with zero
// mismatches, then a cluster of mismatches starts exactly at 16.0. Terminals
// vary in how they handle a codepoint their own tables don't recognize
// either (observed: fine on Ghostty, corrupts the box on Konsole), so rather
// than chase each newly-reported terminal-specific break, this excludes
// anything the pinned width library can't yet vouch for. Self-heals on a
// future go-runewidth upgrade — no manual re-curation needed.
//
// Not applied to kaomoji: there's no "must be wide" ground truth for an
// arbitrary punctuation glyph the way there is for something Unicode itself
// calls an emoji.
func requireKnownWide(icons []Icon) []Icon {
	out := make([]Icon, 0, len(icons))
	for _, ic := range icons {
		runes := []rune(ic.Glyph)
		if len(runes) == 1 && runewidth.RuneWidth(runes[0]) != 2 {
			continue
		}
		out = append(out, ic)
	}
	return out
}
