package config_test

import (
	"testing"
	"time"

	"github.com/ragnar/cyber-tui/internal/config"
)

func TestParseTimezoneLabel_UTC(t *testing.T) {
	for _, label := range []string{"", "UTC"} {
		loc := config.ParseTimezoneLabel(label)
		if loc != time.UTC {
			t.Errorf("ParseTimezoneLabel(%q) = %v, want time.UTC", label, loc)
		}
	}
}

func TestParseTimezoneLabel_PositiveOffset(t *testing.T) {
	loc := config.ParseTimezoneLabel("UTC+2")
	_, offset := time.Now().In(loc).Zone()
	if offset != 2*3600 {
		t.Errorf("UTC+2 offset = %d seconds, want %d", offset, 2*3600)
	}
}

func TestParseTimezoneLabel_NegativeOffset(t *testing.T) {
	loc := config.ParseTimezoneLabel("UTC-5")
	_, offset := time.Now().In(loc).Zone()
	if offset != -5*3600 {
		t.Errorf("UTC-5 offset = %d seconds, want %d", offset, -5*3600)
	}
}

func TestParseTimezoneLabel_HalfHour(t *testing.T) {
	loc := config.ParseTimezoneLabel("UTC+5:30")
	_, offset := time.Now().In(loc).Zone()
	if offset != 5*3600+30*60 {
		t.Errorf("UTC+5:30 offset = %d seconds, want %d", offset, 5*3600+30*60)
	}
}

func TestParseTimezoneLabel_ZoneName(t *testing.T) {
	loc := config.ParseTimezoneLabel("UTC+3")
	name, _ := time.Now().In(loc).Zone()
	if name != "UTC+3" {
		t.Errorf("zone name = %q, want %q", name, "UTC+3")
	}
}

func TestGetLocation_Empty(t *testing.T) {
	cfg := config.Config{}
	if cfg.GetLocation() != time.UTC {
		t.Error("GetLocation on empty Timezone should return time.UTC")
	}
}

func TestGetLocation_Set(t *testing.T) {
	cfg := config.Config{Timezone: "UTC+1"}
	loc := cfg.GetLocation()
	_, offset := time.Now().In(loc).Zone()
	if offset != 3600 {
		t.Errorf("UTC+1 offset = %d, want 3600", offset)
	}
}

func TestAvailableTimezones_ContainsUTC(t *testing.T) {
	found := false
	for _, tz := range config.AvailableTimezones {
		if tz == "UTC" {
			found = true
			break
		}
	}
	if !found {
		t.Error("AvailableTimezones must contain UTC")
	}
}

func TestAvailableTimezones_AllParseable(t *testing.T) {
	for _, label := range config.AvailableTimezones {
		loc := config.ParseTimezoneLabel(label)
		if loc == nil {
			t.Errorf("ParseTimezoneLabel(%q) returned nil", label)
		}
	}
}
