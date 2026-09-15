package screens

import "strings"

// inputHistoryMax caps how many sent lines the recall buffer keeps per screen.
const inputHistoryMax = 100

// inputHistory is a shell-style recall buffer for a compose input: prev
// (Ctrl+Up) walks back through previously sent lines, next (Ctrl+Down) walks
// forward and finally restores the draft that was in the box when recall
// started. Forward navigation does nothing until the first prev call.
type inputHistory struct {
	entries []string // oldest first
	pos     int      // == len(entries) means "not browsing"
	draft   string   // in-progress text stashed on the first prev
}

// record appends a sent line and ends any in-progress browse. Blank lines and
// an immediate duplicate of the last entry are ignored.
func (h *inputHistory) record(s string) {
	if strings.TrimSpace(s) == "" {
		h.pos = len(h.entries)
		return
	}
	if n := len(h.entries); n == 0 || h.entries[n-1] != s {
		h.entries = append(h.entries, s)
		if len(h.entries) > inputHistoryMax {
			h.entries = h.entries[len(h.entries)-inputHistoryMax:]
		}
	}
	h.pos = len(h.entries)
}

// reset keeps the recorded entries but stops any in-progress browse. Call it
// when the compose input is handed a new conversation/room.
func (h *inputHistory) reset() {
	h.pos = len(h.entries)
	h.draft = ""
}

// prev returns the previous entry to display and true, or "" and false when
// nothing older exists. current is the live input, stashed as the draft on the
// first step back.
func (h *inputHistory) prev(current string) (string, bool) {
	if len(h.entries) == 0 || h.pos == 0 {
		return "", false
	}
	if h.pos == len(h.entries) {
		h.draft = current
	}
	h.pos--
	return h.entries[h.pos], true
}

// next walks forward through history; stepping past the newest entry yields the
// stashed draft once. Returns false when not currently browsing.
func (h *inputHistory) next() (string, bool) {
	if h.pos >= len(h.entries) {
		return "", false
	}
	h.pos++
	if h.pos == len(h.entries) {
		return h.draft, true
	}
	return h.entries[h.pos], true
}
