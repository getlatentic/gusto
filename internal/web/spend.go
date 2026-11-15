package web

import (
	"net/http"
	"regexp"
	"time"

	"github.com/getlatentic/gusto/pkg/spend"
)

// UseLedger hands the pages the ledger the proxy counts in and checks caps
// against, so the report and the caps read the same numbers. A server with no
// proxy has none, and the page rebuilds one from the tasks' own records instead.
func (s *Server) UseLedger(ledger *spend.Ledger) { s.ledger = ledger }

func (s *Server) spending() (*spend.Ledger, error) {
	if s.ledger != nil {
		return s.ledger, nil
	}
	return spend.Rebuild(s.index.Root(), spend.Policy{})
}

var aMonth = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)

// spendOf is the ledger this person may read: all of it for an admin, and the
// tasks they may see otherwise, priced as the whole ledger is.
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

// spendPage shows what the agents cost by team, project and task.
func (s *Server) spendPage(w http.ResponseWriter, r *http.Request) {
	page, err := s.spendData(r, "/spend")
	if err != nil {
		answerFailure(w, r, err)
		return
	}
	s.render(w, "spend.html", page)
}

// spendData is what the spend report is drawn from, in the app's template and as the console's JSON, where base
// /admin/spending is the whole organization's.
func (s *Server) spendData(r *http.Request, base string) (map[string]any, error) {
	ledger, err := s.spendOf(r)
	if err != nil {
		return nil, failed{http.StatusInternalServerError, "the spend could not be read: " + err.Error()}
	}
	report, months := reportOf(ledger, r)
	return map[string]any{
		"Title":  productName + " — usage",
		"Nav":    "usage",
		"Base":   base,
		"Tabs":   tabsFor(base != "/spend", "summary"),
		"Ledger": s.viewLedger(report, months, time.Now()),
	}, nil
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
	s.asJSON(w, askers{Report: report, Askers: s.byAsker(report.Tasks)})
}
