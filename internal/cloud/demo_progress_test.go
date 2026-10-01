package cloud

import (
	"testing"
	"time"
)

func TestADemoShowsHowFarItIsFromReady(t *testing.T) {
	marks := func(state State) string {
		var out string
		for _, step := range demoSteps(state) {
			out += step.Mark[:1]
		}
		return out
	}
	for state, want := range map[State]string{Waiting: "ntt", Provisioning: "ntt", Failed: "ntt", Starting: "dnt", Ready: "ddd"} {
		if got := marks(state); got != want {
			t.Errorf("%s: %s, want %s", state, got, want)
		}
	}
}

func TestHowLongAVisitorHasWaitedIsToTheSecond(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	for since, want := range map[time.Duration]string{12 * time.Second: "12 s", 83 * time.Second: "1 min 23 s", 5 * time.Minute: "5 min 00 s"} {
		if got := waited(now.Add(-since), now); got != want {
			t.Errorf("%s: %q, want %q", since, got, want)
		}
	}
	if waited(time.Time{}, now) != "" {
		t.Error("a demo with no start has waited")
	}
}
