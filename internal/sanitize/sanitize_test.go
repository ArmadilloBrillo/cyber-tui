package sanitize_test

import (
	"testing"

	"github.com/ragnar/cyber-tui/internal/sanitize"
)

func TestStrip_RemovesControlCharacters(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"escape sequence", "hi\x1b[31mRED\x1b[0m", "hi[31mRED[0m"},
		{"bell", "ding\a", "ding"},
		{"carriage return", "a\rb", "ab"},
		{"backspace", "ab\bc", "abc"},
		{"del", "x\x7fy", "xy"},
		{"c1 control (NEL)", "x\u0085y", "xy"},
		{"osc title set", "\x1b]0;pwned\a", "]0;pwned"},
		{"preserves tab and newline", "a\tb\nc", "a\tb\nc"},
		{"plain ascii unchanged", "hello world", "hello world"},
		{"unicode unchanged", "cafe — 日本語 ‘quote’", "cafe — 日本語 ‘quote’"},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitize.Strip(tc.in); got != tc.want {
				t.Errorf("Strip(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestStrings_SanitizesNestedStructFields(t *testing.T) {
	type inner struct {
		Bio string
	}
	type wire struct {
		Title    string
		Topics   []string
		Child    *inner
		Children []inner
		Count    int
	}
	w := wire{
		Title:    "good\x1bevil",
		Topics:   []string{"a\x1bb", "clean"},
		Child:    &inner{Bio: "line1\x1b[2Jline2"},
		Children: []inner{{Bio: "x\x07y"}},
		Count:    42,
	}

	sanitize.Strings(&w)

	if w.Title != "goodevil" {
		t.Errorf("Title = %q, want %q", w.Title, "goodevil")
	}
	if w.Topics[0] != "ab" || w.Topics[1] != "clean" {
		t.Errorf("Topics = %v, want [ab clean]", w.Topics)
	}
	if w.Child.Bio != "line1[2Jline2" {
		t.Errorf("Child.Bio = %q, want %q", w.Child.Bio, "line1[2Jline2")
	}
	if w.Children[0].Bio != "xy" {
		t.Errorf("Children[0].Bio = %q, want %q", w.Children[0].Bio, "xy")
	}
	if w.Count != 42 {
		t.Errorf("Count = %d, want 42", w.Count)
	}
}

func TestStrings_IgnoresNilAndNonPointer(t *testing.T) {
	sanitize.Strings(nil)
	var p *struct{ S string }
	sanitize.Strings(p)
	sanitize.Strings(struct{ S string }{S: "x\x1by"})
}
