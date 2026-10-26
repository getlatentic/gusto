package roomlive

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/centrifugal/centrifuge"

	"github.com/getlatentic/gusto/internal/roomlog"
)

// A feed is the goroutine that owns one room's live stream on this node. It runs while the room has a viewer. When the store
// wakes it, it reads the events after the last it handed on and hands them on in order, so the order in which wake-ups arrive
// does not matter: whatever woke it, it reads the log.

// batch is how many events a feed reads from the log at a time.
const batch = 128

type feed struct {
	broker *Broker
	room   string
	// wake holds at most one wake-up, which is all that is needed: a wake-up means look at the log.
	wake chan struct{}
	stop chan struct{}
	once sync.Once

	// last and epoch are the feed goroutine's own: the newest event handed on and the epoch it was in.
	last  uint64
	epoch string
}

// follow starts the feed of a room, from where its log stands now. A viewer that subscribes is brought up to date by replay
// from the position it holds, so the feed only has to continue from here.
func (b *Broker) follow(room string) error {
	head, err := b.log.RoomHead(room)
	if err != nil {
		return err
	}
	each := &feed{broker: b, room: room, wake: make(chan struct{}, 1), stop: make(chan struct{}), last: head.Seq, epoch: head.Epoch}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return errors.New("the live layer is shutting down")
	}
	if _, running := b.feeds[room]; running {
		b.mu.Unlock()
		return nil
	}
	b.feeds[room] = each
	b.wg.Add(1)
	b.mu.Unlock()
	go each.run()
	// An event may have been logged between reading the head and the feed being findable, and its wake-up found nobody.
	each.poke()
	return nil
}

func (b *Broker) unfollow(room string) {
	b.mu.Lock()
	each := b.feeds[room]
	delete(b.feeds, room)
	b.mu.Unlock()
	if each != nil {
		each.stopNow()
	}
}

func (f *feed) poke() {
	select {
	case f.wake <- struct{}{}:
	default:
	}
}

func (f *feed) stopNow() { f.once.Do(func() { close(f.stop) }) }

func (f *feed) stopped() bool {
	select {
	case <-f.stop:
		return true
	default:
		return false
	}
}

func (f *feed) run() {
	defer f.broker.wg.Done()
	for {
		select {
		case <-f.stop:
			return
		case <-f.wake:
			if !f.drain() {
				f.broker.unfollowIf(f)
				return
			}
		}
	}
}

// drain hands on every event after the last one, and says whether the feed can go on. It cannot when the room is gone or its
// log started again: the viewers are then sent away to come back with a position that is true.
func (f *feed) drain() bool {
	for !f.stopped() {
		slice, err := f.broker.log.RoomEvents(f.room, f.last, batch)
		if err != nil {
			f.broker.gone(f.room)
			return false
		}
		if slice.Head.Epoch != f.epoch || slice.Gap {
			f.broker.gone(f.room)
			return false
		}
		if len(slice.Events) == 0 {
			return true
		}
		f.handOn(slice.Events)
	}
	return true
}

func (f *feed) handOn(events []roomlog.Event) {
	b := f.broker
	channel := StreamChannel(f.room)
	for _, event := range events {
		if f.stopped() {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), b.drawTimeout)
		pub := &centrifuge.Publication{Offset: event.Seq, Data: b.frame(ctx, f.room, event), Time: event.At * 1000}
		cancel()
		position := centrifuge.StreamPosition{Offset: event.Seq, Epoch: f.epoch}
		// An event the node refuses is not tried again, which would stop every later one behind it; the viewers' position
		// check finds the gap and brings them back by replay.
		if err := b.handler.HandlePublication(channel, pub, position, false, nil); err != nil {
			slog.Warn("a room's event was not handed to its viewers", "room", f.room, "seq", event.Seq, "error", err)
		} else {
			b.published.Add(1)
		}
		f.last = event.Seq
		if event.Kind == kindPeople && b.people != nil {
			b.people(f.room)
		}
	}
}

// unfollowIf removes a feed that ended on its own, unless a newer one has taken its place.
func (b *Broker) unfollowIf(ended *feed) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.feeds[ended.room] == ended {
		delete(b.feeds, ended.room)
	}
}

func (b *Broker) gone(room string) {
	if b.lost != nil {
		b.lost(room)
	}
}
