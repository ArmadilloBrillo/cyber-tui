package screens

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
