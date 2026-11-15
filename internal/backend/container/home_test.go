package container

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/backend"
	"github.com/getlatentic/gusto/internal/computer"
	"github.com/getlatentic/gusto/pkg/task"
)

const (
	testOrg    = "acme-research-com"
	testPerson = "bob@acme-research.com"
)

// homeBackend is a backend that keeps homes, whose count of a home is what the test says, and its turn.
func homeBackend(t *testing.T, limit int64, count func(context.Context, string) (int64, error)) (*Backend, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "computers")
	b := New(Config{Org: testOrg, Homes: root, HomeLimit: limit})
	b.homes.count = count
	return b, root
}

func fixed(bytes int64) func(context.Context, string) (int64, error) {
	return func(context.Context, string) (int64, error) { return bytes, nil }
}

func aTurn(t *testing.T) backend.Turn {
	t.Helper()
	dir := task.Dir(filepath.Join(t.TempDir(), "20261107-0900-a-turn"))
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	return backend.Turn{Dir: dir, Spec: task.Spec{Session: "20261107-0900-opencode"}, Person: testPerson}
}

func homeNoteOf(t *testing.T, dir task.Dir) homeNote {
	t.Helper()
	text, err := os.ReadFile(dir.OutFile("home.json"))
	if err != nil {
		t.Fatalf("the record does not say why: %v", err)
	}
	var note homeNote
	if err := json.Unmarshal(text, &note); err != nil {
		t.Fatal(err)
	}
	return note
}

// A turn in a home that is over its limit does not start, and its record and its error say why.
func TestATurnInAFullHomeIsRefusedAndTheRecordSaysWhy(t *testing.T) {
	b, _ := homeBackend(t, 1000, fixed(5000))
	turn := aTurn(t)
	id, _ := b.homeOf(turn)
	ran := false
	_, err := b.homes.guard(context.Background(), turn.Dir, id, func(context.Context) (int, error) { ran = true; return 0, nil })
	if !errors.Is(err, ErrHomeFull) || ran {
		t.Fatalf("ran %v, error %v", ran, err)
	}
	if note := homeNoteOf(t, turn.Dir); note.Outcome != "refused" || note.Used != 5000 || note.Limit != 1000 {
		t.Errorf("%+v", note)
	}
	if len(b.homes.active) != 0 {
		t.Error("a refused turn is still counted as running")
	}
}

// A turn that fills its home is stopped, and says so.
func TestATurnThatFillsItsHomeIsStopped(t *testing.T) {
	var grown atomic.Int64
	grown.Store(100)
	b, _ := homeBackend(t, 1000, func(context.Context, string) (int64, error) { return grown.Add(400), nil })
	b.homes.watch = 10 * time.Millisecond
	turn := aTurn(t)
	id, _ := b.homeOf(turn)
	code, err := b.homes.guard(context.Background(), turn.Dir, id, func(ctx context.Context) (int, error) {
		<-ctx.Done()
		return -1, ctx.Err()
	})
	if !errors.Is(err, ErrHomeFull) || code != -1 {
		t.Fatalf("code %d, error %v", code, err)
	}
	if note := homeNoteOf(t, turn.Dir); note.Outcome != "stopped" || note.Used <= note.Limit {
		t.Errorf("%+v", note)
	}
}

// A turn whose caller stopped it is stopped for that, not for the home.
func TestATurnTheServerStopsIsNotBlamedOnTheHome(t *testing.T) {
	b, _ := homeBackend(t, 1000, fixed(10))
	turn := aTurn(t)
	id, _ := b.homeOf(turn)
	ctx, cancel := context.WithCancel(context.Background())
	_, err := b.homes.guard(ctx, turn.Dir, id, func(ctx context.Context) (int, error) {
		cancel()
		<-ctx.Done()
		return -1, ctx.Err()
	})
	if !errors.Is(err, context.Canceled) || errors.Is(err, ErrHomeFull) {
		t.Errorf("%v", err)
	}
}

// What a home takes is kept for the person to see, and the first turn after one ends starts on that count instead of counting
// the home again.
func TestWhatAHomeTakesIsKeptAndNotCountedAgainBeforeAFollowingTurn(t *testing.T) {
	var counts atomic.Int32
	b, _ := homeBackend(t, 1<<20, func(context.Context, string) (int64, error) { counts.Add(1); return 4096, nil })
	if _, known := b.HomeUse(testPerson, ""); known {
		t.Fatal("a home nobody ran in has a count")
	}
	for turns := 1; turns <= 2; turns++ {
		turn := aTurn(t)
		id, _ := b.homeOf(turn)
		if _, err := b.homes.guard(context.Background(), turn.Dir, id, func(context.Context) (int, error) { return 0, nil }); err != nil {
			t.Fatal(err)
		}
	}
	use, known := b.HomeUse("BOB@acme-research.com", "")
	if !known || use.Used != 4096 || use.Limit != 1<<20 || !use.Clean {
		t.Errorf("%+v %v", use, known)
	}
	// The first turn counts at its start and its end, and the second only at its end.
	if counts.Load() != 3 {
		t.Errorf("the home was counted %d times", counts.Load())
	}
}

// A turn that crashed the server leaves the home not clean, so the next turn counts it.
func TestAHomeLeftNotCleanIsCountedAtTheNextTurn(t *testing.T) {
	var counts atomic.Int32
	b, _ := homeBackend(t, 1<<20, func(context.Context, string) (int64, error) { counts.Add(1); return 10, nil })
	turn := aTurn(t)
	id, _ := b.homeOf(turn)
	b.homes.open(id)
	b.homes.write(id, HomeUse{Used: 1, Limit: 1 << 20, Clean: false})
	if _, err := b.homes.guard(context.Background(), turn.Dir, id, func(context.Context) (int, error) { return 0, nil }); err != nil {
		t.Fatal(err)
	}
	if counts.Load() != 2 {
		t.Errorf("counted %d times, once at the start and once at the end", counts.Load())
	}
}

func TestAHomeHasALimitWhenTheOperatorSetsNone(t *testing.T) {
	b, _ := homeBackend(t, 0, fixed(0))
	if b.homes.limit != DefaultHomeLimit {
		t.Errorf("%d", b.homes.limit)
	}
}

func theHome(t *testing.T, b *Backend, root string) (id, home string) {
	t.Helper()
	id = computer.ID(testOrg, "", testPerson)
	mounts, err := b.keptHome(aTurn(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mounts[0].From, "cache.bin"), make([]byte, 100), 0o600); err != nil {
		t.Fatal(err)
	}
	return id, mounts[0].From
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// A person removed from the organization has a mark made for each of their computers, and their home goes with the mark; the
// home of somebody who stayed does not.
func TestAHomeOfAPersonWhoLeftIsRemoved(t *testing.T) {
	b, root := homeBackend(t, 1<<20, fixed(100))
	id, home := theHome(t, b, root)
	other := backend.Turn{Dir: aTurn(t).Dir, Spec: task.Spec{Session: "s"}, Person: "ada@acme-research.com"}
	otherMount, _ := b.keptHome(other)
	b.homes.write(id, HomeUse{Used: 100, Limit: 1 << 20, Clean: true})
	if err := os.WriteFile(b.homes.markFile(id), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	b.homes.removeMarked()
	if exists(home) || exists(b.homes.markFile(id)) || exists(b.homes.usageFile(id)) {
		t.Error("the home of a person who left, or what says so, is still there")
	}
	if !exists(otherMount[0].From) {
		t.Error("the home of somebody who stayed was removed")
	}
}

// A turn of a person being removed is not given a home to put files in again.
func TestAHomeBeingRemovedIsNotMadeAgain(t *testing.T) {
	b, root := homeBackend(t, 1<<20, fixed(100))
	id, home := theHome(t, b, root)
	os.WriteFile(b.homes.markFile(id), nil, 0o600)
	os.RemoveAll(home)
	if _, err := b.keptHome(aTurn(t)); err == nil {
		t.Error("a home was made for a person who is being removed")
	}
	if exists(home) {
		t.Error("the home was made again")
	}
	if _, err := b.homes.guard(context.Background(), aTurn(t).Dir, id, func(context.Context) (int, error) { return 0, nil }); err == nil {
		t.Error("a turn started in a home being removed")
	}
}

// A home with a turn running in it is left for the next look, and removed when the turn has ended.
func TestAHomeWithATurnRunningIsRemovedWhenItEnds(t *testing.T) {
	b, root := homeBackend(t, 1<<20, fixed(100))
	id, home := theHome(t, b, root)
	started, finish, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		turn := aTurn(t)
		b.homes.guard(context.Background(), turn.Dir, id, func(context.Context) (int, error) {
			close(started)
			<-finish
			return 0, nil
		})
	}()
	<-started
	os.WriteFile(b.homes.markFile(id), nil, 0o600)
	b.homes.removeMarked()
	if !exists(home) || !exists(b.homes.markFile(id)) {
		t.Fatal("a home was removed under a running turn")
	}
	close(finish)
	<-done
	b.homes.removeMarked()
	if exists(home) || exists(b.homes.markFile(id)) {
		t.Error("the home was not removed once the turn ended")
	}
}

// Files an agent left that this server may not remove send the home to the trash whole, so it is never kept for a file.
func TestAHomeWithFilesItCannotRemoveGoesToTheTrash(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root may remove anything")
	}
	b, root := homeBackend(t, 1<<20, fixed(100))
	id, home := theHome(t, b, root)
	locked := filepath.Join(home, "locked")
	os.MkdirAll(locked, 0o700)
	os.WriteFile(filepath.Join(locked, "file"), nil, 0o600)
	os.Chmod(locked, 0o500)
	t.Cleanup(func() {
		filepath.WalkDir(filepath.Join(root, "trash"), func(p string, e os.DirEntry, _ error) error { os.Chmod(p, 0o700); return nil })
	})
	os.WriteFile(b.homes.markFile(id), nil, 0o600)

	b.homes.removeMarked()
	if exists(home) {
		t.Fatal("the home was kept for a file that cannot be removed")
	}
	if trash, _ := os.ReadDir(filepath.Join(root, "trash")); len(trash) != 1 {
		t.Errorf("%d homes in the trash", len(trash))
	}
	if exists(b.homes.markFile(id)) {
		t.Error("the mark stayed after the home left")
	}
}

// Only a mark that names a computer's id removes anything.
func TestAMarkThatIsNotAComputersIdRemovesNothing(t *testing.T) {
	b, root := homeBackend(t, 1<<20, fixed(100))
	_, home := theHome(t, b, root)
	os.WriteFile(filepath.Join(root, "..retire"), nil, 0o600)
	os.WriteFile(filepath.Join(root, "nothex!!.retire"), nil, 0o600)
	b.homes.removeMarked()
	if !exists(home) {
		t.Error("a mark that is no id removed a home")
	}
}

// A person whose home is full removes it themselves, and starts again.
func TestAPersonCanResetAFullHome(t *testing.T) {
	b, root := homeBackend(t, 10, fixed(5000))
	id, home := theHome(t, b, root)
	turn := aTurn(t)
	if _, err := b.homes.guard(context.Background(), turn.Dir, id, func(context.Context) (int, error) { return 0, nil }); !errors.Is(err, ErrHomeFull) {
		t.Fatalf("%v", err)
	}
	if err := b.ResetHome(testPerson, ""); err != nil {
		t.Fatal(err)
	}
	if exists(home) {
		t.Error("the home is still there")
	}
	if _, known := b.HomeUse(testPerson, ""); known {
		t.Error("the count of a home that is gone stayed")
	}
}

// The backend removes the homes of people who left as it runs, and stops when it is stopped.
func TestTheBackendTendsHomesUntilItIsStopped(t *testing.T) {
	b, root := homeBackend(t, 1<<20, fixed(100))
	b.homes.tending = 10 * time.Millisecond
	id, home := theHome(t, b, root)
	stop, err := b.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(b.homes.markFile(id), nil, 0o600)
	deadline := time.Now().Add(10 * time.Second)
	for exists(home) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if exists(home) {
		t.Error("the home of a person who left was not removed while the backend ran")
	}
	var stopped sync.WaitGroup
	stopped.Go(stop)
	finished := make(chan struct{})
	go func() { stopped.Wait(); close(finished) }()
	select {
	case <-finished:
	case <-time.After(10 * time.Second):
		t.Fatal("the backend's work did not stop")
	}
}

// A home cannot be reset while a turn runs in it, and says so in a way a page can tell from a failure; a backend with no homes
// says it keeps none.
func TestAHomeWithATurnRunningCannotBeResetAndABackendWithoutHomesSaysSo(t *testing.T) {
	b, root := homeBackend(t, 1<<20, fixed(100))
	id, home := theHome(t, b, root)
	turn := aTurn(t)
	_, err := b.homes.guard(context.Background(), turn.Dir, id, func(context.Context) (int, error) {
		if err := b.ResetHome(testPerson, ""); !errors.Is(err, ErrHomeBusy) {
			t.Errorf("resetting a home a turn runs in answered %v", err)
		}
		return 0, nil
	})
	if err != nil || !exists(home) {
		t.Fatalf("the turn did not run in its home: %v", err)
	}
	if !b.KeepsHomes() || New(Config{}).KeepsHomes() {
		t.Error("KeepsHomes does not say whether homes are kept")
	}
}
