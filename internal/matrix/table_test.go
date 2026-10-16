package matrix_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/catalog"
	"github.com/getlatentic/gusto/internal/job"
	"github.com/getlatentic/gusto/internal/matrix"
	"github.com/getlatentic/gusto/internal/runner"
	"github.com/getlatentic/gusto/internal/sandbox"
)

// door stands in for the server's way out. It writes into the run's
// out/proxy.jsonl what the proxy writes there, one priced call and one it
// could not price, then runs the job here.
type door struct{ done int }

const calls = `{"path":"/v1/chat/completions","status":200,"atUnix":1767225600,"model":"m","inputTokens":900,"outputTokens":100,"usd":0.5}
{"path":"/v1/chat/completions","status":200,"atUnix":1767225601,"model":"m","inputTokens":null,"outputTokens":null,"usd":null}
`

func (d *door) Listen() (func(), error) { return func() {}, nil }

func (d *door) Run(dir job.Dir, _ job.Spec, _ ...sandbox.Mount) (int, error) {
	if err := os.WriteFile(dir.OutFile("proxy.jsonl"), []byte(calls), 0o644); err != nil {
		return 0, err
	}
	return runner.Inside(dir)
}

func (d *door) Done(job.Dir) { d.done++ }

// The proxy's record is the count, and the harness's report is a claim: a run
// that went through the proxy is costed from what the provider answered, and
// a call the proxy could not price makes the cost a floor.
func TestCostIsTheProxysCountWhenTheRunWentThroughIt(t *testing.T) {
	hermetic(t)
	pair := matrix.Pair{Harness: "priced", Model: "m"}
	opened := &door{}
	way := matrix.Contained("in a test container", func(string) (matrix.Door, error) { return opened, nil })
	_, table := measure(t, planned(t, remote(t), pair), "", way)

	row := rowFor(t, table, "one-line-fix", pair)
	if row.CostFrom != matrix.FromProxy || row.CostUSD == nil || *row.CostUSD != 0.5 || row.UnpricedCalls != 1 {
		t.Fatalf("the cost should be the proxy's $0.50 with one call unpriced, not the harness's $0.25: %+v", row)
	}
	if row.Tokens == nil || *row.Tokens != 1000 {
		t.Fatalf("the billed tokens should be the proxy's 1000: %+v", row)
	}
	if opened.done != 2 {
		t.Fatalf("each run's way out should be closed when it ends, and %d were", opened.done)
	}
	var shown bytes.Buffer
	if err := matrix.Markdown(&shown, table); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(shown.String(), "at least $0.5000 (proxy), 1 calls unpriced") {
		t.Fatalf("a floor should read as a floor:\n%s", &shown)
	}
}

func TestCostIsTheHarnesssReportWhenNothingElseCountedIt(t *testing.T) {
	hermetic(t)
	pair := matrix.Pair{Harness: "priced", Model: "m"}
	var ran int
	_, table := measure(t, planned(t, remote(t), pair, finishes), "", counting(&ran))

	row := rowFor(t, table, "one-line-fix", pair)
	if row.CostFrom != matrix.FromHarness || row.CostUSD == nil || *row.CostUSD != 0.25 {
		t.Fatalf("the cost should be the harness's own $0.25: %+v", row)
	}
	silent := rowFor(t, table, "one-line-fix", finishes)
	if silent.CostUSD != nil || silent.Tokens != nil {
		t.Fatalf("a harness that reported nothing is unmeasured, not free: %+v", silent)
	}
	var shown bytes.Buffer
	if err := matrix.Markdown(&shown, table); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(shown.String(), "| unmeasured | unmeasured |") || strings.Contains(shown.String(), "$0.0000") {
		t.Fatalf("an unmeasured cost should say so and never read as $0:\n%s", &shown)
	}
}

// The markdown goes into the README as it is, the JSON is what the catalog
// reads, and a pair that did not finish the simplest job is marked there.
func TestTheTableIsMarkdownForTheReadmeAndJSONForTheCatalog(t *testing.T) {
	hermetic(t)
	path := planned(t, remote(t), finishes, doesNotFinish)
	var out bytes.Buffer
	exit, err := matrix.Answer(&out, path, matrix.Asked{Way: matrix.Here(), Progress: &bytes.Buffer{}})
	if err != nil || exit != 0 {
		t.Fatalf("the matrix should run, exit %d: %v", exit, err)
	}
	where := matrix.Beside(path)
	if read(t, filepath.Join(where, matrix.MarkdownFile)) != out.String() {
		t.Fatal("what is printed and what is kept in the matrix differ")
	}
	for _, want := range []string{
		"| harness and model | job | finished | wall clock | billed tokens | cost | evidence |",
		"| `says`, `yes` | one-line-fix (simplest) | yes |",
		"| `says`, `no` | one-line-fix (simplest) | no |",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("the table should hold %q:\n%s", want, &out)
		}
	}

	entries, err := catalog.WithMatrix([]catalog.Entry{{Name: "says", Ready: true}}, filepath.Join(where, matrix.JSONFile))
	if err != nil {
		t.Fatal(err)
	}
	if _, cannot := entries[0].CannotFinish("yes"); cannot {
		t.Fatal("says@yes finished the simplest job and is marked anyway")
	}
	why, cannot := entries[0].CannotFinish("no")
	if !cannot || !strings.Contains(why, "one-line-fix") || !strings.Contains(why, where) {
		t.Fatalf("says@no should be marked, naming the job and the run: %q", why)
	}
}

// A matrix's directory prints its table again from the evidence alone, and
// writes nothing.
func TestAMatrixIsAddressedByItsDirectory(t *testing.T) {
	hermetic(t)
	path := planned(t, remote(t), finishes)
	var first bytes.Buffer
	if _, err := matrix.Answer(&first, path, matrix.Asked{Way: matrix.Here(), Progress: &bytes.Buffer{}}); err != nil {
		t.Fatal(err)
	}
	var again bytes.Buffer
	exit, err := matrix.Answer(&again, matrix.Beside(path), matrix.Asked{JSON: true})
	if err != nil || exit != 0 {
		t.Fatalf("the matrix should print again, exit %d: %v", exit, err)
	}
	var table matrix.Table
	if err := json.Unmarshal(again.Bytes(), &table); err != nil || len(table.Rows) != 2 {
		t.Fatalf("-json should print the table as JSON: %v\n%s", err, &again)
	}
}

// The two rows from getlatentic/planning#234 are a plan: opencode on two local models, on the one-
// line fix. The plan in the docs has to be one the matrix accepts.
func TestTheRowsFromIssueEightAreAPlan(t *testing.T) {
	at := t.TempDir()
	text := read(t, filepath.Join("..", "..", "docs", "matrix", "issue-8.json"))
	write(t, filepath.Join(at, "plan.json"), text)
	if err := os.MkdirAll(filepath.Join(at, "one-line-fix"), 0o755); err != nil {
		t.Fatal(err)
	}
	plan, err := matrix.ReadPlan(filepath.Join(at, "plan.json"))
	if err != nil {
		t.Fatal(err)
	}
	want := []matrix.Pair{{Harness: "opencode", Model: "ollama/gpt-oss:20b"}, {Harness: "opencode", Model: "ollama/llama3.2:3b"}}
	if len(plan.Pairs) != 2 || plan.Pairs[0] != want[0] || plan.Pairs[1] != want[1] || plan.SimplestJob() != "one-line-fix" {
		t.Fatalf("the plan should be opencode on both local models, on the one-line fix: %+v", plan)
	}
}
