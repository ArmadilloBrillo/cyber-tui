//go:build windows

package imgview

import (
	"os"
	"testing"
)

// TestTerminalCellPixelSize_DoesNotPanicAndReturnsConsistentResult is a smoke
// test: go test's stdout is rarely a real console screen buffer (usually
// redirected/piped), so this mostly exercises the ok=false path, but the
// invariant — a positive size whenever ok is true, zero otherwise — holds
// either way.
func TestTerminalCellPixelSize_DoesNotPanicAndReturnsConsistentResult(t *testing.T) {
	cellW, cellH, ok := TerminalCellPixelSize(int(os.Stdout.Fd()))
	if ok {
		if cellW <= 0 || cellH <= 0 {
			t.Errorf("ok=true but got non-positive size: cellW=%d cellH=%d", cellW, cellH)
		}
	} else if cellW != 0 || cellH != 0 {
		t.Errorf("ok=false but got non-zero size: cellW=%d cellH=%d", cellW, cellH)
	}
}
