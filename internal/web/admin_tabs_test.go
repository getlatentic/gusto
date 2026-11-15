package web

import (
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/registry"
	"github.com/getlatentic/gusto/internal/roster"
	"github.com/getlatentic/gusto/pkg/spend"
)

// withAcme is a console with the registry, an organization's settings and one provider of the admin's, acme.
func withAcme(t *testing.T) (*tokened, *registry.Registry, *site) {
	t.Helper()
	here, reg, _ := registered(t, registry.Rules{})
	here.server.UseOrgSettings(filepath.Join(t.TempDir(), "settings.json"))
	admin := here.as(t, boss)
	admin.ask(t, "POST", "/admin/models/providers", url.Values{"name": {"acme"}, "address": {"https://api.theirs.example"}, "source": {"stored"}, "key": {theirKey}})
	admin.ask(t, "POST", "/admin/models/providers/acme/models", url.Values{"model": {"big-1"}, "input": {"0.5"}, "output": {"1.5"}})
	admin.ask(t, "POST", "/admin/models/access", url.Values{"email": {bob2()}, "model": {"platform/qwen"}})
	return here, reg, admin
}

// modelsParts are the parts of the Models page's data that each tab holds, so each tab can be checked for its own and no other.
var modelsParts = map[string]func(modelsJSON) bool{
	"models": func(m modelsJSON) bool { _, held := m.rowNamed("acme/big-1"); return held },
	"providers": func(m modelsJSON) bool {
		row, held := m.providerNamed("acme")
		return held && row.Href == providerPage("acme")
	},
	"access":   func(m modelsJSON) bool { return len(m.Limits) == 1 },
	"settings": func(m modelsJSON) bool { return m.RunTokens != nil },
}

func TestEachTabOfTheModelsPageShowsItsOwnPartAndNoOther(t *testing.T) {
	_, _, admin := withAcme(t)
	for key, has := range modelsParts {
		page := modelsAt(t, admin, modelsTab(key))
		if !has(page) {
			t.Errorf("the %s tab lacks its part", key)
		}
		if page.Tab != key || !slices.ContainsFunc(page.Tabs, func(each tab) bool { return each.On && each.Href == modelsTab(key) }) {
			t.Errorf("the %s tab is not the one marked: %+v", key, page.Tabs)
		}
		for other, theirs := range modelsParts {
			if other != key && theirs(page) {
				t.Errorf("the %s tab shows the %s tab's part", key, other)
			}
		}
		if page.View.Unnamed {
			t.Errorf("the %s tab does not offer to add a provider", key)
		}
	}
	if page := modelsAt(t, admin, modelsPath); !modelsParts["models"](page) {
		t.Error("the page does not open on the Models tab")
	}
	acme := modelsAt(t, admin, providerPage("acme"))
	if acme.Provider == nil || acme.Provider.Name != "acme" || acme.Provider.Operator || !acme.Provider.Pending || acme.Provider.Key != "stored" {
		t.Fatalf("acme's page is not acme's to change, waiting to be turned on: %+v", acme.Provider)
	}
	if got := acme.modelsOf(); len(got) != 1 || got[0] != "acme/big-1" {
		t.Errorf("acme's page lists %v", got)
	}
}

func TestAFormOnTheModelsPageGoesBackToItsTab(t *testing.T) {
	_, reg, admin := withAcme(t)
	before := reg.Document().Version
	acme := providerPage("acme")
	for _, each := range []struct {
		path string
		form url.Values
		back string
	}{
		{"/admin/models/providers/acme/price", url.Values{"model": {"big-1"}, "input": {"1"}, "output": {"2"}}, acme},
		{"/admin/models/providers/acme/models", url.Values{"model": {"small-1"}}, acme},
		{"/admin/models/providers/acme/addresses", url.Values{"address": {"https://api.theirs.example/anthropic"}, "kind": {"anthropic"}}, acme},
		{"/admin/models/providers/acme/addresses/remove", url.Values{"address": {"https://api.theirs.example/anthropic"}}, acme},
		{"/admin/models/providers/acme/key", url.Values{"key": {"sk-another"}}, acme},
		{"/admin/models/providers/acme/enable", nil, acme},
		{"/admin/models/providers/acme/models/enable", url.Values{"model": {"big-1"}}, acme},
		{"/admin/models/providers/acme/models/disable", url.Values{"model": {"big-1"}}, acme},
		{"/admin/models/providers/acme/models/retire", url.Values{"model": {"small-1"}}, acme},
		{"/admin/models/providers/acme/disable", nil, acme},
		{"/admin/models/access", url.Values{"email": {alice}, "model": {"platform/qwen"}}, modelsTab("access")},
		{"/admin/models/access/clear", url.Values{"email": {alice}}, modelsTab("access")},
		{"/admin/run-tokens", url.Values{"bound": {"no"}}, modelsTab("settings")},
		{"/admin/chat-default", url.Values{"default": {""}}, modelsTab("settings")},
		{"/admin/models/restore", url.Values{"version": {fmt.Sprint(before)}}, modelsTab("settings")},
		{"/admin/models/providers", url.Values{"name": {"other"}, "address": {"https://api.theirs.example/other"}, "source": {"stored"}, "key": {theirKey}}, modelsTab("providers")},
		{"/admin/models/providers/other/retire", nil, modelsTab("providers")},
	} {
		got := admin.ask(t, "POST", each.path, each.form)
		if got.Code != http.StatusSeeOther || got.Header().Get("Location") != each.back {
			t.Errorf("%s went to %d %s, not %s", each.path, got.Code, got.Header().Get("Location"), each.back)
		}
	}
	refusals := []struct{ path, back string }{
		{"/admin/models/providers/acme/models", acme},
		{"/admin/models/access", modelsTab("access")},
		{"/admin/run-tokens", modelsTab("settings")},
		{"/admin/models/restore", modelsTab("settings")},
		{"/admin/models/providers", modelsTab("providers")},
	}
	for _, each := range refusals {
		got := admin.ask(t, "POST", each.path, url.Values{"input": {"x"}})
		where := got.Header().Get("Location")
		if !strings.HasPrefix(where, each.back+"&refused=") {
			t.Errorf("a refusal of %s went to %s", each.path, where)
			continue
		}
		if page := modelsAt(t, admin, where); page.Refused == "" {
			t.Errorf("the tab %s does not say why %s was refused", each.back, each.path)
		}
	}
}

func TestTheModelsTableAndAProvidersModelsArePagedAt25(t *testing.T) {
	_, reg, admin := withAcme(t)
	for i := 2; i <= 30; i++ {
		if err := reg.AddModel("acme", fmt.Sprintf("big-%d", i), nil); err != nil {
			t.Fatal(err)
		}
	}
	// The Models tab lists the operator's qwen before acme's thirty, so its 26th row is big-25.
	for _, each := range []struct {
		here, total string
		names       func(modelsJSON) []string
		twentySixth int
	}{
		{modelsTab("models"), "of 31", modelRowNames, 25},
		{providerPage("acme"), "of 30", modelsJSON.modelsOf, 26},
	} {
		twentyFifth, twentySixth := fmt.Sprintf("acme/big-%d", each.twentySixth-1), fmt.Sprintf("acme/big-%d", each.twentySixth)
		first := modelsAt(t, admin, each.here)
		if names := each.names(first); len(names) != 25 || names[24] != twentyFifth || first.Pages.Shown != "1–25 "+each.total {
			t.Errorf("the first page of %s is not the first 25 rows: %v %+v", each.here, names, first.Pages)
		}
		if first.Pages.Next != each.here+"&page=2" {
			t.Errorf("the first page of %s does not lead to the second: %+v", each.here, first.Pages)
		}
		second := modelsAt(t, admin, each.here+"&page=2")
		if names := each.names(second); len(names) == 0 || names[0] != twentySixth || second.Pages.Previous == "" {
			t.Errorf("the second page of %s does not start at the 26th row: %v %+v", each.here, names, second.Pages)
		}
	}
	if page := modelsAt(t, admin, modelsTab("access")); page.Pages != (pager{}) {
		t.Errorf("a table that fits one page has a pager: %+v", page.Pages)
	}
}

// modelRowNames is the full names of the models the Models tab lists.
func modelRowNames(m modelsJSON) []string {
	var names []string
	for _, row := range m.Rows {
		names = append(names, row.Full)
	}
	return names
}

// spendFor counts a call of this many dollars, made now, against the person who asked for it.
func spendFor(ledger *spend.Ledger, person string, usd float64) {
	task := "job-" + person
	ledger.Begin(task, spend.Terms{Person: person})
	ledger.Count(task, spend.Call{Path: "/v1/chat/completions", Status: 200, AtUnix: time.Now().Unix(), USD: &usd})
}

func TestBudgetsSaysTheBudgetsInALineAndListsEachPersonsAgainstWhatTheySpent(t *testing.T) {
	here, _, _ := budgeted(t)
	ledger := spend.NewLedger(spend.Policy{})
	here.server.UseLedger(ledger)
	setBudget(t, here, "50")
	here.ask(t, "POST", "/admin/member-budget", url.Values{"usd": {"10"}})
	here.ask(t, "POST", "/admin/member-budget", url.Values{"who": {alice}, "usd": {"4"}})
	spendFor(ledger, alice, 5)
	spendFor(ledger, bob, 2.5)
	page := budgets(t, here)
	if !page.Budget.MemberSet || page.Budget.Member != 10 || !page.Budget.Set || page.Budget.Limit != 50 {
		t.Errorf("Budgets does not give both budgets: %+v", page.Budget)
	}
	own, theirs := budgetRowOf(t, page, alice), budgetRowOf(t, page, bob)
	if !own.Budget.Own || own.Budget.Limit != 4 || !own.Budget.Passed || own.Percent != 100 {
		t.Errorf("alice's row does not show her own budget, reached: %+v", own)
	}
	if theirs.Budget.Own || theirs.Budget.Limit != 10 || theirs.Budget.Passed || theirs.Percent != 25 {
		t.Errorf("bob's row does not show everyone's budget, a quarter spent: %+v", theirs)
	}
}

func TestBudgetsIsPagedAt25AndListedAfterUsage(t *testing.T) {
	list := roster.Roster{Written: time.Now(), Members: []roster.Member{{Email: "ada@acme.com", Role: "admin"}}}
	for i := 2; i <= 30; i++ {
		list.Members = append(list.Members, roster.Member{Email: fmt.Sprintf("person%02d@acme.com", i), Role: "runner"})
	}
	here, _ := withPeople(t, list)
	here.server.UseOrgSettings(filepath.Join(t.TempDir(), "settings.json"))
	emails := func(page budgetsJSON) string {
		var all []string
		for _, row := range page.People {
			all = append(all, row.Email)
		}
		return strings.Join(all, " ")
	}
	first := budgets(t, here)
	if !strings.Contains(emails(first), "person25@acme.com") || strings.Contains(emails(first), "person26@acme.com") || first.Pages.Next != "/admin/budgets?page=2" {
		t.Errorf("the first page of Budgets is not the first 25 people: %+v", first.Pages)
	}
	var second budgetsJSON
	here.data(t, "/admin/budgets?page=2", &second)
	if !strings.Contains(emails(second), "person26@acme.com") || strings.Contains(emails(second), "person25@acme.com") {
		t.Error("the second page of Budgets does not start at the 26th person")
	}
	menu := strings.Join(consoleList(t, here), "\n")
	usage, budgetsAt, peopleAt := strings.Index(menu, "/admin/usage"), strings.Index(menu, "/admin/budgets"), strings.Index(menu, "/admin/people")
	if usage < 0 || budgetsAt < usage || peopleAt < budgetsAt {
		t.Errorf("the menu does not list Budgets between Usage and People:\n%s", menu)
	}
}

func TestPeopleIsPagedAt25AndInvitationsHaveATabOfTheirOwn(t *testing.T) {
	list := roster.Roster{Written: time.Now(), Members: []roster.Member{{Email: "ada@acme.com", Role: "admin"}}}
	for i := 2; i <= 30; i++ {
		list.Members = append(list.Members, roster.Member{Email: fmt.Sprintf("person%02d@acme.com", i), Role: "runner"})
	}
	here, _ := withPeople(t, list)
	emails := func(page peopleJSON) string {
		var all []string
		for _, each := range page.People {
			all = append(all, each.Email)
		}
		return strings.Join(all, " ")
	}
	first := people(t, here, "")
	if len(first.People) != 25 || !strings.Contains(emails(first), "person25@acme.com") || first.Pages.Shown != "1–25 of 30" {
		t.Errorf("the first page of People is not the first 25 people: %d, %q", len(first.People), first.Pages.Shown)
	}
	if len(first.Tabs) > 1 {
		t.Error("People has tabs with nobody invited")
	}
	second := people(t, here, "?tab=people&page=2")
	if !strings.HasPrefix(emails(second), "person26@acme.com") || strings.Contains(emails(second), "person25@acme.com") {
		t.Errorf("the second page of People does not start at the 26th person: %s", emails(second))
	}

	list.Invited = []roster.Invited{{Email: "dee@acme.com", By: "ada@acme.com", Created: time.Now(), Expires: time.Now().Add(time.Hour)}}
	invited, _ := withPeople(t, list)
	if page := people(t, invited, ""); len(page.Tabs) != 2 || page.Tabs[1].Label != "Invited" || page.Tabs[1].Count != 1 || page.Tabs[1].Href != "/admin/people?tab=invited" {
		t.Errorf("People has no Invited tab with one invitation: %+v", page.Tabs)
	}
}
