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
