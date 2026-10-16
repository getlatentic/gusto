package runner_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/internal/job"
	"github.com/getlatentic/gusto/internal/runner"
)

// A harness is a program, so a program that makes the change is enough to run
// the whole sequence without a model or an account.
const adapter = `{
  "name": "fixture",
  "program": "sh",
  "argv": ["-c", "printf yes > answer.txt; echo wrote the answer"],
  "needs": "nothing",
  "stream": "plain"
}`

// repo is a checkout whose gate fails until answer.txt says yes.
func repo(t *testing.T) string {
	t.Helper()
	at := t.TempDir()
	write(t, filepath.Join(at, "answer.txt"), "no")
	for _, args := range [][]string{
		{"init", "--quiet"},
		{"-c", "user.email=t@example.com", "-c", "user.name=T", "commit", "--quiet", "--allow-empty", "-m", "start"},
	} {
		command := exec.Command("git", args...)
		command.Dir = at
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	add := exec.Command("git", "-c", "user.email=t@example.com", "-c", "user.name=T", "add", "answer.txt")
	add.Dir = at
	if output, err := add.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v: %s", err, output)
	}
	return at
}

func ran(t *testing.T) (job.Dir, job.Spec) {
	t.Helper()
	harnesses := t.TempDir()
	write(t, filepath.Join(harnesses, "fixture.json"), adapter)
	t.Setenv("GUSTO_HARNESSES", harnesses)
	t.Setenv("GUSTO_SIGNER", "a-test")

	dir := job.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	spec := job.Spec{
		Brief:   "make the answer yes",
		Repo:    repo(t),
		Gate:    []string{"sh", "-c", `test "$(cat answer.txt)" = yes`},
		Harness: "fixture",
	}
	if err := spec.Write(dir); err != nil {
		t.Fatal(err)
	}
	exit, err := runner.Inside(dir)
	if err != nil {
		t.Fatal(err)
	}
	if exit != 0 {
		t.Fatalf("the gate should pass once the answer is yes, exit %d: %s", exit, runner.Tail(dir.OutFile("gate.log"), 5))
	}
	return dir, spec
}

func TestAJobLeavesEvidenceItsOwnManifestAgreesWith(t *testing.T) {
	dir, _ := ran(t)
	wrong, err := evidence.Check(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(wrong) != 0 {
		t.Fatalf("a fresh run should verify, got %v", wrong)
	}
}

func TestTheEvidenceNamesWhatTheHarnessDidAndWhoStandsBehindIt(t *testing.T) {
	dir, _ := ran(t)
	manifest, err := evidence.Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Signer != "a-test" || manifest.Harness != "fixture" {
		t.Fatalf("signer %q, harness %q", manifest.Signer, manifest.Harness)
	}
	if manifest.Tokens != nil || manifest.CostUSD != nil {
		t.Fatal("a harness that reports no usage must leave null, not zero")
	}
	patch, err := os.ReadFile(dir.OutFile("diff.patch"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(patch), "+yes") {
		t.Fatalf("the patch should carry the change:\n%s", patch)
	}
}

func TestChangingTheEvidenceIsDetectable(t *testing.T) {
	dir, _ := ran(t)
	write(t, dir.OutFile("transcript.log"), "something else happened")
	wrong, err := evidence.Check(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(wrong) != 1 || !strings.Contains(wrong[0], "transcript.log") {
		t.Fatalf("want transcript.log reported, got %v", wrong)
	}
}

func TestAFileAddedAfterTheRunIsReported(t *testing.T) {
	dir, _ := ran(t)
	write(t, dir.OutFile("extra.log"), "smuggled in")
	wrong, err := evidence.Check(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(wrong) != 1 || !strings.Contains(wrong[0], "extra.log") {
		t.Fatalf("want extra.log reported, got %v", wrong)
	}
}

func TestTheGateIsTheVerdictNotTheHarness(t *testing.T) {
	harnesses := t.TempDir()
	write(t, filepath.Join(harnesses, "liar.json"), `{
  "name": "liar",
  "program": "sh",
  "argv": ["-c", "echo I have fixed everything"],
  "needs": "nothing",
  "stream": "plain"
}`)
	t.Setenv("GUSTO_HARNESSES", harnesses)
	dir := job.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	spec := job.Spec{
		Brief:   "make the answer yes",
		Repo:    repo(t),
		Gate:    []string{"sh", "-c", `test "$(cat answer.txt)" = yes`},
		Harness: "liar",
	}
	if err := spec.Write(dir); err != nil {
		t.Fatal(err)
	}
	exit, err := runner.Inside(dir)
	if err != nil {
		t.Fatal(err)
	}
	if exit == 0 {
		t.Fatal("a harness that only claims to have finished must not pass the job")
	}
}

func TestABudgetStopsAHarnessThatWillNotFinish(t *testing.T) {
	harnesses := t.TempDir()
	write(t, filepath.Join(harnesses, "slow.json"), `{
  "name": "slow",
  "program": "sh",
  "argv": ["-c", "sleep 30"],
  "needs": "nothing",
  "stream": "plain"
}`)
	t.Setenv("GUSTO_HARNESSES", harnesses)
	dir := job.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	spec := job.Spec{
		Brief:   "never finish",
		Repo:    repo(t),
		Gate:    []string{"sh", "-c", `test "$(cat answer.txt)" = yes`},
		Harness: "slow",
		Budget:  job.Budget{Turns: 1, Seconds: 1},
	}
	if err := spec.Write(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Inside(dir); err != nil {
		t.Fatal(err)
	}
	manifest, err := evidence.Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !manifest.StoppedAtBudget {
		t.Fatal("the manifest should say the budget stopped it")
	}
}

// waitForFile waits until the harness has done what it does first, so that a stop comes while it is working and not while it
// is being set up, which takes longer on a busy machine.
func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Error("the harness never started")
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAJobThatIsStoppedKeepsWhatItDidAndTheManifestSaysWhoStoppedIt(t *testing.T) {
	harnesses := t.TempDir()
	write(t, filepath.Join(harnesses, "slow.json"), `{
  "name": "slow",
  "program": "sh",
  "argv": ["-c", "echo started-work > made.txt; sleep 30"],
  "needs": "nothing",
  "stream": "plain"
}`)
	t.Setenv("GUSTO_HARNESSES", harnesses)
	dir := job.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	spec := job.Spec{Brief: "a long job", Repo: repo(t), Harness: "slow", Budget: job.Budget{Turns: 1, Seconds: 120}}
	if err := spec.Write(dir); err != nil {
		t.Fatal(err)
	}
	if err := runner.NoteStop(dir, "ada@acme.example", 100); err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan error, 1)
	started := time.Now()
	go func() { _, err := runner.InsideContext(ctx, dir); done <- err }()
	waitForFile(t, filepath.Join(dir.Workspace(), "made.txt"))
	stop()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("a stopped job did not stop")
	}
	if time.Since(started) > 10*time.Second {
		t.Errorf("stopping took %v, which is waiting for the harness's sleep", time.Since(started))
	}
	manifest, err := evidence.Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.StoppedBy != "ada@acme.example" || manifest.StoppedAtBudget {
		t.Errorf("the manifest says %+v", manifest)
	}
	if made, err := os.ReadFile(filepath.Join(dir.Workspace(), "made.txt")); err != nil || !strings.Contains(string(made), "started-work") {
		t.Errorf("what the job had done was not kept: %q %v", made, err)
	}
}

func TestAStopWithNoNoteIsASignalAndAJobThatWasNotStoppedSaysNothing(t *testing.T) {
	harnesses := t.TempDir()
	write(t, filepath.Join(harnesses, "slow.json"), `{"name":"slow","program":"sh","argv":["-c","echo up > up.txt; sleep 30"],"needs":"nothing","stream":"plain"}`)
	t.Setenv("GUSTO_HARNESSES", harnesses)
	dir := job.Dir(t.TempDir())
	dir.Make()
	spec := job.Spec{Brief: "x", Repo: repo(t), Harness: "slow", Budget: job.Budget{Turns: 1, Seconds: 120}}
	spec.Write(dir)
	ctx, stop := context.WithCancel(context.Background())
	go func() { waitForFile(t, filepath.Join(dir.Workspace(), "up.txt")); stop() }()
	if _, err := runner.InsideContext(ctx, dir); err != nil {
		t.Fatal(err)
	}
	if manifest, _ := evidence.Read(dir); manifest.StoppedBy != "a signal" {
		t.Errorf("a stop with no note said %q", manifest.StoppedBy)
	}

	finished := job.Dir(t.TempDir())
	finished.Make()
	quick := job.Spec{Brief: "x", Repo: repo(t), Harness: "fixture", Gate: []string{"true"}, Budget: job.Budget{Turns: 1, Seconds: 60}}
	quick.Write(finished)
	write(t, filepath.Join(harnesses, "fixture.json"), `{"name":"fixture","program":"sh","argv":["-c","true"],"needs":"nothing","stream":"plain"}`)
	if _, err := runner.Inside(finished); err != nil {
		t.Fatal(err)
	}
	if manifest, _ := evidence.Read(finished); manifest.StoppedBy != "" {
		t.Errorf("a job that was not stopped says it was by %q", manifest.StoppedBy)
	}
}
