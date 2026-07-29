package screens

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ragnar/cyber-tui/internal/api"
	"github.com/ragnar/cyber-tui/internal/model"
)

func TestCMailTotalUnread(t *testing.T) {
	m := NewCMailModel("me", nil)

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

// TestCMailDetailView_HeaderHasDividerBeforeMessages guards against the
// divider row (mirrors the same fix in chatrooms.go — the header shouldn't
// float with no visual bottom edge, unlike the bordered input box below it)
// being dropped in a future edit.
func TestCMailDetailView_HeaderHasDividerBeforeMessages(t *testing.T) {
	conv := model.Conversation{ID: "c1", Participants: []model.User{{Username: "neo"}, {Username: "trinity"}}}
	m := NewCMailModel("neo", api.NewMockClient())
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
	m := NewCMailModel("neo", api.NewMockClient())
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
