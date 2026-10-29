package web

import (
	"fmt"
	"sort"
	"time"

	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/pkg/spend"
)

// limitRow is one limit the organization is under, what has been used of it, and
// whether it is used up.
type limitRow struct {
	What, Used, Of string
	Over           bool
}

// addLimits lists the limits there are and how much of each is used. A limit that does
// not exist has no row, so a row is never a limit of nothing.
func (s *Server) addLimits(page map[string]any, policy spend.Policy, report spend.Report, budget budgetState, running int, now time.Time) {
	var rows []limitRow
	if budget.Set {
		rows = append(rows, limitRow{
			What: "Budget for " + report.Month, Used: headlineUSD(report.Total.USD), Of: headlineUSD(budget.Limit), Over: budget.Passed,
		})
	}
	rows = append(rows, teamCaps(policy, report)...)
	if limit, capped := policy.OrganizationCap(); capped {
		spent := 0.0
		for _, line := range report.Payers {
			if line.Name == spend.PaidByOperator {
				spent = line.USD
			}
		}
		rows = append(rows, limitRow{What: "Models this server provides, " + report.Month, Used: headlineUSD(spent), Of: headlineUSD(limit), Over: spent >= limit})
	}
	allowed, under, err := s.index.Allowance(now)
	switch {
	case err != nil:
		page["LimitsUnreadable"] = err.Error()
	case under:
		rows = append(rows, s.planRows(allowed, running)...)
	}
	page["Limits"] = rows
}

// teamCaps is each team the spend policy caps for the month, with what it has spent.
func teamCaps(policy spend.Policy, report spend.Report) []limitRow {
	spent := map[string]float64{}
	for _, line := range report.Teams {
		spent[line.Name] = line.USD
	}
	var rows []limitRow
	for name, capped := range policy.Teams {
		rows = append(rows, limitRow{
			What: "Team " + name + ", " + report.Month, Used: headlineUSD(spent[name]), Of: headlineUSD(capped.USDPerMonth),
			Over: spent[name] >= capped.USDPerMonth,
		})
	}
	sort.Slice(rows, func(a, b int) bool { return rows[a].What < rows[b].What })
	return rows
}

// planRows is the limits the organization's quota sets: the jobs it may start, how many
// may run at once, and how many computers it may have.
func (s *Server) planRows(allowed jobs.Allowance, running int) []limitRow {
	within := "in all"
	if allowed.Quota.Monthly {
		within = "this month"
	}
	rows := []limitRow{
		{What: "Tasks started " + within, Used: fmt.Sprint(allowed.Started), Of: fmt.Sprint(allowed.Quota.Jobs), Over: allowed.Started >= allowed.Quota.Jobs},
		{What: "Tasks at once", Used: fmt.Sprint(running), Of: fmt.Sprint(allowed.Quota.Concurrent), Over: running >= allowed.Quota.Concurrent},
	}
	if s.computers == nil {
		return rows
	}
	if all, err := s.computers.All(); err == nil {
		rows = append(rows, limitRow{What: "Workspaces", Used: fmt.Sprint(len(all)), Of: fmt.Sprint(allowed.Quota.Computers), Over: len(all) >= allowed.Quota.Computers})
	}
	return rows
}

// payersOf is the month's spending by whose key paid the provider, in the words an admin reads, and nothing when every call went
// on the keys this server provides: then there is nothing to tell apart.
func payersOf(lines []spend.Line) []share {
	own := false
	for _, line := range lines {
		own = own || line.Name == spend.PaidByOrganization
	}
	if !own {
		return nil
	}
	out := shares(lines)
	for i := range out {
		switch out[i].Name {
		case spend.PaidByOperator:
			out[i].Label = "Models this server provides"
		case spend.PaidByOrganization:
			out[i].Label = "Your organization's own keys"
		}
	}
	return out
}
