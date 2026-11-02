package web

import (
	"net/http"
	"net/url"
	"time"
)

// overviewPage answers one question for an admin: is this month's spending under control. It changes nothing; budgets
// are set on Budgets.
func (s *Server) overviewPage(w http.ResponseWriter, r *http.Request) {
	month := monthStart(time.Now())
	spent, err := s.monthSpend(r, month)
	if err != nil {
		http.Error(w, "the spend could not be read: "+err.Error(), http.StatusInternalServerError)
		return
	}
	budget := s.budgetOf(spent.USD)
	page := map[string]any{
		"Title":       productName + " — admin",
		"MonthName":   month.Format("January"),
		"Spent":       spent,
		"Budget":      budget,
		"PeoplePage":  s.hasRoster(),
		"BudgetsPage": s.hasOrgSettings(),
		"PeopleAll":   s.peopleAll("/admin/usage?" + url.Values{"from": {month.Format("2006-01-02")}}.Encode()),
	}
	if budget.Set {
		page["Percent"] = percentOf(spent.USD, budget.Limit)
	}
	if ledger, err := s.spending(); err == nil {
		if used, limit, held := ledger.Credit(); held {
			page["Credit"] = map[string]float64{"Used": used, "Limit": limit}
		}
	}
	s.drawConsole(w, "admin-overview.html", "overview", page)
}

// percentOf is how much of a budget is spent, as a bar draws it: whole, at most all of it, and never none once anything is.
func percentOf(spent, budget float64) int {
	if spent <= 0 {
		return 0
	}
	return max(1, min(100, int(spent/budget*100+0.5)))
}

// monthStart is the first moment of the month in UTC, which is the month the ledger and the budgets count.
func monthStart(now time.Time) time.Time {
	now = now.UTC()
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
}
