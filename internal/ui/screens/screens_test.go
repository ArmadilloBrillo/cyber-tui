package screens_test

import (
	"testing"

	"github.com/ragnar/cyber-tui/internal/ui/screens"
)

// --- ChatroomsModel.InputFocused ---

func TestChatroomsInputFocused_DefaultFalse(t *testing.T) {
	m := screens.NewChatroomsModel()
	if m.InputFocused() {
		t.Error("input should not be focused on a freshly created ChatroomsModel")
	}
}

// --- DMsModel.InputFocused ---

func TestDMsInputFocused_DefaultFalse(t *testing.T) {
	m := screens.NewDMsModel("neuromancer")
	if m.InputFocused() {
		t.Error("input should not be focused on a freshly created DMsModel")
	}
}
