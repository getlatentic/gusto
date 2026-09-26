package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/job"
)

func TestAJobFromThePageCarriesItsTeamAndItsCap(t *testing.T) {
	site := open(t)
	site.signIn(t)
	answer := site.ask(t, "POST", "/jobs", url.Values{
		"repo": {"/repo"}, "brief": {"do it"}, "gate": {"true"}, "harness": {"fixture"},
		"team": {" platform "}, "usd": {"$2.50"},
	})
	if answer.Code != http.StatusSeeOther {
		t.Fatalf("code %d", answer.Code)
	}
	record, found := site.index.Get(strings.TrimPrefix(answer.Header().Get("Location"), "/jobs/"))
	if !found {
		t.Fatalf("sent to %q", answer.Header().Get("Location"))
	}
	spec, err := job.Read(record.Dir(site.index.Root()))
	if err != nil {
		t.Fatal(err)
	}
	if spec.Team != "platform" || spec.Budget.USD == nil || *spec.Budget.USD != 2.5 {
		t.Fatalf("team %q, cap %v", spec.Team, spec.Budget.USD)
	}
}

// Nothing typed is no cap, and a cap nothing could spend under is refused
// rather than written into a job that would then fail to run.
func TestACapThatIsNotAnAmountIsRefused(t *testing.T) {
	site := open(t)
	site.signIn(t)
	for _, typed := range []string{"five", "-1", "NaN"} {
		answer := site.ask(t, "POST", "/jobs", url.Values{
			"repo": {"/repo"}, "brief": {"do it"}, "gate": {"true"}, "usd": {typed},
		})
		if !strings.Contains(answer.Header().Get("Location"), "refused") {
			t.Fatalf("%q was sent to %q", typed, answer.Header().Get("Location"))
		}
	}
	if len(site.index.List()) != 0 {
		t.Fatal("a refused job left a directory behind")
	}
}
