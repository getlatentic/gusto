package roomlive

import (
	"container/list"
	"sync"
)

// frameCache keeps the newest encoded frames, so an event drawn for the viewers who are watching is not drawn again for a
// viewer who reconnects and replays it. It is bounded in bytes; the least recently used go first.
type frameCache struct {
	mu    sync.Mutex
	limit int
	bytes int
	order *list.List
	held  map[frameKey]*list.Element
}

type frameKey struct {
	room string
	seq  uint64
}

type framed struct {
	key  frameKey
	data []byte
}

func newFrameCache(limit int) *frameCache {
	return &frameCache{limit: limit, order: list.New(), held: map[frameKey]*list.Element{}}
}

func (c *frameCache) get(room string, seq uint64) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	at, found := c.held[frameKey{room, seq}]
	if !found {
		return nil, false
	}
	c.order.MoveToFront(at)
	return at.Value.(*framed).data, true
}

// put keeps a frame unless it alone is more than the cache holds.
func (c *frameCache) put(room string, seq uint64, data []byte) {
	if len(data) > c.limit {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	key := frameKey{room, seq}
	if at, found := c.held[key]; found {
		c.bytes -= len(at.Value.(*framed).data)
		c.order.Remove(at)
	}
	c.held[key] = c.order.PushFront(&framed{key: key, data: data})
	c.bytes += len(data)
	for c.bytes > c.limit {
		oldest := c.order.Back()
		gone := oldest.Value.(*framed)
		c.bytes -= len(gone.data)
		delete(c.held, gone.key)
		c.order.Remove(oldest)
	}
}
