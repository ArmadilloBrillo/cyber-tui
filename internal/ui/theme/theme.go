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

// Cyber palette — green-on-black terminal aesthetic
var (
	ColorGreen      = lipgloss.Color("#00FF41")
	ColorDimGreen   = lipgloss.Color("#007A1F")
	ColorCyan       = lipgloss.Color("#00FFFF")
	ColorYellow     = lipgloss.Color("#FFD700")
	ColorRed        = lipgloss.Color("#FF003C")
	ColorBackground = lipgloss.Color("#0D0D0D")
	ColorMuted      = lipgloss.Color("#444444")
	ColorWhite      = lipgloss.Color("#E0E0E0")
)

var (
	Base = lipgloss.NewStyle().
		Background(ColorBackground).
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
		BorderForeground(ColorGreen).
		Padding(0, 1)

	StatusBar = lipgloss.NewStyle().
		Background(ColorDimGreen).
		Foreground(ColorBackground).
		Padding(0, 1)

	Tab = lipgloss.NewStyle().
		Foreground(ColorMuted).
		Padding(0, 2)

	ActiveTab = lipgloss.NewStyle().
		Background(ColorDimGreen).
		Foreground(ColorGreen).
		Bold(true).
		Padding(0, 2)
)
