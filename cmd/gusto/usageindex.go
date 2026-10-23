package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/getlatentic/gusto/internal/doctor"
	"github.com/getlatentic/gusto/internal/usage"
	"github.com/getlatentic/gusto/internal/web"
	"github.com/getlatentic/gusto/pkg/proxy"
)

// usageURL is where the index of model calls is kept: empty for a SQLite file beside the tasks, or a postgres:// address.
func usageURL() string { return strings.TrimSpace(os.Getenv("GUSTO_DATABASE_URL")) }

// usageDataDir is the directory the server keeps its own data in, the one that holds the tasks directory, so
// that the index sits beside what it is built from and not inside the tasks.
func usageDataDir(tasks string) string { return filepath.Dir(filepath.Clean(tasks)) }

func openUsage(ctx context.Context, tasks string) (usage.Store, error) {
	return usage.Open(ctx, usageURL(), usageDataDir(tasks))
}

// usagePlace says where the index is, with no password in it.
func usagePlace(tasks string) string { return databasePlace(tasks, "usage.db") }

// databasePlace says where an index kept in file is, with no password in it.
func databasePlace(tasks, file string) string {
	raw := usageURL()
	if raw == "" {
		return filepath.Join(usageDataDir(tasks), file)
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "the database in GUSTO_DATABASE_URL"
	}
	place := parsed.Scheme + "://" + parsed.Host + parsed.Path
	if schema := parsed.Query().Get("search_path"); schema != "" {
		place += " (schema " + schema + ")"
	}
	return place
}

// startUsage opens the index, has the proxy record each call into it, fills it from the tasks' own records, and
// gives it to the pages. The returned function stops all of that, writing what is still queued first.
func startUsage(held *proxy.Proxy, pages *web.Server, tasks string) (func(), error) {
	store, err := openUsage(context.Background(), tasks)
	if err != nil {
		return nil, fmt.Errorf("the usage index at %s: %w", usagePlace(tasks), err)
	}
	if os.Getenv("GUSTO_ORG") != "" && strings.HasPrefix(usageURL(), "postgres") && !strings.Contains(usageURL(), "search_path") {
		fmt.Println("warning: GUSTO_DATABASE_URL names no search_path, so every organization using this database shares one usage index")
	}
	progress := &usage.Progress{}
	pages.SetUsage(store)
	pages.SetUsageStatus(progress.Status)
	recorder := usage.NewRecorder(store, pages.UsageWho, pages.UsageProvider)
	if held != nil {
		held.UseRecorder(recorder)
	}
	keeping, stopKeeping := context.WithCancel(context.Background())
	var kept sync.WaitGroup
	kept.Add(1)
	go func() {
		defer kept.Done()
		usage.Keep(keeping, store, tasks, pages.UsageWho, pages.UsageProvider, recorder, progress)
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			stopKeeping()
			kept.Wait()
			recorder.Close()
			store.Close()
		})
	}, nil
}

// usageCheck is the doctor's check of the index: that it opens, and what schema it is at.
func usageCheck() doctor.Result {
	tasks := setting("GUSTO_JOBS", "jobs")
	result := doctor.Result{Name: "usage index"}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	current, latest, err := usage.Version(ctx, usageURL(), usageDataDir(tasks))
	result.Verdict = schemaVerdict(usagePlace(tasks), current, latest, err)
	return result
}

// schemaVerdict is the doctor's verdict on a database that holds an index: that it opens, and what schema it is at.
func schemaVerdict(place string, current, latest int, err error) doctor.Verdict {
	switch {
	case err != nil:
		return doctor.Verdict{Status: doctor.Fail, Said: place + ": " + err.Error(),
			Remedy: "check GUSTO_DATABASE_URL and that the database is up; leave it empty to keep the index in a file beside the tasks"}
	case current > latest:
		return doctor.Verdict{Status: doctor.Fail, Said: fmt.Sprintf("%s is at schema version %d, which is newer than this release reads (%d)", place, current, latest),
			Remedy: "run the release that wrote it"}
	case current == 0:
		return doctor.Verdict{Status: doctor.Pass, Said: fmt.Sprintf("%s does not exist yet; the server makes it at schema version %d", place, latest)}
	case current < latest:
		return doctor.Verdict{Status: doctor.Pass, Said: fmt.Sprintf("%s is at schema version %d; the server upgrades it to %d at start", place, current, latest)}
	}
	return doctor.Verdict{Status: doctor.Pass, Said: fmt.Sprintf("%s opens, at schema version %d", place, current)}
}

// usageStatusLine is what `gusto data status` says of the index. It opens the index only when it is already at the
// schema this release reads, so that asking changes nothing.
func usageStatusLine() string {
	tasks := setting("GUSTO_JOBS", "jobs")
	place := usagePlace(tasks)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	current, latest, err := usage.Version(ctx, usageURL(), usageDataDir(tasks))
	switch {
	case err != nil:
		return fmt.Sprintf("usage index  %s  unavailable: %v", place, err)
	case current == 0:
		return fmt.Sprintf("usage index  %s  not made yet; this release reads schema version %d", place, latest)
	case current != latest:
		return fmt.Sprintf("usage index  %s  schema version %d; this release reads %d", place, current, latest)
	}
	store, err := openUsage(ctx, tasks)
	if err != nil {
		return fmt.Sprintf("usage index  %s  schema version %d  unavailable: %v", place, current, err)
	}
	defer store.Close()
	totals, err := store.Totals(ctx, usage.Filter{})
	if err != nil {
		return fmt.Sprintf("usage index  %s  schema version %d  unavailable: %v", place, current, err)
	}
	return fmt.Sprintf("usage index  %s  schema version %d  %d rows", place, current, totals.Calls)
}
