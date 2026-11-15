package main

import (
	"strings"
	"testing"
)

// The environment column is what a reader trusts to know where a row ran, so a
// plan run against a gateway must say so and must not say it ran in a container.
func TestAMatrixSaysItRanInTheSandboxItRanIn(t *testing.T) {
	t.Setenv("GUSTO_SANDBOX", "openshell")
	t.Setenv("GUSTO_OPENSHELL_IMAGE", "/images/runner.tar.gz")
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

	t.Setenv("GUSTO_SANDBOX", "")
	way, err = matrixWay(false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(way.Where, "in a container, ") {
		t.Fatalf("without a gateway the cells run in containers: %q", way.Where)
	}
}
