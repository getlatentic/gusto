package vm

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/computer"
	"github.com/getlatentic/gusto/internal/openshell"
	"github.com/getlatentic/gusto/internal/openshell/openshelltest"
)

// running is a registry of an organization whose people each have a running computer on the stand-in gateway, last used when
// each says.
func running(t *testing.T, root string, lastUsed map[string]time.Time) *computer.Registry {
	t.Helper()
	registry, err := computer.Open(filepath.Join(t.TempDir(), "computers"), "acme-com")
	if err != nil {
		t.Fatal(err)
	}
	for person, when := range lastUsed {
		held, _, err := registry.Ensure(person, "local")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, held.Sandbox, "sandbox"), 0o755); err != nil {
			t.Fatal(err)
		}
		registry.Change(held.ID, func(each *computer.Computer) { each.State, each.LastActive = computer.Running, when })
	}
	return registry
}

func TestTheJanitorRemovesRetiredComputersAndStopsIdleOnesThatNoOneIsUsing(t *testing.T) {
	root := openshelltest.Install(t)
	cfg := openshell.Config{Binary: "openshell"}
	old, recent := time.Now().Add(-2*time.Hour), time.Now()
	registry := running(t, root, map[string]time.Time{"ada@acme.com": recent, "bob@acme.com": old, "cy@acme.com": old, "dee@acme.com": recent})
	ada, _ := registry.Of("ada@acme.com")
	bob, _ := registry.Of("bob@acme.com")
	cy, _ := registry.Of("cy@acme.com")
	dee, _ := registry.Of("dee@acme.com")

	// Ada was removed from the organization, by the front door, which only marks her computer. Cy has a task running.
	if err := registry.Retire("ada@acme.com"); err != nil {
		t.Fatal(err)
	}
	retire(cfg, registry)
	use := newComputerUse()
	use.enter(cy.ID)
	rest(cfg, registry, use, func(string) bool { return false }, time.Hour)

	deleted, _ := os.ReadFile(filepath.Join(root, "deleted.log"))
	if !strings.Contains(string(deleted), ada.Sandbox) {
		t.Errorf("a retired computer is removed from the gateway: %q", deleted)
	}
	if _, err := registry.Of("ada@acme.com"); err == nil {
		t.Error("and from the record")
	}
	if _, err := os.Stat(filepath.Join(root, bob.Sandbox, "stopped")); err != nil {
		t.Error("an idle computer nobody is using is stopped")
	}
	if got, _ := registry.Of("bob@acme.com"); got.State != computer.Stopped {
		t.Errorf("%+v", got)
	}
	if _, err := os.Stat(filepath.Join(root, cy.Sandbox, "stopped")); err == nil {
		t.Error("a computer with a job running is left running")
	}
	if got, _ := registry.Of("cy@acme.com"); got.State != computer.Running {
		t.Errorf("%+v", got)
	}
	if _, err := os.Stat(filepath.Join(root, dee.Sandbox, "stopped")); err == nil {
		t.Error("a computer that was used a moment ago is not idle")
	}
}

// A person removed while a task of theirs runs has their computer removed; the task ending, or a late start, does not mark it
// running again, so it is not counted against the organization or handed out.
func TestAComputerRemovedWhileATaskUsesItStaysRemoved(t *testing.T) {
	root := openshelltest.Install(t)
	registry := running(t, root, map[string]time.Time{"ada@acme.com": time.Now()})
	ada, _ := registry.Of("ada@acme.com")
	if err := registry.Retire("ada@acme.com"); err != nil {
		t.Fatal(err)
	}
	retire(openshell.Config{Binary: "openshell"}, registry)
	box := &computerBox{registry: registry, id: ada.ID}
	box.update(func(each *computer.Computer) { each.State = computer.Running })
	box.change(computer.Running, "")
	if _, err := registry.Of("ada@acme.com"); err == nil {
		t.Error("a removed computer is live again")
	}
	if all, _ := registry.All(); len(all) != 0 {
		t.Errorf("the organization counts %+v", all)
	}
}

// The idle stop leaves a computer a task is in, whoever asked the task, and a task that starts while a stop is under way
// waits for it, so it is not stopped once it runs. spec/Computer.tla is the model.
func TestTheIdleStopLeavesAComputerATaskIsIn(t *testing.T) {
	use := newComputerUse()
	use.enter("c1")
	if use.stopIdle("c1", func() { t.Error("a computer a task is in was stopped") }) {
		t.Error("the stop said it stopped a busy computer")
	}
	use.leave("c1")
	stopping, release := make(chan struct{}), make(chan struct{})
	stopped := make(chan bool, 1)
	go func() { stopped <- use.stopIdle("c1", func() { close(stopping); <-release }) }()
	<-stopping
	entered := make(chan struct{})
	go func() { use.enter("c1"); close(entered) }()
	select {
	case <-entered:
		t.Fatal("a task was counted in a computer while it was being stopped")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	<-entered
	if !<-stopped {
		t.Error("an idle computer was not stopped")
	}
	if use.stopIdle("c1", func() { t.Error("the task that started after the stop was stopped") }) {
		t.Error("the stop said it stopped a busy computer")
	}
}

// A computer that never started is not left being made: it says why it failed, and does not count as being made for good. One
// that had started keeps its state when a later task fails before starting.
func TestAComputerThatNeverStartedIsNotLeftBeingMade(t *testing.T) {
	registry, err := computer.Open(t.TempDir(), "acme")
	if err != nil {
		t.Fatal(err)
	}
	held, _, err := registry.Ensure("ada@acme.com", "local")
	if err != nil {
		t.Fatal(err)
	}
	failedBeforeStart(registry, held.ID, errors.New("no token for the sandbox"))
	if got, _ := registry.Of("ada@acme.com"); got.State != computer.Failed || got.Note != "no token for the sandbox" {
		t.Errorf("a computer that never started: %s %q", got.State, got.Note)
	}
	registry.Change(held.ID, func(each *computer.Computer) { each.State = computer.Running })
	failedBeforeStart(registry, held.ID, errors.New("a later task failed"))
	if got, _ := registry.Of("ada@acme.com"); got.State != computer.Running {
		t.Errorf("a running computer was marked %s by a task that failed", got.State)
	}
}
