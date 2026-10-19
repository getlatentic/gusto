package main

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/getlatentic/gusto/internal/doctor"
	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/internal/taskindex"
	"github.com/getlatentic/gusto/internal/web"
)

const (
	taskIndexFile     = "tasks.db"
	closeMirrorWithin = 10 * time.Second
)

// How long the keeper waits before it tries a failed run again, and for the rest of a burst of trouble before it
// answers it. Tests shorten them.
var (
	retryReconcileAfter = 30 * time.Second
	gatherTroubleFor    = time.Second
)

// openTasks opens the index of tasks in the database that holds the usage index, the same GUSTO_DATABASE_URL, or
// the file tasks.db beside the tasks directory.
func openTasks(ctx context.Context, tasks string) (taskindex.Store, error) {
	return taskindex.Open(ctx, usageURL(), usageDataDir(tasks))
}

func taskIndexPlace(tasks string) string { return databasePlace(tasks, taskIndexFile) }

// startTasks opens the index of tasks, copies every change to a task into it, brings it up to date with the task
// directories in the background, and gives it to the pages. The returned function stops all of that, writing what
// is still queued first, after workStopped, which waits for the tasks that are running to end, since their last change is
// one of those the index is to be given.
func startTasks(index *jobs.Index, pages *web.Server, workStopped func()) (func(), error) {
	store, err := openTasks(context.Background(), index.Root())
	if err != nil {
		return nil, fmt.Errorf("the task index at %s: %w", taskIndexPlace(index.Root()), err)
	}
	progress := &taskindex.Progress{}
	progress.Set(taskindex.Status{Building: true})
	pages.SetTasks(store)
	pages.SetTasksStatus(progress.Status)
	index.UseMirror(store)
	keeping, stopKeeping := context.WithCancel(context.Background())
	var kept sync.WaitGroup
	kept.Add(1)
	go func() {
		defer kept.Done()
		keepTasks(keeping, index, store, progress)
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			stopKeeping()
			kept.Wait()
			if workStopped != nil {
				workStopped()
			}
			closing, cancel := context.WithTimeout(context.Background(), closeMirrorWithin)
			defer cancel()
			index.CloseMirror(closing)
			store.Close()
		})
	}, nil
}

// keepTasks makes the index say what the task directories say, then does so again each time the mirror reports a
// change it could not write, until ctx ends. A run that fails is tried again later.
func keepTasks(ctx context.Context, index *jobs.Index, store taskindex.Store, progress *taskindex.Progress) {
	trouble := index.MirrorTrouble()
	for ctx.Err() == nil {
		result, err := index.Reconcile(ctx, store, progress)
		switch {
		case err != nil && ctx.Err() == nil:
			slog.Error("the task index could not be brought up to date", "error", taskindex.Scrub(err))
			if !pause(ctx, retryReconcileAfter) {
				return
			}
			continue
		case result.Fixed() > 0:
			slog.Info("the task index was brought up to date", "tasks", result.Tasks, "added", result.Missing, "corrected", result.Stale, "removed", result.Gone)
		}
		select {
		case <-ctx.Done():
			return
		case <-trouble:
			pause(ctx, gatherTroubleFor)
		}
	}
}

// pause waits, and says false if ctx ended first.
func pause(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

// taskIndexCheck is the doctor's check of the task index: that it opens, and what schema it is at.
func taskIndexCheck() doctor.Result {
	tasks := setting("GUSTO_JOBS", "jobs")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	current, latest, err := taskindex.Version(ctx, usageURL(), usageDataDir(tasks))
	return doctor.Result{Name: "task index", Verdict: schemaVerdict(taskIndexPlace(tasks), current, latest, err)}
}

// taskIndexStatusLine is what `gusto data status` says of the task index. It opens the index only when it is
// already at the schema this release reads, so that asking changes nothing.
func taskIndexStatusLine() string {
	tasks := setting("GUSTO_JOBS", "jobs")
	place := taskIndexPlace(tasks)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	current, latest, err := taskindex.Version(ctx, usageURL(), usageDataDir(tasks))
	switch {
	case err != nil:
		return fmt.Sprintf("task index  %s  unavailable: %s", place, taskindex.Scrub(err))
	case current == 0:
		return fmt.Sprintf("task index  %s  not made yet; this release reads schema version %d", place, latest)
	case current != latest:
		return fmt.Sprintf("task index  %s  schema version %d; this release reads %d", place, current, latest)
	}
	store, err := openTasks(ctx, tasks)
	if err != nil {
		return fmt.Sprintf("task index  %s  schema version %d  unavailable: %s", place, current, taskindex.Scrub(err))
	}
	defer store.Close()
	counts, err := store.Count(ctx, taskindex.Filter{})
	if err != nil {
		return fmt.Sprintf("task index  %s  schema version %d  unavailable: %s", place, current, taskindex.Scrub(err))
	}
	rows := 0
	for _, n := range counts {
		rows += n
	}
	return fmt.Sprintf("task index  %s  schema version %d  %d rows", place, current, rows)
}
