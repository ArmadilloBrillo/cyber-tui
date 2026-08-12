package main

import (
	"flag"
	"fmt"
	"log"
	"net"
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
	if cfg.CustomPalette != nil {
		theme.SetCustomPalette(*cfg.CustomPalette)
	}
	theme.Set(cfg.Theme)

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
		if err := validateSSHAddr(cfg.SSHListenAddr, cfg.AllowRemoteSSH); err != nil {
			fmt.Fprintf(os.Stderr, "config: %v\n", err)
			os.Exit(1)
		}
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
	gfxProto := imgview.DetectProtocol()
	// Sixel terminals don't set a reliable env var, so probe only when env-var
	// detection came up empty — Kitty/iTerm2 are higher fidelity when known.
	if gfxProto == imgview.ProtocolNone && imgview.ProbeSixel(os.Stdin, os.Stdout) {
		gfxProto = imgview.ProtocolSixel
	}
	app := ui.NewApp(newClient()).WithGraphicsProtocol(gfxProto)
	// Prefer saved session (token-based) over autoEmail/autoPassword credentials.
	if cfg.RefreshToken != "" {
		app = app.WithSavedSession(cfg)
	} else if cfg.AutoEmail != "" && cfg.AutoPassword != "" {
		app = app.WithAutoLogin(cfg.AutoEmail, cfg.AutoPassword)
	} else if cfg.Email != "" {
		app = app.WithSavedEmail(cfg.Email)
	}
	opts := []tea.ProgramOption{tea.WithAltScreen()}
	// CYBERSPACE_DEBUG_KEYS logs every raw tea.KeyMsg (key + KeyType + runes) to
	// cyber-tui-keys.log, to diagnose terminal-specific keybinding quirks (e.g.
	// a terminal not sending the expected byte for a given ctrl-combo) without
	// having to instrument app logic — see docs/00-project-reference.md.
	if os.Getenv("CYBERSPACE_DEBUG_KEYS") != "" {
		logFile, err := tea.LogToFile("cyber-tui-keys.log", "")
		if err != nil {
			fmt.Fprintf(os.Stderr, "CYBERSPACE_DEBUG_KEYS: %v\n", err)
		} else {
			defer logFile.Close()
			opts = append(opts, tea.WithFilter(func(_ tea.Model, msg tea.Msg) tea.Msg {
				if km, ok := msg.(tea.KeyMsg); ok {
					log.Printf("key: %q type=%v runes=%v alt=%v", km.String(), km.Type, km.Runes, km.Alt)
				}
				return msg // pure observer — never alters the message
			}))
		}
	}
	// cfg.Debug ("debug": true in ~/.cyber-tui.json) enables verbose RTDB
	// output (api.HTTPClient.isDebug) — redirect the standard log package to
	// a file for the run so that output, wherever it's logged from, never
	// hits the terminal and corrupts the alt-screen display.
	if cfg.Debug {
		logFile, err := tea.LogToFile("cyber-tui-debug.log", "")
		if err != nil {
			fmt.Fprintf(os.Stderr, "debug: %v\n", err)
		} else {
			defer logFile.Close()
		}
	}
	p := tea.NewProgram(app, opts...)
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

// validateSSHAddr rejects a non-loopback SSH listen address unless
// allowRemote is set. SSH server mode performs no authentication, so an
// address like ":2222" (all interfaces) would otherwise expose a full,
// unauthenticated session to anyone who can reach it from a single
// misconfigured field.
func validateSSHAddr(addr string, allowRemote bool) error {
	if addr == "" || allowRemote {
		return nil
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid sshListenAddr %q: %w", addr, err)
	}
	switch host {
	case "localhost", "127.0.0.1", "::1":
		return nil
	default:
		return fmt.Errorf("sshListenAddr %q binds a non-loopback address; SSH server mode is unauthenticated. "+
			"Use a loopback address, or set allowRemoteSsh: true to expose it intentionally", addr)
	}
}
