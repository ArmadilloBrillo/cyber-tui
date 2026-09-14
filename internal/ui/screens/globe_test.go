package screens

import (
	"math"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ragnar/cyber-tui/internal/model"
)

func TestBrailleDotBit(t *testing.T) {
	seen := make(map[byte]bool)
	var all byte
	for subRow := 0; subRow < globeSubRows; subRow++ {
		for subCol := 0; subCol < globeSubCols; subCol++ {
			bit := brailleDotBit(subRow, subCol)
			if bit == 0 || bit&(bit-1) != 0 {
				t.Fatalf("brailleDotBit(%d,%d) = %#x, want a single set bit", subRow, subCol, bit)
			}
			if seen[bit] {
				t.Fatalf("brailleDotBit(%d,%d) = %#x, duplicate bit", subRow, subCol, bit)
			}
			seen[bit] = true
			all |= bit
		}
	}
	if all != 0xFF {
		t.Errorf("all 8 sub-positions OR together to %#x, want 0xff", all)
	}
}

func TestQuadrantBlockBit(t *testing.T) {
	seen := make(map[byte]bool)
	var all byte
	for quadRow := 0; quadRow < 2; quadRow++ {
		for quadCol := 0; quadCol < 2; quadCol++ {
			bit := quadrantBlockBit(quadRow, quadCol)
			if bit == 0 || bit&(bit-1) != 0 {
				t.Fatalf("quadrantBlockBit(%d,%d) = %#x, want a single set bit", quadRow, quadCol, bit)
			}
			if seen[bit] {
				t.Fatalf("quadrantBlockBit(%d,%d) = %#x, duplicate bit", quadRow, quadCol, bit)
			}
			seen[bit] = true
			all |= bit
		}
	}
	if all != 0x0F {
		t.Errorf("all 4 quadrant bits OR together to %#x, want 0x0f", all)
	}
}

func TestQuadrantGlyph(t *testing.T) {
	want := map[byte]rune{
		0: ' ', 1: '▘', 2: '▝', 3: '▀',
		4: '▖', 5: '▌', 6: '▞', 7: '▛',
		8: '▗', 9: '▚', 10: '▐', 11: '▜',
		12: '▄', 13: '▙', 14: '▟', 15: '█',
	}
	seen := make(map[rune]bool)
	for bits := 0; bits < 16; bits++ {
		got := quadrantGlyphs[bits]
		if got != want[byte(bits)] {
			t.Errorf("quadrantGlyphs[%d] = %q, want %q", bits, got, want[byte(bits)])
		}
		if seen[got] {
			t.Errorf("quadrantGlyphs[%d] = %q, duplicate glyph", bits, got)
		}
		seen[got] = true
	}
}

func TestClassifyGlobeCell(t *testing.T) {
	tests := []struct {
		name     string
		okBits   byte
		landBits byte
		wantRune rune
		wantKind globeCellKind
	}{
		{"blank", 0x00, 0x00, ' ', cellBlank},
		// rim: shape from disc membership (sr0,sc0 only -> top-left quadrant on).
		{"rim ocean", brailleDotBit(0, 0), 0x00, '▘', cellOcean},
		{"rim land", brailleDotBit(0, 0), brailleDotBit(0, 0), '▘', cellLand},
		// rim tie (1 land sub-pixel of 2 ok) favors land; shape covers both quadrants touched.
		{"rim tied favors land", brailleDotBit(0, 0) | brailleDotBit(0, 1), brailleDotBit(0, 0), '▀', cellLand},
		// fully inside the disc: shape+color both come from the land/ocean split.
		{"solid interior ocean", 0xFF, 0x00, ' ', cellTerrain},
		{"solid interior land", 0xFF, 0xFF, '█', cellTerrain},
		{"coastal mix", 0xFF, brailleDotBit(0, 0) | brailleDotBit(1, 0) | brailleDotBit(2, 0) | brailleDotBit(0, 1), '▛', cellTerrain},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, kind := classifyGlobeCell(tc.okBits, tc.landBits)
			if kind != tc.wantKind {
				t.Errorf("classifyGlobeCell(%#x,%#x) kind = %v, want %v", tc.okBits, tc.landBits, kind, tc.wantKind)
			}
			if r != tc.wantRune {
				t.Errorf("classifyGlobeCell(%#x,%#x) rune = %q, want %q", tc.okBits, tc.landBits, r, tc.wantRune)
			}
		})
	}
}

// TestViewOrientationNorthAtTop locks in that increasing screen row maps to
// decreasing latitude — the north pole renders near the top of the pane and
// the south pole near the bottom, matching every real map/globe convention.
// (Regression: the first implementation had this inverted.)
func TestViewOrientationNorthAtTop(t *testing.T) {
	m := NewGlobeModel()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 60, Height: 40})
	m = m.SetSelf(model.User{Username: "north", LocationLatitude: 89, LocationLongitude: 0})
	m = m.SetFollowUsernames([]string{"south"})
	m = m.SetProfile("south", model.User{Username: "south", LocationLatitude: -89, LocationLongitude: 0})

	lines := strings.Split(stripANSIForTest(m.View()), "\n")
	if len(lines) != 40 {
		t.Fatalf("got %d lines, want 40", len(lines))
	}

	rowOf := func(glyph byte) int {
		for i, line := range lines {
			if strings.IndexByte(line, glyph) >= 0 {
				return i
			}
		}
		return -1
	}

	northRow, southRow := rowOf('@'), rowOf('*')
	if northRow < 0 || southRow < 0 {
		t.Fatalf("expected to find both markers; north row=%d south row=%d", northRow, southRow)
	}
	if northRow >= 20 {
		t.Errorf("north pole marker at row %d, want it in the top half (<20)", northRow)
	}
	if southRow < 20 {
		t.Errorf("south pole marker at row %d, want it in the bottom half (>=20)", southRow)
	}
}

// TestViewMarkerLabelsUsername locks in that a marker's username renders as
// a label immediately following its glyph, not just the bare glyph alone.
func TestViewMarkerLabelsUsername(t *testing.T) {
	m := NewGlobeModel()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 60, Height: 40})
	m = m.SetSelf(model.User{Username: "ragnar", LocationLatitude: 10, LocationLongitude: 10})

	lines := strings.Split(stripANSIForTest(m.View()), "\n")
	for _, line := range lines {
		if i := strings.IndexByte(line, '@'); i >= 0 && strings.HasPrefix(line[i+1:], "ragnar") {
			return
		}
	}
	t.Error("expected the self marker's username label \"ragnar\" immediately after '@'")
}

func stripANSIForTest(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		if inEsc {
			if r == 'm' {
				inEsc = false
			}
			continue
		}
		if r == '\x1b' {
			inEsc = true
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func TestSphereProjectCenter(t *testing.T) {
	lat, lon, ok := sphereProject(0, 0, 0)
	if !ok {
		t.Fatal("center of the disc should be visible")
	}
	if math.Abs(lat) > 1e-9 || math.Abs(lon) > 1e-9 {
		t.Errorf("lat/lon = %v/%v, want 0/0", lat, lon)
	}
}

func TestSphereProjectOutsideDisc(t *testing.T) {
	if _, _, ok := sphereProject(2, 0, 0); ok {
		t.Error("point outside the unit circle should not be visible")
	}
}

func TestSphereProjectMarkerRoundTrip(t *testing.T) {
	cases := []struct{ lat, lon, angle float64 }{
		{0, 0, 0},
		{30, 45, 0},
		{-60, -120, 1.2},
		{10, 170, 2.5},
	}
	for _, c := range cases {
		dx, dy, ok := markerScreenPos(c.lat, c.lon, c.angle)
		if !ok {
			t.Fatalf("markerScreenPos(%v,%v,%v) not visible, expected front-facing", c.lat, c.lon, c.angle)
		}
		lat, lon, ok := sphereProject(dx, dy, c.angle)
		if !ok {
			t.Fatalf("round-tripped point (%v,%v) fell outside the disc", dx, dy)
		}
		if math.Abs(lat-c.lat) > 1e-6 || math.Abs(lon-c.lon) > 1e-6 {
			t.Errorf("round trip lat/lon = %v/%v, want %v/%v", lat, lon, c.lat, c.lon)
		}
	}
}

func TestMarkerScreenPosFarSide(t *testing.T) {
	// lon=180 at angle 0 is the far side of the globe from a viewer facing lon=0.
	if _, _, ok := markerScreenPos(0, 180, 0); ok {
		t.Error("antipodal point should not be visible (far side)")
	}
}

func TestLandAt(t *testing.T) {
	// 2x2 synthetic fixture: row0=[+90,0), row1=[0,-90]; col0=[-180,0), col1=[0,180).
	fixture := []string{"#.", ".#"}
	tests := []struct {
		lat, lon float64
		want     bool
	}{
		{45, -90, true},   // row0,col0 = '#'
		{45, 90, false},   // row0,col1 = '.'
		{-45, -90, false}, // row1,col0 = '.'
		{-45, 90, true},   // row1,col1 = '#'
		{45, 190, true},   // wraps to lon=-170 -> col0 = '#'
	}
	for _, tc := range tests {
		if got := landAt(tc.lat, tc.lon, fixture, 2, 2); got != tc.want {
			t.Errorf("landAt(%v,%v) = %v, want %v", tc.lat, tc.lon, got, tc.want)
		}
	}
}

func TestGlobeZoomClamps(t *testing.T) {
	m := NewGlobeModel()
	for range 100 {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("+")})
	}
	if m.zoom != globeZoomMax {
		t.Errorf("zoom = %v, want clamped to max %v", m.zoom, globeZoomMax)
	}
	for range 100 {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("-")})
	}
	if m.zoom != globeZoomMin {
		t.Errorf("zoom = %v, want clamped to min %v", m.zoom, globeZoomMin)
	}
}

func TestGlobeToggleKeys(t *testing.T) {
	m := NewGlobeModel()
	if !m.showGuild || !m.showFollows {
		t.Fatal("both marker sets should default on")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	if m.showGuild {
		t.Error("'m' should toggle guild markers off")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	if m.showFollows {
		t.Error("'f' should toggle follow markers off")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if !m.paused {
		t.Error("space should toggle pause on")
	}
}

func TestGlobeSetProfileOnlyKeepsLocated(t *testing.T) {
	m := NewGlobeModel()
	m = m.SetProfile("noloc", model.User{Username: "noloc"})
	m = m.SetProfile("hasloc", model.User{Username: "hasloc", LocationLatitude: 1, LocationLongitude: 2})
	if _, ok := m.profiles["noloc"]; ok {
		t.Error("a profile with no location should not be cached for plotting")
	}
	if _, ok := m.profiles["hasloc"]; !ok {
		t.Error("a profile with a location should be cached")
	}
}

func TestGlobeEnqueueDedup(t *testing.T) {
	m := NewGlobeModel()
	m = m.SetProfile("cached", model.User{Username: "cached", LocationLatitude: 1, LocationLongitude: 1})
	m = m.enqueue([]string{"cached", "a", "a", "b"})
	if len(m.pending) != 2 {
		t.Fatalf("pending = %v, want [a b] (cached excluded, a deduped)", m.pending)
	}
	if m.pending[0] != "a" || m.pending[1] != "b" {
		t.Errorf("pending = %v, want [a b]", m.pending)
	}
	// Re-enqueueing while already pending must not duplicate.
	m = m.enqueue([]string{"a"})
	if len(m.pending) != 2 {
		t.Errorf("pending = %v, want still [a b] after re-enqueueing a pending username", m.pending)
	}
}

func TestGlobeNextPendingAndRequeue(t *testing.T) {
	m := NewGlobeModel()
	m = m.enqueue([]string{"a", "b", "c"})

	username, m, ok := m.NextPending()
	if !ok || username != "a" {
		t.Fatalf("NextPending = %q,%v, want a,true", username, ok)
	}
	if _, queued := m.pendingSet["a"]; queued {
		t.Error("popped username should be removed from pendingSet")
	}

	m = m.Requeue("a")
	username, m, ok = m.NextPending()
	if !ok || username != "a" {
		t.Fatalf("after Requeue, NextPending = %q,%v, want a,true (retry goes to the front)", username, ok)
	}

	username, m, ok = m.NextPending()
	if !ok || username != "b" {
		t.Fatalf("NextPending = %q,%v, want b,true", username, ok)
	}
	username, _, ok = m.NextPending()
	if !ok || username != "c" {
		t.Fatalf("NextPending = %q,%v, want c,true", username, ok)
	}
}

func TestGlobeAdvancePausable(t *testing.T) {
	m := NewGlobeModel()
	m = m.Advance()
	if m.angle != globeAngleStep {
		t.Errorf("angle = %v, want %v after one Advance", m.angle, globeAngleStep)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	before := m.angle
	m = m.Advance()
	if m.angle != before {
		t.Error("Advance should be a no-op while paused")
	}
}
