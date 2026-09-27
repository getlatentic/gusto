package spend

import (
	"math"
	"testing"
	"time"
)

// deepseekFlash is DeepSeek's published price for deepseek-flash, read from
// api-docs.deepseek.com/quick_start/pricing on 2026-09-27: the off-peak rate,
// doubled on weekdays from 01:00 to 04:00 and 06:00 to 10:00 UTC.
func deepseekFlash() Price {
	cached := 0.003
	return Price{
		InputPerMillion: 0.15, CachedInputPerMillion: &cached, OutputPerMillion: 0.6,
		Peak: &Peak{Multiplier: 2, Weekdays: []time.Weekday{1, 2, 3, 4, 5}, HoursUTC: [][2]int{{1, 4}, {6, 10}},
			Except: []string{"2026-10-01"}},
	}
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-12 }

func TestCachedInputIsBilledAtItsOwnRate(t *testing.T) {
	offPeak := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC) // a Sunday
	got := deepseekFlash().of(Usage{Input: 1_000_000, Cached: 800_000, Output: 100_000}, offPeak)
	if want := 0.2*0.15 + 0.8*0.003 + 0.1*0.6; !near(got, want) {
		t.Errorf("cost %v, want %v", got, want)
	}
}

func TestPeakHoursCostTheMultiplier(t *testing.T) {
	usage := Usage{Input: 1_000_000, Output: 0}
	for _, tc := range []struct {
		at   time.Time
		want float64
	}{
		{time.Date(2026, 9, 28, 7, 30, 0, 0, time.UTC), 0.30}, // Monday, peak
		{time.Date(2026, 9, 28, 4, 0, 0, 0, time.UTC), 0.15},  // Monday, the hour a window ends
		{time.Date(2026, 9, 27, 7, 30, 0, 0, time.UTC), 0.15}, // Sunday
		{time.Date(2026, 10, 1, 7, 30, 0, 0, time.UTC), 0.15}, // a Thursday the provider excepts
	} {
		if got := deepseekFlash().of(usage, tc.at); !near(got, tc.want) {
			t.Errorf("at %s: cost %v, want %v", tc.at, got, tc.want)
		}
	}
}

func TestAPriceNamedWithItsCredentialComesFirst(t *testing.T) {
	policy := Policy{Prices: map[string]Price{
		"deepseek-flash":           {InputPerMillion: 1},
		"anthropic/deepseek-flash": {InputPerMillion: 2},
	}}
	usage := &Usage{Input: 1_000_000}
	if got := *policy.Cost("anthropic", "deepseek-flash", usage, time.Time{}); got != 2 {
		t.Errorf("through its own credential: %v", got)
	}
	if got := *policy.Cost("deepseek", "deepseek-flash", usage, time.Time{}); got != 1 {
		t.Errorf("through another: %v", got)
	}
}
