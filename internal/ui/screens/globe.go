package screens

import (
	"math"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ragnar/cyber-tui/internal/model"
	"github.com/ragnar/cyber-tui/internal/ui/theme"
)

// globeSubCols/globeSubRows subdivide each terminal character cell into a
// braille dot matrix (Unicode Braille Patterns, U+2800 base) for 8x the
// addressable resolution of one character per cell.
const globeSubCols = 2
const globeSubRows = 4

// globeSubCellAspect is a tunable calibration constant — terminal font
// metrics vary. It's ~1.0 (square) here, not the ~0.5 a whole-character-cell
// grid would need: braille's 2-wide x 4-tall subdivision of a character cell
// that's itself roughly 1-wide x 2-tall produces sub-pixels that are
// approximately square, which is the whole point of sampling this way.
const globeSubCellAspect = 1.0

// globeAngleStep is how far the globe spins per Advance call (radians).
const globeAngleStep = 0.02

// globeZoomMin/Max clamp the zoom level toggled by '+'/'-'.
const globeZoomMin = 0.5
const globeZoomMax = 2.5
const globeZoomStep = 0.1

type globeCellKind byte

const (
	cellBlank globeCellKind = iota
	cellOcean
	cellLand
	cellFollow
	cellGuild
	cellSelf
)

func globeCellStyle(k globeCellKind) lipgloss.Style {
	switch k {
	case cellLand:
		return lipgloss.NewStyle().Foreground(theme.ColorGreen)
	case cellFollow:
		return lipgloss.NewStyle().Foreground(theme.ColorMeta).Bold(true)
	case cellGuild:
		return lipgloss.NewStyle().Foreground(theme.ColorCyan).Bold(true)
	case cellSelf:
		return lipgloss.NewStyle().Foreground(theme.ColorYellow).Bold(true)
	default:
		return theme.Subtle
	}
}

// brailleDotBit returns the Unicode Braille Patterns bit for sub-position
// (subRow 0-3, subCol 0-1) within one character cell, per the standard
// braille dot numbering:
//
//	1 4
//	2 5
//	3 6
//	7 8
func brailleDotBit(subRow, subCol int) byte {
	if subCol == 0 {
		switch subRow {
		case 0:
			return 1 << 0 // dot 1
		case 1:
			return 1 << 1 // dot 2
		case 2:
			return 1 << 2 // dot 3
		default:
			return 1 << 6 // dot 7
		}
	}
	switch subRow {
	case 0:
		return 1 << 3 // dot 4
	case 1:
		return 1 << 4 // dot 5
	case 2:
		return 1 << 5 // dot 6
	default:
		return 1 << 7 // dot 8
	}
}

const brailleBase = 0x2800

// hasLocation reports whether u has a coordinate set — mirrors the
// zero-value check profile.go already uses to decide whether to render a
// user's "City name (lat, lon)" line.
func hasLocation(u model.User) bool {
	return u.LocationLatitude != 0 || u.LocationLongitude != 0
}

// GlobeModel renders a rotating ASCII globe plotting the caller's own
// location plus their guild members' and follow network's locations —
// see docs/55-globe.md. App owns all API calls; this is a pure state
// machine (current rotation/zoom, which marker sets are toggled on, the
// resolved-profile cache, and the paced fetch queue for profiles not yet
// resolved).
type GlobeModel struct {
	width, height int

	angle  float64 // radians; advanced by App's tick while this screen is active
	zoom   float64
	paused bool

	showGuild   bool
	showFollows bool

	self    model.User
	hasSelf bool

	guildUsernames  []string
	guildLoaded     bool
	followUsernames []string
	followsLoaded   bool

	// profiles caches resolved locations for the session, keyed by username —
	// only entries with hasLocation are kept (see SetProfile), since a
	// profile with no location is never drawn and there's nothing to retry.
	profiles map[string]model.User

	// pending/pendingSet is the FIFO queue of usernames still awaiting a
	// profile fetch, paced by App well under the API's 30/min rate limit —
	// see loadGlobeProfileCmd/globeFetchTickMsg in app.go.
	pending    []string
	pendingSet map[string]struct{}
	fetching   bool

	err error
}

func NewGlobeModel() GlobeModel {
	return GlobeModel{
		zoom:        1,
		showGuild:   true,
		showFollows: true,
		profiles:    make(map[string]model.User),
		pendingSet:  make(map[string]struct{}),
	}
}

func (m GlobeModel) SetSelf(u model.User) GlobeModel {
	m.self = u
	m.hasSelf = true
	return m
}

func (m GlobeModel) SetError(err error) GlobeModel {
	m.err = err
	return m
}

// enqueue adds any usernames not already cached or already queued to the
// pending fetch queue.
func (m GlobeModel) enqueue(usernames []string) GlobeModel {
	for _, u := range usernames {
		if _, have := m.profiles[u]; have {
			continue
		}
		if _, queued := m.pendingSet[u]; queued {
			continue
		}
		m.pending = append(m.pending, u)
		m.pendingSet[u] = struct{}{}
	}
	return m
}

// SetGuildMembers records the caller's guild's member usernames (excluding
// the caller) and enqueues any not yet resolved for a profile fetch. Pass
// nil when the caller isn't in a guild.
func (m GlobeModel) SetGuildMembers(usernames []string) GlobeModel {
	m.guildLoaded = true
	m.guildUsernames = usernames
	return m.enqueue(usernames)
}

// SetFollowUsernames records the caller's combined follow-network usernames
// (following + followers, deduplicated) and enqueues any not yet resolved.
func (m GlobeModel) SetFollowUsernames(usernames []string) GlobeModel {
	m.followsLoaded = true
	m.followUsernames = usernames
	return m.enqueue(usernames)
}

func (m GlobeModel) GuildLoaded() bool   { return m.guildLoaded }
func (m GlobeModel) FollowsLoaded() bool { return m.followsLoaded }
func (m GlobeModel) HasPending() bool    { return len(m.pending) > 0 }
func (m GlobeModel) IsFetching() bool    { return m.fetching }

func (m GlobeModel) SetFetching(v bool) GlobeModel {
	m.fetching = v
	return m
}

// NextPending pops the next queued username for a profile fetch. ok is
// false when the queue is empty.
func (m GlobeModel) NextPending() (username string, next GlobeModel, ok bool) {
	if len(m.pending) == 0 {
		return "", m, false
	}
	username = m.pending[0]
	m.pending = m.pending[1:]
	delete(m.pendingSet, username)
	return username, m, true
}

// Requeue pushes username back to the front of the pending queue — used to
// retry after a 429 without losing its place behind usernames queued later.
func (m GlobeModel) Requeue(username string) GlobeModel {
	if _, queued := m.pendingSet[username]; queued {
		return m
	}
	m.pending = append([]string{username}, m.pending...)
	m.pendingSet[username] = struct{}{}
	return m
}

// SetProfile records a resolved profile, but only when it has a location to
// plot — a profile with none is never drawn and there's nothing to retry.
func (m GlobeModel) SetProfile(username string, u model.User) GlobeModel {
	if hasLocation(u) {
		m.profiles[username] = u
	}
	return m
}

func (m GlobeModel) guildMarkers() []model.User  { return m.resolvedProfiles(m.guildUsernames) }
func (m GlobeModel) followMarkers() []model.User { return m.resolvedProfiles(m.followUsernames) }

func (m GlobeModel) resolvedProfiles(usernames []string) []model.User {
	var out []model.User
	for _, u := range usernames {
		if p, ok := m.profiles[u]; ok {
			out = append(out, p)
		}
	}
	return out
}

// Advance spins the globe one tick (no-op while paused).
func (m GlobeModel) Advance() GlobeModel {
	if m.paused {
		return m
	}
	m.angle += globeAngleStep
	if m.angle > 2*math.Pi {
		m.angle -= 2 * math.Pi
	}
	return m
}

func (m GlobeModel) Init() tea.Cmd { return nil }

func (m GlobeModel) Update(msg tea.Msg) (GlobeModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "+", "=":
			m.zoom = min(globeZoomMax, m.zoom+globeZoomStep)
		case "-", "_":
			m.zoom = max(globeZoomMin, m.zoom-globeZoomStep)
		case "m":
			m.showGuild = !m.showGuild
		case "f":
			m.showFollows = !m.showFollows
		case " ":
			m.paused = !m.paused
		}
		return m, nil
	}
	return m, nil
}

// radii returns the sphere's horizontal/vertical radius in sub-pixels
// (globeSubCols x globeSubRows per character cell), scaled by zoom and sized
// to fit within the pane without clipping.
func (m GlobeModel) radii() (rx, ry float64) {
	rx = float64(m.width*globeSubCols) / 2
	ry = rx * globeSubCellAspect
	if maxRY := float64(m.height*globeSubRows) / 2; ry > maxRY {
		ry = maxRY
		rx = ry / globeSubCellAspect
	}
	return rx * m.zoom, ry * m.zoom
}

func (m GlobeModel) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	rx, ry := m.radii()
	if rx <= 0 || ry <= 0 {
		return ""
	}
	subCx, subCy := float64(m.width*globeSubCols)/2, float64(m.height*globeSubRows)/2

	runes := make([][]rune, m.height)
	kinds := make([][]globeCellKind, m.height)
	for row := range runes {
		runes[row] = make([]rune, m.width)
		kinds[row] = make([]globeCellKind, m.width)
		for col := range runes[row] {
			var bits byte
			var land, ocean int
			for sr := 0; sr < globeSubRows; sr++ {
				for sc := 0; sc < globeSubCols; sc++ {
					subCol := col*globeSubCols + sc
					subRow := row*globeSubRows + sr
					dx := (float64(subCol) - subCx) / rx
					// Screen rows increase downward but sphere-space y (and
					// thus latitude) increases northward/upward — negate to
					// keep the north pole at the top of the pane.
					dy := (subCy - float64(subRow)) / ry
					lat, lon, ok := sphereProject(dx, dy, m.angle)
					if !ok {
						continue
					}
					bits |= brailleDotBit(sr, sc)
					if landAt(lat, lon, globeLandmask[:], globeMapWidth, globeMapHeight) {
						land++
					} else {
						ocean++
					}
				}
			}
			if bits == 0 {
				runes[row][col] = ' '
				kinds[row][col] = cellBlank
				continue
			}
			runes[row][col] = rune(brailleBase + int(bits))
			if land >= ocean {
				kinds[row][col] = cellLand
			} else {
				kinds[row][col] = cellOcean
			}
		}
	}

	plot := func(u model.User, glyph rune, kind globeCellKind) {
		dx, dy, ok := markerScreenPos(u.LocationLatitude, u.LocationLongitude, m.angle)
		if !ok {
			return
		}
		// Same rx/ry/subCx/subCy as the terrain loop, so a marker lands on
		// the same sub-pixel position its coastline would, then collapses
		// to the character cell that sub-pixel belongs to.
		subCol := int(math.Round(subCx + dx*rx))
		subRow := int(math.Round(subCy - dy*ry))
		col, row := subCol/globeSubCols, subRow/globeSubRows
		if row >= 0 && row < m.height && col >= 0 && col < m.width {
			runes[row][col] = glyph
			kinds[row][col] = kind
		}
	}

	// Draw lowest-priority markers first so self always wins a shared cell.
	if m.showFollows {
		for _, u := range m.followMarkers() {
			plot(u, '*', cellFollow)
		}
	}
	if m.showGuild {
		for _, u := range m.guildMarkers() {
			plot(u, 'o', cellGuild)
		}
	}
	if m.hasSelf && hasLocation(m.self) {
		plot(m.self, '@', cellSelf)
	}

	lines := make([]string, m.height)
	for row := 0; row < m.height; row++ {
		var b strings.Builder
		col := 0
		for col < m.width {
			j := col + 1
			for j < m.width && kinds[row][j] == kinds[row][col] {
				j++
			}
			b.WriteString(globeCellStyle(kinds[row][col]).Render(string(runes[row][col:j])))
			col = j
		}
		lines[row] = b.String()
	}
	return strings.Join(lines, "\n")
}

// sphereProject inverse-projects a normalized, aspect-corrected screen
// offset (dx, dy, each roughly in [-1,1]) through a sphere rotated by
// angleRad (radians, increasing = spinning eastward) back to the globe's own
// unrotated lat/lon in degrees. ok is false outside the visible unit circle.
func sphereProject(dx, dy, angleRad float64) (lat, lon float64, ok bool) {
	d2 := dx*dx + dy*dy
	if d2 > 1 {
		return 0, 0, false
	}
	nz := math.Sqrt(1 - d2)
	sin, cos := math.Sin(angleRad), math.Cos(angleRad)
	gx := dx*cos - nz*sin
	gy := dy
	gz := dx*sin + nz*cos
	lat = math.Asin(min(1, max(-1, gy))) * 180 / math.Pi
	lon = math.Atan2(gx, gz) * 180 / math.Pi
	return lat, lon, true
}

// markerScreenPos projects a globe location (lat/lon degrees) into the same
// normalized (dx, dy) space sphereProject reads from, for the current
// rotation angle. ok is false when the point is on the far side of the globe.
func markerScreenPos(lat, lon, angleRad float64) (dx, dy float64, ok bool) {
	latR, lonR := lat*math.Pi/180, lon*math.Pi/180
	gx := math.Cos(latR) * math.Sin(lonR)
	gy := math.Sin(latR)
	gz := math.Cos(latR) * math.Cos(lonR)
	sin, cos := math.Sin(angleRad), math.Cos(angleRad)
	vx := gx*cos + gz*sin
	vz := -gx*sin + gz*cos
	if vz < 0 {
		return 0, 0, false
	}
	return vx, gy, true
}

// landAt reports whether (lat, lon) falls on a land cell of bitmap, a w x h
// equirectangular grid (row 0 = +90 lat, column 0 = -180 lon). Takes the
// bitmap as a parameter (rather than reading globeLandmask directly) so
// tests can exercise it against a small synthetic fixture.
func landAt(lat, lon float64, bitmap []string, w, h int) bool {
	lon = math.Mod(lon+180, 360)
	if lon < 0 {
		lon += 360
	}
	lon -= 180
	row := int((90 - lat) / 180 * float64(h))
	col := int((lon + 180) / 360 * float64(w))
	row = max(0, min(h-1, row))
	col = max(0, min(w-1, col))
	return bitmap[row][col] == '#'
}
