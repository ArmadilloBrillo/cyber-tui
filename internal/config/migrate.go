package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Migrate moves a legacy ~/.cyber-tui.json to DefaultPath. It returns a
// non-empty from only when this call moved the file. When both exist the new
// file wins and the legacy one is left untouched, since it may hold different
// data (e.g. written by an older version after a downgrade).
func Migrate() (from, to string, err error) {
	to, err = DefaultPath()
	if err != nil {
		return "", "", err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", to, err
	}
	legacy := filepath.Join(home, ".cyber-tui.json")

	if _, err := os.Stat(to); err == nil {
		return "", to, nil
	}
	if _, err := os.Lstat(legacy); errors.Is(err, os.ErrNotExist) {
		return "", to, nil
	} else if err != nil {
		return "", to, fmt.Errorf("config: %w", err)
	}

	err = os.MkdirAll(filepath.Dir(to), 0o700)
	if err == nil {
		err = move(legacy, to)
	}
	if err != nil {
		// Another instance may have migrated it between our checks and the move.
		if _, statErr := os.Stat(to); statErr == nil {
			return "", to, nil
		}
		return "", to, fmt.Errorf("config: move %s to %s (move it by hand): %w", legacy, to, err)
	}
	return legacy, to, nil
}

// rename is swapped in tests to simulate a cross-filesystem rename failure
// or another instance migrating first.
var rename = os.Rename

// move renames from to to, falling back to copy-then-remove (e.g. across
// filesystems). A symlinked from (dotfile managers) is recreated as a link to
// its absolute target: renaming a relative link into another dir breaks it.
func move(from, to string) error {
	if info, err := os.Lstat(from); err == nil && info.Mode()&os.ModeSymlink != 0 {
		if target, err := filepath.EvalSymlinks(from); err == nil {
			if err := os.Symlink(target, to); err == nil {
				os.Remove(from)
				return nil
			}
		}
		return copyThenRemove(from, to)
	}
	if err := rename(from, to); err == nil {
		return nil
	}
	return copyThenRemove(from, to)
}

// copyThenRemove copies from to a new file at to (0600). Failing to remove
// from afterwards is not an error: the config is already safe at to.
func copyThenRemove(from, to string) error {
	data, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(to) // don't leave a partial file that would win next time
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(to)
		return err
	}
	os.Remove(from)
	return nil
}
