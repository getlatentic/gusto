package usage

import (
	"context"
	"log/slog"
	"regexp"
	"sync"
	"sync/atomic"
	"time"

	"github.com/getlatentic/gusto/internal/spend"
)

const (
	defaultQueue    = 8192
	flushEvery      = 250 * time.Millisecond
	firstRetry      = 250 * time.Millisecond
	longestRetry    = 10 * time.Second
	writeTimeout    = 30 * time.Second
	closingAttempts = 3
	dropLogEvery    = time.Minute
)

type pending struct {
	task string
	line int
	call spend.Call
}

// Recorder writes the proxy's calls into the index as they happen, without ever making the proxy wait. A call
// is queued and one goroutine writes the queue in batches. When the queue is full the call is not written here:
// it is counted, logged, and left for a repair to read from the task's file, which holds every call.
type Recorder struct {
	store    Store
	who      WhoFunc
	provider ProviderFunc

	batch int
	every time.Duration
	retry time.Duration
	now   func() time.Time

	mu     sync.RWMutex
	closed bool
	queue  chan pending
	stop   chan struct{}
	done   chan struct{}

	dropped  atomic.Int64
	unlogged atomic.Int64
	loggedAt atomic.Int64
	drops    chan struct{}

	lostMu sync.Mutex
	lost   map[string]struct{}
}

// NewRecorder starts a recorder that writes to store. Close it to flush what is queued.
func NewRecorder(store Store, who WhoFunc, provider ProviderFunc) *Recorder {
	return newRecorder(store, who, provider, defaultQueue, firstRetry)
}

func newRecorder(store Store, who WhoFunc, provider ProviderFunc, queue int, retry time.Duration) *Recorder {
	r := &Recorder{
		store: store, who: who, provider: provider,
		batch: batchSize, every: flushEvery, retry: retry, now: time.Now,
		queue: make(chan pending, queue),
		stop:  make(chan struct{}), done: make(chan struct{}),
		drops: make(chan struct{}, 1),
		lost:  map[string]struct{}{},
	}
	go r.run()
	return r
}

// Record queues the call that was written as the given line of the task's proxy record. It does not wait, and
// a call that does not go to the model is not an index row.
func (r *Recorder) Record(task string, line int, call spend.Call) {
	if r == nil || !call.ToModel() {
		return
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		return
	}
	select {
	case r.queue <- pending{task, line, call}:
	default:
		r.drop(1, task)
	}
}

// Dropped receives a signal when calls were left unwritten, and TakeLost then names their tasks.
func (r *Recorder) Dropped() <-chan struct{} { return r.drops }

// DroppedCount is how many calls were not written, for a health page.
func (r *Recorder) DroppedCount() int64 { return r.dropped.Load() }

// TakeLost is the tasks that have calls the recorder did not write, and forgets them.
func (r *Recorder) TakeLost() []string {
	r.lostMu.Lock()
	defer r.lostMu.Unlock()
	tasks := make([]string, 0, len(r.lost))
	for task := range r.lost {
		tasks = append(tasks, task)
	}
	clear(r.lost)
	return tasks
}

func (r *Recorder) markLost(tasks ...string) {
	r.lostMu.Lock()
	for _, task := range tasks {
		r.lost[task] = struct{}{}
	}
	r.lostMu.Unlock()
	select {
	case r.drops <- struct{}{}:
	default:
	}
}

func (r *Recorder) drop(count int64, tasks ...string) {
	r.dropped.Add(count)
	r.unlogged.Add(count)
	r.markLost(tasks...)
	r.logDrops(false)
}

// logDrops says how many calls were dropped, at most once a minute, or at once when final.
func (r *Recorder) logDrops(final bool) {
	now := r.now().UnixNano()
	last := r.loggedAt.Load()
	if !final && now-last < int64(dropLogEvery) {
		return
	}
	if !r.loggedAt.CompareAndSwap(last, now) {
		return
	}
	if count := r.unlogged.Swap(0); count > 0 {
		slog.Warn("the usage index could not keep up and left calls to be read from the tasks' records", "calls", count, "total", r.dropped.Load())
	}
}

// Close writes what is queued and stops. A call recorded after Close is ignored.
func (r *Recorder) Close() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		<-r.done
		return nil
	}
	r.closed = true
	close(r.stop)
	close(r.queue)
	r.mu.Unlock()
	<-r.done
	r.logDrops(true)
	return nil
}

func (r *Recorder) run() {
	defer close(r.done)
	tick := time.NewTicker(r.every)
	defer tick.Stop()
	var held []pending
	for {
		select {
		case next, open := <-r.queue:
			if !open {
				r.flush(held)
				return
			}
			held = append(held, next)
			if len(held) >= r.batch {
				r.flush(held)
				held = held[:0]
			}
		case <-tick.C:
			r.flush(held)
			held = held[:0]
		}
	}
}

func (r *Recorder) flush(held []pending) {
	if len(held) == 0 {
		return
	}
	rows := make([]Row, len(held))
	now := r.now()
	who := map[string]Who{}
	for i, p := range held {
		known, seen := who[p.task]
		if !seen {
			known = whoOf(r.who, p.task)
			who[p.task] = known
		}
		rows[i] = rowOf(p.task, p.line, p.call, known, r.provider, now)
	}
	if r.write(rows) {
		return
	}
	tasks := make([]string, 0, len(who))
	for task := range who {
		tasks = append(tasks, task)
	}
	r.drop(int64(len(rows)), tasks...)
}

// write retries a failed write with a growing wait, for as long as the recorder runs, and a few times more once
// it is closing. It says whether the rows were written.
func (r *Recorder) write(rows []Row) bool {
	wait := r.retry
	for attempt := 1; ; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
		err := r.store.Add(ctx, rows)
		cancel()
		if err == nil {
			return true
		}
		slog.Error("the usage index could not be written", "rows", len(rows), "attempt", attempt, "error", scrub(err))
		closing := r.isClosing()
		if closing && attempt >= closingAttempts {
			return false
		}
		time.Sleep(wait)
		if !closing {
			wait = min(wait*2, longestRetry)
		}
	}
}

func (r *Recorder) isClosing() bool {
	select {
	case <-r.stop:
		return true
	default:
		return false
	}
}

var secrets = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(://[^:/@\s]*:)[^@\s]+@`),
	regexp.MustCompile(`(?i)(password\s*=\s*)('[^']*'|\S+)`),
}

// scrub is an error's text with any password in a database URL or connection string taken out.
func scrub(err error) string {
	text := err.Error()
	text = secrets[0].ReplaceAllString(text, "${1}***@")
	return secrets[1].ReplaceAllString(text, "${1}***")
}

func logIndexError(what string, err error) { slog.Error(what, "error", scrub(err)) }
