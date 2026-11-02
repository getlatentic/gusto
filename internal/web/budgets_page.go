package web

import (
	"net/http"

	"github.com/getlatentic/gusto/internal/roster"
)

// budgetsPath is the console's page of what each person may spend on models.
const budgetsPath = "/admin/budgets"

// budgetRow is one person as the table of budgets reads it. Percent is how much of their budget the bar fills.
type budgetRow struct {
	Email, Name string
	Budget      memberBudget
	Percent     int
}

// budgetsPage is everyone's budget and the organization's, then each person's own and what they have spent of it.
func (s *Server) budgetsPage(w http.ResponseWriter, r *http.Request) {
	page := map[string]any{"Title": productName + " — budgets", "Refused": r.URL.Query().Get("refused"), "Budget": s.budgetOf(0),
		"Listed": s.hasRoster()}
	if s.hasRoster() {
		list, err := roster.Read(s.rosterPath)
		if err != nil {
			page["Unreadable"] = err.Error()
		} else {
			asked, err := s.askedThisMonth()
			if err != nil {
				page["SpendUnreadable"] = err.Error()
			}
			page["People"], page["Pages"] = onePage(r, budgetsPath, s.budgetRows(list, asked))
		}
	}
	s.drawConsole(w, "admin-budgets.html", "budgets", page)
}

func (s *Server) budgetRows(list roster.Roster, asked askedSpend) []budgetRow {
	rows := make([]budgetRow, 0, len(list.Members))
	for _, member := range list.Members {
		row := budgetRow{Email: member.Email, Name: s.personName(member.Email), Budget: s.memberBudgetFor(member.Email, asked.spentBy(member.Email))}
		if row.Budget.Set {
			row.Percent = percentOf(row.Budget.Spent, row.Budget.Limit)
		}
		rows = append(rows, row)
	}
	return rows
}
