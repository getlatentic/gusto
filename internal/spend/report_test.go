package spend

import (
	"bytes"
	"strings"
	"testing"
)

func reportedLedger() *Ledger {
	ledger := fixedLedger(september)
	ledger.Begin("fix-a", Terms{Team: "platform", Project: "github.com/o/api"})
	ledger.Begin("fix-b", Terms{Team: "platform", Project: "github.com/o/web"})
	ledger.Begin("lone", Terms{Project: "github.com/o/api"})
	ledger.Count("fix-a", answered(0.75, september))
	ledger.Count("fix-b", answered(0.5, september))
	ledger.Count("lone", Call{Path: "/v1/chat/completions", Status: 200, AtUnix: september.Unix(), Model: "unpriced"})
	ledger.Count("lone", answered(0.1, september.AddDate(0, -1, 0)))
	return ledger
}

func lineOf(t *testing.T, lines []Line, name string) Line {
	t.Helper()
	for _, line := range lines {
		if line.Name == name {
			return line
		}
	}
	t.Fatalf("no line for %q in %+v", name, lines)
	return Line{}
}

func TestTheReportIsByTeamProjectAndJob(t *testing.T) {
	report := reportedLedger().Report("2026-09")
	if team := lineOf(t, report.Teams, "platform"); team.USD != 1.25 || team.Calls != 2 {
		t.Fatalf("platform %+v", team)
	}
	if project := lineOf(t, report.Projects, "github.com/o/api"); project.USD != 0.75 || project.Unmeasured != 1 {
		t.Fatalf("api %+v", project)
	}
	if job := lineOf(t, report.Jobs, "fix-b"); job.USD != 0.5 {
		t.Fatalf("fix-b %+v", job)
	}
	if report.Jobs[0].Name != "fix-a" {
		t.Fatalf("the largest spend should come first: %+v", report.Jobs)
	}
	if report.Total.USD != 1.25 || report.Total.Calls != 3 || report.Total.Unmeasured != 1 {
		t.Fatalf("total %+v", report.Total)
	}
}

// The team cap reads the tally the report shows, so the two cannot differ.
func TestTheReportAndTheCapReadTheSameNumber(t *testing.T) {
	ledger := reportedLedger()
	ledger.Count("fix-a", answered(0.5, september))
	refusal := ledger.Allow("fix-a", "fixture-model")
	team := lineOf(t, ledger.Report("2026-09").Teams, "platform")
	if refusal == nil || refusal.SpentUSD != team.USD {
		t.Fatalf("the cap saw %+v and the report %+v", refusal, team)
	}
}

func TestAMonthIsReportedOnItsOwn(t *testing.T) {
	ledger := reportedLedger()
	august := ledger.Report("2026-08")
	if len(august.Jobs) != 1 || august.Total.USD != 0.1 {
		t.Fatalf("august %+v", august)
	}
	if months := ledger.Months(); strings.Join(months, " ") != "2026-09 2026-08" {
		t.Fatalf("months %v", months)
	}
}

func TestUnmeasuredSpendNeverReadsAsFree(t *testing.T) {
	for _, tc := range []struct {
		tally Tally
		want  string
	}{
		{Tally{Calls: 2, USD: 1.5}, "$1.5000"},
		{Tally{Calls: 2, USD: 1.5, Unmeasured: 1}, "at least $1.5000"},
		{Tally{Calls: 2, Unmeasured: 2}, "unmeasured"},
	} {
		if got := tc.tally.Money(); got != tc.want {
			t.Fatalf("%+v reads %q, want %q", tc.tally, got, tc.want)
		}
	}
	var text bytes.Buffer
	if err := reportedLedger().Report("2026-09").WriteText(&text); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"at least $1.2500", "(none)", "github.com/o/web", "fix-a"} {
		if !strings.Contains(text.String(), want) {
			t.Fatalf("want %q in\n%s", want, text.String())
		}
	}
}
