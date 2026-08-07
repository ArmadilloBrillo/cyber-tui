package screens

import (
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func runesMsg(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestFilterSlugCharsKeyMsg(t *testing.T) {
	cases := []struct {
		name         string
		in           string
		extraAllowed string
		want         string
		wantKeep     bool
	}{
		{"uppercase passes through unchanged", "MUSIC", "", "MUSIC", true},
		{"mixed case passes through unchanged", "MuSiC", "", "MuSiC", true},
		{"disallowed chars dropped", "mu$ic!", "", "muic", true},
		{"hyphen and digits pass through", "a1-b2", "", "a1-b2", true},
		{"extra allowed passes through", "a,b c", ", ", "a,b c", true},
		{"extra not allowed without opt-in", "a,b c", "", "abc", true},
		{"all disallowed drops message", "!!!", "", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, keep := filterSlugCharsKeyMsg(runesMsg(c.in), c.extraAllowed)
			if keep != c.wantKeep {
				t.Fatalf("keep = %v, want %v", keep, c.wantKeep)
			}
			if keep && string(got.Runes) != c.want {
				t.Errorf("Runes = %q, want %q", string(got.Runes), c.want)
			}
		})
	}
}

func TestFilterSlugCharsKeyMsg_NonRunesPassThrough(t *testing.T) {
	msg := tea.KeyMsg{Type: tea.KeyBackspace}
	got, keep := filterSlugCharsKeyMsg(msg, "")
	if !keep || got.Type != msg.Type {
		t.Errorf("non-KeyRunes message should pass through unchanged, got %+v keep=%v", got, keep)
	}
}

func TestTopicsCount(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"music", 1},
		{"music,linux", 2},
		{"music,linux,tui", 3},
		{"music,,linux,", 2},
		{"music, ,linux", 2},
	}
	for _, c := range cases {
		if got := topicsCount(c.in); got != c.want {
			t.Errorf("topicsCount(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestFilterTopicsKeyMsg(t *testing.T) {
	t.Run("below cap comma passes", func(t *testing.T) {
		got, keep := filterTopicsKeyMsg(runesMsg(","), "music,linux")
		if !keep || string(got.Runes) != "," {
			t.Errorf("Runes = %q keep=%v, want \",\" keep=true", string(got.Runes), keep)
		}
	})

	t.Run("at cap comma dropped", func(t *testing.T) {
		got, keep := filterTopicsKeyMsg(runesMsg(","), "music,linux,tui")
		if keep {
			t.Errorf("expected keep=false, got Runes=%q", string(got.Runes))
		}
	})

	t.Run("at cap letters still extend last topic", func(t *testing.T) {
		got, keep := filterTopicsKeyMsg(runesMsg("x"), "music,linux,tui")
		if !keep || string(got.Runes) != "x" {
			t.Errorf("Runes = %q keep=%v, want \"x\" keep=true", string(got.Runes), keep)
		}
	})

	t.Run("below cap letters and comma both pass", func(t *testing.T) {
		got, keep := filterTopicsKeyMsg(runesMsg("x,"), "music")
		if !keep || string(got.Runes) != "x," {
			t.Errorf("Runes = %q keep=%v, want \"x,\" keep=true", string(got.Runes), keep)
		}
	})

	t.Run("at cap non-rune keys still pass through", func(t *testing.T) {
		for _, kt := range []tea.KeyType{tea.KeyBackspace, tea.KeyLeft, tea.KeyRight, tea.KeyDelete} {
			msg := tea.KeyMsg{Type: kt}
			got, keep := filterTopicsKeyMsg(msg, "music,linux,tui")
			if !keep || got.Type != kt {
				t.Errorf("key %v: keep=%v got=%+v, want keep=true and type preserved", kt, keep, got)
			}
		}
	})

	t.Run("leading comma blocked", func(t *testing.T) {
		got, keep := filterTopicsKeyMsg(runesMsg(","), "")
		if keep {
			t.Errorf("expected keep=false for a leading comma, got Runes=%q", string(got.Runes))
		}
	})

	t.Run("duplicate comma blocked", func(t *testing.T) {
		got, keep := filterTopicsKeyMsg(runesMsg(","), "music,")
		if keep {
			t.Errorf("expected keep=false for a comma right after another comma, got Runes=%q", string(got.Runes))
		}
	})

	t.Run("duplicate comma blocked across trailing space", func(t *testing.T) {
		got, keep := filterTopicsKeyMsg(runesMsg(","), "music, ")
		if keep {
			t.Errorf("expected keep=false for a comma after \"comma space\", got Runes=%q", string(got.Runes))
		}
	})

	t.Run("comma after real content still allowed", func(t *testing.T) {
		got, keep := filterTopicsKeyMsg(runesMsg(","), "music")
		if !keep || string(got.Runes) != "," {
			t.Errorf("Runes = %q keep=%v, want \",\" keep=true", string(got.Runes), keep)
		}
	})

	t.Run("leading comma in a run still lets following letters through", func(t *testing.T) {
		got, keep := filterTopicsKeyMsg(runesMsg(",x"), "")
		if !keep || string(got.Runes) != "x" {
			t.Errorf("Runes = %q keep=%v, want \"x\" keep=true", string(got.Runes), keep)
		}
	})

	t.Run("leading space blocked at start of field", func(t *testing.T) {
		got, keep := filterTopicsKeyMsg(runesMsg(" "), "")
		if keep {
			t.Errorf("expected keep=false for a leading space, got Runes=%q", string(got.Runes))
		}
	})

	t.Run("leading space blocked right after a comma", func(t *testing.T) {
		got, keep := filterTopicsKeyMsg(runesMsg(" "), "music,")
		if keep {
			t.Errorf("expected keep=false for a space right after a comma, got Runes=%q", string(got.Runes))
		}
	})

	t.Run("internal space within a topic allowed", func(t *testing.T) {
		got, keep := filterTopicsKeyMsg(runesMsg(" "), "my")
		if !keep || string(got.Runes) != " " {
			t.Errorf("Runes = %q keep=%v, want \" \" keep=true", string(got.Runes), keep)
		}
	})

	t.Run("double space blocked", func(t *testing.T) {
		got, keep := filterTopicsKeyMsg(runesMsg(" "), "my tag ")
		if keep {
			t.Errorf("expected keep=false for a second consecutive space, got Runes=%q", string(got.Runes))
		}
	})

	t.Run("trailing space before comma blocked", func(t *testing.T) {
		got, keep := filterTopicsKeyMsg(runesMsg(","), "music ")
		if keep {
			t.Errorf("expected keep=false for a comma right after a trailing space, got Runes=%q", string(got.Runes))
		}
	})

	t.Run("comma after a topic with an internal space still allowed", func(t *testing.T) {
		got, keep := filterTopicsKeyMsg(runesMsg(","), "my tag")
		if !keep || string(got.Runes) != "," {
			t.Errorf("Runes = %q keep=%v, want \",\" keep=true", string(got.Runes), keep)
		}
	})
}

func newFocusedTopicsInput(value string) textinput.Model {
	input := textinput.New()
	input.Focus() // textinput.Update no-ops on KeyMsg unless focused
	input.SetValue(value)
	return input
}

func TestUpdateTopicsInput(t *testing.T) {
	t.Run("trailing space trimmed so comma still lands", func(t *testing.T) {
		got, _ := updateTopicsInput(newFocusedTopicsInput("music "), runesMsg(","))
		if want := "music,"; got.Value() != want {
			t.Errorf("Value() = %q, want %q", got.Value(), want)
		}
	})

	t.Run("multiple trailing spaces all trimmed", func(t *testing.T) {
		got, _ := updateTopicsInput(newFocusedTopicsInput("music   "), runesMsg(","))
		if want := "music,"; got.Value() != want {
			t.Errorf("Value() = %q, want %q", got.Value(), want)
		}
	})

	t.Run("comma after comma-then-spaces still blocked", func(t *testing.T) {
		got, _ := updateTopicsInput(newFocusedTopicsInput("music, "), runesMsg(","))
		if want := "music, "; got.Value() != want {
			t.Errorf("Value() = %q, want unchanged %q (empty second topic still refused)", got.Value(), want)
		}
	})

	t.Run("comma with no trailing space unaffected", func(t *testing.T) {
		got, _ := updateTopicsInput(newFocusedTopicsInput("music"), runesMsg(","))
		if want := "music,"; got.Value() != want {
			t.Errorf("Value() = %q, want %q", got.Value(), want)
		}
	})

	t.Run("space after trailing space still blocked (double-space rule untouched)", func(t *testing.T) {
		got, _ := updateTopicsInput(newFocusedTopicsInput("my tag "), runesMsg(" "))
		if want := "my tag "; got.Value() != want {
			t.Errorf("Value() = %q, want unchanged %q", got.Value(), want)
		}
	})

	t.Run("non-KeyMsg passes straight through", func(t *testing.T) {
		got, _ := updateTopicsInput(newFocusedTopicsInput("music"), tea.WindowSizeMsg{})
		if got.Value() != "music" {
			t.Errorf("Value() = %q, want unchanged %q", got.Value(), "music")
		}
	})
}
