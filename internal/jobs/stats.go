package jobs

import "time"

// Stats is what the queue can be watched by: how many tasks are in each state,
// how long the oldest has waited, and whether anything held back is still
// waiting for nothing.
type Stats struct {
	// ByState is every task the index holds.
	ByState map[State]int
	// OldestWaitSeconds is how long the task that has been queued longest has waited.
	OldestWaitSeconds int64
	// StuckSeconds is how long the longest-waiting task has waited that nothing
	// holds back: no limit, no busy agent, and a worker free. It is near zero
	// while the workers are alive, and grows when they are not.
	StuckSeconds int64
	// Finished counts tasks that ended since this server started, by state.
	Finished map[State]int64
	// LastStarted and LastFinished are when a task last started and last ended,
	// in unix seconds, or zero when none has since the server started.
	LastStarted, LastFinished int64
	Slots                     int
}

// Stats reads the queue as it stands.
func (i *Index) Stats(now time.Time) Stats {
	i.guard.Lock()
	defer i.guard.Unlock()
	stats := Stats{ByState: map[State]int{}, Finished: map[State]int64{}, LastStarted: i.lastStarted, LastFinished: i.lastFinished, Slots: i.limits.Slots}
	for state, count := range i.finished {
		stats.Finished[state] = count
	}
	running := i.running()
	for _, record := range i.records {
		stats.ByState[record.State]++
		if record.State != Queued {
			continue
		}
		waited := now.Unix() - record.Queued
		stats.OldestWaitSeconds = max(stats.OldestWaitSeconds, waited)
		if _, held := i.hold(record, running); !held {
			stats.StuckSeconds = max(stats.StuckSeconds, waited)
		}
	}
	return stats
}
