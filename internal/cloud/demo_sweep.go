package cloud

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// demoEnder ends the demos whose time is up, each once, and not while it is being ended.
type demoEnder struct {
	mu      sync.Mutex
	ending  map[string]bool
	running sync.WaitGroup
}

// ComputersWait is how long an ending demo waits for its organisation's server to remove its computers, which it
// does when it next looks, before the server is stopped anyway.
const ComputersWait = 90 * time.Second

// RunDemos ends demos as their time is up, every so often, until the context ends.
func (s *Server) RunDemos(ctx context.Context, every time.Duration) {
	if !s.Store.set.Demo.Enabled() {
		return
	}
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		s.EndDueDemos()
		select {
		case <-ctx.Done():
			s.ender.running.Wait()
			return
		case <-tick.C:
		}
	}
}

// EndDueDemos starts to end each demo that is due. It does not wait for them.
func (s *Server) EndDueDemos() {
	due, err := s.Store.DemosDue()
	if err != nil {
		slog.Error("the demos that are due could not be listed", "error", err)
		return
	}
	for slug, reason := range due {
		s.ender.mu.Lock()
		if s.ender.ending == nil {
			s.ender.ending = map[string]bool{}
		}
		already := s.ender.ending[slug]
		s.ender.ending[slug] = true
		s.ender.mu.Unlock()
		if already {
			continue
		}
		s.ender.running.Add(1)
		go func() {
			defer s.ender.running.Done()
			s.endDemo(slug, reason)
		}()
	}
}

// endDemo removes a demo's workspaces, then its server, then the organisation. A demo that cannot be ended is tried
// again at the next sweep, which it is not until this has finished.
func (s *Server) endDemo(slug, reason string) {
	defer func() {
		s.ender.mu.Lock()
		delete(s.ender.ending, slug)
		s.ender.mu.Unlock()
	}()
	if s.Controller != nil {
		s.removeComputers(slug)
		s.Controller.Remove(slug)
	}
	if err := s.Store.EndDemo(slug, reason); err != nil {
		slog.Error("a demo could not be ended", "org", slug, "error", err)
		return
	}
	slog.Info("a demo ended", "org", slug, "reason", reason)
	s.tell("ended", slug)
}

// removeComputers asks for the demo's computers to be removed and waits until they are, for as long as it may.
func (s *Server) removeComputers(slug string) {
	registry, err := s.Store.Computers(slug)
	if err != nil {
		slog.Warn("a demo's workspaces could not be read", "org", slug, "error", err)
		return
	}
	if err := registry.RetireAll(); err != nil {
		slog.Warn("a demo's workspaces could not be marked", "org", slug, "error", err)
		return
	}
	wait := s.computersWait()
	for deadline := time.Now().Add(wait); time.Now().Before(deadline); time.Sleep(time.Second) {
		if left, err := registry.Retiring(); err == nil && len(left) == 0 {
			return
		}
	}
	slog.Warn("a demo's workspaces were not removed in time, so its server is stopped with them still there", "org", slug)
}

func (s *Server) computersWait() time.Duration {
	if s.ComputersWait > 0 {
		return s.ComputersWait
	}
	return ComputersWait
}
