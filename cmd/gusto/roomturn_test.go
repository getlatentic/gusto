package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/pkg/job"
)

// aRoomWithTwoAgents is a room whose folder holds the files given, with agents Ada and Bea.
func aRoomWithTwoAgents(t *testing.T, files map[string]string) (*roomRuns, instance.Instance) {
	t.Helper()
	instances, err := instance.Open(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	held, err := instances.StartRoom("Launch", "ada@acme.com")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Ada", "Bea"} {
		if _, err := instances.AddAgent(held.ID, instance.Agent{Name: name, Harness: "fixture"}); err != nil {
			t.Fatal(err)
		}
	}
	for name, text := range files {
		path := filepath.Join(instances.Workspace(held.ID), name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	held, _ = instances.Get(held.ID)
	return newRoomRuns(instances, nil), held
}

// turnOf is the task directory and spec of a turn of one of the room's agents.
func turnOf(t *testing.T, rooms *roomRuns, held instance.Instance, agent string) (job.Dir, job.Spec) {
	t.Helper()
	dir := job.Dir(filepath.Join(t.TempDir(), "turn"))
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	return dir, job.Spec{Session: held.ID, Thread: rooms.instances.Latest(held.ID).ID, Agent: agent, Harness: "fixture"}
}

func file(t *testing.T, folder, name string) string {
	t.Helper()
	text, err := os.ReadFile(filepath.Join(folder, name))
	if err != nil {
		return "(" + err.Error() + ")"
	}
	return string(text)
}

func sayings(rooms *roomRuns, held instance.Instance) []string {
	var said []string
	for _, note := range rooms.instances.Latest(held.ID).Notes {
		said = append(said, note.Text)
	}
	return said
}

func TestTwoAgentsOfARoomRunAtTheSameTimeEachOnACopyOfTheFolder(t *testing.T) {
	rooms, held := aRoomWithTwoAgents(t, map[string]string{"plan.md": "v1", "notes.md": "n1"})
	folder := rooms.instances.Workspace(held.ID)
	barrier := make(chan struct{})
	var inside sync.WaitGroup
	inside.Add(2)
	var all sync.WaitGroup
	for _, who := range []struct{ agent, file string }{{"a1", "plan.md"}, {"a2", "notes.md"}} {
		dir, spec := turnOf(t, rooms, held, who.agent)
		all.Add(1)
		go func() {
			defer all.Done()
			exit, err := rooms.around(t.Context(), spec, dir, func() (int, error) {
				inside.Done()
				<-barrier
				if strings.HasPrefix(file(t, dir.Workspace(), who.file), "(") {
					t.Error("no copy to work on")
				}
				return 0, os.WriteFile(filepath.Join(dir.Workspace(), who.file), []byte("changed by "+who.agent), 0o644)
			})
			if err != nil || exit != 0 {
				t.Errorf("%s: %d %v", who.agent, exit, err)
			}
		}()
	}
	inside.Wait()
	if file(t, folder, "plan.md") != "v1" {
		t.Error("a turn wrote straight into the room's folder")
	}
	close(barrier)
	all.Wait()
	if file(t, folder, "plan.md") != "changed by a1" || file(t, folder, "notes.md") != "changed by a2" {
		t.Errorf("the folder holds %q and %q", file(t, folder, "plan.md"), file(t, folder, "notes.md"))
	}
	if said := sayings(rooms, held); len(said) != 0 {
		t.Errorf("the room was told of a conflict that was not one: %v", said)
	}
}

func TestTwoAgentsThatChangedOneFileLeaveBothVersionsAndANoteInTheRoom(t *testing.T) {
	rooms, held := aRoomWithTwoAgents(t, map[string]string{"report.md": "draft"})
	folder := rooms.instances.Workspace(held.ID)
	var inside, all sync.WaitGroup
	inside.Add(2)
	proceed := make(chan struct{})
	for _, agent := range []string{"a1", "a2"} {
		dir, spec := turnOf(t, rooms, held, agent)
		all.Add(1)
		go func() {
			defer all.Done()
			if _, err := rooms.around(t.Context(), spec, dir, func() (int, error) {
				inside.Done()
				<-proceed
				return 0, os.WriteFile(filepath.Join(dir.Workspace(), "report.md"), []byte("by "+agent), 0o644)
			}); err != nil {
				t.Error(err)
			}
		}()
	}
	inside.Wait()
	close(proceed)
	all.Wait()

	first, second := file(t, folder, "report.md"), ""
	for _, side := range []string{"report.ada.md", "report.bea.md"} {
		if text := file(t, folder, side); !strings.HasPrefix(text, "(") {
			second = text
		}
	}
	if first == "" || second == "" || first == second {
		t.Fatalf("the folder holds %q and %q, not both versions", first, second)
	}
	said := sayings(rooms, held)
	if len(said) != 1 || !strings.Contains(said[0], "both changed report.md") {
		t.Fatalf("the room was told %q", said)
	}
	if notes := rooms.instances.Latest(held.ID).Notes; !notes[0].Notice {
		t.Error("the note reads as something a person said")
	}
}

// The demo's case: Scout and Draft both write summary.md, in different sections, and the room keeps one file with both.
func TestTwoAgentsThatChangedDifferentLinesOfOneFileLeaveOneFileWithBoth(t *testing.T) {
	const summary = "# Summary\n\n## Market\ntbd\n\n## Plan\ntbd\n"
	rooms, held := aRoomWithTwoAgents(t, map[string]string{"summary.md": summary})
	folder := rooms.instances.Workspace(held.ID)
	var inside, all sync.WaitGroup
	inside.Add(2)
	proceed := make(chan struct{})
	for _, each := range []struct{ agent, section string }{{"a1", "## Market\ntbd"}, {"a2", "## Plan\ntbd"}} {
		dir, spec := turnOf(t, rooms, held, each.agent)
		all.Add(1)
		go func() {
			defer all.Done()
			if _, err := rooms.around(t.Context(), spec, dir, func() (int, error) {
				inside.Done()
				<-proceed
				written := strings.Replace(summary, each.section, strings.TrimSuffix(each.section, "tbd")+"by "+each.agent, 1)
				return 0, os.WriteFile(filepath.Join(dir.Workspace(), "summary.md"), []byte(written), 0o644)
			}); err != nil {
				t.Error(err)
			}
		}()
	}
	inside.Wait()
	close(proceed)
	all.Wait()

	if got, want := file(t, folder, "summary.md"), "# Summary\n\n## Market\nby a1\n\n## Plan\nby a2\n"; got != want {
		t.Fatalf("summary.md holds\n%s", got)
	}
	for _, side := range []string{"summary.ada.md", "summary.bea.md"} {
		if text := file(t, folder, side); !strings.HasPrefix(text, "(") {
			t.Errorf("%s was saved beside: %q", side, text)
		}
	}
	if said := sayings(rooms, held); len(said) != 1 || !strings.Contains(said[0], "in different places, and it holds both changes") {
		t.Fatalf("the room was told %q", said)
	}
}

func TestATurnThatFailedOrWasStoppedChangesNothingInTheRoomsFolder(t *testing.T) {
	rooms, held := aRoomWithTwoAgents(t, map[string]string{"a.md": "v1"})
	folder := rooms.instances.Workspace(held.ID)
	touch := func(dir job.Dir) {
		os.WriteFile(filepath.Join(dir.Workspace(), "a.md"), []byte("half"), 0o644)
		os.WriteFile(filepath.Join(dir.Workspace(), "b.md"), []byte("new"), 0o644)
	}
	failing, spec := turnOf(t, rooms, held, "a1")
	if exit, _ := rooms.around(t.Context(), spec, failing, func() (int, error) { touch(failing); return 1, nil }); exit != 1 {
		t.Errorf("exit %d", exit)
	}
	broken, spec := turnOf(t, rooms, held, "a1")
	if _, err := rooms.around(t.Context(), spec, broken, func() (int, error) { touch(broken); return 0, errors.New("the agent died") }); err == nil {
		t.Error("the error was lost")
	}
	stopped, spec := turnOf(t, rooms, held, "a1")
	ctx, stop := context.WithCancel(t.Context())
	if _, err := rooms.around(ctx, spec, stopped, func() (int, error) { touch(stopped); stop(); return 0, nil }); err != nil {
		t.Errorf("a stopped turn is not an error of the merge: %v", err)
	}
	if file(t, folder, "a.md") != "v1" || !strings.HasPrefix(file(t, folder, "b.md"), "(") {
		t.Errorf("a turn that did not pass reached the folder: %q %q", file(t, folder, "a.md"), file(t, folder, "b.md"))
	}
	if entries, _ := os.ReadDir(failing.Workspace()); len(entries) != 0 {
		t.Error("the turn's copy was kept")
	}
}

func TestAnAgentsMemoryIsItsOwnAndStartsFromTheRoomsState(t *testing.T) {
	rooms, held := aRoomWithTwoAgents(t, nil)
	shared := rooms.instances.State(held.ID)
	os.MkdirAll(filepath.Join(shared, "claude"), 0o755)
	os.WriteFile(filepath.Join(shared, "claude", "session.json"), []byte("earlier"), 0o644)
	dir, spec := turnOf(t, rooms, held, "a1")
	rooms.around(t.Context(), spec, dir, func() (int, error) {
		own := stateDir(rooms.instances, spec)
		if file(t, own, "claude/session.json") != "earlier" {
			t.Errorf("the agent's own state was not seeded: %q", file(t, own, "claude/session.json"))
		}
		return 0, os.WriteFile(filepath.Join(own, "claude", "session.json"), []byte("later"), 0o644)
	})
	if file(t, shared, "claude/session.json") != "earlier" {
		t.Error("the agent wrote to the room's shared state")
	}
	other, spec2 := turnOf(t, rooms, held, "a2")
	rooms.around(t.Context(), spec2, other, func() (int, error) {
		if got := file(t, stateDir(rooms.instances, spec2), "claude/session.json"); got != "earlier" {
			t.Errorf("another agent saw %q", got)
		}
		return 0, nil
	})
}

func TestAnythingButARoomsTurnIsRunAsItIs(t *testing.T) {
	rooms, held := aRoomWithTwoAgents(t, nil)
	ran := false
	exit, err := rooms.around(t.Context(), job.Spec{Session: held.ID}, job.Dir(t.TempDir()), func() (int, error) { ran = true; return 7, nil })
	if !ran || exit != 7 || err != nil {
		t.Errorf("%v %d %v", ran, exit, err)
	}
	if got := stateDir(rooms.instances, job.Spec{Session: held.ID}); got != rooms.instances.State(held.ID) {
		t.Errorf("a conversation's memory is %s", got)
	}
}

func TestATurnWhoseAskerLeftOrWhoseAgentWasTakenOutMidTurnWritesNothing(t *testing.T) {
	for _, each := range []struct {
		name  string
		leave func(rooms *roomRuns, held instance.Instance) error
		said  string
	}{
		{"asker left", func(rooms *roomRuns, held instance.Instance) error {
			return rooms.instances.RemoveMember(held.ID, "bea@acme.com")
		}, "bea@acme.com left the room while Ada worked for them"},
		{"agent taken out", func(rooms *roomRuns, held instance.Instance) error {
			return rooms.instances.RemoveAgent(held.ID, "a1")
		}, "was taken out of the room while it worked"},
	} {
		t.Run(each.name, func(t *testing.T) {
			rooms, held := aRoomWithTwoAgents(t, map[string]string{"plan.md": "v1"})
			if err := rooms.instances.SetMember(held.ID, "bea@acme.com", instance.RoleMember); err != nil {
				t.Fatal(err)
			}
			rooms.askedBy = func(job.Dir) string { return "bea@acme.com" }
			dir, spec := turnOf(t, rooms, held, "a1")
			exit, err := rooms.around(t.Context(), spec, dir, func() (int, error) {
				if err := os.WriteFile(filepath.Join(dir.Workspace(), "plan.md"), []byte("v2"), 0o644); err != nil {
					return 1, err
				}
				return 0, each.leave(rooms, held)
			})
			if exit != 0 || err != nil {
				t.Fatalf("exit %d, %v", exit, err)
			}
			if got := file(t, rooms.instances.Workspace(held.ID), "plan.md"); got != "v1" {
				t.Errorf("the room's file became %q", got)
			}
			if said := sayings(rooms, held); len(said) != 1 || !strings.Contains(said[0], each.said) {
				t.Errorf("the room was told %q", said)
			}
		})
	}
}
