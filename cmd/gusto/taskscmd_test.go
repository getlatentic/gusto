package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getlatentic/gusto/cmd/internal/cmdline/clitest"
	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/internal/taskindex"
	"github.com/getlatentic/gusto/internal/taskindex/taskindextest"
	"github.com/getlatentic/gusto/internal/web"
)

// writeState makes a task directory whose state.json holds the record.
func writeState(t *testing.T, data string, record jobs.Record) {
	t.Helper()
	dir := filepath.Join(data, "jobs", record.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	text, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "state.json"), text, 0o644); err != nil {
		t.Fatal(err)
	}
}

func someTasks(t *testing.T, data string) {
	t.Helper()
	at := time.Date(2026, 10, 5, 9, 30, 0, 0, time.UTC).Unix()
	writeState(t, data, jobs.Record{ID: "20261005-0930-fix-login", State: jobs.Passed, Brief: "Fix the login form", Harness: "codex", Asked: "ada@acme.com", Repo: "acme/web", Queued: at, Seq: 1})
	writeState(t, data, jobs.Record{ID: "20261006-1000-migrate", State: jobs.Failed, Brief: "Migrate 100% of users", Harness: "claude-code", Asked: "bob@acme.com", Repo: "acme/api", Queued: at + 86400, Seq: 2, Exit: 1})
	writeState(t, data, jobs.Record{ID: "20260901-0800-old", State: jobs.Interrupted, Brief: "Old work", Harness: "codex", Asked: "ada@acme.com", Queued: at - 30*86400, Seq: 3, Exit: -1})
}

func TestTheTasksCommandsHelpSaysWhatTheyReadAndHowTheyEnd(t *testing.T) {
	for _, path := range [][]string{{"tasks"}, {"tasks", "list"}, {"tasks", "reindex"}} {
		out, _, code := clitest.Capture(t, func() int { return run(append([]string{"help"}, path...)) })
		if code != 0 {
			t.Fatalf("%v: exit code %d", path, code)
		}
		if len(path) == 2 {
			for _, want := range []string{"GUSTO_DATABASE_URL", "GUSTO_JOBS", "Examples", "Exit codes"} {
				if !strings.Contains(out, want) {
					t.Errorf("%v: help lacks %q:\n%s", path, want, out)
				}
			}
		}
	}
	out, _, _ := clitest.Capture(t, func() int { return run([]string{"help", "tasks", "list"}) })
	for _, flag := range []string{"-person", "-state", "-agent-type", "-repo", "-team", "-search", "-month", "-sort", "-limit", "-json"} {
		if !strings.Contains(out, flag) {
			t.Errorf("list help lacks %s", flag)
		}
	}
}

func TestTasksReindexFillsTheIndexFromTheFilesAndIsRepeatable(t *testing.T) {
	data := withUsageData(t)
	someTasks(t, data)
	out, _, code := clitest.Capture(t, func() int { return run([]string{"tasks", "reindex"}) })
	if code != 0 || !strings.Contains(out, "read 3 task(s): added 3, corrected 0, removed 0") {
		t.Fatalf("reindex gave %d:\n%s", code, out)
	}
	if _, err := os.Stat(filepath.Join(data, "tasks.db")); err != nil {
		t.Fatalf("the index is not beside the tasks directory: %v", err)
	}
	out, _, code = clitest.Capture(t, func() int { return run([]string{"tasks", "reindex"}) })
	if code != 0 || !strings.Contains(out, "added 0, corrected 0, removed 0") {
		t.Errorf("the second reindex gave %d:\n%s", code, out)
	}
	if err := os.RemoveAll(filepath.Join(data, "jobs", "20260901-0800-old")); err != nil {
		t.Fatal(err)
	}
	out, _, _ = clitest.Capture(t, func() int { return run([]string{"tasks", "reindex"}) })
	if !strings.Contains(out, "removed 1") {
		t.Errorf("a task that is gone:\n%s", out)
	}
	out, _, code = clitest.Capture(t, func() int { return run([]string{"tasks", "reindex", "-from-scratch"}) })
	if code != 0 || !strings.Contains(out, "added 2") {
		t.Errorf("from scratch gave %d:\n%s", code, out)
	}
}

func TestTasksListPrintsATableAndFilters(t *testing.T) {
	data := withUsageData(t)
	someTasks(t, data)
	clitest.Capture(t, func() int { return run([]string{"tasks", "reindex"}) })
	out, _, code := clitest.Capture(t, func() int { return run([]string{"tasks", "list"}) })
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 4 || !strings.Contains(lines[1], "20261006-1000-migrate") || !strings.Contains(lines[3], "20260901-0800-old") {
		t.Fatalf("the table:\n%s", out)
	}
	for args, want := range map[string]string{
		"-person ADA@acme.com":             "20261005-0930-fix-login 20260901-0800-old",
		"-state failed,interrupted":        "20261006-1000-migrate 20260901-0800-old",
		"-agent-type codex -month 2026-10": "20261005-0930-fix-login",
		"-search 100%":                     "20261006-1000-migrate",
		"-repo acme/api":                   "20261006-1000-migrate",
		"-sort seq":                        "20260901-0800-old 20261006-1000-migrate 20261005-0930-fix-login",
		"-limit 1":                         "20261006-1000-migrate",
	} {
		out, _, code := clitest.Capture(t, func() int { return run(append([]string{"tasks", "list", "-json"}, strings.Fields(args)...)) })
		var rows []taskindex.Row
		if code != 0 || json.Unmarshal([]byte(out), &rows) != nil {
			t.Fatalf("%s: %d\n%s", args, code, out)
		}
		var ids []string
		for _, r := range rows {
			ids = append(ids, r.ID)
		}
		if got := strings.Join(ids, " "); got != want {
			t.Errorf("%s listed %q, want %q", args, got, want)
		}
	}
	out, _, code = clitest.Capture(t, func() int { return run([]string{"tasks", "list", "-person", "nobody"}) })
	if code != 0 || !strings.Contains(out, "No task matches") {
		t.Errorf("no match gave %d %q", code, out)
	}
	for _, args := range [][]string{{"-sort", "brief"}, {"-limit", "0"}, {"-month", "last"}, {"extra"}} {
		if _, _, code := clitest.Capture(t, func() int { return run(append([]string{"tasks", "list"}, args...)) }); code != 2 {
			t.Errorf("%v gave %d, want 2", args, code)
		}
	}
}

func TestTasksListPagesPastOneThousand(t *testing.T) {
	data := withUsageData(t)
	for n := 0; n < 1105; n++ {
		writeState(t, data, jobs.Record{ID: fmt.Sprintf("t%04d", n), State: jobs.Passed, Brief: "b", Queued: int64(n), Seq: int64(n + 1)})
	}
	clitest.Capture(t, func() int { return run([]string{"tasks", "reindex"}) })
	out, _, _ := clitest.Capture(t, func() int { return run([]string{"tasks", "list", "-json", "-limit", "1100"}) })
	var rows []taskindex.Row
	if err := json.Unmarshal([]byte(out), &rows); err != nil || len(rows) != 1100 || rows[0].ID != "t1104" {
		t.Fatalf("%d rows, %v", len(rows), err)
	}
}

func TestDoctorAndDataStatusReportTheTaskIndex(t *testing.T) {
	data := withUsageData(t)
	result := taskIndexCheck()
	if result.Status != "pass" || result.Name != "task index" || !strings.Contains(result.Said, "does not exist yet") {
		t.Errorf("before the server made it: %+v", result)
	}
	if line := taskIndexStatusLine(); !strings.Contains(line, "not made yet") {
		t.Errorf("data status line before: %q", line)
	}
	if _, err := os.Stat(filepath.Join(data, "tasks.db")); err == nil {
		t.Fatal("asking made the index")
	}
	someTasks(t, data)
	clitest.Capture(t, func() int { return run([]string{"tasks", "reindex"}) })
	if result = taskIndexCheck(); result.Status != "pass" || !strings.Contains(result.Said, "schema version") {
		t.Errorf("after: %+v", result)
	}
	if line := taskIndexStatusLine(); !strings.Contains(line, "schema version") || !strings.Contains(line, "3 rows") {
		t.Errorf("data status line: %q", line)
	}
	out, _, _ := clitest.Capture(t, func() int { return run([]string{"data", "status"}) })
	if !strings.Contains(out, "task index") || !strings.Contains(out, "usage index") {
		t.Errorf("data status:\n%s", out)
	}
	t.Setenv("GUSTO_DATABASE_URL", "ftp://u:hunter2@h/db")
	if result = taskIndexCheck(); result.Status != "fail" || strings.Contains(result.Said, "hunter2") || result.Remedy == "" {
		t.Errorf("a database that cannot be used: %+v", result)
	}
	if line := taskIndexStatusLine(); strings.Contains(line, "hunter2") || !strings.Contains(line, "unavailable") {
		t.Errorf("data status line for a bad URL: %q", line)
	}
}

func TestAServerFillsTheTaskIndexAtStartKeepsItCurrentAndStopsCleanly(t *testing.T) {
	data := withUsageData(t)
	someTasks(t, data)
	index, err := jobs.Open(filepath.Join(data, "jobs"))
	if err != nil {
		t.Fatal(err)
	}
	instances, err := instance.Open(filepath.Join(data, "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	identity, _ := web.NewIdentity(context.Background(), "", "", "", "", web.Access{})
	pages, err := web.New(index, instances, nil, "unused", identity, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	stop, err := startTasks(index, pages, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	waitForRows := func(want int, state string) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for {
			store, err := openTasks(context.Background(), index.Root())
			if err != nil {
				t.Fatal(err)
			}
			counts, _ := store.Count(context.Background(), taskindex.Filter{States: []string{state}})
			store.Close()
			if counts[state] == want {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("the index holds %d %s tasks, want %d", counts[state], state, want)
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	waitForRows(1, "passed")
	added := jobs.Record{ID: "20261007-0800-new", State: jobs.Queued, Brief: "new", Asked: "ada@acme.com", Queued: jobs.Now()}
	if err := os.MkdirAll(added.Dir(index.Root()).Path(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := index.Add(added); err != nil {
		t.Fatal(err)
	}
	waitForRows(1, "queued")
	stop()
	stop()
}

// brokenStore refuses what it is told to, so that the keeper's second chances can be seen.
type brokenStore struct {
	taskindex.Store
	mu          sync.Mutex
	failIndexed int
	failWrites  bool
}

func (b *brokenStore) Indexed(ctx context.Context) (map[string]taskindex.Entry, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failIndexed > 0 {
		b.failIndexed--
		return nil, fmt.Errorf("could not reach postgres://ada:hunter2@db/x")
	}
	return b.Store.Indexed(ctx)
}

func (b *brokenStore) Upsert(ctx context.Context, row taskindex.Row) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failWrites {
		return fmt.Errorf("the database is down")
	}
	return b.Store.Upsert(ctx, row)
}

func (b *brokenStore) UpsertMany(ctx context.Context, rows []taskindex.Row) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failWrites {
		return fmt.Errorf("the database is down")
	}
	return b.Store.UpsertMany(ctx, rows)
}

func (b *brokenStore) mend() {
	b.mu.Lock()
	b.failWrites = false
	b.mu.Unlock()
}

func shortWaits(t *testing.T) {
	t.Helper()
	was, gather := retryReconcileAfter, gatherTroubleFor
	retryReconcileAfter, gatherTroubleFor = 20*time.Millisecond, 10*time.Millisecond
	t.Cleanup(func() { retryReconcileAfter, gatherTroubleFor = was, gather })
}

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); !ok(); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal(what)
		}
	}
}

func TestTheKeeperTriesAgainAfterARunThatFailed(t *testing.T) {
	shortWaits(t)
	data := withUsageData(t)
	someTasks(t, data)
	index, err := jobs.Open(filepath.Join(data, "jobs"))
	if err != nil {
		t.Fatal(err)
	}
	store := &brokenStore{Store: taskindextest.New(), failIndexed: 3}
	progress := &taskindex.Progress{}
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { keepTasks(ctx, index, store, progress); close(done) }()
	defer func() { stop(); <-done }()
	eventually(t, "the index was never filled after the failed runs", func() bool {
		counts, _ := store.Count(ctx, taskindex.Filter{})
		return counts["passed"]+counts["failed"]+counts["interrupted"] == 3
	})
}

func TestTheKeeperRepairsWhatTheMirrorCouldNotWrite(t *testing.T) {
	shortWaits(t)
	data := withUsageData(t)
	index, err := jobs.Open(filepath.Join(data, "jobs"))
	if err != nil {
		t.Fatal(err)
	}
	store := &brokenStore{Store: taskindextest.New(), failWrites: true}
	index.UseMirror(store)
	defer index.CloseMirror(context.Background())
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { keepTasks(ctx, index, store, &taskindex.Progress{}); close(done) }()
	defer func() { stop(); <-done }()
	added := jobs.Record{ID: "20261007-0800-new", State: jobs.Queued, Brief: "new", Asked: "ada@acme.com", Queued: jobs.Now()}
	if err := os.MkdirAll(added.Dir(index.Root()).Path(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := index.Add(added); err != nil {
		t.Fatalf("a database that is down stopped a task: %v", err)
	}
	eventually(t, "the mirror never reported trouble", func() bool { return index.MirrorStats().Failed > 0 })
	store.mend()
	eventually(t, "the task never reached the index once the database was back", func() bool {
		_, err := store.Get(ctx, added.ID)
		return err == nil
	})
}
