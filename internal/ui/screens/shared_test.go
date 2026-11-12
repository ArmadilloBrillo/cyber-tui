package screens

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/ragnar/cyber-tui/internal/model"
)

// TestMessageCopyText covers 'y's precedence: body text first, then
// whichever attachment field is set, so copying a caption-less /gif or
// /song message still copies something useful instead of an empty string.
func TestMessageCopyText(t *testing.T) {
	cases := []struct {
		name string
		msg  model.Message
		want string
	}{
		{"body wins over attachments", model.Message{Body: "hi", ImageUrl: "https://example.com/a.png"}, "hi"},
		{"falls back to ImageUrl", model.Message{ImageUrl: "https://example.com/a.png"}, "https://example.com/a.png"},
		{"falls back to GifUrl", model.Message{GifUrl: "https://example.com/a.gif"}, "https://example.com/a.gif"},
		{"falls back to AudioAttachment.Src", model.Message{AudioAttachment: &model.Attachment{Src: "https://youtu.be/x"}}, "https://youtu.be/x"},
		{"empty message copies nothing", model.Message{}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := messageCopyText(c.msg); got != c.want {
				t.Errorf("messageCopyText() = %q, want %q", got, c.want)
			}
		})
	}
}

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

func TestNormalizeChatBody(t *testing.T) {
	twelve := "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12"
	for name, tc := range map[string]struct{ in, want string }{
		"plain":                {"hello", "hello"},
		"single breaks kept":   {"a\nb\nc", "a\nb\nc"},
		"blank line kept":      {"a\n\nb", "a\n\nb"},
		"blank run collapses":  {"a\n\n\n\nb", "a\n\nb"},
		"whitespace-only run":  {"a\n \n\t\nb", "a\n \nb"},
		"crlf":                 {"a\r\nb\rc", "a\nb\nc"},
		"past tenth joins":     {twelve, "1\n2\n3\n4\n5\n6\n7\n8\n9\n10 11 12"},
		"art untouched":        {"/art\n\n\n x\n" + twelve, "/art\n\n\n x\n" + twelve},
		"art uppercase":        {"/ART\n\n\nx", "/ART\n\n\nx"},
		"art-like word is not": {"/artist\n\n\nx", "/artist\n\nx"},
	} {
		if got := normalizeChatBody(tc.in); got != tc.want {
			t.Errorf("%s: normalizeChatBody(%q) = %q, want %q", name, tc.in, got, tc.want)
		}
	}
}

func TestChatInputOffset_MultiLine(t *testing.T) {
	ta := newChatInput("")
	ta.SetWidth(40)
	ta.Focus()
	ta.SetValue("ab\ncde\nf")
	ta.CursorUp()   // line 1, column clamped to 1 ("f" has length 1)
	ta.SetCursor(2) // "cd|e"

	if got, want := chatInputOffset(ta), len("ab\ncd"); got != want {
		t.Errorf("chatInputOffset = %d, want %d", got, want)
	}
}

func chatInputWith(t *testing.T, v string) textarea.Model {
	t.Helper()
	ta := newChatInput("")
	ta.SetWidth(80)
	ta.Focus()
	ta.SetValue(v)
	return ta
}

func TestUpdateChatInput_NewlineKey(t *testing.T) {
	nl := tea.KeyMsg{Type: tea.KeyEnter, Alt: true}
	ten := strings.TrimSuffix(strings.Repeat("x\n", 10), "\n")
	art := "/art" + strings.Repeat("\n", chatArtMaxLines-1)
	for name, tc := range map[string]struct {
		in   string
		want string
	}{
		"inserts":                {"a", "a\n"},
		"one blank line allowed": {"a\n", "a\n\n"},
		"second blank refused":   {"a\n\n", "a\n\n"},
		"eleventh line refused":  {ten, ten},
		"art keeps blank lines":  {"/art\n\n", "/art\n\n\n"},
		"art 27th line refused":  {art, art},
	} {
		ta, _ := updateChatInput(chatInputWith(t, tc.in), nl, DefaultHardBreakKey)
		if got := ta.Value(); got != tc.want {
			t.Errorf("%s: value = %q, want %q", name, got, tc.want)
		}
	}
}

func TestUpdateChatInput_NewlineKey_MidTextDoubleBlankRefused(t *testing.T) {
	ta := chatInputWith(t, "a\n\nb")
	ta.SetCursor(0) // start of "b"
	ta, _ = updateChatInput(ta, tea.KeyMsg{Type: tea.KeyEnter, Alt: true}, DefaultHardBreakKey)

	if got := ta.Value(); got != "a\n\nb" {
		t.Errorf("value = %q, want unchanged %q", got, "a\n\nb")
	}
}

func TestUpdateChatInput_PasteNormalised(t *testing.T) {
	for name, tc := range map[string]struct{ paste, want string }{
		"crlf":      {"a\r\nb", "a\nb"},
		"blank run": {"a\n\n\n\nb", "a\n\nb"},
		"12 lines":  {"1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12", "1\n2\n3\n4\n5\n6\n7\n8\n9\n10 11 12"},
	} {
		paste := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tc.paste), Paste: true}
		ta, _ := updateChatInput(chatInputWith(t, ""), paste, DefaultHardBreakKey)
		if got := ta.Value(); got != tc.want {
			t.Errorf("%s: value = %q, want %q", name, got, tc.want)
		}
	}
}

func TestFitChatInput_GrowingKeepsFirstLineVisible(t *testing.T) {
	// Built like the screens build it: no SetValue, whose Reset scrolls to
	// the top and would mask the bug.
	ta := newChatInput("")
	ta.SetWidth(40)
	ta.Focus()
	for _, k := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune("one")},
		{Type: tea.KeyEnter, Alt: true},
		{Type: tea.KeyRunes, Runes: []rune("two")},
	} {
		ta, _ = updateChatInput(ta, k, DefaultHardBreakKey)
		ta = fitChatInput(ta)
		_ = ta.View() // the app renders after every key; View feeds textarea's scroll state
	}

	view := ta.View()
	if !strings.Contains(view, "one") || !strings.Contains(view, "two") {
		t.Errorf("expected both lines visible in a 2-row box, got:\n%s", view)
	}
}

func TestFitChatInput_PromptOnlyOnFirstRowOfEachLine(t *testing.T) {
	ta := chatInputWith(t, strings.Repeat("word ", 30)+"\nnext")
	ta = fitChatInput(ta)

	rows := strings.Split(ta.View(), "\n")
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows (2 wrapped + 1), got %d:\n%s", len(rows), ta.View())
	}
	for i, want := range []bool{true, false, true} {
		if got := strings.HasPrefix(rows[i], "> "); got != want {
			t.Errorf("row %d has prompt = %v, want %v: %q", i, got, want, rows[i])
		}
	}
}

func TestRenderChatInput_MatchesTextareaView(t *testing.T) {
	const w = 22 // box width incl. the 2-column prompt; text width 20
	cases := []struct {
		name  string
		value string
		setup func(ta *textarea.Model)
	}{
		{"one line", "hello", nil},
		{"multi-line", "one\ntwo\nthree", nil},
		{"empty middle line", "a\n\nb", nil},
		{"wrapped line", strings.Repeat("word ", 7), nil},
		{"line exactly box width", strings.Repeat("x", 20), nil},
		{"spaces-only longer than width", strings.Repeat(" ", 25), nil},
		{"word longer than width", strings.Repeat("y", 27), nil},
		{"cursor mid-row", "hello world", func(ta *textarea.Model) { ta.SetCursor(3) }},
		{"cursor on earlier line", "one\ntwo", func(ta *textarea.Model) { ta.CursorUp() }},
		{"wide characters", "日本語のテキスト 日本語", nil},
		{"ideographic space and nbsp", "a　b c", nil},
		{"blurred", "hello", func(ta *textarea.Model) { ta.Blur() }},
		{"placeholder focused", "", nil},
		{"placeholder blurred", "", func(ta *textarea.Model) { ta.Blur() }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ta := newChatInput("type a message...")
			ta.SetWidth(w)
			ta.Focus()
			ta.SetValue(c.value)
			if c.setup != nil {
				c.setup(&ta)
			}
			ta = fitChatInput(ta)

			got, want := ansi.Strip(renderChatInput(ta, 0)), ansi.Strip(ta.View())
			if got != want {
				t.Errorf("renderChatInput differs from textarea.View\n got:\n%q\nwant:\n%q", got, want)
			}
		})
	}
}

func chatInputRowsOf(t *testing.T, s string) []string {
	t.Helper()
	return strings.Split(ansi.Strip(s), "\n")
}

func TestRenderChatInput_ShrinkBringsTopRowBack(t *testing.T) {
	ta := chatInputWith(t, "1\n2\n3\n4\n5\n6\n7")
	ta = fitChatInput(ta)
	y := scrollChatInput(ta, 0)
	if y != 2 {
		t.Fatalf("setup: offset = %d, want 2 (cursor on line 7 of a 5-row box)", y)
	}

	ta, _ = updateChatInput(ta, tea.KeyMsg{Type: tea.KeyBackspace}, DefaultHardBreakKey) // "6\n" -> joins
	ta, _ = updateChatInput(ta, tea.KeyMsg{Type: tea.KeyBackspace}, DefaultHardBreakKey)
	ta, _ = updateChatInput(ta, tea.KeyMsg{Type: tea.KeyBackspace}, DefaultHardBreakKey)
	ta, _ = updateChatInput(ta, tea.KeyMsg{Type: tea.KeyBackspace}, DefaultHardBreakKey)
	ta = fitChatInput(ta)
	y = scrollChatInput(ta, y)

	rows := chatInputRowsOf(t, renderChatInput(ta, y))
	if ta.Value() != "1\n2\n3\n4\n5" || !strings.HasPrefix(rows[0], "> 1") {
		t.Errorf("value %q, first row %q; want 5 lines with line 1 visible", ta.Value(), rows[0])
	}
}

func TestRenderChatInput_ViewWithoutUpdateStaysInBox(t *testing.T) {
	ta := chatInputWith(t, "")
	ta = fitChatInput(ta)
	ta.SetValue("1\n2\n3\n4\n5\n6\n7") // no fit, no Update: height still 1

	rows := chatInputRowsOf(t, renderChatInput(ta, 0))
	if len(rows) != ta.Height() || !strings.HasPrefix(rows[0], "> 7") {
		t.Errorf("rows = %q; want %d row(s) showing the cursor line 7", rows, ta.Height())
	}
	for i, r := range rows {
		if lipgloss.Width(r) != 2+ta.Width() {
			t.Errorf("row %d width = %d, want %d", i, lipgloss.Width(r), 2+ta.Width())
		}
	}
}

func TestRenderChatInput_ResetAfterScrollShowsOneRow(t *testing.T) {
	ta := fitChatInput(chatInputWith(t, "1\n2\n3\n4\n5\n6\n7"))
	y := scrollChatInput(ta, 0)
	ta.Reset()
	ta = fitChatInput(ta)
	y = scrollChatInput(ta, y)

	if y != 0 || ta.Height() != 1 {
		t.Errorf("after reset: offset %d, height %d; want 0 and 1", y, ta.Height())
	}
}
