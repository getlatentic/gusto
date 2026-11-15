package harness

import (
	"os"
	"strings"
	"testing"
)

// runnerList is the entries of one of the runner image's lists in deploy/runner, without its comments.
func runnerList(t *testing.T, name string) []string {
	t.Helper()
	text, err := os.ReadFile("../../deploy/runner/" + name)
	if err != nil {
		t.Fatal(err)
	}
	var entries []string
	for _, line := range strings.Split(string(text), "\n") {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
			entries = append(entries, line)
		}
	}
	return entries
}

// The runner image installs each harness from its list, and a person installs it from the adapter's package, so
// the two name the same package and version.
func TestEachAdaptersPackageIsTheOneTheRunnerImageInstalls(t *testing.T) {
	listed := map[string]string{}
	for _, line := range runnerList(t, "harnesses.txt") {
		name, pkg, _ := strings.Cut(line, "=")
		listed[name] = pkg
	}
	for _, name := range Names() {
		adapter, err := Load("", name)
		if err != nil {
			t.Fatal(err)
		}
		if adapter.Package != listed[name] {
			t.Errorf("%s: the adapter installs %q and the runner image %q", name, adapter.Package, listed[name])
		}
	}
}
