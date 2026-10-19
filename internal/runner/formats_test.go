package runner_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/internal/job"
	"github.com/getlatentic/gusto/internal/runner"
	"github.com/getlatentic/gusto/schema"
)

func TestARunLeavesAManifestTheSchemaAccepts(t *testing.T) {
	dir, _ := ran(t)
	for _, tc := range []struct {
		schema []byte
		path   string
	}{
		{schema.Job(), dir.Spec()},
		{schema.Manifest(), dir.OutFile("manifest.json")},
	} {
		written, err := os.ReadFile(tc.path)
		if err != nil {
			t.Fatal(err)
		}
		if err := schema.Validate(tc.schema, written); err != nil {
			t.Fatalf("%s does not match its schema:\n%v\n%s", filepath.Base(tc.path), err, written)
		}
	}
}

// A job written before the version field, carrying a field a newer writer
// added, is what an old job directory looks like after the format grows. It
// still runs, and its evidence still verifies.
func TestAnOldJobStillRunsAfterTheFormatGainsAField(t *testing.T) {
	harnesses := t.TempDir()
	write(t, filepath.Join(harnesses, "fixture.json"), adapter)
	t.Setenv("GUSTO_HARNESSES", harnesses)
	dir := job.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	old, err := json.Marshal(map[string]any{
		"brief":    "make the answer yes",
		"repo":     repo(t),
		"gate":     []string{"sh", "-c", `test "$(cat answer.txt)" = yes`},
		"harness":  "fixture",
		"priority": "a field from a newer writer",
	})
	if err != nil {
		t.Fatal(err)
	}
	write(t, dir.Spec(), string(old))
	exit, err := runner.Inside(dir)
	if err != nil || exit != 0 {
		t.Fatalf("an old job should run and pass, exit %d: %v", exit, err)
	}
	unversion(t, dir)
	wrong, err := evidence.Check(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(wrong) != 0 {
		t.Fatalf("an old manifest should verify, got %v", wrong)
	}
}

// unversion rewrites the manifest as one from before the version field, with a
// field a newer writer added.
func unversion(t *testing.T, dir job.Dir) {
	t.Helper()
	text, err := os.ReadFile(dir.OutFile("manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(text, &fields); err != nil {
		t.Fatal(err)
	}
	delete(fields, "version")
	fields["attestation"] = "a field from a newer writer"
	text, err = json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	write(t, dir.OutFile("manifest.json"), string(text))
}

func TestAJobFromANewerVersionIsRefusedByName(t *testing.T) {
	dir := job.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	write(t, dir.Spec(), `{"version": 2, "brief": "b", "repo": "r", "gate": ["true"], "harness": "fixture"}`)
	_, err := runner.Inside(dir)
	if err == nil || !strings.Contains(err.Error(), "job.json is version 2") {
		t.Fatalf("want the version named, got %v", err)
	}
}
