package usage_test

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/usage"
)

const (
	perfRows     = 200_000
	perfSessions = 20_000
)

// perfRows spreads calls over 30 days: 20,000 sessions of ten calls, 60 people, a few engines and models.
func perfBatch(from, to int) []usage.Row {
	engines := []string{"claude-code", "codex", "pi", "opencode"}
	providers := []string{"anthropic", "openai", "bedrock"}
	rows := make([]usage.Row, 0, to-from)
	for i := from; i < to; i++ {
		sess := i % perfSessions
		r := callRow(fmt.Sprintf("task-%d", sess), i/perfSessions, fmt.Sprintf("sess-%05d", sess), 0)
		r.At = epoch.Add(time.Duration(i) * 13 * time.Second)
		r.Person = fmt.Sprintf("person-%02d", sess%60)
		r.Engine = engines[sess%len(engines)]
		r.Provider = providers[i%len(providers)]
		r.Model = fmt.Sprintf("model-%d", i%8)
		r.Team = fmt.Sprintf("team-%d", sess%6)
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
		u := freshSchemaURL(t)
		s, err := usage.Open(ctx, u, "")
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
	if _, err := db.Exec(`VACUUM ANALYZE usage_calls`); err != nil {
		t.Fatal(err)
	}
}

func perf(t *testing.T, s usage.Store, settle func()) {
	start := time.Now()
	for from := 0; from < perfRows; from += 5000 {
		add(t, s, perfBatch(from, from+5000)...)
	}
	t.Logf("inserted %d rows in %v", perfRows, time.Since(start).Round(time.Millisecond))
	settle()
	week := usage.Filter{From: epoch.Add(23 * 24 * time.Hour)}
	steps := []struct {
		name string
		run  func() error
	}{
		{"sessions, first page", func() error { return pageOf(s, usage.Filter{}, "") }},
		{"sessions, person filter", func() error { return pageOf(s, usage.Filter{Person: "person-07"}, "") }},
		{"sessions, search for a person", func() error { return pageOf(s, usage.Filter{Search: "person-07"}, "") }},
		{"sessions, search for a model", func() error { return pageOf(s, usage.Filter{Search: "MODEL-3"}, "") }},
		{"sessions, last week", func() error { return pageOf(s, week, "") }},
		{"totals", func() error { _, err := s.Totals(ctx, usage.Filter{}); return err }},
		{"totals, person filter", func() error { _, err := s.Totals(ctx, usage.Filter{Person: "person-07"}); return err }},
		{"totals, search", func() error { _, err := s.Totals(ctx, usage.Filter{Search: "person-07"}); return err }},
		{"totals, last week", func() error { _, err := s.Totals(ctx, week); return err }},
		{"facets", func() error { _, err := s.Facets(ctx, usage.Filter{}); return err }},
		{"facets, last week", func() error { _, err := s.Facets(ctx, week); return err }},
		{"export, person filter", func() error {
			return s.Export(ctx, usage.Filter{Person: "person-07"}, func(usage.Row) error { return nil })
		}},
		{"export, everything", func() error { return s.Export(ctx, usage.Filter{}, func(usage.Row) error { return nil }) }},
		{"sessions by cost", func() error { return pageOf(s, usage.Filter{}, "cost") }},
		{"sessions by calls, person filter", func() error { return pageOf(s, usage.Filter{Person: "person-07"}, "calls") }},
		{"indexed", func() error { _, err := s.Indexed(ctx); return err }},
	}
	for _, st := range steps {
		took := best(t, st.run)
		t.Logf("%-32s %v", st.name, took.Round(time.Millisecond))
		if took > time.Second && !strings.HasPrefix(st.name, "export") {
			t.Errorf("%s took %v", st.name, took)
		}
	}
}

func pageOf(s usage.Store, f usage.Filter, sortName string) error {
	_, _, err := s.Sessions(ctx, f, usage.Page{Limit: 50, Sort: sortName})
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
