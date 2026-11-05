package main

import (
	"path/filepath"
	"testing"

	"github.com/getlatentic/gusto/internal/computer"
	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/internal/sandbox"
	"github.com/getlatentic/gusto/pkg/job"
)

// A person's turns in containers share a home of theirs, kept beside their computer's record, so what they installed is there
// next time. A script's task is nobody's, and a room's turn sees only the room: neither is given a person's home.
func TestAPersonsTurnsShareAHomeOfTheirOwnAndARoomOrAScriptHasNone(t *testing.T) {
	t.Setenv("GUSTO_ORG", "acme-research-com")
	computers := filepath.Join(t.TempDir(), "computers")
	t.Setenv("GUSTO_COMPUTERS", computers)
	index, err := jobs.Open(filepath.Join(t.TempDir(), "jobs"))
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range []jobs.Record{
		{ID: "20261105-0900-a-turn", State: jobs.Queued, Asked: "Bob@acme-research.com"},
		{ID: "20261105-0901-a-script", State: jobs.Queued, Asked: "token:ci"},
	} {
		if err := job.Dir(filepath.Join(index.Root(), record.ID)).Make(); err != nil {
			t.Fatal(err)
		}
		if err := index.Add(record); err != nil {
			t.Fatal(err)
		}
	}
	turn := job.Dir(filepath.Join(index.Root(), "20261105-0900-a-turn"))
	mounts, err := keptHome(index, turn, job.Spec{Session: "20261105-0900-opencode"})
	if err != nil {
		t.Fatal(err)
	}
	want := sandbox.Mount{From: filepath.Join(computers, computer.ID("acme-research-com", "bob@acme-research.com"), "home"), To: sandbox.Home, Kept: true}
	if len(mounts) != 1 || mounts[0] != want {
		t.Fatalf("got %+v, want %+v", mounts, want)
	}
	for name, each := range map[string]struct {
		dir  job.Dir
		spec job.Spec
	}{
		"a script's task": {job.Dir(filepath.Join(index.Root(), "20261105-0901-a-script")), job.Spec{}},
		"a room's turn":   {turn, job.Spec{Session: "20261105-0900-room", Agent: "opencode"}},
	} {
		if mounts, err := keptHome(index, each.dir, each.spec); err != nil || mounts != nil {
			t.Errorf("%s was given %+v (%v)", name, mounts, err)
		}
	}
	t.Setenv("GUSTO_COMPUTERS", "")
	if mounts, _ := keptHome(index, turn, job.Spec{}); mounts != nil {
		t.Errorf("a server that keeps no computers kept a home: %+v", mounts)
	}
}
