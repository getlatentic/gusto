package apps

import (
	"errors"
	"sync"
	"testing"
)

var notes = Sharer{Instance: "20260930-1200-notes", Owner: "ada@acme.com"}

// inState is a new app put in a state directly, so each move can be tried from where it starts.
func inState(t *testing.T, store *Store, state State) App {
	t.Helper()
	app, err := store.New(notes, "node a.js", 8080, 100)
	if err != nil {
		t.Fatal(err)
	}
	app.State = state
	if err := store.write(app); err != nil {
		t.Fatal(err)
	}
	return app
}

// A removed app is not made live by a start that finishes late, and an app that ended does not start again.
func TestAnAppMovesOnlyWhereItMayGo(t *testing.T) {
	store, _ := Open(t.TempDir())
	for _, test := range []struct {
		from, to State
		allowed  bool
	}{
		{Starting, Live, true},
		{Starting, Failed, true},
		{Starting, Removing, true},
		{Live, Stopped, true},
		{Live, Removing, true},
		{Removing, Live, false},
		{Removing, Failed, false},
		{Failed, Live, false},
		{Stopped, Live, false},
		{Live, Starting, false},
	} {
		app := inState(t, store, test.from)
		got, err := store.Change(app.ID, func(a *App) { a.State = test.to })
		want := test.from
		if test.allowed {
			want = test.to
		}
		if test.allowed != (err == nil) || (!test.allowed && !errors.Is(err, ErrBadMove)) {
			t.Errorf("%s to %s: %v", test.from, test.to, err)
		}
		if kept, _ := store.Get(app.ID); kept.State != want || got.State != want {
			t.Errorf("%s to %s: kept %s and answered %s, want %s", test.from, test.to, kept.State, got.State, want)
		}
	}
}

// A change that leaves the state alone is always kept.
func TestAChangeThatKeepsTheStateIsKept(t *testing.T) {
	store, _ := Open(t.TempDir())
	app := inState(t, store, Removing)
	got, err := store.Change(app.ID, func(a *App) { a.Note = "still going" })
	if err != nil || got.Note != "still going" {
		t.Errorf("%+v %v", got, err)
	}
}

// Pages asked for at once are one page: the check and the add are one step.
func TestPagesAddedAtOnceAreOnePage(t *testing.T) {
	store, _ := Open(t.TempDir())
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if _, err := store.NewPage(notes, "site", 3); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if all, _ := store.Of(notes.Instance); len(all) != 1 {
		t.Errorf("%d pages were made of one folder", len(all))
	}
}
