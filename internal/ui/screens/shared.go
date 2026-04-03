package screens

import "time"

// SharedConfigMsg is broadcast by App whenever display-affecting settings change
// (dimensions, timezone, display density). Each screen handles the fields it cares
// about in its own Update; fields it doesn't use are ignored.
//
// Adding a new screen only requires handling this message in that screen's Update —
// no App call sites need changing.
type SharedConfigMsg struct {
	Width   int
	Height  int
	Loc     *time.Location
	Relaxed bool
}
