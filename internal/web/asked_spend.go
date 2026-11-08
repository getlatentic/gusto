package web

import (
	"strings"

	"github.com/getlatentic/gusto/pkg/spend"
)

// askedSpend is this month's spending by who asked for it, the measure the proxy holds a person to their budget by. A person's
// agent that somebody else used counts against the one who asked, not the agent's owner.
type askedSpend struct {
	// People is each person's tally, by their address in lower case.
	People map[string]spend.Tally
	// NoOne is the calls of tasks that named nobody, so the people and NoOne add up to Total.
	NoOne, Total spend.Tally
	Month        string
}

// askedThisMonth reads what each person asked for this month from the ledger the proxy counts in.
func (s *Server) askedThisMonth() (askedSpend, error) {
	ledger, err := s.spending()
	if err != nil {
		return askedSpend{People: map[string]spend.Tally{}}, err
	}
	report := ledger.Report(ledger.ThisMonth())
	asked := askedSpend{People: make(map[string]spend.Tally, len(report.People)), Total: report.Total, Month: report.Month, NoOne: report.Total}
	for _, line := range report.People {
		asked.People[strings.ToLower(line.Name)] = line.Tally
		asked.NoOne.Calls -= line.Calls
		asked.NoOne.USD -= line.USD
		asked.NoOne.Unmeasured -= line.Unmeasured
	}
	if asked.NoOne.Calls <= 0 {
		asked.NoOne = spend.Tally{}
	}
	return asked, nil
}

// spentBy is what one person asked for this month, as their budget counts it.
func (a askedSpend) spentBy(who string) float64 { return a.People[strings.ToLower(who)].USD }
