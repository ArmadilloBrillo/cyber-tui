package screens

import (
	"strings"
	"testing"
	"time"
)

// TestFormatTime_Today checks that a timestamp from today shows time only.
func TestFormatTime_Today(t *testing.T) {
	loc := time.UTC
	now := time.Now().In(loc)
	ts := time.Date(now.Year(), now.Month(), now.Day(), 14, 30, 0, 0, loc)

	got := formatTime(ts, loc, "15:04:05")
	if strings.Contains(got, "-") {
		t.Errorf("today's timestamp should not include a date, got %q", got)
	}
	if got != "14:30:00" {
		t.Errorf("formatTime for today = %q, want %q", got, "14:30:00")
	}
}

// TestFormatTime_Yesterday checks that a past timestamp includes the date.
func TestFormatTime_Yesterday(t *testing.T) {
	loc := time.UTC
	ts := time.Now().In(loc).AddDate(0, 0, -1).Truncate(24 * time.Hour).Add(9*time.Hour + 5*time.Minute)

	got := formatTime(ts, loc, "15:04")
	if !strings.Contains(got, "-") {
		t.Errorf("yesterday's timestamp should include a date, got %q", got)
	}
	if !strings.Contains(got, "09:05") {
		t.Errorf("formatTime for yesterday should contain time 09:05, got %q", got)
	}
}

// TestFormatTime_DifferentTimezone checks that timezone offset is applied.
func TestFormatTime_DifferentTimezone(t *testing.T) {
	utcPlus2 := time.FixedZone("UTC+2", 2*3600)
	// A timestamp at midnight UTC is 02:00 in UTC+2 — still "today" in UTC+2.
	now := time.Now().In(utcPlus2)
	ts := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, utcPlus2)

	got := formatTime(ts, utcPlus2, "15:04")
	if strings.Contains(got, "-") {
		t.Errorf("same-day timestamp in UTC+2 should not include date, got %q", got)
	}
}

// TestFeedModel_SetLocation confirms SetLocation stores the location.
func TestFeedModel_SetLocation(t *testing.T) {
	m := NewFeedModel()
	loc := time.FixedZone("UTC+3", 3*3600)
	m2 := m.SetLocation(loc)
	if m2.location() != loc {
		t.Error("SetLocation did not update location()")
	}
}

// TestFeedModel_SetLocation_NilDefaultsToUTC ensures nil coerces to UTC.
func TestFeedModel_SetLocation_NilDefaultsToUTC(t *testing.T) {
	m := NewFeedModel()
	m2 := m.SetLocation(nil)
	if m2.location() != time.UTC {
		t.Error("SetLocation(nil) should default to time.UTC")
	}
}
