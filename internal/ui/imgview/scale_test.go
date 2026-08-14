package imgview_test

import (
	"testing"

	"github.com/ragnar/cyber-tui/internal/ui/imgview"
)

func TestEffectiveCellPx_FallsBackWhenUnavailable(t *testing.T) {
	w, h := imgview.EffectiveCellPx(0, 0)
	if w <= 0 || h <= 0 {
		t.Errorf("EffectiveCellPx(0, 0) = %d, %d, want a positive default", w, h)
	}
	if h != 2*w {
		t.Errorf("EffectiveCellPx(0, 0) = %d, %d, want a 2:1 height:width default", w, h)
	}
}

func TestEffectiveCellPx_PassesThroughRealSize(t *testing.T) {
	w, h := imgview.EffectiveCellPx(16, 36)
	if w != 16 || h != 36 {
		t.Errorf("EffectiveCellPx(16, 36) = %d, %d, want unchanged", w, h)
	}
}

func TestNativeCellBox_CeilDivides(t *testing.T) {
	// 773x512px at 16x36px cells: 773/16 = 48.3 -> 49 cols, 512/36 = 14.2 -> 15 rows.
	cols, rows := imgview.NativeCellBox(773, 512, 16, 36)
	if cols != 49 {
		t.Errorf("NativeCellBox: cols=%d, want 49", cols)
	}
	if rows != 15 {
		t.Errorf("NativeCellBox: rows=%d, want 15", rows)
	}
}

func TestNativeCellBox_FallsBackToDefaultCellSize(t *testing.T) {
	// cellPxW/cellPxH <= 0 should use the same default as EffectiveCellPx,
	// not divide by zero.
	cols, rows := imgview.NativeCellBox(100, 100, 0, 0)
	if cols < 1 || rows < 1 {
		t.Errorf("NativeCellBox with unavailable cell size = %d, %d, want positive", cols, rows)
	}
}

func TestNativeCellBox_AlwaysAtLeastOneCell(t *testing.T) {
	cols, rows := imgview.NativeCellBox(1, 1, 100, 100)
	if cols != 1 || rows != 1 {
		t.Errorf("NativeCellBox(1, 1, 100, 100) = %d, %d, want 1, 1", cols, rows)
	}
}
