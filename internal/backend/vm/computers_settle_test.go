package vm

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/backend"
	"github.com/getlatentic/gusto/internal/computer"
	"github.com/getlatentic/gusto/internal/openshell"
	"github.com/getlatentic/gusto/internal/openshell/openshelltest"
	"github.com/getlatentic/gusto/pkg/task"
)

// quietDoor is a proxy that gives every turn a token and keeps nothing of it.
type quietDoor struct{}

func (quietDoor) Token(task.Dir, task.Spec) (string, error) { return "token", nil }
func (quietDoor) SandboxToken(task.Dir, task.Spec) (string, bool, error) {
	return "token", false, nil
}
func (quietDoor) RecordRun(task.Dir, task.Spec, string, bool) error { return nil }

// A turn the sandbox refuses before it starts, here for tool servers it does not carry, leaves the computer it would have made
// failed and saying why, and not being made for good.
func TestATurnRefusedBeforeItsSandboxStartsDoesNotLeaveTheComputerBeingMade(t *testing.T) {
	openshelltest.Install(t)
	harnesses := t.TempDir()
	if err := os.WriteFile(filepath.Join(harnesses, "fixture.json"), []byte(`{"name":"fixture","program":"sh","argv":["-c","true"],"needs":"nothing","stream":"plain"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GUSTO_HARNESSES", harnesses)
	dir := task.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	spec := task.Spec{Brief: "list", Repo: "unused", Harness: "fixture", Setup: &task.Setup{Servers: []string{"notes"}}}
	if err := spec.Write(dir); err != nil {
		t.Fatal(err)
	}
	registry, err := computer.Open(filepath.Join(t.TempDir(), "computers"), "acme-com")
	if err != nil {
		t.Fatal(err)
	}
	b := New(Config{Gateway: openshell.Config{Binary: "openshell", Image: "runner.tar", Host: "host.openshell.internal"}, Computers: registry, Org: "acme-com", Node: "local"})
	_, err = b.inComputer(context.Background(), backend.Turn{Dir: dir, Spec: spec, Person: "ada@acme.com", Exit: backend.Exit{Door: quietDoor{}, Port: "8041"}})
	if err == nil || !strings.Contains(err.Error(), "tool servers") {
		t.Fatalf("the turn was not refused for its tool servers: %v", err)
	}
	got, err := registry.Of("ada@acme.com", "")
	if err != nil || got.State != computer.Failed || !strings.Contains(got.Note, "tool servers") {
		t.Errorf("the computer is %s %q (%v), not failed with the reason", got.State, got.Note, err)
	}
}

// A computer left being made by a server that ended is said not to have been made, once a making could not still take, and not
// while a task is in it, nor before the making's time is up.
func TestAComputerLeftBeingMadeByAServerThatEndedIsSaidToHaveFailed(t *testing.T) {
	registry, err := computer.Open(filepath.Join(t.TempDir(), "computers"), "acme-com")
	if err != nil {
		t.Fatal(err)
	}
	made := func(person string, age time.Duration) computer.Computer {
		t.Helper()
		held, _, err := registry.Ensure(person, "", "local")
		if err != nil {
			t.Fatal(err)
		}
		held, err = registry.Change(held.ID, func(each *computer.Computer) { each.Created = time.Now().Add(-age) })
		if err != nil {
			t.Fatal(err)
		}
		return held
	}
	stale, working, recent := made("ada@acme.com", 2*computer.MakingWithin), made("bob@acme.com", 2*computer.MakingWithin), made("cy@acme.com", time.Minute)
	use := newComputerUse()
	use.enter(working.ID)
	rest(openshell.Config{Binary: "openshell"}, registry, use, func(string, string) bool { return false }, time.Hour)

	state := func(held computer.Computer) computer.Computer {
		got, err := registry.Of(held.User, "")
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	if got := state(stale); got.State != computer.Failed || !strings.Contains(got.Note, "did not finish") {
		t.Errorf("a computer left being made is %s %q", got.State, got.Note)
	}
	if got := state(working).State; got != computer.Creating {
		t.Errorf("a computer a task is in was marked %s", got)
	}
	if got := state(recent).State; got != computer.Creating {
		t.Errorf("a computer being made a minute ago was marked %s", got)
	}
}

// A computer is stopped for being idle only if it is still idle when nothing is in it: a task that ran and ended after the record
// was read has used it a moment ago.
func TestAComputerUsedSinceTheIdleLookIsNotStopped(t *testing.T) {
	root := openshelltest.Install(t)
	registry := running(t, root, map[string]time.Time{"ada@acme.com": time.Now().Add(-2 * time.Hour)})
	ada, _ := registry.Of("ada@acme.com", "")
	touched := func(string, string) bool {
		registry.Change(ada.ID, func(each *computer.Computer) { each.LastActive = time.Now() })
		return false
	}
	rest(openshell.Config{Binary: "openshell"}, registry, newComputerUse(), touched, time.Hour)
	if _, err := os.Stat(filepath.Join(root, ada.Sandbox, "stopped")); err == nil {
		t.Error("a computer a task used after the look was stopped")
	}
	if got, _ := registry.Of("ada@acme.com", ""); got.State != computer.Running {
		t.Errorf("the computer is %s", got.State)
	}
}
