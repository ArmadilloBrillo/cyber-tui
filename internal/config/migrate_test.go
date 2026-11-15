package config_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ragnar/cyber-tui/internal/config"
)

func writeLegacy(t *testing.T, home, content string) string {
	t.Helper()
	p := filepath.Join(home, ".cyber-tui.json")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return p
}

func TestMigrate_MovesLegacyFile(t *testing.T) {
	home := withTempHome(t)
	legacy := writeLegacy(t, home, `{"refreshToken":"tok"}`)

	from, to, err := config.Migrate()
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if from != legacy || to != filepath.Join(home, ".config", "cyber-tui", "config.json") {
		t.Errorf("Migrate = (%q, %q), want (%q, ~/.config/cyber-tui/config.json)", from, to, legacy)
	}
	if _, err := os.Lstat(legacy); !os.IsNotExist(err) {
		t.Errorf("legacy file still present: %v", err)
	}
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(to); err != nil || info.Mode().Perm() != 0o600 {
			t.Errorf("new file mode = %v, err = %v, want 0600", info.Mode().Perm(), err)
		}
		if info, err := os.Stat(filepath.Dir(to)); err != nil || info.Mode().Perm() != 0o700 {
			t.Errorf("config dir mode = %v, err = %v, want 0700", info.Mode().Perm(), err)
		}
	}
	cfg, err := config.Load()
	if err != nil || cfg.RefreshToken != "tok" {
		t.Errorf("Load after migrate = %+v, %v; want refreshToken tok", cfg, err)
	}
}

func TestMigrate_BothPresentNewWins(t *testing.T) {
	home := withTempHome(t)
	legacy := writeLegacy(t, home, `{"refreshToken":"old"}`)
	if err := config.Save(config.Config{RefreshToken: "new"}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	from, _, err := config.Migrate()
	if err != nil || from != "" {
		t.Fatalf("Migrate = from %q, err %v; want no move", from, err)
	}
	if data, _ := os.ReadFile(legacy); string(data) != `{"refreshToken":"old"}` {
		t.Errorf("legacy file changed: %s", data)
	}
	if cfg, _ := config.Load(); cfg.RefreshToken != "new" {
		t.Errorf("Load refreshToken = %q, want new", cfg.RefreshToken)
	}
}

func TestMigrate_NothingToMove(t *testing.T) {
	home := withTempHome(t)
	from, _, err := config.Migrate()
	if err != nil || from != "" {
		t.Fatalf("Migrate = from %q, err %v; want no move", from, err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config")); !os.IsNotExist(err) {
		t.Errorf("Migrate with nothing to move created dirs: %v", err)
	}
	if err := config.Save(config.Config{}); err != nil {
		t.Errorf("Save on fresh install: %v", err)
	}
}

func TestMigrate_FailureNamesBothPaths(t *testing.T) {
	home := withTempHome(t)
	legacy := writeLegacy(t, home, `{}`)
	blocker := filepath.Join(home, "not-a-dir")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", blocker) // config dir can't be created under a file

	_, _, err := config.Migrate()
	if err == nil || !strings.Contains(err.Error(), legacy) || !strings.Contains(err.Error(), "config.json") {
		t.Errorf("Migrate err = %v, want one naming both paths", err)
	}
	if _, statErr := os.Stat(legacy); statErr != nil {
		t.Errorf("legacy file must survive a failed migration: %v", statErr)
	}
}

func TestMigrate_SymlinkWithRelativeTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	home := withTempHome(t)
	dotfiles := filepath.Join(home, "dotfiles")
	if err := os.Mkdir(dotfiles, 0o700); err != nil {
		t.Fatal(err)
	}
	real := filepath.Join(dotfiles, "cyber-tui.json")
	if err := os.WriteFile(real, []byte(`{"refreshToken":"linked"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(home, ".cyber-tui.json")
	if err := os.Symlink(filepath.Join("dotfiles", "cyber-tui.json"), legacy); err != nil {
		t.Fatal(err)
	}

	_, to, err := config.Migrate()
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if _, err := os.Lstat(legacy); !os.IsNotExist(err) {
		t.Errorf("old link still present: %v", err)
	}
	if info, err := os.Lstat(to); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("new path is not a symlink: %v", err)
	}
	if got, _ := filepath.EvalSymlinks(to); got != real {
		t.Errorf("new link resolves to %q, want %q", got, real)
	}
	if cfg, _ := config.Load(); cfg.RefreshToken != "linked" {
		t.Errorf("Load through link: refreshToken = %q, want linked", cfg.RefreshToken)
	}
}

func TestDir_XDGConfigHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows uses %AppData%")
	}
	home := withTempHome(t)
	xdg := filepath.Join(home, "xdg")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	if d, _ := config.Dir(); d != filepath.Join(xdg, "cyber-tui") {
		t.Errorf("absolute XDG_CONFIG_HOME: Dir = %q", d)
	}
	t.Setenv("XDG_CONFIG_HOME", "relative/path")
	if d, _ := config.Dir(); d != filepath.Join(home, ".config", "cyber-tui") {
		t.Errorf("relative XDG_CONFIG_HOME must be ignored: Dir = %q", d)
	}
}
