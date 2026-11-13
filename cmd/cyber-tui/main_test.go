package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateBaseURL(t *testing.T) {
	cases := []struct {
		raw           string
		allowInsecure bool
		wantErr       bool
	}{
		{"", false, false},
		{"https://api.cyberspace.online", false, false},
		{"http://localhost:8080", false, false},
		{"http://127.0.0.1:9000", false, false},
		{"http://evil.com", false, true},
		{"http://evil.com", true, false},
		{"ftp://host/x", false, true},
	}
	for _, c := range cases {
		err := validateBaseURL(c.raw, c.allowInsecure)
		if (err != nil) != c.wantErr {
			t.Errorf("validateBaseURL(%q, %v) err = %v, wantErr %v", c.raw, c.allowInsecure, err, c.wantErr)
		}
	}
}

func TestValidateSSHAddr(t *testing.T) {
	cases := []struct {
		addr        string
		allowRemote bool
		wantErr     bool
	}{
		{"", false, false},
		{":2222", false, true},
		{"localhost:2222", false, false},
		{"127.0.0.1:2222", false, false},
		{"[::1]:2222", false, false},
		{"0.0.0.0:2222", false, true},
		{"evil.example.com:2222", false, true},
		{":2222", true, false},
		{"evil.example.com:2222", true, false},
		{"not-a-valid-addr", false, true},
	}
	for _, c := range cases {
		err := validateSSHAddr(c.addr, c.allowRemote)
		if (err != nil) != c.wantErr {
			t.Errorf("validateSSHAddr(%q, %v) err = %v, wantErr %v", c.addr, c.allowRemote, err, c.wantErr)
		}
	}
}

func TestPruneLogs_KeepsNewestOfPrefix(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{
		"debug-2026-01-01T00-00-00-1.log",
		"debug-2026-01-02T00-00-00-1.log",
		"debug-2026-01-03T00-00-00-1.log",
		"crash-2026-01-01T00-00-00-1.log",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	pruneLogs(dir, "debug", 2)

	for name, want := range map[string]bool{
		"debug-2026-01-01T00-00-00-1.log": false,
		"debug-2026-01-02T00-00-00-1.log": true,
		"debug-2026-01-03T00-00-00-1.log": true,
		"crash-2026-01-01T00-00-00-1.log": true,
	} {
		_, err := os.Stat(filepath.Join(dir, name))
		if got := err == nil; got != want {
			t.Errorf("%s exists=%v, want %v", name, got, want)
		}
	}
}
