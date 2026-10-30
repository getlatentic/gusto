package web

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/getlatentic/gusto/internal/changelog"
	"github.com/getlatentic/gusto/internal/orgsettings"
)

// UseOrgSettings says where the choices an admin makes for the organization are kept,
// which is the monthly budget. Without it the console has no budget to set.
func (s *Server) UseOrgSettings(path string) { s.orgSettings = orgsettings.Open(path) }

func (s *Server) hasOrgSettings() bool { return s.orgSettings != nil }

// maxBudget is the most a budget can be, in dollars. A larger figure is a mistake in
// the number, and cannot be told from one.
const maxBudget = 1e9

// budgetState is the month's budget and whether it is passed. A budget that cannot be
// read is Err and is not checked, since no answer is not the answer that it is not passed.
type budgetState struct {
	Editable bool
	Set      bool
	Limit    float64
	Passed   bool
	Over     float64
	Err      string
	// Member is what each member may spend in a month, when MemberSet.
	Member    float64
	MemberSet bool
}

// budgetOf says whether what has been spent this month has passed the budget. Spending
// that could not be priced is not in the sum, so a budget is passed by at least that.
func (s *Server) budgetOf(spent float64) budgetState {
	if s.orgSettings == nil {
		return budgetState{}
	}
	settings, err := s.orgSettings.Read()
	if err != nil {
		return budgetState{Editable: true, Err: err.Error()}
	}
	state := budgetState{Editable: true, Set: settings.MonthlyBudgetUSD > 0, Limit: settings.MonthlyBudgetUSD,
		Member: settings.MemberBudgetUSD, MemberSet: settings.MemberBudgetUSD > 0}
	state.Passed = state.Set && spent > state.Limit
	if state.Passed {
		state.Over = spent - state.Limit
	}
	return state
}

// setBudget keeps the monthly budget an admin gives, or takes it away when they give none.
func (s *Server) setBudget(w http.ResponseWriter, r *http.Request) {
	usd, err := parseBudget(r.FormValue("usd"))
	if err != nil {
		s.turnAway(w, r, "/admin", err.Error())
		return
	}
	var was float64
	err = s.orgSettings.Change(func(settings *orgsettings.Settings) {
		was, settings.MonthlyBudgetUSD = settings.MonthlyBudgetUSD, usd
	})
	if err != nil {
		s.turnAway(w, r, "/admin", err.Error())
		return
	}
	if was != usd {
		s.note(r, changelog.BudgetSet, map[string]string{"was": budgetWords(was), "now": budgetWords(usd)})
	}
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

// parseBudget is the dollars an admin typed, to the cent. Nothing typed is no budget.
func parseBudget(typed string) (float64, error) {
	typed = strings.TrimSpace(typed)
	if typed == "" {
		return 0, nil
	}
	usd, err := strconv.ParseFloat(typed, 64)
	if err != nil || math.IsNaN(usd) || usd < 0 || usd > maxBudget {
		return 0, errors.New("the budget is a number of dollars from 0 to a billion, or nothing for no budget")
	}
	return math.Round(usd*100) / 100, nil
}

// budgetWords is a budget as the record of changes says it.
func budgetWords(usd float64) string {
	if usd == 0 {
		return "none"
	}
	return fmt.Sprintf("$%.2f", usd)
}
