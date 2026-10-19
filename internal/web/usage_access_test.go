package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/usage"
	"github.com/getlatentic/gusto/internal/usage/usagetest"
)

// bobsSecret is in only what bob made, so that finding it anywhere in what alice is shown means bob's rows leaked.
func secretsOfBob() []usage.Row {
	hidden := aCall("task-hidden", 0, "bobs-hidden-session", bob, 4)
	hidden.Agent, hidden.Model, hidden.Provider, hidden.Team, hidden.Project = "BobsAgent", "bobs-model", "bobs-provider", "bobs-team", "bobs-repo"
	return append(bothPeople(), hidden)
}

// rowsOfBob are the words that only a call of bob's holds. A page echoes back what was typed in its search box, so for
// a page the words a person could have typed are not among them.
var rowsOfBob = []string{"bobs-hidden-session", "bob-session", "task-hidden", "task-b"}

func leaks(body string) []string {
	return leaksOf(body, append([]string{bob, "BobsAgent", "bobs-model", "bobs-provider", "bobs-team", "bobs-repo"}, rowsOfBob...))
}

func leaksOf(body string, words []string) []string {
	var found []string
	for _, word := range words {
		if strings.Contains(body, word) {
			found = append(found, word)
		}
	}
	return found
}

func TestSomebodyWhoIsNotAnAdminSeesOnlyTheirOwnSessions(t *testing.T) {
	here, _ := withUsage(t, secretsOfBob()...)
	hers := here.as(t, alice)
	page := hers.ask(t, "GET", "/usage", nil)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "alice-session") || len(leaks(page.Body.String())) > 0 {
		t.Fatalf("alice's page showed %v:\n%s", leaks(page.Body.String()), page.Body)
	}
	if strings.Contains(page.Body.String(), `name="person"`) {
		t.Error("a person who sees only their own has no use for a choice of people")
	}
}

func TestNoQueryParameterWidensWhatAPersonSees(t *testing.T) {
	here, _ := withUsage(t, secretsOfBob()...)
	hers := here.as(t, alice)
	for _, query := range []string{
		"person=" + url.QueryEscape(bob), "q=bob", "q=bobs-model", "model=bobs-model", "agent=BobsAgent", "team=bobs-team", "project=bobs-repo",
		"person=", "person=%20", "from=2000-01-01&to=2100-01-01&person=" + url.QueryEscape(bob),
	} {
		for _, path := range []string{"/usage?" + query, "/api/usage/sessions?" + query, "/api/usage/totals?" + query, "/api/usage/facets?" + query, "/api/usage.csv?" + query} {
			got := hers.ask(t, "GET", path, nil)
			seen := leaks
			if strings.HasPrefix(path, "/usage") {
				seen = func(body string) []string { return leaksOf(body, rowsOfBob) }
			}
			if found := seen(got.Body.String()); len(found) > 0 {
				t.Errorf("%s showed alice %v", path, found)
			}
		}
	}
	var totals usage.Totals
	if err := decode(hers.ask(t, "GET", "/api/usage/totals?person="+url.QueryEscape(bob), nil), &totals); err != nil || totals.Calls != 2 {
		t.Errorf("asking for bob's totals gave alice %+v (%v)", totals, err)
	}
}

func TestSomebodyElsesSessionIsNotFoundByItsId(t *testing.T) {
	here, _ := withUsage(t, secretsOfBob()...)
	hers := here.as(t, alice)
	for _, path := range []string{"/usage/bob-session", "/usage/bobs-hidden-session", "/api/usage/calls?session=bob-session", "/api/usage/calls?session=bobs-hidden-session"} {
		if got := hers.ask(t, "GET", path, nil); got.Code != http.StatusNotFound || len(leaks(got.Body.String())) > 0 {
			t.Errorf("%s answered alice %d %s", path, got.Code, got.Body)
		}
	}
	if got := hers.ask(t, "GET", "/usage/alice-session", nil); got.Code != http.StatusOK {
		t.Errorf("her own session answered %d", got.Code)
	}
	if missing := hers.ask(t, "GET", "/usage/no-such-session", nil); missing.Code != http.StatusNotFound {
		t.Errorf("a session that does not exist answered %d, which would tell it from bob's", missing.Code)
	}
}

func TestAPersonsFacetsDoNotListOtherPeople(t *testing.T) {
	here, _ := withUsage(t, secretsOfBob()...)
	var facets usage.Facets
	if err := decode(here.as(t, alice).ask(t, "GET", "/api/usage/facets", nil), &facets); err != nil {
		t.Fatal(err)
	}
	if len(facets.People) != 1 || facets.People[0] != alice || len(facets.Agents) != 1 || len(facets.Models) != 1 || len(facets.Teams) != 0 {
		t.Errorf("alice was offered %+v", facets)
	}
	var everything usage.Facets
	if err := decode(here.ask(t, "GET", "/api/usage/facets", nil), &everything); err != nil || len(everything.People) != 2 || len(everything.Models) != 2 {
		t.Errorf("the admin was offered %+v (%v)", everything, err)
	}
}

func TestACsvHoldsOnlyThePersonsOwnCalls(t *testing.T) {
	here, _ := withUsage(t, secretsOfBob()...)
	got := here.as(t, alice).ask(t, "GET", "/api/usage.csv", nil)
	if got.Code != http.StatusOK || len(leaks(got.Body.String())) > 0 || strings.Count(got.Body.String(), "\n") != 3 {
		t.Errorf("alice's export was %d:\n%s", got.Code, got.Body)
	}
	all := here.ask(t, "GET", "/api/usage.csv", nil)
	if strings.Count(all.Body.String(), "\n") != 5 {
		t.Errorf("the admin's export was:\n%s", all.Body)
	}
}

func TestAPersonsTokenReadsThatPersonsCallsAndNoOthers(t *testing.T) {
	here, _ := withUsage(t, secretsOfBob()...)
	secret := signInADevice(t, here, alice)
	for _, path := range []string{"/api/usage/sessions", "/api/usage/totals", "/api/usage/facets", "/api/usage/calls?session=alice-session", "/api/usage.csv",
		"/api/usage/sessions?person=" + url.QueryEscape(bob), "/api/usage.csv?q=bob"} {
		got := here.call("GET", path, secret)
		if got.Code != http.StatusOK || len(leaks(got.Body.String())) > 0 {
			t.Errorf("%s with alice's token gave %d and %v", path, got.Code, leaks(got.Body.String()))
		}
	}
	if got := here.call("GET", "/api/usage/calls?session=bob-session", secret); got.Code != http.StatusNotFound {
		t.Errorf("bob's session by alice's token answered %d", got.Code)
	}
	if sessions := jsonList(t, here.call("GET", "/api/usage/sessions", secret), "sessions"); len(sessions) != 1 {
		t.Errorf("alice's token saw %d sessions", len(sessions))
	}
}

func TestAnAdminSignedInSeesEveryoneAndAPersonsTokenIsNeverAnAdmins(t *testing.T) {
	here, _ := withUsage(t, secretsOfBob()...)
	here.server.UseAdmins([]string{"admin@example.com"})
	admin := here.as(t, "admin@example.com")
	if sessions := jsonList(t, admin.ask(t, "GET", "/api/usage/sessions", nil), "sessions"); len(sessions) != 3 {
		t.Errorf("the admin saw %d sessions", len(sessions))
	}
	secret := signInADevice(t, here, "admin@example.com")
	if sessions := jsonList(t, here.call("GET", "/api/usage/sessions", secret), "sessions"); len(sessions) != 0 {
		t.Errorf("an admin's own computer is a person and saw %d of other people's sessions", len(sessions))
	}
}

func TestSomebodySignedInWithNoNameSeesNothing(t *testing.T) {
	here, _ := withUsage(t, secretsOfBob()...)
	nameless := here.as(t, "")
	for _, path := range []string{"/usage", "/api/usage/sessions", "/api/usage.csv"} {
		if got := nameless.ask(t, "GET", path, nil); got.Code == http.StatusOK && len(leaks(got.Body.String())) > 0 {
			t.Errorf("%s showed a nameless person %v", path, leaks(got.Body.String()))
		}
	}
}

func TestWithNoTokensTheSharedPasswordReadsEverything(t *testing.T) {
	here := open(t)
	store := usagetest.New()
	if err := store.Add(t.Context(), secretsOfBob()); err != nil {
		t.Fatal(err)
	}
	here.server.SetUsage(store)
	if sessions := jsonList(t, here.call("GET", "/api/usage/sessions", password), "sessions"); len(sessions) != 3 {
		t.Errorf("the shared password saw %d sessions", len(sessions))
	}
	if got := here.call("GET", "/api/usage/sessions", "not-the-password"); got.Code != http.StatusUnauthorized {
		t.Errorf("a wrong password answered %d", got.Code)
	}
}
