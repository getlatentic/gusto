package job_test

import (
	"os"
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/job"
)

func TestAJobWithNoVersionReadsAsVersionOne(t *testing.T) {
	dir := job.Dir(t.TempDir())
	spec := `{"brief": "b", "repo": "r", "gate": ["true"], "harness": "h", "priority": "unknown here"}`
	if err := os.WriteFile(dir.Spec(), []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}
	read, err := job.Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if read.Version != 1 || read.Brief != "b" {
		t.Fatalf("version %d, brief %q", read.Version, read.Brief)
	}
}

func TestAWrittenJobCarriesItsVersion(t *testing.T) {
	dir := job.Dir(t.TempDir())
	if err := (job.Spec{Brief: "b", Repo: "r", Gate: []string{"true"}, Harness: "h"}).Write(dir); err != nil {
		t.Fatal(err)
	}
	text, err := os.ReadFile(dir.Spec())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(text), `"version": 1`) {
		t.Fatalf("want the version written:\n%s", text)
	}
}

func TestAVersionThisBuildDoesNotKnowIsRefused(t *testing.T) {
	for _, got := range []int{2, -1} {
		err := job.Readable("job.json", got, 1)
		if err == nil || !strings.Contains(err.Error(), "reads up to version 1") {
			t.Errorf("version %d: want refused by name, got %v", got, err)
		}
	}
	if err := job.Readable("job.json", 0, 1); err != nil {
		t.Fatalf("no version is version 1: %v", err)
	}
}
