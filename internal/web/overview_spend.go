package web

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/getlatentic/gusto/internal/orgsettings"
	"github.com/getlatentic/gusto/internal/roster"
	"github.com/getlatentic/gusto/internal/usage"
)

// overviewLines is how many lines each split of the month shows before the rest are added up as one.
const overviewLines = 6

// monthSpend is the month's spending, split by person and by agent type. Both splits are of the usage index, the one the
// Usage page reads, so each split adds up to USD and each line's link opens the calls it counts. Split is false on a
// server that keeps no index, which has the total from its ledger and nothing to split it by.
type monthSpend struct {
	USD        float64
	Unmeasured int64
	Split      bool
	Building   bool
	People     []spendLine
	Engines    []spendLine
	// AllUsage is the month on the Usage page.
	AllUsage string
}

// spendLine is one person or agent type and what it spent this month. Budget is the person's own, when they have one.
type spendLine struct {
	Label, Sub, Href string
	USD              float64
	Calls            int64
	Unpriced         bool
	Budget           float64
	Percent          int
	// More is how many people or agent types this line adds up, when it is the rest of a split.
	More int
}

func (s *Server) monthSpend(r *http.Request, month time.Time) (monthSpend, error) {
	if s.usage.store == nil {
		return s.ledgerSpend()
	}
	from := url.Values{"from": {month.Format("2006-01-02")}}
	ctx, filter := r.Context(), usage.Filter{From: month}
	people, err := s.usage.store.Groups(ctx, filter, usage.ByPerson)
	if err != nil {
		return monthSpend{}, err
	}
	engines, err := s.usage.store.Groups(ctx, filter, usage.ByEngine)
	if err != nil {
		return monthSpend{}, err
	}
	spent := monthSpend{Split: true, Building: s.usage.building().Building, AllUsage: "/admin/usage?" + from.Encode()}
	for _, g := range engines {
		spent.USD, spent.Unmeasured = spent.USD+g.USD, spent.Unmeasured+g.Unmeasured
	}
	spent.People = firstLines(s.personLines(people, from), "/admin/people")
	spent.Engines = firstLines(engineLines(engines, from), spent.AllUsage)
	return spent, nil
}

// ledgerSpend is the month's total as the ledger has it, for a server that keeps no index of calls.
func (s *Server) ledgerSpend() (monthSpend, error) {
	ledger, err := s.spending()
	if err != nil {
		return monthSpend{}, err
	}
	total := ledger.Report(ledger.ThisMonth()).Total
	return monthSpend{USD: total.USD, Unmeasured: int64(total.Unmeasured)}, nil
}

// personLines is each person who spent this month, with their budget, and then each member who has spent nothing yet.
func (s *Server) personLines(groups []usage.Group, from url.Values) []spendLine {
	var settings orgsettings.Settings
	if s.orgSettings != nil {
		settings, _ = s.orgSettings.Read()
	}
	seen := map[string]bool{}
	lines := make([]spendLine, 0, len(groups))
	for _, g := range groups {
		seen[strings.ToLower(g.Value)] = true
		line := spendLine{Label: s.askerLabel(g.Value), USD: g.USD, Calls: g.Calls, Unpriced: g.Unmeasured > 0}
		if g.Value != "" {
			line.Href = usageLink(from, "person", g.Value)
		}
		line.withBudget(settings, g.Value)
		lines = append(lines, line)
	}
	for _, member := range s.membersWithout(seen) {
		line := spendLine{Label: s.personName(member)}
		line.withBudget(settings, member)
		lines = append(lines, line)
	}
	return lines
}

func (line *spendLine) withBudget(settings orgsettings.Settings, who string) {
	limit, set := settings.MemberBudget(who)
	if !set || !strings.Contains(who, "@") {
		return
	}
	line.Budget, line.Percent = limit, percentOf(line.USD, limit)
}

// askerLabel is who a line of the index counts against, as a person reads it.
func (s *Server) askerLabel(who string) string {
	if strings.Contains(who, "@") {
		return s.personName(who)
	}
	return askerOf(who)
}

// membersWithout is the organization's members not in seen, in order of name, or nobody where there is no list of them.
func (s *Server) membersWithout(seen map[string]bool) []string {
	if !s.hasRoster() {
		return nil
	}
	list, err := roster.Read(s.rosterPath)
	if err != nil {
		return nil
	}
	var out []string
	for _, member := range list.Members {
		if !seen[strings.ToLower(member.Email)] {
			out = append(out, member.Email)
		}
	}
	sort.Slice(out, func(i, j int) bool { return s.personName(out[i]) < s.personName(out[j]) })
	return out
}

func engineLines(groups []usage.Group, from url.Values) []spendLine {
	lines := make([]spendLine, 0, len(groups))
	for _, g := range groups {
		line := spendLine{Label: g.Value, USD: g.USD, Calls: g.Calls, Unpriced: g.Unmeasured > 0}
		if g.Value == "" {
			line.Label = "Not recorded"
		} else {
			line.Href = usageLink(from, "agent_type", g.Value)
		}
		if g.Agents > 0 {
			line.Sub = fmt.Sprintf("%d %s", g.Agents, plural(int(g.Agents), "agent"))
		}
		lines = append(lines, line)
	}
	return lines
}

// firstLines keeps a split short: past overviewLines, the rest are one line that adds them up, so the split still adds
// up to the month, and that line links to where all of them are.
func firstLines(lines []spendLine, all string) []spendLine {
	if len(lines) <= overviewLines {
		return lines
	}
	rest := spendLine{Href: all, More: len(lines) - overviewLines + 1}
	for _, line := range lines[overviewLines-1:] {
		rest.USD, rest.Calls, rest.Unpriced = rest.USD+line.USD, rest.Calls+line.Calls, rest.Unpriced || line.Unpriced
	}
	rest.Label = fmt.Sprintf("%d more", rest.More)
	return append(lines[:overviewLines-1:overviewLines-1], rest)
}

func usageLink(from url.Values, field, value string) string {
	return "/admin/usage?" + field + "=" + url.QueryEscape(value) + "&" + from.Encode()
}
