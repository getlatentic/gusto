package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/pkg/job"
)

// spentJob leaves a job directory with a proxy record, as a run through the
// proxy would. The lines are fixtures in the format the proxy writes.
func spentJob(t *testing.T, root, id, team string, lines ...string) {
	t.Helper()
	dir := job.Dir(filepath.Join(root, id))
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	spec := job.Spec{Brief: "b", Repo: "github.com/o/api", Gate: []string{"true"}, Harness: "fixture", Team: team}
	if err := spec.Write(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir.OutFile("proxy.jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

const (
	priced     = `{"path":"/v1/chat/completions","status":200,"atUnix":1790000000,"model":"m","inputTokens":10,"outputTokens":5,"usd":0.25}`
	unmeasured = `{"path":"/v1/chat/completions","status":200,"atUnix":1790000000,"model":"m","inputTokens":null,"outputTokens":null,"usd":null}`
)

// The month's page says what was spent, and names the team that spent most once there is more than one to name.
func TestTheSpendPageNamesTheBiggestTeamWhenThereIsMoreThanOne(t *testing.T) {
	site := open(t)
	spentJob(t, site.index.Root(), "fix-it", "platform", priced, priced)
	spentJob(t, site.index.Root(), "write-it", "finance", priced)
	spentJob(t, site.index.Root(), "said-nothing", "", unmeasured)
	site.signIn(t)
	answer := site.ask(t, "GET", "/spend?month=2026-09", nil)
	if answer.Code != http.StatusOK {
		t.Fatalf("code %d", answer.Code)
	}
	page := answer.Body.String()
	for _, want := range []string{"Biggest team", ">platform<", "$0.7500", "spent this month, at least"} {
		if !strings.Contains(page, want) {
			t.Fatalf("want %q on the page:\n%s", want, page)
		}
	}
	for _, not := range []string{"By team", "By agent", "Most expensive messages"} {
		if strings.Contains(page, not) {
			t.Errorf("the page still has the %q table", not)
		}
	}
}

func TestTheSpendReportIsThereForAScript(t *testing.T) {
	site := open(t)
	spentJob(t, site.index.Root(), "fix-it", "platform", priced, unmeasured)
	request := httptest.NewRequest("GET", "/api/spend?month=2026-09", nil)
	request.Header.Set("Authorization", "Bearer "+password)
	answer := httptest.NewRecorder()
	site.handler.ServeHTTP(answer, request)
	var report struct {
		Total struct {
			USD        float64 `json:"usd"`
			Unmeasured int     `json:"unmeasured"`
		} `json:"total"`
	}
	if err := json.Unmarshal(answer.Body.Bytes(), &report); err != nil {
		t.Fatalf("%v: %s", err, answer.Body.String())
	}
	if report.Total.USD != 0.25 || report.Total.Unmeasured != 1 {
		t.Fatalf("report %+v", report)
	}
	stranger := httptest.NewRecorder()
	site.handler.ServeHTTP(stranger, httptest.NewRequest("GET", "/api/spend", nil))
	if stranger.Code != http.StatusUnauthorized {
		t.Fatalf("a stranger got %d", stranger.Code)
	}
}

// One agent, model or team is nothing to compare, so it has no box.
func TestTheSpendPageHasNoBoxForSomethingWithNothingToCompareItWith(t *testing.T) {
	site := open(t)
	site.signIn(t)
	started := site.ask(t, "POST", "/templates/health", url.Values{"harness": {"fixture"}})
	where := started.Header().Get("Location")
	said := site.ask(t, "POST", where+"/say", url.Values{"said": {"Draft a SOAP note from the visit notes"}})
	id := strings.TrimPrefix(said.Header().Get("Location"), where+"#")
	site.wait(t, id)
	dir := job.Dir(filepath.Join(site.index.Root(), id))
	if err := os.WriteFile(dir.OutFile("proxy.jsonl"), []byte(priced+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	page := site.ask(t, "GET", "/spend?month=2026-09", nil).Body.String()
	for _, not := range []string{"Most used agent", "Most used model", "Biggest team"} {
		if strings.Contains(page, not) {
			t.Errorf("a month with one of each should have no %q box", not)
		}
	}
}
