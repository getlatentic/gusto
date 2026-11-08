package spare

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"
)

// shelf is the sandboxes a fake gateway holds, and how the fake maker behaves.
type shelf struct {
	mu      sync.Mutex
	counter int
	live    map[string]bool
	removed []string
	// failures is how many of the next makes fail, and hang is how many hang until their context ends. A failing or hanging make
	// still leaves the sandbox behind, as a gateway can when its command does not come back.
	failures, hang int
	hold           chan struct{}
	made           chan string
}

func newShelf() *shelf { return &shelf{live: map[string]bool{}, made: make(chan string, 100)} }

func (s *shelf) maker() Maker {
	return Maker{
		Name: func() string {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.counter++
			return fmt.Sprintf("s-%02d", s.counter)
		},
		Make: func(ctx context.Context, name string) error {
			s.mu.Lock()
			s.live[name] = true
			failing, hanging, hold := s.failures > 0, s.hang > 0, s.hold
			if failing {
				s.failures--
			}
			if hanging {
				s.hang--
			}
			s.mu.Unlock()
			switch {
			case failing:
				return errors.New("the gateway cannot make a sandbox")
			case hanging:
				<-ctx.Done()
				return ctx.Err()
			case hold != nil:
				select {
				case <-hold:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			s.made <- name
			return nil
		},
		Remove: func(_ context.Context, name string) error {
			s.mu.Lock()
			defer s.mu.Unlock()
			delete(s.live, name)
			s.removed = append(s.removed, name)
			return nil
		},
		Stale: func(context.Context) ([]string, error) { return nil, nil },
	}
}

func (s *shelf) holding() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var names []string
	for name := range s.live {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

var quick = Timing{MakeWithin: time.Second, RemoveWithin: time.Second, RetryFirst: 5 * time.Millisecond, RetryMost: 20 * time.Millisecond}

func next(t *testing.T, made <-chan string) string {
	t.Helper()
	select {
	case name := <-made:
		return name
	case <-time.After(5 * time.Second):
		t.Fatal("no spare was made")
		return ""
	}
}

// ready waits until the pool holds a spare: the maker has returned before the pool has put the spare on its shelf.
func ready(t *testing.T, pool *Pool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !pool.holds() {
		if time.Now().After(deadline) {
			t.Fatal("the pool never held a spare")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestASpareIsMadeAtOpenAndHandedToOneCallerOnly(t *testing.T) {
	gateway := newShelf()
	pool := Open(context.Background(), "key", gateway.maker(), quick)
	defer pool.Close(time.Second)
	first := next(t, gateway.made)
	ready(t, pool)

	var group sync.WaitGroup
	taken := make(chan string, 64)
	for i := 0; i < 64; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			if name, ok := pool.Take("key"); ok {
				taken <- name
			}
		}()
	}
	group.Wait()
	close(taken)
	// The pool makes the next while the callers are still asking, so more than one spare may be handed out, but never one twice.
	seen := map[string]bool{}
	for name := range taken {
		if seen[name] {
			t.Errorf("%s was handed to two callers", name)
		}
		seen[name] = true
	}
	if !seen[first] {
		t.Errorf("handed out %v, want the first spare %s among them", seen, first)
	}
}

func TestOnlyOneOfTwoAtOnceGetsTheSpare(t *testing.T) {
	gateway := newShelf()
	gateway.hold = make(chan struct{})
	pool := Open(context.Background(), "key", gateway.maker(), quick)
	defer pool.Close(time.Second)
	gateway.hold <- struct{}{}
	spare := next(t, gateway.made)
	ready(t, pool)

	start := make(chan struct{})
	results := make(chan string, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			name, _ := pool.Take("key")
			results <- name
		}()
	}
	close(start)
	got := []string{<-results, <-results}
	if (got[0] == spare) == (got[1] == spare) || (got[0] != "" && got[1] != "") {
		t.Errorf("two callers asked at once and got %q and %q: one gets %s and the other nothing", got[0], got[1], spare)
	}
}

func TestNoSpareYetMeansNothingAndNoWaiting(t *testing.T) {
	gateway := newShelf()
	gateway.hold = make(chan struct{})
	pool := Open(context.Background(), "key", gateway.maker(), quick)
	defer pool.Close(time.Second)
	began := time.Now()
	if name, ok := pool.Take("key"); ok || name != "" {
		t.Errorf("took %q while the first was still being made", name)
	}
	if time.Since(began) > time.Second {
		t.Error("asking for a spare waited for one to be made")
	}
}

func TestACallerWithAnotherKeyGetsNoneAndTheSpareStays(t *testing.T) {
	gateway := newShelf()
	pool := Open(context.Background(), "key", gateway.maker(), quick)
	defer pool.Close(time.Second)
	spare := next(t, gateway.made)
	ready(t, pool)
	if _, ok := pool.Take("another"); ok {
		t.Error("a spare made for one policy was handed to a caller with another")
	}
	if name, ok := pool.Take("key"); !ok || name != spare {
		t.Errorf("the spare was lost: %q %v", name, ok)
	}
}

func TestASpareThatFailsToBeMadeIsRemovedAndTriedAgain(t *testing.T) {
	gateway := newShelf()
	gateway.failures = 2
	pool := Open(context.Background(), "key", gateway.maker(), quick)
	defer pool.Close(time.Second)
	good := next(t, gateway.made)
	ready(t, pool)
	if got := gateway.holding(); len(got) != 1 || got[0] != good {
		t.Errorf("the gateway holds %v, want only %s: a spare that failed was left behind", got, good)
	}
	if name, ok := pool.Take("key"); !ok || name != good {
		t.Errorf("took %q %v", name, ok)
	}
}

func TestASpareThatHangsIsGivenUpAndRemoved(t *testing.T) {
	gateway := newShelf()
	gateway.hang = 1
	timing := quick
	timing.MakeWithin = 50 * time.Millisecond
	pool := Open(context.Background(), "key", gateway.maker(), timing)
	defer pool.Close(time.Second)
	good := next(t, gateway.made)
	if got := gateway.holding(); len(got) != 1 || got[0] != good {
		t.Errorf("the gateway holds %v, want only %s", got, good)
	}
}

func TestAnotherIsMadeAfterOneIsTaken(t *testing.T) {
	gateway := newShelf()
	pool := Open(context.Background(), "key", gateway.maker(), quick)
	defer pool.Close(time.Second)
	first := next(t, gateway.made)
	ready(t, pool)
	if name, ok := pool.Take("key"); !ok || name != first {
		t.Fatalf("took %q %v", name, ok)
	}
	second := next(t, gateway.made)
	ready(t, pool)
	if second == first {
		t.Fatal("the same name twice")
	}
	if name, ok := pool.Take("key"); !ok || name != second {
		t.Errorf("took %q %v, want %s", name, ok, second)
	}
	if got := gateway.holding(); len(got) < 1 || len(got) > 2 {
		t.Errorf("the pool keeps one spare and makes the next, and the gateway holds %v", got)
	}
}

func TestOnlyOneIsEverBeingMadeOrKept(t *testing.T) {
	gateway := newShelf()
	pool := Open(context.Background(), "key", gateway.maker(), quick)
	defer pool.Close(time.Second)
	next(t, gateway.made)
	time.Sleep(50 * time.Millisecond)
	if got := gateway.holding(); len(got) != 1 {
		t.Errorf("the gateway holds %v, and a pool is bounded to one", got)
	}
}

func TestTheCellStoppingWhileOneIsBeingMadeLeavesNone(t *testing.T) {
	gateway := newShelf()
	gateway.hold = make(chan struct{})
	pool := Open(context.Background(), "key", gateway.maker(), quick)
	for len(gateway.holding()) == 0 {
		time.Sleep(time.Millisecond)
	}
	pool.Close(5 * time.Second)
	if got := gateway.holding(); len(got) != 0 {
		t.Errorf("the gateway still holds %v after the pool stopped", got)
	}
	if _, ok := pool.Take("key"); ok {
		t.Error("a stopped pool handed out a spare")
	}
}

func TestStoppingRemovesTheSpareThatIsKept(t *testing.T) {
	gateway := newShelf()
	pool := Open(context.Background(), "key", gateway.maker(), quick)
	next(t, gateway.made)
	pool.Close(5 * time.Second)
	if got := gateway.holding(); len(got) != 0 {
		t.Errorf("the gateway still holds %v after the pool stopped", got)
	}
}

func TestTheContextEndingStopsThePoolAndRemovesWhatItHolds(t *testing.T) {
	gateway := newShelf()
	ctx, cancel := context.WithCancel(context.Background())
	pool := Open(ctx, "key", gateway.maker(), quick)
	next(t, gateway.made)
	cancel()
	pool.Close(5 * time.Second)
	if got := gateway.holding(); len(got) != 0 {
		t.Errorf("the gateway still holds %v", got)
	}
}

func TestASpareGivenBackIsKeptAndNeverBecomesTwo(t *testing.T) {
	gateway := newShelf()
	pool := Open(context.Background(), "key", gateway.maker(), quick)
	defer pool.Close(time.Second)
	first := next(t, gateway.made)
	ready(t, pool)
	taken, _ := pool.Take("key")
	pool.Return(taken)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if got := gateway.holding(); len(got) == 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	if got := gateway.holding(); len(got) != 1 {
		t.Errorf("the gateway holds %v, want exactly one after a spare was given back (first was %s)", got, first)
	}
}

func TestSparesAnEarlierServerLeftAreRemovedAtOpen(t *testing.T) {
	gateway := newShelf()
	gateway.live["s-old"] = true
	maker := gateway.maker()
	maker.Stale = func(context.Context) ([]string, error) { return []string{"s-old"}, nil }
	pool := Open(context.Background(), "key", maker, quick)
	defer pool.Close(time.Second)
	next(t, gateway.made)
	for _, name := range gateway.holding() {
		if name == "s-old" {
			t.Error("a spare left by a server that was killed is still on the gateway")
		}
	}
}
