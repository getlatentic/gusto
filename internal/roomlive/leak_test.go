package roomlive

import (
	"fmt"
	"runtime"
	"testing"
	"time"
)

// Everything the live layer starts ends with it: its node, its feeds, the sockets and the goroutines that read them.
func TestShuttingDownLeavesNoGoroutinesBehind(t *testing.T) {
	use := func() {
		r := newRig(t, 512, Options{})
		r.backend.addRoom("room-a", "ada")
		var watching []*client
		for range 10 {
			c := r.dial("ada", "room-a")
			r.join(c, "room-a")
			watching = append(watching, c)
		}
		for i := range 20 {
			r.backend.emit("room-a", "turn", fmt.Sprintf("t%d", i))
		}
		watching[0].frames(20, 5*time.Second)
		for _, c := range watching[1:] {
			c.close()
		}
		r.stop()
	}
	// A first run settles what the runtime and the client library start once and keep.
	use()
	settle := func() int {
		deadline := time.Now().Add(10 * time.Second)
		for {
			runtime.GC()
			if n := runtime.NumGoroutine(); n <= baseline+2 || time.Now().After(deadline) {
				return n
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	baseline = runtime.NumGoroutine()
	use()
	if after := settle(); after > baseline+2 {
		buf := make([]byte, 1<<16)
		t.Fatalf("%d goroutines before, %d after a live layer was started and shut down:\n%s", baseline, after, buf[:runtime.Stack(buf, true)])
	}
}

var baseline int
