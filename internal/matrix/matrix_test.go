package matrix_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/compare"
	"github.com/getlatentic/haas/internal/evidence"
	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/matrix"
)

func TestEveryRowIsARunWithItsEvidenceKept(t *testing.T) {
	hermetic(t)
	var ran int
	where, table := measure(t, planned(t, remote(t), finishes, doesNotFinish), "", counting(&ran))

	if ran != 4 || len(table.Rows) != 4 {
		t.Fatalf("two jobs on two pairs should be four runs and four rows, got %d runs and %+v", ran, table.Rows)
	}
	for _, row := range table.Rows {
		dir := job.Dir(filepath.Join(where, row.Run))
		if wrong, err := evidence.Check(dir); err != nil || len(wrong) > 0 || row.Evidence != "checks" {
			t.Fatalf("%s on %s should keep evidence that checks, got %v %v %q", row.Job, row.Harness, wrong, err, row.Evidence)
		}
		if row.WallSeconds == nil || row.Environment != "a test, here, in this process" {
			t.Fatalf("every finished run should say how long it took and where: %+v", row)
		}
	}
	if yes := rowFor(t, table, "one-line-fix", finishes); yes.Finished == nil || !*yes.Finished {
		t.Fatalf("says@yes should finish: %+v", yes)
	}
	if no := rowFor(t, table, "one-line-fix", doesNotFinish); no.Finished == nil || *no.Finished {
		t.Fatalf("says@no should run and not finish: %+v", no)
	}
	if matrix.ExitCode(table) != 0 {
		t.Fatal("every cell left a verdict, so the matrix did its job whichever pair finished")
	}
}

// A job's column is a comparison `haas compare` can read: one commit, and runs
// that differ in the harness and model alone.
func TestAJobIsAComparisonPinnedToOneCommit(t *testing.T) {
	hermetic(t)
	bare := remote(t)
	started := git(t, bare, "rev-parse", "main")
	var ran int
	where, _ := measure(t, planned(t, bare, finishes, doesNotFinish), "", counting(&ran))

	record, err := compare.Read(filepath.Join(where, "one-line-fix"))
	if err != nil {
		t.Fatal(err)
	}
	if record.Commit != started || len(record.Runs) != 2 {
		t.Fatalf("the comparison should hold two runs at %s: %+v", started, record)
	}
	for _, field := range []string{"brief", "gate", "harness", "ref", "repo"} {
		if !contains(record.Held, field) {
			t.Fatalf("%s is the same in every run and should be held: %v", field, record.Held)
		}
	}
	if contains(record.Held, "model") {
		t.Fatalf("the model differs between the runs, and is claimed as held: %v", record.Held)
	}
	sides := compare.Sides(filepath.Join(where, "one-line-fix"), record)
	if compare.ExitCode(sides) != 0 {
		t.Fatalf("compare should read the matrix's runs as its own: %+v", sides)
	}
}

// Each cell costs money, so a finished one is left alone, and a pair added to
// the plan later runs alone, at the commit the others ran at.
func TestAFinishedCellIsNotRunAgain(t *testing.T) {
	hermetic(t)
	bare := remote(t)
	started := git(t, bare, "rev-parse", "main")
	path := planned(t, bare, finishes)
	var ran int
	where, _ := measure(t, path, "", counting(&ran))
	before := read(t, filepath.Join(where, "one-line-fix", finishes.Dir(), "out", "manifest.json"))

	advance(t, bare)
	plan, _ := matrix.ReadPlan(path)
	plan.Pairs = append(plan.Pairs, doesNotFinish)
	ran = 0
	if _, err := matrix.Measure(plan, where, counting(&ran), matrix.AgainFor(""), &strings.Builder{}); err != nil {
		t.Fatal(err)
	}

	if ran != 2 {
		t.Fatalf("only the new pair should run, once a job, and %d runs happened", ran)
	}
	if read(t, filepath.Join(where, "one-line-fix", finishes.Dir(), "out", "manifest.json")) != before {
		t.Fatal("a finished cell was run again")
	}
	added := filepath.Join(where, "one-line-fix", doesNotFinish.Dir(), "workspace")
	if at := git(t, added, "rev-parse", "HEAD"); at != started {
		t.Fatalf("the pair added later worked on %s rather than the pinned %s", at, started)
	}
}

// Asked for, a finished pair runs again, and the run it replaces is kept.
func TestAgainRunsANamedPairAndKeepsTheOldRun(t *testing.T) {
	hermetic(t)
	path := planned(t, remote(t), finishes, doesNotFinish)
	var ran int
	where, _ := measure(t, path, "", counting(&ran))
	ran = 0
	_, table := measure(t, path, finishes.Name(), counting(&ran))

	if ran != 2 {
		t.Fatalf("says@yes should run again on both jobs and nothing else, and %d runs happened", ran)
	}
	kept := filepath.Join(where, "one-line-fix", finishes.Dir()+".attempt-1")
	if wrong, err := evidence.Check(job.Dir(kept)); err != nil || len(wrong) > 0 {
		t.Fatalf("the run that was replaced should be kept with its evidence: %v %v", wrong, err)
	}
	if matrix.ExitCode(table) != 0 {
		t.Fatalf("the table should read the new runs: %+v", table.Rows)
	}
}

// A run that left no verdict measured nothing: it is said, it is run again next
// time, and its numbers stay unmeasured rather than zero.
func TestARunThatLeftNoVerdictIsRunAgainNextTime(t *testing.T) {
	hermetic(t)
	lost := matrix.Pair{Harness: "missing", Model: "anything"}
	path := planned(t, remote(t), finishes, lost)
	var ran int
	_, table := measure(t, path, "", counting(&ran))
	row := rowFor(t, table, "one-line-fix", lost)
	if row.Finished != nil || row.WallSeconds != nil || row.CostUSD != nil || row.Tokens != nil {
		t.Fatalf("a run with no verdict should measure nothing: %+v", row)
	}
	if matrix.ExitCode(table) != 2 {
		t.Fatal("a cell that left no verdict should make the matrix exit 2")
	}

	ran = 0
	measure(t, path, "", counting(&ran))
	if ran != 2 {
		t.Fatalf("the missing pair should be tried again on both jobs, and only it: %d runs", ran)
	}
}

// A matrix is one environment. Measuring another into it would put two places
// in one row.
func TestAMatrixKeepsToOneEnvironment(t *testing.T) {
	hermetic(t)
	path := planned(t, remote(t), finishes)
	var ran int
	where, _ := measure(t, path, "", counting(&ran))
	plan, _ := matrix.ReadPlan(path)
	plan.Environment = "the box"

	_, err := matrix.Measure(plan, where, counting(&ran), matrix.AgainFor(""), &strings.Builder{})

	if err == nil || !strings.Contains(err.Error(), "-into") {
		t.Fatalf("a second environment should be refused, and told where to go: %v", err)
	}
}

func TestAPlanThatWouldShareADirectoryIsRefused(t *testing.T) {
	hermetic(t)
	bare := remote(t)
	for name, pairs := range map[string][]matrix.Pair{
		"no pairs":          nil,
		"one pair twice":    {finishes, finishes},
		"a path as harness": {{Harness: "../says", Model: "yes"}},
		"two names, one dir": {
			{Harness: "says", Model: "a/b"}, {Harness: "says", Model: "a:b"},
		},
	} {
		if _, err := matrix.ReadPlan(planned(t, bare, pairs...)); err == nil {
			t.Errorf("%s should be refused", name)
		}
	}
}

func contains(names []string, want string) bool {
	for _, name := range names {
		if name == want {
			return true
		}
	}
	return false
}
