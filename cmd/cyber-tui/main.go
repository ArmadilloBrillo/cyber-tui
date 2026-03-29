package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/joho/godotenv"
	"github.com/ragnar/cyber-tui/internal/api"
	internalssh "github.com/ragnar/cyber-tui/internal/ssh"
	"github.com/ragnar/cyber-tui/internal/ui"
)

func main() {
	// Load .env if present — silently ignored when the file doesn't exist.
	godotenv.Load() //nolint:errcheck

	// Determine which client to use.
	// Set CYBERSPACE_USE_MOCK=1 to run against mock data (no credentials needed).
	var client api.Client
	if os.Getenv("CYBERSPACE_USE_MOCK") == "1" {
		fmt.Fprintln(os.Stderr, "CYBERSPACE_USE_MOCK=1 — running with mock data")
		client = api.NewMockClient()
	} else {
		baseURL := os.Getenv("CYBERSPACE_API_BASE_URL")
		if baseURL == "" {
			baseURL = "https://api.cyberspace.online"
		}
		client = api.NewHTTPClient(baseURL)
	}

	// SSH server mode
	if os.Getenv("SSH_LISTEN_ADDR") != "" {
		addr := os.Getenv("SSH_LISTEN_ADDR")
		keyPath := os.Getenv("SSH_HOST_KEY_PATH")
		if keyPath == "" {
			keyPath = "./ssh_host_key"
		}
		fmt.Fprintf(os.Stderr, "starting SSH server on %s\n", addr)
		if err := internalssh.Serve(addr, keyPath, client); err != nil {
			fmt.Fprintf(os.Stderr, "ssh server error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Local TUI mode
	app := ui.NewApp(client)
	if email := os.Getenv("CYBERSPACE_EMAIL"); email != "" {
		if password := os.Getenv("CYBERSPACE_PASSWORD"); password != "" {
			app = app.WithAutoLogin(email, password)
		}
	}
	p := tea.NewProgram(
		app,
		tea.WithAltScreen(),
	)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
