package cloud

import (
	"sync"
	"time"
)

// Limiter counts events in a window that slides, by key, and refuses one past
// the most allowed. One made by NewLimiter is kept in memory, where a restart
// forgets it, which lets a person try again and no more. One made by a Store
// counts in the database, so that every front door on it counts together.
type Limiter struct {
	mu    sync.Mutex
	most  int
	per   time.Duration
	hits  map[string][]time.Time
	now   func() time.Time
	store *Store
	scope string
}

// NewLimiter allows most events for a key in any window of per.
func NewLimiter(most int, per time.Duration) *Limiter {
	return &Limiter{most: most, per: per, hits: map[string][]time.Time{}, now: time.Now}
}

// Allow counts an event for key and says whether it may go ahead. A refused
// event is not counted, so trying again does not lengthen the wait.
func (l *Limiter) Allow(key string) bool {
	if l.store != nil {
		return l.store.allow(l.scope+":"+key, l.most, l.per)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	recent := l.hits[key][:0]
	for _, at := range l.hits[key] {
		if now.Sub(at) < l.per {
			recent = append(recent, at)
		}
	}
	if len(recent) >= l.most {
		l.hits[key] = recent
		return false
	}
	l.hits[key] = append(recent, now)
	if len(l.hits) > 10000 {
		l.forget(now)
	}
	return true
}

// forget drops keys with nothing in the window, so a flood of addresses cannot
// grow the map without bound.
func (l *Limiter) forget(now time.Time) {
	for key, hits := range l.hits {
		if len(hits) == 0 || now.Sub(hits[len(hits)-1]) >= l.per {
			delete(l.hits, key)
		}
	}
}
