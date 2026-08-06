package theme

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Layout constants — shared across app and screens so viewport heights
// are calculated from a single source of truth rather than magic numbers.
const (
	TabBarHeight    = 1 // single text row, no border
	StatusBarHeight = 1
	SeparatorHeight = 1                                                // blank row between tab bar and screen content
	ChromeHeight    = TabBarHeight + StatusBarHeight + SeparatorHeight // = 3
)

// currentName tracks the last theme passed to Set.
var currentName = "cyber"

// CurrentName returns the name of the currently active theme.
func CurrentName() string {
	return currentName
}

// Color vars — reassigned by Set(). Named after the default Cyber palette.
// ColorWhite drives only MeHighlight (own-username highlighting); ColorMeta
// drives the status bar's secondary info text and hint key-descriptions —
// a distinct role that happens to share the same hex value in every
// built-in theme today, but can diverge in a custom theme.
var (
	ColorGreen      = lipgloss.Color("#00FF41")
	ColorDimGreen   = lipgloss.Color("#007A1F")
	ColorCyan       = lipgloss.Color("#00FFFF")
	ColorYellow     = lipgloss.Color("#FFD700")
	ColorRed        = lipgloss.Color("#FF003C")
	ColorBackground = lipgloss.Color("#0D0D0D")
	ColorMuted      = lipgloss.Color("#888888")
	ColorWhite      = lipgloss.Color("#E0E0E0")
	ColorMeta       = lipgloss.Color("#E0E0E0")
)

var (
	Base = lipgloss.NewStyle().
		Foreground(ColorGreen)

	Title = lipgloss.NewStyle().
		Foreground(ColorCyan).
		Bold(true)

	Subtle = lipgloss.NewStyle().
		Foreground(ColorMuted)

	Highlight = lipgloss.NewStyle().
			Foreground(ColorYellow).
			Bold(true)

	// MeHighlight marks the current user's own name — their username as a
	// message author, or a mention of it in someone else's message. White
	// rather than the cyan used elsewhere for "this is you" (e.g. C-Mail),
	// because cIRC already uses cyan for the room title, active border, and
	// tab mnemonics; white stays unclaimed and distinct in that screen.
	MeHighlight = lipgloss.NewStyle().
			Foreground(ColorWhite).
			Bold(true)

	Error = lipgloss.NewStyle().
		Foreground(ColorRed).
		Bold(true)

	Border = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(ColorDimGreen).
		Padding(0, 1)

	ActiveBorder = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(ColorCyan).
			Padding(0, 1)

	StatusBar = lipgloss.NewStyle().
			Background(ColorDimGreen).
			Foreground(ColorBackground).
			Padding(0, 1)

	Tab = lipgloss.NewStyle().
		Foreground(ColorDimGreen).
		Padding(0, 2)

	ActiveTab = lipgloss.NewStyle().
			Background(ColorDimGreen).
			Foreground(ColorGreen).
			Bold(true).
			Padding(0, 2)

	// TabText/ActiveTabText and TabMnemonic/ActiveTabMnemonic are the
	// no-padding building blocks renderTabBar uses to highlight a tab's
	// leader-key mnemonic letter inline. Padding is added manually as literal
	// spaces around the concatenated fragments (see renderTabBar) rather than
	// via .Padding, because each fragment is rendered independently — nesting
	// ANSI-styled spans inside a single .Padding call loses the background
	// on any fragment after the first once its reset code fires.
	TabText = lipgloss.NewStyle().
		Foreground(ColorDimGreen)

	TabMnemonic = lipgloss.NewStyle().
			Foreground(ColorCyan).
			Bold(true)

	ActiveTabText = lipgloss.NewStyle().
			Background(ColorDimGreen).
			Foreground(ColorGreen).
			Bold(true)

	ActiveTabMnemonic = lipgloss.NewStyle().
				Background(ColorDimGreen).
				Foreground(ColorCyan).
				Bold(true)

	// NavMnemonic highlights a leader-key mnemonic letter within the Miller
	// layout's vertical nav sidebar, where rows have no background to
	// preserve, so a single foreground-only style suffices for both the
	// active and inactive row states.
	NavMnemonic = lipgloss.NewStyle().
			Foreground(ColorCyan).
			Bold(true)

	SelectedRow = lipgloss.NewStyle().
			Background(ColorDimGreen).
			Foreground(ColorGreen).
			Bold(true)
)

// Set applies the named theme by reassigning all color and style vars.
// Valid names: "cyber" (default), "c64", "vt320", "bland", "custom".
// Unknown or empty names fall back to "cyber".
func Set(name string) {
	switch name {
	case "c64":
		currentName = "c64"
		setC64()
	case "vt320":
		currentName = "vt320"
		setVT320()
	case "bland":
		currentName = "bland"
		setBland()
	case "custom":
		currentName = "custom"
		applyPalette(customPalette)
	default:
		currentName = "cyber"
		setCyber()
	}
}

// Palette is the set of colors a theme is built from, named after the UI
// role each one drives rather than the color itself — so the theme editor
// and ~/.cyber-tui.json read as "what does this control" instead of raw
// color names. It's the data-driven counterpart to the literal
// setCyber/setC64/etc. themes, used for the user-editable "custom" theme and
// exposed so other packages (the theme editor screen, and a future "detect
// theme in a post" feature) can build and inspect palettes without reaching
// into theme's internals.
//
// Background and CodeBackground are not rendered anywhere in the TUI today —
// there is no fillable full-screen background or code-block background —
// but are kept so a future post-import can carry those two post fields
// through without losing them. They're excluded from the theme editor's
// rows and from Valid()'s required fields.
type Palette struct {
	Foreground string // primary body text, markdown emphasis — matches a post's "Foreground"
	Dimmed     string // secondary/subtle text, strikethrough, separators — matches "Dimmed"
	Border     string // inactive borders, tab bar, code gutters — matches "Border"
	Accent     string // titles, links, active border/tab — no post equivalent
	Highlight  string // @mentions and code text — matches a post's "Code"
	Error      string // error messages/banner — no post equivalent
	BarText    string // text on colored bars: status bar, logo badge, notify banner — no post equivalent
	Self       string // your own username highlight — no post equivalent
	Meta       string // status bar secondary info text & hint key-descriptions — no post equivalent

	// Reserved, unused by the TUI's renderer today; see doc comment above.
	Background     string // matches a post's "Background"
	CodeBackground string // matches a post's "Code BG"
}

var hexColorRe = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

// ValidHex reports whether s is a well-formed "#RRGGBB" hex color.
func ValidHex(s string) bool { return hexColorRe.MatchString(s) }

// validHexOrEmpty reports whether s is either empty or a well-formed hex
// color — used for the reserved fields, which are optional.
func validHexOrEmpty(s string) bool { return s == "" || ValidHex(s) }

// Valid reports whether every rendered field of p is a well-formed hex
// color. The reserved Background/CodeBackground fields are optional.
func (p Palette) Valid() bool {
	return ValidHex(p.Foreground) && ValidHex(p.Dimmed) && ValidHex(p.Border) &&
		ValidHex(p.Accent) && ValidHex(p.Highlight) && ValidHex(p.Error) &&
		ValidHex(p.BarText) && ValidHex(p.Self) && ValidHex(p.Meta) &&
		validHexOrEmpty(p.Background) && validHexOrEmpty(p.CodeBackground)
}

// customPalette is the last palette set via SetCustomPalette, applied when
// Set("custom") is called.
var customPalette Palette

// SetCustomPalette stores p as the "custom" theme's palette. If "custom" is
// the currently active theme, it re-applies immediately for live preview.
func SetCustomPalette(p Palette) {
	customPalette = p
	if currentName == "custom" {
		applyPalette(p)
	}
}

// CurrentPalette returns the colors currently active as a Palette — used to
// prefill the theme editor, and by a future post-theme-detection feature to
// inspect what's on screen right now. The reserved Background/CodeBackground
// fields have no driving Color var, so they're carried over from the stored
// custom palette (empty if none has ever set them).
func CurrentPalette() Palette {
	return Palette{
		Foreground:     string(ColorGreen),
		Dimmed:         string(ColorMuted),
		Border:         string(ColorDimGreen),
		Accent:         string(ColorCyan),
		Highlight:      string(ColorYellow),
		Error:          string(ColorRed),
		BarText:        string(ColorBackground),
		Self:           string(ColorWhite),
		Meta:           string(ColorMeta),
		Background:     customPalette.Background,
		CodeBackground: customPalette.CodeBackground,
	}
}

// postThemeMarker is the fixed marker line cyberspace.online's web UI emits
// at the top of a posted custom-theme block. Its presence (case-insensitive
// substring match) is what triggers detection; everything else is lenient.
const postThemeMarker = "cyberspace custom theme"

// postParseWindow bounds how many lines after the marker ParsePost scans for
// "Key: value" fields — generous for the known fixed format (under 20 lines)
// without letting a false-positive scan run into unrelated later content in
// a long post.
const postParseWindow = 30

// postFieldLineRe matches one "Key: value" line, e.g. "Foreground: #d000ff"
// or "Base Theme: vt320". Lines that don't match this shape (comments like
// "/* Colors */", blank lines) are silently skipped.
var postFieldLineRe = regexp.MustCompile(`^([A-Za-z][A-Za-z ]*?):\s*(.+)$`)

// ParsePost scans content for a posted custom-theme block (see
// docs/40-custom-themes.md) and returns the resulting Palette. Fields the
// block doesn't cover (Accent, Error, BarText, Self, Meta, and any
// missing/malformed color field) are resolved from a base palette: if
// "Base Theme" names one of our own built-ins (case-insensitive), that
// theme's full palette is used; otherwise CurrentPalette() (today's active
// theme). The block's explicit, validly-hex fields are then overlaid on
// top. Returns ok=false only when the marker itself isn't present — an
// otherwise-malformed block still returns ok=true with the base palette
// unchanged for any field it couldn't parse.
func ParsePost(content string) (Palette, bool) {
	lines := strings.Split(content, "\n")
	markerIdx := -1
	for i, line := range lines {
		if strings.Contains(strings.ToLower(line), postThemeMarker) {
			markerIdx = i
			break
		}
	}
	if markerIdx == -1 {
		return Palette{}, false
	}

	end := min(markerIdx+1+postParseWindow, len(lines))
	fields := make(map[string]string)
	for _, line := range lines[markerIdx+1 : end] {
		m := postFieldLineRe.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(m[1]))
		fields[key] = strings.TrimSpace(m[2])
	}

	base := CurrentPalette()
	if name, ok := fields["base theme"]; ok {
		if bp, ok := builtinPalettes[strings.ToLower(strings.TrimSpace(name))]; ok {
			base = bp
		}
	}

	overlay := func(dst *string, key string) {
		if v, ok := fields[key]; ok && ValidHex(v) {
			*dst = v
		}
	}
	overlay(&base.Foreground, "foreground")
	overlay(&base.Background, "background")
	overlay(&base.Dimmed, "dimmed")
	overlay(&base.Border, "border")
	overlay(&base.Highlight, "code")
	overlay(&base.CodeBackground, "code bg")

	return base, true
}

// ExpandHome expands a leading "~" (or "~/...") to the user's home
// directory, matching config.DefaultPath's convention. Paths not starting
// with "~" are returned unchanged. Exported so callers (e.g. App, checking
// whether an export target already exists) resolve paths the same way
// ExportToFile/ImportFromFile do.
func ExpandHome(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") && !strings.HasPrefix(path, `~\`) {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if path == "~" {
		return home, nil
	}
	return filepath.Join(home, path[2:]), nil
}

// ExportToFile writes p as indented JSON to path (mode 0600, matching the
// config file's convention), expanding a leading "~" to the user's home
// directory.
func ExportToFile(path string, p Palette) error {
	resolved, err := ExpandHome(path)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(resolved, data, 0600)
}

// ErrInvalidThemeFile is returned by ImportFromFile when the file parses as
// JSON but doesn't contain a valid palette (missing/malformed colors).
var ErrInvalidThemeFile = errors.New("theme file has missing or invalid colors")

// ImportFromFile reads and parses a theme file previously written by
// ExportToFile (or anything JSON-shaped the same way), expanding a leading
// "~" to the user's home directory. Returns an error if the file is
// missing/unreadable, isn't valid JSON, or parses but fails Valid() — a
// partially-formed file should never silently overwrite a good saved theme.
func ImportFromFile(path string) (Palette, error) {
	resolved, err := ExpandHome(path)
	if err != nil {
		return Palette{}, err
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return Palette{}, err
	}
	var p Palette
	if err := json.Unmarshal(data, &p); err != nil {
		return Palette{}, err
	}
	if !p.Valid() {
		return Palette{}, ErrInvalidThemeFile
	}
	return p, nil
}

// applyPalette sets the rendered color vars from p's driven fields and
// rebuilds all styles. Background/CodeBackground have no Color var to drive.
func applyPalette(p Palette) {
	ColorGreen = lipgloss.Color(p.Foreground)
	ColorDimGreen = lipgloss.Color(p.Border)
	ColorCyan = lipgloss.Color(p.Accent)
	ColorYellow = lipgloss.Color(p.Highlight)
	ColorRed = lipgloss.Color(p.Error)
	ColorBackground = lipgloss.Color(p.BarText)
	ColorMuted = lipgloss.Color(p.Dimmed)
	ColorWhite = lipgloss.Color(p.Self)
	ColorMeta = lipgloss.Color(p.Meta)
	applyStyles()
}

// builtinPalettes holds each built-in theme's full color set as data — the
// single source of truth the set*() functions below apply from, and also
// what lets ParsePost read a named built-in's colors (e.g. to resolve a
// post's "Base Theme: vt320") without actually switching the live theme.
var builtinPalettes = map[string]Palette{
	"cyber": {
		Foreground: "#00FF41",
		Border:     "#007A1F",
		Accent:     "#00FFFF",
		Highlight:  "#FFD700",
		Error:      "#FF003C",
		BarText:    "#0D0D0D",
		Dimmed:     "#888888",
		Self:       "#E0E0E0",
		Meta:       "#E0E0E0",
	},
	// c64 is a Commodore 64-inspired palette: dark blue background, light
	// blue-purple text, white titles, C64 yellow highlights.
	"c64": {
		Foreground: "#9490F5", // C64 light blue-purple (primary text)
		Border:     "#5C5AC0", // C64 medium blue (inactive/borders)
		Accent:     "#FFFFFF", // white (titles)
		Highlight:  "#F4D020", // C64 yellow (highlights)
		Error:      "#883932", // C64 red (errors)
		BarText:    "#3535CE", // C64 cobalt blue (background)
		Dimmed:     "#6B69C4", // dimmed blue-purple (subtle text)
		Self:       "#FFFFFF",
		Meta:       "#FFFFFF",
	},
	// vt320 is an amber phosphor terminal palette inspired by DEC VT320.
	"vt320": {
		Foreground: "#FFB000", // amber (primary text)
		Border:     "#8B5E00", // dark amber (inactive)
		Accent:     "#FFD700", // bright amber (titles)
		Highlight:  "#FFF176", // bright yellow-white (highlights)
		Error:      "#FF6600", // orange-red (errors)
		BarText:    "#1A1200", // near-black amber background
		Dimmed:     "#6B4800", // very dim amber (subtle text)
		Self:       "#FFECB3", // cream
		Meta:       "#FFECB3", // cream
	},
	// bland is no color at all — see setBland for the accompanying
	// Bold/Underline/border-weight overrides that stand in for color.
	"bland": {},
}

// BuiltinPalette returns the named built-in theme's full palette without
// switching to it (name: "cyber"/"c64"/"vt320"/"bland"), or ok=false for
// any other name (including "custom", which has no fixed palette of its
// own — see CurrentPalette/SetCustomPalette for that).
func BuiltinPalette(name string) (Palette, bool) {
	p, ok := builtinPalettes[name]
	return p, ok
}

func setCyber() {
	applyPalette(builtinPalettes["cyber"])
}

func setC64() {
	applyPalette(builtinPalettes["c64"])
}

func setVT320() {
	applyPalette(builtinPalettes["vt320"])
}

// setBland applies no color at all: every style renders in the terminal's
// own default foreground/background, so the theme always matches whatever
// palette the user's terminal is configured with. Active/selected states
// fall back to Bold/Underline and (for the active border) a heavier border
// glyph, since no background or foreground color is forced.
func setBland() {
	applyPalette(builtinPalettes["bland"])

	ActiveBorder = ActiveBorder.BorderStyle(lipgloss.ThickBorder())
	TabMnemonic = TabMnemonic.Underline(true)
	ActiveTabMnemonic = ActiveTabMnemonic.Underline(true)
	NavMnemonic = NavMnemonic.Underline(true)
}

// applyStyles rebuilds all style vars from the current color vars.
// Must be called after any color vars are changed.
func applyStyles() {
	Base = lipgloss.NewStyle().
		Foreground(ColorGreen)

	Title = lipgloss.NewStyle().
		Foreground(ColorCyan).
		Bold(true)

	Subtle = lipgloss.NewStyle().
		Foreground(ColorMuted)

	Highlight = lipgloss.NewStyle().
		Foreground(ColorYellow).
		Bold(true)

	MeHighlight = lipgloss.NewStyle().
		Foreground(ColorWhite).
		Bold(true)

	Error = lipgloss.NewStyle().
		Foreground(ColorRed).
		Bold(true)

	Border = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(ColorDimGreen).
		Padding(0, 1)

	ActiveBorder = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(ColorCyan).
		Padding(0, 1)

	StatusBar = lipgloss.NewStyle().
		Background(ColorDimGreen).
		Foreground(ColorBackground).
		Padding(0, 1)

	Tab = lipgloss.NewStyle().
		Foreground(ColorDimGreen).
		Padding(0, 2)

	ActiveTab = lipgloss.NewStyle().
		Background(ColorDimGreen).
		Foreground(ColorGreen).
		Bold(true).
		Padding(0, 2)

	TabText = lipgloss.NewStyle().
		Foreground(ColorDimGreen)

	TabMnemonic = lipgloss.NewStyle().
		Foreground(ColorCyan).
		Bold(true)

	ActiveTabText = lipgloss.NewStyle().
		Background(ColorDimGreen).
		Foreground(ColorGreen).
		Bold(true)

	ActiveTabMnemonic = lipgloss.NewStyle().
		Background(ColorDimGreen).
		Foreground(ColorCyan).
		Bold(true)

	NavMnemonic = lipgloss.NewStyle().
		Foreground(ColorCyan).
		Bold(true)

	SelectedRow = lipgloss.NewStyle().
		Background(ColorDimGreen).
		Foreground(ColorGreen).
		Bold(true)
}
