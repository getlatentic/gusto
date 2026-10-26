package jobs

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/getlatentic/gusto/internal/taskindex"
)

// Mirror is where a task's row is copied to each time its state.json is written. The taskindex.Store is one.
type Mirror interface {
	Upsert(ctx context.Context, row taskindex.Row) error
	Delete(ctx context.Context, id string) error
}

// manyUpserter is a Mirror that can take a run of rows in one transaction.
type manyUpserter interface {
	UpsertMany(ctx context.Context, rows []taskindex.Row) error
}

// MirrorStats counts what the mirror did not write. Both kinds are left for Reconcile to repair.
type MirrorStats struct {
	// Dropped is changes thrown away because the queue to the mirror was full.
	Dropped int64
	// Failed is changes the mirror refused.
	Failed int64
}

const (
	mirrorQueue    = 4096
	mirrorBatch    = 256
	mirrorTimeout  = 30 * time.Second
	failureLogGap  = time.Minute
	spaceCheckWait = 5 * time.Millisecond
)

// op is one change to hand the mirror, or a marker that tells flush every change before it was handled.
type op struct {
	row     taskindex.Row
	id      string
	delete  bool
	barrier chan struct{}
}

// pump hands changes to a Mirror in the order they were queued, from one goroutine, so that no write to a task
// waits for a database. A change that finds the queue full is counted and left for Reconcile.
type pump struct {
	mirror Mirror
	queue  chan op
	done   chan struct{}
	now    func() time.Time

	mu     sync.RWMutex
	closed bool

	dropped, failed atomic.Int64
	unlogged        atomic.Int64
	loggedAt        atomic.Int64
	trouble         chan struct{}
}

func newPump(m Mirror, size int) *pump {
	p := &pump{mirror: m, queue: make(chan op, size), done: make(chan struct{}), now: time.Now, trouble: make(chan struct{}, 1)}
	go p.run()
	return p
}

// offer queues a change without waiting, and says whether there was room.
func (p *pump) offer(o op) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.closed {
		return true
	}
	select {
	case p.queue <- o:
		return true
	default:
		return false
	}
}

// push is offer for a change that must not be lost: a full queue is a drop, counted and logged.
func (p *pump) push(o op) {
	if !p.offer(o) {
		p.lose(1)
	}
}

func (p *pump) lose(n int64) {
	p.dropped.Add(n)
	p.hurt(n, "the task index could not keep up and left changes to be repaired")
}

func (p *pump) fail(n int64, err error) {
	p.failed.Add(n)
	p.hurt(n, "the task index could not be written; the changes are left to be repaired: "+taskindex.Scrub(err))
}

// hurt counts n changes that were not written, says so at most once a minute, and wakes whoever repairs.
func (p *pump) hurt(n int64, what string) {
	p.unlogged.Add(n)
	now := p.now().UnixNano()
	if last := p.loggedAt.Load(); now-last >= int64(failureLogGap) && p.loggedAt.CompareAndSwap(last, now) {
		slog.Warn(what, "changes", p.unlogged.Swap(0), "dropped", p.dropped.Load(), "failed", p.failed.Load())
	}
	select {
	case p.trouble <- struct{}{}:
	default:
	}
}

func (p *pump) stats() MirrorStats {
	return MirrorStats{Dropped: p.dropped.Load(), Failed: p.failed.Load()}
}

// flush waits until every change queued before it has been handled.
func (p *pump) flush(ctx context.Context) error {
	barrier := op{barrier: make(chan struct{})}
	for !p.offer(barrier) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(spaceCheckWait):
		}
	}
	select {
	case <-barrier.barrier:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-p.done:
		return nil
	}
}

// close handles what is queued and stops. A change offered after close is ignored.
func (p *pump) close() {
	p.mu.Lock()
	if !p.closed {
		p.closed = true
		close(p.queue)
	}
	p.mu.Unlock()
	<-p.done
}

func (p *pump) run() {
	defer close(p.done)
	for first := range p.queue {
		batch := []op{first}
	gather:
		for len(batch) < mirrorBatch {
			select {
			case next, open := <-p.queue:
				if !open {
					break gather
				}
				batch = append(batch, next)
			default:
				break gather
			}
		}
		p.handle(batch)
	}
}

// handle applies a batch in order: a run of upserts goes in together, and anything else on its own.
func (p *pump) handle(batch []op) {
	var run []taskindex.Row
	for _, o := range batch {
		if !o.delete && o.barrier == nil {
			run = append(run, o.row)
			continue
		}
		p.upsert(run)
		run = run[:0]
		switch {
		case o.barrier != nil:
			close(o.barrier)
		default:
			p.remove(o.id)
		}
	}
	p.upsert(run)
}

func (p *pump) upsert(rows []taskindex.Row) {
	if len(rows) == 0 {
		return
	}
	if many, batches := p.mirror.(manyUpserter); batches && len(rows) > 1 {
		ctx, cancel := context.WithTimeout(context.Background(), mirrorTimeout)
		err := many.UpsertMany(ctx, rows)
		cancel()
		if err == nil {
			return
		}
		if errors.Is(err, context.DeadlineExceeded) {
			p.fail(int64(len(rows)), err)
			return
		}
	}
	p.upsertEach(rows)
}

// upsertEach writes rows one at a time, so that a row the database refuses does not keep the others out. A
// database that does not answer in time is not asked again for the rest.
func (p *pump) upsertEach(rows []taskindex.Row) {
	for at, row := range rows {
		ctx, cancel := context.WithTimeout(context.Background(), mirrorTimeout)
		err := p.mirror.Upsert(ctx, row)
		cancel()
		if errors.Is(err, context.DeadlineExceeded) {
			p.fail(int64(len(rows)-at), err)
			return
		}
		if err != nil {
			p.fail(1, err)
		}
	}
}

func (p *pump) remove(id string) {
	ctx, cancel := context.WithTimeout(context.Background(), mirrorTimeout)
	defer cancel()
	if err := p.mirror.Delete(ctx, id); err != nil {
		p.fail(1, err)
	}
}

// UseMirror copies every change to a task's record to m from now on, in the order the changes were made. It
// never makes a change wait: a change the mirror cannot take is counted, logged and left for Reconcile.
// Rows that were written before it was set are Reconcile's too. It is meant to be called once, before tasks change.
func (i *Index) UseMirror(m Mirror) { i.useMirror(m, mirrorQueue) }

func (i *Index) useMirror(m Mirror, size int) {
	i.guard.Lock()
	previous := i.pump
	i.pump = newPump(m, size)
	i.guard.Unlock()
	if previous != nil {
		previous.close()
	}
}

// CloseMirror writes the changes still queued to the mirror and stops copying. It waits no longer than ctx allows.
func (i *Index) CloseMirror(ctx context.Context) {
	i.guard.Lock()
	p := i.pump
	i.pump = nil
	i.guard.Unlock()
	if p == nil {
		return
	}
	closed := make(chan struct{})
	go func() { p.close(); close(closed) }()
	select {
	case <-closed:
	case <-ctx.Done():
	}
}

// MirrorStats is what the mirror has not been given or has refused since it was set.
func (i *Index) MirrorStats() MirrorStats {
	i.guard.Lock()
	defer i.guard.Unlock()
	if i.pump == nil {
		return MirrorStats{}
	}
	return i.pump.stats()
}

// MirrorTrouble receives a signal when a change was dropped or refused, and Reconcile then repairs it. It is nil
// while there is no mirror.
func (i *Index) MirrorTrouble() <-chan struct{} {
	i.guard.Lock()
	defer i.guard.Unlock()
	if i.pump == nil {
		return nil
	}
	return i.pump.trouble
}

// mirrored copies a record that has just been written, and tells whoever watches changes. The caller holds the guard, which is
// what keeps the copies in the order of the changes.
func (i *Index) mirrored(record Record) {
	if i.pump != nil {
		i.pump.push(op{row: RowOf(record)})
	}
	for _, watch := range i.changers {
		watch(record)
	}
}
