package jobs

import (
	"os"
	"sync"
	"testing"
	"time"

	"github.com/getlatentic/gusto/pkg/job"
)

// by queues a job asked for by who, for team, and returns it.
func by(t *testing.T, index *Index, who, team, brief string) Record {
	t.Helper()
	record := Record{ID: reserved(t, index, brief), State: Queued, Brief: brief, Harness: "fixture", Asked: who, Team: team, Queued: Now()}
	if err := os.MkdirAll(record.Dir(index.Root()).Path(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := index.Add(record); err != nil {
		t.Fatal(err)
	}
	return record
}

func claimed(t *testing.T, index *Index) string {
	t.Helper()
	record, found, err := index.Claim()
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		return ""
	}
	return record.Brief
}

// One person queues three jobs ahead of two other people, with two slots and a
// limit of one each: everybody gets a turn and nobody is left behind.
func TestOnePersonCannotTakeEverySlot(t *testing.T) {
	index := at(t)
	index.SetLimits(Limits{Person: 1})
	index.setSlots(2)
	by(t, index, "amy", "", "amy 1")
	by(t, index, "amy", "", "amy 2")
	by(t, index, "amy", "", "amy 3")
	by(t, index, "bo", "", "bo 1")
	by(t, index, "cy", "", "cy 1")

	if got := claimed(t, index); got != "amy 1" {
		t.Fatalf("first job taken: %q", got)
	}
	if got := claimed(t, index); got != "bo 1" {
		t.Fatalf("amy's second job was taken over bo's: %q", got)
	}
	if got := claimed(t, index); got != "" {
		t.Fatalf("a third job started with two slots: %q", got)
	}
}

// Three people and one slot: the pool is the only limit, and each waits for it.
func TestThreePeopleAndOneSlotTakeTurnsInOrder(t *testing.T) {
	index := at(t)
	index.setSlots(1)
	first := by(t, index, "amy", "", "amy")
	by(t, index, "bo", "", "bo")
	by(t, index, "cy", "", "cy")
	if claimed(t, index) != "amy" || claimed(t, index) != "" {
		t.Fatal("more than one job ran in one slot")
	}
	if err := index.Finish(first.ID, Passed, 0); err != nil {
		t.Fatal(err)
	}
	if got := claimed(t, index); got != "bo" {
		t.Fatalf("after amy: %q", got)
	}
}

func TestAJobOverItsPersonsLimitIsTakenWhenTheirOtherJobEnds(t *testing.T) {
	index := at(t)
	index.SetLimits(Limits{Person: 1})
	first := by(t, index, "amy", "", "amy 1")
	by(t, index, "amy", "", "amy 2")
	claimed(t, index)
	if got := claimed(t, index); got != "" {
		t.Fatalf("amy ran two at once: %q", got)
	}
	if err := index.Finish(first.ID, Failed, 1); err != nil {
		t.Fatal(err)
	}
	if got := claimed(t, index); got != "amy 2" {
		t.Fatalf("her next job did not start: %q", got)
	}
}

func TestATeamLimitHoldsTheTeamAndNoOneElse(t *testing.T) {
	index := at(t)
	index.SetLimits(Limits{Team: 1})
	by(t, index, "amy", "platform", "platform 1")
	by(t, index, "bo", "platform", "platform 2")
	by(t, index, "cy", "growth", "growth 1")
	by(t, index, "di", "", "nobody's team")
	got := []string{claimed(t, index), claimed(t, index), claimed(t, index), claimed(t, index)}
	want := []string{"platform 1", "growth 1", "nobody's team", ""}
	for n := range want {
		if got[n] != want[n] {
			t.Fatalf("taken %q, want %q", got, want)
		}
	}
}

func TestAJobSaysWhyItIsWaiting(t *testing.T) {
	index := at(t)
	index.SetLimits(Limits{Person: 1, Team: 2})
	index.setSlots(2)
	amy1 := by(t, index, "amy", "platform", "amy 1")
	amy2 := by(t, index, "amy", "platform", "amy 2")
	bo := by(t, index, "bo", "platform", "bo")
	cy := by(t, index, "cy", "platform", "cy")
	dee := by(t, index, "dee", "growth", "dee")
	claimed(t, index) // amy 1
	claimed(t, index) // bo

	for name, want := range map[string]Wait{
		amy2.ID: {Reason: PersonAtLimit, Position: 1, Running: 1, Limit: 1, Who: "amy"},
		cy.ID:   {Reason: TeamAtLimit, Position: 2, Running: 2, Limit: 2, Who: "platform"},
		dee.ID:  {Reason: PoolFull, Position: 3, Running: 2, Limit: 2},
	} {
		got, queued := index.WaitOf(name)
		if !queued || got != want {
			t.Errorf("%s: %+v, want %+v", name, got, want)
		}
	}
	if _, queued := index.WaitOf(amy1.ID); queued {
		t.Error("a running job was said to be waiting")
	}
	if _, queued := index.WaitOf(bo.ID); queued {
		t.Error("a running job was said to be waiting")
	}
	waiting := index.Waiting()
	if len(waiting) != 3 || waiting[0].Record.ID != amy2.ID || waiting[2].Record.ID != dee.ID || waiting[1].Wait.Position != 2 {
		t.Fatalf("waiting %+v", waiting)
	}
}

func TestATurnWaitsForItsAgentAndSaysSo(t *testing.T) {
	index := at(t)
	first := turnQueued(t, index, "agent-1", "a", "first message")
	second := turnQueued(t, index, "agent-1", "b", "second message")
	claimed(t, index)
	got, queued := index.WaitOf(second.ID)
	if !queued || got.Reason != AgentBusy {
		t.Fatalf("%+v", got)
	}
	_ = first
}

func TestNoLimitMeansNoLimit(t *testing.T) {
	index := at(t)
	for _, brief := range []string{"a", "b", "c", "d"} {
		by(t, index, "amy", "platform", brief)
	}
	for range 4 {
		if claimed(t, index) == "" {
			t.Fatal("a job waited with no limits set")
		}
	}
}

// With four workers and a limit of one, a person's jobs still never overlap,
// and somebody else's job runs beside them rather than behind them.
func TestTheQueueNeverRunsAPersonsJobsAtOnce(t *testing.T) {
	index := at(t)
	index.SetLimits(Limits{Person: 1})
	jobs := map[string]string{}
	for _, brief := range []string{"amy 1", "amy 2", "amy 3"} {
		jobs[by(t, index, "amy", "", brief).Dir(index.Root()).Path()] = "amy"
	}
	jobs[by(t, index, "bo", "", "bo 1").Dir(index.Root()).Path()] = "bo"

	var mu sync.Mutex
	running := map[string]int{}
	widest := map[string]int{}
	amyDone := 0
	// overlapped says bo started before amy's jobs were all done: bo did not wait behind them.
	overlapped := false
	finished := make(chan struct{}, 4)
	// amy's first job holds until bo's has started, so bo starts while amy runs however slowly a worker wakes; a queue that
	// made bo wait behind amy would leave both waiting until the hold gives up.
	boStarted := make(chan struct{})
	queue := Start(index, func(dir job.Dir) (int, error) {
		who := jobs[dir.Path()]
		mu.Lock()
		running[who]++
		widest[who] = max(widest[who], running[who])
		first := who == "amy" && widest["amy"] == 1 && !overlapped
		if who == "bo" {
			overlapped = amyDone < 3
			close(boStarted)
		}
		mu.Unlock()
		if first {
			select {
			case <-boStarted:
			case <-time.After(5 * time.Second):
			}
		}
		time.Sleep(20 * time.Millisecond)
		mu.Lock()
		running[who]--
		if who == "amy" {
			amyDone++
		}
		mu.Unlock()
		finished <- struct{}{}
		return 0, nil
	}, 4)
	defer queue.Stop()
	for range 4 {
		select {
		case <-finished:
		case <-time.After(10 * time.Second):
			t.Fatal("the queue did not finish")
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if widest["amy"] != 1 {
		t.Errorf("amy's jobs overlapped: %d at once", widest["amy"])
	}
	if !overlapped {
		t.Error("bo waited behind amy's jobs with workers free")
	}
}
