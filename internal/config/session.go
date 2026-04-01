package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// Session holds the data persisted to ~/.cyber-tui.json after a successful login.
// Only the RefreshToken (long-lived) is stored; short-lived tokens (IDToken,
// RTDBToken) are obtained fresh from /v1/auth/refresh on every startup.
type Session struct {
	RefreshToken string    `json:"refreshToken"`
	Username     string    `json:"username"`
	Email        string    `json:"email"`
	SavedAt      time.Time `json:"savedAt"`
}

// DefaultPath returns the canonical path for the session file: ~/.cyber-tui.json.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cyber-tui.json"), nil
}

// Load reads the session file at the default path.
// Returns an empty Session (not an error) when the file does not exist.
func Load() (Session, error) {
	path, err := DefaultPath()
	if err != nil {
		return Session{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Session{}, nil
		}
		return Session{}, err
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return Session{}, err
	}
	return s, nil
}

// Save writes the session to the default path with mode 0600 (owner read/write only).
func Save(s Session) error {
	path, err := DefaultPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// Clear removes the session file. Returns nil if the file does not exist.
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
