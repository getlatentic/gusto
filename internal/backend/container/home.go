package container

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/getlatentic/gusto/internal/atomicfile"
	"github.com/getlatentic/gusto/pkg/task"
)

// DefaultHomeLimit is the most a person's kept home may take when the operator sets none: a home on a shared file system that
// nothing limits grows with every cache and download an agent leaves.
const DefaultHomeLimit int64 = 5 << 30

const (
	// homeWatch is how often a running turn's home is counted again.
	homeWatch = time.Minute
	// countTime is the most one count of a home may take.
	countTime = 5 * time.Minute
)

// ErrHomeFull is a turn that did not start, or was stopped, because the person's kept home passed its limit.
var ErrHomeFull = errors.New("the kept home is over its size limit")

// HomeUse is how much of its limit a person's kept home takes, as last counted.
type HomeUse struct {
	Used  int64     `json:"used"`
	Limit int64     `json:"limit"`
	At    time.Time `json:"at"`
	// Clean says no turn has run in the home since it was counted, so the count can stand in for a new one.
	Clean bool `json:"clean"`
}

// homeNote is what a turn's record says about the home, when the home is why the turn did not run or was stopped.
type homeNote struct {
	Outcome string `json:"outcome"`
	Used    int64  `json:"used"`
	Limit   int64  `json:"limit"`
}

// homes are the kept homes under one directory, one for each person's computer.
type homes struct {
	root    string
	limit   int64
	count   func(ctx context.Context, dir string) (int64, error)
	watch   time.Duration
	tending time.Duration

	mu       sync.Mutex
	active   map[string]int
	removing map[string]bool
}

func newHomes(root string, limit int64, count func(context.Context, string) (int64, error)) *homes {
	if limit <= 0 {
		limit = DefaultHomeLimit
	}
	return &homes{root: root, limit: limit, count: count, watch: homeWatch, tending: tendEvery, active: map[string]int{}, removing: map[string]bool{}}
}

func (h *homes) dir(id string) string       { return filepath.Join(h.root, id, "home") }
func (h *homes) usageFile(id string) string { return filepath.Join(h.root, id, "usage.json") }
func (h *homes) markFile(id string) string  { return filepath.Join(h.root, id+".retire") }

// open is the home of a computer, made when it is not there. A home that is being removed, or asked to be, is not made again
// under the person who left.
func (h *homes) open(id string) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.leaving(id); err != nil {
		return "", err
	}
	return h.dir(id), os.MkdirAll(h.dir(id), 0o700)
}

func (h *homes) leaving(id string) error {
	if _, err := os.Stat(h.markFile(id)); h.removing[id] || err == nil {
		return errors.New("this home is being removed: try again in a minute")
	}
	return nil
}

// Use is how much of its limit a computer's home took when it was last counted. ok is false before the first turn ends.
func (h *homes) use(id string) (use HomeUse, ok bool) {
	text, err := os.ReadFile(h.usageFile(id))
	if err != nil || json.Unmarshal(text, &use) != nil {
		return HomeUse{}, false
	}
	return use, true
}

func (h *homes) write(id string, use HomeUse) {
	text, _ := json.Marshal(use)
	err := os.MkdirAll(filepath.Dir(h.usageFile(id)), 0o700)
	if err == nil {
		err = atomicfile.WriteFile(h.usageFile(id), text, 0o600)
	}
	if err != nil {
		slog.Warn("how much a home takes could not be kept", "home", id, "error", err)
	}
}

// guard runs a turn in a home that is within its limit and stops it if it passes the limit. A turn that cannot start says why
// in its record and in the error.
func (h *homes) guard(ctx context.Context, dir task.Dir, id string, run func(context.Context) (int, error)) (int, error) {
	if err := h.begin(ctx, dir, id); err != nil {
		return 0, err
	}
	turn, stop := context.WithCancelCause(ctx)
	watching := make(chan struct{})
	go func() {
		defer close(watching)
		h.keepWatching(turn, dir, id, stop)
	}()
	code, err := run(turn)
	stop(nil)
	<-watching
	h.end(id)
	if cause := context.Cause(turn); errors.Is(cause, ErrHomeFull) && ctx.Err() == nil {
		return -1, cause
	}
	return code, err
}

// begin counts the turn in, and refuses it when the home is over its limit. The first turn to start uses the count made when the
// last one ended, so a turn does not wait for the home to be counted.
func (h *homes) begin(ctx context.Context, dir task.Dir, id string) error {
	counted, err := h.enter(id)
	if err != nil {
		return err
	}
	used, err := h.usedAtStart(ctx, id, counted)
	if err == nil && used > h.limit {
		err = h.over(dir, id, used, "refused")
	}
	if err != nil {
		h.leave(id)
	}
	return err
}

// enter counts a turn in. counted is the count that stands for the home's size when this is the first turn since it was made, or
// nil: the home is marked not clean from here until its last turn ends.
func (h *homes) enter(id string) (counted *HomeUse, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.leaving(id); err != nil {
		return nil, err
	}
	first := h.active[id] == 0
	h.active[id]++
	if use, ok := h.use(id); first && ok && use.Clean {
		counted = &use
		dirty := use
		dirty.Clean = false
		h.write(id, dirty)
	}
	return counted, nil
}

func (h *homes) leave(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.active[id]--; h.active[id] <= 0 {
		delete(h.active, id)
	}
}

func (h *homes) usedAtStart(ctx context.Context, id string, counted *HomeUse) (int64, error) {
	if counted != nil {
		return counted.Used, nil
	}
	return h.counted(ctx, id)
}

func (h *homes) counted(ctx context.Context, id string) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, countTime)
	defer cancel()
	return h.count(ctx, h.dir(id))
}

// over says in the turn's record, and in the error, that the home is past its limit.
func (h *homes) over(dir task.Dir, id string, used int64, outcome string) error {
	h.write(id, HomeUse{Used: used, Limit: h.limit, At: time.Now()})
	if err := writeNote(dir, homeNote{Outcome: outcome, Used: used, Limit: h.limit}); err != nil {
		slog.Warn("a turn's record could not say its home was full", "error", err)
	}
	return fmt.Errorf("%w: it takes %s of the %s it may, so the turn was %s; remove files from it", ErrHomeFull, size(used), size(h.limit), outcome)
}

// keepWatching counts the home while the turn runs, and stops the turn with the reason when the home passes its limit.
func (h *homes) keepWatching(ctx context.Context, dir task.Dir, id string, stop context.CancelCauseFunc) {
	tick := time.NewTicker(h.watch)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		used, err := h.counted(ctx, id)
		if err != nil {
			if ctx.Err() == nil {
				slog.Warn("a running turn's home could not be counted", "home", id, "error", err)
			}
			continue
		}
		if used > h.limit {
			stop(h.over(dir, id, used, "stopped"))
			return
		}
		h.write(id, HomeUse{Used: used, Limit: h.limit, At: time.Now()})
	}
}

// end counts the home when the last turn in it ends, so the next one can start on that count. A home whose count failed stays
// not clean, and the next turn counts it.
func (h *homes) end(id string) {
	ctx, cancel := context.WithTimeout(context.Background(), countTime)
	defer cancel()
	used, err := h.count(ctx, h.dir(id))
	if err != nil {
		slog.Warn("a home could not be counted when its turn ended", "home", id, "error", err)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if err == nil && h.active[id] == 1 {
		h.write(id, HomeUse{Used: used, Limit: h.limit, At: time.Now(), Clean: true})
	}
	if h.active[id]--; h.active[id] <= 0 {
		delete(h.active, id)
	}
}

func writeNote(dir task.Dir, note homeNote) error {
	text, err := json.MarshalIndent(note, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir.Out(), 0o755); err != nil {
		return err
	}
	return atomicfile.WriteFile(dir.OutFile("home.json"), append(text, '\n'), 0o644)
}

// size is a number of bytes in the words a person reads.
func size(bytes int64) string {
	const unit = 1 << 10
	if bytes < unit {
		return fmt.Sprintf("%d bytes", bytes)
	}
	value, suffix := float64(bytes)/unit, "KiB"
	for _, next := range []string{"MiB", "GiB", "TiB"} {
		if value < unit {
			break
		}
		value, suffix = value/unit, next
	}
	return fmt.Sprintf("%.1f %s", value, suffix)
}
