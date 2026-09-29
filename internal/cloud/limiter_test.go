package cloud

import (
	"testing"
	"time"
)

func TestTheLimiterAllowsSoManyInAWindowAndThenRefusesUntilTheyAgeOut(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	l := NewLimiter(3, time.Hour)
	l.now = func() time.Time { return now }
	for i := range 3 {
		if !l.Allow("a") {
			t.Fatalf("event %d should be allowed", i)
		}
		now = now.Add(time.Minute)
	}
	if l.Allow("a") {
		t.Fatal("the fourth in an hour should be refused")
	}
	if !l.Allow("b") {
		t.Error("another key has its own count")
	}
	// A refused event is not counted. At 61 minutes the events of minute 0 and
	// minute 1 have left the window and the one of minute 2 has not, so two more fit.
	now = now.Add(58 * time.Minute)
	if !l.Allow("a") || !l.Allow("a") {
		t.Error("two places have opened in the window")
	}
	if l.Allow("a") {
		t.Error("the window holds three again")
	}
}
