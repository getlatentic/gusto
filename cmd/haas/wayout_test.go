package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/job"
)

// A sealed box reaches its model only through the proxy, on its own loopback,
// so the runner inside cannot tell where the code goes. The server can: the
// credential names the provider. Restricted code bound for a provider outside
// the building is refused before a box starts, and says so where the job is
// read.
func TestRestrictedCodeIsRefusedACredentialOutsideTheBuilding(t *testing.T) {
	credentials := t.TempDir()
	os.WriteFile(filepath.Join(credentials, "deepseek.json"),
		[]byte(`{"address": "https://api.deepseek.com", "key": "sk-test"}`), 0o600)
	t.Setenv("HAAS_CREDENTIALS", credentials)
	way, err := openWayOut(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := job.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	spec := job.Spec{Brief: "b", Harness: "opencode", Model: "deepseek/deepseek-flash", Sensitivity: job.Restricted}
	if _, err := way.Run(dir, spec); err == nil || !strings.Contains(err.Error(), "api.deepseek.com") {
		t.Fatalf("restricted code was not refused the provider: %v", err)
	}
	said, _ := os.ReadFile(dir.OutFile("transcript.log"))
	if !strings.Contains(string(said), "restricted") {
		t.Errorf("the refusal is not where the job is read: %q", said)
	}
}

// A harness that needs no model has no credential to name, and a deployment that holds credentials still runs it, as
// the install check's first job does. A harness that does need one is still asked to name it.
func TestAHarnessThatNeedsNoModelIsNotRefusedForWantOfACredential(t *testing.T) {
	harnesses := t.TempDir()
	os.WriteFile(filepath.Join(harnesses, "plain.json"), []byte(`{"name":"plain","program":"true","needs":"nothing","stream":"plain"}`), 0o600)
	t.Setenv("HAAS_HARNESSES", harnesses)
	dir := job.Dir(t.TempDir())
	if needsModel(dir, job.Spec{Harness: "plain"}) {
		t.Error("a harness that needs nothing was said to need a model")
	}
	if !needsModel(dir, job.Spec{Harness: "opencode"}) || !needsModel(dir, job.Spec{Harness: "nowhere"}) {
		t.Error("a harness that needs a model, or is not found, was said to need none")
	}
}
