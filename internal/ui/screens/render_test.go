package screens

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/ragnar/cyber-tui/internal/model"
)

var circMsgTime = time.Now().UTC()

func circMsg(username, body string) model.Message {
	return model.Message{
		From:      model.User{Username: username},
		Body:      body,
		CreatedAt: circMsgTime,
	}
}

// TestRenderCircMessages_WrapsLongBodyWithinWidth guards against the overflow
// bug where a long single-line message pushed the trailing timestamp past
// the viewport edge instead of wrapping.
func TestRenderCircMessages_WrapsLongBodyWithinWidth(t *testing.T) {
	const width = 60
	body := strings.Repeat("a very long word soup that keeps going and going ", 5)
	out := renderCircMessages([]model.Message{circMsg("alice", body)}, time.UTC, "datetime", width)

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected the long message to wrap onto multiple lines, got %d line(s)", len(lines))
	}
	for i, l := range lines {
		if w := lipgloss.Width(l); w > width {
			t.Errorf("line %d width %d exceeds viewport width %d: %q", i, w, width, l)
		}
	}
	ts := displayTime(circMsgTime, time.UTC, "datetime", true)
	last := lines[len(lines)-1]
	if !strings.Contains(last, ts) {
		t.Errorf("expected timestamp %q on last line, got %q", ts, last)
	}
}

// TestRenderCircMessages_TimestampHasGapFromText ensures at least a small
// fixed gap separates the message text from the trailing timestamp, even
// when the last wrapped line runs close to the viewport edge.
func TestRenderCircMessages_TimestampHasGapFromText(t *testing.T) {
	const width = 60
	out := renderCircMessages([]model.Message{circMsg("bob", "short reply")}, time.UTC, "datetime", width)

	line := strings.TrimRight(out, "\n")
	ts := displayTime(circMsgTime, time.UTC, "datetime", true)
	idx := strings.LastIndex(line, ts)
	if idx == -1 {
		t.Fatalf("timestamp not found in output: %q", line)
	}
	beforeTs := line[:idx]
	trimmed := strings.TrimRight(beforeTs, " ")
	gap := len(beforeTs) - len(trimmed)
	if gap < 2 {
		t.Errorf("expected at least a 2-space gap before the timestamp, got %d: %q", gap, line)
	}
	if w := lipgloss.Width(line); w > width {
		t.Errorf("line width %d exceeds viewport width %d: %q", w, width, line)
	}
}

// TestRenderCircMessages_AdminBadge confirms the [admin] tag renders only for
// IsChatAdmin messages, and that the badge's width is folded into the prefix
// so timestamp alignment isn't thrown off (the same overflow class of bug the
// word-wrap fix guards against).
func TestRenderCircMessages_AdminBadge(t *testing.T) {
	const width = 60
	admin := circMsg("alice", "welcome to the room")
	admin.IsChatAdmin = true
	regular := circMsg("bob", "hi")

	out := renderCircMessages([]model.Message{admin, regular}, time.UTC, "datetime", width)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")

	if !strings.Contains(lines[0], "[admin]") {
		t.Errorf("expected [admin] badge on the admin's line, got: %q", lines[0])
	}
	for i, l := range lines[1:] {
		if strings.Contains(l, "[admin]") {
			t.Errorf("did not expect [admin] badge on non-admin line %d: %q", i+1, l)
		}
	}
	for i, l := range lines {
		if w := lipgloss.Width(l); w > width {
			t.Errorf("line %d width %d exceeds viewport width %d: %q", i, w, width, l)
		}
	}
}

// TestRenderCircMessages_AdminBadgeWrapsCorrectly guards against the badge's
// width not being folded into rawPrefixWidth, which would misalign wrapped
// continuation lines and the trailing timestamp for a long admin message.
func TestRenderCircMessages_AdminBadgeWrapsCorrectly(t *testing.T) {
	const width = 60
	msg := circMsg("alice", strings.Repeat("a very long word soup that keeps going ", 5))
	msg.IsChatAdmin = true

	out := renderCircMessages([]model.Message{msg}, time.UTC, "datetime", width)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected the long admin message to wrap onto multiple lines, got %d line(s)", len(lines))
	}
	for i, l := range lines {
		if w := lipgloss.Width(l); w > width {
			t.Errorf("line %d width %d exceeds viewport width %d: %q", i, w, width, l)
		}
	}
	ts := displayTime(circMsgTime, time.UTC, "datetime", true)
	last := lines[len(lines)-1]
	if !strings.Contains(last, ts) {
		t.Errorf("expected timestamp %q on last line, got %q", ts, last)
	}
}
