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

// ComputersWait is how long an ending demo waits for its organization's server to remove its computers, which it
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
		s.EndDueDemos(ctx)
		select {
		case <-ctx.Done():
			s.ender.running.Wait()
			return
		case <-tick.C:
		}
	}
}

// EndDueDemos starts to end each demo that is due. It does not wait for them, and the asker going away does not stop them.
func (s *Server) EndDueDemos(ctx context.Context) {
	ctx = context.WithoutCancel(ctx)
	due, err := s.Store.DemosDue(ctx)
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
			s.endDemo(ctx, slug, reason)
		}()
	}
}

// endDemo removes a demo's computers, then its server, then the organization. A demo whose computers are not all removed
// keeps its server, its directory and the records of its computers, and is tried again at the next sweep, which it is
// not until this has finished: only the organization's server reaches the gateway, so nothing else can name them.
func (s *Server) endDemo(ctx context.Context, slug, reason string) {
	defer func() {
		s.ender.mu.Lock()
		delete(s.ender.ending, slug)
		s.ender.mu.Unlock()
	}()
	if err := s.Store.MarkEnding(ctx, slug); err != nil {
		slog.Error("a demo could not be marked as ending, and is tried again", "org", slug, "error", err)
		return
	}
	if s.Controller != nil {
		if !s.removeComputers(slug) {
			s.Metrics.endDeferred()
			return
		}
		s.Controller.Remove(ctx, slug)
	}
	if err := s.Store.EndDemo(ctx, slug, reason); err != nil {
		slog.Error("a demo could not be ended", "org", slug, "error", err)
		return
	}
	if s.Controller != nil {
		s.Controller.Ended(slug)
	}
	slog.Info("a demo ended", "org", slug, "reason", reason)
	s.tell(ctx, "ended", slug)
}

// removeComputers asks for the demo's computers to be removed and waits until they are, for as long as it may. It says
// whether none is left.
func (s *Server) removeComputers(slug string) bool {
	registry, err := s.Store.Computers(slug)
	if err != nil {
		slog.Warn("a demo's computers could not be read, so it is not ended yet", "org", slug, "error", err)
		return false
	}
	if err := registry.RetireAll(); err != nil {
		slog.Warn("a demo's computers could not be marked, so it is not ended yet", "org", slug, "error", err)
		return false
	}
	wait := s.computersWait()
	for deadline := time.Now().Add(wait); ; time.Sleep(time.Second) {
		left, err := registry.Retiring()
		if err == nil && len(left) == 0 {
			return true
		}
		if !time.Now().Before(deadline) {
			slog.Warn("a demo's computers were not removed in time, so it is not ended yet", "org", slug, "left", len(left))
			return false
		}
	}
}

func (s *Server) computersWait() time.Duration {
	if s.ComputersWait > 0 {
		return s.ComputersWait
	}
	return ComputersWait
}
