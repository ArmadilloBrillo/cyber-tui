package screens

import "time"

// formatTime formats t in loc with the following rule:
//   - same calendar day in loc → timeFormat only (e.g. "15:04:05")
//   - any other day           → "02-Jan-2006 " + timeFormat
func formatTime(t time.Time, loc *time.Location, timeFormat string) string {
	local := t.In(loc)
	now := time.Now().In(loc)
	if local.Year() == now.Year() && local.YearDay() == now.YearDay() {
		return local.Format(timeFormat)
	}
	return local.Format("02-Jan-2006 " + timeFormat)
}
