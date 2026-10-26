package web

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getlatentic/gusto/pkg/job"
)

// gatedWork answers every turn, holding each until it is let go, and keeps who was inside at once.
type gatedWork struct {
	mu      sync.Mutex
	inside  map[string]int
	entered chan string
	gate    chan struct{}
	once    sync.Once
	peak    map[string]int
	started []time.Time
}

func newGatedWork() *gatedWork {
	return &gatedWork{inside: map[string]int{}, peak: map[string]int{}, entered: make(chan string, 64), gate: make(chan struct{})}
}

func (g *gatedWork) do(ctx context.Context, dir job.Dir) (int, error) {
	text, err := os.ReadFile(dir.Spec())
	if err != nil {
		return 1, err
	}
	var spec job.Spec
	if err := json.Unmarshal(text, &spec); err != nil {
		return 1, err
	}
	g.mu.Lock()
	g.inside[spec.Agent]++
	g.peak[spec.Agent] = max(g.peak[spec.Agent], g.inside[spec.Agent])
	g.started = append(g.started, time.Now())
	g.mu.Unlock()
	g.entered <- spec.Agent
	select {
	case <-g.gate:
	case <-ctx.Done():
	case <-time.After(10 * time.Second):
	}
	g.mu.Lock()
	g.inside[spec.Agent]--
	g.mu.Unlock()
	if err := os.MkdirAll(dir.Out(), 0o755); err != nil {
		return 1, err
	}
	return 0, os.WriteFile(dir.OutFile("transcript.log"), []byte("done\n"), 0o644)
}

func (g *gatedWork) waitEntered(t *testing.T, n int) []string {
	t.Helper()
	var who []string
	for len(who) < n {
		select {
		case agent := <-g.entered:
			who = append(who, agent)
		case <-time.After(5 * time.Second):
			t.Fatalf("%d turns started, not %d", len(who), n)
		}
	}
	return who
}

func (g *gatedWork) release() { g.once.Do(func() { close(g.gate) }) }

func withWorkers(t *testing.T, n int, work *gatedWork) {
	t.Helper()
	testWorkers, testWork = n, work.do
	// A test that fails with turns still held would wait on them forever when the queue stops.
	t.Cleanup(work.release)
	t.Cleanup(func() { testWorkers, testWork = 0, nil })
}

func waitIdle(t *testing.T, site *site) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if site.allDone() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the queue did not empty")
}

func TestTwoAgentsMentionedTogetherRunAtTheSameTime(t *testing.T) {
	work := newGatedWork()
	withWorkers(t, 2, work)
	site, _, thread := aRoom(t, &roomWork{}, [2]string{"Scout", "fixture"}, [2]string{"Draft", "other"})
	site.as(t, alice).ask(t, "POST", thread+"/say", url.Values{"said": {"@Scout and @Draft, go"}})

	who := work.waitEntered(t, 2)
	if (who[0] == who[1]) || len(who) != 2 {
		t.Fatalf("the agents inside at once were %v", who)
	}
	page := site.as(t, alice).ask(t, "GET", thread, nil).Body.String()
	if strings.Count(page, "Working") != 2 || strings.Contains(page, "Waiting for") {
		t.Errorf("two running turns read %d Working and waiting text %v", strings.Count(page, "Working"), strings.Contains(page, "Waiting for"))
	}
	work.release()
	waitIdle(t, site)
}

func TestOneAgentAskedTwiceGoesOneMessageAtATimeWhileAnotherRuns(t *testing.T) {
	work := newGatedWork()
	withWorkers(t, 3, work)
	site, _, thread := aRoom(t, &roomWork{}, [2]string{"Scout", "fixture"}, [2]string{"Draft", "other"})
	mine := site.as(t, alice)
	mine.ask(t, "POST", thread+"/say", url.Values{"said": {"@Scout first"}})
	mine.ask(t, "POST", thread+"/say", url.Values{"said": {"@Scout second"}})
	mine.ask(t, "POST", thread+"/say", url.Values{"said": {"@Draft third"}})

	if who := work.waitEntered(t, 2); (who[0] == who[1]) || len(who) != 2 {
		t.Fatalf("started %v: the second message to Scout started beside the first", who)
	}
	select {
	case again := <-work.entered:
		t.Fatalf("%s started while its agent was still on a message", again)
	case <-time.After(150 * time.Millisecond):
	}
	page := mine.ask(t, "GET", thread, nil).Body.String()
	if !strings.Contains(page, "Waiting for Scout to finish the message before this one.") {
		t.Errorf("the second message to Scout does not say what it waits for:\n%s", page)
	}
	if strings.Count(page, "Working") != 2 {
		t.Errorf("%d turns read Working, and two are working", strings.Count(page, "Working"))
	}
	work.release()
	work.waitEntered(t, 1)
	waitIdle(t, site)
	if work.peak["a1"] != 1 || work.peak["a2"] != 1 {
		t.Errorf("most turns at once: %v", work.peak)
	}
}

func TestATurnWaitingForARunnerSaysItIsWaitingAndForWhat(t *testing.T) {
	work := newGatedWork()
	withWorkers(t, 1, work)
	site, _, thread := aRoom(t, &roomWork{}, [2]string{"Scout", "fixture"}, [2]string{"Draft", "other"})
	mine := site.as(t, alice)
	mine.ask(t, "POST", thread+"/say", url.Values{"said": {"@Scout and @Draft go"}})
	work.waitEntered(t, 1)

	page := mine.ask(t, "GET", thread, nil).Body.String()
	if !strings.Contains(page, "Every runner is busy: 1 of 1") || strings.Count(page, "Working") != 1 {
		t.Errorf("a turn with no runner free reads %d Working:\n%s", strings.Count(page, "Working"), page)
	}
	_, before := site.server.roomActivity(site.server.instances.Latest(strings.Split(strings.TrimPrefix(thread, "/instances/"), "/")[0]))
	work.release()
	waitIdle(t, site)
	_, after := site.server.roomActivity(site.server.instances.Latest(strings.Split(strings.TrimPrefix(thread, "/instances/"), "/")[0]))
	if before == after {
		t.Error("the room's version did not change when the waiting turn started")
	}
	if page := mine.ask(t, "GET", thread, nil).Body.String(); strings.Contains(page, "Every runner is busy") {
		t.Error("a turn that has finished still says it waits")
	}
}

// slowly is work whose turns last long enough to overlap with the other agent's.
func slowly(work *roomWork) func(context.Context, job.Dir) (int, error) {
	return func(ctx context.Context, dir job.Dir) (int, error) {
		time.Sleep(20 * time.Millisecond)
		return work.do(ctx, dir)
	}
}

func TestHandOffsBetweenAgentsRunningAtOnceStopAfterThreeHops(t *testing.T) {
	work := &roomWork{replies: map[string]string{"a1": "@Draft take it", "a2": "@Scout take it"}}
	testWorkers, testWork = 2, slowly(work)
	t.Cleanup(func() { testWorkers = 0 })
	site, _, thread := aRoom(t, work, [2]string{"Scout", "fixture"}, [2]string{"Draft", "other"})
	site.as(t, alice).ask(t, "POST", thread+"/say", url.Values{"said": {"@Scout and @Draft start"}})
	work.waitTurns(t, site, 2*(1+handOffs))
	time.Sleep(150 * time.Millisecond)
	if got := len(work.turns()); got != 2*(1+handOffs) {
		t.Fatalf("two chains of %d turns each made %d turns", 1+handOffs, got)
	}
	for _, record := range site.index.List() {
		if record.Hops > handOffs {
			t.Errorf("a turn is %d hops in", record.Hops)
		}
	}
}
