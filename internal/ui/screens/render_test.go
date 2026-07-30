package screens

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
)

// withTrueColor forces 24-bit color for the duration of a test, so
// style-dependent assertions (MeHighlight vs Highlight) see distinct ANSI
// codes instead of identical plain text — the rest of this file's tests rely
// on the default no-color profile, so this is scoped per-test, not global.
func withTrueColor(t *testing.T) {
	t.Helper()
	theme.Set("cyber")
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })
}

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
	out := renderCircMessages([]model.Message{circMsg("alice", body)}, time.UTC, "datetime", width, "")

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
	out := renderCircMessages([]model.Message{circMsg("bob", "short reply")}, time.UTC, "datetime", width, "")

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

	out := renderCircMessages([]model.Message{sys, regular}, time.UTC, "datetime", width, "")
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

	out := renderCircMessages([]model.Message{action, regular}, time.UTC, "datetime", width, "")
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")

	if !strings.HasPrefix(lines[0], "* ragnar tests the plumbing") {
		t.Errorf("expected classic IRC action format on the first line, got: %q", lines[0])
	}
	if !strings.Contains(lines[0], "*") || strings.Count(lines[0], "*") < 2 {
		t.Errorf("expected both a leading and trailing '*' on the action line, got: %q", lines[0])
	}
	if !strings.Contains(lines[0], "plumbing *") {
		t.Errorf("expected the closing '*' to sit right after the action text with a single space, not right-aligned, got: %q", lines[0])
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

	out := renderCircMessages([]model.Message{action}, time.UTC, "datetime", width, "")
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
	if strings.Contains(last, "  *") {
		t.Errorf("expected the closing '*' to hug the last word with a single space, not be right-aligned/padded, got: %q", last)
	}
}

// TestRenderCircMessages_MarkdownEmphasisBoldCode confirms CIRC messages now
// get inline markdown styling (emphasis, bold, code) — the feature this test
// file's earlier tests predate.
func TestRenderCircMessages_MarkdownEmphasisBoldCode(t *testing.T) {
	const width = 60
	msgs := []model.Message{
		circMsg("alice", "this is *emphasis* text"),
		circMsg("bob", "this is **bold** text"),
		circMsg("carol", "run `go test` please"),
	}
	out := renderCircMessages(msgs, time.UTC, "datetime", width, "")

	if !strings.Contains(out, "emphasis") || strings.Contains(out, "*emphasis*") {
		t.Errorf("expected *emphasis* to be styled, not left as raw markdown: %q", out)
	}
	if !strings.Contains(out, "bold") || strings.Contains(out, "**bold**") {
		t.Errorf("expected **bold** to be styled, not left as raw markdown: %q", out)
	}
	if !strings.Contains(out, "go test") || strings.Contains(out, "`go test`") {
		t.Errorf("expected `go test` to be styled, not left as raw markdown: %q", out)
	}
}

// TestRenderCircMessages_MarkdownLinkAndBareURL confirms markdown links and
// bare URLs both render without their raw markdown syntax.
func TestRenderCircMessages_MarkdownLinkAndBareURL(t *testing.T) {
	const width = 60
	msgs := []model.Message{
		circMsg("alice", "see [the docs](https://example.com/docs)"),
		circMsg("bob", "check https://example.com/path for details"),
	}
	out := renderCircMessages(msgs, time.UTC, "datetime", width, "")

	if !strings.Contains(out, "the docs") || strings.Contains(out, "[the docs](") {
		t.Errorf("expected markdown link syntax to be rendered, not left raw: %q", out)
	}
	if !strings.Contains(out, "https://example.com/path") {
		t.Errorf("expected bare URL to remain visible: %q", out)
	}
}

// TestRenderCircMessages_LeadingBlockCharsStayLiteral guards against CIRC's
// inline-only markdown misinterpreting a plain chat line that happens to
// start with a markdown block character (heading/list/blockquote) as actual
// block syntax — a real risk in freeform one-line chat that full markdown.Render
// would have introduced.
func TestRenderCircMessages_LeadingBlockCharsStayLiteral(t *testing.T) {
	const width = 60
	msgs := []model.Message{
		circMsg("alice", "- get milk"),
		circMsg("bob", "# thoughts for today"),
		circMsg("carol", "> what did you say"),
	}
	out := renderCircMessages(msgs, time.UTC, "datetime", width, "")

	for _, want := range []string{"- get milk", "# thoughts for today", "> what did you say"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected literal chat line %q to survive unmangled, got: %q", want, out)
		}
	}
}

// TestRenderCircMessages_ActionLineWithAsteriskBody guards the specific
// concern raised about /me action lines: renderActionLine wraps the whole
// line in a literal "* username body *" (added after the body is rendered,
// so it's never re-parsed as markdown) — a body that itself contains
// emphasis asterisks must not confuse that outer wrapping.
func TestRenderCircMessages_ActionLineWithAsteriskBody(t *testing.T) {
	const width = 60
	action := circMsg("ragnar", "throws a *loud* party")
	action.IsAction = true

	out := renderCircMessages([]model.Message{action}, time.UTC, "datetime", width, "")
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")

	if !strings.HasPrefix(lines[0], "* ragnar throws a ") {
		t.Errorf("expected the action line to still start with '* ragnar throws a ', got: %q", lines[0])
	}
	if !strings.Contains(lines[0], "loud") || strings.Contains(lines[0], "*loud*") {
		t.Errorf("expected the body's *loud* to be styled as emphasis, not left raw: %q", lines[0])
	}
	if !strings.Contains(lines[0], "party *") {
		t.Errorf("expected the outer action wrapper's closing ' *' to still hug the last word: %q", lines[0])
	}
	if strings.Count(lines[0], " *") < 1 {
		t.Errorf("expected the trailing action-wrapper asterisk to survive alongside the emphasis styling: %q", lines[0])
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
	if !strings.Contains(out, "waves *") {
		t.Errorf("expected the closing '*' right after the action text with a single space, got: %q", out)
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

// TestRenderCircMessages_OwnUsernameUsesMeHighlight confirms the current
// user's own username renders with theme.MeHighlight rather than the
// default theme.Highlight used for other users' names.
func TestRenderCircMessages_OwnUsernameUsesMeHighlight(t *testing.T) {
	withTrueColor(t)
	const width = 60
	mine := circMsg("ragnar", "hi all")
	other := circMsg("bob", "hello")

	out := renderCircMessages([]model.Message{mine, other}, time.UTC, "datetime", width, "ragnar")

	if !strings.Contains(out, "<"+theme.MeHighlight.Render("ragnar")+">") {
		t.Errorf("expected own username styled with MeHighlight, got: %q", out)
	}
	if strings.Contains(out, "<"+theme.Highlight.Render("ragnar")+">") {
		t.Errorf("did not expect own username styled with plain Highlight, got: %q", out)
	}
	if !strings.Contains(out, "<"+theme.Highlight.Render("bob")+">") {
		t.Errorf("expected other user's username to keep plain Highlight styling, got: %q", out)
	}
}

// TestRenderCircMessages_MentionHighlighted confirms an @mention (or bare
// mention) of the current user's name inside someone else's message body
// gets wrapped in theme.MeHighlight, case-insensitively, without touching
// substrings that merely contain the name.
func TestRenderCircMessages_MentionHighlighted(t *testing.T) {
	withTrueColor(t)
	const width = 60
	msg := circMsg("bob", "hey @Ragnar and ragnarwessels, is ragnar around?")

	out := renderCircMessages([]model.Message{msg}, time.UTC, "datetime", width, "ragnar")

	if !strings.Contains(out, theme.MeHighlight.Render("@Ragnar")) {
		t.Errorf("expected case-insensitive @mention to be highlighted, got: %q", out)
	}
	if !strings.Contains(out, theme.MeHighlight.Render("ragnar")) {
		t.Errorf("expected bare-word mention to be highlighted, got: %q", out)
	}
	if strings.Contains(out, theme.MeHighlight.Render("ragnarwessels")) {
		t.Errorf("did not expect a substring match (ragnarwessels) to be highlighted, got: %q", out)
	}
}

// TestRenderCircMessages_MentionStyleContinuesAfterMultipleWords is the
// integration-level regression test for the reported bug: "@ragnar 1"
// rendered "1" correctly in theme.Base's color, but "@ragnar 1 2" rendered
// "1" unstyled and only "2" back in theme.Base — own-username highlighting
// used to be a post-render string-splice into already-ANSI-rendered text,
// and the spliced-in span's own SGR reset silently killed the surrounding
// style for everything after the match. Own-mention highlighting is now done
// in the same rendering pass as the rest of the styling (markdown.RenderInline's
// highlightUser), so there's no already-styled text to splice into.
func TestRenderCircMessages_MentionStyleContinuesAfterMultipleWords(t *testing.T) {
	withTrueColor(t)
	const width = 60
	msg := circMsg("bob", "@ragnar 1 2")

	out := renderCircMessages([]model.Message{msg}, time.UTC, "datetime", width, "ragnar")

	if !strings.Contains(out, theme.MeHighlight.Render("@ragnar")) {
		t.Errorf("expected @ragnar to be highlighted, got: %q", out)
	}
	if !strings.Contains(out, theme.Base.Render(" 1")) {
		t.Errorf("expected '1' to keep theme.Base styling right after the mention, got: %q", out)
	}
	if !strings.Contains(out, theme.Base.Render(" 2")) {
		t.Errorf("expected '2' to also keep theme.Base styling, not left unstyled by a broken reset, got: %q", out)
	}
}
