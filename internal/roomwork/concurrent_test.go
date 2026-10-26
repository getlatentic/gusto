package roomwork

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// The turns of a room run at once, each writing to its copy, and finish in any order.
func TestManyTurnsAtOnceLoseNothing(t *testing.T) {
	const agents = 12
	files := map[string]string{"shared.md": "start"}
	for i := range agents {
		files[fmt.Sprintf("own/%d.md", i)] = "start"
	}
	hub, in := aRoom(t, files)
	turns := make([]*Turn, agents)
	for i := range turns {
		turns[i] = begin(t, hub, in, fmt.Sprintf("Agent%d", i))
	}

	var wg sync.WaitGroup
	outcomes := make([]Outcome, agents)
	for i, turn := range turns {
		wg.Add(1)
		go func() {
			defer wg.Done()
			write(t, turn.dest, "shared.md", fmt.Sprintf("version %d", i))
			write(t, turn.dest, fmt.Sprintf("own/%d.md", i), fmt.Sprintf("by %d", i))
			write(t, turn.dest, fmt.Sprintf("new/%d.md", i), "fresh")
			got, err := turn.Merge(context.Background())
			if err != nil {
				t.Error(err)
			}
			outcomes[i] = got
		}()
	}
	wg.Wait()

	for i := range agents {
		if read(t, in.Folder, fmt.Sprintf("own/%d.md", i)) != fmt.Sprintf("by %d", i) || read(t, in.Folder, fmt.Sprintf("new/%d.md", i)) != "fresh" {
			t.Errorf("agent %d lost work\n%s", i, listing(t, in.Folder))
		}
	}
	conflicts, versions := 0, map[string]bool{read(t, in.Folder, "shared.md"): true}
	for _, got := range outcomes {
		conflicts += len(got.Conflicts)
	}
	for i := range agents {
		side := filepath.Join(in.Folder, fmt.Sprintf("shared.agent%d.md", i))
		if text, err := os.ReadFile(side); err == nil {
			versions[string(text)] = true
		}
	}
	if conflicts != agents-1 || len(versions) != agents {
		t.Errorf("%d conflicts and %d kept versions of the shared file, not %d of each", conflicts, len(versions), agents-1)
	}
	noStaging(t, in)
}

// A room's turns begin while others merge: a copy made then is whole.
func TestACopyMadeWhileAnotherTurnMergesHoldsWholeFiles(t *testing.T) {
	big := strings.Repeat("a", 1<<16)
	hub, in := aRoom(t, map[string]string{"data.txt": big})
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for round := 0; ; round++ {
			select {
			case <-stop:
				return
			default:
			}
			turn, err := hub.Begin(context.Background(), in, filepath.Join(t.TempDir(), "w"), "Writer")
			if err != nil {
				t.Error(err)
				return
			}
			text := strings.Repeat(string(rune('b'+round%20)), 1<<16)
			write(t, turn.dest, "data.txt", text)
			if _, err := turn.Merge(context.Background()); err != nil {
				t.Error(err)
			}
			turn.Close()
		}
	}()
	deadline := time.Now().Add(700 * time.Millisecond)
	for time.Now().Before(deadline) {
		turn := begin(t, hub, in, "Reader")
		text := read(t, turn.dest, "data.txt")
		if len(text) != 1<<16 || strings.Trim(text, text[:1]) != "" {
			t.Fatalf("the copy holds a torn file: %d bytes, starting %q", len(text), text[:8])
		}
		turn.Close()
	}
	close(stop)
	wg.Wait()
}

// A formatter run over the whole folder rewrites files without changing them. That is not a change to a file another turn did
// change.
func TestARewriteThatChangedNothingIsNotAConflict(t *testing.T) {
	hub, in := aRoom(t, map[string]string{"code.go": "package a", "other.go": "package b"})
	ada, bea := begin(t, hub, in, "Ada"), begin(t, hub, in, "Bea")
	write(t, ada.dest, "code.go", "package a // Ada")
	time.Sleep(5 * time.Millisecond)
	write(t, bea.dest, "code.go", "package a")
	write(t, bea.dest, "other.go", "package b")
	write(t, bea.dest, "extra.go", "package c")
	merge(t, ada)
	got := merge(t, bea)
	if len(got.Conflicts) != 0 {
		t.Fatalf("a formatter's rewrite conflicted: %+v", got.Conflicts)
	}
	if read(t, in.Folder, "code.go") != "package a // Ada" || read(t, in.Folder, "extra.go") != "package c" {
		t.Errorf("%s", listing(t, in.Folder))
	}
}

// What a merge keeps for turns that began earlier is let go when they end.
func TestWhatAMergeKeptForOtherTurnsIsLetGoWhenTheyEnd(t *testing.T) {
	hub, in := aRoom(t, map[string]string{"a.md": "v1"})
	ada, bea := begin(t, hub, in, "Ada"), begin(t, hub, in, "Bea")
	write(t, ada.dest, "a.md", "v2")
	merge(t, ada)
	if entries, _ := os.ReadDir(priorDir(in)); len(entries) != 1 {
		t.Fatalf("%d versions kept for Bea", len(entries))
	}
	ada.Close()
	bea.Close()
	if exists(priorDir(in)) {
		t.Error("versions are still kept after every turn ended")
	}
	if len(hub.rooms) != 0 {
		t.Error("the room is still tracked")
	}
}

func TestATurnWaitingForAMergeStopsWhenItIsStopped(t *testing.T) {
	hub, in := aRoom(t, map[string]string{"a.md": "v1"})
	ada := begin(t, hub, in, "Ada")
	write(t, ada.dest, "a.md", "v2")
	release, err := hub.hold(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := ada.Merge(ctx); done <- err }()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("the merge went ahead without the guard")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a stopped merge kept waiting")
	}
	if read(t, in.Folder, "a.md") != "v1" {
		t.Error("the folder changed")
	}
}
