package roomlive

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/roomlive/livetest"
)

// What the live layer costs, measured through a real server in a process of its own, with real sockets: the memory of each
// connection, and how long it takes from an event being logged to a viewer's socket receiving it. The server is this test binary
// run again as a child, so the numbers are the server's alone and not the load generator's.
//
//	ROOMLIVE_MEASURE=1 go test -run TestMeasureTheLiveLayer -count=1 -v ./internal/roomlive/
//
// ROOMLIVE_CONNS and ROOMLIVE_ROOMS size the run. It is not part of the ordinary test run.

func TestMain(m *testing.M) {
	if os.Getenv("ROOMLIVE_CHILD") == "1" {
		runMeasuredServer()
		return
	}
	os.Exit(m.Run())
}

// runMeasuredServer is the child: a live layer over the fake backend, with endpoints to make rooms, log events and report its own
// memory.
func runMeasuredServer() {
	backend := newFakeBackend(os.Getenv("ROOMLIVE_DIR"), 512)
	server, err := New(backend, Options{MaxConnections: 1 << 20, PerUser: 1 << 20})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	backend.server = server
	mux := http.NewServeMux()
	mux.Handle("/live", server.Handler())
	mux.HandleFunc("/room", func(w http.ResponseWriter, r *http.Request) { backend.addRoom(r.URL.Query().Get("id"), "*") })
	mux.HandleFunc("/emit", func(w http.ResponseWriter, r *http.Request) {
		room, count := r.URL.Query().Get("id"), atoiOr(r.URL.Query().Get("n"), 1)
		gap, _ := time.ParseDuration(r.URL.Query().Get("gap"))
		for range count {
			// The reference is the moment before the event is logged, so what is measured includes the log, the feed and the node.
			backend.emit(room, "turn", strconv.FormatInt(time.Now().UnixNano(), 10))
			if gap > 0 {
				time.Sleep(gap)
			}
		}
	})
	mux.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		runtime.GC()
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		_ = json.NewEncoder(w).Encode(childStats{HeapAlloc: m.HeapAlloc, HeapInuse: m.HeapInuse, StackInuse: m.StackInuse, Sys: m.Sys,
			Goroutines: runtime.NumGoroutine(), Conns: server.count(), Following: server.broker.Following(), Drawn: server.broker.drawn.Load()})
	})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("ready", listener.Addr().String())
	_ = http.Serve(listener, mux)
}

type childStats struct {
	HeapAlloc, HeapInuse, StackInuse, Sys uint64
	Goroutines, Conns, Following          int
	Drawn                                 uint64
}

func atoiOr(s string, d int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return d
}

// measured is the child server, started and ready.
type measured struct {
	t    *testing.T
	cmd  *exec.Cmd
	base string
}

func startMeasured(t *testing.T) *measured {
	t.Helper()
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), "ROOMLIVE_CHILD=1", "ROOMLIVE_DIR="+t.TempDir())
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	line, err := bufio.NewReader(out).ReadString('\n')
	if err != nil {
		t.Fatalf("the measured server did not start: %v", err)
	}
	return &measured{t: t, cmd: cmd, base: "http://" + strings.TrimSpace(strings.TrimPrefix(line, "ready"))}
}

func (m *measured) get(path string) []byte {
	response, err := http.Get(m.base + path)
	if err != nil {
		m.t.Fatal(err)
	}
	defer response.Body.Close()
	var out strings.Builder
	_, _ = bufio.NewReader(response.Body).WriteTo(&out)
	return []byte(out.String())
}

func (m *measured) stats() (childStats, uint64) {
	var s childStats
	if err := json.Unmarshal(m.get("/stats"), &s); err != nil {
		m.t.Fatal(err)
	}
	return s, m.rssKB() * 1024
}

// rssKB is the child's resident memory as the operating system counts it.
func (m *measured) rssKB() uint64 {
	out, err := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(m.cmd.Process.Pid)).Output()
	if err != nil {
		m.t.Fatal(err)
	}
	n, _ := strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
	return n
}

// viewers is a crowd of sockets, each reading in the background and noting how long after its event each frame came.
type viewers struct {
	mu      sync.Mutex
	clients []*livetest.Client
	// room is the room whose frames are timed, and latencies what was seen.
	room      string
	latencies []time.Duration
	received  atomic.Int64
}

func (v *viewers) join(t *testing.T, m *measured, count int, room func(i int) string) {
	t.Helper()
	for i := range count {
		r := room(i)
		c, _, err := livetest.Connect(t, m.base, "/live?room="+r, http.Header{"X-Who": {"u" + strconv.Itoa(i)}}, false)
		if err != nil {
			t.Fatalf("connection %d: %v", i, err)
		}
		for _, channel := range []string{StreamChannel(r), LiveChannel(r)} {
			if got := c.Subscribe(channel, nil); got.Err != nil {
				t.Fatalf("subscribe %d: %s", i, got.Err.Message)
			}
		}
		v.mu.Lock()
		v.clients = append(v.clients, c)
		v.mu.Unlock()
		go v.drain(c)
	}
}

func (v *viewers) drain(c *livetest.Client) {
	for {
		select {
		case p := <-c.Pushes:
			if p.Data == nil || !strings.HasPrefix(p.Channel, streamPrefix) {
				continue
			}
			v.received.Add(1)
			var f Frame
			if json.Unmarshal(p.Data, &f) != nil {
				continue
			}
			sent, err := strconv.ParseInt(f.ID, 10, 64)
			if err != nil {
				continue
			}
			v.mu.Lock()
			if v.room == f.Room {
				v.latencies = append(v.latencies, time.Duration(time.Now().UnixNano()-sent))
			}
			v.mu.Unlock()
		case <-c.Closed:
			return
		}
	}
}

func (v *viewers) measure(room string) {
	v.mu.Lock()
	v.room, v.latencies = room, nil
	v.mu.Unlock()
}

func (v *viewers) result() []time.Duration {
	v.mu.Lock()
	defer v.mu.Unlock()
	out := slices.Clone(v.latencies)
	slices.Sort(out)
	return out
}

func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	return sorted[min(len(sorted)-1, int(p*float64(len(sorted))))]
}

func report(t *testing.T, what string, latencies []time.Duration) {
	t.Logf("%-46s n=%-6d p50 %-9v p95 %-9v p99 %-9v max %v", what, len(latencies),
		percentile(latencies, 0.50).Round(time.Microsecond), percentile(latencies, 0.95).Round(time.Microsecond),
		percentile(latencies, 0.99).Round(time.Microsecond), percentile(latencies, 1).Round(time.Microsecond))
}

func TestMeasureTheLiveLayer(t *testing.T) {
	if os.Getenv("ROOMLIVE_MEASURE") == "" {
		t.Skip("a measurement, not a test: set ROOMLIVE_MEASURE=1")
	}
	conns, rooms := atoiOr(os.Getenv("ROOMLIVE_CONNS"), 2000), atoiOr(os.Getenv("ROOMLIVE_ROOMS"), 100)
	livetest.PushBuffer = 256
	m := startMeasured(t)
	for i := range rooms {
		m.get("/room?id=room" + strconv.Itoa(i))
	}
	m.get("/room?id=hot")
	baseStats, baseRSS := m.stats()
	t.Logf("go %s on %s/%s, %d CPUs", runtime.Version(), runtime.GOOS, runtime.GOARCH, runtime.NumCPU())

	crowd := &viewers{}
	crowd.join(t, m, conns, func(i int) string { return "room" + strconv.Itoa(i%rooms) })
	time.Sleep(500 * time.Millisecond)
	stats, rss := m.stats()
	per := func(after, before uint64) float64 { return float64(after-before) / float64(conns) }
	t.Logf("%d connections, each watching its room's stream and live channel, %d rooms", stats.Conns, stats.Following)
	t.Logf("memory per connection: heap in use %.0f B, goroutine stacks %.0f B, resident (RSS) %.0f B", per(stats.HeapInuse, baseStats.HeapInuse),
		per(stats.StackInuse, baseStats.StackInuse), per(rss, baseRSS))
	t.Logf("goroutines per connection: %.2f", float64(stats.Goroutines-baseStats.Goroutines)/float64(conns))

	perRoom := conns / rooms
	crowd.measure("room0")
	m.get("/emit?id=room0&n=300&gap=2ms")
	time.Sleep(500 * time.Millisecond)
	report(t, fmt.Sprintf("one room of %d viewers, 300 events 2 ms apart", perRoom), crowd.result())

	crowd.measure("room0")
	m.get("/emit?id=room0&n=300")
	time.Sleep(time.Second)
	report(t, fmt.Sprintf("one room of %d viewers, 300 events back to back", perRoom), crowd.result())

	hot := &viewers{}
	hotCount := min(conns/2, 1000)
	hot.join(t, m, hotCount, func(int) string { return "hot" })
	hot.measure("hot")
	m.get("/emit?id=hot&n=200&gap=10ms")
	time.Sleep(time.Second)
	report(t, fmt.Sprintf("one room of %d viewers, 200 events 10 ms apart", hotCount), hot.result())

	_, rssLoaded := m.stats()
	t.Logf("resident memory at %d connections: %.1f MiB (%.1f MiB with none)", conns+hotCount, float64(rssLoaded)/(1<<20), float64(baseRSS)/(1<<20))
}
