package runner_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/getlatentic/haas/internal/evidence"
	"github.com/getlatentic/haas/internal/harness"
	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/runner"
)

// configuredRun runs a job carrying a configured harness whose adapter is
// given, beside a prompt, and returns the job and the digest the job carried
// before it ran.
func configuredRun(t *testing.T, adapter string) (job.Dir, string) {
	t.Helper()
	mine := t.TempDir()
	if err := os.MkdirAll(filepath.Join(mine, "configured"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(mine, "configured", "adapter.json"), adapter)
	write(t, filepath.Join(mine, "configured", "prompt.md"), "Answer yes.\n")
	t.Setenv("HAAS_HARNESSES", mine)
	dir := job.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	spec := job.Spec{
		Brief:   "make the answer yes",
		Repo:    repo(t),
		Gate:    []string{"sh", "-c", `test "$(cat answer.txt)" = yes`},
		Harness: "configured",
	}
	if err := spec.Write(dir); err != nil {
		t.Fatal(err)
	}
	if err := harness.Keep(dir, "configured"); err != nil {
		t.Fatal(err)
	}
	carried, found, err := harness.Carrying(dir, "configured")
	if err != nil || !found {
		t.Fatalf("the job should carry its harness, found %v: %v", found, err)
	}
	if exit, err := runner.Inside(dir); err != nil || exit != 0 {
		t.Fatalf("exit %d: %v", exit, err)
	}
	return dir, carried.Digest
}

func TestTheManifestNamesTheConfiguredHarnessTheJobCarried(t *testing.T) {
	dir, carried := configuredRun(t, `{"name": "configured", "program": "sh",
		"argv": ["-c", "printf yes > answer.txt"], "needs": "nothing"}`)
	manifest, err := evidence.Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.HarnessDigest != carried {
		t.Fatalf("the manifest says %q, the job carried %s", manifest.HarnessDigest, carried)
	}
	if wrong, err := evidence.Check(dir); err != nil || len(wrong) != 0 {
		t.Fatalf("an untouched harness should verify: %v %v", wrong, err)
	}
}

// A harness that learns by writing to itself changes the configuration two
// runs would share. The manifest keeps the digest taken before it started, and
// the check names the harness as changed, so the record never passes off the
// rewritten configuration as the one that ran.
func TestAHarnessThatWritesToItselfIsCaught(t *testing.T) {
	dir, carried := configuredRun(t, `{"name": "configured", "program": "sh",
		"argv": ["-c", "printf yes > answer.txt; echo learned >> {harnessDir}/prompt.md"], "needs": "nothing"}`)
	manifest, err := evidence.Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.HarnessDigest != carried {
		t.Fatalf("the manifest says %q, the job carried %s before the run", manifest.HarnessDigest, carried)
	}
	wrong, err := evidence.Check(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(wrong, "harnesses/configured changed") {
		t.Fatalf("want the rewritten harness named, got %v", wrong)
	}
}
