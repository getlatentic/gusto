package spend

import (
	"sort"
)

// Report is what was spent in one month, by team, by project and by job. It is
// read from the ledger's own tallies, the ones a team's cap is checked against,
// rather than added up again, so a report and a cap cannot disagree.
type Report struct {
	Month     string `json:"month"`
	Total     Tally  `json:"total"`
	Teams     []Line `json:"teams"`
	Projects  []Line `json:"projects"`
	Harnesses []Line `json:"harnesses"`
	Models    []Line `json:"models"`
	Jobs      []Line `json:"jobs"`
	// Days are in date order rather than ranked, because they are read as a
	// line across the month.
	Days []Line `json:"days"`
}

// Line is one team, project or job and what it spent. An empty name is a job
// that named no team.
type Line struct {
	Name string `json:"name"`
	Tally
}

// Report is the month given, as "2006-01".
func (l *Ledger) Report(month string) Report {
	l.guard.Lock()
	defer l.guard.Unlock()
	report := Report{Month: month}
	into := map[string]*[]Line{
		ByTeam: &report.Teams, ByProject: &report.Projects, ByJob: &report.Jobs,
		ByHarness: &report.Harnesses, ByModel: &report.Models, ByDay: &report.Days,
	}
	for key, tally := range l.months {
		if key.month != month {
			continue
		}
		*into[key.scope] = append(*into[key.scope], Line{Name: key.name, Tally: *tally})
		if key.scope == ByJob {
			report.Total.join(*tally)
		}
	}
	for scope, lines := range into {
		if scope != ByDay {
			ranked(*lines)
		}
	}
	sort.Slice(report.Days, func(i, j int) bool { return report.Days[i].Name < report.Days[j].Name })
	return report
}

// Months is every month something was spent or refused in, newest first.
func (l *Ledger) Months() []string {
	l.guard.Lock()
	defer l.guard.Unlock()
	seen := map[string]bool{}
	for key := range l.months {
		seen[key.month] = true
	}
	months := make([]string, 0, len(seen))
	for month := range seen {
		months = append(months, month)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(months)))
	return months
}

// Job is everything one job has spent, whatever month it was in, and whether
// it reached a model at all.
func (l *Ledger) Job(id string) (Tally, bool) {
	l.guard.Lock()
	defer l.guard.Unlock()
	held, found := l.jobs[id]
	if !found || held.tally.Calls+held.tally.Refused == 0 {
		return Tally{}, false
	}
	return held.tally, true
}

// ThisMonth is the month a team's cap is counting now.
func (l *Ledger) ThisMonth() string { return monthOf(l.now()) }

func (t *Tally) join(other Tally) {
	t.Calls += other.Calls
	t.USD += other.USD
	t.Unmeasured += other.Unmeasured
	t.Uncounted += other.Uncounted
	t.InputTokens += other.InputTokens
	t.CachedTokens += other.CachedTokens
	t.OutputTokens += other.OutputTokens
	t.Refused += other.Refused
}

// ranked puts the largest spend first. What could not be measured ranks next,
// because an unknown amount is a question somebody has to answer.
func ranked(lines []Line) {
	sort.Slice(lines, func(i, j int) bool {
		a, b := lines[i], lines[j]
		if a.USD != b.USD {
			return a.USD > b.USD
		}
		if a.Unmeasured != b.Unmeasured {
			return a.Unmeasured > b.Unmeasured
		}
		return a.Name < b.Name
	})
}
