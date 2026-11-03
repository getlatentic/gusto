package cell

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/cloud"
)

// realTime starts the rig's clock at the time it is, which the pool places by and signs its requests with.
func realTime(t *testing.T, r *rig) {
	t.Helper()
	was := clockNow
	t.Cleanup(func() { clockNow = was })
	r.now = time.Now()
}

// A host the scaler stops or removes is gone to the pool at once, so the organizations placed on it are placed again rather
// than tried there until it has been silent for half an hour. spec/Hosts.before-unregister.cfg is the trace.
func TestAHostTheScalerStopsOrRemovesIsGoneToThePool(t *testing.T) {
	gone := func(r *rig, id string) {
		t.Helper()
		if _, err := r.scaler.Pool.Get(CellOf(id)); !errors.Is(err, cloud.ErrCellGone) {
			t.Errorf("the pool says of %s, which the scaler stopped: %v", id, err)
		}
	}
	t.Run("idle", func(t *testing.T) {
		r := scaling(t)
		r.scaler.Pool.now = r.scaler.now
		r.hosts.hosts = []Instance{{ID: "i-0aaa1111", Launched: r.now}}
		r.cell("i-0aaa1111", false)
		r.tick(0)
		r.tick(11 * time.Minute)
		if !slices.Contains(r.hosts.terminated, "i-0aaa1111") {
			t.Fatalf("the idle host was not removed: %v", r.hosts.terminated)
		}
		gone(r, "i-0aaa1111")
	})
	t.Run("quiet", func(t *testing.T) {
		r := scaling(t)
		r.scaler.Pool.now = r.scaler.now
		r.scaler.Pool.MaxAge = 30 * time.Second
		r.hosts.hosts = []Instance{{ID: "i-0aaa1111", Launched: r.now}}
		r.cell("i-0aaa1111", false)
		r.tick(0)
		r.tick(11 * time.Minute)
		if !slices.Contains(r.hosts.terminated, "i-0aaa1111") {
			t.Fatalf("the quiet host was not removed: %v", r.hosts.terminated)
		}
		gone(r, "i-0aaa1111")
	})
}

// An organization being placed on the idle host keeps it from being stopped: the placement is made while the scaler asks the
// host whether it has anything on it. spec/Hosts.before-busy.cfg is the trace.
func TestAHostIsNotStoppedUnderAnOrganizationBeingPlacedOnIt(t *testing.T) {
	r := scaling(t)
	realTime(t, r)
	var (
		mu     sync.Mutex
		armed  bool
		placed string
		busy   int
	)
	r.scaler.Busy = func() int { mu.Lock(); defer mu.Unlock(); return busy }
	h := newAgent(t, "cell-0aaa1111", 9000)
	h.agent.Measure = func() Capacity {
		mu.Lock()
		place := armed && placed == ""
		if place {
			placed = "asking"
		}
		mu.Unlock()
		if place {
			on, err := r.scaler.Pool.Place()
			mu.Lock()
			if err == nil {
				placed, busy = on.Name(), busy+1
			}
			mu.Unlock()
		}
		return Capacity{CPUs: 4, MemoryAvailableMiB: 9000}
	}
	r.hosts.hosts = []Instance{{ID: "i-0aaa1111", Launched: r.now}}
	if err := Register(r.scaler.Pool.Dir, h.registration("cell-0aaa1111", r.now)); err != nil {
		t.Fatal(err)
	}
	r.tick(0)
	mu.Lock()
	armed = true
	mu.Unlock()
	r.tick(11 * time.Minute)
	mu.Lock()
	defer mu.Unlock()
	if placed != "cell-0aaa1111" {
		t.Fatalf("the organization was placed on %q", placed)
	}
	if slices.Contains(r.hosts.terminated, "i-0aaa1111") {
		t.Error("the host was stopped under the organization being placed on it")
	}
}

// A kept host that is being stopped and says it is there on its way down is not placed on.
func TestAHostBeingStoppedIsNotPlacedOnWhileItBeatsOnItsWayDown(t *testing.T) {
	r := scaling(t)
	realTime(t, r)
	r.scaler.Keep = 1
	r.hosts.hosts = []Instance{{ID: "i-0aaa1111", Launched: r.now, Kept: true}}
	h := r.cell("i-0aaa1111", false)
	r.tick(0)
	r.tick(11 * time.Minute)
	if len(r.hosts.stopped) != 1 {
		t.Fatalf("the idle kept host was not stopped: %+v", r.hosts.stopped)
	}
	if err := Register(r.scaler.Pool.Dir, h.registration("cell-0aaa1111", time.Now())); err != nil {
		t.Fatal(err)
	}
	if on, err := r.scaler.Pool.Place(); err == nil {
		t.Errorf("an organization was placed on %s, which is being stopped", on.Name())
	}
}

// A front door that starts again counts on from the day's prewarms, which bound what visitors who never send the form cost.
func TestTheDaysPrewarmsAreCountedOnByAFrontDoorThatStartsAgain(t *testing.T) {
	r := scaling(t)
	r.scaler.Prewarms, r.scaler.Max = 1, 5
	r.scaler.Prewarm()
	r.tick(0)
	if r.hosts.launched != 1 {
		t.Fatalf("the day's prewarm started %d hosts", r.hosts.launched)
	}
	r.hosts.hosts = nil
	again := &Scaler{Pool: &Pool{Dir: r.scaler.Pool.Dir, Secret: secret, MaxAge: time.Hour}, Launcher: r.hosts,
		Waiting: func() int { return 0 }, Max: 5, Prewarms: 1, now: r.scaler.now}
	again.Prewarm()
	again.Tick()
	if r.hosts.launched != 1 {
		t.Errorf("a front door that started again started another prewarm the same day: %d", r.hosts.launched)
	}
}

// A front door that starts again knows when a quiet host was last heard from, so one that went quiet a moment ago is left to
// shut down rather than removed.
func TestAHostThatWentQuietAMomentAgoIsLeftByAFrontDoorThatStartsAgain(t *testing.T) {
	r := scaling(t)
	r.scaler.Pool.MaxAge = 30 * time.Second
	r.hosts.hosts = []Instance{{ID: "i-0aaa1111", Launched: r.now, Kept: true}}
	r.now = r.now.Add(20 * time.Minute)
	r.cell("i-0aaa1111", false)
	r.tick(0)
	again := &Scaler{Pool: &Pool{Dir: r.scaler.Pool.Dir, Secret: secret, MaxAge: 30 * time.Second}, Launcher: r.hosts,
		Waiting: func() int { return 0 }, Max: 2, Idle: 10 * time.Minute, Boot: 10 * time.Minute, now: r.scaler.now}
	r.now = r.now.Add(40 * time.Second)
	again.Tick()
	if len(r.hosts.terminated) != 0 {
		t.Errorf("a host heard from 40 seconds ago was removed by a front door that started again: %v", r.hosts.terminated)
	}
}

// A host that goes stops its servers together, each given its grace, and takes its registration away, inside the time its
// service has to stop.
func TestAHostThatGoesStopsItsServersTogetherAndTakesItsRegistrationAway(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a server is asked to end by a signal Windows does not send")
	}
	dir := filepath.Join(t.TempDir(), "cells")
	servers := &cloud.LocalCell{}
	t.Cleanup(func() {
		for _, slug := range servers.Running() {
			servers.Stop(slug, time.Second)
		}
	})
	for _, slug := range []string{"acme-com", "beta-com", "gamma-com"} {
		spec := fakeOrg(t, slug, freePort(t))
		spec.Env = append(spec.Env, "CELL_IGNORE_TERM=1")
		if err := servers.Start(spec); err != nil {
			t.Fatal(err)
		}
	}
	agent := &Agent{Name: "cell-0aaa1111", Private: "127.0.0.1", Secret: secret, Servers: servers, Grace: time.Second}
	ctx, cancel := context.WithCancel(context.Background())
	ran := make(chan error, 1)
	go func() { ran <- agent.Run(ctx, "127.0.0.1:0", dir, time.Hour) }()
	waitFor(t, "the host registered", func() bool { _, err := os.Stat(file(dir, agent.Name)); return err == nil })
	began := time.Now()
	cancel()
	if err := <-ran; err != nil {
		t.Fatal(err)
	}
	if took := time.Since(began); took > 2500*time.Millisecond {
		t.Errorf("three servers that do not end when asked took %s to stop, one after another", took.Round(time.Millisecond))
	}
	if _, err := os.Stat(file(dir, agent.Name)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the registration is still there: %v", err)
	}
}
