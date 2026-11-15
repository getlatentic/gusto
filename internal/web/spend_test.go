package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/pkg/task"
)

// spentTask leaves a task directory with a proxy record, as a run through the
// proxy would. The lines are fixtures in the format the proxy writes.
func spentTask(t *testing.T, root, id, team string, lines ...string) {
	t.Helper()
	spentTaskFor(t, root, id, team, "", lines...)
}

// spentTaskFor is spentTask for a task a person asked for.
func spentTaskFor(t *testing.T, root, id, team, person string, lines ...string) {
	t.Helper()
	dir := task.Dir(filepath.Join(root, id))
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	spec := task.Spec{Brief: "b", Repo: "github.com/o/api", Check: []string{"true"}, Harness: "fixture", Team: team, Person: person}
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
	spentTask(t, site.index.Root(), "fix-it", "platform", priced, priced)
	spentTask(t, site.index.Root(), "write-it", "finance", priced)
	spentTask(t, site.index.Root(), "said-nothing", "", unmeasured)
	site.signIn(t)
	if answer := site.ask(t, "GET", "/spend?month=2026-09", nil); answer.Code != http.StatusOK || !strings.Contains(answer.Body.String(), `<div id="root">`) {
		t.Fatalf("the spend page is not the app: %d", answer.Code)
	}
	ledger := spendAt(t, site, "/spend?month=2026-09").Ledger
	if ledger.Month != "2026-09" || ledger.Total.USD != 0.75 || ledger.Total.Unmeasured == 0 || ledger.Measure != "money" {
		t.Errorf("the month does not say what was spent, and that some calls have no price: %+v", ledger)
	}
	if !slices.ContainsFunc(ledger.Tops, func(top topJSON) bool { return top.Title == "Biggest team" && top.Name == "platform" }) {
		t.Errorf("the biggest team is not named: %+v", ledger.Tops)
	}
}

func TestTheSpendReportIsThereForAScript(t *testing.T) {
	site := open(t)
	spentTask(t, site.index.Root(), "fix-it", "platform", priced, unmeasured)
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
	dir := task.Dir(filepath.Join(site.index.Root(), id))
	if err := os.WriteFile(dir.OutFile("proxy.jsonl"), []byte(priced+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if tops := spendAt(t, site, "/spend?month=2026-09").Ledger.Tops; len(tops) != 0 {
		t.Errorf("a month with one of each should have no box: %+v", tops)
	}
}

// spendJSON is a spend report page's data.
type spendJSON struct {
	Base   string
	Ledger struct {
		Month string `json:"month"`
		Total struct {
			USD               float64 `json:"usd"`
			Calls, Unmeasured int64
		} `json:"total"`
		Months  []string
		Measure string
		Daily   []struct {
			Date, Says string
			Height     float64
		}
		Tops []topJSON
	}
}

type topJSON struct{ Title, Name, Says string }

// spendAt is the data of the spend report at path.
func spendAt(t *testing.T, who dataReader, path string) spendJSON {
	t.Helper()
	var page spendJSON
	who.data(t, path, &page)
	return page
}
