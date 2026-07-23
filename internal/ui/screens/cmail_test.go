package screens

import (
	"testing"

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
