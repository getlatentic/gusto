package node

import (
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"sync"
	"time"

	"github.com/getlatentic/gusto/internal/cloud"
)

// Pool is the nodes the gusto cloud may place organizations on: the nodes that have registered,
// and this machine when it may run servers too.
type Pool struct {
	Dir    string
	Secret []byte
	// MaxAge is how long a node may be silent before it is not placed on or reached.
	MaxAge time.Duration
	// GoneAfter is how long a node may be silent before it is taken to be gone for good, when the
	// organizations placed on it are placed again. It is thirty minutes when not set.
	GoneAfter time.Duration
	// MinFreeMiB is the least memory a node needs to be given another organization.
	MinFreeMiB int
	// Local is this machine as a node, or nothing when it only runs the front door.
	Local cloud.Node
	// Measure says what this machine has left, as an agent does for its own.
	Measure func() Capacity
	now     func() time.Time

	// guard guards retiring, which the scaler sets while the controller places.
	guard sync.Mutex
	// retiring is the nodes the scaler is stopping, by name: none is placed on or started on.
	retiring map[string]bool
}

// setRetiring says the scaler is stopping a node, or is not after all.
func (p *Pool) setRetiring(name string, retiring bool) {
	p.guard.Lock()
	defer p.guard.Unlock()
	if p.retiring == nil {
		p.retiring = map[string]bool{}
	}
	if retiring {
		p.retiring[name] = true
		return
	}
	delete(p.retiring, name)
}

// forgetRetiring lets go of the marks of the nodes that gone says are gone.
func (p *Pool) forgetRetiring(gone func(name string) bool) {
	p.guard.Lock()
	marked := make([]string, 0, len(p.retiring))
	for name := range p.retiring {
		marked = append(marked, name)
	}
	p.guard.Unlock()
	for _, name := range marked {
		if gone(name) {
			p.setRetiring(name, false)
		}
	}
}

func (p *Pool) isRetiring(name string) bool {
	p.guard.Lock()
	defer p.guard.Unlock()
	return p.retiring[name]
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

// Get is the node with that name.
func (p *Pool) Get(name string) (cloud.Node, error) {
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
		return nil, fmt.Errorf("there is no host called %q: %w", name, cloud.ErrNodeGone)
	case err != nil:
		return nil, err
	case p.isRetiring(name):
		return nil, fmt.Errorf("the host %s is being stopped", name)
	case age > gone:
		return nil, fmt.Errorf("the host %s has not been heard from for %s: %w", name, age.Round(time.Second), cloud.ErrNodeGone)
	case age > p.maxAge():
		return nil, fmt.Errorf("the host %s has not been heard from for %s", name, age.Round(time.Second))
	}
	remote := newRemote(registered, p.Secret)
	remote.now = p.clock
	return remote, nil
}

type candidate struct {
	node cloud.Node
	free int
}

// Place is the node with the most memory to spare that has enough, and this machine counts among them
// when it runs servers. A node that does not answer, or that is being stopped, is not a candidate.
func (p *Pool) Place() (cloud.Node, error) {
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
		if p.isRetiring(registered.Name) {
			continue
		}
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
		return found[a].node.Name() < found[b].node.Name()
	})
	if len(found) == 0 || found[0].free < p.MinFreeMiB {
		return nil, fmt.Errorf("no host has %d MiB to give another organization: %w", p.MinFreeMiB, cloud.ErrNoCapacity)
	}
	return found[0].node, nil
}
