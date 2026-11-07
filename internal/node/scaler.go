package node

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// Instance is a machine that was started to be a node, and when it was last started. A kept node is stopped rather than removed
// when it is idle; Stopped says it is stopped now, and not still stopping. Image is the machine image it was made from.
type Instance struct {
	ID       string
	Launched time.Time
	Kept     bool
	Stopped  bool
	Image    string
}

// Launcher starts and stops the machines nodes run on.
type Launcher interface {
	// Launch makes a node, which registers as a node when it is ready.
	Launch(kept bool) (id string, err error)
	Terminate(id string) error
	// Running is the nodes started for nodes that are not gone or stopped.
	Running() ([]Instance, error)
	// Stopped is the nodes that are stopping or stopped, whether kept or not; Start starts one, and Stop stops a kept node that is running.
	Stopped() ([]Instance, error)
	Start(id string) error
	Stop(id string) error
	// LowerDisk takes a stopped node's disk down to the baseline throughput, which is billed while it is stopped; it does nothing
	// to a disk already there.
	LowerDisk(id string) error
	// CurrentImage is the machine image a new node is made from now, and empty when that is not known.
	CurrentImage() (string, error)
}

// Scaler keeps as many nodes as the organizations need: one is started when an organization waits for room and none is
// on its way, and a node with no organization on it for a while is stopped. With nothing to run there is no node, and
// nothing is paid for.
type Scaler struct {
	Pool     *Pool
	Launcher Launcher
	// Waiting is how many organizations wait for a node with room.
	Waiting func() int
	// Busy is how many organizations wait for room or are being placed and started. No node is judged idle while any are,
	// since the one an organization is being placed on may be the idle one. Waiting stands in for it when it is not set.
	Busy func() int
	// Max is the most nodes there may be at once.
	Max int
	// Idle is how long a node may have nothing on it before it is stopped.
	Idle time.Duration
	// Boot is how long a node has to register as a node before it is taken to have failed and is stopped.
	Boot time.Duration
	// Backoff is how long after a node could not be started another is not tried.
	Backoff time.Duration
	// Settle is how long a node that has just registered is given to take the organizations that wait, which are placed
	// on it when the controller next looks. A node is not started for them in that time, since they are about to have room.
	Settle time.Duration
	// Keep is how many nodes are kept: stopped when idle and started again when a node is needed, which a stopped node's
	// disk, already read, makes faster than a new one. Zero keeps none.
	Keep int
	// Prewarms is the most nodes a day started because a visitor began asking for a demo, before anybody waits. It
	// bounds what visitors who never send the form can cost. Zero is none.
	Prewarms int

	now       func() time.Time
	idleSince map[string]time.Time
	firstSeen map[string]time.Time
	retryAt   time.Time
	// lowered is the stopped kept nodes whose disk is known to be at the baseline.
	lowered map[string]bool

	wakeOnce sync.Once
	woken    chan struct{}

	// guard guards expectUntil, which a request sets while Run decides.
	guard       sync.Mutex
	expectUntil time.Time
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

// InstanceOf is the machine a node runs on. A node is named for its machine when it starts.
func InstanceOf(nodeName string) string { return "i-" + strings.TrimPrefix(nodeName, "cell-") }

// Run decides when it is woken, and every so often in case a change was missed, until the context ends. Nodes are left
// running when it ends, since another gusto cloud server will find them.
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

// Tick makes one decision: it stops nodes that failed to become nodes and nodes that have been idle, and starts a node
// when an organization waits and none is starting.
func (s *Scaler) Tick() {
	hosts, err := s.Launcher.Running()
	if err != nil {
		slog.Error("the hosts could not be listed", "error", err)
		return
	}
	if stopped := s.sweep(hosts); stopped != nil {
		s.forgetRetired(hosts, stopped)
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
	s.retire(hosts, alive)
	waiting := s.Waiting()
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
	id, err := s.bringUp(hosts)
	if err != nil {
		s.retryAt = s.clock().Add(orDefault(s.Backoff, time.Minute))
		slog.Error("a host could not be started", "error", err)
		return
	}
	slog.Info("a host was started", "host", id, "because", why, "waiting", waiting)
}

// settling says a node registered a moment ago. It is counted from when it was first seen to be a node, so a gusto cloud server
// that starts again waits the same moment for the nodes it finds.
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

// NodeOf is the node a machine runs: the name InstanceOf takes back to the machine.
func NodeOf(instance string) string { return "cell-" + strings.TrimPrefix(instance, "i-") }

// reap removes a node that has not been a registered node for as long as a node has to become one, counted from when it was started
// or was last heard from, whichever is later: a node that registered a moment ago and went quiet is a node that is shutting
// down, as a node does when it powers itself off, and is left for AWS to stop. It says how many are still starting.
func (s *Scaler) reap(hosts []Instance, alive map[string]Registration) int {
	now, starting := s.clock(), 0
	for _, host := range hosts {
		if _, registered := alive[host.ID]; registered {
			continue
		}
		if now.Sub(s.lastHeard(host)) <= orDefault(s.Boot, 10*time.Minute) {
			starting++
			continue
		}
		slog.Warn("a host did not come up in time, so it is stopped", "host", host.ID)
		if err := s.Launcher.Terminate(host.ID); err != nil {
			slog.Error("a host could not be stopped", "host", host.ID, "error", err)
			continue
		}
		s.unregister(host.ID)
	}
	return starting
}

// lastHeard is when a node was started or last said it was there, whichever is later. The registration keeps the second, so a
// gusto cloud that starts again knows it.
func (s *Scaler) lastHeard(host Instance) time.Time {
	_, age, err := Registered(s.Pool.Dir, NodeOf(host.ID), s.clock())
	if heard := s.clock().Add(-age); err == nil && heard.After(host.Launched) {
		return heard
	}
	return host.Launched
}

// unregister takes away the registration of a node that was stopped or removed, so that the organizations placed on it are
// placed again rather than tried there until it has been silent for long.
func (s *Scaler) unregister(id string) {
	if err := Unregister(s.Pool.Dir, NodeOf(id)); err != nil {
		slog.Error("a stopped host's registration could not be taken away", "host", id, "error", err)
	}
}

func (s *Scaler) busy() int {
	if s.Busy != nil {
		return s.Busy()
	}
	return s.Waiting()
}

// retire stops a node that has had nothing on it for as long as a node may, unless an organization waits for room or is being
// placed or started. A restarted gusto cloud gives each idle node the whole of that time again, which delays a stop and never
// hastens one.
func (s *Scaler) retire(hosts []Instance, alive map[string]Registration) {
	if s.idleSince == nil {
		s.idleSince = map[string]time.Time{}
	}
	onHost, kept := map[string]bool{}, map[string]bool{}
	for _, host := range hosts {
		onHost[host.ID], kept[host.ID] = true, host.Kept
	}
	for id, registration := range alive {
		if onHost[id] && s.idleLongEnough(id, registration) {
			s.retireIdle(id, registration, kept[id])
		}
	}
	for id := range s.idleSince {
		if !onHost[id] {
			delete(s.idleSince, id)
		}
	}
}

// idleLongEnough says a node is idle now and has been for as long as a node may be.
func (s *Scaler) idleLongEnough(id string, registration Registration) bool {
	if !s.idle(registration) {
		delete(s.idleSince, id)
		return false
	}
	since, seen := s.idleSince[id]
	if !seen {
		s.idleSince[id] = s.clock()
		return false
	}
	return s.clock().Sub(since) >= orDefault(s.Idle, 10*time.Minute)
}

// idle says a node has no server on it and no organization waits for room or is being placed or started.
func (s *Scaler) idle(registration Registration) bool {
	status, err := newRemote(registration, s.Pool.Secret).Status()
	return err == nil && len(status.Servers) == 0 && s.busy() == 0
}

// retireIdle stops an idle node. It is marked first, so that from the moment it is checked the last time nothing is placed or
// started on it; an organization placed on it before then is being placed, and keeps it.
func (s *Scaler) retireIdle(id string, registration Registration, kept bool) {
	s.Pool.setRetiring(registration.Name, true)
	if !s.idle(registration) {
		s.Pool.setRetiring(registration.Name, false)
		delete(s.idleSince, id)
		return
	}
	slog.Info("a host has had nothing on it, so it is stopped", "host", id, "kept", kept)
	if err := s.putAway(id, kept); err != nil {
		slog.Error("a host could not be stopped", "host", id, "error", err)
		s.Pool.setRetiring(registration.Name, false)
		return
	}
	delete(s.idleSince, id)
	s.unregister(id)
}
