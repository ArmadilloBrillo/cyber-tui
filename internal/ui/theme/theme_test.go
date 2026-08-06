package theme

import (
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
