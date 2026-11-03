package replay

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/getlatentic/gusto/pkg/job"
)

// A replay cut short while its directory is filled is never taken for an original by a sweep.
func TestAReplayCutShortIsNotTakenForAnOriginal(t *testing.T) {
	t.Setenv("GUSTO_HARNESSES", t.TempDir())
	parent := t.TempDir()
	old := job.Dir(filepath.Join(parent, "fix-it"))
	if err := old.Make(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(old.Spec(), []byte(`{"brief": "fix it", "harness": "nowhere", "repo": "/r", "ref": "main"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	fresh, err := claim(old, parent)
	if err != nil {
		t.Fatal(err)
	}
	if err := fill(old, fresh, "nowhere", Mark{Of: old.Path(), Commit: "abc"}); err == nil {
		t.Fatal("an adapter nobody has was carried")
	}
	found, err := Expand([]string{parent})
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(found, fresh) {
		t.Errorf("a replay that never finished filling is swept as an original: %v", found)
	}
}
