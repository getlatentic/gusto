package cell

import (
	"context"
	"log/slog"
	"strings"
	"time"
)

// Instance is a host that was started to be a cell, and when.
type Instance struct {
	ID       string
	Launched time.Time
}

// Launcher starts and stops the hosts cells run on.
type Launcher interface {
	// Launch starts a host, which registers as a cell when it is ready.
	Launch() (id string, err error)
	Terminate(id string) error
	// Running is the hosts started for cells that are not gone.
	Running() ([]Instance, error)
}

// Scaler keeps as many cells as the organisations need: one is started when an organisation waits for room and none is
// on its way, and a cell with no organisation on it for a while is stopped. With nothing to run there is no cell, and
// nothing is paid for.
type Scaler struct {
	Pool     *Pool
	Launcher Launcher
	// Waiting is how many organisations wait for a cell with room.
	Waiting func() int
	// Max is the most hosts there may be at once.
	Max int
	// Idle is how long a cell may have nothing on it before it is stopped.
	Idle time.Duration
	// Boot is how long a host has to register as a cell before it is taken to have failed and is stopped.
	Boot time.Duration
	// Backoff is how long after a host could not be started another is not tried.
	Backoff time.Duration

	now       func() time.Time
	idleSince map[string]time.Time
	retryAt   time.Time
}

func (s *Scaler) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

func orDefault(d, fallback time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return fallback
}

// InstanceOf is the host a cell runs on. A cell is named for its host when it starts.
func InstanceOf(cellName string) string { return "i-" + strings.TrimPrefix(cellName, "cell-") }

// Run decides every so often, until the context ends. Hosts are left running when it ends, since another front door
// will find them.
func (s *Scaler) Run(ctx context.Context, every time.Duration) {
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		s.Tick()
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// Tick makes one decision: it stops hosts that failed to become cells and cells that have been idle, and starts a host
// when an organisation waits and none is starting.
func (s *Scaler) Tick() {
	hosts, err := s.Launcher.Running()
	if err != nil {
		slog.Error("the hosts that are cells could not be listed", "error", err)
		return
	}
	registered, err := Fresh(s.Pool.Dir, s.clock(), s.Pool.maxAge())
	if err != nil {
		slog.Error("the cells could not be listed", "error", err)
		return
	}
	alive := map[string]Registration{}
	for _, each := range registered {
		alive[InstanceOf(each.Name)] = each
	}
	starting := s.reap(hosts, alive)
	waiting := s.Waiting()
	s.retire(hosts, alive, waiting)
	if waiting > 0 && starting == 0 && len(hosts) < s.Max && !s.clock().Before(s.retryAt) {
		id, err := s.Launcher.Launch()
		if err != nil {
			s.retryAt = s.clock().Add(orDefault(s.Backoff, time.Minute))
			slog.Error("a host for a cell could not be started", "error", err)
			return
		}
		slog.Info("a host was started for a cell, because organisations are waiting", "host", id, "waiting", waiting)
	}
}

// reap stops a host that was started and has not registered in the time it has, and says how many are still starting.
func (s *Scaler) reap(hosts []Instance, alive map[string]Registration) int {
	starting := 0
	for _, host := range hosts {
		if _, registered := alive[host.ID]; registered {
			continue
		}
		if s.clock().Sub(host.Launched) <= orDefault(s.Boot, 10*time.Minute) {
			starting++
			continue
		}
		slog.Warn("a host did not become a cell in time, so it is stopped", "host", host.ID)
		if err := s.Launcher.Terminate(host.ID); err != nil {
			slog.Error("a host could not be stopped", "host", host.ID, "error", err)
		}
	}
	return starting
}

// retire stops a cell that has had nothing on it for as long as a cell may, unless somebody is waiting for room.
func (s *Scaler) retire(hosts []Instance, alive map[string]Registration, waiting int) {
	if s.idleSince == nil {
		s.idleSince = map[string]time.Time{}
	}
	onHost := map[string]bool{}
	for _, host := range hosts {
		onHost[host.ID] = true
	}
	for id, registration := range alive {
		if !onHost[id] {
			continue
		}
		status, err := newRemote(registration, s.Pool.Secret).Status()
		if err != nil || len(status.Servers) > 0 || waiting > 0 {
			delete(s.idleSince, id)
			continue
		}
		since, idle := s.idleSince[id]
		if !idle {
			s.idleSince[id] = s.clock()
			continue
		}
		if s.clock().Sub(since) >= orDefault(s.Idle, 10*time.Minute) {
			slog.Info("a cell has had nothing on it, so its host is stopped", "host", id)
			if err := s.Launcher.Terminate(id); err != nil {
				slog.Error("a host could not be stopped", "host", id, "error", err)
				continue
			}
			delete(s.idleSince, id)
		}
	}
	for id := range s.idleSince {
		if !onHost[id] {
			delete(s.idleSince, id)
		}
	}
}
