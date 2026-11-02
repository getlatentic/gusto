package web

import (
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/internal/roster"
	"github.com/getlatentic/gusto/internal/usage"
	"github.com/getlatentic/gusto/internal/usage/usagetest"
	"github.com/getlatentic/gusto/pkg/job"
)

// overview is the Overview as an admin reads it.
func overview(t *testing.T, here *site) string {
	t.Helper()
	answer := here.ask(t, "GET", "/admin", nil)
	if answer.Code != http.StatusOK {
		t.Fatalf("the overview answered %d", answer.Code)
	}
	return answer.Body.String()
}

// mainOf is the page without the console's own side, which has the form that signs out.
func mainOf(page string) string { return page[strings.Index(page, `<main`):] }

func addJob(t *testing.T, here *site, id string, state jobs.State, age time.Duration) {
	t.Helper()
	if err := job.Dir(filepath.Join(here.index.Root(), id)).Make(); err != nil {
		t.Fatal(err)
	}
	queued := time.Now().Add(-age).Unix()
	if err := here.index.Add(jobs.Record{ID: id, State: state, Brief: id, Harness: "fixture", Queued: queued, Started: queued, Ended: queued}); err != nil {
		t.Fatal(err)
	}
}

// callNow is a call made this month by a person through an agent type, which cost usd, or had no price when usd is below 0.
func callNow(task, person, engine, agent string, usd float64) usage.Row {
	row := usage.Row{Task: task, At: time.Now(), Session: task, Person: person, Engine: engine, Agent: agent, Model: "m"}
	if usd >= 0 {
		row.USD = &usd
	}
	return row
}

// withMonth is a server behind a front door, with its people, budgets and an index of model calls holding the rows given.
func withMonth(t *testing.T, members []roster.Member, rows ...usage.Row) *site {
	t.Helper()
	here, _ := withPeople(t, roster.Roster{Members: members})
	here.server.UseOrgSettings(filepath.Join(t.TempDir(), "settings.json"))
	store := usagetest.New()
	if err := store.Add(t.Context(), rows); err != nil {
		t.Fatal(err)
	}
	here.server.SetUsage(store)
	return here
}

func TestTheOverviewOnlyReadsAndHasNoForm(t *testing.T) {
	here, _, _ := budgeted(t)
	addJob(t, here, "bad-a", jobs.Failed, time.Hour)
	page := mainOf(overview(t, here))
	if strings.Contains(page, "<form") {
		t.Errorf("the overview has a form:\n%s", page)
	}
	for _, gone := range []string{"failed this week", "Limits", "New chats", "Model access in sandboxes", "Each member"} {
		if strings.Contains(page, gone) {
			t.Errorf("the overview still has %q", gone)
		}
	}
	if !strings.Contains(page, "Spent this month") || !strings.Contains(page, "no budget set") {
		t.Errorf("the overview does not say what was spent against what:\n%s", page)
	}
}

func TestTheOverviewSplitsTheMonthByPersonAndByAgentTypeAndEachSplitAddsUp(t *testing.T) {
	members := []roster.Member{{Email: "ada@acme.com", Role: "admin"}, {Email: "bob@acme.com", Role: "runner"}, {Email: "cy@acme.com", Role: "runner"}}
	lastMonth := callNow("old", "cy@acme.com", "Codex", "", 40)
	lastMonth.At = monthStart(time.Now()).Add(-time.Hour)
	here := withMonth(t, members,
		callNow("a1", "ada@acme.com", "Claude Code", "Iris", 1.5),
		callNow("a2", "ada@acme.com", "Claude Code", "Nova", 0.5),
		callNow("b1", "bob@acme.com", "Codex", "Iris", 1),
		callNow("b2", "bob@acme.com", "Codex", "", -1),
		lastMonth,
	)
	here.ask(t, "POST", "/admin/budget", url.Values{"usd": {"50"}, "each": {"10"}})
	// The split by person is of the ledger, by who asked: ada asked for $2, bob for $1 and a call with no price, and one
	// task named nobody.
	spentNow(t, here, "ada-1", "ada@acme.com", 1.5)
	spentNow(t, here, "ada-2", "ada@acme.com", 0.5)
	spentNow(t, here, "bob-1", "bob@acme.com", 1)
	spentJobFor(t, here.index.Root(), "nobody-1", "", "", fmt.Sprintf(`{"path":"/v1/chat/completions","status":200,"atUnix":%d,"model":"m","inputTokens":1,"outputTokens":1,"usd":0.25}`, time.Now().Unix()))
	page := overview(t, here)
	from := "from=" + monthStart(time.Now()).Format("2006-01-02")

	if !strings.Contains(page, ">$3.00</span>") || !strings.Contains(page, "of $50.00") {
		t.Errorf("the month's total is not the sum of its calls against the budget:\n%s", page)
	}
	if strings.Contains(page, "$40") {
		t.Error("last month's calls are counted in this month")
	}
	person := regexp.MustCompile(`(?s)By person, what they asked for</h2>.*?</section>`).FindString(page)
	for _, want := range []string{
		">Ada</span>", "$2.00", "of $10.00", ">Bob</span>", "$1.00", ">No one named</span>", "$0.25",
		">Cy</span>", "nothing yet", `href="/admin/budgets">View all`,
	} {
		if !strings.Contains(person, want) {
			t.Errorf("By person lacks %q:\n%s", want, person)
		}
	}
	if strings.Index(person, ">Ada<") > strings.Index(person, ">Bob<") || strings.Index(person, ">Bob<") > strings.Index(person, ">No one named<") ||
		strings.Index(person, ">No one named<") > strings.Index(person, "nothing yet") {
		t.Errorf("the people are not in order of what they asked for, then nobody, then those who asked for nothing:\n%s", person)
	}
	engines := regexp.MustCompile(`(?s)By agent type</h2>.*?</section>`).FindString(page)
	for _, want := range []string{
		`href="/admin/usage?agent_type=Claude&#43;Code&amp;` + from + `"`, "Claude Code <span class=\"text-subtle\">· 2 agents</span>", "$2.00",
		`href="/admin/usage?agent_type=Codex&amp;` + from + `"`, "Codex <span class=\"text-subtle\">· 1 agent</span>", "$1.00",
		`href="/admin/usage?` + from + `">View all`,
	} {
		if !strings.Contains(engines, want) {
			t.Errorf("By agent type lacks %q:\n%s", want, engines)
		}
	}
	if !strings.Contains(page, "1 call has no price") {
		t.Errorf("a call with no price is not said:\n%s", page)
	}
}

func TestASplitWithManyLinesAddsUpTheRestAsOne(t *testing.T) {
	var lines []spendLine
	for i := range 9 {
		lines = append(lines, spendLine{Label: string(rune('a' + i)), USD: 1, Calls: 1})
	}
	kept := firstLines(lines, "/admin/people")
	if len(kept) != overviewLines {
		t.Fatalf("%d lines are kept", len(kept))
	}
	rest := kept[len(kept)-1]
	if rest.Label != "4 more" || rest.USD != 4 || rest.Href != "/admin/people" {
		t.Errorf("the rest is %+v", rest)
	}
	total := 0.0
	for _, line := range kept {
		total += line.USD
	}
	if total != 9 || lines[overviewLines-1].Label != "f" {
		t.Errorf("the kept lines add up to %v, and the lines given were changed: %+v", total, lines)
	}
}

func TestTheOverviewSaysWhenTheMonthsSpendingHasPassedTheBudget(t *testing.T) {
	here, _, _ := budgeted(t)
	setBudget(t, here, "100")
	spentNow(t, here, "first", "ada@acme.com", 60)
	if page := overview(t, here); strings.Contains(page, `role="alert"`) {
		t.Errorf("60 of 100 is not passed:\n%s", page)
	}
	spentNow(t, here, "second", "bob@acme.com", 40)
	if page := overview(t, here); strings.Contains(page, `role="alert"`) {
		t.Errorf("spending what the budget is has not passed it:\n%s", page)
	}
	spentNow(t, here, "third", "ada@acme.com", 30.5)
	page := overview(t, here)
	alert := regexp.MustCompile(`(?s)<p class="[^"]*" role="alert">(.*?)</p>`).FindStringSubmatch(page)
	if alert == nil {
		t.Fatalf("130.50 of 100 is passed and the page does not say:\n%s", page)
	}
	for _, want := range []string{"budget for", "is $100.00", "$130.50 has been spent", "passed by $30.50"} {
		if !strings.Contains(alert[1], want) {
			t.Errorf("the alert lacks %q: %s", want, alert[1])
		}
	}
	setBudget(t, here, "200")
	if page := overview(t, here); strings.Contains(page, `role="alert"`) {
		t.Errorf("a budget that was raised is still passed:\n%s", page)
	}
	setBudget(t, here, "")
	if page := overview(t, here); strings.Contains(page, `role="alert"`) {
		t.Errorf("no budget is never passed:\n%s", page)
	}
}

func TestSpendingThatHasNoPriceMakesTheBudgetPassedByAtLeast(t *testing.T) {
	here, _, _ := budgeted(t)
	setBudget(t, here, "10")
	spentNow(t, here, "priced", "ada@acme.com", 12)
	spentJob(t, here.index.Root(), "unpriced", "", unmeasuredNow())
	page := overview(t, here)
	alert := regexp.MustCompile(`(?s)<p class="[^"]*" role="alert">(.*?)</p>`).FindStringSubmatch(page)
	if alert == nil || !strings.Contains(alert[1], "more that has no price") || !strings.Contains(alert[1], "at least") {
		t.Errorf("what could not be priced is not said:\n%s", page)
	}
}

func TestTheOverviewSaysWhenTheBudgetCannotBeRead(t *testing.T) {
	here, _, path := budgeted(t)
	spentNow(t, here, "big", "ada@acme.com", 500)
	damage(t, path)
	page := overview(t, here)
	if !strings.Contains(page, "The budget could not be read, so it is not checked.") || strings.Contains(page, `role="alert"`) {
		t.Errorf("a budget that cannot be read:\n%s", page)
	}
}
