package screens

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/ragnar/cyber-tui/internal/api"
	"github.com/ragnar/cyber-tui/internal/model"
)

func TestCMailTotalUnread(t *testing.T) {
	m := NewCMailModel("me", "", nil)

	if got := m.TotalUnread(); got != 0 {
		t.Fatalf("TotalUnread() on empty model = %d, want 0", got)
	}

	m = m.SetConversations([]model.Conversation{
		{ID: "1", UnreadCount: 3},
		{ID: "2", UnreadCount: 0},
		{ID: "3", UnreadCount: 2},
	})

	if got := m.TotalUnread(); got != 5 {
		t.Fatalf("TotalUnread() = %d, want 5", got)
	}
}

// TestOtherParticipant_EmptyUsernameFallsBackToUnknown guards against a stale
// RTDB conversation entry with a blank otherUsername rendering as "@" instead
// of the "unknown" fallback.
func TestOtherParticipant_EmptyUsernameFallsBackToUnknown(t *testing.T) {
	m := NewCMailModel("me", "", nil)
	conv := model.Conversation{ID: "c1", Participants: []model.User{{Username: ""}}}

	if got := m.otherParticipant(conv); got != "unknown" {
		t.Fatalf("otherParticipant() with blank username = %q, want %q", got, "unknown")
	}
}

// TestRenderConvCards_EpochZeroLastMessageAtHidesDate guards against a stale
// conversation with a missing lastMessageAt (converted to epoch 1970-01-01,
// not Go's zero time.Time) rendering a bogus "01-Jan-1970" date.
func TestRenderConvCards_EpochZeroLastMessageAtHidesDate(t *testing.T) {
	m := NewCMailModel("me", "", nil)
	m.width = 80
	m = m.SetConversations([]model.Conversation{
		{ID: "c1", Participants: []model.User{{Username: ""}}, LastMessageAt: time.UnixMilli(0)},
	})

	out := m.renderConvCards()
	if strings.Contains(out, "1970") {
		t.Fatalf("renderConvCards() rendered epoch-0 date, want it hidden:\n%s", out)
	}
	if !strings.Contains(out, "@unknown") {
		t.Fatalf("renderConvCards() = %q, want it to contain %q", out, "@unknown")
	}
}

// TestCMailDetailView_HeaderHasDividerBeforeMessages guards against the
// divider row (mirrors the same fix in chatrooms.go — the header shouldn't
// float with no visual bottom edge, unlike the bordered input box below it)
// being dropped in a future edit.
func TestCMailDetailView_HeaderHasDividerBeforeMessages(t *testing.T) {
	conv := model.Conversation{ID: "c1", Participants: []model.User{{Username: "neo"}, {Username: "trinity"}}}
	m := NewCMailModel("neo", "", api.NewMockClient())
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.activeConvID = "c1"
	m.activeConv = &conv
	m.mode = cmailModeDetail
	m = m.SetConversationMessages("c1", []model.Message{{From: model.User{Username: "trinity"}, Body: "hi"}})

	lines := strings.Split(m.View(), "\n")
	if len(lines) < 3 {
		t.Fatalf("expected at least a header, divider, and message line, got %d lines: %q", len(lines), lines)
	}
	if !strings.Contains(lines[0], "trinity") {
		t.Fatalf("expected line 0 to be the conversation header, got: %q", lines[0])
	}
	if !strings.Contains(lines[1], "─") {
		t.Errorf("expected line 1 to be the divider rule, got: %q", lines[1])
	}
	if strings.Contains(lines[0], "─") {
		t.Errorf("did not expect the divider character on the header line itself, got: %q", lines[0])
	}
}

// TestCMailInputBox_WidthConstantBetweenEmptyAndTyped mirrors the same fix
// (and same bug) in chatrooms.go: textinput.View()'s empty placeholder
// render and its typed-content render total different widths — typed
// content adds Prompt's width plus one more for the phantom end-of-line
// cursor glyph, neither subtracted from its own padding math, silently
// growing the box 3 columns wider than the header above it the instant any
// character is typed. Reuses inputBoxLines from chatrooms_test.go (same
// package).
func TestCMailInputBox_WidthConstantBetweenEmptyAndTyped(t *testing.T) {
	conv := model.Conversation{ID: "c1", Participants: []model.User{{Username: "neo"}, {Username: "trinity"}}}
	m := NewCMailModel("neo", "", api.NewMockClient())
	m, _ = m.Update(tea.WindowSizeMsg{Width: 160, Height: 24})
	m.activeConvID = "c1"
	m.activeConv = &conv
	m.mode = cmailModeDetail
	m.input.Focus()

	top, content, bottom := inputBoxLines(t, m.View())
	for name, l := range map[string]string{"top (empty)": top, "content (empty)": content, "bottom (empty)": bottom} {
		if w := lipgloss.Width(l); w != m.width {
			t.Errorf("%s line width = %d, want %d (m.width)", name, w, m.width)
		}
	}

	m.input.SetValue("hello world")
	m.input.SetCursor(11)
	top, content, bottom = inputBoxLines(t, m.View())
	for name, l := range map[string]string{"top (typed)": top, "content (typed)": content, "bottom (typed)": bottom} {
		if w := lipgloss.Width(l); w != m.width {
			t.Errorf("%s line width = %d, want %d (m.width) — box must not widen once typing starts", name, w, m.width)
		}
	}
}

// --- typing indicator ---

func cmailInConversation(client api.Client, convID string) CMailModel {
	conv := model.Conversation{ID: convID, Participants: []model.User{{Username: "neo"}, {Username: "trinity"}}}
	m := NewCMailModel("neo", "", client)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 160, Height: 24})
	m.activeConvID = convID
	m.activeConv = &conv
	m.mode = cmailModeDetail
	m.input.Focus() // real usage always focuses on conversation open; without this,
	// textinput.Update() no-ops (it early-returns when unfocused), silently
	// breaking any test that types through the normal forwarding path.
	return m
}

// --- slash commands ---

func TestCMail_Send_UnknownCommandIsRejected(t *testing.T) {
	m := cmailInConversation(api.NewMockClient(), "c1")
	m.input.SetValue("/bogus")

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("expected no command for an unknown slash command")
	}
	if view := m.View(); !strings.Contains(view, "unknown command: /bogus") {
		t.Errorf("expected an unknown-command notice in the view, got: %q", view)
	}
}

func TestCMail_Send_KnownCommandStillSends(t *testing.T) {
	cases := []string{
		"/me waves",
		"/comic+rainbow hi",
		"/spoiler secret",
		"/gif https://example.com/a.gif",
		"/song https://youtu.be/x | artist | title",
	}
	for _, body := range cases {
		t.Run(body, func(t *testing.T) {
			m := cmailInConversation(api.NewMockClient(), "c1")
			m.input.SetValue(body)

			m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			if cmd == nil {
				t.Fatalf("expected a send command for known slash command %q", body)
			}
			msg, ok := cmd().(SendCMailMsg)
			if !ok {
				t.Fatalf("expected SendCMailMsg, got %T", cmd())
			}
			if msg.Body != body {
				t.Errorf("Body = %q, want %q", msg.Body, body)
			}
		})
	}
}

// TestCMail_Send_CircOnlyCommandsRejected guards against C-Mail accepting
// /mute-family or /art commands — the server only supports those in CIRC
// (docs/00-latest-api-reference.md).
func TestCMail_Send_CircOnlyCommandsRejected(t *testing.T) {
	for _, body := range []string{"/mute someone", "/art"} {
		t.Run(body, func(t *testing.T) {
			m := cmailInConversation(api.NewMockClient(), "c1")
			m.input.SetValue(body)

			m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			if cmd != nil {
				t.Fatalf("expected no command for CIRC-only command %q in C-Mail", body)
			}
			cmdWord := strings.Fields(body)[0]
			if view := m.View(); !strings.Contains(view, "unknown command: "+cmdWord) {
				t.Errorf("expected an unknown-command notice in the view, got: %q", view)
			}
		})
	}
}

// TestCMail_Send_SpoilerCannotChain mirrors the same CIRC fix — /spoiler is
// a known style but the server rejects it chained with any other style.
func TestCMail_Send_SpoilerCannotChain(t *testing.T) {
	m := cmailInConversation(api.NewMockClient(), "c1")
	m.input.SetValue("/spoiler+rainbow hi")

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("expected no command for /spoiler+rainbow (spoiler cannot be chained)")
	}
	if view := m.View(); !strings.Contains(view, "unknown command: /spoiler+rainbow") {
		t.Errorf("expected an unknown-command notice in the view, got: %q", view)
	}
}

// --- background-tab persistence ---

func TestDMReceived_BumpsUnreadWhileUnfocused(t *testing.T) {
	m := cmailInConversation(api.NewMockClient(), "c1")
	m = m.SetConversations([]model.Conversation{{ID: "c1"}})
	m = m.SetFocused(false)

	m, _ = m.Update(dmReceivedMsg{msg: model.Message{Body: "hey"}})
	m, _ = m.Update(dmReceivedMsg{msg: model.Message{Body: "hey again"}})

	if got := m.TotalUnread(); got != 2 {
		t.Errorf("TotalUnread() = %d, want 2", got)
	}
}

func TestDMReceived_DoesNotBumpUnreadWhileFocused(t *testing.T) {
	m := cmailInConversation(api.NewMockClient(), "c1")
	m = m.SetConversations([]model.Conversation{{ID: "c1"}})
	m = m.SetFocused(true)

	m, _ = m.Update(dmReceivedMsg{msg: model.Message{Body: "hey"}})

	if got := m.TotalUnread(); got != 0 {
		t.Errorf("TotalUnread() = %d, want 0 (actively viewing the conversation)", got)
	}
}

func TestSetFocusedCMail_ClearsUnreadOnReturn(t *testing.T) {
	m := cmailInConversation(api.NewMockClient(), "c1")
	m = m.SetConversations([]model.Conversation{{ID: "c1"}})
	m = m.SetFocused(false)
	m, _ = m.Update(dmReceivedMsg{msg: model.Message{Body: "hey"}})
	if got := m.TotalUnread(); got != 1 {
		t.Fatalf("setup: expected TotalUnread 1, got %d", got)
	}

	m = m.SetFocused(true)
	if got := m.TotalUnread(); got != 0 {
		t.Errorf("TotalUnread() = %d, want 0 after refocusing the tab", got)
	}
}

func TestHasLiveConv(t *testing.T) {
	m := NewCMailModel("neo", "", api.NewMockClient())
	if m.HasLiveConv() {
		t.Error("expected no live conversation before any conversation is opened")
	}

	m = cmailInConversation(api.NewMockClient(), "c1")
	if !m.HasLiveConv() {
		t.Error("expected HasLiveConv() once a conversation is open in detail mode")
	}
}

func TestIsDMStreamMsg(t *testing.T) {
	streamMsgs := []tea.Msg{
		dmReceivedMsg{},
		dmStreamClosedMsg{},
		typingHeartbeatTickMsg{},
		dmTypingReceivedMsg{},
	}
	for _, msg := range streamMsgs {
		if !IsDMStreamMsg(msg) {
			t.Errorf("IsDMStreamMsg(%T) = false, want true", msg)
		}
	}
	if IsDMStreamMsg(tea.KeyMsg{Type: tea.KeyEnter}) {
		t.Error("IsDMStreamMsg(tea.KeyMsg) = true, want false — key input must not be routed to a backgrounded conversation")
	}
}

func TestDMReceived_KeepsStreamingWhileUnfocused(t *testing.T) {
	m := cmailInConversation(api.NewMockClient(), "c1")
	m.dmSub = &dmSubscription{ConvID: "c1", C: make(chan model.Message)}
	m = m.SetFocused(false)

	_, cmd := m.Update(dmReceivedMsg{msg: model.Message{Body: "hey"}})
	if cmd == nil {
		t.Error("expected waitForDM to be re-issued so the stream keeps running in the background")
	}
}

func TestComposeEmptyCMail(t *testing.T) {
	m := cmailInConversation(api.NewMockClient(), "c1")
	if !m.ComposeEmpty() {
		t.Error("expected ComposeEmpty() true on a freshly opened conversation")
	}

	m.input.SetValue("hi")
	if m.ComposeEmpty() {
		t.Error("expected ComposeEmpty() false once text has been typed")
	}
}

func TestTypingAnnounced_StoresCadenceAndSchedulesHeartbeat(t *testing.T) {
	m := cmailInConversation(api.NewMockClient(), "c1")

	m, cmd := m.Update(typingAnnouncedMsg{convID: "c1", heartbeatMs: 3000, staleAfterMs: 9000})
	if m.typingHeartbeatMs != 3000 {
		t.Errorf("typingHeartbeatMs = %d, want 3000", m.typingHeartbeatMs)
	}
	if cmd == nil {
		t.Fatal("expected a scheduled heartbeat command")
	}
}

func TestTypingAnnounced_StaleConvIDIgnored(t *testing.T) {
	m := cmailInConversation(api.NewMockClient(), "c1")

	m2, cmd := m.Update(typingAnnouncedMsg{convID: "old-abandoned-conv", heartbeatMs: 3000, staleAfterMs: 9000})
	if cmd != nil {
		t.Error("expected no command for a stale announce response")
	}
	if m2.typingHeartbeatMs != 0 {
		t.Errorf("expected typingHeartbeatMs to remain unset, got %d", m2.typingHeartbeatMs)
	}
}

func TestTypingHeartbeatTick_StaleConvIDIgnored(t *testing.T) {
	m := cmailInConversation(api.NewMockClient(), "c1")
	m.announcingTyping = true
	m.typingHeartbeatMs = 3000

	_, cmd := m.Update(typingHeartbeatTickMsg{convID: "old-abandoned-conv"})
	if cmd != nil {
		t.Error("expected no command for a heartbeat tick from a conversation the user already left")
	}
}

func TestTypingHeartbeatTick_NotAnnouncingIgnored(t *testing.T) {
	m := cmailInConversation(api.NewMockClient(), "c1")
	m.typingHeartbeatMs = 3000

	_, cmd := m.Update(typingHeartbeatTickMsg{convID: "c1"})
	if cmd != nil {
		t.Error("expected no command for a heartbeat tick after typing already stopped")
	}
}

func TestTypingHeartbeatTick_ActiveReschedules(t *testing.T) {
	m := cmailInConversation(api.NewMockClient(), "c1")
	m.announcingTyping = true
	m.typingHeartbeatMs = 3000

	_, cmd := m.Update(typingHeartbeatTickMsg{convID: "c1"})
	if cmd == nil {
		t.Fatal("expected a batch command (send heartbeat + reschedule tick)")
	}
}

func TestTypingSubscribed_StaleConvIDCancelsAndIgnores(t *testing.T) {
	m := cmailInConversation(api.NewMockClient(), "c1")

	cancelled := false
	sub := &dmTypingSubscription{ConvID: "old-abandoned-conv", cancel: func() { cancelled = true }}

	m2, cmd := m.Update(dmTypingSubscribedMsg{convID: "old-abandoned-conv", sub: sub})
	if !cancelled {
		t.Error("expected the stale subscription to be cancelled")
	}
	if cmd != nil {
		t.Error("expected no command for a stale subscribe response")
	}
	if m2.typingSub != nil {
		t.Error("expected typingSub to remain nil for a stale subscribe response")
	}
}

func TestTypingStreamClosed_ResubscribesWhileConvStillOpen(t *testing.T) {
	m := cmailInConversation(api.NewMockClient(), "c1")

	m2, cmd := m.Update(dmTypingStreamClosedMsg{convID: "c1"})
	if m2.typingSub != nil {
		t.Error("expected typingSub to be cleared")
	}
	if cmd == nil {
		t.Error("expected an immediate resubscribe command")
	}
}

func TestTypingStreamClosed_StaleConvIDIgnored(t *testing.T) {
	m := cmailInConversation(api.NewMockClient(), "c1")

	_, cmd := m.Update(dmTypingStreamClosedMsg{convID: "old-abandoned-conv"})
	if cmd != nil {
		t.Error("expected no command for a stale stream-closed event")
	}
}

// TestKeystroke_NonEmptyInput_AnnouncesTypingOnceAcrossMultipleKeys types "h"
// then "i" and expects announcingTyping to flip false→true exactly once, not
// re-announce on every keystroke.
func TestKeystroke_NonEmptyInput_AnnouncesTypingOnceAcrossMultipleKeys(t *testing.T) {
	m := cmailInConversation(api.NewMockClient(), "c1")

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	if !m.announcingTyping {
		t.Fatal("expected announcingTyping to be true after the first keystroke")
	}
	if cmd == nil {
		t.Fatal("expected an announce+idle-check command on the first keystroke")
	}
	firstKeystrokeAt := m.lastKeystrokeAt

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	if !m.announcingTyping {
		t.Fatal("expected announcingTyping to remain true after the second keystroke")
	}
	if !m.lastKeystrokeAt.After(firstKeystrokeAt) && !m.lastKeystrokeAt.Equal(firstKeystrokeAt) {
		t.Error("expected lastKeystrokeAt to advance on the second keystroke")
	}
	if m.input.Value() != "hi" {
		t.Fatalf("input.Value() = %q, want hi", m.input.Value())
	}
}

// TestKeystroke_EmptyInput_ClearsTypingImmediately backspaces to empty while
// announcing and expects an immediate clear, not a wait for the idle tick.
func TestKeystroke_EmptyInput_ClearsTypingImmediately(t *testing.T) {
	m := cmailInConversation(api.NewMockClient(), "c1")
	m.input.SetValue("h")
	m.input.SetCursor(1)
	m.announcingTyping = true

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if m.input.Value() != "" {
		t.Fatalf("input.Value() = %q, want empty after backspace", m.input.Value())
	}
	if m.announcingTyping {
		t.Error("expected announcingTyping to flip false immediately on emptying the input")
	}
	if cmd == nil {
		t.Fatal("expected a clear-typing command")
	}
}

func TestIdleCheck_NoRecentKeystroke_ClearsTyping(t *testing.T) {
	m := cmailInConversation(api.NewMockClient(), "c1")
	m.announcingTyping = true
	m.lastKeystrokeAt = time.Now().Add(-5 * time.Second)

	m2, cmd := m.Update(typingIdleCheckMsg{convID: "c1"})
	if m2.announcingTyping {
		t.Error("expected announcingTyping to flip false after the idle threshold")
	}
	if cmd == nil {
		t.Fatal("expected a clear-typing command")
	}
}

func TestIdleCheck_RecentKeystroke_Reschedules(t *testing.T) {
	m := cmailInConversation(api.NewMockClient(), "c1")
	m.announcingTyping = true
	m.lastKeystrokeAt = time.Now()

	m2, cmd := m.Update(typingIdleCheckMsg{convID: "c1"})
	if !m2.announcingTyping {
		t.Error("expected announcingTyping to remain true with a recent keystroke")
	}
	if cmd == nil {
		t.Fatal("expected a rescheduled idle-check command")
	}
}

// TestSendMessage_ClearsAnnouncingTypingWithoutNetworkCall presses enter
// while announcing and expects the local flag to clear, but no ClearTyping
// call bundled in — the server auto-clears typing on send.
func TestSendMessage_ClearsAnnouncingTypingWithoutNetworkCall(t *testing.T) {
	m := cmailInConversation(api.NewMockClient(), "c1")
	m.input.SetValue("hello")
	m.input.SetCursor(5)
	m.announcingTyping = true

	m2, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m2.announcingTyping {
		t.Error("expected announcingTyping to flip false on send")
	}
	if cmd == nil {
		t.Fatal("expected a SendCMailMsg command")
	}
	if msg := cmd(); msg == nil {
		t.Fatal("expected the command to produce a message")
	} else if _, ok := msg.(SendCMailMsg); !ok {
		t.Errorf("expected SendCMailMsg, got %T", msg)
	}
}

func TestEsc_SendsClearTypingWhenAnnouncing(t *testing.T) {
	m := cmailInConversation(api.NewMockClient(), "c1")
	m.announcingTyping = true

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected a batch command including clearTypingCmd")
	}
}

func TestEsc_NoClearTypingWhenNotAnnouncing(t *testing.T) {
	m := cmailInConversation(api.NewMockClient(), "c1")

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		t.Error("expected no command when leaving a conversation without an active typing announce")
	}
}

// TestTypingReceived_ShowsIndicatorForOtherParticipant confirms the
// indicator appears right after the header's own "@other" title (not
// repeating the username a second time) so it reads as one sentence.
func TestTypingReceived_ShowsIndicatorForOtherParticipant(t *testing.T) {
	m := cmailInConversation(api.NewMockClient(), "c1")

	m, _ = m.Update(dmTypingReceivedMsg{users: []model.TypingUser{{UserID: "u2", Username: "trinity", Timestamp: time.Now()}}})
	view := m.View()
	if !strings.Contains(view, "@trinity is typing") {
		t.Errorf("expected the header to read '@trinity is typing...', got: %q", view)
	}
	if strings.Count(view, "trinity") != 1 {
		t.Errorf("expected trinity's name to appear exactly once (not repeated in the indicator), got: %q", view)
	}
}

func TestTypingReceived_IgnoresEntryForSelf(t *testing.T) {
	m := cmailInConversation(api.NewMockClient(), "c1")

	m, _ = m.Update(dmTypingReceivedMsg{users: []model.TypingUser{{UserID: "u1", Username: "neo", Timestamp: time.Now()}}})
	if strings.Contains(m.View(), "is typing") {
		t.Errorf("did not expect a typing indicator for the current user, got: %q", m.View())
	}
}

func TestTypingAnimTick_AdvancesFrameAndReschedules(t *testing.T) {
	m := cmailInConversation(api.NewMockClient(), "c1")

	m, cmd := m.Update(typingAnimTickMsg{convID: "c1"})
	if m.typingAnimFrame != 1 {
		t.Errorf("typingAnimFrame = %d, want 1", m.typingAnimFrame)
	}
	if cmd == nil {
		t.Fatal("expected a rescheduled animation tick command")
	}
}

func TestTypingAnimTick_StaleConvIDIgnored(t *testing.T) {
	m := cmailInConversation(api.NewMockClient(), "c1")

	m2, cmd := m.Update(typingAnimTickMsg{convID: "old-abandoned-conv"})
	if cmd != nil {
		t.Error("expected no command for a stale animation tick")
	}
	if m2.typingAnimFrame != 0 {
		t.Errorf("expected typingAnimFrame to remain unchanged, got %d", m2.typingAnimFrame)
	}
}

func TestTypingIndicator_DotsCycleThroughZeroOneTwoThree(t *testing.T) {
	m := cmailInConversation(api.NewMockClient(), "c1")
	m.typingUsers = []model.TypingUser{{UserID: "u2", Username: "trinity", Timestamp: time.Now()}}

	want := []int{0, 1, 2, 3, 0, 1, 2, 3}
	for frame, wantDots := range want {
		m.typingAnimFrame = frame
		got := strings.Count(m.typingIndicator(), ".")
		if got != wantDots {
			t.Errorf("frame %d: dot count = %d, want %d", frame, got, wantDots)
		}
	}
}

// --- style animation ticker (coarse-scoped, see maybeStartStyleAnim) ---

func TestCMailUpdate_StartsStyleAnimTickerWhenAnimatedMessageArrives(t *testing.T) {
	m := cmailInConversation(api.NewMockClient(), "c1")

	m, cmd := m.Update(dmReceivedMsg{msg: model.Message{ID: "m1", Body: "hi", Style: []string{"blink"}}})

	if !m.styleAnimRunning {
		t.Error("expected styleAnimRunning = true after an animated-style message arrived")
	}
	if cmd == nil {
		t.Error("expected a non-nil tea.Cmd to start the animation ticker")
	}
}

func TestCMailUpdate_StyleAnimTick_AdvancesFrameAndRearms(t *testing.T) {
	m := cmailInConversation(api.NewMockClient(), "c1")
	m, _ = m.Update(dmReceivedMsg{msg: model.Message{ID: "m1", Body: "hi", Style: []string{"wave"}}})
	if !m.styleAnimRunning {
		t.Fatal("setup: expected styleAnimRunning = true")
	}

	m, cmd := m.Update(styleAnimTickMsg{})

	if m.styleAnimFrame != 1 {
		t.Errorf("styleAnimFrame = %d, want 1", m.styleAnimFrame)
	}
	if cmd == nil {
		t.Error("expected a non-nil tea.Cmd to rearm the ticker")
	}
}

// TestCMailUpdate_StyleAnimTick_PausedSkipsRerenderButKeepsTicking guards the
// image-modal fix: while animPaused is set, a styleAnimTickMsg must not
// change the rendered viewport content, but the ticker chain must keep
// rearming so the animation resumes immediately once animPaused is cleared.
func TestCMailUpdate_StyleAnimTick_PausedSkipsRerenderButKeepsTicking(t *testing.T) {
	m := cmailInConversation(api.NewMockClient(), "c1")
	m, _ = m.Update(dmReceivedMsg{msg: model.Message{ID: "m1", Body: "hi", Style: []string{"wave"}}})
	if !m.styleAnimRunning {
		t.Fatal("setup: expected styleAnimRunning = true")
	}
	m = m.SetAnimPaused(true)
	before := m.viewport.View()

	m, cmd := m.Update(styleAnimTickMsg{})

	if m.viewport.View() != before {
		t.Error("expected the viewport to be unchanged by a styleAnimTickMsg while animPaused")
	}
	if cmd == nil {
		t.Error("expected a non-nil tea.Cmd to rearm the ticker even while paused")
	}
}

func TestSetFocusedCMail_ResumesStyleAnimAfterBackgroundedTickIsDropped(t *testing.T) {
	m := cmailInConversation(api.NewMockClient(), "c1")
	m, _ = m.Update(dmReceivedMsg{msg: model.Message{ID: "m1", Body: "hi", Style: []string{"wave"}}})
	if !m.styleAnimRunning {
		t.Fatal("setup: expected styleAnimRunning = true")
	}

	// Simulate switching away: the in-flight styleAnimTickMsg is dropped by
	// App's message routing (it's not an IsDMStreamMsg), so updateInner
	// never runs to clear styleAnimRunning. Switching back must still be
	// able to restart the ticker.
	m = m.SetFocused(false)
	m = m.SetFocused(true)

	m, cmd := m.maybeStartStyleAnim()
	if !m.styleAnimRunning {
		t.Error("expected styleAnimRunning = true after resuming")
	}
	if cmd == nil {
		t.Error("expected a non-nil tea.Cmd restarting the ticker after refocusing")
	}
}

// --- VisibleInlineImages ---

// TestCMail_VisibleInlineImages_DisabledByDefault mirrors Feed's equivalent:
// a message with an eligible image URL in its body reports no slots until
// InlineImagesEnabled arrives via SharedConfigMsg.
func TestCMail_VisibleInlineImages_DisabledByDefault(t *testing.T) {
	m := cmailInConversation(api.NewMockClient(), "c1")
	m = m.SetConversationMessages("c1", []model.Message{
		{ID: "m1", From: model.User{Username: "trinity"}, Body: "check this out https://example.com/pic.png", CreatedAt: time.Now()},
	})

	if slots := m.VisibleInlineImages(); slots != nil {
		t.Errorf("expected no slots while disabled, got %+v", slots)
	}
}

// TestCMail_VisibleInlineImages_AttachmentAndBodyURL confirms both image
// sources chatInlineImageURL recognizes (the ImageUrl attachment field, and a
// plain image URL typed in the body) produce a slot once enabled, without
// disturbing the other's existing text/badge.
func TestCMail_VisibleInlineImages_AttachmentAndBodyURL(t *testing.T) {
	m := cmailInConversation(api.NewMockClient(), "c1")
	m, _ = m.Update(tea.WindowSizeMsg{Width: 160, Height: 80}) // tall enough that both messages' image bands stay fully in view
	m, _ = m.Update(SharedConfigMsg{InlineImagesEnabled: true})
	m = m.SetConversationMessages("c1", []model.Message{
		{ID: "m1", From: model.User{Username: "trinity"}, ImageUrl: "https://example.com/attach.png", CreatedAt: time.Now()},
		{ID: "m2", From: model.User{Username: "trinity"}, Body: "look at https://example.com/pic.png", CreatedAt: time.Now().Add(time.Minute)},
	})

	slots := m.VisibleInlineImages()
	if len(slots) != 2 {
		t.Fatalf("expected 2 slots (attachment + body URL), got %d: %+v", len(slots), slots)
	}
	if slots[0].URL != "https://example.com/attach.png" {
		t.Errorf("slots[0].URL = %q, want the attachment URL", slots[0].URL)
	}
	if slots[1].URL != "https://example.com/pic.png" {
		t.Errorf("slots[1].URL = %q, want the body URL", slots[1].URL)
	}
}

// TestCMail_VisibleInlineImages_RowAccountsForHeader mirrors Chatrooms'
// equivalent regression test: View() stacks a 1-line header and a 1-line
// divider above the message viewport (cmailDetailHeaderRows), so a slot's
// screen-relative Row must include that offset or an image near the top of
// the viewport lands on the header/divider instead of its message.
func TestCMail_VisibleInlineImages_RowAccountsForHeader(t *testing.T) {
	m := cmailInConversation(api.NewMockClient(), "c1")
	m, _ = m.Update(tea.WindowSizeMsg{Width: 160, Height: 80})
	m, _ = m.Update(SharedConfigMsg{InlineImagesEnabled: true})
	m = m.SetConversationMessages("c1", []model.Message{
		{ID: "m1", From: model.User{Username: "trinity"}, ImageUrl: "https://example.com/a.png", CreatedAt: time.Now()},
	})
	m.viewport.SetYOffset(0)

	slots := m.VisibleInlineImages()
	if len(slots) != 1 {
		t.Fatalf("expected 1 slot, got %d: %+v", len(slots), slots)
	}
	wantRow := m.msgOffsets[0] + m.msgImages[0][0].Line + cmailDetailHeaderRows
	if slots[0].Row != wantRow {
		t.Errorf("Row = %d, want %d (viewport-relative position + cmailDetailHeaderRows) — a message near the top of the viewport must not land on the header/divider lines above it", slots[0].Row, wantRow)
	}

	lines := strings.Split(m.View(), "\n")
	if slots[0].Row >= len(lines) {
		t.Fatalf("Row %d is out of range of View()'s %d lines", slots[0].Row, len(lines))
	}
	if strings.Contains(lines[slots[0].Row], "@trinity") || strings.Contains(ansi.Strip(lines[slots[0].Row]), "─") {
		t.Errorf("Row %d lands on the header/divider line: %q", slots[0].Row, lines[slots[0].Row])
	}
}

// TestCMail_InlineImages_SuppressesRedundantTextOnceEnabled mirrors
// Chatrooms' equivalent: the attachment badge and a body that's nothing but
// the image URL disappear once inline images are enabled, reappear when
// disabled, and a URL embedded alongside other text is left alone either way.
func TestCMail_InlineImages_SuppressesRedundantTextOnceEnabled(t *testing.T) {
	m := cmailInConversation(api.NewMockClient(), "c1")
	m, _ = m.Update(tea.WindowSizeMsg{Width: 160, Height: 80})
	msgs := []model.Message{
		{ID: "m1", From: model.User{Username: "trinity"}, ImageUrl: "https://example.com/attach.png", CreatedAt: time.Now()},
		{ID: "m2", From: model.User{Username: "trinity"}, Body: "https://example.com/pic.png", CreatedAt: time.Now().Add(time.Minute)},
		{ID: "m3", From: model.User{Username: "trinity"}, Body: "check this out: https://example.com/mixed.png", CreatedAt: time.Now().Add(2 * time.Minute)},
	}

	disabled, _, _, _ := renderChatMessagesWithSelection(msgs, m.currentUser, m.location(), m.timeDisplayFormat, m.viewport.Width, m.styleAnimFrame, "", false, nil)
	if !strings.Contains(disabled, "https://example.com/attach.png") || !strings.Contains(disabled, "https://example.com/pic.png") {
		t.Fatalf("setup: expected both URLs visible while disabled, got: %q", disabled)
	}

	enabled, _, _, _ := renderChatMessagesWithSelection(msgs, m.currentUser, m.location(), m.timeDisplayFormat, m.viewport.Width, m.styleAnimFrame, "", true, nil)
	if strings.Contains(enabled, "https://example.com/attach.png") {
		t.Errorf("expected the attachment URL suppressed once enabled, got: %q", enabled)
	}
	if strings.Contains(enabled, "[image]") {
		t.Errorf("expected the [image] badge suppressed once enabled, got: %q", enabled)
	}
	if strings.Contains(enabled, "https://example.com/pic.png") {
		t.Errorf("expected the body-only URL suppressed once enabled, got: %q", enabled)
	}
	if !strings.Contains(enabled, "https://example.com/mixed.png") {
		t.Errorf("expected a URL embedded alongside other text to stay visible (known scope limit), got: %q", enabled)
	}
}

// TestCMail_SetImageRealRows_ShrinksBandAndMovesLaterMessages mirrors
// Chatrooms' equivalent: once the real fetched row count is known, a later
// message must move up to sit right after the now much shorter band.
func TestCMail_SetImageRealRows_ShrinksBandAndMovesLaterMessages(t *testing.T) {
	m := cmailInConversation(api.NewMockClient(), "c1")
	m, _ = m.Update(tea.WindowSizeMsg{Width: 160, Height: 200})
	m, _ = m.Update(SharedConfigMsg{InlineImagesEnabled: true})
	m = m.SetConversationMessages("c1", []model.Message{
		{ID: "m1", From: model.User{Username: "trinity"}, ImageUrl: "https://example.com/a.png", CreatedAt: time.Now()},
		{ID: "m2", From: model.User{Username: "trinity"}, Body: "after the image", CreatedAt: time.Now().Add(time.Minute)},
	})
	if len(m.msgOffsets) != 2 {
		t.Fatalf("setup: expected 2 message offsets, got %d", len(m.msgOffsets))
	}
	beforeOffset := m.msgOffsets[1]

	m = m.SetImageRealRows(cmailMsgImageKey("m1"), 2)
	afterOffset := m.msgOffsets[1]

	if afterOffset >= beforeOffset {
		t.Errorf("expected m2's offset to move up once m1's image band shrank: before=%d, after=%d", beforeOffset, afterOffset)
	}
}

// TestCMail_SetImageRealRows_StaysAtBottomWhenAlreadyThere mirrors
// Chatrooms' equivalent regression test for the conversation-open scroll gap.
func TestCMail_SetImageRealRows_StaysAtBottomWhenAlreadyThere(t *testing.T) {
	m := cmailInConversation(api.NewMockClient(), "c1")
	m, _ = m.Update(tea.WindowSizeMsg{Width: 160, Height: 12})
	m, _ = m.Update(SharedConfigMsg{InlineImagesEnabled: true})
	m = m.SetConversationMessages("c1", []model.Message{
		{ID: "m1", From: model.User{Username: "trinity"}, ImageUrl: "https://example.com/a.png", CreatedAt: time.Now()},
		{ID: "m2", From: model.User{Username: "trinity"}, Body: "one", CreatedAt: time.Now().Add(time.Minute)},
		{ID: "m3", From: model.User{Username: "trinity"}, Body: "two", CreatedAt: time.Now().Add(2 * time.Minute)},
		{ID: "m4", From: model.User{Username: "trinity"}, Body: "three", CreatedAt: time.Now().Add(3 * time.Minute)},
	})
	if !m.viewport.AtBottom() {
		t.Fatal("setup: expected SetConversationMessages to land at the bottom")
	}

	m = m.SetImageRealRows(cmailMsgImageKey("m1"), 2)

	if !m.viewport.AtBottom() {
		t.Error("expected the viewport to still be at the bottom after the band shrank, not settle above it")
	}
}

// TestCMail_SetImageRealRows_DoesNotYankScrolledUpView mirrors Chatrooms'
// equivalent: a user reading history shouldn't be pulled back to the bottom.
func TestCMail_SetImageRealRows_DoesNotYankScrolledUpView(t *testing.T) {
	m := cmailInConversation(api.NewMockClient(), "c1")
	m, _ = m.Update(tea.WindowSizeMsg{Width: 160, Height: 12})
	m, _ = m.Update(SharedConfigMsg{InlineImagesEnabled: true})
	m = m.SetConversationMessages("c1", []model.Message{
		{ID: "m1", From: model.User{Username: "trinity"}, ImageUrl: "https://example.com/a.png", CreatedAt: time.Now()},
		{ID: "m2", From: model.User{Username: "trinity"}, Body: "one", CreatedAt: time.Now().Add(time.Minute)},
		{ID: "m3", From: model.User{Username: "trinity"}, Body: "two", CreatedAt: time.Now().Add(2 * time.Minute)},
		{ID: "m4", From: model.User{Username: "trinity"}, Body: "three", CreatedAt: time.Now().Add(3 * time.Minute)},
	})
	m.viewport.SetYOffset(0)
	if m.viewport.AtBottom() {
		t.Fatal("setup: expected scrolling to top to leave the bottom")
	}

	m = m.SetImageRealRows(cmailMsgImageKey("m1"), 2)

	if m.viewport.AtBottom() {
		t.Error("expected the scrolled-up view to stay put, not get yanked back to the bottom")
	}
}

// --- Always-sticky scroll + unread-while-scrolled-up ---

// manyPlainCMailMessages returns n one-line messages, enough to overflow a
// small test viewport so AtBottom()/scrolling are actually meaningful.
func manyPlainCMailMessages(n int) []model.Message {
	msgs := make([]model.Message, n)
	for i := range msgs {
		msgs[i] = model.Message{
			ID:        fmt.Sprintf("m%d", i),
			From:      model.User{Username: "trinity"},
			Body:      fmt.Sprintf("message %d", i),
			CreatedAt: time.Now().Add(time.Duration(i) * time.Minute),
		}
	}
	return msgs
}

// TestCMail_AppendMessage_DoesNotScrollWhenScrolledUp mirrors Chatrooms'
// equivalent: any live message must never auto-scroll a scrolled-up view.
func TestCMail_AppendMessage_DoesNotScrollWhenScrolledUp(t *testing.T) {
	m := cmailInConversation(api.NewMockClient(), "c1")
	m, _ = m.Update(tea.WindowSizeMsg{Width: 160, Height: 12})
	m = m.SetConversationMessages("c1", manyPlainCMailMessages(10))
	m.viewport.SetYOffset(0)
	if m.viewport.AtBottom() {
		t.Fatal("setup: expected scrolling to top to leave the bottom")
	}
	yOffsetBefore := m.viewport.YOffset

	m = m.AppendMessage(model.Message{ID: "new", From: model.User{Username: "trinity"}, Body: "just arrived", CreatedAt: time.Now()})

	if m.viewport.YOffset != yOffsetBefore {
		t.Errorf("expected YOffset to stay at %d, got %d — a new message scrolled a scrolled-up view", yOffsetBefore, m.viewport.YOffset)
	}
	if m.viewport.AtBottom() {
		t.Error("expected the scrolled-up view to stay scrolled up after a new message arrives")
	}
}

// TestCMail_AppendMessage_FollowsWhenAlreadyAtBottom confirms the existing
// "caught up" behavior still auto-follows new messages.
func TestCMail_AppendMessage_FollowsWhenAlreadyAtBottom(t *testing.T) {
	m := cmailInConversation(api.NewMockClient(), "c1")
	m, _ = m.Update(tea.WindowSizeMsg{Width: 160, Height: 12})
	m = m.SetConversationMessages("c1", manyPlainCMailMessages(10))
	if !m.viewport.AtBottom() {
		t.Fatal("setup: expected SetConversationMessages to land at the bottom")
	}

	m = m.AppendMessage(model.Message{ID: "new", From: model.User{Username: "trinity"}, Body: "just arrived", CreatedAt: time.Now()})

	if !m.viewport.AtBottom() {
		t.Error("expected the view to keep following once caught up")
	}
}

// TestCMail_TotalUnread_IncrementsWhileFocusedButScrolledUp confirms the tab
// badge grows even while C-Mail is the active screen, as long as the view
// isn't at the bottom.
func TestCMail_TotalUnread_IncrementsWhileFocusedButScrolledUp(t *testing.T) {
	m := cmailInConversation(api.NewMockClient(), "c1")
	m.conversations = []model.Conversation{{ID: "c1"}} // bumpActiveConvUnread/TotalUnread read from here, not activeConv
	m, _ = m.Update(tea.WindowSizeMsg{Width: 160, Height: 12})
	m = m.SetFocused(true)
	m = m.SetConversationMessages("c1", manyPlainCMailMessages(10))
	m.viewport.SetYOffset(0)
	if m.viewport.AtBottom() {
		t.Fatal("setup: expected scrolling to top to leave the bottom")
	}

	m, _ = m.Update(dmReceivedMsg{msg: model.Message{ID: "new", From: model.User{Username: "trinity"}, Body: "hi", CreatedAt: time.Now()}})

	if m.TotalUnread() != 1 {
		t.Errorf("TotalUnread() = %d, want 1", m.TotalUnread())
	}
}

// TestCMail_TotalUnread_NoIncrementWhileFocusedAndAtBottom confirms a
// message you're actively watching arrive never marks itself unread.
func TestCMail_TotalUnread_NoIncrementWhileFocusedAndAtBottom(t *testing.T) {
	m := cmailInConversation(api.NewMockClient(), "c1")
	m.conversations = []model.Conversation{{ID: "c1"}}
	m, _ = m.Update(tea.WindowSizeMsg{Width: 160, Height: 12})
	m = m.SetFocused(true)
	m = m.SetConversationMessages("c1", manyPlainCMailMessages(10))
	if !m.viewport.AtBottom() {
		t.Fatal("setup: expected SetConversationMessages to land at the bottom")
	}

	m, _ = m.Update(dmReceivedMsg{msg: model.Message{ID: "new", From: model.User{Username: "trinity"}, Body: "hi", CreatedAt: time.Now()}})

	if m.TotalUnread() != 0 {
		t.Errorf("TotalUnread() = %d, want 0 (caught up, no reason to mark unread)", m.TotalUnread())
	}
}

// TestCMail_TotalUnread_ClearsWhenScrolledBackToBottom confirms the badge
// clears once the user scrolls back down themselves.
func TestCMail_TotalUnread_ClearsWhenScrolledBackToBottom(t *testing.T) {
	m := cmailInConversation(api.NewMockClient(), "c1")
	m.conversations = []model.Conversation{{ID: "c1"}}
	m, _ = m.Update(tea.WindowSizeMsg{Width: 160, Height: 12})
	m = m.SetFocused(true)
	m = m.SetConversationMessages("c1", manyPlainCMailMessages(10))
	m.viewport.SetYOffset(0)
	m, _ = m.Update(dmReceivedMsg{msg: model.Message{ID: "new", From: model.User{Username: "trinity"}, Body: "hi", CreatedAt: time.Now()}})
	if m.TotalUnread() == 0 {
		t.Fatal("setup: expected TotalUnread > 0 while scrolled up")
	}

	m.viewport.GotoBottom()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 160, Height: 12}) // drive a Update turn so the outer post-check runs

	if m.TotalUnread() != 0 {
		t.Errorf("TotalUnread() = %d, want 0 after scrolling back to the bottom", m.TotalUnread())
	}
}

// --- Last-message image visibility regression ---

// TestCMail_VisibleInlineImages_LastMessageImage_FallbackStage mirrors
// Chatrooms' equivalent regression test: with nothing posted below it, an
// image on the very last message must still be reported visible at the
// fallback-max band stage — the visibility check must use the actual
// reserved clearance for that message, not the old fixed
// inlineImageMaxRows, which over-required room that isn't there when
// there's no later message to push the viewport's bottom edge out further.
func TestCMail_VisibleInlineImages_LastMessageImage_FallbackStage(t *testing.T) {
	m := cmailInConversation(api.NewMockClient(), "c1")
	// Height must be tall enough to fit the image's own band but shorter
	// than the total content, so GotoBottom actually clamps "bottom" to
	// the true content end — see Chatrooms' equivalent test for why.
	// actual viewport.Height = 18 - theme.ChromeHeight(3) - cmailDetailChrome(5) = 10
	m, _ = m.Update(tea.WindowSizeMsg{Width: 160, Height: 18})
	m, _ = m.Update(SharedConfigMsg{InlineImagesEnabled: true})
	msgs := manyPlainCMailMessages(5)
	msgs = append(msgs, model.Message{ID: "last", From: model.User{Username: "trinity"}, ImageUrl: "https://example.com/a.png", CreatedAt: time.Now()})
	m = m.SetConversationMessages("c1", msgs)

	slots := m.VisibleInlineImages()
	if len(slots) != 1 {
		t.Fatalf("expected the last message's image to be visible at the fallback stage, got %d slots: %+v", len(slots), slots)
	}
}

// TestCMail_VisibleInlineImages_LastMessageImage_AfterRealRowsKnown confirms
// the same holds once SetImageRealRows shrinks the band to a real size.
func TestCMail_VisibleInlineImages_LastMessageImage_AfterRealRowsKnown(t *testing.T) {
	m := cmailInConversation(api.NewMockClient(), "c1")
	m, _ = m.Update(tea.WindowSizeMsg{Width: 160, Height: 18})
	m, _ = m.Update(SharedConfigMsg{InlineImagesEnabled: true})
	msgs := manyPlainCMailMessages(5)
	msgs = append(msgs, model.Message{ID: "last", From: model.User{Username: "trinity"}, ImageUrl: "https://example.com/a.png", CreatedAt: time.Now()})
	m = m.SetConversationMessages("c1", msgs)
	m = m.SetImageRealRows(cmailMsgImageKey("last"), 2)

	slots := m.VisibleInlineImages()
	if len(slots) != 1 {
		t.Fatalf("expected the last message's image to stay visible after its band shrank, got %d slots: %+v", len(slots), slots)
	}
}

// --- view profile (see updateCMailBrowsingKey's "p" case) ---

func TestCMailBrowsing_P_EmitsShowUserProfileMsg(t *testing.T) {
	m := cmailInConversation(api.NewMockClient(), "c1")
	m = m.SetConversationMessages("c1", []model.Message{
		{ID: "m1", From: model.User{Username: "trinity"}, Body: "hi", CreatedAt: time.Now()},
	})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.selectedMsgID != "m1" {
		t.Fatalf("setup: selectedMsgID = %q, want m1", m.selectedMsgID)
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	if cmd == nil {
		t.Fatal("expected a cmd")
	}
	sp, ok := cmd().(ShowUserProfileMsg)
	if !ok {
		t.Fatalf("expected ShowUserProfileMsg, got %T", cmd())
	}
	if sp.Username != "trinity" {
		t.Errorf("Username = %q, want trinity", sp.Username)
	}
}

func TestCMailBrowsing_P_NoSelectedMessage_IsNoop(t *testing.T) {
	m := cmailInConversation(api.NewMockClient(), "c1")
	m = m.SetConversationMessages("c1", nil)

	_, cmd := m.updateCMailBrowsingKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	if cmd != nil {
		t.Error("expected no-op when nothing is selected")
	}
}
