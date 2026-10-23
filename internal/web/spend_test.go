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

func TestTheSpendPageShowsTeamsProjectsAndJobs(t *testing.T) {
	site := open(t)
	spentJob(t, site.index.Root(), "fix-it", "platform", priced, priced)
	spentJob(t, site.index.Root(), "said-nothing", "", unmeasured)
	site.signIn(t)
	answer := site.ask(t, "GET", "/spend?month=2026-09", nil)
	if answer.Code != http.StatusOK {
		t.Fatalf("code %d", answer.Code)
	}
	page := answer.Body.String()
	for _, want := range []string{"platform", "github.com/o/api", `href="/jobs/fix-it"`, "$0.5000", "spent this month, at least", "Other work"} {
		if !strings.Contains(page, want) {
			t.Fatalf("want %q on the page:\n%s", want, page)
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

// What an agent spent is added up under the agent's name, and a message is
// listed by what was asked, not by its job's id.
func TestTheSpendPageNamesAgentsAndMessages(t *testing.T) {
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
	for _, want := range []string{"By agent", "Clinical documentation", ">Draft a SOAP note from the visit notes</a>", `href="/jobs/` + id + `"`} {
		if !strings.Contains(page, want) {
			t.Fatalf("want %q on the page:\n%s", want, page)
		}
	}
	if strings.Contains(page, "By team") {
		t.Error("a month where no team was named has nothing to show by team")
	}
}
