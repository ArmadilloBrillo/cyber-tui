package screens

import (
	"strings"
	"testing"
)

func TestSubstituteChars_L33t(t *testing.T) {
	got := substituteChars("elite", "m1", []string{styleL33t}, 0)
	if got != "3l173" {
		t.Errorf("substituteChars l33t = %q, want 3l173", got)
	}
}

func TestSubstituteChars_Cursive(t *testing.T) {
	got := substituteChars("a", "m1", []string{styleCursive}, 0)
	if got == "a" {
		t.Error("substituteChars cursive left input unchanged")
	}
	if strings.ContainsRune(got, 'a') {
		t.Errorf("substituteChars cursive = %q, still contains plain 'a'", got)
	}
}

func TestSubstituteChars_Flip(t *testing.T) {
	got := substituteChars("ab", "m1", []string{styleFlip}, 0)
	// flip lowercases, maps each rune, and reverses order: "ab" -> "aq" reversed -> "qɐ"
	want := "qɐ"
	if got != want {
		t.Errorf("substituteChars flip = %q, want %q", got, want)
	}
}

func TestSubstituteChars_NoStyles_ReturnsUnchanged(t *testing.T) {
	got := substituteChars("hello world", "m1", nil, 0)
	if got != "hello world" {
		t.Errorf("substituteChars with no styles = %q, want unchanged", got)
	}
}

func TestSubstituteChars_Glitch_DeterministicAcrossCalls(t *testing.T) {
	a := substituteChars("hello world", "msg-42", []string{styleGlitch}, 3)
	b := substituteChars("hello world", "msg-42", []string{styleGlitch}, 3)
	if a != b {
		t.Errorf("glitch not deterministic: %q vs %q", a, b)
	}
	if len([]rune(a)) != len([]rune("hello world")) {
		t.Errorf("glitch changed rune count: %q", a)
	}
}

func TestSubstituteChars_Glitch_VariesByFrame(t *testing.T) {
	frames := make(map[string]bool)
	for frame := 0; frame < 10; frame++ {
		frames[substituteChars("the quick brown fox", "msg-1", []string{styleGlitch}, frame)] = true
	}
	if len(frames) < 2 {
		t.Error("glitch produced the same output across all frames, expected variation")
	}
}

func TestSubstituteChars_Slow_InsertsMiddleDots(t *testing.T) {
	got := substituteChars("hi", "m1", []string{styleSlow}, 0)
	if got != "h·i" {
		t.Errorf("substituteChars slow = %q, want %q", got, "h·i")
	}
}

func TestSubstituteChars_Slow_SingleCharUnchanged(t *testing.T) {
	got := substituteChars("h", "m1", []string{styleSlow}, 0)
	if got != "h" {
		t.Errorf("substituteChars slow on a single char = %q, want unchanged", got)
	}
}

func TestSubstituteChars_Slow_SameAcrossFrames(t *testing.T) {
	a := substituteChars("hello", "m1", []string{styleSlow}, 0)
	b := substituteChars("hello", "m1", []string{styleSlow}, 7)
	if a != b {
		t.Errorf("slow should be static (frame-independent), got %q vs %q", a, b)
	}
}

func TestSubstituteChars_Wave_TogglesOneMovingPosition(t *testing.T) {
	got0 := substituteChars("abc", "m1", []string{styleWave}, 0)
	if got0 != "Abc" {
		t.Errorf("substituteChars wave frame=0 = %q, want Abc", got0)
	}
	got1 := substituteChars("abc", "m1", []string{styleWave}, 1)
	if got1 != "aBc" {
		t.Errorf("substituteChars wave frame=1 = %q, want aBc", got1)
	}
	got3 := substituteChars("abc", "m1", []string{styleWave}, 3) // wraps: 3%3 == 0
	if got3 != "Abc" {
		t.Errorf("substituteChars wave frame=3 = %q, want Abc (wraps to position 0)", got3)
	}
}

func TestSubstituteChars_Wave_EmptyBody(t *testing.T) {
	got := substituteChars("", "m1", []string{styleWave}, 0)
	if got != "" {
		t.Errorf("substituteChars wave on empty body = %q, want empty", got)
	}
}

func TestBlinkVisible_TogglesEveryPhase(t *testing.T) {
	cases := []struct {
		frame int
		want  bool
	}{
		{0, true}, {1, true}, {3, true},
		{4, false}, {6, false}, {7, false},
		{8, true}, {11, true},
		{12, false},
	}
	for _, c := range cases {
		if got := blinkVisible(c.frame); got != c.want {
			t.Errorf("blinkVisible(%d) = %v, want %v", c.frame, got, c.want)
		}
	}
}

func TestApplyAttributeStyle_NoStyles_ReturnsUnchanged(t *testing.T) {
	got := applyAttributeStyle("plain text", nil)
	if got != "plain text" {
		t.Errorf("applyAttributeStyle with no styles = %q, want unchanged", got)
	}
}

func TestApplyAttributeStyle_ChangesOutputWhenStyled(t *testing.T) {
	withTrueColor(t)
	got := applyAttributeStyle("hello", []string{styleRainbow})
	if got == "hello" {
		t.Error("applyAttributeStyle rainbow left text unstyled")
	}
}

func TestMaskSpoilerBody_PreservesWhitespace(t *testing.T) {
	got := maskSpoilerBody("hi there")
	if !strings.Contains(got, " ") {
		t.Errorf("maskSpoilerBody %q lost whitespace", got)
	}
	if strings.ContainsAny(got, "hitre") {
		t.Errorf("maskSpoilerBody %q leaked original letters", got)
	}
	if len([]rune(got)) != len([]rune("hi there")) {
		t.Errorf("maskSpoilerBody %q changed length", got)
	}
}

func TestHasAnimatedStyle(t *testing.T) {
	cases := []struct {
		styles []string
		want   bool
	}{
		{nil, false},
		{[]string{styleSlow}, false},
		{[]string{styleRainbow, styleQuiet}, false},
		{[]string{styleBlink}, true},
		{[]string{styleWave}, true},
		{[]string{styleGlitch}, true},
		{[]string{styleBlink, styleWave}, true},
	}
	for _, c := range cases {
		if got := hasAnimatedStyle(c.styles); got != c.want {
			t.Errorf("hasAnimatedStyle(%v) = %v, want %v", c.styles, got, c.want)
		}
	}
}
