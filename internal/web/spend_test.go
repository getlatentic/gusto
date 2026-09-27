package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/job"
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
	for _, want := range []string{"platform", "github.com/o/api", `href="/jobs/fix-it"`, "$0.5000", "calls have no cost", "none named"} {
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
