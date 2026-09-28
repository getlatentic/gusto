package web

import (
	"net/http"

	"github.com/getlatentic/haas/internal/compare"
	"github.com/getlatentic/haas/internal/jobs"
	"github.com/getlatentic/haas/internal/spend"
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
	// Fastest and Cheapest mark the best of the runs that passed.
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
	if !found {
		http.NotFound(w, r)
		return
	}
	ledger, _ := s.spending()
	sides, running := s.sidesOf(lineup, ledger)
	s.render(w, "lineup.html", map[string]any{
		"Title": "haas — " + lineup.Brief, "Nav": "compare",
		"Lineup": lineup, "Sides": sides, "Running": running,
		"Passed": passedCount(sides),
	})
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
	markBest(sides)
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

// markBest names the fastest and the cheapest of the runs that passed. A run
// that failed its check is not best at anything.
func markBest(sides []side) {
	fastest, cheapest := -1, -1
	for i, one := range sides {
		if !one.Passed {
			continue
		}
		if fastest < 0 || one.Seconds() < sides[fastest].Seconds() {
			fastest = i
		}
		if one.Metered && one.Spent.Unmeasured == 0 && (cheapest < 0 || one.Spent.USD < sides[cheapest].Spent.USD) {
			cheapest = i
		}
	}
	if fastest >= 0 {
		sides[fastest].Fastest = true
	}
	if cheapest >= 0 {
		sides[cheapest].Cheapest = true
	}
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
