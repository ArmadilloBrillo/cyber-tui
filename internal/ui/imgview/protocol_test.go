package imgview_test

import (
	"testing"

	"github.com/ragnar/cyber-tui/internal/ui/imgview"
)

func TestParseDA1SixelSupport(t *testing.T) {
	tests := []struct {
		name string
		resp []byte
		want bool
	}{
		{"has attr 4", []byte("\x1b[?62;1;2;4;6;9;15;18;21;22c"), true},
		{"no attr 4", []byte("\x1b[?1;2c"), false},
		{"empty", []byte(""), false},
		{"garbage", []byte("garbage"), false},
		{"missing terminator", []byte("\x1b[?4"), false},
		{"stray leading bytes", []byte("xy\x1b[?4;6c"), true},
		{"attr 4 alone", []byte("\x1b[?4c"), true},
		{"non-numeric field", []byte("\x1b[?4;abc;6c"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := imgview.ParseDA1SixelSupport(tt.resp); got != tt.want {
				t.Errorf("ParseDA1SixelSupport(%q) = %v, want %v", tt.resp, got, tt.want)
			}
		})
	}
}
