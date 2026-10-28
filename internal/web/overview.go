package web

import (
	"net/http"
	"time"

	"github.com/getlatentic/gusto/internal/jobs"
)

// overviewPage is where an admin starts: what the organization has spent this month
// and against which limits, what its jobs are doing, and what computers and apps it has.
// The figures are the ledger's, the queue's and the records' own, so this page and the
// pages they came from cannot disagree.
func (s *Server) overviewPage(w http.ResponseWriter, r *http.Request) {
	ledger, err := s.spending()
	if err != nil {
		http.Error(w, "the spend could not be read: "+err.Error(), http.StatusInternalServerError)
		return
	}
	now := time.Now()
	report := ledger.Report(ledger.ThisMonth())
	view := s.viewLedger(report, nil, now)
	page := map[string]any{
		"Title":    productName + " — admin",
		"Refused":  r.URL.Query().Get("refused"),
		"Month":    report.Month,
		"Spent":    report.Total,
		"People":   shares(s.byAsker(report.Jobs)),
		"Teams":    view.Teams,
		"Models":   view.Models,
		"Projects": view.Projects,
	}
	budget := s.budgetOf(report.Total.USD)
	page["Budget"] = budget
	if s.hasOrgSettings() {
		page["Defaults"] = s.defaultChoices(r)
		page["RunTokens"] = s.runTokensOf()
	}
	running := s.addJobs(page, now)
	s.addLimits(page, ledger.Policy(), report, budget, running, now)
	s.addComputers(page)
	s.addApps(page)
	s.drawConsole(w, "admin-overview.html", "overview", page)
}

// addJobs says what the queue is doing, and how many jobs are running now.
func (s *Server) addJobs(page map[string]any, now time.Time) (running int) {
	stats := s.index.Stats(now)
	running = stats.ByState[jobs.Running]
	page["Running"], page["Slots"] = running, stats.Slots
	page["Waiting"] = stats.ByState[jobs.Queued]
	page["Failed"] = s.failedSince(now.Add(-lately))
	return running
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
