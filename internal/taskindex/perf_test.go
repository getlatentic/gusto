package taskindex_test

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/dbkit/dbkittest"
	"github.com/getlatentic/gusto/internal/taskindex"
)

const (
	perfRows   = 100_000
	slowestRun = 300 * time.Millisecond
)

var words = []string{"fix", "migrate", "refactor", "document", "review", "test", "deploy", "upgrade", "profile", "clean"}

// perfBatch spreads tasks over 90 days: 60 people, 20 repositories, 6 teams, 4 agent types, nearly all of them over.
func perfBatch(from, to int) []taskindex.Row {
	agentTypes := []string{"claude-code", "codex", "pi", "opencode"}
	rows := make([]taskindex.Row, 0, to-from)
	for i := from; i < to; i++ {
		state := []string{"passed", "passed", "passed", "failed", "interrupted"}[i%5]
		if i >= perfRows-30 {
			state = []string{"queued", "running"}[i%2]
		}
		queued := epoch.Unix() + int64(i)*78
		r := taskindex.Row{
			ID:    fmt.Sprintf("%s-%s-%s-%d", time.Unix(queued, 0).UTC().Format("20060102-1504"), words[i%10], words[(i/10)%10], i),
			State: state, Repo: fmt.Sprintf("acme/repo-%02d", i%20), AgentType: agentTypes[i%4], Model: "sonnet",
			Asked: fmt.Sprintf("person-%02d@acme.com", i%60), Team: fmt.Sprintf("team-%d", i%6), Seq: int64(i + 1), Queued: queued,
			Brief: fmt.Sprintf("%s the %s of module %d in the service, and say what changed when it is done", words[i%10], words[(i/7)%10], i%997),
		}
		if state != "queued" && state != "running" {
			r.Started, r.Ended = queued+5, queued+400+int64(i%1000)
		}
		rows = append(rows, r)
	}
	return rows
}

func TestPerf(t *testing.T) {
	if testing.Short() {
		t.Skip("perf test")
	}
	if raceDetector {
		t.Skip("the race detector slows the SQLite driver too much for timings to mean anything")
	}
	t.Run("sqlite", func(t *testing.T) { perf(t, openSQLite(t), func() {}) })
	if os.Getenv(postgresEnv) == "" {
		return
	}
	t.Run("postgres", func(t *testing.T) {
		u := dbkittest.PostgresURL(t)
		s, err := taskindex.Open(ctx, u, "")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { s.Close() })
		perf(t, s, func() { analyze(t, u) })
	})
}

// analyze gathers the statistics that autovacuum would gather soon after a bulk load.
func analyze(t *testing.T, u string) {
	db, err := sql.Open("pgx", u)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`VACUUM ANALYZE tasks`); err != nil {
		t.Fatal(err)
	}
}

type step struct {
	name string
	run  func() error
}

func perf(t *testing.T, s taskindex.Store, settle func()) {
	start := time.Now()
	for from := 0; from < perfRows; from += 5000 {
		if err := s.UpsertMany(ctx, perfBatch(from, from+5000)); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("inserted %d rows in %v", perfRows, time.Since(start).Round(time.Millisecond))
	settle()
	person := taskindex.Filter{Asked: "Person-07@acme.com"}
	week := taskindex.Filter{QueuedFrom: epoch.Add(time.Duration(perfRows-9000) * 78 * time.Second)}
	first := func(f taskindex.Filter, sort string) func() error {
		return func() error { _, _, err := s.List(ctx, f, taskindex.Page{Limit: 50, Sort: sort}); return err }
	}
	count := func(f taskindex.Filter) func() error {
		return func() error { _, err := s.Count(ctx, f); return err }
	}
	for _, st := range []step{
		{"list, first page", first(taskindex.Filter{}, "")},
		{"list, person filter", first(person, "")},
		{"list, person and state", first(taskindex.Filter{Asked: person.Asked, States: []string{"failed", "interrupted"}}, "")},
		{"list, state failed", first(taskindex.Filter{States: []string{"failed", "interrupted"}}, "")},
		{"list, state running (rare)", first(taskindex.Filter{States: []string{"running"}}, "")},
		{"list, agent type", first(taskindex.Filter{AgentType: "pi"}, "")},
		{"list, repository", first(taskindex.Filter{Repo: "acme/repo-03"}, "")},
		{"list, team and agent type", first(taskindex.Filter{Team: "team-2", AgentType: "codex"}, "")},
		{"list, last week", first(week, "")},
		{"list, search for a common word", first(taskindex.Filter{Search: "MIGRATE"}, "")},
		{"list, search for a rare word", first(taskindex.Filter{Search: "module 996 in"}, "")},
		{"list, search that matches nothing", first(taskindex.Filter{Search: "no such words"}, "")},
		{"list, person and search", first(taskindex.Filter{Asked: person.Asked, Search: "deploy"}, "")},
		{"list, sorted by ended", first(taskindex.Filter{}, "ended")},
		{"list, sorted by seq", first(taskindex.Filter{}, "seq")},
		{"list, person sorted by ended", first(person, "ended")},
		{"list, deep page", func() error { return deepPage(s) }},
		{"count", count(taskindex.Filter{})},
		{"count, person", count(person)},
		{"count, last week", count(week)},
		{"count, search", count(taskindex.Filter{Search: "migrate"})},
		{"agent types", func() error { _, err := s.AgentTypes(ctx, taskindex.Filter{}); return err }},
		{"find key", func() error {
			if _, err := s.FindKey(ctx, "person-07@acme.com", "k"); !errors.Is(err, taskindex.ErrNotFound) {
				return err
			}
			return nil
		}},
		{"upsert one", func() error { return s.Upsert(ctx, perfBatch(500, 501)[0]) }},
		{"indexed", func() error { _, err := s.Indexed(ctx); return err }},
	} {
		took := best(t, st.run)
		t.Logf("%-36s %v", st.name, took.Round(100*time.Microsecond))
		if took > slowestRun && st.name != "indexed" {
			t.Errorf("%s took %v", st.name, took)
		}
	}
}

// deepPage reads the page that starts 90,000 tasks down the newest-first list, by the cursor of the page before it.
func deepPage(s taskindex.Store) error {
	rows := perfBatch(perfRows-90_000-1, perfRows-90_000)
	cursor := taskindex.ByQueued.NewCursor(taskindex.ByQueued.PositionOf(rows[0]))
	_, _, err := s.List(ctx, taskindex.Filter{}, taskindex.Page{Limit: 50, After: cursor})
	return err
}

// best is the quickest of three runs, so that a cold cache is not what is reported.
func best(t *testing.T, run func() error) time.Duration {
	t.Helper()
	var fastest time.Duration
	for i := 0; i < 3; i++ {
		start := time.Now()
		if err := run(); err != nil {
			t.Fatal(err)
		}
		if d := time.Since(start); i == 0 || d < fastest {
			fastest = d
		}
	}
	return fastest
}
