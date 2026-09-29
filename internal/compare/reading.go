package compare

import (
	"fmt"
	"strings"
)

// Result is what one run left that a comparison can read: whether it passed the
// check, how long it took, and what it cost.
type Result struct {
	Name    string
	Passed  bool
	Seconds int64
	// USD is nil when the run's model calls were not counted at all. Floor says
	// the figure is a floor: some calls were not priced, so the run cost at
	// least this.
	USD   *float64
	Floor bool
}

// Reading is what a finished comparison says: which runs are best, and a
// sentence a person can carry into a decision.
type Reading struct {
	Headline string
	// Fastest and Cheapest are the indexes of the run that beat every other run
	// that passed by more than the two are level, or -1.
	Fastest, Cheapest int
}

// A difference is too small to mean anything when it is under 15% of the larger
// figure, or under a floor where the figures are tiny. One run each is one
// sample, so anything closer than this is noise a second run could reverse.
const (
	levelShare  = 0.15
	levelSecond = 5
	levelUSD    = 0.001
)

// figure is one run's number on one measure. A floor is a lower bound, so it
// can rule a run out and can never make a run the best.
type figure struct {
	value float64
	floor bool
	known bool
}

// Weigh says what results show. Only runs that passed are ranked: a run that
// failed the check is not fast or cheap at anything.
func Weigh(results []Result) Reading {
	reading := Reading{Fastest: -1, Cheapest: -1}
	var passed []int
	for i, one := range results {
		if one.Passed {
			passed = append(passed, i)
		}
	}
	switch len(passed) {
	case 0:
		reading.Headline = fmt.Sprintf("None of the %d passed the check.", len(results))
		return reading
	case 1:
		reading.Headline = results[passed[0]].Name + " was the only one to pass the check."
		return reading
	}
	taken := func(i int) figure {
		return figure{value: float64(results[i].Seconds), known: results[i].Seconds > 0}
	}
	cost := func(i int) figure {
		if results[i].USD == nil {
			return figure{}
		}
		return figure{value: *results[i].USD, floor: results[i].Floor, known: true}
	}
	fastest, cheapest := rank{what: "time", lead: "fastest", floor: levelSecond, show: seconds, of: taken},
		rank{what: "cost", lead: "cheapest", floor: levelUSD, show: dollars, of: cost}
	var timeSaid, costSaid string
	reading.Fastest, timeSaid = fastest.read(results, passed)
	reading.Cheapest, costSaid = cheapest.read(results, passed)
	sentences := []string{passedWords(len(passed), len(results)), timeSaid, costSaid}
	if reading.Fastest < 0 && reading.Cheapest < 0 {
		sentences = append(sentences, "Nothing separates them on this task.")
	}
	reading.Headline = strings.Join(sentences, " ")
	return reading
}

func passedWords(passed, total int) string {
	if passed == total {
		return fmt.Sprintf("All %d passed the check.", total)
	}
	return fmt.Sprintf("%d of %d passed the check.", passed, total)
}

// level says whether two figures are too close to tell apart.
func level(a, b, floor float64) bool {
	gap := max(a-b, b-a)
	return gap < floor || gap < levelShare*max(a, b)
}

// rank is one measure the runs that passed are ranked on, lowest best.
type rank struct {
	what, lead string
	floor      float64
	show       func(float64) string
	of         func(int) figure
}

// read is the run that stands clear on this measure, or -1, and a sentence
// saying who led, or why nobody did. A run with no figure at all may have been
// the best, so nothing is said of the others.
func (r rank) read(results []Result, among []int) (int, string) {
	best, next := -1, -1
	for _, i := range among {
		f := r.of(i)
		if !f.known {
			return -1, "The " + r.what + " could not be compared: it was not measured for every run."
		}
		switch {
		case best < 0 || f.value < r.of(best).value:
			best, next = i, best
		case next < 0 || f.value < r.of(next).value:
			next = i
		}
	}
	first, second := r.of(best), r.of(next)
	switch {
	case first.floor:
		return -1, fmt.Sprintf("%s could be the %s, but its %s was not priced in full.", results[best].Name, r.lead, r.what)
	case second.floor && level(first.value, second.value, r.floor):
		return -1, fmt.Sprintf("The %s could not be ranked: %s was not priced in full, and may have been as low as %s.",
			r.what, results[next].Name, r.show(second.value))
	case level(first.value, second.value, r.floor):
		return -1, fmt.Sprintf("No run was clearly the %s: %s and %s were level at the front, %s and %s.",
			r.lead, results[best].Name, results[next].Name, r.show(first.value), r.shown(second))
	}
	return best, fmt.Sprintf("%s was the %s, at %s against %s.", results[best].Name, r.lead, r.show(first.value), r.shown(second))
}

// shown is a figure the way it is said, with "at least" before a floor.
func (r rank) shown(f figure) string {
	if f.floor {
		return "at least " + r.show(f.value)
	}
	return r.show(f.value)
}

func seconds(value float64) string { return fmt.Sprintf("%.0f s", value) }

func dollars(value float64) string { return fmt.Sprintf("$%.4f", value) }
