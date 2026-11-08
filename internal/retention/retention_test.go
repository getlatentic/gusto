package retention

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/record"
	"github.com/getlatentic/gusto/internal/signing"
	"github.com/getlatentic/gusto/internal/tasks"
	"github.com/getlatentic/gusto/pkg/task"
)

var noon = time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)

func daysAgo(n int) int64 { return noon.AddDate(0, 0, -n).Unix() }

// finished leaves a task that ended days ago, with a working folder of workingFolderBytes
// and an out/ of a fixed size, so what a sweep frees can be counted.
func finished(t *testing.T, index *tasks.Index, id string, state tasks.State, days int, workingFolderBytes int) tasks.Record {
	t.Helper()
	record := tasks.Record{ID: id, State: state, Brief: "brief " + id, Harness: "fixture", Asked: "amy@acme.test",
		Queued: daysAgo(days), Started: daysAgo(days), Ended: daysAgo(days)}
	dir := filepath.Join(index.Root(), id)
	for path, size := range map[string]int{"workspace/code.go": workingFolderBytes, "out/manifest.json": 100, "job.json": 10} {
		full := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(strings.Repeat("x", size)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := index.Add(record); err != nil {
		t.Fatal(err)
	}
	return record
}

func opened(t *testing.T) *tasks.Index {
	t.Helper()
	index, err := tasks.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return index
}

func exists(t *testing.T, index *tasks.Index, id, path string) bool {
	t.Helper()
	_, err := os.Stat(filepath.Join(index.Root(), id, path))
	return err == nil
}

var policy = Policy{
	WorkingFolderDays: map[string]int{"passed": 7, "failed": 30},
	RecordDays:        map[string]int{"passed": 90, "failed": 365},
}

func TestAWorkingFolderGoesFirstAndTheRecordStaysSigned(t *testing.T) {
	index := opened(t)
	finished(t, index, "young", tasks.Passed, 3, 5000)
	finished(t, index, "old", tasks.Passed, 10, 5000)
	report := Sweep(index, index.Root(), policy, Held{}, nil, noon, false)

	if exists(t, index, "old", "workspace") || !exists(t, index, "old", "out/manifest.json") || !exists(t, index, "old", "job.json") {
		t.Error("an old job did not lose its workspace and keep its record")
	}
	if !exists(t, index, "young", "workspace/code.go") {
		t.Error("a young job lost its workspace")
	}
	if report.Freed != 5000 || report.Removed(WorkingFolder) != 1 || report.Removed(Record) != 0 {
		t.Errorf("report %+v", report)
	}
	if _, found := index.Get("old"); !found {
		t.Error("a job whose workspace went is no longer in the index")
	}
	again := Sweep(index, index.Root(), policy, Held{}, nil, noon, false)
	if len(again.Actions) != 0 {
		t.Errorf("a second sweep did the same again: %+v", again)
	}
}

func TestARecordGoesWhenItsOwnRetentionEnds(t *testing.T) {
	index := opened(t)
	finished(t, index, "kept", tasks.Passed, 89, 1000)
	finished(t, index, "gone", tasks.Passed, 91, 1000)
	report := Sweep(index, index.Root(), policy, Held{}, nil, noon, false)
	if exists(t, index, "gone", "") || exists(t, index, "gone", "out/manifest.json") {
		t.Error("a record past its retention was kept")
	}
	if _, found := index.Get("gone"); found {
		t.Error("the index still lists a removed job")
	}
	if !exists(t, index, "kept", "out/manifest.json") {
		t.Error("a record inside its retention was removed")
	}
	if report.Removed(Record) != 1 {
		t.Errorf("report %+v", report)
	}
}

func TestRetentionIsSetPerState(t *testing.T) {
	index := opened(t)
	finished(t, index, "passed-20", tasks.Passed, 20, 100)
	finished(t, index, "failed-20", tasks.Failed, 20, 100)
	finished(t, index, "interrupted-400", tasks.Interrupted, 400, 100)
	Sweep(index, index.Root(), policy, Held{}, nil, noon, false)
	if exists(t, index, "passed-20", "workspace") || !exists(t, index, "failed-20", "workspace") {
		t.Error("the workspace rules did not follow the state")
	}
	if !exists(t, index, "interrupted-400", "workspace") {
		t.Error("a state the policy does not name lost work")
	}
}

func TestAHeldTaskAndAHeldPersonAreKeptWhole(t *testing.T) {
	index := opened(t)
	finished(t, index, "held-job", tasks.Passed, 200, 100)
	finished(t, index, "held-person", tasks.Passed, 200, 100)
	free := finished(t, index, "free", tasks.Passed, 200, 100)

	holds := OpenHolds(t.TempDir())
	if err := holds.HoldTask("held-job", "the admin", "legal"); err != nil {
		t.Fatal(err)
	}
	// Amy asked for all three, so holding her holds all three.
	if err := holds.HoldPerson("Amy@Acme.test", "the admin", "investigation"); err != nil {
		t.Fatal(err)
	}
	held, err := holds.Read()
	if err != nil {
		t.Fatal(err)
	}
	report := Sweep(index, index.Root(), policy, held, nil, noon, false)
	for _, id := range []string{"held-job", "held-person", free.ID} {
		if !exists(t, index, id, "workspace") {
			t.Errorf("%s lost its workspace though its asker is held", id)
		}
	}
	if len(report.Held) != 3 || len(report.Actions) != 0 {
		t.Errorf("report %+v", report)
	}
	if err := holds.ReleasePerson("amy@acme.test"); err != nil {
		t.Fatal(err)
	}
	held, _ = holds.Read()
	Sweep(index, index.Root(), policy, held, nil, noon, false)
	if !exists(t, index, "held-job", "workspace") {
		t.Error("a job held by itself was swept when its person was released")
	}
	if exists(t, index, "free", "workspace") || exists(t, index, "held-person", "workspace") {
		t.Error("released work was not swept")
	}
}

func TestOnlyFinishedWorkNoAgentOwnsIsSwept(t *testing.T) {
	index := opened(t)
	owned := tasks.Record{ID: "on-an-agent", State: tasks.Passed, Instance: "agent-1", Ended: daysAgo(400), Queued: daysAgo(400), Asked: "amy@acme.test"}
	if err := os.MkdirAll(filepath.Join(index.Root(), owned.ID, "workspace"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(index.Root(), owned.ID, "workspace", "f"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := index.Add(owned); err != nil {
		t.Fatal(err)
	}
	running := tasks.Record{ID: "still-running", State: tasks.Running, Queued: daysAgo(400), Started: daysAgo(400)}
	if err := os.MkdirAll(filepath.Join(index.Root(), running.ID, "workspace"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(index.Root(), running.ID, "workspace", "f"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := index.Add(running); err != nil {
		t.Fatal(err)
	}
	Sweep(index, index.Root(), policy, Held{}, nil, noon, false)
	if !exists(t, index, "on-an-agent", "workspace/f") || !exists(t, index, "still-running", "workspace/f") {
		t.Error("a sweep touched a running job or a job that belongs to an agent")
	}
}

func TestADryRunSaysWhatItWouldDoAndDoesNothing(t *testing.T) {
	index := opened(t)
	finished(t, index, "old", tasks.Passed, 10, 5000)
	finished(t, index, "ancient", tasks.Passed, 100, 5000)
	report := Sweep(Scanned{Root: index.Root()}, index.Root(), policy, Held{}, nil, noon, true)
	if !report.Dry || len(report.Actions) != 2 || report.Freed != 0 {
		t.Fatalf("report %+v", report)
	}
	if !exists(t, index, "old", "workspace/code.go") || !exists(t, index, "ancient", "out/manifest.json") {
		t.Error("a dry run removed something")
	}
	if report.Actions[0].Bytes == 0 && report.Actions[1].Bytes == 0 {
		t.Error("a dry run does not say how much it would free")
	}
}

func TestAPolicyThatCannotBeMeantIsRefused(t *testing.T) {
	for name, p := range map[string]Policy{
		"a state jobs do not end in":    {WorkingFolderDays: map[string]int{"running": 1}},
		"negative days":                 {RecordDays: map[string]int{"passed": -1}},
		"a record before its workspace": {WorkingFolderDays: map[string]int{"passed": 30}, RecordDays: map[string]int{"passed": 7}},
	} {
		if p.Check() == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	if err := policy.Check(); err != nil {
		t.Errorf("a sensible policy was refused: %v", err)
	}
}

func TestAPolicyFileIsReadStrictly(t *testing.T) {
	dir := t.TempDir()
	if p, err := LoadPolicy(dir); err != nil || p.Any() {
		t.Fatalf("no file should keep everything: %+v %v", p, err)
	}
	write := func(text string) error {
		if err := os.WriteFile(filepath.Join(dir, PolicyFile), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := LoadPolicy(dir)
		return err
	}
	if err := write(`{"workspaceDays": {"passed": 7}, "recordDays": {"passed": 90}}`); err != nil {
		t.Fatal(err)
	}
	if write(`{"workspaceDay": {"passed": 7}}`) == nil {
		t.Error("a misspelt field was ignored, and the policy would keep everything")
	}
}

func TestUsageSaysWhatIsUsedAndTheOldestTaskKept(t *testing.T) {
	index := opened(t)
	finished(t, index, "recent", tasks.Passed, 2, 1000)
	finished(t, index, "oldest", tasks.Failed, 50, 3000)
	used := Measure(index.Root(), index.List())
	if used.Tasks != 2 || used.WorkingFolders != 4000 || used.Bytes < 4000+2*110 {
		t.Errorf("usage %+v", used)
	}
	if used.Oldest == nil || used.Oldest.ID != "oldest" {
		t.Errorf("oldest %+v", used.Oldest)
	}
}

// The record a sweep keeps is the record that was signed: verify still says it
// holds, and who signed it, once the working folder is gone.
func TestASweptTaskStillVerifiesAndNamesItsSigner(t *testing.T) {
	index := opened(t)
	finished(t, index, "signed", tasks.Passed, 40, 4000)
	dir := task.Dir(filepath.Join(index.Root(), "signed"))
	if err := os.WriteFile(dir.OutFile("diff.patch"), []byte("a patch"), 0o644); err != nil {
		t.Fatal(err)
	}
	spec := task.Spec{Brief: "b", Repo: "r", Check: []string{"true"}, Harness: "fixture"}
	if err := spec.Write(dir); err != nil {
		t.Fatal(err)
	}
	taskHash, files, err := record.Of(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := record.Write(dir, record.Manifest{Task: taskHash, Files: files, Check: &record.Verdict{Passed: true}, Harness: "fixture", AtUnix: daysAgo(40)}); err != nil {
		t.Fatal(err)
	}
	ring := t.TempDir()
	key, err := signing.Generate(ring, "acme")
	if err != nil {
		t.Fatal(err)
	}
	if err := record.Seal(dir, key); err != nil {
		t.Fatal(err)
	}
	keyring, err := signing.OpenKeyring(ring)
	if err != nil {
		t.Fatal(err)
	}

	report := Sweep(index, index.Root(), policy, Held{}, nil, noon, false)
	if report.Removed(WorkingFolder) != 1 || exists(t, index, "signed", "workspace") {
		t.Fatalf("the workspace stayed: %+v", report)
	}
	if wrong, err := record.Check(dir); err != nil || len(wrong) > 0 {
		t.Fatalf("a swept job no longer checks: %v %v", wrong, err)
	}
	if attested, err := record.Attest(dir, keyring); err != nil || attested.Signer != "acme" {
		t.Fatalf("the signer is not named: %+v %v", attested, err)
	}
}
