package cloud

import (
	"testing"
	"time"
)

// A demo whose organization's server does not remove its computers in time is not ended: its server, its directory and the
// records of its computers stay, so they can be named, and it is ended by the sweep after they are removed
// (getlatentic/planning#831).
func TestADemoWhoseComputersAreNotRemovedIsNotEndedUntilTheyAre(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := demoFront(t)
		startDemoAs(t, f, "Ada", "ada@acme.com", "Acme")
		f.server.Wait()
		slug := demoSlugOf(t, f)
		registry, err := f.store.Computers(slug)
		if err != nil {
			t.Fatal(err)
		}
		made, _, err := registry.Ensure("ada@acme.com", "", "local")
		if err != nil {
			t.Fatal(err)
		}
		f.store.now = func() time.Time { return time.Now().Add(time.Hour) }

		f.server.EndDueDemos(t.Context())
		f.server.ender.running.Wait()
		if _, found, _ := f.store.Org(t.Context(), slug); !found {
			t.Fatal("a demo with a computer still there was deleted")
		}
		if left, _ := registry.Retiring(); len(left) != 1 || left[0].ID != made.ID {
			t.Fatalf("the computer is not named for removal: %+v", left)
		}
		if !f.server.Controller.Node.Alive(slug) {
			t.Error("the server that must remove the computer was stopped")
		}

		if err := registry.Retired(made.ID); err != nil {
			t.Fatal(err)
		}
		f.server.EndDueDemos(t.Context())
		f.server.ender.running.Wait()
		if _, found, _ := f.store.Org(t.Context(), slug); found {
			t.Error("a demo with no computer left was not ended")
		}
	})
}
