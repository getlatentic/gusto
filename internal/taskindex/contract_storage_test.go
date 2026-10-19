package taskindex_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/getlatentic/gusto/internal/taskindex"
)

var storageCases = []contractCase{
	{"a row comes back as it went in", func(t *testing.T, s taskindex.Store) {
		full := taskindex.Row{ID: "full", State: "failed", Brief: "do 100% of it_", Repo: "acme/web", AgentType: "claude-code",
			Model: "sonnet", Instance: "ins", Thread: "3", Asked: "Ada@Acme.com", RequestKey: "key-1", StoppedBy: "bob",
			Team: "core", Seq: -4, Exit: 2, Queued: 1000, Started: 1010, Ended: 1020}
		put(t, s, full)
		got, err := s.Get(ctx, "full")
		if err != nil || got != full {
			t.Fatalf("got %+v, %v", got, err)
		}
	}},
	{"a missing task is not found", func(t *testing.T, s taskindex.Store) {
		if _, err := s.Get(ctx, "none"); !errors.Is(err, taskindex.ErrNotFound) {
			t.Fatalf("err = %v", err)
		}
		if _, err := s.FindKey(ctx, "ada", "key"); !errors.Is(err, taskindex.ErrNotFound) {
			t.Fatalf("err = %v", err)
		}
	}},
	{"upsert replaces the row a task has", func(t *testing.T, s taskindex.Store) {
		first := task("a", 1)
		if err := s.Upsert(ctx, first); err != nil {
			t.Fatal(err)
		}
		done := with(first, func(r *taskindex.Row) { r.State, r.Exit, r.Ended, r.StoppedBy = "failed", 1, 99, "bob" })
		if err := s.Upsert(ctx, done); err != nil {
			t.Fatal(err)
		}
		if got, _ := s.Get(ctx, "a"); got != done {
			t.Fatalf("got %+v", got)
		}
		if counts, _ := s.Count(ctx, taskindex.Filter{}); !reflect.DeepEqual(counts, map[string]int{"failed": 1}) {
			t.Fatalf("counts = %v", counts)
		}
	}},
	{"upsert many keeps the last row of a task named twice", func(t *testing.T, s taskindex.Store) {
		older := task("a", 1)
		put(t, s, older, task("b", 2), with(older, func(r *taskindex.Row) { r.State = "running" }))
		if got, _ := s.Get(ctx, "a"); got.State != "running" {
			t.Fatalf("got %+v", got)
		}
	}},
	{"upsert many takes more rows than one statement holds", func(t *testing.T, s taskindex.Store) {
		put(t, s, numbered(1203)...)
		put(t, s, numbered(1203)[600:900]...)
		if counts, _ := s.Count(ctx, taskindex.Filter{}); counts["queued"] != 1203 {
			t.Fatalf("counts = %v", counts)
		}
	}},
	{"delete removes a row and forgives a missing one", func(t *testing.T, s taskindex.Store) {
		put(t, s, task("a", 1), task("b", 2))
		for _, id := range []string{"a", "a", "never"} {
			if err := s.Delete(ctx, id); err != nil {
				t.Fatal(err)
			}
		}
		same(t, list(t, s, taskindex.Filter{}, taskindex.Page{}), []string{"b"})
	}},
	{"find key matches the asker's own request only", func(t *testing.T, s taskindex.Store) {
		put(t, s, with(task("a", 1), func(r *taskindex.Row) { r.RequestKey = "k1" }),
			with(task("b", 2), func(r *taskindex.Row) { r.RequestKey, r.Asked = "k1", "bob@acme.com" }),
			with(task("c", 3), func(r *taskindex.Row) { r.RequestKey = "" }))
		if got, err := s.FindKey(ctx, "ada@acme.com", "k1"); err != nil || got.ID != "a" {
			t.Fatalf("got %+v, %v", got, err)
		}
		if got, err := s.FindKey(ctx, "bob@acme.com", "k1"); err != nil || got.ID != "b" {
			t.Fatalf("got %+v, %v", got, err)
		}
		for _, c := range [][2]string{{"ADA@acme.com", "k1"}, {"ada@acme.com", "k2"}, {"ada@acme.com", ""}, {"", ""}} {
			if _, err := s.FindKey(ctx, c[0], c[1]); !errors.Is(err, taskindex.ErrNotFound) {
				t.Errorf("FindKey(%q, %q) err = %v", c[0], c[1], err)
			}
		}
	}},
	{"indexed says what a rebuild compares", func(t *testing.T, s taskindex.Store) {
		a := with(task("a", 1), func(r *taskindex.Row) { r.State, r.Ended = "passed", 77 })
		b := task("b", 2)
		put(t, s, a, b)
		got, err := s.Indexed(ctx)
		want := map[string]taskindex.Entry{"a": a.Entry(), "b": b.Entry()}
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v, %v; want %v", got, err, want)
		}
		if got["a"].State != "passed" || got["a"].Ended != 77 || got["a"].Seq != 1 {
			t.Fatalf("entry = %+v", got["a"])
		}
		changed := with(b, func(r *taskindex.Row) { r.StoppedBy = "bob" })
		put(t, s, changed)
		if got, _ := s.Indexed(ctx); got["b"].Sum == b.Entry().Sum || got["b"].Sum != changed.Entry().Sum {
			t.Fatal("a change to any field changes the sum")
		}
	}},
	{"reset deletes every row and counts them", func(t *testing.T, s taskindex.Store) {
		put(t, s, numbered(7)...)
		if n, err := s.Reset(ctx); err != nil || n != 7 {
			t.Fatalf("reset = %d %v", n, err)
		}
		if got, _ := s.Indexed(ctx); len(got) != 0 {
			t.Fatalf("indexed = %v", got)
		}
		if n, _ := s.Reset(ctx); n != 0 {
			t.Fatalf("second reset = %d", n)
		}
	}},
	{"an empty index answers with nothing", func(t *testing.T, s taskindex.Store) {
		rows, next, err := s.List(ctx, taskindex.Filter{}, taskindex.Page{})
		if err != nil || len(rows) != 0 || next != "" {
			t.Fatalf("list = %v %q %v", rows, next, err)
		}
		if counts, err := s.Count(ctx, taskindex.Filter{}); err != nil || len(counts) != 0 {
			t.Fatalf("counts = %v %v", counts, err)
		}
		if types, err := s.AgentTypes(ctx, taskindex.Filter{}); err != nil || len(types) != 0 {
			t.Fatalf("types = %v %v", types, err)
		}
	}},
	{"a cancelled context stops every call", func(t *testing.T, s taskindex.Store) {
		put(t, s, task("a", 1))
		done, cancel := context.WithCancel(ctx)
		cancel()
		_, _, listErr := s.List(done, taskindex.Filter{}, taskindex.Page{})
		_, countErr := s.Count(done, taskindex.Filter{})
		_, getErr := s.Get(done, "a")
		_, indexedErr := s.Indexed(done)
		_, typesErr := s.AgentTypes(done, taskindex.Filter{})
		upsertErr := s.Upsert(done, task("b", 2))
		for name, err := range map[string]error{"list": listErr, "count": countErr, "get": getErr, "indexed": indexedErr,
			"types": typesErr, "upsert": upsertErr} {
			if !errors.Is(err, context.Canceled) {
				t.Errorf("%s: err = %v", name, err)
			}
		}
		if _, err := s.Get(ctx, "b"); err == nil {
			t.Fatal("a cancelled upsert held a row")
		}
	}},
}
