package cloud

import (
	"context"
	"net"
	"strconv"
	"sync"
	"time"
)

// routeFresh is how long a route read from the database is used, and routeMost how many are kept: past that the cache is
// emptied, since each entry is read again within routeFresh anyway.
const (
	routeFresh = time.Second
	routeMost  = 4096
)

// routeCache is the runtimes a gusto cloud server that does not lead read from the database, so that every request for an
// organization is not a query.
type routeCache struct {
	mu   sync.Mutex
	read map[string]routeRead
}

type routeRead struct {
	runtime Runtime
	at      time.Time
}

func (r *routeCache) get(slug string, now time.Time) (Runtime, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	held, found := r.read[slug]
	if !found || now.Sub(held.at) > routeFresh {
		return Runtime{}, false
	}
	return held.runtime, true
}

func (r *routeCache) put(slug string, runtime Runtime, now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.read == nil || len(r.read) >= routeMost {
		r.read = map[string]routeRead{}
	}
	r.read[slug] = routeRead{runtime: runtime, at: now}
}

// routeFollowed is Route for a gusto cloud server that does not lead: where the leader wrote the server is, read from the database.
func (c *Controller) routeFollowed(ctx context.Context, slug string) (string, State) {
	now := time.Now()
	runtime, fresh := c.followed.get(slug, now)
	if !fresh {
		var err error
		if runtime, err = c.Store.Runtime(ctx, slug); err != nil {
			return "", ""
		}
		c.followed.put(slug, runtime, now)
	}
	if runtime.State != Ready {
		return "", runtime.State
	}
	node, err := c.nodeNamed(runtime.Node)
	if err != nil {
		return "", Starting
	}
	return net.JoinHostPort(node.Host(), strconv.Itoa(runtime.Port)), Ready
}
