package cloud

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"
)

// A page waiting on its organization's server is answered the moment the server is ready, and at once when what it shows is
// already out of date.
func TestAPageWaitingOnItsServerIsAnsweredWhenTheServerIsReady(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := withServers(t, "FAKE_DELAY=1500ms")
		b := f.browser()
		b.post("/signup", url.Values{"email": {"ada@acme.com"}})
		drawn := shownAt(t, b, "/org")
		if drawn.Watch == nil || drawn.Watch.State == Ready {
			t.Fatalf("the page does not wait on a server that is starting: %+v", drawn.Watch)
		}
		// The page is told each change, and asks again from what it was told, until the server is ready.
		asked, seen := time.Now(), drawn.Watch.State
		for answers := 0; seen != Ready && answers < 5; answers++ {
			next := serverStateSeen(t, b, seen)
			if next == seen {
				t.Fatalf("the page was answered with what it showed, %q, after %s", seen, time.Since(asked).Round(time.Millisecond))
			}
			seen = next
		}
		if seen != Ready {
			t.Fatalf("the waiting page was last answered %q", seen)
		}
		if took := time.Since(asked); took > 10*time.Second {
			t.Errorf("the page was told the server is ready %s after it asked", took.Round(time.Millisecond))
		}
		if state := serverStateSeen(t, b, Starting); state != Ready {
			t.Errorf("a page drawn before the server was ready is told at once: %q", state)
		}
		if got := shownAt(t, b, "/org"); got.Watch != nil {
			t.Errorf("a page whose server is ready waits on it: %+v", got.Watch)
		}
		if status, _, _ := f.browser().do("GET", "/org/server?seen=ready", nil, nil); status != http.StatusSeeOther {
			t.Errorf("somebody signed out was answered %d", status)
		}
	})
}

func serverStateSeen(t *testing.T, b *browser, seen State) State {
	t.Helper()
	status, body, _ := b.do("GET", "/org/server?seen="+url.QueryEscape(string(seen)), nil, nil)
	var answer watch
	if err := json.Unmarshal([]byte(body), &answer); status != http.StatusOK || err != nil {
		t.Fatalf("%d %v %s", status, err, body)
	}
	return answer.State
}

// Waiting on an organization ends when its state changes, or after the longest wait with the state it had.
func TestWaitingOnAnOrganizationEndsWhenItsStateChanges(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		c := controlled(t, open(t, Settings{}))
		began := time.Now()
		if state := c.AwaitChange(t.Context(), "acme-com", "", 50*time.Millisecond); state != "" || time.Since(began) < 50*time.Millisecond {
			t.Errorf("nothing changed and the wait ended after %s with %q", time.Since(began), state)
		}
		go func() {
			time.Sleep(30 * time.Millisecond)
			c.put("acme-com", Runtime{State: Waiting}, true)
		}()
		if state := c.AwaitChange(t.Context(), "acme-com", "", 10*time.Second); state != Waiting {
			t.Errorf("a change was not seen: %q", state)
		}
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if state := c.AwaitChange(ctx, "acme-com", Waiting, 10*time.Second); state != Waiting {
			t.Errorf("a wait whose asker left answered %q", state)
		}
	})
}

// An organization waiting for room is placed the moment a node says it has started, with no tick and no retry due.
func TestAWaitingOrganizationIsPlacedTheMomentANodeSaysItStarted(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		c := controlled(t, s)
		nodes := &lateNodes{local: &LocalNode{}}
		c.Nodes, c.WaitEvery = nodes, time.Hour
		org := made(t, s, "ada@acme.com", "Acme")
		if err := c.Provision(t.Context(), org.Slug); err == nil {
			t.Fatal("an organization with nowhere to go was provisioned")
		}

		ctx, cancel := context.WithCancel(context.Background())
		ran := make(chan struct{})
		go func() { c.Run(ctx, time.Hour); close(ran) }()
		defer func() { cancel(); <-ran }()
		nodes.open.Store(true)
		c.Wake()
		if state := c.AwaitChange(ctx, org.Slug, Waiting, 10*time.Second); state != Provisioning && state != Starting && state != Ready {
			t.Fatalf("a node that said it started did not end the wait: %q", state)
		}
		deadline := time.Now().Add(15 * time.Second)
		for state := c.settled(t, org.Slug); state != Ready && time.Now().Before(deadline); state = c.settled(t, org.Slug) {
			c.AwaitChange(ctx, org.Slug, state, time.Second)
		}
		if _, state := c.Route(ctx, org.Slug); state != Ready {
			t.Errorf("the organization was not placed on the node that started: %q", state)
		}
	})
}
