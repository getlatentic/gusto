package web

import (
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/roster"
	"github.com/getlatentic/gusto/internal/tasks"
	"github.com/getlatentic/gusto/internal/usage"
	"github.com/getlatentic/gusto/internal/usage/usagetest"
	"github.com/getlatentic/gusto/pkg/task"
)

// overview is the Overview's data as the console's page reads it.
func overview(t *testing.T, here *site) overviewJSON {
	t.Helper()
	var page overviewJSON
	here.data(t, "/admin", &page)
	return page
}

type overviewJSON struct {
	MonthName   string
	PeopleAll   string
	BudgetsPage bool
	Budget      struct {
		Set         bool
		Limit, Over float64
		Passed      bool
		Err         string
	}
	Spent struct {
		USD             float64
		Unmeasured      int64
		Split, Building bool
		People, Engines []spendLine
		AllUsage        string
	}
}

func addTask(t *testing.T, here *site, id string, state tasks.State, age time.Duration) {
	t.Helper()
	if err := task.Dir(filepath.Join(here.index.Root(), id)).Make(); err != nil {
		t.Fatal(err)
	}
	queued := time.Now().Add(-age).Unix()
	if err := here.index.Add(tasks.Record{ID: id, State: state, Brief: id, Harness: "fixture", Queued: queued, Started: queued, Ended: queued}); err != nil {
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

// withMonth is a server behind gusto cloud, with its people, budgets and an index of model calls holding the rows given.
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

func TestTheOverviewIsTheMonthsSpendAgainstTheBudgetAndNothingElse(t *testing.T) {
	here, _, _ := budgeted(t)
	addTask(t, here, "bad-a", tasks.Failed, time.Hour)
	text := here.dataText(t, "/admin")
	for _, gone := range []string{"failed this week", "Limits", "New chats", "Model access in sandboxes", "Each member"} {
		if strings.Contains(text, gone) {
			t.Errorf("the overview still has %q", gone)
		}
	}
	if page := overview(t, here); page.Budget.Set || page.Spent.USD != 0 {
		t.Errorf("the overview of a month with no budget and no spending says %+v", page)
	}
	if got := here.ask(t, "POST", "/admin", nil); got.Code != http.StatusMethodNotAllowed {
		t.Errorf("the overview takes a form: %d", got.Code)
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
	spentTaskFor(t, here.index.Root(), "nobody-1", "", "", fmt.Sprintf(`{"path":"/v1/chat/completions","status":200,"atUnix":%d,"model":"m","inputTokens":1,"outputTokens":1,"usd":0.25}`, time.Now().Unix()))
	page := overview(t, here)
	from := "from=" + monthStart(time.Now()).Format("2006-01-02")

	if page.Spent.USD != 3 || page.Budget.Limit != 50 {
		t.Errorf("the month's total is not the sum of its calls against the budget: %+v", page)
	}
	var people []string
	for _, line := range page.Spent.People {
		people = append(people, fmt.Sprintf("%s %.2f/%.2f", line.Label, line.USD, line.Budget))
	}
	if want := "Ada 2.00/10.00, Bob 1.00/10.00, No one named 0.25/0.00, Cy 0.00/10.00"; strings.Join(people, ", ") != want {
		t.Errorf("By person is\n%s\nand not, in order of what each asked for, then nobody, then those who asked for nothing,\n%s", strings.Join(people, ", "), want)
	}
	if page.PeopleAll != "/admin/budgets" {
		t.Errorf("By person's View all goes to %q", page.PeopleAll)
	}
	var engines []string
	for _, line := range page.Spent.Engines {
		engines = append(engines, fmt.Sprintf("%s · %s %.2f %s", line.Label, line.Sub, line.USD, line.Href))
	}
	want := []string{
		"Claude Code · 2 agents 2.00 /admin/usage?agent_type=Claude+Code&" + from,
		"Codex · 1 agent 1.00 /admin/usage?agent_type=Codex&" + from,
	}
	if strings.Join(engines, "\n") != strings.Join(want, "\n") {
		t.Errorf("By agent type is\n%s\nand not\n%s", strings.Join(engines, "\n"), strings.Join(want, "\n"))
	}
	if page.Spent.AllUsage != "/admin/usage?"+from {
		t.Errorf("By agent type's View all goes to %q", page.Spent.AllUsage)
	}
	if page.Spent.Unmeasured != 1 {
		t.Errorf("the call with no price is counted as %d", page.Spent.Unmeasured)
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
	if page := overview(t, here); page.Budget.Passed {
		t.Errorf("60 of 100 is passed: %+v", page.Budget)
	}
	spentNow(t, here, "second", "bob@acme.com", 40)
	if page := overview(t, here); !page.Budget.Passed || page.Budget.Over != 0 {
		t.Errorf("spending what the budget is does not reach it: %+v", page.Budget)
	}
	spentNow(t, here, "third", "ada@acme.com", 30.5)
	if page := overview(t, here); !page.Budget.Passed || page.Budget.Limit != 100 || page.Spent.USD != 130.5 || page.Budget.Over != 30.5 {
		t.Errorf("130.50 of 100 is not passed by 30.50: %+v %+v", page.Budget, page.Spent.USD)
	}
	setBudget(t, here, "200")
	if page := overview(t, here); page.Budget.Passed {
		t.Errorf("a budget that was raised is still passed: %+v", page.Budget)
	}
	setBudget(t, here, "")
	if page := overview(t, here); page.Budget.Passed {
		t.Errorf("no budget is never passed: %+v", page.Budget)
	}
}

func TestSpendingThatHasNoPriceMakesTheBudgetPassedByAtLeast(t *testing.T) {
	here, _, _ := budgeted(t)
	setBudget(t, here, "10")
	spentNow(t, here, "priced", "ada@acme.com", 12)
	spentTask(t, here.index.Root(), "unpriced", "", unmeasuredNow())
	if page := overview(t, here); !page.Budget.Passed || page.Spent.Unmeasured == 0 {
		t.Errorf("what could not be priced is not said: %+v %+v", page.Budget, page.Spent)
	}
}

func TestTheOverviewSaysWhenTheBudgetCannotBeRead(t *testing.T) {
	here, _, path := budgeted(t)
	spentNow(t, here, "big", "ada@acme.com", 500)
	damage(t, path)
	if page := overview(t, here); page.Budget.Err == "" || page.Budget.Passed {
		t.Errorf("a budget that cannot be read: %+v", page.Budget)
	}
}
