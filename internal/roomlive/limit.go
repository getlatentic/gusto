package roomlive

import (
	"sync"
	"time"
)

// bucket is a token bucket: a burst of commands, then a steady rate. One belongs to each socket, so a page that loops cannot
// make the server do work without end.
type bucket struct {
	mu       sync.Mutex
	tokens   float64
	capacity float64
	perSec   float64
	at       time.Time
}

func newBucket(capacity, perSec float64) *bucket {
	return &bucket{tokens: capacity, capacity: capacity, perSec: perSec, at: time.Now()}
}

func (b *bucket) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	b.tokens = min(b.capacity, b.tokens+now.Sub(b.at).Seconds()*b.perSec)
	b.at = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// typingEvery is the shortest time between two typing messages from one socket in one room that are passed on. A page sends one
// every few seconds while somebody types; a faster sender is coalesced, not queued.
const typingEvery = 2 * time.Second

// typingGate remembers when each room last had a typing message from a socket.
type typingGate struct {
	mu   sync.Mutex
	last map[string]time.Time
}

// pass says whether a typing message for the room is to be passed on now.
func (g *typingGate) pass(room string, now time.Time) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if last, found := g.last[room]; found && now.Sub(last) < typingEvery {
		return false
	}
	if g.last == nil {
		g.last = map[string]time.Time{}
	}
	g.last[room] = now
	return true
}
