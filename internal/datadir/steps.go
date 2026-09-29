package datadir

import "sort"

// Step migrates one store's directory from format From to From+1. Up must be safe
// to run again, because a crash between a step and the new stamp runs it a
// second time. Down undoes Up, and is nil for a step that cannot be undone.
type Step struct {
	Store Store
	From  int
	What  string
	Up    func(dir string) error
	Down  func(dir string) error
}

// Steps is every migration this release knows. None has been needed yet: every
// store is still at format 1.
var Steps = []Step{}

func stepsFor(store Store, from, to int) []Step {
	var pending []Step
	for format := from; format < to; format++ {
		if step, found := stepFrom(store, format); found {
			pending = append(pending, step)
		}
	}
	sort.SliceStable(pending, func(a, b int) bool { return pending[a].From < pending[b].From })
	return pending
}

func stepFrom(store Store, from int) (Step, bool) {
	for _, step := range Steps {
		if step.Store == store && step.From == from {
			return step, true
		}
	}
	return Step{}, false
}
