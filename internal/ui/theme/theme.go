package theme

import "github.com/charmbracelet/lipgloss"

// Layout constants — shared across app and screens so viewport heights
// are calculated from a single source of truth rather than magic numbers.
const (
	TabBarHeight    = 1 // single text row, no border
	StatusBarHeight = 1
	SeparatorHeight = 1 // blank row between tab bar and screen content
	ChromeHeight    = TabBarHeight + StatusBarHeight + SeparatorHeight // = 3
)

// currentName tracks the last theme passed to Set.
var currentName = "cyber"

// CurrentName returns the name of the currently active theme.
func CurrentName() string {
	return currentName
}

// Color vars — reassigned by Set(). Named after the default Cyber palette.
var (
	ColorGreen      = lipgloss.Color("#00FF41")
	ColorDimGreen   = lipgloss.Color("#007A1F")
	ColorCyan       = lipgloss.Color("#00FFFF")
	ColorYellow     = lipgloss.Color("#FFD700")
	ColorRed        = lipgloss.Color("#FF003C")
	ColorBackground = lipgloss.Color("#0D0D0D")
	ColorMuted      = lipgloss.Color("#888888")
	ColorWhite      = lipgloss.Color("#E0E0E0")
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

	SelectedRow = lipgloss.NewStyle().
		Background(ColorDimGreen).
		Foreground(ColorGreen).
		Bold(true)
)

// Set applies the named theme by reassigning all color and style vars.
// Valid names: "cyber" (default), "c64", "vt320".
// Unknown or empty names fall back to "cyber".
func Set(name string) {
	switch name {
	case "c64":
		currentName = "c64"
		setC64()
	case "vt320":
		currentName = "vt320"
		setVT320()
	default:
		currentName = "cyber"
		setCyber()
	}
}

func setCyber() {
	ColorGreen = lipgloss.Color("#00FF41")
	ColorDimGreen = lipgloss.Color("#007A1F")
	ColorCyan = lipgloss.Color("#00FFFF")
	ColorYellow = lipgloss.Color("#FFD700")
	ColorRed = lipgloss.Color("#FF003C")
	ColorBackground = lipgloss.Color("#0D0D0D")
	ColorMuted = lipgloss.Color("#888888")
	ColorWhite = lipgloss.Color("#E0E0E0")
	applyStyles()
}

// setC64 applies a Commodore 64-inspired palette: dark blue background,
// light blue-purple text, white titles, C64 yellow highlights.
func setC64() {
	ColorGreen = lipgloss.Color("#9490F5")      // C64 light blue-purple (primary text)
	ColorDimGreen = lipgloss.Color("#5C5AC0")   // C64 medium blue (inactive/borders)
	ColorCyan = lipgloss.Color("#FFFFFF")       // white (titles)
	ColorYellow = lipgloss.Color("#F4D020")     // C64 yellow (highlights)
	ColorRed = lipgloss.Color("#883932")        // C64 red (errors)
	ColorBackground = lipgloss.Color("#3535CE") // C64 cobalt blue (background)
	ColorMuted = lipgloss.Color("#6B69C4")      // dimmed blue-purple (subtle text)
	ColorWhite = lipgloss.Color("#FFFFFF")
	applyStyles()
}

// setVT320 applies an amber phosphor terminal palette inspired by DEC VT320.
func setVT320() {
	ColorGreen = lipgloss.Color("#FFB000")      // amber (primary text)
	ColorDimGreen = lipgloss.Color("#8B5E00")   // dark amber (inactive)
	ColorCyan = lipgloss.Color("#FFD700")       // bright amber (titles)
	ColorYellow = lipgloss.Color("#FFF176")     // bright yellow-white (highlights)
	ColorRed = lipgloss.Color("#FF6600")        // orange-red (errors)
	ColorBackground = lipgloss.Color("#1A1200") // near-black amber background
	ColorMuted = lipgloss.Color("#6B4800")      // very dim amber (subtle text)
	ColorWhite = lipgloss.Color("#FFECB3")      // cream
	applyStyles()
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

	SelectedRow = lipgloss.NewStyle().
		Background(ColorDimGreen).
		Foreground(ColorGreen).
		Bold(true)
}
