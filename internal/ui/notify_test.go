package ui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ragnar/cyber-tui/internal/model"
)

func TestNotify_SetsTextAndReturnsTick(t *testing.T) {
	a := loggedInApp()
	a, cmd, claimed := a.handleNotify(actionErrMsg{errors.New("not a member")})
	if !claimed {
		t.Fatal("expected handleNotify to claim actionErrMsg")
	}
	if a.notifyText != "not a member" {
		t.Errorf("notifyText = %q, want %q", a.notifyText, "not a member")
	}
	if a.notifyLevel != notifyError {
		t.Errorf("notifyLevel = %d, want notifyError", a.notifyLevel)
	}
	if a.notifyGen != 1 {
		t.Errorf("notifyGen = %d, want 1", a.notifyGen)
	}
	// A non-nil command is the auto-dismiss tick; we don't invoke it here because
	// tea.Tick blocks for the full TTL. Auto-expire is covered by the tests below
	// that feed notifyExpireMsg directly.
	if cmd == nil {
		t.Fatal("expected a non-nil expire command")
	}
}

func TestNotify_AutoExpireClears(t *testing.T) {
	a := loggedInApp()
	a, _, _ = a.handleNotify(actionErrMsg{errors.New("boom")})
	a, _, _ = a.handleNotify(notifyExpireMsg{gen: 1})
	if a.notifyText != "" {
		t.Errorf("notifyText = %q, want empty after expire", a.notifyText)
	}
}

func TestNotify_StaleExpireDoesNotClearNewer(t *testing.T) {
	a := loggedInApp()
	a, _, _ = a.handleNotify(actionErrMsg{errors.New("first")})
	a, _, _ = a.handleNotify(actionErrMsg{errors.New("second")})
	// Fire the stale expire belonging to the first notification.
	a, _, _ = a.handleNotify(notifyExpireMsg{gen: 1})
	if a.notifyText != "second" {
		t.Errorf("notifyText = %q, want %q (stale expire must not clear newer)", a.notifyText, "second")
	}
}

func TestNotify_KeypressDismissesButKeyStillActs(t *testing.T) {
	a := loggedInApp()
	a, _, _ = a.handleNotify(actionErrMsg{errors.New("boom")})
	if a.notifyText == "" {
		t.Fatal("precondition: notification should be visible")
	}
	next, _ := a.Update(keyMsg("?"))
	a2 := next.(App)
	if a2.notifyText != "" {
		t.Errorf("notifyText = %q, want empty after keypress", a2.notifyText)
	}
	if !a2.helpModalOpen {
		t.Error("expected '?' to still open the help modal (key not swallowed)")
	}
	if a2.notifyGen != 2 {
		t.Errorf("notifyGen = %d, want 2 (bumped to invalidate pending tick)", a2.notifyGen)
	}
}

func TestNotify_ExpireAfterKeypressIsNoOp(t *testing.T) {
	a := loggedInApp()
	a, _, _ = a.handleNotify(actionErrMsg{errors.New("boom")}) // gen 1
	next, _ := a.Update(keyMsg("?"))                           // dismiss, gen -> 2
	a = next.(App)
	a, _, _ = a.handleNotify(notifyExpireMsg{gen: 1}) // stale
	if a.notifyText != "" {
		t.Errorf("notifyText = %q, want empty", a.notifyText)
	}
}

func TestActionErrMsg_DoesNotBlankScreen(t *testing.T) {
	a := loggedInApp()
	a.active = screenGuilds
	next, _ := a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	a = next.(App)
	a.guilds = a.guilds.SetGuildPosts([]model.Post{{ID: "1", Content: "hello world"}}, "")

	next, _ = a.Update(actionErrMsg{errors.New("not a member")})
	a = next.(App)

	if a.notifyText != "not a member" {
		t.Errorf("notifyText = %q, want %q", a.notifyText, "not a member")
	}
	if strings.Contains(a.guilds.View(), "guilds error") {
		t.Error("guilds screen was blanked by an action error; content should remain")
	}
	if !strings.Contains(a.View(), "not a member") {
		t.Error("app view should show the action error in the bottom bar")
	}
}
