package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ragnar/cyber-tui/internal/api"
	"github.com/ragnar/cyber-tui/internal/config"
	internalssh "github.com/ragnar/cyber-tui/internal/ssh"
	"github.com/ragnar/cyber-tui/internal/ui"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
	"github.com/ragnar/cyber-tui/internal/version"
)

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Printf("cyber-tui %s (commit %s, built %s)\n",
			version.Version, version.Commit, version.Date)
		os.Exit(0)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}
	theme.Set(cfg.Theme)

	// Determine which client to use.
	// Set useMock: true in ~/.cyber-tui.json to run against mock data.
	var client api.Client
	if cfg.UseMock {
		fmt.Fprintln(os.Stderr, "useMock=true — running with mock data")
		client = api.NewMockClient()
	} else {
		baseURL := cfg.APIBaseURL
		if baseURL == "" {
			baseURL = "https://api.cyberspace.online"
		}
		client = api.NewHTTPClient(baseURL).WithDebug(cfg.Debug)
	}

	// SSH server mode
	if cfg.SSHListenAddr != "" {
		keyPath := cfg.SSHHostKeyPath
		if keyPath == "" {
			keyPath = "./ssh_host_key"
		}
		fmt.Fprintf(os.Stderr, "starting SSH server on %s\n", cfg.SSHListenAddr)
		if err := internalssh.Serve(cfg.SSHListenAddr, keyPath, client); err != nil {
			fmt.Fprintf(os.Stderr, "ssh server error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Local TUI mode
	app := ui.NewApp(client)
	// Prefer saved session (token-based) over autoEmail/autoPassword credentials.
	if cfg.RefreshToken != "" {
		app = app.WithSavedSession(cfg)
	} else if cfg.AutoEmail != "" && cfg.AutoPassword != "" {
		app = app.WithAutoLogin(cfg.AutoEmail, cfg.AutoPassword)
	} else if cfg.Email != "" {
		app = app.WithSavedEmail(cfg.Email)
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
