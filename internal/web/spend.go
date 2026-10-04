package web

import (
	"net/http"
	"regexp"
	"time"

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

// spendOf is the ledger this person may read: all of it for an admin, and the
// jobs they may see otherwise, priced as the whole ledger is.
func (s *Server) spendOf(r *http.Request) (*spend.Ledger, error) {
	whole, err := s.spending()
	if err != nil || s.admin(r) {
		return whole, err
	}
	return spend.RebuildWhere(s.index.Root(), whole.Policy(), func(id string) bool {
		record, found := s.index.Get(id)
		return found && s.mayUse(r, record.Asked)
	})
}

// report is the month the request asks for, or this month.
func reportOf(ledger *spend.Ledger, r *http.Request) (spend.Report, []string) {
	month := r.URL.Query().Get("month")
	if !aMonth.MatchString(month) {
		month = ledger.ThisMonth()
	}
	return ledger.Report(month), ledger.Months()
}

// spendPage shows what the agents cost by team, project and job.
func (s *Server) spendPage(w http.ResponseWriter, r *http.Request) {
	if page, ok := s.spendData(w, r, "/spend"); ok {
		s.render(w, "spend.html", page)
	}
}

// consoleSpend is the same report in the console, for the whole organisation.
func (s *Server) consoleSpend(w http.ResponseWriter, r *http.Request) {
	if page, ok := s.spendData(w, r, "/admin/spending"); ok {
		s.drawConsole(w, "spend.html", "spending", page)
	}
}

// spendData is what the spend report is drawn from. It answers 500 itself when
// the ledger cannot be read.
func (s *Server) spendData(w http.ResponseWriter, r *http.Request, base string) (map[string]any, bool) {
	ledger, err := s.spendOf(r)
	if err != nil {
		http.Error(w, "the spend could not be read: "+err.Error(), http.StatusInternalServerError)
		return nil, false
	}
	report, months := reportOf(ledger, r)
	return map[string]any{
		"Title":  productName + " — spend",
		"Nav":    "spend",
		"Base":   base,
		"Ledger": s.viewLedger(report, months, time.Now()),
	}, true
}

// spendJSON is the same report for a script, behind the same password the API
// takes.
func (s *Server) spendJSON(w http.ResponseWriter, r *http.Request) {
	ledger, err := s.spending()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	report, _ := reportOf(ledger, r)
	s.asJSON(w, askers{Report: report, Askers: s.byAsker(report.Jobs)})
}
