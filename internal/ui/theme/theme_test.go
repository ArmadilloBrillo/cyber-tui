package theme

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestSet_Cyber(t *testing.T) {
	Set("cyber")
	if ColorBackground != lipgloss.Color("#0D0D0D") {
		t.Errorf("cyber: ColorBackground = %q, want #0D0D0D", ColorBackground)
	}
	if ColorGreen != lipgloss.Color("#00FF41") {
		t.Errorf("cyber: ColorGreen = %q, want #00FF41", ColorGreen)
	}
}

func TestSet_C64(t *testing.T) {
	Set("c64")
	if ColorBackground != lipgloss.Color("#3535CE") {
		t.Errorf("c64: ColorBackground = %q, want #3535CE", ColorBackground)
	}
	if ColorGreen != lipgloss.Color("#9490F5") {
		t.Errorf("c64: ColorGreen (primary) = %q, want #9490F5", ColorGreen)
	}
}

func TestSet_VT320(t *testing.T) {
	Set("vt320")
	if ColorBackground != lipgloss.Color("#1A1200") {
		t.Errorf("vt320: ColorBackground = %q, want #1A1200", ColorBackground)
	}
	if ColorGreen != lipgloss.Color("#FFB000") {
		t.Errorf("vt320: ColorGreen (primary) = %q, want #FFB000", ColorGreen)
	}
}

func TestSet_EmptyFallsToCyber(t *testing.T) {
	Set("c64") // change away from cyber first
	Set("")
	if ColorBackground != lipgloss.Color("#0D0D0D") {
		t.Errorf("empty: ColorBackground = %q, want cyber default #0D0D0D", ColorBackground)
	}
}

func TestSet_UnknownFallsToCyber(t *testing.T) {
	Set("vt320") // change away from cyber first
	Set("matrix")
	if ColorBackground != lipgloss.Color("#0D0D0D") {
		t.Errorf("unknown: ColorBackground = %q, want cyber default #0D0D0D", ColorBackground)
	}
}

func TestCurrentName_TracksSet(t *testing.T) {
	Set("c64")
	if CurrentName() != "c64" {
		t.Errorf("CurrentName() = %q, want c64", CurrentName())
	}
}

func TestCurrentName_FallbackIsCyber(t *testing.T) {
	Set("c64")
	Set("")
	if CurrentName() != "cyber" {
		t.Errorf("CurrentName() after Set(\"\") = %q, want cyber", CurrentName())
	}
}

func TestSet_StylesRebuildOnThemeChange(t *testing.T) {
	Set("cyber")
	cyberFg := Base.GetForeground()

	Set("c64")
	c64Fg := Base.GetForeground()

	if cyberFg == c64Fg {
		t.Error("Base foreground should differ between cyber and c64 themes")
	}
}

func TestValidHex(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"#00FF41", true},
		{"#abcdef", true},
		{"00FF41", false},
		{"#0G0000", false},
		{"#FFF", false},
		{"", false},
	}
	for _, c := range cases {
		if got := ValidHex(c.in); got != c.want {
			t.Errorf("ValidHex(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func testPalette9() Palette {
	return Palette{
		Foreground: "#111111", Dimmed: "#222222", Border: "#333333", Accent: "#444444",
		Highlight: "#555555", Error: "#666666", BarText: "#777777", Self: "#888888", Meta: "#999999",
	}
}

func TestPalette_Valid(t *testing.T) {
	valid := testPalette9()
	if !valid.Valid() {
		t.Error("expected fully hex palette to be valid")
	}
	invalid := valid
	invalid.Accent = "not-a-color"
	if invalid.Valid() {
		t.Error("expected palette with malformed field to be invalid")
	}
}

func TestPalette_Valid_ReservedFieldsOptional(t *testing.T) {
	p := testPalette9() // Background/CodeBackground left empty
	if !p.Valid() {
		t.Error("expected palette with empty reserved fields to be valid")
	}
	p.Background = "not-a-color"
	if p.Valid() {
		t.Error("expected palette with malformed reserved field to be invalid")
	}
}

func TestSet_Custom_AppliesStoredPalette(t *testing.T) {
	p := testPalette9()
	SetCustomPalette(p)
	Set("custom")
	if ColorGreen != lipgloss.Color(p.Foreground) {
		t.Errorf("ColorGreen = %q, want %q", ColorGreen, p.Foreground)
	}
	if ColorBackground != lipgloss.Color(p.BarText) {
		t.Errorf("ColorBackground = %q, want %q", ColorBackground, p.BarText)
	}
	if CurrentName() != "custom" {
		t.Errorf("CurrentName() = %q, want custom", CurrentName())
	}
}

func TestSetCustomPalette_LivePreviewWhenActive(t *testing.T) {
	p1 := testPalette9()
	Set("cyber")
	SetCustomPalette(p1)
	Set("custom")

	p2 := Palette{
		Foreground: "#aaaaaa", Dimmed: "#bbbbbb", Border: "#cccccc", Accent: "#dddddd",
		Highlight: "#eeeeee", Error: "#111111", BarText: "#222222", Self: "#333333", Meta: "#444444",
	}
	SetCustomPalette(p2)
	if ColorGreen != lipgloss.Color(p2.Foreground) {
		t.Errorf("ColorGreen = %q, want live-updated %q", ColorGreen, p2.Foreground)
	}
}

func TestCurrentPalette_RoundTrips(t *testing.T) {
	Set("cyber")
	p := CurrentPalette()
	if p.Foreground != "#00FF41" || p.BarText != "#0D0D0D" {
		t.Errorf("CurrentPalette() = %+v, want cyber's literal hex values", p)
	}
}

func TestBuiltinPalette_KnownName(t *testing.T) {
	p, ok := BuiltinPalette("vt320")
	if !ok {
		t.Fatal("expected ok=true for a known built-in")
	}
	if p.Foreground != "#FFB000" {
		t.Errorf("Foreground = %q, want vt320's literal #FFB000", p.Foreground)
	}
}

func TestBuiltinPalette_UnknownName(t *testing.T) {
	if _, ok := BuiltinPalette("not-a-theme"); ok {
		t.Error("expected ok=false for an unknown name")
	}
	if _, ok := BuiltinPalette("custom"); ok {
		t.Error("expected ok=false for \"custom\" — it has no fixed built-in palette")
	}
}

const examplePostBody = `Check out my new theme!

/* Cyberspace Custom Theme */
Base Theme: vt320
/* Colors */
Foreground: #d000ff
Background: #000000
Dimmed: #270082
Border: #270082
Code: #7A5DFF
Code BG: #5100ff
/* Fonts */
Main Font: geist-mono
Code Font: geist-mono
/* Options */
Disable Text Glow: true
Disable Anti-aliasing: false
Enable Noise: false
Enable CRT Box: false
Enable Ligatures: false
Base Font Size: normal
`

// TestPostFieldLineRe_IgnoresSurroundingDecoration guards the property that
// makes ParsePost tolerant of however a client's composer wraps a field
// line: the regex isn't anchored to the line, so decoration on either side
// is simply outside the match, regardless of what it is.
func TestPostFieldLineRe_IgnoresSurroundingDecoration(t *testing.T) {
	cases := map[string]string{
		"Foreground: #ff5d00":     "#ff5d00",
		"`Foreground: #ff5d00`":   "#ff5d00",
		"> Foreground: #f4a4c0":   "#f4a4c0",
		">> Foreground: #f4a4c0":  "#f4a4c0",
		"> `Foreground: #ff5d00`": "#ff5d00",
		"- Foreground: #ff5d00":   "#ff5d00",
		"**Foreground: #ff5d00**": "#ff5d00",
	}
	for in, want := range cases {
		m := postFieldLineRe.FindStringSubmatch(in)
		if m == nil {
			t.Errorf("postFieldLineRe.FindStringSubmatch(%q) = no match, want value %q", in, want)
			continue
		}
		if m[2] != want {
			t.Errorf("postFieldLineRe.FindStringSubmatch(%q) value = %q, want %q", in, m[2], want)
		}
	}
}

func TestParsePost_NoMarker_NotDetected(t *testing.T) {
	_, ok := ParsePost("just a regular post with no theme in it")
	if ok {
		t.Error("expected ok=false when the marker is absent")
	}
}

func TestParsePost_NoMarker_OneFieldOnly_NotDetected(t *testing.T) {
	_, ok := ParsePost("my new setup, the border looks great: Border: #270082")
	if ok {
		t.Error("expected ok=false when only one field matches and the marker is absent")
	}
}

func TestParsePost_NoMarker_ClusteredFields_DetectedViaFallback(t *testing.T) {
	body := `My Custom Theme
Base Theme: vt320
Foreground: #d000ff
Border: #270082
`
	p, ok := ParsePost(body)
	if !ok {
		t.Fatal("expected ok=true via the field-based fallback when the marker is relabeled")
	}
	vt320 := builtinPalettes["vt320"]
	if p.Accent != vt320.Accent {
		t.Errorf("Accent = %q, want vt320's %q (base theme should still resolve)", p.Accent, vt320.Accent)
	}
	if p.Foreground != "#d000ff" || p.Border != "#270082" {
		t.Errorf("explicit fields not overlaid correctly: %+v", p)
	}
}

func TestParsePost_NoMarker_FieldsTooFarApart_NotDetected(t *testing.T) {
	lines := make([]string, 0, postParseWindow+10)
	lines = append(lines, "Foreground: #d000ff")
	for range postParseWindow + 2 {
		lines = append(lines, "just some unrelated line of text")
	}
	lines = append(lines, "Border: #270082")

	_, ok := ParsePost(strings.Join(lines, "\n"))
	if ok {
		t.Error("expected ok=false when matched fields are spread further apart than postParseWindow")
	}
}

func TestParsePost_KnownBaseTheme_FillsUnmappedFromIt(t *testing.T) {
	p, ok := ParsePost(examplePostBody)
	if !ok {
		t.Fatal("expected ok=true")
	}
	vt320 := builtinPalettes["vt320"]
	if p.Accent != vt320.Accent || p.Error != vt320.Error || p.BarText != vt320.BarText ||
		p.Self != vt320.Self || p.Meta != vt320.Meta {
		t.Errorf("unmapped fields = %+v, want vt320's values %+v", p, vt320)
	}
	if p.Foreground != "#d000ff" || p.Background != "#000000" || p.Dimmed != "#270082" ||
		p.Border != "#270082" || p.Highlight != "#7A5DFF" || p.CodeBackground != "#5100ff" {
		t.Errorf("explicit post fields not overlaid correctly: %+v", p)
	}
}

func TestParsePost_UnknownBaseTheme_FallsBackToCurrentPalette(t *testing.T) {
	Set("c64")
	current := CurrentPalette()

	body := `/* Cyberspace Custom Theme */
Base Theme: not-a-real-theme
/* Colors */
Foreground: #123456
`
	p, ok := ParsePost(body)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if p.Accent != current.Accent || p.Self != current.Self {
		t.Errorf("unmapped fields = %+v, want current active theme's values %+v", p, current)
	}
	if p.Foreground != "#123456" {
		t.Errorf("Foreground = %q, want overlaid #123456", p.Foreground)
	}
}

func TestParsePost_MalformedField_FallsBackToBaseForThatFieldOnly(t *testing.T) {
	body := `/* Cyberspace Custom Theme */
Base Theme: vt320
/* Colors */
Foreground: not-a-color
Border: #270082
`
	p, ok := ParsePost(body)
	if !ok {
		t.Fatal("expected ok=true even with a malformed field")
	}
	vt320 := builtinPalettes["vt320"]
	if p.Foreground != vt320.Foreground {
		t.Errorf("Foreground = %q, want base fallback %q", p.Foreground, vt320.Foreground)
	}
	if p.Border != "#270082" {
		t.Errorf("Border = %q, want overlaid #270082", p.Border)
	}
}

func TestParsePost_NoBaseThemeLine_FallsBackToCurrentPalette(t *testing.T) {
	Set("cyber")
	current := CurrentPalette()

	body := `/* Cyberspace Custom Theme */
/* Colors */
Foreground: #ABCDEF
`
	p, ok := ParsePost(body)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if p.Border != current.Border {
		t.Errorf("Border = %q, want current theme's %q", p.Border, current.Border)
	}
	if p.Foreground != "#ABCDEF" {
		t.Errorf("Foreground = %q, want overlaid #ABCDEF", p.Foreground)
	}
}

// TestParsePost_BacktickWrappedFields_StillOverlay covers the actual format
// cyberspace.online's web UI posts in: each field line wrapped in markdown
// backticks (e.g. "`Foreground: #ff5d00`"), which the field regex doesn't
// match unless the backticks are stripped first.
func TestParsePost_BacktickWrappedFields_StillOverlay(t *testing.T) {
	body := "oooooraaaange\n\n\n\n" +
		"`/* Cyberspace Custom Theme */`\n\n" +
		"`Base Theme: dark`\n\n" +
		"`/* Colors */`\n\n" +
		"`Foreground: #ff5d00`\n\n" +
		"`Background: #131313`\n\n" +
		"`Dimmed: #c1c1c1`\n\n" +
		"`Border: #393939`\n\n" +
		"`Code: #f5f5f5`\n\n" +
		"`Code BG: #393939`\n"

	p, ok := ParsePost(body)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if p.Foreground != "#ff5d00" || p.Background != "#131313" || p.Dimmed != "#c1c1c1" ||
		p.Border != "#393939" || p.Highlight != "#f5f5f5" || p.CodeBackground != "#393939" {
		t.Errorf("backtick-wrapped fields not overlaid correctly: %+v", p)
	}
}

// TestParsePost_BlockquoteWrappedFields_StillOverlay covers another real
// posted format: the block quoted with markdown blockquote "> " markers
// instead of backticks (e.g. "> Foreground: #f4a4c0").
func TestParsePost_BlockquoteWrappedFields_StillOverlay(t *testing.T) {
	body := "I was testing palettes for my ui.\n\n" +
		"> /* Cyberspace Custom Theme */\n>\n" +
		"> Base Theme: vt320\n>\n" +
		"> /* Colors */\n>\n" +
		"> Foreground: #f4a4c0\n>\n" +
		"> Background: #450d59\n>\n" +
		"> Dimmed: #ee719e\n>\n" +
		"> Border: #e63b7a\n>\n" +
		"> Code: #c3d117\n>\n" +
		"> Code BG: #6f760a\n"

	p, ok := ParsePost(body)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if p.Foreground != "#f4a4c0" || p.Background != "#450d59" || p.Dimmed != "#ee719e" ||
		p.Border != "#e63b7a" || p.Highlight != "#c3d117" || p.CodeBackground != "#6f760a" {
		t.Errorf("blockquote-wrapped fields not overlaid correctly: %+v", p)
	}
}

// --- CSS color functions (rgb/rgba/hsl/hsla) ---

func TestParseCSSColor(t *testing.T) {
	cases := []struct {
		in     string
		want   string
		wantOK bool
	}{
		{"#D000FF", "#D000FF", true},                    // plain hex passes through unchanged
		{"rgb(255,0,128)", "#FF0080", true},              // no spaces, no alpha
		{"rgb(255, 0, 128)", "#FF0080", true},            // spaces after commas
		{"rgba(255, 0, 128, 0.8)", "#CC0066", true},      // alpha composited against black
		{"hsl(0,0%,100%)", "#ffffff", true},              // white, no alpha
		{"hsla(0,0%,100%,.75)", "#BFBFBF", true},         // alpha composited against black
		{"hsla(0,0%,100%,40%)", "#666666", true},         // alpha as a percentage
		{"hsla(0, 0%, 0%, 1)", "#000000", true},          // alpha=1, black stays black
		{"rgb(255,0)", "", false},                        // too few components
		{"rgb(300,0,0)", "", false},                      // out of range
		{"rgb(x,0,0)", "", false},                        // non-numeric
		{"rgba(255,0,0,1.5)", "", false},                 // alpha out of range
		{"rgba(255,0,0,x)", "", false},                   // alpha non-numeric
		{"cmyk(0,0,0,0)", "", false},                     // unrecognized function
		{"hsla(0,0%,100%,.75", "", false},                // unclosed paren
		{"vt320", "", false},                             // bare word, not a color
	}
	for _, c := range cases {
		got, ok := parseCSSColor(c.in)
		if ok != c.wantOK {
			t.Errorf("parseCSSColor(%q) ok = %v, want %v", c.in, ok, c.wantOK)
			continue
		}
		if ok && !strings.EqualFold(got, c.want) {
			t.Errorf("parseCSSColor(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParsePost_HSLAField_OverlaysConvertedHex(t *testing.T) {
	body := `/* Cyberspace Custom Theme */
Base Theme: vt320
Foreground: hsla(0,0%,100%,.75)
Border: rgb(39,57,57)
`
	p, ok := ParsePost(body)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !strings.EqualFold(p.Foreground, "#BFBFBF") {
		t.Errorf("Foreground = %q, want #BFBFBF (converted from hsla, alpha composited against black)", p.Foreground)
	}
	if !strings.EqualFold(p.Border, "#273939") {
		t.Errorf("Border = %q, want #273939 (converted from rgb)", p.Border)
	}
	vt320 := builtinPalettes["vt320"]
	if p.Dimmed != vt320.Dimmed {
		t.Errorf("Dimmed = %q, want vt320's base value %q (field not present in block)", p.Dimmed, vt320.Dimmed)
	}
}

// --- Export / Import ---

func TestExportImport_RoundTrips(t *testing.T) {
	p := testPalette9()
	path := filepath.Join(t.TempDir(), "theme.json")

	if err := ExportToFile(path, p); err != nil {
		t.Fatalf("ExportToFile: %v", err)
	}
	got, err := ImportFromFile(path)
	if err != nil {
		t.Fatalf("ImportFromFile: %v", err)
	}
	if got != p {
		t.Errorf("round-tripped palette = %+v, want %+v", got, p)
	}
}

func TestExportToFile_ExpandsHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	p := testPalette9()
	if err := ExportToFile("~/theme.json", p); err != nil {
		t.Fatalf("ExportToFile: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "theme.json")); err != nil {
		t.Errorf("expected file at expanded home path: %v", err)
	}
}

func TestImportFromFile_MissingFile(t *testing.T) {
	_, err := ImportFromFile(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if err == nil {
		t.Error("expected an error for a missing file")
	}
}

func TestImportFromFile_InvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "theme.json")
	if err := os.WriteFile(path, []byte("not json"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := ImportFromFile(path)
	if err == nil {
		t.Error("expected an error for invalid JSON")
	}
}

func TestImportFromFile_ValidJSON_InvalidPalette(t *testing.T) {
	path := filepath.Join(t.TempDir(), "theme.json")
	// Valid JSON, but missing required color fields (all empty strings).
	if err := os.WriteFile(path, []byte(`{"Foreground":"not-a-color"}`), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := ImportFromFile(path)
	if !errors.Is(err, ErrInvalidThemeFile) {
		t.Errorf("expected ErrInvalidThemeFile, got %v", err)
	}
}
