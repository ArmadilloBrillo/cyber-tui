package imgview

import "testing"

func TestResolveBadgeIconURL(t *testing.T) {
	cases := []struct {
		code string
		want string
		ok   bool
	}{
		{"pi", "https://cdn.jsdelivr.net/npm/lucide-static/icons/pi.svg", true},
		{"code-filled", "https://cdn.jsdelivr.net/npm/lucide-static/icons/code-filled.svg", true},
		{"ph:crown", "https://raw.githubusercontent.com/phosphor-icons/core/main/assets/regular/crown.svg", true},
		{"lucide-lab:apple-core", "https://raw.githubusercontent.com/lucide-icons/lucide-lab/main/icons/apple-core.svg", true},
		{"", "", false},
		{"unknown-lib:crown", "", false},
		{"../etc/passwd", "", false},
		{"ph:../secrets", "", false},
		{"UPPER", "", false},
		{"has space", "", false},
	}
	for _, c := range cases {
		got, ok := ResolveBadgeIconURL(c.code)
		if ok != c.ok || got != c.want {
			t.Errorf("ResolveBadgeIconURL(%q) = (%q, %v), want (%q, %v)", c.code, got, ok, c.want, c.ok)
		}
	}
}
