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
