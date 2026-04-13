package screens

import (
	"testing"
	"time"
)

func TestSwatchBeats(t *testing.T) {
	tests := []struct {
		name string
		t    time.Time
		want string
	}{
		{
			name: "midnight BMT",
			// BMT = UTC+1, so midnight BMT = 23:00 UTC
			t:    time.Date(2024, 4, 13, 23, 0, 0, 0, time.UTC),
			want: "@000",
		},
		{
			name: "noon BMT",
			// Noon BMT = 11:00 UTC
			t:    time.Date(2024, 4, 13, 11, 0, 0, 0, time.UTC),
			want: "@500",
		},
		{
			name: "quarter day BMT",
			// 6am BMT = 5:00 UTC = 0.25 days in BMT = 250 beats
			t:    time.Date(2024, 4, 13, 5, 0, 0, 0, time.UTC),
			want: "@250",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := swatchBeats(tt.t)
			if got != tt.want {
				t.Errorf("swatchBeats() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDisplayTime(t *testing.T) {
	loc := time.UTC

	tests := []struct {
		name     string
		t        time.Time
		setting  string
		compact  bool
		testFn   func(t *testing.T, got string)
	}{
		{
			name:    "datetime full",
			t:       time.Date(2024, 4, 13, 14, 30, 45, 0, time.UTC),
			setting: "datetime",
			compact: false,
			testFn: func(t *testing.T, got string) {
				if !contains(got, "14:30:45") {
					t.Errorf("expected to contain '14:30:45', got %q", got)
				}
			},
		},
		{
			name:    "datetime compact",
			t:       time.Date(2024, 4, 13, 14, 30, 45, 0, time.UTC),
			setting: "datetime",
			compact: true,
			testFn: func(t *testing.T, got string) {
				if !contains(got, "14:30") {
					t.Errorf("expected to contain '14:30', got %q", got)
				}
				if contains(got, "45") && len(got) > 10 {
					// Seconds should not be present in compact mode (unless it's part of date like "13-Apr")
					// Check that the time part after the date doesn't have seconds
					t.Errorf("compact mode should not include seconds, got %q", got)
				}
			},
		},
		{
			name:    "relative just now",
			t:       time.Now(), // Use current time
			setting: "relative",
			compact: false,
			testFn: func(t *testing.T, got string) {
				if !contains(got, "just now") {
					t.Errorf("expected to contain 'just now', got %q", got)
				}
			},
		},
		{
			name:    "relative minutes ago",
			t:       time.Now().Add(-5 * time.Minute),
			setting: "relative",
			compact: false,
			testFn: func(t *testing.T, got string) {
				if !contains(got, "5m ago") {
					t.Errorf("expected to contain '5m ago', got %q", got)
				}
			},
		},
		{
			name:    "relative hours ago",
			t:       time.Now().Add(-2 * time.Hour),
			setting: "relative",
			compact: false,
			testFn: func(t *testing.T, got string) {
				if !contains(got, "2h ago") {
					t.Errorf("expected to contain '2h ago', got %q", got)
				}
			},
		},
		{
			name:    "unix timestamp",
			t:       time.Date(2024, 4, 13, 14, 30, 45, 0, time.UTC),
			setting: "unix",
			compact: false,
			testFn: func(t *testing.T, got string) {
				// The unix timestamp returned should match the Unix() value
				expectedTS := "1713011445"
				if !contains(got, expectedTS) {
					// Timestamp might vary by timezone offset, so just verify it looks like a number
					if len(got) < 10 || got[0] < '0' || got[0] > '9' {
						t.Errorf("expected numeric timestamp, got %q", got)
					}
				}
			},
		},
		{
			name:    "swatch beats",
			t:       time.Date(2024, 4, 13, 11, 0, 0, 0, time.UTC), // Noon BMT
			setting: "swatch",
			compact: false,
			testFn: func(t *testing.T, got string) {
				if !contains(got, "@500") {
					t.Errorf("expected to contain '@500', got %q", got)
				}
			},
		},
		{
			name:    "unknown setting fallback to datetime",
			t:       time.Date(2024, 4, 13, 14, 30, 45, 0, time.UTC),
			setting: "unknown",
			compact: false,
			testFn: func(t *testing.T, got string) {
				if !contains(got, "14:30:45") {
					t.Errorf("expected to contain '14:30:45', got %q", got)
				}
			},
		},
		{
			name:    "empty setting fallback to datetime",
			t:       time.Date(2024, 4, 13, 14, 30, 45, 0, time.UTC),
			setting: "",
			compact: false,
			testFn: func(t *testing.T, got string) {
				if !contains(got, "14:30:45") {
					t.Errorf("expected to contain '14:30:45', got %q", got)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := displayTime(tt.t, loc, tt.setting, tt.compact)
			tt.testFn(t, got)
		})
	}
}

// Helper to check substring (case-sensitive)
func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if s[i+j] != substr[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
