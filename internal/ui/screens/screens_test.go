package screens_test

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/screens"
)

// --- ChatroomsModel.InputFocused ---

func TestChatroomsInputFocused_DefaultFalse(t *testing.T) {
	m := screens.NewChatroomsModel()
	if m.InputFocused() {
		t.Error("input should not be focused on a freshly created ChatroomsModel")
	}
}

// --- CMailModel.InputFocused ---

func TestCMailInputFocused_DefaultFalse(t *testing.T) {
	m := screens.NewCMailModel("neuromancer")
	if m.InputFocused() {
		t.Error("input should not be focused on a freshly created CMailModel")
	}
}

// --- helpers ---

func twoConvs() []model.Conversation {
	return []model.Conversation{
		{
			ID:           "c1",
			Participants: []model.User{{Username: "neuromancer"}, {Username: "molly"}},
			Messages: []model.Message{
				{ID: "m1", From: model.User{Username: "molly"}, Body: "hey there", CreatedAt: time.Now()},
			},
		},
		{
			ID:           "c2",
			Participants: []model.User{{Username: "neuromancer"}, {Username: "wintermute"}},
			Messages: []model.Message{
				{ID: "m2", From: model.User{Username: "wintermute"}, Body: "i am the one", CreatedAt: time.Now()},
			},
		},
	}
}

func sendKey(m screens.CMailModel, key string) (screens.CMailModel, tea.Cmd) {
	return m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
}

func sendSpecialKey(m screens.CMailModel, keyType tea.KeyType) (screens.CMailModel, tea.Cmd) {
	return m.Update(tea.KeyMsg{Type: keyType})
}

// --- focusLeft navigation ---

func TestCMailCursorDown(t *testing.T) {
	m := screens.NewCMailModel("neuromancer")
	m = m.SetConversations(twoConvs())
	m, _ = sendKey(m, "j")
	if m.SelectedConv() != 1 {
		t.Errorf("expected selectedConv=1 after ↓, got %d", m.SelectedConv())
	}
}

func TestCMailCursorUp_ClampsAtZero(t *testing.T) {
	m := screens.NewCMailModel("neuromancer")
	m = m.SetConversations(twoConvs())
	m, _ = sendKey(m, "k")
	if m.SelectedConv() != 0 {
		t.Errorf("expected selectedConv=0 (clamped), got %d", m.SelectedConv())
	}
}

func TestCMailCursorDown_ClampsAtBottom(t *testing.T) {
	m := screens.NewCMailModel("neuromancer")
	m = m.SetConversations(twoConvs())
	m, _ = sendKey(m, "j")
	m, _ = sendKey(m, "j") // already at bottom
	if m.SelectedConv() != 1 {
		t.Errorf("expected selectedConv=1 (clamped), got %d", m.SelectedConv())
	}
}

// --- Enter in focusLeft opens conversation and shifts to focusRight ---

func TestCMailEnterOpensConversation(t *testing.T) {
	m := screens.NewCMailModel("neuromancer")
	m = m.SetConversations(twoConvs())
	m, _ = sendSpecialKey(m, tea.KeyEnter)
	if !m.HasActiveConv() {
		t.Error("expected activeConv to be set after Enter")
	}
	if m.FocusPane() != screens.FocusCMailRight {
		t.Error("expected focusPane=focusCMailRight after Enter")
	}
	if !m.InputFocused() {
		t.Error("expected input to be focused after Enter")
	}
}

// --- Tab in focusLeft switches to focusRight ---

func TestCMailTabFromLeft_ShiftsToRight(t *testing.T) {
	m := screens.NewCMailModel("neuromancer")
	m = m.SetConversations(twoConvs())
	m, _ = sendSpecialKey(m, tea.KeyTab)
	if m.FocusPane() != screens.FocusCMailRight {
		t.Error("expected focusPane=focusCMailRight after Tab from left")
	}
}

// --- Tab in focusRight switches back to focusLeft ---

func TestCMailTabFromRight_ShiftsToLeft(t *testing.T) {
	m := screens.NewCMailModel("neuromancer")
	m = m.SetConversations(twoConvs())
	// open conversation first
	m, _ = sendSpecialKey(m, tea.KeyEnter)
	// Tab back
	m, _ = sendSpecialKey(m, tea.KeyTab)
	if m.FocusPane() != screens.FocusCMailLeft {
		t.Error("expected focusPane=focusCMailLeft after Tab from right")
	}
	if m.InputFocused() {
		t.Error("expected input to be blurred after Tab from right")
	}
}

// --- Enter in focusRight with non-empty input emits SendCMailMsg ---

func TestCMailSend_EmitsMessage(t *testing.T) {
	m := screens.NewCMailModel("neuromancer")
	m = m.SetConversations(twoConvs())
	m, _ = sendSpecialKey(m, tea.KeyEnter) // open conversation

	// type message via Update with rune messages
	for _, r := range "hello" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	_, cmd := sendSpecialKey(m, tea.KeyEnter)
	if cmd == nil {
		t.Fatal("expected a command to be returned after Enter with text")
	}
	msg := cmd()
	sendMsg, ok := msg.(screens.SendCMailMsg)
	if !ok {
		t.Fatalf("expected SendCMailMsg, got %T", msg)
	}
	if sendMsg.Body != "hello" {
		t.Errorf("expected body='hello', got %q", sendMsg.Body)
	}
	if sendMsg.ConversationID != "c1" {
		t.Errorf("expected conversationID='c1', got %q", sendMsg.ConversationID)
	}
}

// --- Enter with empty body does not emit a command ---

func TestCMailSend_EmptyBodyNoCmd(t *testing.T) {
	m := screens.NewCMailModel("neuromancer")
	m = m.SetConversations(twoConvs())
	m, _ = sendSpecialKey(m, tea.KeyEnter) // open conversation
	// input is empty
	_, cmd := sendSpecialKey(m, tea.KeyEnter)
	if cmd != nil {
		// cmd may be a batch from input/viewport — check it doesn't produce SendCMailMsg
		msg := cmd()
		if _, ok := msg.(screens.SendCMailMsg); ok {
			t.Error("expected no SendCMailMsg for empty body, got one")
		}
	}
}

// --- Esc blurs input when focused, then shifts to focusLeft ---

func TestCMailEsc_BlursInputFirst(t *testing.T) {
	m := screens.NewCMailModel("neuromancer")
	m = m.SetConversations(twoConvs())
	m, _ = sendSpecialKey(m, tea.KeyEnter) // open conversation, input focused

	// first Esc: blur input, stay in focusRight
	m, _ = sendSpecialKey(m, tea.KeyEsc)
	if m.InputFocused() {
		t.Error("expected input to be blurred after first Esc")
	}
	if m.FocusPane() != screens.FocusCMailRight {
		t.Error("expected to stay in focusCMailRight after first Esc")
	}

	// second Esc: shift to focusLeft
	m, _ = sendSpecialKey(m, tea.KeyEsc)
	if m.FocusPane() != screens.FocusCMailLeft {
		t.Error("expected focusCMailLeft after second Esc")
	}
}

// --- WindowSizeMsg sets sidebarWidth correctly ---

func TestCMailSidebarWidth_80(t *testing.T) {
	m := screens.NewCMailModel("neuromancer")
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if m.SidebarWidth() != 20 {
		t.Errorf("expected sidebarWidth=20 at width=80, got %d", m.SidebarWidth())
	}
}

func TestCMailSidebarWidth_120(t *testing.T) {
	m := screens.NewCMailModel("neuromancer")
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	if m.SidebarWidth() != 30 {
		t.Errorf("expected sidebarWidth=30 at width=120, got %d", m.SidebarWidth())
	}
}

func TestCMailSidebarWidth_200(t *testing.T) {
	m := screens.NewCMailModel("neuromancer")
	m, _ = m.Update(tea.WindowSizeMsg{Width: 200, Height: 24})
	if m.SidebarWidth() != 32 {
		t.Errorf("expected sidebarWidth=32 (clamped) at width=200, got %d", m.SidebarWidth())
	}
}
