package taskindex_test

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/getlatentic/gusto/internal/taskindex"
)

var sortCases = []contractCase{
	{"queued is the default and breaks ties by arrival and then by id", func(t *testing.T, s taskindex.Store) {
		put(t, s,
			with(task("a", 10), func(r *taskindex.Row) { r.Seq = 1 }),
			with(task("b", 10), func(r *taskindex.Row) { r.Seq = 2 }),
			with(task("c", 10), func(r *taskindex.Row) { r.Seq = 2 }),
			task("d", 5))
		same(t, list(t, s, taskindex.Filter{}, taskindex.Page{}), []string{"c", "b", "a", "d"})
		same(t, list(t, s, taskindex.Filter{}, taskindex.Page{Sort: "queued"}), []string{"c", "b", "a", "d"})
	}},
	{"ended puts tasks that have not ended last", func(t *testing.T, s taskindex.Store) {
		put(t, s,
			with(task("a", 1), func(r *taskindex.Row) { r.Ended = 50 }),
			with(task("b", 2), func(r *taskindex.Row) { r.Ended = 90 }),
			task("c", 3), task("d", 4),
			with(task("e", 5), func(r *taskindex.Row) { r.Ended = 90 }))
		same(t, list(t, s, taskindex.Filter{}, taskindex.Page{Sort: "ended"}), []string{"e", "b", "a", "d", "c"})
	}},
	{"seq orders by arrival, and negative numbers come first-in", func(t *testing.T, s taskindex.Store) {
		put(t, s,
			with(task("a", 1), func(r *taskindex.Row) { r.Seq = -2 }),
			with(task("b", 2), func(r *taskindex.Row) { r.Seq = 7 }),
			with(task("c", 3), func(r *taskindex.Row) { r.Seq = 0 }))
		same(t, list(t, s, taskindex.Filter{}, taskindex.Page{Sort: "seq"}), []string{"b", "c", "a"})
	}},
	{"an unknown sort is refused", func(t *testing.T, s taskindex.Store) {
		_, _, err := s.List(ctx, taskindex.Filter{}, taskindex.Page{Sort: "brief"})
		if !errors.Is(err, taskindex.ErrBadSort) || !strings.Contains(err.Error(), "queued, ended or seq") {
			t.Fatalf("err = %v", err)
		}
	}},
}

// everyPage reads a listing a page at a time and gives the ids in the order they came.
func everyPage(t *testing.T, s taskindex.Store, f taskindex.Filter, sort string, size int) []string {
	t.Helper()
	var all []string
	var after taskindex.Cursor
	for pages := 0; pages < 1000; pages++ {
		rows, next, err := s.List(ctx, f, taskindex.Page{Limit: size, After: after, Sort: sort})
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) > size {
			t.Fatalf("a page of %d rows for a limit of %d", len(rows), size)
		}
		all = append(all, idsOf(rows)...)
		if next == "" {
			return all
		}
		after = next
	}
	t.Fatal("paging does not end")
	return nil
}

var pagingCases = []contractCase{
	{"pages join up without gaps or repeats, in every sort", func(t *testing.T, s taskindex.Store) {
		rows := numbered(95)
		for i := range rows {
			rows[i].Ended = int64(i % 7)
			rows[i].Seq = int64(i % 11)
			rows[i].Queued = epoch.Unix() + int64(i%13)
		}
		put(t, s, rows...)
		for _, sort := range []string{"", "queued", "ended", "seq"} {
			whole := list(t, s, taskindex.Filter{}, taskindex.Page{Limit: 1000, Sort: sort})
			for _, size := range []int{1, 7, 50, 95, 96} {
				same(t, everyPage(t, s, taskindex.Filter{}, sort, size), whole)
			}
		}
	}},
	{"a filter and its pages agree", func(t *testing.T, s taskindex.Store) {
		rows := numbered(60)
		for i := range rows {
			if i%3 == 0 {
				rows[i].Asked, rows[i].State = "bob@acme.com", "passed"
			}
		}
		put(t, s, rows...)
		f := taskindex.Filter{Asked: "BOB@acme.com", States: []string{"passed"}}
		whole := list(t, s, f, taskindex.Page{Limit: 1000})
		if len(whole) != 20 {
			t.Fatalf("whole = %v", whole)
		}
		same(t, everyPage(t, s, f, "", 6), whole)
	}},
	{"the last page has no cursor, even when it is exactly full", func(t *testing.T, s taskindex.Store) {
		put(t, s, numbered(10)...)
		if _, next, _ := s.List(ctx, taskindex.Filter{}, taskindex.Page{Limit: 10}); next != "" {
			t.Fatalf("next = %q", next)
		}
		if _, next, _ := s.List(ctx, taskindex.Filter{}, taskindex.Page{Limit: 9}); next == "" {
			t.Fatal("no next for a page that is not the last")
		}
	}},
	{"a cursor continues after its row however the rows around it change", func(t *testing.T, s taskindex.Store) {
		put(t, s, numbered(10)...)
		rows, next, _ := s.List(ctx, taskindex.Filter{}, taskindex.Page{Limit: 3})
		same(t, idsOf(rows), []string{"t009", "t008", "t007"})
		if err := s.Delete(ctx, "t007"); err != nil {
			t.Fatal(err)
		}
		put(t, s, task("t100", 100), task("t0065", 6))
		same(t, list(t, s, taskindex.Filter{}, taskindex.Page{Limit: 3, After: next}), []string{"t0065", "t006", "t005"})
	}},
	{"a cursor of another sort is refused, and so is rubbish", func(t *testing.T, s taskindex.Store) {
		put(t, s, numbered(5)...)
		_, queued, _ := s.List(ctx, taskindex.Filter{}, taskindex.Page{Limit: 2})
		_, ended, _ := s.List(ctx, taskindex.Filter{}, taskindex.Page{Limit: 2, Sort: "ended"})
		for _, c := range []struct {
			cursor taskindex.Cursor
			sort   string
		}{{queued, "ended"}, {queued, "seq"}, {ended, ""}, {ended, "queued"}, {"!!", ""}, {taskindex.Cursor("bm9wZQ"), "ended"}} {
			if _, _, err := s.List(ctx, taskindex.Filter{}, taskindex.Page{After: c.cursor, Sort: c.sort}); !errors.Is(err, taskindex.ErrBadCursor) {
				t.Errorf("cursor %q in sort %q: err = %v", c.cursor, c.sort, err)
			}
		}
	}},
	{"an id with any text in it pages", func(t *testing.T, s taskindex.Store) {
		odd := []string{"a|b", "a|b|c", "ünï", "sp ace", "x%y_z", `q"r'`}
		var rows []taskindex.Row
		for _, id := range odd {
			rows = append(rows, task(id, 1))
		}
		put(t, s, rows...)
		whole := list(t, s, taskindex.Filter{}, taskindex.Page{Limit: 100})
		if len(whole) != len(odd) {
			t.Fatalf("whole = %v", whole)
		}
		same(t, everyPage(t, s, taskindex.Filter{}, "", 2), whole)
	}},
	{"the limit is 50 when it is not given and 1000 at most", func(t *testing.T, s taskindex.Store) {
		put(t, s, numbered(1100)...)
		for limit, want := range map[int]int{0: 50, -3: 50, 7: 7, 1000: 1000, 5000: 1000} {
			if got := len(list(t, s, taskindex.Filter{}, taskindex.Page{Limit: limit})); got != want {
				t.Errorf("limit %d gave %d rows, want %d", limit, got, want)
			}
		}
	}},
}

var concurrencyCases = []contractCase{
	{"many goroutines write and list at once", func(t *testing.T, s taskindex.Store) {
		const writers, perWriter = 4, 60
		var wg sync.WaitGroup
		errs := make(chan error, 64)
		for w := 0; w < writers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < perWriter; i += 10 {
					var rows []taskindex.Row
					for j := i; j < i+10; j++ {
						rows = append(rows, task(fmt.Sprintf("w%d-%03d", w, j), int64(j)))
					}
					if err := s.UpsertMany(ctx, rows); err != nil {
						errs <- err
					}
				}
			}()
		}
		for r := 0; r < 4; r++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < 8; i++ {
					if _, _, err := s.List(ctx, taskindex.Filter{Search: "w"}, taskindex.Page{Limit: 10}); err != nil {
						errs <- err
					}
					if _, err := s.Count(ctx, taskindex.Filter{}); err != nil {
						errs <- err
					}
				}
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Error(err)
		}
		if got, _ := s.Count(ctx, taskindex.Filter{}); got["queued"] != writers*perWriter {
			t.Fatalf("counts = %v", got)
		}
	}},
}
