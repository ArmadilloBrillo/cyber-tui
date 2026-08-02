package screens

import (
	"hash/fnv"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
)

// Style command names from the /style-family commands (see
// docs/00-latest-api-reference.md's Commands table). "art" is handled
// separately (see decodeArtBody in internal/api/client.go and
// renderArtMessage below) — it marks the body as base64 ASCII art, not a
// text style. "comic" and "times" name font-family changes that have no
// terminal equivalent (a terminal renders one monospace font app-wide) and
// are intentionally no-ops here.
const (
	styleBlink   = "blink"
	styleL33t    = "l33t"
	styleComic   = "comic"
	styleCursive = "cursive"
	styleTimes   = "times"
	styleRainbow = "rainbow"
	styleFlip    = "flip"
	styleQuiet   = "quiet"
	styleSlow    = "slow"
	styleGlitch  = "glitch"
	styleSpoiler = "spoiler"
	styleWave    = "wave"
	styleArt     = "art"
)

// hasAnimatedStyle reports whether styles contains a frame-driven style
// (blink, wave, glitch) that needs a running tea.Tick to progress. slow is
// a static, one-shot substitution (see applySlowDots) and doesn't need one.
func hasAnimatedStyle(styles []string) bool {
	return slices.Contains(styles, styleBlink) || slices.Contains(styles, styleWave) || slices.Contains(styles, styleGlitch)
}

// styleAnimTickMsg drives the frame counter for blink/wave/glitch styles.
type styleAnimTickMsg struct{}

const styleAnimInterval = 150 * time.Millisecond

func styleAnimTickCmd() tea.Cmd {
	return tea.Tick(styleAnimInterval, func(time.Time) tea.Msg { return styleAnimTickMsg{} })
}

// blinkPhaseFrames is how many animation ticks each blink phase (visible or
// hidden) lasts — 4 ticks * styleAnimInterval (150ms) ≈ 600ms per phase,
// close to a typical terminal cursor blink rate.
const blinkPhaseFrames = 4

// blinkVisible reports whether a blink-styled message should show its real
// content this frame, alternating every blinkPhaseFrames ticks. App-driven
// rather than the ANSI blink SGR attribute (which some terminals/multiplexers
// ignore or disable) — this guarantees the same behavior everywhere at the
// cost of the message needing a running ticker, same as wave/glitch.
func blinkVisible(frame int) bool {
	return (frame/blinkPhaseFrames)%2 == 0
}

// --- character substitution (l33t, cursive, flip, glitch) ---

var l33tTable = map[rune]rune{
	'a': '4', 'A': '4',
	'e': '3', 'E': '3',
	'i': '1', 'I': '1',
	'o': '0', 'O': '0',
	's': '5', 'S': '5',
	't': '7', 'T': '7',
}

// cursiveTable maps ASCII letters to Unicode mathematical script letters.
// A handful of code points in that block are unassigned ("holes") and alias
// to older letterlike-symbol compatibility characters instead (e.g. script
// capital B is U+212C, not U+1D49B+1). Rendering quality depends on the
// terminal font having glyph coverage for this range — an accepted, explicit
// trade-off (see the plan's Trade-offs section), not a bug.
var cursiveTable = buildCursiveTable()

func buildCursiveTable() map[rune]rune {
	m := make(map[rune]rune, 52)
	for i := rune(0); i < 26; i++ {
		m['A'+i] = 0x1D49C + i
	}
	for i := rune(0); i < 26; i++ {
		m['a'+i] = 0x1D4B6 + i
	}
	// Holes in the Mathematical Script block, aliased to Letterlike Symbols.
	holes := map[rune]rune{
		'B': 0x212C, 'E': 0x2130, 'F': 0x2131, 'H': 0x210B,
		'I': 0x2110, 'L': 0x2112, 'M': 0x2133, 'R': 0x211B,
		'e': 0x212F, 'g': 0x210A, 'o': 0x2134,
	}
	for r, v := range holes {
		m[r] = v
	}
	return m
}

// flipTable maps ASCII letters to their conventional "upside-down text"
// lookalikes. Uppercase letters are folded to lowercase first — a faithful
// per-case upside-down mapping is font-dependent and inconsistent across
// terminals, so this codebase accepts the lowercase-only rendering rather
// than chase an unreliable uppercase table.
var flipTable = map[rune]rune{
	'a': 'ɐ', 'b': 'q', 'c': 'ɔ', 'd': 'p', 'e': 'ǝ', 'f': 'ɟ', 'g': 'ƃ',
	'h': 'ɥ', 'i': 'ᴉ', 'j': 'ɾ', 'k': 'ʞ', 'l': 'l', 'm': 'ɯ', 'n': 'u',
	'o': 'o', 'p': 'd', 'q': 'b', 'r': 'ɹ', 's': 's', 't': 'ʇ', 'u': 'n',
	'v': 'ʌ', 'w': 'ʍ', 'x': 'x', 'y': 'ʎ', 'z': 'z',
}

func applyTable(body string, table map[rune]rune) string {
	return strings.Map(func(r rune) rune {
		if v, ok := table[r]; ok {
			return v
		}
		return r
	}, body)
}

func applyFlip(body string) string {
	runes := []rune(strings.ToLower(body))
	slices.Reverse(runes)
	for i, r := range runes {
		if v, ok := flipTable[r]; ok {
			runes[i] = v
		}
	}
	return string(runes)
}

// applyGlitch jitters roughly a third of the letters in body by toggling
// their case, deterministically per (msgID, rune index, frame) so unrelated
// re-renders within the same frame (e.g. a selection change) stay
// byte-identical. Deliberately ASCII-safe — no combining marks — since this
// codebase relies on runewidth-based column-width math elsewhere (see
// shared.go) that zalgo-style stacked diacritics would break.
func applyGlitch(body, msgID string, frame int) string {
	runes := []rune(body)
	for i, r := range runes {
		if !unicode.IsLetter(r) {
			continue
		}
		if glitchHash(msgID, i, frame)%3 != 0 {
			continue
		}
		if unicode.IsUpper(r) {
			runes[i] = unicode.ToLower(r)
		} else {
			runes[i] = unicode.ToUpper(r)
		}
	}
	return string(runes)
}

func glitchHash(msgID string, index, frame int) uint32 {
	h := fnv.New32a()
	h.Write([]byte(msgID))
	h.Write([]byte(":"))
	h.Write([]byte(strconv.Itoa(index)))
	h.Write([]byte(":"))
	h.Write([]byte(strconv.Itoa(frame)))
	return h.Sum32()
}

// applyWave flips the case of a single rune position that sweeps
// left-to-right across body, advancing one position per animation frame —
// the same case-toggle mechanism as applyGlitch, but restricted to one
// moving position instead of jittering ~1/3 of all letters at once. Sweeping
// onto a non-letter (space, punctuation) is a no-op that frame, same as the
// wave passing invisibly through whitespace.
func applyWave(body string, frame int) string {
	runes := []rune(body)
	if len(runes) == 0 {
		return body
	}
	pos := frame % len(runes)
	r := runes[pos]
	if unicode.IsUpper(r) {
		runes[pos] = unicode.ToLower(r)
	} else if unicode.IsLower(r) {
		runes[pos] = unicode.ToUpper(r)
	}
	return string(runes)
}

// slowDot is a middle dot (·, U+00B7) rather than a period, so /slow reads
// as a deliberate visual spacing effect instead of looking like punctuation.
const slowDot = '·'

// applySlowDots inserts slowDot between every character — a static, one-shot
// transform (not frame-animated) evoking a stretched-out reading pace.
func applySlowDots(body string) string {
	runes := []rune(body)
	if len(runes) < 2 {
		return body
	}
	var sb strings.Builder
	for i, r := range runes {
		sb.WriteRune(r)
		if i != len(runes)-1 {
			sb.WriteRune(slowDot)
		}
	}
	return sb.String()
}

// substituteChars applies the character-substitution styles (l33t, cursive,
// flip, slow, wave, glitch) to raw message text, before markdown rendering —
// the substituted characters are plain runes markdown treats literally.
// msgID and frame are only used by wave and glitch. No-op when none of these
// styles are present.
func substituteChars(body, msgID string, styles []string, frame int) string {
	out := body
	if slices.Contains(styles, styleL33t) {
		out = applyTable(out, l33tTable)
	}
	if slices.Contains(styles, styleCursive) {
		out = applyTable(out, cursiveTable)
	}
	if slices.Contains(styles, styleFlip) {
		out = applyFlip(out)
	}
	if slices.Contains(styles, styleSlow) {
		out = applySlowDots(out)
	}
	if slices.Contains(styles, styleWave) {
		out = applyWave(out, frame)
	}
	if slices.Contains(styles, styleGlitch) {
		out = applyGlitch(out, msgID, frame)
	}
	return out
}

// --- attribute styles (quiet, rainbow) ---
// blink is handled separately (see blinkVisible) as a post-wrap visibility
// toggle rather than a strip-and-restyle attribute, so it can blank already
// word-wrapped lines without risking a rewrap on the blanked text — see the
// blink toggle in renderCircMessagesStyled/renderChatMessagesStyled.

var rainbowPalette = []lipgloss.Color{
	theme.ColorRed, theme.ColorYellow, theme.ColorGreen,
	theme.ColorCyan, theme.ColorWhite,
}

func hasAttributeStyle(styles []string) bool {
	return slices.Contains(styles, styleQuiet) || slices.Contains(styles, styleRainbow)
}

// applyAttributeStyle applies quiet/rainbow to already-markdown-rendered
// text. It strips existing ANSI first and restyles the plain text — the same
// technique renderCircMessagesWithSelection uses for theme.SelectedRow —
// because wrapping already-styled text in another style would terminate the
// outer style's codes mid-line. This means a styled message trades away
// inline markdown decoration (bold, mentions) within the styled span, an
// accepted trade-off (see the plan's Trade-offs section).
func applyAttributeStyle(rendered string, styles []string) string {
	if !hasAttributeStyle(styles) {
		return rendered
	}
	plain := ansi.Strip(rendered)
	base := lipgloss.NewStyle()
	if slices.Contains(styles, styleQuiet) {
		base = base.Faint(true).Foreground(theme.ColorMuted)
	}
	if slices.Contains(styles, styleRainbow) {
		var sb strings.Builder
		i := 0
		for _, r := range plain {
			sb.WriteString(base.Foreground(rainbowPalette[i%len(rainbowPalette)]).Render(string(r)))
			i++
		}
		return sb.String()
	}
	return base.Render(plain)
}

// --- spoiler ---

// maskSpoilerBody replaces every non-space rune with a block character,
// preserving whitespace so word-wrapped layout doesn't shift. A pure
// function with no dependency on chatrooms.go's message-selection state, so
// it can be reused once C-Mail gets its own spoiler-reveal mechanism —
// C-Mail doesn't have one yet, see the plan's Trade-offs section.
func maskSpoilerBody(body string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return r
		}
		return '░'
	}, body)
}

// --- art ---

// renderArtMessage renders a style:"art" message's already-decoded body
// verbatim: no word-wrap, no markdown, matching renderDeletedTombstone's
// dedicated-builder convention. ASCII art is column-sensitive, so wrapping
// or markdown-escaping it would corrupt the picture.
func renderArtMessage(username, body, ts string, viewportWidth int) string {
	const tsGap = 2 // matches renderCircMessages/renderDeletedTombstone's minimum gap
	tsWidth := lipgloss.Width(ts)
	rawPrefixWidth := len(username) + 4 // "<username>  ", same accounting as renderCircMessages' styledPrefix
	pad := max(viewportWidth-rawPrefixWidth-tsWidth, 0) + tsGap
	header := "<" + theme.Highlight.Render(username) + ">" + strings.Repeat(" ", pad) + theme.Subtle.Render(ts)
	var sb strings.Builder
	sb.WriteString(header + "\n")
	for line := range strings.SplitSeq(strings.TrimRight(body, "\n"), "\n") {
		sb.WriteString(line + "\n")
	}
	return sb.String()
}
