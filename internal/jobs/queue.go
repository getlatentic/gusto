package jobs

import (
	"log/slog"
	"sync"
	"time"

	"github.com/getlatentic/haas/internal/job"
)

// Work is what a worker does with a claimed job. It is the runner, injected, so
// the queue can be tested without a container and the server can choose between
// running a job here or in a box.
type Work func(job.Dir) (int, error)

// Queue runs a fixed number of jobs at once. The number comes from what the
// machine can hold, which is memory a job at a time rather than cores.
type Queue struct {
	index   *Index
	work    Work
	wake    chan struct{}
	stop    chan struct{}
	stopped sync.Once
	workers sync.WaitGroup
}

func Start(index *Index, work Work, workers int) *Queue {
	if workers < 1 {
		workers = 1
	}
	index.setSlots(workers)
	queue := &Queue{
		index: index,
		work:  work,
		wake:  make(chan struct{}, workers),
		stop:  make(chan struct{}),
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

func (q *Queue) serve() {
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
	exit, err := q.work(record.Dir(q.index.Root()))
	state := Passed
	switch {
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
