package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/job"
)

// The environment column is what a reader trusts to know where a row ran, so a
// plan run against a gateway must say so and must not say it ran in a container.
func TestAMatrixSaysItRanInTheSandboxItRanIn(t *testing.T) {
	t.Setenv("HAAS_SANDBOX", "openshell")
	t.Setenv("HAAS_OPENSHELL_IMAGE", "/images/runner.tar.gz")
	way, err := matrixWay(false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(way.Where, "OpenShell sandbox") || !strings.Contains(way.Where, "/images/runner.tar.gz") {
		t.Fatalf("a sandbox run should say so, and which image: %q", way.Where)
	}
	if strings.Contains(way.Where, "container") {
		t.Fatalf("a sandbox run is not a container run: %q", way.Where)
	}

	t.Setenv("HAAS_SANDBOX", "")
	way, err = matrixWay(false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(way.Where, "in a container, ") {
		t.Fatalf("without a gateway the cells run in containers: %q", way.Where)
	}
}

// A cell run against a gateway is refused by the sandbox path, in words that
// name it. A container start would have failed trying to reach docker.
func TestAMatrixCellOnAGatewayIsRunByTheSandboxPath(t *testing.T) {
	t.Setenv("HAAS_SANDBOX", "openshell")
	t.Setenv("HAAS_CONTAINER", "no-such-container-runtime")
	way, err := matrixWay(false)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	run, stop, err := way.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	dir := job.Dir(filepath.Join(root, "cell"))
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	if err := (job.Spec{Brief: "b", Harness: "opencode", Model: "m/x", Gate: []string{"true"}}).Write(dir); err != nil {
		t.Fatal(err)
	}
	if err := run(dir); err == nil {
		t.Fatal("a gateway with no credential to reach a model with should refuse the cell")
	}
	refused, err := os.ReadFile(dir.OutFile("transcript.log"))
	if err != nil || !strings.Contains(string(refused), "sandbox on a gateway") {
		t.Fatalf("the cell should be refused by the sandbox path: %q %v", refused, err)
	}
}
