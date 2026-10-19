package cell

import (
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"sync"
	"time"

	"github.com/getlatentic/gusto/internal/cloud"
)

// Pool is the hosts the gusto cloud may place organizations on: the hosts that have registered,
// and this machine when it may run servers too.
type Pool struct {
	Dir    string
	Secret []byte
	// MaxAge is how long a host may be silent before it is not placed on or reached.
	MaxAge time.Duration
	// GoneAfter is how long a host may be silent before it is taken to be gone for good, when the
	// organizations placed on it are placed again. It is thirty minutes when not set.
	GoneAfter time.Duration
	// MinFreeMiB is the least memory a host needs to be given another organization.
	MinFreeMiB int
	// Local is this machine as a host, or nothing when it only runs the front door.
	Local cloud.Cell
	// Measure says what this machine has left, as an agent does for its own.
	Measure func() Capacity
	now     func() time.Time
}

func (p *Pool) clock() time.Time {
	if p.now != nil {
		return p.now()
	}
	return time.Now()
}

func (p *Pool) maxAge() time.Duration {
	if p.MaxAge > 0 {
		return p.MaxAge
	}
	return 30 * time.Second
}

// Get is the host with that name.
func (p *Pool) Get(name string) (cloud.Cell, error) {
	if p.Local != nil && name == p.Local.Name() {
		return p.Local, nil
	}
	registered, age, err := Registered(p.Dir, name, p.clock())
	gone := p.GoneAfter
	if gone <= 0 {
		gone = 30 * time.Minute
	}
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, fmt.Errorf("there is no host called %q: %w", name, cloud.ErrCellGone)
	case err != nil:
		return nil, err
	case age > gone:
		return nil, fmt.Errorf("the host %s has not been heard from for %s: %w", name, age.Round(time.Second), cloud.ErrCellGone)
	case age > p.maxAge():
		return nil, fmt.Errorf("the host %s has not been heard from for %s", name, age.Round(time.Second))
	}
	remote := newRemote(registered, p.Secret)
	remote.now = p.clock
	return remote, nil
}

type candidate struct {
	cell cloud.Cell
	free int
}

// Place is the host with the most memory to spare that has enough, and this machine counts among them
// when it runs servers. A host that does not answer is not a candidate.
func (p *Pool) Place() (cloud.Cell, error) {
	fresh, err := Fresh(p.Dir, p.clock(), p.maxAge())
	if err != nil {
		return nil, err
	}
	var (
		mu    sync.Mutex
		found []candidate
		wg    sync.WaitGroup
	)
	for _, registered := range fresh {
		wg.Add(1)
		go func() {
			defer wg.Done()
			remote := newRemote(registered, p.Secret)
			remote.now = p.clock
			if status, err := remote.Status(); err == nil {
				mu.Lock()
				found = append(found, candidate{remote, status.MemoryAvailableMiB})
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if p.Local != nil {
		measure := p.Measure
		if measure == nil {
			measure = Measure
		}
		found = append(found, candidate{p.Local, measure().MemoryAvailableMiB})
	}
	sort.Slice(found, func(a, b int) bool {
		if found[a].free != found[b].free {
			return found[a].free > found[b].free
		}
		return found[a].cell.Name() < found[b].cell.Name()
	})
	if len(found) == 0 || found[0].free < p.MinFreeMiB {
		return nil, fmt.Errorf("no host has %d MiB to give another organization: %w", p.MinFreeMiB, cloud.ErrNoCapacity)
	}
	return found[0].cell, nil
}
