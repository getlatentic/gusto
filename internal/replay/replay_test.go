package replay_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/evidence"
	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/replay"
)

func TestAReplayRunsTheSameJobAtTheCommitTheOldRunStartedFrom(t *testing.T) {
	hermetic(t)
	bare := remote(t)
	started := git(t, bare, "rev-parse", "main")
	old := finished(t, t.TempDir(), "fix", "yes", bare)
	advance(t, bare)
	before := read(t, old.OutFile("manifest.json"))

	rows := replay.Sweep([]job.Dir{old}, "", replay.Here)

	row := rows[0]
	if row.Err != nil {
		t.Fatal(row.Err)
	}
	if row.Change() != replay.Same || !row.After.Gate.Passed {
		t.Fatalf("the same harness on the same commit should pass again, got %s", row.Change())
	}
	if spec, _ := job.Read(row.Replay); spec.Ref != started {
		t.Fatalf("the replay should ask for %s, the commit the old run started from, and asks for %s", started, spec.Ref)
	}
	if at := git(t, row.Replay.Workspace(), "rev-parse", "HEAD"); at != started {
		t.Fatalf("the replay worked on %s rather than %s", at, started)
	}
	if _, err := os.Stat(filepath.Join(row.Replay.Workspace(), "later.txt")); err == nil {
		t.Fatal("the replay saw a commit made after the old run, so it did not run the same job")
	}
	if after := read(t, old.OutFile("manifest.json")); after != before {
		t.Fatal("replaying a job rewrote its manifest")
	}
	if wrong, err := evidence.Check(old); err != nil || len(wrong) > 0 {
		t.Fatalf("replaying a job disturbed its evidence: %v %v", wrong, err)
	}
}

func TestAReplayIsToldApartFromTheOriginalByItsOwnHashedEvidence(t *testing.T) {
	hermetic(t)
	old := finished(t, t.TempDir(), "fix", "yes", remote(t))
	row := replay.Sweep([]job.Dir{old}, "", replay.Here)[0]
	if row.Err != nil {
		t.Fatal(row.Err)
	}

	mark, isReplay, err := replay.Marked(row.Replay)
	if err != nil || !isReplay {
		t.Fatalf("the replay does not say it is one: %v", err)
	}
	if _, isReplay, _ := replay.Marked(old); isReplay {
		t.Fatal("the original reads as a replay")
	}
	absolute, _ := filepath.Abs(old.Path())
	if mark.Of != absolute || len(mark.Manifest) != 64 {
		t.Fatalf("the mark should name the original and its manifest, got %+v", mark)
	}
	named := false
	for _, entry := range row.After.Files {
		named = named || entry.Name == replay.MarkFile
	}
	if !named {
		t.Fatal("the replay's manifest does not hash the mark, so it could be removed without a trace")
	}
}

// A harness that commits moves HEAD onto a commit only its workspace has. A
// replay pinned to HEAD would fail to check it out, or start with the answer
// already in the tree.
func TestAHarnessThatCommittedDoesNotMoveThePin(t *testing.T) {
	hermetic(t)
	bare := remote(t)
	started := git(t, bare, "rev-parse", "main")
	old := finished(t, t.TempDir(), "fix", "committer", bare)
	if head := git(t, old.Workspace(), "rev-parse", "HEAD"); head == started {
		t.Fatal("the fixture harness should have committed")
	}

	row := replay.Sweep([]job.Dir{old}, "", replay.Here)[0]

	if row.Err != nil {
		t.Fatal(row.Err)
	}
	mark, _, _ := replay.Marked(row.Replay)
	if mark.Commit != started || mark.HarnessCommits != 1 || mark.PinnedBy != "refs/remotes/origin/HEAD" {
		t.Fatalf("the pin should be %s with one harness commit on top, got %+v", started, mark)
	}
	if row.Change() != replay.Same {
		t.Fatalf("the replay should pass again from the original commit, got %s", row.Change())
	}
}

func TestAHarnessThatGotWorseFailsTheSweep(t *testing.T) {
	harnesses := hermetic(t)
	old := finished(t, t.TempDir(), "fix", "yes", remote(t))
	write(t, filepath.Join(harnesses, "yes.json"), regressedTo)

	rows := replay.Sweep([]job.Dir{old}, t.TempDir(), replay.Here)

	if rows[0].Err != nil {
		t.Fatal(rows[0].Err)
	}
	if rows[0].Change() != replay.Worse || replay.ExitCode(rows) != 1 {
		t.Fatalf("a job that passed and now fails got worse, got %s and exit %d", rows[0].Change(), replay.ExitCode(rows))
	}
}

func TestASweepOverADirectoryOfJobsGivesOneSummary(t *testing.T) {
	hermetic(t)
	bare := remote(t)
	jobs := t.TempDir()
	finished(t, jobs, "a-passes", "yes", bare)
	finished(t, jobs, "b-fails", "maybe", bare)
	into := t.TempDir()

	olds, err := replay.Expand([]string{jobs})
	if err != nil {
		t.Fatal(err)
	}
	rows := replay.Sweep(olds, into, replay.Here)
	var out bytes.Buffer
	replay.Print(&out, rows)

	if len(rows) != 2 || replay.ExitCode(rows) != 0 {
		t.Fatalf("two jobs that held should exit 0, got %d rows and exit %d:\n%s", len(rows), replay.ExitCode(rows), &out)
	}
	for _, want := range []string{"a-passes", "b-fails", "passed (0)", "failed (1)", "2 replayed: 0 worse, 0 better, 2 same, 0 could not run"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("the summary should say %q:\n%s", want, &out)
		}
	}
	if !strings.HasPrefix(rows[0].Replay.Path(), into) {
		t.Fatalf("the replay went to %s rather than under %s", rows[0].Replay, into)
	}
}

func TestASecondSweepReplaysTheOriginalsAndNotTheFirstSweepsReplays(t *testing.T) {
	hermetic(t)
	jobs := t.TempDir()
	finished(t, jobs, "fix", "yes", remote(t))
	first, _ := replay.Expand([]string{jobs})
	replay.Sweep(first, "", replay.Here)

	second, err := replay.Expand([]string{jobs})
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 1 || filepath.Base(second[0].Path()) != "fix" {
		t.Fatalf("the second sweep should find the one original, found %v", second)
	}
	rows := replay.Sweep(second, "", replay.Here)
	if filepath.Base(rows[0].Replay.Path()) != "fix-replay-2" {
		t.Fatalf("a second replay should not reuse the first one's directory, got %s", rows[0].Replay)
	}
}

func TestAJobWhoseEvidenceChangedIsNotReplayedAndTheSweepSaysSo(t *testing.T) {
	hermetic(t)
	bare := remote(t)
	jobs := t.TempDir()
	tampered := finished(t, jobs, "a-tampered", "maybe", bare)
	fine := finished(t, jobs, "b-fine", "yes", bare)
	write(t, tampered.OutFile("gates.json"), `{"passed": true}`)

	rows := replay.Sweep([]job.Dir{tampered, fine}, "", replay.Here)

	if rows[0].Err == nil || rows[0].Replay != "" {
		t.Fatal("a job whose evidence changed has no verdict worth comparing against")
	}
	if rows[1].Err != nil || rows[1].Change() != replay.Same {
		t.Fatalf("one broken job should not stop the rest: %v", rows[1].Err)
	}
	if replay.ExitCode(rows) != 2 {
		t.Fatalf("a job that could not be replayed should exit 2, got %d", replay.ExitCode(rows))
	}
}

func TestAReplayCarriesTheAdapterTheOldJobCarried(t *testing.T) {
	hermetic(t)
	old := finished(t, t.TempDir(), "fix", "yes", remote(t))
	// Carried is written after the run, as a job made by `haas new` would hold
	// it. It is outside out/, so the evidence still checks.
	if err := os.MkdirAll(filepath.Join(old.Path(), "harnesses"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(old.Path(), "harnesses", "yes.json"), answersYes)
	t.Setenv("HAAS_HARNESSES", t.TempDir())

	row := replay.Sweep([]job.Dir{old}, "", replay.Here)[0]

	if row.Err != nil {
		t.Fatalf("the replay should run on the adapter the job carried: %v", row.Err)
	}
	if read(t, filepath.Join(row.Replay.Path(), "harnesses", "yes.json")) != answersYes {
		t.Fatal("the replay does not carry the old job's adapter")
	}
}
