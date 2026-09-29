package openshell_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/harness"
	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/openshell"
	"github.com/getlatentic/haas/internal/runner"
)

// inComputer is a sandbox that lasts, for a job.
func inComputer(t *testing.T, dir job.Dir, name, jobID string) *openshell.Sandbox {
	t.Helper()
	return inComputerFor(t, dir, name, jobID, "")
}

// inComputerFor is a sandbox that lasts, for a turn in a conversation on an instance.
func inComputerFor(t *testing.T, dir job.Dir, name, jobID, instance string) *openshell.Sandbox {
	t.Helper()
	configured, err := harness.Open(dir, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	box, err := openshell.NewComputer(openshell.Config{Binary: "openshell", Image: "runner.tar", CPU: "1", Memory: "1Gi"}, name, jobID, instance,
		configured.Adapter, "http://host.openshell.internal:8041/model/v1", "http://127.0.0.1:8041/model/v1", "a-job-token")
	if err != nil {
		t.Fatal(err)
	}
	return box
}

func withHarness(t *testing.T, script string) {
	t.Helper()
	harnesses := t.TempDir()
	write(t, filepath.Join(harnesses, "fixture.json"), `{"name":"fixture","program":"sh","argv":["-c",`+quote(script)+`],"needs":"nothing","stream":"plain"}`)
	t.Setenv("HAAS_HARNESSES", harnesses)
}

func quote(text string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(text) + `"`
}

// A computer is made once. The second job finds it, and what the first left in it that is
// not the first's job directory, and neither job removes it.
func TestAComputerIsMadeOnceAndKeepsWhatAPersonPutInItBetweenJobs(t *testing.T) {
	root := installFake(t)
	first, _ := aJob(t)
	withHarness(t, `printf yes > answer.txt; printf remembered > ../../../notes.txt; echo first job in $(pwd | sed 's|.*/sandbox||')`)
	if exit, err := runner.Outside(first, inComputer(t, first, "c-12345678-abcdefgh", "20260929-1200-first")); err != nil || exit != 0 {
		t.Fatalf("exit %d, %v", exit, err)
	}
	second, _ := aJob(t)
	withHarness(t, `printf yes > answer.txt; echo second job sees: $(cat ../../../notes.txt)`)
	if exit, err := runner.Outside(second, inComputer(t, second, "c-12345678-abcdefgh", "20260929-1201-second")); err != nil || exit != 0 {
		t.Fatalf("exit %d, %v", exit, err)
	}

	calls, _ := os.ReadFile(filepath.Join(root, "calls.log"))
	if n := strings.Count(string(calls), "sandbox create"); n != 1 {
		t.Errorf("a computer is made once, and was made %d times:\n%s", n, calls)
	}
	if deleted, _ := os.ReadFile(filepath.Join(root, "deleted.log")); len(deleted) != 0 {
		t.Errorf("a job ending removes the job's directory and not the computer: %q", deleted)
	}
	said, _ := os.ReadFile(second.OutFile("transcript.log"))
	if !strings.Contains(string(said), "second job sees: remembered") {
		t.Errorf("what the first job left in the computer is there for the second:\n%s", said)
	}
	first1, _ := os.ReadFile(first.OutFile("transcript.log"))
	if !strings.Contains(string(first1), "first job in /jobs/20260929-1200-first/workspace") {
		t.Errorf("a job works in a directory of its own in the computer:\n%s", first1)
	}
	for _, gone := range []string{"20260929-1200-first", "20260929-1201-second"} {
		if _, err := os.Stat(filepath.Join(root, "c-12345678-abcdefgh", "sandbox", "jobs", gone)); err == nil {
			t.Errorf("job %s's directory was left in the computer", gone)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "c-12345678-abcdefgh", "sandbox", "notes.txt")); err != nil {
		t.Error("the computer lost what was in it")
	}
	// Each job's record is its own: its workspace came back as it was after its own turn.
	if answer, _ := os.ReadFile(filepath.Join(first.Workspace(), "answer.txt")); string(answer) != "yes" {
		t.Errorf("%q", answer)
	}
}

// The guard's log belongs to the computer, and each job's record has only what happened
// during it.
func TestEachJobInAComputerHasOnlyItsOwnPartOfTheGuardsLog(t *testing.T) {
	t.Setenv("OPENSHELL_FAKE_EVENTS", "1")
	root := installFake(t)
	name := "c-12345678-abcdefgh"
	os.MkdirAll(filepath.Join(root, name), 0o755)
	write(t, filepath.Join(root, name, "events.log"), "before any job 1\nbefore any job 2\n")

	first, _ := aJob(t)
	withHarness(t, `printf yes > answer.txt; echo "first job event" >> ../../../../events.log`)
	if _, err := runner.Outside(first, inComputer(t, first, name, "20260929-1200-first")); err != nil {
		t.Fatal(err)
	}
	second, _ := aJob(t)
	withHarness(t, `printf yes > answer.txt; echo "second job event" >> ../../../../events.log`)
	if _, err := runner.Outside(second, inComputer(t, second, name, "20260929-1201-second")); err != nil {
		t.Fatal(err)
	}
	one, _ := os.ReadFile(first.OutFile(runner.GuardEvents))
	two, _ := os.ReadFile(second.OutFile(runner.GuardEvents))
	if strings.TrimSpace(string(one)) != "first job event" {
		t.Errorf("the first job's part of the log: %q", one)
	}
	if strings.TrimSpace(string(two)) != "second job event" {
		t.Errorf("the second job's part is not the first's or what was there before either: %q", two)
	}
}

func TestAComputerThatWasStoppedIsStartedAgainForTheNextJob(t *testing.T) {
	root := installFake(t)
	name := "c-12345678-abcdefgh"
	first, _ := aJob(t)
	withHarness(t, `printf yes > answer.txt`)
	if _, err := runner.Outside(first, inComputer(t, first, name, "20260929-1200-first")); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(root, name, "stopped"), nil, 0o644)
	second, _ := aJob(t)
	withHarness(t, `printf yes > answer.txt; echo ran`)
	if exit, err := runner.Outside(second, inComputer(t, second, name, "20260929-1201-second")); err != nil || exit != 0 {
		t.Fatalf("a stopped computer is started for a job: exit %d, %v", exit, err)
	}
	if started, _ := os.ReadFile(filepath.Join(root, "starts.log")); strings.TrimSpace(string(started)) != name {
		t.Errorf("it was asked to start once, and only when it did not answer: %q", started)
	}
	calls, _ := os.ReadFile(filepath.Join(root, "calls.log"))
	if n := strings.Count(string(calls), "sandbox create"); n != 1 {
		t.Errorf("a stopped computer is not made again: %d", n)
	}
}

// What one job's setup put where the harness looks for skills is not the next job's.
func TestASkillOneJobWasGivenIsNotLeftForTheNext(t *testing.T) {
	root := installFake(t)
	name := "c-12345678-abcdefgh"
	first, spec := aJob(t)
	harnesses := t.TempDir()
	write(t, filepath.Join(harnesses, "fixture.json"), `{"name":"fixture","program":"sh","skills":"/root/.config/fixture/skills",
	  "argv":["-c","printf yes > answer.txt; echo skills: $(ls ../../../.config/fixture/skills 2>/dev/null | tr '\n' ' ')"],"needs":"nothing","stream":"plain"}`)
	t.Setenv("HAAS_HARNESSES", harnesses)
	os.MkdirAll(filepath.Join(first.SetupDir(), "skills", "review"), 0o755)
	write(t, filepath.Join(first.SetupDir(), "skills", "review", "SKILL.md"), "review\n")
	spec.Setup = &job.Setup{Name: "engineering", Skills: []string{"review"}}
	spec.Write(first)
	if _, err := runner.Outside(first, inComputer(t, first, name, "20260929-1200-first")); err != nil {
		t.Fatal(err)
	}
	if said, _ := os.ReadFile(first.OutFile("transcript.log")); !strings.Contains(string(said), "skills: review") {
		t.Fatalf("the first job has its skill:\n%s", said)
	}
	second, _ := aJob(t)
	t.Setenv("HAAS_HARNESSES", harnesses)
	if _, err := runner.Outside(second, inComputer(t, second, name, "20260929-1201-second")); err != nil {
		t.Fatal(err)
	}
	if said, _ := os.ReadFile(second.OutFile("transcript.log")); strings.Contains(string(said), "review") {
		t.Errorf("the second job was left the first's skill:\n%s", said)
	}
	_ = root
}

// A job's id is made into a name that stays inside its directory in the computer, whatever
// letters or dots or slashes it has, and two that differ stay different.
func TestAJobInAComputerAlwaysWorksInADirectoryOfItsOwnInsideTheJobsDirectory(t *testing.T) {
	installFake(t)
	dir, _ := aJob(t)
	configured, _ := harness.Open(dir, "fixture")
	seen := map[string]string{}
	for _, id := range []string{"20260929-1200-fix-the-adder", "../escape", "a/b", "A-Capital", "-lead", "with space", "20260929-1200-caf\u00e9", "20260929-1200-caf\u00e8",
		strings.Repeat("a", 200), "..", ".", "\u0430\u0431\u0432"} {
		box, err := openshell.NewComputer(openshell.Config{Binary: "openshell"}, "c-x", id, "", configured.Adapter, "http://m", "http://m", "t")
		if err != nil {
			t.Errorf("%q: %v", id, err)
			continue
		}
		work := box.Workdir()
		relative, found := strings.CutPrefix(work, "/sandbox/jobs/")
		name, tail, _ := strings.Cut(relative, "/")
		if !found || tail != "workspace" || name == "" || strings.ContainsAny(name, "/.\\ ") || len(name) > 81 || name[0] == '-' {
			t.Errorf("%q works at %q", id, work)
		}
		if other, twice := seen[name]; twice {
			t.Errorf("%q and %q both work in %s", id, other, name)
		}
		seen[name] = id
	}
	if got := box("20260929-1200-fix-the-adder", t, configured.Adapter); !strings.HasSuffix(got, "/20260929-1200-fix-the-adder/workspace") {
		t.Errorf("an id that is plain is used as it is: %q", got)
	}
}

func box(id string, t *testing.T, adapter harness.Adapter) string {
	b, err := openshell.NewComputer(openshell.Config{Binary: "openshell"}, "c-x", id, "", adapter, "http://m", "http://m", "t")
	if err != nil {
		t.Fatal(err)
	}
	return b.Workdir()
}

// The harness ties a conversation to the directory it began in, so every turn of an instance
// works in one directory of the computer, and what an earlier turn left in it that the
// instance no longer has is not there. A job that is not a turn keeps to a directory of its own.
func TestTurnsOfOneInstanceShareADirectoryThatIsKeptAndIsGivenTheInstancesFilesAgain(t *testing.T) {
	root := installFake(t)
	kept := filepath.Join(root, "c-12345678-abcdefgh", "sandbox", "jobs", "instance-20260929-1200-notes")
	first, _ := aJob(t)
	withHarness(t, `printf yes > answer.txt; printf stale > only-in-the-first-turn.txt; echo turn in $(pwd | sed 's|.*/sandbox||')`)
	if exit, err := runner.Outside(first, inComputerFor(t, first, "c-12345678-abcdefgh", "20260929-1200-first", "20260929-1200-notes")); err != nil || exit != 0 {
		t.Fatalf("exit %d, %v", exit, err)
	}
	if _, err := os.Stat(kept); err != nil {
		t.Fatalf("the conversation's directory goes when a turn ends: %v", err)
	}
	second, _ := aJob(t)
	withHarness(t, `printf yes > answer.txt; ls; echo turn in $(pwd | sed 's|.*/sandbox||')`)
	if exit, err := runner.Outside(second, inComputerFor(t, second, "c-12345678-abcdefgh", "20260929-1201-second", "20260929-1200-notes")); err != nil || exit != 0 {
		t.Fatalf("exit %d, %v", exit, err)
	}
	for _, dir := range []job.Dir{first, second} {
		said, _ := os.ReadFile(dir.OutFile("transcript.log"))
		if !strings.Contains(string(said), "turn in /jobs/instance-20260929-1200-notes/workspace") {
			t.Errorf("both turns work in the instance's directory:\n%s", said)
		}
	}
	said, _ := os.ReadFile(second.OutFile("transcript.log"))
	if strings.Contains(string(said), "only-in-the-first-turn.txt") {
		t.Errorf("the second turn was given what the first left, and not the instance's files:\n%s", said)
	}
	other, _ := aJob(t)
	withHarness(t, `printf yes > answer.txt; echo one-off in $(pwd | sed 's|.*/sandbox||')`)
	if _, err := runner.Outside(other, inComputer(t, other, "c-12345678-abcdefgh", "20260929-1202-oneoff")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "c-12345678-abcdefgh", "sandbox", "jobs", "20260929-1202-oneoff")); err == nil {
		t.Error("a job that is not a turn left its directory")
	}
}
