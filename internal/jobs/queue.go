package jobs

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/getlatentic/gusto/internal/job"
)

// Work is what a worker does with a claimed job. It is the runner, injected, so
// the queue can be tested without a container and the server can choose between
// running a job here or in a box.
type Work func(job.Dir) (int, error)

// StoppableWork is Work that is told when to stop. The context ends when somebody stops the job, and the work then ends the
// job as it would at the end of its budget: what it did is kept, and the record says it did not finish.
type StoppableWork func(context.Context, job.Dir) (int, error)

// Queue runs a fixed number of jobs at once. The number comes from what the
// machine can hold, which is memory a job at a time rather than cores.
type Queue struct {
	index *Index
	work  StoppableWork
	// running holds what ends each job that is running now.
	running map[string]context.CancelFunc
	guard   sync.Mutex
	wake    chan struct{}
	stop    chan struct{}
	stopped sync.Once
	workers sync.WaitGroup
	alive   atomic.Int32
	slots   int
}

func Start(index *Index, work Work, workers int) *Queue {
	return StartStoppable(index, func(_ context.Context, dir job.Dir) (int, error) { return work(dir) }, workers)
}

// StartStoppable is Start for work that ends when it is told to.
func StartStoppable(index *Index, work StoppableWork, workers int) *Queue {
	if workers < 1 {
		workers = 1
	}
	index.setSlots(workers)
	queue := &Queue{
		slots:   workers,
		index:   index,
		work:    work,
		running: map[string]context.CancelFunc{},
		wake:    make(chan struct{}, workers),
		stop:    make(chan struct{}),
	}
	queue.workers.Add(workers)
	for worker := 0; worker < workers; worker++ {
		go queue.serve()
	}
	queue.Wake()
	return queue
}

// Wake tells a worker there may be something to take. A full channel already
// says so, so this never blocks the caller that just queued a job.
func (q *Queue) Wake() {
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

func (q *Queue) Stop() {
	q.stopped.Do(func() { close(q.stop) })
	q.workers.Wait()
}

// Alive is how many workers are running, and Slots how many there should be.
// A worker that is gone leaves a queue that takes jobs and never starts them.
func (q *Queue) Alive() int { return int(q.alive.Load()) }

func (q *Queue) Slots() int { return q.slots }

func (q *Queue) serve() {
	q.alive.Add(1)
	defer q.alive.Add(-1)
	defer q.workers.Done()
	// The tick is a backstop for a wake that was missed. It does not find work
	// nobody told this server about: the index is read from disk once, at Open.
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	for {
		for q.one() {
			// Stopping must not start another job. The one in flight finishes,
			// because it is in the middle of writing the evidence that says what
			// it did, and systemd gives it that time.
			select {
			case <-q.stop:
				return
			default:
			}
		}
		select {
		case <-q.stop:
			return
		case <-q.wake:
		case <-tick.C:
		}
	}
}

// one takes a job if there is one, and says whether to look again.
func (q *Queue) one() bool {
	record, found, err := q.index.Claim()
	if err != nil {
		slog.Error("could not claim a job", "error", err)
		return false
	}
	if !found {
		return false
	}
	ctx, cancel := context.WithCancel(context.Background())
	q.guard.Lock()
	q.running[record.ID] = cancel
	q.guard.Unlock()
	// A stop that came between the job being claimed and its work being given a way to hear it is honoured now.
	if now, found := q.index.Get(record.ID); found && now.StoppedBy != "" {
		cancel()
	}
	exit, err := q.work(ctx, record.Dir(q.index.Root()))
	q.guard.Lock()
	delete(q.running, record.ID)
	q.guard.Unlock()
	cancel()
	state := Passed
	stopped := false
	if now, found := q.index.Get(record.ID); found && now.StoppedBy != "" {
		stopped = true
	}
	switch {
	case stopped:
		// Whoever stopped it ended it: a nonzero exit or an error is what stopping does, and is not the job failing.
		state = Interrupted
		if err != nil {
			slog.Warn("a stopped job ended with an error", "job", record.ID, "error", err)
		}
		exit = -1
	case err != nil:
		slog.Error("the job could not run", "job", record.ID, "error", err)
		state = Failed
		exit = -1
	case exit != 0:
		state = Failed
	}
	if err := q.index.Finish(record.ID, state, exit); err != nil {
		slog.Error("could not record how a job ended", "job", record.ID, "error", err)
	}
	// A job that ends can free a person's or a team's limit for a job another
	// worker is asleep on.
	q.Wake()
	return true
}

// Cancel stops a job for somebody: a queued job never runs, and a running one is told to stop and ends interrupted. A job
// that is over is not stoppable. It says whether the job was running.
func (q *Queue) Cancel(id, by string) (Record, bool, error) {
	record, wasRunning, err := q.index.Stop(id, by)
	if err != nil {
		return record, false, err
	}
	if wasRunning {
		q.guard.Lock()
		cancel := q.running[id]
		q.guard.Unlock()
		if cancel != nil {
			cancel()
		}
	}
	return record, wasRunning, nil
}
