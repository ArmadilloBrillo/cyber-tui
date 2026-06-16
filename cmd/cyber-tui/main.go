package main

import (
	"flag"
	"fmt"
	"net/url"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ragnar/cyber-tui/internal/api"
	"github.com/ragnar/cyber-tui/internal/config"
	internalssh "github.com/ragnar/cyber-tui/internal/ssh"
	"github.com/ragnar/cyber-tui/internal/ui"
	"github.com/ragnar/cyber-tui/internal/ui/imgview"
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
	gfxProto := imgview.DetectProtocol()

	if !cfg.UseMock {
		if err := validateBaseURL(cfg.APIBaseURL, cfg.AllowInsecureAPI); err != nil {
			fmt.Fprintf(os.Stderr, "config: %v\n", err)
			os.Exit(1)
		}
	}

	// newClient builds a fresh API client. Set useMock: true in ~/.cyber-tui.json
	// to run against mock data. SSH server mode calls this once per connection so
	// sessions never share authentication state.
	newClient := func() api.Client {
		if cfg.UseMock {
			return api.NewMockClient()
		}
		baseURL := cfg.APIBaseURL
		if baseURL == "" {
			baseURL = "https://api.cyberspace.online"
		}
		return api.NewHTTPClient(baseURL).WithDebug(cfg.Debug)
	}
	if cfg.UseMock {
		fmt.Fprintln(os.Stderr, "useMock=true — running with mock data")
	}

	// SSH server mode (experimental)
	if cfg.SSHListenAddr != "" {
		keyPath := cfg.SSHHostKeyPath
		if keyPath == "" {
			keyPath = "./ssh_host_key"
		}
		fmt.Fprintf(os.Stderr, "WARNING: SSH server mode is experimental and unauthenticated; "+
			"anyone who can reach %s gets a session. Restrict network exposure.\n", cfg.SSHListenAddr)
		fmt.Fprintf(os.Stderr, "starting SSH server on %s\n", cfg.SSHListenAddr)
		if err := internalssh.Serve(cfg.SSHListenAddr, keyPath, newClient); err != nil {
			fmt.Fprintf(os.Stderr, "ssh server error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Local TUI mode
	app := ui.NewApp(newClient()).WithGraphicsProtocol(gfxProto)
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

// validateBaseURL rejects an API base URL that would send bearer tokens in
// cleartext. https is always allowed; http is allowed only for loopback hosts or
// when allowInsecure is set. An empty URL uses the https default elsewhere.
func validateBaseURL(raw string, allowInsecure bool) error {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid apiBaseURL %q: %w", raw, err)
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		host := u.Hostname()
		if allowInsecure || host == "localhost" || host == "127.0.0.1" || host == "::1" {
			return nil
		}
		return fmt.Errorf("apiBaseURL %q uses http; bearer tokens would be sent in cleartext. "+
			"Use https, or set allowInsecureApi: true for a non-loopback dev server", raw)
	default:
		return fmt.Errorf("apiBaseURL %q must use http or https", raw)
	}
}
