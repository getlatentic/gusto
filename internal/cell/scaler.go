package cell

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// Instance is a host that was started to be a host, and when.
type Instance struct {
	ID       string
	Launched time.Time
}

// Launcher starts and stops the machines hosts run on.
type Launcher interface {
	// Launch starts a host, which registers as a cell when it is ready.
	Launch() (id string, err error)
	Terminate(id string) error
	// Running is the hosts started for cells that are not gone.
	Running() ([]Instance, error)
}

// Scaler keeps as many hosts as the organizations need: one is started when an organization waits for room and none is
// on its way, and a host with no organization on it for a while is stopped. With nothing to run there is no host, and
// nothing is paid for.
type Scaler struct {
	Pool     *Pool
	Launcher Launcher
	// Waiting is how many organizations wait for a host with room.
	Waiting func() int
	// Max is the most hosts there may be at once.
	Max int
	// Idle is how long a host may have nothing on it before it is stopped.
	Idle time.Duration
	// Boot is how long a host has to register as a host before it is taken to have failed and is stopped.
	Boot time.Duration
	// Backoff is how long after a host could not be started another is not tried.
	Backoff time.Duration
	// Settle is how long a host that has just registered is given to take the organizations that wait, which are placed
	// on it when the controller next looks. A host is not started for them in that time, since they are about to have room.
	Settle time.Duration
	// Prewarms is the most hosts a day started because a visitor began asking for a demo, before anybody waits. It
	// bounds what visitors who never send the form can cost. Zero is none.
	Prewarms int

	now       func() time.Time
	idleSince map[string]time.Time
	firstSeen map[string]time.Time
	retryAt   time.Time

	wakeOnce sync.Once
	woken    chan struct{}

	// guard guards the prewarm fields, which a request sets while Run decides.
	guard       sync.Mutex
	expectUntil time.Time
	prewarmDay  string
	prewarmed   int
}

// expected is how long after a visitor begins asking for a demo a host is started for it before it waits: the time it takes
// to send the form, and a little more.
const expected = 2 * time.Minute

// Prewarm says a visitor has begun asking for a demo, so a host is started now, while they finish, if none is up or on its
// way. It never blocks.
func (s *Scaler) Prewarm() {
	s.guard.Lock()
	s.expectUntil = s.clock().Add(expected)
	s.guard.Unlock()
	s.Wake()
}

// expecting says a host should be started for a visitor about to wait, and counts it against the day's prewarms. It is
// asked only when nothing waits, no host is up and none is starting.
func (s *Scaler) expecting() bool {
	s.guard.Lock()
	defer s.guard.Unlock()
	now := s.clock()
	if !now.Before(s.expectUntil) {
		return false
	}
	if day := now.UTC().Format(time.DateOnly); day != s.prewarmDay {
		s.prewarmDay, s.prewarmed = day, 0
	}
	if s.prewarmed >= s.Prewarms {
		return false
	}
	s.prewarmed++
	s.expectUntil = time.Time{}
	return true
}

// wake is the scaler's mailbox: one pending wake-up is enough, since a decision looks at everything there is.
func (s *Scaler) wake() chan struct{} {
	s.wakeOnce.Do(func() { s.woken = make(chan struct{}, 1) })
	return s.woken
}

// Wake has the scaler decide now rather than at its next tick, as when an organization starts waiting for room. It never
// blocks: a wake-up already pending covers this one.
func (s *Scaler) Wake() {
	select {
	case s.wake() <- struct{}{}:
	default:
	}
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

// InstanceOf is the machine a host runs on. A host is named for its machine when it starts.
func InstanceOf(cellName string) string { return "i-" + strings.TrimPrefix(cellName, "cell-") }

// Run decides when it is woken, and every so often in case a change was missed, until the context ends. Hosts are left
// running when it ends, since another gusto cloud will find them.
func (s *Scaler) Run(ctx context.Context, every time.Duration) {
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		s.Tick()
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		case <-s.wake():
		}
	}
}

// Tick makes one decision: it stops hosts that failed to become hosts and hosts that have been idle, and starts a host
// when an organization waits and none is starting.
func (s *Scaler) Tick() {
	hosts, err := s.Launcher.Running()
	if err != nil {
		slog.Error("the hosts could not be listed", "error", err)
		return
	}
	registered, err := Fresh(s.Pool.Dir, s.clock(), s.Pool.maxAge())
	if err != nil {
		slog.Error("the hosts could not be listed", "error", err)
		return
	}
	alive := map[string]Registration{}
	for _, each := range registered {
		alive[InstanceOf(each.Name)] = each
	}
	starting := s.reap(hosts, alive)
	waiting := s.Waiting()
	s.retire(hosts, alive, waiting)
	if starting > 0 || s.settling(alive) || len(hosts) >= s.Max || s.clock().Before(s.retryAt) {
		return
	}
	why := "organizations are waiting"
	if waiting == 0 {
		if len(hosts) > 0 || !s.expecting() {
			return
		}
		why = "a visitor is asking for a demo"
	}
	id, err := s.Launcher.Launch()
	if err != nil {
		s.retryAt = s.clock().Add(orDefault(s.Backoff, time.Minute))
		slog.Error("a host could not be started", "error", err)
		return
	}
	slog.Info("a host was started", "host", id, "because", why, "waiting", waiting)
}

// settling says a cell registered a moment ago. It is counted from when it was first seen to be a cell, so a front door
// that starts again waits the same moment for the cells it finds.
func (s *Scaler) settling(alive map[string]Registration) bool {
	if s.firstSeen == nil {
		s.firstSeen = map[string]time.Time{}
	}
	now, settling := s.clock(), false
	for id := range alive {
		first, seen := s.firstSeen[id]
		if !seen {
			s.firstSeen[id], first = now, now
		}
		settling = settling || now.Sub(first) < orDefault(s.Settle, 30*time.Second)
	}
	for id := range s.firstSeen {
		if _, there := alive[id]; !there {
			delete(s.firstSeen, id)
		}
	}
	return settling
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
		slog.Warn("a host did not come up in time, so it is stopped", "host", host.ID)
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
			slog.Info("a host has had nothing on it, so it is stopped", "host", id)
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
