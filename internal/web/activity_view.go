package web

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/getlatentic/gusto/internal/taskindex"
	"github.com/getlatentic/gusto/internal/tasks"
)

// Activity's data and a run as the work pages show it, in the shapes web/src/pages/work/types.ts names.

const jobsRoute = "/jobs"

// runView is one run as a list of them shows it. Outcome is how it ended in the words of a log; Waiting, for a queued run,
// where it stands, and AskedBy who asked for it. When is the moment it was asked and Hour the time of day.
type runView struct {
	ID, Brief, State string
	Outcome          string
	On               origin
	Brain            string
	AskedBy, Waiting string
	When, Hour       string
	Cost             *costView
}

// costView is what a run cost as a line says it: Title qualifies a figure that is not the provider's own count of money.
type costView struct{ Text, Title string }

// boardView is the record as Activity shows it: the counts across the top, the filters, and a page of runs by day.
type boardView struct {
	Figures   []figure
	State     string
	Harness   string
	Harnesses []string
	Days      []dayView
	Shown     string
	Newer     string
	Older     string
	Index     *indexed
}

type dayView struct {
	Named string
	Runs  []runView
}

// jobsData is Activity: every run this server knows that the person may read, a page at a time.
func (s *Server) jobsData(r *http.Request) (map[string]any, error) {
	shown, err := s.boardFor(r)
	var bad badQuery
	switch {
	case errors.As(err, &bad):
		return nil, failed{http.StatusBadRequest, bad.Error()}
	case err != nil:
		return nil, failed{http.StatusInternalServerError, "the tasks could not be read: " + taskindex.Scrub(err)}
	}
	return map[string]any{"Board": s.boardViewOf(shown)}, nil
}

func (s *Server) boardViewOf(shown board) boardView {
	view := boardView{Figures: shown.Figures, State: shown.State, Harness: shown.Harness, Harnesses: shown.Harnesses,
		Days: make([]dayView, 0, len(shown.Days)), Shown: shown.Shown, Newer: shown.Newer, Older: shown.Older, Index: shown.Index}
	for _, each := range shown.Days {
		day := dayView{Named: each.Named, Runs: make([]runView, 0, len(each.Runs))}
		for _, one := range each.Runs {
			day.Runs = append(day.Runs, s.runViewOf(one))
		}
		view.Days = append(view.Days, day)
	}
	return view
}

func (s *Server) runViewOf(shown run) runView {
	view := runView{ID: shown.ID, Brief: shown.Brief, State: string(shown.State), Outcome: outcomeOf(shown.Record), On: shown.On,
		Brain: modelName(shown.Model), Waiting: s.waiting(shown.ID), When: whenOf(shown.Queued), Hour: hourOf(shown.Queued),
		Cost: costOfRun(shown)}
	if view.Waiting != "" {
		view.AskedBy = askerOf(shown.Asked)
	}
	return view
}

// outcomeOf is how a run ended, in the words of a log: a turn replied, a task passed or failed its check, and one still going
// says so.
func outcomeOf(record tasks.Record) string {
	switch {
	case record.State == tasks.Queued:
		return "waiting"
	case record.State == tasks.Running:
		return "working"
	case record.State == tasks.Interrupted:
		return "interrupted"
	case record.Thread != "" && record.State == tasks.Passed:
		return "replied"
	case record.Thread != "":
		return "failed"
	case record.State == tasks.Passed:
		return "check passed"
	}
	return "check failed"
}

// costOfRun is what the proxy counted, which is the provider's own figure; a run that never went through the proxy shows the
// agent's claim, marked as one.
func costOfRun(shown run) *costView {
	switch {
	case shown.Metered && shown.Spent.Unmeasured == 0:
		return &costView{Text: shown.Spent.Money()}
	case shown.Metered:
		return &costView{Text: compact(shown.Spent.InputTokens+shown.Spent.OutputTokens) + " tokens",
			Title: fmt.Sprintf("%d calls, and no price for this model", shown.Spent.Calls)}
	case shown.Cost != nil:
		return &costView{Text: fmt.Sprintf("$%.4f", *shown.Cost), Title: "the agent's own figure"}
	}
	return nil
}

// whenOf is a moment as a line says it, "2 Jan 15:04", or nothing for none.
func whenOf(unix int64) string {
	if unix == 0 {
		return ""
	}
	return time.Unix(unix, 0).Format("2 Jan 15:04")
}

func hourOf(unix int64) string { return time.Unix(unix, 0).Format("15:04") }
