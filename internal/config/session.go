package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ragnar/cyber-tui/internal/ui/theme"
)

// Config holds all persistent state for the app: session tokens, user preferences,
// and app settings. It is stored as JSON in ~/.cyber-tui.json (mode 0600).
//
// App settings (APIBaseURL, UseMock, etc.) are edited manually in the file.
// Session fields (RefreshToken, Username, Email, SavedAt) are written on login.
// Density is written when the user toggles display density.
type Config struct {
	// Session state — written on successful login.
	RefreshToken string    `json:"refreshToken"`
	Username     string    `json:"username"`
	Email        string    `json:"email"`
	SavedAt      time.Time `json:"savedAt"`

	// User preferences — written on change.
	Density string `json:"density,omitempty"` // "" = dense (default) | "relaxed"
	// Timezone is a fixed UTC offset label, e.g. "UTC+5:30" or "UTC-8".
	// Empty string defaults to UTC.
	Timezone string `json:"timezone,omitempty"`

	// App settings — edit manually in ~/.cyber-tui.json.
	// Theme selects the color palette: "cyber" (default), "c64", "vt320", "bland", "custom".
	Theme string `json:"theme,omitempty"`
	// CustomPalette holds the user-built palette for the "custom" theme,
	// saved from the in-TUI theme editor. Nil until the user saves one.
	CustomPalette *theme.Palette `json:"customPalette,omitempty"`
	// APIBaseURL overrides the default API endpoint (https://api.cyberspace.online).
	APIBaseURL string `json:"apiBaseURL,omitempty"`
	// AllowInsecureAPI permits a plain http:// APIBaseURL to a non-loopback host.
	// Off by default so bearer tokens are never sent in cleartext by accident.
	AllowInsecureAPI bool `json:"allowInsecureApi,omitempty"`
	// UseMock runs the app against mock data (no credentials needed).
	UseMock bool `json:"useMock,omitempty"`
	// Debug enables verbose RTDB debug output.
	Debug bool `json:"debug,omitempty"`
	// AutoEmail and AutoPassword pre-fill credentials for automatic login on startup.
	// Stored in plain text — the file is created with mode 0600.
	AutoEmail    string `json:"autoEmail,omitempty"`
	AutoPassword string `json:"autoPassword,omitempty"`
	// SSHListenAddr enables SSH server mode when non-empty (e.g. ":2222").
	SSHListenAddr string `json:"sshListenAddr,omitempty"`
	// SSHHostKeyPath is the path to the SSH host key file (default: ./ssh_host_key).
	SSHHostKeyPath string `json:"sshHostKeyPath,omitempty"`
	// AllowRemoteSSH permits sshListenAddr to bind a non-loopback address.
	// SSH server mode performs no authentication, so binding beyond loopback
	// exposes a full unauthenticated session to anyone who can reach it.
	// Off by default so a non-loopback sshListenAddr requires two deliberate
	// config choices instead of one.
	AllowRemoteSSH bool `json:"allowRemoteSsh,omitempty"`

	// WanderLust controls the wander mode easter egg, which silently updates
	// the profile location to a random position twice per day.
	// Defaults to false (off) when absent from the JSON file.
	WanderLust bool `json:"wanderLust"`
	// LastWandered records when wander mode last fired.
	// Zero value means it has never run, which triggers an immediate update.
	LastWandered time.Time `json:"lastWandered,omitempty"`

	// MaxThreadDepth controls how many levels of reply nesting are visually
	// indented in the post detail view. 0 is treated as the default (3).
	MaxThreadDepth int `json:"maxThreadDepth,omitempty"`

	// ImageViewer controls how image URLs are opened when a terminal graphics
	// protocol is detected. "terminal" (default) displays the image in a
	// fullscreen modal; "browser" always opens in the OS default browser.
	ImageViewer string `json:"imageViewer,omitempty"`

	// InlineImages enables rendering each post's first image attachment
	// directly in the feed and post detail views, instead of just a link.
	// Experimental — off by default. Still subject to the same graphics
	// protocol detection and ImageViewer/ephemeral-session gating as the
	// fullscreen image viewer.
	InlineImages bool `json:"inlineImages,omitempty"`

	// Layout selects the UI layout. "" or "tabs" = tab bar (default); "miller" = sidebar columns.
	Layout string `json:"layout,omitempty"`

	// GraphicsProtocol overrides automatic terminal graphics-protocol
	// detection. "" (default) autodetects via env vars and a DA1 probe; set
	// to "kitty", "iterm2", "sixel", or "none" to force a choice when
	// autodetection is unreliable — e.g. mintty/Git Bash on Windows, which
	// supports Sixel but doesn't reliably answer the DA1 probe query.
	GraphicsProtocol string `json:"graphicsProtocol,omitempty"`
}

// GetMaxThreadDepth returns MaxThreadDepth, substituting the default (3) when
// the field is zero (i.e. absent from the config file).
func (c Config) GetMaxThreadDepth() int {
	if c.MaxThreadDepth <= 0 {
		return 3
	}
	return c.MaxThreadDepth
}

// DefaultPath returns the canonical path for the config file: ~/.cyber-tui.json.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cyber-tui.json"), nil
}

// Load reads the config file at the default path.
// Returns an empty Config (not an error) when the file does not exist.
func Load() (Config, error) {
	path, err := DefaultPath()
	if err != nil {
		return Config{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, nil
		}
		return Config{}, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return Config{}, err
	}
	return c, nil
}

// Save writes the config to the default path with mode 0600 (owner read/write only).
func Save(c Config) error {
	path, err := DefaultPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// AvailableTimezones is the ordered list of UTC offsets shown in the timezone picker.
// These are fixed offsets — no DST adjustments.
var AvailableTimezones = []string{
	"UTC-12", "UTC-11", "UTC-10", "UTC-9", "UTC-8", "UTC-7", "UTC-6",
	"UTC-5", "UTC-4", "UTC-3", "UTC-2", "UTC-1", "UTC",
	"UTC+1", "UTC+2", "UTC+3", "UTC+4", "UTC+5", "UTC+5:30", "UTC+5:45",
	"UTC+6", "UTC+7", "UTC+8", "UTC+9", "UTC+9:30", "UTC+10", "UTC+11",
	"UTC+12", "UTC+12:45", "UTC+13", "UTC+14",
}

// GetLocation returns the *time.Location for the configured timezone.
// Returns time.UTC when Timezone is empty.
func (c Config) GetLocation() *time.Location {
	return ParseTimezoneLabel(c.Timezone)
}

// ParseTimezoneLabel converts a label like "UTC+5:30" or "UTC-8" to a fixed
// *time.Location. Returns time.UTC for "" or "UTC", and for any label that
// does not parse — the config file is hand-editable, so malformed input must
// degrade to UTC rather than panic or yield a partial offset.
func ParseTimezoneLabel(label string) *time.Location {
	if label == "" || label == "UTC" {
		return time.UTC
	}
	if !strings.HasPrefix(label, "UTC") {
		return time.UTC
	}
	rest := label[3:]
	sign := 1
	if len(rest) > 0 && rest[0] == '-' {
		sign = -1
		rest = rest[1:]
	} else if len(rest) > 0 && rest[0] == '+' {
		rest = rest[1:]
	}
	hours, mins := 0, 0
	if idx := strings.Index(rest, ":"); idx >= 0 {
		if _, err := fmt.Sscanf(rest[:idx], "%d", &hours); err != nil {
			return time.UTC
		}
		if _, err := fmt.Sscanf(rest[idx+1:], "%d", &mins); err != nil {
			return time.UTC
		}
	} else {
		if _, err := fmt.Sscanf(rest, "%d", &hours); err != nil {
			return time.UTC
		}
	}
	offset := sign * (hours*3600 + mins*60)
	return time.FixedZone(label, offset)
}

// IsWanderEnabled reports whether wander mode is active.
func (c Config) IsWanderEnabled() bool {
	return c.WanderLust
}

// WanderInterval is the minimum time between wander mode location updates.
const WanderInterval = 12 * time.Hour

// ShouldWanderNow reports whether a wander mode update should fire right now.
// It returns true when wander is enabled and the last update was either never
// performed (zero timestamp) or occurred more than WanderInterval ago.
func ShouldWanderNow(cfg Config) bool {
	if !cfg.IsWanderEnabled() {
		return false
	}
	return cfg.LastWandered.IsZero() ||
		time.Since(cfg.LastWandered) >= WanderInterval
}
