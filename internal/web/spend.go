package web

import (
	"net/http"
	"regexp"

	"github.com/getlatentic/haas/internal/spend"
)

// UseLedger hands the pages the ledger the proxy counts in and checks caps
// against, so the report and the caps read the same numbers. A server with no
// proxy has none, and the page rebuilds one from the jobs' own records instead.
func (s *Server) UseLedger(ledger *spend.Ledger) { s.ledger = ledger }

func (s *Server) spending() (*spend.Ledger, error) {
	if s.ledger != nil {
		return s.ledger, nil
	}
	return spend.Rebuild(s.index.Root(), spend.Policy{})
}

var aMonth = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)

// report is the month the request asks for, or this month.
func (s *Server) report(r *http.Request) (spend.Report, []string, error) {
	ledger, err := s.spending()
	if err != nil {
		return spend.Report{}, nil, err
	}
	month := r.URL.Query().Get("month")
	if !aMonth.MatchString(month) {
		month = ledger.ThisMonth()
	}
	return ledger.Report(month), ledger.Months(), nil
}

// spendPage shows what the agents cost by team, project and job.
func (s *Server) spendPage(w http.ResponseWriter, r *http.Request) {
	report, months, err := s.report(r)
	if err != nil {
		http.Error(w, "the spend could not be read: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.render(w, "spend.html", map[string]any{
		"Title":    "haas — spend",
		"Subtitle": "what the agents cost",
		"Report":   report,
		"Months":   months,
	})
}

// spendJSON is the same report for a script, behind the same password the API
// takes.
func (s *Server) spendJSON(w http.ResponseWriter, r *http.Request) {
	report, _, err := s.report(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.asJSON(w, report)
}
