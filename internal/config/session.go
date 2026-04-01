package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
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

	// App settings — edit manually in ~/.cyber-tui.json.
	// APIBaseURL overrides the default API endpoint (https://api.cyberspace.online).
	APIBaseURL string `json:"apiBaseURL,omitempty"`
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

// Clear removes the config file. Returns nil if the file does not exist.
func Clear() error {
	path, err := DefaultPath()
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
