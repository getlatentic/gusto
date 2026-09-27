package schema_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/evidence"
	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/signing"
	"github.com/getlatentic/haas/schema"
)

// Each schema's $id carries the version its const names, so a reader can tell
// from either which one it holds.
func TestEachSchemaIsVersionedAndAgreesWithTheCode(t *testing.T) {
	for _, tc := range []struct {
		text    []byte
		id      string
		version int
	}{
		{schema.Job(), "/schema/job/v1.json", job.Version},
		{schema.Manifest(), "/schema/manifest/v1.json", evidence.Version},
		{schema.Signature(), "/schema/signature/v1.json", signing.Version},
	} {
		var read struct {
			ID         string `json:"$id"`
			Properties struct {
				Version struct {
					Const int `json:"const"`
				} `json:"version"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(tc.text, &read); err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(read.ID, tc.id) || read.Properties.Version.Const != tc.version {
			t.Fatalf("$id %q with version %d, want %s with version %d", read.ID, read.Properties.Version.Const, tc.id, tc.version)
		}
	}
}

// The job `haas new` writes is the one people start from, so the test builds
// the command and validates its output rather than a spec made in the test.
func TestAJobFromHaasNewMatchesTheSchema(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "haas")
	build := exec.Command("go", "build", "-o", binary, "../cmd/haas")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v: %s", err, output)
	}
	dir := filepath.Join(t.TempDir(), "job")
	create := exec.Command(binary, "new", dir, "-repo", "https://example.com/r.git",
		"-brief", "make the answer yes", "-harness", "opencode", "--", "sh", "-c", "test yes = yes")
	if output, err := create.CombinedOutput(); err != nil {
		t.Fatalf("haas new: %v: %s", err, output)
	}
	written, err := os.ReadFile(job.Dir(dir).Spec())
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(schema.Job(), written); err != nil {
		t.Fatalf("haas new wrote a job the schema refuses:\n%v\n%s", err, written)
	}
	if !strings.Contains(string(written), `"version": 1`) {
		t.Fatalf("a writer puts the version in:\n%s", written)
	}
}

// A hand-written job with only what a reader needs is valid; the rest has
// defaults.
func TestTheSmallestJobMatchesTheSchema(t *testing.T) {
	smallest := `{"brief": "b", "repo": "r", "gate": ["true"], "harness": "h"}`
	if err := schema.Validate(schema.Job(), []byte(smallest)); err != nil {
		t.Fatal(err)
	}
}

func TestAJobWithATeamAndACapMatchesTheSchema(t *testing.T) {
	document := `{"brief": "b", "repo": "r", "gate": ["true"], "harness": "h", "team": "platform", "budget": {"usd": 2.5}}`
	if err := schema.Validate(schema.Job(), []byte(document)); err != nil {
		t.Fatal(err)
	}
}

func TestTheJobSchemaRefusesWhatTheReaderRefuses(t *testing.T) {
	for _, document := range []string{
		`{"brief": "b", "repo": "r", "gate": ["true"], "harness": "h", "sensitivity": "secret"}`,
		`{"brief": "b", "repo": "r", "gate": ["true"], "harness": "h", "version": 2}`,
		`{"repo": "r", "gate": ["true"], "harness": "h"}`,
	} {
		if err := schema.Validate(schema.Job(), []byte(document)); err == nil {
			t.Errorf("want refused: %s", document)
		}
	}
}

// The digest joined the manifest at version 1, so a manifest with it and one
// from before it are both version 1.
func TestAManifestIsValidWithAndWithoutTheHarnessDigest(t *testing.T) {
	before := `{"job": "j", "files": [], "harness": "opencode", "harnessExit": 0, "stoppedAtBudget": false,
		"modelEndpoint": "", "tokens": null, "costUsd": null, "signer": "unsigned", "atUnix": 1}`
	after := strings.Replace(before, `"harness": "opencode",`,
		`"harness": "opencode", "harnessDigest": "sha256:3e8c088aa809533e7f9fa7a54f545d896c2dcbe9a3c81b4d543ad97de277fa7f",`, 1)
	for _, document := range []string{before, after} {
		if err := schema.Validate(schema.Manifest(), []byte(document)); err != nil {
			t.Errorf("%v\n%s", err, document)
		}
	}
}
