package roomlive

import (
	"bufio"
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/centrifugal/centrifuge"
	"github.com/prometheus/client_golang/prometheus"
)

// Options sizes and times the live layer. Zero values take the defaults, which are the ones the server runs with.
type Options struct {
	// MaxConnections bounds the sockets open at once; one more is turned away with 503.
	MaxConnections int
	// PerUser bounds one person's sockets, which is their open tabs.
	PerUser int
	// Recheck is how often an open socket is asked whether its session is still good.
	Recheck time.Duration
	// PingInterval and PongTimeout are the heartbeat. A proxy that closes an idle connection after a minute needs the first
	// below it.
	PingInterval, PongTimeout time.Duration
	// WriteTimeout is how long a write to a socket may take before the socket is given up on.
	WriteTimeout time.Duration
	// ReadLimit is the most a socket may send in one message.
	ReadLimit int
	// QueueBytes bounds what is waiting to be written to one socket. A viewer that falls this far behind is disconnected, and
	// comes back by replay.
	QueueBytes int
	Broker     BrokerOptions
}

func (o Options) withDefaults() Options {
	set := func(v *int, d int) {
		if *v <= 0 {
			*v = d
		}
	}
	setTime := func(v *time.Duration, d time.Duration) {
		if *v <= 0 {
			*v = d
		}
	}
	set(&o.MaxConnections, 10000)
	set(&o.PerUser, 100)
	set(&o.ReadLimit, 64<<10)
	set(&o.QueueBytes, 1<<20)
	set(&o.Broker.MaxReplay, 64)
	set(&o.Broker.ReplayBytes, o.QueueBytes/2)
	setTime(&o.Recheck, 2*time.Minute)
	setTime(&o.PingInterval, 30*time.Second)
	setTime(&o.PongTimeout, 10*time.Second)
	setTime(&o.WriteTimeout, 5*time.Second)
	return o
}

// Server is the live layer: the Centrifuge node, the socket endpoint, and the bookkeeping of who is watching which room.
type Server struct {
	backend Backend
	opts    Options
	node    *centrifuge.Node
	broker  *Broker
	socket  http.Handler

	closing atomic.Bool
	dedupe  *dedupe

	mu      sync.Mutex
	conns   map[string]*conn
	channel map[string]map[string]*conn
	// inflight is the commands being carried out, which shutdown waits for.
	inflight int
	// idle is closed once, when shutdown has begun and the last socket has gone and the last command has finished.
	idle     chan struct{}
	idleOnce sync.Once

	commands atomic.Uint64
	repeated atomic.Uint64
}

// New starts the live layer on a Backend. The caller wires the store's events to Wake, and serves Handler at the socket's path.
func New(backend Backend, opts Options) (*Server, error) {
	opts = opts.withDefaults()
	s := &Server{
		backend: backend, opts: opts, dedupe: newDedupe(8192, 10*time.Minute),
		conns: map[string]*conn{}, channel: map[string]map[string]*conn{}, idle: make(chan struct{}),
	}
	node, err := centrifuge.New(s.nodeConfig())
	if err != nil {
		return nil, err
	}
	s.node = node
	s.broker = NewBroker(backend, backend, opts.Broker, s.restart)
	s.broker.people = s.Revalidate
	node.SetBroker(s.broker)
	node.OnConnecting(s.connecting)
	node.OnConnect(s.connected)
	if err := node.Run(); err != nil {
		return nil, err
	}
	s.socket = centrifuge.NewWebsocketHandler(node, centrifuge.WebsocketConfig{
		CheckOrigin:      sameOrigin,
		MessageSizeLimit: opts.ReadLimit,
		WriteTimeout:     opts.WriteTimeout,
		// Most sockets are idle most of the time. With pooled write buffers and a read loop that does not keep the stack of its
		// deepest command a connection holds 24 KB resident, against 35 KB without them (measure_test.go).
		UseWriteBufferPool:         true,
		ProcessCommandsOffReadLoop: true,
		PingPongConfig:             centrifuge.PingPongConfig{PingInterval: opts.PingInterval, PongTimeout: opts.PongTimeout},
	})
	return s, nil
}

func (s *Server) nodeConfig() centrifuge.Config {
	return centrifuge.Config{
		Name:                         "gusto",
		LogLevel:                     centrifuge.LogLevelWarn,
		LogHandler:                   logToSlog,
		Metrics:                      centrifuge.MetricsConfig{RegistererGatherer: prometheus.NewRegistry()},
		ClientQueueMaxSize:           s.opts.QueueBytes,
		ClientChannelLimit:           16,
		UserConnectionLimit:          s.opts.PerUser,
		ClientStaleCloseDelay:        10 * time.Second,
		ClientPresenceUpdateInterval: 25 * time.Second,
		RecoveryMaxPublicationLimit:  s.opts.Broker.MaxReplay,
		UseSingleFlight:              true,
		GetChannelMediumOptions: func(channel string) centrifuge.ChannelMediumOptions {
			if _, stream, ok := channelRoom(channel); ok && stream {
				return centrifuge.ChannelMediumOptions{SharedPositionSync: true}
			}
			return centrifuge.ChannelMediumOptions{}
		},
	}
}

func logToSlog(entry centrifuge.LogEntry) {
	attrs := make([]any, 0, 2*len(entry.Fields))
	for key, value := range entry.Fields {
		attrs = append(attrs, key, value)
	}
	level := slog.LevelWarn
	if entry.Level >= centrifuge.LogLevelError {
		level = slog.LevelError
	}
	slog.Log(context.Background(), level, "live: "+entry.Message, attrs...)
}

// Handler is the socket endpoint. It authenticates the person and, when the request names a room, authorizes them for it,
// both before the connection is upgraded.
func (s *Server) Handler() http.Handler { return http.HandlerFunc(s.serve) }

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	if s.closing.Load() || s.count() >= s.opts.MaxConnections {
		w.Header().Set("Retry-After", "5")
		http.Error(w, "the live connection is not available just now", http.StatusServiceUnavailable)
		return
	}
	viewer, err := s.backend.Authenticate(r)
	if err != nil {
		http.Error(w, err.Error(), statusOf(err))
		return
	}
	if room := r.URL.Query().Get("room"); room != "" {
		if err := s.backend.Authorize(viewer, room); err != nil {
			http.Error(w, err.Error(), statusOf(err))
			return
		}
	}
	credentials := &centrifuge.Credentials{UserID: viewer.Who, Info: viewer.Info, ExpireAt: s.expiry()}
	ctx := centrifuge.SetCredentials(withViewer(r.Context(), viewer), credentials)
	s.socket.ServeHTTP(hijackable{w, http.NewResponseController(w)}, r.WithContext(ctx))
}

// hijackable gives the socket handler the Hijacker it asks for by type. The writer it is given may be wrapped, by the page's
// headers or the access log, and a wrapper that does not pass Hijack on still lets the response controller reach the connection
// by unwrapping.
type hijackable struct {
	http.ResponseWriter
	control *http.ResponseController
}

func (h hijackable) Hijack() (net.Conn, *bufio.ReadWriter, error) { return h.control.Hijack() }

// expiry is when a socket's session is next asked about, in whole seconds as Centrifuge counts them, rounded up so a short
// interval is not already past when it is set.
func (s *Server) expiry() int64 { return time.Now().Add(s.opts.Recheck).Unix() + 1 }

func statusOf(err error) int {
	if errors.Is(err, ErrUnauthenticated) {
		return http.StatusUnauthorized
	}
	return http.StatusNotFound
}

// sameOrigin lets a page of this server open a socket and refuses a page of another. A request with no Origin is not from a
// browser page, and carries no cookie of a person to act as.
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	return err == nil && strings.EqualFold(parsed.Host, r.Host)
}

// Wake tells the live layer that a room's log moved.
func (s *Server) Wake(room string) { s.broker.Wake(room) }

// Stats is what the live layer is doing, for the metrics page.
type Stats struct {
	Connections int
	Rooms       int
	Published   uint64
	Drawn       uint64
	Replayed    uint64
	Commands    uint64
	Repeated    uint64
}

func (s *Server) Stats() Stats {
	return Stats{
		Connections: s.count(), Rooms: s.broker.Following(), Published: s.broker.published.Load(),
		Drawn: s.broker.drawn.Load(), Replayed: s.broker.cached.Load(), Commands: s.commands.Load(), Repeated: s.repeated.Load(),
	}
}

// Shutdown closes every socket with 1001, going away, which a browser takes as leave to reconnect elsewhere, and waits for the
// sockets and the commands in flight to finish or for ctx to end. http.Server.Shutdown does not wait for a hijacked connection,
// so this is what makes a deploy end the sockets before the process does.
func (s *Server) Shutdown(ctx context.Context) error {
	s.closing.Store(true)
	for _, each := range s.snapshot() {
		each.client.Disconnect(goingAway)
	}
	s.mu.Lock()
	s.settleLocked()
	s.mu.Unlock()
	var waitErr error
	select {
	case <-s.idle:
	case <-ctx.Done():
		waitErr = ctx.Err()
	}
	return errors.Join(waitErr, s.node.Shutdown(ctx))
}

// settleLocked closes idle once shutdown has begun and nothing is left open or running. The lock is held.
func (s *Server) settleLocked() {
	if s.closing.Load() && len(s.conns) == 0 && s.inflight == 0 {
		s.idleOnce.Do(func() { close(s.idle) })
	}
}
