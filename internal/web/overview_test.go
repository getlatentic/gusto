package web

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/apps"
	"github.com/getlatentic/gusto/internal/changelog"
	"github.com/getlatentic/gusto/internal/computer"
	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/internal/quota"
	"github.com/getlatentic/gusto/pkg/job"
	"github.com/getlatentic/gusto/pkg/spend"
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

// figureSays is the number over a label in the figures across the top.
func figureSays(t *testing.T, page, label string) string {
	t.Helper()
	found := regexp.MustCompile(`<span class="[^"]*">([^<]*)</span>\s*<span class="[^"]*">(?:<span class="[^"]*"></span>)?` + regexp.QuoteMeta(label)).FindStringSubmatch(page)
	if found == nil {
		t.Fatalf("no figure says %q:\n%s", label, page)
	}
	return found[1]
}

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

func TestTheOverviewCountsWhatIsRunningWaitingAndRecentlyFailed(t *testing.T) {
	here := open(t)
	here.signIn(t)
	addJob(t, here, "run-a", jobs.Running, time.Minute)
	addJob(t, here, "wait-a", jobs.Queued, time.Minute)
	addJob(t, here, "wait-b", jobs.Queued, time.Minute)
	addJob(t, here, "bad-a", jobs.Failed, time.Hour)
	addJob(t, here, "bad-b", jobs.Interrupted, 24*time.Hour)
	addJob(t, here, "bad-old", jobs.Failed, 30*24*time.Hour)
	addJob(t, here, "good", jobs.Passed, time.Hour)

	page := overview(t, here)
	for label, want := range map[string]string{"running": "1", "waiting": "2", "failed this week": "2"} {
		if got := figureSays(t, page, label); got != want {
			t.Errorf("%s: want %s, the page says %s", label, want, got)
		}
	}
	if !strings.Contains(page, "up to 1 at once") {
		t.Errorf("the page does not say how many jobs may run at once:\n%s", page)
	}
}

func TestTheOverviewShowsSpendByPersonTeamModelAndRepositoryAgainstTheTeamsCap(t *testing.T) {
	here := open(t)
	here.signIn(t)
	for id, each := range map[string]struct {
		asked, team string
		usd         float64
	}{"one": {"ada@acme.com", "platform", 4}, "two": {"bob@acme.com", "platform", 2}, "three": {"ada@acme.com", "", 1}, "four": {"cy@acme.com", "design", 8}} {
		spentNowIn(t, here, id, each.asked, each.team, each.usd)
	}
	ledger, err := spend.Rebuild(here.index.Root(), spend.Policy{Teams: map[string]spend.TeamCap{
		"platform": {USDPerMonth: 5}, "research": {USDPerMonth: 50}, "design": {USDPerMonth: 8},
	}})
	if err != nil {
		t.Fatal(err)
	}
	here.server.UseLedger(ledger)

	page := overview(t, here)
	for _, want := range []string{"By person", "ada@acme.com", "bob@acme.com", "By team", "platform", "By model", "By repository", "github.com/o/api"} {
		if !strings.Contains(page, want) {
			t.Errorf("the overview lacks %q:\n%s", want, page)
		}
	}
	if row := limitOf(t, page, "Team platform"); !strings.Contains(row, `<span class="font-semibold text-bad">$6.00</span>`) || !strings.Contains(row, "$5.00") {
		t.Errorf("a team over its cap is not shown so: %s", row)
	}
	if row := limitOf(t, page, "Team research"); strings.Contains(row, "text-bad") || !strings.Contains(row, "$0.0000") || !strings.Contains(row, "$50.00") {
		t.Errorf("a team under its cap that spent nothing: %s", row)
	}
	if row := limitOf(t, page, "Team design"); !strings.Contains(row, `<span class="font-semibold text-bad">$8.00</span>`) {
		t.Errorf("a team that has spent exactly its cap has none left: %s", row)
	}
	person := regexp.MustCompile(`(?s)By person.*?</section>`).FindString(page)
	if !strings.Contains(person, "$5.00") || !strings.Contains(person, "$2.00") {
		t.Errorf("each person's spend is not summed: %s", person)
	}
	for range 30 {
		limits := regexp.MustCompile(`(?s)<h2[^>]*>Limits</h2>.*?</table>`).FindString(overview(t, here))
		design, platform, research := strings.Index(limits, "Team design"), strings.Index(limits, "Team platform"), strings.Index(limits, "Team research")
		if !(design < platform && platform < research) {
			t.Fatalf("the teams are not in order:\n%s", limits)
		}
	}
}

// limitOf is the row of the Limits table that begins with what.
func limitOf(t *testing.T, page, what string) string {
	t.Helper()
	found := regexp.MustCompile(`(?s)<tr><td>` + regexp.QuoteMeta(what) + `[^<]*</td>(.*?)</tr>`).FindStringSubmatch(page)
	if found == nil {
		t.Fatalf("no limit is %q:\n%s", what, page)
	}
	return found[1]
}

func TestTheOverviewShowsTheOrganizationsQuotaAndHowMuchOfItIsUsed(t *testing.T) {
	here := open(t)
	path := filepath.Join(t.TempDir(), "quota.json")
	if err := quota.Write(path, quota.Quota{Jobs: 10, Monthly: true, Concurrent: 2, Computers: 3}); err != nil {
		t.Fatal(err)
	}
	here.index.UseQuota(quota.Open(path))
	registry, err := computer.Open(filepath.Join(t.TempDir(), "computers"), "acme-com")
	if err != nil {
		t.Fatal(err)
	}
	for _, person := range []string{"ada@acme.com", "bob@acme.com"} {
		if _, _, err := registry.Ensure(person, "local"); err != nil {
			t.Fatal(err)
		}
	}
	here.server.UseComputers(registry)
	here.signIn(t)
	addJob(t, here, "run-a", jobs.Running, time.Minute)
	addJob(t, here, "done-a", jobs.Passed, time.Minute)
	addJob(t, here, "done-last-month", jobs.Passed, 40*24*time.Hour)

	page := overview(t, here)
	for what, want := range map[string][]string{
		"Tasks started this month": {">2<", ">10<"},
		"Tasks at once":            {">1<", ">2<"},
		"Workspaces":               {">2<", ">3<"},
	} {
		row := limitOf(t, page, what)
		for _, piece := range want {
			if !strings.Contains(row, piece) {
				t.Errorf("the limit %q does not say %s: %s", what, piece, row)
			}
		}
	}
}

func TestALimitThatIsUsedUpIsMarkedSo(t *testing.T) {
	here := open(t)
	path := filepath.Join(t.TempDir(), "quota.json")
	if err := quota.Write(path, quota.Quota{Jobs: 2, Concurrent: 1, Computers: 2}); err != nil {
		t.Fatal(err)
	}
	here.index.UseQuota(quota.Open(path))
	registry, err := computer.Open(filepath.Join(t.TempDir(), "computers"), "acme-com")
	if err != nil {
		t.Fatal(err)
	}
	for _, person := range []string{"ada@acme.com", "bob@acme.com"} {
		if _, _, err := registry.Ensure(person, "local"); err != nil {
			t.Fatal(err)
		}
	}
	here.server.UseComputers(registry)
	here.signIn(t)
	addJob(t, here, "run-a", jobs.Running, time.Minute)
	addJob(t, here, "wait-a", jobs.Queued, time.Minute)

	page := overview(t, here)
	for _, what := range []string{"Tasks started in all", "Tasks at once", "Workspaces"} {
		if row := limitOf(t, page, what); !strings.Contains(row, `text-bad`) {
			t.Errorf("%s is at its limit and is not marked: %s", what, row)
		}
	}
	if row := limitOf(t, page, "Tasks at once"); !strings.Contains(row, ">1<") {
		t.Errorf("a job waiting is not running: %s", row)
	}
}

func TestTheOverviewSaysWhenNothingLimitsTheOrganizationAndWhenTheLimitsCannotBeRead(t *testing.T) {
	plain := open(t)
	plain.signIn(t)
	if page := overview(t, plain); !strings.Contains(page, "Nothing limits this organization.") {
		t.Errorf("a server with no limit:\n%s", page)
	}

	broken := open(t)
	path := filepath.Join(t.TempDir(), "quota.json")
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	broken.index.UseQuota(quota.Open(path))
	broken.signIn(t)
	page := overview(t, broken)
	if !strings.Contains(page, "The limits could not be read.") || strings.Contains(page, "Nothing limits this organization.") {
		t.Errorf("a quota that cannot be read is shown as no limit:\n%s", page)
	}
}

func TestTheOverviewListsComputersAndAppsAndOnlyWhereTheServerKeepsThem(t *testing.T) {
	here := open(t)
	here.signIn(t)
	page := overview(t, here)
	if strings.Contains(page, "<h2>Workspaces</h2>") || strings.Contains(page, "<h2>Apps</h2>") {
		t.Errorf("a server with neither lists them:\n%s", page)
	}

	registry, err := computer.Open(filepath.Join(t.TempDir(), "computers"), "acme-com")
	if err != nil {
		t.Fatal(err)
	}
	ada, _, _ := registry.Ensure("ada@acme.com", "local")
	registry.Change(ada.ID, func(c *computer.Computer) {
		c.State, c.LastActive = computer.Running, time.Now().Add(-3*time.Hour)
	})
	bob, _, _ := registry.Ensure("bob@acme.com", "local")
	registry.Change(bob.ID, func(c *computer.Computer) { c.State, c.Note = computer.Failed, "the gateway said no" })
	here.server.UseComputers(registry)

	shared, err := apps.Open(filepath.Join(t.TempDir(), "apps"))
	if err != nil {
		t.Fatal(err)
	}
	made, err := shared.New("gone", "ada@acme.com", "python -m http.server", 8000)
	if err != nil {
		t.Fatal(err)
	}
	shared.Change(made.ID, func(a *apps.App) { a.State = apps.Live })
	here.server.UseApps(shared, nil)

	page = overview(t, here)
	computers := regexp.MustCompile(`(?s)<h2[^>]*>Workspaces</h2>.*?</section>`).FindString(page)
	for _, want := range []string{"ada@acme.com", `>running</span>`, "3h ago", "bob@acme.com", `>failed</span>`, "the gateway said no", "never"} {
		if !strings.Contains(computers, want) {
			t.Errorf("the computers lack %q: %s", want, computers)
		}
	}
	listed := regexp.MustCompile(`(?s)<h2[^>]*>Apps</h2>.*?</section>`).FindString(page)
	for _, want := range []string{"ada@acme.com", "an agent that was removed", `>live</span>`} {
		if !strings.Contains(listed, want) {
			t.Errorf("the apps lack %q: %s", want, listed)
		}
	}
}

func TestAnAppsAgentIsNamedWhileItIsThere(t *testing.T) {
	here := open(t)
	runner := here.as(t, alice)
	where, _ := started(t, runner)
	shared, err := apps.Open(filepath.Join(t.TempDir(), "apps"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := shared.New(strings.TrimPrefix(where, "/instances/"), alice, "serve", 8000); err != nil {
		t.Fatal(err)
	}
	here.server.UseApps(shared, nil)
	here.server.UseAdmins([]string{boss})
	page := overview(t, here.as(t, boss))
	if strings.Contains(page, "an agent that was removed") || !strings.Contains(page, alice) {
		t.Errorf("an app whose agent is there is not named by it:\n%s", page)
	}
}

// budgeted is a server that keeps a budget, with the change record on and an admin signed in.
func budgeted(t *testing.T) (*site, *changelog.Log, string) {
	t.Helper()
	here, log := recording(t)
	path := filepath.Join(t.TempDir(), "settings.json")
	here.server.UseOrgSettings(path)
	here.signIn(t)
	return here, log, path
}

func setBudget(t *testing.T, here *site, typed string) *http.Response {
	t.Helper()
	return here.ask(t, "POST", "/admin/budget", url.Values{"usd": {typed}}).Result()
}

func TestABudgetIsSetChangedAndClearedAndEachChangeIsRecorded(t *testing.T) {
	here, log, path := budgeted(t)
	if page := overview(t, here); !strings.Contains(page, "No budget is set.") || strings.Contains(page, `role="alert"`) {
		t.Fatalf("with none set:\n%s", page)
	}
	if got := setBudget(t, here, "100"); got.StatusCode != http.StatusSeeOther || got.Header.Get("Location") != "/admin" {
		t.Fatalf("setting it: %d %v", got.StatusCode, got.Header)
	}
	page := overview(t, here)
	if !strings.Contains(page, "$100.00 a month.") || !strings.Contains(page, `value="100.00"`) {
		t.Errorf("the budget is not shown:\n%s", page)
	}
	if limitOf(t, page, "Budget for") == "" {
		t.Error("the budget is not among the limits")
	}
	setBudget(t, here, "100")
	setBudget(t, here, "250.5")
	setBudget(t, here, "")
	if page := overview(t, here); !strings.Contains(page, "No budget is set.") {
		t.Errorf("a budget left empty is not cleared:\n%s", page)
	}
	var said []string
	for _, event := range ofKind(written(t, log), changelog.BudgetSet) {
		if event.Who != "the shared sign-in" || event.Role != "admin" {
			t.Errorf("who set it: %+v", event)
		}
		said = append(said, event.What["was"]+">"+event.What["now"])
	}
	if strings.Join(said, " ") != "none>$100.00 $100.00>$250.50 $250.50>none" {
		t.Errorf("the record has %v, and setting the same figure again is no change", said)
	}
	activity := here.ask(t, "GET", "/admin/activity", nil).Body.String()
	for _, want := range []string{"changed the monthly budget from none to $100.00", "changed the monthly budget from $250.50 to none"} {
		if !strings.Contains(activity, want) {
			t.Errorf("the activity page does not say %q:\n%s", want, activity)
		}
	}
	if text, _ := os.ReadFile(path); strings.Contains(string(text), "250") {
		t.Errorf("the settings were not cleared: %s", text)
	}
}

func TestABudgetIsRoundedToTheCentAndAnythingElseIsRefusedAndChangesNothing(t *testing.T) {
	here, log, _ := budgeted(t)
	setBudget(t, here, "1.999")
	if page := overview(t, here); !strings.Contains(page, "$2.00 a month.") {
		t.Errorf("1.999 is not $2.00:\n%s", page)
	}
	if kept, err := here.server.orgSettings.Read(); err != nil || kept.MonthlyBudgetUSD != 2 {
		t.Errorf("what is kept is to the cent: %v %v", kept.MonthlyBudgetUSD, err)
	}
	for _, typed := range []string{"-5", "abc", "NaN", "Inf", "1e400", "2000000000", "12 dollars", "$40"} {
		got := setBudget(t, here, typed)
		why, _ := url.QueryUnescape(got.Header.Get("Location"))
		if !strings.Contains(why, "the budget is a number of dollars") {
			t.Errorf("%q was accepted or refused for another reason: %d %v", typed, got.StatusCode, got.Header)
		}
	}
	page := overview(t, here)
	if !strings.Contains(page, "$2.00 a month.") {
		t.Errorf("a refused budget changed the budget:\n%s", page)
	}
	refused := here.ask(t, "GET", "/admin?refused="+url.QueryEscape("the budget is a number"), nil).Body.String()
	if !strings.Contains(refused, `>the budget is a number</p>`) {
		t.Errorf("a refusal is not shown:\n%s", refused)
	}
	if n := len(ofKind(written(t, log), changelog.BudgetSet)); n != 1 {
		t.Errorf("%d changes were recorded, and one was made", n)
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
	if row := limitOf(t, page, "Budget for"); !strings.Contains(row, `text-bad">$130.50`) {
		t.Errorf("the budget's row does not say it is used up: %s", row)
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

func TestABudgetThatCannotBeReadIsSaidAndNotCheckedOrWrittenOver(t *testing.T) {
	here, _, path := budgeted(t)
	spentNow(t, here, "big", "ada@acme.com", 500)
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	page := overview(t, here)
	if !strings.Contains(page, "The budget could not be read, so it is not checked.") || strings.Contains(page, `role="alert"`) {
		t.Errorf("a budget that cannot be read:\n%s", page)
	}
	got := setBudget(t, here, "1")
	if !strings.Contains(got.Header.Get("Location"), "refused=") {
		t.Errorf("a damaged file was written to: %d %v", got.StatusCode, got.Header)
	}
	if text, _ := os.ReadFile(path); string(text) != "{" {
		t.Errorf("the damaged file was written over: %q", text)
	}
}

func TestABudgetIsKeptAcrossARestart(t *testing.T) {
	here, _, path := budgeted(t)
	setBudget(t, here, "75")
	again := open(t)
	again.server.UseOrgSettings(path)
	again.signIn(t)
	if page := overview(t, again); !strings.Contains(page, "$75.00 a month.") {
		t.Errorf("a server that started after the budget was set does not have it:\n%s", page)
	}
}

func TestThereIsNoBudgetWhereTheServerKeepsNoSettings(t *testing.T) {
	here := open(t)
	here.signIn(t)
	if page := overview(t, here); strings.Contains(page, "<h2>Budget</h2>") {
		t.Errorf("a server that cannot keep a budget offers one:\n%s", page)
	}
	if got := setBudget(t, here, "5"); got.StatusCode != http.StatusNotFound {
		t.Errorf("setting a budget where none is kept: %d", got.StatusCode)
	}
}

func setRunTokens(t *testing.T, here *site, bound string) *http.Response {
	t.Helper()
	return here.ask(t, "POST", "/admin/run-tokens", url.Values{"bound": {bound}}).Result()
}

func TestRunsAreBoundUntilAnAdminChoosesPlainTokensAndEachChangeIsRecorded(t *testing.T) {
	here, log, _ := budgeted(t)
	page := overview(t, here)
	if !strings.Contains(page, "bound to its sandbox") || !strings.Contains(page, "Use plain tokens") {
		t.Fatal("an organization's runs are not shown bound before anyone chose")
	}
	if got := setRunTokens(t, here, "no"); got.StatusCode != http.StatusSeeOther {
		t.Fatalf("choosing plain tokens gave %d", got.StatusCode)
	}
	if kept, err := here.server.orgSettings.Read(); err != nil || !kept.PlainRunTokens {
		t.Fatalf("plain tokens were not kept: %+v %v", kept, err)
	}
	if page := overview(t, here); !strings.Contains(page, "Bind to the sandbox") {
		t.Error("the overview does not offer to bind runs again")
	}
	setRunTokens(t, here, "no")
	setRunTokens(t, here, "yes")
	if got := setRunTokens(t, here, "maybe"); got.Header.Get("Location") == "/admin" {
		t.Error("a choice that is neither yes nor no was taken")
	}
	var said []string
	for _, event := range ofKind(written(t, log), changelog.RunTokensSet) {
		said = append(said, event.What["was"]+">"+event.What["now"])
	}
	if strings.Join(said, " ") != "bound>plain plain>bound" {
		t.Errorf("the activity says %v", said)
	}
}
