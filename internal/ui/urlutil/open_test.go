package urlutil_test

import (
	"testing"

	"github.com/ragnar/cyber-tui/internal/ui/urlutil"
)

func TestHTTPURL(t *testing.T) {
	cases := []struct {
		u    string
		want bool
	}{
		{"http://example.com", true},
		{"https://example.com/path?a=1&b=2", true},
		{"https://cyberspace.online/u/neuromancer", true},
		{"file:///etc/passwd", false},
		{"javascript:alert(1)", false},
		{"mailto:a@b.com", false},
		{"ftp://host/file", false},
		{"ms-msdt:/id", false},
		{"/relative/path", false},
		{"example.com", false},
		{"", false},
		{"http://", false},
		{"https://evil.com/\x1b]0;pwned\a", false},
	}
	for _, c := range cases {
		if got := urlutil.HTTPURL(c.u); got != c.want {
			t.Errorf("HTTPURL(%q) = %v, want %v", c.u, got, c.want)
		}
	}
}
