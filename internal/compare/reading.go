package compare

import (
	"fmt"
	"strings"
)

// Result is what one run left that a comparison can read: whether it passed the
// check, how long it took, and what it cost. Cost is nil when the model calls
// were not all priced, because a floor is not a figure to rank on.
type Result struct {
	Name    string
	Passed  bool
	Seconds int64
	USD     *float64
}

// Reading is what a finished comparison says: which runs are best, and a
// sentence a person can carry into a decision.
type Reading struct {
	Headline string
	// Fastest and Cheapest are the indexes of the run that beat every other run
	// that passed by more than the two are level, or -1.
	Fastest, Cheapest int
}

// A difference is too small to mean anything when it is under 15% of the
// larger figure, or under a floor where the figures are tiny. One run each is
// one sample, so anything closer than this is noise a second run could reverse.
const (
	levelShare  = 0.15
	levelSecond = 5
	levelUSD    = 0.001
)

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
		reading.Headline = "None of the " + fmt.Sprint(len(results)) + " passed the check."
		return reading
	case 1:
		reading.Headline = results[passed[0]].Name + " was the only one to pass the check."
		return reading
	}
	taken := func(i int) (float64, bool) { return float64(results[i].Seconds), results[i].Seconds > 0 }
	spent := func(i int) (float64, bool) {
		if results[i].USD == nil {
			return 0, false
		}
		return *results[i].USD, true
	}
	reading.Fastest = leader(passed, taken, func(best, next float64) bool { return level(best, next, levelSecond) })
	reading.Cheapest = leader(passed, spent, func(best, next float64) bool { return level(best, next, levelUSD) })

	sentences := []string{passedWords(len(passed), len(results))}
	sentences = append(sentences, measure(results, passed, reading.Fastest, "time", seconds, taken))
	sentences = append(sentences, measure(results, passed, reading.Cheapest, "cost", dollars, spent))
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
	gap, larger := a-b, max(a, b)
	if gap < 0 {
		gap = -gap
	}
	return gap < floor || gap < levelShare*larger
}

// leader is the run whose figure is lowest and not level with the next lowest,
// or -1 when no run stands clear. When a run has no figure nothing can be said
// of the others, because the missing one may have been the best.
func leader(among []int, figure func(int) (float64, bool), same func(best, next float64) bool) int {
	best, runnerUp := -1, -1
	var bestFigure, runnerFigure float64
	for _, i := range among {
		value, ok := figure(i)
		if !ok {
			return -1
		}
		switch {
		case best < 0 || value < bestFigure:
			best, bestFigure, runnerUp, runnerFigure = i, value, best, bestFigure
		case runnerUp < 0 || value < runnerFigure:
			runnerUp, runnerFigure = i, value
		}
	}
	if best < 0 || runnerUp < 0 || same(bestFigure, runnerFigure) {
		return -1
	}
	return best
}

// measure says how the runs that passed compare on one figure: who led and by
// how much, that they were level, or that a figure is missing.
func measure(results []Result, passed []int, lead int, what string, show func(float64) string, figure func(int) (float64, bool)) string {
	var have []int
	for _, i := range passed {
		if _, ok := figure(i); ok {
			have = append(have, i)
		}
	}
	if len(have) < len(passed) {
		return "The " + what + " could not be compared: it was not measured for every run."
	}
	if lead < 0 {
		low, high := figureRange(have, figure)
		return fmt.Sprintf("The %s was level, %s to %s.", what, show(low), show(high))
	}
	next := -1.0
	best, _ := figure(lead)
	for _, i := range have {
		if value, _ := figure(i); i != lead && (next < 0 || value < next) {
			next = value
		}
	}
	return fmt.Sprintf("%s was the %s, at %s against %s.", results[lead].Name, superlative(what), show(best), show(next))
}

func superlative(what string) string {
	if what == "time" {
		return "fastest"
	}
	return "cheapest"
}

func figureRange(among []int, figure func(int) (float64, bool)) (low, high float64) {
	for n, i := range among {
		value, _ := figure(i)
		if n == 0 || value < low {
			low = value
		}
		if n == 0 || value > high {
			high = value
		}
	}
	return low, high
}

func seconds(value float64) string { return fmt.Sprintf("%.0f s", value) }

func dollars(value float64) string { return fmt.Sprintf("$%.4f", value) }
