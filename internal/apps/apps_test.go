package apps

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAnAppIsMadeStartingKeptChangedAndFoundByItsAgent(t *testing.T) {
	root := filepath.Join(t.TempDir(), "apps")
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return time.Unix(1000, 0) }
	one, err := store.New("20260930-1200-notes", "Ada@Acme.com", "node server.js", 8080)
	if err != nil {
		t.Fatal(err)
	}
	if !Valid(one.ID) || one.State != Starting || one.Owner != "ada@acme.com" || one.Started != 1000 {
		t.Fatalf("%+v", one)
	}
	store.now = func() time.Time { return time.Unix(2000, 0) }
	two, _ := store.New("20260930-1200-notes", "ada@acme.com", "python3 -m http.server $PORT", 9000)
	other, _ := store.New("20260930-1300-other", "bob@acme.com", "npm start", 3000)

	changed, err := store.Change(one.ID, func(a *App) {
		a.State, a.Host, a.Target = Live, "default--sb--app.openshell.localhost:18080", "127.0.0.1:18080"
	})
	if err != nil || changed.State != Live || changed.Host == "" {
		t.Fatalf("%+v %v", changed, err)
	}
	reopened, _ := Open(root)
	if got, err := reopened.Get(one.ID); err != nil || got.State != Live || got.Target != "127.0.0.1:18080" {
		t.Errorf("a change was not kept: %+v %v", got, err)
	}
	mine, _ := store.Of("20260930-1200-notes")
	if len(mine) != 2 || mine[0].ID != two.ID || mine[1].ID != one.ID {
		t.Errorf("an agent's apps, newest first: %+v", mine)
	}
	if theirs, _ := store.Of("20260930-1300-other"); len(theirs) != 1 || theirs[0].ID != other.ID {
		t.Errorf("%+v", theirs)
	}
	if err := store.Delete(two.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(two.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("%v", err)
	}
}

// An id names a file, so nothing that is not one may be used to reach a file.
func TestOnlyAnIdOfTheRightShapeNamesAnApp(t *testing.T) {
	store, _ := Open(t.TempDir())
	os.WriteFile(filepath.Join(filepath.Dir(store.dir), "secret.json"), []byte(`{"id":"x"}`), 0o600)
	for _, id := range []string{"", "../secret", "..%2fsecret", "0123456789a", "zzzzzzzzzz", "0123456789/../secret"} {
		if _, err := store.Get(id); !errors.Is(err, ErrNotFound) {
			t.Errorf("%q gave %v", id, err)
		}
		if err := store.Delete(id); !errors.Is(err, ErrNotFound) {
			t.Errorf("deleting %q gave %v", id, err)
		}
		if _, err := store.Change(id, func(*App) {}); !errors.Is(err, ErrNotFound) {
			t.Errorf("changing %q gave %v", id, err)
		}
	}
}

func TestAFileThatIsNotAnAppIsSaidSoAndNotSkipped(t *testing.T) {
	store, _ := Open(t.TempDir())
	os.WriteFile(filepath.Join(store.dir, "0123456789.json"), []byte("not json"), 0o600)
	if _, err := store.All(); err == nil {
		t.Error("a broken record was skipped")
	}
}
