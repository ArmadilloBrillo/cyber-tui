package main

import "testing"

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
