package cloud

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// twoDoors is two front doors on one database, on one clock that advance moves.
func twoDoors(t *testing.T, set Settings) (a, b *Store, advance func(time.Duration)) {
	t.Helper()
	stores := doors(t, set, 2)
	at := time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)
	var mu sync.Mutex
	clock := func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return at
	}
	for _, s := range stores {
		s.now = clock
	}
	return stores[0], stores[1], func(d time.Duration) {
		mu.Lock()
		defer mu.Unlock()
		at = at.Add(d)
	}
}

// One front door holds the lease until it runs out; the next to take it gets the next epoch, and what the earlier one writes
// after is refused.
func TestOneFrontDoorLeadsUntilItsLeaseRunsOut(t *testing.T) {
	eachShared(t, func(t *testing.T, set Settings) {
		a, b, advance := twoDoors(t, set)
		org := made(t, a, "ada@acme.com", "Acme")
		if epoch, err := a.Lead(t.Context(), "a", 30*time.Second); err != nil || epoch != 1 {
			t.Fatalf("the first front door: %d %v", epoch, err)
		}
		if epoch, _ := b.Lead(t.Context(), "b", 30*time.Second); epoch != 0 {
			t.Errorf("a second front door led while the first's lease held: %d", epoch)
		}
		advance(20 * time.Second)
		if epoch, _ := a.Lead(t.Context(), "a", 30*time.Second); epoch != 1 {
			t.Errorf("renewing changed the epoch to %d", epoch)
		}
		advance(31 * time.Second)
		if epoch, _ := b.Lead(t.Context(), "b", 30*time.Second); epoch != 2 {
			t.Fatalf("the lease that ran out was not taken: %d", epoch)
		}
		if err := a.SetRuntimeAt(t.Context(), 1, org.Slug, Runtime{State: Ready}); !errors.Is(err, ErrNotLeading) {
			t.Errorf("the earlier leader's write was taken: %v", err)
		}
		if err := b.SetRuntimeAt(t.Context(), 2, org.Slug, Runtime{State: Ready}); err != nil {
			t.Errorf("the leader's write was refused: %v", err)
		}
		if err := b.Resign(t.Context(), "b"); err != nil {
			t.Fatal(err)
		}
		if epoch, _ := a.Lead(t.Context(), "a", 30*time.Second); epoch != 3 {
			t.Errorf("a lease given up was not taken at once: %d", epoch)
		}
	})
}

// A controller whose lease has moved on starts nothing, however its provision was begun: the trace TLC finds in
// spec/FrontDoors.tla when writes carry no epoch.
func TestAControllerThatLostTheLeaseStartsNothing(t *testing.T) {
	eachShared(t, func(t *testing.T, set Settings) {
		a, b, advance := twoDoors(t, set)
		org := made(t, a, "ada@acme.com", "Acme")
		old := controlled(t, a)
		epoch, _ := a.Lead(t.Context(), "a", 30*time.Second)
		old.Lead(epoch)
		advance(31 * time.Second)
		if epoch, _ := b.Lead(t.Context(), "b", 30*time.Second); epoch != 2 {
			t.Fatalf("the lease was not taken: %d", epoch)
		}
		if err := old.Provision(t.Context(), org.Slug); !errors.Is(err, ErrNotLeading) {
			t.Errorf("a controller that lost the lease provisioned: %v", err)
		}
		if old.Node.Alive(org.Slug) {
			t.Error("a controller that lost the lease started a server")
		}
	})
}

// A front door that does not lead starts nothing, and routes to where the leader started the server.
func TestAFollowerStartsNothingAndRoutesWhereTheLeaderStarted(t *testing.T) {
	eachShared(t, func(t *testing.T, set Settings) {
		a, b, _ := twoDoors(t, set)
		org := made(t, a, "ada@acme.com", "Acme")
		leader, follower := controlled(t, a), controlled(t, b)
		epoch, _ := a.Lead(t.Context(), "a", 30*time.Second)
		leader.Lead(epoch)
		follower.Follow()
		if err := follower.Provision(t.Context(), org.Slug); err != nil || follower.Node.Alive(org.Slug) {
			t.Fatalf("a follower provisioned: %v", err)
		}
		if err := leader.Provision(t.Context(), org.Slug); err != nil {
			t.Fatal(err)
		}
		led, state := leader.Route(t.Context(), org.Slug)
		followed, followedState := follower.Route(t.Context(), org.Slug)
		if state != Ready || followedState != Ready || followed != led {
			t.Errorf("the follower routes to %q (%s), the leader to %q (%s)", followed, followedState, led, state)
		}
	})
}

// An organization signed up on a front door that does not lead is provisioned by the leader.
func TestTheLeaderProvisionsAnOrganizationSignedUpElsewhere(t *testing.T) {
	eachShared(t, func(t *testing.T, set Settings) {
		a, _, _ := twoDoors(t, set)
		org := made(t, a, "ada@acme.com", "Acme")
		leader := controlled(t, a)
		epoch, _ := a.Lead(t.Context(), "a", 30*time.Second)
		leader.Lead(epoch)
		leader.supervise(t.Context())
		deadline := time.Now().Add(20 * time.Second)
		for !leader.Node.Alive(org.Slug) {
			if time.Now().After(deadline) {
				t.Fatal("the leader did not provision an organization with no runtime")
			}
			time.Sleep(50 * time.Millisecond)
		}
	})
}

// Two front doors run the leader's work one at a time, and the second takes over when the first stops.
func TestLeadershipRunsTheWorkOnOneFrontDoorAtATime(t *testing.T) {
	eachShared(t, func(t *testing.T, set Settings) {
		stores := doors(t, set, 2)
		var running, most atomic.Int32
		led := make(chan string, 8)
		work := func(name string) func(context.Context, int64) {
			return func(ctx context.Context, _ int64) {
				now := running.Add(1)
				for seen := most.Load(); now > seen && !most.CompareAndSwap(seen, now); seen = most.Load() {
				}
				led <- name
				<-ctx.Done()
				running.Add(-1)
			}
		}
		first, stopFirst := context.WithCancel(context.Background())
		second, stopSecond := context.WithCancel(context.Background())
		defer stopSecond()
		var wg sync.WaitGroup
		wg.Go(func() {
			(&Leadership{Store: stores[0], Holder: "a", TTL: time.Minute, Every: 20 * time.Millisecond}).Run(first, work("a"))
		})
		if got := <-led; got != "a" {
			t.Fatalf("%s led first", got)
		}
		wg.Go(func() {
			(&Leadership{Store: stores[1], Holder: "b", TTL: time.Minute, Every: 20 * time.Millisecond}).Run(second, work("b"))
		})
		time.Sleep(100 * time.Millisecond)
		stopFirst()
		select {
		case got := <-led:
			if got != "b" {
				t.Errorf("%s led after the first stopped", got)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("the second front door did not take over when the first gave the lease up")
		}
		stopSecond()
		wg.Wait()
		if most.Load() != 1 {
			t.Errorf("%d front doors led at once", most.Load())
		}
	})
}
