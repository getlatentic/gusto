package roomlive

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/centrifugal/centrifuge"

	"github.com/getlatentic/gusto/internal/roomlog"
)

// Broker is Centrifuge's Broker over the rooms' event logs. A stream's publications are the log's events, its offsets are the
// log's sequence numbers and its epoch is the log's, so the history that recovers a client is the log itself and survives a
// restart. Nothing is published into a stream: the store appends to the log, wakes the room's feed, and the feed reads what is
// new and hands it to the node in order. The live channel has no history; what is published to it goes straight to its viewers.
type Broker struct {
	log       Log
	renderer  Renderer
	cache     *frameCache
	maxReplay int
	// replayBytes is the most a replay may weigh.
	replayBytes int
	// drawTimeout bounds how long an event may take to draw.
	drawTimeout time.Duration
	// lost is told when a room's stream cannot be followed any more, because the room is gone or its log started again.
	lost func(room string)
	// people is told, after a room's people or agents changed event has gone to its viewers, to ask again who may still watch.
	people func(room string)

	handler centrifuge.BrokerEventHandler

	mu     sync.Mutex
	feeds  map[string]*feed
	closed bool
	wg     sync.WaitGroup

	published atomic.Uint64
	drawn     atomic.Uint64
	cached    atomic.Uint64
}

// BrokerOptions sizes a Broker.
type BrokerOptions struct {
	// MaxReplay is the most events a reconnecting client is replayed; a client further behind starts again from a snapshot.
	MaxReplay int
	// ReplayBytes is the most a replay may weigh. It is kept under a socket's queue, so that replaying cannot overflow it.
	ReplayBytes int
	// CacheBytes bounds the frames kept for replay.
	CacheBytes int
	// DrawTimeout bounds how long drawing one event may take.
	DrawTimeout time.Duration
}

// maxFrame is the most a frame may weigh. A larger one is sent as a refresh, which makes the page fetch the conversation.
const maxFrame = 256 << 10

// NewBroker makes a Broker that follows the logs in log and draws their events with renderer. lost may be nil.
func NewBroker(log Log, renderer Renderer, opts BrokerOptions, lost func(room string)) *Broker {
	if opts.MaxReplay <= 0 {
		opts.MaxReplay = 64
	}
	if opts.ReplayBytes <= 0 {
		opts.ReplayBytes = 512 << 10
	}
	if opts.CacheBytes <= 0 {
		opts.CacheBytes = 8 << 20
	}
	if opts.DrawTimeout <= 0 {
		opts.DrawTimeout = 10 * time.Second
	}
	return &Broker{
		log: log, renderer: renderer, cache: newFrameCache(opts.CacheBytes), maxReplay: opts.MaxReplay, replayBytes: opts.ReplayBytes,
		drawTimeout: opts.DrawTimeout, lost: lost, feeds: map[string]*feed{},
	}
}

var _ centrifuge.Broker = (*Broker)(nil)

// RegisterBrokerEventHandler gives the Broker the node it hands publications to.
func (b *Broker) RegisterBrokerEventHandler(handler centrifuge.BrokerEventHandler) error {
	b.handler = handler
	return nil
}

// Subscribe starts following the rooms whose streams the node has its first viewer for.
func (b *Broker) Subscribe(channels ...string) error {
	for _, channel := range channels {
		if room, stream, ok := channelRoom(channel); ok && stream {
			if err := b.follow(room); err != nil {
				return err
			}
		}
	}
	return nil
}

// Unsubscribe stops following the rooms whose last viewer left. An event being handed on at that moment is finished and
// dropped by the node, which has no viewer for it.
func (b *Broker) Unsubscribe(channels ...string) error {
	for _, channel := range channels {
		if room, stream, ok := channelRoom(channel); ok && stream {
			b.unfollow(room)
		}
	}
	return nil
}

// Publish sends a message on a live channel. A stream is not published to; the log is.
func (b *Broker) Publish(channel string, data []byte, opts centrifuge.PublishOptions) (centrifuge.PublishResult, error) {
	if _, stream, ok := channelRoom(channel); !ok || stream {
		return centrifuge.PublishResult{}, errors.New("only the room's log publishes to its stream")
	}
	pub := &centrifuge.Publication{Data: data, Info: opts.ClientInfo, Time: time.Now().UnixMilli()}
	return centrifuge.PublishResult{}, b.handler.HandlePublication(channel, pub, centrifuge.StreamPosition{}, false, nil)
}

// PublishJoin tells a live channel's viewers that somebody came.
func (b *Broker) PublishJoin(channel string, info *centrifuge.ClientInfo) error {
	return b.handler.HandleJoin(channel, info)
}

// PublishLeave tells a live channel's viewers that somebody went.
func (b *Broker) PublishLeave(channel string, info *centrifuge.ClientInfo) error {
	return b.handler.HandleLeave(channel, info)
}

// RemoveHistory does nothing: a room's log is the room's record and is not removed by a viewer.
func (b *Broker) RemoveHistory(string) error { return nil }

// History is the events of a stream after a position, drawn, and where the stream stands. A position from another epoch, or
// from before the window, or further back than a replay is allowed to go, gets none, which tells the node it cannot recover
// the client.
func (b *Broker) History(channel string, opts centrifuge.HistoryOptions) ([]*centrifuge.Publication, centrifuge.StreamPosition, error) {
	room, stream, ok := channelRoom(channel)
	if !ok || !stream {
		return nil, centrifuge.StreamPosition{}, nil
	}
	var since centrifuge.StreamPosition
	if opts.Filter.Since != nil {
		since = *opts.Filter.Since
	}
	if opts.Filter.Limit == 0 {
		head, err := b.log.RoomHead(room)
		return nil, streamPosition(head), err
	}
	limit := opts.Filter.Limit
	if limit < 0 || limit > b.maxReplay {
		limit = b.maxReplay
	}
	slice, err := b.log.RoomEvents(room, since.Offset, limit)
	if err != nil {
		return nil, centrifuge.StreamPosition{}, err
	}
	position := streamPosition(slice.Head)
	if slice.Gap || (since.Epoch != "" && since.Epoch != slice.Head.Epoch) {
		return nil, position, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), b.drawTimeout)
	defer cancel()
	pubs, fits := b.replay(ctx, room, slice.Events)
	if !fits {
		return nil, position, nil
	}
	return pubs, position, nil
}

func streamPosition(head roomlog.Position) centrifuge.StreamPosition {
	return centrifuge.StreamPosition{Offset: head.Seq, Epoch: head.Epoch}
}

// replay draws events for a client catching up. Where a turn or note is changed more than once in the batch, the earlier events
// are sent as skips, since the last carries the thing as it is. A replay is sent in one reply, so one that weighs more than the
// bytes it is allowed does not fit: the client could not be sent it without being disconnected for falling behind, and over and
// over, so it is told it cannot be recovered and takes a snapshot.
func (b *Broker) replay(ctx context.Context, room string, events []roomlog.Event) ([]*centrifuge.Publication, bool) {
	last := make(map[string]int, len(events))
	for at, event := range events {
		last[supersedeKey(event)] = at
	}
	pubs := make([]*centrifuge.Publication, len(events))
	weight := 0
	for at, event := range events {
		var data []byte
		if key := supersedeKey(event); key != "" && last[key] != at {
			data = Frame{V: Version, Type: FrameSkip, Room: room, Thread: event.Thread, Seq: event.Seq}.encode()
		} else {
			data = b.frame(ctx, room, event)
		}
		if weight += len(data); weight > b.replayBytes {
			return nil, false
		}
		pubs[at] = &centrifuge.Publication{Offset: event.Seq, Data: data, Time: event.At * 1000}
	}
	return pubs, true
}

// supersedeKey names what an event changed, for the kinds whose later events replace the earlier; it is empty for the others.
func supersedeKey(event roomlog.Event) string {
	if event.Ref == "" || (event.Kind != kindTurn && event.Kind != kindNote) {
		return ""
	}
	return event.Kind + "\x00" + event.Thread + "\x00" + event.Ref
}

// frame is an event drawn and encoded, from the cache when it was drawn lately. A page that cannot be drawn is a refresh.
func (b *Broker) frame(ctx context.Context, room string, event roomlog.Event) []byte {
	if data, found := b.cache.get(room, event.Seq); found {
		b.cached.Add(1)
		return data
	}
	b.drawn.Add(1)
	frame := Frame{V: Version, Room: room, Seq: event.Seq, Thread: event.Thread}
	drawn, err := b.renderer.Draw(ctx, room, event)
	if err != nil {
		frame.Type = FrameRefresh
		return frame.encode()
	}
	frame.Type, frame.ID, frame.HTML = drawn.Type, drawn.ID, drawn.HTML
	if drawn.Thread != "" {
		frame.Thread = drawn.Thread
	}
	data := frame.encode()
	if len(data) > maxFrame {
		return Frame{V: Version, Type: FrameRefresh, Room: room, Thread: frame.Thread, Seq: event.Seq}.encode()
	}
	b.cache.put(room, event.Seq, data)
	return data
}

// Wake tells a room's feed that its log moved. It does not wait.
func (b *Broker) Wake(room string) {
	b.mu.Lock()
	following := b.feeds[room]
	b.mu.Unlock()
	if following != nil {
		following.poke()
	}
}

// Following is how many rooms have a feed running.
func (b *Broker) Following() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.feeds)
}

// Close stops every feed and waits for them to end.
func (b *Broker) Close(ctx context.Context) error {
	b.mu.Lock()
	b.closed = true
	for room, each := range b.feeds {
		each.stopNow()
		delete(b.feeds, room)
	}
	b.mu.Unlock()
	done := make(chan struct{})
	go func() {
		b.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
