package config_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/ragnar/cyber-tui/internal/config"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
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


func TestSaveAndLoad_CustomPalette(t *testing.T) {
	withTempHome(t)

	want := config.Config{
		Theme: "custom",
		CustomPalette: &theme.Palette{
			Foreground: "#111111", Dimmed: "#222222", Border: "#333333", Accent: "#444444",
			Highlight: "#555555", Error: "#666666", BarText: "#777777", Self: "#888888", Meta: "#999999",
		},
	}
	if err := config.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Theme != "custom" {
		t.Errorf("Theme = %q, want custom", got.Theme)
	}
	if got.CustomPalette == nil {
		t.Fatal("CustomPalette = nil, want saved palette")
	}
	if *got.CustomPalette != *want.CustomPalette {
		t.Errorf("CustomPalette = %+v, want %+v", *got.CustomPalette, *want.CustomPalette)
	}
}

func TestLoad_CustomPaletteAbsentIsNil(t *testing.T) {
	home := withTempHome(t)
	path := filepath.Join(home, ".cyber-tui.json")
	if err := os.WriteFile(path, []byte(`{"theme":"cyber"}`), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.CustomPalette != nil {
		t.Errorf("expected CustomPalette nil when absent from JSON, got %+v", cfg.CustomPalette)
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

func TestGetImageScale(t *testing.T) {
	cases := []struct {
		name string
		in   float64
		want float64
	}{
		{"zero defaults to 1.0", 0, 1.0},
		{"negative defaults to 1.0", -1, 1.0},
		{"within bounds unchanged", 1.5, 1.5},
		{"below min clamps up", 0.05, config.MinImageScale},
		{"above max clamps down", 10, config.MaxImageScale},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := config.Config{ImageScale: c.in}.GetImageScale()
			if got != c.want {
				t.Errorf("GetImageScale(%v) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestGetDitherSharpness(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty defaults to medium", "", "medium"},
		{"rough passes through", "rough", "rough"},
		{"medium passes through", "medium", "medium"},
		{"sharp passes through", "sharp", "sharp"},
		{"crisp passes through", "crisp", "crisp"},
		{"unrecognized defaults to medium", "garbage", "medium"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := config.Config{DitherSharpness: c.in}.GetDitherSharpness()
			if got != c.want {
				t.Errorf("GetDitherSharpness(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
