package roomlive

import (
	"container/list"
	"context"
	"sync"
	"time"
)

// dedupe makes a command sent twice be carried out once. A browser that does not hear its command answered sends it again
// under the same id after it reconnects; the second send finds the first's result, or waits for it if it is still being carried
// out. Only commands that succeeded are kept, so a failure can be tried again. The results are bounded in number and in age.
type dedupe struct {
	mu    sync.Mutex
	limit int
	ttl   time.Duration
	byKey map[string]*call
	order *list.List
}

type call struct {
	key    string
	done   chan struct{}
	result Result
	at     time.Time
	place  *list.Element
}

func newDedupe(limit int, ttl time.Duration) *dedupe {
	return &dedupe{limit: limit, ttl: ttl, byKey: map[string]*call{}, order: list.New()}
}

// do runs run unless the same key has already succeeded or is running. It says whether it was a repeat.
func (d *dedupe) do(ctx context.Context, key string, run func() Result) (Result, bool) {
	d.mu.Lock()
	if earlier, found := d.byKey[key]; found {
		d.mu.Unlock()
		return d.await(ctx, earlier)
	}
	mine := &call{key: key, done: make(chan struct{}), at: time.Now()}
	d.byKey[key] = mine
	mine.place = d.order.PushFront(mine)
	d.trim()
	d.mu.Unlock()

	mine.result = run()
	d.mu.Lock()
	if !mine.result.OK {
		d.forget(mine)
	}
	d.mu.Unlock()
	close(mine.done)
	return mine.result, false
}

// await gives the result of a call that is running or has run.
func (d *dedupe) await(ctx context.Context, earlier *call) (Result, bool) {
	select {
	case <-earlier.done:
	case <-ctx.Done():
		return Result{Error: "the earlier send is still being carried out"}, true
	}
	result := earlier.result
	result.Dupe = result.OK
	return result, true
}

// forget drops a call. The lock is held.
func (d *dedupe) forget(gone *call) {
	if d.byKey[gone.key] == gone {
		delete(d.byKey, gone.key)
		d.order.Remove(gone.place)
	}
}

// trim drops the oldest finished calls beyond the bound or past their age. The lock is held. A call still running is kept,
// since a second send must find it.
func (d *dedupe) trim() {
	for oldest := d.order.Back(); oldest != nil; oldest = d.order.Back() {
		each := oldest.Value.(*call)
		keep := d.order.Len() <= d.limit && time.Since(each.at) <= d.ttl
		if keep || !isDone(each.done) {
			return
		}
		d.forget(each)
	}
}

func isDone(done chan struct{}) bool {
	select {
	case <-done:
		return true
	default:
		return false
	}
}
