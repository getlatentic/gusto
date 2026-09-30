package web

import (
	"net/http"
	"time"

	"github.com/getlatentic/haas/internal/jobs"
)

// overviewPage is where an admin starts: what the organisation has spent this
// month and what its jobs are doing now.
func (s *Server) overviewPage(w http.ResponseWriter, r *http.Request) {
	ledger, err := s.spending()
	if err != nil {
		http.Error(w, "the spend could not be read: "+err.Error(), http.StatusInternalServerError)
		return
	}
	now := time.Now()
	stats := s.index.Stats(now)
	report := ledger.Report(ledger.ThisMonth())
	s.drawConsole(w, "admin-overview.html", "overview", map[string]any{
		"Title":   "haas — admin",
		"Month":   report.Month,
		"Spent":   report.Total,
		"Running": stats.ByState[jobs.Running],
		"Waiting": stats.ByState[jobs.Queued],
		"Failed":  s.failedSince(now.Add(-lately)),
	})
}

// failedSince is how many jobs asked for after a time failed or were
// interrupted, as the record of jobs counts them.
func (s *Server) failedSince(since time.Time) int {
	failed := 0
	for _, record := range s.index.List() {
		if filed(record.State) == string(jobs.Failed) && record.Queued >= since.Unix() {
			failed++
		}
	}
	return failed
}
