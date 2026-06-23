package ui

import tea "github.com/charmbracelet/bubbletea"

// Layout arranges the app's screens and handles navigation for a specific UI paradigm.
// All application state lives on App; Layout provides only method implementations.
type Layout interface {
	View(a App) string
	HandleNav(msg tea.KeyMsg, a App) (App, tea.Cmd, bool)
	DelegateUpdate(msg tea.Msg, a App) (App, tea.Cmd)
	HasFocusedInput(a App) bool
	ContentWidth(termWidth int) int
	// ContentHeight returns the height to send to screens in WindowSizeMsg. Screens subtract
	// theme.ChromeHeight to get viewport height; layouts that use fewer chrome rows must compensate
	// so the viewport fills the available content pane exactly.
	ContentHeight(termHeight int) int
}
