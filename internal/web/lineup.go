package web

import (
	"net/http"

	"github.com/getlatentic/gusto/internal/compare"
	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/internal/spend"
)

// side is one engine's run as the comparison shows it: whether the check
// passed, what it took and cost, and how much it changed.
type side struct {
	Engine  string
	Model   string
	Record  jobs.Record
	Checked bool
	Passed  bool
	Spent   spend.Tally
	Metered bool
	Files   int
	Added   int
	Removed int
	// Fastest and Cheapest mark the run that stands clear of every other run
	// that passed, and neither is set when they are level.
	Fastest  bool
	Cheapest bool
}

// Seconds is the run's time, the box starting included.
func (s side) Seconds() int64 {
	if s.Record.Started == 0 || s.Record.Ended == 0 {
		return 0
	}
	return s.Record.Ended - s.Record.Started
}

func (s *Server) lineupPage(w http.ResponseWriter, r *http.Request) {
	if s.lineups == nil {
		http.NotFound(w, r)
		return
	}
	lineup, found := s.lineups.Get(r.PathValue("id"))
	if !found || !s.mayUse(r, lineup.Who) {
		http.NotFound(w, r)
		return
	}
	ledger, _ := s.spending()
	sides, running := s.sidesOf(lineup, ledger)
	page := map[string]any{
		"Title": productName + " — " + lineup.Brief, "Nav": "compare",
		"Lineup": lineup, "Sides": sides, "Running": running,
		"Passed": passedCount(sides),
	}
	// A verdict on runs that are still working would name a winner that a run
	// still to finish could overturn.
	if !running && len(sides) > 1 {
		page["Reading"] = weigh(sides)
	}
	s.render(w, "lineup.html", page)
}

func (s *Server) sidesOf(lineup compare.Lineup, ledger *spend.Ledger) ([]side, bool) {
	sides := make([]side, 0, len(lineup.Runs))
	running := false
	for _, entry := range lineup.Runs {
		record, found := s.index.Get(entry.Job)
		if !found {
			continue
		}
		running = running || !record.State.Done()
		sides = append(sides, s.sideOf(entry, record, ledger))
	}
	return sides, running
}

func (s *Server) sideOf(entry compare.Entry, record jobs.Record, ledger *spend.Ledger) side {
	shown := side{Engine: s.engineName(entry.Harness), Model: modelName(record.Model), Record: record}
	dir := record.Dir(s.index.Root())
	if manifest := readManifest(dir); manifest != nil && manifest.Gate != nil {
		shown.Checked, shown.Passed = true, manifest.Gate.Passed
	}
	shown.Spent, shown.Metered = spentBy(ledger, record.ID)
	if patch, found := readPatch(dir, record.ID); found {
		shown.Files = len(patch.Files)
		for _, file := range patch.Files {
			shown.Added += file.Added
			shown.Removed += file.Removed
		}
	}
	return shown
}

// weigh reads the finished runs together: which stands clear on time and on
// cost, and a sentence saying so, or saying that nothing separates them.
func weigh(sides []side) compare.Reading {
	results := make([]compare.Result, len(sides))
	for i, one := range sides {
		results[i] = compare.Result{Name: one.Engine, Passed: one.Passed, Seconds: one.Seconds()}
		if one.Metered {
			usd := one.Spent.USD
			results[i].USD, results[i].Floor = &usd, one.Spent.Unmeasured > 0
		}
	}
	reading := compare.Weigh(results)
	if reading.Fastest >= 0 {
		sides[reading.Fastest].Fastest = true
	}
	if reading.Cheapest >= 0 {
		sides[reading.Cheapest].Cheapest = true
	}
	return reading
}

func passedCount(sides []side) int {
	passed := 0
	for _, one := range sides {
		if one.Passed {
			passed++
		}
	}
	return passed
}
