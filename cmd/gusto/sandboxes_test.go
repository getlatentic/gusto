package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/getlatentic/gusto/internal/backend"
	"github.com/getlatentic/gusto/internal/backend/container"
	"github.com/getlatentic/gusto/internal/backend/process"
	"github.com/getlatentic/gusto/internal/backend/vm"
	"github.com/getlatentic/gusto/internal/catalog"
	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/signing"
	"github.com/getlatentic/gusto/internal/tasks"
	"github.com/getlatentic/gusto/pkg/task"
)

// Each value of GUSTO_SANDBOX makes the backend it names, and anything else is a container.
func TestEachSandboxSettingMakesItsBackend(t *testing.T) {
	t.Parallel()
	for setting, want := range map[string]string{"here": "process", "process": "process", "openshell": "vm", "container": "container", "": "container"} {
		set := map[string]string{"GUSTO_OPENSHELL_IMAGE": "/images/runner.tar.gz", "GUSTO_SANDBOX": setting}
		cfg, _, err := loadServe(func(name string) string { return set[name] })
		if err != nil {
			t.Fatal(err)
		}
		boxes, err := openBackend(cfg, signing.Identity{}, nil, "8041")
		if err != nil {
			t.Fatalf("%q: %v", setting, err)
		}
		var got string
		switch boxes.(type) {
		case *process.Backend:
			got = "process"
		case *vm.Backend:
			got = "vm"
		case *container.Backend:
			got = "container"
		}
		if got != want {
			t.Errorf("GUSTO_SANDBOX=%q made %T, want %s", setting, boxes, want)
		}
	}
}

// recording is a backend that runs nothing: it keeps the turn it was given, and writes the record a turn leaves.
type recording struct {
	turn    backend.Turn
	session string
}

func (r *recording) String() string                        { return "nowhere" }
func (r *recording) Check() error                          { return nil }
func (r *recording) Reach() backend.Reach                  { return backend.Loopback }
func (r *recording) Start(context.Context) (func(), error) { return func() {}, nil }
func (r *recording) Offered() []catalog.Entry              { return nil }

func (r *recording) Run(_ context.Context, turn backend.Turn) (int, error) {
	r.turn = turn
	return 0, evidence.Write(turn.Dir, evidence.Manifest{Session: r.session})
}

// A turn in a conversation is handed to its backend with who pays, the instance's folder and the agent's memory, and the
// backend's name for the conversation is kept for the next turn, whichever backend ran it.
func TestATurnIsHandedItsConversationAndWhatTheAgentCallsItIsKept(t *testing.T) {
	index, err := tasks.Open(filepath.Join(t.TempDir(), "jobs"))
	if err != nil {
		t.Fatal(err)
	}
	instances, err := instance.Open(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	held, err := instances.Start("20261106-1200-notes", instance.Instance{Title: "Notes", Harness: "fixture", Who: "ada@acme.com"})
	if err != nil {
		t.Fatal(err)
	}
	thread := instances.Latest(held.ID).ID
	record := tasks.Record{ID: "20261106-1200-a-turn", State: tasks.Running, Asked: "Ada@Acme.com"}
	dir := record.Dir(index.Root())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	if err := (task.Spec{Brief: "b", Harness: "fixture", Session: held.ID, Thread: thread}).Write(dir); err != nil {
		t.Fatal(err)
	}
	if err := index.Add(record); err != nil {
		t.Fatal(err)
	}
	way, err := openWayOut(index.Root(), fromEnv(t).way, fromEnv(t).core.local)
	if err != nil {
		t.Fatal(err)
	}
	boxes := &recording{session: "the-agents-own-name"}
	if _, err := turns(boxes, way, instances, index, newRoomRuns(instances, nil, fromEnv(t).boxes.roomCopy(), nil))(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	got := boxes.turn
	if got.Person != "ada@acme.com" || got.AgentFolder != instances.AgentFolder(held.ID) || got.State != instances.State(held.ID) {
		t.Errorf("the turn was handed %+v", got)
	}
	if got.Exit.Direct == "" && got.Exit.Door == nil {
		t.Error("the turn was given no way out")
	}
	if remembered := instances.Latest(held.ID).Remembered; remembered != "the-agents-own-name" {
		t.Errorf("the next turn would be handed %q", remembered)
	}
}
