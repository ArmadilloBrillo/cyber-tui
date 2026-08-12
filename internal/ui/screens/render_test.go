package screens

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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
	out := renderCircMessages([]model.Message{circMsg("alice", body)}, time.UTC, "datetime", width, "", nil)

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
	out := renderCircMessages([]model.Message{circMsg("bob", "short reply")}, time.UTC, "datetime", width, "", nil)

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

	out := renderCircMessages([]model.Message{sys, regular}, time.UTC, "datetime", width, "", nil)
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

// TestRenderCircMessages_DeletedTombstone confirms a soft-deleted message
// renders as a muted "[DELETED]" tombstone, keeping the author and
// timestamp (per the API's "your name and the original timestamp stay"),
// but never the original body text.
func TestRenderCircMessages_DeletedTombstone(t *testing.T) {
	const width = 60
	deleted := model.Message{
		ID:        "m1",
		From:      model.User{Username: "molly"},
		Body:      "this secret sauce recipe leaked",
		Deleted:   true,
		CreatedAt: circMsgTime,
	}

	out := renderCircMessages([]model.Message{deleted}, time.UTC, "datetime", width, "", nil)

	if !strings.Contains(out, "<molly>") {
		t.Errorf("expected the author to still be shown, got: %q", out)
	}
	if !strings.Contains(out, "[DELETED]") {
		t.Errorf("expected a [DELETED] marker, got: %q", out)
	}
	if strings.Contains(out, "secret sauce") {
		t.Errorf("expected the original body to NOT appear, got: %q", out)
	}
}

// TestRenderCircMessages_DeletedTombstone_TimestampAlignsWithNormalMessage
// guards a real bug found in manual testing: the tombstone's timestamp
// landed two columns left of where every other message's timestamp sits,
// because its gap math used the "[DELETED]" marker's own short width
// instead of the same elastic body-field width renderCircMessages reserves
// for normal messages.
func TestRenderCircMessages_DeletedTombstone_TimestampAlignsWithNormalMessage(t *testing.T) {
	const width = 60
	normal := circMsg("molly", "hello there")
	deleted := model.Message{
		ID:        "m1",
		From:      model.User{Username: "molly"},
		Body:      "irrelevant once deleted",
		Deleted:   true,
		CreatedAt: circMsgTime,
	}

	normalLine := strings.TrimRight(ansi.Strip(renderCircMessages([]model.Message{normal}, time.UTC, "datetime", width, "", nil)), "\n")
	deletedLine := strings.TrimRight(ansi.Strip(renderCircMessages([]model.Message{deleted}, time.UTC, "datetime", width, "", nil)), "\n")

	ts := displayTime(circMsgTime, time.UTC, "datetime", true)
	normalIdx := strings.Index(normalLine, ts)
	deletedIdx := strings.Index(deletedLine, ts)
	if normalIdx == -1 || deletedIdx == -1 {
		t.Fatalf("timestamp %q not found: normal=%q deleted=%q", ts, normalLine, deletedLine)
	}
	if normalIdx != deletedIdx {
		t.Errorf("tombstone timestamp starts at column %d, want %d (same as a normal message)", deletedIdx, normalIdx)
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

	out := renderCircMessages([]model.Message{action, regular}, time.UTC, "datetime", width, "", nil)
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

	out := renderCircMessages([]model.Message{action}, time.UTC, "datetime", width, "", nil)
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
	out := renderCircMessages(msgs, time.UTC, "datetime", width, "", nil)

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
	out := renderCircMessages(msgs, time.UTC, "datetime", width, "", nil)

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
	out := renderCircMessages(msgs, time.UTC, "datetime", width, "", nil)

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

	out := renderCircMessages([]model.Message{action}, time.UTC, "datetime", width, "", nil)
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

	out := renderCircMessages([]model.Message{mine, other}, time.UTC, "datetime", width, "ragnar", nil)

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

	out := renderCircMessages([]model.Message{msg}, time.UTC, "datetime", width, "ragnar", nil)

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

	out := renderCircMessages([]model.Message{msg}, time.UTC, "datetime", width, "ragnar", nil)

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

// TestRenderAttachments_GifBadge confirms a "gif" attachment gets its own
// [gif] badge rather than falling into the generic [attachment] default.
func TestRenderAttachments_GifBadge(t *testing.T) {
	out := renderAttachments([]model.Attachment{{Type: "gif", Src: "https://cyberspace.online/a.gif"}})
	if !strings.Contains(out, "[gif]") {
		t.Errorf("expected [gif] badge, got: %q", out)
	}
	if !strings.Contains(out, "https://cyberspace.online/a.gif") {
		t.Errorf("expected gif URL in output, got: %q", out)
	}
}

// TestAttachmentIndicator_CountsImages confirms the badge is blank with no
// images, flat "[img]" for exactly one, and "[img +N]" (extra beyond the one
// inline rendering shows) for multiple — audio attachments never count, and
// markdown-embedded images (the actual mechanism real posts use — see the
// "markdown only" cases) count the same as structured Attachments.
func TestAttachmentIndicator_CountsImages(t *testing.T) {
	cases := []struct {
		name        string
		attachments []model.Attachment
		content     string
		want        string
	}{
		{"none", nil, "just text", ""},
		{"audio only", []model.Attachment{{Type: "audio"}}, "", ""},
		{"one image", []model.Attachment{{Type: "image"}}, "", "[img]"},
		{"one gif", []model.Attachment{{Type: "gif"}}, "", "[img]"},
		{"image plus gif", []model.Attachment{{Type: "image"}, {Type: "gif"}}, "", "[img +1]"},
		{"three images", []model.Attachment{{Type: "image"}, {Type: "audio"}, {Type: "image"}, {Type: "image"}}, "", "[img +2]"},
		{"markdown only, one image", nil, "hi\n\n![a](https://x/a.png)\n\n", "[img]"},
		{"markdown only, two images", nil, "![a](https://x/a.png)\n\n![b](https://x/b.png)\n\n", "[img +1]"},
		{"attachment plus markdown image", []model.Attachment{{Type: "audio"}}, "![a](https://x/a.png)\n\n", "[img]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := attachmentIndicator(c.attachments, c.content)
			if c.want == "" {
				if out != "" {
					t.Errorf("expected no badge, got: %q", out)
				}
				return
			}
			if !strings.Contains(out, c.want) {
				t.Errorf("expected badge to contain %q, got: %q", c.want, out)
			}
		})
	}
}

// TestRenderCircMessages_AttachmentOnlyBodySkipsDuplicateURL confirms that
// when Body merely duplicates ImageUrl (an attachment-only message), the URL
// is shown once via the attachment badge, not a second time as wrapped body text.
func TestRenderCircMessages_AttachmentOnlyBodySkipsDuplicateURL(t *testing.T) {
	msg := circMsg("case", "https://cyberspace.online/img.png")
	msg.ImageUrl = "https://cyberspace.online/img.png"

	out := renderCircMessages([]model.Message{msg}, time.UTC, "datetime", 60, "", nil)

	if n := strings.Count(out, "https://cyberspace.online/img.png"); n != 1 {
		t.Errorf("expected the image URL to appear exactly once, got %d times in: %q", n, out)
	}
	if !strings.Contains(out, "[image]") {
		t.Errorf("expected [image] attachment badge, got: %q", out)
	}
}

// TestRenderCircMessages_AttachmentOnlyHasNoBlankLine guards against a
// regression where an attachment-only message (empty display body) rendered
// a blank-looking line — just the username prefix and timestamp, no visible
// content — before the attachment badge. The username/timestamp should land
// directly on the attachment line instead.
func TestRenderCircMessages_AttachmentOnlyHasNoBlankLine(t *testing.T) {
	msg := circMsg("case", "https://cyberspace.online/img.png")
	msg.Body = ""
	msg.ImageUrl = "https://cyberspace.online/img.png"

	out := renderCircMessages([]model.Message{msg}, time.UTC, "datetime", 60, "", nil)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")

	if len(lines) != 1 {
		t.Fatalf("expected exactly 1 line for a single attachment-only message, got %d: %q", len(lines), out)
	}
	if !strings.Contains(lines[0], "<case>") || !strings.Contains(lines[0], "[image]") {
		t.Errorf("expected username and [image] badge on the same line, got: %q", lines[0])
	}
}

// TestRenderCircMessages_AttachmentOnlyLongURL_TimestampStaysInBounds guards
// against a regression where an attachment-only message's URL, merged onto
// the username/timestamp line without going through the same
// Width(bodyWidth).Render wrapping normal text bodies use, could run well
// past viewportWidth for a long URL — pushing the timestamp far to the
// right instead of wrapping the URL underneath it.
func TestRenderCircMessages_AttachmentOnlyLongURL_TimestampStaysInBounds(t *testing.T) {
	const width = 60
	msg := circMsg("case", "")
	msg.GifUrl = "https://cyberspace.online/uploads/reactions/mind-blown-reaction.gif"

	out := renderCircMessages([]model.Message{msg}, time.UTC, "datetime", width, "", nil)
	plain := ansi.Strip(out)

	ts := displayTime(circMsgTime, time.UTC, "datetime", true)
	for _, line := range strings.Split(strings.TrimRight(plain, "\n"), "\n") {
		if lipgloss.Width(line) > width {
			t.Errorf("line exceeds viewportWidth %d: %q (%d cols)", width, line, lipgloss.Width(line))
		}
	}
	if !strings.Contains(plain, ts) {
		t.Fatalf("timestamp %q not found anywhere in output: %q", ts, plain)
	}
	lastLine := strings.Split(strings.TrimRight(plain, "\n"), "\n")
	last := lastLine[len(lastLine)-1]
	if !strings.HasSuffix(strings.TrimRight(last, " "), ts) {
		t.Errorf("expected the timestamp flush right on the last line, got: %q", last)
	}
}

// TestRenderChatMessages_AttachmentOnlyHasNoBlankLine is the C-Mail
// equivalent of TestRenderCircMessages_AttachmentOnlyHasNoBlankLine: the
// bordered bubble must not contain a blank row between the header and the
// attachment block for an attachment-only message.
func TestRenderChatMessages_AttachmentOnlyHasNoBlankLine(t *testing.T) {
	msg := circMsg("case", "")
	msg.ImageUrl = "https://cyberspace.online/img.png"

	out := renderChatMessages([]model.Message{msg}, "", time.UTC, "datetime", 60)
	plain := ansi.Strip(out)

	// A blank content row inside the bubble would sit between the header
	// line (contains "case") and the attachment line (contains "[image]");
	// check there's no all-whitespace line between them.
	lines := strings.Split(plain, "\n")
	headerIdx, attIdx := -1, -1
	for i, line := range lines {
		if strings.Contains(line, "case") && headerIdx == -1 {
			headerIdx = i
		}
		if strings.Contains(line, "[image]") {
			attIdx = i
		}
	}
	if headerIdx == -1 || attIdx == -1 {
		t.Fatalf("expected both a header line and an [image] line, got: %q", plain)
	}
	if attIdx != headerIdx+1 {
		t.Errorf("expected the attachment line immediately after the header with no blank line between, got lines:\n%q", lines[headerIdx:attIdx+1])
	}
}

// TestRenderCircMessages_ArtStyleSkipsWrapAndMarkdown confirms a style:"art"
// message's body is printed verbatim, line for line, with no word-wrap and
// no markdown reinterpretation of leading spaces (which would otherwise be
// read as an indented code block and mangle the picture).
func TestRenderCircMessages_ArtStyleSkipsWrapAndMarkdown(t *testing.T) {
	art := "  /\\_/\\\n ( o.o )\n  > ^ <"
	msg := circMsg("case", art)
	msg.Style = []string{"art"}

	out := renderCircMessages([]model.Message{msg}, time.UTC, "datetime", 20, "", nil)

	for _, line := range strings.Split(art, "\n") {
		if !strings.Contains(out, line) {
			t.Errorf("expected art line %q preserved verbatim, got: %q", line, out)
		}
	}
}

// TestRenderCircMessages_ArtStyle_TimestampAlignsWithNormalMessage guards
// against a regression where renderArtMessage's header-line width
// calculation came up 2 columns short of viewportWidth, shifting the
// timestamp left of where every other message type right-aligns it.
func TestRenderCircMessages_ArtStyle_TimestampAlignsWithNormalMessage(t *testing.T) {
	const width = 60
	normal := circMsg("molly", "hello there")
	art := circMsg("molly", "  /\\_/\\\n ( o.o )")
	art.Style = []string{"art"}

	normalLine := strings.Split(strings.TrimRight(ansi.Strip(renderCircMessages([]model.Message{normal}, time.UTC, "datetime", width, "", nil)), "\n"), "\n")[0]
	artLines := strings.Split(strings.TrimRight(ansi.Strip(renderCircMessages([]model.Message{art}, time.UTC, "datetime", width, "", nil)), "\n"), "\n")
	artHeader := artLines[0]

	ts := displayTime(circMsgTime, time.UTC, "datetime", true)
	normalIdx := strings.Index(normalLine, ts)
	artIdx := strings.Index(artHeader, ts)
	if normalIdx == -1 || artIdx == -1 {
		t.Fatalf("timestamp %q not found: normal=%q art=%q", ts, normalLine, artHeader)
	}
	if normalIdx != artIdx {
		t.Errorf("art header timestamp starts at column %d, want %d (same as a normal message)", artIdx, normalIdx)
	}
}

// TestRenderCircMessagesStyled_Blink_HidesAndShowsWithoutChangingLineCount
// confirms blink toggles the body's visibility across animation frames while
// keeping the same line count and per-line width — blanking must happen
// after word-wrap, not before, or an all-space string could rewrap to fewer
// lines than the real text did (see the comment at the blink toggle site).
func TestRenderCircMessagesStyled_Blink_HidesAndShowsWithoutChangingLineCount(t *testing.T) {
	const width = 60
	msg := circMsg("molly", "hello there")
	msg.Style = []string{"blink"}

	visible := renderCircMessagesStyled([]model.Message{msg}, time.UTC, "datetime", width, "", nil, 0, nil)
	hidden := renderCircMessagesStyled([]model.Message{msg}, time.UTC, "datetime", width, "", nil, blinkPhaseFrames, nil)

	if !strings.Contains(ansi.Strip(visible), "hello there") {
		t.Errorf("expected the visible phase to contain the body text, got: %q", visible)
	}
	plainHidden := ansi.Strip(hidden)
	if strings.Contains(plainHidden, "hello") || strings.Contains(plainHidden, "there") {
		t.Errorf("expected the hidden phase to blank the body text, got: %q", hidden)
	}

	visibleLines := strings.Split(strings.TrimRight(ansi.Strip(visible), "\n"), "\n")
	hiddenLines := strings.Split(strings.TrimRight(plainHidden, "\n"), "\n")
	if len(visibleLines) != len(hiddenLines) {
		t.Fatalf("line count changed across blink phases: visible=%d hidden=%d", len(visibleLines), len(hiddenLines))
	}
	for i := range visibleLines {
		if lipgloss.Width(visibleLines[i]) != lipgloss.Width(hiddenLines[i]) {
			t.Errorf("line %d width changed across blink phases: visible=%d hidden=%d", i, lipgloss.Width(visibleLines[i]), lipgloss.Width(hiddenLines[i]))
		}
	}

	ts := displayTime(circMsgTime, time.UTC, "datetime", true)
	if !strings.Contains(plainHidden, ts) {
		t.Errorf("expected the timestamp to stay visible during the hidden phase, got: %q", hidden)
	}
}

// TestRenderCircMessages_MutedSenderHidden confirms a muted sender's messages
// are dropped entirely from output, while other senders' messages still render.
func TestRenderCircMessages_MutedSenderHidden(t *testing.T) {
	const width = 60
	msgs := []model.Message{circMsg("alice", "hi from alice"), circMsg("bob", "hi from bob")}
	muted := map[string]bool{"alice": true}

	out := renderCircMessages(msgs, time.UTC, "datetime", width, "", muted)

	if strings.Contains(out, "hi from alice") {
		t.Errorf("expected alice's message to be hidden, got: %q", out)
	}
	if !strings.Contains(out, "hi from bob") {
		t.Errorf("expected bob's message to still render, got: %q", out)
	}
}

// TestRenderCircMessagesWithSelection_MutedSenderKeepsOffsetsAligned confirms
// a muted sender's message contributes zero-height output but the
// offsets/heights slices stay 1:1 with msgs, matching the invariant relied on
// by selection/scrolling code.
func TestRenderCircMessagesWithSelection_MutedSenderKeepsOffsetsAligned(t *testing.T) {
	const width = 60
	muted := circMsg("alice", "hi from alice")
	muted.ID = "m1"
	visible := circMsg("bob", "hi from bob")
	visible.ID = "m2"
	msgs := []model.Message{muted, visible}

	content, offsets, heights := renderCircMessagesWithSelection(msgs, time.UTC, "datetime", width, "", "", nil, 0,
		map[string]bool{"alice": true})

	if len(offsets) != len(msgs) || len(heights) != len(msgs) {
		t.Fatalf("offsets/heights not 1:1 with msgs: len(offsets)=%d len(heights)=%d want %d", len(offsets), len(heights), len(msgs))
	}
	if heights[0] != 0 {
		t.Errorf("expected muted message's height to be 0, got %d", heights[0])
	}
	if strings.Contains(content, "hi from alice") {
		t.Errorf("expected alice's message to be hidden, got: %q", content)
	}
	if !strings.Contains(content, "hi from bob") {
		t.Errorf("expected bob's message to still render, got: %q", content)
	}
}

// TestRenderChatMessagesWithSelection_HighlightsSelectedAndMatchesUnselected
// mirrors TestRenderCircMessagesWithSelection_MutedSenderKeepsOffsetsAligned
// for CMail's renderChatMessagesWithSelection: offsets/heights stay 1:1 with
// msgs, rendering with selectedID == "" is byte-identical to
// renderChatMessagesStyled, and selecting a message changes its rendered
// block (the SelectedRow highlight).
func TestRenderChatMessagesWithSelection_HighlightsSelectedAndMatchesUnselected(t *testing.T) {
	const width = 60
	msgs := []model.Message{
		{ID: "m1", From: model.User{Username: "molly"}, Body: "first", CreatedAt: circMsgTime},
		{ID: "m2", From: model.User{Username: "case"}, Body: "second", CreatedAt: circMsgTime},
	}

	unselected := renderChatMessagesStyled(msgs, "case", time.UTC, "datetime", width, 0)
	content, offsets, heights := renderChatMessagesWithSelection(msgs, "case", time.UTC, "datetime", width, 0, "")
	if content != unselected {
		t.Errorf("renderChatMessagesWithSelection with selectedID==\"\" should match renderChatMessagesStyled;\ngot:  %q\nwant: %q", content, unselected)
	}
	if len(offsets) != len(msgs) || len(heights) != len(msgs) {
		t.Fatalf("offsets/heights not 1:1 with msgs: len(offsets)=%d len(heights)=%d want %d", len(offsets), len(heights), len(msgs))
	}

	selected, _, _ := renderChatMessagesWithSelection(msgs, "case", time.UTC, "datetime", width, 0, "m1")
	if selected == unselected {
		t.Error("expected selecting m1 to change the rendered content (highlight)")
	}
}

// TestFeedRenderPost_CachesBodyAcrossSelectionChange guards the fix that
// splits renderPostBody (cacheable) out from the selection border: moving
// the cursor between posts must reuse the cached body and only change
// styling, and an edited post must invalidate the cache instead of serving
// stale text.
func TestFeedRenderPost_CachesBodyAcrossSelectionChange(t *testing.T) {
	withTrueColor(t) // Border vs ActiveBorder differ only in color; see withTrueColor's doc comment
	m := NewFeedModel()
	m.width = 80
	post := model.Post{ID: "p1", AuthorUsername: "alice", Content: "hello world"}

	unselected, _ := m.renderPost(post, false)
	selected, _ := m.renderPost(post, true)

	if unselected == selected {
		t.Error("expected selected vs unselected rendering to differ (border style)")
	}
	if _, hit := m.bodyCache["p1"]; !hit {
		t.Fatal("expected renderPost to populate the body cache")
	}

	post.Content = "edited content"
	edited, _ := m.renderPost(post, false)
	if strings.Contains(ansi.Strip(edited), "hello world") {
		t.Error("expected cache to invalidate after post content changed, got stale body")
	}
	if !strings.Contains(ansi.Strip(edited), "edited content") {
		t.Errorf("expected edited content in output, got: %q", edited)
	}
}

// BenchmarkRenderCircMessagesWithSelection measures how render cost scales
// with the number of loaded messages — chatrooms.go never caps m.messages,
// and this render runs in full on every 150ms style-animation tick
// (maybeStartStyleAnim, chatrooms.go:940), so cost-per-tick grows with
// however much history a long-lived room has accumulated.
func BenchmarkRenderCircMessagesWithSelection(b *testing.B) {
	for _, n := range []int{100, 1000, 5000, 10000} {
		msgs := make([]model.Message, n)
		for i := range msgs {
			msgs[i] = circMsg("bob", "hey alice, message number "+strconv.Itoa(i)+" in the room")
		}
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				renderCircMessagesWithSelection(msgs, time.UTC, "datetime", 80, "alice", "", nil, 0, nil)
			}
		})
	}
}

// BenchmarkFeedBuildContent_SelectionChange measures the cost of the rebuild
// that runs on every up/down key press (refreshContent -> buildContent,
// feed.go:552-559/626-633) once the body cache is already warm — i.e. only
// the selected index changes between calls, the realistic case for arrow-key
// navigation. Before the body cache, this cost was O(n) in loaded posts
// (every post's markdown re-parsed on every keystroke); with it, only the
// selection styling should redo work.
func BenchmarkFeedBuildContent_SelectionChange(b *testing.B) {
	for _, n := range []int{100, 1000, 5000} {
		m := NewFeedModel()
		m.width = 80
		posts := make([]model.Post, n)
		for i := range posts {
			posts[i] = model.Post{ID: strconv.Itoa(i), AuthorUsername: "bob", Content: "post body number " + strconv.Itoa(i)}
		}
		m.posts = posts
		m.buildContent() // warm the cache once, like the initial refreshContent after load

		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				m.selectedIndex = i % n
				m.buildContent()
			}
		})
	}
}
