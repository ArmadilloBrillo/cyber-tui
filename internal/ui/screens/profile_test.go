package screens_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/screens"
)

func testUser() model.User {
	return model.User{
		ID:             "uid-123",
		Username:       "ragnar",
		Bio:            "test bio",
		FollowersCount: 35,
		FollowingCount: 45,
		PostsCount:     6,
	}
}

// --- SetFollowState ---

func TestProfileSetFollowState_True(t *testing.T) {
	m := screens.NewProfileModel().SetUser(testUser()).SetReadOnly(true)
	m = m.SetFollowState(true, "follow-id-abc")

	view := m.View()
	if !strings.Contains(view, "unfollow") {
		t.Errorf("View should contain 'unfollow' when following, got:\n%s", view)
	}
}

func TestProfileSetFollowState_False(t *testing.T) {
	m := screens.NewProfileModel().SetUser(testUser()).SetReadOnly(true)
	m = m.SetFollowState(false, "")

	view := m.View()
	if !strings.Contains(view, "f · follow") {
		t.Errorf("View should contain 'f · follow' when not following, got:\n%s", view)
	}
}

// --- counts display ---

func TestProfileView_CountsDisplayed(t *testing.T) {
	m := screens.NewProfileModel().SetUser(testUser())
	view := m.View()

	if !strings.Contains(view, "35 followers") {
		t.Errorf("View should contain '35 followers', got:\n%s", view)
	}
	if !strings.Contains(view, "45 following") {
		t.Errorf("View should contain '45 following', got:\n%s", view)
	}
	if !strings.Contains(view, "6 posts") {
		t.Errorf("View should contain '6 posts', got:\n%s", view)
	}
}

// --- 'f' key — follow ---

func TestProfileUpdate_FKey_EmitsFollowMsg_WhenNotFollowing(t *testing.T) {
	m := screens.NewProfileModel().SetUser(testUser()).SetReadOnly(true).SetFollowState(false, "")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	_ = updated
	if cmd == nil {
		t.Fatal("expected a cmd, got nil")
	}
	msg := cmd()
	followMsg, ok := msg.(screens.FollowUserMsg)
	if !ok {
		t.Fatalf("expected FollowUserMsg, got %T", msg)
	}
	if followMsg.UserID != "uid-123" {
		t.Errorf("UserID = %q, want uid-123", followMsg.UserID)
	}
}

// --- 'f' key — unfollow ---

func TestProfileUpdate_FKey_EmitsUnfollowMsg_WhenFollowing(t *testing.T) {
	m := screens.NewProfileModel().SetUser(testUser()).SetReadOnly(true).SetFollowState(true, "follow-id-abc")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	_ = updated
	if cmd == nil {
		t.Fatal("expected a cmd, got nil")
	}
	msg := cmd()
	unfollowMsg, ok := msg.(screens.UnfollowUserMsg)
	if !ok {
		t.Fatalf("expected UnfollowUserMsg, got %T", msg)
	}
	if unfollowMsg.FollowID != "follow-id-abc" {
		t.Errorf("FollowID = %q, want follow-id-abc", unfollowMsg.FollowID)
	}
}

// --- 'f' key ignored on own profile ---

func TestProfileUpdate_FKey_NoEffect_OnOwnProfile(t *testing.T) {
	// readOnly=false means it's the user's own profile — f should do nothing
	m := screens.NewProfileModel().SetUser(testUser()).SetReadOnly(false)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	if cmd != nil {
		t.Errorf("expected nil cmd on own profile, got non-nil")
	}
}

// --- IncrementFollowersCount ---

func TestProfileIncrementFollowersCount(t *testing.T) {
	m := screens.NewProfileModel().SetUser(testUser()) // FollowersCount = 35
	m = m.IncrementFollowersCount(1)

	view := m.View()
	if !strings.Contains(view, "36 followers") {
		t.Errorf("View should contain '36 followers' after increment, got:\n%s", view)
	}

	m = m.IncrementFollowersCount(-1)
	view = m.View()
	if !strings.Contains(view, "35 followers") {
		t.Errorf("View should contain '35 followers' after decrement, got:\n%s", view)
	}
}

// --- follow feedback ---

func TestProfileSetFollowFeedback(t *testing.T) {
	m := screens.NewProfileModel().SetUser(testUser()).SetFollowFeedback("following.")

	view := m.View()
	if !strings.Contains(view, "following.") {
		t.Errorf("View should contain 'following.', got:\n%s", view)
	}
}

// --- hint bar shows no follow button on own profile ---

func TestProfileView_NoFollowHint_OwnProfile(t *testing.T) {
	m := screens.NewProfileModel().SetUser(testUser()).SetReadOnly(false)
	view := m.View()

	if strings.Contains(view, "f · follow") {
		t.Errorf("View should not contain 'f · follow' on own profile, got:\n%s", view)
	}
}
