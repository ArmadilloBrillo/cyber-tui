package config_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/ragnar/cyber-tui/internal/config"
)

// withTempHome redirects os.UserHomeDir to a temp directory for the duration
// of the test. It sets both HOME (Unix) and USERPROFILE (Windows) so that
// os.UserHomeDir resolves to the temp dir on all platforms.
func withTempHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	return dir
}

func TestLoadMissing(t *testing.T) {
	withTempHome(t)

	sess, err := config.Load()
	if err != nil {
		t.Fatalf("Load on missing file returned error: %v", err)
	}
	if sess.RefreshToken != "" {
		t.Errorf("expected empty session, got RefreshToken=%q", sess.RefreshToken)
	}
}

func TestSaveAndLoad(t *testing.T) {
	withTempHome(t)

	want := config.Config{
		RefreshToken: "tok-abc",
		Username:     "neuromancer",
		Email:        "case@tessier-ashpool.co",
		SavedAt:      time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
	}

	if err := config.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.RefreshToken != want.RefreshToken {
		t.Errorf("RefreshToken = %q, want %q", got.RefreshToken, want.RefreshToken)
	}
	if got.Username != want.Username {
		t.Errorf("Username = %q, want %q", got.Username, want.Username)
	}
	if got.Email != want.Email {
		t.Errorf("Email = %q, want %q", got.Email, want.Email)
	}
	if !got.SavedAt.Equal(want.SavedAt) {
		t.Errorf("SavedAt = %v, want %v", got.SavedAt, want.SavedAt)
	}
}

func TestClear(t *testing.T) {
	home := withTempHome(t)

	if err := config.Save(config.Config{RefreshToken: "tok"}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := config.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}

	path := filepath.Join(home, ".cyber-tui.json")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("expected file to be removed after Clear")
	}

	// Second Clear must be a no-op (no error).
	if err := config.Clear(); err != nil {
		t.Fatalf("second Clear returned error: %v", err)
	}
}

// --- Wander mode ---

func TestIsWanderEnabled_True(t *testing.T) {
	c := config.Config{WanderLust: true}
	if !c.IsWanderEnabled() {
		t.Error("WanderLust true should be enabled")
	}
}

func TestIsWanderEnabled_False(t *testing.T) {
	c := config.Config{WanderLust: false}
	if c.IsWanderEnabled() {
		t.Error("WanderLust false should be disabled")
	}
}

func TestLoad_WanderLustDefaultsFalse(t *testing.T) {
	home := withTempHome(t)
	// Write a config that has no wanderLust key.
	path := filepath.Join(home, ".cyber-tui.json")
	if err := os.WriteFile(path, []byte(`{"refreshToken":"tok"}`), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.WanderLust {
		t.Error("expected WanderLust to default to false when absent from JSON")
	}
}

func TestLoad_WanderLustExplicitFalsePreserved(t *testing.T) {
	home := withTempHome(t)
	path := filepath.Join(home, ".cyber-tui.json")
	if err := os.WriteFile(path, []byte(`{"wanderLust":false}`), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.WanderLust {
		t.Error("expected WanderLust to stay false when explicitly set to false in JSON")
	}
}

func TestShouldWanderNow_DisabledNeverFires(t *testing.T) {
	c := config.Config{WanderLust: false}
	if config.ShouldWanderNow(c) {
		t.Error("should not wander when disabled")
	}
}

func TestShouldWanderNow_ZeroTimestampFires(t *testing.T) {
	c := config.Config{WanderLust: true} // zero LastWandered = never run
	if !config.ShouldWanderNow(c) {
		t.Error("should wander on first run (zero timestamp)")
	}
}

func TestShouldWanderNow_RecentTimestampNoFire(t *testing.T) {
	c := config.Config{
		WanderLust:   true,
		LastWandered: time.Now().Add(-1 * time.Hour),
	}
	if config.ShouldWanderNow(c) {
		t.Error("should not wander when last update was recent")
	}
}

func TestShouldWanderNow_StaleTimestampFires(t *testing.T) {
	c := config.Config{
		WanderLust:   true,
		LastWandered: time.Now().Add(-13 * time.Hour),
	}
	if !config.ShouldWanderNow(c) {
		t.Error("should wander when last update was >12h ago")
	}
}

func TestShouldWanderNow_ExactlyAtIntervalFires(t *testing.T) {
	c := config.Config{
		WanderLust:   true,
		LastWandered: time.Now().Add(-config.WanderInterval),
	}
	if !config.ShouldWanderNow(c) {
		t.Error("should wander when exactly at the interval boundary")
	}
}

func TestFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits are not supported on Windows")
	}
	home := withTempHome(t)

	if err := config.Save(config.Config{RefreshToken: "tok"}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	path := filepath.Join(home, ".cyber-tui.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0600 {
		t.Errorf("file mode = %04o, want 0600", mode)
	}
}
