package taskindex_test

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/taskindex"
)

type contractCase struct {
	name string
	run  func(t *testing.T, s taskindex.Store)
}

// TestContract holds every store to the same rules: the in-memory fake, SQLite, and Postgres when
// GUSTO_TEST_POSTGRES_URL names a server.
func TestContract(t *testing.T) {
	var cases []contractCase
	for _, group := range [][]contractCase{storageCases, filterCases, searchCases, sortCases, pagingCases, concurrencyCases} {
		cases = append(cases, group...)
	}
	for _, b := range backends() {
		t.Run(b.name, func(t *testing.T) {
			for _, c := range cases {
				t.Run(c.name, func(t *testing.T) { c.run(t, b.open(t)) })
			}
		})
	}
}

var epoch = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

// task is a queued task asked for at the given second after the epoch, by ada, with every other field set.
func task(id string, second int64) taskindex.Row {
	return taskindex.Row{ID: id, State: "queued", Brief: "brief of " + id, Repo: "acme/web", AgentType: "codex", Model: "gpt",
		Asked: "ada@acme.com", Seq: second, Queued: epoch.Unix() + second}
}

func with(r taskindex.Row, change func(*taskindex.Row)) taskindex.Row {
	change(&r)
	return r
}

func put(t *testing.T, s taskindex.Store, rows ...taskindex.Row) {
	t.Helper()
	if err := s.UpsertMany(ctx, rows); err != nil {
		t.Fatal(err)
	}
}

func idsOf(rows []taskindex.Row) []string {
	ids := []string{}
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	return ids
}

func list(t *testing.T, s taskindex.Store, f taskindex.Filter, p taskindex.Page) []string {
	t.Helper()
	rows, _, err := s.List(ctx, f, p)
	if err != nil {
		t.Fatal(err)
	}
	return idsOf(rows)
}

func same(t *testing.T, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func numbered(n int) []taskindex.Row {
	rows := make([]taskindex.Row, n)
	for i := range rows {
		rows[i] = task(fmt.Sprintf("t%03d", i), int64(i))
	}
	return rows
}
