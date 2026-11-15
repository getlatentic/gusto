package web

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/changelog"
	"github.com/getlatentic/gusto/internal/roster"
)

// budgeted is a server that keeps a budget and a list of people, with the change record on and an admin signed in.
func budgeted(t *testing.T) (*site, *changelog.Log, string) {
	t.Helper()
	here, log := recording(t)
	path := filepath.Join(t.TempDir(), "settings.json")
	here.server.UseOrgSettings(path)
	members := filepath.Join(t.TempDir(), "members.json")
	if err := roster.Write(members, roster.Roster{Members: []roster.Member{{Email: alice, Role: "admin"}, {Email: bob, Role: "runner"}}}); err != nil {
		t.Fatal(err)
	}
	here.server.UseRoster(members)
	here.signIn(t)
	return here, log, path
}

func setBudget(t *testing.T, here *site, typed string) *http.Response {
	t.Helper()
	return here.ask(t, "POST", "/admin/budget", url.Values{"usd": {typed}}).Result()
}

// budgetsJSON is the Budgets page's data.
type budgetsJSON struct {
	Budget                               budgetState
	People                               []budgetRow
	Pages                                pager
	Listed                               bool
	Refused, Unreadable, SpendUnreadable string
}

func budgets(t *testing.T, here *site) budgetsJSON {
	t.Helper()
	var page budgetsJSON
	here.data(t, budgetsPath, &page)
	return page
}

// budgetRowOf is one person's row of Budgets, by their address.
func budgetRowOf(t *testing.T, page budgetsJSON, email string) budgetRow {
	t.Helper()
	for _, each := range page.People {
		if each.Email == email {
			return each
		}
	}
	t.Fatalf("Budgets has no row for %s: %+v", email, page.People)
	return budgetRow{}
}

func damage(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestBudgetsSaysBothBudgetsAndOneFormSavesBothAndRecordsEachChange(t *testing.T) {
	here, log, _ := budgeted(t)
	if page := budgets(t, here); page.Budget.Set || page.Budget.MemberSet || !page.Budget.Editable {
		t.Errorf("with no budgets, Budgets says %+v", page.Budget)
	}
	got := here.ask(t, "POST", "/admin/budget", url.Values{"usd": {"50"}, "each": {"10"}}).Result()
	if got.StatusCode != http.StatusSeeOther || got.Header.Get("Location") != budgetsPath {
		t.Fatalf("saving both: %d %v", got.StatusCode, got.Header)
	}
	if page := budgets(t, here); !page.Budget.Set || page.Budget.Limit != 50 || !page.Budget.MemberSet || page.Budget.Member != 10 {
		t.Errorf("both budgets are not said: %+v", page.Budget)
	}
	if kept, err := here.server.orgSettings.Read(); err != nil || kept.MonthlyBudgetUSD != 50 || kept.MemberBudgetUSD != 10 {
		t.Errorf("kept %+v %v", kept, err)
	}
	here.ask(t, "POST", "/admin/budget", url.Values{"usd": {"50"}, "each": {""}})
	events := written(t, log)
	if org := ofKind(events, changelog.BudgetSet); len(org) != 1 || org[0].What["now"] != "$50.00" {
		t.Errorf("the organization's budget was recorded as %+v", org)
	}
	var each []string
	for _, event := range ofKind(events, changelog.MemberBudgetSet) {
		each = append(each, event.What["whose"]+":"+event.What["was"]+">"+event.What["now"])
	}
	if strings.Join(each, " ") != ":none>$10.00 :$10.00>none" {
		t.Errorf("each person's budget was recorded as %v", each)
	}
}

func TestABudgetIsSetChangedAndClearedAndEachChangeIsRecorded(t *testing.T) {
	here, log, path := budgeted(t)
	setBudget(t, here, "100")
	if page := budgets(t, here); !page.Budget.Set || page.Budget.Limit != 100 {
		t.Errorf("the budget is not shown: %+v", page.Budget)
	}
	if page := overview(t, here); !page.Budget.Set || page.Budget.Limit != 100 {
		t.Errorf("the overview does not measure the month against the budget: %+v", page.Budget)
	}
	setBudget(t, here, "100")
	setBudget(t, here, "250.5")
	setBudget(t, here, "")
	if page := budgets(t, here); page.Budget.Set {
		t.Errorf("a budget left empty is not cleared: %+v", page.Budget)
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
	if n := len(ofKind(written(t, log), changelog.MemberBudgetSet)); n != 0 {
		t.Errorf("a form with only the organization's budget changed each person's %d times", n)
	}
	activity := here.ask(t, "GET", "/admin/records", nil).Body.String()
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
	if kept, err := here.server.orgSettings.Read(); err != nil || kept.MonthlyBudgetUSD != 2 {
		t.Errorf("what is kept is to the cent: %v %v", kept.MonthlyBudgetUSD, err)
	}
	for _, typed := range []string{"-5", "abc", "NaN", "Inf", "1e400", "2000000000", "12 dollars", "$40"} {
		for _, form := range []url.Values{{"usd": {typed}}, {"usd": {"2"}, "each": {typed}}} {
			got := here.ask(t, "POST", "/admin/budget", form).Result()
			why, _ := url.QueryUnescape(got.Header.Get("Location"))
			if !strings.HasPrefix(why, budgetsPath+"?refused=") || !strings.Contains(why, "the budget is a number of dollars") {
				t.Errorf("%v was accepted or refused for another reason: %d %v", form, got.StatusCode, got.Header)
			}
		}
	}
	if kept, _ := here.server.orgSettings.Read(); kept.MonthlyBudgetUSD != 2 || kept.MemberBudgetUSD != 0 {
		t.Errorf("a refused budget changed the budgets: %+v", kept)
	}
	var refused budgetsJSON
	here.data(t, here.server.withMessage(budgetsPath, "refused", "the budget is a number"), &refused)
	if refused.Refused != "the budget is a number" {
		t.Errorf("a refusal is not given: %q", refused.Refused)
	}
	if n := len(ofKind(written(t, log), changelog.BudgetSet)); n != 1 {
		t.Errorf("%d changes were recorded, and one was made", n)
	}
}

func TestABudgetThatCannotBeReadIsSaidAndNotWrittenOver(t *testing.T) {
	here, _, path := budgeted(t)
	damage(t, path)
	if page := budgets(t, here); page.Budget.Err == "" {
		t.Errorf("Budgets does not say the budgets cannot be read: %+v", page.Budget)
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
	if page := overview(t, again); page.Budget.Limit != 75 {
		t.Errorf("a server that started after the budget was set does not have it: %+v", page.Budget)
	}
}

func TestThereIsNoBudgetWhereTheServerKeepsNoSettings(t *testing.T) {
	here, _ := withPeople(t, roster.Roster{Members: []roster.Member{{Email: "ada@acme.com", Role: "admin"}}})
	if page := people(t, here, ""); page.Budgets {
		t.Errorf("a server that cannot keep a budget offers one: %+v", page)
	}
	if got := here.ask(t, "GET", budgetsPath, nil); got.Code != http.StatusNotFound {
		t.Errorf("Budgets where none is kept: %d", got.Code)
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
	if !here.server.runTokensOf().Bound {
		t.Fatal("an organization's runs are not bound before anyone chose")
	}
	if got := setRunTokens(t, here, "no"); got.StatusCode != http.StatusSeeOther {
		t.Fatalf("choosing plain tokens gave %d", got.StatusCode)
	}
	if kept, err := here.server.orgSettings.Read(); err != nil || !kept.PlainRunTokens {
		t.Fatalf("plain tokens were not kept: %+v %v", kept, err)
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
