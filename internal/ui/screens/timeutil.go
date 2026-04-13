package screens

import (
	"fmt"
	"time"
)

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

// formatRelativeTime returns a compact human-readable duration relative to now.
// Falls back to "02-Jan" for anything older than 7 days.
func formatRelativeTime(t, now time.Time, loc *time.Location) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return t.In(loc).Format("02-Jan")
	}
}

// dayLabel returns a human-readable label for the calendar day of t relative to now.
func dayLabel(t, now time.Time, loc *time.Location) string {
	today := now.In(loc).Truncate(24 * time.Hour)
	day := t.In(loc).Truncate(24 * time.Hour)
	switch {
	case day.Equal(today):
		return "today"
	case day.Equal(today.AddDate(0, 0, -1)):
		return "yesterday"
	default:
		return t.In(loc).Format("Mon 2 Jan")
	}
}

// swatchBeats returns the Swatch Internet Time for t as "@NNN" (000–999).
// BMT (Biel Mean Time) = UTC+1.
func swatchBeats(t time.Time) string {
	bmt := t.UTC().Add(time.Hour)
	secs := bmt.Hour()*3600 + bmt.Minute()*60 + bmt.Second()
	beats := (secs * 1000) / 86400
	return fmt.Sprintf("@%03d", beats)
}

// displayTime formats t according to the timeDisplayFormat API setting.
// compact=true omits seconds (used in chat/C-Mail where space is tight).
// Falls back to "datetime" for unknown/empty values.
func displayTime(t time.Time, loc *time.Location, setting string, compact bool) string {
	switch setting {
	case "relative":
		return formatRelativeTime(t, time.Now(), loc)
	case "unix":
		return fmt.Sprintf("%d", t.Unix())
	case "swatch":
		return swatchBeats(t)
	default: // "datetime" or empty
		tf := "15:04:05"
		if compact {
			tf = "15:04"
		}
		return formatTime(t, loc, tf)
	}
}
