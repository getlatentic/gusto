package web

import (
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
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

func TestEachTabOfTheModelsPageShowsItsOwnPartAndNoOther(t *testing.T) {
	_, _, admin := withAcme(t)
	parts := map[string]string{
		"models":    `title="acme/big-1"`,
		"providers": `href="/admin/models?tab=providers&amp;provider=acme"`,
		"access":    `action="/admin/models/access"`,
		"settings":  `action="/admin/run-tokens"`,
	}
	for tab, own := range parts {
		page := admin.ask(t, "GET", modelsTab(tab), nil).Body.String()
		if !strings.Contains(page, own) {
			t.Errorf("the %s tab lacks %s", tab, own)
		}
		if !strings.Contains(page, `href="/admin/models?tab=`+tab+`" aria-current="page"`) {
			t.Errorf("the %s tab is not the one marked", tab)
		}
		for other, theirs := range parts {
			if other != tab && strings.Contains(page, theirs) {
				t.Errorf("the %s tab shows the %s tab's %s", tab, other, theirs)
			}
		}
		if !strings.Contains(page, `data-open="add-provider"`) {
			t.Errorf("the %s tab does not offer to add a provider", tab)
		}
	}
	if page := admin.ask(t, "GET", "/admin/models", nil).Body.String(); !strings.Contains(page, parts["models"]) {
		t.Error("the page does not open on the Models tab")
	}
	provider := admin.ask(t, "GET", providerPage("acme"), nil).Body.String()
	for _, want := range []string{`action="/admin/models/providers/acme/price"`, `action="/admin/models/providers/acme/models"`, `action="/admin/models/providers/acme/addresses"`,
		`action="/admin/models/providers/acme/key"`, `action="/admin/models/providers/acme/enable"`, `action="/admin/models/providers/acme/retire"`} {
		if !strings.Contains(provider, want) {
			t.Errorf("acme's page lacks %s", want)
		}
	}
	if strings.Contains(provider, "<details class=\"group ") {
		t.Error("acme's page is folded")
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
		if page := admin.ask(t, "GET", where, nil).Body.String(); !strings.Contains(page, `role="alert"`) {
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
	for _, each := range []struct{ here, row, total string }{
		{modelsTab("models"), `title="acme/big-%d"`, "of 31"},
		{providerPage("acme"), `<code>acme/big-%d</code>`, "of 30"},
	} {
		twentyFifth, twentySixth := 25, 26
		if each.here == modelsTab("models") {
			twentyFifth, twentySixth = 24, 25
		}
		first := admin.ask(t, "GET", each.here, nil).Body.String()
		if !strings.Contains(first, fmt.Sprintf(each.row, twentyFifth)) || strings.Contains(first, fmt.Sprintf(each.row, twentySixth)) || !strings.Contains(first, "1–25 "+each.total) {
			t.Errorf("the first page of %s is not the first 25 rows", each.here)
		}
		next := strings.ReplaceAll(each.here, "&", "&amp;") + "&amp;page=2"
		if !strings.Contains(first, `href="`+next+`" rel="next"`) {
			t.Errorf("the first page of %s does not lead to the second", each.here)
		}
		second := admin.ask(t, "GET", each.here+"&page=2", nil).Body.String()
		if !strings.Contains(second, fmt.Sprintf(each.row, twentySixth)) || strings.Contains(second, fmt.Sprintf(each.row, twentyFifth)) || !strings.Contains(second, `rel="prev"`) {
			t.Errorf("the second page of %s does not start at the 26th row", each.here)
		}
	}
	if page := admin.ask(t, "GET", modelsTab("access"), nil).Body.String(); strings.Contains(page, `aria-label="Pages"`) {
		t.Error("a table that fits one page has a pager")
	}
}

// spendFor counts a call of this many dollars, made now, against the person who asked for it.
func spendFor(ledger *spend.Ledger, person string, usd float64) {
	job := "job-" + person
	ledger.Begin(job, spend.Terms{Person: person})
	ledger.Count(job, spend.Call{Path: "/v1/chat/completions", Status: 200, AtUnix: time.Now().Unix(), USD: &usd})
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
	if !strings.Contains(page, "Each person: $10.00 a month; their agents stop there. Organization: $50.00 a month, which only warns.") {
		t.Errorf("Budgets does not say both budgets in a line:\n%s", page)
	}
	own, theirs := rowOf(t, page, alice), rowOf(t, page, bob)
	if !strings.Contains(own, "$4.00 <span") || !strings.Contains(own, ">own</span>") || !strings.Contains(own, "reached") || !strings.Contains(own, "bg-bad") || !strings.Contains(own, "width:100%") {
		t.Errorf("alice's row does not show her own budget, reached:\n%s", own)
	}
	if !strings.Contains(theirs, "$10.00") || strings.Contains(theirs, ">own<") || strings.Contains(theirs, "bg-bad") || !strings.Contains(theirs, "width:25%") {
		t.Errorf("bob's row does not show everyone's budget, a quarter spent:\n%s", theirs)
	}
	dialog, form := strings.Index(page, `<dialog class="`), strings.Index(page, `action="/admin/budget"`)
	if strings.Count(page, `data-open="budgets"`) != 1 || dialog < 0 || form < dialog || form < strings.Index(page, "</table>") {
		t.Error("the one Change does not open the budget form in a dialog")
	}
	if strings.Count(page, `action="/admin/member-budget"`) != 2 {
		t.Error("each row does not hold its own budget form behind its menu")
	}
}

func TestBudgetsIsPagedAt25AndListedAfterUsage(t *testing.T) {
	list := roster.Roster{Written: time.Now(), Members: []roster.Member{{Email: "ada@acme.com", Role: "admin"}}}
	for i := 2; i <= 30; i++ {
		list.Members = append(list.Members, roster.Member{Email: fmt.Sprintf("person%02d@acme.com", i), Role: "runner"})
	}
	here, _ := withPeople(t, list)
	here.server.UseOrgSettings(filepath.Join(t.TempDir(), "settings.json"))
	first := budgets(t, here)
	if !strings.Contains(first, "person25@acme.com") || strings.Contains(first, "person26@acme.com") || !strings.Contains(first, `href="/admin/budgets?page=2" rel="next"`) {
		t.Error("the first page of Budgets is not the first 25 people")
	}
	second := here.ask(t, "GET", "/admin/budgets?page=2", nil).Body.String()
	if !strings.Contains(second, "person26@acme.com") || strings.Contains(second, "person25@acme.com") {
		t.Error("the second page of Budgets does not start at the 26th person")
	}
	usage, budgetsAt, peopleAt := strings.Index(first, `href="/admin/usage"`), strings.Index(first, `href="/admin/budgets"`), strings.Index(first, `href="/admin/people"`)
	if usage < 0 || budgetsAt < usage || peopleAt < budgetsAt {
		t.Error("the menu does not list Budgets between Usage and People")
	}
	people := here.ask(t, "GET", "/admin/people", nil).Body.String()
	for _, gone := range []string{`action="/admin/budget"`, `action="/admin/member-budget"`, `data-open="budgets"`, "Each person:"} {
		if strings.Contains(people, gone) {
			t.Errorf("People still has %s", gone)
		}
	}
}

func TestPeopleIsPagedAt25AndInvitationsHaveATabOfTheirOwn(t *testing.T) {
	list := roster.Roster{Written: time.Now(), Members: []roster.Member{{Email: "ada@acme.com", Role: "admin"}}}
	for i := 2; i <= 30; i++ {
		list.Members = append(list.Members, roster.Member{Email: fmt.Sprintf("person%02d@acme.com", i), Role: "runner"})
	}
	here, _ := withPeople(t, list)
	first := here.ask(t, "GET", "/admin/people", nil).Body.String()
	if !strings.Contains(first, "person25@acme.com") || strings.Contains(first, "person26@acme.com") || !strings.Contains(first, "1–25 of 30") {
		t.Error("the first page of People is not the first 25 people")
	}
	if strings.Contains(first, "tab=invited") || strings.Contains(first, `aria-label="Views"`) {
		t.Error("People has tabs with nobody invited")
	}
	second := here.ask(t, "GET", "/admin/people?tab=people&page=2", nil).Body.String()
	if !strings.Contains(second, "person26@acme.com") || strings.Contains(second, "person25@acme.com") {
		t.Error("the second page of People does not start at the 26th person")
	}

	list.Invited = []roster.Invited{{Email: "dee@acme.com", By: "ada@acme.com", Created: time.Now(), Expires: time.Now().Add(time.Hour)}}
	invited, _ := withPeople(t, list)
	if page := invited.ask(t, "GET", "/admin/people", nil).Body.String(); !strings.Contains(page, `href="/admin/people?tab=invited">Invited <span class="font-normal text-subtle">1</span>`) {
		t.Error("People has no Invited tab with one invitation")
	}
}
