package cloud

import (
	"fmt"
	"time"
)

// minutes is a time in words a person reads, to the minute, and never as less than one.
func minutes(d time.Duration) string {
	n := int((d + 30*time.Second) / time.Minute)
	switch {
	case n <= 1:
		return "a minute"
	case n < 60:
		return fmt.Sprintf("%d minutes", n)
	}
	hours, rest := n/60, n%60
	if rest == 0 {
		return fmt.Sprintf("%d hours", hours)
	}
	return fmt.Sprintf("%d h %d min", hours, rest)
}

// clock is a time of day, in UTC, which is what the host's own log uses.
func clock(t time.Time) string { return t.UTC().Format("15:04") + " UTC" }

// minuteLong is a length as an adjective: "30-minute".
func minuteLong(d time.Duration) string {
	return fmt.Sprintf("%d-minute", int((d+30*time.Second)/time.Minute))
}
