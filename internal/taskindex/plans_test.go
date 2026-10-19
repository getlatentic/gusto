package taskindex

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"
)

// A page of any filter and order reads from an index, never the whole table, whichever filters it names. This runs
// SQLite's own plan for each statement the store makes.
func TestEveryListIsServedByAnIndex(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(t.Context(), "", dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	rows := make([]Row, 2000)
	for i := range rows {
		rows[i] = Row{ID: fmt.Sprintf("t%05d", i), State: []string{"passed", "failed", "running"}[i%3], Asked: fmt.Sprintf("p%d@x", i%20),
			Team: fmt.Sprintf("team-%d", i%5), Repo: fmt.Sprintf("r%d", i%7), AgentType: fmt.Sprintf("a%d", i%4), Instance: fmt.Sprintf("i%d", i%50),
			Seq: int64(i), Queued: int64(i), Ended: int64(i / 2)}
	}
	if err := s.UpsertMany(t.Context(), rows); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", sqlitePathIn(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`ANALYZE`); err != nil {
		t.Fatal(err)
	}
	cursor := ByQueued.NewCursor(Position{Key: []int64{1000, 1000}, ID: "t01000"})
	for name, f := range map[string]Filter{
		"none": {}, "person": {Asked: "p3@x"}, "state": {States: []string{"failed"}}, "states": {States: []string{"failed", "running"}},
		"team": {Team: "team-1"}, "repo": {Repo: "r2"}, "agent type": {AgentType: "a1"}, "instance": {Instance: "i7"},
		"time": {QueuedFrom: time.Unix(500, 0), QueuedTo: time.Unix(900, 0)}, "person and state": {Asked: "p3@x", States: []string{"failed"}},
	} {
		for _, page := range []Page{{}, {Sort: "ended"}, {Sort: "seq"}, {After: cursor}} {
			query, args, _, err := s.(*store).listQuery(f, page)
			if err != nil {
				t.Fatal(err)
			}
			if scan := scansTable(t, db, query, args); scan != "" {
				t.Errorf("%s, %+v: %s", name, page, scan)
			}
		}
	}
}

// scansTable is the plan line that reads the table whole, or nothing when every read is by an index.
func scansTable(t *testing.T, db *sql.DB, query string, args []any) string {
	t.Helper()
	rows, err := db.Query(`EXPLAIN QUERY PLAN `+query, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(detail, "SCAN tasks") && !strings.Contains(detail, "INDEX") {
			return detail
		}
	}
	return ""
}

func sqlitePathIn(dir string) string { return dir + "/tasks.db" }
