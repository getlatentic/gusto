package dbkit_test

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/dbkit"
	"github.com/getlatentic/gusto/internal/dbkit/dbkittest"
)

func TestTimesRoundTripInUTC(t *testing.T) {
	dbkittest.Each(t, func(t *testing.T, db *dbkit.DB) {
		col := `INTEGER`
		if db.Dialect() == dbkit.Postgres {
			col = `TIMESTAMPTZ`
		}
		if _, err := db.ExecContext(ctx, `CREATE TABLE stamps (id INTEGER PRIMARY KEY, at `+col+`, maybe `+col+`)`); err != nil {
			t.Fatal(err)
		}
		local := time.Date(2026, 10, 19, 8, 30, 15, 123456000, time.FixedZone("x", 3600))
		if _, err := db.ExecContext(ctx, `INSERT INTO stamps (id, at) VALUES (1, ?)`, db.TimeArg(local)); err != nil {
			t.Fatal(err)
		}
		var at dbkit.Time
		var maybe sqlNullTime
		if err := db.QueryRowContext(ctx, `SELECT at, maybe FROM stamps`).Scan(&at, &maybe); err != nil {
			t.Fatal(err)
		}
		if !at.Equal(local) || at.Location() != time.UTC || maybe.Valid {
			t.Fatalf("at = %v maybe = %v", at, maybe)
		}
		now := dbkit.Now()
		if _, err := db.ExecContext(ctx, `UPDATE stamps SET at = ?`, db.TimeArg(now)); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `SELECT at, maybe FROM stamps`).Scan(&at, &maybe); err != nil || !at.Equal(now) {
			t.Fatalf("now = %v back = %v %v", now, at.Time, err)
		}
	})
}

func TestTimeRefusesOtherKinds(t *testing.T) {
	var c dbkit.Time
	if err := c.Scan("yesterday"); err == nil {
		t.Fatal("a string scanned")
	}
}

func TestUpsertInsertsUpdatesAndSkips(t *testing.T) {
	dbkittest.Each(t, func(t *testing.T, db *dbkit.DB) {
		if _, err := db.ExecContext(ctx, `CREATE TABLE kv (k TEXT PRIMARY KEY, v TEXT NOT NULL, n INTEGER NOT NULL)`); err != nil {
			t.Fatal(err)
		}
		cols := []string{"k", "v", "n"}
		skip := dbkit.Upsert{Table: "kv", Columns: cols, Conflict: []string{"k"}}
		set := dbkit.Upsert{Table: "kv", Columns: cols, Conflict: []string{"k"}, Update: []string{"v"}}
		write := func(u dbkit.Upsert, args ...any) {
			t.Helper()
			if _, err := db.ExecContext(ctx, u.SQL(len(args)/3), args...); err != nil {
				t.Fatal(err)
			}
		}
		write(skip, "a", "one", 1, "b", "two", 2)
		write(skip, "a", "changed", 9)
		write(set, "b", "TWO", 99, "c", "three", 3)
		got := map[string]string{}
		rows, err := db.QueryContext(ctx, `SELECT k, v || ':' || n FROM kv`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var k, v string
			if err := rows.Scan(&k, &v); err != nil {
				t.Fatal(err)
			}
			got[k] = v
		}
		want := map[string]string{"a": "one:1", "b": "TWO:2", "c": "three:3"}
		for k, v := range want {
			if got[k] != v {
				t.Errorf("%s = %q, want %q", k, got[k], v)
			}
		}
	})
}

func TestUpsertSQLShape(t *testing.T) {
	got := dbkit.Upsert{Table: "t", Columns: []string{"a", "b"}, Conflict: []string{"a"}, Update: []string{"b"}}.SQL(2)
	want := "INSERT INTO t (a, b) VALUES (?,?),(?,?) ON CONFLICT (a) DO UPDATE SET b = excluded.b"
	if got != want {
		t.Fatalf("got %s", got)
	}
}

type profile struct {
	Name  string            `json:"name"`
	Tags  []string          `json:"tags"`
	Extra map[string]string `json:"extra,omitempty"`
}

func TestJSONColumnRoundTripsAStruct(t *testing.T) {
	dbkittest.Each(t, func(t *testing.T, db *dbkit.DB) {
		col := `TEXT`
		if db.Dialect() == dbkit.Postgres {
			col = `JSONB`
		}
		if _, err := db.ExecContext(ctx, `CREATE TABLE docs (id INTEGER PRIMARY KEY, body `+col+`)`); err != nil {
			t.Fatal(err)
		}
		in := profile{Name: "Ada", Tags: []string{"a", "b"}, Extra: map[string]string{"k": "v"}}
		if _, err := db.ExecContext(ctx, `INSERT INTO docs (id, body) VALUES (1, ?), (2, NULL)`, dbkit.JSON[profile]{V: in}); err != nil {
			t.Fatal(err)
		}
		var out, none dbkit.JSON[profile]
		if err := db.QueryRowContext(ctx, `SELECT body FROM docs WHERE id = 1`).Scan(&out); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `SELECT body FROM docs WHERE id = 2`).Scan(&none); err != nil {
			t.Fatal(err)
		}
		if out.V.Name != "Ada" || len(out.V.Tags) != 2 || out.V.Extra["k"] != "v" || none.V.Name != "" {
			t.Fatalf("out = %+v none = %+v", out.V, none.V)
		}
	})
}

func TestJSONColumnRefusesBadInput(t *testing.T) {
	var j dbkit.JSON[profile]
	if err := j.Scan(int64(3)); err == nil {
		t.Fatal("an integer scanned")
	}
	if err := j.Scan("{not json"); err == nil {
		t.Fatal("bad JSON scanned")
	}
}

func TestLikeMatchesASubstringWithoutCaseAndTakesWildcardsLiterally(t *testing.T) {
	dbkittest.Each(t, func(t *testing.T, db *dbkit.DB) {
		if _, err := db.ExecContext(ctx, `CREATE TABLE names (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`); err != nil {
			t.Fatal(err)
		}
		for i, n := range []string{"Alpha Beta", "50% off", "snake_case", `back\slash`, "plain"} {
			if _, err := db.ExecContext(ctx, `INSERT INTO names VALUES (?, ?)`, i, n); err != nil {
				t.Fatal(err)
			}
		}
		find := func(s string) []string {
			rows, err := db.QueryContext(ctx, `SELECT name FROM names WHERE `+db.Like("name")+` ORDER BY id`, dbkit.ContainsPattern(s))
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			var out []string
			for rows.Next() {
				var n string
				if err := rows.Scan(&n); err != nil {
					t.Fatal(err)
				}
				out = append(out, n)
			}
			return out
		}
		for in, want := range map[string]string{
			"ALPHA": "Alpha Beta", "a b": "Alpha Beta", "50%": "50% off", "e_c": "snake_case", `k\s`: `back\slash`,
		} {
			if got := strings.Join(find(in), ","); got != want {
				t.Errorf("find(%q) = %q, want %q", in, got, want)
			}
		}
		for _, wild := range []string{"%", "_", "p_ain", "5%f"} {
			if got := find(wild); len(got) > 1 || (wild == "p_ain" || wild == "5%f") && len(got) != 0 {
				t.Errorf("find(%q) = %v: a wildcard acted as one", wild, got)
			}
		}
	})
}

func TestRebindOnlyChangesPostgres(t *testing.T) {
	dbkittest.Each(t, func(t *testing.T, db *dbkit.DB) {
		got := db.Rebind("SELECT ? + ?")
		want := "SELECT ? + ?"
		if db.Dialect() == dbkit.Postgres {
			want = "SELECT $1 + $2"
		}
		if got != want {
			t.Fatalf("got %s", got)
		}
	})
}

type sqlNullTime = sql.Null[dbkit.Time]
