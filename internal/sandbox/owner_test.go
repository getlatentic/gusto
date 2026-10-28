package sandbox

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestAContainerIsLabelledWithItsOrganizationPersonAgentAndJob(t *testing.T) {
	args, err := sealedArgv("/jobs/20261028-1811-a-turn", "/jobs/.proxy.sock", "t", Owner{Org: "acme-research-com", Person: "3fa1c2d0", Instance: "20261028-1811-opencode"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		LabelPrefix + "org=acme-research-com",
		LabelPrefix + "person=3fa1c2d0",
		LabelPrefix + "instance=20261028-1811-opencode",
		LabelPrefix + "job=20261028-1811-a-turn",
	} {
		at := slices.Index(args, want)
		if at < 1 || args[at-1] != "--label" {
			t.Errorf("want --label %s in %v", want, args)
		}
	}
}

func TestAnOwnerNobodyNamedIsNoLabel(t *testing.T) {
	args, err := argv("/jobs/a", nil, Owner{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	labels := 0
	for _, arg := range args {
		if strings.HasPrefix(arg, LabelPrefix) {
			labels++
			if !strings.HasPrefix(arg, LabelPrefix+"job=") {
				t.Errorf("an empty owner was labelled: %s", arg)
			}
		}
	}
	if labels != 1 {
		t.Errorf("only the job is labelled: %v", args)
	}
}

// fakeRuntime lists the containers named in RUNNING for the filter it is given, and writes what it was asked to remove.
const fakeRuntime = `#!/bin/sh
case "$1" in
ps) if [ "$4" = "label=com.getlatentic.gusto.org=acme-research-com" ]; then printf '%s\n' $RUNNING; fi;;
rm) shift 2; echo "$@" >> "$REMOVED";;
esac
`

func TestRemovingAnOrganizationRemovesItsContainersAndNoOneElses(t *testing.T) {
	dir := t.TempDir()
	runtime, removed := filepath.Join(dir, "runtime"), filepath.Join(dir, "removed")
	if err := os.WriteFile(runtime, []byte(fakeRuntime), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RUNNING", "aaa bbb")
	t.Setenv("REMOVED", removed)
	if err := RemoveOrg(runtime, "acme-research-com"); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(removed); string(got) != "aaa bbb\n" {
		t.Errorf("removed %q", got)
	}
	os.Remove(removed)
	if err := RemoveOrg(runtime, "globex-industries-com"); err != nil {
		t.Fatal(err)
	}
	if err := RemoveOrg(runtime, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(removed); !os.IsNotExist(err) {
		t.Error("another organization's containers, or nobody's, were removed")
	}
}
