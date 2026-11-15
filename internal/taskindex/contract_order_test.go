package taskindex_test

import (
	"math/rand"
	"sync"
	"testing"

	"github.com/getlatentic/gusto/internal/taskindex"
)

func running(r taskindex.Row) taskindex.Row {
	r.State, r.Started = "running", r.Queued+1
	return r
}

func stopped(r taskindex.Row) taskindex.Row {
	r = running(r)
	r.StoppedBy = "bob"
	return r
}

func passed(r taskindex.Row, ended int64) taskindex.Row {
	r.State, r.Started, r.Ended = "passed", r.Queued+1, ended
	return r
}

var orderCases = []contractCase{
	{"a row that is behind does not replace the one held", func(t *testing.T, s taskindex.Store) {
		base := task("a", 1)
		done := passed(base, epoch.Unix()+50)
		put(t, s, done)
		for _, behind := range []taskindex.Row{base, running(base), stopped(base)} {
			put(t, s, behind)
		}
		if got, _ := s.Get(ctx, "a"); got != done {
			t.Fatalf("got %+v, want %+v", got, done)
		}
		put(t, s, passed(base, epoch.Unix()+40))
		if got, _ := s.Get(ctx, "a"); got != done {
			t.Fatalf("an earlier end replaced a later one: %+v", got)
		}
	}},
	{"a row that is further on, or as far on and not older, replaces the one held", func(t *testing.T, s taskindex.Store) {
		base := task("a", 1)
		for _, next := range []taskindex.Row{running(base), stopped(base), passed(base, 60), passed(base, 60), passed(base, 61)} {
			put(t, s, next)
			if got, _ := s.Get(ctx, "a"); got != next {
				t.Fatalf("got %+v, want %+v", got, next)
			}
		}
	}},
	{"a queued row is rewritten by an equal one with new fields", func(t *testing.T, s taskindex.Store) {
		base := task("a", 1)
		put(t, s, base)
		again := with(base, func(r *taskindex.Row) { r.Brief = "reworded" })
		put(t, s, again)
		if got, _ := s.Get(ctx, "a"); got != again {
			t.Fatalf("got %+v", got)
		}
	}},
	{"a batch holds each task at the furthest of its rows, and the other tasks are written", func(t *testing.T, s taskindex.Store) {
		a, b := task("a", 1), task("b", 2)
		put(t, s, passed(a, 70))
		put(t, s, b, a, running(a), running(b), b)
		gotA, _ := s.Get(ctx, "a")
		gotB, _ := s.Get(ctx, "b")
		if gotA != passed(a, 70) || gotB != running(b) {
			t.Fatalf("a = %+v, b = %+v", gotA, gotB)
		}
	}},
	{"an agent's turn keeps who took it, who handed it on and who pays", func(t *testing.T, s taskindex.Store) {
		turn := with(task("turn", 1), func(r *taskindex.Row) {
			r.Instance, r.Agent, r.HandedBy, r.Answers, r.Hops, r.PaidBy = "ins", "scout", "writer", "turn-0", 2, "owner@acme.com"
		})
		put(t, s, turn)
		got, err := s.Get(ctx, "turn")
		if err != nil || got != turn {
			t.Fatalf("got %+v, %v", got, err)
		}
		if indexed, _ := s.Indexed(ctx); indexed["turn"] != turn.Entry() {
			t.Fatalf("indexed = %+v", indexed["turn"])
		}
		plain := with(turn, func(r *taskindex.Row) { r.Agent, r.HandedBy, r.Answers, r.Hops, r.PaidBy = "", "", "", 0, "" })
		if plain.Sum() == turn.Sum() {
			t.Fatal("the agent fields are not in the sum")
		}
	}},
	{"writers racing for one task leave the furthest row, whatever order they land in", func(t *testing.T, s taskindex.Store) {
		const tasks, writers = 12, 8
		base := make([]taskindex.Row, tasks)
		var steps [][]taskindex.Row
		for i := range base {
			base[i] = task("t"+string(rune('a'+i)), int64(i))
		}
		for w := 0; w < writers; w++ {
			steps = append(steps, nil)
			for _, b := range base {
				steps[w] = append(steps[w], b, running(b), stopped(b), passed(b, epoch.Unix()+100))
			}
			rand.New(rand.NewSource(int64(w))).Shuffle(len(steps[w]), func(i, j int) { steps[w][i], steps[w][j] = steps[w][j], steps[w][i] })
		}
		var wg sync.WaitGroup
		errs := make(chan error, writers*len(steps[0]))
		for w := range steps {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < len(steps[w]); i += 6 {
					if err := s.UpsertMany(ctx, steps[w][i:min(i+6, len(steps[w]))]); err != nil {
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
		for _, b := range base {
			if got, _ := s.Get(ctx, b.ID); got != passed(b, epoch.Unix()+100) {
				t.Fatalf("%s ended as %+v", b.ID, got)
			}
		}
	}},
}
