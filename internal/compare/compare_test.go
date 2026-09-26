package compare_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/compare"
	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/replay"
)

func TestOneSubmissionMakesTwoRunsThatDifferOnlyByHarness(t *testing.T) {
	hermetic(t)
	bare := remote(t)
	started := git(t, bare, "rev-parse", "main")
	submission := submitted(t, bare, "local/fixture-model")

	where, err := compare.Prepare(submission, "", []string{"yes", "maybe"})
	if err != nil {
		t.Fatal(err)
	}
	sides, err := compare.Start(where, replay.Here)
	if err != nil {
		t.Fatal(err)
	}

	record, _ := compare.Read(where)
	if record.Commit != started || len(record.Runs) != 2 {
		t.Fatalf("the record should name two runs at %s, got %+v", started, record)
	}
	yes, maybe := specOf(t, record.Runs[0].Job(where)), specOf(t, record.Runs[1].Job(where))
	if yes["harness"] != "yes" || maybe["harness"] != "maybe" {
		t.Fatalf("each run should name its own harness, got %v and %v", yes["harness"], maybe["harness"])
	}
	delete(yes, "harness")
	delete(maybe, "harness")
	if !reflect.DeepEqual(yes, maybe) {
		t.Fatalf("the runs differ in more than the harness:\n%v\n%v", yes, maybe)
	}
	if yes["ref"] != started {
		t.Fatalf("each run should ask for the pinned commit, and asks for %v", yes["ref"])
	}
	for _, field := range []string{"brief", "budget", "gate", "model", "ref", "repo"} {
		if !contains(record.Held, field) {
			t.Fatalf("the record should say %s is held, and holds %v", field, record.Held)
		}
	}
	if !sides[0].Manifest.Gate.Passed || sides[1].Manifest.Gate.Passed || compare.ExitCode(sides) != 0 {
		t.Fatalf("yes should pass and maybe fail, and both ran: %+v", sides)
	}
}

func TestEachRunKeepsItsOwnEvidenceAndNeitherSeesTheOther(t *testing.T) {
	hermetic(t)
	where, err := compare.Prepare(submitted(t, remote(t), ""), "", []string{"yes", "maybe"})
	if err != nil {
		t.Fatal(err)
	}
	sides, err := compare.Start(where, replay.Here)
	if err != nil {
		t.Fatal(err)
	}

	yes, maybe := sides[0].Job(where), sides[1].Job(where)
	if yes.Workspace() == maybe.Workspace() || yes.Out() == maybe.Out() {
		t.Fatal("the runs share a directory")
	}
	if read(t, filepath.Join(yes.Workspace(), "answer.txt")) != "yes" || read(t, filepath.Join(maybe.Workspace(), "answer.txt")) != "maybe" {
		t.Fatal("one harness's work reached the other's workspace")
	}
	for _, side := range sides {
		if len(side.Wrong) > 0 || side.Err != nil {
			t.Fatalf("%s's evidence does not check: %v %v", side.Harness, side.Wrong, side.Err)
		}
		if _, err := os.Stat(filepath.Join(side.Job(where).Path(), compare.RecordFile)); err == nil {
			t.Fatalf("%s's run can read the comparison, which names the other harness", side.Harness)
		}
	}
	if strings.Contains(read(t, yes.OutFile("manifest.json")), "maybe") {
		t.Fatal("one run's evidence names the other")
	}
}

// The ref is resolved once, when the comparison is made. A branch that moves
// before the runs clone must not hand the harnesses different code.
func TestTheCommitIsPinnedBeforeAnyRunClones(t *testing.T) {
	hermetic(t)
	bare := remote(t)
	started := git(t, bare, "rev-parse", "main")
	where, err := compare.Prepare(submitted(t, bare, ""), "", []string{"yes", "maybe"})
	if err != nil {
		t.Fatal(err)
	}
	advance(t, bare)

	sides, err := compare.Start(where, replay.Here)

	if err != nil {
		t.Fatal(err)
	}
	for _, side := range sides {
		dir := side.Job(where)
		if at := git(t, dir.Workspace(), "rev-parse", "HEAD"); at != started {
			t.Fatalf("%s worked on %s rather than the pinned %s", side.Harness, at, started)
		}
		if _, err := os.Stat(filepath.Join(dir.Workspace(), "later.txt")); err == nil {
			t.Fatalf("%s saw a commit made after the pin", side.Harness)
		}
	}
}

func TestAComparisonIsAddressedByItsDirectory(t *testing.T) {
	hermetic(t)
	submission := submitted(t, remote(t), "")
	var first bytes.Buffer
	exit, err := compare.Answer(&first, []string{submission.Path(), "yes", "maybe"}, "", replay.Here)
	if err != nil || exit != 0 {
		t.Fatalf("the comparison should run, exit %d: %v\n%s", exit, err, &first)
	}
	where := filepath.Join(filepath.Dir(submission.Path()), "fix-compare-1")

	var again bytes.Buffer
	exit, err = compare.Answer(&again, []string{where}, "", nil)

	if err != nil || exit != 0 {
		t.Fatalf("the comparison should print again from its directory, exit %d: %v", exit, err)
	}
	if again.String() != first.String() {
		t.Fatalf("reading the comparison again should say what running it said:\n%s\n%s", &first, &again)
	}
	for _, want := range []string{"passed (0)", "failed (1)", "checks"} {
		if !strings.Contains(again.String(), want) {
			t.Fatalf("the summary should say %q:\n%s", want, &again)
		}
	}
}

func TestARunThatCannotStartIsReportedBesideTheOneThatRan(t *testing.T) {
	hermetic(t)
	var out bytes.Buffer
	exit, err := compare.Answer(&out, []string{submitted(t, remote(t), "").Path(), "yes", "missing"}, "", replay.Here)

	if err != nil {
		t.Fatal(err)
	}
	if exit != 2 || !strings.Contains(out.String(), "passed (0)") || !strings.Contains(out.String(), "missing: ") {
		t.Fatalf("the missing harness should be named, beside yes passing, and exit 2, got %d:\n%s", exit, &out)
	}
}

func TestAComparisonNeedsTwoDifferentHarnessesItCanRun(t *testing.T) {
	hermetic(t)
	bare := remote(t)
	for _, harnesses := range [][]string{{"yes"}, {"yes", "yes"}, {"yes", "../maybe"}, {"yes", "nobody-has-this"}} {
		submission := submitted(t, bare, "")
		if _, err := compare.Prepare(submission, "", harnesses); err == nil {
			t.Fatalf("%v should be refused", harnesses)
		}
		if _, err := os.Stat(filepath.Join(filepath.Dir(submission.Path()), "fix-compare-1")); err == nil {
			t.Fatalf("a refused comparison of %v left its directory behind", harnesses)
		}
	}
}

func specOf(t *testing.T, dir job.Dir) map[string]any {
	t.Helper()
	var fields map[string]any
	if err := json.Unmarshal([]byte(read(t, dir.Spec())), &fields); err != nil {
		t.Fatal(err)
	}
	return fields
}

func contains(names []string, want string) bool {
	for _, name := range names {
		if name == want {
			return true
		}
	}
	return false
}
