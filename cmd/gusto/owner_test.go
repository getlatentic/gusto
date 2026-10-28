package main

import (
	"path/filepath"
	"testing"

	"github.com/getlatentic/gusto/internal/computer"
	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/internal/sandbox"
	"github.com/getlatentic/gusto/pkg/job"
)

// A turn's container is labelled with the organization, the person as the id their computer would have, and the agent; a task a
// script sent is nobody's, so it has no person.
func TestATurnsContainerIsOwnedByItsOrganizationPersonAndAgent(t *testing.T) {
	t.Setenv("GUSTO_ORG", "acme-research-com")
	index, err := jobs.Open(filepath.Join(t.TempDir(), "jobs"))
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range []jobs.Record{
		{ID: "20261028-1811-a-turn", State: jobs.Queued, Asked: "Bob@acme-research.com"},
		{ID: "20261028-1812-a-script", State: jobs.Queued, Asked: "token:ci"},
	} {
		if err := job.Dir(filepath.Join(index.Root(), record.ID)).Make(); err != nil {
			t.Fatal(err)
		}
		if err := index.Add(record); err != nil {
			t.Fatal(err)
		}
	}
	spec := job.Spec{Session: "20261028-1811-opencode"}
	turn := ownerOf(index, job.Dir(filepath.Join(index.Root(), "20261028-1811-a-turn")), spec)
	want := sandbox.Owner{Org: "acme-research-com", Person: computer.ID("acme-research-com", "bob@acme-research.com"), Instance: spec.Session}
	if turn != want {
		t.Errorf("got %+v, want %+v", turn, want)
	}
	if script := ownerOf(index, job.Dir(filepath.Join(index.Root(), "20261028-1812-a-script")), job.Spec{}); script.Person != "" {
		t.Errorf("a script's task was given a person: %+v", script)
	}
}
