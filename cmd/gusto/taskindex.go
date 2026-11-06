package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
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
func (x indexes) openTasks(ctx context.Context) (taskindex.Store, error) {
	return taskindex.Open(ctx, x.url, x.dataDir())
}

func (x indexes) taskPlace() string { return x.place(taskIndexFile) }

// startTasks opens the index of tasks, copies every change to a task into it, brings it up to date with the task
// directories in the background, and gives it to the pages. The returned function stops all of that, writing what
// is still queued first, after workStopped, which waits for the tasks that are running to end, since their last change is
// one of those the index is to be given.
func startTasks(index *jobs.Index, pages *web.Server, workStopped func(), databaseURL string) (func(), error) {
	where := indexes{url: strings.TrimSpace(databaseURL), tasks: index.Root()}
	store, err := where.openTasks(context.Background())
	if err != nil {
		return nil, fmt.Errorf("the task index at %s: %w", where.taskPlace(), err)
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
func taskIndexCheck(where indexes) doctor.Result {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	current, latest, err := taskindex.Version(ctx, where.url, where.dataDir())
	return doctor.Result{Name: "task index", Verdict: schemaVerdict(where.taskPlace(), current, latest, err)}
}

// taskIndexStatusLine is what `gusto data status` says of the task index. It opens the index only when it is
// already at the schema this release reads, so that asking changes nothing.
func taskIndexStatusLine(where indexes) string {
	place := where.taskPlace()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	current, latest, err := taskindex.Version(ctx, where.url, where.dataDir())
	switch {
	case err != nil:
		return fmt.Sprintf("task index  %s  unavailable: %s", place, taskindex.Scrub(err))
	case current == 0:
		return fmt.Sprintf("task index  %s  not made yet; this release reads schema version %d", place, latest)
	case current != latest:
		return fmt.Sprintf("task index  %s  schema version %d; this release reads %d", place, current, latest)
	}
	store, err := where.openTasks(ctx)
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
