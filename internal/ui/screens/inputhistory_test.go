package screens

import "testing"

func TestInputHistoryRecallAndDraft(t *testing.T) {
	var h inputHistory

	// Nothing recorded yet: prev/next are no-ops.
	if _, ok := h.prev("typing"); ok {
		t.Fatal("prev on empty history should return false")
	}
	if _, ok := h.next(); ok {
		t.Fatal("next before any prev should return false")
	}

	h.record("first")
	h.record("second")
	h.record("second") // immediate duplicate ignored
	h.record("  ")     // blank ignored
	if len(h.entries) != 2 {
		t.Fatalf("want 2 entries, got %d: %v", len(h.entries), h.entries)
	}

	// Ctrl+Up walks back, stashing the live draft on the first step.
	if v, ok := h.prev("draft"); !ok || v != "second" {
		t.Fatalf("prev #1 = %q,%v; want \"second\",true", v, ok)
	}
	if v, ok := h.prev("draft"); !ok || v != "first" {
		t.Fatalf("prev #2 = %q,%v; want \"first\",true", v, ok)
	}
	if _, ok := h.prev("draft"); ok {
		t.Fatal("prev past oldest should return false")
	}

	// Ctrl+Down walks forward, then yields the stashed draft once.
	if v, ok := h.next(); !ok || v != "second" {
		t.Fatalf("next #1 = %q,%v; want \"second\",true", v, ok)
	}
	if v, ok := h.next(); !ok || v != "draft" {
		t.Fatalf("next #2 = %q,%v; want \"draft\",true", v, ok)
	}
	if _, ok := h.next(); ok {
		t.Fatal("next past newest should return false")
	}
}

func TestInputHistoryCapAndReset(t *testing.T) {
	var h inputHistory
	for i := range inputHistoryMax + 50 {
		h.record(string(rune('a'+i%26)) + "-line")
	}
	if len(h.entries) != inputHistoryMax {
		t.Fatalf("want cap %d, got %d", inputHistoryMax, len(h.entries))
	}

	h.prev("x")
	h.reset()
	if _, ok := h.next(); ok {
		t.Fatal("next after reset should return false")
	}
	if len(h.entries) != inputHistoryMax {
		t.Fatal("reset must not drop recorded entries")
	}
}

func TestCMail_SentHistory_PerConversation(t *testing.T) {
	m := NewCMailModel("me", "", nil)
	m.histFor("c1").record("hello c1")

	if _, ok := m.histFor("c2").prev(""); ok {
		t.Fatal("c2 must not see c1's sent lines")
	}
	if v, ok := m.histFor("c1").prev(""); !ok || v != "hello c1" {
		t.Fatalf("c1 recall = %q,%v; want \"hello c1\",true", v, ok)
	}
}

func TestChatrooms_SentHistory_PerRoom(t *testing.T) {
	m := NewChatroomsModel("me", nil)
	m.histFor("room-a").record("hello a")

	if _, ok := m.histFor("room-b").prev(""); ok {
		t.Fatal("room-b must not see room-a's sent lines")
	}
	if v, ok := m.histFor("room-a").prev(""); !ok || v != "hello a" {
		t.Fatalf("room-a recall = %q,%v; want \"hello a\",true", v, ok)
	}
}
