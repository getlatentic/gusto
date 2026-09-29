package compare

import (
	"strings"
	"testing"
)

func usd(v float64) *float64 { return &v }

func TestWhatAComparisonSays(t *testing.T) {
	for name, c := range map[string]struct {
		results           []Result
		fastest, cheapest int
		says              []string
		notSays           []string
	}{
		"nobody passed": {
			results: []Result{{Name: "A"}, {Name: "B"}}, fastest: -1, cheapest: -1,
			says: []string{"None of the 2 passed the check."},
		},
		"one passed, however slow or dear": {
			results: []Result{{Name: "A", Passed: true, Seconds: 300, USD: usd(9)}, {Name: "B", Seconds: 1, USD: usd(0.01)}},
			fastest: -1, cheapest: -1,
			says: []string{"A was the only one to pass the check."},
		},
		"both passed and one is clearly faster and cheaper": {
			results: []Result{{Name: "A", Passed: true, Seconds: 60, USD: usd(0.50)}, {Name: "B", Passed: true, Seconds: 120, USD: usd(0.20)}},
			fastest: 0, cheapest: 1,
			says: []string{"All 2 passed the check.", "A was the fastest, at 60 s against 120 s.", "B was the cheapest, at $0.2000 against $0.5000."},
		},
		"a failed run is not fast or cheap": {
			results: []Result{{Name: "A", Passed: true, Seconds: 200, USD: usd(1)}, {Name: "B", Seconds: 10, USD: usd(0.01)},
				{Name: "C", Passed: true, Seconds: 100, USD: usd(2)}},
			fastest: 2, cheapest: 0,
			says: []string{"2 of 3 passed the check.", "C was the fastest, at 100 s against 200 s."},
		},
		"close figures are level": {
			results: []Result{{Name: "A", Passed: true, Seconds: 100, USD: usd(1.00)}, {Name: "B", Passed: true, Seconds: 110, USD: usd(1.10)}},
			fastest: -1, cheapest: -1,
			says: []string{"The time was level, 100 s to 110 s.", "The cost was level, $1.0000 to $1.1000.", "Nothing separates them"},
		},
		"a few seconds is level however large the share": {
			results: []Result{{Name: "A", Passed: true, Seconds: 2, USD: usd(1)}, {Name: "B", Passed: true, Seconds: 6, USD: usd(2)}},
			fastest: -1, cheapest: 0,
			says: []string{"The time was level, 2 s to 6 s.", "A was the cheapest"},
		},
		"an unmeasured cost is not ranked": {
			results: []Result{{Name: "A", Passed: true, Seconds: 30, USD: usd(0.10)}, {Name: "B", Passed: true, Seconds: 90}},
			fastest: 0, cheapest: -1,
			says:    []string{"The cost could not be compared: it was not measured for every run."},
			notSays: []string{"cheapest"},
		},
		"a third run with no time may have been the fastest": {
			results: []Result{{Name: "A", Passed: true, Seconds: 30, USD: usd(1)}, {Name: "B", Passed: true, Seconds: 200, USD: usd(1)},
				{Name: "C", Passed: true, USD: usd(1)}},
			fastest: -1, cheapest: -1,
			says: []string{"The time could not be compared"},
		},
		"a passing run that has no time is not ranked on time": {
			results: []Result{{Name: "A", Passed: true, Seconds: 30, USD: usd(1)}, {Name: "B", Passed: true, USD: usd(3)}},
			fastest: -1, cheapest: 0,
			says: []string{"The time could not be compared"},
		},
	} {
		got := Weigh(c.results)
		if got.Fastest != c.fastest || got.Cheapest != c.cheapest {
			t.Errorf("%s: fastest %d cheapest %d, want %d and %d: %s", name, got.Fastest, got.Cheapest, c.fastest, c.cheapest, got.Headline)
		}
		for _, want := range c.says {
			if !strings.Contains(got.Headline, want) {
				t.Errorf("%s: %q lacks %q", name, got.Headline, want)
			}
		}
		for _, not := range c.notSays {
			if strings.Contains(got.Headline, not) {
				t.Errorf("%s: %q contains %q", name, got.Headline, not)
			}
		}
	}
}
