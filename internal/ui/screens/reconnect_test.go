package screens

import (
	"testing"

	"github.com/ragnar/cyber-tui/internal/api"
	"github.com/ragnar/cyber-tui/internal/model"
)

// --- CIRC ---

func TestChatroomsReconnect_StaleEventIgnored(t *testing.T) {
	m := NewChatroomsModel("neo", api.NewMockClient())
	m.activeRoomID = "zion"
	m.mode = chatroomModeDetail

	_, cmd := m.Update(roomStreamClosedMsg{roomID: "old-abandoned-room"})
	if cmd != nil {
		t.Error("expected no reconnect command for a stale stream-closed event")
	}
}

func TestChatroomsReconnect_NotTriggeredOutsideDetailMode(t *testing.T) {
	m := NewChatroomsModel("neo", api.NewMockClient())
	m.activeRoomID = "zion"
	m.mode = chatroomModeList

	_, cmd := m.Update(roomStreamClosedMsg{roomID: "zion"})
	if cmd != nil {
		t.Error("expected no reconnect command when not viewing a room's detail")
	}
}

func TestChatroomsReconnect_SucceedsForActiveRoom(t *testing.T) {
	room := model.Room{ID: "r1", Slug: "zion", Name: "Zion"}
	m := NewChatroomsModel("neo", api.NewMockClient())
	m.activeRoomID = "zion"
	m.activeRoom = &room
	m.mode = chatroomModeDetail

	_, cmd := m.Update(roomStreamClosedMsg{roomID: "zion"})
	if cmd == nil {
		t.Fatal("expected a reconnect command")
	}
	msg := cmd()
	reconnected, ok := msg.(roomReconnectedMsg)
	if !ok {
		t.Fatalf("expected roomReconnectedMsg, got %T", msg)
	}
	if reconnected.sub == nil || reconnected.sub.RoomID != "zion" {
		t.Fatalf("expected reconnected sub for room zion, got %+v", reconnected.sub)
	}

	m2, cmd2 := m.Update(reconnected)
	if m2.sub != reconnected.sub {
		t.Error("expected m.sub to be set to the reconnected subscription")
	}
	if cmd2 == nil {
		t.Error("expected a batch command (resume waiting + reconnected toast) after reconnect")
	}
}

func TestChatroomsReconnect_IgnoresResultForAbandonedRoom(t *testing.T) {
	m := NewChatroomsModel("neo", api.NewMockClient())
	m.activeRoomID = "sprawl" // user navigated to a different room in the meantime
	m.mode = chatroomModeDetail

	sub := &roomSubscription{RoomID: "zion", C: make(chan model.Message), cancel: func() {}}
	m2, cmd := m.Update(roomReconnectedMsg{sub: sub})
	if m2.sub != nil {
		t.Error("expected sub to stay nil when the reconnect result is for an abandoned room")
	}
	// cmd may be non-nil (it cancels the stale sub synchronously already), but must not
	// be the reconnected-toast batch.
	_ = cmd
}

// --- C-Mail ---

func TestCMailReconnect_StaleEventIgnored(t *testing.T) {
	m := NewCMailModel("neo", api.NewMockClient())
	m.activeConvID = "c1"
	m.mode = cmailModeDetail

	_, cmd := m.Update(dmStreamClosedMsg{convID: "old-abandoned-conv"})
	if cmd != nil {
		t.Error("expected no reconnect command for a stale stream-closed event")
	}
}

func TestCMailReconnect_SucceedsForActiveConversation(t *testing.T) {
	conv := model.Conversation{ID: "c1", Participants: []model.User{{Username: "neo"}, {Username: "trinity"}}}
	m := NewCMailModel("neo", api.NewMockClient())
	m.activeConvID = "c1"
	m.activeConv = &conv
	m.mode = cmailModeDetail

	_, cmd := m.Update(dmStreamClosedMsg{convID: "c1"})
	if cmd == nil {
		t.Fatal("expected a reconnect command")
	}
	msg := cmd()
	reconnected, ok := msg.(dmReconnectedMsg)
	if !ok {
		t.Fatalf("expected dmReconnectedMsg, got %T", msg)
	}
	if reconnected.sub == nil || reconnected.sub.ConvID != "c1" {
		t.Fatalf("expected reconnected sub for conv c1, got %+v", reconnected.sub)
	}

	m2, cmd2 := m.Update(reconnected)
	if m2.dmSub != reconnected.sub {
		t.Error("expected m.dmSub to be set to the reconnected subscription")
	}
	if cmd2 == nil {
		t.Error("expected a batch command (resume waiting + reconnected toast) after reconnect")
	}
}
