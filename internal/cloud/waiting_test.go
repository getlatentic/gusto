package cloud

import (
	"sync/atomic"
	"testing"
	"time"
)

// lateCells has no room until it is opened, as a front door does while a cell starts.
type lateCells struct {
	open  atomic.Bool
	local *LocalCell
}

func (l *lateCells) Get(string) (Cell, error) { return l.local, nil }
func (l *lateCells) Place() (Cell, error) {
	if !l.open.Load() {
		return nil, ErrNoCapacity
	}
	return l.local, nil
}

func TestAnOrganisationWithNoCellToGoOnWaitsAndIsStartedWhenOneHasRoom(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		c := controlled(t, s)
		cells := &lateCells{local: &LocalCell{}}
		c.Cells, c.WaitEvery = cells, 10*time.Millisecond
		org := made(t, s, "ada@acme.com", "Acme")

		if err := c.Provision(org.Slug); err == nil {
			t.Fatal("an organisation with nowhere to go was provisioned")
		}
		if address, state := c.Route(org.Slug); state != Waiting || address != "" || c.Waiting() != 1 {
			t.Fatalf("it waits, which is not failing: %q %q %d", address, state, c.Waiting())
		}
		if runtime, _ := s.Runtime(org.Slug); runtime.State != Waiting || runtime.Restarts != 0 || runtime.Cell != "" {
			t.Errorf("a wait is not counted as a restart: %+v", runtime)
		}

		time.Sleep(30 * time.Millisecond)
		c.supervise()
		time.Sleep(50 * time.Millisecond)
		if _, state := c.Route(org.Slug); state != Waiting {
			t.Errorf("it is tried again and there is still no room: %q", state)
		}

		cells.open.Store(true)
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			c.supervise()
			if _, state := c.Route(org.Slug); state == Ready {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		if address, state := c.Route(org.Slug); state != Ready || address == "" {
			t.Fatalf("it did not start once there was room: %q %q", address, state)
		}
		if c.Waiting() != 0 {
			t.Errorf("%d still wait", c.Waiting())
		}
		if runtime, _ := s.Runtime(org.Slug); runtime.Restarts != 0 {
			t.Errorf("%+v", runtime)
		}
	})
}
