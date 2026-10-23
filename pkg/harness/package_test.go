package harness

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// The runner image installs each harness from the deployment's list, and a person installs it from the adapter's package, so
// the two name the same package and version.
func TestEachAdaptersPackageIsTheOneTheRunnerImageInstalls(t *testing.T) {
	text, err := os.ReadFile("../../deploy/ansible/group_vars/all.yml")
	if err != nil {
		t.Fatal(err)
	}
	block, _, _ := strings.Cut(strings.SplitN(string(text), "runner_harnesses:", 2)[1], "\n\n")
	listed := map[string]string{}
	for _, member := range regexp.MustCompile(`(?m)^\s+- (\S+)=(\S+)\s*$`).FindAllStringSubmatch(block, -1) {
		listed[member[1]] = member[2]
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
