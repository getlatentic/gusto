package container

import (
	"path/filepath"
	"testing"

	"github.com/getlatentic/gusto/internal/backend"
	"github.com/getlatentic/gusto/internal/computer"
	"github.com/getlatentic/gusto/internal/sandbox"
	"github.com/getlatentic/gusto/pkg/task"
)

// A turn's container is labelled with the organization, the person as the id their computer would have, and the agent; a task a
// script sent is nobody's, so it has no person.
func TestATurnsContainerIsOwnedByItsOrganizationPersonAndAgent(t *testing.T) {
	b := New(Config{Org: "acme-research-com"})
	spec := task.Spec{Session: "20261028-1811-opencode"}
	got := b.owner(backend.Turn{Spec: spec, Person: "bob@acme-research.com"})
	want := sandbox.Owner{Org: "acme-research-com", Person: computer.ID("acme-research-com", "bob@acme-research.com"), Instance: spec.Session}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if script := b.owner(backend.Turn{}); script.Person != "" {
		t.Errorf("a script's task was given a person: %+v", script)
	}
}

// A person's turns in containers share a home of theirs, kept beside their computer's record, so what they installed is there
// next time. A script's task is nobody's, and a room's turn sees only the room: neither is given a person's home.
func TestAPersonsTurnsShareAHomeOfTheirOwnAndARoomOrAScriptHasNone(t *testing.T) {
	homes := filepath.Join(t.TempDir(), "computers")
	b := New(Config{Org: "acme-research-com", Homes: homes})
	mounts, err := b.keptHome(backend.Turn{Spec: task.Spec{Session: "20261105-0900-opencode"}, Person: "bob@acme-research.com"})
	if err != nil {
		t.Fatal(err)
	}
	want := sandbox.Mount{From: filepath.Join(homes, computer.ID("acme-research-com", "bob@acme-research.com"), "home"), To: sandbox.Home, Kept: true}
	if len(mounts) != 1 || mounts[0] != want {
		t.Fatalf("got %+v, want %+v", mounts, want)
	}
	for name, turn := range map[string]backend.Turn{
		"a script's task": {},
		"a room's turn":   {Spec: task.Spec{Session: "20261105-0900-room", Agent: "opencode"}, Person: "bob@acme-research.com"},
	} {
		if mounts, err := b.keptHome(turn); err != nil || mounts != nil {
			t.Errorf("%s was given %+v (%v)", name, mounts, err)
		}
	}
	keepsNone := New(Config{Org: "acme-research-com"})
	if mounts, _ := keepsNone.keptHome(backend.Turn{Person: "bob@acme-research.com"}); mounts != nil {
		t.Errorf("a server that keeps no computers kept a home: %+v", mounts)
	}
}

// A conversation's turn has its workspace and its memory mounted, a room's turn only its memory, and a task of its own neither.
func TestATurnIsGivenTheConversationsWorkspaceAndMemory(t *testing.T) {
	conversation := staying(backend.Turn{Spec: task.Spec{Session: "s"}, Workspace: "/w", State: "/m"})
	if len(conversation) != 2 || conversation[0].To != "/job/workspace" || conversation[1].From != "/m" {
		t.Errorf("a conversation's turn was given %+v", conversation)
	}
	if room := staying(backend.Turn{Spec: task.Spec{Session: "s", Agent: "a"}, State: "/m"}); len(room) != 1 || room[0].To != "/root/.local/share" {
		t.Errorf("a room's turn was given %+v", room)
	}
	if alone := staying(backend.Turn{}); alone != nil {
		t.Errorf("a task of its own was given %+v", alone)
	}
}
