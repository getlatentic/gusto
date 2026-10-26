package jobs

import (
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// agentTurn is a turn of one of a room's agents.
func agentTurn(t *testing.T, index *Index, room, agent, brief string) Record {
	t.Helper()
	record := Record{
		ID: reserved(t, index, brief), State: Queued, Brief: brief, Harness: "fixture", Asked: "a-test", Queued: Now(),
		Instance: room, Thread: "1", Agent: agent,
	}
	if err := os.MkdirAll(record.Dir(index.Root()).Path(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := index.Add(record); err != nil {
		t.Fatal(err)
	}
	return record
}

// A room's agents run beside each other, each on a copy of the room's folder.
func TestTwoAgentsOfOneRoomRunAtTheSameTime(t *testing.T) {
	index := at(t)
	agentTurn(t, index, "room-launch", "a1", "scout, do this")
	agentTurn(t, index, "room-launch", "a2", "draft, do this")
	for turn := range 2 {
		if _, found, err := index.Claim(); err != nil || !found {
			t.Fatalf("turn %d of the room was not claimed: %v %v", turn, found, err)
		}
	}
}

// One agent has one conversation and one memory, so its own turns go one at a time, in the order they were asked.
func TestOneAgentsTurnsWaitForEachOtherWhileTheRoomsOtherAgentRuns(t *testing.T) {
	index := at(t)
	first := agentTurn(t, index, "room-launch", "a1", "first")
	second := agentTurn(t, index, "room-launch", "a1", "second")
	other := agentTurn(t, index, "room-launch", "a2", "other")

	claimed, _, _ := index.Claim()
	if claimed.ID != first.ID {
		t.Fatalf("the queue took %s before %s", claimed.ID, first.ID)
	}
	next, found, _ := index.Claim()
	if !found || next.ID != other.ID {
		t.Fatalf("the other agent's turn waited behind %s: %+v %v", second.ID, next, found)
	}
	if _, found, _ := index.Claim(); found {
		t.Fatal("a second turn of one agent was claimed while its first ran")
	}
	wait, queued := index.WaitOf(second.ID)
	if !queued || wait.Reason != AgentBusy {
		t.Errorf("the waiting turn says %+v", wait)
	}

	if err := index.Finish(first.ID, Passed, 0); err != nil {
		t.Fatal(err)
	}
	if again, found, _ := index.Claim(); !found || again.ID != second.ID {
		t.Fatalf("the agent's next turn was not taken: %+v %v", again, found)
	}
}

func TestTheSameAgentIdInTwoRoomsIsTwoAgents(t *testing.T) {
	index := at(t)
	agentTurn(t, index, "room-one", "a1", "one")
	agentTurn(t, index, "room-two", "a1", "two")
	for turn := range 2 {
		if _, found, _ := index.Claim(); !found {
			t.Fatalf("turn %d waited for an agent of another room", turn)
		}
	}
}

// A conversation that is not a room's has one workspace, so the instance is still the lane.
func TestATurnWithNoAgentStillWaitsForItsInstance(t *testing.T) {
	index := at(t)
	turnQueued(t, index, "20260926-1200-work", "1", "one")
	turnQueued(t, index, "20260926-1200-work", "2", "two")
	if _, found, _ := index.Claim(); !found {
		t.Fatal("the first turn was not claimed")
	}
	if _, found, _ := index.Claim(); found {
		t.Fatal("two turns of one conversation ran at once")
	}
}

// Under the race detector: many workers claim, run and finish a room's turns, and no agent is ever on two at once.
func TestWorkersNeverPutOneAgentOnTwoTurns(t *testing.T) {
	index := at(t)
	const agents, turns = 4, 12
	for turn := range turns {
		for agent := range agents {
			agentTurn(t, index, "room-busy", string(rune('a'+agent)), string(rune('a'+agent))+string(rune('a'+turn)))
		}
	}
	var guard sync.Mutex
	var finished atomic.Int32
	on := map[string]bool{}
	var done sync.WaitGroup
	for range 8 {
		done.Add(1)
		go func() {
			defer done.Done()
			for {
				record, found, err := index.Claim()
				if err != nil {
					t.Error(err)
					return
				}
				if !found {
					if finished.Load() == agents*turns {
						return
					}
					runtime.Gosched()
					continue
				}
				guard.Lock()
				if on[record.Agent] {
					t.Errorf("agent %s was on two turns at once", record.Agent)
				}
				on[record.Agent] = true
				guard.Unlock()
				time.Sleep(200 * time.Microsecond)
				guard.Lock()
				on[record.Agent] = false
				guard.Unlock()
				if err := index.Finish(record.ID, Passed, 0); err != nil {
					t.Error(err)
				}
				finished.Add(1)
			}
		}()
	}
	done.Wait()
}
