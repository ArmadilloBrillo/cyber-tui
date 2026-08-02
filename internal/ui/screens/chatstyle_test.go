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
		{[]string{styleBlink}, false},
		{[]string{styleRainbow, styleQuiet}, false},
		{[]string{styleSlow}, true},
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
