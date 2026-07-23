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

// TestRenderCircMessages_SystemNotice confirms a /help-style local reply
// renders without a username bracket or trailing timestamp column, and still
// respects the viewport width when wrapped.
func TestRenderCircMessages_SystemNotice(t *testing.T) {
	const width = 40
	sys := model.Message{
		Body:      "Commands: /me <action> · /poke /hug /hi5 /slap [@user] · /dice · /8ball · /fortune · /help",
		IsSystem:  true,
		CreatedAt: circMsgTime,
	}
	regular := circMsg("bob", "hi")

	out := renderCircMessages([]model.Message{sys, regular}, time.UTC, "datetime", width)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")

	if !strings.HasPrefix(lines[0], "***") {
		t.Errorf("expected the system notice to start with '***', got: %q", lines[0])
	}
	if strings.Contains(out, "<system>") || strings.Contains(out, "<>") {
		t.Errorf("expected no username bracket for a system notice, got: %q", out)
	}
	for i, l := range lines {
		if w := lipgloss.Width(l); w > width {
			t.Errorf("line %d width %d exceeds viewport width %d: %q", i, w, width, l)
		}
	}
	found := false
	for _, l := range lines {
		if strings.Contains(l, "bob") {
			found = true
		}
	}
	if !found {
		t.Error("expected the regular message after the system notice to still render")
	}
}

// TestRenderChatMessages_SystemNotice confirms a /help-style local reply in
// C-Mail renders as a plain notice, not a bidirectional bubble.
func TestRenderChatMessages_SystemNotice(t *testing.T) {
	const width = 60
	sys := model.Message{Body: "Commands: /me, /dice, /help", IsSystem: true, CreatedAt: circMsgTime}
	out := renderChatMessages([]model.Message{sys}, "neuromancer", time.UTC, "datetime", width)

	if !strings.Contains(out, "Commands: /me, /dice, /help") {
		t.Errorf("expected the system reply text in the output, got: %q", out)
	}
	for i, l := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if w := lipgloss.Width(l); w > width {
			t.Errorf("line %d width %d exceeds viewport width %d: %q", i, w, width, l)
		}
	}
}

// TestRenderCircMessages_ActionLine confirms an IsAction message (e.g. from
// /me) renders in classic IRC form "* username body *" with no username
// bracket, and that a regular message immediately after is unaffected.
func TestRenderCircMessages_ActionLine(t *testing.T) {
	const width = 60
	action := circMsg("ragnar", "tests the plumbing")
	action.IsAction = true
	regular := circMsg("bob", "hi there")

	out := renderCircMessages([]model.Message{action, regular}, time.UTC, "datetime", width)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")

	if !strings.HasPrefix(lines[0], "* ragnar tests the plumbing") {
		t.Errorf("expected classic IRC action format on the first line, got: %q", lines[0])
	}
	if !strings.Contains(lines[0], "*") || strings.Count(lines[0], "*") < 2 {
		t.Errorf("expected both a leading and trailing '*' on the action line, got: %q", lines[0])
	}
	if strings.Contains(lines[0], "<ragnar>") {
		t.Errorf("expected no username bracket on an action line, got: %q", lines[0])
	}
	ts := displayTime(circMsgTime, time.UTC, "datetime", true)
	if !strings.Contains(lines[0], ts) {
		t.Errorf("expected the timestamp to still trail the action line, got: %q", lines[0])
	}
	for i, l := range lines {
		if w := lipgloss.Width(l); w > width {
			t.Errorf("line %d width %d exceeds viewport width %d: %q", i, w, width, l)
		}
	}
	found := false
	for _, l := range lines[1:] {
		if strings.Contains(l, "<bob>") {
			found = true
		}
	}
	if !found {
		t.Error("expected the regular message after the action line to still render normally")
	}
}

// TestRenderCircMessages_ActionLineWrapsCorrectly guards against the action
// prefix/suffix width not being folded into the wrap budget, which would
// misalign continuation lines and the trailing timestamp for a long action.
func TestRenderCircMessages_ActionLineWrapsCorrectly(t *testing.T) {
	const width = 60
	action := circMsg("ragnar", strings.Repeat("does a very long dramatic action sequence ", 5))
	action.IsAction = true

	out := renderCircMessages([]model.Message{action}, time.UTC, "datetime", width)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected the long action to wrap onto multiple lines, got %d line(s)", len(lines))
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
	if !strings.Contains(last, "*") {
		t.Errorf("expected the trailing '*' on the last line, got: %q", last)
	}
}

// TestRenderChatMessages_ActionLine confirms an IsAction message in C-Mail
// renders as a plain "* username body *" line, not a bidirectional bubble.
func TestRenderChatMessages_ActionLine(t *testing.T) {
	const width = 60
	action := model.Message{From: model.User{Username: "ragnar"}, Body: "waves", IsAction: true, CreatedAt: circMsgTime}
	out := renderChatMessages([]model.Message{action}, "ragnar", time.UTC, "datetime", width)

	if !strings.Contains(out, "* ragnar waves") {
		t.Errorf("expected classic IRC action format in the output, got: %q", out)
	}
	if strings.Contains(out, "╭") || strings.Contains(out, "╰") {
		t.Errorf("expected no bubble border around an action message, got: %q", out)
	}
	for i, l := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if w := lipgloss.Width(l); w > width {
			t.Errorf("line %d width %d exceeds viewport width %d: %q", i, w, width, l)
		}
	}
}
