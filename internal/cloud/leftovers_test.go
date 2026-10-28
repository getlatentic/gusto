package cloud

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeDocker writes each removal it is asked for, and lists one container for any organization.
const fakeDocker = `#!/bin/sh
case "$1" in
ps) echo "c-of-${4##*=}";;
rm) shift 2; echo "$@" >> "$REMOVED";;
esac
`

func leftoverRuntime(t *testing.T) (runtime, removed string) {
	t.Helper()
	dir := t.TempDir()
	runtime, removed = filepath.Join(dir, "docker"), filepath.Join(dir, "removed")
	if err := os.WriteFile(runtime, []byte(fakeDocker), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REMOVED", removed)
	return runtime, removed
}

func TestAServerThatRunsContainersHasItsLeftoversRemovedWhenItStopsAndBeforeItStarts(t *testing.T) {
	runtime, removed := leftoverRuntime(t)
	cell := &LocalCell{}
	spec := ServerSpec{Slug: "acme-com", Command: []string{"sleep", "60"}, Log: filepath.Join(t.TempDir(), "log"),
		Env: []string{"GUSTO_SANDBOX=openshell", "GUSTO_SANDBOX=container", "GUSTO_CONTAINER=" + runtime}}
	if err := cell.Start(spec); err != nil {
		t.Fatal(err)
	}
	if err := cell.Start(spec); err != nil {
		t.Fatal(err)
	}
	if err := cell.Stop("acme-com", time.Second); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(removed)
	if lines := strings.Fields(string(got)); len(lines) != 2 || lines[0] != "c-of-acme-com" || lines[1] != "c-of-acme-com" {
		t.Errorf("removed before the first start and after the stop, and not while it ran: %q", got)
	}
}

func TestAServerThatRunsNoContainersHasNothingRemoved(t *testing.T) {
	runtime, removed := leftoverRuntime(t)
	cell := &LocalCell{}
	spec := ServerSpec{Slug: "acme-com", Command: []string{"sleep", "60"}, Log: filepath.Join(t.TempDir(), "log"),
		Env: []string{"GUSTO_SANDBOX=openshell", "GUSTO_CONTAINER=" + runtime}}
	if err := cell.Start(spec); err != nil {
		t.Fatal(err)
	}
	cell.Stop("acme-com", time.Second)
	if _, err := os.Stat(removed); !os.IsNotExist(err) {
		t.Error("a server in OpenShell had containers removed")
	}
}
