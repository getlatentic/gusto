package web

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/getlatentic/haas/internal/spend"
)

// ledgerView is a month of the ledger as a dashboard: the figures, the month
// day by day, and where the spending went.
type ledgerView struct {
	spend.Report
	Months []string
	// Measure is what the bars and shares are drawn in: money when any of the
	// month was priced, and tokens when none was, because a chart of zeros
	// says nothing and the tokens are still the provider's own count.
	Measure   string
	Daily     []bar
	Models    []share
	Harnesses []share
	Agents    []share
	Teams     []share
	Projects  []share
	Jobs      []share
}

// bar is one day of the month.
type bar struct {
	Date   string
	Label  string
	Height float64
	Says   string
}

// share is one line of a breakdown and how much of the largest line it is.
// Label and Sub are what a person reads for a line whose name is an id, and
// Href is where the line leads.
type share struct {
	spend.Line
	Width float64
	Label string
	Sub   string
	Href  string
}

// topJobs is how many jobs the dashboard lists. The month's whole list is in
// the JSON report and the ledger; the page is for finding the expensive ones.
const topJobs = 10

// plainly merges a breakdown's lines under the names a person reads: two
// credentials serving one model are one model, and a request that asked no
// model anything is one of the other requests.
func plainly(lines []spend.Line, name func(string) string) []spend.Line {
	merged := map[string]*spend.Line{}
	var order []string
	for _, line := range lines {
		named := name(line.Name)
		held, found := merged[named]
		if !found {
			held = &spend.Line{Name: named}
			merged[named] = held
			order = append(order, named)
		}
		held.Tally.Add(line.Tally)
	}
	out := make([]spend.Line, 0, len(order))
	for _, named := range order {
		out = append(out, *merged[named])
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].USD > out[b].USD })
	return out
}

// modelOf is a ledger's model line by the model's plain name.
func modelOf(line string) string {
	if strings.Contains(line, "/v1/") || strings.HasPrefix(line, "/") {
		return "Other requests"
	}
	if i := strings.LastIndex(line, "/"); i >= 0 {
		line = line[i+1:]
	}
	return modelName(line)
}

func (s *Server) viewLedger(report spend.Report, months []string, now time.Time) ledgerView {
	report.Models = plainly(report.Models, modelOf)
	report.Harnesses = plainly(report.Harnesses, s.engineName)
	view := ledgerView{Report: report, Months: months, Measure: "tokens"}
	if report.Total.USD > 0 {
		view.Measure = "money"
	}
	view.Daily = daysOf(report, view.Measure, now)
	view.Models = shares(report.Models, view.Measure)
	view.Harnesses = shares(report.Harnesses, view.Measure)
	view.Agents = shares(s.byAgent(report.Jobs), view.Measure)
	view.Teams = anyNamed(shares(report.Teams, view.Measure))
	view.Projects = anyNamed(shares(report.Projects, view.Measure))
	jobs := report.Jobs
	if len(jobs) > topJobs {
		jobs = jobs[:topJobs]
	}
	view.Jobs = s.labelled(shares(jobs, view.Measure))
	return view
}

func amount(tally spend.Tally, measure string) float64 {
	if measure == "money" {
		return tally.USD
	}
	return float64(tally.InputTokens + tally.OutputTokens)
}

// daysOf is every day of the month up to today, the quiet ones too, since a
// day with no bar is a day nothing ran and the chart should say so.
func daysOf(report spend.Report, measure string, now time.Time) []bar {
	first, err := time.Parse("2006-01", report.Month)
	if err != nil {
		return nil
	}
	spent := map[string]spend.Tally{}
	for _, line := range report.Days {
		spent[line.Name] = line.Tally
	}
	var days []bar
	most := 0.0
	for day := first; day.Month() == first.Month() && !day.After(now); day = day.AddDate(0, 0, 1) {
		date := day.Format("2006-01-02")
		days = append(days, bar{Date: date, Label: day.Format("2")})
		most = max(most, amount(spent[date], measure))
	}
	for i := range days {
		tally := spent[days[i].Date]
		if most > 0 {
			days[i].Height = 100 * amount(tally, measure) / most
		}
		days[i].Says = fmt.Sprintf("%s: %s, %d calls, %s tokens", days[i].Date, tally.Money(), tally.Calls,
			compact(tally.InputTokens+tally.OutputTokens))
	}
	return days
}

func shares(lines []spend.Line, measure string) []share {
	most := 0.0
	for _, line := range lines {
		most = max(most, amount(line.Tally, measure))
	}
	out := make([]share, 0, len(lines))
	for _, line := range lines {
		width := 0.0
		if most > 0 {
			width = 100 * amount(line.Tally, measure) / most
		}
		out = append(out, share{Line: line, Width: width})
	}
	return out
}

// compact is a count of tokens as a person reads one: 950, 12.4k, 3.1M.
func compact(count int64) string {
	switch {
	case count >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(count)/1e6)
	case count >= 10_000:
		return fmt.Sprintf("%.0fk", float64(count)/1e3)
	case count >= 1_000:
		return fmt.Sprintf("%.1fk", float64(count)/1e3)
	}
	return fmt.Sprint(count)
}
