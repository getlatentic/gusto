package usage

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Progress is how far a rebuild has got, for a page to read while it runs. The zero value is ready, and a nil
// *Progress is a rebuild nobody watches.
type Progress struct {
	// Changed, when set, is called with the new status each time it changes.
	Changed func(Status)

	mu     sync.Mutex
	status Status
}

// Status is the progress now.
func (p *Progress) Status() Status {
	if p == nil {
		return Status{}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.status
}

func (p *Progress) set(status Status) {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.status = status
	changed := p.Changed
	p.mu.Unlock()
	if changed != nil {
		changed(status)
	}
}

// Result is what a rebuild did. Rows is how many rows it handed to the store, which holds each once.
type Result struct {
	Tasks     int
	Rows      int
	Malformed int
}

// Backfill reads the model calls the index does not hold yet from every task's out/proxy.jsonl under jobsDir. It
// asks the store how many of each task's lines it holds and parses only the lines past that, so it can be run
// again at any time, resumes where an interrupted run stopped, and adds nothing twice.
func Backfill(ctx context.Context, store Store, jobsDir string, who WhoFunc, provider ProviderFunc, progress *Progress) (Result, error) {
	return rebuild(ctx, store, jobsDir, who, provider, progress, nil)
}

// Repair reads the named tasks again from their first line. A call the recorder could not write leaves a gap that
// Backfill, which starts after the last line held, does not look back for.
func Repair(ctx context.Context, store Store, jobsDir string, who WhoFunc, provider ProviderFunc, progress *Progress, tasks []string) (Result, error) {
	if len(tasks) == 0 {
		return Result{}, nil
	}
	again := make(map[string]bool, len(tasks))
	for _, task := range tasks {
		again[task] = true
	}
	return rebuild(ctx, store, jobsDir, who, provider, progress, again)
}

const retryAfterFailedRepair = 30 * time.Second

type file struct{ task, path string }

func recordsUnder(jobsDir string, only map[string]bool) ([]file, error) {
	paths, err := filepath.Glob(filepath.Join(jobsDir, "*", "out", "proxy.jsonl"))
	if err != nil {
		return nil, err
	}
	var found []file
	for _, path := range paths {
		task := filepath.Base(filepath.Dir(filepath.Dir(path)))
		if only == nil || only[task] {
			found = append(found, file{task, path})
		}
	}
	return found, nil
}

func rebuild(ctx context.Context, store Store, jobsDir string, who WhoFunc, provider ProviderFunc, progress *Progress, only map[string]bool) (Result, error) {
	var result Result
	files, err := recordsUnder(jobsDir, only)
	if err != nil {
		return result, err
	}
	held, err := store.Indexed(ctx)
	if err != nil {
		return result, fmt.Errorf("reading what the index holds: %w", err)
	}
	status := Status{Building: true, Total: len(files)}
	progress.set(status)
	defer func() { progress.set(Status{Done: status.Done, Total: status.Total}) }()
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		from := held[f.task]
		if only != nil {
			from = 0
		}
		got, err := indexFile(ctx, store, f, from, who, provider)
		result.Rows += got.Rows
		result.Malformed += got.Malformed
		if err != nil {
			return result, fmt.Errorf("%s: %w", f.task, err)
		}
		result.Tasks++
		status.Done++
		progress.set(status)
	}
	return result, nil
}

func indexFile(ctx context.Context, store Store, f file, from int, who WhoFunc, provider ProviderFunc) (parsed, error) {
	opened, err := os.Open(f.path)
	if errors.Is(err, fs.ErrNotExist) {
		return parsed{}, nil
	}
	if err != nil {
		return parsed{}, err
	}
	defer opened.Close()
	info, err := opened.Stat()
	if err != nil {
		return parsed{}, err
	}
	return parseFrom(opened, f.task, from, whoOf(who, f.task), provider, info.ModTime(), func(rows []Row) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return store.Add(ctx, rows)
	})
}

// Keep fills the index from the files, then repairs whatever the recorder drops or fails to write, until ctx ends.
func Keep(ctx context.Context, store Store, jobsDir string, who WhoFunc, provider ProviderFunc, recorder *Recorder, progress *Progress) {
	if _, err := Backfill(ctx, store, jobsDir, who, provider, progress); err != nil && ctx.Err() == nil {
		logIndexError("the usage index could not be filled from the tasks' records", err)
	}
	if recorder == nil {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-recorder.Dropped():
			repairLost(ctx, store, jobsDir, who, provider, recorder, progress)
		}
	}
}

func repairLost(ctx context.Context, store Store, jobsDir string, who WhoFunc, provider ProviderFunc, recorder *Recorder, progress *Progress) {
	// The drops that follow the signal are still arriving; letting them gather means one repair reads each task once.
	select {
	case <-ctx.Done():
		return
	case <-time.After(time.Second):
	}
	tasks := recorder.TakeLost()
	if _, err := Repair(ctx, store, jobsDir, who, provider, progress, tasks); err != nil && ctx.Err() == nil {
		logIndexError("the usage index could not catch up with calls it missed", err)
		select {
		case <-ctx.Done():
		case <-time.After(retryAfterFailedRepair):
		}
		recorder.markLost(tasks...)
	}
}
