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
	if ColorBackground != lipgloss.Color("#352879") {
		t.Errorf("c64: ColorBackground = %q, want #352879", ColorBackground)
	}
	if ColorGreen != lipgloss.Color("#7869C4") {
		t.Errorf("c64: ColorGreen (primary) = %q, want #7869C4", ColorGreen)
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
