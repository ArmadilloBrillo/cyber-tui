package screens

import (
	"strings"

	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/imgview"
	"github.com/ragnar/cyber-tui/internal/ui/markdown"
)

// inlineImageMaxRows is the fixed row budget reserved for one inline image
// within a post/reply card — a thumbnail, not a hero image, since every
// eligible image in a post renders inline (not just the first). The image
// is fit into this box preserving aspect ratio (never upscaled), so it may
// occupy fewer rows than this in practice — the rest of the band just stays
// blank. Deliberately fixed rather than computed from the image's real
// dimensions: those aren't known until an async fetch completes, and
// reflowing the whole card once they arrive is real complexity this spike
// doesn't need yet (see the plan).
const inlineImageMaxRows = 8

// inlineImageBandRows is the total number of lines spliceImageBand reserves
// per image: one blank spacer line, the image's own rows, one more blank
// spacer line — so consecutive inline images (or an image and surrounding
// text) never look like they're touching. The image itself is drawn
// starting one line into the band (see renderBodyWithInlineImage).
const inlineImageBandRows = inlineImageMaxRows + 2

// inlineImageEncodeMaxRows is what InlineImageSlot.MaxRows is actually set
// to — inlineImageMaxRows minus a small safety margin, not the full budget.
// Kitty/iTerm2 tell the terminal "display this in exactly N rows" and it
// scales to fit, so they're self-correcting; Sixel has no such instruction —
// it just emits raw pixels, and how many terminal rows those actually cover
// depends on TerminalCellPixelSize's cell-height estimate, which is only
// ever approximate (some terminals don't report real geometry at all, and
// the fallback is a hardcoded guess). An image that exactly fills the
// reserved band has zero margin for that estimate to be wrong; a tall/narrow
// image that hits the row cap is the case that actually shows it — wide
// images naturally end up shorter than the cap and have slack regardless.
// ponytail: a fixed fudge factor, not a measured one — tune (or replace with
// something that reads the terminal's real font metrics) if 2 rows isn't
// enough slack on some terminal/font combination.
const inlineImageEncodeMaxRows = inlineImageMaxRows - 2

// postImageSlot is the eligible inline image found for one post/reply card,
// in that card's own line coordinates (0-based from the first line of the
// string that card's render function returns, i.e. including its border).
// URL == "" means no eligible image.
type postImageSlot struct {
	URL  string
	Line int
}

// InlineImageSlot is a post/reply's inline image location in viewport
// coordinates, as reported by a screen's VisibleInlineImages(). App uses
// this purely as a "where, if anywhere" query each frame — it doesn't fetch,
// encode, or track placement/cache state itself.
type InlineImageSlot struct {
	URL       string
	Row       int // viewport-relative row (0 = top of the visible viewport)
	ColIndent int // left padding (border + card indent) before the image starts
	MaxCols   int
	MaxRows   int
	Key       string // stable per post/reply, for placement tracking (Kitty) and caching
}

// badgeIconCols is the reserved column width for one inline badge icon (a
// supporter/guild icon rendered next to a username) — small enough to sit
// inline within a text line rather than reserving a row band like
// InlineImageSlot's other uses. A trailing single space keeps it from
// touching whatever text follows.
const badgeIconCols = 2

// badgeGap returns the literal blank space a rendered line must reserve
// after a piece of text for n badge icons to be composited over later, in
// left-to-right order.
func badgeGap(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat(" "+strings.Repeat(" ", badgeIconCols), n)
}

// userBadgeCodes returns the SupporterIcon code to render next to a
// username, skipping it if empty or the user isn't a supporter. GuildIcon
// deliberately isn't included here: confirmed live (GET /v1/guilds, all 20
// real guilds) that Guild.Icon/User.GuildIcon values are Unicode/CLDR emoji
// character names (e.g. "crown", "black-sun-with-rays") or an unidentified
// "dinkie-icons:"-prefixed set — never the Lucide/Lucide-Lab/Phosphor scheme
// ResolveBadgeIconURL resolves, which was confirmed only against a
// SupporterIcon value. See docs/00-api-backlog.md for the follow-up.
func userBadgeCodes(u model.User) []string {
	var codes []string
	if u.IsSupporter && u.SupporterIcon != "" {
		codes = append(codes, u.SupporterIcon)
	}
	return codes
}

// badgeSlot builds an InlineImageSlot for one badge icon code
// (SupporterIcon), anchored at (row, col) in the caller's viewport
// coordinates — col is the on-screen column immediately after the text the
// badge follows, from badgeGap's reserved space. Returns ok=false for an
// empty code (nothing to show).
func badgeSlot(code string, row, col int, key string) (InlineImageSlot, bool) {
	if code == "" {
		return InlineImageSlot{}, false
	}
	return InlineImageSlot{
		URL:       imgview.BadgeURLPrefix + code,
		Row:       row,
		ColIndent: col,
		MaxCols:   badgeIconCols,
		MaxRows:   1,
		Key:       key,
	}, true
}

// spliceImageBand replaces lines[at] with n blank lines, reserving room for
// an inline image to be overlaid at that position later. Returns lines
// unchanged if at is out of range.
func spliceImageBand(lines []string, at, n int) []string {
	if at < 0 || at >= len(lines) {
		return lines
	}
	out := make([]string, 0, len(lines)+n-1)
	out = append(out, lines[:at]...)
	for i := 0; i < n; i++ {
		out = append(out, "")
	}
	out = append(out, lines[at+1:]...)
	return out
}

// spliceInlineImageBands splices a reserved, spacer-inclusive band into
// lines for every hit, in document order, tracking the shift each earlier
// splice introduces so later hits still land at the right spot (splicing at
// line N shifts everything after N down by inlineImageBandRows-1). A hit
// whose current (shift-adjusted) position falls outside lines is skipped —
// this is what makes a hit past a truncation cutoff (e.g. Feed's
// postMaxBodyLines) simply disappear, the same way its plain-text
// placeholder would have been truncated away. lineBase translates each
// hit's position into the caller's card-local coordinate space for the
// returned slots.
func spliceInlineImageBands(lines []string, hits []markdown.ImageHit, lineBase int) ([]string, []postImageSlot) {
	var slots []postImageSlot
	shift := 0
	for _, h := range hits {
		at := h.Line + shift
		if at < 0 || at >= len(lines) {
			continue
		}
		lines = spliceImageBand(lines, at, inlineImageBandRows)
		slots = append(slots, postImageSlot{URL: h.URL, Line: lineBase + at + 1})
		shift += inlineImageBandRows - 1
	}
	return lines, slots
}
