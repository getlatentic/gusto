package web

import (
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/changelog"
	"github.com/getlatentic/gusto/internal/roster"
	"github.com/getlatentic/gusto/pkg/job"
)

// An admin gives every member a budget and one person their own, sees both on People, and each change is recorded.
func TestAnAdminSetsEveryMembersBudgetAndOnePersonsOwn(t *testing.T) {
	here, log, _ := budgeted(t)
	members := filepath.Join(t.TempDir(), "members.json")
	if err := roster.Write(members, roster.Roster{Members: []roster.Member{{Email: alice, Role: "runner"}, {Email: bob, Role: "runner"}}}); err != nil {
		t.Fatal(err)
	}
	here.server.UseRoster(members)
	set := func(form url.Values) *http.Response {
		return here.ask(t, "POST", "/admin/member-budget", form).Result()
	}
	if got := set(url.Values{"usd": {"20"}}); got.Header.Get("Location") != "/admin" {
		t.Fatalf("setting everyone's went to %q", got.Header.Get("Location"))
	}
	if page := overview(t, here); !strings.Contains(page, "$20.00 a month for each person.") {
		t.Errorf("everyone's budget is not on the overview")
	}
	if got := set(url.Values{"who": {alice}, "usd": {"50"}}); got.Header.Get("Location") != "/admin/people" {
		t.Fatalf("setting alice's went to %q", got.Header.Get("Location"))
	}
	people := here.ask(t, "GET", "/admin/people", nil).Body.String()
	if !strings.Contains(people, `aria-label="Budget a month for `+alice+`" placeholder="no limit" value="50.00"`) || !strings.Contains(people, `placeholder="20.00 each"`) {
		t.Errorf("People does not show alice's own budget and everyone else's:\n%s", people)
	}
	set(url.Values{"who": {alice}, "usd": {""}})
	if people := here.ask(t, "GET", "/admin/people", nil).Body.String(); strings.Count(people, `placeholder="20.00 each"`) != 2 {
		t.Error("alice's own budget, cleared, does not fall back to everyone's")
	}
	if got := set(url.Values{"usd": {"-3"}}); !strings.Contains(got.Header.Get("Location"), "refused=") {
		t.Errorf("a budget below nothing was taken: %q", got.Header.Get("Location"))
	}
	var said []string
	for _, event := range ofKind(written(t, log), changelog.MemberBudgetSet) {
		said = append(said, event.What["whose"]+":"+event.What["was"]+">"+event.What["now"])
	}
	if strings.Join(said, " ") != ":none>$20.00 Alice's:$20.00>$50.00 Alice's:$50.00>$20.00" {
		t.Errorf("the record has %v", said)
	}
}

// A person sees their own month against their budget, and a task they start is counted as theirs.
func TestAPersonSeesTheirBudgetAndTheirTasksAreTheirs(t *testing.T) {
	here, _, _ := budgeted(t)
	here.ask(t, "POST", "/admin/member-budget", url.Values{"usd": {"20"}})
	if page := here.ask(t, "GET", "/usage", nil).Body.String(); !strings.Contains(page, "You have spent $0.0000 of your $20.00 budget this month.") {
		t.Errorf("the person's usage does not show their budget:\n%s", page[:min(len(page), 2000)])
	}
	where := startedThread(t, here)
	said := here.ask(t, "POST", where+"/say", url.Values{"said": {"hello"}})
	id := strings.TrimPrefix(said.Header().Get("Location")[strings.Index(said.Header().Get("Location"), "#")+1:], "#")
	spec, err := job.Read(job.Dir(filepath.Join(here.index.Root(), id)))
	record, _ := here.index.Get(id)
	if err != nil || spec.Person == "" || spec.Person != record.Asked {
		t.Errorf("the task is not counted as the person's: %q, asked by %q, %v", spec.Person, record.Asked, err)
	}
}
